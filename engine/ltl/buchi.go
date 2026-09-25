package ltl

import (
	"sort"
	"strconv"
	"strings"
)

// Lit is a literal of a transition label: atom index, possibly negated.
type Lit struct {
	Atom int
	Neg  bool
}

// Trans is a transition. Label is a conjunction of literals (empty = true)
// that must hold in the state the automaton observes when it takes the
// transition — in never-claim terms, the guard of the claim edge into
// Target.
type Trans struct {
	Target int
	Label  []Lit
}

// State of a Büchi automaton.
type State struct {
	Accepting bool
	Trans     []Trans
	// Name is a SPIN-like label for the never claim: T0_init, T<i>,
	// accept_S<i>.
	Name string
}

// Automaton is a (non-generalised) Büchi automaton over the atoms. State 0
// is the initial pseudo-state: it consumes no input on its own; its
// transitions read the first state of the run, exactly like SPIN's T0_init.
type Automaton struct {
	Atoms  []*Atom
	States []State
}

// Info summarises a translation for the report.
type Info struct {
	Formula          string   `json:"formula"`
	Negated          string   `json:"negated"`
	Atoms            []string `json:"atoms"`
	StutterInvariant bool     `json:"stutter_invariant"`
	States           int      `json:"automaton_states"`
	Transitions      int      `json:"automaton_transitions"`
	Accepting        int      `json:"automaton_accepting"`
	// Antecedents are the left-hand sides of the implications of the
	// formula, as written; they are the vacuity candidates of FR-011 (an
	// antecedent that is never true makes the implication hold for a reason
	// unrelated to its consequent).
	Antecedents []string `json:"antecedents,omitempty"`
}

// Translate builds the Büchi automaton of f (Gerth–Peled–Vardi–Wolper
// tableau, then degeneralisation with one counter, then simplification).
// The language of the result is exactly the set of infinite words over
// the atoms that satisfy f; TestLanguage checks this on bounded lassos
// against the brute-force semantics.
func Translate(f *Formula) *Automaton {
	g := NNF(f)
	atoms := g.Atoms()
	index := map[string]int{}
	for i, a := range atoms {
		index[a.Text] = i
	}
	nodes := tableau(g)
	gba := buildGBA(g, nodes, index)
	a := degeneralize(gba)
	a.Atoms = atoms
	a = simplify(a)
	nameStates(a)
	return a
}

// ---- tableau ----------------------------------------------------------------

type fset map[string]*Formula

func (s fset) clone() fset {
	c := make(fset, len(s))
	for k, v := range s {
		c[k] = v
	}
	return c
}

func (s fset) add(f *Formula) { s[f.String()] = f }
func (s fset) has(f *Formula) bool {
	_, ok := s[f.String()]
	return ok
}

func (s fset) key() string { return strings.Join(sortedKeys(s), "\x00") }

type node struct {
	id       int
	incoming map[int]bool
	new, old fset
	next     fset
}

type tableauState struct {
	nodes  []*node
	nextID int
}

// tableau runs the GPVW expansion from the formula g (in NNF).
func tableau(g *Formula) []*node {
	ts := &tableauState{nextID: 1} // id 0 is the initial pseudo-node
	start := &node{id: ts.nextID, incoming: map[int]bool{0: true}, new: fset{}, old: fset{}, next: fset{}}
	ts.nextID++
	start.new.add(g)
	ts.expand(start)
	return ts.nodes
}

func (ts *tableauState) newNode(incoming map[int]bool, new, old, next fset) *node {
	n := &node{id: ts.nextID, incoming: incoming, new: new, old: old, next: next}
	ts.nextID++
	return n
}

func (ts *tableauState) expand(n *node) {
	if len(n.new) == 0 {
		oldKey, nextKey := n.old.key(), n.next.key()
		for _, r := range ts.nodes {
			if r.old.key() == oldKey && r.next.key() == nextKey {
				for k := range n.incoming {
					r.incoming[k] = true
				}
				return
			}
		}
		ts.nodes = append(ts.nodes, n)
		q := ts.newNode(map[int]bool{n.id: true}, n.next.clone(), fset{}, fset{})
		ts.expand(q)
		return
	}
	// Pick the smallest formula key: deterministic node numbering.
	key := sortedKeys(n.new)[0]
	eta := n.new[key]
	delete(n.new, key)
	switch eta.Op {
	case True:
		// Kept in Old (labels ignore it) so that an acceptance set for
		// <>true / (μ U true) sees its right-hand side satisfied.
		n.old.add(eta)
		ts.expand(n)
	case False:
		return // contradiction: the node is discarded
	case AtomOp, Not:
		if n.old.has(negLit(eta)) {
			return
		}
		n.old.add(eta)
		ts.expand(n)
	case And:
		addNew(n, eta.L)
		addNew(n, eta.R)
		n.old.add(eta)
		ts.expand(n)
	case Or:
		n1 := ts.newNode(copyIn(n.incoming), n.new.clone(), n.old.clone(), n.next.clone())
		addNew(n1, eta.L)
		n1.old.add(eta)
		n2 := ts.newNode(copyIn(n.incoming), n.new.clone(), n.old.clone(), n.next.clone())
		addNew(n2, eta.R)
		n2.old.add(eta)
		ts.expand(n1)
		ts.expand(n2)
	case Until, Eventually:
		// μ U ψ: (ψ) or (μ and X(μ U ψ)); <>ψ = true U ψ.
		n1 := ts.newNode(copyIn(n.incoming), n.new.clone(), n.old.clone(), n.next.clone())
		if eta.Op == Until {
			addNew(n1, eta.L)
		}
		n1.next.add(eta)
		n1.old.add(eta)
		n2 := ts.newNode(copyIn(n.incoming), n.new.clone(), n.old.clone(), n.next.clone())
		addNew(n2, untilRight(eta))
		n2.old.add(eta)
		ts.expand(n1)
		ts.expand(n2)
	case Release, Always:
		// μ V ψ: ψ and (μ or X(μ V ψ)); []ψ = false V ψ.
		n1 := ts.newNode(copyIn(n.incoming), n.new.clone(), n.old.clone(), n.next.clone())
		addNew(n1, releaseRight(eta))
		n1.next.add(eta)
		n1.old.add(eta)
		ts.expand(n1)
		if eta.Op == Release {
			n2 := ts.newNode(copyIn(n.incoming), n.new.clone(), n.old.clone(), n.next.clone())
			addNew(n2, eta.L)
			addNew(n2, eta.R)
			n2.old.add(eta)
			ts.expand(n2)
		}
	case Next:
		n.next.add(eta.L)
		n.old.add(eta)
		ts.expand(n)
	default:
		panic("ltl: tableau on a formula that is not in NNF: " + eta.String())
	}
}

func untilRight(f *Formula) *Formula {
	if f.Op == Eventually {
		return f.L
	}
	return f.R
}

func releaseRight(f *Formula) *Formula {
	if f.Op == Always {
		return f.L
	}
	return f.R
}

func addNew(n *node, f *Formula) {
	if !n.old.has(f) {
		n.new.add(f)
	}
}

func copyIn(m map[int]bool) map[int]bool {
	c := make(map[int]bool, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// negLit is the complementary literal of an atom or negated atom.
func negLit(f *Formula) *Formula {
	if f.Op == Not {
		return f.L
	}
	return NotF(f)
}

// ---- generalised Büchi automaton --------------------------------------------------

type gba struct {
	nodes  []*node
	byID   map[int]*node
	labels map[int][]Lit // per node: its literals
	// acc[i] is the i-th acceptance set: node ids that satisfy the i-th
	// until-subformula's obligation.
	acc []map[int]bool
}

func buildGBA(g *Formula, nodes []*node, index map[string]int) *gba {
	b := &gba{nodes: nodes, byID: map[int]*node{}, labels: map[int][]Lit{}}
	for _, n := range nodes {
		b.byID[n.id] = n
		var lits []Lit
		for _, k := range sortedKeys(n.old) {
			f := n.old[k]
			switch {
			case f.Op == AtomOp:
				lits = append(lits, Lit{Atom: index[f.Atom.Text]})
			case f.Op == Not && f.L.Op == AtomOp:
				lits = append(lits, Lit{Atom: index[f.L.Atom.Text], Neg: true})
			}
		}
		sort.Slice(lits, func(i, j int) bool {
			if lits[i].Atom != lits[j].Atom {
				return lits[i].Atom < lits[j].Atom
			}
			return !lits[i].Neg && lits[j].Neg
		})
		b.labels[n.id] = lits
	}
	// One acceptance set per until-type subformula (U and <>), in a
	// deterministic order.
	untils := map[string]*Formula{}
	var walk func(f *Formula)
	walk = func(f *Formula) {
		if f == nil {
			return
		}
		if f.Op == Until || f.Op == Eventually {
			untils[f.String()] = f
		}
		walk(f.L)
		walk(f.R)
	}
	walk(g)
	for _, k := range sortedKeys(untils) {
		u := untils[k]
		set := map[int]bool{}
		for _, n := range nodes {
			if !n.old.has(u) || n.old.has(untilRight(u)) {
				set[n.id] = true
			}
		}
		b.acc = append(b.acc, set)
	}
	return b
}

// degeneralize turns the GBA into a Büchi automaton with a source-based
// counter: state (q, j) moves to (q', j') where j' = (j+1) mod k if q is in
// acceptance set j, else j; accepting states are (q, k-1) with q in set
// k-1. A run is then accepting iff it visits every set infinitely often:
// each visit to an accepting state wraps the counter, and the counter can
// only come back to k-1 by passing through a state of each other set.
// With no acceptance set (no U / <> in the formula), every state is
// accepting (k = 1, set 0 = all nodes).
func degeneralize(b *gba) *Automaton {
	k := len(b.acc)
	acc := b.acc
	if k == 0 {
		k = 1
		all := map[int]bool{}
		for _, n := range b.nodes {
			all[n.id] = true
		}
		acc = []map[int]bool{all}
	}
	// state numbering: 0 = init; then (node position, layer) in node order.
	pos := map[int]int{}
	for i, n := range b.nodes {
		pos[n.id] = i
	}
	idOf := func(nodeID, layer int) int { return 1 + pos[nodeID]*k + layer }
	a := &Automaton{States: make([]State, 1+len(b.nodes)*k)}
	for _, n := range b.nodes {
		if n.incoming[0] {
			a.States[0].Trans = append(a.States[0].Trans, Trans{Target: idOf(n.id, 0), Label: b.labels[n.id]})
		}
	}
	for _, n := range b.nodes {
		for j := 0; j < k; j++ {
			st := &a.States[idOf(n.id, j)]
			st.Accepting = j == k-1 && acc[k-1][n.id]
			jn := j
			if acc[j][n.id] {
				jn = (j + 1) % k
			}
			for _, m := range b.nodes {
				if m.incoming[n.id] {
					st.Trans = append(st.Trans, Trans{Target: idOf(m.id, jn), Label: b.labels[m.id]})
				}
			}
		}
	}
	return a
}

// ---- simplification --------------------------------------------------------------

// simplify removes unreachable states, states from which no accepting
// cycle is reachable, duplicate and subsumed transitions, and merges
// states with equal signatures (a bisimulation quotient). None of this
// changes the language.
func simplify(a *Automaton) *Automaton {
	for {
		before := len(a.States)
		a = pruneUnreachable(a)
		a = pruneDead(a)
		a = mergeEquivalent(a)
		for i := range a.States {
			a.States[i].Trans = dedupTrans(a.States[i].Trans)
		}
		if len(a.States) == before {
			return a
		}
	}
}

func pruneUnreachable(a *Automaton) *Automaton {
	reach := make([]bool, len(a.States))
	stack := []int{0}
	reach[0] = true
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, t := range a.States[s].Trans {
			if !reach[t.Target] {
				reach[t.Target] = true
				stack = append(stack, t.Target)
			}
		}
	}
	return keep(a, reach)
}

// pruneDead keeps the states that can reach a non-trivial strongly
// connected component containing an accepting state (Tarjan), plus the
// initial state.
func pruneDead(a *Automaton) *Automaton {
	n := len(a.States)
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	comp := make([]int, n)
	for i := range index {
		index[i] = -1
		comp[i] = -1
	}
	var stack []int
	counter, ncomp := 0, 0
	var strong func(v int)
	strong = func(v int) {
		index[v], low[v] = counter, counter
		counter++
		stack = append(stack, v)
		onStack[v] = true
		for _, t := range a.States[v].Trans {
			w := t.Target
			if index[w] < 0 {
				strong(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && index[w] < low[v] {
				low[v] = index[w]
			}
		}
		if low[v] == index[v] {
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
	}
	for v := 0; v < n; v++ {
		if index[v] < 0 {
			strong(v)
		}
	}
	// A component is good if it has an accepting state and an internal edge.
	size := make([]int, ncomp)
	internal := make([]bool, ncomp)
	hasAcc := make([]bool, ncomp)
	for v := 0; v < n; v++ {
		size[comp[v]]++
		if a.States[v].Accepting {
			hasAcc[comp[v]] = true
		}
		for _, t := range a.States[v].Trans {
			if comp[t.Target] == comp[v] {
				internal[comp[v]] = true
			}
		}
	}
	good := make([]bool, n)
	for v := 0; v < n; v++ {
		good[v] = hasAcc[comp[v]] && internal[comp[v]]
	}
	// Backward closure: a state that can reach a good state is live.
	live := make([]bool, n)
	changed := true
	for changed {
		changed = false
		for v := 0; v < n; v++ {
			if live[v] {
				continue
			}
			if good[v] {
				live[v], changed = true, true
				continue
			}
			for _, t := range a.States[v].Trans {
				if live[t.Target] {
					live[v], changed = true, true
					break
				}
			}
		}
	}
	live[0] = true
	return keep(a, live)
}

// keep renumbers the automaton to the states with keep[i] (state 0 stays
// 0) and drops transitions into removed states.
func keep(a *Automaton, keepMask []bool) *Automaton {
	newID := make([]int, len(a.States))
	n := 0
	for i := range a.States {
		if keepMask[i] {
			newID[i] = n
			n++
		} else {
			newID[i] = -1
		}
	}
	out := &Automaton{Atoms: a.Atoms, States: make([]State, n)}
	for i, st := range a.States {
		if newID[i] < 0 {
			continue
		}
		ns := State{Accepting: st.Accepting, Name: st.Name}
		for _, t := range st.Trans {
			if newID[t.Target] >= 0 {
				ns.Trans = append(ns.Trans, Trans{Target: newID[t.Target], Label: t.Label})
			}
		}
		out.States[newID[i]] = ns
	}
	return out
}

func litsKey(l []Lit) string {
	var b strings.Builder
	for _, x := range l {
		if x.Neg {
			b.WriteByte('!')
		}
		b.WriteString(strconv.Itoa(x.Atom))
		b.WriteByte(',')
	}
	return b.String()
}

// mergeEquivalent computes the coarsest partition in which two states have
// the same acceptance and the same set of (label, target class) pairs, and
// merges each class into its lowest-numbered state.
func mergeEquivalent(a *Automaton) *Automaton {
	n := len(a.States)
	class := make([]int, n)
	for i := range class {
		if a.States[i].Accepting {
			class[i] = 1
		}
	}
	for {
		sig := make([]string, n)
		for i, st := range a.States {
			var parts []string
			for _, t := range st.Trans {
				parts = append(parts, litsKey(t.Label)+"->"+strconv.Itoa(class[t.Target]))
			}
			sort.Strings(parts)
			sig[i] = strconv.Itoa(class[i]) + "|" + strings.Join(parts, ";")
		}
		next := make([]int, n)
		seen := map[string]int{}
		rep := map[string]int{}
		for i := 0; i < n; i++ {
			c, ok := seen[sig[i]]
			if !ok {
				c = len(seen)
				seen[sig[i]] = c
				rep[sig[i]] = i
			}
			next[i] = c
		}
		stable := true
		for i := range next {
			if next[i] != class[i] {
				stable = false
			}
		}
		class = next
		if stable {
			// Representative of each class: its first state.
			repOf := make([]int, n)
			for i := 0; i < n; i++ {
				repOf[i] = rep[sig[i]]
			}
			mask := make([]bool, n)
			for i := 0; i < n; i++ {
				mask[i] = repOf[i] == i
			}
			mask[0] = true
			b := &Automaton{Atoms: a.Atoms, States: make([]State, n)}
			for i, st := range a.States {
				ns := State{Accepting: st.Accepting, Name: st.Name}
				for _, t := range st.Trans {
					ns.Trans = append(ns.Trans, Trans{Target: repOf[t.Target], Label: t.Label})
				}
				b.States[i] = ns
			}
			return keep(b, mask)
		}
	}
}

// dedupTrans drops duplicate transitions and transitions subsumed by a
// weaker one to the same target (label A ⊂ label B means B implies A, so B
// is redundant).
func dedupTrans(ts []Trans) []Trans {
	var out []Trans
	for i, t := range ts {
		redundant := false
		for j, u := range ts {
			if i == j || u.Target != t.Target {
				continue
			}
			if subset(u.Label, t.Label) && (len(u.Label) < len(t.Label) || j < i) {
				redundant = true
				break
			}
		}
		if !redundant {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return litsKey(out[i].Label) < litsKey(out[j].Label)
	})
	return out
}

func subset(a, b []Lit) bool {
	for _, x := range a {
		found := false
		for _, y := range b {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func nameStates(a *Automaton) {
	for i := range a.States {
		switch {
		case i == 0:
			a.States[i].Name = "T0_init"
		case a.States[i].Accepting:
			a.States[i].Name = "accept_S" + strconv.Itoa(i)
		default:
			a.States[i].Name = "T" + strconv.Itoa(i)
		}
	}
}

// Accepting counts the accepting states.
func (a *Automaton) Accepting() int {
	n := 0
	for _, s := range a.States {
		if s.Accepting {
			n++
		}
	}
	return n
}

// Transitions counts the transitions.
func (a *Automaton) Transitions() int {
	n := 0
	for _, s := range a.States {
		n += len(s.Trans)
	}
	return n
}

// LabelText renders a label as a guard in SPIN's style: "(1)" for true,
// else the conjunction of literals.
func (a *Automaton) LabelText(l []Lit) string {
	if len(l) == 0 {
		return "(1)"
	}
	var parts []string
	for _, x := range l {
		t := "(" + a.Atoms[x.Atom].Text + ")"
		if x.Neg {
			t = "!" + t
		}
		parts = append(parts, t)
	}
	return "(" + strings.Join(parts, " && ") + ")"
}
