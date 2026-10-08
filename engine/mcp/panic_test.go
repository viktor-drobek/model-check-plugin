package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// lockedBuffer is the server's standard error in a test: the handler's
// goroutine writes it while the test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// panicArgs are arguments of each tool that get past its own argument checks,
// so that the call is opened in the manifest and the injected fault is the
// first thing that goes wrong. The session is the one that parsed the net.
func panicArgs(tool, session string, net map[string]any) map[string]any {
	switch tool {
	case "mc_parse":
		return map[string]any{"session_id": session, "petri": net}
	case "mc_simulate":
		return map[string]any{"session_id": session, "mode": "random", "steps": 3}
	case "mc_explain":
		return map[string]any{"session_id": session, "counterexample_id": "cex-1"}
	case "mc_lint_property":
		return map[string]any{"session_id": session, "kind": "invariant", "expr": "p0"}
	}
	return map[string]any{"session_id": session}
}

// TestPanicInAnyToolIsAnErrorInTheManifest: a panic inside any of the seven
// tools is a tool failure for the client (isError, the cause in the message,
// the stack on the server's standard error, the next call served) and an
// error in the session manifest, as an ordinary tool error is. The panic is
// injected through Server.fault, right after the call is opened in the
// manifest: the place from which a defect in the handler would unwind.
func TestPanicInAnyToolIsAnErrorInTheManifest(t *testing.T) {
	data, err := os.ReadFile("../testdata/petri/petrinet1.json")
	if err != nil {
		t.Fatal(err)
	}
	var net map[string]any
	if err := json.Unmarshal(data, &net); err != nil {
		t.Fatal(err)
	}
	if len(ToolNames) != 7 {
		t.Fatalf("%d tools: this test names the arguments of each of the seven", len(ToolNames))
	}
	for _, tool := range ToolNames {
		t.Run(tool, func(t *testing.T) {
			srv, err := New(Config{SessionBase: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer srv.Close()
			stderr := &lockedBuffer{}
			srv.errw = stderr
			var armed atomic.Bool
			srv.fault = func(name string) {
				if armed.Load() && name == tool {
					panic("injected failure in " + name)
				}
			}
			ctx := context.Background()
			ct, st := sdk.NewInMemoryTransports()
			if _, err := srv.Connect(ctx, st); err != nil {
				t.Fatal(err)
			}
			cs, err := sdk.NewClient(&sdk.Implementation{Name: "panic-test", Version: "0"}, nil).Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			call := func(name string, args map[string]any) *sdk.CallToolResult {
				t.Helper()
				res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
				if err != nil {
					t.Fatalf("%s: protocol error (the server did not answer): %v", name, err)
				}
				return res
			}
			text := func(res *sdk.CallToolResult) string {
				var b strings.Builder
				for _, c := range res.Content {
					if tc, ok := c.(*sdk.TextContent); ok {
						b.WriteString(tc.Text)
					}
				}
				return b.String()
			}

			// A session with a parsed model, then the fault.
			first := call("mc_parse", map[string]any{"petri": net})
			if first.IsError {
				t.Fatalf("setup: %s", text(first))
			}
			session, _ := first.StructuredContent.(map[string]any)["session_id"].(string)
			if session == "" {
				t.Fatalf("setup: no session id in %v", first.StructuredContent)
			}
			armed.Store(true)
			res := call(tool, panicArgs(tool, session, net))
			armed.Store(false)

			// The client: a tool error with the cause, nothing claimed.
			msg := text(res)
			if !res.IsError || res.StructuredContent != nil {
				t.Fatalf("a panic in %s is a tool error, got isError=%v content=%v", tool, res.IsError, res.StructuredContent)
			}
			for _, want := range []string{"internal error in " + tool, "injected failure in " + tool} {
				if !strings.Contains(msg, want) {
					t.Errorf("client message %q does not mention %q", msg, want)
				}
			}
			// The stack is on the server's standard error, and only there.
			if got := stderr.String(); !strings.Contains(got, "injected failure in "+tool) || !strings.Contains(got, "goroutine ") || !strings.Contains(got, "panic_test.go") {
				t.Errorf("the server's standard error lacks the panic and the stack of its origin: %q", got)
			}
			if strings.Contains(msg, "goroutine ") {
				t.Errorf("the stack leaked into the client message: %q", msg)
			}

			// The manifest, in memory and as the file in the session
			// directory, records the call as an error with the cause and
			// without the stack.
			sess, err := srv.Sessions().Get(session)
			if err != nil {
				t.Fatal(err)
			}
			m, _ := sess.snapshot()
			fileData, err := os.ReadFile(filepath.Join(sess.Dir, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var onDisk Manifest
			if err := json.Unmarshal(fileData, &onDisk); err != nil {
				t.Fatal(err)
			}
			for _, view := range []struct {
				name  string
				calls []Call
			}{{"manifest", m.Calls}, {"manifest.json", onDisk.Calls}} {
				// The setup parse and the panicked call, nothing else.
				if len(view.calls) != 2 || view.calls[1].Tool != tool {
					t.Fatalf("%s: want the calls [mc_parse %s], got %+v", view.name, tool, view.calls)
				}
				last := &view.calls[1]
				if last.Outcome != "error" {
					t.Errorf("%s: the panicked %s call has outcome %q, want \"error\" (an ordinary tool error is recorded as one)", view.name, tool, last.Outcome)
				}
				for _, want := range []string{"internal error in " + tool, "injected failure in " + tool} {
					if !strings.Contains(last.Error, want) {
						t.Errorf("%s: error %q does not mention %q", view.name, last.Error, want)
					}
				}
				if strings.Contains(last.Error, "goroutine ") || strings.Contains(last.Error, ".go:") {
					t.Errorf("%s: the stack is in the manifest: %q", view.name, last.Error)
				}
			}

			// The next call is served, and the manifest it returns says the same.
			after := call("mc_manifest", map[string]any{"session_id": session})
			if after.IsError {
				t.Fatalf("the call after the panic: %s", text(after))
			}
			shown, _ := after.StructuredContent.(map[string]any)["manifest"].(map[string]any)
			calls, _ := shown["calls"].([]any)
			var found bool
			for _, c := range calls {
				cm, _ := c.(map[string]any)
				if cm["tool"] == tool && cm["outcome"] == "error" {
					found = true
				}
			}
			if !found {
				t.Errorf("mc_manifest after the panic does not show %s as an error: %v", tool, calls)
			}
		})
	}
}
