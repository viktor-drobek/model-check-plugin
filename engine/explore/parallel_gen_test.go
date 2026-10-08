package explore

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"modelcheck/ir"
)

// The random models of the parallel oracle (performance plan 5, §5.3). The
// reduction's generator (randomPORModel) is the base: it already has asserts,
// invariants and reach conditions over one and two variables, d_step chains,
// else edges, channels and clears. The parallel search supports what the
// reduction refuses, so this generator adds it: atomic sequences, rendezvous
// channels, `timeout`, `provided`, dynamic processes started by `run` (with
// pools small enough to be exhausted), a never claim that is stored and not
// executed, guards and property expressions that can fail to evaluate (a
// division by a variable, an array indexed by one), and a vacuity watch. Half of
// the models also get two to four more invariant and reach properties, some of
// which fail to evaluate on the states that another one is decided on (see
// addErringProperties): a property that is decided is not evaluated again, and
// the checks of the properties that are not decided must go on whatever
// happens to the ones that are.

func randomParModel(rnd *rand.Rand) (*ir.Model, []*ir.Expr) {
	for {
		m := randomPORModel(rnd)
		watch := enrichParModel(rnd, m)
		if ir.Validate(m) == nil {
			n := len(m.Properties)
			addErringProperties(rnd, m)
			if ir.Validate(m) != nil {
				m.Properties = m.Properties[:n]
			}
			return m, watch
		}
	}
}

// addErringProperties adds, to half of the models, two to four invariant and
// reach properties. Each is one of: an array indexed by a global (it fails where
// the global is not a valid index), a division by a global (it fails where the
// global is zero), a comparison of a global with a constant (it never fails), or
// a companion of an earlier one that fails: a reach condition or an invariant
// that is decided on exactly the states where that one fails. A property that
// fails on a state and a property that is decided on the same state are the
// pair that a search can get wrong if it stops checking a state at an error, and
// that a random pair meets only once in tens of thousands of models.
func addErringProperties(rnd *rand.Rand, m *ir.Model) {
	if rnd.Intn(2) == 0 {
		return
	}
	var globals []string
	hasArray := false
	for _, g := range m.Globals {
		if g.Len == 0 {
			globals = append(globals, g.Name)
		} else if g.Name == "a" {
			hasArray = true
		}
	}
	if len(globals) == 0 {
		return
	}
	type failure struct {
		v   string
		bad int64 // the value of v on which the expression fails
	}
	var failing []failure
	for i, n := 0, 2+rnd.Intn(3); i < n; i++ {
		var e *ir.Expr
		v := globals[rnd.Intn(len(globals))]
		kind := ir.KindInvariant
		if rnd.Intn(2) == 0 {
			kind = ir.KindReach
		}
		switch k := rnd.Intn(5); {
		case k <= 1 && hasArray:
			e = ir.Binary("eq", ir.Index("a", ir.Ref(v)), ir.Const(int64(rnd.Intn(3))))
			failing = append(failing, failure{v, 2})
		case k <= 2:
			e = ir.Binary("gt", ir.Binary("div", ir.Const(6), ir.Ref(v)), ir.Const(int64(rnd.Intn(3))))
			failing = append(failing, failure{v, 0})
		case k == 3 && len(failing) > 0:
			f := failing[rnd.Intn(len(failing))]
			if kind == ir.KindReach {
				e = ir.Binary("eq", ir.Ref(f.v), ir.Const(f.bad))
			} else {
				e = ir.Binary("ne", ir.Ref(f.v), ir.Const(f.bad))
			}
		default:
			e = ir.Binary("eq", ir.Ref(v), ir.Const(int64(rnd.Intn(3))))
		}
		id := fmt.Sprintf("x%d", i)
		m.Properties = append(m.Properties, ir.Property{ID: id, Kind: kind, Expr: e, Text: id})
	}
}

func enrichParModel(rnd *rand.Rand, m *ir.Model) []*ir.Expr {
	np := len(m.Processes)
	var globals []string
	hasArray := false
	for _, g := range m.Globals {
		if g.Len == 0 {
			globals = append(globals, g.Name)
		} else if g.Name == "a" {
			hasArray = true
		}
	}
	glob := func() string { return globals[rnd.Intn(len(globals))] }
	val := func() *ir.Expr { return ir.Const(int64(rnd.Intn(3))) }
	// A rendezvous channel in one model of four that has a channel.
	if len(m.Channels) > 0 && rnd.Intn(4) == 0 {
		m.Channels[rnd.Intn(len(m.Channels))].Capacity = 0
	}
	// Atomic steps are added only where they cannot form a loop that never
	// blocks: an atomic edge forward in the process, in a model with no d_step
	// (a d_step continues into the next edge at once, atomic or not, so d_step
	// and an atomic edge can loop forever) and no rendezvous (a handshake hands
	// the exclusive control to the partner). The sequential searches cannot be
	// run on such a loop: the breadth-first one keeps a copy of the chain of
	// moves per intermediate state and needs memory quadratic in its length, the
	// depth-first one has no bound on an atomic sequence and explores the tree
	// of its branches. A directed test covers the bound, for the parallel search.
	atomicOK := true
	for _, ch := range m.Channels {
		atomicOK = atomicOK && ch.Capacity > 0
	}
	for p := range m.Processes {
		for _, e := range m.Processes[p].Edges {
			atomicOK = atomicOK && !e.DStep
		}
	}
	for p := range m.Processes {
		pr := &m.Processes[p]
		for ei := range pr.Edges {
			e := &pr.Edges[ei]
			switch rnd.Intn(14) {
			case 0, 1:
				if atomicOK && e.To > e.From {
					e.Atomic = true // keep exclusive control after this edge
				}
			case 2:
				// timeout: alone, or as a conjunct of the guard
				if e.Else {
					break
				}
				if e.Guard == nil {
					e.Guard = ir.Timeout()
				} else {
					e.Guard = ir.And(e.Guard, ir.Timeout())
				}
			case 3:
				// a guard that cannot always be evaluated
				if e.Else {
					break
				}
				g := ir.Binary("gt", ir.Binary("div", ir.Const(10), ir.Ref(glob())), ir.Const(int64(rnd.Intn(4))))
				if e.Guard == nil {
					e.Guard = g
				} else {
					e.Guard = ir.And(e.Guard, g)
				}
			case 4:
				if e.Send == nil && e.Recv == nil && e.Run == nil {
					// an effect that can leave its domain
					e.Effect = append(e.Effect, ir.Assign{Var: glob(), Value: ir.Binary("add", ir.Ref(glob()), ir.Const(int64(1+rnd.Intn(255))))})
				}
			}
		}
		// provided: a process-level guard
		if rnd.Intn(8) == 0 {
			if rnd.Intn(2) == 0 && np > 1 {
				pr.Provided = ir.Binary("ne", ir.PC((p+1)%np), ir.Const(int64(rnd.Intn(3))))
			} else {
				pr.Provided = ir.Binary("ne", ir.Ref(glob()), val())
			}
		}
	}
	// Dynamic processes and a `run` that starts them: a pool of one or two
	// instances, started from an edge of an existing process, so that a model
	// can try to start a third while two are alive (a pool exhausted).
	if rnd.Intn(4) == 0 {
		pool := 1 + rnd.Intn(2)
		var idx []int
		for k := 0; k < pool; k++ {
			d := ir.Process{Name: fmt.Sprintf("D%d", k), Dynamic: true, Locations: make([]ir.Location, 3), Initial: 0}
			d.Edges = []ir.Edge{
				{From: 1, To: 2, Effect: []ir.Assign{{Var: glob(), Value: val()}}, Text: "d work"},
				{From: 2, To: 0, Text: "d end"},
			}
			if atomicOK && rnd.Intn(2) == 0 {
				d.Edges[0].Atomic = true // 1 -> 2: forward, so it cannot loop
			}
			idx = append(idx, len(m.Processes))
			m.Processes = append(m.Processes, d)
		}
		sp := rnd.Intn(np)
		pr := &m.Processes[sp]
		from := rnd.Intn(len(pr.Locations))
		pr.Edges = append(pr.Edges, ir.Edge{From: from, To: rnd.Intn(len(pr.Locations)),
			Run: &ir.RunOp{Proc: idx[0], Pool: append([]int(nil), idx...), Entry: 1}, Text: "run D"})
	}
	// A never claim: stored, never executed, not part of the deadlock rule.
	if rnd.Intn(5) == 0 {
		m.Processes = append(m.Processes, ir.Process{Name: "claim", Claim: true, Locations: make([]ir.Location, 2),
			Edges: []ir.Edge{{From: 0, To: 1, Guard: ir.Binary("eq", ir.Ref(glob()), val()), Text: "claim edge"}}})
	}
	// Property expressions that can fail to evaluate on some reachable state.
	if rnd.Intn(4) == 0 {
		bad := ir.Binary("eq", ir.Index("a", ir.Ref(glob())), val())
		if !hasArray || rnd.Intn(2) == 0 {
			bad = ir.Binary("gt", ir.Binary("div", ir.Const(6), ir.Ref(glob())), ir.Const(int64(rnd.Intn(3))))
		}
		kind := ir.KindInvariant
		if rnd.Intn(3) == 0 {
			kind = ir.KindReach
		}
		m.Properties = append(m.Properties, ir.Property{ID: "erring", Kind: kind, Expr: bad, Text: "erring"})
	}
	var watch []*ir.Expr
	if rnd.Intn(3) == 0 {
		watch = append(watch, ir.Binary("eq", ir.Ref(glob()), val()), ir.Binary("ne", ir.PC(rnd.Intn(np)), ir.Const(int64(rnd.Intn(3)))))
	}
	return watch
}

func TestParallelAgreesWithTheSequentialSearchesOnRichRandomModels(t *testing.T) {
	models, first := parModelCount(2400)
	var ending, complete, violated, skipped, atomic, pools, loops, erring, failed int
	for seed := first; seed < first+int64(models); seed++ {
		// A subtest per model, so that a campaign over many seeds reports every
		// failing seed and not only the first.
		if !t.Run(fmt.Sprint(seed), func(t *testing.T) {
			m, watch := randomParModel(rand.New(rand.NewSource(seed)))
			name := fmt.Sprintf("rich seed %d", seed)
			started := time.Now()
			kn := parKnobSets[int(seed)%len(parKnobSets)]
			workers := []int{1, 2, 4}[int(seed)%3]
			base := Options{Budget: Budget{MaxStates: 30000, MaxDepth: 20000}, Watch: watch}
			// The same model without the sweep, where a search stops once every
			// property is decided and the properties it has decided are not
			// evaluated again; the verdicts must agree there too.
			checkParallelNoSweep(t, name+" (no sweep)", m, workers, kn, base)
			runs := checkParallelCore(t, name, m, workers, kn, base, false, true)
			if d := time.Since(started); d > 3*time.Second {
				t.Logf("%s took %v", name, d)
			}
			switch {
			case runs.atomicLoop:
				loops++
				return
			case hitBudget(runs.dfs) || hitBudget(runs.par):
				skipped++
				return
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
			if runs.dfs.AtomicSteps > 0 {
				atomic++
			}
			if len(m.Processes) > 0 && hasRunEdge(m) {
				pools++
			}
			if hasExtraProperties(m) {
				erring++
			}
		}) {
			failed++
		}
	}
	t.Logf("%d rich random models: %d complete (%d with a violation), %d ending in an error or a bound, %d skipped (budget), %d with an atomic loop; %d with atomic steps, %d with a run, %d with extra properties; %d failed", models, complete, violated, ending, skipped, loops, atomic, pools, erring, failed)
	if failed > 0 {
		return
	}
	if complete < models/5 || violated < models/20 || atomic < models/60 || pools < models/20 || ending < models/20 || erring < models/20 {
		t.Fatalf("the generator no longer exercises the search: %d complete, %d violated, %d atomic, %d run, %d ending, %d with extra properties of %d", complete, violated, atomic, pools, ending, erring, models)
	}
}

// randomFailingPropertyModel is a small model built for one situation: a state
// on which the expression of one property fails to evaluate (an array indexed
// by 5, a division by 0) while another property is decided there, and the first
// one was decided on an earlier state of the same layer, so that a search does
// not evaluate it again and the run goes on. The processes are acyclic (every
// edge goes forward), so every run completes unless a property expression fails
// on a state before the property is decided. Where the properties of rich
// random models meet this once in tens of thousands of models, these meet it in
// a few of every hundred.
func randomFailingPropertyModel(rnd *rand.Rand) *ir.Model {
	m := &ir.Model{Schema: ir.Schema, Name: "failing-properties",
		Globals: []ir.Var{{Name: "i", Type: ir.Byte}, {Name: "j", Type: ir.Byte, Init: []int64{1}}, {Name: "a", Type: ir.Byte, Len: 2}}}
	effect := func() (ir.Assign, string) {
		switch k := rnd.Intn(5); {
		case k <= 1:
			v := []int64{0, 1, 2, 5}[rnd.Intn(4)]
			return set("i", v), fmt.Sprintf("i = %d", v)
		case k == 2:
			v := int64(rnd.Intn(3))
			return set("j", v), fmt.Sprintf("j = %d", v)
		default:
			idx, v := int64(rnd.Intn(2)), int64(rnd.Intn(3))
			return ir.Assign{Var: "a", Index: ir.Const(idx), Value: ir.Const(v)}, fmt.Sprintf("a[%d] = %d", idx, v)
		}
	}
	for p, np := 0, 1+rnd.Intn(2); p < np; p++ {
		nloc := 3 + rnd.Intn(2)
		pr := ir.Process{Name: fmt.Sprintf("P%d", p), Locations: make([]ir.Location, nloc)}
		for e, ne := 0, 2+rnd.Intn(4); e < ne; e++ {
			from := rnd.Intn(nloc - 1)
			as, text := effect()
			pr.Edges = append(pr.Edges, ir.Edge{From: from, To: from + 1 + rnd.Intn(nloc-from-1), Effect: []ir.Assign{as}, Atomic: rnd.Intn(6) == 0, Text: text})
		}
		m.Processes = append(m.Processes, pr)
	}
	for k, n := 0, 3+rnd.Intn(4); k < n; k++ {
		kind := ir.KindInvariant
		if rnd.Intn(2) == 0 {
			kind = ir.KindReach
		}
		var e *ir.Expr
		switch rnd.Intn(5) {
		case 0:
			e = ir.Binary("eq", ir.Index("a", ir.Ref("i")), ir.Const(int64(rnd.Intn(3))))
		case 1:
			e = ir.Binary("gt", ir.Binary("div", ir.Const(6), ir.Ref("j")), ir.Const(int64(rnd.Intn(3))))
		case 2:
			op := []string{"eq", "ne"}[rnd.Intn(2)]
			e = ir.Binary(op, ir.Ref("i"), ir.Const([]int64{0, 1, 2, 5}[rnd.Intn(4)]))
		case 3:
			op := []string{"eq", "ne"}[rnd.Intn(2)]
			e = ir.Binary(op, ir.Ref("j"), ir.Const(int64(rnd.Intn(3))))
		default:
			e = ir.Binary("eq", ir.Index("a", ir.Const(int64(rnd.Intn(2)))), ir.Const(int64(rnd.Intn(3))))
		}
		id := fmt.Sprintf("p%d", k)
		m.Properties = append(m.Properties, ir.Property{ID: id, Kind: kind, Expr: e, Text: id})
	}
	if rnd.Intn(2) == 0 {
		m.Properties = append(m.Properties, ir.Property{ID: "deadlock", Kind: ir.KindDeadlock})
	}
	return m
}

// A property whose expression fails on a reachable state, and the other
// properties, with and without the sweep, on models made for the pair (see
// randomFailingPropertyModel). A search that evaluates a property that the
// sequential search skips, and treats its failure as the end of the check of the
// state, shows here: the verdicts of the properties that are not decided
// would be wrong, or differ from the sequential ones.
func TestParallelAgreesWithTheSequentialSearchesWhenAPropertyFailsToEvaluate(t *testing.T) {
	models, first := parModelCount(6000)
	var complete, skipped, ending, failed int
	for seed := first; seed < first+int64(models); seed++ {
		if !t.Run(fmt.Sprint(seed), func(t *testing.T) {
			m := randomFailingPropertyModel(rand.New(rand.NewSource(seed)))
			if err := ir.Validate(m); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			kn := parKnobSets[int(seed)%len(parKnobSets)]
			workers := []int{1, 2, 4}[int(seed)%3]
			name := fmt.Sprintf("failing-property seed %d", seed)
			base := Options{Budget: Budget{MaxStates: 30000, MaxDepth: 20000}}
			checkParallelNoSweep(t, name+" (no sweep)", m, workers, kn, base)
			runs := checkParallelCore(t, name, m, workers, kn, base, true, true)
			if endingEvent(runs.dfs) || endingEvent(runs.par) {
				ending++
				return
			}
			complete++
			if skippedFailure(t, m, runs.par) {
				skipped++
			}
		}) {
			failed++
		}
	}
	t.Logf("%d models: %d complete, %d of them with a property that fails on a state after it was decided, %d ending in an error; %d failed", models, complete, skipped, ending, failed)
	if failed == 0 && (complete < models/3 || skipped < models/20) {
		t.Fatalf("the generator no longer exercises the pair: %d complete, %d with a skipped failure of %d", complete, skipped, models)
	}
}

// skippedFailure says whether a property of the model, decided in the run by a
// violation or a witness, fails to evaluate on a reachable state: the failure
// that a search that skips decided properties never meets.
func skippedFailure(t *testing.T, m *ir.Model, r *Result) bool {
	t.Helper()
	g, err := BuildGraph(context.Background(), m, Options{Budget: Budget{MaxStates: 60000}})
	if err != nil {
		t.Fatal(err)
	}
	if !g.Stats.Complete {
		return false
	}
	for _, o := range r.Outcomes {
		if (o.Property.Kind != ir.KindInvariant && o.Property.Kind != ir.KindReach) || o.Status == Verified && o.Property.Kind == ir.KindInvariant {
			continue
		}
		if o.Status == Violated && o.Property.Kind == ir.KindReach {
			continue // decided by elimination: every state was evaluated
		}
		expr, err := g.Layout.Compile(o.Property.Expr, -1)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < g.Len(); i++ {
			if _, err := g.Eval(expr, i); err != nil {
				return true
			}
		}
	}
	return false
}

// hasExtraProperties: addErringProperties gave the model properties (x0, x1, ...).
func hasExtraProperties(m *ir.Model) bool {
	for _, p := range m.Properties {
		if strings.HasPrefix(p.ID, "x") {
			return true
		}
	}
	return false
}

func hasRunEdge(m *ir.Model) bool {
	for _, p := range m.Processes {
		for _, e := range p.Edges {
			if e.Run != nil {
				return true
			}
		}
	}
	return false
}

// The report of a parallel run is the same for every worker count and for a
// repeated run (A11), on random models with random sizes of segment and inline
// threshold, budget-truncated runs included.
func TestParallelRandomModelsAreDeterministicAcrossWorkers(t *testing.T) {
	models, first := parModelCount(2000)
	for seed := first; seed < first+int64(models); seed++ {
		rnd := rand.New(rand.NewSource(seed))
		m, watch := randomParModel(rnd)
		bud := Budget{MaxStates: 20000, MaxDepth: 5000}
		if rnd.Intn(3) == 0 {
			bud.MaxStates = 1 + rnd.Intn(400)
		}
		base := Options{Sweep: rnd.Intn(2) == 0, Budget: bud, Watch: watch}
		if hasAtomic(m) {
			// an atomic loop is bounded by the parallel search only (see checkParallelCore)
		}
		var want string
		for i, w := range []int{1, 2, 3, 8, 16, 8} {
			o := base
			o.Workers = w
			o.par = &parKnobs{segment: 1 + rnd.Intn(300), inline: []int{-1, 0, 1 << 30}[rnd.Intn(3)]}
			got := resultDigest(runOpts(t, m, o))
			if i == 0 {
				want = got
			} else if got != want {
				t.Fatalf("seed %d, %d workers, knobs %+v:\n--- one worker\n%s--- this\n%s", seed, w, *o.par, want, got)
			}
		}
	}
}
