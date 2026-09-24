package explore

import (
	"errors"
	"fmt"

	"modelcheck/cex"
	"modelcheck/ir"
)

// Stepper exposes the explorer's successor relation one move at a time, for
// simulation (plan 14 §6, mc_simulate). It reuses the compiled model and the
// same enabledness and firing code as Run (nextEnabled, fire), so a
// simulated run is a run of the model exactly as the search sees it: same
// order of enabled moves ("process, then edge", exclusive holder first,
// timeout phase last), same rendezvous pairing, same d_step continuation,
// same domain checks.
type Stepper struct {
	s *search
}

// EdgeRef identifies an edge: process index and edge index in the IR.
type EdgeRef struct {
	Proc int
	Edge int
}

// Move is one step of the system: an edge and, for a rendezvous handshake,
// the receiving edge taken in the same step.
type Move struct {
	Edge    EdgeRef
	Partner *EdgeRef
}

// Ref converts the move to the counterexample builder's reference.
func (m Move) Ref() cex.Ref {
	r := cex.Ref{Proc: m.Edge.Proc, Edge: m.Edge.Edge}
	if m.Partner != nil {
		r.HasPartner, r.PartnerProc, r.PartnerEdge = true, m.Partner.Proc, m.Partner.Edge
	}
	return r
}

// ErrNotEnabled is returned by Apply when the move is not among the enabled
// moves of the given state.
var ErrNotEnabled = errors.New("move is not enabled in this state")

// NewStepper compiles m. The error is the compiler's explanation (an IR that
// validated but cannot be compiled).
func NewStepper(m *ir.Model) (*Stepper, error) {
	c, err := compile(m)
	if err != nil {
		return nil, err
	}
	s := &search{c: c, cur: make([]byte, c.layout.Size), next: make([]byte, c.layout.Size)}
	return &Stepper{s: s}, nil
}

// Layout is the state layout of the compiled model.
func (st *Stepper) Layout() *ir.Layout { return st.s.c.layout }

// Initial returns a fresh copy of the initial state.
func (st *Stepper) Initial() []byte { return st.s.c.layout.Initial() }

// Edge returns the IR edge behind ref.
func (st *Stepper) Edge(ref EdgeRef) *ir.Edge {
	return st.s.c.procs[ref.Proc].edge[ref.Edge].e
}

func toMove(m move) Move {
	out := Move{Edge: EdgeRef{Proc: m.e.proc, Edge: m.e.idx}}
	if m.partner != nil {
		out.Partner = &EdgeRef{Proc: m.partner.proc, Edge: m.partner.idx}
	}
	return out
}

// Enabled lists the moves enabled in state, in the explorer's order. An
// evaluation error (division by zero, index out of range in a guard) is
// returned as the reason the model is invalid.
func (st *Stepper) Enabled(state []byte) ([]Move, error) {
	f := frame{proc: -1}
	var out []Move
	for {
		m, ok, err := st.s.nextEnabled(&f, state)
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		out = append(out, toMove(m))
	}
}

// Terminated reports whether every process is at an end location or at a
// location without outgoing edges — the second half of the deadlock rule: a
// state with no enabled move is a deadlock only if Terminated is false.
func (st *Stepper) Terminated(state []byte) bool { return st.s.allTerminated(state) }

// Apply returns the successor of state via mv. It returns ErrNotEnabled if
// mv is not enabled (this includes exclusive control: while a process holds
// it and can move, no other process's move is enabled), or the firing error
// (domain overflow, bad index, blocked d_step) that makes the model invalid.
// failed is the edge whose assert was false during the step, if any: the
// successor is still returned, and the step is the last one of a violating
// run.
func (st *Stepper) Apply(state []byte, mv Move) (next []byte, failed *ir.Edge, err error) {
	f := frame{proc: -1}
	var chosen *move
	for {
		m, ok, e := st.s.nextEnabled(&f, state)
		if e != nil {
			return nil, nil, e
		}
		if !ok {
			break
		}
		if cand := toMove(m); cand.Edge == mv.Edge && samePartner(cand.Partner, mv.Partner) {
			mm := m
			chosen = &mm
			break
		}
	}
	if chosen == nil {
		if mv.Edge.Proc < 0 || mv.Edge.Proc >= len(st.s.c.procs) || mv.Edge.Edge < 0 || mv.Edge.Edge >= len(st.s.c.procs[mv.Edge.Proc].edge) {
			return nil, nil, fmt.Errorf("edge %d/%d does not exist", mv.Edge.Proc, mv.Edge.Edge)
		}
		return nil, nil, ErrNotEnabled
	}
	copy(st.s.cur, state)
	fe, err := st.s.fire(*chosen)
	if err != nil {
		return nil, nil, err
	}
	if fe != nil {
		failed = fe.e
	}
	return append([]byte(nil), st.s.next...), failed, nil
}

func samePartner(a, b *EdgeRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
