package explore

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"modelcheck/frontend/promela"
	"modelcheck/ir"
	"modelcheck/ltl"
)

// An oracle for the cycle search that shares nothing with the copies
// construction of cycle.go: it builds the explicit graph of the product (the
// system, and the never claim when there is one) and decides the question by
// strongly connected components. A run that is weakly fair and accepting
// exists iff some reachable SCC that has a cycle (a state with a self-loop,
// or more than one state) contains an accepting state and, for every process
// p, either a step of p or a state in which p has no move of its own — then
// a run that walks the whole SCC forever moves every process that is ever
// enabled, and every weakly fair run lives in one such SCC. The stutter
// extension (a state without any move repeats, the claim moving alone) is
// part of the graph, except for the non-progress product, whose blocked
// states have no successor (pan -l).
//
// The models are random, small and made of the pieces the weak-fairness search
// handles: guards on variables and on program counters, bound and plain
// assignments, buffered and rendezvous channels, `provided` clauses, accept
// and progress labels, never claims over the variables, bare `timeout` guards
// (extendWFCase: a pending timeout is a move of its own in a timeout state, so
// the oracle sees what the definition says and the engine's `blocked` must
// agree), atomic edges (the oracle collapses an atomic sequence into one step
// of the product, as the engine does not store its intermediate states; a
// sequence that does not end within 200 steps ends the row), `else` edges in
// the never claim (some into an `end` location), and the never claim at any
// position among the processes of the IR.
//
// Why a second implementation and not only pan: pan -f has gaps of its own
// (a process kept from moving by `provided` is not counted as blocked, and a
// stuttering run is fair only while a count is open), so a disagreement with
// pan on such a model says nothing about the engine; the oracle follows the
// definition and has none of them. See steps/fix-weakfairness-confirmation.md.

type wfCase struct {
	m     *ir.Model
	claim int    // index of the never claim in m.Processes, -1 if none
	mode  string // "acc", "prog" or "claim"
}

func wfCond(rnd *rand.Rand) *ir.Expr {
	g := []string{"x", "y"}[rnd.Intn(2)]
	op := []string{"eq", "eq", "ne"}[rnd.Intn(3)]
	return ir.Binary(op, ir.Ref(g), ir.Const(int64(rnd.Intn(3))))
}

func randomWFCase(rnd *rand.Rand, mode string) wfCase {
	m := &ir.Model{Schema: ir.Schema, Name: "wf",
		Globals: []ir.Var{byteVar("x"), byteVar("y")}}
	chanKind := rnd.Intn(8) // 0: rendezvous, 1 and 2: buffered, else none
	if chanKind <= 2 {
		capacity := 1
		if chanKind == 0 {
			capacity = 0
		}
		m.Channels = []ir.Channel{{Name: "ch", Capacity: capacity, Fields: []ir.Type{ir.Byte}}}
	}
	np := 1 + rnd.Intn(3)
	nlocs := make([]int, np)
	for p := range nlocs {
		nlocs[p] = 2 + rnd.Intn(3)
	}
	for p := 0; p < np; p++ {
		pr := ir.Process{Name: fmt.Sprintf("P%d", p), Locations: make([]ir.Location, nlocs[p])}
		for l := range pr.Locations {
			switch {
			case mode == "acc" && rnd.Intn(10) < 3:
				pr.Locations[l].Labels = []ir.Label{ir.Accept}
			case mode == "prog" && rnd.Intn(10) < 3:
				pr.Locations[l].Labels = []ir.Label{ir.Progress}
			}
		}
		if rnd.Intn(12) == 0 {
			pr.Provided = wfCond(rnd)
		}
		for e, ne := 0, 2+rnd.Intn(4); e < ne; e++ {
			ed := ir.Edge{From: rnd.Intn(nlocs[p]), To: rnd.Intn(nlocs[p])}
			switch k := rnd.Intn(20); {
			case k < 8: // always enabled
			case k < 14:
				ed.Guard = wfCond(rnd)
			case k < 17 && np > 1:
				j := (p + 1 + rnd.Intn(np-1)) % np
				ed.Guard = ir.Binary("eq", ir.PC(j), ir.Const(int64(rnd.Intn(nlocs[j]))))
			default:
				ed.Guard = ir.And(wfCond(rnd), wfCond(rnd))
			}
			g := []string{"x", "y"}[rnd.Intn(2)]
			switch k := rnd.Intn(12); {
			case k < 4:
				ed.Effect = []ir.Assign{{Var: g, Value: ir.Const(int64(rnd.Intn(3)))}}
			case k < 7:
				ed.Effect = []ir.Assign{{Var: g, Value: ir.Binary("mod", ir.Binary("add", ir.Ref(g), ir.Const(1)), ir.Const(3))}}
			case k < 9 && len(m.Channels) > 0:
				ed.Send = &ir.ChanOp{Chan: "ch", Args: []*ir.Expr{ir.Const(int64(rnd.Intn(3)))}}
			case k < 11 && len(m.Channels) > 0:
				ed.Recv = &ir.RecvOp{Chan: "ch", Args: []ir.RecvArg{{Var: g}}}
			}
			pr.Edges = append(pr.Edges, ed)
		}
		m.Processes = append(m.Processes, pr)
	}
	c := wfCase{m: m, claim: -1, mode: mode}
	id, kind := "acc", ir.KindLTL
	switch mode {
	case "prog":
		id, kind = "progress", ir.KindProgress
	case "claim":
		nl := 2 + rnd.Intn(2)
		cl := ir.Process{Name: "never", Claim: true, Locations: make([]ir.Location, nl)}
		for l := range cl.Locations {
			if rnd.Intn(10) < 4 || l == nl-1 && rnd.Intn(2) == 0 {
				cl.Locations[l].Labels = []ir.Label{ir.Accept}
			}
		}
		for l := 0; l < nl; l++ {
			for e, ne := 0, 1+rnd.Intn(3); e < ne; e++ {
				ed := ir.Edge{From: l, To: rnd.Intn(nl)}
				switch k := rnd.Intn(10); {
				case k < 2:
				case k < 7:
					ed.Guard = wfCond(rnd)
				default:
					ed.Guard = ir.And(wfCond(rnd), wfCond(rnd))
				}
				cl.Edges = append(cl.Edges, ed)
			}
		}
		c.claim = len(m.Processes)
		m.Processes = append(m.Processes, cl)
		id = "never"
	}
	m.Properties = []ir.Property{{ID: id, Kind: kind}}
	return c
}

// remapPC returns a copy of e with the process index of every `pc` read
// passed through f.
func remapPC(e *ir.Expr, f func(int) int) *ir.Expr {
	if e == nil {
		return nil
	}
	c := *e
	if c.Op == "pc" {
		c.Value = int64(f(int(c.Value)))
	}
	if len(e.Args) > 0 {
		c.Args = make([]*ir.Expr, len(e.Args))
		for i, a := range e.Args {
			c.Args[i] = remapPC(a, f)
		}
	}
	return &c
}

// extendWFCase adds what the first generator left out: bare `timeout` guards,
// atomic edges and the never claim anywhere among the processes. It draws from
// its own random source, so the models of a seed keep the shape they had
// before, and reports whether the model has atomic edges (a loop of them never
// ends, so the engine gets a depth budget and a row it cannot decide is
// skipped).
func extendWFCase(c *wfCase, rx *rand.Rand) (atomic bool) {
	m := c.m
	if rx.Intn(3) == 0 {
		for p := range m.Processes {
			if p == c.claim {
				continue
			}
			for e := range m.Processes[p].Edges {
				if rx.Intn(4) != 0 {
					continue
				}
				ed := &m.Processes[p].Edges[e]
				if rx.Intn(5) == 0 {
					ed.Guard = ir.And(ir.Timeout(), wfCond(rx))
				} else {
					ed.Guard = ir.Timeout()
				}
			}
		}
	}
	if rx.Intn(4) == 0 {
		atomic = true
		for p := range m.Processes {
			if p == c.claim {
				continue
			}
			for e := range m.Processes[p].Edges {
				if rx.Intn(3) == 0 {
					m.Processes[p].Edges[e].Atomic = true
				}
			}
		}
	}
	if c.claim >= 0 && rx.Intn(3) == 0 {
		// `else` edges in the claim (enabled only when no other edge of the
		// location is), some of them into a location with the `end` label (the
		// claim runs off its closing brace: a violation on a finite prefix)
		cl := &m.Processes[c.claim]
		endLoc := len(cl.Locations)
		cl.Locations = append(cl.Locations, ir.Location{Name: "-end-", Labels: []ir.Label{ir.End}})
		for l := 0; l < endLoc; l++ {
			if rx.Intn(3) == 0 {
				to := endLoc
				if rx.Intn(2) == 0 {
					to = rx.Intn(endLoc)
				}
				cl.Edges = append(cl.Edges, ir.Edge{From: l, To: to, Else: true, Text: "else"})
			}
		}
	}
	if c.claim >= 0 && rx.Intn(2) == 0 {
		// move the claim from the end to a random position and renumber the
		// `pc` reads of the guards
		to := rx.Intn(len(m.Processes))
		old := c.claim
		newIdx := func(i int) int {
			switch {
			case i == old:
				return to
			case old > to && i >= to && i < old:
				return i + 1
			case old < to && i > old && i <= to:
				return i - 1
			}
			return i
		}
		moved := make([]ir.Process, len(m.Processes))
		for i := range m.Processes {
			moved[newIdx(i)] = m.Processes[i]
		}
		for p := range moved {
			for e := range moved[p].Edges {
				moved[p].Edges[e].Guard = remapPC(moved[p].Edges[e].Guard, newIdx)
			}
			moved[p].Provided = remapPC(moved[p].Provided, newIdx)
		}
		m.Processes = moved
		c.claim = to
	}
	return atomic
}

type wfEdge struct {
	to int
	p1 []int // processes that move: initiators and rendezvous partners
}

type wfGraph struct {
	states  [][]byte
	succ    [][]wfEdge
	enabled [][]bool // per state: the process has a move of its own
	// endHit: a reachable state has an enabled claim edge into an end
	// location, which is a violation on a finite prefix, whatever fairness says.
	endHit bool
}

// wfChain follows the atomic sequence that a move starts: the successors of s
// by this move are the states in which the sequence ends (the exclusive holder
// is blocked or gone), because only those are states of the product. The
// movers are the processes that moved on the way. d_step is inside Apply.
func wfChain(st *Stepper, s []byte, mv Move, movers []int, depth int, emit func(to []byte, movers []int)) error {
	if depth > 200 {
		return fmt.Errorf("atomic sequence longer than 200 steps")
	}
	n, _, err := st.Apply(s, mv)
	if err != nil {
		return err
	}
	ms := append(append([]int(nil), movers...), mv.Edge.Proc)
	if mv.Partner != nil {
		ms = append(ms, mv.Partner.Proc)
	}
	inter, err := st.s.intermediate(n)
	if err != nil {
		return err
	}
	if !inter {
		emit(n, ms)
		return nil
	}
	moves, err := st.Enabled(n)
	if err != nil {
		return err
	}
	for _, m2 := range moves {
		if err := wfChain(st, n, m2, ms, depth+1, emit); err != nil {
			return err
		}
	}
	return nil
}

func wfHasLabel(m *ir.Model, p, loc int, lb ir.Label) bool {
	for _, x := range m.Processes[p].Locations[loc].Labels {
		if x == lb {
			return true
		}
	}
	return false
}

// buildWFGraph builds the explicit product graph. limit caps its size.
func buildWFGraph(c wfCase, limit int) (*wfGraph, *Stepper, error) {
	st, err := NewStepper(c.m)
	if err != nil {
		return nil, nil, err
	}
	l := st.Layout()
	nproc := len(c.m.Processes)
	g := &wfGraph{}
	index := map[string]int{}
	add := func(s []byte) (int, bool) {
		if i, ok := index[string(s)]; ok {
			return i, false
		}
		i := len(g.states)
		index[string(s)] = i
		g.states = append(g.states, s)
		g.succ = append(g.succ, nil)
		g.enabled = append(g.enabled, nil)
		return i, true
	}
	add(st.Initial())
	stutter := c.mode != "prog"
	for i := 0; i < len(g.states); i++ {
		if len(g.states) > limit {
			return nil, nil, fmt.Errorf("graph larger than %d states", limit)
		}
		s := g.states[i]
		moves, err := st.Enabled(s)
		if err != nil {
			return nil, nil, err
		}
		en := make([]bool, nproc)
		for _, mv := range moves {
			en[mv.Edge.Proc] = true
		}
		g.enabled[i] = en
		// The claim moves first, reading the state; without a claim there is
		// one choice that changes nothing.
		type claimChoice struct{ to int }
		var choices []claimChoice
		if c.claim < 0 {
			choices = []claimChoice{{-1}}
		} else {
			cp := &st.s.c.procs[c.claim]
			var elses []int
			for _, ei := range cp.out[l.ReadPC(s, c.claim)] {
				if cp.edge[ei].e.Else {
					elses = append(elses, cp.edge[ei].e.To)
					continue
				}
				ok, err := cp.edge[ei].guard.Truth(s)
				if err != nil {
					return nil, nil, err
				}
				if ok {
					choices = append(choices, claimChoice{cp.edge[ei].e.To})
				}
			}
			if len(choices) == 0 {
				for _, to := range elses {
					choices = append(choices, claimChoice{to})
				}
			}
			kept := choices[:0]
			for _, ch := range choices {
				if wfHasLabel(c.m, c.claim, ch.to, ir.End) {
					g.endHit = true
					continue
				}
				kept = append(kept, ch)
			}
			choices = kept
		}
		for _, ch := range choices {
			setClaim := func(n []byte) []byte {
				if ch.to >= 0 {
					l.WritePC(n, c.claim, ch.to)
				}
				return n
			}
			if len(moves) == 0 {
				if stutter {
					j, _ := add(setClaim(append([]byte(nil), s...)))
					g.succ[i] = append(g.succ[i], wfEdge{to: j})
				}
				continue
			}
			for _, mv := range moves {
				err := wfChain(st, s, mv, nil, 0, func(to []byte, movers []int) {
					j, _ := add(setClaim(to))
					g.succ[i] = append(g.succ[i], wfEdge{to: j, p1: movers})
				})
				if err != nil {
					return nil, nil, err
				}
			}
		}
	}
	return g, st, nil
}

// oracleCycle reports whether the product has an accepting (acc, claim) or
// non-progress (prog) cycle, weakly fair when fair is set.
func oracleCycle(c wfCase, fair bool, limit int) (bool, error) {
	g, st, err := buildWFGraph(c, limit)
	if err != nil {
		return false, err
	}
	if g.endHit {
		return true, nil
	}
	l := st.Layout()
	n := len(g.states)
	nonClaim := []int{}
	for p := range c.m.Processes {
		if p != c.claim {
			nonClaim = append(nonClaim, p)
		}
	}
	hasLabel := func(p, loc int, lb ir.Label) bool {
		for _, x := range c.m.Processes[p].Locations[loc].Labels {
			if x == lb {
				return true
			}
		}
		return false
	}
	allowed := make([]bool, n)
	accepting := make([]bool, n)
	for i, s := range g.states {
		allowed[i] = true
		switch c.mode {
		case "prog":
			for _, p := range nonClaim {
				if hasLabel(p, l.ReadPC(s, p), ir.Progress) {
					allowed[i] = false
				}
			}
			accepting[i] = true
		case "claim":
			accepting[i] = hasLabel(c.claim, l.ReadPC(s, c.claim), ir.Accept)
		default:
			for _, p := range nonClaim {
				if hasLabel(p, l.ReadPC(s, p), ir.Accept) {
					accepting[i] = true
				}
			}
		}
	}
	// Tarjan, iterative, over the allowed subgraph.
	idx := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	comp := make([]int, n)
	for i := range idx {
		idx[i], comp[i] = -1, -1
	}
	var stack []int
	counter, ncomp := 0, 0
	type fr struct{ v, e int }
	for root := 0; root < n; root++ {
		if !allowed[root] || idx[root] >= 0 {
			continue
		}
		work := []fr{{root, 0}}
		idx[root], low[root] = counter, counter
		counter++
		stack = append(stack, root)
		onStack[root] = true
		for len(work) > 0 {
			top := &work[len(work)-1]
			v := top.v
			if top.e < len(g.succ[v]) {
				w := g.succ[v][top.e].to
				top.e++
				if !allowed[w] {
					continue
				}
				if idx[w] < 0 {
					idx[w], low[w] = counter, counter
					counter++
					stack = append(stack, w)
					onStack[w] = true
					work = append(work, fr{w, 0})
				} else if onStack[w] && idx[w] < low[v] {
					low[v] = idx[w]
				}
				continue
			}
			if low[v] == idx[v] {
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					comp[w] = ncomp
					if w == v {
						break
					}
				}
				ncomp++
			}
			work = work[:len(work)-1]
			if len(work) > 0 {
				u := work[len(work)-1].v
				if low[v] < low[u] {
					low[u] = low[v]
				}
			}
		}
	}
	// Per component: has a cycle, has an accepting state, which processes are
	// covered by a step of theirs or by a state where they have no move.
	type info struct {
		cyc, acc bool
		covered  map[int]bool
	}
	comps := make([]info, ncomp)
	for i := range comps {
		comps[i].covered = map[int]bool{}
	}
	for v := 0; v < n; v++ {
		if !allowed[v] {
			continue
		}
		ci := &comps[comp[v]]
		if accepting[v] {
			ci.acc = true
		}
		for _, p := range nonClaim {
			if !g.enabled[v][p] {
				ci.covered[p] = true
			}
		}
		for _, e := range g.succ[v] {
			if !allowed[e.to] || comp[e.to] != comp[v] {
				continue
			}
			ci.cyc = true
			for _, p := range e.p1 {
				ci.covered[p] = true
			}
		}
	}
	for _, ci := range comps {
		if !ci.cyc || !ci.acc {
			continue
		}
		ok := true
		if fair {
			for _, p := range nonClaim {
				if !ci.covered[p] {
					ok = false
				}
			}
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// TestWeakFairnessMatchesTheSCCOracle: on random models the engine's verdict
// under none and weak fairness is the oracle's. Both directions matter, and
// both are checked: a violation the oracle does not know is a wrong
// `violated` (the defect behind this file: the copies alone closed a cycle);
// a violation the engine misses is a wrong `verified`.
func TestWeakFairnessMatchesTheSCCOracle(t *testing.T) {
	models := 3000
	if testing.Short() {
		models = 600
	}
	if v, err := strconv.Atoi(os.Getenv("MCD_WF_MODELS")); err == nil && v > 0 {
		models = v // a deeper one-off run: MCD_WF_MODELS=30000 go test ./explore -run TestWeakFairnessMatches
	}
	first := int64(1)
	if v, err := strconv.ParseInt(os.Getenv("MCD_WF_SEED"), 10, 64); err == nil && v > 0 {
		first = v // MCD_WF_SEED=500000 moves the run to models nobody has checked yet
	}
	modes := []string{"acc", "prog", "claim"}
	clockStops := 0 // rows stopped by the deadline above
	type tally struct{ rows, violated, fairMatters, skipped int }
	// how many compared rows exercise what extendWFCase adds
	var withTimeout, withAtomic, claimMoved, timeoutFairMatters int
	counts := map[string]*tally{}
	for _, md := range modes {
		counts[md] = &tally{}
	}
	for i := 0; i < models; i++ {
		seed := first + int64(i)
		md := modes[seed%int64(len(modes))]
		c := randomWFCase(rand.New(rand.NewSource(seed)), md)
		atomic := extendWFCase(&c, rand.New(rand.NewSource(seed*7919+13)))
		budget := Budget{}
		if atomic {
			budget = Budget{MaxDepth: 300, MaxStates: 200000}
		}
		tl := counts[md]
		var verdict [2]bool
		skip := false
		for fi, fairness := range []string{"none", "weak"} {
			want, err := oracleCycle(c, fairness == "weak", 4000)
			if err != nil {
				skip = true
				break
			}
			// No recover: a panic of the engine (the lasso defect of the cycle
			// search was one, fixed on fix/cycle-lasso-panic and merged) is a
			// failure of this test, never a skipped row.
			// A model with an atomic edge that loops for ever (an atomic self-loop
			// with no guard) keeps the process in control and the search busy with
			// no state to store: only a clock stops it, on 0.2.0 as well. The
			// runs of the wide campaigns met it about once in 100 000 models.
			// The deadline turns it into a counted skip; the default 3 000
			// models never come near it.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			res, err := Run(ctx, c.m, Options{Sweep: true, Fairness: fairness, Budget: budget})
			timedOut := ctx.Err() != nil
			cancel()
			if timedOut {
				clockStops++
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
			got := o.Status == Violated
			verdict[fi] = want
			if got != want {
				t.Errorf("seed %d (%s, fairness %s): the engine says %s, the SCC oracle says a cycle exists: %v (MCD_WF_SEED=%d MCD_WF_MODELS=1 reproduces it)",
					seed, md, fairness, o.Status, want, seed)
			}
		}
		if skip {
			tl.skipped++
			continue
		}
		tl.rows++
		if verdict[0] {
			tl.violated++
		}
		if verdict[0] != verdict[1] {
			tl.fairMatters++
		}
		hasTimeout, hasAtomic := false, false
		for p := range c.m.Processes {
			for _, e := range c.m.Processes[p].Edges {
				hasTimeout = hasTimeout || e.Guard.Uses("timeout")
				hasAtomic = hasAtomic || e.Atomic
			}
		}
		if hasTimeout {
			withTimeout++
			if verdict[0] != verdict[1] {
				timeoutFairMatters++
			}
		}
		if hasAtomic {
			withAtomic++
		}
		if c.claim >= 0 && c.claim != len(c.m.Processes)-1 {
			claimMoved++
		}
	}
	t.Logf("rows stopped by the clock (an atomic loop that never gives up the control): %d", clockStops)
	t.Logf("compared rows with a timeout guard %d (fairness changes the verdict in %d), with atomic edges %d, with the claim not last %d", withTimeout, timeoutFairMatters, withAtomic, claimMoved)
	if models >= 600 && (withTimeout < models/10 || withAtomic < models/20 || claimMoved < models/20 || timeoutFairMatters < models/1000) {
		t.Errorf("the extended generator is too tame: %d timeout rows (%d where fairness matters), %d atomic rows, %d rows with the claim not last, of %d models", withTimeout, timeoutFairMatters, withAtomic, claimMoved, models)
	}
	fairTotal := 0
	for _, md := range modes {
		tl := counts[md]
		fairTotal += tl.fairMatters
		t.Logf("%-5s rows %4d  violated without fairness %4d  fairness changes the verdict %4d  skipped %4d", md, tl.rows, tl.violated, tl.fairMatters, tl.skipped)
		// A test that never sees a violation, or never sees fairness matter,
		// proves nothing about the construction.
		if models >= 600 && tl.violated < tl.rows/20 {
			t.Errorf("%s: the generator is too tame (%d violated of %d rows)", md, tl.violated, tl.rows)
		}
	}
	// Fairness changes the verdict in about 1% of the models (3 000 models:
	// 9, 21 and 3 in the three modes), so it is counted over all the modes and
	// at the size of the run: a floor of 3 per mode failed `go test -short` (600
	// models: 1, 2 and 0) and sat exactly on the count of the claim mode at the
	// default size.
	if models >= 600 && fairTotal < models/400 {
		t.Errorf("the generator is too tame: fairness changes the verdict in %d of %d models (at least %d wanted)", fairTotal, models, models/400)
	}
}

// wfPromelaCase loads a Promela model for the comparison: mode "claim" is the
// model's own never claim, "acc" its accept labels, "prog" its progress
// labels, "ltl" the negated formula's automaton. It returns the model the
// engine runs (with the property), the oracle's case, the property id and the
// defines; ok is false for a case the oracle does not cover (a claim with an
// assert or an atomic continuation, as `spin -f` writes them).
func wfPromelaCase(path, mode, formula string) (em *ir.Model, c wfCase, id string, defs map[string]string, ok bool, err error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, c, "", nil, false, err
	}
	res, perr := promela.Parse(src, path, nil)
	if perr != nil {
		return nil, c, "", nil, false, perr
	}
	base, defs := res.Model, res.Defines
	em = base
	claim := -1
	for p := range base.Processes {
		if base.Processes[p].Claim {
			claim = p
		}
	}
	has := func(id string) bool {
		for _, pr := range base.Properties {
			if pr.ID == id {
				return true
			}
		}
		return false
	}
	switch mode {
	case "claim":
		if claim < 0 || !has("never") {
			return nil, c, "", nil, false, nil
		}
		id, c = "never", wfCase{m: base, claim: claim, mode: "claim"}
	case "acc":
		if claim >= 0 || !has("accept") {
			return nil, c, "", nil, false, nil
		}
		id, c = "accept", wfCase{m: base, claim: -1, mode: "acc"}
	case "prog":
		if !has("progress") {
			mm := *base
			mm.Properties = append(append([]ir.Property(nil), base.Properties...), ir.Property{ID: "progress", Kind: ir.KindProgress})
			em = &mm
		}
		id, c = "progress", wfCase{m: base, claim: -1, mode: "prog"}
	case "ltl":
		l0, err := ir.NewLayout(base)
		if err != nil {
			return nil, c, "", nil, false, err
		}
		cl, err := ltl.ForProperty("never:ltl1", formula, ltl.Options{Defines: defs, Scope: l0.Scope(-1)})
		if err != nil {
			return nil, c, "", nil, false, err
		}
		with := *base
		with.Processes = append(append([]ir.Process(nil), base.Processes...), cl.Process)
		mm := *base
		mm.Properties = append(append([]ir.Property(nil), base.Properties...), ir.Property{ID: "ltl1", Kind: ir.KindLTL, Formula: formula})
		em, id, c = &mm, "ltl1", wfCase{m: &with, claim: len(base.Processes), mode: "claim"}
	default:
		return nil, c, "", nil, false, fmt.Errorf("unknown mode %q", mode)
	}
	if c.claim >= 0 {
		for _, e := range c.m.Processes[c.claim].Edges {
			if e.Assert != nil || e.Atomic {
				return nil, c, "", nil, false, nil
			}
		}
	}
	return em, c, id, defs, true, nil
}

// wfCompare runs the engine and the oracle on one Promela model under none and
// weak fairness; it returns false when the case is not comparable.
func wfCompare(t *testing.T, path, mode, formula string) (compared bool, why string) {
	t.Helper()
	em, c, id, defs, ok, err := wfPromelaCase(path, mode, formula)
	if err != nil {
		return false, err.Error()
	}
	if !ok {
		return false, "outside what the oracle covers (no such property, or a claim with an assert or an atomic edge)"
	}
	for _, fairness := range []string{"none", "weak"} {
		want, err := oracleCycle(c, fairness == "weak", 20000)
		if err != nil {
			return false, "oracle: " + err.Error()
		}
		res, err := Run(context.Background(), em, Options{Defines: defs, Fairness: fairness, Budget: Budget{MaxStates: 200000}})
		if err != nil {
			return false, "engine: " + err.Error()
		}
		o := outcomeOf(t, res, id)
		if o.Status != Violated && o.Status != Verified {
			return false, "engine: " + string(o.Status)
		}
		if got := o.Status == Violated; got != want {
			t.Errorf("%s (%s %s, fairness %s): the engine says %s, the SCC oracle says a cycle exists: %v", filepath.Base(path), mode, formula, fairness, o.Status, want)
		}
	}
	return true, ""
}

// TestWeakFairnessOracleOnPromela: the same decision procedure on Promela
// models, which bring atomic and d_step sequences, `provided` clauses,
// else branches, and never claims and LTL formulas as they are written. The
// models of testdata/weakfair are generated ones, each with its mode in the
// first line (`/* wf mode=claim */`, `/* wf mode=ltl formula=<>[]p */`); they
// include the models on which `pan -f` and the engine give different answers
// (see steps/fix-weakfairness-confirmation.md), where this test decides who is
// right. MCD_WF_PROMELA_DIR=<dir with name.pml and name.meta files, as written
// by the generator of the differential run> checks a larger set.
func TestWeakFairnessOracleOnPromela(t *testing.T) {
	files, _ := filepath.Glob("../testdata/weakfair/*.pml")
	if len(files) == 0 {
		// without them the 50-model regression would silently not run
		t.Fatal("no model found under testdata/weakfair: the regression set of the weak-fairness fix is missing")
	}
	compared := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		first := strings.SplitN(string(src), "\n", 2)[0]
		if !strings.HasPrefix(first, "/* wf ") {
			t.Fatalf("%s: the first line must be /* wf mode=... */", f)
		}
		mode, formula := "", ""
		for _, kv := range strings.Fields(strings.TrimSuffix(strings.TrimPrefix(first, "/* wf "), "*/")) {
			switch {
			case strings.HasPrefix(kv, "mode="):
				mode = strings.TrimPrefix(kv, "mode=")
			case strings.HasPrefix(kv, "formula="):
				formula = strings.TrimPrefix(kv, "formula=")
			}
		}
		if mode == "ltl" {
			// the formula may contain spaces: everything after "formula="
			formula = strings.TrimSpace(strings.TrimSuffix(first[strings.Index(first, "formula=")+len("formula="):], "*/"))
		}
		if ok, why := wfCompare(t, f, mode, formula); ok {
			compared++
		} else {
			t.Logf("%s not compared: %s", filepath.Base(f), why)
		}
	}
	if compared < len(files)*9/10 {
		t.Errorf("only %d of %d models of testdata/weakfair could be compared", compared, len(files))
	}
	if dir := os.Getenv("MCD_WF_PROMELA_DIR"); dir != "" {
		gen, _ := filepath.Glob(filepath.Join(dir, "*.pml"))
		n := 0
		for _, f := range gen {
			raw, err := os.ReadFile(strings.TrimSuffix(f, ".pml") + ".meta")
			if err != nil {
				continue
			}
			var meta struct{ Mode, Formula string }
			if json.Unmarshal(raw, &meta) != nil {
				continue
			}
			if ok, _ := wfCompare(t, f, meta.Mode, meta.Formula); ok {
				n++
			}
		}
		t.Logf("%s: %d of %d models compared", dir, n, len(gen))
		if n == 0 {
			t.Errorf("MCD_WF_PROMELA_DIR=%s: no model could be compared (name.pml and name.meta files expected)", dir)
		}
	}
}
