package explore

import (
	"fmt"
	"testing"

	"modelcheck/ir"
)

func incr(v string) []ir.Assign {
	return []ir.Assign{{Var: v, Value: ir.Binary("add", ir.Ref(v), ir.Const(1))}}
}

// pipeline is a source, a filter and a sink over two buffered channels, each
// with exactly one sender and one receiver: k messages numbered 0..k-1, and
// the sink asserts that they come out in order.
func pipeline(k, capC, capD int) *ir.Model {
	kk := ir.Const(int64(k))
	lt := func(v string) *ir.Expr { return ir.Binary("lt", ir.Ref(v), kk) }
	ge := func(v string) *ir.Expr { return ir.Binary("ge", ir.Ref(v), kk) }
	src := proc("Source", []ir.Var{byteVar("n")}, 3,
		ir.Edge{From: 0, To: 1, Guard: lt("n"), Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Ref("n")}}, Text: "c!n"},
		ir.Edge{From: 1, To: 0, Effect: incr("n"), Text: "n++"},
		ir.Edge{From: 0, To: 2, Guard: ge("n"), Text: "done"})
	flt := proc("Filter", []ir.Var{byteVar("k"), byteVar("m")}, 4,
		ir.Edge{From: 0, To: 1, Guard: lt("k"), Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Var: "m"}}}, Text: "c?m"},
		ir.Edge{From: 1, To: 2, Send: &ir.ChanOp{Chan: "d", Args: []*ir.Expr{ir.Ref("m")}}, Text: "d!m"},
		ir.Edge{From: 2, To: 0, Effect: incr("k"), Text: "k++"},
		ir.Edge{From: 0, To: 3, Guard: ge("k"), Text: "done"})
	snk := proc("Sink", []ir.Var{byteVar("e"), byteVar("v")}, 5,
		ir.Edge{From: 0, To: 1, Guard: lt("e"), Recv: &ir.RecvOp{Chan: "d", Args: []ir.RecvArg{{Var: "v"}}}, Text: "d?v"},
		ir.Edge{From: 1, To: 2, Assert: ir.Binary("eq", ir.Ref("v"), ir.Ref("e")), Text: "assert(v == e)"},
		ir.Edge{From: 2, To: 0, Effect: incr("e"), Text: "e++"},
		ir.Edge{From: 0, To: 3, Guard: ge("e"), Text: "done"})
	return model(nil, []ir.Channel{chanDecl("c", capC), chanDecl("d", capD)},
		[]ir.Property{{ID: "assert", Kind: ir.KindAssert}}, src, flt, snk)
}

func TestPORAPipelineReducesAndKeepsTheVerdicts(t *testing.T) {
	m := pipeline(3, 2, 2)
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if !red.Reduction.Applied {
		t.Fatalf("not applied: %s", red.Reduction.Reason)
	}
	sameStatuses(t, "pipeline", red, full)
	if s := outcome(t, full, "deadlock").Status; s != Verified {
		t.Fatalf("the pipeline deadlocks: %s", s)
	}
	if s := outcome(t, red, "assert").Status; s != Verified {
		t.Fatalf("assert %s, the channels are first in, first out", s)
	}
	if red.States*2 > full.States {
		t.Fatalf("reduced %d states against %d: a pipeline of single-sender channels should shrink by more than half", red.States, full.States)
	}
	t.Logf("pipeline(3,2,2): %d states full, %d reduced", full.States, red.States)
}

// A sender at a full channel has a disabled send among its alternatives. The
// receiver's pop would enable it, and it is an alternative of the free edge
// that is enabled now, so the sender must not be expanded alone then: the
// deadlock is behind the send that only the pop enables.
func fullChannelTrap() *ir.Model {
	return model([]ir.Var{byteVar("x")}, []ir.Channel{chanDecl("c", 1)}, nil,
		proc("S", nil, 5,
			sendEdge(0, 1, "c", 1),
			sendEdge(1, 2, "c", 2),  // disabled while c is full; the pop enables it
			ir.Edge{From: 1, To: 3}, // free
			ir.Edge{From: 2, To: 4, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(5))}), // never: S is stuck at 2
		proc("R", nil, 2, recvEdge(0, 1, "c")))
}

// The mirror: a receiver at an empty channel has a disabled receive among its
// alternatives; the sender's push would enable it.
func emptyChannelTrap() *ir.Model {
	return model([]ir.Var{byteVar("x")}, []ir.Channel{chanDecl("c", 1)}, nil,
		proc("S", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("R", nil, 5,
			recvEdge(0, 1, "c"),     // disabled while c is empty; the push enables it
			ir.Edge{From: 0, To: 3}, // free
			ir.Edge{From: 1, To: 2, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(5))})) // never: R is stuck at 1
}

// swapped is m with its first two processes exchanged. pick takes the first
// eligible process by index, so a requirement that only one of the two ends
// is subject to is masked when that end happens to go first: both orders must
// be checked, or half of the rule is untested.
func swapped(m *ir.Model) *ir.Model {
	c := *m
	c.Processes = append([]ir.Process(nil), m.Processes...)
	c.Processes[0], c.Processes[1] = c.Processes[1], c.Processes[0]
	return &c
}

func TestPORAChannelOpEnabledByTheOtherEndKeepsTheDeadlock(t *testing.T) {
	for name, m := range map[string]*ir.Model{
		"full channel":               fullChannelTrap(),
		"empty channel":              emptyChannelTrap(),
		"full channel, other order":  swapped(fullChannelTrap()),
		"empty channel, other order": swapped(emptyChannelTrap()),
	} {
		t.Run(name, func(t *testing.T) {
			full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
			if s := outcome(t, full, "deadlock").Status; s != Violated {
				t.Fatalf("full search: deadlock %s, want violated", s)
			}
			if !red.Reduction.Applied {
				t.Fatalf("not applied: %s", red.Reduction.Reason)
			}
			sameStatuses(t, name, red, full)
		})
	}
}

// With nothing to enable, the same shapes are reduced: the requirement is
// about the state, not a ban on the pattern.
func TestPORATheRequirementIsAboutTheStateNotThePattern(t *testing.T) {
	// A capacity of 2 leaves room after the first send, so S at location 1 is
	// expanded alone whenever it is S's turn and nothing is full.
	m := fullChannelTrap()
	m.Channels[0].Capacity = 2
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	sameStatuses(t, "room", red, full)
	if red.Reduction.ReducedStates == 0 {
		t.Fatalf("nothing was reduced although the channel never fills: %+v", red.Reduction)
	}
}

func TestPORATwoSendersKeepTheOrderThatMatters(t *testing.T) {
	// R reads two messages and asserts the first was 1; S2's 2 can arrive first.
	m := model([]ir.Var{byteVar("a")}, []ir.Channel{chanDecl("c", 2)},
		[]ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("S1", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("S2", nil, 2, sendEdge(0, 1, "c", 2)),
		proc("R", nil, 4, recvInto(0, 1, "c", "a"),
			ir.Edge{From: 1, To: 2, Assert: ir.Binary("eq", ir.Ref("a"), ir.Const(1))},
			recvEdge(2, 3, "c")))
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if s := outcome(t, full, "assert").Status; s != Violated {
		t.Fatalf("full search: assert %s, want violated", s)
	}
	sameStatuses(t, "two senders", red, full)
}

func TestPORAChannelLengthReadKeepsWhatItObserves(t *testing.T) {
	// W waits for the length of c to be exactly 1 and then sets x; R then
	// waits for x == 1 and has nothing else to do. Whether W ever sees length 1
	// depends on how the sender and the receiver interleave.
	m := model([]ir.Var{byteVar("x")}, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("S", nil, 3, sendEdge(0, 1, "c", 1), sendEdge(1, 2, "c", 2)),
		proc("R", nil, 3, recvEdge(0, 1, "c"), recvEdge(1, 2, "c")),
		proc("W", nil, 3, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.Len("c"), ir.Const(1)), Effect: []ir.Assign{set("x", 1)}},
			ir.Edge{From: 1, To: 2, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(7))}))
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	sameStatuses(t, "len", red, full)
	if s := outcome(t, full, "deadlock").Status; s != Violated {
		t.Fatalf("full search: deadlock %s, want violated (W waits for x == 7)", s)
	}
}

// rotated is m with its first process moved to the end. pick takes the first
// eligible process by index, so a check that lets one process hide a missing
// check on another needs the processes in more than one order.
func rotated(m *ir.Model) *ir.Model {
	c := *m
	c.Processes = append(append([]ir.Process(nil), m.Processes[1:]...), m.Processes[0])
	return &c
}

// P sends on c, then clears it; Q receives from c and then asserts false. The
// assert is reached only if Q's receive comes before P's clear. A clear uses
// the whole channel, so it overlaps the receive end too: if it overlapped the
// send end only, P's clear would be expanded alone before Q's receive and the
// assert would be lost. (Found by the third cross-review: the mutant that
// writes the clear as the send end passed every test and the random oracle.)
func clearBeforeReceive() *ir.Model {
	return model(nil, []ir.Channel{chanDecl("c", 2)}, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("P", nil, 3, sendEdge(0, 1, "c", 1), ir.Edge{From: 1, To: 2, ClearChans: []int{0}}),
		proc("Q", []ir.Var{byteVar("x")}, 3, recvInto(0, 1, "c", "x"), ir.Edge{From: 1, To: 2, Assert: ir.Const(0)}))
}

func TestPORAClearOverlapsTheReceiveEndAndKeepsTheAssert(t *testing.T) {
	for name, m := range map[string]*ir.Model{"clear first": clearBeforeReceive(), "other order": rotated(clearBeforeReceive())} {
		t.Run(name, func(t *testing.T) {
			full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
			if s := outcome(t, full, "assert").Status; s != Violated {
				t.Fatalf("full search: assert %s, want violated", s)
			}
			sameStatuses(t, name, red, full)
		})
	}
}

// Two channels. P's alternatives mix a send that is blocked (d is full) with a
// free send on the other channel c. A requirement that reads the wrong channel
// (the first one, say) finds room on c and expands P alone, and the branch
// that only the pop of d enables is lost: in it P never sends its second
// message on c, and S, which waits for it, is left blocked forever, the only
// deadlock of the model.
//
// With deep set, P first sends on c (location 0: the requirement there is
// "room on c", which holds later too) and only then fills d, and R may pop d
// only once P has (flag f, set by the very edge that fills d): every way to the
// deadlock goes through the state in which P is at the alternatives and d is
// full, and the requirement of the first location must not be read there. P is
// the only sender on d, so nothing makes P ineligible there except the
// requirement itself. (A flag and not a guard on P's program counter: after P
// has left, R must still be able to pop, or the other branch would deadlock
// too and tell nothing.) Without deep P fills d at once and the pop can come at
// any time.
//
// rot moves the first process to the end that many times, because pick takes
// the first eligible process by index and the order can hide a missing check.
func twoChannelTrap(deep bool, rot int) *ir.Model {
	var p, r, s ir.Process
	var globals []ir.Var
	if !deep {
		p = proc("P", nil, 4, sendEdge(0, 1, "d", 5), sendEdge(1, 2, "d", 1), sendEdge(1, 3, "c", 1)) // the blocked one first
		r = proc("R", nil, 2, recvEdge(0, 1, "d"))
		s = proc("S", nil, 2, recvEdge(0, 1, "c"))
	} else {
		globals = []ir.Var{byteVar("f")}
		fill := sendEdge(1, 2, "d", 5)
		fill.Effect = []ir.Assign{set("f", 1)}
		p = proc("P", nil, 5, sendEdge(0, 1, "c", 7), fill, sendEdge(2, 3, "d", 1), sendEdge(2, 4, "c", 1))
		pop := recvEdge(0, 1, "d")
		pop.Guard = ir.Binary("eq", ir.Ref("f"), ir.Const(1))
		r = proc("R", nil, 2, pop)
		s = proc("S", nil, 3, recvEdge(0, 1, "c"), recvEdge(1, 2, "c")) // two messages are expected
	}
	ps := []ir.Process{p, proc("Q", nil, 1), r, s}
	ps = append(append([]ir.Process(nil), ps[rot%4:]...), ps[:rot%4]...)
	return model(globals, []ir.Channel{chanDecl("c", 2), chanDecl("d", 1)}, nil, ps...)
}

func TestPORAThePerChannelRequirementNamesTheRightChannelAndLocation(t *testing.T) {
	for _, deep := range []bool{false, true} {
		for rot := 0; rot < 4; rot++ {
			m := twoChannelTrap(deep, rot)
			name := fmt.Sprintf("deep=%v rot=%d", deep, rot)
			t.Run(name, func(t *testing.T) {
				full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
				if s := outcome(t, full, "deadlock").Status; s != Violated {
					t.Fatalf("full search: deadlock %s, want violated", s)
				}
				if !red.Reduction.Applied {
					t.Fatalf("not applied: %s", red.Reduction.Reason)
				}
				sameStatuses(t, name, red, full)
			})
		}
	}
}

// A receive that matches 5 is blocked while the head is 3. A sender cannot
// change the head (it appends at the tail), but another receiver's pop can:
// Q pops the 3, P's receive of 5 is enabled, and P, which then never sends on
// c2, leaves its partner blocked: a deadlock behind a receive that only a pop
// by another receiver enables. The two receives share the receive end, so
// neither is expanded alone.
func TestPORAReceiveEnabledByAnotherReceiversPopKeepsTheDeadlock(t *testing.T) {
	m := model(nil, []ir.Channel{chanDecl("c", 2), chanDecl("e", 1)}, nil,
		proc("P", nil, 3,
			ir.Edge{From: 0, To: 1, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Match: ir.Const(5)}}}},
			sendEdge(0, 2, "e", 1)), // the free alternative: tell the partner
		proc("S", nil, 3, sendEdge(0, 1, "c", 3), sendEdge(1, 2, "c", 5)),
		proc("Q", nil, 2, recvEdge(0, 1, "c")),
		proc("Partner", nil, 2, recvEdge(0, 1, "e")))
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if s := outcome(t, full, "deadlock").Status; s != Violated {
		t.Fatalf("full search: deadlock %s, want violated", s)
	}
	sameStatuses(t, "head mismatch", red, full)
}

// An eligible process with no enabled move is passed over, not a reason to
// expand the state in full: P0's only edge waits for a global nobody writes,
// and the two counters behind it are independent.
func TestPORAnEligibleProcessWithoutMovesIsPassedOver(t *testing.T) {
	counters := independentCounters(2, 3)
	waiting := proc("Waiting", nil, 2, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(5))})
	m := model([]ir.Var{byteVar("x")}, nil, nil, append([]ir.Process{waiting}, counters.Processes...)...)
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	sameStatuses(t, "waiting", red, full)
	if red.States*2 > full.States {
		t.Fatalf("reduced %d states against %d: the process without moves must not stop the others being expanded alone", red.States, full.States)
	}
}
