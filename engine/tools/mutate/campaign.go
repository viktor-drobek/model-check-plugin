package mutate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"modelcheck/tools/pandiff"
)

// A campaign runs, for one model and each of its checks, the engine and
// SPIN's pan on the original and on every mutant, and classifies each mutant
// (plan 14 §8.1, §9 K3). The classes are the ones the feature file fixes:
//
//	(i)   detected           — the verdict changed for BOTH sides, to the same verdict
//	(ii)  verdict-equivalent — the verdict changed for neither
//	(iii) DISAGREEMENT       — the two disagree on the mutant, or exactly one changed
//	(iv)  not comparable     — a side produced no verdict at all (a rejection,
//	                           invalid-model, an incomplete search, a tool error)
//	                           or the original itself did not agree
//
// Detection rate = (i) / ((i) + (iii)): of the mutants that moved a verdict,
// the share on which the engine moved with pan rather than away from it.
// (ii) is deliberately outside the ratio — a mutant that changes no verdict
// is evidence about the mutant, not about the engine.
const (
	ClassDetected   = "(i) detected"
	ClassEquivalent = "(ii) verdict-equivalent"
	ClassDisagree   = "(iii) DISAGREEMENT"
	ClassNotCompar  = "(iv) not comparable"
)

// Check is one property run of a model. Mode "" is the safety run (the
// model's asserts and invalid end states: `mcd check` against plain `pan`);
// mode "a" and "l" are the temporal runs of the G4 triple (`pan -a` / `pan
// -l`), with Formula empty meaning "the model as written" (its never claim,
// its accept labels, or the np_ automaton).
type Check struct {
	Name     string `json:"name"`
	Mode     string `json:"mode,omitempty"`
	Formula  string `json:"formula,omitempty"`
	Fairness string `json:"fairness,omitempty"`
}

// ModelSpec is one corpus model with the checks that are natural to it.
type ModelSpec struct {
	Path    string   `json:"path"`
	Defines []string `json:"defines,omitempty"`
	Checks  []Check  `json:"checks"`
	Note    string   `json:"note,omitempty"`
}

// CheckResult is the pair of verdicts for one check of one source.
type CheckResult struct {
	Check        string `json:"check"`
	Engine       string `json:"engine"`
	Pan          string `json:"pan"`
	EngineStates int    `json:"engine_states"`
	PanStates    int    `json:"pan_states"`
	PanClass     string `json:"pan_class,omitempty"`
	Class        string `json:"class,omitempty"`
	Note         string `json:"note,omitempty"`
}

// MutantResult is one mutant with its class and every check behind it.
type MutantResult struct {
	Entry
	Class  string        `json:"class"`
	Checks []CheckResult `json:"checks"`
}

// ModelResult is one corpus model: the verdicts of the original and every
// mutant.
type ModelResult struct {
	Model    string         `json:"model"`
	Note     string         `json:"note,omitempty"`
	Baseline []CheckResult  `json:"baseline"`
	Mutants  []MutantResult `json:"mutants"`
}

// Counts are the four classes.
type Counts struct {
	Detected     int `json:"detected"`
	Equivalent   int `json:"equivalent"`
	Disagreement int `json:"disagreement"`
	NotComparabl int `json:"not_comparable"`
}

// Total of all four.
func (c Counts) Total() int {
	return c.Detected + c.Equivalent + c.Disagreement + c.NotComparabl
}

// Rate is (i)/((i)+(iii)); ok is false when the denominator is 0, in which
// case there is no rate to report rather than a rate of 0 or 1.
func (c Counts) Rate() (rate float64, ok bool) {
	den := c.Detected + c.Disagreement
	if den == 0 {
		return 0, false
	}
	return float64(c.Detected) / float64(den), true
}

// Disagreement is one class-(iii) finding, ready to be pasted into a bug
// report for G5: it names the mutant file and both verdicts.
type Disagreement struct {
	Model    string `json:"model"`
	Mutant   string `json:"mutant"`
	Operator string `json:"operator"`
	Line     int    `json:"line"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
	Check    string `json:"check"`
	Engine   string `json:"engine"`
	Pan      string `json:"pan"`
	Baseline string `json:"baseline"`
}

// Results is the whole campaign.
type Results struct {
	Engine        string         `json:"engine"`
	Spin          string         `json:"spin"`
	Started       string         `json:"started"`
	Operators     []string       `json:"operators"`
	Counts        Counts         `json:"counts"`
	Models        []ModelResult  `json:"models"`
	Disagreements []Disagreement `json:"disagreements"`
	ToolErrors    []string       `json:"tool_errors,omitempty"`
}

// CampaignOptions configure RunCampaign.
type CampaignOptions struct {
	Models     []ModelSpec
	Operators  []Operator
	CorpusRoot string
	OutDir     string
	MCD        string
	Spin       string
	GCC        string
	Jobs       int
	TimeoutSec int
	Progress   io.Writer
	// KeepDir, when set, receives a copy of every mutant that came out in
	// class (iii). The campaign's own output directory is temporary — 235
	// mutants are not an artefact worth keeping — but a disagreement is a
	// bug report, and a bug report whose file has been deleted is not one.
	// The recorded path of such a mutant is its path under KeepDir.
	KeepDir string
	// KeepPrefix replaces KeepDir in the recorded paths, so that a report
	// can cite a repository path while the copy is written through a
	// working-directory-relative one.
	KeepPrefix string
}

// RunCampaign generates and runs every mutant. Mutants run in parallel; the
// results are sorted back into manifest order, so the output does not depend
// on the scheduling.
func RunCampaign(ctx context.Context, o CampaignOptions) (*Results, error) {
	if o.Jobs <= 0 {
		o.Jobs = 1
	}
	if o.TimeoutSec <= 0 {
		o.TimeoutSec = 120
	}
	if o.MCD == "" {
		o.MCD = "mcd"
	}
	if o.Spin == "" {
		o.Spin = "spin"
	}
	if o.GCC == "" {
		o.GCC = "gcc"
	}
	tools := pandiff.Tools{Spin: o.Spin, GCC: o.GCC}
	if !tools.Available() {
		return nil, fmt.Errorf("spin or gcc not found (spin=%q gcc=%q)", o.Spin, o.GCC)
	}
	res := &Results{
		Engine:    firstLine(runOut(ctx, o.MCD, "version")),
		Spin:      firstLine(runOut(ctx, o.Spin, "-V")),
		Started:   time.Now().UTC().Format(time.RFC3339),
		Operators: names(effectiveOps(o.Operators)),
	}
	for _, spec := range o.Models {
		path := spec.Path
		if o.CorpusRoot != "" {
			path = filepath.Join(o.CorpusRoot, spec.Path)
		}
		src, err := os.ReadFile(path)
		if err != nil {
			res.ToolErrors = append(res.ToolErrors, fmt.Sprintf("%s: %v", spec.Path, err))
			continue
		}
		mr := ModelResult{Model: spec.Path, Note: spec.Note}
		dir := filepath.Join(o.OutDir, slug(spec.Path))
		muts := Generate(src, spec.Path, o.Operators...)
		if _, err := WriteAll(dir, muts); err != nil {
			return nil, err
		}
		// The original goes through exactly the same path as a mutant, so
		// that a difference between the two cannot come from the harness.
		origPath := filepath.Join(dir, "original.pml")
		if err := os.WriteFile(origPath, src, 0o644); err != nil {
			return nil, err
		}
		for _, c := range spec.Checks {
			mr.Baseline = append(mr.Baseline, runCheck(ctx, o, tools, origPath, spec.Defines, c))
		}
		progressf(o, "%s: %d mutants, baseline %s\n", spec.Path, len(muts), summary(mr.Baseline))
		results := make([]MutantResult, len(muts))
		sem := make(chan struct{}, o.Jobs)
		var wg sync.WaitGroup
		for i, m := range muts {
			wg.Add(1)
			go func(i int, m Mutant) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				file := filepath.Join(dir, m.FileName())
				mres := MutantResult{Entry: m.Entry}
				mres.Entry.File = file
				for ci, c := range spec.Checks {
					cr := runCheck(ctx, o, tools, file, spec.Defines, c)
					cr.Class = classify(mr.Baseline[ci], cr)
					if cr.Class == ClassNotCompar && cr.Note == "" {
						cr.Note = notComparableReason(mr.Baseline[ci], cr)
					}
					mres.Checks = append(mres.Checks, cr)
				}
				mres.Class = worstClass(mres.Checks)
				results[i] = mres
			}(i, m)
		}
		wg.Wait()
		mr.Mutants = results
		res.Models = append(res.Models, mr)
		progressf(o, "%s: %s\n", spec.Path, classSummary(results))
	}
	for mi := range res.Models {
		mr := &res.Models[mi]
		for xi := range mr.Mutants {
			m := &mr.Mutants[xi]
			switch m.Class {
			case ClassDetected:
				res.Counts.Detected++
			case ClassEquivalent:
				res.Counts.Equivalent++
			case ClassDisagree:
				res.Counts.Disagreement++
			default:
				res.Counts.NotComparabl++
			}
			if m.Class == ClassDisagree && o.KeepDir != "" {
				p, err := keepMutant(o, mr.Model, *m)
				if err != nil {
					res.ToolErrors = append(res.ToolErrors, fmt.Sprintf("keeping %s: %v", m.File, err))
				} else {
					m.File = p // the temporary copy is about to be deleted
				}
			}
			kept := m.File
			for ci, c := range m.Checks {
				if c.Class != ClassDisagree {
					continue
				}
				base := mr.Baseline[ci]
				res.Disagreements = append(res.Disagreements, Disagreement{
					Model: mr.Model, Mutant: kept, Operator: m.Operator, Line: m.Line,
					Original: m.Original, Mutated: m.Mutated, Check: c.Check,
					Engine: c.Engine, Pan: c.Pan,
					Baseline: fmt.Sprintf("engine %s / pan %s", base.Engine, base.Pan),
				})
			}
		}
	}
	return res, nil
}

func effectiveOps(ops []Operator) []Operator {
	if len(ops) == 0 {
		return Operators
	}
	return ops
}

func progressf(o CampaignOptions, format string, args ...any) {
	if o.Progress != nil {
		fmt.Fprintf(o.Progress, format, args...)
	}
}

func summary(rs []CheckResult) string {
	var b []string
	for _, r := range rs {
		b = append(b, fmt.Sprintf("%s: engine %s (%d) / pan %s (%d)", r.Check, r.Engine, r.EngineStates, r.Pan, r.PanStates))
	}
	return strings.Join(b, "; ")
}

func classSummary(ms []MutantResult) string {
	var c Counts
	for _, m := range ms {
		switch m.Class {
		case ClassDetected:
			c.Detected++
		case ClassEquivalent:
			c.Equivalent++
		case ClassDisagree:
			c.Disagreement++
		default:
			c.NotComparabl++
		}
	}
	return fmt.Sprintf("(i) %d, (ii) %d, (iii) %d, (iv) %d", c.Detected, c.Equivalent, c.Disagreement, c.NotComparabl)
}

func slug(path string) string {
	s := strings.NewReplacer("/", "_", "\\", "_", " ", "-").Replace(path)
	return strings.TrimSuffix(s, ".pml")
}

// isVerdict reports whether v is one of the two words that can be compared.
// Everything else — a rejection, invalid-model, inconclusive, a tool error —
// is not a verdict and puts the check in class (iv).
func isVerdict(v string) bool { return v == "verified" || v == "violated" }

func classify(base, mut CheckResult) string {
	if !isVerdict(base.Engine) || !isVerdict(base.Pan) || !isVerdict(mut.Engine) || !isVerdict(mut.Pan) {
		return ClassNotCompar
	}
	if base.Engine != base.Pan {
		return ClassNotCompar // the original already disagrees: reported separately
	}
	if mut.Engine != mut.Pan {
		return ClassDisagree
	}
	engineMoved, panMoved := mut.Engine != base.Engine, mut.Pan != base.Pan
	if engineMoved != panMoved {
		return ClassDisagree
	}
	if engineMoved {
		return ClassDetected
	}
	return ClassEquivalent
}

func notComparableReason(base, mut CheckResult) string {
	switch {
	case !isVerdict(base.Engine) || !isVerdict(base.Pan):
		return "the original gives no verdict: engine " + base.Engine + ", pan " + base.Pan
	case base.Engine != base.Pan:
		return "the original already disagrees: engine " + base.Engine + ", pan " + base.Pan
	case !isVerdict(mut.Engine):
		return "engine: " + mut.Engine
	default:
		return "pan: " + mut.Pan
	}
}

// classRank orders the classes for worstClass: a disagreement dominates
// everything, then "not comparable" (we cannot claim a detection when some
// check produced no verdict), then a detection, then equivalence.
var classRank = map[string]int{ClassDisagree: 3, ClassNotCompar: 2, ClassDetected: 1, ClassEquivalent: 0}

func worstClass(rs []CheckResult) string {
	best, name := -1, ClassEquivalent
	for _, r := range rs {
		if classRank[r.Class] > best {
			best, name = classRank[r.Class], r.Class
		}
	}
	return name
}

// CompareOne runs one check on one file — the same path the campaign uses
// for a mutant — and returns both verdicts. It exists for the feature tests,
// which check single named mutants rather than a whole campaign.
func CompareOne(ctx context.Context, o CampaignOptions, path string, defines []string, c Check) CheckResult {
	if o.MCD == "" {
		o.MCD = "mcd"
	}
	if o.Spin == "" {
		o.Spin = "spin"
	}
	if o.GCC == "" {
		o.GCC = "gcc"
	}
	if o.TimeoutSec <= 0 {
		o.TimeoutSec = 120
	}
	return runCheck(ctx, o, pandiff.Tools{Spin: o.Spin, GCC: o.GCC}, path, defines, c)
}

// Classify gives the class of a mutant check against the same check of the
// original.
func Classify(base, mut CheckResult) string { return classify(base, mut) }

// runCheck produces the two verdicts for one check of one file.
func runCheck(ctx context.Context, o CampaignOptions, tools pandiff.Tools, path string, defines []string, c Check) CheckResult {
	cctx, cancel := context.WithTimeout(ctx, time.Duration(o.TimeoutSec)*time.Second)
	defer cancel()
	name := c.Name
	if name == "" {
		name = checkName(c)
	}
	if c.Mode == "" {
		return runSafety(cctx, o, tools, path, defines, name)
	}
	// The temporal checks go through the G4 triple, which already knows how
	// to line the engine's automaton up with pan -a / -l and with SPIN's own
	// never claim for the formula (steps/g4-confirmation.md §3.1).
	tr, err := pandiff.RunTriple(cctx, tools, path, defines, c.Formula, c.Fairness, c.Mode)
	if err != nil {
		return CheckResult{Check: name, Engine: rejectionWord(err), Pan: "n/a", Note: trim(err.Error(), 300)}
	}
	return CheckResult{
		Check: name, Engine: word(tr.Engine), Pan: word(tr.Pan),
		EngineStates: tr.EngineStates, PanStates: tr.PanStates, PanClass: tr.PanClass,
		Note: triAnnotation(tr),
	}
}

func triAnnotation(tr *pandiff.Triple) string {
	if tr.Claim == "n/a" {
		return ""
	}
	return "engine with SPIN's claim: " + tr.Claim
}

func checkName(c Check) string {
	parts := []string{"safety"}
	switch c.Mode {
	case "a":
		parts = []string{"acceptance (pan -a)"}
	case "l":
		parts = []string{"non-progress (pan -l)"}
	}
	if c.Formula != "" {
		parts = append(parts, c.Formula)
	}
	if c.Fairness != "" && c.Fairness != "none" {
		parts = append(parts, c.Fairness+" fairness")
	}
	return strings.Join(parts, " ")
}

// word keeps the first word of a verdict, so that "inconclusive: budget" is
// distinguishable from "verified" but a long reason does not enter a table.
func word(v string) string {
	if isVerdict(v) {
		return v
	}
	return v
}

func rejectionWord(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "outside-subset"):
		return "rejected (outside-subset)"
	case strings.Contains(s, "syntax"):
		return "rejected (syntax)"
	case strings.Contains(s, "semantic"):
		return "rejected (semantic)"
	case strings.Contains(s, "spin -a failed"), strings.Contains(s, "gcc failed"):
		return "spin rejects"
	case strings.Contains(s, "context deadline exceeded"):
		return "timeout"
	}
	return "tool error"
}

// runSafety is the "model as written" run: the engine's asserts and
// deadlock property through the `mcd` CLI, against plain pan.
func runSafety(ctx context.Context, o CampaignOptions, tools pandiff.Tools, path string, defines []string, name string) CheckResult {
	out := CheckResult{Check: name}
	out.Engine, out.EngineStates, out.Note = engineSafety(ctx, o, path, defines)
	pv, states, class, note := panSafety(ctx, tools, path, defines)
	out.Pan, out.PanStates, out.PanClass = pv, states, class
	if note != "" {
		out.Note = strings.TrimSpace(out.Note + " " + note)
	}
	return out
}

// engineSafety runs `mcd check` and reduces its report to one verdict over
// the model's own assert and deadlock properties. A property that is neither
// verified nor violated (invalid-model, inconclusive, not-executed) makes the
// whole check inconclusive: no verdict may be claimed from it.
func engineSafety(ctx context.Context, o CampaignOptions, path string, defines []string) (verdict string, states int, note string) {
	args := []string{"check", "--promela", path, "--sweep", "--no-timing",
		"--budget-states", "2000000", "--budget-depth", "2000000", "--budget-ms", "60000", "--budget-mem-mb", "2048"}
	for _, d := range defines {
		args = append(args, "-D", d)
	}
	cmd := exec.CommandContext(ctx, o.MCD, args...)
	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	code := cmd.ProcessState.ExitCode()
	if ctx.Err() != nil {
		return "timeout", 0, ""
	}
	if code == 2 {
		var rej struct {
			Error struct {
				Kind    string `json:"kind"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(stdout.String()), &rej) == nil && rej.Error.Kind != "" {
			return "rejected (" + rej.Error.Kind + ")", 0, trim(rej.Error.Message, 200)
		}
		return "rejected", 0, trim(stdout.String(), 200)
	}
	if err != nil || code != 0 {
		return "tool error", 0, trim(stderr.String(), 200)
	}
	var rep struct {
		Search struct {
			States   int  `json:"-"`
			Complete bool `json:"complete"`
			Stop     string
		} `json:"search"`
		Properties []struct {
			ID       string `json:"id"`
			Kind     string `json:"kind"`
			Status   string `json:"status"`
			Complete bool   `json:"complete"`
			Reason   string `json:"reason"`
			Counters struct {
				States int `json:"states"`
			} `json:"counters"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &rep); err != nil {
		return "tool error", 0, trim(err.Error(), 200)
	}
	verdict = "verified"
	seen := false
	for _, p := range rep.Properties {
		if p.Kind != "assert" && p.Kind != "deadlock" {
			continue
		}
		seen = true
		if p.Counters.States > states {
			states = p.Counters.States
		}
		switch {
		case p.Status == "violated":
			if verdict != "inconclusive" {
				verdict = "violated"
			}
		case p.Status == "verified" && p.Complete:
		default:
			verdict = p.Status
			note = trim(p.Reason, 200)
		}
	}
	if !seen {
		return "no safety property", states, ""
	}
	return verdict, states, note
}

// panSafety runs SPIN's pan without -a: assertion violations and invalid end
// states, the same two properties the engine's safety run decides.
func panSafety(ctx context.Context, tools pandiff.Tools, path string, defines []string) (verdict string, states int, class, note string) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "tool error", 0, "", err.Error()
	}
	dir, err := os.MkdirTemp("", "k3pan-")
	if err != nil {
		return "tool error", 0, "", err.Error()
	}
	defer os.RemoveAll(dir)
	// As in the G4 triple: -D through spin goes via a shell and breaks on
	// values with parentheses, so the defines are prepended to the copy.
	if len(defines) > 0 {
		var b strings.Builder
		for _, d := range defines {
			name, val := d, "1"
			if i := strings.IndexByte(d, '='); i >= 0 {
				name, val = d[:i], d[i+1:]
			}
			fmt.Fprintf(&b, "#define %s %s\n", name, val)
		}
		src = append([]byte(b.String()), src...)
	}
	if err := os.WriteFile(filepath.Join(dir, "m.pml"), src, 0o644); err != nil {
		return "tool error", 0, "", err.Error()
	}
	if out, err := runIn(ctx, dir, tools.Spin, "-a", "-o1", "-o2", "-o3", "m.pml"); err != nil || strings.Contains(out, "Error") {
		return "spin rejects", 0, "", trim(out, 200)
	}
	if out, err := runIn(ctx, dir, tools.GCC, "-O2", "-DNOREDUCE", "-o", "pan", "pan.c"); err != nil {
		return "gcc rejects", 0, "", trim(out, 200)
	}
	pan := filepath.Join(dir, "pan")
	c0, _ := runIn(ctx, dir, pan, "-c0", "-m200000")
	if ctx.Err() != nil {
		return "timeout", 0, "", ""
	}
	stored, _, errors, perr := pandiff.ParseC0(c0)
	if perr != nil {
		return "pan error", 0, "", trim(c0, 200)
	}
	states = stored
	first, _ := runIn(ctx, dir, pan, "-m200000")
	_, class = pandiff.ParseFirst(first, errors)
	switch {
	case errors > 0:
		return "violated", states, class, ""
	case strings.Contains(c0, "Search not completed") || strings.Contains(c0, "max search depth too small"):
		return "inconclusive (pan search not completed)", states, class, ""
	default:
		return "verified", states, class, ""
	}
}

func runIn(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runOut(ctx context.Context, name string, args ...string) string {
	cmd := exec.CommandContext(ctx, name, args...)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func trim(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " | "))
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// Markdown renders the report: the counts, the rate, the per-model table and
// every disagreement in full.
func (r *Results) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# K3 mutation results\n\n")
	fmt.Fprintf(&b, "Engine: `%s`. SPIN: `%s`. Run started %s.\n", r.Engine, r.Spin, r.Started)
	fmt.Fprintf(&b, "Operators: %s.\n\n", strings.Join(r.Operators, ", "))
	fmt.Fprintf(&b, "| class | mutants |\n|---|---|\n")
	fmt.Fprintf(&b, "| (i) detected | %d |\n", r.Counts.Detected)
	fmt.Fprintf(&b, "| (ii) verdict-equivalent | %d |\n", r.Counts.Equivalent)
	fmt.Fprintf(&b, "| (iii) DISAGREEMENT | %d |\n", r.Counts.Disagreement)
	fmt.Fprintf(&b, "| (iv) not comparable | %d |\n", r.Counts.NotComparabl)
	fmt.Fprintf(&b, "| total | %d |\n\n", r.Counts.Total())
	if rate, ok := r.Counts.Rate(); ok {
		fmt.Fprintf(&b, "Detection rate = (i)/((i)+(iii)) = %d/%d = **%.3f**.\n\n",
			r.Counts.Detected, r.Counts.Detected+r.Counts.Disagreement, rate)
	} else {
		fmt.Fprintf(&b, "Detection rate: undefined — no mutant moved a verdict on either side.\n\n")
	}
	fmt.Fprintf(&b, "## Per model\n\n| model | mutants | (i) | (ii) | (iii) | (iv) | baseline |\n|---|---|---|---|---|---|---|\n")
	for _, m := range r.Models {
		var c Counts
		for _, mu := range m.Mutants {
			switch mu.Class {
			case ClassDetected:
				c.Detected++
			case ClassEquivalent:
				c.Equivalent++
			case ClassDisagree:
				c.Disagreement++
			default:
				c.NotComparabl++
			}
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %s |\n", m.Model, len(m.Mutants),
			c.Detected, c.Equivalent, c.Disagreement, c.NotComparabl, summary(m.Baseline))
	}
	b.WriteString("\n## Disagreements (class (iii))\n\n")
	if len(r.Disagreements) == 0 {
		b.WriteString("None.\n")
	} else {
		b.WriteString("| mutant | operator | line | original → mutated | check | engine | pan | original |\n|---|---|---|---|---|---|---|---|\n")
		for _, d := range r.Disagreements {
			m := d.Mutated
			if m == "" {
				m = "(removed)"
			}
			fmt.Fprintf(&b, "| `%s` | %s | %d | `%s` → `%s` | %s | **%s** | **%s** | %s |\n",
				d.Mutant, d.Operator, d.Line, d.Original, m, d.Check, d.Engine, d.Pan, d.Baseline)
		}
	}
	if len(r.ToolErrors) > 0 {
		b.WriteString("\n## Tool errors\n\n")
		for _, e := range r.ToolErrors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}
	b.WriteString("\n## Class (iv) by reason\n\n")
	reasons := map[string]int{}
	for _, m := range r.Models {
		for _, mu := range m.Mutants {
			if mu.Class != ClassNotCompar {
				continue
			}
			for _, c := range mu.Checks {
				if c.Class == ClassNotCompar {
					reasons[reasonKey(c)]++
					break
				}
			}
		}
	}
	keys := make([]string, 0, len(reasons))
	for k := range reasons {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	b.WriteString("| reason | mutants |\n|---|---|\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "| %s | %d |\n", k, reasons[k])
	}
	return b.String()
}

func reasonKey(c CheckResult) string {
	switch {
	case !isVerdict(c.Engine) && !isVerdict(c.Pan):
		return "engine " + c.Engine + ", pan " + c.Pan
	case !isVerdict(c.Engine):
		return "engine " + c.Engine
	case !isVerdict(c.Pan):
		return "pan " + c.Pan
	}
	return "the original did not agree"
}

// keepMutant copies one disagreeing mutant out of the temporary campaign
// directory, into a place a bug report can cite, and returns the path to
// record. The copy keeps a header naming the model, the operator, the
// position and both verdicts, so that the file is self-explanatory when it is
// opened months later without this report beside it.
func keepMutant(o CampaignOptions, model string, m MutantResult) (string, error) {
	dir := filepath.Join(o.KeepDir, slug(model))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	src, err := os.ReadFile(m.File)
	if err != nil {
		return "", err
	}
	var h strings.Builder
	fmt.Fprintf(&h, "/*\n * K3 class (iii) DISAGREEMENT (plan 14 §8.1, §9 K3).\n")
	fmt.Fprintf(&h, " * Model:    %s\n * Mutation: %s at line %d, column %d\n", model, m.Operator, m.Line, m.Col)
	mutated := m.Mutated
	if mutated == "" {
		mutated = "(removed)"
	}
	fmt.Fprintf(&h, " *           %q -> %s\n", m.Original, mutated)
	for _, c := range m.Checks {
		if c.Class != ClassDisagree {
			continue
		}
		fmt.Fprintf(&h, " * Check %-24s engine %s, pan %s\n", c.Check+":", c.Engine, c.Pan)
	}
	fmt.Fprintf(&h, " * Reported, not fixed: the engine belongs to step G5.\n */\n")
	name := filepath.Base(m.File)
	out := filepath.Join(dir, name)
	if err := os.WriteFile(out, append([]byte(h.String()), src...), 0o644); err != nil {
		return "", err
	}
	if o.KeepPrefix != "" {
		return filepath.ToSlash(filepath.Join(o.KeepPrefix, slug(model), name)), nil
	}
	return filepath.ToSlash(out), nil
}
