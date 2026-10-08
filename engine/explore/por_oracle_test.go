package explore

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"modelcheck/ir"
)

// The oracles of the partial-order reduction (performance plan, step 6,
// section 7): O1 here (the verdict differential), O2 in por_audit_test.go (the
// semantic audit of the ample sets), O3 in por_acyclic_test.go (the cycle
// proviso as a property of the reduced graph). They share the generators of
// por_gen_test.go and the switches below, which size a run.
//
//	MCD_POR_MODELS   models per generator (the default is small: go test must
//	                 stay quick; the release bar is 300 000 per generator)
//	MCD_POR_SEED     first seed (the default seeds are not the ones to quote)
//	MCD_POR_GEN      comma-separated generator names (see porGenerators)
//	MCD_POR_WORKERS  goroutines (default 1; the box is shared)
//	MCD_POR_ALL      keep going after a failure and count them all
//	MCD_POR_AUDIT_K  macro-steps of the other processes the audit tries in a
//	                 row (default 2; 3 is used for the small models only)

// porOutcome classifies one model of an oracle run.
type porOutcome int

const (
	porRefused   porOutcome = iota // the reduction was not applied; the counts were equal
	porSame                        // applied, no state saved
	porSmaller                     // applied, fewer states
	porBothError                   // both searches stop on an error of the model
	porSkipped                     // a budget ran out in one of them: nothing is compared
	porUncounted                   // an audit that keeps its own counts
)

// porTally counts what an oracle run saw.
type porTally struct {
	models, refused, same, smaller, bothErr, skipped int
	audited                                          int // (state, process) pairs the audit looked at
	atRun                                            int // of them, the process stands at a run edge
	checked                                          int // models the acyclicity audit looked at
	failures                                         int
}

func (a *porTally) add(b porTally) {
	a.models += b.models
	a.refused += b.refused
	a.same += b.same
	a.smaller += b.smaller
	a.bothErr += b.bothErr
	a.skipped += b.skipped
	a.audited += b.audited
	a.atRun += b.atRun
	a.checked += b.checked
	a.failures += b.failures
}

func (a *porTally) note(o porOutcome) {
	switch o {
	case porRefused:
		a.refused++
	case porSame:
		a.same++
	case porSmaller:
		a.smaller++
	case porBothError:
		a.bothErr++
	case porSkipped:
		a.skipped++
	}
}

func (a porTally) String() string {
	out := fmt.Sprintf("%d models", a.models)
	for _, c := range []struct {
		n    int
		what string
	}{
		{a.refused, "refused"},
		{a.same + a.smaller, "applied"},
		{a.smaller, "smaller"},
		{a.bothErr, "error in both"},
		{a.skipped, "skipped"},
		{a.audited, "audited"},
		{a.atRun, "audited at a run"},
		{a.checked, "checked for cycles"},
	} {
		if c.n > 0 {
			out += fmt.Sprintf(", %d %s", c.n, c.what)
		}
	}
	return out + fmt.Sprintf(", %d failures", a.failures)
}

func envInt(name string) (int64, bool) {
	v, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	return v, err == nil && v > 0
}

// porGeneratorsToRun is the generators a test covers, filtered by MCD_POR_GEN.
func porGeneratorsToRun(only ...string) []porGen {
	want := map[string]bool{}
	if v := os.Getenv("MCD_POR_GEN"); v != "" {
		for _, n := range strings.Split(v, ",") {
			want[strings.TrimSpace(n)] = true
		}
	}
	var out []porGen
	for _, g := range porGenerators {
		if len(want) > 0 && !want[g.name] {
			continue
		}
		if len(only) > 0 {
			found := false
			for _, o := range only {
				found = found || o == g.name
			}
			if !found {
				continue
			}
		}
		out = append(out, g)
	}
	return out
}

// porForSeeds runs fn on the models of generator g for the seeds of the
// current scale and reports the tally. fn returns the outcome of the model and
// an error for a disagreement; it may add to the tally it is given. A failure
// stops the run unless MCD_POR_ALL is set.
func porForSeeds(t *testing.T, layer string, g porGen, defaultModels int, fn func(m *ir.Model, tl *porTally) (porOutcome, error)) porTally {
	t.Helper()
	models := defaultModels
	if testing.Short() {
		models = max(1, models/8)
	}
	if v, ok := envInt("MCD_POR_MODELS"); ok {
		models = int(v)
	}
	first := g.first
	if v, ok := envInt("MCD_POR_SEED"); ok {
		first = v
	}
	workers := 1
	if v, ok := envInt("MCD_POR_WORKERS"); ok {
		workers = int(v)
	}
	all := os.Getenv("MCD_POR_ALL") != ""

	var (
		mu     sync.Mutex
		total  porTally
		shown  int
		stop   atomic.Bool
		nextAt atomic.Int64
	)
	nextAt.Store(first)
	end := first + int64(models)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var tl porTally
			for !stop.Load() {
				seed := nextAt.Add(1) - 1
				if seed >= end {
					break
				}
				m := g.gen(newRand(seed))
				tl.models++
				o, err := fn(m, &tl)
				if err != nil {
					tl.failures++
					mu.Lock()
					if shown < 3 {
						shown++
						t.Errorf("%s [%s] seed %d: %v", layer, g.name, seed, err)
					}
					mu.Unlock()
					if !all {
						stop.Store(true)
					}
					continue
				}
				tl.note(o)
			}
			mu.Lock()
			total.add(tl)
			mu.Unlock()
		}()
	}
	wg.Wait()
	t.Logf("por %s [%s]: %s", layer, g.name, total)
	return total
}

func errorStop(r *Result) bool {
	return r.Stop == "invalid model" || strings.HasPrefix(r.Stop, "process budget")
}

// otherBudget: a limit that the search chose, not an error of the model. The
// process budget is an error of the model in the sense of the comparison: a
// `run` that finds its pool exhausted is reachable in both searches or in
// neither.
func otherBudget(r *Result) bool {
	return strings.Contains(r.Stop, "budget") && !strings.HasPrefix(r.Stop, "process budget")
}

// porOracleBudget is what an oracle run of one model may use.
var porOracleBudget = Budget{MaxStates: 5000, MaxDepth: 3000}

// porDifferential is O1: m is checked in full and with the reduction, and the
// two must agree on
//
//   - whether an error of the model is reachable (an evaluation error, a
//     domain overflow, an exhausted process pool), compared before any
//     completeness skip, because an error ends the search;
//   - the status and the evidence of every property;
//   - the set of stored states without an enabled move, up to the exclusive
//     byte (two orders of commuting steps can leave it differently, and the
//     two states are the same state: Lemma E of the plan);
//   - the stored states of the reduced search, which must all be states of the
//     full one (exactly);
//   - the counts, which must be equal when the reduction is refused and never
//     larger when it is applied;
//
// and every counterexample and witness of the reduced search must replay as a
// run of the model.
func porDifferential(m *ir.Model) (porOutcome, error) { return porDifferentialWith(m, 0, 0) }

// porDifferentialWith is porDifferential with the limits of the proviso's chain
// walk given (zero: the defaults). The defaults are never reached by a random
// model; tight ones make exhausting them an everyday event, and the answer to
// exhaustion must be a full expansion, which no verdict can tell from the
// unreduced search.
func porDifferentialWith(m *ir.Model, chainLimit, pickBudget int) (porOutcome, error) {
	opt := Options{Sweep: true, Budget: porOracleBudget}
	var fullRec, redRec *recorder
	opt.NewVisited = func(n int) Visited { fullRec = &recorder{Visited: defaultVisited(n)}; return fullRec }
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	full, err := Run(ctx, m, opt)
	if err != nil {
		return 0, fmt.Errorf("full search: %w", err)
	}
	opt.POR, opt.porChainLimit, opt.porPickBudget = true, chainLimit, pickBudget
	opt.NewVisited = func(n int) Visited { redRec = &recorder{Visited: defaultVisited(n)}; return redRec }
	red, err := Run(ctx, m, opt)
	if err != nil {
		return 0, fmt.Errorf("reduced search: %w", err)
	}
	if red.Reduction == nil {
		return 0, fmt.Errorf("no reduction record")
	}
	// An evaluation error or a domain overflow ends the search ("invalid
	// model") and leaves it incomplete, so whether an error is reachable has
	// to be compared before the completeness skip. Runs that stopped on a
	// budget of their own say nothing: the other may have reached the error
	// first.
	if !otherBudget(full) && !otherBudget(red) {
		if errorStop(full) != errorStop(red) {
			return 0, fmt.Errorf("an error is reachable with the reduction: %v (%q), without: %v (%q)", errorStop(red), red.Stop, errorStop(full), full.Stop)
		}
		if errorStop(full) {
			return porBothError, nil
		}
	}
	if !full.Complete || !red.Complete {
		return porSkipped, nil
	}
	outcome := porSame
	if !red.Reduction.Applied {
		outcome = porRefused
		if red.States != full.States || red.Transitions != full.Transitions {
			return 0, fmt.Errorf("refused (%s) but %d/%d states/transitions against %d/%d", red.Reduction.Reason, red.States, red.Transitions, full.States, full.Transitions)
		}
	} else if red.States < full.States {
		outcome = porSmaller
	}
	if red.States > full.States {
		return 0, fmt.Errorf("the reduced graph has %d states, the full one %d", red.States, full.States)
	}
	st, err := NewStepper(m)
	if err != nil {
		return 0, err
	}
	excl := st.Layout().Excl
	canon := func(b []byte) string {
		x := append([]byte(nil), b...)
		x[excl] = 0
		return string(x)
	}
	noMove := func(states [][]byte) map[string]bool {
		out := map[string]bool{}
		for _, s := range states {
			mv, err := st.Enabled(s)
			if err == nil && len(mv) == 0 {
				out[canon(s)] = true
			}
		}
		return out
	}
	fullDead, redDead := noMove(fullRec.states), noMove(redRec.states)
	for k := range fullDead {
		if !redDead[k] {
			return 0, fmt.Errorf("a state without an enabled move of the full graph is missing from the reduced one (%d such states against %d, up to the exclusive byte)", len(fullDead), len(redDead))
		}
	}
	fullSet := map[string]bool{}
	for _, s := range fullRec.states {
		fullSet[string(s)] = true
	}
	for _, s := range redRec.states {
		if !fullSet[string(s)] {
			return 0, fmt.Errorf("the reduced search stored a state the full search never reaches")
		}
	}
	for i := range full.Outcomes {
		a, b := red.Outcomes[i], full.Outcomes[i]
		if a.Status != b.Status || a.Evidence != b.Evidence {
			return 0, fmt.Errorf("property %s is %s/%s with the reduction and %s/%s without\nreduced: %s\nfull: %s\nreduction: %+v",
				a.Property.ID, a.Status, a.Evidence, b.Status, b.Evidence, a.Reason, b.Reason, red.Reduction)
		}
		if a.Trace != nil {
			if err := replay(m, a.Trace); err != nil {
				return 0, fmt.Errorf("the %s trace of the reduced run is not a run of the model: %v\n%s", a.Property.ID, err, a.Trace.Summary)
			}
		}
	}
	return outcome, nil
}

// porReducedShare is, per generator, the share of models that a run must show
// coming out smaller (see porFloorOf): a generator that stops exercising the
// reduction fails its test instead of passing silently. The shares are about
// three quarters of what 24 000 fresh seeds per generator gave (smaller models,
// with the unreduced limits: base 0.239, atomic 0.157, loop 0.555, run 0.281,
// run-atomic 0.250, reads 0.415, atomic-reads 0.142; nrpr, added by the integration of the `_nr_pr` fix: 0.987 on its default seeds, 0.989 and 0.988 on two windows of 3 000 fresh ones, 910 000 001 and 920 000 001; almost every static model has a free first edge). A generator whose rule is
// not implemented (its models are refused) has no entry.
var porReducedShare = map[string]float64{"base": 0.17, "atomic": 0.12, "loop": 0.40, "run": 0.20, "run-atomic": 0.19, "reads": 0.31, "atomic-reads": 0.10, "nrpr": 0.90}

func TestPORDifferentialOnTheGenerators(t *testing.T) {
	for _, g := range porGeneratorsToRun() {
		if g.name == "base" {
			continue // TestPORAgreesWithTheFullSearchOnRandomModels, the original name
		}
		t.Run(g.name, func(t *testing.T) {
			tl := porForSeeds(t, "O1", g, 3000, func(m *ir.Model, _ *porTally) (porOutcome, error) { return porDifferential(m) })
			if share, ok := porReducedShare[g.name]; ok && tl.failures == 0 {
				if floor := porFloorOf(tl.models, share, porFloorSigmas); tl.smaller < floor {
					t.Fatalf("only %d of %d models were reduced (the floor is %d): the generator no longer exercises the reduction", tl.smaller, tl.models, floor)
				}
			}
		})
	}
}

// The limits of the chain walk of the tight runs: a chain of more than two
// micro-steps, or more than three fired in one pick, is not followed.
const (
	porTightChain  = 1
	porTightBudget = 3
)

// TestPORDifferentialWithTightChainLimits is O1 with the limits of the
// proviso's chain walk where the models reach them, on the generators that make
// atomic sequences.
func TestPORDifferentialWithTightChainLimits(t *testing.T) {
	for _, g := range porGeneratorsToRun("atomic", "loop", "run-atomic", "atomic-reads") {
		t.Run(g.name, func(t *testing.T) {
			tl := porForSeeds(t, "O1 tight", g, 3000, func(m *ir.Model, _ *porTally) (porOutcome, error) {
				return porDifferentialWith(m, porTightChain, porTightBudget)
			})
			// Tight limits that made every pick a full expansion would pass
			// vacuously: the oracle must still see models that the reduction shrinks.
			if share, ok := porTightShare[g.name]; ok && tl.failures == 0 {
				if floor := porFloorOf(tl.models, share, porTightSigmas); tl.smaller < floor {
					t.Fatalf("only %d of %d models were reduced under the tight limits (the floor is %d): the oracle no longer exercises the reduction", tl.smaller, tl.models, floor)
				}
			}
		})
	}
}

// porTightShare is, per generator, the share of models that must still come out
// smaller under the tight limits of the chain walk (see porFloorOf, with
// porTightSigmas): what 24 000 fresh seeds per generator gave, atomic 0.147,
// atomic-reads 0.137, loop 0.305, run-atomic 0.243. At 3 000 models and the
// default seeds the run gives atomic 435, atomic-reads 417, loop 901 and
// run-atomic 748 smaller models; the floors are 344, 316, 788 and 611; and
// lowering the tight budget of the proviso from three micro-steps to one (the red
// check) gives 289, 311, 708 and 696: the floors of the first three fail it (the
// third by five models only), the fourth cannot (the budget moves that count by
// 6%). On a fresh seed the lowered budget fails them in 100%, 64% and 100%
// of the windows of 3 000 seeds (24 000 seeds per generator, every window). The budget of a pick is
// shared by its candidates, so the tight budget of three is spent sooner than
// when it was per candidate (loop: one in two before). A run of go test -short
// cannot tell a lowered budget from chance (the counts of 375 models wander by 7
// around 50), so its floors only check that the generator is not dead.
var porTightShare = map[string]float64{"atomic": 0.147, "loop": 0.305, "run-atomic": 0.243, "atomic-reads": 0.137}
