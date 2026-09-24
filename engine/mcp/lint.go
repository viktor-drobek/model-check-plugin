package mcp

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/ir"
)

// LintIn is the input of mc_lint_property. In G2 the expression language is
// the IR's boolean state expressions, used by invariant and reach; temporal
// formulas (ltl, ctl) are linted from G4/G5.
type LintIn struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"session holding the parsed model; omitted = a new session (then ir is required)"`
	IR        any    `json:"ir,omitempty" jsonschema:"IR JSON; omitted = the session's parsed model"`
	Expr      any    `json:"expr" jsonschema:"boolean state expression: IR expression JSON or a bare variable name"`
	Kind      string `json:"kind,omitempty" jsonschema:"invariant | reach (default invariant); decides the class"`
}

// LintOut is the answer of mc_lint_property.
type LintOut struct {
	SessionID  string   `json:"session_id"`
	Expr       string   `json:"expr" jsonschema:"the expression as the engine reads it"`
	Kind       string   `json:"kind"`
	Atoms      []string `json:"atoms" jsonschema:"variables the expression reads, in order of first occurrence"`
	Undefined  []string `json:"undefined" jsonschema:"atoms that are not global variables of the model"`
	TypeOK     bool     `json:"type_ok"`
	TypeError  string   `json:"type_error,omitempty"`
	Class      string   `json:"class" jsonschema:"safety | reachability | liveness | unknown"`
	ClassBasis string   `json:"class_basis"`
	XFree      bool     `json:"x_free" jsonschema:"true: no next-state operator (state expressions have none)"`
	Temporal   bool     `json:"temporal" jsonschema:"false: a state expression has no temporal operators"`
	Constant   bool     `json:"constant" jsonschema:"true when the expression reads no variable: it is vacuously true or false everywhere"`
	Notes      []string `json:"notes"`
}

func (s *Server) lint(ctx context.Context, req *sdk.CallToolRequest, in LintIn) (*sdk.CallToolResult, *LintOut, error) {
	if in.SessionID == "" && in.IR == nil {
		return nil, nil, errors.New("no model: pass `ir` inline or a session_id whose model was parsed with mc_parse")
	}
	if in.Expr == nil {
		return nil, nil, errors.New("expr is required")
	}
	kind := in.Kind
	if kind == "" {
		kind = "invariant"
	}
	switch kind {
	case "invariant", "reach":
	case "ltl", "ctl", "progress":
		return nil, nil, fmt.Errorf("kind %s is not linted in G2: temporal formulas are linted from G4 (ltl, progress) and G5 (ctl); this tool accepts invariant and reach", kind)
	default:
		return nil, nil, fmt.Errorf("kind must be invariant or reach, got %q", kind)
	}
	e, err := exprFrom(in.Expr)
	if err != nil {
		return nil, nil, err
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
