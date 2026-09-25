package mcp

import (
	"strings"
	"testing"

	"modelcheck/ir"
)

type lintScope struct{}

func (lintScope) LookupVar(name string) *ir.Var {
	if name == "x" || name == "done" {
		return &ir.Var{Name: name, Type: ir.Byte}
	}
	return nil
}
func (lintScope) LookupChan(string) *ir.Channel { return nil }
func (lintScope) ProcessCount() int             { return 1 }

func TestLintLTL(t *testing.T) {
	out := lintLTL("s", "[](p -> <>done)", map[string]string{"p": "(x > 0)"}, lintScope{})
	if !out.Temporal || !out.XFree || out.Class != "liveness" || len(out.Atoms) != 2 || len(out.Undefined) != 0 || !out.TypeOK {
		t.Fatalf("%+v", out)
	}
	if out.NNF != "[](!x > 0 || <>done)" {
		t.Fatalf("nnf %q", out.NNF)
	}
	found := false
	for _, n := range out.Notes {
		if strings.Contains(n, "antecedent") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no vacuity note: %v", out.Notes)
	}
	out = lintLTL("s", "[]!nosuch", nil, lintScope{})
	if out.Class != "safety" || len(out.Undefined) != 1 || out.TypeOK {
		t.Fatalf("%+v", out)
	}
	out = lintLTL("s", "X x", nil, lintScope{})
	if out.XFree {
		t.Fatal("X x flagged x_free")
	}
	out = lintLTL("s", "[] (", nil, lintScope{})
	if out.TypeError == "" || out.Class != "unknown" {
		t.Fatalf("%+v", out)
	}
}
