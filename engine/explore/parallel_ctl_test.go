package explore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"modelcheck/cex"
	"modelcheck/ir"
)

// parallelGraphForTest exposes the graph builder's parallel merge separately
// from Run's eligibility decision. Graph construction is ordinarily selected
// by Run only after it has established that a CTL-only call can use it.
func parallelGraphForTest(t *testing.T, m *ir.Model, opt Options) *Graph {
	return parallelGraphForContextTest(t, context.Background(), m, opt)
}

func parallelGraphForContextTest(t *testing.T, ctx context.Context, m *ir.Model, opt Options) *Graph {
	t.Helper()
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, opt: opt, ctx: ctx, visited: defaultVisited(c.layout.Size), res: &Result{StateBytes: c.layout.Size}}
	g := &Graph{Layout: c.layout, Visited: s.visited, Stats: &Stats{StateBytes: c.layout.Size}, s: s}
	if err := g.buildParallel(opt.Workers); err != nil {
		t.Fatal(err)
	}
	return g
}

func sequentialGraphForTest(t *testing.T, m *ir.Model, opt Options) *Graph {
	t.Helper()
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, opt: opt, ctx: context.Background(), visited: defaultVisited(c.layout.Size), res: &Result{StateBytes: c.layout.Size}}
	g := &Graph{Layout: c.layout, Visited: s.visited, Stats: &Stats{StateBytes: c.layout.Size}, s: s}
	g.build()
	return g
}

type graphSnapshot struct {
	Succ       [][]int32
	Chains     [][][]cex.Ref
	Stuttered  []bool
	Parents    []int32
	Depths     []int32
	Stats      Stats
	Invalid    string
	InvalidRun *cex.Trace
}

func snapshotGraph(g *Graph) graphSnapshot {
	chains := make([][][]cex.Ref, len(g.chain))
	for from, edges := range g.chain {
		chains[from] = make([][]cex.Ref, len(edges))
		for edge, chain := range edges {
			chains[from][edge] = chain.refs()
		}
	}
	stats := *g.Stats
	stats.Elapsed = 0
	return graphSnapshot{
		Succ:       g.Succ,
		Chains:     chains,
		Stuttered:  g.Stuttered,
		Parents:    g.s.parent,
		Depths:     g.s.depth,
		Stats:      stats,
		Invalid:    g.Invalid,
		InvalidRun: g.InvalidTrace,
	}
}

func expectedEdgeBytes(g *Graph) int64 {
	var total int64
	for _, successors := range g.Succ {
		total += graphStateBytes + int64(len(successors))*graphEdgeBytes
	}
	return total
}

func wantExactEdgeAccounting(t *testing.T, g *Graph) {
	t.Helper()
	if got, want := g.edgeBytes(), expectedEdgeBytes(g); got != want {
		t.Fatalf("edge bytes %d, want %d from successors", got, want)
	}
}

func graphEarlierErrorBeforeFanOut(fan int) *ir.Model {
	p := ir.Process{Name: "P", Locations: locs("start", "bad", "wide")}
	p.Edges = append(p.Edges,
		ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}},
		ir.Edge{From: 0, To: 2, Effect: []ir.Assign{set("x", 2)}},
		ir.Edge{From: 1, To: 1, Effect: []ir.Assign{{Var: "s", Value: ir.Binary("div", ir.Const(1), ir.Const(0))}}},
	)
	for i := 0; i < fan; i++ {
		p.Edges = append(p.Edges, ir.Edge{From: 2, To: 2, Effect: []ir.Assign{{Var: "s", Value: ir.Const(int64(i))}}})
	}
	return model([]ir.Var{{Name: "x", Type: ir.Byte}, {Name: "s", Type: ir.Short}}, nil, nil, p)
}

func graphEdgeBeforeError() *ir.Model {
	return model([]ir.Var{byteVar("x")}, nil, nil,
		proc("P", nil, 2,
			ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}},
			ir.Edge{From: 0, To: 1, Effect: []ir.Assign{{Var: "x", Value: ir.Binary("div", ir.Const(1), ir.Const(0))}}},
		),
	)
}

// A graph worker can find an error after it has already finished an earlier
// edge. The sequential build keeps the earlier edge, then stops at the error;
// the parallel merge must replay that same order rather than discard the edge.
func TestParallelCTLGraphMergesEdgesBeforeLaterError(t *testing.T) {
	g := parallelGraphForTest(t, graphEdgeBeforeError(), Options{Workers: 2})
	if g.Invalid == "" || !strings.Contains(g.Invalid, "division by zero") {
		t.Fatalf("invalid %q, want the later division error", g.Invalid)
	}
	if got := len(g.Succ[0]); got != 1 {
		t.Fatalf("initial state has %d successor(s), want the successful edge before the error", got)
	}
}

// Worker-to-coordinator handoffs are part of the graph construction's live
// memory. A low budget must stop before accepting more graph storage.
func TestParallelCTLGraphAccountsExpansionBatchMemory(t *testing.T) {
	g := parallelGraphForTest(t, fanOutModel(2000, false), Options{Workers: 4, Budget: Budget{MaxMemBytes: 100 << 10}})
	if g.Stats.Complete || !strings.HasPrefix(g.Stats.Stop, "memory budget exhausted") {
		t.Fatalf("complete %v stop %q, want a temporary-expansion memory stop", g.Stats.Complete, g.Stats.Stop)
	}
}

func TestParallelCTLGraphHonoursCancelledContextDuringWideExpansion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g := parallelGraphForContextTest(t, ctx, fanOutModel(20_000, false), Options{Workers: 4})
	if g.Stats.Complete || g.Stats.Stop != "time budget exhausted" {
		t.Fatalf("complete %v stop %q, want cancelled graph construction", g.Stats.Complete, g.Stats.Stop)
	}
}

func TestParallelCTLGraphReplaysEarlierErrorBeforeSpeculativeMemoryStop(t *testing.T) {
	for n := 0; n < 20; n++ {
		g := parallelGraphForTest(t, graphEarlierErrorBeforeFanOut(2_000), Options{
			Workers: 2,
			Budget:  Budget{MaxMemBytes: 100 << 10},
		})
		if g.Invalid == "" || !strings.Contains(g.Invalid, "division by zero") {
			t.Fatalf("run %d: invalid %q, want earlier graph-ID division error", n, g.Invalid)
		}
		if strings.HasPrefix(g.Stats.Stop, "memory budget exhausted") {
			t.Fatalf("run %d: stop %q, want the earlier invalid-model error to win", n, g.Stats.Stop)
		}
	}
}

// This crosses compact visited-arena chunk boundaries while a batch of
// workers expands a prior layer. Under -race it catches workers reading an
// arena concurrently with the coordinator extending it.
func TestParallelCTLGraphSnapshotsWorkerSourcesBeforeStore(t *testing.T) {
	m := atomicCount(9_000)
	for n := 0; n < 3; n++ {
		g := parallelGraphForTest(t, m, Options{Workers: 4})
		if !g.Stats.Complete {
			t.Fatalf("run %d: graph stopped: %s", n, g.Stats.Stop)
		}
	}
}

func TestParallelCTLGraphBoundsAtomicWorkerScratch(t *testing.T) {
	g := parallelGraphForTest(t, atomicCount(20_000), Options{
		Workers: 2,
		Budget:  Budget{MaxMemBytes: 256 << 10},
	})
	if g.Stats.Complete || !strings.HasPrefix(g.Stats.Stop, "memory budget exhausted") {
		t.Fatalf("complete %v stop %q, want a bounded memory stop", g.Stats.Complete, g.Stats.Stop)
	}
}

func TestParallelCTLGraphMatchesSequentialOutcome(t *testing.T) {
	m, _ := parseFile(t, filepath.Join("../testdata/promela", "ctl-switch.pml"))
	m.Properties = []ir.Property{{
		ID:      "ctl1",
		Kind:    KindCTL,
		Formula: "AG (x < 2)",
		Text:    "AG (x < 2)",
	}}

	sequential, err := Run(context.Background(), m, Options{Mode: BFS})
	if err != nil {
		t.Fatal(err)
	}
	parallel, err := Run(context.Background(), m, Options{Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if parallel.Parallel == nil || !parallel.Parallel.Applied || parallel.Parallel.Workers != 4 {
		t.Fatalf("parallel metadata %+v, want an applied four-worker CTL graph build", parallel.Parallel)
	}

	seqOutcome := outcome(t, sequential, "ctl1")
	parallelOutcome := outcome(t, parallel, "ctl1")
	// Elapsed is wall-clock data from two independent runs; it is excluded by
	// --no-timing in the CLI scenario and cannot be expected to match here.
	seqOutcome.Stats.Elapsed = 0
	parallelOutcome.Stats.Elapsed = 0
	if !reflect.DeepEqual(parallelOutcome, seqOutcome) {
		t.Fatalf("parallel outcome %+v, want sequential outcome %+v", parallelOutcome, seqOutcome)
	}
}

func TestParallelCTLGraphMatchesSequentialGraphWithAtomicPathsAndDepthBudget(t *testing.T) {
	tests := []struct {
		name  string
		model *ir.Model
		opt   Options
	}{
		{name: "atomic chain", model: atomicCount(4)},
		{name: "atomic chain depth budget", model: atomicCount(4), opt: Options{Budget: Budget{MaxDepth: 1}}},
		{name: "terminal fan-out after atomic chain", model: atomicPrefixTerminalFanout(4, 3)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sequential := sequentialGraphForTest(t, tc.model, tc.opt)
			wantExactEdgeAccounting(t, sequential)
			want := snapshotGraph(sequential)
			for _, workers := range []int{1, 2, 8} {
				opt := tc.opt
				opt.Workers = workers
				parallel := parallelGraphForTest(t, tc.model, opt)
				wantExactEdgeAccounting(t, parallel)
				if got := snapshotGraph(parallel); !reflect.DeepEqual(got, want) {
					t.Fatalf("workers=%d graph differs from sequential\n got: %+v\nwant: %+v", workers, got, want)
				}
			}
		})
	}
}

func TestParallelCTLGraphReportsConstructionMetadata(t *testing.T) {
	m, _ := parseFile(t, filepath.Join("../testdata/promela", "ctl-switch.pml"))
	m.Properties = []ir.Property{{ID: "ctl1", Kind: KindCTL, Formula: "AG (x < 2)", Text: "AG (x < 2)"}}
	r, err := Run(context.Background(), m, Options{Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	p := r.Parallel
	if p == nil || !p.Applied || p.Layers == 0 || p.MaxLayerStates < 2 || p.WorkerBytesEst <= 0 {
		t.Fatalf("parallel metadata %+v, want CTL graph construction measurements", p)
	}
}

func TestParallelCTLGraphPropagatesCompleteGraphStats(t *testing.T) {
	m, _ := parseFile(t, filepath.Join("../testdata/promela", "ctl-switch.pml"))
	m.Properties = []ir.Property{{ID: "ctl1", Kind: KindCTL, Formula: "AG (x < 2)", Text: "AG (x < 2)"}}
	r, err := Run(context.Background(), m, Options{Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	o := outcome(t, r, "ctl1")
	if !r.Complete || r.Stop != "complete" || o.Stats == nil {
		t.Fatalf("top-level complete=%v stop=%q, outcome stats=%+v", r.Complete, r.Stop, o.Stats)
	}
	if r.States != o.Stats.States || r.Transitions != o.Stats.Transitions || r.AtomicSteps != o.Stats.AtomicSteps || r.MaxDepth != o.Stats.MaxDepth || r.MemBytes != o.Stats.MemBytes {
		t.Fatalf("top-level graph stats %+v, want outcome graph stats %+v", r, o.Stats)
	}
}

func TestParallelCTLGraphGivesEachOutcomeOwnedStats(t *testing.T) {
	m, _ := parseFile(t, filepath.Join("../testdata/promela", "ctl-switch.pml"))
	m.Properties = []ir.Property{
		{ID: "one", Kind: KindCTL, Formula: "AG (x < 2)", Text: "AG (x < 2)"},
		{ID: "two", Kind: KindCTL, Formula: "EF (x == 2)", Text: "EF (x == 2)"},
	}
	r, err := Run(context.Background(), m, Options{Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	one, two := outcome(t, r, "one"), outcome(t, r, "two")
	if one.Stats == nil || two.Stats == nil || one.Stats == two.Stats {
		t.Fatalf("outcome stats one=%p two=%p, want independent values", one.Stats, two.Stats)
	}
	before := two.Stats.Elapsed
	one.Stats.Elapsed = 0
	if two.Stats.Elapsed != before {
		t.Fatalf("second outcome elapsed changed from %v to %v after mutating the first", before, two.Stats.Elapsed)
	}
}

func TestParallelCTLGraphGroupsAtomicSuccessorsByStoredStateLevel(t *testing.T) {
	m := model([]ir.Var{byteVar("x")}, nil, nil,
		proc("P", nil, 4,
			ir.Edge{From: 0, To: 1, Atomic: true, Effect: []ir.Assign{set("x", 1)}},
			ir.Edge{From: 1, To: 2},
			ir.Edge{From: 0, To: 3, Effect: []ir.Assign{set("x", 2)}},
		),
	)
	g := parallelGraphForTest(t, m, Options{Workers: 2})
	if got, want := g.parallel.layers, 2; got != want {
		t.Fatalf("parallel layers %d, want %d stored-state frontiers", got, want)
	}
	if got, want := g.parallel.maxLayer, 2; got != want {
		t.Fatalf("parallel max layer %d, want the two stored successors", got)
	}
}

func TestCTLGraphDoesNotUseParallelPathWithoutWorkers(t *testing.T) {
	m, _ := parseFile(t, filepath.Join("../testdata/promela", "ctl-switch.pml"))
	m.Properties = []ir.Property{{ID: "ctl1", Kind: KindCTL, Formula: "AG (x < 2)", Text: "AG (x < 2)"}}

	r, err := Run(context.Background(), m, Options{Mode: BFS})
	if err != nil {
		t.Fatal(err)
	}
	if r.Parallel != nil {
		t.Fatalf("parallel metadata %+v, want no parallel path without workers", r.Parallel)
	}
}

func TestParallelCTLGraphFallsBackForUnsupportedOptions(t *testing.T) {
	m, _ := parseFile(t, filepath.Join("../testdata/promela", "ctl-switch.pml"))
	m.Properties = []ir.Property{{ID: "ctl1", Kind: KindCTL, Formula: "AG (x < 2)", Text: "AG (x < 2)"}}

	for _, tc := range []struct {
		name   string
		opt    Options
		mix    bool
		reason string
	}{
		{name: "fairness", opt: Options{Workers: 2, Fairness: FairnessWeak}},
		{name: "partial-order reduction", opt: Options{Workers: 2, POR: true}, reason: parRefuseCTLPOR},
		{name: "custom visited", opt: Options{Workers: 2, NewVisited: defaultVisited}},
		{name: "mixed ltl request", opt: Options{Workers: 2}, mix: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputValue := *m
			inputValue.Properties = append([]ir.Property(nil), m.Properties...)
			input := &inputValue
			if tc.mix {
				input.Properties = append(input.Properties, ir.Property{ID: "ltl", Kind: KindLTL, Formula: "[] true"})
			}
			r, err := Run(context.Background(), input, tc.opt)
			if err != nil {
				t.Fatal(err)
			}
			if r.Parallel == nil || r.Parallel.Applied {
				t.Fatalf("parallel metadata %+v, want sequential fallback", r.Parallel)
			}
			if tc.reason != "" && r.Parallel.Reason != tc.reason {
				t.Fatalf("parallel reason %q, want %q", r.Parallel.Reason, tc.reason)
			}
			if tc.reason != "" {
				if got := outcome(t, r, "ctl1"); got == nil || got.Status != Violated {
					t.Fatalf("CTL outcome %+v, want violated", got)
				}
			}
		})
	}
}

func TestParallelCTLGraphDoesNotBuildForOnlyTableRefusedFormulas(t *testing.T) {
	m := tableless()
	m.Properties = []ir.Property{{ID: "none", Kind: KindCTL, Formula: "EF (_nr_pr == 0)"}}
	r := runOpts(t, m, Options{Workers: 2})
	wantRefused(t, outcome(t, r, "none"), "formula")
	if r.States != 0 {
		t.Fatalf("a call with only refused CTL formulas stored %d states", r.States)
	}
	if r.Parallel == nil || r.Parallel.Applied || r.Parallel.Reason != parRefuseIdle {
		t.Fatalf("parallel metadata %+v, want an idle refusal", r.Parallel)
	}
}

func TestParallelCTLGraphKeepsSafetySequentialBesideRefusedCTL(t *testing.T) {
	m := tableless(
		ir.Property{ID: "safe", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.Ref("x"), ir.Const(2))},
		ir.Property{ID: "none", Kind: KindCTL, Formula: "EF (_nr_pr == 0)"},
	)
	r := runOpts(t, m, Options{Workers: 2})
	if r.Parallel == nil || r.Parallel.Applied || r.Parallel.Reason != parRefuseMixedCTL {
		t.Fatalf("parallel metadata %+v, want a recorded mixed safety/CTL fallback", r.Parallel)
	}
	wantRefused(t, outcome(t, r, "none"), "formula")
}

func TestParallelCTLGraphAccountsExistingEdgeListsDuringConstruction(t *testing.T) {
	c, err := compile(fanOutModel(1, false))
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, next: make([]byte, c.layout.Size), visited: defaultVisited(c.layout.Size)}
	g := &Graph{
		s:    s,
		Succ: [][]int32{{1, 2, 3}, nil, nil, nil},
	}
	g.edgeMem = expectedEdgeBytes(g)
	wantExactEdgeAccounting(t, g)
	withoutEdges := s.bfsExtra(0) + int64(len(s.next))*3 + int64(graphStreamWindow)*(int64(len(s.next))+bfsNodeBytes+chainNodeBytes+graphHandoffBytes)
	if got, want := g.parallelBFSExtra(0), withoutEdges+g.edgeBytes(); got != want {
		t.Fatalf("parallel construction estimate %d, want %d including %d bytes of existing edges", got, want, g.edgeBytes())
	}
}

func TestParallelCTLGraphKeepsSafetySequentialBesideExecutableCTL(t *testing.T) {
	m, _ := parseFile(t, filepath.Join("../testdata/promela", "ctl-switch.pml"))
	m.Properties = []ir.Property{
		{ID: "reach", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.Ref("x"), ir.Const(2))},
		{ID: "ctl", Kind: KindCTL, Formula: "AG (x < 2)", Text: "AG (x < 2)"},
	}
	sequential, err := Run(context.Background(), m, Options{Mode: BFS})
	if err != nil {
		t.Fatal(err)
	}
	parallel, err := Run(context.Background(), m, Options{Mode: BFS, Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if parallel.Parallel == nil || parallel.Parallel.Applied {
		t.Fatalf("parallel metadata %+v, want sequential safety fallback", parallel.Parallel)
	}
	for _, id := range []string{"reach", "ctl"} {
		want, got := outcome(t, sequential, id), outcome(t, parallel, id)
		if want == nil || got == nil {
			t.Fatalf("%s outcomes sequential=%+v parallel=%+v", id, want, got)
		}
		wantCopy, gotCopy := *want, *got
		if wantCopy.Stats != nil {
			stats := *wantCopy.Stats
			stats.Elapsed = 0
			wantCopy.Stats = &stats
		}
		if gotCopy.Stats != nil {
			stats := *gotCopy.Stats
			stats.Elapsed = 0
			gotCopy.Stats = &stats
		}
		if !reflect.DeepEqual(gotCopy, wantCopy) {
			t.Fatalf("%s outcome %+v, want sequential outcome %+v", id, gotCopy, wantCopy)
		}
	}
}

// atomicPrefixTerminalFanout has a single long atomic prefix, followed by
// many terminal moves. Each terminal event carries the same long path, so a
// copied []cex.Ref payload would make the bounded handoff queue allocate one
// complete path per queued event.
func atomicPrefixTerminalFanout(prefix int64, fan int) *ir.Model {
	p := ir.Process{Name: "P", Locations: locs("atomic", "done")}
	p.Edges = append(p.Edges, ir.Edge{From: 0, To: 0, Atomic: true,
		Guard:  ir.Binary("lt", ir.Ref("n"), ir.Const(prefix)),
		Effect: []ir.Assign{{Var: "n", Value: ir.Binary("add", ir.Ref("n"), ir.Const(1))}},
	})
	for i := 0; i < fan; i++ {
		p.Edges = append(p.Edges, ir.Edge{From: 0, To: 1,
			Guard:  ir.Binary("ge", ir.Ref("n"), ir.Const(prefix)),
			Effect: []ir.Assign{{Var: "x", Value: ir.Const(int64(i))}},
		})
	}
	return model([]ir.Var{{Name: "n", Type: ir.Short}, {Name: "x", Type: ir.Short}}, nil, nil, p)
}

func TestParallelCTLGraphDoesNotCopyAtomicPathsIntoTerminalEvents(t *testing.T) {
	const prefix, fan = 5_000, 256
	var g *Graph
	got := allocated(func() {
		g = parallelGraphForTest(t, atomicPrefixTerminalFanout(prefix, fan), Options{Workers: 4})
	})
	if !g.Stats.Complete {
		t.Fatalf("complete %v stop %q, want completed terminal fan-out", g.Stats.Complete, g.Stats.Stop)
	}
	if g.Stats.Transitions < int(prefix)+fan {
		t.Fatalf("transitions %d, want at least %d after the terminal fan-out", g.Stats.Transitions, int(prefix)+fan)
	}
	if g.Stats.MemBytes > 2<<20 {
		t.Fatalf("parallel graph retained %d MB for a %d-step atomic prefix and %d terminal moves", g.Stats.MemBytes>>20, prefix, fan)
	}
	if got > 16<<20 {
		t.Fatalf("parallel graph allocated %d MB for a %d-step atomic prefix and %d terminal moves", got>>20, prefix, fan)
	}
}

func graphDigest(g *Graph) string {
	return fmt.Sprintf("%d|%d|%v", g.Stats.States, g.Stats.Transitions, g.Succ)
}

func TestParallelCTLGraphMemoryStopIsWorkerCountIndependent(t *testing.T) {
	m := fanOutModel(2_000, false)
	var wantStop, wantDigest string
	var wantDepth int
	for _, workers := range []int{1, 2, 4, 8} {
		g := parallelGraphForTest(t, m, Options{Workers: workers, Budget: Budget{MaxMemBytes: 100 << 10}})
		if !strings.HasPrefix(g.Stats.Stop, "memory budget exhausted") {
			t.Fatalf("workers=%d stop %q, want a memory stop", workers, g.Stats.Stop)
		}
		gotDigest := graphDigest(g)
		if workers == 1 {
			wantStop, wantDigest, wantDepth = g.Stats.Stop, gotDigest, g.Stats.MaxDepth
			continue
		}
		if g.Stats.Stop != wantStop || gotDigest != wantDigest || g.Stats.MaxDepth != wantDepth {
			t.Fatalf("workers=%d stop/digest/depth %q/%s/%d, want %q/%s/%d", workers, g.Stats.Stop, gotDigest, g.Stats.MaxDepth, wantStop, wantDigest, wantDepth)
		}
	}
}

func TestParallelCTLGraphPreservesLTLFirstFormulaErrorBeforeCTLPreflight(t *testing.T) {
	m := tableless(
		ir.Property{ID: "ltl-first", Kind: KindLTL, Formula: "[] ("},
		ir.Property{ID: "ctl-second", Kind: KindCTL, Formula: "EF ("},
	)
	_, err := Run(context.Background(), m, Options{Workers: 2})
	var formulaErr *FormulaError
	if !errors.As(err, &formulaErr) {
		t.Fatalf("Run error %v, want an LTL FormulaError", err)
	}
	if formulaErr.Logic != "ltl" || formulaErr.PropertyID != "ltl-first" {
		t.Fatalf("formula error %+v, want first LTL formula", formulaErr)
	}
}
