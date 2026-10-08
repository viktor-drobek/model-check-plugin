package explore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"modelcheck/cex"
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

// TestBlockedStateHasNoSuccessorInTheNPProduct is the G5 addendum: under
// `pan -l` a state with no enabled transition has no successors at all, so
// no cycle passes through it and a deadlocked system is not a non-progress
// cycle. G4 applied the stutter extension to every product, which turned
// every deadlock into a non-progress cycle; eight mutants of
// CH4/dijkstra_progress.pml found it (K3's campaign). The model below is
// the smallest shape of those eight: one process that blocks for good,
// with a progress label it never reaches.
func TestBlockedStateHasNoSuccessorInTheNPProduct(t *testing.T) {
	// P: x == 1 -> (blocks forever, x is 0); the progress label is on the
	// statement it can never take.
	m := &ir.Model{Schema: ir.Schema, Name: "blocked",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte}},
		Processes: []ir.Process{{
			Name:      "P",
			Locations: []ir.Location{{Name: "loop"}, {Name: "run", Labels: []ir.Label{ir.Progress}}},
			Edges: []ir.Edge{
				{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(1)), Text: "(x == 1)"},
				{From: 1, To: 0, Text: "skip"},
			},
		}},
		Properties: []ir.Property{
			{ID: "deadlock", Kind: ir.KindDeadlock},
			{ID: "progress", Kind: ir.KindProgress},
		}}
	res, err := Run(context.Background(), m, Options{Sweep: true})
	if err != nil {
		t.Fatal(err)
	}
	// The blocked state is a deadlock, and the deadlock is reported — by the
	// search that owns that question.
	if o := outcomeOf(t, res, "deadlock"); o.Status != Violated {
		t.Fatalf("deadlock: %s / %q; a system that cannot move is a deadlock", o.Status, o.Reason)
	}
	// It is NOT a non-progress cycle: there is no cycle through it.
	o := outcomeOf(t, res, "progress")
	if o.Status != Verified {
		t.Fatalf("progress: %s / %q; a blocked state has no successor in the np_ product, so no cycle passes through it", o.Status, o.Reason)
	}
	if o.Trace != nil {
		t.Fatalf("a verified progress property carries no counterexample: %+v", o.Trace)
	}

	// The same model without the progress label still has no non-progress
	// cycle, and still deadlocks: the rule is about successors, not labels.
	m2 := *m
	m2.Processes = append([]ir.Process(nil), m.Processes...)
	m2.Processes[0].Locations = []ir.Location{{Name: "loop"}, {Name: "run"}}
	res2, err := Run(context.Background(), &m2, Options{Sweep: true})
	if err != nil {
		t.Fatal(err)
	}
	if o := outcomeOf(t, res2, "progress"); o.Status != Verified {
		t.Fatalf("progress without a label: %s / %q", o.Status, o.Reason)
	}
}

// TestStutterExtensionStaysForLTL: the addendum turns the extension off for
// the np_ product and for nothing else. On the same blocked model, `[]p`
// with p false in the blocked state is still violated — that is pan's
// answer, and it is what makes a terminating or blocked run testable at all.
func TestStutterExtensionStaysForLTL(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "blocked",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte}},
		Processes: []ir.Process{{
			Name:      "P",
			Locations: []ir.Location{{Name: "loop"}, {Name: "run"}},
			Edges: []ir.Edge{
				{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(1)), Text: "(x == 1)"},
				{From: 1, To: 0, Text: "skip"},
			},
		}},
		Properties: []ir.Property{{ID: "live", Kind: ir.KindLTL, Formula: "[](x == 1)"}}}
	res, err := Run(context.Background(), m, Options{Sweep: true})
	if err != nil {
		t.Fatal(err)
	}
	o := outcomeOf(t, res, "live")
	if o.Status != Violated {
		t.Fatalf("ltl on a blocked model: %s / %q; the stutter extension belongs here", o.Status, o.Reason)
	}
	if o.Trace == nil || o.Trace.Loop == nil {
		t.Fatalf("the counterexample of an acceptance cycle is a lasso: %+v", o.Trace)
	}
}

// systemSteps keeps the steps of the system among the first n steps of a
// trace: the claim's steps change no variable and are not moves of the
// model, so the stepper cannot replay them.
func systemSteps(m *ir.Model, tr *cex.Trace, n int) *cex.Trace {
	claim := map[string]bool{}
	for _, p := range m.Processes {
		if p.Claim {
			claim[p.Name] = true
		}
	}
	out := &cex.Trace{Final: tr.Final}
	for _, st := range tr.Steps[:n] {
		if !claim[st.Process] {
			out.Steps = append(out.Steps, st)
		}
	}
	return out
}

// TestLassoThroughAnAtomicSequence: an acceptance cycle whose loop runs
// through the unstored intermediate state of an atomic sequence is rendered
// as an exact run. The inner search of the nested DFS keeps such a state on
// its own stack and released it when it returned, before the lasso was drawn
// from that stack: the engine stopped with an index out of range. The second
// model reaches the cycle through the atomic pair only after the first
// cycle has decided the property, which is what --sweep (Options.Sweep, pan
// -c0) goes on to do. pan -a -c0 (SPIN 6.5.2): one state stored for the
// first model, two for the second.
func TestLassoThroughAnAtomicSequence(t *testing.T) {
	cases := []struct {
		file   string
		sweep  bool
		states int // pan -c0 "states, stored" under sweep; 0 = not compared
	}{
		{"claim-atomic-loop.pml", false, 1},
		{"claim-atomic-loop.pml", true, 1},
		{"claim-atomic-second-cycle.pml", false, 0},
		{"claim-atomic-second-cycle.pml", true, 2},
		// A second process starved while the first loops on an atomic pair,
		// and an atomic sequence that begins with a timeout (pan -a -c0: 2
		// and 6 states stored).
		{"claim-atomic-starve.pml", false, 0},
		{"claim-atomic-starve.pml", true, 2},
		{"claim-atomic-timeout.pml", false, 0},
		{"claim-atomic-timeout.pml", true, 6},
	}
	for _, c := range cases {
		name := c.file
		if c.sweep {
			name += " sweep"
		}
		t.Run(name, func(t *testing.T) {
			m, defs := parseFile(t, "../testdata/promela/"+c.file)
			res, err := Run(context.Background(), m, Options{Sweep: c.sweep, Defines: defs})
			if err != nil {
				t.Fatal(err)
			}
			o := outcomeOf(t, res, "never")
			if o.Status != Violated || o.Evidence != Exhaustive || !strings.Contains(o.Reason, "acceptance cycle") {
				t.Fatalf("never: %s/%s %q", o.Status, o.Evidence, o.Reason)
			}
			if c.sweep && c.states > 0 && o.Stats.States != c.states {
				t.Fatalf("never: %d states stored, pan -c0 says %d", o.Stats.States, c.states)
			}
			tr := o.Trace
			if tr == nil || tr.Loop == nil {
				t.Fatalf("an acceptance cycle is a lasso: %+v", tr)
			}
			// The run replays from the initial state, and the loop closes:
			// the state before the loop is the state after the last step.
			if err := replay(m, systemSteps(m, tr, len(tr.Steps))); err != nil {
				t.Fatalf("the lasso is not a run of the model: %v", err)
			}
			if err := replay(m, systemSteps(m, tr, tr.Loop.Start-1)); err != nil {
				t.Fatalf("the loop is not closed: the state before it is not the final state: %v", err)
			}
		})
	}
}

// TestCycleSearchHandsBackItsTmpStack: the cycle search keeps the intermediate
// states of atomic sequences in s.tmp, one per frame with a negative index, and
// must hand the stack back empty when the outer stack is. A leak does not
// change a verdict (a stack: a stale top entry is popped in its owner's place)
// but grows the stack and the memory estimate on every cycle found under
// --sweep, so the end of the search checks it, in constant time, and an
// imbalance is an internal error (ErrInternal), never a result.
func TestCycleSearchHandsBackItsTmpStack(t *testing.T) {
	state := [][]byte{{1}}
	cases := []struct {
		name  string
		stack []frame
		tmp   [][]byte
		leak  bool
	}{
		{"finished and balanced", nil, nil, false},
		{"stopped inside an atomic sequence: the entries belong to the stack", []frame{{}}, state, false},
		{"finished with a stale entry", nil, state, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cs := &cycleSearch{s: &search{stack: c.stack, tmp: c.tmp}}
			err := cs.balanced()
			if !c.leak {
				if err != nil {
					t.Fatalf("balanced search reported: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInternal) || !strings.Contains(err.Error(), "internal: ") || !strings.Contains(err.Error(), "1 intermediate") {
				t.Fatalf("a stale entry must be an internal error, got: %v", err)
			}
		})
	}
}
