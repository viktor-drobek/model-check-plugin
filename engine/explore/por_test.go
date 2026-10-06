package explore

import (
	"strings"
	"testing"

	"modelcheck/ir"
)

// ---- builders -------------------------------------------------------------

func locs(names ...string) []ir.Location {
	out := make([]ir.Location, len(names))
	for i, n := range names {
		out[i] = ir.Location{Name: n}
	}
	return out
}

func set(v string, val int64) ir.Assign { return ir.Assign{Var: v, Value: ir.Const(val)} }

// proc builds a process with the given edges over len(edges)+1 locations
// unless the test lists its own.
func proc(name string, locals []ir.Var, nloc int, edges ...ir.Edge) ir.Process {
	l := make([]ir.Location, nloc)
	return ir.Process{Name: name, Locals: locals, Locations: l, Edges: edges}
}

func model(globals []ir.Var, chans []ir.Channel, props []ir.Property, procs ...ir.Process) *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "por-test", Globals: globals, Channels: chans,
		Processes: procs, Properties: append([]ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}}, props...)}
}

func byteVar(n string) ir.Var { return ir.Var{Name: n, Type: ir.Byte} }

func planOf(t *testing.T, m *ir.Model) *porPlan {
	t.Helper()
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	return analyzePOR(c)
}

func wantReason(t *testing.T, pl *porPlan, sub string) {
	t.Helper()
	if sub == "" {
		if pl.reason != "" {
			t.Fatalf("POR refused: %q", pl.reason)
		}
		return
	}
	if !strings.Contains(pl.reason, sub) {
		t.Fatalf("reason %q does not mention %q", pl.reason, sub)
	}
}

func wantEligible(t *testing.T, pl *porPlan, p, loc int, want bool) {
	t.Helper()
	if pl.reason != "" {
		t.Fatalf("POR refused: %q", pl.reason)
	}
	if got := pl.eligible[p][loc]; got != want {
		t.Fatalf("process %d at location %d: eligible = %v, want %v", p, loc, got, want)
	}
}

// ---- the conflict analysis ---------------------------------------------------

func TestPORLocalStepsAreEligible(t *testing.T) {
	pl := planOf(t, counters(4, 3))
	for p := 0; p < 3; p++ {
		wantEligible(t, pl, p, 0, true)
	}
}

func TestPORSharedWriteBlocksBoth(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 2)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)
}

func TestPORReadAgainstWriteBlocksBoth(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("R", nil, 2, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(1))}),
		proc("W", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false) // a write by W could enable or disable R's guard
	wantEligible(t, pl, 1, 0, false)
}

func TestPORTwoReadersDoNotConflict(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("R1", nil, 2, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(0))}),
		proc("R2", nil, 2, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(0))}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 1, 0, true)
}

func TestPORConstantIndexIsPrecise(t *testing.T) {
	arr := ir.Var{Name: "a", Type: ir.Byte, Len: 2}
	m := model([]ir.Var{arr}, nil, nil,
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{{Var: "a", Index: ir.Const(0), Value: ir.Const(1)}}}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{{Var: "a", Index: ir.Const(1), Value: ir.Const(1)}}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 1, 0, true)

	// An index that is not a constant may name any element: a[i] against a[1]
	// and against a[0].
	m = model([]ir.Var{arr}, nil, nil,
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{{Var: "a", Index: ir.Const(0), Value: ir.Const(1)}}}),
		proc("Q", []ir.Var{byteVar("i")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{{Var: "a", Index: ir.Ref("i"), Value: ir.Const(1)}}}))
	pl = planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)
	m.Processes[0].Edges[0].Effect[0].Index = ir.Const(1)
	pl = planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)
}

func TestPORLocalsNeverConflict(t *testing.T) {
	// Both processes name their local c; a name is not a cell.
	m := model(nil, nil, nil,
		proc("P", []ir.Var{byteVar("c")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("c", 1)}}),
		proc("Q", []ir.Var{byteVar("c")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("c", 2)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 1, 0, true)
}

func TestPORVisibleWriteIsNeverAmple(t *testing.T) {
	m := model([]ir.Var{byteVar("x"), byteVar("y")}, nil,
		[]ir.Property{{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("ne", ir.Ref("x"), ir.Const(9))}},
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("y", 1)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false) // writes x, which the invariant reads
	wantEligible(t, pl, 1, 0, true)  // writes y, which nobody reads
}

func TestPORReachPropertyIsVisibleToo(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil,
		[]ir.Property{{ID: "r", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.Ref("x"), ir.Const(1))}},
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		proc("Q", []ir.Var{byteVar("c")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("c", 1)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, true)
}

func TestPORAssertEdgeIsVisible(t *testing.T) {
	m := model(nil, nil, nil,
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Assert: ir.Const(1)}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, true)
}

func TestPORChannels(t *testing.T) {
	c1 := ir.Channel{Name: "c1", Capacity: 1, Fields: []ir.Type{ir.Byte}}
	c2 := ir.Channel{Name: "c2", Capacity: 1, Fields: []ir.Type{ir.Byte}}
	send := func(ch string) ir.Edge {
		return ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: ch, Args: []*ir.Expr{ir.Const(1)}}}
	}
	recv := func(ch string) ir.Edge {
		return ir.Edge{From: 0, To: 1, Recv: &ir.RecvOp{Chan: ch, Args: []ir.RecvArg{{}}}}
	}
	// The one sender and the one receiver of a buffered channel are independent
	// (the channel is directed; por_channels_test.go has the rest of that rule) ...
	pl := planOf(t, model(nil, []ir.Channel{c1}, nil, proc("S", nil, 2, send("c1")), proc("R", nil, 2, recv("c1"))))
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 1, 0, true)
	// ... two senders on one channel conflict ...
	pl = planOf(t, model(nil, []ir.Channel{c1}, nil, proc("S1", nil, 2, send("c1")), proc("S2", nil, 2, send("c1"))))
	wantEligible(t, pl, 0, 0, false)
	// ... processes on different channels do not.
	pl = planOf(t, model(nil, []ir.Channel{c1, c2}, nil, proc("S1", nil, 2, send("c1")), proc("S2", nil, 2, send("c2"))))
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 1, 0, true)
}

func TestPORChannelLengthInAPropertyMakesChannelEdgesVisible(t *testing.T) {
	c1 := ir.Channel{Name: "c1", Capacity: 2, Fields: []ir.Type{ir.Byte}}
	m := model(nil, []ir.Channel{c1},
		[]ir.Property{{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("le", ir.Len("c1"), ir.Const(1))}},
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: "c1", Args: []*ir.Expr{ir.Const(1)}}}),
		proc("Q", []ir.Var{byteVar("c")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("c", 1)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, true)
}

func TestPORProgramCounterReadIsVisible(t *testing.T) {
	m := model(nil, nil,
		[]ir.Property{{ID: "r", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.PC(0), ir.Const(1))}},
		proc("P", nil, 2, ir.Edge{From: 0, To: 1}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false) // moving P changes pc(0), which the property reads
	wantEligible(t, pl, 1, 0, true)
}

func TestPORProgramCounterReadByAGuardConflictsWithItsProcess(t *testing.T) {
	m := model(nil, nil, nil,
		proc("P", nil, 2, ir.Edge{From: 0, To: 1}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.PC(0), ir.Const(0))}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false) // every edge of P writes pc(0), which Q's guard reads
	wantEligible(t, pl, 1, 0, false)
}

func TestPORDStepClosureCountsTowardsTheFirstEdge(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("P", []ir.Var{byteVar("c")}, 3,
			ir.Edge{From: 0, To: 1, DStep: true, Effect: []ir.Assign{set("c", 1)}},
			ir.Edge{From: 1, To: 2, Effect: []ir.Assign{set("x", 1)}}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 2)}}))
	pl := planOf(t, m)
	// The first edge is local, but the d_step goes on into a write of x in
	// the same step.
	wantEligible(t, pl, 0, 0, false)
}

func TestPORElseDependsOnTheGuardsOfItsSiblings(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("P", nil, 3,
			ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(1))},
			ir.Edge{From: 0, To: 2, Else: true}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
}

// ---- what is refused, and why --------------------------------------------------

func TestPORRefusals(t *testing.T) {
	// q builds a fresh process each time: the cases below edit their copy.
	q := func() ir.Process { return proc("Q", nil, 2, ir.Edge{From: 0, To: 1}) }
	plain := func() *ir.Model {
		return model([]ir.Var{byteVar("x")}, nil, nil, proc("P", nil, 2, ir.Edge{From: 0, To: 1}), q())
	}

	atomic := plain()
	atomic.Processes[1].Edges[0].Atomic = true

	timeout := plain()
	timeout.Processes[1].Edges[0].Guard = ir.Timeout()

	provided := plain()
	provided.Processes[1].Provided = ir.Binary("eq", ir.Ref("x"), ir.Const(0))

	rendezvous := model(nil, []ir.Channel{{Name: "r", Capacity: 0, Fields: []ir.Type{ir.Byte}}}, nil,
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: "r", Args: []*ir.Expr{ir.Const(1)}}}),
		proc("R", nil, 2, ir.Edge{From: 0, To: 1, Recv: &ir.RecvOp{Chan: "r", Args: []ir.RecvArg{{}}}}))

	dynChan := model(nil, []ir.Channel{{Name: "c", Capacity: 1, Fields: []ir.Type{ir.Byte}}}, nil,
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Sel: ir.Const(1), Args: []*ir.Expr{ir.Const(1)}}}),
		q())

	dynamic := plain()
	dynamic.Processes[1].Dynamic = true

	needsTable := model(nil, nil,
		[]ir.Property{{ID: "r", Kind: ir.KindReach, Expr: ir.Binary("gt", ir.NrPr(), ir.Const(1))}},
		proc("P", nil, 2, ir.Edge{From: 0, To: 1}), q())

	ltl := plain()
	ltl.Properties = append(ltl.Properties, ir.Property{ID: "l", Kind: ir.KindLTL, Formula: "[]true"})

	ctl := plain()
	ctl.Properties = append(ctl.Properties, ir.Property{ID: "c", Kind: ir.KindCTL, Formula: "AG true"})

	for _, c := range []struct {
		name string
		m    *ir.Model
		want string
	}{
		{"atomic", atomic, "atomic"},
		{"timeout", timeout, "timeout"},
		{"provided", provided, "provided"},
		{"rendezvous", rendezvous, "rendezvous"},
		{"dynamic channel", dynChan, "dynamic channel"},
		{"dynamic process", dynamic, "process creation"},
		{"process table", needsTable, "process table"},
		{"ltl", ltl, "temporal"},
		{"ctl", ctl, "temporal"},
		{"fine", plain(), ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			wantReason(t, planOf(t, c.m), c.want)
		})
	}
}

// ---- program-counter guards: SPIN's termination order ---------------------------

// A Promela process may only terminate when every younger process is dead:
// its `-end-` edge is guarded by pc(j) == dead for each younger j. The dead
// location has no edge, so that guard and any edge of j are never enabled at
// once, and the guard does not make j's steps dependent on it.
func TestPORTerminationOrderGuardDoesNotBlockTheYoungerProcess(t *testing.T) {
	m := model(nil, nil, nil,
		proc("Old", nil, 2, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.PC(1), ir.Const(2))}),
		proc("Young", []ir.Var{byteVar("c")}, 3,
			ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("c", 1)}},
			ir.Edge{From: 1, To: 2, Effect: []ir.Assign{set("c", 2)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 1, 0, true) // Old waits for pc(1) == 2, a location Young's edges never leave
	wantEligible(t, pl, 1, 1, true)
	// Old's own edge is expanded alone only if nobody can enable or disable
	// it, and Young's steps do: its guard reads the whole counter of Young.
	// It is enabled only once Young is dead, so nothing is lost.
	wantEligible(t, pl, 0, 0, false)
}

func TestPORGuardOnALocationTheOtherProcessLeavesStillConflicts(t *testing.T) {
	// Old's guard holds while Young is at 0, and Young's edge out of 0 turns it off.
	m := model(nil, nil, nil,
		proc("Old", nil, 2, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.PC(1), ir.Const(0))}),
		proc("Young", nil, 2, ir.Edge{From: 0, To: 1}))
	pl := planOf(t, m)
	wantEligible(t, pl, 1, 0, false)
	wantEligible(t, pl, 0, 0, false)
}

func TestPORGuardAsOneConjunctOfSeveral(t *testing.T) {
	guard := ir.And(ir.Binary("eq", ir.PC(1), ir.Const(2)), ir.Binary("eq", ir.PC(2), ir.Const(2)))
	m := model(nil, nil, nil,
		proc("A", nil, 2, ir.Edge{From: 0, To: 1, Guard: guard}),
		proc("B", nil, 3, ir.Edge{From: 0, To: 1}, ir.Edge{From: 1, To: 2}),
		proc("C", nil, 3, ir.Edge{From: 0, To: 1}, ir.Edge{From: 1, To: 2}))
	pl := planOf(t, m)
	for p := 1; p <= 2; p++ {
		wantEligible(t, pl, p, 0, true)
		wantEligible(t, pl, p, 1, true)
	}
}

func TestPORProgramCounterGuardInsideADisjunctIsNotRefined(t *testing.T) {
	// Not a top-level conjunct: the guard may hold at any location of Young.
	guard := ir.Binary("or", ir.Binary("eq", ir.PC(1), ir.Const(2)), ir.Binary("eq", ir.PC(1), ir.Const(0)))
	m := model(nil, nil, nil,
		proc("Old", nil, 2, ir.Edge{From: 0, To: 1, Guard: guard}),
		proc("Young", nil, 3, ir.Edge{From: 0, To: 1}, ir.Edge{From: 1, To: 2}))
	pl := planOf(t, m)
	wantEligible(t, pl, 1, 0, false)
	wantEligible(t, pl, 1, 1, false)
}

func TestPORElseSeesTheProgramCounterGuardsOfItsSiblingsWhole(t *testing.T) {
	// Old: (pc(1) == 1) -> ... ; else -> ... Young enters location 1 from 0.
	// With Young at 0 the else edge is enabled, and Young's step disables it
	// by making the guard true: the two are dependent even though the guard
	// "is about location 1" and Young's edge leaves location 0.
	m := model(nil, nil, nil,
		proc("Old", nil, 3,
			ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.PC(1), ir.Const(1))},
			ir.Edge{From: 0, To: 2, Else: true}),
		proc("Young", nil, 2, ir.Edge{From: 0, To: 1}))
	pl := planOf(t, m)
	wantEligible(t, pl, 1, 0, false)
	// Without the else edge the same guard is not enabled at Young's location 0.
	m = model(nil, nil, nil,
		proc("Old", nil, 3, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.PC(1), ir.Const(1))}),
		proc("Young", nil, 2, ir.Edge{From: 0, To: 1}))
	wantEligible(t, planOf(t, m), 1, 0, true)
}

func TestPORPropertyReadingAProgramCounterMakesEveryEdgeOfThatProcessVisible(t *testing.T) {
	// reach pc(1) == 1 reads the whole counter of process 1, so every edge of
	// it is visible, entering, leaving or neither.
	m := model(nil, nil,
		[]ir.Property{{ID: "r", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.PC(1), ir.Const(1))}},
		proc("Other", nil, 2, ir.Edge{From: 0, To: 1}),
		proc("Watched", nil, 4,
			ir.Edge{From: 0, To: 1}, // enters 1
			ir.Edge{From: 1, To: 2}, // leaves 1
			ir.Edge{From: 2, To: 3}))
	pl := planOf(t, m)
	wantEligible(t, pl, 1, 0, false)
	wantEligible(t, pl, 1, 1, false)
	wantEligible(t, pl, 1, 2, false)
	wantEligible(t, pl, 0, 0, true)
}

// ---- the refinement is for the other processes' edges, not for the alternatives ----

func TestPORAnEdgeAnotherProcessCanEnableBlocksItsSiblingsFromBeingExpandedAlone(t *testing.T) {
	// P at 0: a free edge f and an edge e guarded by pc(Q) == 1. Q's step into
	// 1 enables e, and e is an alternative of f (the same process), so f
	// cannot be expanded alone. The guard reads "Q at 1" and Q's step leaves
	// 0: read as the other processes see it, there would be no conflict.
	pl := planOf(t, porEnabling())
	wantEligible(t, pl, 0, 0, false)
	// Q's own step is still fine: P's guard is enabled only while Q is at 1,
	// where Q has no edge.
	wantEligible(t, pl, 1, 0, true)
}

func TestPORAnEdgeAStepIntoWhichDependsOnAProgramCounterIsNotExpandedAlone(t *testing.T) {
	// The d_step goes on into the first enabled of two edges, one guarded by
	// pc(Q) == 1: whether Q has entered 1 decides the result of the step.
	wantEligible(t, planOf(t, porDStepGuard()), 0, 0, false)

	// The same with a single continuation, which blocks (an error) when its
	// guard is false: the step still depends on Q.
	m := model(nil, nil, nil,
		proc("P", nil, 3,
			ir.Edge{From: 0, To: 1, DStep: true},
			ir.Edge{From: 1, To: 2, Guard: ir.Binary("eq", ir.PC(1), ir.Const(1))}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1}))
	wantEligible(t, planOf(t, m), 0, 0, false)
}

func TestPORTheOtherProcessesSeeAnElseSiblingWholeAndAnExactGuardOtherwise(t *testing.T) {
	// Q has two alternatives at 0, one guarded by pc(P) == 1 (no else): from
	// P's point of view it is "Q's edge, enabled while P is at 1", and P's
	// step out of 0 is independent of it. P is still eligible at 0.
	m := model(nil, nil, nil,
		proc("P", nil, 3, ir.Edge{From: 0, To: 1}, ir.Edge{From: 1, To: 2}),
		proc("Q", nil, 3,
			ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.PC(0), ir.Const(1))},
			ir.Edge{From: 0, To: 2}))
	wantEligible(t, planOf(t, m), 0, 0, true)
}

// ---- predicates a second review found nothing pinned ---------------------------------

func TestPORVisibilityCoversEveryPropertyNotOnlyTheFirst(t *testing.T) {
	m := model([]ir.Var{byteVar("x"), byteVar("y"), byteVar("z")}, nil,
		[]ir.Property{
			{ID: "i1", Kind: ir.KindInvariant, Expr: ir.Binary("ne", ir.Ref("x"), ir.Const(9))},
			{ID: "i2", Kind: ir.KindInvariant, Expr: ir.Binary("ne", ir.Ref("y"), ir.Const(9))},
			{ID: "r1", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.Ref("z"), ir.Const(9))}},
		proc("WX", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		proc("WY", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("y", 1)}}),
		proc("WZ", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("z", 1)}}),
		proc("W", []ir.Var{byteVar("c")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("c", 1)}}))
	pl := planOf(t, m)
	for p := 0; p < 3; p++ {
		wantEligible(t, pl, p, 0, false) // each writes what one of the three properties reads
	}
	wantEligible(t, pl, 3, 0, true)
}

func TestPORAnInequalityOnAProgramCounterIsReadWhole(t *testing.T) {
	// Old waits for pc(Young) < 3: true while Young is at 0, 1 or 2, and Young's
	// step 2 -> 3 turns it off. Only an equality can be read as "Young at c".
	m := model(nil, nil, nil,
		proc("Old", nil, 2, ir.Edge{From: 0, To: 1, Guard: ir.Binary("lt", ir.PC(1), ir.Const(3))}),
		proc("Young", nil, 5,
			ir.Edge{From: 0, To: 1}, ir.Edge{From: 1, To: 2}, ir.Edge{From: 2, To: 3}, ir.Edge{From: 3, To: 4}))
	pl := planOf(t, m)
	wantEligible(t, pl, 1, 2, false) // the step that turns the guard off
	wantEligible(t, pl, 1, 0, false)
	// The mirror: pc(Young) >= 2 is turned on by 1 -> 2 and stays on.
	m.Processes[0].Edges[0].Guard = ir.Binary("ge", ir.PC(1), ir.Const(2))
	wantEligible(t, planOf(t, m), 1, 3, false)
}

func TestPORLocationsWithoutEdgesAreNeverEligible(t *testing.T) {
	// Both processes write x, so nothing can be expanded alone. The dead ends
	// (locations with no edge) must not count as eligible either: they have
	// nothing to expand, and counting them makes `any` true for every model.
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 2)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 1, false)
	wantEligible(t, pl, 1, 1, false)
	if pl.any {
		t.Fatal("plan.any is true for a model in which nothing can be expanded alone")
	}
}

func TestPORAChannelNamedByValueInAPropertyIsAnyChannel(t *testing.T) {
	c := ir.Channel{Name: "c", Capacity: 2, Fields: []ir.Type{ir.Byte}}
	m := model(nil, []ir.Channel{c},
		[]ir.Property{{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("le", ir.CLen(ir.Const(1)), ir.Const(1))}},
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(1)}}}),
		proc("Q", []ir.Var{byteVar("l")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("l", 1)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false) // clen(1) reads whichever channel has id 1: any send is visible
	wantEligible(t, pl, 1, 0, true)
}

// The ample process is held in a uint8 as index + 1. A model may have at most
// 254 processes (the exclusive-control byte names a process by index + 1), so
// the index always fits; this pins both ends of that.
func TestPORAmpleIndexFitsTheProcessLimit(t *testing.T) {
	red := runPOR(t, independentCounters(254, 1), true, DFS)
	if !red.Reduction.Applied || red.States != 254*2+1 || !red.Complete {
		t.Fatalf("254 processes: applied=%v, %d states, complete=%v", red.Reduction.Applied, red.States, red.Complete)
	}
	if _, err := compile(independentCounters(255, 1)); err == nil {
		t.Fatal("a model of 255 processes compiles; the uint8 index of the ample process could wrap")
	}
}
