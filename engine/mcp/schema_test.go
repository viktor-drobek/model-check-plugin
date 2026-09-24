package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect starts a server with the given config over an in-memory transport
// and returns a connected client session.
func connect(t *testing.T, cfg Config) (*Server, *sdk.ClientSession) {
	t.Helper()
	if cfg.SessionBase == "" {
		cfg.SessionBase = t.TempDir()
	}
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ct, st := sdk.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, st); err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return srv, cs
}

// TestSchemas: the SDK derives an object schema for every input and output
// type; the tool list is exactly the seven names in order; descriptions
// carry the `jsonschema` tags.
func TestSchemas(t *testing.T) {
	_, cs := connect(t, Config{})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		for _, sch := range []struct {
			what string
			s    any
		}{{"input", tool.InputSchema}, {"output", tool.OutputSchema}} {
			m, ok := sch.s.(map[string]any)
			if !ok || m["type"] != "object" {
				t.Errorf("%s: %s schema is not an object: %v", tool.Name, sch.what, sch.s)
			}
		}
		if tool.Description == "" {
			t.Errorf("%s: no description", tool.Name)
		}
	}
	// The SDK lists tools sorted by name; the contract is the set.
	want := append([]string(nil), ToolNames...)
	sort.Strings(names)
	sort.Strings(want)
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("tools = %v, want %v", names, want)
	}
	// A spot check that a struct tag became a property description and that
	// the recursive IR is an open field, not a byte array.
	for _, tool := range res.Tools {
		if tool.Name != "mc_check" {
			continue
		}
		b, _ := json.Marshal(tool.InputSchema)
		s := string(b)
		if !strings.Contains(s, "clamped") {
			t.Errorf("mc_check input schema lacks the budget description: %s", s)
		}
		if strings.Contains(s, `"maximum":255`) {
			t.Errorf("mc_check input schema encodes a byte array: %s", s)
		}
	}
}

// TestToolErrorIsNotAResult: a tool failure is an isError result with text
// and no structured content (NFR-007 seam).
func TestToolErrorIsNotAResult(t *testing.T) {
	_, cs := connect(t, Config{})
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "mc_manifest", Arguments: map[string]any{"session_id": "nope"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || res.StructuredContent != nil {
		t.Fatalf("expected isError without structured content, got %+v", res)
	}
	txt, _ := res.Content[0].(*sdk.TextContent)
	if txt == nil || !strings.Contains(txt.Text, "unknown session") {
		t.Errorf("error text = %v", res.Content)
	}
}

// TestConcurrencySemaphore: the semaphore is sized by Config.Concurrency
// and acquire fails when the context ends first.
func TestConcurrencySemaphore(t *testing.T) {
	srv, err := New(Config{SessionBase: t.TempDir(), Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	release, err := srv.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := srv.acquire(ctx); err == nil {
		t.Error("second acquire succeeded with the slot taken")
	}
	release()
	if r, err := srv.acquire(context.Background()); err != nil {
		t.Error(err)
	} else {
		r()
	}
}

// TestAllowRead: without --allow-read a file is refused; with it, only files
// under the prefix are read.
func TestAllowRead(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.json")
	os.WriteFile(f, []byte("{}"), 0o644)
	srv, _ := New(Config{SessionBase: t.TempDir()})
	if _, err := srv.allowedRead(f); err == nil || !strings.Contains(err.Error(), "--allow-read") {
		t.Errorf("no allow list: %v", err)
	}
	srv, _ = New(Config{SessionBase: t.TempDir(), AllowRead: []string{dir}})
	if _, err := srv.allowedRead(f); err != nil {
		t.Errorf("allowed file refused: %v", err)
	}
	other := filepath.Join(t.TempDir(), "y.json")
	os.WriteFile(other, []byte("{}"), 0o644)
	if _, err := srv.allowedRead(other); err == nil {
		t.Error("file outside the prefix was allowed")
	}
	if _, err := srv.allowedRead(filepath.Join(dir, "..", "y.json")); err == nil {
		t.Error("lexical escape was allowed")
	}
}
