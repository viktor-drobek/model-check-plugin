package explore

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"modelcheck/ir"
)

// Step 5 of performance plan 5: the goroutine pool. Many workers give the same
// result as one, whatever the scheduler does; one worker starts no goroutine;
// the pool does not leak; a panic in a worker ends the run with an
// InternalError and does not hang; the time budget stops a run at a group
// boundary and never leaves a half-inserted group.

var parWorkerCounts = []int{1, 2, 3, 8, 64}

func TestParallelManyWorkersGiveTheSameResult(t *testing.T) {
	for _, c := range parFixtures(t) {
		for _, opt := range []Options{{Sweep: true}, {}} {
			want := ""
			for _, kn := range []*parKnobs{nil, {inline: -1}, {inline: -1, segment: 1, group: 5}, {inline: -1, segment: 3}} {
				for _, w := range parWorkerCounts {
					o := opt
					o.Workers, o.par = w, kn
					res := runOpts(t, c.m, o)
					got := resultDigest(res)
					if res.Parallel.Workers != w {
						t.Fatalf("%s: %d workers asked, %d ran", c.name, w, res.Parallel.Workers)
					}
					if kn != nil && kn.group > 0 {
						continue // a forced group size is semantics for a run that stops early
					}
					if want == "" {
						want = got
					} else if got != want {
						t.Fatalf("%s sweep=%v knobs %+v workers %d: result differs:\n--- first\n%s--- this\n%s", c.name, opt.Sweep, kn, w, want, got)
					}
				}
			}
		}
	}
}

// Perturbing the scheduler at every barrier and at every segment changes
// nothing.
func TestParallelResultSurvivesAPerturbedScheduler(t *testing.T) {
	models := []parCase{{"mutex_flaw", mutexFlaw()}, {"counters(5,4)", counters(5, 4)}}
	for seed := int64(1); seed <= 40; seed++ {
		m, _ := randomParModel(rand.New(rand.NewSource(seed)))
		models = append(models, parCase{fmt.Sprintf("random %d", seed), m})
	}
	for _, c := range models {
		base := Options{Sweep: true, Budget: Budget{MaxStates: 20000, MaxDepth: 5000}}
		ref := base
		ref.Workers = 1
		want := resultDigest(runOpts(t, c.m, ref))
		for _, w := range []int{3, 8} {
			o := base
			o.Workers = w
			var n atomic.Int64
			o.par = &parKnobs{inline: -1, segment: 2, hook: func(stage string, _ int) {
				switch n.Add(1) % 3 {
				case 0:
					runtime.Gosched()
				case 1:
					time.Sleep(time.Microsecond * time.Duration(n.Load()%7))
				}
			}}
			if got := resultDigest(runOpts(t, c.m, o)); got != want {
				t.Fatalf("%s with %d workers and a perturbed scheduler:\n--- one worker\n%s--- perturbed\n%s", c.name, w, want, got)
			}
		}
	}
}

// With one worker the parallel algorithm runs on the calling goroutine: no
// goroutine is started, which is also what makes a data race impossible there.
func TestParallelOneWorkerStartsNoGoroutine(t *testing.T) {
	before := runtime.NumGoroutine()
	var most atomic.Int64
	kn := &parKnobs{inline: -1, segment: 2, hook: func(string, int) {
		if g := int64(runtime.NumGoroutine()); g > most.Load() {
			most.Store(g)
		}
	}}
	runOpts(t, counters(5, 4), Options{Workers: 1, Sweep: true, par: kn})
	if most.Load() > int64(before) {
		t.Fatalf("%d goroutines during a one-worker run, %d before it", most.Load(), before)
	}
}

// The helper goroutines of a run end with it, also when the run ends early.
func TestParallelPoolDoesNotLeakGoroutines(t *testing.T) {
	settle := func() int {
		for i := 0; i < 50; i++ {
			runtime.GC()
			time.Sleep(2 * time.Millisecond)
		}
		return runtime.NumGoroutine()
	}
	before := settle()
	var during atomic.Int64
	kn := &parKnobs{inline: -1, segment: 2, hook: func(string, int) {
		if g := int64(runtime.NumGoroutine()); g > during.Load() {
			during.Store(g)
		}
	}}
	for _, m := range []*ir.Model{counters(5, 4), mutexFlaw(), petrinet1()} {
		for _, opt := range []Options{{Sweep: true}, {}} {
			opt.Workers, opt.par = 6, kn
			runOpts(t, m, opt)
		}
	}
	if during.Load() < int64(before)+5 {
		t.Fatalf("%d goroutines during a six-worker run (%d before): the pool was not started", during.Load(), before)
	}
	if after := settle(); after > before {
		t.Fatalf("%d goroutines after the runs, %d before: the pool leaks", after, before)
	}
}

// A panic in a worker (an engine defect) is recovered, counts as the worker's
// arrival at the barrier, and ends the run with an InternalError: the call
// returns, there is no verdict, and no goroutine is left.
func TestParallelAPanicInAWorkerEndsTheRunWithAnInternalError(t *testing.T) {
	before := runtime.NumGoroutine()
	for _, stage := range []string{"expand-mid", "insert-mid"} {
		for _, w := range []int{1, 2, 8} {
			var once sync.Once
			kn := &parKnobs{inline: -1, segment: 2, hook: func(s string, _ int) {
				if s == stage {
					once.Do(func() { panic("injected " + stage) })
				}
			}}
			done := make(chan error, 1)
			go func() {
				_, err := Run(context.Background(), counters(5, 4), Options{Workers: w, Sweep: true, par: kn})
				done <- err
			}()
			select {
			case err := <-done:
				var ie *InternalError
				if !errors.As(err, &ie) || !strings.Contains(ie.Error(), "injected "+stage) {
					t.Fatalf("stage %s, %d workers: error %v", stage, w, err)
				}
			case <-time.After(30 * time.Second):
				t.Fatalf("stage %s, %d workers: the run did not return after a panic in a worker", stage, w)
			}
		}
	}
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	if after := runtime.NumGoroutine(); after > before+1 {
		t.Fatalf("%d goroutines after the panicking runs, %d before", after, before)
	}
}

// The partition set is written by the owner of a partition in the insertion
// phase and by nobody in the expansion phase: a write during the expansion is
// a defect of the search and stops it, not a silent race.
func TestParallelTripwireFiresOnAWriteDuringTheExpansion(t *testing.T) {
	m := counters(5, 4)
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, opt: Options{Sweep: true}, ctx: context.Background(), res: &Result{StateBytes: c.layout.Size}}
	s.initOutcomes()
	r, err := newParRun(s, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.set.reading = true
	defer func() {
		if recover() == nil {
			t.Fatal("an insert during the expansion phase did not panic")
		}
	}()
	v := make([]byte, c.layout.Size)
	r.set.add(parHash(v), v, parNoParent)
}

// A deadline that expires while a group is being expanded discards the group;
// one that expires while it is being inserted lets the insertion finish. The
// run ends at a group boundary either way: the states are the sum of the
// groups that completed, the run is not complete, and no property is verified.
func TestParallelTimeBudgetStopsAtAGroupBoundary(t *testing.T) {
	m := counters(6, 4)
	kn := func(hook func(string, int)) *parKnobs {
		return &parKnobs{inline: -1, segment: 7, group: 40, hook: hook}
	}
	// The boundaries of an uncancelled run: the number of states after each group.
	var boundaries []int
	whole := runOpts(t, m, Options{Workers: 1, Sweep: true, par: kn(func(stage string, states int) {
		if stage == "merge" {
			boundaries = append(boundaries, states)
		}
	})})
	if !whole.Complete || len(boundaries) < 10 {
		t.Fatalf("uncancelled run: complete %v, %d groups", whole.Complete, len(boundaries))
	}
	for _, tc := range []struct {
		stage string
		after int // the group during which the deadline expires (1-based)
		w     int
	}{{"expand-mid", 3, 1}, {"expand-mid", 7, 4}, {"insert-mid", 3, 1}, {"insert-mid", 6, 4}, {"between", 5, 2}, {"insert", 2, 8}} {
		ctx, cancel := context.WithCancel(context.Background())
		var group atomic.Int64
		var once sync.Once
		o := Options{Workers: tc.w, Sweep: true, par: kn(func(stage string, _ int) {
			if stage == "expand" {
				group.Add(1)
			}
			if stage == tc.stage && int(group.Load()) == tc.after {
				once.Do(cancel)
			}
		})}
		res, err := Run(ctx, m, o)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if res.Stop != "time budget exhausted" || res.Complete {
			t.Fatalf("%+v: stop %q complete %v", tc, res.Stop, res.Complete)
		}
		// "expand" is called once per group of the run; a deadline that expires
		// while group k is expanded discards it (the states are those after
		// group k-1), one that expires while it is inserted lets it finish (the
		// states are those after group k), and the same for the checks between.
		want := boundaries[tc.after-1]
		switch tc.stage {
		case "expand-mid", "between":
			want = boundaries[tc.after-2]
		}
		if res.States != want {
			t.Fatalf("%+v: %d states, want %d (boundaries %v)", tc, res.States, want, boundaries[:tc.after])
		}
		for _, o := range res.Outcomes {
			if o.Status == Verified {
				t.Fatalf("%+v: %s is verified on a run cut by the clock", tc, o.Property.ID)
			}
			if o.Status == Inconclusive && o.Evidence != EvUnknown {
				t.Fatalf("%+v: %s evidence %s, want unknown for time", tc, o.Property.ID, o.Evidence)
			}
		}
	}
}

// Many workers over a large model, tiny groups and segments, many levels, and
// a model that touches many partitions per level: the shapes the plan names for
// the race detector.
func TestParallelStress(t *testing.T) {
	models := []parCase{
		{"counters(10,4)", counters(10, 4)},
		{"mutex_flaw", mutexFlaw()},
		{"chain", promelaModel(t, `int c; active proctype P() { do :: c < 3000 -> c++ :: else -> break od }`)},
		{"one state", &ir.Model{Schema: ir.Schema, Name: "one", Processes: []ir.Process{{Name: "P", Locations: locs("l0")}}, Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}}}},
	}
	if testing.Short() {
		models = models[1:2]
	}
	for _, c := range models {
		want := resultDigest(runOpts(t, c.m, Options{Workers: 1, Sweep: true}))
		for _, w := range []int{2, 16, 64} {
			for _, kn := range []*parKnobs{{inline: -1, segment: 1}, {inline: -1, segment: 3, group: 16}, {inline: 1 << 30}, nil} {
				o := Options{Workers: w, Sweep: true, par: kn}
				got := resultDigest(runOpts(t, c.m, o))
				if kn != nil && kn.group > 0 {
					continue
				}
				if got != want {
					t.Fatalf("%s workers %d knobs %+v differ:\n%s\n---\n%s", c.name, w, kn, want, got)
				}
			}
		}
	}
}

// More workers than cores shakes out interleavings the scheduler would not
// otherwise choose; the result does not depend on how many cores there are.
func TestParallelResultDoesNotDependOnGOMAXPROCS(t *testing.T) {
	m := mutexFlaw()
	want := resultDigest(runOpts(t, m, Options{Workers: 1, Sweep: true}))
	for _, procs := range []int{1, 2} {
		old := runtime.GOMAXPROCS(procs)
		for _, w := range []int{3, 64} {
			o := Options{Workers: w, Sweep: true, par: &parKnobs{inline: -1, segment: 5}}
			if got := resultDigest(runOpts(t, m, o)); got != want {
				runtime.GOMAXPROCS(old)
				t.Fatalf("GOMAXPROCS %d, %d workers:\n%s\n---\n%s", procs, w, want, got)
			}
		}
		runtime.GOMAXPROCS(old)
	}
}

// A panic of the coordinating goroutine (not of a worker) is also an
// InternalError and not a crash of the process.
func TestParallelAPanicOfTheCoordinatorIsAnInternalError(t *testing.T) {
	kn := &parKnobs{inline: -1, hook: func(stage string, _ int) {
		if stage == "merge" {
			panic("injected in the coordinator")
		}
	}}
	_, err := Run(context.Background(), counters(5, 3), Options{Workers: 3, Sweep: true, par: kn})
	var ie *InternalError
	if !errors.As(err, &ie) || !strings.Contains(ie.Error(), "injected in the coordinator") {
		t.Fatalf("error %v", err)
	}
}
