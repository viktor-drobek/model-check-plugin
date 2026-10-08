package explore

import (
	"strings"
	"testing"

	"modelcheck/frontend/promela"
	"modelcheck/ir"
)

// Step 4 of performance plan 5: what the search takes over from the model
// through nextEnabled and fire (atomic sequences and their bound, d_step,
// rendezvous, `run` and the exhaustion of its pool, `timeout`, `provided`,
// clears, a never claim that is only stored, the vacuity watch) is the
// sequential semantics, because the same functions are called. These are
// directed models; the random oracle covers the combinations.

// A loop of atomic steps that never blocks must end in the declared bound and
// say so, not hang (scenario 25): the walk over the intermediate states has the
// sequential bound, and it checks the stop flag as it goes.
func TestParallelAnAtomicLoopThatNeverBlocksEndsAtTheBound(t *testing.T) {
	m := parAtomicLoop()
	for _, kn := range parKnobSets {
		res := runOpts(t, m, Options{Workers: 1, par: kn, Budget: Budget{MaxStates: 1000}})
		want := "depth budget exhausted: an atomic sequence exceeds 100000 steps"
		if res.Stop != want {
			t.Fatalf("knobs %+v: stop %q, want %q", kn, res.Stop, want)
		}
		// The walk goes exactly as far as the bound and not a step beyond.
		if res.AtomicSteps != dstepLimit+1 || res.Transitions != dstepLimit+1 {
			t.Fatalf("knobs %+v: %d atomic steps and %d transitions, want %d of each", kn, res.AtomicSteps, res.Transitions, dstepLimit+1)
		}
		if res.Complete {
			t.Fatal("complete")
		}
		for _, o := range res.Outcomes {
			if o.Status != Inconclusive || o.Evidence != Bounded || o.Reason != want {
				t.Fatalf("%s: %s/%s %q", o.Property.ID, o.Status, o.Evidence, o.Reason)
			}
		}
	}
	// The depth-first search is also stopped, by its depth budget.
	if seq := runOpts(t, m, Options{Budget: Budget{MaxDepth: 5000}}); seq.Complete || !hitBudget(seq) {
		t.Fatalf("depth-first: %q", seq.Stop)
	}
}

// A `run` whose pool is smaller than the number of instances the model starts
// is a bound of the engine, not a verdict, and the parallel search says the
// same sentence as the sequential one (scenario 8).
func TestParallelPoolExhaustionIsTheSequentialBound(t *testing.T) {
	src := `byte n;
proctype W() { n++ }
active proctype P() { do :: run W() od }`
	res, perr := promela.ParseWith([]byte(src), "pool.pml", nil, promela.Options{MaxProcs: 2})
	if perr != nil {
		t.Fatal(perr)
	}
	m := res.Model
	dfs := runOpts(t, m, Options{Sweep: true})
	bfs := runOpts(t, m, Options{Sweep: true, Mode: BFS})
	if !strings.HasPrefix(dfs.Stop, "process budget exhausted") || !strings.HasPrefix(bfs.Stop, "process budget exhausted") {
		t.Fatalf("sequential stops %q / %q", dfs.Stop, bfs.Stop)
	}
	for _, kn := range parKnobSets {
		par := runOpts(t, m, Options{Sweep: true, Workers: 1, par: kn})
		if par.Stop != bfs.Stop || par.Complete {
			t.Fatalf("knobs %+v: stop %q, sequential breadth-first %q", kn, par.Stop, bfs.Stop)
		}
		for i, o := range par.Outcomes {
			b := bfs.Outcomes[i]
			if o.Status != Inconclusive || o.Evidence != Bounded || o.Status != b.Status || o.Reason != b.Reason {
				t.Fatalf("%s: %s/%s %q, breadth-first %s/%s %q", o.Property.ID, o.Status, o.Evidence, o.Reason, b.Status, b.Evidence, b.Reason)
			}
		}
	}
}

func TestParallelAgreesOnDirectedModels(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"rendezvous", `chan c = [0] of { byte };
active proctype S() { c!1; c!2 }
active proctype R() { byte v; c?v; c?v }`},
		{"rendezvous with a match and a deadlock", `chan c = [0] of { byte, byte };
active proctype S() { c!1,5; c!2,6 }
active proctype R() { c?2,_ }`},
		{"timeout", `byte x;
active proctype P() { do :: x < 3 -> x++ :: timeout -> break od }`},
		{"timeout and a second process", `byte x; byte y;
active proctype P() { do :: x < 3 -> x++ :: timeout -> y = 1; break od }
active proctype Q() { do :: y == 0 -> skip :: y == 1 -> break od }`},
		{"provided", `byte x;
active proctype P() provided (x == 0) { x = 1; x = 2 }
active proctype Q() { x = 3 }`},
		{"d_step", `byte x; byte y;
active proctype P() { d_step { x = 1; y = x + 1; x = y } ; assert(x == 2) }
active proctype Q() { x = 5 }`},
		{"atomic with a blocking guard", `byte x; byte y;
active proctype P() { atomic { x = 1; y == 1; x = 2 } }
active proctype Q() { y = 1 }`},
		{"atomic and an assert in the middle", `byte x;
active proctype P() { atomic { x = 1; assert(x == 0); x = 2 } }
active proctype Q() { x = 7 }`},
		{"run and an end", `byte n;
proctype W(byte k) { n = n + k }
active proctype P() { run W(1); run W(2) }`},
		{"a buffered channel with clears at the end", `chan c = [2] of { byte };
active proctype P() { c!1; c!2; c!3 }
active proctype Q() { byte v; c?v; c?v }`},
		{"nr_pr", `byte n;
proctype W() { n++ }
active proctype P() { run W(); (_nr_pr == 1) }`},
	} {
		m := promelaModel(t, tc.src)
		for _, kn := range parKnobSets {
			checkParallel(t, tc.name, m, 1, kn)
		}
		sameForAllKnobs(t, tc.name, m, Options{Sweep: true})
		sameForAllKnobs(t, tc.name+" (no sweep)", m, Options{})
	}
}

// A never claim that is not one of the properties of the run is a process that
// the safety search stores and never executes: the state vector holds its
// location, and the counts are the sequential run's (scenario 28).
func TestParallelStoresAndDoesNotExecuteANeverClaim(t *testing.T) {
	m := parClaimModelIR(t)
	claims := 0
	for _, p := range m.Processes {
		if p.Claim {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("%d claim processes", claims)
	}
	for _, kn := range parKnobSets {
		runs := checkParallel(t, "never claim", m, 1, kn)
		if runs.par.States != runs.dfs.States || runs.par.States < 3 {
			t.Fatalf("states %d, sequential %d", runs.par.States, runs.dfs.States)
		}
	}
}

// parClaimModel is a model with a never claim and no property of the claim:
// the frontend turns the claim into a process and an ltl property, and the
// property is dropped here.
func parClaimModel(t testing.TB) *ir.Model {
	t.Helper()
	m := promelaModel(t, `byte x;
active proctype P() { x = 1; x = 2 }
active proctype Q() { x = 3 }
never { do :: x > 1 -> break :: else od }`)
	var props []ir.Property
	for _, p := range m.Properties {
		if p.Kind != ir.KindLTL {
			props = append(props, p)
		}
	}
	m.Properties = props
	return m
}

// The vacuity watch records over the stored states whether each expression was
// ever true and ever false; the parallel search records it where the states
// are stored, per worker, and combines the flags at the end.
func TestParallelWatchCoverageIsTheSequentialOne(t *testing.T) {
	m := mutexFlaw()
	for _, watch := range [][]*ir.Expr{
		{ir.Binary("eq", ir.Ref("cnt"), ir.Const(2))},            // never true
		{ir.Binary("ge", ir.Ref("cnt"), ir.Const(0))},            // always true
		{ir.Binary("eq", ir.Ref("x"), ir.Const(1)), ir.Ref("z")}, // both
		{ir.Binary("eq", ir.PC(0), ir.Const(3)), ir.Binary("gt", ir.Ref("y"), ir.Const(0))},
	} {
		for _, kn := range parKnobSets {
			runs := checkParallelOpt(t, "watch", m, 1, kn, Options{Watch: watch})
			if !runs.par.Complete || len(runs.par.Coverage) != len(watch) {
				t.Fatalf("complete %v coverage %v", runs.par.Complete, runs.par.Coverage)
			}
		}
	}
}
