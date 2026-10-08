package explore

import (
	"bytes"
	"fmt"

	"modelcheck/cex"
)

// Counterexamples of the parallel search (performance plan 5, §2.7). A state
// stores one parent id; a run is rebuilt after the fact by following the parents
// to the initial state and re-expanding each parent single-threaded to find
// which move, or atomic sequence, led to the child.

// traceTo is the run to stored state id: the parent links give the stored
// states on the way and each hop is re-derived.
func (r *parRun) traceTo(id uint32) (*cex.Trace, error) {
	states, refs, err := r.pathTo(id)
	if err != nil {
		return nil, err
	}
	return cex.Build(r.s.c.layout, states, refs), nil
}

// traceFrom is the run to stored state from, extended by the moves of chain
// (the atomic sequence and the step of an event); when final is not nil it is
// the state after the last move, which a failed step does not leave in a state
// that can be fired again.
func (r *parRun) traceFrom(from uint32, chain []cex.Ref, final []byte) (*cex.Trace, error) {
	states, refs, err := r.pathTo(from)
	if err != nil {
		return nil, err
	}
	s := r.workers[0].s
	for k, ref := range chain {
		refs = append(refs, ref)
		if k == len(chain)-1 && final != nil {
			states = append(states, append([]byte(nil), final...))
			break
		}
		next, err := r.fireRef(states[len(states)-1], ref)
		if err != nil {
			return nil, err
		}
		states = append(states, next)
	}
	return cex.Build(s.c.layout, states, refs), nil
}

// fireRef is the state after the move ref from state.
func (r *parRun) fireRef(state []byte, ref cex.Ref) ([]byte, error) {
	s := r.workers[0].s
	m := move{e: &s.c.procs[ref.Proc].edge[ref.Edge]}
	if ref.HasPartner {
		m.partner = &s.c.procs[ref.PartnerProc].edge[ref.PartnerEdge]
	}
	copy(s.cur, state)
	if _, err := s.fire(m); err != nil {
		return nil, &InternalError{Msg: fmt.Sprintf("replaying a move of a counterexample failed: %v", err)}
	}
	return append([]byte(nil), s.next...), nil
}

// pathTo returns the stored states on the run to id, the initial state first,
// and the moves between them (including the intermediate states of an atomic
// sequence, which are not stored and are recovered by firing).
func (r *parRun) pathTo(id uint32) ([][]byte, []cex.Ref, error) {
	var ids []uint32
	for x := id; x != parNoParent; x = r.set.parentOf(x) {
		ids = append(ids, x)
	}
	initial := r.set.get(ids[len(ids)-1])
	if !bytes.Equal(initial, r.s.c.layout.Initial()) {
		return nil, nil, &InternalError{Msg: "a run does not start at the initial state"}
	}
	states := [][]byte{append([]byte(nil), initial...)}
	var refs []cex.Ref
	w := r.workers[0]
	for k := len(ids) - 2; k >= 0; k-- {
		chain, err := w.deriveChain(ids[k+1], ids[k])
		if err != nil {
			return nil, nil, err
		}
		for _, ref := range chain {
			next, err := r.fireRef(states[len(states)-1], ref)
			if err != nil {
				return nil, nil, err
			}
			refs = append(refs, ref)
			states = append(states, next)
		}
		if !bytes.Equal(states[len(states)-1], r.set.get(ids[k])) {
			return nil, nil, &InternalError{Msg: "the replay of a hop does not reach the stored state"}
		}
	}
	return states, refs, nil
}

// deriveChain finds how stored state parent led to stored state child: the
// first move, or atomic sequence, in the order of the search whose stored
// successor is child. That is the record that won the insertion, so the path
// is the one the search took.
func (w *parWorker) deriveChain(parent, child uint32) ([]cex.Ref, error) {
	var sink parCounts
	w.mode, w.cnt, w.target, w.found = pmFind, &sink, w.r.set.get(child), nil
	w.walk(parent)
	chain := w.found
	w.target, w.found = nil, nil
	if chain == nil {
		return nil, &InternalError{Msg: fmt.Sprintf("state %#x is not a successor of its parent %#x", child, parent)}
	}
	return chain, nil
}
