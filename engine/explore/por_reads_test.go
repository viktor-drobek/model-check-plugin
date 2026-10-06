package explore

// What each kind of read and write must count as. Hand-built models, found by
// the second cross-review: a mutant of por.go that forgets one of them is
// answered `verified` by every other test, and by 15000 random models. In every
// model below the bad run (a deadlock) exists only when Q writes y BEFORE P
// reads it, so expanding P alone first, which is what an analysis that forgot
// the read would do, neverTrue reaches it.

import (
	"context"
	"testing"

	"modelcheck/ir"
)

func readsRun(t *testing.T, m *ir.Model) (full, red *Result) {
	t.Helper()
	var err error
	full, err = Run(context.Background(), m, Options{Sweep: true})
	if err != nil {
		t.Fatal(err)
	}
	red, err = Run(context.Background(), m, Options{Sweep: true, POR: true})
	if err != nil {
		t.Fatal(err)
	}
	return
}

func readsCheck(t *testing.T, name string, m *ir.Model) {
	t.Helper()
	full, red := readsRun(t, m)
	if !red.Reduction.Applied {
		t.Fatalf("%s: not applied: %s", name, red.Reduction.Reason)
	}
	for i := range full.Outcomes {
		if full.Outcomes[i].Status != red.Outcomes[i].Status {
			t.Errorf("%s: %s is %s with the reduction, %s without", name, full.Outcomes[i].Property.ID, red.Outcomes[i].Status, full.Outcomes[i].Status)
		}
	}
	if full.Outcomes[0].Status != Violated {
		t.Errorf("%s: the model is supposed to have a deadlock, full search says %s", name, full.Outcomes[0].Status)
	}
}

var neverTrue = ir.Const(0)

func qWritesY() ir.Process {
	return proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("y", 1)}})
}

// A d_step two levels deep: the second continuation reads y.
func TestPORClosureFollowsAChainOfDStepSteps(t *testing.T) {
	m := model([]ir.Var{byteVar("y")}, nil, nil,
		proc("P", nil, 5,
			ir.Edge{From: 0, To: 1, DStep: true},
			ir.Edge{From: 1, To: 2, DStep: true},
			ir.Edge{From: 2, To: 3, Guard: ir.Binary("eq", ir.Ref("y"), ir.Const(1))}, // taken only if Q went first: stuck at 3
			ir.Edge{From: 2, To: 4},
			ir.Edge{From: 3, To: 4, Guard: neverTrue}),
		qWritesY())
	readsCheck(t, "d_step two hops", m)
}

// A receive that matches on y.
func TestPORRecvMatchIsARead(t *testing.T) {
	m := model([]ir.Var{byteVar("y")}, []ir.Channel{{Name: "c", Capacity: 1, Fields: []ir.Type{ir.Byte}}}, nil,
		proc("P", nil, 5,
			ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(1)}}},
			ir.Edge{From: 1, To: 2, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Match: ir.Ref("y")}}}}, // enabled only after y = 1
			ir.Edge{From: 1, To: 3},
			ir.Edge{From: 2, To: 4, Guard: neverTrue}),
		qWritesY())
	// from 2 the process is stuck at an edge that neverTrue fires; from 3 it has none
	readsCheck(t, "recv match", m)
}

// A send whose argument is y.
func TestPORSendArgumentIsARead(t *testing.T) {
	m := model([]ir.Var{byteVar("y")}, []ir.Channel{{Name: "c", Capacity: 1, Fields: []ir.Type{ir.Byte}}}, nil,
		proc("P", nil, 5,
			ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Ref("y")}}},
			ir.Edge{From: 1, To: 2, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Match: ir.Const(1)}}}},
			ir.Edge{From: 1, To: 3, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Match: ir.Const(0)}}}},
			ir.Edge{From: 2, To: 4, Guard: neverTrue}),
		qWritesY())
	readsCheck(t, "send arg", m)
}

// An assignment a[y] = 1: the index reads y.
func TestPOREffectIndexIsARead(t *testing.T) {
	m := model([]ir.Var{byteVar("y"), {Name: "a", Type: ir.Byte, Len: 2}}, nil, nil,
		proc("P", nil, 5,
			ir.Edge{From: 0, To: 1, Effect: []ir.Assign{{Var: "a", Index: ir.Ref("y"), Value: ir.Const(1)}}},
			ir.Edge{From: 1, To: 2, Guard: ir.Binary("eq", ir.Index("a", ir.Const(1)), ir.Const(1))},
			ir.Edge{From: 1, To: 3, Guard: ir.Binary("eq", ir.Index("a", ir.Const(0)), ir.Const(1))},
			ir.Edge{From: 2, To: 4, Guard: neverTrue}),
		qWritesY())
	readsCheck(t, "effect index", m)
}

// A receive into a[y]: the bind index reads y.
func TestPORRecvBindIndexIsARead(t *testing.T) {
	m := model([]ir.Var{byteVar("y"), {Name: "a", Type: ir.Byte, Len: 2}}, []ir.Channel{{Name: "c", Capacity: 1, Fields: []ir.Type{ir.Byte}}}, nil,
		proc("P", nil, 6,
			ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(1)}}},
			ir.Edge{From: 1, To: 2, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Var: "a", Index: ir.Ref("y")}}}},
			ir.Edge{From: 2, To: 3, Guard: ir.Binary("eq", ir.Index("a", ir.Const(1)), ir.Const(1))},
			ir.Edge{From: 2, To: 4, Guard: ir.Binary("eq", ir.Index("a", ir.Const(0)), ir.Const(1))},
			ir.Edge{From: 3, To: 5, Guard: neverTrue}),
		qWritesY())
	readsCheck(t, "recv bind index", m)
}

// An assert that reads y: P's assert(y == 1) fails when it runs before Q's
// write, so Q's write must not be taken first alone.
func TestPORAssertOperandIsARead(t *testing.T) {
	m := model([]ir.Var{byteVar("y")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Assert: ir.Binary("eq", ir.Ref("y"), ir.Const(1))}),
		qWritesY())
	full, red := readsRun(t, m)
	if !red.Reduction.Applied {
		t.Fatalf("not applied: %s", red.Reduction.Reason)
	}
	for i := range full.Outcomes {
		if full.Outcomes[i].Status != red.Outcomes[i].Status {
			t.Errorf("%s is %s with the reduction, %s without", full.Outcomes[i].Property.ID, red.Outcomes[i].Status, full.Outcomes[i].Status)
		}
	}
	if full.Outcomes[len(full.Outcomes)-1].Status != Violated {
		t.Errorf("the assert must be violated without the reduction")
	}
}

// A receive that binds the global y: Q reads y, so P's receive must not be
// taken alone first (Q's e needs y == 0, which holds only before the receive).
func TestPORRecvBindIsAWrite(t *testing.T) {
	m := model([]ir.Var{byteVar("y")}, []ir.Channel{{Name: "c", Capacity: 1, Fields: []ir.Type{ir.Byte}}}, nil,
		proc("P", nil, 3,
			ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(1)}}},
			ir.Edge{From: 1, To: 2, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Var: "y"}}}}),
		proc("Q", nil, 4,
			ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("y"), ir.Const(0))},
			ir.Edge{From: 0, To: 2, Guard: ir.Binary("eq", ir.Ref("y"), ir.Const(1))},
			ir.Edge{From: 1, To: 3, Guard: neverTrue}))
	readsCheck(t, "recv bind write", m)
}

// A d_step whose continuation is an else edge that writes x: Q reads x, and
// Q's e (x == 0) leads to a deadlock that exists only if Q goes before the step.
func TestPORDStepElseContinuationWrites(t *testing.T) {
	m := model([]ir.Var{byteVar("x"), byteVar("y")}, nil, nil,
		proc("P", nil, 4,
			ir.Edge{From: 0, To: 1, DStep: true},
			ir.Edge{From: 1, To: 2, Guard: ir.Binary("eq", ir.Ref("y"), ir.Const(5))},
			ir.Edge{From: 1, To: 3, Else: true, Effect: []ir.Assign{set("x", 1)}}),
		proc("Q", nil, 4,
			ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(0))},
			ir.Edge{From: 0, To: 2, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(1))},
			ir.Edge{From: 1, To: 3, Guard: neverTrue}))
	readsCheck(t, "d_step else continuation", m)
}

// A guard on the length of a channel named by a value (clen): it reads every
// channel, so Q's send on c must not be postponed behind P's free edge.
func TestPORChannelLengthByValueReadsEveryChannel(t *testing.T) {
	m := model(nil, []ir.Channel{{Name: "c", Capacity: 1, Fields: []ir.Type{ir.Byte}}}, nil,
		proc("P", nil, 4,
			ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.CLen(ir.Const(1)), ir.Const(1))}, // channel id 1 is c
			ir.Edge{From: 0, To: 2},
			ir.Edge{From: 1, To: 3, Guard: neverTrue}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(1)}}}))
	readsCheck(t, "clen by value", m)
}
