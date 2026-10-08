package explore

// Atomic sequences under the reduction (performance plan, step 6, section 3).
// The traps of section 3.6, each a hand-built model. Every trap is run in every
// order of its processes: pick takes the first eligible process by index, and
// the order can hide a missing conflict.
//
// In most of them the bad run (a failed assert, a state a property asks for)
// exists only when another process moves BEFORE the atomic sequence is made, so
// an analysis that expands the sequence's process alone first never sees it.
// Each trap is also given to the audit as the plan a broken analysis would make
// (TestPORAuditSeesTheAtomicTraps): that is what shows the trap is one.

import (
	"strings"
	"testing"

	"modelcheck/ir"
)

func eqv(name string, v int64) *ir.Expr { return ir.Binary("eq", ir.Ref(name), ir.Const(v)) }

// permutations are the orders of n processes (all of them: n is at most 4).
func permutations(n int) [][]int {
	if n <= 1 {
		return [][]int{{0}}
	}
	var out [][]int
	for _, rest := range permutations(n - 1) {
		for at := 0; at < n; at++ {
			p := append(append(append([]int(nil), rest[:at]...), n-1), rest[at:]...)
			out = append(out, p)
		}
	}
	return out
}

// everyOrder runs fn on m with its processes in every order.
func everyOrder(t *testing.T, m *ir.Model, fn func(t *testing.T, m *ir.Model)) {
	t.Helper()
	for _, order := range permutations(len(m.Processes)) {
		name := ""
		for _, p := range order {
			name += m.Processes[p].Name
		}
		t.Run(name, func(t *testing.T) { fn(t, reordered(t, m, order...)) })
	}
}

// reducedTrap checks that the reduction is applied to m in every order of its
// processes, that the full search says what the model is supposed to say, and
// that every property has the same status with the reduction.
func reducedTrap(t *testing.T, m *ir.Model, want map[string]Status) {
	t.Helper()
	everyOrder(t, m, func(t *testing.T, m *ir.Model) {
		full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
		if !red.Reduction.Applied {
			t.Fatalf("the reduction is not applied: %s", red.Reduction.Reason)
		}
		for id, st := range want {
			if got := outcome(t, full, id).Status; got != st {
				t.Fatalf("full search: %s is %s, the trap needs %s", id, got, st)
			}
		}
		sameStatuses(t, "atomic trap", red, full)
	})
}

// A sequence whose continuation reads a variable that another process writes:
// the first edge is isolated, the continuation is not. After the first edge P
// goes on into whichever of c1 (x == 1) and c2 (x == 0) is enabled, so what
// the sequence does depends on whether Q has written x. The assert is reached
// only if Q goes first.
func atomicGuardTrap() *ir.Model {
	return model([]ir.Var{byteVar("x")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("P", nil, 4,
			ir.Edge{From: 0, To: 1, Atomic: true, Text: "enter"},
			ir.Edge{From: 1, To: 2, Guard: eqv("x", 1), Text: "x == 1"},
			ir.Edge{From: 1, To: 3, Guard: eqv("x", 0), Text: "x == 0"},
			ir.Edge{From: 2, To: 3, Assert: ir.Const(0), Text: "assert(false)"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}))
}

// The same with an else in place of the second guard: whether the else edge is
// enabled depends on its sibling, which reads x.
func atomicElseTrap() *ir.Model {
	m := atomicGuardTrap()
	m.Processes[0].Edges[2] = ir.Edge{From: 1, To: 3, Else: true, Text: "else"}
	return m
}

// A chain of three atomic edges: the guard is two hops from the first edge.
func atomicLongChainTrap() *ir.Model {
	return model([]ir.Var{byteVar("x")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("P", nil, 5,
			ir.Edge{From: 0, To: 1, Atomic: true, Text: "step 1"},
			ir.Edge{From: 1, To: 2, Atomic: true, Text: "step 2"},
			ir.Edge{From: 2, To: 3, Guard: eqv("x", 1), Text: "x == 1"},
			ir.Edge{From: 2, To: 4, Guard: eqv("x", 0), Text: "x == 0"},
			ir.Edge{From: 3, To: 4, Assert: ir.Const(0), Text: "assert(false)"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}))
}

// An edge that is both atomic and d_step: the d_step goes on into the first
// enabled edge of its target, and the sequence goes on after that.
func atomicDStepTrap() *ir.Model {
	m := atomicGuardTrap()
	m.Processes[0].Edges[0].DStep = true
	m.Processes[0].Edges[1].Atomic = false
	return m
}

// A d_step that ends in an atomic edge: the exclusive byte is the one the last
// edge of the d_step leaves, so a d_step edge that is not atomic starts an
// atomic sequence. The closure of the first edge crosses from the d_step
// mechanism to the atomic one.
func dstepIntoAtomicTrap() *ir.Model {
	return model([]ir.Var{byteVar("x")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("P", nil, 6,
			ir.Edge{From: 0, To: 1, DStep: true, Text: "d_step"},
			ir.Edge{From: 1, To: 2, Atomic: true, Text: "atomic"},
			ir.Edge{From: 2, To: 3, Guard: eqv("x", 1), Text: "x == 1"},
			ir.Edge{From: 2, To: 4, Guard: eqv("x", 0), Text: "x == 0"},
			ir.Edge{From: 3, To: 4, Assert: ir.Const(0), Text: "assert(false)"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}))
}

func TestPORAnAtomicContinuationThatReadsAVariableIsNotExpandedAlone(t *testing.T) {
	for name, m := range map[string]*ir.Model{
		"guard":                      atomicGuardTrap(),
		"else":                       atomicElseTrap(),
		"three hops":                 atomicLongChainTrap(),
		"atomic and d_step":          atomicDStepTrap(),
		"d_step that ends in atomic": dstepIntoAtomicTrap(),
	} {
		t.Run(name, func(t *testing.T) { reducedTrap(t, m, map[string]Status{"assert": Violated}) })
	}
}

// A continuation guarded by another process's program counter, which that
// process enters. The sequence's choice at the continuation depends on whether
// Q has moved: with Q at 1 it may take c1 as well as c2, so Q moving first
// adds a branch that P-first does not have.
func atomicPCTrap() *ir.Model {
	return model(nil, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("P", nil, 4,
			ir.Edge{From: 0, To: 1, Atomic: true, Text: "enter"},
			ir.Edge{From: 1, To: 2, Guard: ir.Binary("eq", ir.PC(1), ir.Const(1)), Text: "pc(Q) == 1"},
			ir.Edge{From: 1, To: 3, Text: "otherwise"},
			ir.Edge{From: 2, To: 3, Assert: ir.Const(0), Text: "assert(false)"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1}))
}

func TestPORAnAtomicContinuationGuardedByAProgramCounterIsNotExpandedAlone(t *testing.T) {
	reducedTrap(t, atomicPCTrap(), map[string]Status{"assert": Violated})
}

// A continuation that is a send on a channel whose receiver pops. P fills the
// channel, then enters a sequence whose continuation sends again if there is
// room (and then asserts false) or, with no room, goes elsewhere. Q receives.
// The send is enabled by Q's pop, so P is not independent of Q at the entry, and
// the directed ends of the channel (a send against a receive) must not be
// applied to an edge a sequence goes on into.
func atomicSendTrap() *ir.Model {
	return model(nil, []ir.Channel{chanDecl("c", 1)}, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("P", nil, 5,
			sendEdge(0, 1, "c", 1),
			ir.Edge{From: 1, To: 2, Atomic: true, Text: "enter"},
			sendEdge(2, 3, "c", 2),
			ir.Edge{From: 2, To: 4, Else: true, Text: "else"},
			ir.Edge{From: 3, To: 4, Assert: ir.Const(0), Text: "assert(false)"}),
		proc("Q", nil, 2, recvEdge(0, 1, "c")))
}

func TestPORAnAtomicContinuationThatSendsOnAChannelKeepsTheChannelWhole(t *testing.T) {
	reducedTrap(t, atomicSendTrap(), map[string]Status{"assert": Violated})
}

// A write in the middle of a sequence that a property reads. P's sequence writes
// x at its continuation; Q writes y; the property asks for the state "x old, y
// new", which is reached only if Q moves before P's sequence is made. The write
// must count for visibility although it is not the first edge's.
func atomicVisibleTrap() *ir.Model {
	return model([]ir.Var{byteVar("x"), byteVar("y")}, nil,
		[]ir.Property{{ID: "reach", Kind: ir.KindReach, Expr: ir.And(eqv("x", 0), eqv("y", 1))}},
		proc("P", nil, 3,
			ir.Edge{From: 0, To: 1, Atomic: true, Text: "enter"},
			ir.Edge{From: 1, To: 2, Effect: []ir.Assign{set("x", 1)}, Text: "x = 1"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("y", 1)}}))
}

func TestPORAWriteInTheMiddleOfASequenceThatAPropertyReadsIsVisible(t *testing.T) {
	reducedTrap(t, atomicVisibleTrap(), map[string]Status{"reach": Verified})
}

// An atomic edge into a sink: the target has no edge, so the holder is blocked
// after the edge, exclusive control is lost and the byte stays set. The Promela
// frontend cannot emit it (an atomic edge always has a successor statement), so
// no generator makes it; this is direct IR. Q waits for "x written, y not yet",
// which is reached only if P goes before R, and then asserts.
func atomicIntoASink() *ir.Model {
	return model([]ir.Var{byteVar("x"), byteVar("y")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("P", nil, 2, ir.Edge{From: 0, To: 1, Atomic: true, Effect: []ir.Assign{set("x", 1)}, Text: "x = 1"}),
		proc("R", nil, 2, ir.Edge{From: 0, To: 1, Atomic: true, Effect: []ir.Assign{set("y", 1)}, Text: "y = 1"}),
		proc("Q", nil, 3,
			ir.Edge{From: 0, To: 1, Guard: ir.And(eqv("x", 1), eqv("y", 0)), Text: "x == 1 && y == 0"},
			ir.Edge{From: 1, To: 2, Assert: ir.Const(0), Text: "assert(false)"}))
}

func TestPORAnAtomicEdgeIntoASinkKeepsTheVerdict(t *testing.T) {
	reducedTrap(t, atomicIntoASink(), map[string]Status{"assert": Violated})
}

// Two sequences that block inside: the holder is blocked after its first edge,
// so exclusive control is lost but the byte stays set, and the state is stored
// with it. Reached by P then R and by R then P, the two orders leave different
// bytes in the same state. Both stuck: a deadlock.
func twoBlockedSequences() *ir.Model {
	blocked := func(name string) ir.Process {
		return proc(name, nil, 3,
			ir.Edge{From: 0, To: 1, Atomic: true, Text: "enter"},
			ir.Edge{From: 1, To: 2, Guard: eqv("z", 7), Text: "z == 7"})
	}
	return model([]ir.Var{byteVar("z")}, nil, nil, blocked("P"), blocked("R"))
}

func TestPORTwoBlockedSequencesAreTheSameStateUpToTheExclusiveByte(t *testing.T) {
	m := twoBlockedSequences()
	reducedTrap(t, m, map[string]Status{"deadlock": Violated})
	// 5 states in full (the two stuck states differ only in the byte), 3 reduced.
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if full.States != 5 || red.States != 3 {
		t.Fatalf("%d states in full, %d reduced; want 5 and 3", full.States, red.States)
	}
	// The set of states without a move is the same up to the byte and not
	// exactly: that is why the oracle compares it up to the byte.
	var fullRec, redRec *recorder
	opt := Options{Sweep: true, NewVisited: func(n int) Visited { fullRec = &recorder{Visited: defaultVisited(n)}; return fullRec }}
	run(t, m, opt)
	opt.POR, opt.NewVisited = true, func(n int) Visited { redRec = &recorder{Visited: defaultVisited(n)}; return redRec }
	run(t, m, opt)
	exact := func(states [][]byte) map[string]bool { out, _ := noMoveStates(m, states); return out }
	fe, re := exact(fullRec.states), exact(redRec.states)
	if len(fe) != 2 || len(re) != 1 {
		t.Fatalf("states without a move: %d in full, %d reduced; want 2 and 1", len(fe), len(re))
	}
	st, _ := NewStepper(m)
	canon := func(set map[string]bool) map[string]bool {
		out := map[string]bool{}
		for k := range set {
			b := []byte(k)
			b[st.Layout().Excl] = 0
			out[string(b)] = true
		}
		return out
	}
	cf, cr := canon(fe), canon(re)
	if len(cf) != 1 || len(cr) != 1 {
		t.Fatalf("up to the exclusive byte there is one such state in each: %d and %d", len(cf), len(cr))
	}
	for k := range cr {
		if !cf[k] {
			t.Fatal("the reduced run's state without a move is not the full run's up to the exclusive byte")
		}
	}
}

// At a stored state whose holder is blocked every process may move, and the
// reduction expands one of them alone: P blocks inside its sequence for ever,
// R and S count. The state with the byte set is expanded through an ample set.
func blockedHolderWithWorkers() *ir.Model {
	m := model([]ir.Var{byteVar("z")}, nil, nil,
		proc("P", nil, 3,
			ir.Edge{From: 0, To: 1, Atomic: true, Text: "enter"},
			ir.Edge{From: 1, To: 2, Guard: eqv("z", 7), Text: "z == 7"}))
	for _, n := range []string{"R", "S"} {
		m.Processes = append(m.Processes, proc(n, []ir.Var{byteVar("c")}, 1,
			ir.Edge{From: 0, To: 0, Guard: ir.Binary("lt", ir.Ref("c"), ir.Const(2)),
				Effect: []ir.Assign{{Var: "c", Value: ir.Binary("add", ir.Ref("c"), ir.Const(1))}}, Text: "c++"}))
	}
	return m
}

func TestPORAStoredStateWhoseHolderIsBlockedIsExpandedThroughAnAmpleSet(t *testing.T) {
	m := blockedHolderWithWorkers()
	reducedTrap(t, m, map[string]Status{"deadlock": Violated})
	picked := 0
	var excl int
	opt := Options{Sweep: true, POR: true, porTrace: func(state []byte, ample uint8) {
		if state[excl] != 0 && ample != 0 {
			picked++
		}
	}}
	st, err := NewStepper(m)
	if err != nil {
		t.Fatal(err)
	}
	excl = st.Layout().Excl
	red := run(t, m, opt)
	full := run(t, m, Options{Sweep: true})
	if picked == 0 {
		t.Fatalf("no stored state with the exclusive byte set was expanded through an ample set (reduced %d states, full %d)", red.States, full.States)
	}
	if red.States*2 > full.States {
		t.Fatalf("%d states reduced against %d in full: the workers were not reduced", red.States, full.States)
	}
}

// A process that loops through an atomic chain back to its own start: the
// macro-step ends in the state it started from, which is on the stack. Q's
// assert is reached only if Q is ever expanded, which a proviso that does not
// see the end of the chain forgets: P is expanded alone for ever.
func cycleThroughAChain(branches int) *ir.Model {
	p := proc("P", nil, 4, ir.Edge{From: 0, To: 1, Atomic: true, Text: "enter"})
	// the branches of the chain: the first leaves the cycle, the last closes it
	for i := 0; i < branches-1; i++ {
		p.Edges = append(p.Edges, ir.Edge{From: 1, To: 2, Text: "leave"})
	}
	p.Edges = append(p.Edges, ir.Edge{From: 1, To: 0, Text: "again"})
	q := proc("Q", nil, 3,
		ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}},
		ir.Edge{From: 1, To: 2, Assert: ir.Const(0), Text: "assert(false)"})
	return model([]ir.Var{byteVar("x")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}}, p, q)
}

func TestPORACycleThroughAnAtomicChainIsExpandedInFull(t *testing.T) {
	reducedTrap(t, cycleThroughAChain(1), map[string]Status{"assert": Violated})
	reducedTrap(t, cycleThroughAChain(2), map[string]Status{"assert": Violated})
}

// The cycle of the reduced graph that only some branches of a chain close must
// be seen by the proviso as well: no cycle of states that were not expanded in
// full, in every order, and with the branch that leaves the cycle first.
func TestPORTheProvisoLooksAtEveryBranchOfAChain(t *testing.T) {
	for _, branches := range []int{2, 3} {
		everyOrder(t, cycleThroughAChain(branches), func(t *testing.T, m *ir.Model) {
			checked, err := acyclic(m, false)
			if err != nil {
				t.Fatal(err)
			}
			if !checked {
				t.Fatal("the reduction was not applied to the model, so nothing was checked")
			}
		})
	}
}

// The proviso has to look at every move of the ample process, not only the
// first: P's first move leaves for a state that is not on the stack, its second
// goes back to the state it is expanded from. No verdict depends on it (the
// search reaches a fully expanded state through the first), but a cycle of
// states that were not expanded in full is what C3 forbids.
func TestPORTheProvisoLooksAtEveryMoveOfTheAmpleProcess(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("P", nil, 3, ir.Edge{From: 0, To: 2, Text: "leave"}, ir.Edge{From: 0, To: 0, Text: "again"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}))
	everyOrder(t, m, func(t *testing.T, m *ir.Model) {
		checked, err := acyclic(m, false)
		if err != nil || !checked {
			t.Fatalf("checked %v, %v", checked, err)
		}
	})
}

// ---- the limits of the chain walk ---------------------------------------------------

// aChainThatNeverEnds: a d_step into an atomic edge that goes back to the start
// of the d_step, so the macro-step does not end. It is the only process, so a
// full expansion is what the limit must give, and the chain is eligible.
func aChainThatNeverEnds() *ir.Model {
	return model(nil, nil, nil,
		proc("P", nil, 3,
			ir.Edge{From: 0, To: 1, DStep: true, Text: "d_step"},
			ir.Edge{From: 1, To: 0, Atomic: true, Text: "atomic, back"}))
}

// pickAt returns what the reduction chooses at the initial state of m, with the
// limits of the chain walk given.
func pickAt(t *testing.T, m *ir.Model, chainLimit, pickBudget int) uint8 {
	t.Helper()
	return pickPlan(t, m, func(c *compiled) *porPlan {
		plan := analyzePOR(c)
		if plan.reason != "" {
			t.Fatalf("refused: %s", plan.reason)
		}
		return plan
	}, chainLimit, pickBudget)
}

// pickPlan is pickAt with the plan given: the analysis proper, or a plan that a
// broken analysis would make (forcedPlan), to look at the search side alone.
func pickPlan(t *testing.T, m *ir.Model, mk func(*compiled) *porPlan, chainLimit, pickBudget int) uint8 {
	t.Helper()
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	plan := mk(c)
	s := &search{c: c, cur: c.layout.Initial(), next: make([]byte, c.layout.Size), visited: defaultVisited(c.layout.Size)}
	idx, _ := s.visited.Add(s.cur)
	r := &porRun{plan: plan, chainLimit: chainLimit, pickBudget: pickBudget}
	r.mark(idx, true)
	return r.pick(s)
}

func TestPORAChainThatNeverEndsIsExpandedInFullNotFollowedForEver(t *testing.T) {
	if got := pickAt(t, aChainThatNeverEnds(), 50, 0); got != 0 {
		t.Fatalf("picked process %d, want a full expansion", got-1)
	}
	// With the budget of one pick below the limit of the chain, the budget
	// answers first.
	if got := pickAt(t, aChainThatNeverEnds(), 0, 30); got != 0 {
		t.Fatalf("picked process %d under a budget of 30 micro-steps, want a full expansion", got-1)
	}
}

// Without the limit a chain is followed to its end, and a state whose chains
// leave the stack is reduced; with a budget smaller than the chain the answer is
// a full expansion.
func TestPORTheBudgetOfOnePickFlipsAWideChainToAFullExpansion(t *testing.T) {
	// P enters a sequence at location 1 and goes on, three times over, into one
	// of 4 alternatives: 4^3 = 64 chains.
	p := proc("P", nil, 5, ir.Edge{From: 0, To: 1, Atomic: true})
	for l := 1; l <= 3; l++ {
		for k := 0; k < 4; k++ {
			p.Edges = append(p.Edges, ir.Edge{From: l, To: l + 1, Atomic: l < 3})
		}
	}
	m := model(nil, nil, nil, p)
	if got := pickAt(t, m, 0, 0); got != 1 {
		t.Fatalf("picked %d with the default budget, want process 0", got)
	}
	if got := pickAt(t, m, 0, 10); got != 0 {
		t.Fatalf("picked process %d under a budget of 10 micro-steps, want a full expansion", got-1)
	}
}

// The budget is one for the whole pick, not one per candidate process: the
// proviso walks the moves of every eligible process in turn until one leaves the
// stack, and what the earlier candidates fired counts against the same budget.
// L is eligible first and its only move goes back to the state it is expanded
// from (on the stack), so the walk goes on to P, whose chain costs 85 micro-steps
// (1 + 4 + 16 + 64) by itself. Alone, 85 is enough; behind L, which fires one
// micro-step, it is not, and the state is expanded in full; with 86 it is enough.
func TestPORTheBudgetIsOneForTheWholePickNotOnePerCandidate(t *testing.T) {
	wide := func() ir.Process {
		p := proc("P", nil, 5, ir.Edge{From: 0, To: 1, Atomic: true})
		for l := 1; l <= 3; l++ {
			for k := 0; k < 4; k++ {
				p.Edges = append(p.Edges, ir.Edge{From: l, To: l + 1, Atomic: l < 3})
			}
		}
		return p
	}
	alone := model(nil, nil, nil, wide())
	if got := pickAt(t, alone, 0, 85); got != 1 {
		t.Fatalf("P alone picked %d under a budget of 85, want process 0: the chain costs 85 micro-steps", got)
	}
	if got := pickAt(t, alone, 0, 84); got != 0 {
		t.Fatalf("P alone picked process %d under a budget of 84, want a full expansion", got-1)
	}
	behind := model(nil, nil, nil, proc("L", nil, 1, ir.Edge{From: 0, To: 0, Text: "again"}), wide())
	if got := pickAt(t, behind, 0, 85); got != 0 {
		t.Fatalf("picked process %d behind a candidate that spent a micro-step, under a budget of 85: the budget must be shared, want a full expansion", got-1)
	}
	if got := pickAt(t, behind, 0, 86); got != 2 {
		t.Fatalf("picked %d under a budget of 86, want process 1 (L spends one micro-step, P 85)", got)
	}
}

// The depth budget of the search stops a macro-step that never ends: the run
// answers inconclusive, with the reduction applied, and does not hang.
func TestPORAChainThatNeverEndsStopsOnTheDepthBudget(t *testing.T) {
	m := aChainThatNeverEnds()
	m.Processes = append(m.Processes, proc("Q", []ir.Var{byteVar("c")}, 2, ir.Edge{From: 0, To: 1}))
	for _, order := range permutations(2) {
		mm := reordered(t, m, order...)
		for _, por := range []bool{false, true} {
			r := run(t, mm, Options{POR: por, Sweep: true, Budget: Budget{MaxDepth: 40}})
			if r.Complete || !strings.Contains(r.Stop, "depth budget") {
				t.Fatalf("order %v, por %v: complete %v, stop %q", order, por, r.Complete, r.Stop)
			}
		}
	}
}

// ---- a failing assert inside a chain -----------------------------------------------------

// assertInAChain: P enters an atomic sequence whose continuation is an assert
// (a, going on to location `to`), Q writes x, and an invariant says x is never
// 1. With a = assert(false) and to = 0 this is the model of the double mutant
// of the harness (x10 and a12 together): the failing assert is the only way the
// search leaves P's macro-step, and it leads back to the state the step starts
// from.
func assertInAChain(a *ir.Expr, to int) *ir.Model {
	return model([]ir.Var{byteVar("x")}, nil,
		[]ir.Property{{ID: "assert", Kind: ir.KindAssert}, {ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("ne", ir.Ref("x"), ir.Const(1))}},
		proc("P", nil, 3,
			ir.Edge{From: 0, To: 1, Atomic: true, Text: "atomic"},
			ir.Edge{From: 1, To: to, Assert: a, Text: "assert"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}))
}

// The walk of the cycle proviso counts a failing assert in a chain as a step
// that does not leave the stack, so the state is expanded in full and the
// ordinary search takes the step and reports it. In production the clause is
// out of reach: a location whose closure has an assert is never eligible (the
// `!u.assert` of the analysis), so no failing assert gets to the walk. Each of
// the two clauses is harmless only while the other stands (removed together
// they give a false `verified`, the harness's double mutant), so this test pins
// the walk's own clause: P is made eligible at every location by hand, which
// is what an analysis without `!u.assert` would do, and the search side alone
// has to say "full expansion".
func TestPORAFailingAssertInAChainIsNotALeavingStepWhateverTheAnalysisSays(t *testing.T) {
	forced := func(t *testing.T, m *ir.Model) uint8 {
		p := -1
		for i := range m.Processes {
			if m.Processes[i].Name == "P" {
				p = i
			}
		}
		return pickPlan(t, m, func(*compiled) *porPlan { return forcedPlan(t, m, p) }, 0, 0)
	}
	for _, c := range []struct {
		name string
		m    *ir.Model
		fail bool
	}{
		{"assert(false) back to the start of the step (the double mutant's model)", assertInAChain(ir.Const(0), 0), true},
		{"assert(false) on to a location nobody has seen", assertInAChain(ir.Const(0), 2), true},
		// The control: the same walk with an assert that holds leaves for a state
		// that is not on the stack, so P is picked. Without it the test would pass
		// for any plan that is never used.
		{"control: assert(true) on to a location nobody has seen", assertInAChain(ir.Const(1), 2), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			everyOrder(t, c.m, func(t *testing.T, m *ir.Model) {
				got := forced(t, m)
				if c.fail && got != 0 {
					t.Fatalf("picked process %d, want a full expansion: a failing assert in the chain does not leave the stack", got-1)
				}
				if !c.fail && got == 0 {
					t.Fatal("a full expansion where the chain leaves the stack: the forced plan is not being used")
				}
			})
		})
	}
}

// The verdicts of the model of the double mutant, in every order of the
// processes: the invariant is violated (Q writes x) and so is the assert, with
// and without the reduction. A reduction that expanded P alone would loop on the
// failing assert for ever and never let Q move.
func TestPORAFailingAssertInAChainDoesNotHideTheInvariant(t *testing.T) {
	reducedTrap(t, assertInAChain(ir.Const(0), 0), map[string]Status{"assert": Violated, "inv": Violated, "deadlock": Verified})
}

// ---- the audit sees each of these traps ------------------------------------------------

// Each trap is the plan a broken analysis would make: the first process
// eligible at its first location although the sequence it starts conflicts.
func TestPORAuditSeesTheAtomicTraps(t *testing.T) {
	for name, m := range map[string]*ir.Model{
		"guard":                      atomicGuardTrap(),
		"else":                       atomicElseTrap(),
		"three hops":                 atomicLongChainTrap(),
		"atomic and d_step":          atomicDStepTrap(),
		"d_step that ends in atomic": dstepIntoAtomicTrap(),
		"program counter":            atomicPCTrap(),
		"send on a channel":          atomicSendTrap(),
		"visible write":              atomicVisibleTrap(),
	} {
		t.Run(name, func(t *testing.T) {
			for _, order := range permutations(len(m.Processes)) {
				mm := reordered(t, m, order...)
				p := 0 // the process that starts the sequence is P, wherever it went
				for i := range mm.Processes {
					if mm.Processes[i].Name == "P" {
						p = i
					}
				}
				want := "C1"
				if name == "visible write" {
					want = "C2"
				}
				wantAuditFailure(t, name, mm, want, p)
			}
		})
	}
}
