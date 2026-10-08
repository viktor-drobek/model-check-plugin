package explore

// Process creation and the process table under the reduction (performance plan,
// step 6, section 4). The traps of section 4.4, each a hand-built model run in
// every order of its processes (pick takes the first eligible process by index:
// a creator picked first masks a missing conflict with a process picked later).
//
// The table is one cell, T: `nrpr`, `pid` and `youngest` read it, every `run`
// and every edge that leaves the table write it. A `run` also writes the
// program counter of each target at its dormant location, which is also what
// it reads of the pool, and reads the cells of its arguments and initialisers.

import (
	"strings"
	"testing"

	"modelcheck/ir"
)

// dynProc builds a dynamic process: nloc locations, location 0 dormant. Its
// first params locals are the parameters of a run.
func dynProc(name string, locals []ir.Var, params, nloc int, edges ...ir.Edge) ir.Process {
	return ir.Process{Name: name, Locals: locals, Params: params, Locations: make([]ir.Location, nloc), Dynamic: true, Initial: 0, Edges: edges}
}

// procNamed is the index of the process called name (the orders of a trap move it).
func procNamed(t *testing.T, m *ir.Model, name string) int {
	t.Helper()
	for i := range m.Processes {
		if m.Processes[i].Name == name {
			return i
		}
	}
	t.Fatalf("no process %s", name)
	return -1
}

func runOf(proc int, entry int, args ...*ir.Expr) *ir.RunOp {
	return &ir.RunOp{Proc: proc, Entry: entry, Args: args}
}

// endEdge is the `-end-` edge the frontend gives a process when the table is
// on: guarded by `youngest` and leaving the table.
func endEdge(from, to, proc int) ir.Edge {
	return ir.Edge{From: from, To: to, Guard: ir.Youngest(proc), Leave: true, Text: "-end-"}
}

// A creator S runs D, which has no edge of its own, and R waits for D to start
// (a guard on D's program counter) beside a free edge. The guard is enabled by
// the run and by nothing else, so the free edge expanded alone first never
// reaches the assert. The counter of D at its dormant location must be written
// by the run: there is no other write of it in the model, and no end edge
// anywhere (so the table is never left).
func runEnablesAGuard() *ir.Model {
	return model(nil, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("R", nil, 4,
			ir.Edge{From: 0, To: 1, Text: "free"},
			ir.Edge{From: 0, To: 2, Guard: ir.Binary("eq", ir.PC(2), ir.Const(1)), Text: "pc(D) == 1"},
			ir.Edge{From: 2, To: 3, Assert: ir.Const(0), Text: "assert(false)"}),
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(2, 1), Text: "run D()"}),
		dynProc("D", nil, 0, 2))
}

// The same, with the guard on the number of live processes.
func runEnablesANrPrGuard() *ir.Model {
	m := runEnablesAGuard()
	m.Processes[0].Edges[1].Guard = ir.Binary("eq", ir.NrPr(), ir.Const(3))
	m.Processes[0].Edges[1].Text = "_nr_pr == 3"
	return m
}

// A property that asks for the state "two live processes, y set": it is reached
// only if Q writes y before the run. The run writes the table and a property
// reads it, so the run is visible and not expanded alone.
func nrPrPropertyTrap() *ir.Model {
	return model([]ir.Var{byteVar("y")}, nil,
		[]ir.Property{{ID: "reach", Kind: ir.KindReach, Expr: ir.And(ir.Binary("eq", ir.NrPr(), ir.Const(2)), eqv("y", 1))}},
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(2, 1), Text: "run D()"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("y", 1)}}),
		dynProc("D", nil, 0, 2))
}

// The arguments of a run read a global that Q writes: D's parameter is the
// value at the moment of the run, so the order of Q and S is visible, and D
// asserts that its parameter is 0.
func runArgsTrap() *ir.Model {
	return model([]ir.Var{byteVar("x")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(2, 1, ir.Ref("x")), Text: "run D(x)"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		dynProc("D", []ir.Var{byteVar("a")}, 1, 3,
			ir.Edge{From: 1, To: 2, Assert: eqv("a", 0), Text: "assert(a == 0)"}))
}

// The initialiser of a local of D, run in D's scope, reads the same global.
func runInitTrap() *ir.Model {
	m := runArgsTrap()
	m.Processes[0].Edges[0].Run = &ir.RunOp{Proc: 2, Entry: 1, Init: []ir.Assign{{Var: "l", Value: ir.Ref("x")}}}
	m.Processes[2] = dynProc("D", []ir.Var{byteVar("l")}, 0, 3,
		ir.Edge{From: 1, To: 2, Assert: eqv("l", 0), Text: "assert(l == 0)"})
	return m
}

// `pid p = run D()` is a run whose effect reads the number of live processes
// after the table has been extended. Q is the youngest process and may leave
// the table while nothing younger exists; if it leaves before the run, the run
// is made with one process fewer, S's `p` is 1 and the assert fails.
func runEffectReadsTheTable() *ir.Model {
	return model(nil, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("S", []ir.Var{byteVar("p")}, 3,
			ir.Edge{From: 0, To: 1, Run: runOf(2, 1), Effect: []ir.Assign{{Var: "p", Value: ir.Binary("sub", ir.NrPr(), ir.Const(1))}}, Text: "p = run D()"},
			ir.Edge{From: 1, To: 2, Assert: ir.Binary("ne", ir.Ref("p"), ir.Const(1)), Text: "assert(p != 1)"}),
		proc("Q", nil, 2, endEdge(0, 1, 1)),
		dynProc("D", nil, 0, 2))
}

// The pid of a process that is not live is -1, which does not fit a byte: an
// error of the model that is reachable only if P looks before D has started.
func pidOfADormantProcess() *ir.Model {
	return model(nil, nil, nil,
		proc("P", []ir.Var{byteVar("l")}, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{{Var: "l", Value: ir.PID(2)}}, Text: "l = pid(D)"}),
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(2, 1), Text: "run D()"}),
		dynProc("D", nil, 0, 2))
}

// A creator that runs D twice: the second run exhausts the pool of one unless D
// has ended and returned to its dormant location in between. With the guard on
// D's counter the second run waits for that.
func runTwice(waits bool) *ir.Model {
	second := ir.Edge{From: 1, To: 2, Run: runOf(1, 1), Text: "run D()"}
	if waits {
		second.Guard = ir.Binary("eq", ir.PC(1), ir.Const(0))
	}
	return model(nil, nil, nil,
		proc("S", nil, 4, ir.Edge{From: 0, To: 1, Run: runOf(1, 1), Text: "run D()"}, second, endEdge(2, 3, 0)),
		dynProc("D", nil, 0, 3,
			ir.Edge{From: 1, To: 2, Text: "work"},
			ir.Edge{From: 2, To: 0, Guard: ir.Youngest(1), Leave: true, Text: "-end-"}))
}

// A run in a loop with a pool of two: the third run exhausts it, an error of the
// bound that both searches reach.
func runInALoop() *ir.Model {
	m := model(nil, nil, nil,
		proc("S", nil, 1, ir.Edge{From: 0, To: 0, Run: &ir.RunOp{Proc: 1, Pool: []int{1, 2}, Entry: 1}, Text: "run D()"}),
		dynProc("D1", nil, 0, 2), dynProc("D2", nil, 0, 2))
	return m
}

// S runs A, A runs B, and B asserts something about x that Q changes: a created
// process that itself creates one.
func nestedCreation() *ir.Model {
	return model([]ir.Var{byteVar("x")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(2, 1), Text: "run A()"}),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		dynProc("A", nil, 0, 3, ir.Edge{From: 1, To: 2, Run: runOf(3, 1), Text: "run B()"}),
		dynProc("B", nil, 0, 3, ir.Edge{From: 1, To: 2, Assert: eqv("x", 0), Text: "assert(x == 0)"}))
}

// A run whose arguments read the channel the same edge clears and the creator's
// own program counter (direct IR: apply moves the counter, leaves the table and
// clears before it evaluates them).
func runArgsReadWhatTheEdgeChanges() *ir.Model {
	return model(nil, []ir.Channel{chanDecl("c", 2)}, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, ClearChans: []int{0},
			Run: runOf(2, 1, ir.Len("c"), ir.PC(0)), Text: "run D(len(c), pc(S))"}),
		proc("Q", nil, 3, sendEdge(0, 1, "c", 1), sendEdge(1, 2, "c", 2)),
		dynProc("D", []ir.Var{byteVar("a"), byteVar("b")}, 2, 3,
			ir.Edge{From: 1, To: 2, Assert: ir.And(eqv("a", 0), eqv("b", 1)), Text: "assert(a == 0 && b == 1)"}))
}

// An initialiser that assigns a GLOBAL (direct IR: the frontend's initialisers
// assign locals of the new process, which are not cells, and `mcd check --ir` and
// `mc_check` with an inline `ir` accept the shape). W waits for g == 1 beside a
// free edge; the only writer of g is the initialiser of S's `run`. The assert is
// reached only if the run is made before W takes the free edge. If the analysis
// forgets that the initialiser writes g, W reads a cell nobody writes and is
// expanded alone first, the free edge is the only move, and the search answers
// `verified` for a model that violates its assert. (The first review of this
// step found the mutant that drops the write: the harness had called it
// equivalent, because no generator and no frontend model assigns a global.)
func runInitAssignsAGlobal() *ir.Model {
	return model([]ir.Var{byteVar("g")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("W", nil, 4,
			ir.Edge{From: 0, To: 1, Guard: eqv("g", 1), Text: "g == 1"},
			ir.Edge{From: 0, To: 2, Text: "free"},
			ir.Edge{From: 1, To: 3, Assert: ir.Const(0), Text: "assert(false)"}),
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: &ir.RunOp{Proc: 2, Entry: 1, Init: []ir.Assign{set("g", 1)}}, Text: "run D() with g = 1"}),
		dynProc("D", nil, 0, 2))
}

// A HETEROGENEOUS pool: S runs the pool {D1, D2} twice (the first run takes D1,
// the second D2, the first instance that is still dormant), and the instances
// are not copies of one proctype. Direct IR only: the frontend's pools are the
// instances of one proctype. The footprint of the run is the union over every
// target of the pool, each read in its own scope. Here the initialiser `l = 1`
// assigns a local of D1 (not a cell) and a GLOBAL for D2, which has no local
// called l, so the second run writes l and the first does not. W waits for l == 1
// beside a free edge. An analysis that evaluates the initialisers in the scope of
// the first target only expands W alone first and answers `verified`.
func heterogeneousPoolInitialiser() *ir.Model {
	pool := func() *ir.RunOp {
		return &ir.RunOp{Proc: 2, Pool: []int{2, 3}, Entry: 1, Init: []ir.Assign{set("l", 1)}}
	}
	return model([]ir.Var{byteVar("l")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("W", nil, 4,
			ir.Edge{From: 0, To: 1, Guard: eqv("l", 1), Text: "l == 1"},
			ir.Edge{From: 0, To: 2, Text: "free"},
			ir.Edge{From: 1, To: 3, Assert: ir.Const(0), Text: "assert(false)"}),
		proc("S", nil, 3,
			ir.Edge{From: 0, To: 1, Run: pool(), Text: "run D()"},
			ir.Edge{From: 1, To: 2, Run: pool(), Text: "run D()"}),
		dynProc("D1", []ir.Var{byteVar("l")}, 0, 2),
		dynProc("D2", nil, 0, 2))
}

// A heterogeneous pool whose instances have bodies of their own: D2 writes y = 2
// only after X has written x, and only once S has made its second run (D1 is
// taken by the first). W waits for y == 2. The assert is reachable, and only by
// running every step of S, X and D2 in a particular order.
func heterogeneousPoolBodies() *ir.Model {
	pool := func() *ir.RunOp { return &ir.RunOp{Proc: 1, Pool: []int{1, 2}, Entry: 1} }
	return model([]ir.Var{byteVar("x"), byteVar("y")}, nil, []ir.Property{{ID: "assert", Kind: ir.KindAssert}},
		proc("S", nil, 3,
			ir.Edge{From: 0, To: 1, Run: pool(), Text: "run D()"},
			ir.Edge{From: 1, To: 2, Run: pool(), Text: "run D()"}),
		dynProc("D1", nil, 0, 3,
			ir.Edge{From: 1, To: 2, Effect: []ir.Assign{set("y", 1)}, Text: "y = 1"},
			endEdge(2, 0, 1)),
		dynProc("D2", nil, 0, 3,
			ir.Edge{From: 1, To: 2, Guard: eqv("x", 1), Effect: []ir.Assign{set("y", 2)}, Text: "x == 1 -> y = 2"},
			endEdge(2, 0, 2)),
		proc("X", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}, Text: "x = 1"}),
		proc("W", nil, 3,
			ir.Edge{From: 0, To: 1, Guard: eqv("y", 2), Text: "y == 2"},
			ir.Edge{From: 1, To: 2, Assert: ir.Const(0), Text: "assert(false)"}))
}

func TestPORACreatorWhoseRunEnablesAGuardKeepsItsAlternative(t *testing.T) {
	reducedTrap(t, runEnablesAGuard(), map[string]Status{"assert": Violated})
}

func TestPORAGuardOnTheNumberOfLiveProcessesIsEnabledByTheRun(t *testing.T) {
	reducedTrap(t, runEnablesANrPrGuard(), map[string]Status{"assert": Violated})
}

func TestPORARunIsVisibleToAPropertyThatReadsTheTable(t *testing.T) {
	reducedTrap(t, nrPrPropertyTrap(), map[string]Status{"reach": Verified})
}

func TestPORTheArgumentsAndInitialisersOfARunAreReads(t *testing.T) {
	t.Run("arguments", func(t *testing.T) { reducedTrap(t, runArgsTrap(), map[string]Status{"assert": Violated}) })
	t.Run("initialisers", func(t *testing.T) { reducedTrap(t, runInitTrap(), map[string]Status{"assert": Violated}) })
}

// The youngest process is the last one the table was given, which is the static
// process of the highest index, so Q may leave first only where it comes after S:
// the orders in which S is before Q (D, the dynamic one, is anywhere).
func TestPORAnEffectOfARunThatReadsTheTableSeesTheTableAfterTheRun(t *testing.T) {
	m := runEffectReadsTheTable()
	for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {2, 0, 1}} {
		mm := reordered(t, m, order...)
		full, red := runPOR(t, mm, false, DFS), runPOR(t, mm, true, DFS)
		if !red.Reduction.Applied {
			t.Fatalf("order %v: not applied: %s", order, red.Reduction.Reason)
		}
		if got := outcome(t, full, "assert").Status; got != Violated {
			t.Fatalf("order %v: full search: assert is %s, the trap needs violated", order, got)
		}
		sameStatuses(t, "effect", red, full)
	}
}

func TestPORArgumentsThatReadWhatTheSameEdgeChangesAreNotAnOrderDependence(t *testing.T) {
	reducedTrap(t, runArgsReadWhatTheEdgeChanges(), map[string]Status{"assert": Verified})
}

func TestPORANestedCreationKeepsTheVerdict(t *testing.T) {
	reducedTrap(t, nestedCreation(), map[string]Status{"assert": Violated})
}

// The pid of a process that is not live is an error of the model: both searches
// reach it, in every order.
func TestPORAPidOfADormantProcessIsAReachableErrorInBothSearches(t *testing.T) {
	reducedTrap(t, pidOfADormantProcess(), map[string]Status{"deadlock": InvalidModel})
}

// A pool of one run twice: with no wait the second run finds D alive (the pool
// is exhausted, an error of the bound that stops both searches); with the wait it
// does not, and the model is clean.
func TestPORAPoolOfOneThatIsRunTwice(t *testing.T) {
	t.Run("no wait", func(t *testing.T) { reducedTrap(t, runTwice(false), map[string]Status{"deadlock": Inconclusive}) })
	t.Run("waits for the end", func(t *testing.T) { reducedTrap(t, runTwice(true), map[string]Status{"deadlock": Verified}) })
}

func TestPORARunInALoopExhaustsItsPoolInBothSearches(t *testing.T) {
	reducedTrap(t, runInALoop(), map[string]Status{"deadlock": Inconclusive})
}

// An initialiser that assigns a global is a write of it, in every order of the
// processes (W picked before S masks nothing: it is the order in which a missing
// write is a false `verified`). The plan must say the same: W is not expanded
// alone at its first location, because S's run writes what W's guard reads.
func TestPORAnInitialiserThatAssignsAGlobalIsAWriteOfIt(t *testing.T) {
	m := runInitAssignsAGlobal()
	reducedTrap(t, m, map[string]Status{"assert": Violated})
	for _, order := range permutations(len(m.Processes)) {
		mm := reordered(t, m, order...)
		wantEligible(t, planOf(t, mm), procNamed(t, mm, "W"), 0, false)
	}
}

// The footprint of a run is the union over every target of its pool, each read
// in its own scope: a pool whose instances differ (direct IR only).
func TestPORAHeterogeneousPoolIsTheUnionOverItsTargets(t *testing.T) {
	t.Run("the initialiser is a global for one target only", func(t *testing.T) {
		m := heterogeneousPoolInitialiser()
		reducedTrap(t, m, map[string]Status{"assert": Violated})
		for _, order := range permutations(len(m.Processes)) {
			mm := reordered(t, m, order...)
			wantEligible(t, planOf(t, mm), procNamed(t, mm, "W"), 0, false)
		}
	})
	t.Run("the instances have bodies of their own", func(t *testing.T) {
		reducedTrap(t, heterogeneousPoolBodies(), map[string]Status{"assert": Violated})
	})
}

// ---- what the analysis says ---------------------------------------------------------

// Without an edge that leaves the table the creating step is expanded alone: its
// write of the table conflicts with nothing. (Frontend output always has end
// edges when it creates a process, so a run is then never expanded alone.)
func TestPORARunIsEligibleWhenNothingElseTouchesTheTable(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(2, 1)}),
		proc("W", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		dynProc("D", nil, 0, 2))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, true)
	wantEligible(t, pl, 1, 0, true)
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if !red.Reduction.Applied || red.States >= full.States {
		t.Fatalf("applied %v, %d states against %d", red.Reduction.Applied, red.States, full.States)
	}
	sameStatuses(t, "run", red, full)
}

func TestPORARunIsNotEligibleWhereAnEdgeLeavesTheTable(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("S", nil, 3, ir.Edge{From: 0, To: 1, Run: runOf(2, 1)}, endEdge(1, 2, 0)),
		proc("W", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
		dynProc("D", nil, 0, 3, endEdge(2, 0, 2)))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false) // the run writes the table, the end edges read and write it
	wantEligible(t, pl, 1, 0, true)  // W touches nothing else
}

func TestPORTheTableCellIsReadByEveryOperatorThatAsksAboutIt(t *testing.T) {
	for name, expr := range map[string]*ir.Expr{
		"nrpr":     ir.Binary("eq", ir.NrPr(), ir.Const(2)),
		"pid":      ir.Binary("eq", ir.PID(1), ir.Const(1)),
		"youngest": ir.Youngest(1),
	} {
		t.Run(name, func(t *testing.T) {
			// R reads the table; S writes it (a run). Neither may be expanded
			// alone while the other is there.
			m := model(nil, nil, nil,
				proc("R", nil, 2, ir.Edge{From: 0, To: 1, Guard: expr}),
				proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(2, 1)}),
				dynProc("D", nil, 0, 2))
			pl := planOf(t, m)
			wantEligible(t, pl, 0, 0, false)
			wantEligible(t, pl, 1, 0, false)
		})
	}
}

func TestPORAPropertyThatReadsTheTableMakesTheRunVisible(t *testing.T) {
	m := model(nil, nil, []ir.Property{{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("le", ir.NrPr(), ir.Const(5))}},
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(2, 1)}),
		proc("W", []ir.Var{byteVar("c")}, 2, ir.Edge{From: 0, To: 1}),
		dynProc("D", nil, 0, 2))
	pl := planOf(t, m)
	wantEligible(t, pl, 0, 0, false)
	wantEligible(t, pl, 1, 0, true)
}

// ---- the two checks that refuse ------------------------------------------------------

// porDormantReentry is testdata/ir/por-dormant.json (the G7 feature): a dynamic
// process whose end goes back to its dormant location without leaving the table.
// The Promela frontend never emits it.
func porDormantReentry() *ir.Model {
	return model(nil, nil, nil,
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(1, 1), Text: "run D()"}),
		dynProc("D", nil, 0, 3,
			ir.Edge{From: 1, To: 2, Text: "work"},
			ir.Edge{From: 2, To: 0, Text: "back to dormant, still in the table"}))
}

func TestPORADynamicProcessThatReentersItsDormantLocationWithoutLeavingIsRefused(t *testing.T) {
	m := porDormantReentry()
	wantReason(t, planOf(t, m), "dormant")
	m.Processes[1].Edges[1].Leave = true
	wantReason(t, planOf(t, m), "")
}

func TestPORARunThatEntersItsTargetAtTheDormantLocationIsRefused(t *testing.T) {
	m := model(nil, nil, nil,
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(1, 0)}),
		dynProc("D", nil, 0, 2, ir.Edge{From: 1, To: 1}))
	wantReason(t, planOf(t, m), "dormant")
	m.Processes[0].Edges[0].Run.Entry = 1
	wantReason(t, planOf(t, m), "")
}

// The refused models run in full.
func TestPORARefusedRunModelIsTheFullSearch(t *testing.T) {
	m := model(nil, nil, nil,
		proc("S", nil, 2, ir.Edge{From: 0, To: 1, Run: runOf(1, 1)}),
		dynProc("D", nil, 0, 3, ir.Edge{From: 1, To: 2}, ir.Edge{From: 2, To: 0}))
	full, red := runPOR(t, m, false, DFS), runPOR(t, m, true, DFS)
	if red.Reduction.Applied || !strings.Contains(red.Reduction.Reason, "dormant") || red.States != full.States {
		t.Fatalf("reduction %+v, %d states against %d", red.Reduction, red.States, full.States)
	}
}

// ---- the audit sees each of these traps -----------------------------------------------

func TestPORAuditSeesTheRunTraps(t *testing.T) {
	for _, c := range []struct {
		name string
		m    *ir.Model
		proc string // the process a broken analysis would expand alone
		want string
	}{
		{"run enables a guard on a counter", runEnablesAGuard(), "R", "C1"},
		{"run enables a guard on the table", runEnablesANrPrGuard(), "R", "C1"},
		{"the run is visible to a property on the table", nrPrPropertyTrap(), "S", "C2"},
		{"the arguments of the run are a read", runArgsTrap(), "S", "C1"},
		{"the initialisers of the run are a read", runInitTrap(), "S", "C1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, order := range permutations(len(c.m.Processes)) {
				mm := reordered(t, c.m, order...)
				p := 0
				for i := range mm.Processes {
					if mm.Processes[i].Name == c.proc {
						p = i
					}
				}
				wantAuditFailure(t, c.name, mm, c.want, p)
			}
		})
	}
}
