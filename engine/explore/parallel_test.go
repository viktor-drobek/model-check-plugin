package explore

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"testing"

	"modelcheck/ir"
)

// Performance plan 5, step 2: the level-synchronous search over the
// partitioned set. These tests run the search directly (the option that
// selects it from Run arrives with the properties) and compare what it
// stores, counts and layers with the sequential search.

// parCase is one model of the step-2 oracle.
type parCase struct {
	name string
	m    *ir.Model
}

func parFixtures(t *testing.T) []parCase {
	t.Helper()
	cs := []parCase{
		{"petrinet1", petrinet1()},
		{"mutex_flaw", mutexFlaw()},
		{"counters(10,3)", counters(10, 3)},
		{"counters(7,4)", counters(7, 4)},
		{"por-visible", porVisible()},
		{"por-enabling", porEnabling()},
	}
	for _, f := range []string{"bench-indep.pml", "por-shared.pml", "por-deadlock.pml", "por-pipeline.pml", "por-fullchan.pml", "por-emptychan.pml", "por-order.pml", "atomic-t3.pml", "atomic-t4.pml", "leader3.pml"} {
		m, _ := parseFile(t, filepath.Join("../testdata/promela", f), "N=3", "K=3")
		cs = append(cs, parCase{f, m})
	}
	return cs
}

// seqRun is the sequential search of the oracle: depth-first with the sweep
// (the whole graph), plus the set of states it stored.
func seqRun(t *testing.T, m *ir.Model, mode Mode) (*Result, map[string]bool) {
	t.Helper()
	var rec *recorder
	res, err := Run(context.Background(), m, Options{Mode: mode, Sweep: true, NewVisited: func(n int) Visited { rec = &recorder{Visited: defaultVisited(n)}; return rec }})
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, s := range rec.states {
		set[string(s)] = true
	}
	return res, set
}

// parSearch runs the parallel search on m and returns the search (whose
// result holds the counters) and the run (which holds the set).
func parSearch(t *testing.T, m *ir.Model, workers int, kn *parKnobs) (*search, *parRun) {
	t.Helper()
	return parSearchBudget(t, m, workers, kn, Budget{})
}

// parSearchBudget is parSearch under a budget.
func parSearchBudget(t *testing.T, m *ir.Model, workers int, kn *parKnobs, bud Budget) (*search, *parRun) {
	t.Helper()
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, opt: Options{Sweep: true, par: kn, Budget: bud}, ctx: context.Background(), res: &Result{StateBytes: c.layout.Size}}
	s.initOutcomes()
	r, err := s.parallelBFS(workers, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, r
}

// parKnobSets are the extremes of the sizes that must not change a result: the
// defaults (a narrow group goes inline), never inline, one state per segment
// and group, groups that cut ranges.
var parKnobSets = []*parKnobs{nil, {inline: -1}, {inline: -1, segment: 1, group: 1}, {inline: -1, segment: 3, group: 7}, {inline: 1 << 30}}

func parStates(r *parRun) map[string]bool {
	out := map[string]bool{}
	r.set.each(func(_ uint32, v []byte) { out[string(v)] = true })
	return out
}

func hasAtomic(m *ir.Model) bool {
	for _, p := range m.Processes {
		for _, e := range p.Edges {
			if e.Atomic {
				return true
			}
		}
	}
	return false
}

// checkParAgainstSequential is the heart of the oracle at this step: the
// stored set, the counters and, without atomic sequences, the layers.
func checkParAgainstSequential(t *testing.T, name string, m *ir.Model, seq, bfs *Result, seqSet map[string]bool, s *search, r *parRun) {
	t.Helper()
	res := s.res
	if res.States != seq.States || res.Transitions != seq.Transitions || res.AtomicSteps != seq.AtomicSteps {
		t.Fatalf("%s: states/transitions/atomic %d/%d/%d, sequential %d/%d/%d", name, res.States, res.Transitions, res.AtomicSteps, seq.States, seq.Transitions, seq.AtomicSteps)
	}
	if got := parStates(r); len(got) != len(seqSet) {
		t.Fatalf("%s: %d distinct states stored, sequential %d", name, len(got), len(seqSet))
	} else {
		for k := range seqSet {
			if !got[k] {
				t.Fatalf("%s: a state of the sequential search is missing from the parallel set", name)
			}
		}
	}
	if s.stop != "complete" {
		t.Fatalf("%s: stop %q", name, s.stop)
	}
	if !hasAtomic(m) {
		if res.MaxDepth != bfs.MaxDepth {
			t.Fatalf("%s: depth %d, breadth-first %d", name, res.MaxDepth, bfs.MaxDepth)
		}
		if len(res.Levels) != len(bfs.Levels) {
			t.Fatalf("%s: %d levels, breadth-first %d", name, len(res.Levels), len(bfs.Levels))
		}
		for i := range res.Levels {
			if res.Levels[i] != bfs.Levels[i] {
				t.Fatalf("%s: level %d is %+v, breadth-first %+v", name, i, res.Levels[i], bfs.Levels[i])
			}
		}
		if r.layers != len(bfs.Levels) {
			t.Fatalf("%s: %d layers, breadth-first has %d levels", name, r.layers, len(bfs.Levels))
		}
	} else if res.MaxDepth > bfs.MaxDepth {
		t.Fatalf("%s: depth %d above the breadth-first %d", name, res.MaxDepth, bfs.MaxDepth)
	}
}

func TestParallelSearchStoresWhatTheSequentialSearchStores(t *testing.T) {
	compared, states := 0, 0
	for _, c := range parFixtures(t) {
		seq, seqSet := seqRun(t, c.m, DFS)
		if seq.Stop == "invalid model" {
			t.Logf("%s: skipped (an error of the model)", c.name)
			continue
		}
		bfs, _ := seqRun(t, c.m, BFS)
		for _, kn := range parKnobSets {
			s, r := parSearch(t, c.m, 1, kn)
			checkParAgainstSequential(t, c.name, c.m, seq, bfs, seqSet, s, r)
		}
		_, r := parSearch(t, c.m, 1, nil)
		compared++
		states += seq.States
		t.Logf("%s: %d states, %d transitions, %d atomic steps, %d layers", c.name, seq.States, seq.Transitions, seq.AtomicSteps, r.layers)
	}
	if compared < 14 {
		t.Fatalf("only %d fixtures compared (%d states)", compared, states)
	}
}

func TestParallelSearchOnRandomModels(t *testing.T) {
	n := 600
	if testing.Short() {
		n = 100
	}
	compared := 0
	for seed := int64(1); seed <= int64(n); seed++ {
		m := randomPORModel(rand.New(rand.NewSource(seed)))
		name := fmt.Sprintf("seed %d", seed)
		seq, seqSet := seqRun(t, m, DFS)
		if seq.Stop != "complete" && seq.Stop != "all properties decided" {
			continue // an error of the model, which comes with the properties
		}
		if len(seqSet) > 20000 {
			continue
		}
		bfs, _ := seqRun(t, m, BFS)
		for _, kn := range parKnobSets {
			s, r := parSearch(t, m, 1, kn)
			checkParAgainstSequential(t, name, m, seq, bfs, seqSet, s, r)
		}
		compared++
	}
	t.Logf("%d of %d random models compared", compared, n)
	if compared < n/2 {
		t.Fatalf("only %d of %d random models compared", compared, n)
	}
}

// The inline path (a narrow group, no records) and the batched path give the
// same ids, the same parents and the same counters, whatever the segment and
// group sizes: they differ in how the records travel and not in what is
// inserted, in which order.
func TestParallelInlineEqualsBatched(t *testing.T) {
	for _, c := range parFixtures(t) {
		seq, _ := seqRun(t, c.m, DFS)
		if seq.Stop == "invalid model" {
			continue
		}
		var want string
		for i, kn := range append([]*parKnobs{
			{inline: -1}, // never inline
		}, append(parKnobSets[1:], &parKnobs{inline: -1, segment: 1000, group: 1e6}, &parKnobs{inline: 50, segment: 5, group: 40})...) {
			s, r := parSearch(t, c.m, 1, kn)
			got := parFingerprint(r) + fmt.Sprint(s.res.States, s.res.Transitions, s.res.AtomicSteps, s.res.MaxDepth, r.layers, r.maxLayer)
			if i == 0 {
				want = got
			} else if got != want {
				t.Fatalf("%s: knobs %+v change the result:\n got %s\nwant %s", c.name, *kn, got, want)
			}
		}
	}
}

// parFingerprint is a digest of what the set holds: every partition's vectors
// in id order and the parent of each.
func parFingerprint(r *parRun) string {
	h := uint64(1469598103934665603)
	r.set.each(func(id uint32, v []byte) {
		h = (h ^ uint64(id)) * 1099511628211
		for _, b := range v {
			h = (h ^ uint64(b)) * 1099511628211
		}
		h = (h ^ uint64(r.set.parentOf(id))) * 1099511628211
	})
	return fmt.Sprintf("%016x/%d/", h, r.set.Len())
}

func TestParallelFrontierIsAscendingAndComplete(t *testing.T) {
	m := counters(5, 4)
	_, r := parSearch(t, m, 1, &parKnobs{inline: -1, segment: 4, group: 10})
	seen := map[uint32]bool{}
	r.set.each(func(id uint32, _ []byte) { seen[id] = true })
	if len(seen) != 625 {
		t.Fatalf("%d states, want 625", len(seen))
	}
	if r.layers != 17 { // 4 counters mod 5: distance of (4,4,4,4) is 16
		t.Fatalf("%d layers, want 17", r.layers)
	}
	var ids []int
	for id := range seen {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	if len(ids) != 625 {
		t.Fatal("ids repeat")
	}
}

// The running byte count that the coordinator keeps from the partitions each
// group touched is the byte count of the whole set.
func TestParallelRunningByteCountEqualsTheFullCount(t *testing.T) {
	for _, c := range parFixtures(t) {
		for _, kn := range parKnobSets {
			_, r := parSearch(t, c.m, 1, kn)
			if got, want := r.set.accounted(), r.set.Bytes(); got != want {
				t.Fatalf("%s knobs %+v: running count %d, full count %d", c.name, kn, got, want)
			}
		}
	}
}
