package explore

import (
	"fmt"
	"time"

	"modelcheck/ctl"
	"modelcheck/ir"
	"modelcheck/ltl"
)

// CTL checking (G5): build the whole reachable graph (graph.go), label it
// with the normalised formula (package ctl), read the verdict off the
// initial state and render the run the division of witnesses allows.
//
// A CTL property is decided only on a *complete* graph. A budget that stops
// the construction makes it inconclusive: a fixed point over a partial
// graph would answer a different question (some successors are missing, so
// both "there is a path" and "every path" can flip), and the engine does
// not trade a wrong verdict for a fast one.

const (
	// KindCTL is the property kind this file executes.
	KindCTL = ir.KindCTL
	// ctlFairnessReason is the FR-008-shaped answer for CTL: the assumption
	// is refused, never silently dropped.
	ctlFairnessReason = "CTL under fairness is not executed by this engine version (plan 14 §4.2: \"fairness for CTL is out of scope\"): fair CTL needs a different fixed point (fair states must lie on a fair cycle), which this version does not compute; rerun the ctl property with fairness none, or express the fairness assumption in an ltl property, where weak fairness is implemented"
	// ctlTotalNote explains the self-loop that makes the relation total.
	ctlTotalNote = "CTL is evaluated over the whole reachable graph; a state in which no process can move (a deadlock, or a system whose processes have all terminated) carries a self-loop, so that the transition relation is total and EG/AF are defined there — the same stutter extension the LTL product uses"
)

// runCTL checks one ctl property and returns its outcome.
func runCTL(ctx0 *search, base *ir.Model, prop ir.Property, opt Options) (Outcome, error) {
	start := time.Now()
	o := Outcome{Property: prop}
	info := &TemporalInfo{Logic: "ctl", Source: "formula", Formula: prop.Formula, Atoms: []string{}, Fairness: opt.Fairness, Note: ctlTotalNote}
	if info.Fairness == "" {
		info.Fairness = "none"
	}
	o.Temporal = info
	if opt.Fairness == FairnessWeak || opt.Fairness == FairnessStrong {
		o.Status, o.Evidence, o.Reason = NotExecuted, EvUnknown, ctlFairnessReason
		return o, nil
	}
	if prop.Formula == "" {
		return o, &FormulaError{PropertyID: prop.ID, Logic: "ctl",
			Err: fmt.Errorf("a ctl property needs a formula: there is no \"the model as written\" reading of CTL (that reading belongs to a never claim, which is an ltl property)")}
	}
	l0, err := ir.NewLayout(base)
	if err != nil {
		return o, err
	}
	f, perr := ctl.Parse(prop.Formula, ctl.Options{Defines: opt.Defines, Env: ctl.ModelEnv(l0)})
	if perr != nil {
		return o, &FormulaError{PropertyID: prop.ID, Logic: "ctl", Err: perr}
	}
	nf := ctl.Normalise(f)
	info.Normalised = nf.String()
	for _, a := range f.Atoms() {
		info.Atoms = append(info.Atoms, a.Text)
	}

	g, err := BuildGraph(ctx0.ctx, base, opt)
	if err != nil {
		return o, err
	}
	o.Stats = g.Stats
	if g.Invalid != "" {
		o.Status, o.Evidence, o.Reason, o.Trace = InvalidModel, EvUnknown, g.Invalid, g.InvalidTrace
		o.Stats.Elapsed = time.Since(start)
		return o, nil
	}
	if !g.Stats.Complete {
		o.Status, o.Evidence = Inconclusive, budgetEvidence(g.Stats.Stop)
		o.Reason = g.Stats.Stop + "; a CTL verdict needs the complete reachable graph, because a fixed point over a partial graph answers a different question"
		o.Stats.Elapsed = time.Since(start)
		return o, nil
	}
	g.BuildPred()
	gg := ctlGraph{g}
	cache := map[string][]bool{}
	val := func(a *ctl.Atom) ([]bool, error) {
		if s, ok := cache[a.Text]; ok {
			return s, nil
		}
		c, err := g.Compile(a.Expr)
		if err != nil {
			return nil, err
		}
		s := make([]bool, g.Len())
		for i := range s {
			v, err := g.Eval(c, i)
			if err != nil {
				return nil, err
			}
			s[i] = v
		}
		cache[a.Text] = s
		return s, nil
	}
	lab, err := ctl.Label(gg, nf, val)
	if err != nil {
		return o, err
	}
	sat, err := lab.Eval(nf)
	if err != nil {
		return o, err
	}
	holds := sat[0]
	w, err := ctl.BuildWitness(gg, f, lab, holds)
	if err != nil {
		return o, err
	}
	o.Evidence = Exhaustive
	if holds {
		o.Status = Verified
		o.Reason = "the initial state satisfies " + f.String() + " on the complete reachable graph (" + fmt.Sprint(g.Len()) + " states)"
	} else {
		o.Status = Violated
		o.Reason = "the initial state does not satisfy " + f.String() + " on the complete reachable graph (" + fmt.Sprint(g.Len()) + " states)"
	}
	switch w.Kind {
	case "path":
		o.Trace = g.Trace(w.Path, -1)
	case "lasso":
		o.Trace = g.Trace(w.Path, w.Loop)
	default:
		info.WitnessNote = w.Why
	}
	if w.Note != "" {
		info.WitnessNote = w.Note
	}
	if o.Trace == nil && info.WitnessNote == "" {
		info.WitnessNote = "not available"
	}
	// Vacuity (FR-011): a hint from the atom statistics of the same graph,
	// never a change of the verdict.
	stats, err := ctl.Stats(gg, f, val)
	if err != nil {
		return o, err
	}
	var cov []Coverage
	for _, st := range stats {
		cov = append(cov, Coverage{Text: st.Atom.Text, EverTrue: st.EverTrue, EverFalse: st.EverFalse})
	}
	applyVacuity(&o, cov, ctl.Antecedents(f), g.Len())
	o.Stats.Elapsed = time.Since(start)
	return o, nil
}

// ctlGraph adapts the explorer's graph to what the labeller needs.
type ctlGraph struct{ g *Graph }

func (c ctlGraph) Len() int           { return c.g.Len() }
func (c ctlGraph) Succ(i int) []int32 { return c.g.Succ[i] }
func (c ctlGraph) Pred(i int) []int32 { return c.g.Pred[i] }

// Coverage is what a watched expression did over the states of a complete
// search: it is the raw material of the vacuity hints.
type Coverage struct {
	Text      string
	EverTrue  bool
	EverFalse bool
}

// applyVacuity turns atom coverage into the hints of FR-011. Three kinds,
// and nothing else:
//
//   - an atom that is never true in any reachable state — the property may
//     be about a situation the model never reaches;
//   - an atom that is never false — the property may be trivially satisfied
//     by it;
//   - an implication whose antecedent is never true — the property holds
//     vacuously, which sets Vacuous and names the atom.
//
// None of the three changes Status or Evidence: the verdict is what the
// search found. A vacuous `verified` stays `verified`.
func applyVacuity(o *Outcome, cov []Coverage, antecedents []string, states int) {
	byText := map[string]Coverage{}
	for _, c := range cov {
		byText[c.Text] = c
	}
	for _, c := range cov {
		switch {
		case !c.EverTrue:
			o.Warnings = append(o.Warnings, fmt.Sprintf("vacuity hint for %s: the atom %s is never true in any of the %d reachable states, so the property may not be about anything the model does", o.Property.ID, c.Text, states))
		case !c.EverFalse:
			o.Warnings = append(o.Warnings, fmt.Sprintf("vacuity hint for %s: the atom %s is never false in any of the %d reachable states, so it constrains nothing", o.Property.ID, c.Text, states))
		}
	}
	for _, a := range antecedents {
		text := unparen(a)
		c, ok := byText[text]
		if !ok || c.EverTrue {
			continue
		}
		if o.Temporal != nil {
			o.Temporal.Vacuous = true
			o.Temporal.VacuousAtom = text
		}
		o.Warnings = append(o.Warnings, fmt.Sprintf("vacuous: the antecedent %s of the implication in %s is never true in any reachable state, so the implication holds for a reason that has nothing to do with its consequent; the verdict below is unchanged — this is a hint, not a status", text, o.Property.ID))
	}
}

func unparen(s string) string {
	if len(s) >= 2 && s[0] == '(' && s[len(s)-1] == ')' {
		return s[1 : len(s)-1]
	}
	return s
}

// ---- vacuity for ltl properties --------------------------------------------------

// vacuityCoverage picks, out of the coverage recorded by the safety search,
// the entries that belong to the atoms of this property. An empty result
// (no safety search, or one that did not finish) means no hint is given:
// a hint about a partial graph could be wrong, and a wrong hint is worse
// than none.
func vacuityCoverage(res *Result, info *TemporalInfo) []Coverage {
	if info == nil || len(res.Coverage) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, a := range info.Atoms {
		want[a] = true
	}
	var out []Coverage
	for _, c := range res.Coverage {
		if want[c.Text] {
			out = append(out, c)
		}
	}
	return out
}

func antecedentsOf(info *TemporalInfo) []string {
	if info == nil {
		return nil
	}
	return info.Antecedents
}

// WatchExprs collects the atoms of every ltl property of m that the safety
// search should record for the vacuity hints. A formula that does not parse
// contributes nothing: the error is raised where the property is checked,
// not here.
func WatchExprs(m *ir.Model, defines map[string]string) []*ir.Expr {
	l, err := ir.NewLayout(m)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []*ir.Expr
	for _, p := range m.Properties {
		if p.Kind != ir.KindLTL || p.Formula == "" {
			continue
		}
		f, err := ltl.Parse(p.Formula, ltl.Options{Defines: defines, Scope: l.Scope(-1)})
		if err != nil {
			continue
		}
		for _, a := range f.Atoms() {
			if !seen[a.Text] {
				seen[a.Text] = true
				out = append(out, a.Expr)
			}
		}
	}
	return out
}
