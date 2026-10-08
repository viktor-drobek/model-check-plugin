package explore

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"modelcheck/ir"
)

// CTL oracle (the CTL campaign of the 0.3.0 integration).
//
// The engine decides a ctl property by labelling the reachable graph of its
// own breadth-first builder (graph.go, package ctl). This oracle decides the
// same formula in a different way: it builds the reachable graph again from
// the Stepper (one edge per step, no search code in common with graph.go) and
// evaluates the formula by naive fixed-point iteration over whole state sets
// (EX/AX one step, EG and AG the greatest fixed point, EF/AF/EU/AU the least),
// never the normal form of package ctl. A state with no enabled move carries
// a self-loop, the same totality rule the engine states in its report.
//
// Only models whose steps are single edges are compared: an atomic or d_step
// sequence is one step of the engine's graph and several states of the
// Stepper's, so the two graphs differ by construction there.
//
// The knobs are those of the other oracles: MCD_POR_MODELS, MCD_POR_SEED,
// MCD_POR_GEN, MCD_POR_ALL (the generators are the ones of the reduction's
// oracles; the default list is the generators without atomic sequences).

type ctlNode struct {
	op   string
	a, b *ctlNode
	atom int
}

type ctlAtom struct {
	text string
	expr *ir.Expr
}

func (f *ctlNode) text(at []ctlAtom) string {
	switch f.op {
	case "atom":
		return "(" + at[f.atom].text + ")"
	case "not":
		return "!" + f.a.text(at)
	case "and":
		return "(" + f.a.text(at) + " && " + f.b.text(at) + ")"
	case "or":
		return "(" + f.a.text(at) + " || " + f.b.text(at) + ")"
	case "imp":
		return "(" + f.a.text(at) + " -> " + f.b.text(at) + ")"
	case "AU":
		return "A[" + f.a.text(at) + " U " + f.b.text(at) + "]"
	case "EU":
		return "E[" + f.a.text(at) + " U " + f.b.text(at) + "]"
	}
	return f.op + " " + f.a.text(at) // AX EX AG EG AF EF
}

func ctlAtomsOf(m *ir.Model, table bool) []ctlAtom {
	var at []ctlAtom
	for _, v := range m.Globals {
		if v.Len != 0 || v.Type != ir.Byte {
			continue
		}
		for c := int64(0); c < 3; c++ {
			at = append(at,
				ctlAtom{fmt.Sprintf("%s == %d", v.Name, c), ir.Binary("eq", ir.Ref(v.Name), ir.Const(c))},
				ctlAtom{fmt.Sprintf("%s != %d", v.Name, c), ir.Binary("ne", ir.Ref(v.Name), ir.Const(c))})
		}
		at = append(at, ctlAtom{v.Name + " < 2", ir.Binary("lt", ir.Ref(v.Name), ir.Const(2))})
	}
	if table {
		for c := int64(0); c < 4; c++ {
			at = append(at, ctlAtom{fmt.Sprintf("_nr_pr == %d", c), ir.Binary("eq", ir.NrPr(), ir.Const(c))})
		}
	}
	return at
}

func ctlRandom(rnd *rand.Rand, depth, natoms int) *ctlNode {
	if depth == 0 || rnd.Intn(5) == 0 {
		return &ctlNode{op: "atom", atom: rnd.Intn(natoms)}
	}
	un := []string{"not", "AX", "EX", "AG", "EG", "AF", "EF"}
	bin := []string{"and", "or", "imp", "AU", "EU"}
	if rnd.Intn(3) != 0 {
		return &ctlNode{op: un[rnd.Intn(len(un))], a: ctlRandom(rnd, depth-1, natoms)}
	}
	return &ctlNode{op: bin[rnd.Intn(len(bin))], a: ctlRandom(rnd, depth-1, natoms), b: ctlRandom(rnd, depth-1, natoms)}
}

// ctlRefGraph is the explicit graph of the Stepper with the totality self-loops.
type ctlRefGraph struct {
	states [][]byte
	succ   [][]int
	init   int
}

// ctlBuild returns the graph or the reason the model is not compared.
func ctlBuild(st *Stepper, limit int) (*ctlRefGraph, string) {
	g := &ctlRefGraph{}
	index := map[string]int{}
	add := func(s []byte) int {
		if i, ok := index[string(s)]; ok {
			return i
		}
		index[string(s)] = len(g.states)
		g.states = append(g.states, s)
		g.succ = append(g.succ, nil)
		return len(g.states) - 1
	}
	g.init = add(st.Initial())
	for i := 0; i < len(g.states); i++ {
		if len(g.states) > limit {
			return nil, "graph above the limit"
		}
		mvs, err := st.Enabled(g.states[i])
		if err != nil {
			return nil, "enabled: " + err.Error()
		}
		if len(mvs) == 0 {
			g.succ[i] = []int{i}
			continue
		}
		seen := map[int]bool{}
		for _, mv := range mvs {
			next, failed, err := st.Apply(g.states[i], mv)
			if err != nil {
				return nil, "apply: " + err.Error()
			}
			if failed != nil {
				return nil, "a failing assert"
			}
			j := add(next)
			if !seen[j] {
				seen[j] = true
				g.succ[i] = append(g.succ[i], j)
			}
		}
		sort.Ints(g.succ[i])
	}
	return g, ""
}

func (g *ctlRefGraph) ex(z []bool) []bool {
	out := make([]bool, len(z))
	for i, ss := range g.succ {
		for _, j := range ss {
			if z[j] {
				out[i] = true
				break
			}
		}
	}
	return out
}

func (g *ctlRefGraph) ax(z []bool) []bool {
	out := make([]bool, len(z))
	for i, ss := range g.succ {
		out[i] = true
		for _, j := range ss {
			if !z[j] {
				out[i] = false
				break
			}
		}
	}
	return out
}

func ctlEq(a, b []bool) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (g *ctlRefGraph) eval(f *ctlNode, atoms [][]bool) []bool {
	n := len(g.states)
	switch f.op {
	case "atom":
		return atoms[f.atom]
	case "not":
		a := g.eval(f.a, atoms)
		out := make([]bool, n)
		for i := range a {
			out[i] = !a[i]
		}
		return out
	case "and", "or", "imp":
		a, b := g.eval(f.a, atoms), g.eval(f.b, atoms)
		out := make([]bool, n)
		for i := range out {
			switch f.op {
			case "and":
				out[i] = a[i] && b[i]
			case "or":
				out[i] = a[i] || b[i]
			default:
				out[i] = !a[i] || b[i]
			}
		}
		return out
	case "EX":
		return g.ex(g.eval(f.a, atoms))
	case "AX":
		return g.ax(g.eval(f.a, atoms))
	}
	a := g.eval(f.a, atoms)
	var b []bool
	if f.op == "AU" || f.op == "EU" {
		b = g.eval(f.b, atoms)
	}
	next := func(z []bool) []bool { // one application of the operator's functional
		var pre []bool
		if f.op == "EG" || f.op == "EF" || f.op == "EU" {
			pre = g.ex(z)
		} else {
			pre = g.ax(z)
		}
		out := make([]bool, n)
		for i := range out {
			switch f.op {
			case "EG", "AG":
				out[i] = a[i] && pre[i]
			case "EF", "AF":
				out[i] = a[i] || pre[i]
			default: // EU, AU
				out[i] = b[i] || (a[i] && pre[i])
			}
		}
		return out
	}
	z := make([]bool, n)
	if f.op == "EG" || f.op == "AG" { // greatest fixed point: start from everything
		for i := range z {
			z[i] = true
		}
	}
	for {
		nz := next(z)
		if ctlEq(nz, z) {
			return z
		}
		z = nz
	}
}

// ctlOracle compares the engine's verdict of a handful of random formulas on m.
func ctlOracle(m *ir.Model, seed int64, tl *porTally) (porOutcome, error) {
	for _, p := range m.Processes {
		for _, e := range p.Edges {
			if e.Atomic || e.DStep {
				return porSkipped, nil
			}
		}
	}
	st, err := NewStepper(m)
	if err != nil {
		return porSkipped, nil
	}
	g, why := ctlBuild(st, porOracleBudget.MaxStates)
	if why != "" {
		return porSkipped, nil
	}
	table := st.Layout().HasTable()
	atoms := ctlAtomsOf(m, table)
	if len(atoms) == 0 {
		return porSkipped, nil
	}
	truth := make([][]bool, len(atoms))
	for i, a := range atoms {
		c, err := st.Layout().Compile(a.expr, -1)
		if err != nil {
			return porSkipped, nil
		}
		truth[i] = make([]bool, len(g.states))
		for j, s := range g.states {
			v, err := c.Truth(s)
			if err != nil {
				return porSkipped, nil
			}
			truth[i][j] = v
		}
	}
	rnd := rand.New(rand.NewSource(seed*7919 + 13))
	var fs []*ctlNode
	m2 := *m
	m2.Properties = nil
	for k := 0; k < 8; k++ {
		f := ctlRandom(rnd, 1+rnd.Intn(3), len(atoms))
		fs = append(fs, f)
		m2.Properties = append(m2.Properties, ir.Property{ID: fmt.Sprintf("c%d", k), Kind: ir.KindCTL, Formula: f.text(atoms)})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := Run(ctx, &m2, Options{Budget: porOracleBudget})
	if err != nil {
		return 0, fmt.Errorf("Run: %w", err)
	}
	if errorStop(res) || otherBudget(res) {
		return porSkipped, nil
	}
	compared := 0
	for k, f := range fs {
		var o *Outcome
		for i := range res.Outcomes {
			if res.Outcomes[i].Property.ID == fmt.Sprintf("c%d", k) {
				o = &res.Outcomes[i]
			}
		}
		if o == nil {
			return 0, fmt.Errorf("no outcome for formula %q", m2.Properties[k].Formula)
		}
		want := g.eval(f, truth)[g.init]
		switch o.Status {
		case Verified, Violated:
			if got := o.Status == Verified; got != want {
				norm := ""
				if o.Temporal != nil {
					norm = o.Temporal.Normalised
				}
				return 0, fmt.Errorf("formula %q (normalised %q): engine %s, oracle %v (%d states)", m2.Properties[k].Formula, norm, o.Status, want, len(g.states))
			}
			compared++
		default:
			tl.refused++ // not executed / inconclusive: nothing to compare
		}
	}
	tl.checked += compared
	return porSame, nil
}

func TestCTLAgreesWithTheNaiveOracleOnTheGenerators(t *testing.T) {
	gens := porGeneratorsToRun("base", "provided", "run", "reads", "nrpr")
	for _, g := range gens {
		g := g
		t.Run(g.name, func(t *testing.T) {
			var seed atomic.Int64
			tal := porForSeeds(t, "ctl oracle", g, 300, func(m *ir.Model, tl *porTally) (porOutcome, error) {
				return ctlOracle(m, seed.Add(1), tl)
			})
			// A run that skips every model (a Stepper that refuses them, a graph
			// above the limit) would otherwise pass having compared nothing.
			if tal.checked < 2*tal.models {
				t.Errorf("ctl oracle [%s]: %d formulas compared on %d models, want at least %d", g.name, tal.checked, tal.models, 2*tal.models)
			}
		})
	}
}
