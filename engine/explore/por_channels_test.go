package explore

import (
	"testing"

	"modelcheck/ir"
)

// ---- builders ---------------------------------------------------------------------

func chanDecl(name string, capacity int) ir.Channel {
	return ir.Channel{Name: name, Capacity: capacity, Fields: []ir.Type{ir.Byte}}
}

func sendEdge(from, to int, ch string, v int64) ir.Edge {
	return ir.Edge{From: from, To: to, Send: &ir.ChanOp{Chan: ch, Args: []*ir.Expr{ir.Const(v)}}, Text: ch + "!" + string(rune('0'+v))}
}

func recvEdge(from, to int, ch string) ir.Edge {
	return ir.Edge{From: from, To: to, Recv: &ir.RecvOp{Chan: ch, Args: []ir.RecvArg{{}}}, Text: ch + "?"}
}

func recvInto(from, to int, ch, v string) ir.Edge {
	return ir.Edge{From: from, To: to, Recv: &ir.RecvOp{Chan: ch, Args: []ir.RecvArg{{Var: v}}}, Text: ch + "?" + v}
}

// reqOf is the directed-channel requirements the plan records for p at l, as
// "send"/"recv" strings in edge order; nil when the plan records none.
func reqOf(pl *porPlan, p, l int) []string {
	if pl.req == nil {
		return nil
	}
	var out []string
	for _, r := range pl.req[p][l] {
		if r.send {
			out = append(out, "send")
		} else {
			out = append(out, "recv")
		}
	}
	return out
}

func wantReq(t *testing.T, pl *porPlan, p, l int, want ...string) {
	t.Helper()
	got := reqOf(pl, p, l)
	if len(got) != len(want) {
		t.Fatalf("process %d at %d: requirements %v, want %v", p, l, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("process %d at %d: requirements %v, want %v", p, l, got, want)
		}
	}
}

// ---- directed channels: what is independent ---------------------------------------

func TestPORADirectedChannelsSendAndReceiveDoNotConflict(t *testing.T) {
	m := model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("S", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("R", nil, 2, recvEdge(0, 1, "c")))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 1, 0, true)
	// Each is expanded alone only while its channel end can act now.
	wantReq(t, pl, 0, 0, "send")
	wantReq(t, pl, 1, 0, "recv")
}

func TestPORADirectedChannelKeepsEveryAlternativeOfTheLocationInTheRequirement(t *testing.T) {
	m := model(nil, []ir.Channel{chanDecl("c", 2), chanDecl("d", 2)}, nil,
		proc("S", nil, 2, sendEdge(0, 1, "c", 1), sendEdge(0, 1, "c", 2), sendEdge(0, 1, "d", 1)),
		proc("R", nil, 2, recvEdge(0, 1, "c"), recvEdge(0, 1, "d")))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantReq(t, pl, 0, 0, "send", "send", "send")
	wantReq(t, pl, 1, 0, "recv", "recv")
}

func TestPORAPipelineIsEligibleAtEveryStage(t *testing.T) {
	// source -> c -> filter -> d -> sink
	m := model(nil, []ir.Channel{chanDecl("c", 2), chanDecl("d", 2)}, nil,
		proc("Source", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("Filter", []ir.Var{byteVar("m")}, 3, recvInto(0, 1, "c", "m"), ir.Edge{From: 1, To: 2, Send: &ir.ChanOp{Chan: "d", Args: []*ir.Expr{ir.Ref("m")}}}),
		proc("Sink", nil, 2, recvEdge(0, 1, "d")))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 1, 0, true)
	wantEligible(t, pl, 1, 1, true)
	wantEligible(t, pl, 2, 0, true)
}

// ---- what keeps the conflict --------------------------------------------------------

func TestPORATwoSendersConflictWithEachOtherNotWithTheReceiver(t *testing.T) {
	// Two sends share the send end: the order of the messages in the buffer.
	m := model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("S1", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("S2", nil, 2, sendEdge(0, 1, "c", 2)),
		proc("R", nil, 2, recvEdge(0, 1, "c")))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)
	// The receive end is the receiver's alone, and a receive is independent of both sends.
	wantEligible(t, pl, 2, 0, true)
	wantReq(t, pl, 2, 0, "recv")
}

func TestPORATwoReceiversConflictWithEachOtherNotWithTheSender(t *testing.T) {
	m := model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("S", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("R1", nil, 2, recvEdge(0, 1, "c")),
		proc("R2", nil, 2, recvEdge(0, 1, "c")))
	pl := planOf(t, m)
	wantEligible(t, pl, 1, 0, false)
	wantEligible(t, pl, 2, 0, false)
	wantEligible(t, pl, 0, 0, true)
	wantReq(t, pl, 0, 0, "send")
}

func TestPORAProcessAtBothEndsIsJudgedLocationByLocation(t *testing.T) {
	// P sends at 0 and receives at 1; Q receives. P's send is independent of
	// Q's receive, P's own receive is not (two receives share the end).
	m := model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("P", nil, 3, sendEdge(0, 1, "c", 1), recvEdge(1, 2, "c")),
		proc("Q", nil, 2, recvEdge(0, 1, "c")))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 0, 1, false)
	wantEligible(t, pl, 1, 0, false)
}

func TestPORALengthReadAnywhereKeepsTheChannelWhole(t *testing.T) {
	base := func() *ir.Model {
		return model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
			proc("S", nil, 2, sendEdge(0, 1, "c", 1)),
			proc("R", nil, 2, recvEdge(0, 1, "c")),
			proc("W", []ir.Var{byteVar("l")}, 2, ir.Edge{From: 0, To: 1}))
	}
	m := base()
	wantEligible(t, planOf(t, m), 0, 0, true) // control: without the read it is directed

	// In a guard of a third process.
	m = base()
	m.Processes[2].Edges[0].Guard = ir.Binary("eq", ir.Len("c"), ir.Const(1))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)

	// In an assert, an effect, a send argument, a receive match.
	for name, edit := range map[string]func(*ir.Model){
		"assert": func(m *ir.Model) { m.Processes[2].Edges[0].Assert = ir.Binary("le", ir.Len("c"), ir.Const(2)) },
		"effect": func(m *ir.Model) {
			m.Processes[2].Edges[0].Effect = []ir.Assign{{Var: "l", Value: ir.Len("c")}}
		},
		"match": func(m *ir.Model) {
			m.Processes[1].Edges[0].Recv.Args = []ir.RecvArg{{Match: ir.Len("c")}}
		},
	} {
		m := base()
		edit(m)
		if pl := planOf(t, m); pl.eligible[0][0] {
			t.Errorf("a length read in %s: the sender is still eligible", name)
		}
	}

	// In a property.
	m = base()
	m.Properties = append(m.Properties, ir.Property{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("le", ir.Len("c"), ir.Const(1))})
	wantEligible(t, planOf(t, m), 0, 0, false)
}

func TestPORAChannelNamedByValueAnywhereKeepsEveryChannelWhole(t *testing.T) {
	m := model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("S", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("R", nil, 2, recvEdge(0, 1, "c")),
		proc("W", nil, 2, ir.Edge{From: 0, To: 1, Guard: ir.Binary("eq", ir.CLen(ir.Const(1)), ir.Const(0))}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)
}

func TestPORAClearedChannelConflictsWithBothEnds(t *testing.T) {
	// R clears the channel at 1. A clear is a use of the whole channel: the
	// sender's send is no longer independent of everything R does, and R at 1
	// is not independent of the sender. R's receive at 0 still is.
	m := model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("S", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("R", nil, 3, recvEdge(0, 1, "c"), ir.Edge{From: 1, To: 2, ClearChans: []int{0}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, true)
	wantEligible(t, pl, 1, 1, false)
}

func TestPORAnElseSiblingKeepsTheChannelOpsWhole(t *testing.T) {
	// S: c!1 or, when the channel is full, else. The receiver's pop turns the
	// else off by making room, so the pair is dependent.
	m := model(nil, []ir.Channel{chanDecl("c", 1)}, nil,
		proc("S", nil, 3, sendEdge(0, 1, "c", 1), ir.Edge{From: 0, To: 2, Else: true}),
		proc("R", nil, 2, recvEdge(0, 1, "c")))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)
	wantReq(t, pl, 0, 0)
}

func TestPORADStepKeepsTheChannelOpsWhole(t *testing.T) {
	// The d_step edge itself sends; and a location a d_step enters has a send.
	m := model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, DStep: true, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(1)}}}, sendEdge(1, 1, "c", 2)),
		proc("R", nil, 2, recvEdge(0, 1, "c")))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)

	m = model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("S", []ir.Var{byteVar("t")}, 3,
			ir.Edge{From: 0, To: 1, DStep: true, Effect: []ir.Assign{set("t", 1)}}, // enters 1
			sendEdge(1, 2, "c", 1)), // the continuation sends
		proc("R", nil, 2, recvEdge(0, 1, "c")))
	pl = planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)
}

func TestPORDirectedChannelsStillRefuseTheRendezvous(t *testing.T) {
	m := model(nil, []ir.Channel{chanDecl("c", 0)}, nil,
		proc("S", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("R", nil, 2, recvEdge(0, 1, "c")))
	wantReason(t, planOf(t, m), "rendezvous")
}

func TestPORDirectedChannelsRespectTheVisibleCells(t *testing.T) {
	// The receiver binds the global g; an invariant reads g: the receive is
	// visible and is never expanded alone, whatever the channel says.
	m := model([]ir.Var{byteVar("g")}, []ir.Channel{chanDecl("c", 2)},
		[]ir.Property{{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("ne", ir.Ref("g"), ir.Const(9))}},
		proc("S", nil, 2, sendEdge(0, 1, "c", 1)),
		proc("R", nil, 2, recvInto(0, 1, "c", "g")))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 1, 0, false)
}

func TestPORADStepEdgeKeepsItsOwnChannelOpWhole(t *testing.T) {
	// The d_step edge sends and the step goes on into a local edge: the
	// continuation touches no channel, so what keeps the sender from being
	// expanded alone is the d_step edge itself. (Conservative: such a step
	// is probably as independent of the receive as a plain send is.)
	m := model(nil, []ir.Channel{chanDecl("c", 2)}, nil,
		proc("S", []ir.Var{byteVar("t")}, 3,
			ir.Edge{From: 0, To: 1, DStep: true, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(1)}}},
			ir.Edge{From: 1, To: 2, Effect: []ir.Assign{set("t", 1)}}),
		proc("R", nil, 2, recvEdge(0, 1, "c")))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, false)
}

func TestPORAPrivateChannelStillAsksForRoomAndMessage(t *testing.T) {
	// P is the only process on c, at both ends. Nobody else can change the
	// channel, so the requirement is not needed; it is recorded anyway, which
	// can only make P wait for a state in which it holds, never lose a verdict.
	m := model(nil, []ir.Channel{chanDecl("c", 1)}, nil,
		proc("P", nil, 3, sendEdge(0, 1, "c", 1), recvEdge(1, 2, "c")),
		proc("Q", []ir.Var{byteVar("l")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("l", 1)}}))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 0, 1, true)
	wantReq(t, pl, 0, 0, "send")
	wantReq(t, pl, 0, 1, "recv")
}

func TestPORAClearOverlapsTheReceiveEndOfAnotherProcess(t *testing.T) {
	// P sends and then clears; Q only receives. The clear uses the whole
	// channel, so Q's receive is not independent of it.
	pl := planOf(t, clearBeforeReceive())
	wantEligible(t, pl, 0, 0, true)  // P's send, independent of Q's receive
	wantEligible(t, pl, 0, 1, false) // the clear
	wantEligible(t, pl, 1, 0, false) // Q's receive, against P's clear
}

// channelsCanAct must read the state being expanded (s.cur), not the scratch
// successor a speculative firing left in s.next, and must compare with the
// capacity of the channel it names.
func TestPORChannelsCanActReadsTheStateBeingExpanded(t *testing.T) {
	m := model(nil, []ir.Channel{chanDecl("c", 2), chanDecl("d", 1)}, nil,
		proc("S", nil, 2, sendEdge(0, 1, "c", 1), sendEdge(0, 1, "d", 1)),
		proc("R", nil, 2, recvEdge(0, 1, "c"), recvEdge(0, 1, "d")))
	cp, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	l := cp.layout
	fill := func(state []byte, ch, n int) {
		for i := 0; i < n; i++ {
			l.ChanPush(state, ch, []int64{1})
		}
	}
	state := func(c, d int) []byte {
		s := l.Initial()
		fill(s, 0, c)
		fill(s, 1, d)
		return s
	}
	r := &porRun{}
	for _, tc := range []struct {
		name     string
		cur, nxt []byte
		reqs     []porChanReq
		want     bool
	}{
		{"room now, full in the successor", state(1, 0), state(2, 0), []porChanReq{{0, true}}, true},
		{"full now, room in the successor", state(2, 0), state(0, 0), []porChanReq{{0, true}}, false},
		{"one of two slots taken is room", state(1, 0), state(1, 0), []porChanReq{{0, true}}, true},
		{"a message now, none in the successor", state(1, 0), state(0, 0), []porChanReq{{0, false}}, true},
		{"none now, a message in the successor", state(0, 0), state(1, 0), []porChanReq{{0, false}}, false},
		{"the second channel is the one asked", state(0, 1), state(0, 1), []porChanReq{{1, true}}, false},
		{"the second channel has room, the first is full", state(2, 0), state(2, 0), []porChanReq{{1, true}}, true},
		{"every requirement must hold", state(1, 1), state(1, 1), []porChanReq{{0, true}, {1, true}}, false},
		{"none required", state(2, 1), state(0, 0), nil, true},
	} {
		s := &search{c: cp, cur: tc.cur, next: tc.nxt}
		if got := r.channelsCanAct(s, tc.reqs); got != tc.want {
			t.Errorf("%s: channelsCanAct = %v, want %v", tc.name, got, tc.want)
		}
	}
}
