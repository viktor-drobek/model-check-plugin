package explore

import (
	"context"
	"errors"
	"fmt"
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
	// Chain[i][k] is the sequence of moves from state i to Succ[i][k]; it has
	// more than one element only when the step passes through intermediate
	// states of an atomic sequence. Empty for a stutter self-loop.
	Chain [][][]cex.Ref
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

	s *search
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
	g.build()
	g.Stats.Elapsed = time.Since(start)
	return g, nil
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
	g.Chain = append(g.Chain, nil)
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
				g.fail(err, func() *cex.Trace { return s.bfsPath(head, n.chain, nil) })
				break
			}
			if !ok {
				nodes = nodes[:len(nodes)-1]
				continue
			}
			n.f.enabled++
			g.Stats.Transitions++
			chain := append(append([]cex.Ref(nil), n.chain...), s.ref(int32(m.e.proc), int32(m.e.idx), partnerCode(m)))
			failed, err := s.fire(m)
			if err != nil {
				g.fail(err, func() *cex.Trace { return s.bfsPath(head, chain, s.next) })
				break
			}
			if failed != nil {
				// An assert that fails is a property of the model, not of the
				// graph: the safety search reports it. The graph goes on.
				_ = failed
			}
			inter, err := s.intermediate(s.next)
			if err != nil {
				g.fail(err, func() *cex.Trace { return s.bfsPath(head, chain, s.next) })
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
				g.Chain = append(g.Chain, nil)
				g.Stuttered = append(g.Stuttered, false)
				s.parent = append(s.parent, int32(head))
				s.chains = append(s.chains, chain)
				s.depth = append(s.depth, int32(d+len(chain)))
				g.Stats.States = s.visited.Len()
				if !s.checkBudgets(int64(s.visited.Len()) * bfsBytesPerState) {
					break
				}
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
			g.Chain[head] = [][]cex.Ref{nil}
		}
		head++
	}
	g.Stats.States = s.visited.Len()
	g.Stats.MemBytes = s.visited.Bytes() + int64(cap(s.parent))*bfsBytesPerState + g.edgeBytes()
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

func (g *Graph) edgeBytes() int64 {
	n := int64(0)
	for _, ss := range g.Succ {
		n += int64(len(ss))*4 + 24
	}
	return n
}

func (g *Graph) addSucc(from, to int, chain []cex.Ref) {
	for _, x := range g.Succ[from] {
		if int(x) == to {
			return
		}
	}
	g.Succ[from] = append(g.Succ[from], int32(to))
	g.Chain[from] = append(g.Chain[from], chain)
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
			return g.Chain[from][k]
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
