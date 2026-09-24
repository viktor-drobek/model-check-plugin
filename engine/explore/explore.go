// Package explore is the explicit-state explorer over the IR (plan 14 §4.1):
// flat byte-vector states, a compact visited set, deterministic transition
// order (process index, then edge index), DFS with an explicit stack of lazy
// successor iterators, BFS for shortest witnesses, budgets, and the
// properties of the MVP: deadlock, invariants, edge asserts, reachability,
// with domain overflow reported as invalid-model.
//
// # Deadlock
//
// A state is a deadlock when no transition is enabled in it and not every
// process is terminated. A process is terminated when its control location
// carries the `end` label or has no outgoing edge. This is the definition
// the feature file and the report use. For a Petri net encoded as one
// looping process it is Holzmann's *hang*. It is meant to coincide with
// SPIN's "invalid end state" on the Promela subset; that coincidence is a
// claim to be checked differentially in G1, not a fact established here.
//
// # Statuses
//
// Every property ends in exactly one status of 11 §14:
//
//	violated       a counterexample was found (exact run → evidence
//	               exhaustive); for reach: a complete search found no
//	               satisfying state (exhaustive because complete)
//	verified       undecided after a complete search (evidence exhaustive);
//	               for reach: a satisfying state was found (witness, exact run)
//	inconclusive   undecided and a budget stopped the search (bounded)
//	invalid-model  undecided when the model misbehaved: domain overflow,
//	               index out of range, division by zero (evidence unknown;
//	               the trace to the offending step is attached)
//	not-executed   the property kind is not executed by this version
//	unknown        never produced by this package
//
// For `reach` the roles of verified and violated follow the property's
// statement "some reachable state satisfies E": finding one verifies it
// (with a witness), a complete search without one violates it.
// A verdict decided before the search stopped (violated, or reach verified)
// stands whatever stopped the search afterwards — a budget or an invalid
// step elsewhere — because its witness is an exact run from the initial
// state that itself contains no invalid step (had it contained one, the run
// would have ended there as invalid-model).
package explore

import (
	"context"
	"fmt"
	"time"

	"modelcheck/cex"
	"modelcheck/ir"
)

// Mode selects the search order.
type Mode string

const (
	DFS Mode = "dfs"
	BFS Mode = "bfs"
)

// Budget bounds a run. Zero means unlimited. Wall time comes from the
// context deadline.
type Budget struct {
	MaxStates int
	// MaxDepth bounds the depth (transitions from the initial state) of the
	// states that are expanded: a successor at depth MaxDepth+1 is stored
	// but not expanded, in both DFS and BFS.
	MaxDepth    int
	MaxMemBytes int64
}

// Options configures a run.
type Options struct {
	Mode   Mode
	Budget Budget
	// NewVisited overrides the visited set (tests use the map reference).
	NewVisited func(stateLen int) Visited
}

// Status and Evidence values (11 §14).
type Status string

const (
	Verified     Status = "verified"
	Violated     Status = "violated"
	Inconclusive Status = "inconclusive"
	Unknown      Status = "unknown"
	NotExecuted  Status = "not-executed"
	InvalidModel Status = "invalid-model"
)

type Evidence string

const (
	Exhaustive  Evidence = "exhaustive"
	Bounded     Evidence = "bounded"
	Approximate Evidence = "approximate"
	EvUnknown   Evidence = "unknown"
)

// Outcome is the result for one property.
type Outcome struct {
	Property ir.Property
	Status   Status
	Evidence Evidence
	Reason   string
	// Trace is the counterexample (violated), the witness (reach verified)
	// or the run to the offending step (invalid-model).
	Trace *cex.Trace
}

// Result is the outcome of one run.
type Result struct {
	Outcomes    []Outcome
	States      int
	Transitions int
	// MaxDepth is the greatest depth of an expanded state (never above
	// Budget.MaxDepth when that is set).
	MaxDepth   int
	MemBytes   int64
	Elapsed    time.Duration
	StateBytes int
	// Complete is true only when the whole reachable graph was expanded.
	Complete bool
	// Stop says why the search ended: "complete", "all properties decided",
	// "invalid model", or the budget reason.
	Stop string
}

// Run explores m from its initial state.
func Run(ctx context.Context, m *ir.Model, opt Options) (*Result, error) {
	start := time.Now()
	c, err := compile(m)
	if err != nil {
		return nil, err
	}
	if opt.Mode == "" {
		opt.Mode = DFS
	}
	newVisited := opt.NewVisited
	if newVisited == nil {
		newVisited = func(n int) Visited { return NewCompact(n, 1024) }
	}
	s := &search{
		c:       c,
		opt:     opt,
		ctx:     ctx,
		visited: newVisited(c.layout.Size),
		res:     &Result{StateBytes: c.layout.Size},
	}
	s.initOutcomes()
	if opt.Mode == BFS {
		s.bfs()
	} else {
		s.dfs()
	}
	s.finish()
	s.res.Elapsed = time.Since(start)
	return s.res, nil
}

// ---- compiled model -------------------------------------------------------

type cAssign struct {
	slot  *ir.Slot
	index *ir.Compiled // nil for scalars
	value *ir.Compiled
}

type cEdge struct {
	proc, idx int
	e         *ir.Edge
	guard     *ir.Compiled
	assert    *ir.Compiled
	effect    []cAssign
}

type cProc struct {
	out  [][]int // per location: edge indices, ascending
	end  []bool  // per location: carries the end label
	edge []cEdge
}

type cProp struct {
	i    int // index into outcomes
	expr *ir.Compiled
}

type compiled struct {
	m        *ir.Model
	layout   *ir.Layout
	procs    []cProc
	hasAsrt  bool
	props    []ir.Property
	invs     []cProp
	reaches  []cProp
	deadlock []int
	asserts  []int
}

func compile(m *ir.Model) (*compiled, error) {
	l, err := ir.NewLayout(m)
	if err != nil {
		return nil, err
	}
	c := &compiled{m: m, layout: l}
	for p := range m.Processes {
		pr := &m.Processes[p]
		cp := cProc{out: make([][]int, len(pr.Locations)), end: make([]bool, len(pr.Locations))}
		for i, loc := range pr.Locations {
			for _, lb := range loc.Labels {
				if lb == ir.End {
					cp.end[i] = true
				}
			}
		}
		for i := range pr.Edges {
			e := &pr.Edges[i]
			ce := cEdge{proc: p, idx: i, e: e}
			if ce.guard, err = l.Compile(e.Guard, p); err != nil {
				return nil, fmt.Errorf("%s edge %d guard: %w", pr.Name, i, err)
			}
			if ce.assert, err = l.Compile(e.Assert, p); err != nil {
				return nil, fmt.Errorf("%s edge %d assert: %w", pr.Name, i, err)
			}
			if e.Assert != nil {
				c.hasAsrt = true
			}
			for _, a := range e.Effect {
				ca := cAssign{slot: l.Resolve(a.Var, p)}
				if ca.index, err = l.Compile(a.Index, p); err != nil {
					return nil, err
				}
				if ca.value, err = l.Compile(a.Value, p); err != nil {
					return nil, err
				}
				ce.effect = append(ce.effect, ca)
			}
			cp.edge = append(cp.edge, ce)
			cp.out[e.From] = append(cp.out[e.From], i)
		}
		c.procs = append(c.procs, cp)
	}
	c.props = append(c.props, m.Properties...)
	if c.hasAsrt {
		found := false
		for _, p := range c.props {
			if p.Kind == ir.KindAssert {
				found = true
			}
		}
		if !found {
			// Edge asserts are never checked silently: give them a property.
			c.props = append(c.props, ir.Property{ID: "assert", Kind: ir.KindAssert, Text: "no assert statement fails"})
		}
	}
	for i, p := range c.props {
		switch p.Kind {
		case ir.KindDeadlock:
			c.deadlock = append(c.deadlock, i)
		case ir.KindAssert:
			c.asserts = append(c.asserts, i)
		case ir.KindInvariant, ir.KindReach:
			ce, err := l.Compile(p.Expr, -1)
			if err != nil {
				return nil, fmt.Errorf("property %s: %w", p.ID, err)
			}
			if p.Kind == ir.KindInvariant {
				c.invs = append(c.invs, cProp{i, ce})
			} else {
				c.reaches = append(c.reaches, cProp{i, ce})
			}
		}
	}
	return c, nil
}

// ---- search state -----------------------------------------------------------

type frame struct {
	idx      int32 // state index in visited
	proc     int32 // iterator: current process; -1 = not started
	pos      int32 // iterator: position in out[proc][pc]
	viaProc  int32 // edge that led here (-1 for the initial state)
	viaEdge  int32
	enabled  uint32
	exclOnly bool
}

type search struct {
	c       *compiled
	opt     Options
	ctx     context.Context
	visited Visited
	res     *Result

	cur, next []byte
	stack     []frame
	// BFS bookkeeping (parallel to visited indices)
	parent, viaP, viaE, depth []int32

	undecided int
	stop      string // set when the search must stop early
	budgetHit bool
	truncated int // states stored but not expanded because of MaxDepth
	invalid   bool
}

func (s *search) initOutcomes() {
	for _, p := range s.c.props {
		o := Outcome{Property: p}
		switch p.Kind {
		case ir.KindDeadlock, ir.KindInvariant, ir.KindReach, ir.KindAssert:
			s.undecided++
		default:
			o.Status = NotExecuted
			o.Evidence = EvUnknown
			o.Reason = fmt.Sprintf("property kind %q is not executed by this engine version (G0 executes deadlock, invariant, reach, assert)", p.Kind)
		}
		s.res.Outcomes = append(s.res.Outcomes, o)
	}
}

func (s *search) decide(i int, st Status, ev Evidence, reason string, tr *cex.Trace) {
	o := &s.res.Outcomes[i]
	if o.Status != "" {
		return // first verdict stands
	}
	o.Status, o.Evidence, o.Reason, o.Trace = st, ev, reason, tr
	s.undecided--
	if s.undecided == 0 && s.stop == "" {
		s.stop = "all properties decided"
	}
}

func (s *search) fail(reason string, tr *cex.Trace) {
	// The model misbehaved: every undecided property becomes invalid-model.
	s.invalid = true
	s.stop = "invalid model"
	for i := range s.res.Outcomes {
		s.decide(i, InvalidModel, EvUnknown, reason, tr)
	}
}

func (s *search) budget(reason string) {
	s.budgetHit = true
	if s.stop == "" {
		s.stop = reason
	}
}

// allTerminated reports whether every process is at an end location or at a
// location without outgoing edges (the second half of the deadlock rule).
func (s *search) allTerminated(state []byte) bool {
	for p := range s.c.procs {
		loc := s.c.layout.ReadPC(state, p)
		if s.c.procs[p].end[loc] || len(s.c.procs[p].out[loc]) == 0 {
			continue
		}
		return false
	}
	return true
}

// hasEnabled reports whether process p has an enabled edge in state.
func (s *search) hasEnabled(p int, state []byte) (bool, error) {
	loc := s.c.layout.ReadPC(state, p)
	for _, ei := range s.c.procs[p].out[loc] {
		ok, err := s.c.procs[p].edge[ei].guard.Truth(state)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// startIter positions f's iterator for state: only the exclusive holder if
// it has an enabled edge (Promela atomic), else every process from 0.
func (s *search) startIter(f *frame, state []byte) error {
	f.proc, f.pos = 0, 0
	if ex := state[s.c.layout.Excl]; ex != 0 {
		p := int(ex) - 1
		ok, err := s.hasEnabled(p, state)
		if err != nil {
			return err
		}
		if ok {
			f.proc, f.exclOnly = int32(p), true
		}
	}
	return nil
}

// nextEnabled advances f's iterator to the next enabled edge of state.
func (s *search) nextEnabled(f *frame, state []byte) (*cEdge, bool, error) {
	if f.proc < 0 {
		if err := s.startIter(f, state); err != nil {
			return nil, false, err
		}
	}
	for int(f.proc) < len(s.c.procs) {
		p := int(f.proc)
		outs := s.c.procs[p].out[s.c.layout.ReadPC(state, p)]
		if int(f.pos) >= len(outs) {
			if f.exclOnly {
				return nil, false, nil
			}
			f.proc++
			f.pos = 0
			continue
		}
		e := &s.c.procs[p].edge[outs[f.pos]]
		f.pos++
		ok, err := e.guard.Truth(state)
		if err != nil {
			return nil, false, err
		}
		if ok {
			return e, true, nil
		}
	}
	return nil, false, nil
}

// fire computes into s.next the state after e from s.cur. It returns an
// evaluation/overflow error as the reason the model is invalid.
func (s *search) fire(e *cEdge) error {
	l := s.c.layout
	copy(s.next, s.cur)
	l.WritePC(s.next, e.proc, e.e.To)
	if e.e.Atomic {
		s.next[l.Excl] = byte(e.proc + 1)
	} else {
		s.next[l.Excl] = 0
	}
	for _, a := range e.effect {
		slot := a.slot
		if a.index != nil {
			i, err := a.index.Eval(s.next)
			if err != nil {
				return err
			}
			if i < 0 || i >= int64(slot.Var.Len) {
				return fmt.Errorf("index %d out of range for %s[%d]", i, slot.Var.Name, slot.Var.Len)
			}
			slot = slot.At(int(i))
		}
		v, err := a.value.Eval(s.next)
		if err != nil {
			return err
		}
		if !slot.InDomain(v) {
			return fmt.Errorf("domain overflow: %s = %d leaves [%d, %d]%s in step %q",
				slot.Name(), v, slot.Min, slot.Max, domainNote(slot), cex.CommandText(e.e))
		}
		slot.Write(s.next, v)
	}
	return nil
}

func domainNote(sl *ir.Slot) string {
	if sl.Var.Max != nil || sl.Var.Min != nil {
		return " (declared capacity)"
	}
	return " (" + string(sl.Var.Type) + " domain)"
}

// checkState evaluates invariants and reach conditions on a newly stored
// state; path is the run that reaches it.
func (s *search) checkState(state []byte, path func() *cex.Trace) error {
	for _, ip := range s.c.invs {
		if s.res.Outcomes[ip.i].Status != "" {
			continue
		}
		ok, err := ip.expr.Truth(state)
		if err != nil {
			return err
		}
		if !ok {
			s.decide(ip.i, Violated, Exhaustive, "invariant "+ip.expr.String()+" is false in the final state of the counterexample", path())
		}
	}
	for _, rp := range s.c.reaches {
		if s.res.Outcomes[rp.i].Status != "" {
			continue
		}
		ok, err := rp.expr.Truth(state)
		if err != nil {
			return err
		}
		if ok {
			s.decide(rp.i, Verified, Exhaustive, "a reachable state satisfies "+rp.expr.String()+"; see the witness", path())
		}
	}
	return nil
}

func (s *search) deadlockAt(path func() *cex.Trace) {
	if len(s.c.deadlock) == 0 {
		return
	}
	var tr *cex.Trace
	for _, i := range s.c.deadlock {
		if s.res.Outcomes[i].Status == "" {
			if tr == nil {
				tr = path()
			}
			s.decide(i, Violated, Exhaustive, "deadlock: no transition is enabled and not every process is terminated (at an end label or without outgoing edges)", tr)
		}
	}
}

func (s *search) assertFailed(e *cEdge, path func() *cex.Trace) {
	var tr *cex.Trace
	for _, i := range s.c.asserts {
		if s.res.Outcomes[i].Status == "" {
			if tr == nil {
				tr = path()
			}
			s.decide(i, Violated, Exhaustive, "assert("+e.assert.String()+") fails in the last step of the counterexample", tr)
		}
	}
}

// checkBudgets is called after each newly stored state.
func (s *search) checkBudgets(extraBytes int64) bool {
	b := s.opt.Budget
	n := s.visited.Len()
	if n&1023 == 0 {
		if s.ctx.Err() != nil {
			s.budget("time budget exhausted")
			return false
		}
		if b.MaxMemBytes > 0 {
			est := s.visited.Bytes() + extraBytes
			if est > b.MaxMemBytes {
				s.budget(fmt.Sprintf("memory budget exhausted: estimate %d bytes exceeds %d", est, b.MaxMemBytes))
				return false
			}
		}
	}
	return true
}

// store adds s.next; it returns its index, whether it was new, and false
// as the third value when the state budget forbids storing a new state.
func (s *search) store() (int, bool, bool) {
	b := s.opt.Budget
	if b.MaxStates > 0 && s.visited.Len() >= b.MaxStates {
		if _, present := s.visited.Has(s.next); present {
			return 0, false, true
		}
		s.budget(fmt.Sprintf("state budget exhausted: %d states stored", s.visited.Len()))
		return 0, false, false
	}
	idx, isNew := s.visited.Add(s.next)
	return idx, isNew, true
}

// ---- DFS --------------------------------------------------------------------

const frameBytes = 32

func (s *search) dfs() {
	l := s.c.layout
	init := l.Initial()
	s.cur = make([]byte, l.Size)
	s.next = make([]byte, l.Size)
	idx0, _ := s.visited.Add(init)
	s.stack = append(s.stack, frame{idx: int32(idx0), proc: -1, viaProc: -1, viaEdge: -1})
	copy(s.cur, init)
	curIdx := idx0
	s.res.MaxDepth = 0

	pathToTop := func() *cex.Trace { return s.dfsPath(nil, nil) }
	if err := s.checkState(s.cur, pathToTop); err != nil {
		s.fail(err.Error(), pathToTop())
	}

	for len(s.stack) > 0 && s.stop == "" {
		top := &s.stack[len(s.stack)-1]
		if int(top.idx) != curIdx {
			copy(s.cur, s.visited.Get(int(top.idx)))
			curIdx = int(top.idx)
		}
		e, ok, err := s.nextEnabled(top, s.cur)
		if err != nil {
			s.fail(err.Error(), pathToTop())
			break
		}
		if !ok {
			if top.enabled == 0 && !s.allTerminated(s.cur) {
				s.deadlockAt(pathToTop)
			}
			s.stack = s.stack[:len(s.stack)-1]
			continue
		}
		top.enabled++
		s.res.Transitions++
		// Guard held: compute the successor (effect), then evaluate the
		// assert in the source state. A failed assert's witness ends with
		// this step and shows the successor as its final state.
		if err := s.fire(e); err != nil {
			s.fail(err.Error(), s.dfsPath(e, s.next))
			break
		}
		if e.assert != nil {
			ok, err := e.assert.Truth(s.cur)
			if err != nil {
				s.fail(err.Error(), pathToTop())
				break
			}
			if !ok {
				s.assertFailed(e, func() *cex.Trace { return s.dfsPath(e, s.next) })
				if s.stop != "" {
					break
				}
			}
		}
		idx, isNew, allowed := s.store()
		if !allowed {
			break
		}
		if !isNew {
			continue
		}
		s.res.States = s.visited.Len()
		pathToNext := func() *cex.Trace { return s.dfsPath(e, s.next) }
		if err := s.checkState(s.next, pathToNext); err != nil {
			s.fail(err.Error(), pathToNext())
			break
		}
		if s.stop != "" {
			break
		}
		if !s.checkBudgets(int64(len(s.stack)) * frameBytes) {
			break
		}
		depth := len(s.stack) // transitions from the initial state to next
		if s.opt.Budget.MaxDepth > 0 && depth > s.opt.Budget.MaxDepth {
			s.truncated++
			continue
		}
		if depth > s.res.MaxDepth {
			s.res.MaxDepth = depth
		}
		s.stack = append(s.stack, frame{idx: int32(idx), proc: -1, viaProc: int32(e.proc), viaEdge: int32(e.idx)})
	}
	s.res.States = s.visited.Len()
	s.res.MemBytes = s.visited.Bytes() + int64(cap(s.stack))*frameBytes
	if s.stop == "" && len(s.stack) == 0 {
		s.stop = "complete"
	}
}

// dfsPath renders the run along the stack, optionally extended by one more
// edge into state last.
func (s *search) dfsPath(extra *cEdge, last []byte) *cex.Trace {
	var states [][]byte
	var refs []cex.Ref
	for i, f := range s.stack {
		states = append(states, append([]byte(nil), s.visited.Get(int(f.idx))...))
		if i > 0 {
			refs = append(refs, cex.Ref{Proc: int(f.viaProc), Edge: int(f.viaEdge)})
		}
	}
	if extra != nil {
		states = append(states, append([]byte(nil), last...))
		refs = append(refs, cex.Ref{Proc: extra.proc, Edge: extra.idx})
	}
	return cex.Build(s.c.layout, states, refs)
}

// ---- BFS --------------------------------------------------------------------

const bfsBytesPerState = 16

func (s *search) bfs() {
	l := s.c.layout
	init := l.Initial()
	s.cur = make([]byte, l.Size)
	s.next = make([]byte, l.Size)
	s.visited.Add(init)
	s.parent = append(s.parent, -1)
	s.viaP = append(s.viaP, -1)
	s.viaE = append(s.viaE, -1)
	s.depth = append(s.depth, 0)

	pathTo := func(idx int) func() *cex.Trace { return func() *cex.Trace { return s.bfsPath(idx, nil, nil) } }
	if err := s.checkState(init, pathTo(0)); err != nil {
		s.fail(err.Error(), pathTo(0)())
	}

	head := 0
	for head < s.visited.Len() && s.stop == "" {
		copy(s.cur, s.visited.Get(head))
		d := int(s.depth[head])
		if s.opt.Budget.MaxDepth > 0 && d > s.opt.Budget.MaxDepth {
			s.truncated++ // stored (by its parent) but not expanded, as in DFS
			head++
			continue
		}
		if d > s.res.MaxDepth {
			s.res.MaxDepth = d
		}
		f := frame{proc: -1}
		for s.stop == "" {
			e, ok, err := s.nextEnabled(&f, s.cur)
			if err != nil {
				s.fail(err.Error(), pathTo(head)())
				break
			}
			if !ok {
				if f.enabled == 0 && !s.allTerminated(s.cur) {
					s.deadlockAt(pathTo(head))
				}
				break
			}
			f.enabled++
			s.res.Transitions++
			if err := s.fire(e); err != nil {
				s.fail(err.Error(), s.bfsPath(head, e, s.next))
				break
			}
			if e.assert != nil {
				ok, err := e.assert.Truth(s.cur)
				if err != nil {
					s.fail(err.Error(), pathTo(head)())
					break
				}
				if !ok {
					s.assertFailed(e, func() *cex.Trace { return s.bfsPath(head, e, s.next) })
					if s.stop != "" {
						break
					}
				}
			}
			idx, isNew, allowed := s.store()
			if !allowed {
				break
			}
			if !isNew {
				continue
			}
			s.parent = append(s.parent, int32(head))
			s.viaP = append(s.viaP, int32(e.proc))
			s.viaE = append(s.viaE, int32(e.idx))
			s.depth = append(s.depth, int32(d+1))
			s.res.States = s.visited.Len()
			if err := s.checkState(s.next, pathTo(idx)); err != nil {
				s.fail(err.Error(), pathTo(idx)())
				break
			}
			if !s.checkBudgets(int64(s.visited.Len()) * bfsBytesPerState) {
				break
			}
		}
		head++
	}
	s.res.States = s.visited.Len()
	s.res.MemBytes = s.visited.Bytes() + int64(cap(s.parent))*bfsBytesPerState
	if s.stop == "" && head >= s.visited.Len() {
		s.stop = "complete"
	}
}

// bfsPath renders the shortest run to state idx (via parent links),
// optionally extended by one edge into last.
func (s *search) bfsPath(idx int, extra *cEdge, last []byte) *cex.Trace {
	var chain []int
	for i := idx; i >= 0; i = int(s.parent[i]) {
		chain = append(chain, i)
	}
	var states [][]byte
	var refs []cex.Ref
	for k := len(chain) - 1; k >= 0; k-- {
		i := chain[k]
		states = append(states, append([]byte(nil), s.visited.Get(i)...))
		if k < len(chain)-1 {
			refs = append(refs, cex.Ref{Proc: int(s.viaP[i]), Edge: int(s.viaE[i])})
		}
	}
	if extra != nil {
		states = append(states, append([]byte(nil), last...))
		refs = append(refs, cex.Ref{Proc: extra.proc, Edge: extra.idx})
	}
	return cex.Build(s.c.layout, states, refs)
}

// ---- finalisation -------------------------------------------------------------

func (s *search) finish() {
	r := s.res
	r.Complete = s.stop == "complete" && !s.budgetHit && s.truncated == 0 && !s.invalid
	if s.truncated > 0 && !s.invalid {
		reason := fmt.Sprintf("depth budget exhausted: %d state(s) at depth > %d were stored but not expanded", s.truncated, s.opt.Budget.MaxDepth)
		s.budgetHit = true
		if s.stop == "complete" || s.stop == "" {
			s.stop = reason
		}
	}
	r.Stop = s.stop
	for i := range r.Outcomes {
		o := &r.Outcomes[i]
		if o.Status != "" {
			continue
		}
		// Undecided after the search: the verdict depends only on whether
		// the search was complete. (invalid-model was assigned in fail.)
		if r.Complete {
			switch o.Property.Kind {
			case ir.KindReach:
				o.Status, o.Evidence = Violated, Exhaustive
				o.Reason = "no reachable state satisfies " + o.Property.Expr.String() + " (complete search)"
			default:
				o.Status, o.Evidence = Verified, Exhaustive
			}
			continue
		}
		o.Status, o.Evidence = Inconclusive, Bounded
		switch {
		case s.budgetHit:
			o.Reason = s.stop
		case s.stop == "all properties decided":
			// Cannot happen: an undecided property contradicts the stop
			// reason. Kept for the partition to be total.
			o.Reason = "search stopped before this property was decided"
		default:
			o.Reason = "search stopped early: " + s.stop
		}
	}
}
