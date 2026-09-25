package ctl

import (
	"fmt"
	"sort"
)

// Labelling over the reachable graph (Clarke–Emerson–Sistla): every
// subformula of the normalised formula is evaluated once, bottom-up, as the
// set of states that satisfy it. Each of the three temporal cases costs one
// pass over the edges, so the whole labelling is O(|states| + |edges|) per
// subformula and therefore linear in |graph| × |formula|.
//
//	EX f       states with a successor in sat(f)                  one pass
//	E[f U g]   least fixed point: sat(g), then back along edges    worklist
//	           through states of sat(f)
//	EG f       greatest fixed point: the states of sat(f) that     worklist
//	           keep a successor inside the set — computed by
//	           repeatedly dropping the states of sat(f) whose
//	           successors have all been dropped
//
// The graph's transition relation is total (the caller adds a self-loop to
// every state without a move), which is what makes EG on a terminating run
// well defined.

// Graph is what the labeller needs of a model's reachable graph.
type Graph interface {
	Len() int
	// Succ returns the successors of state i; it is never empty.
	Succ(i int) []int32
	// Pred returns the predecessors of state i.
	Pred(i int) []int32
}

// Valuation evaluates an atom on every state; the slice it returns is
// indexed by state.
type Valuation func(*Atom) ([]bool, error)

// Labelling holds sat sets by subformula text.
type Labelling struct {
	g   Graph
	sat map[string][]bool
	val Valuation
}

// Label evaluates f (which must be normalised) over g.
func Label(g Graph, f *Formula, val Valuation) (*Labelling, error) {
	if !IsNormal(f) {
		return nil, fmt.Errorf("ctl: Label needs a formula in the EX/EU/EG basis; call Normalise first (got %s)", f)
	}
	l := &Labelling{g: g, sat: map[string][]bool{}, val: val}
	if _, err := l.eval(f); err != nil {
		return nil, err
	}
	return l, nil
}

// Sat returns the set of states satisfying the subformula, or nil when it
// was not evaluated.
func (l *Labelling) Sat(f *Formula) []bool { return l.sat[f.String()] }

// Eval returns the set of states satisfying f, evaluating it (and reusing
// what is already labelled) if need be. f must be in the EX/EU/EG basis.
func (l *Labelling) Eval(f *Formula) ([]bool, error) {
	if !IsNormal(f) {
		return nil, fmt.Errorf("ctl: Eval needs a formula in the EX/EU/EG basis (got %s)", f)
	}
	return l.eval(f)
}

// Holds reports whether state i satisfies f.
func (l *Labelling) Holds(f *Formula, i int) bool {
	s := l.sat[f.String()]
	return s != nil && s[i]
}

func (l *Labelling) eval(f *Formula) ([]bool, error) {
	key := f.String()
	if s, ok := l.sat[key]; ok {
		return s, nil
	}
	n := l.g.Len()
	out := make([]bool, n)
	switch f.Op {
	case True:
		for i := range out {
			out[i] = true
		}
	case False:
		// all false
	case AtomOp:
		s, err := l.val(f.Atom)
		if err != nil {
			return nil, err
		}
		if len(s) != n {
			return nil, fmt.Errorf("ctl: the valuation of %s covers %d states, the graph has %d", f, len(s), n)
		}
		out = s
	case Not:
		a, err := l.eval(f.L)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i] = !a[i]
		}
	case And:
		a, err := l.eval(f.L)
		if err != nil {
			return nil, err
		}
		b, err := l.eval(f.R)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i] = a[i] && b[i]
		}
	case Or:
		a, err := l.eval(f.L)
		if err != nil {
			return nil, err
		}
		b, err := l.eval(f.R)
		if err != nil {
			return nil, err
		}
		for i := range out {
			out[i] = a[i] || b[i]
		}
	case EX:
		a, err := l.eval(f.L)
		if err != nil {
			return nil, err
		}
		for i := 0; i < n; i++ {
			for _, t := range l.g.Succ(i) {
				if a[t] {
					out[i] = true
					break
				}
			}
		}
	case EU:
		a, err := l.eval(f.L)
		if err != nil {
			return nil, err
		}
		b, err := l.eval(f.R)
		if err != nil {
			return nil, err
		}
		out = euSat(l.g, a, b)
	case EG:
		a, err := l.eval(f.L)
		if err != nil {
			return nil, err
		}
		out = egSat(l.g, a)
	default:
		return nil, fmt.Errorf("ctl: %s is not in the EX/EU/EG basis", f)
	}
	l.sat[key] = out
	return out, nil
}

// euSat is the least fixed point of X = b ∨ (a ∧ EX X).
func euSat(g Graph, a, b []bool) []bool {
	n := g.Len()
	out := make([]bool, n)
	var queue []int
	for i := 0; i < n; i++ {
		if b[i] {
			out[i] = true
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		cur := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, p := range g.Pred(cur) {
			if !out[p] && a[p] {
				out[p] = true
				queue = append(queue, int(p))
			}
		}
	}
	return out
}

// egSat is the greatest fixed point of X = a ∧ EX X: start from every state
// of a and drop those whose successors have all been dropped. The counter
// makes it one pass over the edges.
func egSat(g Graph, a []bool) []bool {
	n := g.Len()
	out := append([]bool(nil), a...)
	count := make([]int32, n)
	var queue []int
	for i := 0; i < n; i++ {
		if !out[i] {
			continue
		}
		c := int32(0)
		for _, t := range g.Succ(i) {
			if out[t] {
				c++
			}
		}
		count[i] = c
		if c == 0 {
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		cur := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		out[cur] = false
		for _, p := range g.Pred(cur) {
			if !out[p] {
				continue
			}
			count[p]--
			if count[p] == 0 {
				queue = append(queue, int(p))
			}
		}
	}
	return out
}

// ---- witnesses ------------------------------------------------------------------

// Witness is the run the engine can show for a verdict, or the honest
// statement that there is none. Kind is "path" (a finite run), "lasso" (a
// run with a repeating suffix) or "none".
type Witness struct {
	Kind string
	// Path is the sequence of state indices; for a lasso the states from
	// Loop on repeat, and Path ends at the state Path[Loop] again.
	Path []int
	Loop int
	// Why explains a "none": which rule of the division applies.
	Why string
	// Note is extra honesty attached to a run, e.g. that a nested
	// subformula's failure is not itself a finite run.
	Note string
}

// The four texts of the "none" branch, written once so that the division
// below and the documentation cannot drift apart.
const (
	whyUniversalHolds   = "not available: a universal property that holds is justified by the complete reachable graph, not by one run — there is no single run to show"
	whyExistentialFails = "not available: a failing existential property has no run to show — the claim is that no run of the whole graph fulfils it"
	whyBoolean          = "not available: the formula's top operator is a boolean connective or an atom, so the verdict is a property of the initial state itself and no run leads to it"
	NestedNote          = "the counterexample ends at the state where the outer property fails; why the nested subformula fails there is not a finite run and is not shown"
)

// BuildWitness picks the run for the verdict of the *original* formula f
// (before normalisation) at the initial state, given the labelling of its
// normalisation nf. The division is exhaustive and exclusive:
//
//	holds & top is EF / EX / E[·U·]   → a finite witness path
//	holds & top is EG                 → a lasso
//	fails & top is AG                 → a finite path to a violating state
//	fails & top is AX                 → a one-step path to a violating successor
//	fails & top is AF / A[·U·]        → a lasso that never reaches the goal
//	anything else                     → Kind "none" with the reason
//
// "Anything else" is: an existential top operator that fails, a universal
// one that holds, and a top-level boolean connective or atom either way.
func BuildWitness(g Graph, f *Formula, l *Labelling, holds bool) (Witness, error) {
	sat := l.Eval
	switch f.Op {
	case EF, EX, EU, EG:
		if !holds {
			return Witness{Kind: "none", Why: whyExistentialFails}, nil
		}
	case AG, AX, AF, AU:
		if holds {
			return Witness{Kind: "none", Why: whyUniversalHolds}, nil
		}
	default:
		return Witness{Kind: "none", Why: whyBoolean}, nil
	}
	switch f.Op {
	case EX:
		inner, err := sat(Normalise(f.L))
		if err != nil {
			return Witness{}, err
		}
		for _, t := range g.Succ(0) {
			if inner[t] {
				return Witness{Kind: "path", Path: []int{0, int(t)}, Loop: -1}, nil
			}
		}
		return Witness{Kind: "none", Why: whyExistentialFails}, nil
	case EF:
		goal, err := sat(Normalise(f.L))
		if err != nil {
			return Witness{}, err
		}
		return pathWitness(g, allStates(g), goal)
	case EU:
		left, err := sat(Normalise(f.L))
		if err != nil {
			return Witness{}, err
		}
		goal, err := sat(Normalise(f.R))
		if err != nil {
			return Witness{}, err
		}
		within := make([]bool, g.Len())
		for i := range within {
			within[i] = left[i] || goal[i]
		}
		return pathWitness(g, within, goal)
	case EG:
		inner, err := sat(Normalise(f.L))
		if err != nil {
			return Witness{}, err
		}
		return lassoWitness(g, egSat(g, inner))
	case AG:
		inner, err := sat(Normalise(f.L))
		if err != nil {
			return Witness{}, err
		}
		bad := negate(inner)
		w, err := pathWitness(g, allStates(g), bad)
		if err == nil && w.Kind == "path" && hasTemporal(f.L) {
			w.Note = NestedNote
		}
		return w, err
	case AX:
		inner, err := sat(Normalise(f.L))
		if err != nil {
			return Witness{}, err
		}
		for _, t := range g.Succ(0) {
			if !inner[t] {
				w := Witness{Kind: "path", Path: []int{0, int(t)}, Loop: -1}
				if hasTemporal(f.L) {
					w.Note = NestedNote
				}
				return w, nil
			}
		}
		return Witness{Kind: "none", Why: whyUniversalHolds}, nil
	case AF:
		inner, err := sat(Normalise(f.L))
		if err != nil {
			return Witness{}, err
		}
		return lassoWitness(g, egSat(g, negate(inner)))
	case AU:
		right, err := sat(Normalise(f.R))
		if err != nil {
			return Witness{}, err
		}
		left, err := sat(Normalise(f.L))
		if err != nil {
			return Witness{}, err
		}
		// ¬A[f U g] = E[¬g U (¬f ∧ ¬g)] ∨ EG ¬g; the first disjunct gives a
		// finite path, the second a lasso. Both are checked, the finite one
		// first because it is the shorter answer.
		nl, nr := negate(left), negate(right)
		both := make([]bool, g.Len())
		for i := range both {
			both[i] = nl[i] && nr[i]
		}
		within := make([]bool, g.Len())
		for i := range within {
			within[i] = nr[i] || both[i]
		}
		if w, err := pathWitness(g, within, both); err == nil && w.Kind == "path" {
			return w, nil
		}
		return lassoWitness(g, egSat(g, nr))
	}
	return Witness{Kind: "none", Why: whyBoolean}, nil
}

func allStates(g Graph) []bool {
	s := make([]bool, g.Len())
	for i := range s {
		s[i] = true
	}
	return s
}

func negate(s []bool) []bool {
	out := make([]bool, len(s))
	for i, v := range s {
		out[i] = !v
	}
	return out
}

// hasTemporal reports whether g contains a path operator, i.e. whether a
// state that violates it violates something that is itself a claim about
// runs.
func hasTemporal(g *Formula) bool {
	if g == nil {
		return false
	}
	switch g.Op {
	case EX, EF, EG, EU, AX, AF, AG, AU:
		return true
	}
	return hasTemporal(g.L) || hasTemporal(g.R)
}

// pathWitness is a shortest path from the initial state to a goal state
// using only states of within.
func pathWitness(g Graph, within, goal []bool) (Witness, error) {
	if !within[0] {
		return Witness{Kind: "none", Why: whyExistentialFails}, nil
	}
	if goal[0] {
		return Witness{Kind: "path", Path: []int{0}, Loop: -1}, nil
	}
	prev := make([]int32, g.Len())
	for i := range prev {
		prev[i] = -1
	}
	seen := make([]bool, g.Len())
	seen[0] = true
	queue := []int{0}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, t := range g.Succ(cur) {
			if seen[t] || !within[t] {
				continue
			}
			seen[t] = true
			prev[t] = int32(cur)
			if goal[t] {
				return Witness{Kind: "path", Path: backtrack(prev, int(t)), Loop: -1}, nil
			}
			queue = append(queue, int(t))
		}
	}
	return Witness{Kind: "none", Why: whyExistentialFails}, nil
}

// lassoWitness walks from the initial state inside the set (every state of
// which has a successor in the set, because the set is egSat of something)
// until a state repeats: the prefix and the loop are then read off the walk.
func lassoWitness(g Graph, set []bool) (Witness, error) {
	if !set[0] {
		return Witness{Kind: "none", Why: whyExistentialFails}, nil
	}
	pos := map[int]int{}
	var path []int
	cur := 0
	for {
		if k, ok := pos[cur]; ok {
			path = append(path, cur)
			return Witness{Kind: "lasso", Path: path, Loop: k}, nil
		}
		pos[cur] = len(path)
		path = append(path, cur)
		nxt := -1
		for _, t := range g.Succ(cur) {
			if set[t] {
				nxt = int(t)
				break
			}
		}
		if nxt < 0 {
			return Witness{}, fmt.Errorf("ctl: state %d is in an EG set but has no successor in it", cur)
		}
		cur = nxt
	}
}

func backtrack(prev []int32, end int) []int {
	var rev []int
	for i := end; ; {
		rev = append(rev, i)
		p := int(prev[i])
		if p < 0 {
			break
		}
		i = p
	}
	out := make([]int, len(rev))
	for i, v := range rev {
		out[len(rev)-1-i] = v
	}
	return out
}

// AtomStats reports, per atom, whether it was ever true and ever false over
// the whole graph — the raw material of the vacuity hints of FR-011. The
// atoms come back in the order of first occurrence in the formula, so the
// hints are deterministic.
type AtomStat struct {
	Atom      *Atom
	EverTrue  bool
	EverFalse bool
}

// Stats evaluates every atom of f over the graph.
func Stats(g Graph, f *Formula, val Valuation) ([]AtomStat, error) {
	var out []AtomStat
	for _, a := range f.Atoms() {
		s, err := val(a)
		if err != nil {
			return nil, err
		}
		st := AtomStat{Atom: a}
		for _, v := range s {
			if v {
				st.EverTrue = true
			} else {
				st.EverFalse = true
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// Antecedents lists the antecedents of the implications of f, innermost
// last, as the text of the atom or subformula on the left of `->`. They are
// the candidates FR-011 asks about for `AG(a -> b)`.
func Antecedents(f *Formula) []string {
	var out []string
	var walk func(g *Formula)
	walk = func(g *Formula) {
		if g == nil {
			return
		}
		if g.Op == Impl {
			out = append(out, g.L.String())
		}
		walk(g.L)
		walk(g.R)
	}
	walk(f)
	sort.Strings(out)
	return out
}
