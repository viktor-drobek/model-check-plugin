package explore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"modelcheck/frontend/promela"
	"modelcheck/ir"
)

const corpus = "../../../Promela - examples"

func parseFile(t *testing.T, path string, defines ...string) (*ir.Model, map[string]string) {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, perr := promela.Parse(src, path, defines)
	if perr != nil {
		t.Fatal(perr)
	}
	return res.Model, res.Defines
}

func withClaim(t *testing.T, model, claim string) string {
	t.Helper()
	a, err := os.ReadFile(model)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(claim)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), filepath.Base(model))
	if err := os.WriteFile(p, append(append(a, '\n'), b...), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func outcomeOf(t *testing.T, res *Result, id string) Outcome {
	t.Helper()
	for _, o := range res.Outcomes {
		if o.Property.ID == id {
			return o
		}
	}
	t.Fatalf("no outcome %q in %+v", id, res.Outcomes)
	return Outcome{}
}

// pan numbers: SPIN 6.5.2, spin -a -o1 -o2 -o3, gcc -O2 -DNOREDUCE
// [-DNP], pan -a [-f] -c0 / pan -l -c0 (2026-09-25).
func TestCorpusCycles(t *testing.T) {
	cases := []struct {
		name     string
		file     string
		defines  []string
		prop     string
		fairness string
		status   Status
		reason   string
		loop     bool
		states   int // pan -c0 "states, stored"; 0 = not compared
	}{
		{"prop PHI", "CH4/prop.pml", []string{"PHI"}, "never", "none", Violated, "acceptance cycle", true, 3},
		{"prop no PHI", "CH4/prop.pml", nil, "never", "none", Violated, "end state in claim reached", false, 5},
		{"App_A", "App_A/example", nil, "never", "none", Verified, "no acceptance cycle", false, 10},
		{"trivial", "CH8/trivial.pml", nil, "never", "none", Violated, "acceptance cycle", true, 2},
		{"trivial weak", "CH8/trivial.pml", nil, "never", "weak", Violated, "acceptance cycle", true, 0},
		// pan -a -c0 prints 5 for fairness.pml: its counter includes a
		// nested-search re-insertion (pan -DCHECK: "New state 3+"); the
		// product has the 4 states of the plain run (pan -c0: 4).
		{"fairness", "CH8/fairness.pml", nil, "accept", "none", Violated, "acceptance cycle", true, 4},
		{"fairness weak", "CH8/fairness.pml", nil, "accept", "weak", Violated, "acceptance cycle", true, 0},
		{"fair_accept", "CH4/fair_accept.pml", nil, "accept", "none", Violated, "acceptance cycle", true, 4},
		{"fair_accept weak", "CH4/fair_accept.pml", nil, "accept", "weak", Violated, "acceptance cycle", true, 0},
		{"dijkstra progress", "CH4/dijkstra_progress.pml", nil, "progress", "none", Verified, "no non-progress cycle", false, 39},
		{"dijkstra progress weak", "CH4/dijkstra_progress.pml", nil, "progress", "weak", Verified, "no non-progress cycle", false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, _ := parseFile(t, filepath.Join(corpus, c.file), c.defines...)
			res, err := Run(context.Background(), m, Options{Sweep: true, Fairness: c.fairness})
			if err != nil {
				t.Fatal(err)
			}
			o := outcomeOf(t, res, c.prop)
			if o.Status != c.status || !strings.Contains(o.Reason, c.reason) {
				t.Fatalf("%s: %s / %q", c.prop, o.Status, o.Reason)
			}
			if (o.Trace != nil && o.Trace.Loop != nil) != c.loop {
				t.Fatalf("loop presence: want %v, trace %+v", c.loop, o.Trace)
			}
			if o.Stats == nil || !o.Stats.Complete {
				t.Fatalf("stats %+v", o.Stats)
			}
			if c.states > 0 && o.Stats.States != c.states {
				t.Fatalf("states %d, pan %d", o.Stats.States, c.states)
			}
		})
	}
}

func TestFairProgress(t *testing.T) {
	m, _ := parseFile(t, filepath.Join(corpus, "CH4/fair.pml"))
	m.Properties = append(m.Properties, ir.Property{ID: "progress", Kind: ir.KindProgress})
	res, err := Run(context.Background(), m, Options{Sweep: true})
	if err != nil {
		t.Fatal(err)
	}
	o := outcomeOf(t, res, "progress")
	if o.Status != Violated || !strings.Contains(o.Reason, "non-progress cycle") || o.Trace.Loop == nil {
		t.Fatalf("%s / %q", o.Status, o.Reason)
	}
	// pan -l -c0 prints 5: as for fairness.pml, one nested-search
	// re-insertion is counted; the np_ product has 2 × 2 = 4 states.
	if o.Stats.States != 4 {
		t.Fatalf("fair.pml -l: %d states, want 4", o.Stats.States)
	}
}

func TestFormulaOnCorpus(t *testing.T) {
	cases := []struct {
		file     string
		formula  string
		fairness string
		status   Status
		loop     bool
	}{
		// App_A's never claim is the automaton FOR <>[]p (it accepts the runs
		// that satisfy it); pan's "no errors" on that claim means no run
		// satisfies <>[]p, i.e. the property []<>!p holds and <>[]p itself
		// is violated. x runs 4 → 2 → 1 → 4 …, p = (x < 4).
		{"App_A/example", "<>[]p", "none", Violated, true},
		{"App_A/example", "[]<>!p", "none", Verified, false},
		{"App_A/example", "[]<>p", "none", Verified, false},
		{"App_A/example", "X p", "none", Violated, true},
		{"CH4/prop.pml", "[]p", "none", Violated, true},
		{"CH4/prop.pml", "<>[]p", "none", Violated, true},
		{"CH4/prop.pml", "[]<>p", "none", Violated, true},
		{"CH4/prop.pml", "<>!p", "none", Violated, true},
		{"CH4/fair.pml", "[]<>(x == 1)", "none", Verified, false},
		{"CH4/fair.pml", "[]<>(x == 1)", "weak", Verified, false},
		{"CH3/alternatingbit.pml", "[] (len(to_rcvr) > 0 -> <> (len(to_rcvr) == 0))", "none", Verified, false},
		{"CH3/alternatingbit.pml", "[] (len(to_rcvr) > 0 -> <> (len(to_rcvr) == 0))", "weak", Verified, false},
		{"CH3/alternatingbit.pml", "[]<> (len(to_rcvr) == 2)", "none", Violated, true},
		{"CH4/false.pml", "[]true", "none", Verified, false},
	}
	for _, c := range cases {
		t.Run(c.file+" "+c.formula+" "+c.fairness, func(t *testing.T) {
			m, defs := parseFile(t, filepath.Join(corpus, c.file))
			m.Properties = []ir.Property{{ID: "f", Kind: ir.KindLTL, Formula: c.formula}}
			res, err := Run(context.Background(), m, Options{Fairness: c.fairness, Defines: defs})
			if err != nil {
				t.Fatal(err)
			}
			o := outcomeOf(t, res, "f")
			if o.Status != c.status {
				t.Fatalf("%s: %q", o.Status, o.Reason)
			}
			if (o.Trace != nil && o.Trace.Loop != nil) != c.loop {
				t.Fatalf("loop: %+v", o.Trace)
			}
			if o.Temporal == nil || o.Temporal.Negated != "!("+c.formula+")" {
				t.Fatalf("temporal %+v", o.Temporal)
			}
		})
	}
}

func TestStarvation(t *testing.T) {
	m, defs := parseFile(t, "../testdata/promela/starvation.pml")
	m.Properties = append(m.Properties, ir.Property{ID: "live", Kind: ir.KindLTL, Formula: "<>done"})
	res, err := Run(context.Background(), m, Options{Defines: defs})
	if err != nil {
		t.Fatal(err)
	}
	o := outcomeOf(t, res, "live")
	if o.Status != Violated || o.Trace == nil || o.Trace.Loop == nil {
		t.Fatalf("%s %q", o.Status, o.Reason)
	}
	for _, st := range o.Trace.LoopSteps() {
		if st.Process != "A:0" && !strings.HasPrefix(st.Process, "never") {
			t.Fatalf("loop step by %s", st.Process)
		}
	}
	if !strings.Contains(o.Reason, "B:1 is enabled throughout the loop and never moves") {
		t.Fatalf("reason %q", o.Reason)
	}
	// The loop closes: the state after the last step equals the state
	// before the first loop step.
	res, err = Run(context.Background(), m, Options{Defines: defs, Fairness: "weak"})
	if err != nil {
		t.Fatal(err)
	}
	o = outcomeOf(t, res, "live")
	if o.Status != Verified || !strings.Contains(o.Reason, "weak fairness") {
		t.Fatalf("weak: %s %q", o.Status, o.Reason)
	}
	res, err = Run(context.Background(), m, Options{Defines: defs, Fairness: "strong"})
	if err != nil {
		t.Fatal(err)
	}
	o = outcomeOf(t, res, "live")
	if o.Status != NotExecuted || !strings.Contains(o.Reason, "strong fairness") {
		t.Fatalf("strong: %s %q", o.Status, o.Reason)
	}
	// The deadlock property is still decided by the safety search.
	if d := outcomeOf(t, res, "deadlock"); d.Status != Verified {
		t.Fatalf("deadlock %s", d.Status)
	}
}

func TestLeader3(t *testing.T) {
	p := withClaim(t, "../testdata/promela/leader3.pml", filepath.Join(corpus, "CH12/leader.ltl"))
	m, defs := parseFile(t, p)
	m.Properties = append(m.Properties, ir.Property{ID: "f", Kind: ir.KindLTL, Formula: "<>[]oneLeader"})
	res, err := Run(context.Background(), m, Options{Sweep: true, Defines: defs})
	if err != nil {
		t.Fatal(err)
	}
	if o := outcomeOf(t, res, "never"); o.Status != Verified || o.Stats.States != 1340 {
		t.Fatalf("never: %s, %d states (pan 1340)", o.Status, o.Stats.States)
	}
	if o := outcomeOf(t, res, "f"); o.Status != Verified || o.Temporal.Atoms[0] != "nr_leaders == 1" {
		t.Fatalf("formula: %s %v", o.Status, o.Temporal)
	}
	if o := outcomeOf(t, res, "assert"); o.Status != Verified {
		t.Fatalf("assert %s", o.Status)
	}
}

// TestNestedDFSHandBuilt: hand-built graphs. Process P walks 0 → 1 → 2 →
// 1 (a cycle through 1 and 2) with a dead-end branch 0 → 3. With `accept`
// on 2 the cycle is accepting; with `accept` only on 0 (left forever) no
// accepting cycle exists; with `accept` on the dead end 3 the stutter
// extension makes the terminal state repeat: an "accept stutter" cycle.
func TestNestedDFSHandBuilt(t *testing.T) {
	build := func(acceptAt int) *ir.Model {
		locs := make([]ir.Location, 4)
		for i := range locs {
			locs[i].Name = []string{"L0", "L1", "L2", "L3"}[i]
		}
		locs[acceptAt].Labels = []ir.Label{ir.Accept}
		return &ir.Model{Schema: ir.Schema, Name: "hand", Processes: []ir.Process{{
			Name: "P", Locations: locs, Edges: []ir.Edge{
				{From: 0, To: 1, Text: "a"}, {From: 0, To: 3, Text: "d"},
				{From: 1, To: 2, Text: "b"}, {From: 2, To: 1, Text: "c"},
			},
		}}, Properties: []ir.Property{{ID: "acc", Kind: ir.KindLTL}}}
	}
	res, err := Run(context.Background(), build(2), Options{})
	if err != nil {
		t.Fatal(err)
	}
	o := outcomeOf(t, res, "acc")
	if o.Status != Violated || o.Trace.Loop == nil || o.Trace.Loop.Start != 2 || o.Trace.Loop.Steps != 2 {
		t.Fatalf("cycle through accept: %s %+v", o.Status, o.Trace)
	}
	if o.Trace.Summary != "a; loop: b, c" {
		t.Fatalf("summary %q", o.Trace.Summary)
	}
	res, err = Run(context.Background(), build(0), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if o := outcomeOf(t, res, "acc"); o.Status != Verified || o.Stats.States != 4 {
		t.Fatalf("accept on the initial state: %s %d", o.Status, o.Stats.States)
	}
	res, err = Run(context.Background(), build(3), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if o := outcomeOf(t, res, "acc"); o.Status != Violated || o.Trace.Loop == nil || o.Trace.Loop.Steps != 1 || o.Trace.Steps[len(o.Trace.Steps)-1].Process != "-" {
		t.Fatalf("accept stutter on the dead end: %s %+v", o.Status, o.Trace)
	}
	// Budgets: the state limit stops the product search → inconclusive,
	// bounded.
	res, err = Run(context.Background(), build(0), Options{Budget: Budget{MaxStates: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if o := outcomeOf(t, res, "acc"); o.Status != Inconclusive || o.Evidence != Bounded {
		t.Fatalf("state budget: %s %s %q", o.Status, o.Evidence, o.Reason)
	}
}

func TestBudgetEvidence(t *testing.T) {
	if budgetEvidence("state budget exhausted: 5 states stored") != Bounded ||
		budgetEvidence("depth budget exhausted: x") != Bounded ||
		budgetEvidence("time budget exhausted") != EvUnknown ||
		budgetEvidence("memory budget exhausted: y") != EvUnknown {
		t.Fatal("budget evidence partition")
	}
}

func TestFormulaErrors(t *testing.T) {
	m, defs := parseFile(t, filepath.Join(corpus, "App_A/example"))
	for _, f := range []string{"[] (p ->", "<> nosuchvar"} {
		m.Properties = []ir.Property{{ID: "f", Kind: ir.KindLTL, Formula: f}}
		_, err := Run(context.Background(), m, Options{Defines: defs})
		var fe *FormulaError
		if err == nil || !asFormulaError(err, &fe) {
			t.Fatalf("%q: %v", f, err)
		}
	}
}

func asFormulaError(err error, target **FormulaError) bool {
	fe, ok := err.(*FormulaError)
	if ok {
		*target = fe
	}
	return ok
}
