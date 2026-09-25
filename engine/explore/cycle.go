package explore

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"modelcheck/cex"
	"modelcheck/ir"
	"modelcheck/ltl"
)

// Cycle search (G4): the synchronous product of the model with a claim,
// searched for acceptance cycles by nested depth-first search
// (Courcoubetis–Vardi–Wolper–Yannakakis, with Holzmann's "close on the
// outer stack" shortcut).
//
// # Product semantics (SPIN's, stated once here)
//
// A product state is a stored model state whose vector already holds the
// claim's control location. From it, the claim moves first: one of its
// edges whose guard holds in the current state is taken (the claim reads
// the state before any process changes it); then the system takes one of
// its moves. The state after the claim move alone is never stored — pan
// does the same — so the stored states are exactly the pairs (system
// state, claim location) after a system step, and the state counts agree
// with pan's "states, stored". The claim does not move inside an atomic
// sequence: intermediate atomic states (G1 rule) are expanded without a
// claim step and stay unstored. A claim with no enabled edge prunes the
// path. A claim edge into a location with the `end` label (the claim's
// closing brace) is a violation on a finite prefix ("end state in claim
// reached"), as is a claim edge whose assert is false ("assertion
// violated" in the claim, which is how `spin -f` encodes the safety part
// of a formula). A claim edge with Atomic continues at once, in the same
// claim step and on the same system state, with the first enabled edge of
// its new location, and so on while the edges taken are atomic — this is
// how `spin -f` claims evaluate their assert on the state that made the
// guard true. (Branching inside an atomic claim sequence takes the first
// enabled alternative; generated claims never branch there.) The claim's
// moves are rendered as steps of the claim process in counterexamples.
//
// # Stutter extension (pan's default; pan -DNOSTUTTER turns it off)
//
// In a state where the system has no enabled move at all — every process
// terminated, or blocked (an invalid end state) — the run is extended by
// repeating that state forever: the claim keeps moving alone (a "stutter"
// move of the product: claim step, no system step). Without a claim, a
// process at an accept label in such a state is on an acceptance cycle
// (pan: "accept stutter"). This is what makes []p on a terminating model
// violated when p fails in the final state, as SPIN reports it.
//
// # Acceptance and the direction of the negation
//
// A stored state is accepting when the claim's location carries `accept`;
// without a claim (an `ltl` property without formula on a model without a
// never claim) when some process is at an `accept` location, as pan -a.
// The claim of an `ltl` formula φ is the automaton of !(φ) (ltl.ForProperty),
// so an acceptance cycle is a run satisfying !(φ): φ is violated. For a
// `progress` property the claim is pan's np_ automaton — `np_` is true when
// no process is at a `progress` location; s0: np_ → s1 | true → s0; s1
// (accept): np_ → s1 — so an acceptance cycle is a cycle in which no
// progress label is ever visited: a non-progress cycle.
//
// # Weak fairness (n + 2 copies, as pan -f)
//
// With Options.Fairness "weak" the product state carries one more byte,
// the copy k ∈ 0..n+1 (n = number of non-claim processes). Accepting
// states are those of copy 0; from an accepting state of copy 0 the only
// move is a null step to copy 1; in copy k (1 ≤ k ≤ n) a move of process
// k-1 leads to copy k+1, and a null step to copy k+1 is possible when
// process k-1 has no enabled move in the state; from copy n+1 the only
// move is a null step back to copy 0. A cycle through an accepting state
// therefore passes through every copy, i.e. every process moved or was
// blocked somewhere on it — exactly the runs that are weakly fair. Null
// steps appear in counterexamples as steps of process "-" with a note.
// Strong fairness is not implemented (plan 14 §4.2): the property is
// not-executed with a reason (FR-008).
//
// # Verdicts
//
// acceptance cycle → violated (exhaustive; the lasso is an exact run);
// claim end / claim assert → violated (exhaustive; finite prefix);
// outer search complete without a cycle → verified (exhaustive);
// budget → inconclusive, evidence bounded for the declared states/depth
// limits, unknown for time/memory (plan 14 §6); an evaluation error →
// invalid-model. A system assert that fails during the product search is
// not reported here: the safety search of the same run reports it over
// the whole state space.

// Stats are the counters of one search.
type Stats struct {
	States      int
	Transitions int
	AtomicSteps int
	MaxDepth    int
	MemBytes    int64
	Elapsed     time.Duration
	StateBytes  int
	Complete    bool
	Stop        string
}

// TemporalInfo describes how an ltl / progress property was checked.
type TemporalInfo struct {
	// Source: "formula" (an automaton built from Formula), "never-claim"
	// (the model's own claim), "accept-labels" (no claim: accept labels of
	// the processes), "np" (the non-progress automaton).
	Source           string
	Formula          string
	Negated          string
	Atoms            []string
	StutterInvariant *bool
	AutomatonStates  int
	AutomatonTrans   int
	AutomatonAccept  int
	Fairness         string
	Claim            string // name of the claim process in the trace
}

// FormulaError is returned by Run when an ltl formula cannot be parsed or
// resolved against the model; the input is rejected, nothing is claimed.
type FormulaError struct {
	PropertyID string
	Err        error
}

func (e *FormulaError) Error() string {
	return fmt.Sprintf("property %s: %v", e.PropertyID, e.Err)
}

func (e *FormulaError) Unwrap() error { return e.Err }

const (
	KindLTL      = "ltl"
	KindProgress = "progress"
	// NPClaim is the name of the synthesised non-progress claim process.
	NPClaim = "np_"
)

// strongFairnessReason is the FR-008 text.
const strongFairnessReason = "strong fairness is not executed by this engine version (plan 14 §4.2): only weak fairness (every continuously enabled process eventually moves; pan -f) is implemented by the n+2 copies construction; rerun with fairness weak or none"

// productMove is one transition of the product.
type productMove struct {
	none    bool
	chosen  bool  // a claim edge was just selected; no system move yet
	null    bool  // fairness null step
	stutter bool  // the system has no move: the state repeats (claim alone)
	toCopy  int   // for null steps
	claim   int32 // first claim edge of the claim step, -1 none
	claimTo int   // the claim's location after the claim step
	m       move
}

// claimStepFrom follows the atomic continuation of claim edge first on
// system state sys: it returns the edges taken and, if the sequence hits a
// failing assert or the claim's end, the violation text.
func (cs *cycleSearch) claimStepFrom(sys []byte, first int32) (edges []int32, violation string, err error) {
	procs := cs.s.c.procs[cs.claim]
	e := &procs.edge[first]
	for {
		edges = append(edges, int32(e.idx))
		if e.assert != nil {
			ok, err := e.assert.Truth(sys)
			if err != nil {
				return edges, "", err
			}
			if !ok {
				return edges, "assertion violated in the never claim: assert(" + e.assert.String() + ") is false when the claim takes " + cex.CommandText(e.e) + " — the claim for the negated property detects the violation on this finite prefix (SPIN: assertion violated)", nil
			}
		}
		if cs.cEnd[e.e.To] {
			return edges, "end state in claim reached: the never claim ran to its closing brace after " + cex.CommandText(e.e) + ", which SPIN reports as a violation of the claim (SPIN: end state in claim reached)", nil
		}
		if !e.e.Atomic {
			return edges, "", nil
		}
		var next *cEdge
		for _, oi := range cs.cOut[e.e.To] {
			o := &procs.edge[oi]
			ok, err := o.guard.Truth(sys)
			if err != nil {
				return edges, "", err
			}
			if ok {
				next = o
				break
			}
		}
		if next == nil {
			return edges, "", nil // the atomic sequence blocks: the claim stays here
		}
		e = next
	}
}

type cycleSearch struct {
	s      *search
	prop   int // outcome index
	kind   string
	claim  int // claim process index, -1: accept labels of the system
	cOut   [][]int
	cAcc   []bool
	cEnd   []bool
	sysAcc [][]bool
	fair   bool
	nproc  int
	size   int // layout size
	pl     int // product vector length
	pcur   []byte
	pnext  []byte
	// per stored state
	inner   []bool
	onStack []bool
	// counters
	transitionsSinceTick int
	info                 *TemporalInfo
	res                  *Stats
	found                bool
	loopStates           [][]byte // system states of the last rendered loop
}

// runCycle checks one ltl / progress property and returns its outcome.
func runCycle(ctx0 *search, base *ir.Model, prop ir.Property, propIndex int, opt Options) (Outcome, error) {
	start := time.Now()
	o := Outcome{Property: prop}
	info := &TemporalInfo{Fairness: opt.Fairness, Atoms: []string{}}
	if info.Fairness == "" {
		info.Fairness = "none"
	}
	if opt.Fairness == "strong" {
		o.Status, o.Evidence, o.Reason = NotExecuted, EvUnknown, strongFairnessReason
		o.Temporal = info
		return o, nil
	}
	m := base
	claimName := ""
	switch {
	case prop.Kind == KindProgress:
		mm := *base
		mm.Processes = append(append([]ir.Process(nil), base.Processes...), npClaim(base))
		m = &mm
		claimName = NPClaim
		info.Source = "np"
		info.Claim = NPClaim
	case prop.Formula != "":
		l0, err := ir.NewLayout(base)
		if err != nil {
			return o, err
		}
		claimName = "never:" + prop.ID
		c, err := ltl.ForProperty(claimName, prop.Formula, ltl.Options{Defines: opt.Defines, Scope: l0.Scope(-1)})
		if err != nil {
			return o, &FormulaError{PropertyID: prop.ID, Err: err}
		}
		mm := *base
		mm.Processes = append(append([]ir.Process(nil), base.Processes...), c.Process)
		m = &mm
		si := c.Info.StutterInvariant
		info.Source, info.Formula, info.Negated, info.Atoms = "formula", c.Info.Formula, c.Info.Negated, c.Info.Atoms
		info.StutterInvariant = &si
		info.AutomatonStates, info.AutomatonTrans, info.AutomatonAccept = c.Info.States, c.Info.Transitions, c.Info.Accepting
		info.Claim = claimName
	default:
		for _, p := range base.Processes {
			if p.Claim {
				claimName = p.Name
			}
		}
		if claimName != "" {
			info.Source, info.Claim = "never-claim", claimName
		} else {
			info.Source = "accept-labels"
		}
	}
	c, err := compile(m)
	if err != nil {
		return o, err
	}
	s := &search{c: c, opt: opt, ctx: ctx0.ctx, res: &Result{StateBytes: c.layout.Size}}
	cs := &cycleSearch{s: s, prop: propIndex, kind: prop.Kind, claim: -1, fair: opt.Fairness == "weak", size: c.layout.Size, info: info}
	cs.res = &Stats{StateBytes: c.layout.Size}
	for p := range c.procs {
		if c.procs[p].claim && m.Processes[p].Name == claimName {
			cs.claim = p
		}
		if !c.procs[p].claim {
			cs.nproc++
		}
	}
	if cs.claim >= 0 {
		pr := &m.Processes[cs.claim]
		cs.cOut = c.procs[cs.claim].out
		cs.cAcc = make([]bool, len(pr.Locations))
		cs.cEnd = make([]bool, len(pr.Locations))
		for i, loc := range pr.Locations {
			for _, lb := range loc.Labels {
				if lb == ir.Accept {
					cs.cAcc[i] = true
				}
				if lb == ir.End {
					cs.cEnd[i] = true
				}
			}
		}
	} else {
		for p := range c.procs {
			acc := make([]bool, len(m.Processes[p].Locations))
			if !c.procs[p].claim {
				for i, loc := range m.Processes[p].Locations {
					for _, lb := range loc.Labels {
						if lb == ir.Accept {
							acc[i] = true
						}
					}
				}
			}
			cs.sysAcc = append(cs.sysAcc, acc)
		}
	}
	cs.pl = cs.size
	if cs.fair {
		cs.pl++
		if cs.nproc+2 > 255 {
			return o, fmt.Errorf("weak fairness: %d processes exceed the 253 the copy byte can count", cs.nproc)
		}
	}
	cs.pcur = make([]byte, cs.pl)
	cs.pnext = make([]byte, cs.pl)
	s.cur = cs.pcur[:cs.size]
	s.next = cs.pnext[:cs.size]
	newVisited := opt.NewVisited
	if newVisited == nil {
		newVisited = func(n int) Visited { return NewCompact(n, 1024) }
	}
	s.visited = newVisited(cs.pl)
	// The outcome slot: cs.decide writes into s.res.Outcomes[0].
	s.res.Outcomes = []Outcome{{Property: prop}}
	s.undecided = 1
	cs.run()
	out := s.res.Outcomes[0]
	cs.finish(&out)
	cs.res.Elapsed = time.Since(start)
	out.Stats = cs.res
	out.Temporal = info
	return out, nil
}

// npClaim builds pan's non-progress automaton for the model.
func npClaim(m *ir.Model) ir.Process {
	var conj []*ir.Expr
	for p, pr := range m.Processes {
		if pr.Claim {
			continue
		}
		for i, loc := range pr.Locations {
			for _, lb := range loc.Labels {
				if lb == ir.Progress {
					conj = append(conj, ir.Binary("ne", ir.PC(p), ir.Const(int64(i))))
				}
			}
		}
	}
	var np *ir.Expr
	if len(conj) > 0 {
		np = ir.And(conj...)
	}
	text := "(np_)"
	return ir.Process{
		Name:  NPClaim,
		Claim: true,
		Locations: []ir.Location{
			{Name: "np_0"},
			{Name: "np_1", Labels: []ir.Label{ir.Accept}},
		},
		Edges: []ir.Edge{
			{From: 0, To: 1, Guard: np, Text: text + " -> goto np_1", Origin: &ir.Origin{Name: "np_: no process at a progress label"}},
			{From: 0, To: 0, Text: "(1) -> goto np_0", Origin: &ir.Origin{Name: "true"}},
			{From: 1, To: 1, Guard: np, Text: text + " -> goto np_1", Origin: &ir.Origin{Name: "np_: no process at a progress label"}},
		},
		Origin: &ir.Origin{Name: "np_ (non-progress automaton, SPIN -DNP)"},
	}
}

// accepting reports whether the stored product state is accepting.
func (cs *cycleSearch) accepting(st []byte) bool {
	if cs.fair && st[cs.size] != 0 {
		return false
	}
	return cs.acceptingSys(st[:cs.size])
}

func (cs *cycleSearch) acceptingSys(sys []byte) bool {
	l := cs.s.c.layout
	if cs.claim >= 0 {
		return cs.cAcc[l.ReadPC(sys, cs.claim)]
	}
	for p := range cs.sysAcc {
		if cs.sysAcc[p][l.ReadPC(sys, p)] {
			return true
		}
	}
	return false
}

// blocked reports whether process p has no enabled move in sys.
func (cs *cycleSearch) blocked(p int, sys []byte) (bool, error) {
	ok, err := cs.s.hasEnabled(p, sys)
	return !ok, err
}

// nextProduct advances the frame's iterator over the product moves of st.
func (cs *cycleSearch) nextProduct(f *frame, st []byte) (productMove, error) {
	s := cs.s
	sys := st[:cs.size]
	if cs.fair && f.cpos != -2 {
		k := int(st[cs.size])
		if f.eps == 0 {
			f.eps = 1
			switch {
			case k == 0:
				if cs.acceptingSys(sys) {
					return productMove{null: true, toCopy: 1, claim: -1}, nil
				}
			case k == cs.nproc+1:
				return productMove{null: true, toCopy: 0, claim: -1}, nil
			default:
				b, err := cs.blocked(k-1, sys)
				if err != nil {
					return productMove{}, err
				}
				if b {
					return productMove{null: true, toCopy: k + 1, claim: -1}, nil
				}
			}
		}
		if (k == 0 && cs.acceptingSys(sys)) || k == cs.nproc+1 {
			return productMove{none: true}, nil
		}
	}
	for {
		if cs.claim >= 0 && f.cpos != -2 {
			if f.cedge < 0 {
				loc := s.c.layout.ReadPC(sys, cs.claim)
				outs := cs.cOut[loc]
				if f.cpos < 0 {
					f.cpos = 0
				}
				s.c.layout.Timeout = false
				for int(f.cpos) < len(outs) {
					e := &s.c.procs[cs.claim].edge[outs[f.cpos]]
					f.cpos++
					ok, err := e.guard.Truth(sys)
					if err != nil {
						return productMove{}, err
					}
					if ok {
						f.cedge = int32(e.idx)
						f.proc = -1
						return productMove{chosen: true, claim: f.cedge}, nil
					}
				}
				return productMove{none: true}, nil
			}
			m, ok, err := s.nextEnabled(f, sys)
			if err != nil {
				return productMove{}, err
			}
			if ok {
				f.sysSeen = true
				return productMove{claim: f.cedge, claimTo: int(f.cto), m: m}, nil
			}
			if !f.sysSeen && !f.stut {
				// The system cannot move: stutter extension, once per claim edge.
				f.stut = true
				return productMove{stutter: true, claim: f.cedge, claimTo: int(f.cto)}, nil
			}
			f.cedge, f.stut = -1, false
			continue
		}
		m, ok, err := s.nextEnabled(f, sys)
		if err != nil {
			return productMove{}, err
		}
		if ok {
			f.sysSeen = true
			return productMove{claim: -1, m: m}, nil
		}
		if !f.sysSeen && !f.stut {
			f.stut = true
			return productMove{stutter: true, claim: -1}, nil
		}
		return productMove{none: true}, nil
	}
}

// apply computes cs.pnext from cs.pcur by pm.
func (cs *cycleSearch) apply(pm productMove) error {
	s := cs.s
	if pm.null {
		copy(cs.pnext, cs.pcur)
		cs.pnext[cs.size] = byte(pm.toCopy)
		return nil
	}
	if pm.stutter {
		copy(cs.pnext, cs.pcur)
		if pm.claim >= 0 {
			s.c.layout.WritePC(s.next, cs.claim, pm.claimTo)
		}
		return nil
	}
	if _, err := s.fire(pm.m); err != nil {
		return err
	}
	if pm.claim >= 0 {
		s.c.layout.WritePC(s.next, cs.claim, pm.claimTo)
	}
	if cs.fair {
		k := int(cs.pcur[cs.size])
		cs.pnext[cs.size] = byte(k)
		if k >= 1 && k <= cs.nproc && pm.m.e != nil {
			moved := pm.m.e.proc == k-1 || (pm.m.partner != nil && pm.m.partner.proc == k-1)
			if moved {
				cs.pnext[cs.size] = byte(k + 1)
			}
		}
	}
	return nil
}

func (cs *cycleSearch) load(f *frame) {
	if f.idx < 0 {
		copy(cs.pcur, cs.s.tmp[-int(f.idx)-1])
	} else {
		copy(cs.pcur, cs.s.visited.Get(int(f.idx)))
	}
}

func newFrame(idx int32, pm productMove) frame {
	f := frame{idx: idx, proc: -1, rv: -1, viaProc: -1, viaEdge: -1, viaPart: -1, viaClaim: -1, cpos: -1, cedge: -1}
	switch {
	case pm.null:
		f.viaNull = int8(pm.toCopy + 1)
	case pm.stutter:
		f.viaNull, f.viaClaim = -1, pm.claim
	case pm.m.e != nil:
		f.viaProc, f.viaEdge, f.viaPart, f.viaClaim = int32(pm.m.e.proc), int32(pm.m.e.idx), partnerCode(pm.m), pm.claim
	}
	if idx < 0 {
		f.cpos, f.eps = -2, 1 // intermediate atomic state: no claim step, no null step
	}
	return f
}

// tick checks the time and memory budgets every 1024 transitions.
func (cs *cycleSearch) tick(extra int64) bool {
	cs.transitionsSinceTick++
	if cs.transitionsSinceTick < 1024 {
		return true
	}
	cs.transitionsSinceTick = 0
	s := cs.s
	if s.ctx.Err() != nil {
		s.budget("time budget exhausted")
		return false
	}
	if b := s.opt.Budget; b.MaxMemBytes > 0 {
		est := s.visited.Bytes() + extra
		if est > b.MaxMemBytes {
			s.budget(fmt.Sprintf("memory budget exhausted: estimate %d bytes exceeds %d", est, b.MaxMemBytes))
			return false
		}
	}
	return true
}

func (cs *cycleSearch) store() (int, bool, bool) {
	s := cs.s
	b := s.opt.Budget
	if b.MaxStates > 0 && s.visited.Len() >= b.MaxStates {
		if _, present := s.visited.Has(cs.pnext); present {
			return 0, false, true
		}
		s.budget(fmt.Sprintf("state budget exhausted: %d states stored", s.visited.Len()))
		return 0, false, false
	}
	idx, isNew := s.visited.Add(cs.pnext)
	if isNew {
		cs.inner = append(cs.inner, false)
		cs.onStack = append(cs.onStack, false)
	}
	return idx, isNew, true
}

func (cs *cycleSearch) memExtra() int64 {
	s := cs.s
	return int64(len(s.stack))*frameBytes + int64(len(s.tmp))*int64(cs.pl) + int64(len(cs.inner))*2
}

// run is the outer DFS.
func (cs *cycleSearch) run() {
	s := cs.s
	l := s.c.layout
	init := make([]byte, cs.pl)
	copy(init, l.Initial())
	idx0, _ := s.visited.Add(init)
	cs.inner, cs.onStack = []bool{false}, []bool{true}
	s.stack = append(s.stack, newFrame(int32(idx0), productMove{claim: -1}))
	var istack []frame // the inner stack, kept for the trace of a found cycle

	for len(s.stack) > 0 && s.stop == "" {
		top := &s.stack[len(s.stack)-1]
		cs.load(top)
		pm, err := cs.nextProduct(top, cs.pcur)
		if err != nil {
			s.fail(err.Error(), cs.path(nil, nil, -1))
			break
		}
		if pm.chosen {
			edges, violation, err := cs.claimStepFrom(cs.pcur[:cs.size], pm.claim)
			if err != nil {
				s.fail(err.Error(), cs.path(nil, nil, -1))
				break
			}
			top.cto = int32(s.c.procs[cs.claim].edge[edges[len(edges)-1]].e.To)
			if violation != "" {
				cs.violated(violation, cs.path(&productMove{chosen: true, claim: pm.claim}, nil, -1))
			}
			continue
		}
		if pm.none {
			if top.idx >= 0 {
				if cs.accepting(cs.pcur) && s.stop == "" {
					closing, closeMove, ok, err := cs.innerDFS(int(top.idx), &istack)
					if err != nil {
						s.fail(err.Error(), cs.path(nil, nil, -1))
						break
					}
					if ok {
						cs.foundCycle(closing, istack, closeMove)
					}
					if s.stop != "" {
						break
					}
				}
				cs.onStack[top.idx] = false
			} else {
				s.tmp = s.tmp[:len(s.tmp)-1]
			}
			s.stack = s.stack[:len(s.stack)-1]
			continue
		}
		top.enabled++
		cs.res.Transitions++
		if err := cs.apply(pm); err != nil {
			s.fail(err.Error(), cs.path(&pm, cs.pnext, -1))
			break
		}
		if !cs.tick(cs.memExtra()) {
			break
		}
		depth := len(s.stack)
		if !pm.null && !pm.stutter {
			inter, err := s.intermediate(cs.pnext[:cs.size])
			if err != nil {
				s.fail(err.Error(), cs.path(&pm, cs.pnext, -1))
				break
			}
			if inter {
				cs.res.AtomicSteps++
				if s.opt.Budget.MaxDepth > 0 && depth > s.opt.Budget.MaxDepth {
					s.truncated++
					continue
				}
				if depth > cs.res.MaxDepth {
					cs.res.MaxDepth = depth
				}
				s.tmp = append(s.tmp, append([]byte(nil), cs.pnext...))
				s.stack = append(s.stack, newFrame(-int32(len(s.tmp)), pm))
				continue
			}
		}
		idx, isNew, allowed := cs.store()
		if !allowed {
			break
		}
		if !isNew {
			continue
		}
		cs.res.States = s.visited.Len()
		if s.opt.Budget.MaxDepth > 0 && depth > s.opt.Budget.MaxDepth {
			s.truncated++
			continue
		}
		if depth > cs.res.MaxDepth {
			cs.res.MaxDepth = depth
		}
		cs.onStack[idx] = true
		s.stack = append(s.stack, newFrame(int32(idx), pm))
	}
	cs.res.States = s.visited.Len()
	cs.res.MemBytes = s.visited.Bytes() + int64(cap(s.stack))*frameBytes + int64(len(cs.inner))*2
	if s.stop == "" && len(s.stack) == 0 {
		s.stop = "complete"
	}
}

// innerDFS searches from the seed (the outer top, a stored accepting
// state) for a path back to a state on the outer stack. It returns the
// outer stack position the cycle closes at and the closing move; the
// inner stack is left in istack for the trace.
func (cs *cycleSearch) innerDFS(seed int, istack *[]frame) (closing int, closeMove productMove, found bool, err error) {
	s := cs.s
	*istack = (*istack)[:0]
	*istack = append(*istack, newFrame(int32(seed), productMove{claim: -1}))
	cs.inner[seed] = true
	tmpBase := len(s.tmp)
	defer func() { s.tmp = s.tmp[:tmpBase] }()
	for len(*istack) > 0 && s.stop == "" {
		top := &(*istack)[len(*istack)-1]
		cs.load(top)
		pm, err := cs.nextProduct(top, cs.pcur)
		if err != nil {
			return 0, pm, false, err
		}
		if pm.chosen {
			edges, _, err := cs.claimStepFrom(cs.pcur[:cs.size], pm.claim)
			if err != nil {
				return 0, pm, false, err
			}
			top.cto = int32(s.c.procs[cs.claim].edge[edges[len(edges)-1]].e.To)
			continue
		}
		if pm.none {
			if top.idx < 0 {
				s.tmp = s.tmp[:len(s.tmp)-1]
			}
			*istack = (*istack)[:len(*istack)-1]
			continue
		}
		top.enabled++
		cs.res.Transitions++
		if err := cs.apply(pm); err != nil {
			return 0, pm, false, err
		}
		if !cs.tick(cs.memExtra() + int64(len(*istack))*frameBytes) {
			return 0, pm, false, nil
		}
		depth := len(s.stack) + len(*istack)
		if !pm.null && !pm.stutter {
			inter, err := s.intermediate(cs.pnext[:cs.size])
			if err != nil {
				return 0, pm, false, err
			}
			if inter {
				cs.res.AtomicSteps++
				if s.opt.Budget.MaxDepth > 0 && depth > s.opt.Budget.MaxDepth {
					s.truncated++
					continue
				}
				if depth > cs.res.MaxDepth {
					cs.res.MaxDepth = depth
				}
				s.tmp = append(s.tmp, append([]byte(nil), cs.pnext...))
				*istack = append(*istack, newFrame(-int32(len(s.tmp)), pm))
				continue
			}
		}
		idx, present := s.visited.Has(cs.pnext)
		if !present {
			// Reachable from the seed but not stored by the outer search:
			// only possible when a budget cut the outer expansion short.
			var allowed bool
			idx, _, allowed = cs.store()
			if !allowed {
				return 0, pm, false, nil
			}
			cs.res.States = s.visited.Len()
		}
		if idx == seed || cs.onStack[idx] {
			for i := range s.stack {
				if s.stack[i].idx == int32(idx) {
					return i, pm, true, nil
				}
			}
			return 0, pm, false, fmt.Errorf("internal: closing state %d not on the outer stack", idx)
		}
		if cs.inner[idx] {
			continue
		}
		cs.inner[idx] = true
		if s.opt.Budget.MaxDepth > 0 && depth > s.opt.Budget.MaxDepth {
			s.truncated++
			continue
		}
		if depth > cs.res.MaxDepth {
			cs.res.MaxDepth = depth
		}
		*istack = append(*istack, newFrame(int32(idx), pm))
	}
	return 0, productMove{}, false, nil
}

func (cs *cycleSearch) violated(reason string, tr *cex.Trace) {
	cs.found = true
	cs.s.decide(0, Violated, Exhaustive, reason, tr)
}

// foundCycle renders the lasso: the outer stack up to its top (the seed),
// the inner stack from the seed, and the closing move into outer stack
// state closing.
func (cs *cycleSearch) foundCycle(closing int, istack []frame, closeMove productMove) {
	s := cs.s
	target := s.visited.Get(int(s.stack[closing].idx))
	tr := cs.lasso(closing, istack, closeMove, target)
	var reason string
	switch cs.kind {
	case KindProgress:
		reason = "non-progress cycle: the loop of the counterexample visits no progress label, so the model can run forever without progress (SPIN pan -l: non-progress cycle)"
	default:
		what := "the claim's accept state"
		switch cs.info.Source {
		case "formula":
			what = fmt.Sprintf("an accept state of the automaton for %s", cs.info.Negated)
		case "accept-labels":
			what = "an accept label of a process"
		}
		reason = "acceptance cycle: the loop of the counterexample passes through " + what + " infinitely often, so the run satisfies the negated property (SPIN pan -a: acceptance cycle)"
	}
	if note := cs.unfairNote(tr); note != "" {
		reason += "; " + note
	}
	cs.violated(reason, tr)
}

// unfairNote names, for a cycle found without fairness, the non-claim
// processes that never move in the loop although they have an enabled
// move in every state of it — the reading "the loop starves them".
func (cs *cycleSearch) unfairNote(tr *cex.Trace) string {
	if cs.fair || tr.Loop == nil {
		return ""
	}
	moved := map[string]bool{}
	for _, st := range tr.LoopSteps() {
		moved[st.Process] = true
		if st.Partner != nil {
			moved[st.Partner.Process] = true
		}
	}
	var starved []string
	m := cs.s.c.m
	for p := range m.Processes {
		if m.Processes[p].Claim || moved[m.Processes[p].Name] {
			continue
		}
		always := true
		for _, st := range cs.loopStates {
			ok, err := cs.s.hasEnabled(p, st)
			if err != nil || !ok {
				always = false
				break
			}
		}
		if always {
			starved = append(starved, m.Processes[p].Name)
		}
	}
	sort.Strings(starved)
	if len(starved) == 0 {
		return ""
	}
	return fmt.Sprintf("in the loop only %s move(s); %s is enabled throughout the loop and never moves (the loop is not weakly fair; rerun with fairness weak to exclude such runs)",
		strings.Join(sortedKeys(moved), ", "), strings.Join(starved, ", "))
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- trace rendering ---------------------------------------------------------

// path renders the outer stack, optionally extended by one more move
// (extra) into last; a chosen-only extra renders the claim step alone.
func (cs *cycleSearch) path(extra *productMove, last []byte, _ int) *cex.Trace {
	var states [][]byte
	var refs []cex.Ref
	for i := range cs.s.stack {
		f := &cs.s.stack[i]
		if i == 0 {
			states = append(states, append([]byte(nil), cs.stateOf(f)...))
			continue
		}
		cs.render(&states, &refs, f, cs.stateOf(f))
	}
	if extra != nil {
		prev := states[len(states)-1]
		if extra.chosen {
			cs.claimStep(&states, &refs, prev, extra.claim)
		} else {
			cs.renderMove(&states, &refs, prev, *extra, last)
		}
	}
	return cex.Build(cs.s.c.layout, states, refs)
}

// lasso renders outer stack + inner stack + closing move.
func (cs *cycleSearch) lasso(closing int, istack []frame, closeMove productMove, target []byte) *cex.Trace {
	var states [][]byte
	var refs []cex.Ref
	loopStart := 0
	for i := range cs.s.stack {
		f := &cs.s.stack[i]
		if i == 0 {
			states = append(states, append([]byte(nil), cs.stateOf(f)...))
		} else {
			cs.render(&states, &refs, f, cs.stateOf(f))
		}
		if i == closing {
			loopStart = len(refs)
		}
	}
	for i := 1; i < len(istack); i++ {
		f := &istack[i]
		cs.render(&states, &refs, f, cs.stateOf(f))
	}
	cs.renderMove(&states, &refs, states[len(states)-1], closeMove, target)
	// States of the loop, for the fairness note.
	cs.loopStates = nil
	for _, st := range states[loopStart:] {
		cs.loopStates = append(cs.loopStates, st[:cs.size])
	}
	return cex.BuildLasso(cs.s.c.layout, states, refs, loopStart)
}

func (cs *cycleSearch) stateOf(f *frame) []byte {
	if f.idx < 0 {
		return cs.s.tmp[-int(f.idx)-1]
	}
	return cs.s.visited.Get(int(f.idx))
}

// render appends the steps that lead to frame f's state from the previous
// state in the list.
func (cs *cycleSearch) render(states *[][]byte, refs *[]cex.Ref, f *frame, st []byte) {
	prev := (*states)[len(*states)-1]
	pm := productMove{claim: f.viaClaim}
	switch {
	case f.viaNull > 0:
		pm.null, pm.toCopy = true, int(f.viaNull)-1
	case f.viaNull < 0:
		pm.stutter = true
	default:
		pm.m = move{e: &cs.s.c.procs[f.viaProc].edge[f.viaEdge]}
		if f.viaPart >= 0 {
			pm.m.partner = &cs.s.c.procs[f.viaPart>>16].edge[f.viaPart&0xffff]
		}
	}
	cs.renderMove(states, refs, prev, pm, st)
}

func (cs *cycleSearch) renderMove(states *[][]byte, refs *[]cex.Ref, prev []byte, pm productMove, next []byte) {
	if pm.null {
		note := fmt.Sprintf("(weak fairness: no process moves; copy %d -> %d", int(prev[cs.size]), pm.toCopy)
		switch {
		case pm.toCopy == 1:
			note += ", leaving an accepting state)"
		case pm.toCopy == 0:
			note += ", every process has moved or been blocked once)"
		default:
			note += fmt.Sprintf(", process %s is blocked)", cs.s.c.m.Processes[pm.toCopy-2].Name)
		}
		*refs = append(*refs, cex.Ref{Null: true, Note: note})
		*states = append(*states, append([]byte(nil), next...))
		return
	}
	if pm.claim >= 0 {
		prev = cs.claimStep(states, refs, prev, pm.claim)
	}
	if pm.stutter {
		*refs = append(*refs, cex.Ref{Null: true, Note: "(stutter: no process can move, the system state repeats forever; SPIN's stutter extension)"})
		*states = append(*states, append([]byte(nil), next...))
		return
	}
	*refs = append(*refs, cs.s.ref(int32(pm.m.e.proc), int32(pm.m.e.idx), partnerCode(pm.m)))
	*states = append(*states, append([]byte(nil), next...))
}

// claimStep appends the claim's step from prev — the first claim edge and
// its atomic continuation, each as an intermediate state with the claim's
// new location — and returns the last of them.
func (cs *cycleSearch) claimStep(states *[][]byte, refs *[]cex.Ref, prev []byte, claimEdge int32) []byte {
	edges, _, _ := cs.claimStepFrom(prev[:cs.size], claimEdge)
	cur := prev
	for _, ei := range edges {
		e := &cs.s.c.procs[cs.claim].edge[ei]
		mid := append([]byte(nil), cur...)
		cs.s.c.layout.WritePC(mid, cs.claim, e.e.To)
		*refs = append(*refs, cex.Ref{Proc: cs.claim, Edge: int(ei)})
		*states = append(*states, mid)
		cur = mid
	}
	return cur
}

// finish sets the outcome for an undecided property.
func (cs *cycleSearch) finish(o *Outcome) {
	s := cs.s
	r := cs.res
	r.Complete = s.stop == "complete" && !s.budgetHit && s.truncated == 0 && !s.invalid
	if s.truncated > 0 && !s.invalid {
		reason := fmt.Sprintf("depth budget exhausted: %d state(s) at depth > %d were stored but not expanded", s.truncated, s.opt.Budget.MaxDepth)
		s.budgetHit = true
		if s.stop == "complete" || s.stop == "" {
			s.stop = reason
		}
	}
	r.Stop = s.stop
	if o.Status != "" {
		return
	}
	if r.Complete {
		o.Status, o.Evidence = Verified, Exhaustive
		switch {
		case cs.kind == KindProgress:
			o.Reason = "no non-progress cycle: every cycle of the complete product visits a progress label (SPIN pan -l)"
		case cs.info.Source == "accept-labels" && len(cs.sysAcc) > 0 && !cs.anySysAccept():
			o.Reason = "no never claim and no accept label in the model: no state is accepting, so no acceptance cycle exists (SPIN pan -a)"
		case cs.info.Source == "formula":
			o.Reason = "no acceptance cycle in the complete product with the automaton for " + cs.info.Negated + ": no run satisfies the negated property"
		default:
			o.Reason = "no acceptance cycle in the complete product with the claim (SPIN pan -a)"
		}
		if cs.fair {
			o.Reason += "; under weak fairness"
		}
		return
	}
	o.Status, o.Evidence = Inconclusive, budgetEvidence(s.stop)
	o.Reason = s.stop
	if !s.budgetHit {
		o.Reason = "search stopped early: " + s.stop
	}
}

func (cs *cycleSearch) anySysAccept() bool {
	for _, acc := range cs.sysAcc {
		for _, a := range acc {
			if a {
				return true
			}
		}
	}
	return false
}
