// Package cli implements the `mcd` command line (plan 14 §6, the fallback
// interface of the skill in environments without MCP). It is a library so
// that the feature tests can run the CLI in-process; cmd/mcd is a one-line
// wrapper.
//
// Commands:
//
//	mcd parse   --petri file.json | --ir file.json            → IR JSON
//	mcd check  (--petri file.json | --ir file.json)
//	           [--budget-states N] [--budget-depth N] [--budget-ms N]
//	           [--budget-mem-mb N] [--bfs] [--no-timing]        → report JSON
//	mcd version                                                → "mcd <version>"
//
// Exit codes, one per outcome: 0 — a result document (report or IR) was
// produced, whatever the verdicts, including invalid-model; 2 — no result:
// the input was rejected by a frontend (schema violation, unsupported
// construct, invalid IR) and stdout carries a JSON
// `{"error": {kind, path, message}}` instead; 1 — no result: tool error
// (unreadable file, bad flags, internal failure), message on stderr.
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

	"modelcheck/explore"
	"modelcheck/frontend/petri"
	"modelcheck/ir"
	"modelcheck/report"
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
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

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
	kind string
	path string
	data []byte
}

func loadInput(petriPath, irPath string) (*input, error) {
	if (petriPath == "") == (irPath == "") {
		return nil, errors.New("exactly one of --petri or --ir is required")
	}
	in := &input{kind: "petri", path: petriPath}
	if irPath != "" {
		in.kind, in.path = "ir", irPath
	}
	data, err := os.ReadFile(in.path)
	if err != nil {
		return nil, err
	}
	in.data = data
	return in, nil
}

// toIR translates the input; a *petri.Error or an IR validation error is a
// rejection (exit 2), anything else a tool error.
func toIR(in *input) (*ir.Model, *rejectionBody) {
	switch in.kind {
	case "petri":
		name := strings.TrimSuffix(filepath.Base(in.path), filepath.Ext(in.path))
		net, err := petri.Parse(in.data, name)
		if err != nil {
			var pe *petri.Error
			if errors.As(err, &pe) {
				return nil, &rejectionBody{Kind: pe.Kind, Path: pe.Path, Message: pe.Message}
			}
			return nil, &rejectionBody{Kind: "schema", Message: err.Error()}
		}
		return net.ToIR(in.path), nil
	default:
		m, err := ir.UnmarshalJSON(in.data)
		if err != nil {
			return nil, &rejectionBody{Kind: "ir", Message: err.Error()}
		}
		return m, nil
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
	if err := fs.Parse(args); err != nil {
		return ExitTool
	}
	in, err := loadInput(*petriPath, *irPath)
	if err != nil {
		fmt.Fprintln(stderr, "mcd parse:", err)
		return ExitTool
	}
	m, rej := toIR(in)
	if rej != nil {
		return reject(stdout, rej)
	}
	out, err := ir.MarshalJSON(m)
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
	states := fs.Int("budget-states", DefaultStates, "maximum number of stored states (0 = unlimited)")
	depth := fs.Int("budget-depth", DefaultDepth, "maximum search depth in transitions (0 = unlimited)")
	ms := fs.Int64("budget-ms", DefaultMS, "wall-clock budget in milliseconds (0 = unlimited)")
	memMB := fs.Int64("budget-mem-mb", DefaultMemMB, "memory estimate budget in MiB (0 = unlimited)")
	bfs := fs.Bool("bfs", false, "breadth-first search (shortest counterexamples)")
	noTiming := fs.Bool("no-timing", false, "omit time_ms so that reports are byte-for-byte reproducible")
	if err := fs.Parse(args); err != nil {
		return ExitTool
	}
	in, err := loadInput(*petriPath, *irPath)
	if err != nil {
		fmt.Fprintln(stderr, "mcd check:", err)
		return ExitTool
	}
	m, rej := toIR(in)
	if rej != nil {
		return reject(stdout, rej)
	}
	mode := explore.DFS
	if *bfs {
		mode = explore.BFS
	}
	budget := explore.Budget{MaxStates: *states, MaxDepth: *depth, MaxMemBytes: *memMB << 20}
	ctx := context.Background()
	if *ms > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*ms)*time.Millisecond)
		defer cancel()
	}
	res, err := explore.Run(ctx, m, explore.Options{Mode: mode, Budget: budget})
	if err != nil {
		// The IR validated but could not be compiled: treat as a rejection
		// of the input, with the compiler's explanation.
		return reject(stdout, &rejectionBody{Kind: "ir", Message: err.Error()})
	}
	sum := sha256.Sum256(in.data)
	rep, err := report.Build(m, res, report.Meta{
		Inputs:   []report.Input{{Kind: in.kind, Path: in.path, SHA256: hex.EncodeToString(sum[:])}},
		Mode:     mode,
		Budget:   report.Budget{States: *states, Depth: *depth, TimeMS: *ms, MemBytes: *memMB << 20},
		NoTiming: *noTiming,
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
