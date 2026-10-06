package ir

import "testing"

// TestWalkExprsVisitsEveryExpressionSlot: an analysis that asks what a model
// reads (the process-table test does) must see an expression wherever the IR
// can carry one. Each slot below holds a constant of its own; a slot that
// walkExprs misses is a read the analysis never sees. A slot added to the IR
// and not to the walk fails here.
func TestWalkExprsVisitsEveryExpressionSlot(t *testing.T) {
	c := func(v int64) *Expr { return Const(v) }
	m := &Model{
		Processes: []Process{
			{
				Name:     "P",
				Provided: c(1),
				Edges: []Edge{
					{Guard: c(2), Assert: c(3),
						Effect: []Assign{{Var: "x", Index: c(4), Value: c(5)}},
						Send:   &ChanOp{Chan: "c", Sel: c(6), Args: []*Expr{c(7), c(8)}}},
					{Recv: &RecvOp{Chan: "c", Sel: c(9), Args: []RecvArg{{Var: "y", Index: c(10), Match: c(11)}}}},
					{Run: &RunOp{Args: []*Expr{c(12)}, Init: []Assign{{Var: "z", Index: c(13), Value: c(14)}}}},
				},
			},
		},
		Properties: []Property{{ID: "p", Kind: KindInvariant, Expr: Binary("add", c(15), c(16))}},
	}
	seen := map[int64]bool{}
	walkExprs(m, func(e *Expr) {
		if e.Op == "const" {
			seen[e.Value] = true
		}
	})
	for v := int64(1); v <= 16; v++ {
		if !seen[v] {
			t.Errorf("the expression holding the constant %d was not visited", v)
		}
	}
}
