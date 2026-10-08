package explore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"modelcheck/cex"
	"modelcheck/ir"
)

// A weak-fairness null step is bookkeeping of the copies construction: it
// advances the copy and leaves the system state and the claim where they are.
// It is not a step of the product, and a loop made of null steps alone is not
// a run, however many copies it passes through. pan counts a process that
// moves or cannot move inside the steps of the product (a claim step, then a
// system step), so a claim without an enabled edge ends the path, and under
// -DNP a state without a move has no successor at all. The tests below hold
// the engine to that: found by differential testing against pan, where the
// engine reported violations on such states and pan reported none
// (steps/fix-weakfairness-confirmation.md).

const weakfairTestdata = "../testdata/promela"

// decisionTestdata holds the models of the weak-fairness decision study, with
// hand-encoded graphs and an independent checker (wfcheck.py).
const decisionTestdata = "../testdata/weakdecision/models"

// weakfairOutcome runs one property of a testdata model and returns its
// outcome; kind "progress" adds the property, any other id is the model's own.
func weakfairOutcome(t *testing.T, file, id, fairness string) Outcome {
	t.Helper()
	dir := weakfairTestdata
	if strings.HasPrefix(file, "decision:") {
		dir, file = decisionTestdata, strings.TrimPrefix(file, "decision:")
	}
	m, defs := parseFile(t, filepath.Join(dir, file))
	if id == "progress" {
		have := false
		for _, p := range m.Properties {
			if p.ID == "progress" {
				have = true
			}
		}
		if !have {
			m.Properties = append(m.Properties, ir.Property{ID: "progress", Kind: ir.KindProgress})
		}
	}
	res, err := Run(context.Background(), m, Options{Sweep: true, Defines: defs, Fairness: fairness})
	if err != nil {
		t.Fatal(err)
	}
	return outcomeOf(t, res, id)
}

func isNullStep(st cex.Step) bool {
	return st.Process == cex.NullProcess && strings.HasPrefix(st.Command, "(weak fairness:")
}

// TestWeakFairnessNoStepNoCycle: on these models every run of the product
// ends in a state where the claim has no enabled edge (never claim, np_ with
// a process on a progress label) or where the non-progress product has no
// move at all. No cycle exists with or without fairness; the engine found
// one under weak fairness, in the copies alone.
func TestWeakFairnessNoStepNoCycle(t *testing.T) {
	cases := []struct{ file, prop string }{
		{"weakfair-claim-blocked.pml", "never"},
		{"weakfair-claim-m5271.pml", "never"},
		{"weakfair-np-claim.pml", "progress"},
		{"weakfair-progress-r1.pml", "progress"},
		{"weakfair-np-deadlock.pml", "progress"},
	}
	for _, c := range cases {
		for _, fairness := range []string{"none", "weak"} {
			t.Run(c.file+"/"+fairness, func(t *testing.T) {
				o := weakfairOutcome(t, c.file, c.prop, fairness)
				if o.Status != Verified || o.Stats == nil || !o.Stats.Complete {
					t.Fatalf("%s under %s: %s / %q (trace %v); pan -a/-l, with and without -f, finds no error",
						c.prop, fairness, o.Status, o.Reason, o.Trace != nil)
				}
			})
		}
	}
}

// TestWeakFairnessStutterStays: the other direction. Where every process is
// blocked and the claim can keep moving, the stuttering run is weakly fair
// (nothing is enabled, so nothing is owed a move) and accepted; its loop has
// a product step in it besides the copy bookkeeping. weakfair-stutter-accept
// is a case pan -a -f agrees on; in weakfair-stutter-cycle the claim passes
// through a non-accepting location on the loop, which pan 6.5.2 -f does not
// report and the engine, which keeps the stutter extension under fairness,
// does.
func TestWeakFairnessStutterStays(t *testing.T) {
	for _, file := range []string{"weakfair-stutter-accept.pml", "weakfair-stutter-cycle.pml"} {
		for _, fairness := range []string{"none", "weak"} {
			t.Run(file+"/"+fairness, func(t *testing.T) {
				o := weakfairOutcome(t, file, "never", fairness)
				if o.Status != Violated || o.Trace == nil || o.Trace.Loop == nil {
					t.Fatalf("%s: %s / %q; a blocked system with a claim that keeps moving is an acceptance cycle", fairness, o.Status, o.Reason)
				}
				real := 0
				for _, st := range o.Trace.LoopSteps() {
					if !isNullStep(st) {
						real++
					}
				}
				if real == 0 {
					t.Fatalf("%s: the loop of the counterexample is %d fairness null steps and nothing else", fairness, len(o.Trace.LoopSteps()))
				}
			})
		}
	}
}

// TestWeakFairnessLoopsHaveAProductStep: whatever the model, the loop of a
// weak-fairness counterexample contains a step that is not a null step. The
// models are the testdata ones above plus the corpus models that have an
// accept label, a claim or a progress label and are small enough.
func TestWeakFairnessLoopsHaveAProductStep(t *testing.T) {
	type mc struct {
		file, prop string
		corpus     bool
		defines    []string
	}
	cases := []mc{
		{"weakfair-claim-blocked.pml", "never", false, nil},
		{"weakfair-stutter-accept.pml", "never", false, nil},
		{"weakfair-stutter-cycle.pml", "never", false, nil},
		{"CH8/fairness.pml", "accept", true, nil},
		{"CH4/fair_accept.pml", "accept", true, nil},
		{"CH8/trivial.pml", "never", true, nil},
		{"CH4/fair.pml", "progress", true, nil},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			path := filepath.Join(weakfairTestdata, c.file)
			if c.corpus {
				path = filepath.Join(corpus, c.file)
			}
			m, defs := parseFile(t, path, c.defines...)
			if c.prop == "progress" {
				m.Properties = append(m.Properties, ir.Property{ID: "progress", Kind: ir.KindProgress})
			}
			res, err := Run(context.Background(), m, Options{Sweep: true, Defines: defs, Fairness: "weak"})
			if err != nil {
				t.Fatal(err)
			}
			o := outcomeOf(t, res, c.prop)
			if o.Status != Violated {
				return // nothing to check on a verified property
			}
			if o.Trace == nil || o.Trace.Loop == nil {
				t.Fatalf("violated without a loop: %+v", o.Trace)
			}
			for _, st := range o.Trace.LoopSteps() {
				if !isNullStep(st) {
					return
				}
			}
			t.Fatalf("the loop of the weak-fairness counterexample is made of null steps only (%d steps)", len(o.Trace.LoopSteps()))
		})
	}
}

// TestWeakFairnessTimeoutOwesAMove: `timeout` is true exactly when no other
// statement is executable, so a process whose next statement is a bare
// `timeout` is enabled in every timeout state and weak fairness owes it a move
// there. d3_one: both loops are made of timeout states only and Q never moves
// on the cycle, so no weakly fair accepting cycle exists (pan -f: no error).
// The controls pass through a state where another statement is executable, so
// Q is disabled there and the cycle is weakly fair. The engine used to judge
// enabledness by ordinary statements only and said `violated` on d3_one.
func TestWeakFairnessTimeoutOwesAMove(t *testing.T) {
	cases := []struct {
		file, prop string
		weak       Status
	}{
		{"decision:d3_one.pml", "never", Verified},
		{"decision:d3_ltl.pml", "never", Verified},
		{"decision:d3_np.pml", "progress", Verified},
		{"decision:d3_ctl_split.pml", "never", Violated},
		{"decision:d3_ctl_skip.pml", "never", Violated},
		{"decision:d3_two_loop.pml", "never", Violated},
		{"decision:d3_mixed.pml", "never", Violated},
		{"decision:d3_np_flip.pml", "progress", Violated},
		{"decision:to1.pml", "never", Violated},
		{"decision:to2.pml", "never", Violated},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			if o := weakfairOutcome(t, c.file, c.prop, "none"); o.Status != Violated {
				t.Errorf("without fairness: %s (%s), want violated", o.Status, o.Reason)
			}
			if o := weakfairOutcome(t, c.file, c.prop, "weak"); o.Status != c.weak {
				t.Errorf("weak fairness: %s (%s), want %s", o.Status, o.Reason, c.weak)
			}
		})
	}
}

// TestWeakFairnessClaimNeedNotBeLast: the copy that stands for system process k
// must be tied to the k-th process that is not the claim, wherever the claim
// stands in the IR. The Promela frontend, the LTL claim and np_ put the claim
// last; an IR given to `mcd check --ir` or `mc_check` need not. With the claim
// first, copy 1 was the claim (never "blocked" while it has an edge, its steps
// never counting as moves), so the round never closed, every cycle was lost and
// the last real process was never covered: `verified` where the cycle exists.
// Here P0 flips a bit for ever, P1 is blocked for ever and the claim accepts
// everything: weakly fair and accepting, whatever the order.
func TestWeakFairnessClaimNeedNotBeLast(t *testing.T) {
	p0 := ir.Process{Name: "P0", Locations: []ir.Location{{}},
		Edges: []ir.Edge{{From: 0, To: 0, Effect: []ir.Assign{{Var: "a", Value: ir.Binary("sub", ir.Const(1), ir.Ref("a"))}}, Text: "a = 1 - a"}}}
	p1 := ir.Process{Name: "P1", Locations: []ir.Location{{}, {}},
		Edges: []ir.Edge{{From: 0, To: 1, Guard: ir.Binary("eq", ir.Const(0), ir.Const(1)), Text: "0 == 1"}}}
	claim := ir.Process{Name: "never", Claim: true, Locations: []ir.Location{{Name: "accept_S0", Labels: []ir.Label{ir.Accept}}},
		Edges: []ir.Edge{{From: 0, To: 0, Text: "true"}}}
	procs := []ir.Process{p0, p1, claim}
	for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		m := &ir.Model{Schema: ir.Schema, Name: "claim-order", Globals: []ir.Var{{Name: "a", Type: ir.Bit}},
			Properties: []ir.Property{{ID: "never", Kind: ir.KindLTL}}}
		var names []string
		for _, i := range order {
			m.Processes = append(m.Processes, procs[i])
			names = append(names, procs[i].Name)
		}
		for _, fairness := range []string{"none", "weak"} {
			t.Run(strings.Join(names, ",")+"/"+fairness, func(t *testing.T) {
				res, err := Run(context.Background(), m, Options{Sweep: true, Fairness: fairness})
				if err != nil {
					t.Fatal(err)
				}
				o := outcomeOf(t, res, "never")
				if o.Status != Violated {
					t.Fatalf("%s: %s (%s), want violated: P0 moves for ever, P1 is blocked for ever, the claim accepts", fairness, o.Status, o.Reason)
				}
				// the null steps of the trace name the process whose copy they
				// are, never the claim (copy k stands for the k-th process that
				// is not the claim)
				for _, st := range o.Trace.Steps {
					if isNullStep(st) && strings.Contains(st.Command, "process never") {
						t.Errorf("%s: the note %q names the claim as a blocked process", fairness, st.Command)
					}
				}
			})
		}
	}
}

// TestWeakFairnessEveryClaimEdgeSeesTimeoutMoves: the system's moves are
// enumerated once per enabled claim edge, and `timeout` is true when the state
// has no other move. A claim with two enabled edges in a timeout state used to
// get the timeout moves on the first edge only (the counter that gates the
// timeout phase had been raised by the first edge's moves): the accepting run
// of weakfair-timeout-claim.pml was lost and the engine said verified, with and
// without fairness. pan: 2 errors without -f, 1 with.
func TestWeakFairnessEveryClaimEdgeSeesTimeoutMoves(t *testing.T) {
	for _, fairness := range []string{"none", "weak"} {
		if o := weakfairOutcome(t, "weakfair-timeout-claim.pml", "never", fairness); o.Status != Violated {
			t.Errorf("%s: %s (%s), want violated", fairness, o.Status, o.Reason)
		}
	}
}

// TestWeakFairnessNullStepKeepsTimeoutMoves: a null step is not a move of the
// frame. weakfair-timeout-null.pml has a process that is blocked for good in
// front of a process that lives on timeout moves; the copy of the blocked
// process is left by a null step in every state, and the timeout moves of the
// system must still be enumerated in that frame. The product has 16 states
// (testdata/weakdecision/copies_tc3.py derives the number from the construction
// alone); with the null step counted among the moves it had 8 (the timeout
// moves were lost and a stutter step was offered instead).
func TestWeakFairnessNullStepKeepsTimeoutMoves(t *testing.T) {
	o := weakfairOutcome(t, "weakfair-timeout-null.pml", "accept", "weak")
	if o.Status != Violated || o.Stats == nil {
		t.Fatalf("%s (%s), want violated", o.Status, o.Reason)
	}
	if o.Stats.States != 16 {
		t.Fatalf("%d product states, want the 16 of the construction", o.Stats.States)
	}
}

// TestWeakFairnessNoteNamesTimeoutProcess: the note of a loop found without
// fairness names the processes that are enabled throughout it and never move;
// a process waiting for a timeout is enabled in every timeout state.
func TestWeakFairnessNoteNamesTimeoutProcess(t *testing.T) {
	o := weakfairOutcome(t, "decision:d3_one.pml", "never", "none")
	if o.Status != Violated || !strings.Contains(o.Reason, "Q:1 is enabled throughout the loop and never moves") {
		t.Fatalf("%s: %q, want the note that names Q:1", o.Status, o.Reason)
	}
}

// TestWeakFairnessClaimElse: `else` in a never claim is enabled only when no
// other edge of its location is. The product search enumerated the edges of the
// claim by their guards and an `else` has none, so it was always enabled: a
// claim that had to keep looping could fall off its closing brace at once ("end
// state in claim reached", a violation that is no run; pan finds no error).
func TestWeakFairnessClaimElse(t *testing.T) {
	for _, c := range []struct {
		file string
		want Status
	}{{"claim-else-idle.pml", Verified}, {"claim-else-taken.pml", Violated}} {
		for _, fairness := range []string{"none", "weak"} {
			if o := weakfairOutcome(t, c.file, "never", fairness); o.Status != c.want {
				t.Errorf("%s under %s: %s (%s), want %s", c.file, fairness, o.Status, o.Reason, c.want)
			}
		}
	}
}

// The same rule after an atomic claim edge, which continues at once with the
// first enabled edge of its new location (claimStepFrom): the `else` is written
// first here, and must be taken only if the other edge is not enabled. x stays
// 0, so the claim keeps looping and never reaches its end.
func TestWeakFairnessClaimElseAfterAtomicEdge(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "claim-else-atomic", Globals: []ir.Var{{Name: "x", Type: ir.Bit}},
		Properties: []ir.Property{{ID: "never", Kind: ir.KindLTL}}}
	m.Processes = []ir.Process{
		{Name: "P", Locations: []ir.Location{{}}, Edges: []ir.Edge{{From: 0, To: 0, Effect: []ir.Assign{{Var: "x", Value: ir.Const(0)}}, Text: "x = 0"}}},
		{Name: "never", Claim: true,
			Locations: []ir.Location{{Name: "S0"}, {Name: "S1"}, {Name: "-end-", Labels: []ir.Label{ir.End}}},
			Edges: []ir.Edge{
				{From: 0, To: 1, Atomic: true, Text: "true"},
				{From: 1, To: 2, Else: true, Text: "else"},
				{From: 1, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(0)), Text: "x == 0"},
			}},
	}
	for _, fairness := range []string{"none", "weak"} {
		res, err := Run(context.Background(), m, Options{Sweep: true, Fairness: fairness})
		if err != nil {
			t.Fatal(err)
		}
		if o := outcomeOf(t, res, "never"); o.Status != Verified {
			t.Errorf("%s: %s (%s), want verified: x == 0 holds for ever, so `else` is never enabled", fairness, o.Status, o.Reason)
		}
	}
}
