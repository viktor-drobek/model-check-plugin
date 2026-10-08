package explore

import (
	"context"
	"fmt"
	"testing"
	"time"

	"modelcheck/ir"
)

// O3, the acyclicity audit of the cycle proviso (performance plan, step 6,
// sections 3.3 and 7.1).
//
// C3 says that every cycle of the reduced graph contains a state that was
// expanded in full. The reduced graph here is the exact one: the stored states
// of the reduced search, each with the choice pick made at it. A verdict
// oracle sees a hole in the proviso only when the hole changes a verdict, and
// on a depth-first search it rarely does (the search reaches a fully expanded
// state elsewhere); a cycle of reduced states is nevertheless what C3
// forbids, so this audit checks the graph itself: the states that were not
// expanded in full contain no cycle of ample steps.

// porChoice is what the search chose at a stored state.
type porChoice struct {
	full bool
	proc int // the process expanded alone when !full
}

// acyclic runs the reduced search of m, records every choice, and looks for a
// cycle among the states expanded through an ample set, following the
// macro-steps of the chosen process. checked is false when the model is
// refused or the search did not finish.
func acyclic(m *ir.Model, noProviso bool) (checked bool, err error) {
	return acyclicWith(m, noProviso, 0, 0)
}

// acyclicWith is acyclic with the limits of the proviso's chain walk given
// (zero: the defaults).
func acyclicWith(m *ir.Model, noProviso bool, chainLimit, pickBudget int) (checked bool, err error) {
	choices := map[string]porChoice{}
	opt := Options{Sweep: true, POR: true, porNoProviso: noProviso, porChainLimit: chainLimit, porPickBudget: pickBudget,
		Budget: Budget{MaxStates: 3000, MaxDepth: 3000},
		porTrace: func(state []byte, ample uint8) {
			choices[string(state)] = porChoice{full: ample == 0, proc: int(ample) - 1}
		}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := Run(ctx, m, opt)
	if err != nil || !r.Complete || r.Reduction == nil || !r.Reduction.Applied {
		return false, err
	}
	st, err := NewStepper(m)
	if err != nil {
		return false, err
	}
	a := &auditCtx{st: st}
	succ := map[string][]string{}
	for k, ch := range choices {
		if ch.full {
			continue
		}
		steps, _ := a.paths([]byte(k), func(q int) bool { return q == ch.proc })
		for _, sp := range steps {
			if sp.bad {
				continue
			}
			if c2, ok := choices[string(sp.end)]; ok && !c2.full {
				succ[k] = append(succ[k], string(sp.end))
			}
		}
	}
	const (
		white = iota
		grey
		black
	)
	colour := map[string]int{}
	var visit func(k string) bool
	visit = func(k string) bool {
		colour[k] = grey
		for _, n := range succ[k] {
			switch colour[n] {
			case grey:
				return true
			case white:
				if visit(n) {
					return true
				}
			}
		}
		colour[k] = black
		return false
	}
	for k := range succ {
		if colour[k] == white && visit(k) {
			return true, fmt.Errorf("a cycle of states that were not expanded in full")
		}
	}
	return true, nil
}

func TestPORAcyclicityOfTheReducedGraph(t *testing.T) {
	for _, g := range porGeneratorsToRun() {
		t.Run(g.name, func(t *testing.T) {
			tl := porForSeeds(t, "O3", g, 3000, func(m *ir.Model, tl *porTally) (porOutcome, error) {
				checked, err := acyclic(m, false)
				if checked {
					tl.checked++
				}
				return porUncounted, err
			})
			if n, ok := porAcyclicFloor[g.name]; ok && tl.failures == 0 && tl.checked < tl.models/n {
				t.Fatalf("only %d of %d models were checked for cycles: the generator no longer exercises the proviso", tl.checked, tl.models)
			}
		})
	}
}

// TestPORAcyclicityWithTightChainLimits is O3 with the limits of the proviso's
// chain walk where the models reach them: a chain the walk gives up on must
// answer "not leaving", and a cycle through it is then a cycle of states that
// were expanded in full.
func TestPORAcyclicityWithTightChainLimits(t *testing.T) {
	for _, g := range porGeneratorsToRun("atomic", "loop", "run-atomic", "atomic-reads") {
		t.Run(g.name, func(t *testing.T) {
			porForSeeds(t, "O3 tight", g, 3000, func(m *ir.Model, tl *porTally) (porOutcome, error) {
				checked, err := acyclicWith(m, false, porTightChain, porTightBudget)
				if checked {
					tl.checked++
				}
				return porUncounted, err
			})
		})
	}
}

// porAcyclicFloor is, per generator, how few models make the check vacuous:
// one in n. A generator whose rule is not implemented has no entry.
var porAcyclicFloor = map[string]int{"base": 2, "atomic": 2, "loop": 2, "run": 4, "run-atomic": 4, "reads": 2, "atomic-reads": 2, "nrpr": 2}

// With the proviso switched off the audit must see cycles: it is calibrated on
// the thing it is for. Of the base generator's models it finds them in about
// three in ten.
func TestPORAcyclicityAuditSeesAMissingProviso(t *testing.T) {
	for _, name := range []string{"base", "loop"} {
		g, _ := porGeneratorByName(name)
		seen, models := 0, 400
		for seed := g.first; seed < g.first+int64(models); seed++ {
			if _, err := acyclic(g.gen(newRand(seed)), true); err != nil {
				seen++
			}
		}
		if seen < models/10 {
			t.Fatalf("%s: the audit saw a cycle in %d of %d models without the proviso", name, seen, models)
		}
	}
}
