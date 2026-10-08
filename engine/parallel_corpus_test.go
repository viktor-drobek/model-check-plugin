package modelcheck_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"modelcheck/cex"
	"modelcheck/cli"
	"modelcheck/explore"
	"modelcheck/ir"
)

// parallelWorkerCounts are the worker counts the corpus is run with: one (the
// parallel algorithm on the calling goroutine) and several.
var parallelWorkerCounts = []int{1, 8}

// TestParallelAgreesWithTheSequentialSearchesOnTheCorpus runs every Promela
// model of the fixtures and of the SPIN textbook corpus that the frontend
// accepts under the sequential depth-first and breadth-first searches and the
// parallel search, and requires (performance plan 5, A10-A13): for a model
// whose run ends in no error of the model, the same status, evidence and reason
// for every property, the same states, transitions and atomic steps when the
// run completes, the breadth-first depth and layers when there is no atomic
// step, and traces that replay as runs of the model, as short as the
// breadth-first ones where there is none; for a model with a temporal
// property the parallel search is refused and the run is the sequential one,
// byte for byte; and the same result for every worker count.
func TestParallelAgreesWithTheSequentialSearchesOnTheCorpus(t *testing.T) {
	var files []string
	for _, root := range []string{"testdata/promela", "testdata/corpus2", corpusDir} {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".pml") {
				files = append(files, path)
			}
			return nil
		})
	}
	sort.Strings(files)
	if len(files) < 50 {
		t.Skipf("only %d Promela files found", len(files))
	}
	var parsed, compared, refused, ending, complete, violated, atomic int
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, rej := cli.ParsePromela(src, f, nil, 0)
		if rej != nil {
			continue // outside the supported subset
		}
		parsed++
		run := func(opt explore.Options) *explore.Result {
			opt.Sweep, opt.Defines = true, p.Defines
			opt.Budget = explore.Budget{MaxStates: 200000}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			r, err := explore.Run(ctx, p.Model, opt)
			if err != nil {
				return nil
			}
			return r
		}
		dfs := run(explore.Options{})
		par := make([]*explore.Result, len(parallelWorkerCounts))
		for i, w := range parallelWorkerCounts {
			par[i] = run(explore.Options{Workers: w})
		}
		if dfs == nil || par[0] == nil {
			continue
		}
		for i := 1; i < len(par); i++ {
			if par[i] == nil || digest(par[i]) != digest(par[0]) {
				t.Errorf("%s: the result differs between %d and %d workers", f, parallelWorkerCounts[0], parallelWorkerCounts[i])
			}
		}
		hasTemporal := false
		for _, o := range dfs.Outcomes {
			if o.Property.Kind == ir.KindLTL || o.Property.Kind == ir.KindProgress || o.Property.Kind == ir.KindCTL {
				hasTemporal = true
			}
		}
		if hasTemporal {
			refused++
			for i, r := range par {
				if r.Parallel == nil || r.Parallel.Applied || r.Parallel.Reason == "" {
					t.Errorf("%s: a temporal property and the parallel search was not refused: %+v", f, r.Parallel)
					continue
				}
				// The run is the sequential one, byte for byte.
				if got, want := digest(withoutParallel(r)), digest(dfs); got != want {
					t.Errorf("%s (%d workers): the refused run differs from the sequential one:\n%s\n---\n%s", f, parallelWorkerCounts[i], got, want)
				}
			}
			continue
		}
		hasAtomic := false
		for _, pr := range p.Model.Processes {
			for _, e := range pr.Edges {
				hasAtomic = hasAtomic || e.Atomic
			}
		}
		bfs := run(explore.Options{Mode: explore.BFS})
		if bfs == nil {
			continue
		}
		skip := func(r *explore.Result) bool { return strings.Contains(r.Stop, "budget") }
		if skip(dfs) || skip(bfs) || skip(par[0]) {
			// A run that stopped on a budget says nothing: the orders may differ.
			if par[0].Complete {
				t.Errorf("%s: the parallel run is complete although a budget stopped a sequential one", f)
			}
			continue
		}
		compared++
		ends := dfs.Stop == "invalid model" || bfs.Stop == "invalid model" || par[0].Stop == "invalid model"
		if ends {
			ending++
			if par[0].Complete {
				t.Errorf("%s: complete with an ending event", f)
			}
			for _, o := range par[0].Outcomes {
				if o.Status == explore.Verified && o.Property.Kind != ir.KindReach {
					t.Errorf("%s: property %s verified on a run that ended on %q", f, o.Property.ID, par[0].Stop)
				}
			}
		} else {
			if par[0].Stop != dfs.Stop || par[0].Complete != dfs.Complete {
				t.Errorf("%s: stop %q (complete %v), sequential %q (%v)", f, par[0].Stop, par[0].Complete, dfs.Stop, dfs.Complete)
			}
			if par[0].Complete {
				complete++
				if par[0].States != dfs.States || par[0].Transitions != dfs.Transitions || par[0].AtomicSteps != dfs.AtomicSteps {
					t.Errorf("%s: states/transitions/atomic %d/%d/%d, sequential %d/%d/%d", f, par[0].States, par[0].Transitions, par[0].AtomicSteps, dfs.States, dfs.Transitions, dfs.AtomicSteps)
				}
				if fmt.Sprint(par[0].Coverage) != fmt.Sprint(dfs.Coverage) {
					t.Errorf("%s: coverage %v, sequential %v", f, par[0].Coverage, dfs.Coverage)
				}
				if !hasAtomic {
					if par[0].MaxDepth != bfs.MaxDepth || fmt.Sprint(par[0].Levels) != fmt.Sprint(bfs.Levels) {
						t.Errorf("%s: depth %d, breadth-first %d (levels %d against %d)", f, par[0].MaxDepth, bfs.MaxDepth, len(par[0].Levels), len(bfs.Levels))
					}
				} else if par[0].MaxDepth > bfs.MaxDepth || par[0].MaxDepth > dfs.MaxDepth {
					t.Errorf("%s: depth %d above the sequential ones (%d, %d)", f, par[0].MaxDepth, dfs.MaxDepth, bfs.MaxDepth)
				}
			}
			for i := range par[0].Outcomes {
				a, d, b := par[0].Outcomes[i], dfs.Outcomes[i], bfs.Outcomes[i]
				if a.Status != d.Status || a.Evidence != d.Evidence || a.Status != b.Status || a.Evidence != b.Evidence {
					t.Errorf("%s: property %s is %s/%s in parallel, %s/%s depth-first, %s/%s breadth-first", f, a.Property.ID, a.Status, a.Evidence, d.Status, d.Evidence, b.Status, b.Evidence)
				}
				if a.Status == explore.Violated {
					violated++
				}
				if reasonOf(a) != reasonOf(d) {
					t.Errorf("%s: property %s reason %q, sequential %q", f, a.Property.ID, a.Reason, d.Reason)
				}
				if a.Trace != nil && !hasAtomic && b.Trace != nil && len(a.Trace.Steps) != len(b.Trace.Steps) {
					t.Errorf("%s: the %s trace has %d steps, the breadth-first one %d", f, a.Property.ID, len(a.Trace.Steps), len(b.Trace.Steps))
				}
			}
		}
		if hasAtomic {
			atomic++
		}
		for _, o := range par[0].Outcomes {
			// A trace through a state whose enabledness cannot be listed (an
			// error of the model) is replayed by the explore oracle, which can
			// list the moves before the error; the Stepper cannot.
			if o.Trace != nil && !ends {
				if err := replayTrace(p.Model, o.Trace, o.Status == explore.InvalidModel); err != nil {
					t.Errorf("%s: the %s trace of the parallel run does not replay: %v\n%s", f, o.Property.ID, err, o.Trace.Summary)
				}
			}
		}
	}
	t.Logf("%d Promela files accepted: %d compared (%d complete, %d with an ending event, %d with atomic steps), %d refused for a temporal property; %d violated properties replayed", parsed, compared, complete, ending, atomic, refused, violated)
	if compared < 30 || complete < 20 || refused < 5 {
		t.Fatalf("the corpus is not being read: %d compared, %d complete, %d refused", compared, complete, refused)
	}
}

func reasonOf(o explore.Outcome) string {
	if o.Status == explore.Violated && o.Property.Kind == ir.KindAssert {
		return "assert violated" // which of several failing asserts the order meets first
	}
	return o.Reason
}

// digest is what the report shows of a run, apart from its time.
func digest(r *explore.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d/%d/%d/%d/%v/%q/%d/%v\n", r.States, r.Transitions, r.AtomicSteps, r.MaxDepth, r.Complete, r.Stop, r.MemBytes, r.Levels)
	for _, o := range r.Outcomes {
		steps := -1
		summary := ""
		if o.Trace != nil {
			steps, summary = len(o.Trace.Steps), o.Trace.Summary
		}
		fmt.Fprintf(&b, "%s|%s|%s|%s|%d|%s\n", o.Property.ID, o.Status, o.Evidence, o.Reason, steps, summary)
	}
	return b.String()
}

// withoutParallel is a copy of r without its Parallel record, for the
// comparison of a refused run with the sequential run it is.
func withoutParallel(r *explore.Result) *explore.Result {
	c := *r
	c.Parallel = nil
	return &c
}

// replayTrace checks that tr is a run of m from its initial state: every step
// an enabled move, in the explorer's order of moves, edges with the same text
// tried in turn. With mayFail the last step of an invalid-model trace may be
// the step that failed.
func replayTrace(m *ir.Model, tr *cex.Trace, mayFail bool) error {
	st, err := explore.NewStepper(m)
	if err != nil {
		return err
	}
	n := len(tr.Steps)
	dead := map[string]bool{}
	var walk func(i int, state []byte) error
	walk = func(i int, state []byte) error {
		if i == n || (mayFail && i == n-1) {
			return nil
		}
		key := fmt.Sprintf("%d|%x", i, state)
		if dead[key] {
			return fmt.Errorf("step %d: no continuation from this state", i+1)
		}
		moves, _ := st.Enabled(state) // moves before an evaluation error are still moves
		last := fmt.Errorf("step %d (%s: %s) is not an enabled move", i+1, tr.Steps[i].Process, tr.Steps[i].Command)
		for _, mv := range moves {
			if m.Processes[mv.Edge.Proc].Name != tr.Steps[i].Process || cex.CommandText(st.Edge(mv.Edge)) != tr.Steps[i].Command {
				continue
			}
			next, _, err := st.Apply(state, mv)
			if err != nil {
				continue
			}
			if err := walk(i+1, next); err == nil {
				return nil
			} else {
				last = err
			}
		}
		dead[key] = true
		return last
	}
	return walk(0, st.Initial())
}
