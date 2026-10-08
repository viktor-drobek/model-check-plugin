package explore

import (
	"fmt"
	"strconv"

	"modelcheck/ir"
)

// Partial-order reduction (performance plan, step 2).
//
// Two steps taken by different processes that do not touch the same thing
// can be taken in either order and reach the same state, so a search that
// explores both orders does the same work twice. The reduction explores, in
// a state, the moves of one process alone — an "ample set" — when it can
// prove that nothing the other processes do could matter before those moves
// are made. What it must preserve is every verdict of the safety search
// (deadlock, assert, invariant, reach); the number of states and the first
// counterexample are allowed to change.
//
// Soundness rests on four conditions (Clarke–Grumberg–Peled ch. 10,
// Baier–Katoen ch. 8). The analysis here is deliberately static and
// conservative: when in doubt, a state is expanded in full.
//
//	C0  The ample set is not empty: the process has an enabled move.
//	C1  Nothing dependent on the ample moves can happen before them. The
//	    criterion used: at its current location, no edge of the process (all
//	    of them, enabled or not, with the d_step continuation of each)
//	    conflicts with any edge of any other process, anywhere in that
//	    process. Two edges conflict when one writes a cell the other reads or
//	    writes. A cell is a global variable (one array element when the index
//	    is a constant, else the whole array), a channel, or the program
//	    counter of a process. Every edge writes its own process's program
//	    counter. A process's local variables are not cells: nobody else can
//	    name them. So the other processes can neither enable nor disable any
//	    edge of this one, nor change what its moves do, and its moves can be
//	    made first.
//
//	    One refinement is needed for Promela itself. Processes terminate
//	    youngest first, and the frontend lowers that as a guard on the
//	    `-end-` edge of every older process: pc(j) == dead for each younger
//	    j. Read as a plain read of pc(j) it would make every younger process
//	    dependent on every older one, and nothing would ever be reduced. But
//	    a guard that has pc(j) == c as a top-level conjunct can only be
//	    enabled while j is at c, so it is never enabled together with an edge
//	    of j that starts anywhere else, and the pair is independent. For the
//	    edges of OTHER processes such a guard is therefore a read of "pc(j)
//	    at c", which conflicts only with the edges of j that leave c.
//
//	    The refinement is for the other processes' edges only. The edges of
//	    the process being expanded alone are its own alternatives, so they
//	    are dependent on each other, and another process entering c can
//	    enable one of them: with `f` enabled and `e` guarded by pc(j) == c,
//	    a step of j into c followed by `e` runs a transition dependent on
//	    `f` before `f`, and expanding `f` alone would lose everything behind
//	    `e`. The same holds for the edges a d_step goes on into: which of
//	    them is taken is decided when the step is made. So for the edges
//	    expanded alone and their d_step continuations, and for the edges of a
//	    location that has an else edge or is entered by a d_step, every read
//	    of a program counter reads the whole of it. (The first version of
//	    this analysis applied the refinement to all of them; cross-review
//	    found the models in which that gave a false `verified`, and
//	    explore/por_random_test.go now compares the states without a move.)
//
//	    Everything else that reads a program counter (a property, any other
//	    position in a guard) reads the whole of it. In particular a property
//	    that reads pc(j) makes every edge of j visible, not only those
//	    entering or leaving the location it names; a finer reading would
//	    also have to count the edges that enter it.
//	    Channels. The send end and the receive end of a buffered channel are
//	    separate cells: a send and a receive are independent (see
//	    directedChannels), so a producer and its consumer are not a conflict.
//	    Two sends or two receives share an end and conflict, and every other
//	    use of a channel is a use of the whole of it (its length, a clear, a
//	    channel named by a value), which overlaps both ends. What the other
//	    end can still do to the process expanded alone is ENABLE one of its
//	    alternatives, which C1 forbids: the receiver's pop makes room for a
//	    send that is blocked by a full channel, the sender's push gives a
//	    receive that is blocked by an empty one a message. So an edge that
//	    sends or receives on a buffered channel makes its process expandable
//	    alone only in a state where that edge could act, which the search
//	    checks when it picks (channelsCanAct), not in the static plan. A
//	    receive blocked by a head that does not match its pattern cannot be
//	    enabled by a sender, which writes at the tail. The ends stay whole
//	    for the edges of a location with an else (an else is enabled when its
//	    siblings are not, and a pop or a push flips them), for a d_step edge
//	    and for the locations a d_step enters.
//
//	C2  Invisibility: an edge that writes a cell a property reads (an
//	    invariant or reach expression), or that carries an assert, is never
//	    expanded alone, so a reduced state never hides a change of a
//	    property's value or a failing assert.
//	C3  The cycle proviso. The moves of a process that is always eligible
//	    could be taken forever and the others never: a loop of local steps.
//	    A state uses its ample set only if none of its successors is on the
//	    depth-first search stack; otherwise it is expanded in full. Every
//	    cycle of the reduced graph then contains a fully expanded state.
//
// Atomic sequences (performance plan, step 6). The unit that is commuted is
// the macro-step: what a process does from a stored state to the next stored
// state, which is one edge, or an edge with its d_step continuation, or an
// atomic sequence (the process keeps exclusive control after an Atomic edge and
// goes on with an edge of its own; the states inside the sequence are not
// stored). Which edges a macro-step is made of is decided when it is made, by
// guards, channel conditions and else siblings, so
//
//   - the footprint of a macro-step is the union over every edge out of every
//     location the sequence can pass, enabled or not (closure): after an Atomic
//     edge as after a DStep one;
//   - a location that an atomic edge enters is read whole, like one a d_step
//     enters (a guard on a program counter there is a read of the whole
//     counter), and the channel operations of the edge and of those locations
//     are whole-channel cells, not the separate ends of a directed channel: a
//     continuation `c!x` is enabled by the receiver's pop and blocked by its
//     absence, which changes where the sequence ends;
//   - the cycle proviso follows each move through the sequence, along every
//     branch, to the stored states it can end in, and compares them exactly with
//     the stack (leavesStack). A chain longer than a limit, or more micro-steps
//     in one pick than a budget, counts as not leaving: full expansion.
//
// The exclusive byte is what makes two orders of commuting macro-steps differ:
// the step that comes last decides it. The byte is read in two places only, to
// restrict the moves to a holder that can move and to decide that a state is
// inside a sequence; a stored state has no holder that can move, so two stored
// states that differ only in the byte have the same moves, effects, property
// values and successors up to the byte (an equivalence the oracles use: the
// states without a move are compared up to it). The proof that the reduction is
// sound runs on exact states, with that equivalence to carry a path past a
// commutation; the cycle proviso is a property of the exact reduced graph.
//
// Process creation and the process table (performance plan, step 6). The table
// of live processes (`_nr_pr`, a runtime pid, which process is the youngest and
// may leave) is one cell, T: the processes are entered and left in an order that
// is part of the state, so any two writers conflict and every reader is enabled
// or disabled by any of them. Every `run` and every edge that leaves the table
// write T; `nrpr`, `pid` and `youngest` read it, wherever they stand (a guard,
// an effect, the arguments of a `run`, a property: a property that reads it makes
// every `run` and every end visible). A `run` also reads and writes the program
// counter of each target of its pool at the dormant location (it takes the first
// dormant slot, and it is what enables a guard on the counter of a process that
// has no edge of its own), reads the cells of its arguments (in the creator's
// scope) and of its initialisers (in the target's), and writes nothing at the
// entry location: a guard there is enabled by the run and never together with
// it. The locals of the target are not cells. Two checks refuse what the
// Promela frontend never emits: a dynamic process that goes back to its dormant
// location without leaving the table (the slot would become free with no write
// that the `run` is ordered against), and a `run` that enters its target at
// the dormant location. The consequence for frontend output: when a model
// creates processes every process's end edge leaves the table, so the creating
// step is itself expanded in full and the gain comes from the other processes.
//
// Out of this version, refused with a reason that goes into the report:
// rendezvous channels (a handshake is one step of two processes: measured, no
// model has two independent pairs, so a rule would have a proof and no gain),
// channels named by a value (a dynamic channel: the dependence relation would
// depend on the state; postponed), `timeout` (postponed), `provided` (a guard
// of every edge of the process; refused although the argument is short, because
// it gained nothing), breadth-first search, and every temporal property (the
// reduction preserves the safety properties only, and a vacuity watch counts
// states).

// Reduction is what a reduced run records in the report.
type Reduction struct {
	Kind string `json:"kind"`
	// Applied is false when the reduction was asked for but the search ran
	// in full; Reason then says why.
	Applied bool   `json:"applied"`
	Reason  string `json:"reason,omitempty"`
	// ReducedStates is the number of stored states expanded through an
	// ample set, FullStates the number expanded in full (no eligible
	// process, or the cycle proviso).
	ReducedStates int `json:"reduced_states"`
	FullStates    int `json:"fully_expanded_states"`
	// Note tells a reader that the state and transition counts of the report
	// are those of the reduced graph.
	Note string `json:"note,omitempty"`
}

const (
	porKind = "partial-order"
	porNote = "states and transitions are those of the reduced graph, not of the whole reachable graph; the verdicts of the safety properties are the same, the first counterexample may differ; under any budget (depth, states, time or memory) either search can answer inconclusive where the other decides, and neither contradicts a search that completes"
)

type porCellKind uint8

const (
	cellVar   porCellKind = iota // a global variable (or one element of an array)
	cellChan                     // a channel, by name; "*" is any channel
	cellPC                       // the program counter of a process, by index
	cellTable                    // the live-process table: its count and its order
)

// porCell is something an edge can read or write. elem is the element of an
// array variable, or -1 for a scalar or for any element.
type porCell struct {
	kind porCellKind
	name string
	elem int
}

// porTable is the live-process table, one cell: a `run` enters it and the end
// of a process leaves it, both in an order that is part of the state, so any
// two writers conflict, and `_nr_pr`, a pid, and "is this the youngest process"
// read all of it.
var porTable = porCell{cellTable, "T", -1}

type porKey struct {
	kind porCellKind
	name string
}

type porElems struct {
	whole bool
	elems map[int]bool
}

// porSet is a set of cells with overlap queries.
type porSet struct {
	m        map[porKey]*porElems
	anyChan  bool // holds the wildcard channel
	hasChan  bool // holds some channel
	nonempty bool
}

func newPorSet() *porSet { return &porSet{m: map[porKey]*porElems{}} }

func (s *porSet) add(c porCell) {
	s.nonempty = true
	if c.kind == cellChan {
		s.hasChan = true
		if c.name == "*" {
			s.anyChan = true
		}
	}
	k := porKey{c.kind, c.name}
	e := s.m[k]
	if e == nil {
		e = &porElems{elems: map[int]bool{}}
		s.m[k] = e
	}
	if c.elem < 0 {
		e.whole = true
	} else {
		e.elems[c.elem] = true
	}
}

// hits reports whether c overlaps a cell of the set.
func (s *porSet) hits(c porCell) bool {
	if c.kind == cellChan {
		if c.name == "*" {
			return s.hasChan
		}
		if s.anyChan {
			return true
		}
	}
	e := s.m[porKey{c.kind, c.name}]
	if e == nil {
		return false
	}
	return e.whole || c.elem < 0 || e.elems[c.elem]
}

// porFoot is what an edge (with its d_step continuation) reads and writes.
type porFoot struct {
	reads, writes []porCell
	assert        bool
}

func (f *porFoot) merge(o *porFoot) {
	f.reads = append(f.reads, o.reads...)
	f.writes = append(f.writes, o.writes...)
	f.assert = f.assert || o.assert
}

// porPlan is the result of the static analysis.
type porPlan struct {
	// reason is non-empty when the reduction cannot be applied to this model
	// and property set; eligible is then nil.
	reason string
	// eligible[p][l]: process p, at location l, may be expanded alone (C1
	// and C2 hold for every edge out of l).
	eligible [][]bool
	// any: some process is eligible at some location.
	any bool
	// req[p][l]: the channel operations among the out-edges of p at l that
	// the plan treats as the end of a directed channel; pick expands p alone
	// there only while each has room (a send) or a message (a receive).
	req [][][]porChanReq
}

// porChanReq is one such operation.
type porChanReq struct {
	ch   int  // channel index
	send bool // a send needs room, a receive needs a message
}

type porAnalysis struct {
	c      *compiled
	reason string
	// directed: the channels whose send end and receive end are separate
	// cells (see directedChannels).
	directed map[string]bool
}

func (a *porAnalysis) refuse(format string, args ...any) {
	if a.reason == "" {
		a.reason = fmt.Sprintf(format, args...)
	}
}

// cell resolves a variable name seen from process proc (-1: property scope).
// ok is false for a process's local variable, which no other process names.
func (a *porAnalysis) cell(name string, proc, elem int) (porCell, bool) {
	sl := a.c.layout.Resolve(name, proc)
	if sl == nil || sl.Proc >= 0 {
		return porCell{}, false
	}
	if sl.Var.Len == 0 {
		elem = -1
	}
	return porCell{cellVar, name, elem}, true
}

func constElem(e *ir.Expr) int {
	if e != nil && e.Op == "const" && e.Value >= 0 && e.Value < 1<<30 {
		return int(e.Value)
	}
	return -1
}

// reads adds the cells expression e reads, as seen from process proc.
func (a *porAnalysis) reads(e *ir.Expr, proc int, f *porFoot) {
	if e == nil {
		return
	}
	switch e.Op {
	case "var":
		if c, ok := a.cell(e.Var, proc, -1); ok {
			f.reads = append(f.reads, c)
		}
	case "index":
		if len(e.Args) == 1 {
			if c, ok := a.cell(e.Var, proc, constElem(e.Args[0])); ok {
				f.reads = append(f.reads, c)
			}
		}
	case "len":
		f.reads = append(f.reads, porCell{cellChan, e.Var, -1})
	case "clen", "cfull":
		f.reads = append(f.reads, porCell{cellChan, "*", -1})
	case "pc":
		f.reads = append(f.reads, porCell{cellPC, strconv.FormatInt(e.Value, 10), -1})
	case "timeout":
		a.refuse("the model uses timeout, which reads whether any other process can move")
	case "nrpr", "pid", "youngest":
		f.reads = append(f.reads, porTable)
	}
	for _, x := range e.Args {
		a.reads(x, proc, f)
	}
}

// guardReads adds what an edge's guard reads. A top-level conjunct
// pc(j) == c is read as "j at c" (see the C1 note above) unless whole is set,
// which it is for the siblings of an else edge: an else is enabled when they
// are all false, so a step of j that makes pc(j) == c true disables it, and
// that step does not leave c.
func (a *porAnalysis) guardReads(g *ir.Expr, proc int, f *porFoot, whole bool) {
	if g == nil {
		return
	}
	if !whole {
		switch g.Op {
		case "and":
			for _, x := range g.Args {
				a.guardReads(x, proc, f, whole)
			}
			return
		case "eq":
			if len(g.Args) == 2 {
				l, r := g.Args[0], g.Args[1]
				if l.Op == "const" {
					l, r = r, l
				}
				if l.Op == "pc" && r.Op == "const" && r.Value >= 0 && r.Value < 1<<30 {
					f.reads = append(f.reads, porCell{cellPC, strconv.FormatInt(l.Value, 10), int(r.Value)})
					return
				}
			}
		}
	}
	a.reads(g, proc, f)
}

func (a *porAnalysis) writeVar(name string, index *ir.Expr, proc int, f *porFoot) {
	if c, ok := a.cell(name, proc, constElem(index)); ok {
		f.writes = append(f.writes, c)
	}
}

// edge computes the footprint of one edge of process p, without its d_step
// continuation.
func (a *porAnalysis) edge(p int, e *ir.Edge, whole, dirOK bool) *porFoot {
	m := a.c.m
	f := &porFoot{}
	// The edge leaves e.From, which is where it is enabled.
	f.writes = append(f.writes, porCell{cellPC, strconv.Itoa(p), e.From})
	a.guardReads(e.Guard, p, f, whole)
	if e.Assert != nil {
		f.assert = true
		a.reads(e.Assert, p, f)
	}
	if e.Leave {
		f.writes = append(f.writes, porTable)
	}
	if e.Run != nil {
		a.run(p, e.Run, f)
	}
	for _, ci := range e.ClearChans {
		f.writes = append(f.writes, porCell{cellChan, m.Channels[ci].Name, -1})
	}

	for _, as := range e.Effect {
		a.reads(as.Index, p, f)
		a.reads(as.Value, p, f)
		a.writeVar(as.Var, as.Index, p, f)
	}
	if e.Send != nil {
		a.channelOp(e.Send.Sel, e.Send.Chan, true, dirOK, f)
		for _, x := range e.Send.Args {
			a.reads(x, p, f)
		}
	}
	if e.Recv != nil {
		a.channelOp(e.Recv.Sel, e.Recv.Chan, false, dirOK, f)
		for _, ra := range e.Recv.Args {
			a.reads(ra.Match, p, f)
			a.reads(ra.Index, p, f)
			if ra.Var != "" {
				a.writeVar(ra.Var, ra.Index, p, f)
			}
		}
	}
	return f
}

// run adds what the `run` of process p reads and writes. It pushes the new
// process on the table, takes the first dormant slot of its pool (so it reads
// whether each slot is dormant, which is the program counter at the dormant
// location), moves that counter to the entry (a write of the same cell: a guard
// of another process on the counter is enabled or disabled by it, and for a
// process with no edge of its own that is the only write there is), and
// evaluates its arguments in the creator's scope and its initialisers in the
// target's. Nothing is written at the entry location: a guard on it is enabled
// by the run and is never enabled together with it. The locals of the target
// are not cells. An initialiser may nevertheless assign a GLOBAL (the frontend
// never emits one, but IR written by hand may, and so may a heterogeneous pool,
// whose targets resolve the same name in different scopes): that is a write of
// the global, resolved in each target's own scope, and two directed tests pin it.
func (a *porAnalysis) run(p int, r *ir.RunOp, f *porFoot) {
	f.writes = append(f.writes, porTable)
	for _, q := range r.Targets() {
		dormant := porCell{cellPC, strconv.Itoa(q), a.c.m.Processes[q].Initial}
		f.reads = append(f.reads, dormant)
		f.writes = append(f.writes, dormant)
		for _, as := range r.Init {
			a.reads(as.Index, q, f)
			a.reads(as.Value, q, f)
			a.writeVar(as.Var, as.Index, q, f) // a local is not a cell; a global is
		}
	}
	for _, x := range r.Args {
		a.reads(x, p, f)
	}
}

func (a *porAnalysis) channelOp(sel *ir.Expr, name string, send, dirOK bool, f *porFoot) {
	if sel != nil {
		a.refuse("the model names a channel by a value (a dynamic channel)")
		f.writes = append(f.writes, porCell{cellChan, "*", -1})
		return
	}
	for i := range a.c.m.Channels {
		if a.c.m.Channels[i].Name == name && a.c.m.Channels[i].Capacity == 0 {
			a.refuse("the model has a rendezvous channel (%s): a handshake moves two processes in one step", name)
		}
	}
	elem := -1 // the whole channel
	if dirOK && a.directed[name] {
		elem = 1 // the receive end
		if send {
			elem = 0 // the send end
		}
	}
	f.writes = append(f.writes, porCell{cellChan, name, elem})
}

// closure is the footprint of taking edge ei of process p as one step: the
// edge, and while the edge taken carries DStep or Atomic, any edge out of its
// target location (which of them is taken depends on the state). It is the
// footprint of a macro-step, the unit the reduction commutes: after an atomic
// edge the process keeps exclusive control and goes on with an edge of its own,
// so the sequence up to the next state the search stores is one step, and the
// edges it can be made of are all the edges out of the locations it passes. Every
// one of them counts, enabled or not: whether it is enabled decides where the
// sequence ends.
func (a *porAnalysis) closure(p, ei int, foot [][]*porFoot, seen map[[2]int]bool, into *porFoot) {
	key := [2]int{p, ei}
	if seen[key] {
		return
	}
	seen[key] = true
	into.merge(foot[p][ei])
	cp := &a.c.procs[p]
	if e := cp.edge[ei].e; !e.DStep && !e.Atomic {
		return
	}
	for _, oi := range cp.out[cp.edge[ei].e.To] {
		a.closure(p, oi, foot, seen, into)
	}
}

// directedChannels finds the channels whose two ends are separate cells:
// every buffered channel. A send and a receive on a channel are independent
// (below), so a send is a write of the send end and a receive a write of the
// receive end. Two sends, or two receives, share an end and conflict, and
// every other way to touch a channel is a use of the whole channel, which
// overlaps both ends: the length read by a guard, an assert, an effect or a
// property, a channel named by a value, a clear. So nothing has to be
// checked here about how many processes send or receive, or about lengths:
// the cells do it.
//
// Why a send and a receive are independent. When both are enabled the buffer
// is neither full nor empty. The send appends at the tail and the receive
// removes the head and shifts the rest down, zeroing the freed slot, so the
// buffer is the same bytes in either order; neither disables the other (the
// send leaves the head alone, which is all a Match looks at, and the receive
// only makes room); and the variables one binds and the other reads are
// cells of their own. When they are not both enabled there is nothing to
// commute.
func (a *porAnalysis) directedChannels() map[string]bool {
	out := map[string]bool{}
	for i := range a.c.m.Channels {
		// A rendezvous channel (capacity 0) is refused before this matters, in
		// channelOp; the test here is for the day that refusal is lifted, when
		// a handshake would not be independent of anything.
		if a.c.m.Channels[i].Capacity > 0 {
			out[a.c.m.Channels[i].Name] = true
		}
	}
	return out
}

// analyzePOR decides whether the reduction applies to the compiled model and
// its properties, and where a process may be expanded alone.
func analyzePOR(c *compiled) *porPlan {
	a := &porAnalysis{c: c}
	m := c.m
	for i := range m.Properties {
		switch m.Properties[i].Kind {
		case ir.KindLTL, ir.KindProgress, ir.KindCTL:
			return &porPlan{reason: fmt.Sprintf("the property %s is temporal (%s); the reduction preserves the safety properties only", m.Properties[i].ID, m.Properties[i].Kind)}
		}
	}
	for p := range m.Processes {
		pr := &m.Processes[p]
		if pr.Provided != nil {
			return &porPlan{reason: fmt.Sprintf("the process %s has a provided clause, which guards every step of the process and which this version of the reduction does not model", pr.Name)}
		}
		// Two checks on process creation. The first: a dynamic process becomes
		// dormant again, and so a free slot of its pool, only through an edge
		// that leaves the table, which writes the table and so is ordered
		// against a `run`; the Promela frontend always does this. The second: a
		// `run` that enters its target at the dormant location would leave a
		// live process that looks dormant, and a second `run` would take it.
		if pr.Dynamic {
			for ei := range pr.Edges {
				if e := &pr.Edges[ei]; e.To == pr.Initial && !e.Leave {
					return &porPlan{reason: fmt.Sprintf("the dynamic process %s re-enters its dormant location without leaving the process table", pr.Name)}
				}
			}
		}
		for ei := range pr.Edges {
			if e := &pr.Edges[ei]; e.Run != nil {
				for _, q := range e.Run.Targets() {
					if e.Run.Entry == m.Processes[q].Initial {
						return &porPlan{reason: fmt.Sprintf("a run of %s enters it at its dormant location", m.Processes[q].Name)}
					}
				}
			}
		}
	}

	a.directed = a.directedChannels()

	// Footprints of every edge of every non-claim process, in two readings.
	// own: what the edge reads and writes when it is one of the edges
	// expanded alone, program counters read whole. foot: what the other
	// processes see of it, a guard pc(j) == c read as "j at c" unless the
	// edge's location has an else edge or is entered by a d_step.
	foot := make([][]*porFoot, len(m.Processes))
	own := make([][]*porFoot, len(m.Processes))
	for p := range m.Processes {
		foot[p] = make([]*porFoot, len(m.Processes[p].Edges))
		own[p] = make([]*porFoot, len(m.Processes[p].Edges))
		if m.Processes[p].Claim {
			continue
		}
		whole := map[int]bool{} // locations whose edges are read whole by the others
		for ei := range m.Processes[p].Edges {
			e := &m.Processes[p].Edges[ei]
			if e.Else {
				whole[e.From] = true // the siblings of an else
			}
			if e.DStep || e.Atomic {
				whole[e.To] = true // the continuation is chosen when the step is made
			}
		}
		for ei := range m.Processes[p].Edges {
			e := &m.Processes[p].Edges[ei]
			// The ends of a directed channel are separate cells only for an
			// edge that is neither a d_step nor atomic nor in a location with
			// an else or one a d_step or an atomic edge enters (see the header).
			dirOK := !whole[e.From] && !e.DStep && !e.Atomic
			foot[p][ei] = a.edge(p, e, whole[e.From], dirOK)
			own[p][ei] = a.edge(p, e, true, dirOK)
		}
	}
	// What a property reads is visible. A property that was refused (it reads
	// the process table of a model that keeps none, tableread.go) is not
	// evaluated and reads nothing: it must not refuse the reduction for the
	// table, which only a process that reads it does.
	vis := newPorSet()
	for i := range m.Properties {
		pr := &m.Properties[i]
		if pr.Kind != ir.KindInvariant && pr.Kind != ir.KindReach {
			continue
		}
		if _, refused := c.refused[i]; refused {
			continue
		}
		f := &porFoot{}
		a.reads(pr.Expr, -1, f)
		for _, r := range f.reads {
			vis.add(r)
		}
	}
	if a.reason != "" {
		return &porPlan{reason: a.reason}
	}

	// What every process, anywhere, reads or writes.
	w := make([]*porSet, len(m.Processes))
	rw := make([]*porSet, len(m.Processes))
	for p := range m.Processes {
		w[p], rw[p] = newPorSet(), newPorSet()
		for _, f := range foot[p] {
			if f == nil {
				continue
			}
			for _, x := range f.writes {
				w[p].add(x)
				rw[p].add(x)
			}
			for _, x := range f.reads {
				rw[p].add(x)
			}
		}
	}

	plan := &porPlan{eligible: make([][]bool, len(m.Processes)), req: make([][][]porChanReq, len(m.Processes))}
	for p := range m.Processes {
		cp := &c.procs[p]
		plan.eligible[p] = make([]bool, len(m.Processes[p].Locations))
		plan.req[p] = make([][]porChanReq, len(m.Processes[p].Locations))
		if cp.claim {
			continue
		}
		for l := range cp.out {
			u := &porFoot{}
			seen := map[[2]int]bool{}
			for _, ei := range cp.out[l] {
				a.closure(p, ei, own, seen, u)
			}
			// A location without an edge has nothing to expand; counting it
			// as eligible would make plan.any true for every Promela model,
			// whose processes all end in one, and the cheap exit in pick
			// would never be taken.
			ok := !u.assert && len(cp.out[l]) > 0
			for _, x := range u.writes {
				if vis.hits(x) {
					ok = false
				}
			}
			for q := range m.Processes {
				if !ok {
					break
				}
				if q == p || c.procs[q].claim {
					continue
				}
				for _, x := range u.writes {
					if rw[q].hits(x) {
						ok = false
						break
					}
				}
				for _, x := range u.reads {
					if ok && w[q].hits(x) {
						ok = false
						break
					}
				}
			}
			plan.eligible[p][l] = ok
			plan.any = plan.any || ok
			if ok {
				// The directed channel operations among the edges of this
				// location: pick expands p alone only while each can act.
				// A channel operation read whole (an else location, a d_step
				// edge or target) overlaps the other end, wherever that is, so
				// the location is not eligible, unless nobody else uses the
				// channel; and then it is recorded here too, which can only
				// make the process wait for a state in which it could act.
				for _, ei := range cp.out[l] {
					e := &m.Processes[p].Edges[ei]
					if e.Send != nil && a.directed[e.Send.Chan] {
						ci, _ := c.layout.ChanIndex(e.Send.Chan)
						plan.req[p][l] = append(plan.req[p][l], porChanReq{ci, true})
					}
					if e.Recv != nil && a.directed[e.Recv.Chan] {
						ci, _ := c.layout.ChanIndex(e.Recv.Chan)
						plan.req[p][l] = append(plan.req[p][l], porChanReq{ci, false})
					}
				}
			}
		}
	}
	return plan
}

// ---- the search side --------------------------------------------------------

// porRun is the state of the reduction during one depth-first search.
type porRun struct {
	plan      *porPlan
	onStack   []uint64 // bit i: the stored state with index i is on the DFS stack
	noProviso bool
	// trace is nil unless a test sets it (Options.porTrace).
	trace func(state []byte, ample uint8)
	// chainLimit and pickBudget override the limits of the chain walk of the
	// cycle proviso (zero: the defaults). Tests only.
	chainLimit, pickBudget int
	bufs                   [][]byte // scratch states of the chain walk, by depth
	reduced                int
	full                   int
}

func (r *porRun) mark(idx int, on bool) {
	w := idx >> 6
	for w >= len(r.onStack) {
		r.onStack = append(r.onStack, 0)
	}
	if on {
		r.onStack[w] |= 1 << (uint(idx) & 63)
	} else {
		r.onStack[w] &^= 1 << (uint(idx) & 63)
	}
}

func (r *porRun) on(idx int) bool {
	w := idx >> 6
	return w < len(r.onStack) && r.onStack[w]&(1<<(uint(idx)&63)) != 0
}

// pick chooses the process the state in s.cur is expanded through, as
// process index + 1 (at most 254: a model has at most 254 processes, see
// ir.Validate, so it fits the uint8 of frame.ample), or 0 for a full
// expansion. The first eligible process
// with an enabled move whose moves lead off the DFS stack is taken.
func (r *porRun) pick(s *search) uint8 {
	p := r.choose(s)
	if r.trace != nil {
		r.trace(s.cur, p)
	}
	return p
}

func (r *porRun) choose(s *search) uint8 {
	if !r.plan.any {
		r.full++
		return 0
	}
	l := s.c.layout
	// One budget for the whole pick: every candidate's chain walk spends from it.
	budget := r.budgetMax()
	for p := range s.c.procs {
		if s.c.procs[p].claim {
			continue
		}
		loc := l.ReadPC(s.cur, p)
		if !r.plan.eligible[p][loc] || !r.channelsCanAct(s, r.plan.req[p][loc]) {
			continue
		}
		moves, ok := r.movesOf(s, p)
		if !ok {
			break // an evaluation error: let the full search meet it
		}
		if len(moves) == 0 {
			continue
		}
		if r.noProviso || r.leavesStack(s, moves, &budget) {
			r.reduced++
			return uint8(p + 1)
		}
	}
	r.full++
	return 0
}

// channelsCanAct is the state-dependent half of the rule for directed
// channels: every directed send among the edges of the location has room and
// every directed receive finds a message. Otherwise the other end can enable
// an edge of this process that is disabled now, and that edge is an
// alternative of the ones expanded alone, so it would be run before them.
// (A receive that is disabled only because the head does not match its
// pattern cannot be enabled by the sender, which writes at the tail, and the
// channel being non-empty is all this needs to check.)
func (r *porRun) channelsCanAct(s *search, reqs []porChanReq) bool {
	l := s.c.layout
	for _, q := range reqs {
		n := l.ChanLen(s.cur, q.ch)
		if q.send && n >= l.Chans[q.ch].Chan.Capacity {
			return false
		}
		if !q.send && n == 0 {
			return false
		}
	}
	return true
}

// movesOf lists the enabled moves of process p in s.cur.
func (r *porRun) movesOf(s *search, p int) ([]move, bool) {
	f := frame{proc: -1, rv: -1, ample: uint8(p + 1)}
	var out []move
	for {
		m, ok, err := s.nextEnabled(&f, s.cur)
		if err != nil {
			return nil, false
		}
		if !ok {
			return out, true
		}
		out = append(out, m)
	}
}

// The limits of the chain walk of the cycle proviso. A chain longer than
// porChainLimit micro-steps (the limit of a d_step), or more than porPickBudget
// micro-steps fired in one pick (a wide `if` inside an atomic block multiplies
// the chains), is not followed any further: the state is expanded in full, as
// for an error. Neither limit can lose a verdict, because a full expansion is
// the ordinary search; the second keeps pick from dominating the search.
const (
	porChainLimit = dstepLimit
	porPickBudget = 10000
)

func (r *porRun) chainMax() int {
	if r.chainLimit > 0 {
		return r.chainLimit
	}
	return porChainLimit
}

func (r *porRun) budgetMax() int {
	if r.pickBudget > 0 {
		return r.pickBudget
	}
	return porPickBudget
}

// leavesStack is the cycle proviso (C3): no move leads to a state that is on
// the DFS stack. A move that starts an atomic sequence is followed through the
// sequence, along every branch, to the stored states it can end in: those are
// the successors in the graph the search builds (the states inside a sequence
// are not stored), and the stored states are compared exactly, byte for byte.
// A move that cannot be fired cleanly (an error, a failing assert) or a chain
// that exceeds a limit counts as not leaving the stack, so the state is expanded
// in full and the ordinary search reports whatever it is. The budget of
// micro-steps is the pick's, shared by every candidate process it asks about
// (choose allocates it once).
func (r *porRun) leavesStack(s *search, moves []move, budget *int) bool {
	for _, m := range moves {
		if !r.chainLeaves(s, s.cur, m, 0, budget) {
			return false
		}
	}
	return true
}

// chainLeaves fires m from the state from and follows the chain it starts: true
// when every stored state it can end in is off the stack. s.cur is put aside
// while a micro-step is fired from a state inside a sequence, because fire
// reads it.
func (r *porRun) chainLeaves(s *search, from []byte, m move, depth int, budget *int) bool {
	if *budget--; *budget < 0 || depth > r.chainMax() {
		return false
	}
	cur := s.cur
	s.cur = from
	failed, err := s.fire(m)
	s.cur = cur
	if err != nil || failed != nil {
		return false
	}
	inter, err := s.intermediate(s.next)
	if err != nil {
		return false
	}
	if !inter {
		idx, ok := s.visited.Has(s.next)
		return !(ok && r.on(idx))
	}
	// Inside a sequence: the holder's moves are the only ones, and each is a
	// branch of the chain.
	here := r.buffer(depth, len(s.next))
	copy(here, s.next)
	f := frame{proc: -1, rv: -1}
	for {
		m2, ok, err := s.nextEnabled(&f, here)
		if err != nil {
			return false
		}
		if !ok {
			return true
		}
		if !r.chainLeaves(s, here, m2, depth+1, budget) {
			return false
		}
	}
}

// buffer is the scratch state of the chain walk at a depth, kept between picks.
func (r *porRun) buffer(depth, size int) []byte {
	for len(r.bufs) <= depth {
		r.bufs = append(r.bufs, make([]byte, size))
	}
	return r.bufs[depth]
}
