package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestConcurrentCallsKeepManifestFileWhole: the SDK serves tool calls
// concurrently, so many calls end at the same time, and manifest.json, which
// every one of them rewrites, must stay a whole file that says what the
// manifest in memory says. Half of the calls are ordinary ones
// (mc_lint_property); the other half panic at the place where a defect of a
// handler would (Server.fault), so the file also has a second writer, the
// recovery that marks the call as an error. Written after the lock was
// released, the snapshots of two calls could land in the other order or one
// over the tail of the other: a file that is not JSON (a shorter snapshot over
// a longer one), or one that lacks a call or an error that memory has.
func TestConcurrentCallsKeepManifestFileWhole(t *testing.T) {
	data, err := os.ReadFile("../testdata/petri/petrinet1.json")
	if err != nil {
		t.Fatal(err)
	}
	var net map[string]any
	if err := json.Unmarshal(data, &net); err != nil {
		t.Fatal(err)
	}
	const (
		rounds = 6
		n      = 150 // calls of each kind per round
	)
	for round := 1; round <= rounds; round++ {
		t.Run(fmt.Sprintf("round %d", round), func(t *testing.T) {
			srv, err := New(Config{SessionBase: t.TempDir(), Concurrency: 8})
			if err != nil {
				t.Fatal(err)
			}
			defer srv.Close()
			srv.errw = &lockedBuffer{}
			srv.fault = func(name string) {
				if name == "mc_simulate" {
					panic("injected failure in " + name)
				}
			}
			ctx := context.Background()
			ct, st := sdk.NewInMemoryTransports()
			if _, err := srv.Connect(ctx, st); err != nil {
				t.Fatal(err)
			}
			cs, err := sdk.NewClient(&sdk.Implementation{Name: "manifest-race", Version: "0"}, nil).Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			first, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "mc_parse", Arguments: map[string]any{"petri": net}})
			if err != nil || first.IsError {
				t.Fatalf("setup: %v %v", err, first)
			}
			session, _ := first.StructuredContent.(map[string]any)["session_id"].(string)
			if session == "" {
				t.Fatalf("setup: no session id in %v", first.StructuredContent)
			}

			var wg sync.WaitGroup
			errs := make(chan error, 2*n)
			for i := 0; i < n; i++ {
				wg.Add(2)
				go func() {
					defer wg.Done()
					res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "mc_simulate", Arguments: map[string]any{"session_id": session, "mode": "random", "steps": 3}})
					if err != nil || !res.IsError {
						errs <- fmt.Errorf("a panicking mc_simulate must answer isError: %v %v", res, err)
					}
				}()
				go func() {
					defer wg.Done()
					res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "mc_lint_property", Arguments: map[string]any{"session_id": session, "kind": "invariant", "expr": "p0"}})
					if err != nil || res.IsError {
						errs <- fmt.Errorf("mc_lint_property must answer: %v %v", res, err)
					}
				}()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Error(err)
			}

			sess, err := srv.Sessions().Get(session)
			if err != nil {
				t.Fatal(err)
			}
			mem, _ := sess.snapshot()
			if len(mem.Calls) != 2*n+1 {
				t.Fatalf("the manifest in memory has %d calls, want %d", len(mem.Calls), 2*n+1)
			}
			raw, err := os.ReadFile(filepath.Join(sess.Dir, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var disk Manifest
			if err := json.Unmarshal(raw, &disk); err != nil {
				t.Fatalf("manifest.json is not valid JSON: %v", err)
			}
			if len(disk.Calls) != len(mem.Calls) {
				t.Fatalf("manifest.json has %d calls, the manifest in memory %d", len(disk.Calls), len(mem.Calls))
			}
			bad := 0
			for i := range mem.Calls {
				d, m := disk.Calls[i], mem.Calls[i]
				if d.Tool != m.Tool || d.Outcome != m.Outcome || d.Error != m.Error {
					if bad++; bad <= 3 {
						t.Errorf("call %d: manifest.json says %s/%s %q, memory %s/%s %q", i+1, d.Tool, d.Outcome, d.Error, m.Tool, m.Outcome, m.Error)
					}
				}
			}
			if bad > 3 {
				t.Errorf("... and %d more calls that differ", bad-3)
			}
			// No file of a writer that did not finish is left behind.
			entries, err := os.ReadDir(sess.Dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if strings.Contains(e.Name(), "tmp") {
					t.Errorf("a temporary file %q is left in the session directory", e.Name())
				}
			}
		})
	}
}
