// Package pandiff compares the engine with SPIN's pan on one Promela model
// (plan 14 §8.1, §9 row G1): it runs `spin -a -o1 -o2 -o3`, compiles pan
// with `gcc -O2 -DNOREDUCE`, runs `./pan -c0` for the state count, `./pan`
// for the first error, `./pan -d` for the statement table, and sets the
// results against the engine's report and IR.
//
// What "agree" means here:
//
//   - verdict: pan found an error ⇔ some property is violated;
//   - error class: pan's first-error line ("assertion violated", "invalid
//     end state", "block in d_step seq" …) against the engine's violated
//     property (assert ⇔ assertion violated, deadlock ⇔ invalid end state)
//     or invalid-model reason;
//   - state count: pan's "states, stored" under -c0 against the engine's
//     stored states of a complete sweep (--sweep); not compared when the
//     engine's search was not complete (invalid-model stops it);
//   - statement table: per proctype, the multiset of source lines of pan's
//     transitions (pan -d) against the multiset of source lines of the IR
//     edges of one instance of that proctype. The texts are printed side by
//     side for a human; the comparison is on lines because pan rewrites
//     text (`cnt++` → `cnt = (cnt+1)`, `skip` → `(1)`).
//
// A never claim is skipped on both sides (G1 does not execute it).
package pandiff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"modelcheck/explore"
	"modelcheck/frontend/promela"
	"modelcheck/ir"
)

// PanTransition is one line of pan -d.
type PanTransition struct {
	From, To int
	Line     int
	Text     string
	Flags    string
}

// PanResult is what SPIN/pan said.
type PanResult struct {
	Stored, Matched, Errors int
	// Class is "no error", "assertion violated", "invalid end state", or
	// pan's own words for anything else.
	Class      string
	FirstError string
	// Incomplete: pan itself did not finish (depth or memory limit), so
	// Stored is a partial count and is not compared.
	Incomplete bool
	// Statements maps a proctype name (":init:" for init) to its
	// transitions in pan -d order; Order lists the names as pan printed them.
	Statements map[string][]PanTransition
	Order      []string
	SpinOutput string
}

// EngineResult is what the engine said.
type EngineResult struct {
	States   int
	Complete bool
	Class    string
	Reason   string
	// Statements maps a proctype name (":init:" for init) to the IR edges
	// of its first instance: (line, text) in edge order. Line is the line
	// pan -d prints (promela.Result.PanLines): it differs from the edge's
	// origin only for the first statement of an if/do option.
	Statements map[string][]EngineEdge
	Order      []string
	Warnings   []string
	Model      *ir.Model
}

// EngineEdge is one IR edge for the table.
type EngineEdge struct {
	Line int
	Text string
}

// panDepth is the search-depth flag every pan run gets (pan's default is
// 10000, which is smaller than the deepest model of the corpus).
const panDepth = "-m1000000"

// Tools locates spin and gcc.
type Tools struct {
	Spin string // default "spin"
	GCC  string // default "gcc"
}

// Available reports whether spin and gcc are on PATH.
func (t Tools) Available() bool {
	_, err1 := exec.LookPath(t.spin())
	_, err2 := exec.LookPath(t.gcc())
	return err1 == nil && err2 == nil
}

func (t Tools) spin() string {
	if t.Spin == "" {
		return "spin"
	}
	return t.Spin
}

func (t Tools) gcc() string {
	if t.GCC == "" {
		return "gcc"
	}
	return t.GCC
}

// RunSpin runs the SPIN pipeline in dir (created by the caller).
func (t Tools) RunSpin(ctx context.Context, model string, defines []string, dir string) (*PanResult, error) {
	src, err := os.ReadFile(model)
	if err != nil {
		return nil, err
	}
	base := filepath.Base(model)
	local := filepath.Join(dir, base)
	if err := os.WriteFile(local, src, 0o644); err != nil {
		return nil, err
	}
	args := []string{"-a", "-o1", "-o2", "-o3"}
	for _, d := range defines {
		args = append(args, "-D"+d)
	}
	args = append(args, base)
	out, err := run(ctx, dir, t.spin(), args...)
	res := &PanResult{Statements: map[string][]PanTransition{}, SpinOutput: out}
	if err != nil || strings.Contains(out, "Error") {
		return nil, fmt.Errorf("spin -a failed: %s%v", firstLines(out, 3), errNote(err))
	}
	if out, err := run(ctx, dir, t.gcc(), "-O2", "-DNOREDUCE", "-o", "pan", "pan.c"); err != nil {
		return nil, fmt.Errorf("gcc failed: %s%v", firstLines(out, 3), errNote(err))
	}
	pan := filepath.Join(dir, "pan")
	// -m raises pan's default depth limit of 10000: CH15/client_server.pml
	// reaches depth 31309, and a search cut off by the default would report
	// a partial count that is not comparable with the engine's.
	c0, _ := run(ctx, dir, pan, "-c0", panDepth)
	if res.Stored, res.Matched, res.Errors, err = ParseC0(c0); err != nil {
		return nil, fmt.Errorf("pan -c0: %w\n%s", err, firstLines(c0, 8))
	}
	res.Incomplete = strings.Contains(c0, "Search not completed") || strings.Contains(c0, "max search depth too small")
	first, _ := run(ctx, dir, pan, panDepth)
	res.FirstError, res.Class = ParseFirst(first, res.Errors)
	d, _ := run(ctx, dir, pan, "-d")
	res.Statements, res.Order = ParseD(d)
	return res, nil
}

func errNote(err error) string {
	if err == nil {
		return ""
	}
	return " (" + err.Error() + ")"
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " | ")
}

func run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

var (
	reStored  = regexp.MustCompile(`(?m)^\s*(\d+) states, stored`)
	reMatched = regexp.MustCompile(`(?m)^\s*(\d+) states, matched`)
	reErrors  = regexp.MustCompile(`errors: (\d+)`)
	reFirst   = regexp.MustCompile(`(?m)^pan:\d+: (.*?)(?: \(at depth \d+\))?$`)
	reSection = regexp.MustCompile(`^(?:proctype (\S+)|(init)|claim (\S+))\s*$`)
	reTrans   = regexp.MustCompile(`^\s*state\s+(\d+) -\(tr\s+\d+\)-> state\s+(\d+)\s+\[id\s+\d+ tp\s+\d+\]\s+\[([^\]]+)\]\s+\S+?:(\d+) => (.*)$`)
)

// ParseC0 reads the summary of pan -c0.
func ParseC0(out string) (stored, matched, errors int, err error) {
	m := reStored.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("no \"states, stored\" line")
	}
	stored, _ = strconv.Atoi(m[1])
	if m := reMatched.FindStringSubmatch(out); m != nil {
		matched, _ = strconv.Atoi(m[1])
	}
	m = reErrors.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, 0, fmt.Errorf("no \"errors:\" line")
	}
	errors, _ = strconv.Atoi(m[1])
	return stored, matched, errors, nil
}

// ParseFirst classifies the first error of a plain pan run.
func ParseFirst(out string, errors int) (first, class string) {
	m := reFirst.FindStringSubmatch(out)
	if m == nil {
		if errors == 0 {
			return "", "no error"
		}
		return "", fmt.Sprintf("%d error(s), no pan: line", errors)
	}
	first = m[1]
	switch {
	case strings.HasPrefix(first, "assertion violated"):
		return first, "assertion violated"
	case strings.HasPrefix(first, "invalid end state"):
		return first, "invalid end state"
	case strings.HasPrefix(first, "block in d_step seq"):
		return first, "block in d_step seq"
	}
	return first, first
}

// ParseD reads the statement table of pan -d.
func ParseD(out string) (map[string][]PanTransition, []string) {
	stmts := map[string][]PanTransition{}
	var order []string
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		if m := reSection.FindStringSubmatch(line); m != nil {
			switch {
			case m[1] != "":
				cur = m[1]
			case m[2] != "":
				cur = ":init:"
			default:
				cur = "claim " + m[3]
			}
			order = append(order, cur)
			if _, ok := stmts[cur]; !ok {
				stmts[cur] = nil
			}
			continue
		}
		if m := reTrans.FindStringSubmatch(line); m != nil && cur != "" {
			if strings.TrimSpace(m[5]) == ".(goto)" {
				// pan's join pseudo-transition at the end of an if/do inside an
				// atomic sequence: an unstored atomic step with no statement
				// behind it; the IR joins locations directly.
				continue
			}
			from, _ := strconv.Atoi(m[1])
			to, _ := strconv.Atoi(m[2])
			ln, _ := strconv.Atoi(m[4])
			stmts[cur] = append(stmts[cur], PanTransition{From: from, To: to, Flags: m[3], Line: ln, Text: strings.TrimSpace(m[5])})
		}
	}
	return stmts, order
}

// RunEngine parses model with the Promela frontend and sweeps it.
func RunEngine(ctx context.Context, model string, defines []string, maxStates int) (*EngineResult, error) {
	src, err := os.ReadFile(model)
	if err != nil {
		return nil, err
	}
	res, perr := promela.Parse(src, model, defines)
	if perr != nil {
		return nil, perr
	}
	if maxStates <= 0 {
		maxStates = 5_000_000
	}
	r, err := explore.Run(ctx, res.Model, explore.Options{Budget: explore.Budget{MaxStates: maxStates}, Sweep: true})
	if err != nil {
		return nil, err
	}
	er := &EngineResult{States: r.States, Complete: r.Complete, Warnings: res.Warnings, Model: res.Model, Statements: map[string][]EngineEdge{}}
	er.Class = "no error"
	for _, o := range r.Outcomes {
		switch o.Status {
		case explore.Violated:
			c := "violated " + o.Property.ID
			switch o.Property.Kind {
			case ir.KindAssert:
				c = "assertion violated"
			case ir.KindDeadlock:
				c = "invalid end state"
			}
			if er.Class == "no error" {
				er.Class = c
			} else if !strings.Contains(er.Class, c) {
				er.Class += " + " + c
			}
		case explore.InvalidModel:
			if er.Class == "no error" || strings.HasPrefix(er.Class, "invalid-model") {
				er.Class = "invalid-model"
				er.Reason = o.Reason
				if strings.HasPrefix(o.Reason, "block in d_step seq") {
					er.Class = "block in d_step seq"
				}
			}
		case explore.Inconclusive:
			if er.Class == "no error" {
				er.Class = "inconclusive: " + o.Reason
			}
		}
	}
	seen := map[string]bool{}
	for _, p := range res.Model.Processes {
		if p.Claim || p.Origin == nil {
			continue
		}
		name := p.Origin.Name
		if name == "init" {
			name = ":init:"
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		er.Order = append(er.Order, name)
		pan := res.PanLines[p.Name]
		for i, e := range p.Edges {
			ln := 0
			if e.Origin != nil {
				ln = e.Origin.Line
			}
			if i < len(pan) {
				ln = pan[i]
			}
			er.Statements[name] = append(er.Statements[name], EngineEdge{Line: ln, Text: e.Text})
		}
	}
	return er, nil
}

// Row is one compared quantity.
type Row struct {
	Name   string
	Pan    string
	Engine string
	// Agree is false only for a real disagreement; a quantity that could
	// not be compared has Agree = true and Skipped = true, and the table
	// prints "n/a", not "agree".
	Agree   bool
	Skipped bool
	Note    string
}

// StmtRow is the per-proctype statement comparison.
type StmtRow struct {
	Proctype   string
	PanLines   []int
	EngLines   []int
	Agree      bool
	PanTexts   []string
	EngTexts   []string
	OnlyPan    []int
	OnlyEngine []int
}

// Comparison is the whole result.
type Comparison struct {
	Rows      []Row
	Stmts     []StmtRow
	Agree     bool // verdict, class and state count
	StmtAgree bool
}

// Compare sets pan against the engine.
func Compare(pan *PanResult, eng *EngineResult) *Comparison {
	c := &Comparison{Agree: true, StmtAgree: true}
	add := func(name, p, e string, agree bool, note string) {
		c.Rows = append(c.Rows, Row{Name: name, Pan: p, Engine: e, Agree: agree, Note: note})
		if !agree {
			c.Agree = false
		}
	}
	panErr := pan.Errors > 0
	engErr := eng.Class != "no error"
	add("verdict", verdictWord(panErr), verdictWord(engErr), panErr == engErr, "")
	classAgree := pan.Class == eng.Class || (pan.Errors == 0 && eng.Class == "no error")
	note := ""
	if !classAgree && eng.Class == "invalid-model" {
		note = "engine: " + eng.Reason
	}
	add("error class", pan.Class, eng.Class, classAgree, note)
	switch {
	case pan.Incomplete:
		add("states stored", fmt.Sprintf("%d (pan search not completed)", pan.Stored), strconv.Itoa(eng.States), true, "not compared: pan hit its depth or memory limit")
		c.Rows[len(c.Rows)-1].Skipped = true
	case eng.Complete:
		add("states stored", strconv.Itoa(pan.Stored), strconv.Itoa(eng.States), pan.Stored == eng.States, "pan -c0 vs engine --sweep")
	default:
		add("states stored", strconv.Itoa(pan.Stored), fmt.Sprintf("%d (search not complete)", eng.States), true, "not compared: the engine stopped early")
		c.Rows[len(c.Rows)-1].Skipped = true
	}
	names := append([]string(nil), pan.Order...)
	for _, n := range eng.Order {
		if _, ok := pan.Statements[n]; !ok {
			names = append(names, n)
		}
	}
	for _, n := range names {
		if strings.HasPrefix(n, "claim") {
			continue
		}
		sr := StmtRow{Proctype: n}
		for _, t := range pan.Statements[n] {
			sr.PanLines = append(sr.PanLines, t.Line)
			sr.PanTexts = append(sr.PanTexts, fmt.Sprintf("%d: %s", t.Line, t.Text))
		}
		for _, e := range eng.Statements[n] {
			sr.EngLines = append(sr.EngLines, e.Line)
			sr.EngTexts = append(sr.EngTexts, fmt.Sprintf("%d: %s", e.Line, e.Text))
		}
		sr.OnlyPan, sr.OnlyEngine = multisetDiff(sr.PanLines, sr.EngLines)
		sr.Agree = len(sr.OnlyPan) == 0 && len(sr.OnlyEngine) == 0
		if !sr.Agree {
			c.StmtAgree = false
		}
		c.Stmts = append(c.Stmts, sr)
	}
	return c
}

func verdictWord(err bool) string {
	if err {
		return "error found"
	}
	return "no error"
}

func multisetDiff(a, b []int) (onlyA, onlyB []int) {
	count := map[int]int{}
	for _, x := range a {
		count[x]++
	}
	for _, x := range b {
		count[x]--
	}
	keys := make([]int, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		for i := 0; i < count[k]; i++ {
			onlyA = append(onlyA, k)
		}
		for i := 0; i > count[k]; i-- {
			onlyB = append(onlyB, k)
		}
	}
	return onlyA, onlyB
}

// Table renders the comparison.
func (c *Comparison) Table(model string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", model)
	fmt.Fprintf(&b, "  %-14s %-32s %-40s %s\n", "quantity", "pan", "engine", "")
	for _, r := range c.Rows {
		mark := "agree"
		if r.Skipped {
			mark = "n/a"
		} else if !r.Agree {
			mark = "DISAGREE"
		}
		if r.Note != "" {
			mark += "  (" + r.Note + ")"
		}
		fmt.Fprintf(&b, "  %-14s %-32s %-40s %s\n", r.Name, r.Pan, r.Engine, mark)
	}
	fmt.Fprintf(&b, "  statement table (source lines of transitions per proctype):\n")
	for _, s := range c.Stmts {
		mark := "agree"
		if !s.Agree {
			mark = fmt.Sprintf("DISAGREE: only pan %v, only engine %v", s.OnlyPan, s.OnlyEngine)
		}
		fmt.Fprintf(&b, "    %-14s pan %d transition(s), engine %d edge(s): %s\n", s.Proctype, len(s.PanLines), len(s.EngLines), mark)
		if !s.Agree {
			fmt.Fprintf(&b, "      pan:    %s\n", strings.Join(s.PanTexts, " | "))
			fmt.Fprintf(&b, "      engine: %s\n", strings.Join(s.EngTexts, " | "))
		}
	}
	return b.String()
}

// Run does everything for one model in a fresh temp dir.
func Run(ctx context.Context, tools Tools, model string, defines []string, keep bool) (*Comparison, *PanResult, *EngineResult, error) {
	dir, err := os.MkdirTemp("", "pandiff-")
	if err != nil {
		return nil, nil, nil, err
	}
	if !keep {
		defer os.RemoveAll(dir)
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
	}
	pan, err := tools.RunSpin(ctx, model, defines, dir)
	if err != nil {
		return nil, nil, nil, err
	}
	eng, err := RunEngine(ctx, model, defines, 0)
	if err != nil {
		return nil, pan, nil, err
	}
	return Compare(pan, eng), pan, eng, nil
}
