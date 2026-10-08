package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"modelcheck/mcp"
	"modelcheck/report"
)

// serve implements `mcd serve`: the MCP server over stdio (plan 14 §6).
//
//	mcd serve [--session-dir D] [--allow-read DIR]...
//	          [--max-states N] [--max-depth N] [--max-ms N] [--max-memory-mb N]
//	          [--concurrency K] [--max-workers N] [--cleanup]
//
// --session-dir defaults to $MCD_SESSION_DIR, then to a fresh temporary
// directory. The --max-* flags are ceilings a client budget cannot exceed
// (0 = no ceiling); the defaults applied to budget fields a client leaves at
// 0 are the CLI defaults, clamped into the ceilings. --cleanup removes the
// session directories at shutdown (opt-in). The Promela frontend (G1) is
// linked through mcp.PromelaViaCLI (G4), so mc_parse accepts Promela with
// the same rejections as `mcd parse --promela`.
func serve(args []string) int {
	fs := flag.NewFlagSet("mcd serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	sessionDir := fs.String("session-dir", os.Getenv("MCD_SESSION_DIR"), "base directory for session directories (default $MCD_SESSION_DIR, else a temporary directory)")
	var allow stringList
	fs.Var(&allow, "allow-read", "directory whose files mc_parse may read when the client names them (repeatable)")
	maxStates := fs.Int("max-states", 0, "ceiling for the states budget (0 = none)")
	maxDepth := fs.Int("max-depth", 0, "ceiling for the depth budget (0 = none)")
	maxMS := fs.Int64("max-ms", 0, "ceiling for the time budget in ms (0 = none)")
	maxMem := fs.Int64("max-memory-mb", 0, "ceiling for the memory budget in MiB (0 = none)")
	concurrency := fs.Int("concurrency", 2, "maximum simultaneous mc_check/mc_estimate runs")
	maxWorkers := fs.Int("max-workers", 0, "ceiling for the workers of one mc_check call (0 = GOMAXPROCS); the CPU the server may use is concurrency times this")
	cleanup := fs.Bool("cleanup", false, "remove session directories at shutdown")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "mcd serve: unexpected arguments %q\n", fs.Args())
		return 1
	}
	srv, err := mcp.New(mcp.Config{
		SessionBase: *sessionDir,
		Cleanup:     *cleanup,
		AllowRead:   allow,
		Ceiling:     mcp.Budget{States: *maxStates, Depth: *maxDepth, MS: *maxMS, MemoryMB: *maxMem},
		Concurrency: *concurrency,
		MaxWorkers:  *maxWorkers,
		Promela:     mcp.PromelaViaCLI,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcd serve:", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "mcd serve %s: sessions under %s\n", report.EngineVersion, srv.Sessions().Base())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "mcd serve:", err)
		return 1
	}
	return 0
}

// dispatch routes `serve` to the MCP server and everything else to the CLI
// (cli.Run), so that main.go registers the command in one line.
func dispatch(args []string, cliRun func([]string, io.Writer, io.Writer) int) int {
	if len(args) > 0 && args[0] == "serve" {
		return serve(args[1:])
	}
	return cliRun(args, os.Stdout, os.Stderr)
}

// stringList is a repeatable string flag.
type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }
