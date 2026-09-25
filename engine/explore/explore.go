// Package explore is the explicit-state explorer over the IR (plan 14 §4.1):
// flat byte-vector states, a compact visited set, deterministic transition
// order (process index, then edge index, then — for a rendezvous send — the
// receiving process and edge), DFS with an explicit stack of lazy successor
// iterators, BFS for shortest witnesses, budgets, and the properties of the
// MVP: deadlock, invariants, edge asserts, reachability, with domain
// overflow reported as invalid-model.
//
// # Enabledness (G1)
//
// An edge is enabled in a state when its guard holds and its operation can
// proceed: a buffered send needs room, a buffered receive needs a head
// message satisfying its Match values, a rendezvous send needs a partner —
// another process whose current location has a Recv edge on the same
// channel, itself enabled, whose Match values equal the sent values; every
// such partner is a separate transition. An `else` edge is enabled iff no
// other edge out of the same location is enabled. `run` is always enabled.
//
// `timeout` is evaluated in two phases per state: first every edge is
// tried with timeout = false; only if none is enabled are the edges whose
// guard mentions timeout tried again with timeout = true. So timeout is
// true in a state iff nothing is enabled with timeout taken as false
// (SPIN's rule), and a state with an enabled timeout edge is not a
// deadlock.
//
// # Atomic sequences and stored states (G1)
//
// After an edge with Atomic the process holds exclusive control. As long as
// the holder can move, the state is an *intermediate* state of the atomic
// sequence: it is expanded (only the holder moves) but not stored in the
// visited set — pan does the same, counting these transitions as "atomic
// steps" apart from the stored states. When the holder is blocked the
// sequence is interrupted: the state is stored like any other and every
// process may move (Promela: a blocked atomic loses atomicity). Invariants
// and reach conditions are evaluated on stored states only, so an atomic
// sequence hides its intermediate states from them — the same visibility a
// never claim has in SPIN, which does not move inside an atomic sequence;
// asserts are evaluated on every step.
//
// An edge with DStep continues at once, in the same step, with the first
// enabled edge of the process's new location, and so on while the edges
// taken carry DStep; the intermediate states are never stored. No enabled
// continuation is a model error ("block in d_step seq"), reported as
// invalid-model with the run to the offending step.
//
// A process marked Claim (a never claim) is stored but never executed by
// the safety search and does not take part in the deadlock rule; the
// cycle search of an ltl / progress property (cycle.go, G4) runs the claim
// in synchronous product with the system.
//
// # Deadlock
//
// A state is a deadlock when no transition is enabled in it and not every
// process is terminated. A process is terminated when its control location
// carries the `end` label or has no outgoing edge. This is the definition
// the feature file and the report use. For a Petri net encoded as one
// looping process it is Holzmann's *hang*. On the Promela subset it is
// meant to coincide with SPIN's "invalid end state": the frontend encodes
// SPIN's process termination (the `-end-` transition and its "no younger
// process alive" condition) so that the two rules see the same states. The
// coincidence is established model by model by the differential tests —
// on every chapter 2–3 model in the subset as of G1
// (steps/g1-confirmation.md) — not proved in general.
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
//	inconclusive   undecided and a budget stopped the search: evidence
//	               bounded for the declared states/depth limits, unknown
//	               for time/memory (plan 14 §6)
//	invalid-model  undecided when the model misbehaved: domain overflow,
//	               index out of range, division by zero, blocking inside
//	               d_step (evidence unknown; the trace to the offending step
//	               is attached)
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
	"errors"
	"fmt"
	"strings"
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
	// Sweep keeps searching after every property is decided, so that the
	// state count is that of the whole reachable graph (pan -c0). The
	// verdicts do not change: the first one stands.
	Sweep bool
	// Fairness applies to ltl and progress properties: "" or "none",
	// "weak" (pan -f, see cycle.go), "strong" (not executed, FR-008). A ctl
	// property asked with any fairness is not executed (ctlcheck.go).
	Fairness string
	// Defines are the object-like #define macros of a Promela input, for
	// the atoms of ltl and ctl formulas.
	Defines map[string]string
	// Watch are boolean expressions whose truth is recorded over the stored
	// states of the safety search, for the vacuity hints of FR-011. They do
	// not influence the search in any way.
	Watch []*ir.Expr
}

// Fairness values, shared with the CLI and the MCP server.
const (
	FairnessNone   = "none"
	FairnessWeak   = "weak"
	FairnessStrong = "strong"
)

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
	// Stats are the counters of the property's own search when it had
	// one (ltl, progress: the product search); nil means the Result's.
	Stats *Stats
	// Temporal describes the claim used for an ltl / progress property or
	// the formula used for a ctl one.
	Temporal *TemporalInfo
	// Warnings are notes that do not change the verdict but that a reader
	// must know to interpret it — the vacuity hints of FR-011.
	Warnings []string
}

// Result is the outcome of one run.
type Result struct {
	Outcomes    []Outcome
	States      int
	Transitions int
	// AtomicSteps counts the transitions taken from intermediate states of
	// atomic sequences (never stored), as pan reports them.
	AtomicSteps int
	// MaxDepth is the greatest depth of an expanded state (never above
	// Budget.MaxDepth when that is set).
	MaxDepth   int
	MemBytes   int64
	Elapsed    time.Duration
	StateBytes int
	// Complete is true only when the whole reachable graph was expanded
	// (by the safety search; a temporal property's own search reports its
	// completeness in Outcome.Stats).
	Complete bool
	// Coverage is the record of Options.Watch over the stored states; it is
	// meaningful only when Complete.
	Coverage []Coverage
	// Stop says why the search ended: "complete", "all properties decided",
	// "invalid model", or the budget reason.
	Stop string
}

// budgetEvidence maps a stop reason to the evidence of an inconclusive
// verdict (plan 14 §6): a declared bound on states, depth or the number of
// process instances was reached → bounded; time or memory ran out →
// unknown (the bound was not declared as a search bound but happened).
func budgetEvidence(stop string) Evidence {
	for _, p := range []string{"state budget", "depth budget", "process budget"} {
		if strings.HasPrefix(stop, p) {
			return Bounded
		}
	}
	return EvUnknown
}

// poolExhausted is a `run` that finds no dormant instance left in its pool
// (G5). It is a declared engine bound, not a fault of the model: the
// properties become inconclusive with evidence bounded, never invalid-model
// and never a verdict. SPIN's pan aborts in the same situation ("too many
// processes"), at its own fixed limit of 255.
type poolExhausted struct {
	proc string
	n    int
	step string
}

func (e *poolExhausted) Error() string {
	return fmt.Sprintf("process budget exhausted: the step %q starts another %s, but the engine pre-instantiates at most %d instance(s) of it; raise the bound with --max-procs (mcd) or max_procs (mc_check) and rerun",
		e.step, e.proc, e.n)
}

// handleErr routes an error raised while firing a step: an exhausted
// process pool is a bound (the search stops and says so), anything else is
// a model error (invalid-model with the run to the offending step).
func (s *search) handleErr(err error, tr func() *cex.Trace) {
	var pe *poolExhausted
	if errors.As(err, &pe) {
		s.budget(err.Error())
		return
	}
	s.fail(err.Error(), tr())
}

// Run explores m from its initial state. Safety properties (deadlock,
// invariant, reach, assert) are decided by one DFS / BFS over the model;
// each ltl / progress property is decided by its own product search
// (cycle.go) with its own counters and completeness (Outcome.Stats). The
// Result's counters and Complete are those of the safety search, or of
// the first temporal search when there is no safety property.
func Run(ctx context.Context, m *ir.Model, opt Options) (*Result, error) {
	start := time.Now()
	c, err := compile(m)
	if err != nil {
		return nil, err
	}
	if opt.Mode == "" {
		opt.Mode = DFS
	}
	switch opt.Fairness {
	case "", "none", "weak", "strong":
	default:
		return nil, fmt.Errorf("fairness must be none, weak or strong, got %q", opt.Fairness)
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
	var temporal, ctls []int
	for i, p := range c.props {
		switch p.Kind {
		case KindLTL, KindProgress:
			temporal = append(temporal, i)
		case KindCTL:
			ctls = append(ctls, i)
		}
	}
	hasSafety := s.undecided > 0
	if opt.Watch == nil {
		// The atoms of the ltl properties, so that one safety search can
		// answer the vacuity questions of FR-011 for all of them.
		opt.Watch = WatchExprs(m, opt.Defines)
	}
	s.watch = make([]Coverage, len(opt.Watch))
	for i, e := range opt.Watch {
		ce, err := c.layout.Compile(e, -1)
		if err != nil {
			return nil, err
		}
		s.watched = append(s.watched, ce)
		s.watch[i] = Coverage{Text: e.String()}
	}
	if hasSafety || (len(temporal) == 0 && len(ctls) == 0) {
		if opt.Mode == BFS {
			s.bfs()
		} else {
			s.dfs()
		}
		s.finish()
	}
	if s.res.Complete {
		s.res.Coverage = s.watch
	}
	for _, i := range temporal {
		o, err := runCycle(s, m, c.props[i], i, opt)
		if err != nil {
			return nil, err
		}
		applyVacuity(&o, vacuityCoverage(s.res, o.Temporal), antecedentsOf(o.Temporal), s.res.States)
		s.res.Outcomes[i] = o
	}
	for _, i := range ctls {
		o, err := runCTL(s, m, c.props[i], opt)
		if err != nil {
			return nil, err
		}
		s.res.Outcomes[i] = o
	}
	if !hasSafety && len(temporal) > 0 {
		if st := s.res.Outcomes[temporal[0]].Stats; st != nil {
			s.res.States, s.res.Transitions, s.res.AtomicSteps, s.res.MaxDepth = st.States, st.Transitions, st.AtomicSteps, st.MaxDepth
			s.res.MemBytes, s.res.Complete, s.res.Stop = st.MemBytes, st.Complete, st.Stop
		} else {
			s.res.Complete, s.res.Stop = true, "not executed"
		}
	}
	s.res.Elapsed = time.Since(start)
	return s.res, nil
}

// ---- compiled model -------------------------------------------------------

type cAssign struct {
	slot  *ir.Slot
	index *ir.Compiled // nil for scalars
	value *ir.Compiled
}

type cSend struct {
	ch   int          // static channel index; -1 when sel is set
	sel  *ir.Compiled // dynamic channel id (G5)
	args []*ir.Compiled
}

type cRecvArg struct {
	slot  *ir.Slot     // bind target (nil: no binding)
	index *ir.Compiled // array index of the bind target
	match *ir.Compiled // required value (nil: none)
}

type cRecv struct {
	ch   int
	sel  *ir.Compiled
	args []cRecvArg
}

// cRun is a compiled `run`: pool lists the interchangeable instances in
// allocation order, params[k] are the parameter slots of pool[k].
type cRun struct {
	pool   []int
	entry  int
	args   []*ir.Compiled
	params [][]*ir.Slot
	// init[k] are the initialisers of pool[k]'s locals, compiled in that
	// instance's own scope and applied after its parameters.
	init [][]cAssign
}

type cEdge struct {
	proc, idx int
	e         *ir.Edge
	guard     *ir.Compiled
	assert    *ir.Compiled
	effect    []cAssign
	send      *cSend
	recv      *cRecv
	run       *cRun
	// usesTimeout: the guard mentions timeout (phase-1 candidate).
	usesTimeout bool
	// rv: a send on a rendezvous channel; receivers lists every Recv edge
	// on that channel in the other processes, in (process, edge) order.
	// For a send on a dynamic channel (dyn) the capacity is only known in a
	// state, so receivers lists every Recv edge of the other processes and
	// rvMatch decides, in the state, whether the two channels are the same.
	rv        bool
	dyn       bool
	receivers []*cEdge
}

type cProc struct {
	out   [][]int // per location: edge indices, ascending
	end   []bool  // per location: carries the end label
	edge  []cEdge
	claim bool
	// provided is the compiled `provided (expr)` clause: while it is false
	// no edge of the process is enabled (Promela's process-level guard).
	provided *ir.Compiled
}

type cProp struct {
	i    int // index into outcomes
	expr *ir.Compiled
}

type compiled struct {
	m          *ir.Model
	layout     *ir.Layout
	procs      []cProc
	hasAsrt    bool
	hasTimeout bool
	props      []ir.Property
	invs       []cProp
	reaches    []cProp
	deadlock   []int
	asserts    []int
}

func compile(m *ir.Model) (*compiled, error) {
	l, err := ir.NewLayout(m)
	if err != nil {
		return nil, err
	}
	c := &compiled{m: m, layout: l}
	for p := range m.Processes {
		pr := &m.Processes[p]
		cp := cProc{out: make([][]int, len(pr.Locations)), end: make([]bool, len(pr.Locations)), claim: pr.Claim}
		if cp.provided, err = l.Compile(pr.Provided, p); err != nil {
			return nil, fmt.Errorf("%s provided: %w", pr.Name, err)
		}
		for i, loc := range pr.Locations {
			for _, lb := range loc.Labels {
				if lb == ir.End {
					cp.end[i] = true
				}
			}
		}
		for i := range pr.Edges {
			e := &pr.Edges[i]
			ce := cEdge{proc: p, idx: i, e: e, usesTimeout: e.Guard.Uses("timeout")}
			if ce.guard, err = l.Compile(e.Guard, p); err != nil {
				return nil, fmt.Errorf("%s edge %d guard: %w", pr.Name, i, err)
			}
			if ce.assert, err = l.Compile(e.Assert, p); err != nil {
				return nil, fmt.Errorf("%s edge %d assert: %w", pr.Name, i, err)
			}
			if e.Assert != nil && !pr.Claim {
				c.hasAsrt = true // a claim's assert belongs to the claim, not to the model
			}
			if ce.usesTimeout {
				c.hasTimeout = true
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
			if e.Send != nil {
				cs := &cSend{ch: -1}
				if e.Send.Sel != nil {
					if cs.sel, err = l.Compile(e.Send.Sel, p); err != nil {
						return nil, err
					}
					ce.dyn, ce.rv = true, true
				} else {
					cs.ch, _ = l.ChanIndex(e.Send.Chan)
					ce.rv = m.Channels[cs.ch].Capacity == 0
				}
				for _, a := range e.Send.Args {
					ca, err := l.Compile(a, p)
					if err != nil {
						return nil, err
					}
					cs.args = append(cs.args, ca)
				}
				ce.send = cs
			}
			if e.Recv != nil {
				ci := -1
				var sel *ir.Compiled
				if e.Recv.Sel != nil {
					if sel, err = l.Compile(e.Recv.Sel, p); err != nil {
						return nil, err
					}
				} else {
					ci, _ = l.ChanIndex(e.Recv.Chan)
				}
				cr := &cRecv{ch: ci, sel: sel}
				for _, a := range e.Recv.Args {
					var ra cRecvArg
					if a.Var != "" {
						ra.slot = l.Resolve(a.Var, p)
						if ra.index, err = l.Compile(a.Index, p); err != nil {
							return nil, err
						}
					}
					if ra.match, err = l.Compile(a.Match, p); err != nil {
						return nil, err
					}
					cr.args = append(cr.args, ra)
				}
				ce.recv = cr
			}
			if e.Run != nil {
				r := &cRun{pool: e.Run.Targets(), entry: e.Run.Entry}
				for _, a := range e.Run.Args {
					ca, err := l.Compile(a, p)
					if err != nil {
						return nil, err
					}
					r.args = append(r.args, ca)
				}
				for _, q := range r.pool {
					target := &m.Processes[q]
					var slots []*ir.Slot
					for j := range e.Run.Args {
						slots = append(slots, l.Resolve(target.Locals[j].Name, q))
					}
					r.params = append(r.params, slots)
					var inits []cAssign
					for _, a := range e.Run.Init {
						ca := cAssign{slot: l.Resolve(a.Var, q)}
						if ca.index, err = l.Compile(a.Index, q); err != nil {
							return nil, err
						}
						if ca.value, err = l.Compile(a.Value, q); err != nil {
							return nil, err
						}
						inits = append(inits, ca)
					}
					r.init = append(r.init, inits)
				}
				ce.run = r
			}
			cp.edge = append(cp.edge, ce)
			cp.out[e.From] = append(cp.out[e.From], i)
		}
		c.procs = append(c.procs, cp)
	}
	// Rendezvous partners: every Recv edge on the channel in another,
	// non-claim process, in (process, edge) order. A send whose channel is
	// dynamic cannot name the channel here, so it takes every Recv edge as
	// a candidate and rvMatch decides in the state (see cEdge.dyn).
	for p := range c.procs {
		for i := range c.procs[p].edge {
			e := &c.procs[p].edge[i]
			if !e.rv {
				continue
			}
			for q := range c.procs {
				if q == p || c.procs[q].claim {
					continue
				}
				for j := range c.procs[q].edge {
					r := &c.procs[q].edge[j]
					if r.recv == nil {
						continue
					}
					if e.dyn || r.recv.sel != nil || r.recv.ch == e.send.ch {
						e.receivers = append(e.receivers, r)
					}
				}
			}
		}
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

// move is one transition: an edge and, for a rendezvous handshake, the
// receiving edge taken in the same step.
type move struct {
	e, partner *cEdge
}

type frame struct {
	idx      int32 // state index in visited
	proc     int32 // iterator: current process; -1 = not started
	pos      int32 // iterator: position in out[proc][pc]
	viaProc  int32 // edge that led here (-1 for the initial state)
	viaEdge  int32
	viaPart  int32 // partner (process<<16 | edge) or -1
	enabled  uint32
	rv       int32  // iterator: next receiver of pend to try; -1 = none pending
	pend     *cEdge // the rendezvous send whose receivers are enumerated
	phase    int8   // 0: timeout = false; 1: timeout = true (see package doc)
	exclOnly bool
	// Product search (cycle.go): the claim edge taken to reach this state
	// (-1 none), a null fairness step (0 none, else target copy + 1), the
	// claim iterator (cpos: -1 not started, -2 no claim step here; cedge:
	// the chosen claim edge or -1) and whether the null step was offered.
	viaClaim int32
	viaNull  int8
	cpos     int32
	cedge    int32
	cto      int32 // the claim's location after the chosen claim step
	eps      int8
	sysSeen  bool // a system move was returned from this frame
	stut     bool // the stutter move was returned for the current claim edge
}

type search struct {
	c       *compiled
	opt     Options
	ctx     context.Context
	visited Visited
	res     *Result

	cur, next []byte
	stack     []frame
	// tmp holds the intermediate atomic states of the DFS stack (frames
	// with idx < 0 refer to tmp[-idx-1]); it grows and shrinks with the
	// stack.
	tmp [][]byte
	// BFS bookkeeping (parallel to visited indices): the parent stored
	// state and the chain of moves from it (one move, or an atomic
	// sequence's moves).
	parent []int32
	chains [][]cex.Ref
	depth  []int32

	watched []*ir.Compiled
	watch   []Coverage

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
		case KindLTL, KindProgress:
			// Placeholder: Run replaces it by the product search's outcome.
			o.Status = NotExecuted
			o.Evidence = EvUnknown
			o.Reason = "temporal property not yet checked"
		default:
			o.Status = NotExecuted
			o.Evidence = EvUnknown
			o.Reason = fmt.Sprintf("property kind %q is not executed by this engine version (it executes deadlock, invariant, reach, assert, ltl, progress)", p.Kind)
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
	if s.undecided == 0 && s.stop == "" && !s.opt.Sweep {
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

// allTerminated reports whether every non-claim process is at an end
// location or at a location without outgoing edges (the second half of the
// deadlock rule).
func (s *search) allTerminated(state []byte) bool {
	for p := range s.c.procs {
		if s.c.procs[p].claim {
			continue
		}
		loc := s.c.layout.ReadPC(state, p)
		if s.c.procs[p].end[loc] || len(s.c.procs[p].out[loc]) == 0 {
			continue
		}
		return false
	}
	return true
}

// ---- enabledness ---------------------------------------------------------------

// enabled reports whether e can be taken in state (timeout as currently set
// in the layout). For a rendezvous send it asks whether some partner exists.
func (s *search) enabled(e *cEdge, state []byte) (bool, error) {
	if pr := s.c.procs[e.proc].provided; pr != nil {
		// `provided` gates every transition of the process, `else` and the
		// `-end-` transition included.
		ok, err := pr.Truth(state)
		if err != nil || !ok {
			return false, err
		}
	}
	if e.e.Else {
		loc := s.c.layout.ReadPC(state, e.proc)
		for _, oi := range s.c.procs[e.proc].out[loc] {
			o := &s.c.procs[e.proc].edge[oi]
			if o == e || o.e.Else {
				continue
			}
			ok, err := s.enabled(o, state)
			if err != nil || ok {
				return false, err
			}
		}
		return true, nil
	}
	ok, err := e.guard.Truth(state)
	if err != nil || !ok {
		return false, err
	}
	l := s.c.layout
	switch {
	case e.send != nil:
		ci, err := s.sendChan(e, state)
		if err != nil {
			return false, err
		}
		if l.Chans[ci].Chan.Capacity == 0 {
			for _, r := range e.receivers {
				ok, err := s.rvMatch(e, r, state)
				if err != nil || ok {
					return ok, err
				}
			}
			return false, nil
		}
		return l.ChanLen(state, ci) < l.Chans[ci].Chan.Capacity, nil
	case e.recv != nil:
		ci, err := s.recvChan(e, state)
		if err != nil {
			return false, err
		}
		return s.recvMatch(e.recv, ci, state)
	}
	return true, nil
}

// sendChan resolves the channel of a send in state, checking the id and the
// message shape: a send on the null channel or on a channel with a
// different number of fields is a model error, not a blocked step.
func (s *search) sendChan(e *cEdge, state []byte) (int, error) {
	if e.send.sel == nil {
		return e.send.ch, nil
	}
	id, err := e.send.sel.Eval(state)
	if err != nil {
		return 0, err
	}
	return s.resolveChan(id, len(e.send.args), e, "!")
}

func (s *search) recvChan(e *cEdge, state []byte) (int, error) {
	if e.recv.sel == nil {
		return e.recv.ch, nil
	}
	id, err := e.recv.sel.Eval(state)
	if err != nil {
		return 0, err
	}
	return s.resolveChan(id, len(e.recv.args), e, "?")
}

func (s *search) resolveChan(id int64, nargs int, e *cEdge, op string) (int, error) {
	ci, ok := s.c.layout.ChanByID(id)
	if !ok {
		return 0, fmt.Errorf("channel id %d is not a channel of the model in step %q (0 is Promela's null channel: %s on it has no meaning)", id, cex.CommandText(e.e), op)
	}
	if got := len(s.c.layout.Chans[ci].Chan.Fields); got != nargs {
		return 0, fmt.Errorf("channel %s has %d message field(s) but step %q uses %d: the engine does not guess a correspondence between differently shaped messages",
			s.c.layout.Chans[ci].Chan.Name, got, cex.CommandText(e.e), nargs)
	}
	return ci, nil
}

// recvMatch: the buffer is not empty and the head satisfies every Match.
func (s *search) recvMatch(r *cRecv, ci int, state []byte) (bool, error) {
	l := s.c.layout
	if l.ChanLen(state, ci) == 0 {
		return false, nil
	}
	for f, a := range r.args {
		if a.match == nil {
			continue
		}
		want, err := a.match.Eval(state)
		if err != nil {
			return false, err
		}
		if l.ChanField(state, ci, 0, f) != want {
			return false, nil
		}
	}
	return true, nil
}

// rvMatch: receiver r is at the location of its edge, it reads the same
// channel as e writes, its own guard holds and its Match values equal what
// e sends.
func (s *search) rvMatch(e, r *cEdge, state []byte) (bool, error) {
	if s.c.layout.ReadPC(state, r.proc) != r.e.From {
		return false, nil
	}
	ok, err := r.guard.Truth(state)
	if err != nil || !ok {
		return false, err
	}
	sc, err := s.sendChan(e, state)
	if err != nil {
		return false, err
	}
	rc, err := s.recvChan(r, state)
	if err != nil {
		return false, err
	}
	if sc != rc {
		return false, nil
	}
	for f, a := range r.recv.args {
		if a.match == nil {
			continue
		}
		want, err := a.match.Eval(state)
		if err != nil {
			return false, err
		}
		got, err := e.send.args[f].Eval(state)
		if err != nil {
			return false, err
		}
		if got != want {
			return false, nil
		}
	}
	return true, nil
}

// hasEnabled reports whether process p has an enabled edge in state
// (timeout = false).
func (s *search) hasEnabled(p int, state []byte) (bool, error) {
	s.c.layout.Timeout = false
	loc := s.c.layout.ReadPC(state, p)
	for _, ei := range s.c.procs[p].out[loc] {
		ok, err := s.enabled(&s.c.procs[p].edge[ei], state)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

// startIter positions f's iterator for state: only the exclusive holder if
// it has an enabled edge (Promela atomic), else every process from 0.
func (s *search) startIter(f *frame, state []byte) error {
	f.proc, f.pos, f.rv, f.phase = 0, 0, -1, 0
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

// nextEnabled advances f's iterator to the next enabled move of state.
func (s *search) nextEnabled(f *frame, state []byte) (move, bool, error) {
	if f.proc < 0 {
		if err := s.startIter(f, state); err != nil {
			return move{}, false, err
		}
	}
	l := s.c.layout
	for {
		if f.rv >= 0 {
			// Enumerating the partners of a pending rendezvous send.
			e := f.pend
			l.Timeout = f.phase == 1
			for int(f.rv) < len(e.receivers) {
				r := e.receivers[f.rv]
				f.rv++
				ok, err := s.rvMatch(e, r, state)
				if err != nil {
					return move{}, false, err
				}
				if ok {
					return move{e, r}, true, nil
				}
			}
			f.rv, f.pend = -1, nil
			continue
		}
		if int(f.proc) >= len(s.c.procs) {
			if f.phase == 0 && f.enabled == 0 && s.c.hasTimeout {
				f.phase, f.proc, f.pos = 1, 0, 0
				continue
			}
			return move{}, false, nil
		}
		p := int(f.proc)
		if s.c.procs[p].claim {
			f.proc++
			f.pos = 0
			continue
		}
		outs := s.c.procs[p].out[l.ReadPC(state, p)]
		if int(f.pos) >= len(outs) {
			if f.exclOnly {
				return move{}, false, nil
			}
			f.proc++
			f.pos = 0
			continue
		}
		e := &s.c.procs[p].edge[outs[f.pos]]
		f.pos++
		if f.phase == 1 && !e.usesTimeout {
			continue
		}
		l.Timeout = f.phase == 1
		if e.rv {
			ok, err := e.guard.Truth(state)
			if err != nil {
				return move{}, false, err
			}
			if !ok {
				continue
			}
			rv := true
			if e.dyn {
				// The capacity is a property of the channel the selector
				// names in this state, so the choice between a handshake and
				// a buffered send is made here, not at compile time.
				ci, err := s.sendChan(e, state)
				if err != nil {
					return move{}, false, err
				}
				rv = l.Chans[ci].Chan.Capacity == 0
			}
			if rv {
				f.pend, f.rv = e, 0
				continue
			}
		}
		ok, err := s.enabled(e, state)
		if err != nil {
			return move{}, false, err
		}
		if ok {
			return move{e, nil}, true, nil
		}
	}
}

// ---- firing -------------------------------------------------------------------

const dstepLimit = 100000

// fire computes into s.next the state after m from s.cur, including the
// d_step continuation. It returns the edge whose assert failed (evaluated
// in the state just before that edge), if any, and an evaluation/overflow
// error as the reason the model is invalid.
func (s *search) fire(m move) (failed *cEdge, err error) {
	copy(s.next, s.cur)
	s.c.layout.Timeout = false
	e := m.e
	if e.assert != nil {
		ok, err := e.assert.Truth(s.next)
		if err != nil {
			return nil, err
		}
		if !ok {
			failed = e
		}
	}
	if err := s.apply(e, m.partner); err != nil {
		return failed, err
	}
	last := e
	for n := 0; last.e.DStep; n++ {
		if n >= dstepLimit {
			return failed, fmt.Errorf("d_step starting at %q did not finish after %d steps", cex.CommandText(e.e), dstepLimit)
		}
		p := last.proc
		loc := s.c.layout.ReadPC(s.next, p)
		var chosen *cEdge
		var texts []string
		for _, oi := range s.c.procs[p].out[loc] {
			o := &s.c.procs[p].edge[oi]
			texts = append(texts, cex.CommandText(o.e))
			if o.rv {
				return failed, fmt.Errorf("rendezvous inside d_step (%q) is not executable in this engine version", cex.CommandText(o.e))
			}
			ok, err := s.enabled(o, s.next)
			if err != nil {
				return failed, err
			}
			if ok {
				chosen = o
				break
			}
		}
		if chosen == nil {
			return failed, fmt.Errorf("block in d_step seq: %s is not executable inside the d_step starting at %q", strings.Join(texts, " | "), cex.CommandText(e.e))
		}
		if chosen.assert != nil && failed == nil {
			ok, err := chosen.assert.Truth(s.next)
			if err != nil {
				return failed, err
			}
			if !ok {
				failed = chosen
			}
		}
		if err := s.apply(chosen, nil); err != nil {
			return failed, err
		}
		last = chosen
	}
	return failed, nil
}

// apply performs one edge (with its rendezvous partner) on s.next.
func (s *search) apply(e, partner *cEdge) error {
	l := s.c.layout
	st := s.next
	l.WritePC(st, e.proc, e.e.To)
	excl := 0
	if e.e.Atomic {
		excl = e.proc + 1
	}
	if partner != nil {
		l.WritePC(st, partner.proc, partner.e.To)
		if partner.e.Atomic {
			excl = partner.proc + 1
		}
	}
	st[l.Excl] = byte(excl)
	if e.e.Leave {
		l.Leave(st)
	}
	for _, ci := range e.e.ClearChans {
		l.ClearChan(st, ci)
	}
	if r := e.run; r != nil {
		// The first dormant instance of the pool becomes the new process:
		// the k-th live instance of a proctype is its k-th pool slot, which
		// is what keeps the vector in step with pan's process stack.
		slot := -1
		for k, q := range r.pool {
			if l.ReadPC(st, q) == s.c.m.Processes[q].Initial && s.c.m.Processes[q].Dynamic {
				slot = k
				break
			}
		}
		if slot < 0 {
			return &poolExhausted{proc: s.c.m.Processes[r.pool[0]].Name, n: len(r.pool), step: cex.CommandText(e.e)}
		}
		q := r.pool[slot]
		for i, a := range r.args {
			v, err := a.Eval(st)
			if err != nil {
				return err
			}
			if err := s.store1(r.params[slot][i], v, e); err != nil {
				return err
			}
		}
		for _, a := range r.init[slot] {
			slot2 := a.slot
			if a.index != nil {
				i, err := a.index.Eval(st)
				if err != nil {
					return err
				}
				if i < 0 || i >= int64(slot2.Var.Len) {
					return fmt.Errorf("index %d out of range for %s[%d]", i, slot2.Var.Name, slot2.Var.Len)
				}
				slot2 = slot2.At(int(i))
			}
			v, err := a.value.Eval(st)
			if err != nil {
				return err
			}
			if err := s.store1(slot2, v, e); err != nil {
				return err
			}
		}
		l.WritePC(st, q, r.entry)
		if l.HasTable() && !l.Enter(st, q) {
			return &poolExhausted{proc: s.c.m.Processes[q].Name, n: len(s.c.m.Processes), step: cex.CommandText(e.e)}
		}
	}
	if e.send != nil {
		ci, err := s.sendChan(e, st)
		if err != nil {
			return err
		}
		vals := make([]int64, len(e.send.args))
		for i, a := range e.send.args {
			v, err := a.Eval(st)
			if err != nil {
				return err
			}
			vals[i] = v
		}
		if partner == nil {
			l.ChanPush(st, ci, vals)
		} else {
			for f, a := range partner.recv.args {
				if err := s.bind(a, vals[f], partner); err != nil {
					return err
				}
			}
		}
	}
	if e.recv != nil {
		ci, err := s.recvChan(e, st)
		if err != nil {
			return err
		}
		for f, a := range e.recv.args {
			if err := s.bind(a, l.ChanField(st, ci, 0, f), e); err != nil {
				return err
			}
		}
		l.ChanPop(st, ci)
	}
	for _, a := range e.effect {
		slot := a.slot
		if a.index != nil {
			i, err := a.index.Eval(st)
			if err != nil {
				return err
			}
			if i < 0 || i >= int64(slot.Var.Len) {
				return fmt.Errorf("index %d out of range for %s[%d]", i, slot.Var.Name, slot.Var.Len)
			}
			slot = slot.At(int(i))
		}
		v, err := a.value.Eval(st)
		if err != nil {
			return err
		}
		if err := s.store1(slot, v, e); err != nil {
			return err
		}
	}
	return nil
}

// bind writes a received field into the receive argument's variable.
func (s *search) bind(a cRecvArg, v int64, e *cEdge) error {
	if a.slot == nil {
		return nil
	}
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
	return s.store1(slot, v, e)
}

// store1 writes v into slot after the domain check.
func (s *search) store1(slot *ir.Slot, v int64, e *cEdge) error {
	if !slot.InDomain(v) {
		return fmt.Errorf("domain overflow: %s = %d leaves [%d, %d]%s in step %q",
			slot.Name(), v, slot.Min, slot.Max, domainNote(slot), cex.CommandText(e.e))
	}
	slot.Write(s.next, v)
	return nil
}

func domainNote(sl *ir.Slot) string {
	if sl.Var.Max != nil || sl.Var.Min != nil {
		return " (declared capacity)"
	}
	return " (" + string(sl.Var.Type) + " domain)"
}

// ---- property checks ------------------------------------------------------------

// checkState evaluates invariants and reach conditions on a newly stored
// state; path is the run that reaches it.
func (s *search) checkState(state []byte, path func() *cex.Trace) error {
	s.c.layout.Timeout = false
	for i, w := range s.watched {
		ok, err := w.Truth(state)
		if err != nil {
			return err
		}
		if ok {
			s.watch[i].EverTrue = true
		} else {
			s.watch[i].EverFalse = true
		}
	}
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

func partnerCode(m move) int32 {
	if m.partner == nil {
		return -1
	}
	return int32(m.partner.proc<<16 | m.partner.idx)
}

func (s *search) ref(proc, edge, part int32) cex.Ref {
	r := cex.Ref{Proc: int(proc), Edge: int(edge)}
	if part >= 0 {
		r.PartnerProc, r.PartnerEdge, r.HasPartner = int(part>>16), int(part&0xffff), true
	}
	return r
}

// ---- DFS --------------------------------------------------------------------

const frameBytes = 64

// intermediate reports whether state is inside an atomic sequence whose
// holder can move (pan does not store such states).
func (s *search) intermediate(state []byte) (bool, error) {
	ex := state[s.c.layout.Excl]
	if ex == 0 {
		return false, nil
	}
	return s.hasEnabled(int(ex)-1, state)
}

// stateOf returns the state a frame refers to.
func (s *search) stateOf(f *frame) []byte {
	if f.idx < 0 {
		return s.tmp[-int(f.idx)-1]
	}
	return s.visited.Get(int(f.idx))
}

func (s *search) dfs() {
	l := s.c.layout
	init := l.Initial()
	s.cur = make([]byte, l.Size)
	s.next = make([]byte, l.Size)
	idx0, _ := s.visited.Add(init)
	s.stack = append(s.stack, frame{idx: int32(idx0), proc: -1, viaProc: -1, viaEdge: -1, viaPart: -1, rv: -1})
	copy(s.cur, init)
	curIdx := int32(idx0)
	s.res.MaxDepth = 0

	pathToTop := func() *cex.Trace { return s.dfsPath(nil, nil) }
	if err := s.checkState(s.cur, pathToTop); err != nil {
		s.fail(err.Error(), pathToTop())
	}

	for len(s.stack) > 0 && s.stop == "" {
		top := &s.stack[len(s.stack)-1]
		if top.idx != curIdx {
			copy(s.cur, s.stateOf(top))
			curIdx = top.idx
		}
		m, ok, err := s.nextEnabled(top, s.cur)
		if err != nil {
			s.handleErr(err, pathToTop)
			break
		}
		if !ok {
			if top.enabled == 0 && !s.allTerminated(s.cur) {
				s.deadlockAt(pathToTop)
			}
			if top.idx < 0 {
				s.tmp = s.tmp[:len(s.tmp)-1]
			}
			s.stack = s.stack[:len(s.stack)-1]
			continue
		}
		top.enabled++
		s.res.Transitions++
		// Guard held: compute the successor; the assert of each edge taken
		// is evaluated in the state just before it. A failed assert's
		// witness ends with this step and shows the successor as its final
		// state.
		failed, err := s.fire(m)
		if err != nil {
			s.handleErr(err, func() *cex.Trace { return s.dfsPath(&m, s.next) })
			break
		}
		if failed != nil {
			s.assertFailed(failed, func() *cex.Trace { return s.dfsPath(&m, s.next) })
			if s.stop != "" {
				break
			}
		}
		depth := len(s.stack) // transitions from the initial state to next
		inter, err := s.intermediate(s.next)
		if err != nil {
			s.handleErr(err, func() *cex.Trace { return s.dfsPath(&m, s.next) })
			break
		}
		if inter {
			// Inside an atomic sequence: expand, do not store.
			s.res.AtomicSteps++
			if !s.checkBudgets(int64(len(s.stack))*frameBytes + int64(len(s.tmp))*int64(l.Size)) {
				break
			}
			if s.opt.Budget.MaxDepth > 0 && depth > s.opt.Budget.MaxDepth {
				s.truncated++
				continue
			}
			if depth > s.res.MaxDepth {
				s.res.MaxDepth = depth
			}
			s.tmp = append(s.tmp, append([]byte(nil), s.next...))
			s.stack = append(s.stack, frame{idx: -int32(len(s.tmp)), proc: -1, viaProc: int32(m.e.proc), viaEdge: int32(m.e.idx), viaPart: partnerCode(m), rv: -1})
			continue
		}
		idx, isNew, allowed := s.store()
		if !allowed {
			break
		}
		if !isNew {
			continue
		}
		s.res.States = s.visited.Len()
		pathToNext := func() *cex.Trace { return s.dfsPath(&m, s.next) }
		if err := s.checkState(s.next, pathToNext); err != nil {
			s.fail(err.Error(), pathToNext())
			break
		}
		if s.stop != "" {
			break
		}
		if !s.checkBudgets(int64(len(s.stack))*frameBytes + int64(len(s.tmp))*int64(l.Size)) {
			break
		}
		if s.opt.Budget.MaxDepth > 0 && depth > s.opt.Budget.MaxDepth {
			s.truncated++
			continue
		}
		if depth > s.res.MaxDepth {
			s.res.MaxDepth = depth
		}
		s.stack = append(s.stack, frame{idx: int32(idx), proc: -1, viaProc: int32(m.e.proc), viaEdge: int32(m.e.idx), viaPart: partnerCode(m), rv: -1})
	}
	s.res.States = s.visited.Len()
	s.res.MemBytes = s.visited.Bytes() + int64(cap(s.stack))*frameBytes + int64(len(s.tmp))*int64(l.Size)
	if s.stop == "" && len(s.stack) == 0 {
		s.stop = "complete"
	}
}

// dfsPath renders the run along the stack, optionally extended by one more
// move into state last.
func (s *search) dfsPath(extra *move, last []byte) *cex.Trace {
	var states [][]byte
	var refs []cex.Ref
	for i := range s.stack {
		f := &s.stack[i]
		states = append(states, append([]byte(nil), s.stateOf(f)...))
		if i > 0 {
			refs = append(refs, s.ref(f.viaProc, f.viaEdge, f.viaPart))
		}
	}
	if extra != nil {
		states = append(states, append([]byte(nil), last...))
		refs = append(refs, s.ref(int32(extra.e.proc), int32(extra.e.idx), partnerCode(*extra)))
	}
	return cex.Build(s.c.layout, states, refs)
}

// ---- BFS --------------------------------------------------------------------

const bfsBytesPerState = 40

// bfsNode is an intermediate atomic state being expanded depth-first
// inside a BFS step.
type bfsNode struct {
	state []byte
	f     frame
	chain []cex.Ref
}

func (s *search) bfs() {
	l := s.c.layout
	init := l.Initial()
	s.cur = make([]byte, l.Size)
	s.next = make([]byte, l.Size)
	s.visited.Add(init)
	s.parent = append(s.parent, -1)
	s.chains = append(s.chains, nil)
	s.depth = append(s.depth, 0)

	pathTo := func(idx int) func() *cex.Trace { return func() *cex.Trace { return s.bfsPath(idx, nil, nil) } }
	if err := s.checkState(init, pathTo(0)); err != nil {
		s.fail(err.Error(), pathTo(0)())
	}

	head := 0
	for head < s.visited.Len() && s.stop == "" {
		d := int(s.depth[head])
		if s.opt.Budget.MaxDepth > 0 && d > s.opt.Budget.MaxDepth {
			s.truncated++ // stored (by its parent) but not expanded, as in DFS
			head++
			continue
		}
		if d > s.res.MaxDepth {
			s.res.MaxDepth = d
		}
		// Depth-first over the intermediate atomic states hanging off head;
		// the stored successors get head as parent and the chain of moves.
		nodes := []bfsNode{{state: append([]byte(nil), s.visited.Get(head)...), f: frame{proc: -1, rv: -1}}}
		for len(nodes) > 0 && s.stop == "" {
			n := &nodes[len(nodes)-1]
			copy(s.cur, n.state)
			m, ok, err := s.nextEnabled(&n.f, s.cur)
			if err != nil {
				s.handleErr(err, func() *cex.Trace { return s.bfsPath(head, n.chain, nil) })
				break
			}
			if !ok {
				if len(nodes) == 1 && n.f.enabled == 0 && !s.allTerminated(s.cur) {
					s.deadlockAt(pathTo(head))
				}
				nodes = nodes[:len(nodes)-1]
				continue
			}
			n.f.enabled++
			s.res.Transitions++
			chain := append(append([]cex.Ref(nil), n.chain...), s.ref(int32(m.e.proc), int32(m.e.idx), partnerCode(m)))
			failed, err := s.fire(m)
			if err != nil {
				s.handleErr(err, func() *cex.Trace { return s.bfsPath(head, chain, s.next) })
				break
			}
			if failed != nil {
				s.assertFailed(failed, func() *cex.Trace { return s.bfsPath(head, chain, s.next) })
				if s.stop != "" {
					break
				}
			}
			inter, err := s.intermediate(s.next)
			if err != nil {
				s.handleErr(err, func() *cex.Trace { return s.bfsPath(head, chain, s.next) })
				break
			}
			if inter {
				s.res.AtomicSteps++
				if len(nodes) > dstepLimit {
					s.budget(fmt.Sprintf("depth budget exhausted: an atomic sequence exceeds %d steps", dstepLimit))
					break
				}
				nodes = append(nodes, bfsNode{state: append([]byte(nil), s.next...), f: frame{proc: -1, rv: -1}, chain: chain})
				continue
			}
			idx, isNew, allowed := s.store()
			if !allowed {
				break
			}
			if !isNew {
				continue
			}
			s.parent = append(s.parent, int32(head))
			s.chains = append(s.chains, chain)
			s.depth = append(s.depth, int32(d+len(chain)))
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

// bfsPath renders the shortest run to stored state idx (via parent links,
// replaying each chain of moves to recover intermediate atomic states),
// optionally extended by more moves (extra) into last.
func (s *search) bfsPath(idx int, extra []cex.Ref, last []byte) *cex.Trace {
	var order []int
	for i := idx; i >= 0; i = int(s.parent[i]) {
		order = append(order, i)
	}
	states := [][]byte{append([]byte(nil), s.visited.Get(order[len(order)-1])...)}
	var refs []cex.Ref
	saveCur, saveNext := append([]byte(nil), s.cur...), append([]byte(nil), s.next...)
	replay := func(chain []cex.Ref, final []byte) {
		for k, r := range chain {
			refs = append(refs, r)
			if k == len(chain)-1 && final != nil {
				states = append(states, append([]byte(nil), final...))
				return
			}
			copy(s.cur, states[len(states)-1])
			m := move{e: &s.c.procs[r.Proc].edge[r.Edge]}
			if r.HasPartner {
				m.partner = &s.c.procs[r.PartnerProc].edge[r.PartnerEdge]
			}
			s.fire(m) // deterministic replay; errors were reported when first taken
			states = append(states, append([]byte(nil), s.next...))
		}
	}
	for k := len(order) - 2; k >= 0; k-- {
		i := order[k]
		replay(s.chains[i], s.visited.Get(i))
	}
	if len(extra) > 0 {
		replay(extra, last)
	}
	copy(s.cur, saveCur)
	copy(s.next, saveNext)
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
		o.Status, o.Evidence = Inconclusive, budgetEvidence(s.stop)
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
