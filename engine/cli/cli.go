// Package cli implements the `mcd` command line (plan 14 §6, the fallback
// interface of the skill in environments without MCP). It is a library so
// that the feature tests can run the CLI in-process; cmd/mcd is a one-line
// wrapper.
//
// Commands:
//
//	mcd parse   --petri file.json | --ir file.json | --promela file.pml [-D NAME[=val]]…
//	                                                            → IR JSON
//	mcd check  (--petri file.json | --ir file.json | --promela file.pml [-D …])
//	           [--ltl 'formula']… [--ctl 'formula']… [--progress]
//	           [--fairness none|weak|strong] [--max-procs N]
//	           [--budget-states N] [--budget-depth N] [--budget-ms N]
//	           [--budget-mem-mb N] [--unlimited] [--bfs] [--sweep] [--no-timing]
//	                                                            → report JSON
//	mcd check  --estimate [--estimate-ms N] [--target-depth N] (input flags)
//	                                                            → estimate JSON
//	mcd version                                                → "mcd <version>"
//
// Budgets (plan 14 §6 as amended in G4): a budget flag that is absent or 0
// means the default (the "medium model" of §12: 1e6 states, depth 1e6,
// 60 s, 1 GiB) — the same reading as the MCP server's; `--unlimited` lifts
// every limit (the report echoes 0 for them) and exists for differential
// tests against pan. Before G4 the CLI read 0 as "unlimited".
//
// `--ltl` adds an ltl property (ids ltl1, ltl2, …) whose formula is in
// SPIN syntax (package ltl); `#define`d symbols of a Promela input are
// expanded in it. `--progress` adds a `progress` property (no non-progress
// cycle) when the model does not carry one already. `--fairness weak`
// applies weak fairness to ltl and progress properties; `strong` makes
// them not-executed with a reason (FR-008).
//
// Exit codes, one per outcome: 0 — a result document (report or IR) was
// produced, whatever the verdicts, including invalid-model; 2 — no result:
// the input was rejected by a frontend (schema violation, syntax or
// semantic error, construct outside the subset, invalid IR, malformed or
// unresolvable LTL formula — kind "ltl") and stdout
// carries a JSON `{"error": {kind, status, path, message}}` instead — its
// status is always "not-executed": nothing rejected has been executed, so
// no verdict and no invalid-model can be claimed; 1 — no result: tool error
// (unreadable file, bad flags, internal failure), message on stderr.
//
// Frontend warnings (G1: printf ignored, never claim not executed) go to
// stderr as "warning: …" lines and into the report's "warnings" field.
//
// Flag names are part of the skill's contract and stay stable.
package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modelcheck/estimate"
	"modelcheck/explore"
	"modelcheck/frontend/petri"
	"modelcheck/frontend/promela"
	"modelcheck/ir"
	"modelcheck/report"
)

// Fairness values accepted by --fairness and the MCP server.
const (
	FairnessNone   = "none"
	FairnessWeak   = "weak"
	FairnessStrong = "strong"
)

// Exit codes.
const (
	ExitOK       = 0
	ExitTool     = 1
	ExitRejected = 2
)

// Default budgets: the "medium model" of plan 14 §12 (A4).
const (
	DefaultStates = 1_000_000
	DefaultDepth  = 1_000_000
	DefaultMS     = 60_000
	DefaultMemMB  = 1024
)

// rejection is the JSON body printed for exit code 2.
type rejection struct {
	Error rejectionBody `json:"error"`
}

type rejectionBody struct {
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// defineList collects repeated -D flags.
type defineList []string

func (d *defineList) String() string     { return strings.Join(*d, " ") }
func (d *defineList) Set(s string) error { *d = append(*d, s); return nil }

// Run executes args (without the program name) and returns the exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: mcd (parse|check|version) [flags]")
		return ExitTool
	}
	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "%s %s (ir %s, report %s)\n", report.EngineName, report.EngineVersion, ir.Schema, report.ReportSchema)
		return ExitOK
	case "parse":
		return runParse(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "mcd: unknown command %q (parse|check|version)\n", args[0])
	return ExitTool
}

type input struct {
	kind     string
	path     string
	data     []byte
	defines  []string
	maxProcs int
}

// Parsed is the frontend's output: the model, its warnings and, for a
// Promela input, the #define table for LTL atoms.
type Parsed struct {
	Model    *ir.Model
	Warnings []string
	Defines  map[string]string
}

func loadInput(petriPath, irPath, promelaPath string, defines []string) (*input, error) {
	n := 0
	in := &input{defines: defines}
	for kind, path := range map[string]string{"petri": petriPath, "ir": irPath, "promela": promelaPath} {
		if path != "" {
			n++
			in.kind, in.path = kind, path
		}
	}
	if n != 1 {
		return nil, errors.New("exactly one of --petri, --ir or --promela is required")
	}
	data, err := os.ReadFile(in.path)
	if err != nil {
		return nil, err
	}
	in.data = data
	return in, nil
}

// toIR translates the input; a frontend error or an IR validation error is
// a rejection (exit 2), anything else a tool error. The warnings are the
// frontend's.
func toIR(in *input) (*Parsed, *rejectionBody) {
	switch in.kind {
	case "petri":
		name := strings.TrimSuffix(filepath.Base(in.path), filepath.Ext(in.path))
		net, err := petri.Parse(in.data, name)
		if err != nil {
			var pe *petri.Error
			if errors.As(err, &pe) {
				return nil, &rejectionBody{Kind: pe.Kind, Status: "not-executed", Path: pe.Path, Message: pe.Message}
			}
			return nil, &rejectionBody{Kind: "schema", Status: "not-executed", Message: err.Error()}
		}
		return &Parsed{Model: net.ToIR(in.path)}, nil
	case "promela":
		res, perr := ParsePromela(in.data, in.path, in.defines, in.maxProcs)
		if perr != nil {
			return nil, perr
		}
		return res, nil
	default:
		m, err := ir.UnmarshalJSON(in.data)
		if err != nil {
			return nil, &rejectionBody{Kind: "ir", Status: "not-executed", Message: err.Error()}
		}
		return &Parsed{Model: m}, nil
	}
}

// ParsePromela runs the Promela frontend and maps its error to a
// rejection; the MCP server uses it too, so both interfaces reject the
// same inputs with the same words.
func ParsePromela(src []byte, path string, defines []string, maxProcs int) (*Parsed, *rejectionBody) {
	res, perr := promela.ParseWith(src, path, defines, promela.Options{MaxProcs: maxProcs})
	if perr != nil {
		return nil, &rejectionBody{
			Kind:    perr.Kind,
			Status:  "not-executed",
			Path:    fmt.Sprintf("%s:%d:%d", perr.File, perr.Line, perr.Col),
			Message: fmt.Sprintf("%s (%s, line %d)", perr.Message, filepath.Base(perr.File), perr.Line),
		}
	}
	return &Parsed{Model: res.Model, Warnings: res.Warnings, Defines: res.Defines}, nil
}

// Rejection is the public view of a frontend rejection (for the server).
type Rejection = rejectionBody

// Fields of a rejection, exported for the server.
func (r *rejectionBody) KindOf() string    { return r.Kind }
func (r *rejectionBody) PathOf() string    { return r.Path }
func (r *rejectionBody) MessageOf() string { return r.Message }

func printWarnings(stderr io.Writer, warnings []string) {
	for _, w := range warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
}

func reject(stdout io.Writer, body *rejectionBody) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	enc.Encode(rejection{*body})
	return ExitRejected
}

func runParse(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mcd parse", flag.ContinueOnError)
	fs.SetOutput(stderr)
	petriPath := fs.String("petri", "", "Petri net JSON file (frontend/petri/schema.json)")
	irPath := fs.String("ir", "", "IR JSON file (validated and re-printed)")
	promelaPath := fs.String("promela", "", "Promela file (the subset of plan 14 §5.2)")
	var defines defineList
	fs.Var(&defines, "D", "preprocessor symbol NAME or NAME=value (repeatable)")
	maxProcs := fs.Int("max-procs", promela.DefaultMaxProcs, "instances pre-instantiated per proctype that a run can create repeatedly")
	if err := fs.Parse(args); err != nil {
		return ExitTool
	}
	in, err := loadInput(*petriPath, *irPath, *promelaPath, defines)
	if err != nil {
		fmt.Fprintln(stderr, "mcd parse:", err)
		return ExitTool
	}
	in.maxProcs = *maxProcs
	parsed, rej := toIR(in)
	if rej != nil {
		return reject(stdout, rej)
	}
	printWarnings(stderr, parsed.Warnings)
	out, err := ir.MarshalJSON(parsed.Model)
	if err != nil {
		fmt.Fprintln(stderr, "mcd parse:", err)
		return ExitTool
	}
	stdout.Write(out)
	return ExitOK
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mcd check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	petriPath := fs.String("petri", "", "Petri net JSON file")
	irPath := fs.String("ir", "", "IR JSON file")
	promelaPath := fs.String("promela", "", "Promela file (the subset of plan 14 §5.2)")
	var defines defineList
	fs.Var(&defines, "D", "preprocessor symbol NAME or NAME=value (repeatable)")
	var ltls defineList
	fs.Var(&ltls, "ltl", "LTL formula in SPIN syntax to check (repeatable; properties ltl1, ltl2, …)")
	var ctls defineList
	fs.Var(&ctls, "ctl", "CTL formula to check by graph labelling (repeatable; properties ctl1, ctl2, …)")
	maxProcs := fs.Int("max-procs", promela.DefaultMaxProcs, "instances pre-instantiated per proctype that a run can create repeatedly")
	doEstimate := fs.Bool("estimate", false, "print the state-space growth estimate (plan 14 §6) instead of checking properties")
	estimateMS := fs.Int64("estimate-ms", 0, "time limit of --estimate in milliseconds (absent or 0 = 1000)")
	targetDepth := fs.Int("target-depth", 0, "depth --estimate projects the growth to (absent or 0 = only the next level)")
	progress := fs.Bool("progress", false, "check for non-progress cycles (property progress; SPIN pan -l)")
	fairness := fs.String("fairness", FairnessNone, "none | weak | strong: fairness for ltl and progress properties (strong is not executed, FR-008)")
	sweep := fs.Bool("sweep", false, "keep searching after every property is decided (state count of the whole graph, as pan -c0)")
	states := fs.Int("budget-states", 0, "maximum number of stored states (absent or 0 = default 1000000)")
	depth := fs.Int("budget-depth", 0, "maximum search depth in transitions (absent or 0 = default 1000000)")
	ms := fs.Int64("budget-ms", 0, "wall-clock budget in milliseconds (absent or 0 = default 60000)")
	memMB := fs.Int64("budget-mem-mb", 0, "memory estimate budget in MiB (absent or 0 = default 1024)")
	unlimited := fs.Bool("unlimited", false, "lift every budget (for differential tests); the report echoes 0 for the lifted limits")
	bfs := fs.Bool("bfs", false, "breadth-first search (shortest counterexamples)")
	noTiming := fs.Bool("no-timing", false, "omit time_ms so that reports are byte-for-byte reproducible")
	if err := fs.Parse(args); err != nil {
		return ExitTool
	}
	switch *fairness {
	case FairnessNone, FairnessWeak, FairnessStrong:
	default:
		fmt.Fprintf(stderr, "mcd check: --fairness must be none, weak or strong, got %q\n", *fairness)
		return ExitTool
	}
	in, err := loadInput(*petriPath, *irPath, *promelaPath, defines)
	if err != nil {
		fmt.Fprintln(stderr, "mcd check:", err)
		return ExitTool
	}
	in.maxProcs = *maxProcs
	parsed, rej := toIR(in)
	if rej != nil {
		return reject(stdout, rej)
	}
	m := parsed.Model
	if len(ltls) > 0 || len(ctls) > 0 || *progress {
		mm := *m
		mm.Properties = append([]ir.Property(nil), m.Properties...)
		for i, f := range ltls {
			mm.Properties = append(mm.Properties, ir.Property{ID: fmt.Sprintf("ltl%d", i+1), Kind: ir.KindLTL, Formula: f, Text: f})
		}
		for i, f := range ctls {
			mm.Properties = append(mm.Properties, ir.Property{ID: fmt.Sprintf("ctl%d", i+1), Kind: ir.KindCTL, Formula: f, Text: f})
		}
		if *progress && !hasKind(mm.Properties, ir.KindProgress) {
			mm.Properties = append(mm.Properties, ir.Property{ID: "progress", Kind: ir.KindProgress,
				Text: "no non-progress cycle: every infinite run visits a progress label infinitely often (SPIN: pan -l)"})
		}
		m = &mm
	}
	printWarnings(stderr, parsed.Warnings)
	if *doEstimate {
		return runEstimate(m, *estimateMS, *targetDepth, Budgets(*states, *depth, *ms, *memMB, *unlimited), stdout, stderr)
	}
	mode := explore.DFS
	if *bfs {
		mode = explore.BFS
	}
	b := Budgets(*states, *depth, *ms, *memMB, *unlimited)
	ctx := context.Background()
	if b.TimeMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(b.TimeMS)*time.Millisecond)
		defer cancel()
	}
	res, err := explore.Run(ctx, m, explore.Options{Mode: mode, Budget: b.Explore(), Sweep: *sweep, Fairness: *fairness, Defines: parsed.Defines})
	if err != nil {
		// The IR validated but could not be compiled (an undeclared variable
		// in a property, a malformed LTL formula): the input is refused,
		// with the compiler's explanation.
		kind := "ir"
		var fe *explore.FormulaError
		if errors.As(err, &fe) {
			kind = fe.Kind()
		}
		return reject(stdout, &rejectionBody{Kind: kind, Status: "not-executed", Message: err.Error()})
	}
	sum := sha256.Sum256(in.data)
	rep, err := report.Build(m, res, report.Meta{
		Inputs:   []report.Input{{Kind: in.kind, Path: in.path, SHA256: hex.EncodeToString(sum[:])}},
		Mode:     mode,
		Budget:   b,
		NoTiming: *noTiming,
		Warnings: parsed.Warnings,
	})
	if err != nil {
		fmt.Fprintln(stderr, "mcd check: internal:", err)
		return ExitTool
	}
	out, err := rep.JSON()
	if err != nil {
		fmt.Fprintln(stderr, "mcd check:", err)
		return ExitTool
	}
	stdout.Write(out)
	return ExitOK
}

// runEstimate prints the growth estimate document (plan 14 §6). It is not
// a report: no property gets a status here, which the document says.
func runEstimate(m *ir.Model, ms int64, targetDepth int, b report.Budget, stdout, stderr io.Writer) int {
	ctx := context.Background()
	if b.TimeMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(b.TimeMS)*time.Millisecond)
		defer cancel()
	}
	res, err := estimate.Run(ctx, m, estimate.Options{TimeLimitMS: ms, TargetDepth: targetDepth, Budget: b.Explore()})
	if err != nil {
		return reject(stdout, &rejectionBody{Kind: "ir", Status: "not-executed", Message: err.Error()})
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(res); err != nil {
		fmt.Fprintln(stderr, "mcd check --estimate:", err)
		return ExitTool
	}
	return ExitOK
}

func hasKind(props []ir.Property, kind string) bool {
	for _, p := range props {
		if p.Kind == kind {
			return true
		}
	}
	return false
}

// Budgets applies the CLI budget semantics: 0 → default; unlimited → 0
// (no limit) in every field. The result is what the report echoes.
func Budgets(states, depth int, ms, memMB int64, unlimited bool) report.Budget {
	if unlimited {
		return report.Budget{}
	}
	if states == 0 {
		states = DefaultStates
	}
	if depth == 0 {
		depth = DefaultDepth
	}
	if ms == 0 {
		ms = DefaultMS
	}
	if memMB == 0 {
		memMB = DefaultMemMB
	}
	return report.Budget{States: states, Depth: depth, TimeMS: ms, MemBytes: memMB << 20}
}
