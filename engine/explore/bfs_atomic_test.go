package explore

import (
	"context"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"modelcheck/cex"
	"modelcheck/frontend/promela"
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
// the fix: the allocation is measured (TotalAlloc) after the run and compared
// with a bound; a runaway is not contained, it is reported.
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

// bfsExitEveryStep is the model of issue #7: an atomic loop that can leave
// at every step, so that every level of the descent stores a new state with
// a chain of its own length. Stored as copies, the chains were n²/2
// references.
func bfsExitEveryStep(t *testing.T) *ir.Model {
	t.Helper()
	src, err := os.ReadFile("../testdata/budget/bfs-memory.pml")
	if err != nil {
		t.Fatal(err)
	}
	res, perr := promela.Parse(src, "bfs-memory.pml", nil)
	if perr != nil {
		t.Fatal(perr)
	}
	return res.Model
}

// TestChainNodeBytesIsTheNode pins the per-node costs of a kept chain node
// and of a node of the descent.
func TestChainNodeBytesIsTheNode(t *testing.T) {
	if got := int64(unsafe.Sizeof(chainNode{})); got != chainNodeBytes {
		t.Fatalf("chainNodeBytes = %d, but a chain node is %d bytes", int64(chainNodeBytes), got)
	}
	if got := int64(unsafe.Sizeof(bfsNode{})); got != bfsNodeBytes {
		t.Fatalf("bfsNodeBytes = %d, but a bfsNode is %d bytes", int64(bfsNodeBytes), got)
	}
}

// TestBFSExitAtEveryStepIsLinear: the breadth-first search and the graph
// over the issue's model keep one chain node per move, so 3 000 stored
// levels cost megabytes, not the 300 MB of 3 000 copied chains, and the
// estimate reports the nodes.
func TestBFSExitAtEveryStepIsLinear(t *testing.T) {
	m := bfsExitEveryStep(t)
	const states = 3000
	var res *Result
	got := allocated(func() {
		r, err := Run(context.Background(), m, Options{Mode: BFS, Budget: Budget{MaxStates: states}})
		if err != nil {
			t.Fatal(err)
		}
		res = r
	})
	if o := outcome(t, res, "deadlock"); o.Status != Inconclusive || !strings.Contains(o.Reason, "state budget") {
		t.Fatalf("deadlock: %s: %s", o.Status, o.Reason)
	}
	if got > 100<<20 {
		t.Errorf("a BFS over %d levels that each store a state allocated %d MB: the chains of moves are copied per stored state", states, got>>20)
	}
	if res.MemBytes < states*chainNodeBytes {
		t.Errorf("memory estimate %d does not count the %d kept chain nodes", res.MemBytes, states)
	}
	var g *Graph
	got = allocated(func() {
		var err error
		g, err = BuildGraph(context.Background(), m, Options{Budget: Budget{MaxStates: states}})
		if err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(g.Stats.Stop, "state budget") || got > 100<<20 {
		t.Errorf("graph: stop %q, allocated %d MB", g.Stats.Stop, got>>20)
	}
	if g.Stats.MemBytes < states*chainNodeBytes {
		t.Errorf("graph memory estimate %d does not count the kept chain nodes", g.Stats.MemBytes)
	}
}

// TestBFSExitAtEveryStepStopsOnTheMemoryBudget: with the chain nodes in the
// estimate, a memory budget stops the run where it used to die.
func TestBFSExitAtEveryStepStopsOnTheMemoryBudget(t *testing.T) {
	m := bfsExitEveryStep(t)
	r, err := Run(context.Background(), m, Options{Mode: BFS, Budget: Budget{MaxMemBytes: 8 << 20}})
	if err != nil {
		t.Fatal(err)
	}
	if o := outcome(t, r, "deadlock"); o.Status != Inconclusive || !strings.Contains(o.Reason, "memory budget") {
		t.Fatalf("deadlock: %s: %s", o.Status, o.Reason)
	}
	g, err := BuildGraph(context.Background(), m, Options{Budget: Budget{MaxMemBytes: 8 << 20}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(g.Stats.Stop, "memory budget") {
		t.Fatalf("graph: stop %q", g.Stats.Stop)
	}
}

// TestChainNodesReplayTheSameTrace: the references read off the kept nodes
// are the moves of the descent, in order — a violation after an atomic
// sequence of two alternating edges has the same run in BFS as in DFS, step
// by step, and the graph's trace through the chain is that run as well.
func TestChainNodesReplayTheSameTrace(t *testing.T) {
	const limit = 20
	// Even steps take `even`, odd steps `odd`: a reversed or shuffled chain
	// would replay the wrong edge and show in the step texts.
	even := ir.Edge{From: 0, To: 0, Atomic: true, Text: "even: n++",
		Guard:  ir.Binary("and", ir.Binary("lt", ir.Ref("n"), ir.Const(limit)), ir.Binary("eq", ir.Binary("mod", ir.Ref("n"), ir.Const(2)), ir.Const(0))),
		Effect: []ir.Assign{{Var: "n", Value: ir.Binary("add", ir.Ref("n"), ir.Const(1))}}}
	odd := ir.Edge{From: 0, To: 0, Atomic: true, Text: "odd: n++",
		Guard:  ir.Binary("and", ir.Binary("lt", ir.Ref("n"), ir.Const(limit)), ir.Binary("eq", ir.Binary("mod", ir.Ref("n"), ir.Const(2)), ir.Const(1))),
		Effect: []ir.Assign{{Var: "n", Value: ir.Binary("add", ir.Ref("n"), ir.Const(1))}}}
	leave := ir.Edge{From: 0, To: 1, Text: "leave", Guard: ir.Binary("ge", ir.Ref("n"), ir.Const(limit)), Assert: ir.Binary("lt", ir.Ref("n"), ir.Const(limit))}
	m := model([]ir.Var{{Name: "n", Type: ir.Short}, byteVar("y")}, nil, nil,
		proc("P", nil, 2, even, odd, leave),
		proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("y", 1)}}))
	commands := func(tr *cex.Trace) []string {
		if tr == nil {
			return nil
		}
		var out []string
		for _, st := range tr.Steps {
			out = append(out, st.Process+": "+st.Command)
		}
		return out
	}
	dfs := run(t, m, Options{})
	bfs := run(t, m, Options{Mode: BFS})
	od, ob := outcome(t, dfs, "assert"), outcome(t, bfs, "assert")
	if od.Status != Violated || ob.Status != Violated {
		t.Fatalf("assert: dfs %s, bfs %s", od.Status, ob.Status)
	}
	want := commands(od.Trace)
	if len(want) != limit+1 || !slices.Equal(want, commands(ob.Trace)) {
		t.Fatalf("the traces differ or are short:\ndfs %v\nbfs %v", want, commands(ob.Trace))
	}
	for i, c := range want[:limit] {
		if (i%2 == 0) != strings.Contains(c, "even") {
			t.Fatalf("step %d of the run is %q: the chain is out of order", i+1, c)
		}
	}
	g, err := BuildGraph(context.Background(), m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !g.Stats.Complete {
		t.Fatalf("graph: %s", g.Stats.Stop)
	}
	// State 0 reaches its first successor through the whole atomic count.
	got := commands(g.Trace([]int{0, int(g.Succ[0][0])}, -1))
	if !slices.Equal(got, want) {
		t.Fatalf("the graph trace through the chain differs from the run:\ngraph %v\nrun   %v", got, want)
	}
}
