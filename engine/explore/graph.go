package explore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"modelcheck/cex"
	"modelcheck/ir"
)

// Reachable-graph construction (G5). CTL labelling needs what the on-the-fly
// searches of G0/G4 deliberately avoid: the whole reachable graph *with its
// successor lists*, held at once, so that a fixed point can be computed
// backwards over it. Graph builds exactly that.
//
// The stored states are the stored states of the safety search — the same
// rule, so the same count: intermediate states of an `atomic` sequence are
// expanded but not stored (G1), and every stored state is a state pan would
// count under `-c0`. A successor of a stored state is a stored state reached
// by one move, or by a chain of moves through intermediate atomic states;
// the chain is kept so that a CTL witness can be rendered as a run.
//
// Totality. CTL is interpreted over a transition relation that is total:
// `EX true` must hold everywhere, and `EG f` on a run that simply stops
// would otherwise be undefined. A state with no enabled move — a deadlock,
// or a system in which every process has terminated — therefore gets a
// self-loop, which is the same stutter extension the LTL product uses
// (cycle.go). Graph.Stuttered lists those states so that the report can say
// it. The self-loop does not change the state count.
//
// Never claims are not executed here: a claim process is skipped exactly as
// the safety search skips it. A CTL property is a question about the model,
// not about a claim.

// Graph is the fully stored reachable graph of a model.
type Graph struct {
	Layout *ir.Layout
	// Visited holds the states; index i of Succ refers to Visited.Get(i).
	Visited Visited
	// Succ[i] lists the distinct successors of state i, ascending by first
	// discovery. Every state has at least one successor (see Totality).
	Succ [][]int32
	// chain[i][k] is the sequence of moves from state i to Succ[i][k], as
	// the persistent node of the descent that found it (the nodes of one
	// descent share their prefixes, issue #7); it has more than one move
	// only when the step passes through intermediate states of an atomic
	// sequence, and is nil for a stutter self-loop. chainBetween reads the
	// references off it for a trace.
	chain [][]*chainNode
	// Pred[i] lists the predecessors of i (built on demand by BuildPred).
	Pred [][]int32
	// Stuttered marks the states whose only successor is the added self-loop.
	Stuttered []bool
	// Stats are the counters of the construction.
	Stats *Stats
	// Invalid, when set, is the model error that stopped the construction
	// (domain overflow, blocking d_step …) with the run to the offending step.
	Invalid      string
	InvalidTrace *cex.Trace

	s       *search
	edgeMem int64

	parallel graphParallelStats
}

type graphParallelStats struct {
	workers     int
	layers      int
	maxLayer    int
	maxScratch  int
	workerBytes int64
}

// BuildGraph explores m completely and returns its reachable graph. It
// returns an error only when the model cannot be compiled; a budget that
// stops the construction leaves Stats.Complete false, and a model error
// leaves Invalid set — in both cases the caller must not claim a verdict.
func BuildGraph(ctx context.Context, m *ir.Model, opt Options) (*Graph, error) {
	start := time.Now()
	c, err := compile(m)
	if err != nil {
		return nil, err
	}
	newVisited := opt.NewVisited
	if newVisited == nil {
		newVisited = defaultVisited
	}
	s := &search{c: c, opt: opt, ctx: ctx, visited: newVisited(c.layout.Size), res: &Result{StateBytes: c.layout.Size}}
	// The graph carries no properties of its own: the labelling decides them.
	s.undecided = 0
	g := &Graph{Layout: c.layout, Visited: s.visited, Stats: &Stats{StateBytes: c.layout.Size}, s: s}
	if opt.graphParallel {
		if err := g.buildParallel(opt.Workers); err != nil {
			return nil, err
		}
	} else {
		g.build()
	}
	g.Stats.Elapsed = time.Since(start)
	return g, nil
}

// graphEdge is one fully expanded edge of a stored state. chain includes every
// move through an atomic sequence; it is immutable, so workers can hand the
// coordinator a path without copying its references.
type graphEdge struct {
	state []byte
	chain *chainNode
}

// graphEvent is one transition in the order the sequential graph builder
// would observe it. An event has either edge or err; intermediate atomic steps
// have neither, but still carry counters and work for budgets.
type graphEvent struct {
	edge        *graphEdge
	trans, atom int
	work        int64
	// scratch is the live atomic-path depth in the worker when it emitted
	// this event. The coordinator charges it in replay order.
	scratch  int
	err      error
	errChain *chainNode
	errFinal []byte
}

// graphStreamWindow bounds the number of completed events each worker can
// retain while the coordinator replays an earlier graph-ID stream. It leaves
// enough work in flight for parallel expansion without restoring an
// unbounded layer-sized result buffer.
const graphStreamWindow = 16

const (
	graphHandoffBytes = 128
	graphEdgeBytes    = 12
	graphStateBytes   = 48
)

// graphHandoff carries one worker event. A bounded stream limits the live
// events per worker while the coordinator retains sequential replay order.
type graphHandoff struct {
	event graphEvent
}

// buildParallel expands all states of one breadth-first layer concurrently,
// then merges their results in ascending graph ID. The merge deliberately owns
// visited insertion, successors, chains, and budget checks: this gives the
// graph exactly the sequential BFS IDs and trace choices while the expensive
// enabledness/fire walks genuinely run on independent worker copies.
func (g *Graph) buildParallel(workers int) error {
	s := g.s
	l := s.c.layout
	s.cur, s.next = make([]byte, l.Size), make([]byte, l.Size)
	s.visited.Add(l.Initial())
	g.Succ, g.chain, g.Stuttered = append(g.Succ, nil), append(g.chain, nil), append(g.Stuttered, false)
	g.edgeMem += graphStateBytes
	s.parent, s.chains, s.depth = append(s.parent, -1), append(s.chains, nil), append(s.depth, 0)
	levels := []int32{0}
	if workers < 1 {
		workers = 1
	}
	cs := make([]*compiled, workers)
	cs[0] = s.c
	for i := 1; i < workers; i++ {
		c, err := compile(s.c.m)
		if err != nil {
			return err
		}
		cs[i] = c
	}
	g.parallel.workers = len(cs)
	head := 0
	for head < s.visited.Len() && s.stop == "" {
		level := levels[head]
		end := head + 1
		for end < s.visited.Len() && levels[end] == level {
			end++
		}
		g.parallel.layers++
		if width := end - head; width > g.parallel.maxLayer {
			g.parallel.maxLayer = width
		}
		for first := head; first < end; first += workers {
			last := min(first+workers, end)
			ctx, cancel := context.WithCancel(s.ctx)
			streams := make([]<-chan graphHandoff, last-first)
			sources := make([][]byte, last-first)
			skipped := make([]bool, last-first)
			for i := first; i < last; i++ {
				d := int(s.depth[i])
				if b := s.opt.Budget; b.MaxDepth > 0 && d > b.MaxDepth {
					s.truncated++
					skipped[i-first] = true
					continue
				}
				sources[i-first] = append([]byte(nil), s.visited.Get(i)...)
			}
			var wg sync.WaitGroup
			for i := first; i < last; i++ {
				if skipped[i-first] {
					continue
				}
				stream := make(chan graphHandoff, graphStreamWindow)
				streams[i-first] = stream
				wg.Add(1)
				go func(i int, stream chan<- graphHandoff) {
					defer wg.Done()
					defer close(stream)
					expandGraphState(ctx, cs[(i-first)%len(cs)], sources[i-first], stream)
				}(i, stream)
			}
			for i, stream := range streams {
				from := first + i
				if skipped[i] {
					continue
				}
				if d := int(s.depth[from]); d > g.Stats.MaxDepth {
					g.Stats.MaxDepth = d
				}
				moved := false
				for handoff := range stream {
					event := handoff.event
					if event.scratch > g.parallel.maxScratch {
						g.parallel.maxScratch = event.scratch
					}
					s.work += event.work
					g.Stats.Transitions += event.trans
					g.Stats.AtomicSteps += event.atom
					if event.err != nil {
						g.fail(event.err, func() *cex.Trace { return s.bfsPath(from, event.errChain.refs(), event.errFinal) })
					} else if !s.checkBudgets(g.parallelBFSExtra(event.scratch)) {
						// Budget decisions are replayed in graph-ID/event order.
					} else if event.edge != nil {
						chain := event.edge.chain
						copy(s.next, event.edge.state)
						idx, isNew, allowed := s.store()
						if !allowed {
							// store recorded the state-budget stop.
						} else {
							moved = true
							if isNew {
								g.Succ = append(g.Succ, nil)
								g.chain = append(g.chain, nil)
								g.Stuttered = append(g.Stuttered, false)
								g.edgeMem += graphStateBytes
								s.parent = append(s.parent, int32(from))
								s.chains = append(s.chains, chain)
								s.depth = append(s.depth, int32(int(s.depth[from])+chain.len()))
								levels = append(levels, level+1)
							}
							g.addSucc(from, idx, chain)
						}
					}
					if s.stop != "" {
						cancel()
						break
					}
				}
				if s.stop == "" && s.ctx.Err() != nil {
					s.budget("time budget exhausted")
					cancel()
				}
				if !moved && len(g.Succ[from]) == 0 && s.stop == "" {
					g.Stuttered[from] = true
					g.Succ[from] = []int32{int32(from)}
					g.chain[from] = []*chainNode{nil}
					g.edgeMem += graphEdgeBytes
				}
				if s.stop != "" {
					break
				}
			}
			cancel()
			wg.Wait()
			if s.stop != "" {
				break
			}
		}
		head = end
	}
	g.Stats.States = s.visited.Len()
	g.Stats.MemBytes = s.visited.Bytes() + int64(cap(s.parent))*bfsBytesPerState + g.edgeBytes() + s.chainNodes*chainNodeBytes
	if s.stop == "" && head >= s.visited.Len() {
		s.stop = "complete"
	}
	g.Stats.Complete = s.stop == "complete" && !s.budgetHit && s.truncated == 0 && g.Invalid == ""
	if s.truncated > 0 && g.Invalid == "" {
		s.budgetHit = true
		if s.stop == "complete" || s.stop == "" {
			s.stop = fmt.Sprintf("depth budget exhausted: %d state(s) at depth > %d were stored but not expanded", s.truncated, s.opt.Budget.MaxDepth)
		}
	}
	g.Stats.Stop = s.stop
	g.parallel.workerBytes = g.parallelWorkerBytes()
	return nil
}

// parallelWorkerBytes estimates the physical memory held by CTL graph workers
// but deliberately excluded from the logical, worker-count-independent budget.
func (g *Graph) parallelWorkerBytes() int64 {
	const edgeBytes, procBytes, propBytes, baseBytes = 512, 256, 128, 4096
	c := g.s.c
	model := int64(baseBytes + len(c.procs)*procBytes + len(c.props)*propBytes)
	for _, p := range c.procs {
		model += int64(len(p.edge)) * edgeBytes
	}
	stateBytes := int64(c.layout.Size)
	per := model + 3*stateBytes + int64(g.parallel.maxScratch)*(stateBytes+bfsNodeBytes+chainNodeBytes) +
		int64(graphStreamWindow)*(stateBytes+chainNodeBytes+graphHandoffBytes)
	return int64(g.parallel.workers) * per
}

// parallelBFSExtra accounts for the current stream's actual atomic path and
// one bounded handoff queue. Worker count changes physical buffering, but not
// the logical graph budget: otherwise the same model has a different reported
// stop point solely because its caller requested more workers.
func (g *Graph) parallelBFSExtra(scratch int) int64 {
	s := g.s
	stateBytes := int64(len(s.next))
	return s.bfsExtra(scratch) + g.edgeBytes() + stateBytes*3 + int64(graphStreamWindow)*(stateBytes+bfsNodeBytes+chainNodeBytes+graphHandoffBytes)
}

// expandGraphState performs the mutable part of one graph-state expansion on
// a worker-local search shell and hands events to the coordinator in order.
func expandGraphState(ctx context.Context, c *compiled, state []byte, events chan<- graphHandoff) {
	s := &search{c: c, cur: make([]byte, c.layout.Size), next: make([]byte, c.layout.Size)}
	type node struct {
		state []byte
		frame frame
		chain *chainNode
	}
	emit := func(event graphEvent) bool {
		handoff := graphHandoff{event: event}
		select {
		case events <- handoff:
			return true
		case <-ctx.Done():
			return false
		}
	}
	nodes := []node{{state: append([]byte(nil), state...), frame: frame{proc: -1, rv: -1}}}
	for len(nodes) > 0 {
		if ctx.Err() != nil {
			return
		}
		n := &nodes[len(nodes)-1]
		copy(s.cur, n.state)
		m, ok, err := s.nextEnabled(&n.frame, s.cur)
		if err != nil {
			emit(graphEvent{scratch: len(nodes), err: err, errChain: n.chain})
			return
		}
		if !ok {
			nodes = nodes[:len(nodes)-1]
			continue
		}
		n.frame.enabled++
		chain := n.chain.push(s.ref(int32(m.e.proc), int32(m.e.idx), partnerCode(m)))
		before := s.work
		_, err = s.fire(m)
		if err != nil {
			emit(graphEvent{trans: 1, work: s.work - before, scratch: len(nodes), err: err, errChain: chain, errFinal: append([]byte(nil), s.next...)})
			return
		}
		inter, err := s.intermediate(s.next)
		if err != nil {
			emit(graphEvent{trans: 1, work: s.work - before, scratch: len(nodes), err: err, errChain: chain, errFinal: append([]byte(nil), s.next...)})
			return
		}
		if inter {
			if !emit(graphEvent{trans: 1, atom: 1, work: s.work - before, scratch: len(nodes)}) {
				return
			}
			if len(nodes) > dstepLimit {
				emit(graphEvent{scratch: len(nodes), err: fmt.Errorf("depth budget exhausted: an atomic sequence exceeds %d steps", dstepLimit), errChain: chain, errFinal: append([]byte(nil), s.next...)})
				return
			}
			nodes = append(nodes, node{state: append([]byte(nil), s.next...), frame: frame{proc: -1, rv: -1}, chain: chain})
			continue
		}
		edge := graphEdge{state: append([]byte(nil), s.next...), chain: chain}
		if !emit(graphEvent{edge: &edge, trans: 1, work: s.work - before, scratch: len(nodes)}) {
			return
		}
	}
}

// build is a breadth-first sweep that records successors. The inner loop
// over intermediate atomic states is the one bfs() uses, so the stored
// states are the same ones.
func (g *Graph) build() {
	s := g.s
	l := s.c.layout
	init := l.Initial()
	s.cur = make([]byte, l.Size)
	s.next = make([]byte, l.Size)
	s.visited.Add(init)
	g.Succ = append(g.Succ, nil)
	g.chain = append(g.chain, nil)
	g.edgeMem += graphStateBytes
	s.parent = append(s.parent, -1)
	s.chains = append(s.chains, nil)
	s.depth = append(s.depth, 0)
	g.Stuttered = append(g.Stuttered, false)

	head := 0
	for head < s.visited.Len() && s.stop == "" {
		d := int(s.depth[head])
		if b := s.opt.Budget; b.MaxDepth > 0 && d > b.MaxDepth {
			s.truncated++
			head++
			continue
		}
		if d > g.Stats.MaxDepth {
			g.Stats.MaxDepth = d
		}
		nodes := []bfsNode{{state: append([]byte(nil), s.visited.Get(head)...), f: frame{proc: -1, rv: -1}}}
		moved := false
		for len(nodes) > 0 && s.stop == "" {
			n := &nodes[len(nodes)-1]
			copy(s.cur, n.state)
			m, ok, err := s.nextEnabled(&n.f, s.cur)
			if err != nil {
				g.fail(err, func() *cex.Trace { return s.bfsPath(head, n.chain.refs(), nil) })
				break
			}
			if !ok {
				nodes = nodes[:len(nodes)-1]
				continue
			}
			n.f.enabled++
			g.Stats.Transitions++
			chain := n.chain.push(s.ref(int32(m.e.proc), int32(m.e.idx), partnerCode(m)))
			failed, err := s.fire(m)
			if err != nil {
				g.fail(err, func() *cex.Trace { return s.bfsPath(head, chain.refs(), s.next) })
				break
			}
			if failed != nil {
				// An assert that fails is a property of the model, not of the
				// graph: the safety search reports it. The graph goes on.
				_ = failed
			}
			inter, err := s.intermediate(s.next)
			if err != nil {
				g.fail(err, func() *cex.Trace { return s.bfsPath(head, chain.refs(), s.next) })
				break
			}
			// One look at the budgets per edge fired, whatever the successor
			// turns out to be; the node stack and the chain of a descent, and
			// the chain nodes the edges keep, are in the memory estimate
			// (issues #8 and #7).
			if !s.checkBudgets(s.bfsExtra(len(nodes))) {
				break
			}
			if inter {
				g.Stats.AtomicSteps++
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
			moved = true
			if isNew {
				g.Succ = append(g.Succ, nil)
				g.chain = append(g.chain, nil)
				g.Stuttered = append(g.Stuttered, false)
				g.edgeMem += graphStateBytes
				s.parent = append(s.parent, int32(head))
				s.chains = append(s.chains, chain) // the node, shared with the edge below
				s.depth = append(s.depth, int32(d+chain.len()))
				g.Stats.States = s.visited.Len()
			}
			g.addSucc(head, idx, chain)
		}
		if !moved && len(g.Succ[head]) == 0 && s.stop == "" {
			// Totality: a state with no move stutters (see the package note).
			// The stop check matters: a state whose expansion a budget or a
			// model error cut short has no move *yet*, and giving it a
			// self-loop would put an edge in the graph that the model does
			// not have. Such a graph is incomplete anyway, and an incomplete
			// graph yields no CTL verdict.
			g.Stuttered[head] = true
			g.Succ[head] = []int32{int32(head)}
			g.chain[head] = []*chainNode{nil}
			g.edgeMem += graphEdgeBytes
		}
		head++
	}
	g.Stats.States = s.visited.Len()
	g.Stats.MemBytes = s.visited.Bytes() + int64(cap(s.parent))*bfsBytesPerState + g.edgeBytes() + s.chainNodes*chainNodeBytes
	if s.stop == "" && head >= s.visited.Len() {
		s.stop = "complete"
	}
	g.Stats.Complete = s.stop == "complete" && !s.budgetHit && s.truncated == 0 && g.Invalid == ""
	if s.truncated > 0 && g.Invalid == "" {
		s.budgetHit = true
		if s.stop == "complete" || s.stop == "" {
			s.stop = fmt.Sprintf("depth budget exhausted: %d state(s) at depth > %d were stored but not expanded", s.truncated, s.opt.Budget.MaxDepth)
		}
	}
	g.Stats.Stop = s.stop
}

// edgeBytes is the memory of the edge lists: the successor index and the
// chain pointer per edge, the two slice headers per state. It is updated as
// the graph grows so budget checks do not rescan every existing edge.
func (g *Graph) edgeBytes() int64 {
	return g.edgeMem
}

func (g *Graph) addSucc(from, to int, chain *chainNode) {
	for _, x := range g.Succ[from] {
		if int(x) == to {
			return
		}
	}
	g.Succ[from] = append(g.Succ[from], int32(to))
	g.chain[from] = append(g.chain[from], chain)
	g.edgeMem += graphEdgeBytes
	chain.keep(g.s) // the edge holds the nodes: counted once, here
}

// fail records why the construction stopped: an exhausted process pool is
// a declared bound (the graph is simply incomplete), anything else is a
// model error that makes the model invalid.
func (g *Graph) fail(err error, tr func() *cex.Trace) {
	var pe *poolExhausted
	if errors.As(err, &pe) {
		g.s.budget(err.Error())
		return
	}
	if g.Invalid == "" {
		g.Invalid, g.InvalidTrace = err.Error(), tr()
	}
	g.s.stop = "invalid model"
	g.s.invalid = true
}

// BuildPred fills Pred (idempotent).
func (g *Graph) BuildPred() {
	if g.Pred != nil {
		return
	}
	g.Pred = make([][]int32, len(g.Succ))
	for i, ss := range g.Succ {
		for _, t := range ss {
			g.Pred[t] = append(g.Pred[t], int32(i))
		}
	}
}

// Len is the number of stored states.
func (g *Graph) Len() int { return len(g.Succ) }

// State returns the vector of state i.
func (g *Graph) State(i int) []byte { return g.Visited.Get(i) }

// Eval evaluates a compiled global expression on state i.
func (g *Graph) Eval(c *ir.Compiled, i int) (bool, error) {
	g.Layout.Timeout = false
	return c.Truth(g.Visited.Get(i))
}

// Compile compiles a global expression against the graph's layout.
func (g *Graph) Compile(e *ir.Expr) (*ir.Compiled, error) { return g.Layout.Compile(e, -1) }

// Trace renders the run along the given sequence of state indices, which
// must be a path of the graph (path[k+1] ∈ Succ[path[k]]). loopStart < 0
// gives a finite run; otherwise the steps from index loopStart on are the
// loop and the last state equals states[loopStart].
func (g *Graph) Trace(path []int, loopStart int) *cex.Trace {
	states := [][]byte{append([]byte(nil), g.Visited.Get(path[0])...)}
	var refs []cex.Ref
	loopRef := -1
	for k := 0; k+1 < len(path); k++ {
		if k == loopStart {
			loopRef = len(refs)
		}
		chain := g.chainBetween(path[k], path[k+1])
		if len(chain) == 0 {
			// The stutter self-loop of a terminal state.
			refs = append(refs, cex.Ref{Null: true, Note: "(stutter: no process can move, the state repeats forever; the self-loop that makes the transition relation total for CTL)"})
			states = append(states, append([]byte(nil), g.Visited.Get(path[k+1])...))
			continue
		}
		g.replay(&states, &refs, chain, g.Visited.Get(path[k+1]))
	}
	if loopStart == len(path)-1 {
		loopRef = len(refs)
	}
	if loopStart >= 0 && loopRef >= 0 {
		return cex.BuildLasso(g.Layout, states, refs, loopRef)
	}
	return cex.Build(g.Layout, states, refs)
}

func (g *Graph) chainBetween(from, to int) []cex.Ref {
	for k, t := range g.Succ[from] {
		if int(t) == to {
			return g.chain[from][k].refs()
		}
	}
	return nil
}

// replay re-fires a chain of moves to recover the intermediate atomic
// states, exactly as bfsPath does.
func (g *Graph) replay(states *[][]byte, refs *[]cex.Ref, chain []cex.Ref, final []byte) {
	s := g.s
	for k, r := range chain {
		*refs = append(*refs, r)
		if k == len(chain)-1 {
			*states = append(*states, append([]byte(nil), final...))
			return
		}
		copy(s.cur, (*states)[len(*states)-1])
		m := move{e: &s.c.procs[r.Proc].edge[r.Edge]}
		if r.HasPartner {
			m.partner = &s.c.procs[r.PartnerProc].edge[r.PartnerEdge]
		}
		s.fire(m) // deterministic replay; errors were reported when first taken
		*states = append(*states, append([]byte(nil), s.next...))
	}
}
