package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/internal/testgen"
)

// TestLimitsKeepTheServerAlive: the inputs of issue #9 at the size that used
// to end the process reach the server through mc_parse and
// mc_lint_property, are answered as structured rejections, and the next
// ordinary calls are served by the same server.
func TestLimitsKeepTheServerAlive(t *testing.T) {
	srv, cs := connect(t, Config{SessionBase: t.TempDir(), Promela: PromelaViaCLI})
	stderr := &lockedBuffer{}
	srv.errw = stderr
	ctx := context.Background()
	call := func(name string, args map[string]any) string {
		t.Helper()
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: protocol error (the server did not answer): %v\nserver stderr: %s", name, err, stderr.String())
		}
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*sdk.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		if res.IsError {
			t.Fatalf("%s: a tool error, not a structured answer: %s", name, b.String())
		}
		return b.String()
	}
	field := func(doc, key string) string {
		var m map[string]any
		if err := json.Unmarshal([]byte(doc), &m); err != nil {
			t.Fatalf("answer is not JSON: %v\n%.300s", err, doc)
		}
		v, _ := json.Marshal(m[key])
		return string(v)
	}

	data, err := os.ReadFile("../testdata/petri/petrinet1.json")
	if err != nil {
		t.Fatal(err)
	}
	var net map[string]any
	if err := json.Unmarshal(data, &net); err != nil {
		t.Fatal(err)
	}
	out := call("mc_parse", map[string]any{"petri": net})
	if field(out, "outcome") != `"ir"` {
		t.Fatalf("the server does not answer an ordinary parse after the rejections: %.300s", out)
	}
	session := strings.Trim(field(out, "session_id"), `"`)
	out = call("mc_lint_property", map[string]any{"session_id": session, "kind": "ltl", "formula": "[] (p1 >= 0)"})
	if field(out, "type_error") != "null" {
		t.Fatalf("an ordinary lint fails: %.300s", out)
	}

	// max_procs beyond what the engine can name is refused before a pool
	// is built (a billion used to allocate a billion instances).
	out = call("mc_parse", map[string]any{"promela": "proctype P() { skip }\nactive proctype Q() { do :: run P() od }\n", "max_procs": 1000000000})
	if field(out, "outcome") != `"rejected"` || !strings.Contains(out, "254") {
		t.Fatalf("mc_parse with max_procs 1e9: want a rejection naming 254, got %.400s", out)
	}

	// mc_parse: 600 000 parentheses (1.2 MB), the doubling defines, the
	// heavy IR — each a structured rejection naming the limit.
	for i, src := range []string{testgen.NestedParens(600000), testgen.DefineDoubling(23), testgen.Chains(100, 999)} {
		out := call("mc_parse", map[string]any{"promela": src})
		if field(out, "outcome") != `"rejected"` || !strings.Contains(out, "limit") {
			t.Fatalf("mc_parse of a %d-byte input: want a rejection naming the limit, got %.400s", len(src), out)
		}
		t.Logf("parse %d (%d bytes) rejected; probing the server", i, len(src))
		out = call("mc_lint_property", map[string]any{"session_id": session, "kind": "ltl", "formula": "[] (p1 >= 0)"})
		if field(out, "type_error") != "null" {
			t.Fatalf("after parse %d an ordinary lint fails: %.300s", i, out)
		}
	}

	// mc_lint_property: the formula limits on the same session.
	// The formulas stay around a megabyte: the limits are reached long
	// before, and a JSON-RPC message has a size of its own.
	for _, c := range []struct{ kind, expr, want string }{
		{"ltl", testgen.IffChain(40), "normal form"},
		{"ltl", testgen.AndChain(150000), "deeper than 1000"},
		{"ctl", testgen.AUNest(14, "x == 1"), "normal form"},
		{"ctl", testgen.FormulaParens(600000), "deeper than 1000"},
	} {
		// The session holds petrinet1: its places are the variables, so the
		// generated atoms over x are renamed to p1 (CTL checks every atom
		// against the model as it parses).
		formula := strings.ReplaceAll(c.expr, "x ", "p1 ")
		t.Logf("lint %s of %d bytes", c.kind, len(formula))
		out := call("mc_lint_property", map[string]any{"session_id": session, "kind": c.kind, "formula": formula})
		if te := field(out, "type_error"); !strings.Contains(te, c.want) {
			t.Fatalf("mc_lint_property %s of a %d-byte formula: type_error %s, want %q", c.kind, len(c.expr), te, c.want)
		}
	}
	out = call("mc_lint_property", map[string]any{"session_id": session, "kind": "ltl", "formula": "[] (p1 >= 0)"})
	if field(out, "type_error") != "null" {
		t.Fatalf("an ordinary lint after the rejections fails: %.300s", out)
	}
}
