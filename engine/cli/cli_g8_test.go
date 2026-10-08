package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"modelcheck/explore"
)

// Surface of performance plan 5 (the parallel search) in the CLI: the exit code
// of a defect of the engine is not the exit code of a refusal, the usage errors
// of --workers, and the mode a report states.

func TestAnInternalErrorIsAToolErrorNotARejection(t *testing.T) {
	var out, errb bytes.Buffer
	code := runFailure(fmt.Errorf("wrapped: %w", &explore.InternalError{Msg: "a worker panicked: boom"}), &out, &errb)
	if code != ExitTool {
		t.Fatalf("exit %d, want %d", code, ExitTool)
	}
	if out.Len() != 0 {
		t.Fatalf("a defect of the engine printed a document on stdout: %s", out.String())
	}
	if !strings.Contains(errb.String(), "internal") || !strings.Contains(errb.String(), "boom") {
		t.Fatalf("stderr %q", errb.String())
	}
	// Anything else is the input being refused.
	out.Reset()
	errb.Reset()
	if code := runFailure(errors.New("undeclared variable"), &out, &errb); code != ExitRejected || !strings.Contains(out.String(), "not-executed") {
		t.Fatalf("a compile error: exit %d, stdout %s", code, out.String())
	}
}

func TestWorkersUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"check", "--promela", "../testdata/promela/por-shared.pml", "--workers", "-1"},
		{"check", "--promela", "../testdata/promela/por-shared.pml", "--workers", "257"},
		{"check", "--promela", "../testdata/promela/por-shared.pml", "--workers", "2", "--estimate"},
	} {
		var out, errb bytes.Buffer
		if code := Run(args, &out, &errb); code != ExitTool {
			t.Fatalf("%v: exit %d, want %d (stdout %q)", args, code, ExitTool, out.String())
		}
		if !strings.Contains(errb.String(), "--workers") {
			t.Fatalf("%v: stderr %q does not name the flag", args, errb.String())
		}
	}
	// 0 and 256 are accepted; 0 with --estimate is the default and no error.
	for _, args := range [][]string{
		{"check", "--promela", "../testdata/promela/por-shared.pml", "--workers", "0", "--no-timing"},
		{"check", "--promela", "../testdata/promela/por-shared.pml", "--workers", "256", "--no-timing", "--sweep"},
	} {
		var out, errb bytes.Buffer
		if code := Run(args, &out, &errb); code != ExitOK {
			t.Fatalf("%v: exit %d: %s", args, code, errb.String())
		}
	}
}

func TestReportedMode(t *testing.T) {
	applied := &explore.Result{Parallel: &explore.Parallel{Requested: 4, Applied: true}}
	refused := &explore.Result{Parallel: &explore.Parallel{Requested: 4, Reason: "x"}}
	for _, tc := range []struct {
		requested explore.Mode
		res       *explore.Result
		want      explore.Mode
	}{
		{explore.DFS, nil, explore.DFS},
		{explore.DFS, &explore.Result{}, explore.DFS},
		{explore.DFS, applied, explore.BFS},
		{explore.BFS, applied, explore.BFS},
		{explore.DFS, refused, explore.DFS},
		{explore.BFS, refused, explore.BFS},
	} {
		if got := ReportedMode(tc.requested, tc.res); got != tc.want {
			t.Errorf("requested %s, result %+v: mode %s, want %s", tc.requested, tc.res, got, tc.want)
		}
	}
}
