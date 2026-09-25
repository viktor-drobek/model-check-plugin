package mcp

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/ctl"
	"modelcheck/ir"
	"modelcheck/ltl"
)

// LintIn is the input of mc_lint_property: a boolean state expression for
// invariant and reach, an LTL formula (SPIN syntax) for ltl, a CTL formula
// for ctl. The two logics are linted by their own parsers: a formula that
// is CTL is not read as LTL, and a formula that is LTL is not read as CTL.
type LintIn struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"session holding the parsed model; omitted = a new session (then ir is required)"`
	IR        any    `json:"ir,omitempty" jsonschema:"IR JSON; omitted = the session's parsed model"`
	Expr      any    `json:"expr,omitempty" jsonschema:"invariant, reach: boolean state expression, IR expression JSON or a bare variable name"`
	Formula   string `json:"formula,omitempty" jsonschema:"ltl: the formula in SPIN syntax; ctl: the formula in CTL syntax; #define symbols of a Promela model parsed in this session are expanded"`
	Kind      string `json:"kind,omitempty" jsonschema:"invariant | reach | ltl | ctl (default invariant); decides the class"`
}

// LintOut is the answer of mc_lint_property.
type LintOut struct {
	SessionID string `json:"session_id"`
	// Logic is "ltl" or "ctl" for a temporal formula, empty for a state
	// expression: the caller is told which parser read the text.
	Logic      string   `json:"logic,omitempty"`
	Expr       string   `json:"expr" jsonschema:"the expression as the engine reads it"`
	Kind       string   `json:"kind"`
	Atoms      []string `json:"atoms" jsonschema:"variables the expression reads, in order of first occurrence"`
	Undefined  []string `json:"undefined" jsonschema:"atoms that are not global variables of the model"`
	TypeOK     bool     `json:"type_ok"`
	TypeError  string   `json:"type_error,omitempty"`
	Class      string   `json:"class" jsonschema:"safety | reachability | liveness | unknown"`
	ClassBasis string   `json:"class_basis"`
	XFree      bool     `json:"x_free" jsonschema:"true when the formula has no X (next) operator; a state expression has none; a formula with X is not stutter-invariant"`
	Temporal   bool     `json:"temporal" jsonschema:"true for an ltl formula, false for a state expression"`
	NNF        string   `json:"nnf,omitempty" jsonschema:"ltl: the formula in negation normal form as the engine reads it"`
	Normalised string   `json:"normalised,omitempty" jsonschema:"ctl: the formula in the EX/EU/EG basis, as it will be labelled"`
	Constant   bool     `json:"constant" jsonschema:"true when the expression reads no variable: it is vacuously true or false everywhere"`
	Notes      []string `json:"notes"`
}

func (s *Server) lint(ctx context.Context, req *sdk.CallToolRequest, in LintIn) (*sdk.CallToolResult, *LintOut, error) {
	if in.SessionID == "" && in.IR == nil {
		return nil, nil, errors.New("no model: pass `ir` inline or a session_id whose model was parsed with mc_parse")
	}
	kind := in.Kind
	if kind == "" {
		kind = "invariant"
		if in.Expr == nil && in.Formula != "" {
			kind = "ltl"
		}
	}
	switch kind {
	case "invariant", "reach":
		if in.Expr == nil {
			return nil, nil, errors.New("expr is required for invariant and reach")
		}
	case "ltl":
		if in.Formula == "" {
			return nil, nil, errors.New("formula is required for ltl")
		}
	case "ctl":
		if in.Formula == "" {
			return nil, nil, errors.New("formula is required for ctl")
		}
	case "progress":
		return nil, nil, errors.New("kind progress has no formula to lint: it is the absence of non-progress cycles (SPIN pan -l)")
	default:
		return nil, nil, fmt.Errorf("kind must be invariant, reach, ltl or ctl, got %q", kind)
	}
	var e *ir.Expr
	if kind != "ltl" && kind != "ctl" {
		var err error
		if e, err = exprFrom(in.Expr); err != nil {
			return nil, nil, err
		}
	}
	sess, err := s.session(in.SessionID, true)
	if err != nil {
		return nil, nil, err
	}
	timer := begin(sess, "mc_lint_property")
	defer func() { timer.end(err, nil, nil) }()
	m, rej, err := s.modelFor(sess, in.IR)
	if err != nil {
		return nil, nil, err
	}
	if rej != nil {
		err = rejectedInput(rej)
		return nil, nil, err
	}
	l, err := ir.NewLayout(m)
	if err != nil {
		return nil, nil, err
	}
	scope := l.Scope(-1)
	if kind == "ltl" || kind == "ctl" {
		sess.mu.Lock()
		defines := sess.defines
		sess.mu.Unlock()
		if in.IR != nil {
			defines = nil
		}
		if kind == "ctl" {
			return nil, lintCTL(sess.ID, in.Formula, defines, l), nil
		}
		return nil, lintLTL(sess.ID, in.Formula, defines, scope), nil
	}
	out := &LintOut{SessionID: sess.ID, Expr: e.String(), Kind: kind, Atoms: []string{}, Undefined: []string{}, XFree: true, Notes: []string{}}
	seen := map[string]bool{}
	var walk func(x *ir.Expr)
	walk = func(x *ir.Expr) {
		if x == nil {
			return
		}
		if x.Op == "var" || x.Op == "index" {
			if !seen[x.Var] {
				seen[x.Var] = true
				out.Atoms = append(out.Atoms, x.Var)
				if scope.LookupVar(x.Var) == nil {
					out.Undefined = append(out.Undefined, x.Var)
				}
			}
		}
		for _, a := range x.Args {
			walk(a)
		}
	}
	walk(e)
	out.Constant = len(out.Atoms) == 0
	if k, cerr := ir.Check(e, scope); cerr != nil {
		out.TypeError = cerr.Error()
	} else {
		out.TypeOK = true
		if k != ir.KBool {
			out.Notes = append(out.Notes, "the expression is of kind int; it is read as true when non-zero (Promela convention)")
		}
	}
	switch kind {
	case "invariant":
		out.Class = "safety"
		out.ClassBasis = "an invariant is a safety property: a violation is a finite run ending in a state where the expression is false"
	case "reach":
		out.Class = "reachability"
		out.ClassBasis = "reach is existential: one run to a state where the expression holds proves it; refuting it needs the whole reachable graph"
	}
	if out.Constant {
		out.Notes = append(out.Notes, "the expression is constant (it reads no variable): it is vacuously true or false in every state — a vacuity candidate")
	}
	for _, u := range out.Undefined {
		out.Notes = append(out.Notes, fmt.Sprintf("%q is not a global variable of the model; process locals are not visible to properties", u))
	}
	return nil, out, nil
}

// lintLTL parses the formula (without type-checking, so that every atom can
// be listed) and reports its atoms, the undefined ones, X-freeness and a
// syntactic class: a formula whose negation normal form has no U and no <>
// is a safety property (its violations are finite prefixes); otherwise it
// has a liveness part (its violations need a loop). The class is a
// syntactic sufficient condition, not a semantic classification.
// lintCTL parses the formula with the CTL parser and reports its atoms,
// its normalisation into the EX/EU/EG basis and the vacuity candidates of
// FR-011. A text that is not CTL is reported as such, with no attempt to
// read it as LTL: the two logics answer different questions, and guessing
// which one the user meant would be the substitution E6 exists to forbid.
func lintCTL(session, formula string, defines map[string]string, l *ir.Layout) *LintOut {
	out := &LintOut{SessionID: session, Logic: "ctl", Expr: formula, Kind: "ctl", Atoms: []string{}, Undefined: []string{},
		Temporal: true, XFree: true, Notes: []string{}}
	f, err := ctl.Parse(formula, ctl.Options{Defines: defines, Env: ctl.ModelEnv(l)})
	if err != nil {
		out.TypeError = err.Error()
		out.Class = "unknown"
		out.ClassBasis = "the formula does not parse as CTL (CTL has AG/AF/AX/EG/EF/EX and A[..U..]/E[..U..]; [] and <> are LTL operators and belong to an ltl property)"
		return out
	}
	out.Normalised = ctl.Normalise(f).String()
	for _, a := range f.Atoms() {
		out.Atoms = append(out.Atoms, a.Text)
	}
	out.TypeOK = true
	out.Constant = len(out.Atoms) == 0
	if out.Constant {
		out.Notes = append(out.Notes, "the formula has no atom: it is a constant, true or false in every state — a vacuity candidate")
	}
	out.Class, out.ClassBasis = ctlClass(f)
	for _, a := range ctl.Antecedents(f) {
		out.Notes = append(out.Notes, fmt.Sprintf("the implication's antecedent %s must become true in some reachable state, else the implication holds vacuously — mc_check reports it as `vacuous` when it does not", a))
	}
	out.Notes = append(out.Notes, "CTL is decided by labelling the whole stored reachable graph; a budget that stops the graph short makes the property inconclusive, and fairness for CTL is out of scope (plan 14 §4.2)")
	return out
}

// ctlClass is a syntactic class, not a semantic one: a formula built only
// from AG/AX and boolean connectives over atoms constrains every reachable
// state (safety); one that contains EF/EU/E[..] asks for the existence of a
// run (reachability); anything with AF or A[..U..] asks that something must
// eventually happen (liveness).
func ctlClass(f *ctl.Formula) (string, string) {
	var has func(g *ctl.Formula, ops ...ctl.Op) bool
	has = func(g *ctl.Formula, ops ...ctl.Op) bool {
		if g == nil {
			return false
		}
		for _, o := range ops {
			if g.Op == o {
				return true
			}
		}
		return has(g.L, ops...) || has(g.R, ops...)
	}
	switch {
	case has(f, ctl.AF, ctl.AU):
		return "liveness", "the formula contains AF or A[..U..]: it asks that something happens on every run, which no finite prefix can establish; state the fairness assumption in an ltl property, because fairness for CTL is out of scope"
	case has(f, ctl.EF, ctl.EU, ctl.EX, ctl.EG):
		return "reachability", "the formula contains an existential path quantifier (EF, EX, EG or E[..U..]): it asks whether some run exists, which one run proves and only the whole graph refutes"
	default:
		return "safety", "the formula has only AG, AX and boolean connectives over atoms: it constrains every reachable state, and a violation is one reachable state"
	}
}

func lintLTL(session, formula string, defines map[string]string, scope ir.Scope) *LintOut {
	out := &LintOut{SessionID: session, Logic: "ltl", Expr: formula, Kind: "ltl", Atoms: []string{}, Undefined: []string{}, Temporal: true, Notes: []string{}}
	f, err := ltl.Parse(formula, ltl.Options{Defines: defines})
	if err != nil {
		out.TypeError = err.Error()
		out.Class = "unknown"
		out.ClassBasis = "the formula does not parse"
		return out
	}
	nnf := ltl.NNF(f)
	out.NNF = nnf.String()
	out.XFree = !f.HasNext()
	for _, a := range f.Atoms() {
		out.Atoms = append(out.Atoms, a.Text)
		if _, cerr := ir.Check(a.Expr, scope); cerr != nil {
			out.Undefined = append(out.Undefined, a.Text)
			out.Notes = append(out.Notes, fmt.Sprintf("atom %s: %v (process locals are not visible to properties)", a.Text, cerr))
		}
	}
	out.TypeOK = len(out.Undefined) == 0
	if !out.TypeOK {
		out.TypeError = "some atoms do not resolve against the model's globals"
	}
	out.Constant = len(out.Atoms) == 0
	if out.Constant {
		out.Notes = append(out.Notes, "the formula has no atom: it is a constant, vacuously true or false on every run — a vacuity candidate")
	}
	if !out.XFree {
		out.Notes = append(out.Notes, "the formula contains X: it is not stutter-invariant; partial-order reduction (a later version) will refuse it")
	}
	if hasLiveness(nnf) {
		out.Class = "liveness"
		out.ClassBasis = "the negation normal form contains U or <>: a violation needs an infinite run (a lasso), so the property has a liveness part; weak fairness may matter — state the fairness assumption"
	} else {
		out.Class = "safety"
		out.ClassBasis = "the negation normal form has no U and no <> (only [], V, X and boolean connectives): every violation is a finite prefix"
	}
	if kind, ok := implicationAntecedent(f); ok {
		out.Notes = append(out.Notes, kind)
	}
	return out
}

func hasLiveness(f *ltl.Formula) bool {
	if f == nil {
		return false
	}
	return f.Op == ltl.Until || f.Op == ltl.Eventually || hasLiveness(f.L) || hasLiveness(f.R)
}

// implicationAntecedent notes the vacuity risk of [](a -> …): if a never
// holds, the property is vacuously true (plan 14 §4.2, FR-011).
func implicationAntecedent(f *ltl.Formula) (string, bool) {
	if f == nil {
		return "", false
	}
	if f.Op == ltl.Impl {
		return fmt.Sprintf("the implication's antecedent %s must become true on some run, else the property holds vacuously — check it with a reach property", f.L.String()), true
	}
	if s, ok := implicationAntecedent(f.L); ok {
		return s, true
	}
	return implicationAntecedent(f.R)
}
