package ltl

import (
	"modelcheck/ir"
)

// Claim is the never claim built for a property.
type Claim struct {
	// Process is the automaton as an IR claim process: one location per
	// automaton state (location 0 = T0_init), one edge per transition with
	// the transition's label as guard, `accept` on accepting locations.
	Process ir.Process
	Info    Info
}

// ForProperty builds the never claim that detects violations of the
// property formula: it parses the formula, NEGATES it, translates the
// negation and lowers the automaton to a claim process named name. An
// acceptance cycle of the product of the model with this claim is a run
// satisfying !(formula), i.e. a counterexample to the property.
func ForProperty(name, formula string, opt Options) (*Claim, error) {
	f, err := Parse(formula, opt)
	if err != nil {
		return nil, err
	}
	neg := NotF(f)
	a := Translate(neg)
	c := &Claim{Process: Lower(a, name, "!("+formula+")")}
	c.Info = Info{
		Formula:          formula,
		Negated:          "!(" + formula + ")",
		Atoms:            []string{},
		StutterInvariant: !f.HasNext(),
		States:           len(a.States),
		Transitions:      a.Transitions(),
		Accepting:        a.Accepting(),
	}
	for _, at := range f.Atoms() {
		c.Info.Atoms = append(c.Info.Atoms, at.Text)
	}
	return c, nil
}

// Lower turns a into a claim process. The edge order is the automaton's
// (state by state, transitions as simplified), so equal formulas give
// equal processes.
func Lower(a *Automaton, name, origin string) ir.Process {
	p := ir.Process{Name: name, Claim: true, Origin: &ir.Origin{Name: origin}}
	for _, s := range a.States {
		loc := ir.Location{Name: s.Name}
		if s.Accepting {
			loc.Labels = []ir.Label{ir.Accept}
		}
		p.Locations = append(p.Locations, loc)
	}
	for from, s := range a.States {
		for _, t := range s.Trans {
			text := a.LabelText(t.Label)
			e := ir.Edge{From: from, To: t.Target, Guard: guardOf(a, t.Label), Text: text + " -> goto " + a.States[t.Target].Name,
				Origin: &ir.Origin{Name: text}}
			p.Edges = append(p.Edges, e)
		}
	}
	if p.Edges == nil {
		p.Edges = []ir.Edge{}
	}
	return p
}

// guardOf is the conjunction of the literals, nil for an empty label.
func guardOf(a *Automaton, l []Lit) *ir.Expr {
	if len(l) == 0 {
		return nil
	}
	var parts []*ir.Expr
	for _, x := range l {
		e := a.Atoms[x.Atom].Expr
		if x.Neg {
			e = ir.Unary("not", e)
		}
		parts = append(parts, e)
	}
	return ir.And(parts...)
}
