package explore

import (
	"fmt"
	"os"
	"testing"

	"modelcheck/ir"
)

// Directed models of the parallel search that the feature file also runs
// (testdata/ir/par-*.json, generated from these constructors).

// parInitialViolation: the invariant is false and the reach condition true in
// the initial state itself (scenario 26).
func parInitialViolation() *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "par-initial",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte, Init: []int64{1}}},
		Processes: []ir.Process{{Name: "P", Locations: locs("l0", "l1"), Edges: []ir.Edge{
			{From: 0, To: 1, Effect: []ir.Assign{{Var: "x", Value: ir.Const(2)}}, Text: "x = 2"},
		}}},
		Properties: []ir.Property{
			{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("eq", ir.Ref("x"), ir.Const(2)), Text: "x == 2"},
			{ID: "can", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.Ref("x"), ir.Const(1)), Text: "x == 1"},
			{ID: "deadlock", Kind: ir.KindDeadlock},
		}}
}

// parInitialError: evaluating the invariant on the initial state fails (the
// array index is out of range): the model is invalid at once (scenario 26).
func parInitialError() *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "par-initial-error",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte, Init: []int64{5}}, {Name: "a", Type: ir.Byte, Len: 2}},
		Processes: []ir.Process{{Name: "P", Locations: locs("l0", "l1"), Edges: []ir.Edge{
			{From: 0, To: 1, Text: "skip"},
		}}},
		Properties: []ir.Property{
			{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("eq", ir.Index("a", ir.Ref("x")), ir.Const(0)), Text: "a[x] == 0"},
			{ID: "deadlock", Kind: ir.KindDeadlock},
		}}
}

// parDecided: the invariant a[i] == 0 is violated in layer 1; in layer 2 the
// index i is out of range, but a decided property is not evaluated again, so
// the deadlock property is still verified and the run completes (scenario 24).
func parDecided() *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "par-decided",
		Globals: []ir.Var{{Name: "i", Type: ir.Byte}, {Name: "a", Type: ir.Byte, Len: 2}},
		Processes: []ir.Process{{Name: "P", Locations: locs("l0", "l1", "l2", "l3"), Edges: []ir.Edge{
			{From: 0, To: 1, Effect: []ir.Assign{{Var: "a", Index: ir.Const(0), Value: ir.Const(1)}}, Text: "a[0] = 1"},
			{From: 1, To: 2, Effect: []ir.Assign{{Var: "i", Value: ir.Const(5)}}, Text: "i = 5"},
			{From: 2, To: 3, Text: "skip"},
		}}},
		Properties: []ir.Property{
			{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("eq", ir.Index("a", ir.Ref("i")), ir.Const(0)), Text: "a[i] == 0"},
			{ID: "deadlock", Kind: ir.KindDeadlock},
		}}
}

// parSkippedReach: after the first move the invariant a[i] == 0 is violated
// (state A), and after the second the index i is 5, so evaluating the invariant
// fails (state B). Both states are in layer 1, and so in the same group. The
// invariant is decided by A, which comes first, so the sequential search does
// not evaluate it on B, and checks the reach condition i == 5 there. The error of
// the invariant on B is dropped when the group's events are applied, and must
// not take the reach condition's check on B with it (scenario 36).
func parSkippedReach() *ir.Model {
	m := parSkipBase("par-skip-reach")
	m.Properties = append(m.Properties,
		ir.Property{ID: "reach5", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.Ref("i"), ir.Const(5)), Text: "i == 5"})
	return m
}

// parSkippedInvariant is parSkippedReach with a second invariant, i != 5, which
// is violated on B and never otherwise.
func parSkippedInvariant() *ir.Model {
	m := parSkipBase("par-skip-inv")
	m.Properties = append(m.Properties,
		ir.Property{ID: "inv5", Kind: ir.KindInvariant, Expr: ir.Binary("ne", ir.Ref("i"), ir.Const(5)), Text: "i != 5"})
	return m
}

// parSkipBase is the model of parSkippedReach and parSkippedInvariant without
// the second property.
func parSkipBase(name string) *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: name,
		Globals: []ir.Var{{Name: "i", Type: ir.Byte}, {Name: "a", Type: ir.Byte, Len: 2}},
		Processes: []ir.Process{{Name: "P", Locations: locs("l0", "l1", "l2"), Edges: []ir.Edge{
			{From: 0, To: 1, Effect: []ir.Assign{{Var: "a", Index: ir.Const(0), Value: ir.Const(1)}}, Text: "a[0] = 1"},
			{From: 0, To: 2, Effect: []ir.Assign{{Var: "i", Value: ir.Const(5)}}, Text: "i = 5"},
		}}},
		Properties: []ir.Property{
			{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("eq", ir.Index("a", ir.Ref("i")), ir.Const(0)), Text: "a[i] == 0"},
		}}
}

// parAssertOverflow: one edge carries an assert that is false and an effect
// that overflows its variable. The step is an error of the model, and the
// assert that the same step failed is not reported (scenario 30).
func parAssertOverflow() *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "par-assert-overflow",
		Globals: []ir.Var{byteVar("x")},
		Processes: []ir.Process{{Name: "P", Locations: locs("l0", "l1"), Edges: []ir.Edge{
			{From: 0, To: 1, Assert: ir.Const(0), Effect: []ir.Assign{{Var: "x", Value: ir.Const(300)}}, Text: "assert(0); x = 300"},
		}}},
		Properties: []ir.Property{{ID: "assert", Kind: ir.KindAssert}, {ID: "deadlock", Kind: ir.KindDeadlock}}}
}

// parAtomicLoop: one process whose only edge is atomic and loops forever, so
// the atomic sequence never ends (scenario 25).
func parAtomicLoop() *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "par-atomic-loop",
		Globals: []ir.Var{byteVar("x")},
		Processes: []ir.Process{{Name: "P", Locations: locs("l0"), Edges: []ir.Edge{
			{From: 0, To: 0, Atomic: true, Effect: []ir.Assign{{Var: "x", Value: ir.Binary("mod", ir.Binary("add", ir.Ref("x"), ir.Const(1)), ir.Const(2))}}, Text: "x = (x + 1) % 2"},
		}}},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}, {ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("lt", ir.Ref("x"), ir.Const(5)), Text: "x < 5"}}}
}

// parClaimModelIR is parClaimModel with its two safety properties: the claim
// is a process and the run does not ask for its property (scenario 28).
func parClaimModelIR(t testing.TB) *ir.Model {
	m := parClaimModel(t)
	m.Properties = []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}, {ID: "assert", Kind: ir.KindAssert}}
	return m
}

// fanOutModel: every state has `fan` successors, one per value of the counter
// it can jump to, so the records of a group are far more than its states.
func fanOutModel(fan int, assertFails bool) *ir.Model {
	p := ir.Process{Name: "P", Locations: locs("l0")}
	for i := 0; i < fan; i++ {
		e := ir.Edge{From: 0, To: 0, Effect: []ir.Assign{{Var: "s", Value: ir.Const(int64(i))}}, Text: fmt.Sprintf("s = %d", i)}
		if assertFails {
			e.Assert = ir.Const(0)
		}
		p.Edges = append(p.Edges, e)
	}
	return &ir.Model{Schema: ir.Schema, Name: "fan-out", Globals: []ir.Var{{Name: "s", Type: ir.Short}},
		Processes:  []ir.Process{p},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}, {ID: "assert", Kind: ir.KindAssert}}}
}

// TestGenerateParallelTestdata writes the IR files of the parallel feature.
// Run explicitly:
//
//	MCD_GEN_TESTDATA=1 go test ./explore -run TestGenerateParallelTestdata
func TestGenerateParallelTestdata(t *testing.T) {
	if os.Getenv("MCD_GEN_TESTDATA") == "" {
		t.Skip("set MCD_GEN_TESTDATA=1 to regenerate testdata/ir/par-*.json")
	}
	for file, m := range map[string]*ir.Model{
		"par-initial.json":         parInitialViolation(),
		"par-initial-error.json":   parInitialError(),
		"par-decided.json":         parDecided(),
		"par-skip-reach.json":      parSkippedReach(),
		"par-skip-inv.json":        parSkippedInvariant(),
		"par-assert-overflow.json": parAssertOverflow(),
		"par-atomic-loop.json":     parAtomicLoop(),
		"par-claim.json":           parClaimModelIR(t),
		"par-fanout.json":          fanOutModel(600, false),
		"par-fanout-assert.json":   fanOutModel(600, true),
	} {
		b, err := ir.MarshalJSON(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("../testdata/ir/"+file, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
