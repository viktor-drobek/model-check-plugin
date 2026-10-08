package explore

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"modelcheck/ir"
)

// The Promela frontend gives a model that reads `_nr_pr` and has no `run` the
// table encoding of a model with `run` (the `_nr_pr` fix): every `-end-` edge
// leaves the live-process table behind `youngest(k)`. The reduction models the
// table (cell T, perf6) and its oracles were built on models that create
// processes by `run`; the only table model without a `run` that they generated
// never left the table. The `nrpr` generator (genNrPr) makes the shape the
// frontend now emits, so that the three oracles see it.

// nrPrShape describes a model of that shape: what its edges do with the table.
type nrPrShape struct {
	run         bool // some edge creates a process
	leaves      int  // edges that leave the table
	youngest    int  // end edges guarded by youngest(k)
	readsTable  int  // edges (not counting the youngest guard of an end edge) or properties that read nrpr
	processes   int
	table       bool // ir.NeedsTable
	propReadsNr bool
}

func shapeOfNrPr(m *ir.Model) nrPrShape {
	s := nrPrShape{processes: len(m.Processes), table: ir.NeedsTable(m)}
	for p := range m.Processes {
		for _, e := range m.Processes[p].Edges {
			if e.Run != nil {
				s.run = true
			}
			if e.Leave {
				s.leaves++
				if e.Guard != nil && e.Guard.Op == "youngest" {
					s.youngest++
				}
				continue
			}
			read := e.Guard.Uses("nrpr") || e.Assert.Uses("nrpr")
			for _, a := range e.Effect {
				read = read || a.Value.Uses("nrpr")
			}
			if read {
				s.readsTable++
			}
		}
	}
	for _, pr := range m.Properties {
		if pr.Expr.Uses("nrpr") {
			s.propReadsNr = true
		}
	}
	return s
}

func TestPORNrPrGeneratorMakesTheFrontendShape(t *testing.T) {
	g, ok := porGeneratorByName("nrpr")
	if !ok {
		t.Fatal("no generator named nrpr: the oracles see no table model without run that leaves the table")
	}
	const models = 600
	var withReads, withLeaveAll, propReads, dead int
	procs := map[int]int{}
	for seed := g.first; seed < g.first+models; seed++ {
		m := g.gen(newRand(seed))
		s := shapeOfNrPr(m)
		if s.run {
			t.Fatalf("seed %d: the model has a run", seed)
		}
		if !s.table {
			t.Fatalf("seed %d: the model has no table", seed)
		}
		procs[s.processes]++
		if s.readsTable > 0 {
			withReads++
		}
		if s.leaves == s.processes && s.youngest == s.leaves {
			withLeaveAll++
		}
		if s.propReadsNr {
			propReads++
		}
		if s.leaves == 0 {
			dead++
		}
	}
	t.Logf("nrpr generator, %d models: %d process counts %v; every end edge leaves behind youngest in %d; some edge reads _nr_pr in %d; a property reads it in %d; no leave at all in %d",
		models, len(procs), procs, withLeaveAll, withReads, propReads, dead)
	if procs[2] == 0 || procs[3] == 0 || procs[4] == 0 {
		t.Errorf("the process counts %v: two to four static processes are wanted", procs)
	}
	if withLeaveAll < models/2 {
		t.Errorf("only %d of %d models have the frontend's end edges on every process", withLeaveAll, models)
	}
	if withReads < models/2 {
		t.Errorf("only %d of %d models read _nr_pr in an edge", withReads, models)
	}
	if propReads < models/10 {
		t.Errorf("only %d of %d models have a property that reads _nr_pr", propReads, models)
	}
}

// The parallel search on the shape the frontend gives a model that reads
// `_nr_pr` and has no `run` (the `nrpr` generator): the two features were built
// on different branches, and the parallel oracles generate table models only
// through `run`. The parallel run must agree with the breadth-first and the
// depth-first runs, with and without the sweep, at every knob set.
func TestParallelAgreesWithTheSequentialSearchesOnTheNrPrShape(t *testing.T) {
	g, _ := porGeneratorByName("nrpr")
	models, first := parModelCount(600)
	first += g.first - 1 // the oracle's own default seed is 1; this generator has its own range
	var ending, complete, violated, skipped, withTable int
	for seed := first; seed < first+int64(models); seed++ {
		m := g.gen(newRand(seed))
		name := fmt.Sprintf("nrpr seed %d", seed)
		kn := parKnobSets[int(seed)%len(parKnobSets)]
		workers := []int{1, 2, 4}[int(seed)%3]
		bud := Budget{MaxStates: 30000, MaxDepth: 20000}
		checkParallelNoSweep(t, name+" (no sweep)", m, workers, kn, Options{Budget: bud})
		runs := checkParallelBudget(t, name, m, workers, kn, bud)
		if ir.NeedsTable(m) {
			withTable++
		}
		switch {
		case runs.atomicLoop, hitBudget(runs.dfs), hitBudget(runs.par):
			skipped++
		case endingEvent(runs.dfs):
			ending++
		default:
			complete++
			for _, o := range runs.par.Outcomes {
				if o.Status == Violated {
					violated++
					break
				}
			}
		}
	}
	t.Logf("%d nrpr models (%d with the table): %d complete (%d with a violation), %d ending in an error of the model, %d skipped (budget)", models, withTable, complete, violated, ending, skipped)
	if withTable != models || complete < models/2 || violated < models/10 {
		t.Fatalf("the generator no longer exercises the search: %d with the table, %d complete, %d violated of %d", withTable, complete, violated, models)
	}
}

// Weak fairness on models with the live-process table. The `_nr_pr` fix and the
// weak-fairness fix met on the merged tree: a process that ends leaves the table
// behind `youngest(k)`, so an older process whose turn has not come is "blocked"
// at its end edge, and a never claim may stand anywhere among the processes
// (the table lists processes, never the claim). The SCC oracle decides "a weakly
// fair accepting cycle exists" on the explicit product graph built with the
// Stepper, which applies `leave`; the engine's copies construction must give the
// same answer, under none and weak fairness.
//
// tableWFCase gives a random case of the weak-fairness oracle what the frontend
// gives a model that reads `_nr_pr`: an end location and an `-end-` edge that
// leaves the table behind `youngest` on every non-claim process, and `_nr_pr`
// guards on some edges.
func tableWFCase(c *wfCase, rx *rand.Rand) {
	m := c.m
	for p := range m.Processes {
		if p == c.claim {
			continue
		}
		pr := &m.Processes[p]
		endFrom := rx.Intn(len(pr.Locations))
		pr.Locations = append(pr.Locations, ir.Location{Name: "-dead-"})
		pr.Edges = append(pr.Edges, ir.Edge{From: endFrom, To: len(pr.Locations) - 1,
			Guard: ir.Youngest(p), Leave: true, Text: "-end-"})
		for e := range pr.Edges {
			if rx.Intn(6) != 0 || pr.Edges[e].Leave {
				continue
			}
			ed := &pr.Edges[e]
			cond := ir.Binary([]string{"eq", "lt", "ge", "ne"}[rx.Intn(4)], ir.NrPr(), ir.Const(int64(1+rx.Intn(3))))
			if ed.Guard == nil {
				ed.Guard = cond
			} else {
				ed.Guard = ir.And(ed.Guard, cond)
			}
		}
	}
}

func TestWeakFairnessMatchesTheSCCOracleOnTableModels(t *testing.T) {
	models := 1500
	if testing.Short() {
		models = 300
	}
	if v, err := strconv.Atoi(os.Getenv("MCD_WF_MODELS")); err == nil && v > 0 {
		models = v
	}
	first := int64(70_000_001)
	if v, err := strconv.ParseInt(os.Getenv("MCD_WF_SEED"), 10, 64); err == nil && v > 0 {
		first = v
	}
	modes := []string{"acc", "prog", "claim"}
	var rows, violated, fairMatters, skipped, claimMoved, withProvided int
	for i := 0; i < models; i++ {
		seed := first + int64(i)
		md := modes[seed%int64(len(modes))]
		c := randomWFCase(rand.New(rand.NewSource(seed)), md)
		atomic := extendWFCase(&c, rand.New(rand.NewSource(seed*7919+13)))
		tableWFCase(&c, rand.New(rand.NewSource(seed*104729+7)))
		if err := ir.Validate(c.m); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		budget := Budget{}
		if atomic {
			budget = Budget{MaxDepth: 300, MaxStates: 200000}
		}
		var verdict [2]bool
		skip := false
		for fi, fairness := range []string{"none", "weak"} {
			want, err := oracleCycle(c, fairness == "weak", 4000)
			if err != nil {
				skip = true
				break
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			res, err := Run(ctx, c.m, Options{Sweep: true, Fairness: fairness, Budget: budget})
			timedOut := ctx.Err() != nil
			cancel()
			if timedOut {
				skip = true
				break
			}
			if err != nil {
				t.Fatalf("seed %d (%s, %s): %v", seed, md, fairness, err)
			}
			o := outcomeOf(t, res, c.m.Properties[0].ID)
			if o.Status != Violated && o.Status != Verified {
				skip = true
				break
			}
			verdict[fi] = want
			if got := o.Status == Violated; got != want {
				t.Errorf("seed %d (%s, fairness %s): the engine says %s, the SCC oracle says a cycle exists: %v (MCD_WF_SEED=%d MCD_WF_MODELS=1 reproduces it)",
					seed, md, fairness, o.Status, want, seed)
			}
		}
		if skip {
			skipped++
			continue
		}
		rows++
		if verdict[0] {
			violated++
		}
		if verdict[0] != verdict[1] {
			fairMatters++
		}
		if c.claim >= 0 && c.claim != len(c.m.Processes)-1 {
			claimMoved++
		}
		for p := range c.m.Processes {
			if c.m.Processes[p].Provided != nil {
				withProvided++
				break
			}
		}
	}
	t.Logf("%d table models: %d compared, %d violated without fairness, fairness changes the verdict in %d, %d with the claim not last, %d with a provided clause, %d skipped", models, rows, violated, fairMatters, claimMoved, withProvided, skipped)
	if models >= 300 && (rows < models/2 || violated < models/10 || fairMatters < models/300 || claimMoved < models/20) {
		t.Errorf("the generator is too tame: %d compared, %d violated, %d where fairness matters, %d with the claim moved, of %d", rows, violated, fairMatters, claimMoved, models)
	}
}
