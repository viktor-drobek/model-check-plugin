package explore

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"modelcheck/cex"
	"modelcheck/frontend/promela"
	"modelcheck/ir"
)

// The differential oracle of the parallel search (performance plan 5, §5.3):
// every model is run in full by the sequential depth-first search, by the
// sequential breadth-first search and by the parallel search, and the contract
// of the plan is checked:
//
//   - a model without an ending event (an evaluation error, a pool exhaustion,
//     an over-long atomic sequence) has, in every property, the status, the
//     evidence and the reason of the sequential searches, and, when complete,
//     their states, transitions and atomic steps; a trace replays as a run of
//     the model, ends in a state that shows its verdict (the invariant false,
//     the reach condition true, no move enabled, an assert failing in the last
//     step) and, without atomic sequences, is as long as the breadth-first one
//     (A10, A12, A13);
//   - a model with one has no property verified, every counterexample replays
//     and shows its verdict, and the run stops where the sequential ones do
//     (A12b);
//   - the result is the same for every knob setting of the sizes that must not
//     change it (A11).

func promelaModel(t testing.TB, src string, defines ...string) *ir.Model {
	t.Helper()
	res, perr := promela.Parse([]byte(src), "par.pml", defines)
	if perr != nil {
		t.Fatalf("par.pml: %v\n%s", perr, src)
	}
	return res.Model
}

type parRuns struct {
	dfs, bfs, par *Result
	// atomicLoop: the parallel search ended on the bound of an atomic sequence
	// (a cycle of atomic steps that never blocks), and the sequential searches
	// were not run: the sequential breadth-first search keeps a copy of the
	// chain of moves for every intermediate state, so it needs memory
	// quadratic in the length of the sequence, and 100 000 steps are 100 GB.
	atomicLoop bool
}

func runOpts(t testing.TB, m *ir.Model, opt Options) *Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := Run(ctx, m, opt)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// endingEvent: the run met an event that ends the search whatever the
// properties say (an invalid model, a bound of the engine).
func endingEvent(r *Result) bool {
	return r.Stop == "invalid model" || strings.HasPrefix(r.Stop, "process budget") || strings.Contains(r.Stop, "an atomic sequence exceeds")
}

// reasonKey drops what depends on which of several failing asserts the order
// meets first.
func reasonKey(o Outcome) string {
	if o.Status == Violated && o.Property.Kind == ir.KindAssert {
		return "assert violated"
	}
	return o.Reason
}

// enabledPrefix lists the moves enabled in state in the order of the search,
// up to the first evaluation error: in a state whose enabledness cannot be
// evaluated to the end, the moves before the error are still moves the search
// took (it expands lazily), so a trace through such a state is a run.
func enabledPrefix(st *Stepper, state []byte) []Move {
	f := frame{proc: -1}
	var out []Move
	for {
		m, ok, err := st.s.nextEnabled(&f, state)
		if err != nil || !ok {
			return out
		}
		out = append(out, toMove(m))
	}
}

// replayTrace replays a trace as a run of the model from the initial state,
// every step an enabled move (edges with the same text make a step ambiguous,
// so every matching move is tried). With mayFail the last step of an
// invalid-model trace may be the step that failed, or the state it leaves one
// whose enabledness cannot be evaluated, and is not checked; otherwise the
// valuation reached must be the trace's final one. Two more checks are part of
// the walk and not made after it, so that a step that matches several moves is
// settled by the one that passes them: failedLast demands that the last step
// is one in which an assert fails (the move that Apply reports with a failed
// edge), and atEnd, when not nil, is called on the state the whole trace
// reaches and must accept it. Neither is made for a trace that mayFail. A
// trace of no step cannot have a last step that fails an assert, and the walk
// would return at its first call, before it looks at failedLast, so that case
// is refused up front.
func replayTrace(m *ir.Model, tr *cex.Trace, mayFail, failedLast bool, atEnd func(st *Stepper, state []byte) error) error {
	st, err := NewStepper(m)
	if err != nil {
		return err
	}
	n := len(tr.Steps)
	if failedLast && !mayFail && n == 0 {
		return fmt.Errorf("the trace of a violated assert has no step, so no step of it fails an assert")
	}
	dead := map[string]bool{}
	var walk func(i int, state []byte) error
	walk = func(i int, state []byte) error {
		if i == n || (mayFail && i == n-1) {
			if mayFail {
				return nil
			}
			if err := sameValuation(st, state, tr.Final); err != nil {
				return err
			}
			if atEnd != nil {
				return atEnd(st, state)
			}
			return nil
		}
		key := fmt.Sprintf("%d|%x", i, state)
		if dead[key] {
			return fmt.Errorf("step %d: no continuation from this state", i+1)
		}
		last := fmt.Errorf("step %d (%s: %s) is not an enabled move", i+1, tr.Steps[i].Process, tr.Steps[i].Command)
		for _, mv := range enabledPrefix(st, state) {
			if m.Processes[mv.Edge.Proc].Name != tr.Steps[i].Process || cex.CommandText(st.Edge(mv.Edge)) != tr.Steps[i].Command {
				continue
			}
			next, failed, err := st.Apply(state, mv)
			if err != nil {
				continue
			}
			if failedLast && !mayFail && i == n-1 && failed == nil {
				last = fmt.Errorf("step %d (%s: %s) is the last step of a violated assert, and no assert fails in it", i+1, tr.Steps[i].Process, tr.Steps[i].Command)
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

// replayOutcome replays the trace of an outcome, and checks what the verdict
// says about the state the trace ends in: the final state of a counterexample
// or a witness is the state that shows the property, not any state of the run.
// A trace that replays and ends in its own final valuation but in the wrong
// state (an id mix-up in the same layer is the case: the run, its length and
// its valuation are those of the other state) passes the replay alone.
//
//	invariant, violated    the expression is false in the final state
//	reach, verified        the expression is true in it
//	deadlock, violated     no move is enabled in it and not every process is terminated
//	assert, violated       the last step of the trace is a step in which an assert fails
//
// With a graph (the reachable states of a complete run) the final state must
// also be a state of it, and the expression is evaluated there.
func replayOutcome(m *ir.Model, o Outcome, g *Graph) error {
	if o.Trace == nil {
		return nil
	}
	mayFail := o.Status == InvalidModel
	failedLast := !mayFail && o.Status == Violated && o.Property.Kind == ir.KindAssert
	return replayTrace(m, o.Trace, mayFail, failedLast, func(st *Stepper, state []byte) error {
		var want bool // the truth the expression must have in the final state
		switch {
		case o.Status == Violated && o.Property.Kind == ir.KindInvariant:
			want = false
		case o.Status == Verified && o.Property.Kind == ir.KindReach:
			want = true
		case o.Status == Violated && o.Property.Kind == ir.KindDeadlock:
			moves, err := st.Enabled(state)
			if err != nil {
				return fmt.Errorf("the final state of the deadlock trace cannot be examined: %w", err)
			}
			if len(moves) > 0 || st.Terminated(state) {
				return fmt.Errorf("the final state of the deadlock trace has %d enabled moves (terminated: %v)", len(moves), st.Terminated(state))
			}
			return inGraph(g, state, nil, false)
		default:
			return nil
		}
		expr, err := st.Layout().Compile(o.Property.Expr, -1)
		if err != nil {
			return err
		}
		st.Layout().Timeout = false
		got, err := expr.Truth(state)
		if err != nil {
			return fmt.Errorf("the expression of %s fails in the final state of its trace: %w", o.Property.ID, err)
		}
		if got != want {
			return fmt.Errorf("the %s trace of %s (%s) ends in a state where its expression is %v", o.Property.Kind, o.Property.ID, o.Status, got)
		}
		return inGraph(g, state, o.Property.Expr, want)
	})
}

// inGraph: with a graph, the final state of a trace is one of its states, and
// the expression, when there is one, has the truth want in it there too.
func inGraph(g *Graph, state []byte, e *ir.Expr, want bool) error {
	if g == nil {
		return nil
	}
	i, ok := g.Visited.Has(state)
	if !ok {
		return fmt.Errorf("the final state of the trace is not a state of the reachable graph")
	}
	if e == nil {
		return nil
	}
	expr, err := g.Layout.Compile(e, -1)
	if err != nil {
		return err
	}
	got, err := g.Eval(expr, i)
	if err != nil {
		return fmt.Errorf("the expression fails in the graph state %d that the trace ends in: %w", i, err)
	}
	if got != want {
		return fmt.Errorf("the graph state %d that the trace ends in gives the expression %v, not %v", i, got, want)
	}
	return nil
}

// invalidKind says what ends an invalid-model trace, by replaying it: "step"
// (the last move fails to fire), "enabledness" (the moves of the last state
// cannot be listed) or "property" (an invariant or reach expression cannot be
// evaluated in the last state). It is an error if the trace is not a run that
// ends in one of them. Only the last kind depends on the order of the search:
// a property that is decided is never evaluated again, so an expression that
// fails on a state is an error in the order that meets the state first.
func invalidKind(m *ir.Model, tr *cex.Trace) (string, error) {
	st, err := NewStepper(m)
	if err != nil {
		return "", err
	}
	c, err := compile(m)
	if err != nil {
		return "", err
	}
	n := len(tr.Steps)
	var kind string
	dead := map[string]bool{} // (step, state) pairs that led nowhere: edges with equal text make the walk exponential without it
	var walk func(i int, state []byte) bool
	walk = func(i int, state []byte) bool {
		if dead[fmt.Sprintf("%d|%x", i, state)] {
			return false
		}
		if i == n {
			if _, err := st.Enabled(state); err != nil {
				kind = "enabledness"
				return true
			}
			c.layout.Timeout = false
			for _, p := range append(append([]cProp(nil), c.invs...), c.reaches...) {
				if _, err := p.expr.Truth(state); err != nil {
					kind = "property"
					return true
				}
			}
			return false
		}
		for _, mv := range enabledPrefix(st, state) {
			if m.Processes[mv.Edge.Proc].Name != tr.Steps[i].Process || cex.CommandText(st.Edge(mv.Edge)) != tr.Steps[i].Command {
				continue
			}
			next, _, err := st.Apply(state, mv)
			if err != nil {
				if i == n-1 {
					kind = "step"
					return true
				}
				continue
			}
			if walk(i+1, next) {
				return true
			}
		}
		dead[fmt.Sprintf("%d|%x", i, state)] = true
		return false
	}
	if !walk(0, st.Initial()) {
		return "", fmt.Errorf("the trace %q does not end in an error of the model", tr.Summary)
	}
	return kind, nil
}

// endingKind is the kind of the error that ended a run in an invalid model, or
// "bound" for a pool or an atomic bound.
func endingKind(t *testing.T, name string, m *ir.Model, r *Result) string {
	t.Helper()
	if !endingEvent(r) {
		return ""
	}
	if r.Stop != "invalid model" {
		return "bound"
	}
	for _, o := range r.Outcomes {
		if o.Status == InvalidModel && o.Trace != nil {
			k, err := invalidKind(m, o.Trace)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			return k
		}
	}
	return "unknown"
}

// checkParallel runs the three searches (with the sweep, so that a complete
// graph is a complete run) and checks the contract.
func checkParallel(t *testing.T, name string, m *ir.Model, workers int, kn *parKnobs) parRuns {
	t.Helper()
	return checkParallelCore(t, name, m, workers, kn, Options{}, true, true)
}

// checkParallelBudget is checkParallel under a budget that all three searches
// are given; a run that hits it says nothing and is returned unchecked.
func checkParallelBudget(t *testing.T, name string, m *ir.Model, workers int, kn *parKnobs, bud Budget) parRuns {
	t.Helper()
	return checkParallelOpt(t, name, m, workers, kn, Options{Budget: bud})
}

// checkParallelOpt is the oracle under the options base (a budget, a watch)
// that all three searches share; the sweep, the mode and the worker count are
// set here.
func checkParallelOpt(t *testing.T, name string, m *ir.Model, workers int, kn *parKnobs, base Options) parRuns {
	t.Helper()
	return checkParallelCore(t, name, m, workers, kn, base, false, true)
}

// checkParallelNoSweep is checkParallelOpt without the sweep: the searches stop
// as soon as every property is decided, as a run without --sweep does. The
// sweep hides what a search skips once the properties are decided (an
// expression that is no longer evaluated), so a defect there shows only here.
func checkParallelNoSweep(t *testing.T, name string, m *ir.Model, workers int, kn *parKnobs, base Options) parRuns {
	t.Helper()
	return checkParallelCore(t, name, m, workers, kn, base, false, false)
}

// checkParallelCore is the oracle. bfsOnAtomic says whether the sequential
// breadth-first search may run on a model with atomic steps: yes for a model
// that a person wrote (finite, small), no for a random one (see below). sweep
// says whether the three searches are asked to go on after every property is
// decided.
func checkParallelCore(t *testing.T, name string, m *ir.Model, workers int, kn *parKnobs, base Options, bfsOnAtomic, sweep bool) parRuns {
	t.Helper()
	base.Sweep = sweep
	dfsOpt, bfsOpt, parOpt := base, base, base
	bfsOpt.Mode = BFS
	parOpt.Workers, parOpt.par = workers, kn
	par := runOpts(t, m, parOpt)
	if par.Parallel == nil || !par.Parallel.Applied {
		t.Fatalf("%s: the parallel search was not applied: %+v", name, par.Parallel)
	}
	if strings.Contains(par.Stop, "an atomic sequence exceeds") {
		// A cycle of atomic steps that never blocks. Neither sequential search
		// can be run on it: the breadth-first one needs memory quadratic in the
		// length of the sequence, and the depth-first one, which has no bound on an
		// atomic sequence, explores the tree of its branches (exponential in the
		// depth the depth budget allows) without storing a state it could look at
		// the clock for. The directed test of scenario 25 covers the bound.
		return parRuns{par: par, atomicLoop: true}
	}
	dfs := runOpts(t, m, dfsOpt)
	// The sequential breadth-first search is not run on a model with atomic
	// steps: it keeps a copy of the chain of moves per intermediate state, so a
	// loop of atomic steps costs memory quadratic in its length before the bound
	// of 100 000 steps stops it, which a random model can reach and the machine
	// cannot afford. The comparison with it (layers, trace lengths) is for models
	// without atomic steps in any case; the depth-first search stands for both
	// where the verdicts are compared.
	noAtomic := !hasAtomic(m)
	var bfs *Result
	if noAtomic || bfsOnAtomic {
		bfs = runOpts(t, m, bfsOpt)
	}
	if hitBudget(dfs) || hitBudget(par) || (bfs != nil && hitBudget(bfs)) {
		return parRuns{dfs: dfs, bfs: bfs, par: par}
	}
	checkExhaustiveVerdictsAgainstTheGraph(t, name, m, par)
	// Whether a model has an ending event is a property of its graph except for
	// the errors of property expressions: a property that is decided is not
	// evaluated again, so an expression that fails on a state is an error only in
	// a search that meets the state before deciding the property. Searches may
	// then disagree on whether the run ends in an error; they must not disagree
	// about anything else. Without the sweep a search that has decided every
	// property stops, and never meets an error that lies further on in the graph:
	// a run that stops early is no witness that the graph has no such error. Only
	// a run that went through the whole graph is.
	runsOf := []*Result{dfs, par}
	if bfs != nil {
		runsOf = append(runsOf, bfs)
	}
	var ends, completes int
	for _, r := range runsOf {
		switch {
		case endingEvent(r):
			ends++
		case r.Stop == "complete":
			completes++
		}
	}
	if ends > 0 && ends < len(runsOf) {
		if completes > 0 {
			for _, r := range runsOf {
				if k := endingKind(t, name, m, r); k != "" && k != "property" {
					t.Fatalf("%s: the searches disagree on whether the run ends (depth-first %q, parallel %q) and one ends on an error of kind %q, which is a property of the graph", name, dfs.Stop, par.Stop, k)
				}
			}
		}
		for _, o := range par.Outcomes {
			if endingEvent(par) && o.Status == Verified && o.Property.Kind != ir.KindReach {
				t.Fatalf("%s: property %s verified although the run ended on %q", name, o.Property.ID, par.Stop)
			}
			if o.Trace != nil {
				if err := replayOutcome(m, o, nil); err != nil {
					t.Fatalf("%s: the %s trace of the parallel run (%s) is not a run of the model that shows its verdict: %v\n%s", name, o.Property.ID, o.Status, err, o.Trace.Summary)
				}
			}
		}
		return parRuns{dfs: dfs, bfs: bfs, par: par}
	}
	if endingEvent(dfs) || (bfs != nil && endingEvent(bfs)) {
		// A12b: the contract of a model whose graph has an ending event.
		if !endingEvent(par) {
			t.Fatalf("%s: the sequential search ends on %q (breadth-first: %v), the parallel one on %q", name, dfs.Stop, bfs != nil && endingEvent(bfs), par.Stop)
		}
		if par.Complete {
			t.Fatalf("%s: complete with an ending event", name)
		}
		for _, o := range par.Outcomes {
			if o.Status == Verified && o.Property.Kind != ir.KindReach {
				t.Fatalf("%s: property %s verified although the run ended on %q", name, o.Property.ID, par.Stop)
			}
			if o.Trace != nil {
				if err := replayOutcome(m, o, nil); err != nil {
					t.Fatalf("%s: the %s trace of the parallel run (%s) is not a run of the model that shows its verdict: %v\n%s", name, o.Property.ID, o.Status, err, o.Trace.Summary)
				}
			}
		}
		return parRuns{dfs: dfs, bfs: bfs, par: par}
	}
	if par.Stop != dfs.Stop {
		t.Fatalf("%s: stop %q, sequential %q", name, par.Stop, dfs.Stop)
	}
	if par.Complete != dfs.Complete {
		t.Fatalf("%s: complete %v, sequential %v", name, par.Complete, dfs.Complete)
	}
	if par.Complete {
		if fmt.Sprint(par.Coverage) != fmt.Sprint(dfs.Coverage) {
			t.Fatalf("%s: vacuity coverage %v, sequential %v", name, par.Coverage, dfs.Coverage)
		}
		if par.States != dfs.States || par.Transitions != dfs.Transitions || par.AtomicSteps != dfs.AtomicSteps {
			t.Fatalf("%s: states/transitions/atomic %d/%d/%d, sequential %d/%d/%d", name, par.States, par.Transitions, par.AtomicSteps, dfs.States, dfs.Transitions, dfs.AtomicSteps)
		}
		if noAtomic {
			if par.MaxDepth != bfs.MaxDepth || fmt.Sprint(par.Levels) != fmt.Sprint(bfs.Levels) {
				t.Fatalf("%s: depth %d levels %v, breadth-first %d %v", name, par.MaxDepth, par.Levels, bfs.MaxDepth, bfs.Levels)
			}
		} else if par.MaxDepth > dfs.MaxDepth {
			t.Fatalf("%s: depth %d above the depth-first %d", name, par.MaxDepth, dfs.MaxDepth)
		}
	}
	for i := range par.Outcomes {
		p, d := par.Outcomes[i], dfs.Outcomes[i]
		if p.Status != d.Status || p.Evidence != d.Evidence {
			t.Fatalf("%s: property %s is %s/%s in parallel, %s/%s depth-first\npar: %s\ndfs: %s", name, p.Property.ID, p.Status, p.Evidence, d.Status, d.Evidence, p.Reason, d.Reason)
		}
		if bfs != nil {
			if b := bfs.Outcomes[i]; p.Status != b.Status || p.Evidence != b.Evidence {
				t.Fatalf("%s: property %s is %s/%s in parallel, %s/%s breadth-first", name, p.Property.ID, p.Status, p.Evidence, b.Status, b.Evidence)
			}
		}
		if reasonKey(p) != reasonKey(d) {
			t.Fatalf("%s: property %s reason %q, sequential %q", name, p.Property.ID, p.Reason, d.Reason)
		}
		if p.Trace != nil {
			if err := replayOutcome(m, p, nil); err != nil {
				t.Fatalf("%s: the %s trace of the parallel run is not a run of the model that shows its verdict: %v\n%s", name, p.Property.ID, err, p.Trace.Summary)
			}
			if noAtomic {
				if b := bfs.Outcomes[i]; b.Trace != nil && len(p.Trace.Steps) != len(b.Trace.Steps) {
					t.Fatalf("%s: the %s trace has %d steps, the breadth-first one %d\npar: %s\nbfs: %s", name, p.Property.ID, len(p.Trace.Steps), len(b.Trace.Steps), p.Trace.Summary, b.Trace.Summary)
				}
			}
		}
		if (p.Trace == nil) != (d.Trace == nil) {
			t.Fatalf("%s: property %s has a trace: %v, depth-first has one: %v", name, p.Property.ID, p.Trace != nil, d.Trace != nil)
		}
	}
	return parRuns{dfs: dfs, bfs: bfs, par: par}
}

// checkExhaustiveVerdictsAgainstTheGraph checks, against the reachable graph
// that BuildGraph stores (a search that has no property, no event and no order
// to agree on), the verdicts that a complete run reaches by elimination: an
// invariant that stays undecided is verified, a reach condition that stays
// undecided is violated. Such a property was evaluated on every stored state, so
// no reachable state may make the invariant false or the reach condition true,
// and none may make the expression fail (a failing evaluation of an undecided
// property ends the run). This holds whatever order a search takes, so it also
// covers the runs on which the comparison with the sequential searches has to be
// loose, because the order of a search decides which expression fails first.
// It also checks, for every counterexample and witness, that the run ends in a
// state of the graph that shows the verdict (replayOutcome).
func checkExhaustiveVerdictsAgainstTheGraph(t *testing.T, name string, m *ir.Model, par *Result) {
	t.Helper()
	if !par.Complete {
		return
	}
	g, err := BuildGraph(context.Background(), m, Options{Budget: Budget{MaxStates: 60000}})
	if err != nil {
		t.Fatal(err)
	}
	if !g.Stats.Complete || g.Invalid != "" {
		return
	}
	if g.Len() != par.States {
		t.Fatalf("%s: the parallel run stored %d states, the reachable graph has %d", name, par.States, g.Len())
	}
	for _, o := range par.Outcomes {
		var want bool // the truth that every reachable state must give the expression
		var kind string
		switch {
		case o.Property.Kind == ir.KindInvariant && o.Status == Verified:
			want, kind = true, "invariant"
		case o.Property.Kind == ir.KindReach && o.Status == Violated:
			want, kind = false, "reach condition"
		default:
			continue
		}
		expr, err := g.Layout.Compile(o.Property.Expr, -1)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < g.Len(); i++ {
			got, err := g.Eval(expr, i)
			if err != nil {
				t.Fatalf("%s: the parallel run reports the %s %s as %s although its expression fails on a reachable state (%v): the property was skipped, not checked", name, kind, o.Property.ID, o.Status, err)
			}
			if got != want {
				t.Fatalf("%s: the parallel run reports the %s %s as %s (%q) although a reachable state makes it %v", name, kind, o.Property.ID, o.Status, o.Reason, got)
			}
		}
	}
	// The other direction: what a counterexample or a witness shows. Its run
	// ends in a state of the graph where the invariant is false, the reach
	// condition true, or no move is enabled; a trace to another state, of the
	// right length and a valid run, does not show it.
	for _, o := range par.Outcomes {
		if err := replayOutcome(m, o, g); err != nil {
			t.Fatalf("%s: the %s trace of the parallel run (%s) does not end in a state that shows its verdict: %v\n%s", name, o.Property.ID, o.Status, err, o.Trace.Summary)
		}
	}
}

// resultDigest is everything of a result that the report would show.
func resultDigest(r *Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d/%d/%d/%d/%v/%q/%d/%v\n", r.States, r.Transitions, r.AtomicSteps, r.MaxDepth, r.Complete, r.Stop, r.MemBytes, r.Levels)
	b.WriteString(verdictDigest(r))
	if p := r.Parallel; p != nil {
		fmt.Fprintf(&b, "layers %d max %d\n", p.Layers, p.MaxLayerStates)
	}
	return b.String()
}

// verdictDigest is the verdicts, reasons and traces of a result.
func verdictDigest(r *Result) string {
	var b strings.Builder
	for _, o := range r.Outcomes {
		tr, _ := json.Marshal(o.Trace)
		fmt.Fprintf(&b, "%s|%s|%s|%s|%s\n", o.Property.ID, o.Status, o.Evidence, o.Reason, tr)
	}
	return b.String()
}

// sameForAllKnobs: the result does not depend on the sizes that are scheduling
// and not semantics (segment size, the inline threshold): it is the same down
// to the bytes. The group size is semantics for a run that stops early (the
// counters are those of a group boundary), so a forced group size is compared
// with the default on the verdicts, the traces and the stop reason, and, for a
// run that completes, on the counters too.
func sameForAllKnobs(t *testing.T, name string, m *ir.Model, opt Options) {
	t.Helper()
	opt.Workers = 1
	var def string
	var defRes *Result
	byGroup := map[int]string{}
	for i, kn := range parKnobSets {
		o := opt
		o.par = kn
		res := runOpts(t, m, o)
		got := resultDigest(res)
		g := 0
		if kn != nil {
			g = kn.group
		}
		if i == 0 {
			def, defRes = got, res
		}
		if g == 0 {
			if got != def {
				t.Fatalf("%s: knobs %+v change the result:\n--- default\n%s--- knobs\n%s", name, kn, def, got)
			}
			continue
		}
		if want, ok := byGroup[g]; ok && got != want {
			t.Fatalf("%s: knobs %+v change the result for the group size %d:\n--- first\n%s--- knobs\n%s", name, kn, g, want, got)
		}
		byGroup[g] = got
		if verdictDigest(res) != verdictDigest(defRes) || res.Stop != defRes.Stop || res.Complete != defRes.Complete {
			t.Fatalf("%s: the group size %d changes the verdicts, traces or stop reason:\n--- default (%q)\n%s--- group %d (%q)\n%s", name, g, defRes.Stop, verdictDigest(defRes), g, res.Stop, verdictDigest(res))
		}
		if res.Complete && (res.States != defRes.States || res.Transitions != defRes.Transitions || res.AtomicSteps != defRes.AtomicSteps || res.MaxDepth != defRes.MaxDepth) {
			t.Fatalf("%s: the group size %d changes the counters of a complete run", name, g)
		}
	}
}

func TestParallelAgreesWithTheSequentialSearchesOnTheFixtures(t *testing.T) {
	for _, c := range parFixtures(t) {
		for _, kn := range parKnobSets {
			checkParallel(t, c.name, c.m, 1, kn)
		}
		sameForAllKnobs(t, c.name, c.m, Options{Sweep: true})
		sameForAllKnobs(t, c.name+" (no sweep)", c.m, Options{})
	}
}

func TestParallelFindsTheShortestCounterexamples(t *testing.T) {
	// A violated assert, a deadlock, an invariant and a reach witness.
	cases := []struct {
		name string
		m    *ir.Model
		want map[string]Status
	}{
		{"mutex_flaw", mutexFlaw(), map[string]Status{"assert": Violated, "deadlock": Verified}},
		{"petrinet1", petrinet1(), map[string]Status{"deadlock": Violated}},
		{"por-visible", porVisible(), map[string]Status{"inv": Violated, "can1": Verified, "deadlock": Verified}},
	}
	for _, c := range cases {
		for _, kn := range parKnobSets {
			runs := checkParallel(t, c.name, c.m, 1, kn)
			for id, want := range c.want {
				if got := outcome(t, runs.par, id).Status; got != want {
					t.Fatalf("%s: property %s is %s, want %s", c.name, id, got, want)
				}
			}
			for _, o := range runs.par.Outcomes {
				if o.Status == Violated && o.Trace == nil {
					t.Fatalf("%s: %s violated without a trace", c.name, o.Property.ID)
				}
			}
		}
	}
}

// The checks of the initial state belong to no expansion; without their own step
// a violation there is missed (an invariant false in the initial state is
// found only when the parallel search checks the state it stored first).
func TestParallelChecksTheInitialState(t *testing.T) {
	m := parInitialViolation()
	for _, kn := range parKnobSets {
		runs := checkParallel(t, "initial", m, 1, kn)
		inv := outcome(t, runs.par, "inv")
		if inv.Status != Violated || len(inv.Trace.Steps) != 0 {
			t.Fatalf("invariant false in the initial state: %s with %d steps", inv.Status, len(inv.Trace.Steps))
		}
		if can := outcome(t, runs.par, "can"); can.Status != Verified || len(can.Trace.Steps) != 0 {
			t.Fatalf("reach true in the initial state: %s", can.Status)
		}
	}
	// An evaluation error in an expression on the initial state is the model's
	// error, with the one-state run.
	bad := parInitialError()
	for _, kn := range parKnobSets {
		runs := checkParallel(t, "initial error", bad, 1, kn)
		o := outcome(t, runs.par, "inv")
		if o.Status != InvalidModel || o.Trace == nil || len(o.Trace.Steps) != 0 || runs.par.Stop != "invalid model" {
			t.Fatalf("an index error on the initial state: %+v stop %q", o, runs.par.Stop)
		}
		if runs.par.States != 1 {
			t.Fatalf("%d states after an error on the initial state", runs.par.States)
		}
	}
}

// The oracle must not accept a trace that cannot show its verdict, and a trace
// of no step is the case that slipped through: replayTrace's walk returned at
// i == n before it looked at failedLast, and replayOutcome has no case for an
// assert, so an empty counterexample of a violated assert passed every check
// (a run of zero steps ends in the initial state, where no assert has failed:
// an assert is a step). For the other kinds an empty trace is a legitimate run
// of zero steps when the initial state is itself the answer
// (TestParallelChecksTheInitialState), and is rejected by the final-state check
// when it is not. The outcomes are hand-built from those of a real parallel run
// (which the oracle must accept, or the test would pass for a check that
// rejects everything), with two empty traces: the one a run of zero steps has,
// with the valuation of the initial state, and one with no valuation at all.
// The last rows cut the last step off a trace of two steps, the case that was
// already caught.
func TestParallelOracleRejectsATraceWithoutTheStepsOfItsVerdict(t *testing.T) {
	cases := []struct {
		name   string
		m      *ir.Model
		id     string
		status Status
	}{
		{"assert", promelaModel(t, "byte x; active proctype P() { x = 1; assert(x == 0) }"), "assert", Violated},
		{"invariant", porVisible(), "inv", Violated},
		{"reach", porVisible(), "can1", Verified},
		{"deadlock", petrinet1(), "deadlock", Violated},
	}
	for _, c := range cases {
		res := runOpts(t, c.m, Options{Workers: 2, Budget: Budget{MaxStates: 100000}})
		o := *outcome(t, res, c.id)
		if o.Status != c.status || o.Trace == nil || len(o.Trace.Steps) == 0 {
			t.Fatalf("%s: the run gives %s with trace %v, want %s and a trace of steps", c.name, o.Status, o.Trace, c.status)
		}
		if err := replayOutcome(c.m, o, nil); err != nil {
			t.Fatalf("%s: the oracle rejects the real trace: %v", c.name, err)
		}
		st, err := NewStepper(c.m)
		if err != nil {
			t.Fatal(err)
		}
		empty := map[string]*cex.Trace{
			"with the initial valuation": cex.Build(st.Layout(), [][]byte{st.Initial()}, nil),
			"with no valuation":          {},
		}
		for what, tr := range empty {
			if len(tr.Steps) != 0 {
				t.Fatalf("%s: the empty trace %s has %d steps", c.name, what, len(tr.Steps))
			}
			bad := o
			bad.Trace = tr
			if err := replayOutcome(c.m, bad, nil); err == nil {
				t.Errorf("%s: the oracle accepts a trace of no step %s as the %s of %s", c.name, what, bad.Status, c.id)
			}
		}
	}
	// The case that was already caught: the failing step is cut off.
	m := promelaModel(t, "byte x; active proctype P() { x = 1; assert(x == 0) }")
	res := runOpts(t, m, Options{Workers: 2, Budget: Budget{MaxStates: 1000}})
	o := *outcome(t, res, "assert")
	if o.Trace == nil || len(o.Trace.Steps) != 2 {
		t.Fatalf("the assert trace has %v, want two steps", o.Trace)
	}
	st, err := NewStepper(m)
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.Enabled(st.Initial())
	if err != nil || len(first) != 1 {
		t.Fatalf("the first move of the model: %v (%d moves)", err, len(first))
	}
	next, _, err := st.Apply(st.Initial(), first[0])
	if err != nil {
		t.Fatal(err)
	}
	cut := o
	cut.Trace = cex.Build(st.Layout(), [][]byte{st.Initial(), next}, []cex.Ref{first[0].Ref()})
	if len(cut.Trace.Steps) != 1 {
		t.Fatalf("the cut trace has %d steps", len(cut.Trace.Steps))
	}
	if err := replayOutcome(m, cut, nil); err == nil {
		t.Errorf("the oracle accepts an assert trace without the step that fails the assert")
	}
}

func TestParallelInvalidModelTraceReachesTheOffendingStep(t *testing.T) {
	m := promelaModel(t, `byte b = 254;
active proctype P() { do :: b = b + 1 od }`)
	for _, kn := range parKnobSets {
		runs := checkParallel(t, "overflow", m, 1, kn)
		for _, o := range runs.par.Outcomes {
			if o.Status != InvalidModel || !strings.Contains(o.Reason, "domain overflow") {
				t.Fatalf("%s: %s %q", o.Property.ID, o.Status, o.Reason)
			}
			b := outcome(t, runs.bfs, o.Property.ID)
			if len(o.Trace.Steps) != len(b.Trace.Steps) || o.Trace.Summary != b.Trace.Summary {
				t.Fatalf("trace %q (%d steps), breadth-first %q (%d)", o.Trace.Summary, len(o.Trace.Steps), b.Trace.Summary, len(b.Trace.Steps))
			}
		}
	}
}

// A fire that returns an error does not also report its failed assert: the
// sequential code looks at the error first (scenario 30). One edge carries an
// assert that is false and an effect that overflows its variable.
func TestParallelAnErrorOfAStepSuppressesItsFailedAssert(t *testing.T) {
	m := parAssertOverflow()
	for _, kn := range parKnobSets {
		runs := checkParallel(t, "assert and overflow", m, 1, kn)
		a := outcome(t, runs.par, "assert")
		if a.Status != InvalidModel || !strings.Contains(a.Reason, "domain overflow") {
			t.Fatalf("assert is %s (%s), want invalid-model as sequentially (%s)", a.Status, a.Reason, outcome(t, runs.dfs, "assert").Status)
		}
	}
}

// An evaluation error in the expression of a decided property never surfaces
// (scenario 24): the invariant is violated in layer 1, a later state has the
// array index out of range, and no sweep is asked for.
func TestParallelADecidedPropertyIsNeverEvaluatedAgain(t *testing.T) {
	m := parDecided()
	for _, opt := range []Options{{}, {Sweep: true}} {
		seq := runOpts(t, m, opt)
		if seq.Outcomes[0].Status != Violated || seq.Outcomes[1].Status != Verified || !seq.Complete {
			t.Fatalf("sequential: invariant %s deadlock %s complete %v", seq.Outcomes[0].Status, seq.Outcomes[1].Status, seq.Complete)
		}
		for _, kn := range parKnobSets {
			o := opt
			o.Workers, o.par = 1, kn
			par := runOpts(t, m, o)
			for i := range par.Outcomes {
				if par.Outcomes[i].Status != seq.Outcomes[i].Status {
					t.Fatalf("sweep=%v knobs %+v: %s is %s in parallel, %s sequentially (stop %q / %q)", opt.Sweep, kn, par.Outcomes[i].Property.ID, par.Outcomes[i].Status, seq.Outcomes[i].Status, par.Stop, seq.Stop)
				}
			}
			if !par.Complete {
				t.Fatalf("knobs %+v: not complete (%s)", kn, par.Stop)
			}
		}
	}
}

// An evaluation error in the expression of a property that an earlier state of
// the group has decided is dropped when the events are applied (scenario 24),
// and the checks of the other properties on the same state must not be dropped
// with it: the sequential search skips the decided property and goes on to the
// next one. A parallel search that stops checking the state at the error misses
// the only witness of a reach condition (reported as violated by a complete
// search) or the only violation of an invariant (reported as verified).
func TestParallelAnErrorOfADecidedPropertyDoesNotHideTheOtherChecksOfTheState(t *testing.T) {
	cases := []struct {
		name string
		m    *ir.Model
		want map[string]Status
		// prop is the property whose check on the state of the error the early
		// exit took, and steps the length of its trace
		prop  string
		steps int
	}{
		{"reach", parSkippedReach(), map[string]Status{"inv": Violated, "reach5": Verified}, "reach5", 1},
		{"invariant", parSkippedInvariant(), map[string]Status{"inv": Violated, "inv5": Violated}, "inv5", 1},
	}
	for _, c := range cases {
		for _, sweep := range []bool{false, true} {
			seqs := map[string]*Result{
				"depth-first":   runOpts(t, c.m, Options{Sweep: sweep}),
				"breadth-first": runOpts(t, c.m, Options{Sweep: sweep, Mode: BFS}),
			}
			for mode, seq := range seqs {
				for id, want := range c.want {
					if got := outcome(t, seq, id).Status; got != want {
						t.Fatalf("%s sweep=%v: the %s search says %s is %s, the test expects %s", c.name, sweep, mode, id, got, want)
					}
				}
			}
			for _, kn := range parKnobSets {
				for _, w := range []int{1, 3, 4} {
					par := runOpts(t, c.m, Options{Sweep: sweep, Workers: w, par: kn})
					for id, want := range c.want {
						o := outcome(t, par, id)
						if o.Status != want || o.Evidence != Exhaustive {
							t.Fatalf("%s sweep=%v knobs %+v %d workers: %s is %s/%s (%s), the sequential searches say %s/exhaustive", c.name, sweep, kn, w, id, o.Status, o.Evidence, o.Reason, want)
						}
					}
					if tr := outcome(t, par, c.prop).Trace; tr == nil || len(tr.Steps) != c.steps {
						t.Fatalf("%s sweep=%v knobs %+v %d workers: the trace of %s is %+v, want %d step", c.name, sweep, kn, w, c.prop, tr, c.steps)
					}
					if par.Stop != seqs["breadth-first"].Stop {
						t.Fatalf("%s sweep=%v knobs %+v %d workers: stop %q, breadth-first %q", c.name, sweep, kn, w, par.Stop, seqs["breadth-first"].Stop)
					}
				}
			}
		}
	}
}

// A watch expression that fails to evaluate is not like a property that is
// decided: it is evaluated on every stored state and never skipped, and the
// sequential checkState returns at the failure, so the checks after it on that
// state are not made and the run ends there. The parallel search stops checking
// the state at it too. On the model of parSkippedReach the watch a[i] == 0 fails
// on the state with i == 5, the state that decides the reach condition: the
// invariant was decided by the first state and keeps its verdict, and the reach
// condition is invalid-model in every search.
func TestParallelAFailingWatchExpressionEndsTheRunLikeTheSequentialSearch(t *testing.T) {
	m := parSkippedReach()
	watch := []*ir.Expr{ir.Binary("eq", ir.Index("a", ir.Ref("i")), ir.Const(0))}
	for _, sweep := range []bool{false, true} {
		for _, mode := range []Mode{DFS, BFS} {
			seq := runOpts(t, m, Options{Sweep: sweep, Mode: mode, Watch: watch})
			if outcome(t, seq, "inv").Status != Violated || outcome(t, seq, "reach5").Status != InvalidModel || seq.Stop != "invalid model" {
				t.Fatalf("sequential %s sweep=%v: inv %s, reach5 %s, stop %q", mode, sweep, outcome(t, seq, "inv").Status, outcome(t, seq, "reach5").Status, seq.Stop)
			}
			for _, kn := range parKnobSets {
				for _, w := range []int{1, 3} {
					par := runOpts(t, m, Options{Sweep: sweep, Watch: watch, Workers: w, par: kn})
					for _, id := range []string{"inv", "reach5"} {
						if got, want := outcome(t, par, id).Status, outcome(t, seq, id).Status; got != want {
							t.Fatalf("sweep=%v knobs %+v %d workers: %s is %s in parallel, %s sequentially (%s)", sweep, kn, w, id, got, want, mode)
						}
					}
					if par.Stop != seq.Stop {
						t.Fatalf("sweep=%v knobs %+v %d workers: stop %q, sequential %q", sweep, kn, w, par.Stop, seq.Stop)
					}
				}
			}
		}
	}
}

// Errors of the enabledness probing of a state and an assert on neighbouring
// steps of the same expansion follow the order of the sequential code's steps
// (scenario 22).
func TestParallelEventsFollowTheOrderOfTheStepsOfOneExpansion(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		// the assert fails on the first move; the probing of the second edge
		// fails after it
		{"assert then guard error", `byte z; byte x;
active proctype P() { do :: x = 1; assert(x == 0) :: (10 / z > 0) -> x = 2 od }`},
		// the probing of the first edge fails before any move is made
		{"guard error then assert", `byte z; byte x;
active proctype P() { do :: (10 / z > 0) -> x = 2 :: x = 1; assert(x == 0) od }`},
		{"two asserts", `byte x;
active proctype P() { do :: x = 1; assert(x == 0) :: x = 2; assert(x == 5) od }`},
	} {
		m := promelaModel(t, tc.src)
		for _, kn := range parKnobSets {
			runs := checkParallel(t, tc.name, m, 1, kn)
			for i := range runs.par.Outcomes {
				if runs.par.Outcomes[i].Status != runs.bfs.Outcomes[i].Status {
					t.Fatalf("%s knobs %+v: %s is %s in parallel, %s breadth-first", tc.name, kn, runs.par.Outcomes[i].Property.ID, runs.par.Outcomes[i].Status, runs.bfs.Outcomes[i].Status)
				}
			}
		}
	}
}

// The model of plan §3.7: an assert violated on one branch and a domain
// overflow on the other. The depth-first and the breadth-first search already
// answer differently (the assert is violated in one, invalid-model in the
// other); the parallel search has its own order and the contract is the
// legal outcomes and determinism, not a winner.
func TestParallelOrderDependentOutcomes(t *testing.T) {
	m := promelaModel(t, `byte x; byte b = 255;
active proctype A() { x = 1; x = 2; assert(x != 2) }
active proctype B() { b = b + 1 }`)
	var want string
	for i, kn := range parKnobSets {
		runs := checkParallel(t, "par-order", m, 1, kn)
		a := outcome(t, runs.par, "assert")
		if a.Status != Violated && a.Status != InvalidModel {
			t.Fatalf("assert is %s", a.Status)
		}
		for _, o := range runs.par.Outcomes {
			if o.Status == Verified {
				t.Fatalf("%s verified on a run that ends in an invalid model", o.Property.ID)
			}
		}
		got := resultDigest(runs.par)
		if i == 0 {
			want = got
		} else if got != want {
			t.Fatalf("knobs %+v change the result", kn)
		}
	}
}

func parModelCount(def int) (n int, first int64) {
	n, first = def, 1
	if testing.Short() {
		n = def / 6
	}
	if v, err := strconv.Atoi(os.Getenv("MCD_PAR_MODELS")); err == nil && v > 0 {
		n = v // a deeper one-off run: MCD_PAR_MODELS=100000 go test ./explore -run TestParallelAgrees
	}
	if v, err := strconv.ParseInt(os.Getenv("MCD_PAR_SEED"), 10, 64); err == nil && v > 0 {
		first = v // MCD_PAR_SEED=500000 moves the run to models nobody has checked yet
	}
	return n, first
}

func TestParallelAgreesWithTheSequentialSearchesOnRandomModels(t *testing.T) {
	models, first := parModelCount(2400)
	var ending, complete, violated, skipped int
	for seed := first; seed < first+int64(models); seed++ {
		m := randomPORModel(rand.New(rand.NewSource(seed)))
		name := fmt.Sprintf("seed %d", seed)
		kn := parKnobSets[int(seed)%len(parKnobSets)]
		bud := Budget{MaxStates: 30000, MaxDepth: 20000}
		checkParallelNoSweep(t, name+" (no sweep)", m, 1, kn, Options{Budget: bud})
		runs := checkParallelBudget(t, name, m, 1, kn, bud)
		switch {
		case runs.atomicLoop:
			skipped++
		case hitBudget(runs.dfs) || hitBudget(runs.par):
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
	t.Logf("%d random models: %d complete (%d with a violation), %d ending in an error of the model, %d skipped (budget)", models, complete, violated, ending, skipped)
	if complete < models/2 || violated < models/10 {
		t.Fatalf("the generator no longer exercises the search: %d complete, %d violated of %d", complete, violated, models)
	}
}

// Two moves of one state that reach the same successor: the counterexample
// takes the first, the one the search took (the record that won the insertion),
// and not the last. The two edges have different texts so the trace says which.
func TestParallelCounterexampleTakesTheFirstOfTwoMovesToTheSameState(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "duplicate-successor",
		Globals: []ir.Var{byteVar("x")},
		Processes: []ir.Process{{Name: "P", Locations: locs("l0", "l1", "l2"), Edges: []ir.Edge{
			{From: 0, To: 1, Effect: []ir.Assign{{Var: "x", Value: ir.Const(1)}}, Text: "first: x = 1"},
			{From: 0, To: 1, Effect: []ir.Assign{{Var: "x", Value: ir.Const(1)}}, Text: "second: x = 1"},
			{From: 1, To: 2, Assert: ir.Const(0), Text: "assert(0)"},
		}}},
		Properties: []ir.Property{{ID: "assert", Kind: ir.KindAssert}}}
	seq := runOpts(t, m, Options{Mode: BFS, Sweep: true})
	for _, kn := range parKnobSets {
		for _, w := range []int{1, 4} {
			res := runOpts(t, m, Options{Workers: w, Sweep: true, par: kn})
			tr := outcome(t, res, "assert").Trace
			if tr == nil || len(tr.Steps) != 2 || tr.Steps[0].Command != "first: x = 1" {
				t.Fatalf("knobs %+v, %d workers: trace %+v, want the first of the two duplicate moves", kn, w, tr)
			}
			if got, want := tr.Summary, outcome(t, seq, "assert").Trace.Summary; got != want {
				t.Fatalf("trace %q, breadth-first %q", got, want)
			}
		}
	}
}

// reachableStates is every state a run of the model can be in, found by a plain
// search through the Stepper that skips a move whose firing fails: the states
// of a run that ends in an error of the model must be among them, whatever was
// reached before the run stopped.
func reachableStates(t *testing.T, m *ir.Model, limit int) (map[string]bool, bool) {
	t.Helper()
	st, err := NewStepper(m)
	if err != nil {
		t.Fatal(err)
	}
	init := st.Initial()
	seen := map[string]bool{string(init): true}
	queue := [][]byte{init}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		for _, mv := range enabledPrefix(st, s) {
			next, _, err := st.Apply(s, mv)
			if err != nil {
				continue
			}
			if !seen[string(next)] {
				if len(seen) >= limit {
					return nil, false
				}
				seen[string(next)] = true
				queue = append(queue, next)
			}
		}
	}
	return seen, true
}

// A move whose firing fails leaves a half-written vector in the scratch buffer:
// it must not be stored (mutant: an errored fire still leaves a record). The
// states of every run, also one that ends in an error, are reachable states.
func TestParallelStoredStatesAreReachableEvenWhenTheRunEndsInAnError(t *testing.T) {
	models, first := parModelCount(600)
	checked := 0
	for seed := first; seed < first+int64(models); seed++ {
		m, _ := randomParModel(rand.New(rand.NewSource(seed)))
		if hasAtomic(m) {
			continue // an atomic loop could not be run sequentially (see checkParallelCore)
		}
		seq := runOpts(t, m, Options{Sweep: true, Budget: Budget{MaxStates: 20000, MaxDepth: 20000}})
		if seq.Stop != "invalid model" {
			continue
		}
		reach, ok := reachableStates(t, m, 20000)
		if !ok {
			continue
		}
		for _, kn := range []*parKnobs{nil, {inline: -1, segment: 2}} {
			_, r := parSearchBudget(t, m, 1, kn, Budget{MaxStates: 20000})
			for st := range parStates(r) {
				if !reach[st] {
					t.Fatalf("seed %d knobs %+v: the parallel run stored a state that no run reaches (%x)", seed, kn, st)
				}
			}
		}
		checked++
	}
	if checked < 10 {
		t.Fatalf("only %d models ending in an error were checked", checked)
	}
}
