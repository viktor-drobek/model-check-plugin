package explore

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"modelcheck/ir"
)

// Step 6 of performance plan 5: budgets. The state budget is exact (the run
// stores the budget and not one state more, and no event after the stop point
// counts), the depth budget cuts at a layer, the memory budget is checked on an
// estimate that is a function of the run, and a group whose records would not
// fit is redone with half the states. Every one of them is independent of the
// worker count, the segment size and the inline threshold.

func TestParallelStateBudgetIsExact(t *testing.T) {
	for _, c := range parFixtures(t) {
		full := runOpts(t, c.m, Options{Workers: 1, Sweep: true})
		if full.Stop == "invalid model" {
			continue
		}
		total := full.States
		for _, n := range []int{1, 2, 5, 17, 100, total / 2, total - 1, total, total + 1} {
			if n < 1 {
				continue
			}
			var want string
			for i, kn := range []*parKnobs{nil, {inline: -1}, {inline: -1, segment: 1}, {inline: 1 << 30}, {inline: -1, segment: 3}} {
				for _, w := range []int{1, 3} {
					res := runOpts(t, c.m, Options{Workers: w, Sweep: true, par: kn, Budget: Budget{MaxStates: n}})
					if n < total {
						if res.States != n || res.Complete || !strings.HasPrefix(res.Stop, "state budget exhausted") && res.Stop != "invalid model" {
							t.Fatalf("%s budget %d (total %d) knobs %+v: states %d stop %q complete %v", c.name, n, total, kn, res.States, res.Stop, res.Complete)
						}
						if res.Stop == fmt.Sprintf("state budget exhausted: %d states stored", n) {
							for _, o := range res.Outcomes {
								if o.Status == Verified && o.Property.Kind != ir.KindReach {
									t.Fatalf("%s budget %d: %s is verified on a truncated run", c.name, n, o.Property.ID)
								}
								if o.Status == Inconclusive && o.Evidence != Bounded {
									t.Fatalf("%s budget %d: %s inconclusive with evidence %s", c.name, n, o.Property.ID, o.Evidence)
								}
							}
						}
					} else if res.States != total || !res.Complete {
						t.Fatalf("%s budget %d (total %d): states %d complete %v", c.name, n, total, res.States, res.Complete)
					}
					got := resultDigest(res)
					if i == 0 && w == 1 {
						want = got
					} else if got != want {
						t.Fatalf("%s budget %d knobs %+v workers %d differ:\n--- first\n%s--- this\n%s", c.name, n, kn, w, want, got)
					}
				}
			}
		}
	}
}

// An event after the stop point of the state budget is not claimed (scenario
// 23): the assert fails in a state that the budget never lets the search store.
func TestParallelStateBudgetDropsEventsAfterTheStopPoint(t *testing.T) {
	// A counts to 4; B counts to 4; the assert on A's last step fails.
	m := promelaModel(t, `byte a; byte b;
active proctype A() { do :: a < 4 -> a++ :: a == 4 -> assert(0); break od }
active proctype B() { do :: b < 4 -> b++ :: else -> break od }`)
	full := runOpts(t, m, Options{Workers: 1, Sweep: true})
	if outcome(t, full, "assert").Status != Violated {
		t.Fatal("the assert is not violated")
	}
	seenViolated, seenCut := false, false
	for n := 1; n < full.States; n++ {
		for _, kn := range parKnobSets {
			res := runOpts(t, m, Options{Workers: 1, Sweep: true, par: kn, Budget: Budget{MaxStates: n}})
			a := outcome(t, res, "assert")
			if a.Status == Violated {
				seenViolated = true
				if err := replayOutcome(m, *a, nil); err != nil {
					t.Fatalf("budget %d: %v", n, err)
				}
			} else {
				seenCut = true
				if a.Status != Inconclusive {
					t.Fatalf("budget %d: assert %s", n, a.Status)
				}
			}
		}
	}
	if !seenViolated || !seenCut {
		t.Fatalf("violated for some budget: %v, cut for some: %v", seenViolated, seenCut)
	}
}

func TestParallelDepthBudgetCutsAtALayer(t *testing.T) {
	for _, c := range []parCase{{"counters(5,4)", counters(5, 4)}, {"mutex_flaw", mutexFlaw()}, {"petrinet1", petrinet1()}} {
		bfsFull := runOpts(t, c.m, Options{Sweep: true, Mode: BFS})
		last := bfsFull.MaxDepth
		for d := 1; d <= last+2; d++ { // 0 is no budget
			bfs := runOpts(t, c.m, Options{Sweep: true, Mode: BFS, Budget: Budget{MaxDepth: d}})
			var want string
			for i, kn := range []*parKnobs{nil, {inline: -1, segment: 2}, {inline: 1 << 30}} {
				for _, w := range []int{1, 4} {
					res := runOpts(t, c.m, Options{Workers: w, Sweep: true, par: kn, Budget: Budget{MaxDepth: d}})
					// The sentence and the counters are the breadth-first search's: a layer
					// counts one hop, and the model has no atomic sequence.
					if res.Stop != bfs.Stop || res.Complete != bfs.Complete || res.MaxDepth != bfs.MaxDepth || res.States != bfs.States {
						t.Fatalf("%s depth budget %d: parallel stop %q states %d depth %d complete %v; breadth-first %q %d %d %v",
							c.name, d, res.Stop, res.States, res.MaxDepth, res.Complete, bfs.Stop, bfs.States, bfs.MaxDepth, bfs.Complete)
					}
					if fmt.Sprint(res.Levels) != fmt.Sprint(bfs.Levels) {
						t.Fatalf("%s depth budget %d: levels %v, breadth-first %v", c.name, d, res.Levels, bfs.Levels)
					}
					if d < last {
						for _, o := range res.Outcomes {
							if o.Status == Verified && o.Property.Kind != ir.KindReach {
								t.Fatalf("%s depth budget %d: %s verified on a cut run", c.name, d, o.Property.ID)
							}
							if o.Status == Inconclusive && o.Evidence != Bounded {
								t.Fatalf("%s depth budget %d: %s evidence %s", c.name, d, o.Property.ID, o.Evidence)
							}
						}
					}
					got := resultDigest(res)
					if i == 0 && w == 1 {
						want = got
					} else if got != want {
						t.Fatalf("%s depth budget %d knobs %+v workers %d differ", c.name, d, kn, w)
					}
				}
			}
		}
	}
}

// The layer D+1 is stored and not expanded, but its states are still checked:
// a violation there is found (scenario 27).
func TestParallelDepthBudgetStillChecksTheStoredLayer(t *testing.T) {
	m := porVisible() // x = 2 happens at the second step of A: layer 2
	res := runOpts(t, m, Options{Workers: 1, Sweep: true, Budget: Budget{MaxDepth: 1}})
	if res.Complete || !strings.HasPrefix(res.Stop, "depth budget exhausted") {
		t.Fatalf("stop %q", res.Stop)
	}
	if inv := outcome(t, res, "inv"); inv.Status != Violated {
		t.Fatalf("the invariant is %s: the stored layer 2 was not checked", inv.Status)
	}
}

func TestParallelMemoryBudgetStopsOnTheEstimate(t *testing.T) {
	m := counters(10, 4)
	whole := runOpts(t, m, Options{Workers: 1, Sweep: true})
	if whole.MemBytes <= 0 {
		t.Fatal("no estimate")
	}
	for _, frac := range []int{2, 4, 10} {
		budget := whole.MemBytes / int64(frac)
		var want string
		for i, kn := range []*parKnobs{nil, {inline: -1, segment: 5}, {inline: 1 << 30}} {
			for _, w := range []int{1, 8} {
				res := runOpts(t, m, Options{Workers: w, Sweep: true, par: kn, Budget: Budget{MaxMemBytes: budget}})
				if res.Complete || !strings.HasPrefix(res.Stop, "memory budget exhausted") {
					t.Fatalf("budget %d: stop %q complete %v", budget, res.Stop, res.Complete)
				}
				for _, o := range res.Outcomes {
					if o.Status == Verified || (o.Status == Inconclusive && o.Evidence != EvUnknown) {
						t.Fatalf("%s: %s/%s on a run cut by memory", o.Property.ID, o.Status, o.Evidence)
					}
				}
				got := resultDigest(res)
				if i == 0 && w == 1 {
					want = got
				} else if got != want {
					t.Fatalf("budget %d knobs %+v workers %d differ:\n%s\n---\n%s", budget, kn, w, want, got)
				}
			}
		}
	}
	// The estimate is a function of the run: the same for every worker count and
	// every segment size, with no run-time capacity in it.
	a := runOpts(t, m, Options{Workers: 1, Sweep: true}).MemBytes
	b := runOpts(t, m, Options{Workers: 7, Sweep: true, par: &parKnobs{inline: -1, segment: 3}}).MemBytes
	if a != b {
		t.Fatalf("estimate %d with one worker, %d with seven", a, b)
	}
}

// A group whose records would not fit in the room the memory budget leaves is
// cut after the longest prefix of its states whose records do fit, so a run
// with an abrupt fan-out and a small memory budget returns a report, the same
// for every worker count, segment size and inline threshold (scenarios 21 and
// 31). The test checks that the group really was cut: the run has more groups
// than levels.
func TestParallelRecordGuardCutsAnOverflowedGroup(t *testing.T) {
	for _, assertFails := range []bool{false, true} {
		m := fanOutModel(2000, assertFails)
		for _, mem := range []int64{3 << 20, 6 << 20} {
			var want string
			for i, kn := range []*parKnobs{nil, {inline: -1, segment: 4}, {inline: 1 << 30}} {
				for _, w := range []int{1, 8} {
					groups, levels := 0, 0
					var base *parKnobs
					if kn != nil {
						base = &parKnobs{}
						*base = *kn
					} else {
						base = &parKnobs{}
					}
					base.hook = func(stage string, _ int) {
						switch stage {
						case "merge":
							groups++
						case "level":
							levels++
						}
					}
					res := runOpts(t, m, Options{Workers: w, Sweep: true, par: base, Budget: Budget{MaxMemBytes: mem}})
					got := resultDigest(res)
					if res.Complete && groups <= levels {
						t.Fatalf("assertFails=%v mem=%d knobs %+v workers %d: %d groups over %d levels, so no group was cut", assertFails, mem, kn, w, groups, levels)
					}
					if i == 0 && w == 1 {
						want = got
						t.Logf("assertFails=%v mem=%d: stop %q states %d complete %v est %d, %d groups over %d levels", assertFails, mem, res.Stop, res.States, res.Complete, res.MemBytes, groups, levels)
					} else if got != want {
						t.Fatalf("assertFails=%v mem=%d knobs %+v workers %d differ:\n%s\n---\n%s", assertFails, mem, kn, w, want, got)
					}
					for _, o := range res.Outcomes {
						if o.Status == Verified && !res.Complete {
							t.Fatalf("%s verified on an incomplete run", o.Property.ID)
						}
					}
				}
			}
		}
	}
	// One state whose records alone do not fit ends the run with the sentence.
	m := fanOutModel(5000, false)
	res := runOpts(t, m, Options{Workers: 4, Sweep: true, Budget: Budget{MaxMemBytes: 100 << 10}})
	if res.Complete || !strings.HasPrefix(res.Stop, "memory budget exhausted: the expansion of one state needs") {
		t.Fatalf("stop %q", res.Stop)
	}
	for _, o := range res.Outcomes {
		if o.Status != Inconclusive || o.Evidence != EvUnknown {
			t.Fatalf("%s: %s/%s", o.Property.ID, o.Status, o.Evidence)
		}
	}
}

// The groups, and so every stop point, are the same for any worker count,
// segment size and inline threshold (mutants 28 and 40): the number of states
// stored at the end of each group is. The last model has layers wider than the
// group size a worker count of 64 would give if the size depended on it.
func TestParallelGroupsDoNotDependOnWorkersOrSegments(t *testing.T) {
	for _, m := range []*ir.Model{counters(8, 4), mutexFlaw(), fanOutModel(60, false), counters(10, 5), counters(6, 7)} {
		var want string
		for i, tc := range []struct {
			w  int
			kn *parKnobs
		}{{1, nil}, {1, &parKnobs{inline: -1}}, {3, &parKnobs{inline: -1, segment: 1}}, {8, &parKnobs{inline: -1, segment: 9}}, {64, &parKnobs{inline: 1 << 30}}} {
			var bounds []int
			kn := &parKnobs{}
			if tc.kn != nil {
				*kn = *tc.kn
			}
			kn.hook = func(stage string, states int) {
				if stage == "merge" {
					bounds = append(bounds, states)
				}
			}
			runOpts(t, m, Options{Workers: tc.w, Sweep: true, par: kn})
			got := fmt.Sprint(bounds)
			if i == 0 {
				want = got
				if len(bounds) < 2 {
					t.Fatalf("only %d groups", len(bounds))
				}
			} else if got != want {
				t.Fatalf("workers %d knobs %+v: group boundaries %s, want %s", tc.w, tc.kn, got, want)
			}
		}
	}
}

// The budgets of a random run are the same for every worker count: states,
// depth and memory budgets chosen at random, truncated runs included (A11).
func TestParallelBudgetedRunsAreTheSameForEveryWorkerCount(t *testing.T) {
	models, first := parModelCount(600)
	var cut int
	for seed := first; seed < first+int64(models); seed++ {
		rnd := rand.New(rand.NewSource(seed))
		m, watch := randomParModel(rnd)
		bud := Budget{MaxDepth: 20000}
		switch rnd.Intn(3) {
		case 0:
			bud.MaxStates = 1 + rnd.Intn(300)
		case 1:
			bud.MaxDepth = 1 + rnd.Intn(12)
		default:
			bud.MaxMemBytes = int64(20000 + rnd.Intn(200000))
		}
		base := Options{Sweep: rnd.Intn(2) == 0, Budget: bud, Watch: watch}
		var want string
		var wantRes *Result
		for i, tc := range []struct {
			w  int
			kn *parKnobs
		}{{1, nil}, {2, &parKnobs{inline: -1, segment: 2}}, {5, &parKnobs{inline: 1 << 30}}, {16, &parKnobs{inline: -1, segment: 1}}} {
			o := base
			o.Workers, o.par = tc.w, tc.kn
			res := runOpts(t, m, o)
			got := resultDigest(res)
			if i == 0 {
				want, wantRes = got, res
			} else if got != want {
				t.Fatalf("seed %d budget %+v workers %d knobs %+v differ:\n--- one worker\n%s--- this\n%s", seed, bud, tc.w, tc.kn, want, got)
			}
		}
		if !wantRes.Complete {
			cut++
			for _, o := range wantRes.Outcomes {
				if o.Status == Verified && o.Property.Kind != ir.KindReach {
					t.Fatalf("seed %d: %s verified on an incomplete run (%s)", seed, o.Property.ID, wantRes.Stop)
				}
			}
			// A violation found before the stop point stands: its trace replays.
			for _, o := range wantRes.Outcomes {
				if o.Trace != nil && endingKindOf(wantRes) == "" {
					if err := replayOutcome(m, o, nil); err != nil {
						t.Fatalf("seed %d: the %s trace does not replay: %v", seed, o.Property.ID, err)
					}
				}
			}
		}
	}
	if cut < models/4 {
		t.Fatalf("only %d of %d runs were cut by a budget", cut, models)
	}
}

func endingKindOf(r *Result) string {
	if r.Stop == "invalid model" {
		return "invalid"
	}
	return ""
}

// The records a worker writes are added to the group's shared total in batches
// and at the end of a segment. A segment whose records are fewer than a batch is
// accounted only by that last flush, so a group whose records do not fit in a
// small room must still be flagged (mutant: no flush at the end of a segment).
func TestParallelTheEndOfASegmentFlushesTheRecordBytes(t *testing.T) {
	m := counters(5, 3)
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, ctx: context.Background(), res: &Result{StateBytes: c.layout.Size}}
	s.initOutcomes()
	r, err := newParRun(s, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.refreshDecided()
	w := r.workers[0]
	init := c.layout.Initial()
	h := parHash(init)
	w.store(h, init, parNoParent, 0)
	r.rcap = 1 // no record fits
	r.usedRecs.Store(0)
	r.overflow.Store(false)
	w.expandSegment(&parSeg{part: parPartOf(h), lo: 0, hi: 1})
	if !r.overflow.Load() {
		t.Fatalf("%d record bytes written against a room of %d, and the group is not flagged as overflowed", r.usedRecs.Load(), r.rcap)
	}
}
