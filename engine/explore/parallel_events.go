package explore

import (
	"errors"
	"fmt"
	"sort"

	"modelcheck/cex"
)

// Events of the parallel search (performance plan 5, §2.7).
//
// Everything the sequential search does as a side effect of expanding a state
// (decide a property, fail the run, hit a budget) becomes an event with a total
// key that places it where the sequential breadth-first search would have done
// it:
//
//	key = (u, q, slot, sub)
//
//	u     the id of the state whose expansion it belongs to (ids ascend in the
//	      frontier order, which is the order of expansion);
//	q     a counter of the observable steps of that expansion, in the order the
//	      sequential code takes them: each call of nextEnabled, each fire and
//	      each test of whether the successor is inside an atomic sequence; the
//	      atomic intermediates continue the same counter in the order of their
//	      depth-first walk;
//	slot  what happens within one step: 0 the single outcome of the step, 1 the
//	      decision to store the successor, 2 the checks of the stored successor;
//	sub   the order of the checks of one state, as checkState evaluates them: the
//	      watch expressions, the invariants, the reach conditions.
//
// A deadlock of a state has q = the largest value (it follows the last step),
// and the checks of the initial state, which belong to no expansion, are
// merged by themselves before the first group.
//
// The events of a group are sorted by key and applied through the same decide,
// fail and budget code the sequential search uses, and the first event with a
// terminating effect drops every later one: a truncated run never claims a
// violation that lies after its stop point in this order.

// parKey is the total order of the events.
type parKey struct {
	u    uint32
	q    uint32
	slot uint8
	sub  uint32
}

// parQEnd is the q of a deadlock: after every step of the state.
const parQEnd = ^uint32(0)

func (a parKey) less(b parKey) bool {
	if a.u != b.u {
		return a.u < b.u
	}
	if a.q != b.q {
		return a.q < b.q
	}
	if a.slot != b.slot {
		return a.slot < b.slot
	}
	return a.sub < b.sub
}

type parEvKind uint8

const (
	evAssert      parEvKind = iota // a failed assert of a fire: decides every undecided assert property
	evDeadlock                     // a state without a move that is not an end state
	evError                        // an error of nextEnabled, fire or the atomic test: the model is invalid
	evPool                         // a run that finds no dormant instance: a bound, not an error
	evAtomic                       // an atomic sequence longer than dstepLimit: a bound
	evCapacity                     // a partition at capacity: a bound
	evWatchErr                     // an evaluation error of a watch expression on a new state
	evStateBudget                  // the state budget cannot store another state
	evInvariant                    // an invariant false on a new state (prop)
	evReach                        // a reach condition true on a new state (prop)
	evCheckErr                     // an evaluation error of an invariant or reach condition (prop)
	nParEvKinds
)

// parEvent is one event. It carries what the application needs to build the
// run that shows it.
type parEvent struct {
	key  parKey
	kind parEvKind
	prop int // index of the property in the outcomes (evInvariant, evReach, evCheckErr)

	// A trace is the run to the stored state from (and the moves chain from it,
	// ending in final when final is not nil), or the run to the stored state
	// id.
	from  uint32
	chain []cex.Ref
	final []byte
	id    uint32
	byID  bool

	text string // the error message, or the text of the failed assert
}

// class is the retention class of an event: a worker keeps one event of each
// class, the one with the smallest key. A class is a kind, and for the checks
// of a property the property: of the decisions of one property only the first
// can apply, and of its errors either the first applies and ends the run or it
// comes after the decision and every later one is dropped.
func (e *parEvent) class() int { return parClass(e.kind, e.prop) }

func parClass(k parEvKind, prop int) int {
	switch k {
	case evInvariant, evReach:
		return int(nParEvKinds) + 2*prop
	case evCheckErr:
		return int(nParEvKinds) + 2*prop + 1
	}
	return int(k)
}

// wants reports whether an event of the class with the key would be kept by the
// worker, so that a caller does not build an event it would drop. A model that
// fails an assert on every transition makes one comparison per transition.
func (w *parWorker) wants(cls int, key parKey) bool {
	cur := w.evs[cls]
	return cur == nil || key.less(cur.key)
}

func (w *parWorker) keep(e *parEvent) {
	c := e.class()
	if w.evs[c] == nil {
		w.nEv++
	}
	w.evs[c] = e
}

// errorEvent keeps the event of an error of the expansion of u at step q. The
// moves of the run to the error are the chain of the walk, plus the move m being
// fired when with is set, and final is the state the failed step left (nil: the
// error came before any move of this step). A pool exhaustion is a bound and
// everything else a model error, as handleErr tells them apart.
func (w *parWorker) errorEvent(u, q uint32, err error, m move, with bool, final []byte) {
	kind := evError
	var pe *poolExhausted
	if errors.As(err, &pe) {
		kind = evPool
	}
	key := parKey{u: u, q: q}
	if !w.wants(int(kind), key) {
		return
	}
	var f []byte
	if final != nil {
		f = append([]byte(nil), final...)
	}
	w.keep(&parEvent{key: key, kind: kind, from: u, chain: w.copyChain(m, with), final: f, text: err.Error()})
}

// checkNew evaluates the watch expressions, the invariants and the reach
// conditions on a state that was just stored, in the order checkState does, and
// keeps the events (for a property that was undecided when the group began). u
// and q are the step that produced the state.
//
// A failing evaluation of a watch expression ends the run whatever else the
// state holds (the sequential checkState returns at it, and it is never
// skipped), so the checks after it are dropped with the rest of the stream and
// the walk stops there. A failing evaluation of an invariant or a reach
// condition is not like that: the property may have been decided on an earlier
// state of the same group, which this worker cannot know, and then the sequential
// search never evaluates it, so the failure is dropped when the events are
// applied and the state still has to be checked against every property after it.
// The walk goes on to the next property, which is what the sequential search
// does with a property it skips. If the failure is the one that applies, it
// ends the run and drops the events of this state that come after it.
func (w *parWorker) checkNew(id uint32, vec []byte, u, q uint32) {
	r := w.r
	if !r.checking {
		return
	}
	c := w.s.c
	c.layout.Timeout = false
	sub := uint32(0)
	for i, wc := range w.watched {
		ok, err := wc.Truth(vec)
		if err != nil {
			w.checkError(evWatchErr, -1, parKey{u, q, 2, sub}, id, err)
			return
		}
		if ok {
			w.cov[i][0] = true
		} else {
			w.cov[i][1] = true
		}
		sub++
	}
	for _, ip := range c.invs {
		k := parKey{u, q, 2, sub}
		sub++
		if r.decided[ip.i] {
			continue
		}
		ok, err := ip.expr.Truth(vec)
		if err != nil {
			w.checkError(evCheckErr, ip.i, k, id, err)
			continue // the next property, not the end of the state: see above
		}
		if !ok && w.wants(parClass(evInvariant, ip.i), k) {
			w.keep(&parEvent{key: k, kind: evInvariant, prop: ip.i, id: id, byID: true})
		}
	}
	for _, rp := range c.reaches {
		k := parKey{u, q, 2, sub}
		sub++
		if r.decided[rp.i] {
			continue
		}
		ok, err := rp.expr.Truth(vec)
		if err != nil {
			w.checkError(evCheckErr, rp.i, k, id, err)
			continue // the next property, not the end of the state: see above
		}
		if ok && w.wants(parClass(evReach, rp.i), k) {
			w.keep(&parEvent{key: k, kind: evReach, prop: rp.i, id: id, byID: true})
		}
	}
}

func (w *parWorker) checkError(kind parEvKind, prop int, key parKey, id uint32, err error) {
	if w.wants(parClass(kind, prop), key) {
		w.keep(&parEvent{key: key, kind: kind, prop: prop, id: id, byID: true, text: err.Error()})
	}
}

// refreshDecided takes the statuses at the start of a group: the properties
// that are decided are not evaluated again (the sequential checkState skips
// them, so an evaluation error in the expression of a decided property never
// surfaces), and an assert or deadlock that no property is open for makes no
// event.
func (r *parRun) refreshDecided() {
	res := r.s.res
	if r.decided == nil {
		r.decided = make([]bool, len(res.Outcomes))
	}
	for i := range res.Outcomes {
		r.decided[i] = res.Outcomes[i].Status != ""
	}
	open := func(idx []int) bool {
		for _, i := range idx {
			if !r.decided[i] {
				return true
			}
		}
		return false
	}
	c := r.s.c
	r.openAssert, r.openDeadlock = open(c.asserts), open(c.deadlock)
	r.checking = len(r.workers[0].watched) > 0
	for _, ip := range c.invs {
		r.checking = r.checking || !r.decided[ip.i]
	}
	for _, rp := range c.reaches {
		r.checking = r.checking || !r.decided[rp.i]
	}
}

// mergeEvents sorts the events the workers kept, applies them in key order
// through decide, fail and budget, and stops at the first one that ends the
// search.
func (r *parRun) mergeEvents() error {
	var evs []*parEvent
	for _, w := range r.workers[:r.nActive] {
		if w.nEv == 0 {
			continue
		}
		for i, e := range w.evs {
			if e != nil {
				evs = append(evs, e)
				w.evs[i] = nil
			}
		}
		w.nEv = 0
	}
	if len(evs) == 0 {
		return nil
	}
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].key != evs[j].key {
			return evs[i].key.less(evs[j].key)
		}
		return evs[i].kind < evs[j].kind
	})
	return r.applyEvents(evs)
}

// applyEvents applies sorted events; every event after the one that stopped the
// search is dropped.
func (r *parRun) applyEvents(evs []*parEvent) error {
	s := r.s
	for _, e := range evs {
		if s.stop != "" {
			break
		}
		if err := r.apply(e); err != nil {
			return err
		}
	}
	return nil
}

func (r *parRun) apply(e *parEvent) error {
	s := r.s
	var tr *cex.Trace
	trace := func() (*cex.Trace, error) {
		if tr != nil {
			return tr, nil
		}
		var err error
		if e.byID {
			tr, err = r.traceTo(e.id)
		} else {
			tr, err = r.traceFrom(e.from, e.chain, e.final)
		}
		return tr, err
	}
	switch e.kind {
	case evAssert:
		for _, i := range s.c.asserts {
			if s.res.Outcomes[i].Status == "" {
				t, err := trace()
				if err != nil {
					return err
				}
				s.decide(i, Violated, Exhaustive, "assert("+e.text+") fails in the last step of the counterexample", t)
			}
		}
	case evDeadlock:
		for _, i := range s.c.deadlock {
			if s.res.Outcomes[i].Status == "" {
				t, err := trace()
				if err != nil {
					return err
				}
				s.decide(i, Violated, Exhaustive, "deadlock: no transition is enabled and not every process is terminated (at an end label or without outgoing edges)", t)
			}
		}
	case evInvariant:
		if s.res.Outcomes[e.prop].Status == "" {
			t, err := trace()
			if err != nil {
				return err
			}
			s.decide(e.prop, Violated, Exhaustive, "invariant "+r.exprText(e.prop)+" is false in the final state of the counterexample", t)
		}
	case evReach:
		if s.res.Outcomes[e.prop].Status == "" {
			t, err := trace()
			if err != nil {
				return err
			}
			s.decide(e.prop, Verified, Exhaustive, "a reachable state satisfies "+r.exprText(e.prop)+"; see the witness", t)
		}
	case evCheckErr, evWatchErr:
		if e.kind == evCheckErr && s.res.Outcomes[e.prop].Status != "" {
			return nil // decided earlier in the key order: its expression is not evaluated again
		}
		t, err := trace()
		if err != nil {
			return err
		}
		s.fail(e.text, t)
	case evError:
		t, err := trace()
		if err != nil {
			return err
		}
		s.fail(e.text, t)
	case evPool:
		s.budget(e.text)
	case evAtomic:
		s.budget(fmt.Sprintf("depth budget exhausted: an atomic sequence exceeds %d steps", dstepLimit))
	case evCapacity, evStateBudget:
		s.budget(e.text)
	}
	return nil
}

// exprText is the text of the expression of invariant or reach property i, as
// the sequential search prints it.
func (r *parRun) exprText(i int) string {
	for _, ip := range r.s.c.invs {
		if ip.i == i {
			return ip.expr.String()
		}
	}
	for _, rp := range r.s.c.reaches {
		if rp.i == i {
			return rp.expr.String()
		}
	}
	return ""
}
