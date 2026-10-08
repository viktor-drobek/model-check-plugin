// Package pormut is the mutation harness of the partial-order reduction
// (performance plan, step 6, section 7.4).
//
// A mutant is an exact textual replacement in a source file of the engine,
// kept as data (mutants.json). The harness copies the engine tree into a
// scratch directory, applies one mutant at a time to the copy, and runs the
// reduction's oracles on it, layer by layer:
//
//	O1        the verdict differential (por_oracle_test.go, por_random_test.go),
//	          also with tight limits of the proviso's chain walk
//	O2        the semantic audit of the ample sets (por_audit_test.go)
//	O3        the acyclicity audit of the cycle proviso (por_acyclic_test.go)
//	directed  every other TestPOR test of the explore package: the hand-built
//	          traps, the analysis tests, the search tests
//
// A layer that fails on a mutant has killed it. Before anything is copied or
// run, the harness checks itself: every anchor of the list (the selected
// mutants or not) must apply, and every layer's -run regex must match a test,
// because go test calls a regex that matches nothing "ok [no tests to run]";
// all that fail are listed at once. The unmutated copy is then run first, with
// the same layers and the same sizes, and the harness refuses to go on when any
// layer is red there: an earlier harness counted every mutant as killed by a
// baseline that was already failing. A mutant whose source does not build is
// reported as such and is never counted as killed.
//
// Every mutant says what is expected of it. "killed" (the default) must be
// killed by O1 or O2 or O3 on the generators alone; a mutant that only a
// directed test kills means the generators have a blind spot, and is reported
// as "directed only" (a failure unless the mutant says so). "equivalent" and
// "power" mutants (they change nothing a verdict can see, or only lose
// reduction power) may survive, and say why in their note; if one is killed the
// harness reports a surprise. A "pinned" mutant is a refusal that no verdict
// depends on (the argument is in its note): the oracles stay green and a
// directed test that pins the refusal kills it.
//
// Usage, from the engine directory (the harness runs go test, so it should run
// under ulimit -v and a timeout, one job at a time):
//
//	go run ./cmd/pormut -scratch DIR [-models N] [-workers W] [-only id,id] [-list]
package pormut

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Mutant is one textual replacement.
type Mutant struct {
	ID   string `json:"id"`
	Rule string `json:"rule"` // the rule of the plan it belongs to: A, R, P, base, proviso ...
	// File is relative to the engine directory; "explore/por.go" when empty.
	File string `json:"file,omitempty"`
	// Old must occur in the file exactly once; New replaces it.
	Old string `json:"old"`
	New string `json:"new"`
	// Gens limits the oracle runs to these generators (MCD_POR_GEN); empty is all.
	Gens string `json:"gens,omitempty"`
	// Expect is "killed" (default), "equivalent", "power" or "pinned".
	Expect string `json:"expect,omitempty"`
	// With makes the entry a combination: the ids of two or more other mutants
	// of the list that are applied to the same copy, in this order. A
	// combination has no Old and New of its own. Two mutants that are each
	// harmless can be a verdict together (the clauses of a rule may cover for
	// each other), and only a combination can be put through the harness for it.
	With []string `json:"with,omitempty"`
	// Note says what the mutant removes, and why it survives when it may.
	Note string `json:"note"`
}

// file is the mutant's file with the default applied.
func (m *Mutant) file() string {
	if m.File == "" {
		return "explore/por.go"
	}
	return m.File
}

func (m *Mutant) expect() string {
	if m.Expect == "" {
		return "killed"
	}
	return m.Expect
}

// pkgUnderTest is the package whose tests the layers run.
const pkgUnderTest = "./explore"

// Layer is one oracle layer and the go test arguments that run it.
type Layer struct {
	Name string
	Args []string
}

const oracleTests = `TestPORAgreesWithTheFullSearchOnRandomModels|TestPORDifferentialOnTheGenerators|TestPORDifferentialWithTightChainLimits|TestPORAuditOfTheEligibleProcesses|TestPORAcyclicityOfTheReducedGraph|TestPORAcyclicityWithTightChainLimits`

// Layers are the layers in the order they are run.
var Layers = []Layer{
	{"O1", []string{"-run", `^(TestPORAgreesWithTheFullSearchOnRandomModels|TestPORDifferentialOnTheGenerators|TestPORDifferentialWithTightChainLimits)$`}},
	{"O2", []string{"-run", `^TestPORAuditOfTheEligibleProcesses$`}},
	{"O3", []string{"-run", `^(TestPORAcyclicityOfTheReducedGraph|TestPORAcyclicityWithTightChainLimits)$`}},
	{"directed", []string{"-run", `^TestPOR`, "-skip", `^(` + oracleTests + `)$`}},
}

// LoadMutants reads the mutant list.
func LoadMutants(path string) ([]Mutant, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ms []Mutant
	if err := json.Unmarshal(b, &ms); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	seen := map[string]bool{}
	for i := range ms {
		m := &ms[i]
		if m.ID == "" || seen[m.ID] {
			return nil, fmt.Errorf("%s: mutant %d: empty or repeated id %q", path, i, m.ID)
		}
		seen[m.ID] = true
		switch m.expect() {
		case "killed":
		case "equivalent", "power", "pinned":
			if strings.TrimSpace(m.Note) == "" {
				return nil, fmt.Errorf("%s: mutant %s may survive and must say why in its note", path, m.ID)
			}
		default:
			return nil, fmt.Errorf("%s: mutant %s: unknown expect %q", path, m.ID, m.Expect)
		}
		if len(m.With) > 0 {
			continue // a combination: checked below, once every id is known
		}
		if m.Old == "" || m.Old == m.New {
			return nil, fmt.Errorf("%s: mutant %s: old must be non-empty and differ from new", path, m.ID)
		}
	}
	byID := index(ms)
	for i := range ms {
		m := &ms[i]
		if len(m.With) == 0 {
			continue
		}
		if len(m.With) < 2 || m.Old != "" || m.New != "" {
			return nil, fmt.Errorf("%s: mutant %s: a combination names two or more mutants in with and has no old and new of its own", path, m.ID)
		}
		parts := map[string]bool{}
		for _, id := range m.With {
			part, ok := byID[id]
			switch {
			case !ok:
				return nil, fmt.Errorf("%s: mutant %s: with names %q, which is not in the list", path, m.ID, id)
			case len(part.With) > 0:
				return nil, fmt.Errorf("%s: mutant %s: with names the combination %q: combinations do not nest", path, m.ID, id)
			case parts[id]:
				return nil, fmt.Errorf("%s: mutant %s: with names %q twice", path, m.ID, id)
			}
			parts[id] = true
		}
	}
	return ms, nil
}

// parts returns the replacements the entry stands for: itself, or the mutants
// of its combination, in order.
func (m *Mutant) parts(byID map[string]*Mutant) ([]*Mutant, error) {
	if len(m.With) == 0 {
		return []*Mutant{m}, nil
	}
	var out []*Mutant
	for _, id := range m.With {
		part, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("combination %s: with names %q, which is not in the list", m.ID, id)
		}
		out = append(out, part)
	}
	return out, nil
}

func index(ms []Mutant) map[string]*Mutant {
	byID := map[string]*Mutant{}
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	return byID
}

// patch is one file under a mutant: what it was and what it becomes.
type patch struct{ orig, mut []byte }

// applyParts applies the parts one after another to the files read gives, each
// part to the file as the earlier ones left it, and returns the patch of every
// file they touch. A part that does not apply is an error naming it.
func applyParts(parts []*Mutant, read func(rel string) ([]byte, error)) (map[string]*patch, error) {
	out := map[string]*patch{}
	for _, part := range parts {
		pt, ok := out[part.file()]
		if !ok {
			src, err := read(part.file())
			if err != nil {
				return nil, fmt.Errorf("mutant %s: %w", part.ID, err)
			}
			pt = &patch{orig: src, mut: src}
			out[part.file()] = pt
		}
		mut, err := part.Apply(pt.mut)
		if err != nil {
			return nil, err
		}
		pt.mut = mut
	}
	return out, nil
}

// Apply returns src with the mutant applied, or an error when its anchor does
// not occur exactly once.
func (m *Mutant) Apply(src []byte) ([]byte, error) {
	n := bytes.Count(src, []byte(m.Old))
	if n != 1 {
		return nil, fmt.Errorf("mutant %s: the anchor occurs %d times in %s, want exactly 1", m.ID, n, m.file())
	}
	return bytes.Replace(src, []byte(m.Old), []byte(m.New), 1), nil
}

// CopyEngine copies the engine tree to dst, without bin (the tracked release
// binaries: a scratch copy does not need them and must never touch them). The
// engine directory holds no version control files, so none is skipped: the .git
// directory, or the .git file of a worktree, is at the root of the repository,
// outside the tree that is copied.
func CopyEngine(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "bin" || strings.HasPrefix(rel, "bin"+string(filepath.Separator)) {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// Config sizes a run.
type Config struct {
	Models  int           // MCD_POR_MODELS
	Workers int           // MCD_POR_WORKERS
	Timeout time.Duration // per layer
	Seed    int64         // MCD_POR_SEED, 0 for each generator's own default
	Keep    bool          // keep the scratch copy
}

// Verdict of one layer on one tree.
type Verdict int

// The verdicts of a layer.
const (
	Green Verdict = iota
	Red
	BuildFailed
	TimedOut
)

func (v Verdict) String() string {
	return [...]string{"green", "RED", "build failed", "timeout"}[v]
}

// RunLayer runs one layer's tests in dir.
func RunLayer(ctx context.Context, dir string, l Layer, gens string, cfg Config) (Verdict, string) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	// go test's own limit is ten minutes for the whole binary: give it the
	// layer's.
	args := append([]string{"test", "-count=1", "-p", "1", "-timeout", cfg.Timeout.String(), pkgUnderTest}, l.Args...)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"MCD_POR_MODELS="+strconv.Itoa(cfg.Models),
		"MCD_POR_WORKERS="+strconv.Itoa(cfg.Workers),
		"MCD_POR_GEN="+gens,
	)
	if cfg.Seed > 0 {
		cmd.Env = append(cmd.Env, "MCD_POR_SEED="+strconv.FormatInt(cfg.Seed, 10))
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := out.String()
	switch {
	case err == nil:
		return Green, text
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return TimedOut, text
	case strings.Contains(text, "[build failed]") || strings.Contains(text, "cannot use") || strings.Contains(text, "undefined:") || strings.Contains(text, "declared and not used") || strings.Contains(text, "syntax error"):
		return BuildFailed, text
	}
	return Red, text
}

// Result is what happened to one mutant.
type Result struct {
	Mutant  Mutant
	Layers  map[string]Verdict
	Outcome string // killed, directed only, SURVIVED, survived (as expected), SURPRISE, build failed
	Bad     bool   // a failure of the harness's contract
	Detail  string
}

// Classify turns the layer verdicts of a mutant into an outcome.
func Classify(m Mutant, v map[string]Verdict) (outcome string, bad bool) {
	for _, l := range Layers {
		if v[l.Name] == BuildFailed {
			return "build failed", true
		}
		if v[l.Name] == TimedOut {
			return "timeout", true
		}
	}
	oracle := v["O1"] == Red || v["O2"] == Red || v["O3"] == Red
	directed := v["directed"] == Red
	switch m.expect() {
	case "killed":
		switch {
		case oracle:
			return "killed", false
		case directed:
			return "directed only", true
		}
		return "SURVIVED", true
	case "pinned":
		// A check that no verdict can tell (the oracles stay green) and that a
		// directed test pins by its reason: the test must still kill it.
		switch {
		case oracle:
			return "SURPRISE (killed by an oracle: not merely pinned)", true
		case directed:
			return "pinned by a directed test", false
		}
		return "SURVIVED (a pinned mutant must be killed by its directed test)", true
	default:
		if oracle || directed {
			return "SURPRISE (expected to survive: " + m.expect() + ")", true
		}
		return "survived (" + m.expect() + ")", false
	}
}

// checkOnly refuses an -only id that names no mutant: a typo would otherwise
// select nothing and the run would report an empty, successful harness.
func checkOnly(ms []Mutant, only map[string]bool) error {
	known := map[string]bool{}
	for _, m := range ms {
		known[m.ID] = true
	}
	var unknown []string
	for id := range only {
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("-only names no mutant of the list: %s", strings.Join(unknown, ", "))
}

// CheckAnchors applies every mutant of the list, not only the selected ones, to
// its file under engineDir (in memory: nothing is written) and reports all the
// ones that do not apply at once. An anchor goes stale when the code it patches
// moves; applied lazily, after the baseline and for the selected mutants only, a
// stale anchor stayed unseen for the others (b4 and a7b were broken for three
// commits) and aborted a selected run after its whole baseline.
func CheckAnchors(engineDir string, ms []Mutant) error {
	files := map[string][]byte{}
	read := func(rel string) ([]byte, error) {
		if src, ok := files[rel]; ok {
			return src, nil
		}
		src, err := os.ReadFile(filepath.Join(engineDir, rel))
		if err == nil {
			files[rel] = src
		}
		return src, err
	}
	byID := index(ms)
	var bad []string
	for i := range ms {
		m := &ms[i]
		parts, err := m.parts(byID)
		if err == nil {
			_, err = applyParts(parts, read)
		}
		switch {
		case err == nil:
		case len(m.With) > 0:
			bad = append(bad, fmt.Sprintf("combination %s: %v", m.ID, err))
		default:
			bad = append(bad, err.Error())
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%d of %d mutants do not apply, so nothing was run:\n  %s", len(bad), len(ms), strings.Join(bad, "\n  "))
	}
	return nil
}

// layerPatterns returns the -run and -skip regular expressions of a layer, the
// way go test reads them (no -run: every test; no -skip: none).
func layerPatterns(l Layer) (run, skip string) {
	run = "."
	for i := 0; i+1 < len(l.Args); i++ {
		switch l.Args[i] {
		case "-run":
			run = l.Args[i+1]
		case "-skip":
			skip = l.Args[i+1]
		}
	}
	return run, skip
}

// CheckLayers lists the tests of the explore package in dir and reports every
// layer that would run none: go test calls a -run regex that matches nothing "ok
// [no tests to run]", which the harness counted as green. A layer that is empty
// kills nothing, and with it the baseline is green however the oracles are
// named.
func CheckLayers(ctx context.Context, dir string, layers []Layer, cfg Config) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-list", ".", pkgUnderTest)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("listing the tests of %s to check the layers: %v\n%s", pkgUnderTest, err, tail(out.String(), 20))
	}
	var tests []string
	for _, l := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(l, "Test") {
			tests = append(tests, strings.TrimSpace(l))
		}
	}
	var bad []string
	for _, l := range layers {
		run, skip := layerPatterns(l)
		runRe, err := regexp.Compile(run)
		if err != nil {
			bad = append(bad, fmt.Sprintf("layer %s: the -run regex %q does not parse: %v", l.Name, run, err))
			continue
		}
		var skipRe *regexp.Regexp
		if skip != "" {
			if skipRe, err = regexp.Compile(skip); err != nil {
				bad = append(bad, fmt.Sprintf("layer %s: the -skip regex %q does not parse: %v", l.Name, skip, err))
				continue
			}
		}
		n := 0
		for _, name := range tests {
			if runRe.MatchString(name) && (skipRe == nil || !skipRe.MatchString(name)) {
				n++
			}
		}
		if n == 0 {
			bad = append(bad, fmt.Sprintf("layer %s: -run %q (-skip %q) matches no test of %s", l.Name, run, skip, pkgUnderTest))
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("a layer would run no test, so it would count as green; nothing was run:\n  %s", strings.Join(bad, "\n  "))
	}
	return nil
}

// Run is the whole harness: baseline first, then the mutants in order.
func Run(ctx context.Context, engineDir, mutantsPath, scratch string, only map[string]bool, cfg Config, log io.Writer) ([]Result, error) {
	ms, err := LoadMutants(mutantsPath)
	if err != nil {
		return nil, err
	}
	// The harness is checked as a whole before anything is copied or run: every
	// anchor of the list, selected or not, and every layer.
	if err := checkOnly(ms, only); err != nil {
		return nil, err
	}
	if err := CheckAnchors(engineDir, ms); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(scratch, "pormut-")
	if err != nil {
		return nil, err
	}
	if err := CopyEngine(engineDir, dir); err != nil {
		return nil, fmt.Errorf("copying the engine: %w", err)
	}
	if !cfg.Keep {
		defer os.RemoveAll(dir)
	}
	fmt.Fprintf(log, "scratch copy: %s\n", dir)
	if err := CheckLayers(ctx, dir, Layers, cfg); err != nil {
		return nil, err
	}

	// The baseline: the unmutated copy, every layer, on all generators. A red
	// baseline would make every mutant look killed.
	for _, l := range Layers {
		v, out := RunLayer(ctx, dir, l, "", cfg)
		fmt.Fprintf(log, "baseline %-8s %s\n", l.Name, v)
		if v != Green {
			return nil, fmt.Errorf("the baseline is %s on layer %s, so no mutant can be counted:\n%s", v, l.Name, tail(out, 40))
		}
	}

	byID := index(ms)
	var results []Result
	for _, m := range ms {
		if len(only) > 0 && !only[m.ID] {
			continue
		}
		parts, err := m.parts(byID)
		if err != nil {
			return results, err
		}
		patches, err := applyParts(parts, func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, rel)) })
		if err != nil {
			return results, err
		}
		if err := writePatches(dir, patches, false); err != nil {
			return results, err
		}
		r := Result{Mutant: m, Layers: map[string]Verdict{}}
		var details []string
		for _, l := range Layers {
			v, out := RunLayer(ctx, dir, l, m.Gens, cfg)
			r.Layers[l.Name] = v
			if v == BuildFailed {
				details = append(details, tail(out, 8))
				break // the other layers would say the same
			}
			if v == Red {
				details = append(details, firstFailure(out))
			}
		}
		if err := writePatches(dir, patches, true); err != nil {
			return results, err
		}
		r.Outcome, r.Bad = Classify(m, r.Layers)
		r.Detail = strings.Join(details, "\n")
		fmt.Fprintf(log, "%-28s %-4s O1=%-6s O2=%-6s O3=%-6s directed=%-6s %s\n", m.ID, m.Rule,
			short(r.Layers["O1"]), short(r.Layers["O2"]), short(r.Layers["O3"]), short(r.Layers["directed"]), r.Outcome)
		results = append(results, r)
	}
	return results, nil
}

// writePatches puts the mutated files into dir, or the original ones back.
func writePatches(dir string, patches map[string]*patch, restore bool) error {
	var first error
	for rel, pt := range patches {
		body := pt.mut
		if restore {
			body = pt.orig
		}
		if err := os.WriteFile(filepath.Join(dir, rel), body, 0o644); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func short(v Verdict) string {
	switch v {
	case Green:
		return "pass"
	case Red:
		return "KILL"
	case BuildFailed:
		return "build"
	}
	return "time"
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// firstFailure picks the first failing line of a go test output and the
// summary lines of the oracles ("por O2 [base]: 3000 models, ... 16 failures"),
// which carry the counts when MCD_POR_ALL is set in the environment of the
// harness.
func firstFailure(out string) string {
	var parts []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if strings.Contains(l, "por O") && strings.Contains(l, " failures") && !strings.HasSuffix(l, " 0 failures") {
			parts = append(parts, l[strings.Index(l, "por O"):])
		}
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "seed ") || strings.Contains(l, "--- FAIL") || strings.Contains(l, "FAIL:") {
			parts = append([]string{strings.TrimSpace(l)}, parts...)
			break
		}
	}
	if len(parts) == 0 {
		return tail(out, 3)
	}
	return strings.Join(parts, "\n")
}

// Summary sorts and totals the results.
func Summary(rs []Result) (text string, bad int) {
	counts := map[string]int{}
	for _, r := range rs {
		counts[strings.SplitN(r.Outcome, " (", 2)[0]]++
		if r.Bad {
			bad++
		}
	}
	var keys []string
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
	}
	return fmt.Sprintf("%d mutants: %s; %d against the contract", len(rs), strings.Join(parts, ", "), bad), bad
}
