package explore

import (
	"context"
	"strings"
	"testing"

	"modelcheck/ir"
)

// Hand-built IR models for the G1 explorer features. Locations are
// numbered 0..n-1 in order; every process ends in a location without
// outgoing edges (terminated).

func procLocs(n int) []ir.Location {
	locs := make([]ir.Location, n)
	return locs
}

func runModel(t *testing.T, m *ir.Model, opt Options) *Result {
	t.Helper()
	res, err := Run(context.Background(), m, opt)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestBufferedChannelSendReceiveAndMatch(t *testing.T) {
	// A: c!1,7 ; c!2,8      B: c?2,v (must skip the head 1 → blocked until…) — no: FIFO, so B blocks forever on 2
	// Use matching on the head: B: c?1,v ; c?2,v
	m := &ir.Model{Schema: ir.Schema, Name: "buf",
		Channels: []ir.Channel{{Name: "c", Capacity: 2, Fields: []ir.Type{ir.Byte, ir.Byte}}},
		Processes: []ir.Process{
			{Name: "A", Locations: procLocs(3), Edges: []ir.Edge{
				{From: 0, To: 1, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(1), ir.Const(7)}}, Text: "c!1,7"},
				{From: 1, To: 2, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(2), ir.Const(8)}}, Text: "c!2,8"},
			}},
			{Name: "B", Locals: []ir.Var{{Name: "v", Type: ir.Byte}}, Locations: procLocs(3), Edges: []ir.Edge{
				{From: 0, To: 1, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Match: ir.Const(1)}, {Var: "v"}}}, Text: "c?1,v"},
				{From: 1, To: 2, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Match: ir.Const(2)}, {Var: "v"}}}, Text: "c?2,v"},
			}},
		},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock},
			{ID: "v8", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.Ref("v"), ir.Const(8))}},
	}
	// v is local to B; reach properties are global scope — refer via a global mirror instead.
	m.Properties = m.Properties[:1]
	for _, mode := range []Mode{DFS, BFS} {
		res := runModel(t, m, Options{Mode: mode})
		if res.Outcomes[0].Status != Verified || !res.Complete {
			t.Fatalf("%s: %+v", mode, res.Outcomes[0])
		}
		// states: s0, A sent 1, {A sent 2 | B took 1} …: 1 + 2 + 2 + 1 = 6 interleavings
		if res.States != 6 {
			t.Fatalf("%s: %d states, want 6", mode, res.States)
		}
	}
	// A receive whose match fails on the head blocks: swap B's order → deadlock.
	m.Processes[1].Edges[0].Recv.Args[0].Match = ir.Const(2)
	m.Processes[1].Edges[1].Recv.Args[0].Match = ir.Const(1)
	res := runModel(t, m, Options{})
	if res.Outcomes[0].Status != Violated {
		t.Fatalf("mismatched head must block: %+v", res.Outcomes[0])
	}
	tr := res.Outcomes[0].Trace
	if len(tr.FinalChannels) != 1 || len(tr.FinalChannels[0].Messages) != 2 || tr.FinalChannels[0].Messages[0][0] != 1 {
		t.Fatalf("final channel contents %+v", tr.FinalChannels)
	}
}

func TestRendezvousHandshakeIsOneStep(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "rv",
		Channels: []ir.Channel{{Name: "c", Capacity: 0, Fields: []ir.Type{ir.Byte}}},
		Processes: []ir.Process{
			{Name: "A", Locations: procLocs(3), Edges: []ir.Edge{
				{From: 0, To: 1, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(5)}}, Text: "c!5"},
				{From: 1, To: 2, Send: &ir.ChanOp{Chan: "c", Args: []*ir.Expr{ir.Const(6)}}, Text: "c!6"},
			}},
			{Name: "B", Locals: []ir.Var{{Name: "v", Type: ir.Byte}}, Locations: procLocs(2), Edges: []ir.Edge{
				{From: 0, To: 1, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Var: "v"}}}, Text: "c?v"},
			}},
			{Name: "C", Locals: []ir.Var{{Name: "w", Type: ir.Byte}}, Locations: procLocs(2), Edges: []ir.Edge{
				{From: 0, To: 1, Recv: &ir.RecvOp{Chan: "c", Args: []ir.RecvArg{{Match: ir.Const(6)}}}, Text: "c?6"},
			}},
		},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}},
	}
	res := runModel(t, m, Options{Sweep: true})
	// s0; A!5→B (C cannot: 6≠5); then A!6→C; = 3 states; no intermediate.
	if res.States != 3 || !res.Complete || res.Outcomes[0].Status != Verified {
		t.Fatalf("%d states, %+v", res.States, res.Outcomes[0])
	}
	// Two candidate receivers for the same value: two transitions.
	m.Processes[2].Edges[0].Recv.Args[0] = ir.RecvArg{Var: "w"}
	res = runModel(t, m, Options{Sweep: true})
	// s0; {B took 5 | C took 5}; from B-took-5: C takes 6; from C-took-5: B takes 6 → 5 states
	if res.States != 5 || res.Transitions != 4 {
		t.Fatalf("%d states %d transitions, want 5 and 4", res.States, res.Transitions)
	}
	// The trace names the partner.
	m.Processes[2].Edges[0].Recv.Args[0] = ir.RecvArg{Match: ir.Const(9)}
	m.Processes = m.Processes[:2] // A, B only: second send has no partner → deadlock
	res = runModel(t, m, Options{})
	if res.Outcomes[0].Status != Violated {
		t.Fatalf("%+v", res.Outcomes[0])
	}
	st := res.Outcomes[0].Trace.Steps[0]
	if st.Partner == nil || st.Partner.Process != "B" || st.Partner.Command != "c?v" || st.Changes[0].Var != "B.v" || st.Changes[0].After != 5 {
		t.Fatalf("handshake step %+v partner %+v", st, st.Partner)
	}
}

func TestElseIsEnabledOnlyWhenSiblingsAreNot(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "else",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte}},
		Processes: []ir.Process{{Name: "A", Locations: procLocs(3), Edges: []ir.Edge{
			{From: 0, To: 1, Guard: ir.Binary("gt", ir.Ref("x"), ir.Const(0)), Text: "x > 0"},
			{From: 0, To: 2, Else: true, Text: "else"},
			{From: 1, To: 0, Effect: []ir.Assign{{Var: "x", Value: ir.Const(0)}}, Text: "x = 0"},
		}}},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock},
			{ID: "r", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.PC(0), ir.Const(1))}},
	}
	res := runModel(t, m, Options{})
	// x = 0 initially: only else is enabled → location 2, terminated; location 1 unreachable
	if res.States != 2 || res.Outcomes[1].Status != Violated {
		t.Fatalf("%d states, reach %+v", res.States, res.Outcomes[1])
	}
	m.Globals[0].Init = []int64{1}
	res = runModel(t, m, Options{})
	// x = 1: only the guard is enabled (else disabled) → 1 → x = 0 → 0 (x=0): then else → 2
	if res.Outcomes[1].Status != Verified || res.States != 4 {
		t.Fatalf("%d states, reach %+v", res.States, res.Outcomes[1])
	}
}

func TestTimeoutIsEnabledOnlyWhenNothingElseIs(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "timeout",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte}},
		Processes: []ir.Process{
			{Name: "A", Locations: procLocs(2), Edges: []ir.Edge{
				{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(1)), Text: "x == 1"},
			}},
			{Name: "B", Locations: procLocs(2), Edges: []ir.Edge{
				{From: 0, To: 1, Guard: ir.Timeout(), Effect: []ir.Assign{{Var: "x", Value: ir.Const(1)}}, Text: "timeout -> x = 1"},
			}},
			{Name: "C", Locations: procLocs(2), Edges: []ir.Edge{
				{From: 0, To: 1, Text: "skip"},
			}},
		},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}},
	}
	res := runModel(t, m, Options{Sweep: true})
	// s0: only C's skip (timeout false because C can move); s1: nothing else → timeout fires (x=1); s2: A moves; s3: all terminated.
	if res.States != 4 || res.Outcomes[0].Status != Verified {
		t.Fatalf("%d states %+v", res.States, res.Outcomes[0])
	}
	if res.Transitions != 3 {
		t.Fatalf("%d transitions, want 3 (timeout never fires while another edge is enabled)", res.Transitions)
	}
}

func TestDStepRunsAsOneStepAndBlockingIsInvalidModel(t *testing.T) {
	mk := func(guard *ir.Expr) *ir.Model {
		return &ir.Model{Schema: ir.Schema, Name: "dstep",
			Globals: []ir.Var{{Name: "x", Type: ir.Byte}},
			Processes: []ir.Process{
				{Name: "A", Locations: procLocs(4), Edges: []ir.Edge{
					{From: 0, To: 1, Effect: []ir.Assign{{Var: "x", Value: ir.Const(1)}}, DStep: true, Text: "x = 1"},
					{From: 1, To: 2, Guard: guard, DStep: true, Text: "x == 2"},
					{From: 2, To: 3, Effect: []ir.Assign{{Var: "x", Value: ir.Const(3)}}, Text: "x = 3"},
				}},
				{Name: "B", Locations: procLocs(2), Edges: []ir.Edge{
					{From: 0, To: 1, Effect: []ir.Assign{{Var: "x", Value: ir.Const(2)}}, Text: "x = 2"},
				}},
			},
			Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}},
		}
	}
	res := runModel(t, mk(nil), Options{Sweep: true})
	// d_step is one step: s0; A done (x=3); B done (x=2); A then B (x=2);
	// B then A (x=3) — 5 states, none inside the d_step
	if res.States != 5 || res.Transitions != 4 || res.Outcomes[0].Status != Verified {
		t.Fatalf("%d states %+v", res.States, res.Outcomes[0])
	}
	res = runModel(t, mk(ir.Binary("eq", ir.Ref("x"), ir.Const(2))), Options{})
	o := res.Outcomes[0]
	if o.Status != InvalidModel || !strings.Contains(o.Reason, "block in d_step seq") || !strings.Contains(o.Reason, "x == 2") {
		t.Fatalf("%+v", o)
	}
	if o.Trace == nil || o.Trace.Steps[len(o.Trace.Steps)-1].Command != "x = 1" {
		t.Fatalf("trace must end at the d_step entry: %+v", o.Trace)
	}
}

func TestRunStartsDormantProcessWithArguments(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "run",
		Processes: []ir.Process{
			{Name: "init", Locations: procLocs(2), Edges: []ir.Edge{
				{From: 0, To: 1, Run: &ir.RunOp{Proc: 1, Entry: 1, Args: []*ir.Expr{ir.Const(36), ir.Const(12)}}, Text: "run E(36, 12)"},
			}},
			{Name: "E", Params: 2, Dynamic: true, Locals: []ir.Var{{Name: "x", Type: ir.Int}, {Name: "y", Type: ir.Int}},
				Locations: append(procLocs(3), ir.Location{}), Initial: 0, Edges: []ir.Edge{
					{From: 1, To: 2, Guard: ir.Binary("gt", ir.Ref("x"), ir.Ref("y")), Effect: []ir.Assign{{Var: "x", Value: ir.Binary("sub", ir.Ref("x"), ir.Ref("y"))}}, Text: "x = x - y"},
					{From: 2, To: 1, Text: "loop"},
					{From: 1, To: 3, Guard: ir.Binary("eq", ir.Ref("x"), ir.Ref("y")), Text: "done"},
				}},
		},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock},
			{ID: "gcd", Kind: ir.KindReach, Expr: ir.Binary("and", ir.Binary("eq", ir.PC(1), ir.Const(3)), ir.Binary("eq", ir.PC(0), ir.Const(1)))}},
	}
	res := runModel(t, m, Options{})
	if res.Outcomes[0].Status != Verified || res.Outcomes[1].Status != Verified {
		t.Fatalf("%+v", res.Outcomes)
	}
	w := res.Outcomes[1].Trace
	if w.Steps[0].Command != "run E(36, 12)" || w.Steps[0].Changes[0].Var != "E.x" || w.Steps[0].Changes[0].After != 36 {
		t.Fatalf("run step %+v", w.Steps[0])
	}
	if got := w.Final; got[0].Value != 12 || got[1].Value != 12 {
		t.Fatalf("final %+v", got)
	}
}

func TestClaimProcessIsStoredNotRun(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "claim",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte}},
		Processes: []ir.Process{
			{Name: "A", Locations: procLocs(1), Edges: []ir.Edge{{From: 0, To: 0, Effect: []ir.Assign{{Var: "x", Value: ir.Binary("sub", ir.Const(1), ir.Ref("x"))}}, Text: "flip"}}},
			{Name: "never", Claim: true, Locations: procLocs(2), Edges: []ir.Edge{{From: 0, To: 1, Text: "claim step"}}},
		},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}},
	}
	res := runModel(t, m, Options{})
	if res.States != 2 || res.Transitions != 2 || res.Outcomes[0].Status != Verified {
		t.Fatalf("%d states %d transitions %+v", res.States, res.Transitions, res.Outcomes[0])
	}
}

func TestSweepKeepsSearchingAfterVerdicts(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "sweep",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte}},
		Processes: []ir.Process{{Name: "A", Locations: procLocs(1), Edges: []ir.Edge{
			{From: 0, To: 0, Guard: ir.Binary("lt", ir.Ref("x"), ir.Const(9)), Assert: ir.Binary("lt", ir.Ref("x"), ir.Const(1)), Effect: []ir.Assign{{Var: "x", Value: ir.Binary("add", ir.Ref("x"), ir.Const(1))}}, Text: "x++"},
		}}},
		Properties: []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
	}
	early := runModel(t, m, Options{})
	full := runModel(t, m, Options{Sweep: true})
	if early.Outcomes[0].Status != Violated || full.Outcomes[0].Status != Violated {
		t.Fatalf("%+v %+v", early.Outcomes[0], full.Outcomes[0])
	}
	if early.Complete || early.Stop != "all properties decided" {
		t.Fatalf("early: %d states, stop %q", early.States, early.Stop)
	}
	if !full.Complete || full.States != 10 {
		t.Fatalf("sweep: %d states complete=%v", full.States, full.Complete)
	}
	if full.Outcomes[0].Trace.Summary != early.Outcomes[0].Trace.Summary {
		t.Fatalf("the first verdict must stand under sweep")
	}
}
