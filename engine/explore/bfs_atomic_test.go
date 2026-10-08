package explore

import (
	"context"
	"runtime"
	"testing"
	"time"

	"modelcheck/ir"
)

// The breadth-first search (--bfs, mc_check search "bfs", mc_estimate, and the
// graph the CTL checker builds) expands the intermediate states of an atomic
// sequence depth-first inside one step, and every node of that descent kept its
// own copy of the chain of moves that led to it: a sequence of n steps cost n^2/2
// references, so a sequence that never ends (about 100 000 steps before the
// bound of an atomic sequence answers) needed some 100 GB, and the process was
// killed long before. The old frontend could not produce such a sequence from
// `atomic { do ... od }` (it let go of the control at the back edge), so it never
// came up; with the loop kept inside the block (the weak-fairness branch) it is
// one line of Promela, and mc_estimate, which a client runs first, died on it.

// atomicCount is P counting to n in one atomic sequence (an atomic self-loop
// while n < limit, then a plain edge out) beside a process that sets a flag.
func atomicCount(limit int64) *ir.Model {
	count := ir.Edge{From: 0, To: 0, Atomic: true,
		Guard:  ir.Binary("lt", ir.Ref("n"), ir.Const(limit)),
		Effect: []ir.Assign{{Var: "n", Value: ir.Binary("add", ir.Ref("n"), ir.Const(1))}}}
	leave := ir.Edge{From: 0, To: 1, Guard: ir.Binary("ge", ir.Ref("n"), ir.Const(limit))}
	return model([]ir.Var{{Name: "n", Type: ir.Short}, byteVar("y")}, nil, nil,
		proc("P", nil, 2, count, leave),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("y", 1)}}))
}

// allocated runs fn and reports what the Go allocator handed out meanwhile.
func allocated(fn func()) uint64 {
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	fn()
	runtime.ReadMemStats(&b)
	return b.TotalAlloc - a.TotalAlloc
}

func TestBFSAtomicSequenceCostsMemoryLinearInItsLength(t *testing.T) {
	const limit = 5000 // 12.5 million references (about 400 MB) when every node copies its chain
	var res *Result
	got := allocated(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		r, err := Run(ctx, atomicCount(limit), Options{Mode: BFS, Sweep: true})
		if err != nil {
			t.Fatal(err)
		}
		res = r
	})
	if !res.Complete || res.AtomicSteps < limit {
		t.Fatalf("the search did not finish the sequence: complete %v, %d atomic steps", res.Complete, res.AtomicSteps)
	}
	if o := outcome(t, res, "deadlock"); o.Status != Verified {
		t.Errorf("deadlock: %s (%s)", o.Status, o.Reason)
	}
	if got > 100<<20 {
		t.Errorf("a BFS over an atomic sequence of %d steps allocated %d MB: the chain of moves is copied at every step", limit, got>>20)
	}
}

func TestGraphAtomicSequenceCostsMemoryLinearInItsLength(t *testing.T) {
	const limit = 5000
	var g *Graph
	got := allocated(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var err error
		g, err = BuildGraph(ctx, atomicCount(limit), Options{})
		if err != nil {
			t.Fatal(err)
		}
	})
	if !g.Stats.Complete {
		t.Fatalf("the graph is not complete: %s", g.Stats.Stop)
	}
	if got > 100<<20 {
		t.Errorf("the graph of an atomic sequence of %d steps allocated %d MB: the chain of moves is copied at every step", limit, got>>20)
	}
}

// A sequence that never ends meets the bound of an atomic sequence and answers
// inconclusive; it must do so in memory that does not depend on the bound
// squared. The limit here is the production one, so the test is only as safe as
// the fix: the allocation is checked through a runtime memory limit that turns a
// runaway into a failure of the test and not into the end of the machine.
func TestBFSNeverEndingAtomicSequenceIsBoundedInMemory(t *testing.T) {
	loop := ir.Edge{From: 0, To: 0, Atomic: true, Effect: []ir.Assign{set("y", 1)}}
	m := model([]ir.Var{byteVar("y")}, nil, nil, proc("P", nil, 1, loop),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1}))
	got := allocated(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		r, err := Run(ctx, m, Options{Mode: BFS})
		if err != nil {
			t.Fatal(err)
		}
		if o := outcome(t, r, "deadlock"); o.Status != Inconclusive || o.Evidence != Bounded {
			t.Errorf("deadlock: %s/%s (%s), want inconclusive/bounded", o.Status, o.Evidence, o.Reason)
		}
	})
	if got > 400<<20 {
		t.Errorf("a BFS over a never-ending atomic sequence allocated %d MB", got>>20)
	}
}
