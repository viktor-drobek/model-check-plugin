package explore

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"modelcheck/cex"
	"modelcheck/ir"
)

// The differential oracle of the reduction: small random concurrent models,
// each checked in full and reduced. The verdict of every safety property must
// be the same, the reduced graph must not be bigger, and every counterexample
// and witness the reduced search reports must replay as a run of the model.
// There is no other ground truth for the reduction; this is the test that
// fails when an independence argument is wrong.

func randomPORModel(rnd *rand.Rand) *ir.Model {
	if rnd.Intn(3) == 0 {
		return randomPipelineModel(rnd)
	}
	m := &ir.Model{Schema: ir.Schema, Name: "random"}
	globals := []string{"x", "y", "z"}
	for _, g := range globals {
		m.Globals = append(m.Globals, byteVar(g))
	}
	m.Globals = append(m.Globals, ir.Var{Name: "a", Type: ir.Byte, Len: 2})
	withChan := rnd.Intn(3) == 0
	val := func() *ir.Expr { return ir.Const(int64(rnd.Intn(3))) }
	glob := func() string { return globals[rnd.Intn(len(globals))] }
	next := func(v string) *ir.Expr { // (v + 1) % 3 keeps every byte in range
		return ir.Binary("mod", ir.Binary("add", ir.Ref(v), ir.Const(1)), ir.Const(3))
	}
	hasAssert := false
	np := 2 + rnd.Intn(3)
	// Channels. Either one channel that any process may send on and receive
	// from (so never directed), or a pipeline: two or three channels, each with
	// a chosen sender and a different chosen receiver, and processes that touch
	// a channel only from their own end. The pipelines are where the reduction
	// treats a channel as directed; the length guards, ClearChans and the other
	// alternatives below are what has to make it back off.
	type chanEnds struct{ sender, receiver int } // -1: any process
	var ends []chanEnds
	pipe := false
	if withChan {
		if rnd.Intn(2) == 0 {
			pipe = true
			for i, n := 0, 2+rnd.Intn(2); i < n; i++ {
				s := rnd.Intn(np)
				r := (s + 1 + rnd.Intn(np-1)) % np
				m.Channels = append(m.Channels, ir.Channel{Name: fmt.Sprintf("ch%d", i), Capacity: 1 + rnd.Intn(3), Fields: chanFields(rnd)})
				ends = append(ends, chanEnds{s, r})
			}
		} else {
			capacity := 1 + rnd.Intn(2)
			if rnd.Intn(8) == 0 {
				capacity = 0 // a rendezvous: the reduction must refuse, and say so
			}
			m.Channels = []ir.Channel{{Name: "ch", Capacity: capacity, Fields: chanFields(rnd)}}
			ends = []chanEnds{{-1, -1}}
		}
	}
	// pickChan: a channel process p may send on (or receive from), if any.
	pickChan := func(p int, send bool) (string, bool) {
		var ok []int
		for i, e := range ends {
			if (send && (e.sender == -1 || e.sender == p)) || (!send && (e.receiver == -1 || e.receiver == p)) {
				ok = append(ok, i)
			}
		}
		if len(ok) == 0 {
			return "", false
		}
		return m.Channels[ok[rnd.Intn(len(ok))]].Name, true
	}
	anyChan := func() string { return m.Channels[rnd.Intn(len(m.Channels))].Name }
	// A length read makes a channel undirected, so in a pipeline it is rare.
	lenRead := func() bool { return !pipe || rnd.Intn(8) == 0 }
	// Half of the models are "rich": dead-end locations, d_step chains, many
	// guards on program counters. They are where a missed dependency hides,
	// and they are so interconnected that little is reduced. The other half
	// are "light" (few program-counter guards, no d_step, no dead ends), and
	// are the ones in which the reduction does something.
	rich := rnd.Intn(2) == 0
	nlocs := make([]int, np)
	sink := make([]bool, np) // the last location has no edge out of it: a dead end that pc guards can wait for
	for p := range nlocs {
		nlocs[p] = 2 + rnd.Intn(3)
		if rich {
			nlocs[p]++
			sink[p] = rnd.Intn(10) < 7
		}
	}
	edgesPerProc := func() int {
		if rich {
			return 2 + rnd.Intn(6)
		}
		return 2 + rnd.Intn(4)
	}
	for p := 0; p < np; p++ {
		nloc := nlocs[p]
		nOut := nloc // locations that may have out-edges
		if sink[p] {
			nOut--
		}
		pr := ir.Process{Name: fmt.Sprintf("P%d", p), Locals: []ir.Var{byteVar("l")}, Locations: make([]ir.Location, nloc)}
		// A guard on another process's program counter: alone, as one conjunct,
		// negated, inside a disjunction, or waiting for its last location, as
		// Promela's termination order does (pc(j) == dead).
		pcGuard := func() *ir.Expr {
			j := rnd.Intn(np)
			if j == p {
				j = (j + 1) % np
			}
			c := ir.Const(int64(rnd.Intn(nlocs[j])))
			switch rnd.Intn(6) {
			case 0:
				return ir.Binary("eq", ir.PC(j), c)
			case 1:
				return ir.Binary("eq", c, ir.PC(j))
			case 2:
				return ir.And(ir.Binary("eq", ir.PC(j), c), ir.Binary("ne", ir.Ref(glob()), val()))
			case 3:
				return ir.Binary("ne", ir.PC(j), c)
			case 4:
				return ir.Binary("or", ir.Binary("eq", ir.PC(j), c), ir.Binary("eq", ir.Ref(glob()), val()))
			default:
				return ir.Binary("eq", ir.PC(j), ir.Const(int64(nlocs[j]-1)))
			}
		}
		for e, ne := 0, edgesPerProc(); e < ne; e++ {
			ed := ir.Edge{From: rnd.Intn(nOut), To: rnd.Intn(nloc)}
			switch rnd.Intn(9) {
			case 0, 1, 2:
				if np > 1 && (rich || rnd.Intn(4) == 0) {
					ed.Guard = pcGuard()
				}
			case 3:
				ed.Guard = ir.Binary([]string{"eq", "ne", "lt", "ge"}[rnd.Intn(4)], ir.Ref(glob()), val())
			case 4:
				ed.Guard = ir.Binary([]string{"eq", "ne", "lt", "ge"}[rnd.Intn(4)], ir.Ref("l"), val())
			case 5:
				ed.Guard = ir.Binary("eq", ir.Index("a", ir.Const(int64(rnd.Intn(2)))), val())
			case 6:
				ed.Guard = ir.Binary("eq", ir.Index("a", ir.Binary("mod", ir.Ref("l"), ir.Const(2))), val())
			case 7:
				if withChan && lenRead() {
					ed.Guard = ir.Binary([]string{"eq", "gt"}[rnd.Intn(2)], ir.Len(anyChan()), ir.Const(int64(rnd.Intn(2))))
				}
			}
			switch rnd.Intn(10) {
			case 0:
				ed.Effect = []ir.Assign{{Var: glob(), Value: val()}}
			case 1:
				ed.Effect = []ir.Assign{{Var: glob(), Value: next(glob())}}
			case 2:
				ed.Effect = []ir.Assign{{Var: "l", Value: next("l")}}
			case 3:
				ed.Effect = []ir.Assign{{Var: "a", Index: ir.Const(int64(rnd.Intn(2))), Value: val()}}
			case 4:
				ed.Effect = []ir.Assign{{Var: "a", Index: ir.Binary("mod", ir.Ref("l"), ir.Const(2)), Value: val()}}
			case 5:
				if ch, ok := pickChan(p, true); ok {
					ed.Send = &ir.ChanOp{Chan: ch, Args: sendArgs(fieldsOf(m, ch), val)}
				}
			case 6:
				if ch, ok := pickChan(p, false); ok {
					ed.Recv = &ir.RecvOp{Chan: ch, Args: recvArgs(rnd, fieldsOf(m, ch), glob, val)}
				}
			case 7:
				if rnd.Intn(3) == 0 {
					ed.Assert = ir.Binary("ne", ir.Ref(glob()), ir.Const(2))
					hasAssert = true
				}
			case 8:
				ed.Guard = nil
				ed.Else = true
			}
			if withChan && ed.Send == nil && ed.Recv == nil && rnd.Intn(12) == 0 && (!pipe || rnd.Intn(6) == 0) {
				ed.ClearChans = []int{rnd.Intn(len(m.Channels))} // the frontend puts it on -end- edges, which do not receive
			}
			pr.Edges = append(pr.Edges, ed)
		}
		m.Processes = append(m.Processes, pr)
	}
	// d_step: mark some edges; the location they enter offers a few guarded
	// continuations and an unguarded one, so the step never blocks.
	for p := range m.Processes {
		pr := &m.Processes[p]
		nOut := nlocs[p]
		if sink[p] {
			nOut--
		}
		for ei, n := 0, len(pr.Edges); ei < n; ei++ {
			e := &pr.Edges[ei]
			if !rich || e.Else || rnd.Intn(7) != 0 {
				continue
			}
			if e.To >= nOut {
				e.To = rnd.Intn(nOut)
			}
			e.DStep = true
			for k := rnd.Intn(3); k > 0; k-- {
				c := ir.Edge{From: e.To, To: rnd.Intn(nlocs[p])}
				switch rnd.Intn(3) {
				case 0:
					if j := rnd.Intn(np); j != p {
						c.Guard = ir.Binary("eq", ir.PC(j), ir.Const(int64(rnd.Intn(nlocs[j]))))
					}
				case 1:
					c.Guard = ir.Binary("ne", ir.Ref(glob()), val())
				}
				if rnd.Intn(3) == 0 {
					c.Effect = []ir.Assign{{Var: glob(), Value: val()}}
				}
				pr.Edges = append(pr.Edges, c)
			}
			last := ir.Edge{From: e.To, To: rnd.Intn(nlocs[p])}
			if rnd.Intn(2) == 0 {
				last.Effect = []ir.Assign{{Var: glob(), Value: val()}}
			}
			pr.Edges = append(pr.Edges, last)
		}
	}
	m.Properties = []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}}
	if hasAssert {
		m.Properties = append(m.Properties, ir.Property{ID: "assert", Kind: ir.KindAssert})
	}
	// A property that reads two variables written by different processes is
	// what makes visibility (C2) necessary: the state "x new, y old" is one
	// the reduction may skip unless the writes are kept in every order.
	pair := func() *ir.Expr {
		return ir.And(ir.Binary("eq", ir.Ref(glob()), val()), ir.Binary("eq", ir.Ref(glob()), val()))
	}
	// What a property may read: a scalar, two or three cells, an element of
	// the array, the length of the channel, the program counter of a process.
	atom := func() *ir.Expr {
		switch rnd.Intn(6) {
		case 0:
			return ir.Binary("eq", ir.Index("a", ir.Const(int64(rnd.Intn(2)))), val())
		case 1:
			if withChan && lenRead() {
				return ir.Binary("eq", ir.Len(anyChan()), ir.Const(int64(rnd.Intn(2))))
			}
		case 2:
			return ir.Binary("eq", ir.PC(rnd.Intn(np)), ir.Const(int64(rnd.Intn(3))))
		case 3:
			return ir.And(pair(), ir.Binary("ne", ir.Ref(glob()), val()))
		}
		return ir.Binary("eq", ir.Ref(glob()), val())
	}
	if rnd.Intn(10) < 7 {
		expr := ir.Binary("ne", ir.Ref(glob()), ir.Const(int64(1+rnd.Intn(2))))
		switch rnd.Intn(3) {
		case 0:
			expr = ir.Unary("not", pair())
		case 1:
			expr = ir.Unary("not", atom())
		}
		m.Properties = append(m.Properties, ir.Property{ID: "inv", Kind: ir.KindInvariant, Expr: expr})
	}
	if rnd.Intn(10) < 6 {
		expr := ir.And(ir.Binary("eq", ir.Ref(glob()), ir.Const(2)), ir.Binary("ne", ir.Ref(glob()), ir.Const(0)))
		switch rnd.Intn(3) {
		case 0:
			expr = pair()
		case 1:
			expr = atom()
		}
		m.Properties = append(m.Properties, ir.Property{ID: "reach", Kind: ir.KindReach, Expr: expr})
	}
	return m
}

// randomPipelineModel is a model in which the processes talk only through
// channels: nothing shared but a global x that no process writes, so a guard
// on it is a branch nobody can take. Most channels have exactly one sender and
// one receiver (directed, for the reduction), and the locations mix sends,
// receives, free edges and such trap branches, so that an alternative the
// other end enables and an expansion that forgets it are told apart by the
// states left without a move. A few channels are made undirected on purpose:
// a second sender or receiver, a length guard, a clear, an else, a d_step.
func randomPipelineModel(rnd *rand.Rand) *ir.Model {
	m := &ir.Model{Schema: ir.Schema, Name: "pipeline", Globals: []ir.Var{byteVar("x")}}
	np := 2 + rnd.Intn(3)
	type ends struct{ s, r int }
	var es []ends
	for i, n := 0, 1+rnd.Intn(3); i < n; i++ {
		s := rnd.Intn(np)
		r := (s + 1 + rnd.Intn(np-1)) % np
		m.Channels = append(m.Channels, ir.Channel{Name: fmt.Sprintf("q%d", i), Capacity: 1 + rnd.Intn(2), Fields: chanFields(rnd)})
		es = append(es, ends{s, r})
	}
	// One model in five gets a second sender or receiver on some channel.
	extra := -1
	extraSend := false
	extraProc := 0
	if rnd.Intn(4) == 0 {
		extra = rnd.Intn(len(es))
		extraSend = rnd.Intn(2) == 0
		extraProc = rnd.Intn(np)
		// The extra process must be a stranger to the channel: a second sender
		// or receiver, not the one that is already there.
		for (extraSend && extraProc == es[extra].s) || (!extraSend && extraProc == es[extra].r) {
			extraProc = rnd.Intn(np)
		}
	}
	clearing := rnd.Intn(8) == 0 // a model in which some edges clear a channel
	nlocs := make([]int, np)
	for p := range nlocs {
		nlocs[p] = 3 + rnd.Intn(3) // the last one has no edge: where a process ends or sticks
	}
	val := func() *ir.Expr { return ir.Const(int64(rnd.Intn(3))) }
	for p := 0; p < np; p++ {
		nloc := nlocs[p]
		pr := ir.Process{Name: fmt.Sprintf("P%d", p), Locals: []ir.Var{byteVar("l")}, Locations: make([]ir.Location, nloc)}
		mine := func(send bool) (string, bool) { // a channel p may use at that end
			var ok []int
			for i, e := range es {
				if (send && e.s == p) || (!send && e.r == p) {
					ok = append(ok, i)
				}
			}
			if send && extra >= 0 && extraSend && extraProc == p {
				ok = append(ok, extra)
			}
			if !send && extra >= 0 && !extraSend && extraProc == p {
				ok = append(ok, extra)
			}
			if len(ok) == 0 {
				return "", false
			}
			return m.Channels[ok[rnd.Intn(len(ok))]].Name, true
		}
		for e, ne := 0, 3+rnd.Intn(4); e < ne; e++ {
			ed := ir.Edge{From: rnd.Intn(nloc - 1), To: rnd.Intn(nloc)}
			switch rnd.Intn(9) {
			case 0, 1:
				if ch, ok := mine(true); ok {
					ed.Send = &ir.ChanOp{Chan: ch, Args: sendArgs(fieldsOf(m, ch), val)}
				}
			case 2, 3:
				if ch, ok := mine(false); ok {
					// a receive that waits for a value in some field of the head
					ed.Recv = &ir.RecvOp{Chan: ch, Args: recvArgs(rnd, fieldsOf(m, ch), func() string { return "l" }, val)}
				}
			case 4:
				ed.Effect = []ir.Assign{{Var: "l", Value: ir.Binary("mod", ir.Binary("add", ir.Ref("l"), ir.Const(1)), ir.Const(3))}}
			case 5:
				ed.Guard = ir.Binary("eq", ir.Ref("x"), ir.Const(5)) // a branch nobody can take
			case 6:
				ed.Guard = ir.Binary([]string{"eq", "ne", "lt"}[rnd.Intn(3)], ir.Ref("l"), val())
			case 7:
				if rnd.Intn(4) == 0 {
					ed.Else = true
				}
			}
			if !ed.Else && rnd.Intn(30) == 0 {
				ed.Guard = ir.Binary("eq", ir.Len(m.Channels[rnd.Intn(len(m.Channels))].Name), ir.Const(int64(rnd.Intn(2)))) // a length read
			}
			if ed.Send == nil && ed.Recv == nil && clearing && rnd.Intn(5) == 0 {
				ed.ClearChans = []int{rnd.Intn(len(m.Channels))}
			}
			pr.Edges = append(pr.Edges, ed)
		}
		if extra >= 0 && extraProc == p {
			ed := ir.Edge{From: rnd.Intn(nloc - 1), To: rnd.Intn(nloc)}
			if extraSend {
				ed.Send = &ir.ChanOp{Chan: m.Channels[extra].Name, Args: sendArgs(len(m.Channels[extra].Fields), val)}
			} else {
				ed.Recv = &ir.RecvOp{Chan: m.Channels[extra].Name, Args: recvArgs(rnd, len(m.Channels[extra].Fields), func() string { return "l" }, val)}
			}
			pr.Edges = append(pr.Edges, ed)
		}
		// A d_step now and then, with an unguarded continuation.
		if rnd.Intn(8) == 0 {
			i := rnd.Intn(len(pr.Edges))
			if !pr.Edges[i].Else {
				pr.Edges[i].DStep = true
				pr.Edges[i].To = rnd.Intn(nloc - 1)
				pr.Edges = append(pr.Edges, ir.Edge{From: pr.Edges[i].To, To: rnd.Intn(nloc)})
			}
		}
		m.Processes = append(m.Processes, pr)
	}
	m.Properties = []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}}
	return m
}

// chanFields is the field list of a random channel: one to three bytes, so that
// a message has a head field a receive can match and later fields it can match
// instead, or bind, or ignore.
func chanFields(rnd *rand.Rand) []ir.Type {
	fs := make([]ir.Type, 1+rnd.Intn(3))
	for i := range fs {
		fs[i] = ir.Byte
	}
	return fs
}

// fieldsOf is how many fields the messages of channel name have.
func fieldsOf(m *ir.Model, name string) int {
	for i := range m.Channels {
		if m.Channels[i].Name == name {
			return len(m.Channels[i].Fields)
		}
	}
	return 1
}

func sendArgs(n int, val func() *ir.Expr) []*ir.Expr {
	args := make([]*ir.Expr, n)
	for i := range args {
		args[i] = val()
	}
	return args
}

// recvArgs gives each field a receive: bound to the local or to a global,
// matched against a value (a receive that waits for it), or ignored.
func recvArgs(rnd *rand.Rand, n int, glob func() string, val func() *ir.Expr) []ir.RecvArg {
	args := make([]ir.RecvArg, n)
	for i := range args {
		switch rnd.Intn(4) {
		case 0:
			args[i] = ir.RecvArg{Var: "l"}
		case 1:
			args[i] = ir.RecvArg{Var: glob()}
		case 2:
			args[i] = ir.RecvArg{Match: val()}
		}
	}
	return args
}

// hitBudget: the search stopped because a limit ran out, not because it had
// seen everything or met an error.
func hitBudget(r *Result) bool { return strings.Contains(r.Stop, "budget") }

// recorder keeps a copy of every state a search stores, so that a test can
// compare the sets of states the full and the reduced search reach.
type recorder struct {
	Visited
	states [][]byte
}

func (r *recorder) Add(s []byte) (int, bool) {
	idx, isNew := r.Visited.Add(s)
	if isNew {
		r.states = append(r.states, append([]byte(nil), s...))
	}
	return idx, isNew
}

// noMoveStates is the set of recorded states in which no move is enabled.
// The reduction must keep every one of them: a state without moves is a
// deadlock or an end state, and a reduction that loses one has lost a verdict
// even when another deadlock hides the loss from the status of the property.
func noMoveStates(m *ir.Model, states [][]byte) (map[string]bool, error) {
	st, err := NewStepper(m)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, s := range states {
		mv, err := st.Enabled(s)
		if err != nil {
			continue
		}
		if len(mv) == 0 {
			out[string(s)] = true
		}
	}
	return out, nil
}

// replay checks that a trace is a run of the model from its initial state.
// Edges with the same text in one process make a step ambiguous, so every
// matching move is tried, and the final valuation must be the trace's own.
func replay(m *ir.Model, tr *cex.Trace) error {
	st, err := NewStepper(m)
	if err != nil {
		return err
	}
	// Edges of one process with the same text make a step ambiguous, and a
	// trace of many such steps would be tried in exponentially many ways;
	// a (step, state) pair that has led nowhere is not tried again.
	dead := map[string]bool{}
	var walk func(i int, state []byte) error
	walk = func(i int, state []byte) error {
		if i == len(tr.Steps) {
			return sameValuation(st, state, tr.Final)
		}
		key := fmt.Sprintf("%d|%x", i, state)
		if dead[key] {
			return fmt.Errorf("step %d: no continuation from this state", i+1)
		}
		moves, err := st.Enabled(state)
		if err != nil {
			return err
		}
		var last error = fmt.Errorf("step %d (%s: %s) is not an enabled move", i+1, tr.Steps[i].Process, tr.Steps[i].Command)
		for _, mv := range moves {
			e := st.Edge(mv.Edge)
			if m.Processes[mv.Edge.Proc].Name != tr.Steps[i].Process || cex.CommandText(e) != tr.Steps[i].Command {
				continue
			}
			next, _, err := st.Apply(state, mv)
			if err != nil {
				continue
			}
			if err := walk(i+1, next); err == nil {
				return nil
			} else {
				last = err
			}
		}
		dead[key] = true
		return last
	}
	return walk(0, st.Initial())
}

func sameValuation(st *Stepper, state []byte, final []cex.Value) error {
	l := st.Layout()
	want := map[string]int64{}
	for _, v := range final {
		want[v.Var] = v.Value
	}
	for _, sl := range l.Slots {
		name := sl.Name()
		if sl.Proc >= 0 {
			name = l.Model.Processes[sl.Proc].Name + "." + name
		}
		w, ok := want[name]
		if !ok {
			continue
		}
		if got := sl.Read(state); got != w {
			return fmt.Errorf("final state: %s = %d, the trace says %d", name, got, w)
		}
	}
	return nil
}

// TestPORAgreesWithTheFullSearchOnRandomModels is O1 (porDifferential) on the
// base generator, kept under the name the earlier steps know it by. The other
// generators are TestPORDifferentialOnTheGenerators.
func TestPORAgreesWithTheFullSearchOnRandomModels(t *testing.T) {
	for _, g := range porGeneratorsToRun("base") {
		tl := porForSeeds(t, "O1", g, 8000, func(m *ir.Model, _ *porTally) (porOutcome, error) { return porDifferential(m) })
		if floor := porFloorOf(tl.models, porReducedShare[g.name], porFloorSigmas); tl.failures == 0 && tl.smaller < floor {
			t.Fatalf("only %d of %d models were reduced (the floor is %d): the generator no longer exercises the reduction", tl.smaller, tl.models, floor)
		}
	}
}
