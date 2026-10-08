package explore

import (
	"strings"
	"testing"

	"modelcheck/ir"
)

func runPOR(t *testing.T, m *ir.Model, por bool, mode Mode) *Result {
	t.Helper()
	return run(t, m, Options{POR: por, Mode: mode, Sweep: true})
}

func sameStatuses(t *testing.T, name string, a, b *Result) {
	t.Helper()
	if len(a.Outcomes) != len(b.Outcomes) {
		t.Fatalf("%s: %d outcomes against %d", name, len(a.Outcomes), len(b.Outcomes))
	}
	for i := range a.Outcomes {
		x, y := a.Outcomes[i], b.Outcomes[i]
		if x.Status != y.Status || x.Evidence != y.Evidence {
			t.Fatalf("%s: property %s: %s/%s with the reduction, %s/%s without (%s | %s)",
				name, x.Property.ID, x.Status, x.Evidence, y.Status, y.Evidence, x.Reason, y.Reason)
		}
	}
}

// independentCounters is n processes counting a private byte to k: (k+2)
// local states each, so (k+2)^n in full and one process after the other in
// the reduction.
func independentCounters(n, k int) *ir.Model {
	m := &ir.Model{Schema: ir.Schema, Name: "independent",
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}}}
	for i := 0; i < n; i++ {
		m.Processes = append(m.Processes, ir.Process{
			Name:      string(rune('A' + i)),
			Locals:    []ir.Var{{Name: "c", Type: ir.Byte}},
			Locations: locs("count", "done"),
			Edges: []ir.Edge{
				{From: 0, To: 0, Guard: ir.Binary("lt", ir.Ref("c"), ir.Const(int64(k))),
					Effect: []ir.Assign{{Var: "c", Value: ir.Binary("add", ir.Ref("c"), ir.Const(1))}}},
				{From: 0, To: 1, Guard: ir.Binary("ge", ir.Ref("c"), ir.Const(int64(k)))},
			},
		})
	}
	return m
}

func TestPORIndependentProcessesRunOneAfterTheOther(t *testing.T) {
	m := independentCounters(4, 3)
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if full.States != 625 { // (3+1 counts + done) ^ 4
		t.Fatalf("full search: %d states, want 625", full.States)
	}
	// One chain of 4*(3+1) moves: 17 states.
	if red.States != 17 || red.Transitions != 16 {
		t.Fatalf("reduced search: %d states, %d transitions, want 17 and 16", red.States, red.Transitions)
	}
	if r := red.Reduction; r == nil || !r.Applied || r.ReducedStates != 16 || r.FullStates != 1 {
		t.Fatalf("reduction record %+v (the last state is a dead end, expanded in full)", r)
	}
	sameStatuses(t, "independent", red, full)
	if full.Reduction != nil {
		t.Fatalf("a run without POR carries a reduction: %+v", full.Reduction)
	}
}

func TestPORKeepsAWriteThatAPropertyReads(t *testing.T) {
	m := porVisible()
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	sameStatuses(t, "visible", red, full)
	if outcome(t, red, "inv").Status != Violated || outcome(t, red, "can1").Status != Verified {
		t.Fatalf("inv %s, can1 %s", outcome(t, red, "inv").Status, outcome(t, red, "can1").Status)
	}
	if red.States >= full.States {
		t.Fatalf("reduced %d states, full %d", red.States, full.States)
	}
}

// skipLoopAndWrite: P loops forever on a step that touches nothing, Q writes
// the x that the invariant reads. Taking P's loop alone is eligible in every
// state, so without the cycle proviso Q would never run.
func skipLoopAndWrite() *ir.Model {
	return model([]ir.Var{byteVar("x")}, nil,
		[]ir.Property{{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("eq", ir.Ref("x"), ir.Const(0))}},
		proc("P", nil, 1, ir.Edge{From: 0, To: 0}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}))
}

func TestPORCycleProvisoStopsTheIgnoringProblem(t *testing.T) {
	m := skipLoopAndWrite()
	if s := outcome(t, runPOR(t, m, false, DFS), "inv").Status; s != Violated {
		t.Fatalf("full search: inv %s, want violated", s)
	}
	if s := outcome(t, runPOR(t, m, true, DFS), "inv").Status; s != Violated {
		t.Fatalf("reduced search: inv %s, want violated", s)
	}
	// What the proviso is for: the same reduction without it never lets Q
	// move and calls the invariant verified. If this ever stops failing, the
	// test above has stopped testing the proviso.
	blind := run(t, m, Options{POR: true, porNoProviso: true, Sweep: true})
	if s := outcome(t, blind, "inv").Status; s != Verified {
		t.Fatalf("without the proviso: inv %s; the ignoring problem was meant to show here", s)
	}
}

func TestPORFallsBackToFullExpansionOnAnEvaluationError(t *testing.T) {
	// P's local counter overflows its byte: the step is an error of the model.
	m := model(nil, nil, nil,
		proc("P", []ir.Var{byteVar("c")}, 1,
			ir.Edge{From: 0, To: 0, Effect: []ir.Assign{{Var: "c", Value: ir.Binary("add", ir.Ref("c"), ir.Const(1))}}}),
		proc("Q", []ir.Var{byteVar("d")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("d", 1)}}))
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if s := outcome(t, full, "deadlock").Status; s != InvalidModel {
		t.Fatalf("full search: deadlock %s, want invalid-model", s)
	}
	if !red.Reduction.Applied {
		t.Fatalf("not applied: %s", red.Reduction.Reason)
	}
	sameStatuses(t, "overflow", red, full)
}

func TestPORRefusedRunIsTheFullRun(t *testing.T) {
	rendezvous := model(nil, []ir.Channel{{Name: "r", Capacity: 0, Fields: []ir.Type{ir.Byte}}}, nil,
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: "r", Args: []*ir.Expr{ir.Const(1)}}}),
		proc("R", nil, 2, ir.Edge{From: 0, To: 1, Recv: &ir.RecvOp{Chan: "r", Args: []ir.RecvArg{{}}}}))
	ltl := independentCounters(3, 2)
	ltl.Properties = append(ltl.Properties, ir.Property{ID: "l", Kind: ir.KindLTL, Formula: "[]true"})
	for _, c := range []struct {
		name string
		m    *ir.Model
		mode Mode
		want string
	}{
		{"rendezvous", rendezvous, DFS, "rendezvous"},
		{"temporal", ltl, DFS, "temporal"},
		{"bfs", independentCounters(3, 2), BFS, "breadth-first"},
	} {
		t.Run(c.name, func(t *testing.T) {
			full, red := runPOR(t, c.m, false, c.mode), runPOR(t, c.m, true, c.mode)
			r := red.Reduction
			if r == nil || r.Applied || !strings.Contains(r.Reason, c.want) || r.Note != "" {
				t.Fatalf("reduction record %+v, want a refusal mentioning %q", r, c.want)
			}
			if red.States != full.States || red.Transitions != full.Transitions {
				t.Fatalf("refused run: %d/%d states/transitions, full %d/%d", red.States, red.Transitions, full.States, full.Transitions)
			}
			sameStatuses(t, c.name, red, full)
		})
	}
}

func TestPORIsOffByDefault(t *testing.T) {
	m := independentCounters(3, 2)
	a, b := run(t, m, Options{Sweep: true}), run(t, m, Options{Sweep: true, POR: false})
	if a.Reduction != nil || b.Reduction != nil || a.States != b.States || a.States != 64 { // (2+1+1)^3
		t.Fatalf("default run: %d states, reduction %+v", a.States, a.Reduction)
	}
}

func TestPORNoEligibleProcessIsAFullSearch(t *testing.T) {
	// Both processes write x: nothing is eligible, the reduction is applied
	// and changes nothing.
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 2)}}))
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if !red.Reduction.Applied || red.Reduction.ReducedStates != 0 || red.States != full.States || red.Transitions != full.Transitions {
		t.Fatalf("reduction %+v, %d/%d vs full %d/%d", red.Reduction, red.States, red.Transitions, full.States, full.Transitions)
	}
}

// A property over two variables written by different processes sees the state
// "x unchanged, y changed". That state is reached only by taking the writes in
// one particular order, so the writes are visible and are kept in every order.
func TestPORKeepsEveryOrderOfWritesThatOnePropertyReads(t *testing.T) {
	m := model([]ir.Var{byteVar("x"), byteVar("y")}, nil,
		[]ir.Property{
			{ID: "reach", Kind: ir.KindReach, Expr: ir.And(ir.Binary("eq", ir.Ref("x"), ir.Const(0)), ir.Binary("eq", ir.Ref("y"), ir.Const(1)))},
			{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Unary("not", ir.And(ir.Binary("eq", ir.Ref("x"), ir.Const(2)), ir.Binary("eq", ir.Ref("y"), ir.Const(0))))},
		},
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 2)}}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("y", 1)}}))
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if outcome(t, full, "reach").Status != Verified || outcome(t, full, "inv").Status != Violated {
		t.Fatalf("full search: reach %s, inv %s", outcome(t, full, "reach").Status, outcome(t, full, "inv").Status)
	}
	sameStatuses(t, "two variables", red, full)
	if red.States != full.States {
		t.Fatalf("both writes are visible, so nothing is reduced: %d states against %d", red.States, full.States)
	}
}
