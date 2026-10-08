package explore

import (
	"strings"
	"testing"

	"modelcheck/ir"
)

// Properties that read the live-process table (_nr_pr, a pid, the youngest
// test) over a model whose processes keep none: refused, one by one, with the
// reason; the properties beside them are answered as if they were alone.

// tableless is two processes that each set a global and read nothing of the
// table: no run, no _nr_pr, no leave on any edge.
func tableless(props ...ir.Property) *ir.Model {
	return model([]ir.Var{byteVar("x")}, nil, props,
		proc("A", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		proc("B", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 2)}}))
}

func nrprIs(n int64) *ir.Expr { return ir.Binary("eq", ir.NrPr(), ir.Const(n)) }

func sane() ir.Property {
	return ir.Property{ID: "sane", Kind: ir.KindInvariant, Expr: ir.Binary("le", ir.Ref("x"), ir.Const(2))}
}

func wantRefused(t *testing.T, o *Outcome, mention string) {
	t.Helper()
	if o.Status != NotExecuted || o.Evidence != EvUnknown {
		t.Errorf("%s: status %s evidence %s (%s), want not-executed/unknown", o.Property.ID, o.Status, o.Evidence, o.Reason)
		return
	}
	for _, s := range []string{"process table", mention, "_nr_pr"} {
		if !strings.Contains(o.Reason, s) {
			t.Errorf("%s: the reason %q does not mention %q", o.Property.ID, o.Reason, s)
		}
	}
}

func TestInvariantAndReachThatReadNrPrAreRefusedWithoutATable(t *testing.T) {
	m := tableless(
		ir.Property{ID: "none", Kind: ir.KindReach, Expr: nrprIs(0)},
		ir.Property{ID: "two", Kind: ir.KindInvariant, Expr: nrprIs(2)},
		sane())
	r := run(t, m, Options{})
	wantRefused(t, outcome(t, r, "none"), "_nr_pr")
	wantRefused(t, outcome(t, r, "two"), "_nr_pr")
	if o := outcome(t, r, "sane"); o.Status != Verified || o.Evidence != Exhaustive {
		t.Errorf("sane: %s/%s (%s)", o.Status, o.Evidence, o.Reason)
	}
	if o := outcome(t, r, "deadlock"); o.Status != Verified || o.Evidence != Exhaustive {
		t.Errorf("deadlock: %s/%s (%s)", o.Status, o.Evidence, o.Reason)
	}
}

// A refused property leaves no trace on the search of the others: same
// vector, same states, same counters as when it was never sent.
func TestARefusedPropertyDoesNotChangeTheSearchOfTheOthers(t *testing.T) {
	alone := run(t, tableless(sane()), Options{})
	both := run(t, tableless(ir.Property{ID: "none", Kind: ir.KindReach, Expr: nrprIs(0)}, sane()), Options{})
	if alone.StateBytes != both.StateBytes {
		t.Errorf("state vector: %d bytes alone, %d with a refused property beside it", alone.StateBytes, both.StateBytes)
	}
	if alone.States != both.States || alone.Transitions != both.Transitions || alone.MaxDepth != both.MaxDepth {
		t.Errorf("counters: alone %d/%d/%d, with a refused property %d/%d/%d",
			alone.States, alone.Transitions, alone.MaxDepth, both.States, both.Transitions, both.MaxDepth)
	}
	for _, id := range []string{"sane", "deadlock"} {
		a, b := outcome(t, alone, id), outcome(t, both, id)
		if a.Status != b.Status || a.Evidence != b.Evidence || a.Reason != b.Reason {
			t.Errorf("%s: %s/%s alone, %s/%s beside a refused property", id, a.Status, a.Evidence, b.Status, b.Evidence)
		}
	}
}

// The sibling that used to give the model a table: an invariant that reads
// _nr_pr beside a CTL formula that reads it. Both are refused.
func TestASiblingThatReadsNrPrDoesNotSwitchOffTheCTLRefusal(t *testing.T) {
	m := tableless(
		ir.Property{ID: "ef0", Kind: ir.KindCTL, Formula: "EF (_nr_pr == 0)"},
		ir.Property{ID: "ge0", Kind: ir.KindInvariant, Expr: ir.Binary("ge", ir.NrPr(), ir.Const(0))})
	r := run(t, m, Options{})
	wantRefused(t, outcome(t, r, "ef0"), "formula")
	wantRefused(t, outcome(t, r, "ge0"), "expression")
}

// A CTL formula that does not read the table is still answered beside a
// refused invariant: the refusal is per property.
func TestTheRefusalIsPerProperty(t *testing.T) {
	m := tableless(
		ir.Property{ID: "ef2", Kind: ir.KindCTL, Formula: "EF (x == 2)"},
		ir.Property{ID: "efn", Kind: ir.KindCTL, Formula: "EF (_nr_pr == 0)"},
		ir.Property{ID: "ge0", Kind: ir.KindInvariant, Expr: ir.Binary("ge", ir.NrPr(), ir.Const(0))})
	r := run(t, m, Options{})
	if o := outcome(t, r, "ef2"); o.Status != Verified || o.Evidence != Exhaustive {
		t.Errorf("ef2: %s/%s (%s)", o.Status, o.Evidence, o.Reason)
	}
	wantRefused(t, outcome(t, r, "efn"), "formula")
	wantRefused(t, outcome(t, r, "ge0"), "expression")
}

// A pid and the youngest test can only appear in a hand-written IR; they are
// answered by the same table and refused the same way, naming the read.
func TestPidAndYoungestAreRefusedToo(t *testing.T) {
	m := tableless(
		ir.Property{ID: "pid", Kind: ir.KindInvariant, Expr: ir.Binary("ge", ir.PID(1), ir.Const(0))},
		ir.Property{ID: "yng", Kind: ir.KindReach, Expr: ir.Youngest(1)})
	r := run(t, m, Options{})
	wantRefused(t, outcome(t, r, "pid"), "pid(1)")
	wantRefused(t, outcome(t, r, "yng"), "youngest(1)")
}

// When every property is refused nothing is searched: there is nothing to
// decide, and the refusal must not cost a full exploration.
func TestOnlyRefusedPropertiesRunNoSearch(t *testing.T) {
	m := tableless()
	m.Properties = []ir.Property{{ID: "none", Kind: ir.KindReach, Expr: nrprIs(0)}}
	r := run(t, m, Options{})
	wantRefused(t, outcome(t, r, "none"), "_nr_pr")
	if r.States != 0 {
		t.Errorf("a run with only refused properties stored %d states", r.States)
	}
}

// Nothing that worked moves: a model whose own processes read _nr_pr has the
// table, and the same properties are decided.
func TestPropertiesThatReadNrPrAreAnsweredWhenTheProcessesKeepTheTable(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil,
		[]ir.Property{
			{ID: "two", Kind: ir.KindReach, Expr: nrprIs(2)},
			{ID: "three", Kind: ir.KindInvariant, Expr: nrprIs(3)},
			{ID: "ef2", Kind: ir.KindCTL, Formula: "EF (_nr_pr == 2)"},
		},
		proc("A", nil, 2, ir.Edge{From: 0, To: 1, Assert: ir.Binary("ge", ir.NrPr(), ir.Const(0))}),
		proc("B", nil, 2, ir.Edge{From: 0, To: 1}))
	r := run(t, m, Options{})
	if o := outcome(t, r, "two"); o.Status != Verified {
		t.Errorf("two: %s (%s)", o.Status, o.Reason)
	}
	if o := outcome(t, r, "three"); o.Status != Violated {
		t.Errorf("three: %s (%s)", o.Status, o.Reason)
	}
	if o := outcome(t, r, "ef2"); o.Status != Verified {
		t.Errorf("ef2: %s (%s)", o.Status, o.Reason)
	}
}

// A hand-written IR whose edges leave the table keeps it for a property that
// reads the count, as before: the processes keep the table, so the count is
// the one they maintain. The edge that leaves belongs to the youngest process
// (B, the last of the two: Edge.Leave pops the youngest live entry, so only
// the youngest process may carry it), and it has no guard of its own, so that
// the `leave` is the only reason the model has a table: the property is
// answered because of it. The count falls from 2 to 1 and never to 0.
func TestAnEdgeThatLeavesTheTableKeepsItForAProperty(t *testing.T) {
	m := model(nil, nil,
		[]ir.Property{
			{ID: "one", Kind: ir.KindReach, Expr: nrprIs(1)},
			{ID: "zero", Kind: ir.KindReach, Expr: nrprIs(0)},
		},
		proc("A", nil, 2, ir.Edge{From: 0, To: 1}),
		proc("B", nil, 2, ir.Edge{From: 0, To: 1, Leave: true}))
	if !ir.NeedsTable(m) {
		t.Fatal("an edge that leaves the table does not give the model one")
	}
	r := run(t, m, Options{})
	if o := outcome(t, r, "one"); o.Status != Verified || o.Evidence != Exhaustive {
		t.Errorf("one: %s/%s (%s), want verified: B leaves while A is live", o.Status, o.Evidence, o.Reason)
	}
	if o := outcome(t, r, "zero"); o.Status != Violated || o.Evidence != Exhaustive {
		t.Errorf("zero: %s/%s (%s), want violated: A never leaves", o.Status, o.Evidence, o.Reason)
	}
}

// A property refused for the table is not a reason to refuse the reduction:
// it is not evaluated, so it reads nothing for the analysis to make visible,
// and no process reads the table. The search of the others is reduced exactly
// as when the refused property was never sent.
func TestAPropertyRefusedForTheTableDoesNotRefuseTheReduction(t *testing.T) {
	alone := run(t, counters(3, 3), Options{POR: true})
	m := counters(3, 3)
	m.Properties = append(m.Properties, ir.Property{ID: "two", Kind: ir.KindReach, Expr: nrprIs(2)})
	both := run(t, m, Options{POR: true})
	wantRefused(t, outcome(t, both, "two"), "_nr_pr")
	if !alone.Reduction.Applied {
		t.Fatalf("the baseline is not reduced: %s", alone.Reduction.Reason)
	}
	if !both.Reduction.Applied {
		t.Fatalf("a refused property that reads _nr_pr switched the reduction off: %s", both.Reduction.Reason)
	}
	if alone.States != both.States || alone.Transitions != both.Transitions ||
		alone.Reduction.ReducedStates != both.Reduction.ReducedStates {
		t.Errorf("alone %d states, %d transitions, %d reduced; beside a refused property %d, %d, %d",
			alone.States, alone.Transitions, alone.Reduction.ReducedStates,
			both.States, both.Transitions, both.Reduction.ReducedStates)
	}
	if o := outcome(t, both, "deadlock"); o.Status != Verified || o.Evidence != Exhaustive {
		t.Errorf("deadlock: %s/%s (%s)", o.Status, o.Evidence, o.Reason)
	}
}

// A process that reads the table (an assert on _nr_pr in an edge) makes the
// model keep it, and the property that reads it is evaluated. Since step 6 of
// the performance plan the reduction models the table (cell T: `nrpr`, `pid`
// and `youngest` read it, `run` and an edge that leaves write it), so it is
// applied, and what it answers equals the full search. (Before step 6 it was
// refused for the process table; the refusal of a property that reads the
// table over a model without one is a different thing and stays: see the test
// above.)
func TestTheReductionIsAppliedWhenAProcessReadsTheTable(t *testing.T) {
	m := counters(3, 3)
	m.Processes[0].Edges[0].Assert = ir.Binary("ge", ir.NrPr(), ir.Const(0))
	m.Properties = append(m.Properties, ir.Property{ID: "two", Kind: ir.KindReach, Expr: nrprIs(2)})
	full := run(t, m, Options{})
	r := run(t, m, Options{POR: true})
	if !r.Reduction.Applied {
		t.Errorf("applied=false, reason %q; the table is a cell of the analysis since step 6", r.Reduction.Reason)
	}
	for _, id := range []string{"deadlock", "assert", "two"} {
		a, b := outcome(t, full, id), outcome(t, r, id)
		if a.Status != b.Status || a.Evidence != b.Evidence {
			t.Errorf("%s: full %s/%s, reduced %s/%s", id, a.Status, a.Evidence, b.Status, b.Evidence)
		}
	}
	if o := outcome(t, r, "two"); o.Status == NotExecuted {
		t.Errorf("two: not executed (%s), but the model keeps the table", o.Reason)
	}
}

// Simulation never evaluates a property: a model that carries one that reads
// the table is steppable, with the layout its processes ask for.
func TestStepperIgnoresAPropertyThatReadsTheTable(t *testing.T) {
	st, err := NewStepper(tableless(ir.Property{ID: "none", Kind: ir.KindReach, Expr: nrprIs(0)}))
	if err != nil {
		t.Fatal(err)
	}
	if st.Layout().HasTable() {
		t.Error("a property gave the simulated model a table")
	}
}
