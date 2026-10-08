package mcp

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"modelcheck/explore"
)

func TestAnInternalErrorIsAToolErrorNotARejection(t *testing.T) {
	s := &Server{errw: io.Discard}
	err := s.internalToolError(&explore.InternalError{Msg: "a worker panicked: boom"})
	if err == nil || !strings.HasPrefix(err.Error(), "internal:") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("tool error %v", err)
	}
	var ie *explore.InternalError
	if !errors.As(err, &ie) {
		t.Fatal("the tool error does not wrap the internal error")
	}
	if got := s.internalToolError(errors.New("undeclared variable")); got != nil {
		t.Fatalf("an ordinary run error became a tool error: %v", got)
	}
}

func TestWorkersCeilingDefaultsToGOMAXPROCSAndIsBounded(t *testing.T) {
	c := Config{}
	if err := c.normalize(); err != nil {
		t.Fatal(err)
	}
	if c.MaxWorkers < 1 || c.MaxWorkers > explore.MaxWorkers {
		t.Fatalf("default worker ceiling %d", c.MaxWorkers)
	}
	c = Config{MaxWorkers: 100000}
	if err := c.normalize(); err != nil {
		t.Fatal(err)
	}
	if c.MaxWorkers != explore.MaxWorkers {
		t.Fatalf("worker ceiling %d, want %d", c.MaxWorkers, explore.MaxWorkers)
	}
}

// The two nets for a defect of the engine must give one story. A panic in a tool
// handler (recoverTool) keeps the stack out of the answer and out of the
// manifest and writes it to the server's standard error; a panic in a parallel
// worker comes back from explore.Run as an InternalError whose message carries
// the stack, and it used to travel unchanged into the isError answer and into
// the manifest entry of the call (steps/perf5-confirmation.md left it as "decided
// not to fix"; the integration with fix/cycle-lasso-panic made it a mismatch).
func TestAnInternalErrorOfTheParallelSearchKeepsItsStackOutOfTheAnswer(t *testing.T) {
	var stderr bytes.Buffer
	s := &Server{errw: &stderr}
	stack := "goroutine 7 [running]:\nmodelcheck/explore.(*parWorker).walk(...)\n\t/home/someone/src/walk.go:12"
	err := s.internalToolError(&explore.InternalError{Msg: "a worker panicked: boom\n" + stack})
	if err == nil {
		t.Fatal("no tool error")
	}
	for _, bad := range []string{"goroutine", "walk.go", "\n"} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("the tool error carries %q: %q", bad, err.Error())
		}
	}
	for _, want := range []string{"internal:", "boom", "defect of mcd"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the tool error %q does not say %q", err.Error(), want)
		}
	}
	var ie *explore.InternalError
	if !errors.As(err, &ie) {
		t.Error("the tool error does not wrap the internal error")
	}
	if !strings.Contains(stderr.String(), "goroutine 7") || !strings.Contains(stderr.String(), "boom") {
		t.Errorf("the stack is not on the server's standard error: %q", stderr.String())
	}
	if got := s.internalToolError(errors.New("undeclared variable")); got != nil {
		t.Fatalf("an ordinary run error became a tool error: %v", got)
	}
	if strings.Count(stderr.String(), "boom") != 1 {
		t.Errorf("the failure is written to standard error more than once: %q", stderr.String())
	}
}
