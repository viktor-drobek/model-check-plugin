package explore

import (
	"fmt"
	"testing"

	"modelcheck/ir"
)

// The parallel search and the refusal of a property that reads the live-process
// table over a model that keeps none (tableread.go) were built on different
// branches. A refused property is neither compiled nor evaluated, so it is in no
// list the parallel workers walk; these tests pin what the two features owe each
// other: the others' answers and counters are those of the breadth-first search,
// and a call whose properties are all refused searches nothing under any option.

func TestParallelSearchBesideAPropertyRefusedForTheTable(t *testing.T) {
	props := []ir.Property{
		{ID: "none", Kind: ir.KindReach, Expr: nrprIs(0)},
		{ID: "two", Kind: ir.KindInvariant, Expr: nrprIs(2)},
		sane(),
	}
	for _, sweep := range []bool{true, false} {
		seq := run(t, tableless(props...), Options{Mode: BFS, Sweep: sweep})
		for _, workers := range []int{1, 2, 4} {
			t.Run(fmt.Sprintf("sweep=%v/workers=%d", sweep, workers), func(t *testing.T) {
				par := run(t, tableless(props...), Options{Workers: workers, Sweep: sweep})
				if par.Parallel == nil || !par.Parallel.Applied {
					t.Fatalf("the parallel search was not applied: %+v", par.Parallel)
				}
				wantRefused(t, outcome(t, par, "none"), "_nr_pr")
				wantRefused(t, outcome(t, par, "two"), "_nr_pr")
				for _, id := range []string{"sane", "deadlock"} {
					a, b := outcome(t, seq, id), outcome(t, par, id)
					if a == nil || b == nil {
						continue
					}
					if a.Status != b.Status || a.Evidence != b.Evidence || a.Reason != b.Reason {
						t.Errorf("%s: breadth-first %s/%s (%s), parallel %s/%s (%s)", id, a.Status, a.Evidence, a.Reason, b.Status, b.Evidence, b.Reason)
					}
				}
				if seq.States != par.States || seq.Transitions != par.Transitions || seq.StateBytes != par.StateBytes {
					t.Errorf("counters: breadth-first %d states %d transitions %d bytes, parallel %d, %d, %d",
						seq.States, seq.Transitions, seq.StateBytes, par.States, par.Transitions, par.StateBytes)
				}
			})
		}
	}
}

func TestParallelWhenEveryPropertyIsRefusedForTheTable(t *testing.T) {
	// The helper adds the implicit deadlock property; the call under test has
	// none, so that every property of it is refused.
	only := func() *ir.Model {
		m := tableless()
		m.Properties = []ir.Property{
			{ID: "none", Kind: ir.KindReach, Expr: nrprIs(0)},
			{ID: "two", Kind: ir.KindInvariant, Expr: nrprIs(2)},
		}
		return m
	}
	seq := run(t, only(), Options{})
	if seq.States != 0 {
		t.Fatalf("the sequential run with only refused properties stored %d states", seq.States)
	}
	for _, workers := range []int{1, 3} {
		par := run(t, only(), Options{Workers: workers})
		wantRefused(t, outcome(t, par, "none"), "_nr_pr")
		wantRefused(t, outcome(t, par, "two"), "_nr_pr")
		if par.States != seq.States || par.Transitions != seq.Transitions || par.Stop != seq.Stop || par.Complete != seq.Complete {
			t.Errorf("workers %d: states %d, transitions %d, stop %q, complete %v; sequential %d, %d, %q, %v",
				workers, par.States, par.Transitions, par.Stop, par.Complete, seq.States, seq.Transitions, seq.Stop, seq.Complete)
		}
		if par.Parallel == nil {
			t.Fatalf("workers %d: no parallel object although the option was given", workers)
		}
		if par.Parallel.Applied {
			t.Errorf("workers %d: the parallel search is reported applied, but nothing was searched (%+v)", workers, par.Parallel)
		}
		if par.Parallel.Reason == "" {
			t.Errorf("workers %d: not applied and no reason", workers)
		}
	}
}
