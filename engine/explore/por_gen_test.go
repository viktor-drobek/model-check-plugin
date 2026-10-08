package explore

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"modelcheck/ir"
)

// Random model generators of the partial-order reduction's oracles
// (performance plan, step 6, section 7.3). Each is a function of a seed and is
// aimed at the shapes in which a rule has a trap: atomic sequences (finite
// macro-steps, a holder that blocks inside its sequence, a d_step that ends in
// an atomic edge), a cycle through an atomic chain with several branches,
// process creation with the process table, and `provided`. They build on
// randomPORModel and randomPipelineModel and are validated by
// TestPORGeneratorsProduceValidModels.
//
// Every generator keeps its models small: the oracles run them under a state
// budget and a depth budget (a macro-step that never ends would otherwise use
// all the memory before a clock is looked at).

// porGen is a named generator with the first seed of its default test run.
// The default seeds of different generators do not overlap, and none overlaps
// the seeds the earlier steps used (1, 11 000 000, 13 000 000), so that a
// default run is a set of models nobody has tuned a rule against.
type porGen struct {
	name  string
	first int64
	gen   func(*rand.Rand) *ir.Model
}

var porGenerators = []porGen{
	{"base", 1, randomPORModel},
	{"atomic", 20_000_001, genAtomic},
	{"loop", 30_000_001, genLoop},
	{"run", 40_000_001, genRun},
	{"run-atomic", 50_000_001, genRunAtomic},
	{"provided", 60_000_001, genProvided},
	{"reads", 70_000_001, genReads},
	{"atomic-reads", 80_000_001, genAtomicReads},
	{"nrpr", 90_000_001, genNrPr},
}

// newRand is the generator of a seed.
func newRand(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }

// porGeneratorByName finds a generator for the test switches.
func porGeneratorByName(name string) (porGen, bool) {
	for _, g := range porGenerators {
		if g.name == name {
			return g, true
		}
	}
	return porGen{}, false
}

func hasOut(pr *ir.Process, loc int) bool {
	for i := range pr.Edges {
		if pr.Edges[i].From == loc {
			return true
		}
	}
	return false
}

func outOf(pr *ir.Process, loc int) []int {
	var o []int
	for i := range pr.Edges {
		if pr.Edges[i].From == loc {
			o = append(o, i)
		}
	}
	return o
}

func scalarGlobals(m *ir.Model) []string {
	var gs []string
	for _, g := range m.Globals {
		if g.Len == 0 {
			gs = append(gs, g.Name)
		}
	}
	return gs
}

// forwardDSteps makes every d_step edge of pr go to a later location, by
// retargeting the ones that do not (with an unguarded continuation at the new
// target) and unmarking the ones that have no later location. A d_step that
// goes back can chain into itself, and a macro-step that never ends is not a
// shape the oracle can finish; a forward d_step keeps the shape that matters
// (a d_step whose last edge is atomic) and keeps it finite.
func forwardDSteps(pr *ir.Process, rnd *rand.Rand) {
	n := len(pr.Locations)
	for ei := 0; ei < len(pr.Edges); ei++ {
		e := &pr.Edges[ei]
		if !e.DStep || e.From < e.To {
			continue
		}
		if e.From >= n-1 {
			e.DStep = false
			continue
		}
		e.To = e.From + 1 + rnd.Intn(n-1-e.From)
		pr.Edges = append(pr.Edges, ir.Edge{From: e.To, To: rnd.Intn(n)})
	}
}

// genAtomic marks sequences of atomic edges in a random model. Every Atomic
// edge and every DStep edge goes forward (to a later location), so every
// macro-step is finite; the location an atomic edge enters has edges that
// continue the sequence, some of them guarded (the holder blocks inside its
// sequence and is stored with the exclusive byte set), some reading another
// process's program counter, some an else, some a channel operation, some
// carrying an assert, a write, or a d_step of their own.
func genAtomic(rnd *rand.Rand) *ir.Model {
	m := randomPORModel(rnd)
	val := func() *ir.Expr { return ir.Const(int64(rnd.Intn(3))) }
	gs := scalarGlobals(m)
	gv := func() string { return gs[rnd.Intn(len(gs))] }
	for p := range m.Processes {
		forwardDSteps(&m.Processes[p], rnd)
	}
	np := len(m.Processes)
	// continueAt makes sure the sequence entered at location to of process p
	// goes on: it adds an edge there when the location has none (or, now and
	// then, anyway), of one of the shapes in the comment above.
	continueAt := func(p, to int, always bool) {
		pr := &m.Processes[p]
		n := len(pr.Locations)
		sink := !hasOut(pr, n-1)
		if hasOut(pr, to) && !always && rnd.Intn(3) != 0 {
			return
		}
		c := ir.Edge{From: to, To: rnd.Intn(n)}
		if sink && c.To == n-1 && rnd.Intn(2) == 0 {
			c.To = rnd.Intn(n - 1)
		}
		switch rnd.Intn(10) {
		case 0, 1:
			c.Guard = ir.Binary("ne", ir.Ref(gv()), val()) // may block the holder
		case 2:
			c.Guard = ir.Binary("eq", ir.Ref(gv()), val())
		case 3:
			if np > 1 {
				j := (p + 1 + rnd.Intn(np-1)) % np
				c.Guard = ir.Binary("eq", ir.PC(j), ir.Const(int64(rnd.Intn(len(m.Processes[j].Locations)))))
			}
		case 4:
			if hasOut(pr, to) {
				c.Else = true
			}
		case 5:
			if len(m.Channels) > 0 {
				ch := m.Channels[rnd.Intn(len(m.Channels))]
				if rnd.Intn(2) == 0 {
					c.Send = &ir.ChanOp{Chan: ch.Name, Args: sendArgs(len(ch.Fields), val)}
				} else {
					c.Recv = &ir.RecvOp{Chan: ch.Name, Args: recvArgs(rnd, len(ch.Fields), gv, val)}
				}
			}
		}
		if rnd.Intn(2) == 0 {
			c.Effect = []ir.Assign{{Var: gv(), Value: val()}}
		}
		if rnd.Intn(6) == 0 {
			c.Assert = ir.Binary("ne", ir.Ref(gv()), ir.Const(2))
		}
		if c.From < c.To && !c.Else && rnd.Intn(6) == 0 {
			c.DStep = true // a d_step continuation: it needs a continuation of its own
			pr.Edges = append(pr.Edges, ir.Edge{From: c.To, To: rnd.Intn(n)})
		}
		pr.Edges = append(pr.Edges, c)
	}
	// A d_step that ends in an atomic edge: the exclusive byte is the one the
	// last edge of the d_step leaves, so a d_step edge that is not atomic starts
	// an atomic sequence, and the closure of its first edge has to cross from the
	// d_step mechanism to the atomic one. One process in four gets such a chain
	// of its own (d_step f to t1, atomic t1 to t2, then a continuation at t2, all
	// going forward), and the d_step edges the model has are given an atomic
	// continuation now and then.
	for p := range m.Processes {
		pr := &m.Processes[p]
		n := len(pr.Locations)
		hi := n - 1
		if !hasOut(pr, n-1) {
			hi = n - 2
		}
		if hi >= 2 && rnd.Intn(4) == 0 {
			f := rnd.Intn(hi - 1)
			t1 := f + 1 + rnd.Intn(hi-f-1)
			t2 := t1 + 1 + rnd.Intn(hi-t1)
			d := ir.Edge{From: f, To: t1, DStep: true}
			if rnd.Intn(2) == 0 {
				d.Effect = []ir.Assign{{Var: gv(), Value: val()}}
			}
			a := ir.Edge{From: t1, To: t2, Atomic: true}
			if rnd.Intn(3) == 0 {
				a.Guard = ir.Binary("ne", ir.Ref(gv()), val())
			}
			pr.Edges = append(pr.Edges, d, a)
			continueAt(p, t2, true)
		}
		for ei, ne := 0, len(pr.Edges); ei < ne; ei++ {
			e := pr.Edges[ei]
			if !e.DStep || e.Atomic || rnd.Intn(3) != 0 {
				continue
			}
			for _, ci := range outOf(pr, e.To) {
				if c := &pr.Edges[ci]; c.From < c.To && !c.Else && !c.Atomic {
					c.Atomic = true
					continueAt(p, c.To, true)
					break
				}
			}
		}
	}
	for p := range m.Processes {
		pr := &m.Processes[p]
		n := len(pr.Locations)
		if n < 2 || rnd.Intn(3) == 0 {
			continue
		}
		sink := !hasOut(pr, n-1)
		for k := 1 + rnd.Intn(2); k > 0; k-- {
			// candidates: forward edges whose target is not a sink
			var cand []int
			for i := range pr.Edges {
				e := &pr.Edges[i]
				if e.From < e.To && !(sink && e.To == n-1) {
					cand = append(cand, i)
				}
			}
			var ei int
			if len(cand) == 0 || rnd.Intn(5) == 0 {
				hi := n - 1
				if sink {
					hi = n - 2
				}
				if hi < 1 {
					continue
				}
				f := rnd.Intn(hi)
				t := f + 1 + rnd.Intn(hi-f)
				e := ir.Edge{From: f, To: t}
				if rnd.Intn(2) == 0 {
					e.Effect = []ir.Assign{{Var: gv(), Value: val()}}
				}
				pr.Edges = append(pr.Edges, e)
				ei = len(pr.Edges) - 1
			} else {
				ei = cand[rnd.Intn(len(cand))]
			}
			e := &pr.Edges[ei]
			e.Atomic = true
			if rnd.Intn(8) == 0 && !e.Else {
				e.DStep = true // an atomic step that is also a d_step
			}
			to := e.To
			continueAt(p, to, false)
			// a longer sequence: one of the continuations is atomic too
			if rnd.Intn(3) == 0 {
				for _, ci := range outOf(pr, to) {
					c := &pr.Edges[ci]
					if c.From < c.To && !(sink && c.To == n-1) && !c.Else {
						c.Atomic = true
						break
					}
				}
			}
		}
	}
	if err := ir.Validate(m); err != nil {
		panic(fmt.Sprintf("genAtomic: invalid model: %v", err))
	}
	return m
}

// genLoop is a process that cycles through an atomic chain with two or three
// branches, beside a process that writes a visible variable and one that
// asserts on it, in both orders of the processes. The branches return to the
// start, to the branch location or leave the cycle, so a cycle of reduced
// states runs through a chain of which only some branches close it. It exists
// for the cycle proviso over chains: a proviso that looks at the first branch
// only gives the right verdicts on every other generator.
func genLoop(rnd *rand.Rand) *ir.Model {
	m := &ir.Model{Schema: ir.Schema, Name: "loop", Globals: []ir.Var{byteVar("x"), byteVar("y")}}
	// The chain is one to three atomic edges, then the branch location b.
	chain := 1 + rnd.Intn(3)
	b := chain
	p := ir.Process{Name: "P", Locals: []ir.Var{byteVar("l")}, Locations: make([]ir.Location, b+3)}
	for i := 0; i < chain; i++ {
		p.Edges = append(p.Edges, ir.Edge{From: i, To: i + 1, Atomic: true})
	}
	nb := 2 + rnd.Intn(2)
	for i := 0; i < nb; i++ {
		to := []int{0, 0, b, b + 1, b + 2}[rnd.Intn(5)]
		e := ir.Edge{From: b, To: to}
		if rnd.Intn(3) == 0 {
			e.Effect = []ir.Assign{{Var: "l", Value: ir.Binary("mod", ir.Binary("add", ir.Ref("l"), ir.Const(1)), ir.Const(2))}}
		}
		p.Edges = append(p.Edges, e)
	}
	if rnd.Intn(2) == 0 {
		p.Edges = append(p.Edges, ir.Edge{From: b + 1, To: 0})
	}
	m.Processes = append(m.Processes, p)
	q := ir.Process{Name: "Q", Locations: locs("q0", "q1", "q2"), Edges: []ir.Edge{{From: 0, To: 1, Effect: []ir.Assign{{Var: "x", Value: ir.Const(1)}}}}}
	if rnd.Intn(2) == 0 {
		q.Edges = append(q.Edges, ir.Edge{From: 1, To: 2, Assert: ir.Binary("ne", ir.Ref("x"), ir.Const(1))})
	}
	m.Processes = append(m.Processes, q)
	if rnd.Intn(2) == 0 { // order of the processes
		m.Processes[0], m.Processes[1] = m.Processes[1], m.Processes[0]
	}
	m.Properties = []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}, {ID: "assert", Kind: ir.KindAssert}}
	if rnd.Intn(2) == 0 {
		m.Properties = append(m.Properties, ir.Property{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("ne", ir.Ref("x"), ir.Const(1))})
	}
	if err := ir.Validate(m); err != nil {
		panic(fmt.Sprintf("genLoop: invalid model: %v", err))
	}
	return m
}

// genProvided gives a third of the processes of a random model a `provided`
// clause over a global, an array element, another process's program counter or
// a local. The reduction refuses these models, so the oracles check that a
// refusal is the full search.
func genProvided(rnd *rand.Rand) *ir.Model {
	m := randomPORModel(rnd)
	gs := scalarGlobals(m)
	for p := range m.Processes {
		if rnd.Intn(3) != 0 {
			continue
		}
		g := gs[rnd.Intn(len(gs))]
		var e *ir.Expr
		switch rnd.Intn(4) {
		case 0:
			e = ir.Binary("ne", ir.Ref(g), ir.Const(int64(rnd.Intn(3))))
		case 1:
			if len(m.Globals) > 3 {
				e = ir.Binary("lt", ir.Index("a", ir.Const(int64(rnd.Intn(2)))), ir.Const(2))
			}
		case 2:
			if len(m.Processes) > 1 {
				e = ir.Binary("ne", ir.PC((p+1)%len(m.Processes)), ir.Const(int64(rnd.Intn(3))))
			}
		}
		if e == nil && len(m.Processes[p].Locals) > 0 {
			e = ir.Binary("ge", ir.Ref("l"), ir.Const(0))
			if rnd.Intn(2) == 0 {
				e = ir.Binary("lt", ir.Ref("l"), ir.Const(2))
			}
		}
		if e != nil {
			m.Processes[p].Provided = e
		}
	}
	if err := ir.Validate(m); err != nil {
		panic(fmt.Sprintf("genProvided: invalid model: %v", err))
	}
	return m
}

// ---- run and the process table ----------------------------------------------------

// runType is a proctype created by `run`: nloc locations (0 dormant, 1 to
// nloc-2 real, nloc-1 the end location), a body shared by every instance of its
// pool, and the pool of interchangeable instances.
type runType struct {
	nloc   int
	pool   []int
	locals []ir.Var
}

// genRun builds models that create processes and read the process table. Three
// modes, by the number drawn first. Frontend-shaped (half of the models): every
// `-end-` edge carries the `youngest` guard and leaves the table, a dynamic
// process goes back to its dormant location with its locals reset. No end edge
// at all (a third): the table is never left, so the `run` is the only writer
// of it and the process that makes it is eligible, which is the only way to
// exercise the footprint of the `run` itself. A dynamic end edge that returns
// to the dormant location without leaving the table (the rest): the analysis
// must refuse, since the dormancy of a pool slot is then not ordered against
// the `run`. One model in twenty has every `run` enter its target at the
// dormant location (check c2 must refuse).
//
// Pools have one to three interchangeable instances; one type in eight has no
// body edge at all (a counter nobody writes but the `run`); `run` edges appear
// in static and in dynamic processes (nested creation); `Args` and `Init` read
// locals and globals; `pid p = run` is an effect that reads `nrpr`; `nrpr`,
// `pid(k)` (also of a process that is not live), `youngest(j)` and
// `pc(j) == 1` of a created process appear in guards, effects and properties.
// Light models touch few globals, so that processes are independent where the
// reduction should act. One model in eight has the table and no `run`.
func genRun(rnd *rand.Rand) *ir.Model { return genRunShape(rnd, false) }

// genNrPr is genRun without any `run`: two to four static processes, every one
// of which leaves the live-process table when it ends (the `-end-` edge carries
// `leave` behind `youngest`), and guards, asserts, effects and properties that
// read the table. It is the shape the Promela frontend gives a model that reads
// `_nr_pr` and starts its processes with `active` (the `_nr_pr` fix, on its own
// branch, which this generator was written for): in genRun the one model in eight
// that has the table and no `run` never leaves it, which is the shape the
// frontend used to emit and no longer does.
func genNrPr(rnd *rand.Rand) *ir.Model { return genRunShape(rnd, true) }

// genRunShape is the body of genRun and genNrPr. noRun changes only draws that
// genRun does not make differently: with it false every draw and every
// decision is the one genRun always made, so the seeds of the earlier
// generators still mean the same models.
func genRunShape(rnd *rand.Rand, noRun bool) *ir.Model {
	m := &ir.Model{Schema: ir.Schema, Name: "run"}
	if noRun {
		m.Name = "nrpr"
	}
	for _, g := range []string{"x", "y", "z"} {
		m.Globals = append(m.Globals, byteVar(g))
	}
	val := func() *ir.Expr { return ir.Const(int64(rnd.Intn(3))) }
	glob := func() string { return []string{"x", "y", "z"}[rnd.Intn(3)] }
	mode := rnd.Intn(20) // 0-9 frontend-shaped end edges, 10-16 none, 17-19 a dynamic end edge that does not leave the table
	nStatic := 1 + rnd.Intn(2)
	nTypes := 1 + rnd.Intn(2)
	hasDyn := rnd.Intn(8) != 0 // one model in eight has the table without any run
	if !hasDyn {
		nTypes = 0
	}
	if noRun {
		hasDyn, nTypes = false, 0
		nStatic = 2 + rnd.Intn(3)
		if mode >= 10 && mode < 17 {
			mode -= 10 // most of the models have the end edges the frontend emits
		}
	}
	// tbl: the model has the live-process table (a process reads it); hasDyn:
	// some process is created by `run`. genRun's models with a table and no run
	// never leave it (the old frontend); genNrPr's leave it.
	tbl := hasDyn || noRun
	// process indices: statics first, then the pools
	types := make([]*runType, nTypes)
	next := nStatic
	for t := range types {
		ty := &runType{nloc: 3 + rnd.Intn(2), locals: []ir.Var{byteVar("a"), byteVar("l")}}
		for j, n := 0, 1+rnd.Intn(3); j < n; j++ {
			ty.pool = append(ty.pool, next)
			next++
		}
		types[t] = ty
	}
	total := next
	rdWord := func() *ir.Expr { // an expression that reads the table or a process counter
		switch rnd.Intn(5) {
		case 0:
			return ir.Binary([]string{"eq", "lt", "ge", "ne"}[rnd.Intn(4)], ir.NrPr(), ir.Const(int64(rnd.Intn(4))))
		case 1:
			return ir.Binary("ge", ir.PID(rnd.Intn(total)), ir.Const(int64(rnd.Intn(3))))
		case 2:
			return ir.Youngest(rnd.Intn(total))
		case 3:
			j := rnd.Intn(total)
			if total > nStatic && rnd.Intn(2) == 0 {
				j = nStatic + rnd.Intn(total-nStatic) // a process that is created: waiting for it to start
				return ir.Binary("eq", ir.PC(j), ir.Const(1))
			}
			return ir.Binary("eq", ir.PC(j), ir.Const(int64(rnd.Intn(3))))
		}
		return ir.Binary("ne", ir.Ref(glob()), val())
	}
	light := rnd.Intn(3) != 0
	pglob := func() bool { return !light || rnd.Intn(8) == 0 }
	entry := 1
	if rnd.Intn(20) == 0 {
		entry = 0 // a run that enters its target at the dormant location: check c2 refuses
	}
	// Most runs go forward, so that they are taken a bounded number of times:
	// a run in a cycle exhausts its pool, an error that both searches reach and
	// that says nothing else about the model.
	forwardRun := func(e *ir.Edge, nloc int) {
		if rnd.Intn(3) != 0 {
			e.From = 1 + rnd.Intn(nloc-2)
			e.To = e.From + 1 + rnd.Intn(nloc-1-e.From)
		}
	}
	// the code of a body: random edges among real locations 1..nloc-2
	body := func(nloc int, maySpawn []int) []ir.Edge {
		var es []ir.Edge
		for k, ne := 0, 2+rnd.Intn(4); k < ne; k++ {
			e := ir.Edge{From: 1 + rnd.Intn(nloc-2), To: 1 + rnd.Intn(nloc-1)}
			switch rnd.Intn(8) {
			case 0:
				if pglob() {
					e.Guard = ir.Binary([]string{"eq", "ne", "lt", "ge"}[rnd.Intn(4)], ir.Ref(glob()), val())
				}
			case 1:
				e.Guard = ir.Binary("lt", ir.Ref("l"), ir.Const(1+int64(rnd.Intn(2))))
			case 2:
				if tbl || rnd.Intn(2) == 0 {
					e.Guard = rdWord()
				}
			case 3:
				e.Guard = ir.Binary("ne", ir.Ref("l"), val())
			}
			switch rnd.Intn(8) {
			case 0:
				if pglob() {
					e.Effect = []ir.Assign{{Var: glob(), Value: val()}}
				}
			case 1:
				if pglob() {
					e.Effect = []ir.Assign{{Var: glob(), Value: ir.Binary("mod", ir.Binary("add", ir.Ref(glob()), ir.Const(1)), ir.Const(3))}}
				}
			case 2, 3:
				e.Effect = []ir.Assign{{Var: "l", Value: ir.Binary("mod", ir.Binary("add", ir.Ref("l"), ir.Const(1)), ir.Const(3))}}
			case 4:
				if tbl && pglob() {
					e.Effect = []ir.Assign{{Var: glob(), Value: ir.Binary("mod", ir.NrPr(), ir.Const(3))}}
				}
			case 5:
				if len(maySpawn) > 0 {
					ty := types[maySpawn[rnd.Intn(len(maySpawn))]]
					r := &ir.RunOp{Proc: ty.pool[0], Entry: entry}
					if len(ty.pool) > 1 || rnd.Intn(2) == 0 {
						r.Pool = ty.pool
					}
					r.Args = []*ir.Expr{ir.Binary("mod", ir.Ref("l"), ir.Const(3))}
					if pglob() && rnd.Intn(2) == 0 {
						r.Args = []*ir.Expr{ir.Binary("mod", ir.Ref(glob()), ir.Const(3))}
					}
					if rnd.Intn(2) == 0 {
						r.Args = []*ir.Expr{val()}
					}
					if rnd.Intn(3) == 0 {
						r.Init = []ir.Assign{{Var: "l", Value: val()}}
					}
					e.Run = r
					forwardRun(&e, nloc)
					if rnd.Intn(4) == 0 { // pid p = run P(): the pid of the new process is nrpr - 1
						e.Effect = []ir.Assign{{Var: "l", Value: ir.Binary("sub", ir.NrPr(), ir.Const(1))}}
					}
				}
			case 6:
				if rnd.Intn(4) == 0 {
					e.Assert = ir.Binary("ne", ir.Ref("l"), ir.Const(2))
					if pglob() {
						e.Assert = ir.Binary("ne", ir.Ref(glob()), ir.Const(2))
					}
				}
			}
			es = append(es, e)
		}
		return es
	}
	// static processes
	for s := 0; s < nStatic; s++ {
		nloc := 3 + rnd.Intn(2) // 0 start, real..., nloc-1 the end location, + a dead location
		pr := ir.Process{Name: fmt.Sprintf("S%d", s), Locals: []ir.Var{byteVar("l")}, Locations: make([]ir.Location, nloc+1)}
		spawn := make([]int, len(types))
		for i := range spawn {
			spawn[i] = i
		}
		pr.Edges = body(nloc, spawn)
		// a creator: most of the time a static process has a run or two of its own
		for k := rnd.Intn(3); hasDyn && k > 0; k-- {
			ty := types[rnd.Intn(len(types))]
			r := &ir.RunOp{Proc: ty.pool[0], Entry: entry, Args: []*ir.Expr{val()}}
			if len(ty.pool) > 1 || rnd.Intn(2) == 0 {
				r.Pool = ty.pool
			}
			if rnd.Intn(2) == 0 {
				r.Init = []ir.Assign{{Var: "l", Value: val()}}
				if rnd.Intn(2) == 0 {
					r.Init = []ir.Assign{{Var: "l", Value: ir.Ref(glob())}} // read in the target's scope
				}
			}
			ed := ir.Edge{From: 1 + rnd.Intn(nloc-2), To: 1 + rnd.Intn(nloc-1), Run: r}
			if rnd.Intn(3) == 0 {
				ed.Effect = []ir.Assign{{Var: "l", Value: ir.Binary("sub", ir.NrPr(), ir.Const(1))}}
				if rnd.Intn(3) == 0 {
					ed.Effect = []ir.Assign{{Var: "l", Value: ir.Binary("mod", ir.Binary("add", ir.Ref(glob()), ir.NrPr()), ir.Const(3))}}
				}
			}
			if rnd.Intn(2) == 0 {
				r.Args = []*ir.Expr{ir.Binary("mod", ir.Ref(glob()), ir.Const(3))}
			}
			forwardRun(&ed, nloc)
			pr.Edges = append(pr.Edges, ed)
		}
		// a writer of a global beside the creator
		if s > 0 && rnd.Intn(2) == 0 {
			pr.Edges = append(pr.Edges, ir.Edge{From: 1 + rnd.Intn(nloc-2), To: 1 + rnd.Intn(nloc-1), Effect: []ir.Assign{{Var: glob(), Value: val()}}})
		}
		// the start edge: location 0 -> 1
		pr.Edges = append(pr.Edges, ir.Edge{From: 0, To: 1})
		// the end edge
		end := ir.Edge{From: nloc - 1, To: nloc, Text: "-end-"}
		if hasDyn || noRun {
			end.Guard = ir.Youngest(s)
			end.Leave = true
		}
		if mode < 10 || mode >= 17 {
			pr.Edges = append(pr.Edges, end)
		}
		m.Processes = append(m.Processes, pr)
	}
	// A model that reads `_nr_pr` has the table because of it: most of the models
	// of genNrPr get one edge that waits for, or tests, the count (the frontend
	// only keeps the table for a model that reads it; the rest read it through
	// the guards of their end edges only, which `youngest` is).
	if noRun && rnd.Intn(5) != 0 {
		p := &m.Processes[rnd.Intn(nStatic)]
		nloc := len(p.Locations) - 1
		cmp := []string{"eq", "lt", "ge", "ne"}[rnd.Intn(4)]
		p.Edges = append(p.Edges, ir.Edge{From: 1 + rnd.Intn(nloc-2), To: 1 + rnd.Intn(nloc-1),
			Guard: ir.Binary(cmp, ir.NrPr(), ir.Const(int64(rnd.Intn(4)))), Text: "_nr_pr guard"})
	}
	// A waiter: a static process with a free edge and an edge guarded by the
	// program counter of a created process (any member of a pool), which only a
	// `run` enables, at its first real location; the guarded edge leads to a
	// location of its own, whose one edge is a failed assert (an assert on the
	// guarded edge itself would make the location ineligible and the trap
	// vacuous). If the run does not write the counter, the waiter expanded alone
	// never takes the guarded edge (section 4.4, trap 3).
	if hasDyn && rnd.Intn(2) == 0 {
		w := &m.Processes[rnd.Intn(nStatic)]
		ty := types[rnd.Intn(len(types))]
		k := rnd.Intn(len(ty.pool))
		if len(ty.pool) > 1 && rnd.Intn(3) != 0 {
			k = 1 + rnd.Intn(len(ty.pool)-1) // most waiters wait for a member after the first
		}
		q := ty.pool[k]
		// The member q is the k-th of its pool, started by the k-th `run` of
		// the pool: a creator makes that many, one after the other.
		if k > 0 {
			c := &m.Processes[rnd.Intn(nStatic)]
			cend := len(c.Locations) - 2
			prev := 1
			for i := 0; i <= k; i++ {
				c.Locations = append(c.Locations, ir.Location{})
				to := len(c.Locations) - 1
				c.Edges = append(c.Edges, ir.Edge{From: prev, To: to, Run: &ir.RunOp{Proc: ty.pool[0], Pool: ty.pool, Entry: entry, Args: []*ir.Expr{val()}}, Text: "run D()"})
				prev = to
			}
			c.Edges = append(c.Edges, ir.Edge{From: prev, To: cend})
		}
		last := len(w.Locations) - 2 // the end location is len-2: the last is the dead location
		w.Locations = append(w.Locations, ir.Location{})
		mid := len(w.Locations) - 1
		w.Edges = append(w.Edges,
			ir.Edge{From: 1, To: last, Text: "free"},
			ir.Edge{From: 1, To: mid, Guard: ir.Binary("eq", ir.PC(q), ir.Const(1)), Text: "pc(D) == 1"},
			ir.Edge{From: mid, To: last, Assert: ir.Const(0), Text: "assert(false)"})
	}
	for t, ty := range types {
		var spawn []int
		for u := t + 1; u < len(types); u++ {
			spawn = append(spawn, u)
		}
		code := body(ty.nloc, spawn)
		if rnd.Intn(8) == 0 {
			code = nil // a process with nothing to do: only the program counter says it is alive
		}
		for _, k := range ty.pool {
			pr := ir.Process{Name: fmt.Sprintf("D%d_%d", t, k), Locals: append([]ir.Var(nil), ty.locals...), Params: 1,
				Locations: make([]ir.Location, ty.nloc), Dynamic: true, Initial: 0}
			pr.Edges = append(pr.Edges, code...)
			endE := ir.Edge{From: ty.nloc - 1, To: 0, Guard: ir.Youngest(k), Leave: true, Text: "-end-",
				Effect: []ir.Assign{{Var: "a", Value: ir.Const(0)}, {Var: "l", Value: ir.Const(0)}}}
			if mode >= 17 {
				endE.Guard, endE.Leave = nil, false // returns to the dormant location and stays in the table
			}
			if mode < 10 || mode >= 17 {
				pr.Edges = append(pr.Edges, endE)
			}
			m.Processes = append(m.Processes, pr)
		}
	}
	m.Properties = []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}}
	hasAssert := false
	for p := range m.Processes {
		for _, e := range m.Processes[p].Edges {
			if e.Assert != nil {
				hasAssert = true
			}
		}
	}
	if hasAssert {
		m.Properties = append(m.Properties, ir.Property{ID: "assert", Kind: ir.KindAssert})
	}
	if rnd.Intn(10) < 7 {
		var expr *ir.Expr
		switch rnd.Intn(4) {
		case 0:
			expr = ir.Binary("ne", ir.Ref(glob()), ir.Const(int64(1+rnd.Intn(2))))
		case 1:
			if tbl {
				expr = ir.Binary("ne", ir.NrPr(), ir.Const(int64(2+rnd.Intn(3))))
			}
		case 2:
			expr = ir.Unary("not", ir.And(ir.Binary("eq", ir.Ref(glob()), val()), ir.Binary("eq", ir.Ref(glob()), val())))
		}
		if expr == nil {
			expr = ir.Binary("ne", ir.Ref(glob()), ir.Const(2))
		}
		m.Properties = append(m.Properties, ir.Property{ID: "inv", Kind: ir.KindInvariant, Expr: expr})
	}
	if rnd.Intn(10) < 5 {
		var expr *ir.Expr
		if tbl && rnd.Intn(2) == 0 {
			expr = ir.Binary("eq", ir.NrPr(), ir.Const(int64(1+rnd.Intn(3))))
		} else {
			expr = ir.And(ir.Binary("eq", ir.Ref(glob()), ir.Const(2)), ir.Binary("ne", ir.Ref(glob()), ir.Const(0)))
		}
		m.Properties = append(m.Properties, ir.Property{ID: "reach", Kind: ir.KindReach, Expr: expr})
	}
	if rnd.Intn(8) == 0 {
		// The order of the processes: a creator picked first by index masks a
		// missing conflict with a process that is picked later (section 4.4,
		// trap 3). A model lists its static processes first by construction,
		// so rotate the static ones among themselves.
		if nStatic >= 2 {
			idx := make([]int, len(m.Processes))
			for i := range idx {
				idx[i] = i
			}
			idx[0], idx[1] = 1, 0
			renumberProcesses(m, idx)
		}
	}
	if err := ir.Validate(m); err != nil {
		panic(fmt.Sprintf("genRun: invalid model: %v", err))
	}
	return m
}

// renumberProcesses puts process i of m at position newIndex[i] and renames the
// process index wherever one appears: the `pc`, `pid` and `youngest`
// expressions, and the `Run` operations. The result is the same model with its
// processes in another order, which is what a trap that depends on which
// process pick looks at first needs.
func renumberProcesses(m *ir.Model, newIndex []int) {
	procs := make([]ir.Process, len(m.Processes))
	for i, pr := range m.Processes {
		procs[newIndex[i]] = pr
	}
	m.Processes = procs
	seen := map[*ir.Expr]bool{} // the edges of a pool share their expressions
	var walk func(e *ir.Expr)
	walk = func(e *ir.Expr) {
		if e == nil || seen[e] {
			return
		}
		seen[e] = true
		switch e.Op {
		case "pc", "pid", "youngest":
			e.Value = int64(newIndex[e.Value])
		}
		for _, x := range e.Args {
			walk(x)
		}
	}
	pools := map[*int]bool{}
	for p := range m.Processes {
		pr := &m.Processes[p]
		walk(pr.Provided)
		for i := range pr.Edges {
			e := &pr.Edges[i]
			walk(e.Guard)
			walk(e.Assert)
			for _, as := range e.Effect {
				walk(as.Index)
				walk(as.Value)
			}
			if e.Send != nil {
				walk(e.Send.Sel)
				for _, x := range e.Send.Args {
					walk(x)
				}
			}
			if e.Recv != nil {
				walk(e.Recv.Sel)
				for _, ra := range e.Recv.Args {
					walk(ra.Match)
					walk(ra.Index)
				}
			}
			if e.Run != nil {
				e.Run.Proc = newIndex[e.Run.Proc]
				for k := range e.Run.Pool {
					if len(e.Run.Pool) > 0 && !pools[&e.Run.Pool[k]] { // a pool slice is shared by the edges that run it
						pools[&e.Run.Pool[k]] = true
						e.Run.Pool[k] = newIndex[e.Run.Pool[k]]
					}
				}
				for _, x := range e.Run.Args {
					walk(x)
				}
				for _, as := range e.Run.Init {
					walk(as.Index)
					walk(as.Value)
				}
			}
		}
	}
	for i := range m.Properties {
		walk(m.Properties[i].Expr)
	}
}

// reordered is a copy of m whose processes are in the given order: order[k] is
// the index in m of the process that goes to position k.
func reordered(t testing.TB, m *ir.Model, order ...int) *ir.Model {
	t.Helper()
	b, err := ir.MarshalJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	c, err := ir.UnmarshalJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	newIndex := make([]int, len(order))
	for k, old := range order {
		newIndex[old] = k
	}
	renumberProcesses(c, newIndex)
	return c
}

// genRunAtomic is genRun with atomic sequences around some of the steps, the
// mix of the Promela idiom `init { atomic { run A(); run B() } }`: forward
// edges, `run` edges first among them, are marked atomic, so a creating step
// continues with the creator's next step, whose guard may block the holder
// (an `-end-` edge guarded by `youngest` does).
func genRunAtomic(rnd *rand.Rand) *ir.Model {
	m := genRun(rnd)
	for p := range m.Processes {
		pr := &m.Processes[p]
		if rnd.Intn(2) == 0 {
			continue
		}
		var cand []int
		for i := range pr.Edges {
			e := &pr.Edges[i]
			if e.From < e.To && !e.Leave && hasOut(pr, e.To) {
				cand = append(cand, i)
				if e.Run != nil {
					cand = append(cand, i, i) // runs are the point
				}
			}
		}
		for k := 1 + rnd.Intn(2); k > 0 && len(cand) > 0; k-- {
			e := &pr.Edges[cand[rnd.Intn(len(cand))]]
			e.Atomic = true
			if rnd.Intn(8) == 0 && !e.Else && e.Run == nil {
				e.DStep = true
			}
		}
	}
	for p := range m.Processes {
		forwardDSteps(&m.Processes[p], rnd)
	}
	if err := ir.Validate(m); err != nil {
		panic(fmt.Sprintf("genRunAtomic: invalid model: %v", err))
	}
	return m
}

// ---- reads of a global that another process writes --------------------------------

// The generators above send and match constants and index an array with a local,
// so the footprint clauses for a send's arguments, a receive's match and index
// and an effect's index (each a READ of whatever the expression names) are
// exercised only by the models in which some other clause happens to hide the
// same hole. A hole in one of them is a step whose enabledness or whose effect
// depends on a global that another process writes, expanded alone. genReads and
// genAtomicReads build those steps on purpose, on the base and pipeline
// generators: a writer of the scalars beside the others, send arguments and
// receive matches that read a scalar, a receive that binds an element of an
// array chosen by a scalar, an effect on an array element chosen by a scalar.
// The arrays that a scalar indexes are, but for one case, the process's own
// (b<p>, touched by nobody else), so that the step is still eligible and a
// footprint that has forgotten the read would expand it alone; an array shared
// by every process would make every such step dependent on the others by its
// write alone. The one case is the effects on the shared array a that the base
// generator makes: readShapes indexes some of them by a scalar too, and there the
// whole-array write conflicts anyway, so the read cannot show. readShapesOf
// counts the two apart: of 600 models, 41 of the 285 `reads` models and 94 of the
// 438 `atomic-reads` models that have the effect-index shape have it only on a
// (TestPORReadsGeneratorsMakeTheirShapes logs the figures).

// scalarRead returns a read of a scalar global, now and then plus one modulo 3.
func scalarRead(rnd *rand.Rand, gs []string) *ir.Expr {
	g := ir.Ref(gs[rnd.Intn(len(gs))])
	if rnd.Intn(3) == 0 {
		return ir.Binary("mod", ir.Binary("add", g, ir.Const(1)), ir.Const(3))
	}
	return g
}

// readShapes rewrites m in place (see above). A fifth shape, an assert that reads
// a scalar of its own that only the writer touches (t: set to 1 and back to 0),
// is drawn in one model in three, but only a model of at most three processes can
// take it, so it is made in about one model in five (667 of 3 000 models of
// `reads` and 663 of `atomic-reads` at the default seeds, 22%; the calibration
// test counts it): the writer is the only process that could be expanded alone
// past the moment when the assert would see t == 1, so a footprint that forgets
// the reads of an assert lets the writer go first.
func readShapes(rnd *rand.Rand, m *ir.Model) {
	gs := scalarGlobals(m)
	val := func() *ir.Expr { return ir.Const(int64(rnd.Intn(3))) }
	// A writer of a scalar beside the others: a pipeline model has a scalar that
	// no process writes. It goes at a random place in the order of the processes
	// (pick takes the first eligible one by index).
	writes := false
	for p := range m.Processes {
		for _, e := range m.Processes[p].Edges {
			for _, as := range e.Effect {
				writes = writes || (as.Index == nil && as.Var != "l")
			}
			if e.Recv != nil {
				for _, ra := range e.Recv.Args {
					writes = writes || (ra.Var != "" && ra.Var != "l" && ra.Index == nil)
				}
			}
		}
	}
	useT := rnd.Intn(3) == 0
	writer := -1
	if len(m.Processes) <= 3 && (!writes || useT || rnd.Intn(3) == 0) {
		g := gs[rnd.Intn(len(gs))]
		w := ir.Process{Name: "Wr", Locations: make([]ir.Location, 3)}
		w.Edges = append(w.Edges, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{{Var: g, Value: ir.Const(int64(1 + rnd.Intn(2)))}}})
		switch k := rnd.Intn(6); {
		case useT:
			w.Locations = make([]ir.Location, 4)
			m.Globals = append(m.Globals, byteVar("t"))
			w.Edges = append(w.Edges,
				ir.Edge{From: 1, To: 2, Effect: []ir.Assign{set("t", 1)}},
				ir.Edge{From: 2, To: 3, Effect: []ir.Assign{set("t", 0)}})
		case k < 3:
			w.Edges = append(w.Edges, ir.Edge{From: 1, To: 2, Effect: []ir.Assign{{Var: g, Value: val()}}})
		case k == 3: // a writer that goes on for ever: the scalar cycles through 0, 1, 2
			w.Edges = append(w.Edges, ir.Edge{From: 1, To: 0, Effect: []ir.Assign{{Var: g, Value: ir.Binary("mod", ir.Binary("add", ir.Ref(g), ir.Const(1)), ir.Const(3))}}})
		}
		n := len(m.Processes)
		m.Processes = append(m.Processes, w)
		pos := rnd.Intn(n + 1)
		idx := make([]int, n+1)
		for i := 0; i < n; i++ {
			idx[i] = i
			if i >= pos {
				idx[i] = i + 1
			}
		}
		idx[n] = pos
		renumberProcesses(m, idx)
		writer = pos
		if useT {
			// the asserting edge, in a process other than the writer
			p := rnd.Intn(n + 1)
			if p == writer {
				p = (p + 1) % (n + 1)
			}
			pr := &m.Processes[p]
			pr.Edges = append(pr.Edges, ir.Edge{From: rnd.Intn(len(pr.Locations)), To: rnd.Intn(len(pr.Locations)),
				Assert: ir.Binary("ne", ir.Ref("t"), ir.Const(1))})
		}
	}
	index := func() *ir.Expr { return ir.Binary("mod", ir.Ref(gs[rnd.Intn(len(gs))]), ir.Const(2)) }
	own := map[int]string{}
	array := func(p int) string { // the array of process p, declared on first use
		if n, ok := own[p]; ok {
			return n
		}
		n := fmt.Sprintf("b%d", p)
		m.Globals = append(m.Globals, ir.Var{Name: n, Type: ir.Byte, Len: 2})
		own[p] = n
		return n
	}
	for p := range m.Processes {
		pr := &m.Processes[p]
		for ei := range pr.Edges {
			e := &pr.Edges[ei]
			if e.Send != nil {
				for i := range e.Send.Args {
					if rnd.Intn(3) == 0 {
						e.Send.Args[i] = scalarRead(rnd, gs)
					}
				}
			}
			if e.Recv != nil {
				for i := range e.Recv.Args {
					ra := &e.Recv.Args[i]
					switch {
					case ra.Match != nil:
						if rnd.Intn(2) == 0 {
							ra.Match = scalarRead(rnd, gs)
						}
					case ra.Var == "":
						if rnd.Intn(4) == 0 {
							ra.Match = scalarRead(rnd, gs)
						}
					default:
						if rnd.Intn(4) == 0 {
							ra.Var, ra.Index = array(p), index()
						}
					}
				}
			}
			for i := range e.Effect {
				if as := &e.Effect[i]; as.Var == "a" && rnd.Intn(2) == 0 {
					as.Index = index() // the shared array, indexed by a scalar
				}
			}
			if p != writer && e.Send == nil && e.Recv == nil && e.Run == nil && len(e.Effect) == 0 && !e.Else && e.Assert == nil && rnd.Intn(6) == 0 {
				e.Effect = []ir.Assign{{Var: array(p), Index: index(), Value: val()}}
			}
		}
	}
}

// genReads is the pipeline generator (two models in three) or the base
// generator with readShapes applied.
func genReads(rnd *rand.Rand) *ir.Model {
	var m *ir.Model
	if rnd.Intn(3) != 0 {
		m = randomPipelineModel(rnd)
	} else {
		m = randomPORModel(rnd)
	}
	readShapes(rnd, m)
	if err := ir.Validate(m); err != nil {
		panic(fmt.Sprintf("genReads: invalid model: %v", err))
	}
	return m
}

// genAtomicReads is genAtomic with readShapes applied: the sends, matches and
// array effects of the continuations of a sequence read a scalar too.
func genAtomicReads(rnd *rand.Rand) *ir.Model {
	m := genAtomic(rnd)
	readShapes(rnd, m)
	if err := ir.Validate(m); err != nil {
		panic(fmt.Sprintf("genAtomicReads: invalid model: %v", err))
	}
	return m
}

// TestPORGeneratorsProduceValidModels keeps the generators honest: every
// model validates (the generators panic otherwise), is deterministic in its
// seed, and every Atomic and DStep edge of an atomic model goes forward.
func TestPORGeneratorsProduceValidModels(t *testing.T) {
	for _, g := range porGenerators {
		t.Run(g.name, func(t *testing.T) {
			for seed := g.first; seed < g.first+300; seed++ {
				a := g.gen(rand.New(rand.NewSource(seed)))
				b := g.gen(rand.New(rand.NewSource(seed)))
				ja, err1 := json.Marshal(a)
				jb, err2 := json.Marshal(b)
				if err1 != nil || err2 != nil || string(ja) != string(jb) {
					t.Fatalf("seed %d: the generator is not deterministic (%v %v)", seed, err1, err2)
				}
				if _, err := compile(a); err != nil {
					t.Fatalf("seed %d: %v", seed, err)
				}
				if g.name != "atomic" && g.name != "run-atomic" && g.name != "atomic-reads" {
					continue
				}
				for p := range a.Processes {
					for i, e := range a.Processes[p].Edges {
						if (e.Atomic || e.DStep) && e.From >= e.To {
							t.Fatalf("seed %d: process %d edge %d is atomic or d_step and goes back (%d to %d)", seed, p, i, e.From, e.To)
						}
					}
				}
			}
		})
	}
}

// readShapesOf says which of the shapes of readShapes a model has, each with a
// scalar that ANOTHER process writes: a send argument that reads it, a receive
// match, a receive that binds an array element indexed by it, an effect on an
// array element indexed by it (effectIndexPrivate: on an array of the process's
// own, b<p>; the shared array a is the only other one the generators make), and,
// apart from that, an assert that reads the scalar t.
type readShapeSet struct{ send, match, recvIndex, effectIndex, effectIndexPrivate, assertT bool }

func readShapesOf(m *ir.Model) readShapeSet {
	scalar := map[string]bool{}
	for _, g := range m.Globals {
		if g.Len == 0 {
			scalar[g.Name] = true
		}
	}
	written := make([]map[string]bool, len(m.Processes)) // the scalars each process writes
	for p := range m.Processes {
		written[p] = map[string]bool{}
		for _, e := range m.Processes[p].Edges {
			for _, as := range e.Effect {
				if scalar[as.Var] {
					written[p][as.Var] = true
				}
			}
			if e.Recv != nil {
				for _, ra := range e.Recv.Args {
					if scalar[ra.Var] {
						written[p][ra.Var] = true
					}
				}
			}
		}
	}
	// readsOther: expression e, in process p, reads a scalar some other process writes.
	var readsOther func(e *ir.Expr, p int) bool
	readsOther = func(e *ir.Expr, p int) bool {
		if e == nil {
			return false
		}
		if e.Op == "var" && scalar[e.Var] {
			for q := range written {
				if q != p && written[q][e.Var] {
					return true
				}
			}
		}
		for _, x := range e.Args {
			if readsOther(x, p) {
				return true
			}
		}
		return false
	}
	var c readShapeSet
	for p := range m.Processes {
		for _, e := range m.Processes[p].Edges {
			if e.Send != nil {
				for _, x := range e.Send.Args {
					c.send = c.send || readsOther(x, p)
				}
			}
			if e.Recv != nil {
				for _, ra := range e.Recv.Args {
					c.match = c.match || readsOther(ra.Match, p)
					c.recvIndex = c.recvIndex || readsOther(ra.Index, p)
				}
			}
			for _, as := range e.Effect {
				if readsOther(as.Index, p) {
					c.effectIndex = true
					c.effectIndexPrivate = c.effectIndexPrivate || as.Var != "a"
				}
			}
			c.assertT = c.assertT || mentions(e.Assert, "t")
		}
	}
	return c
}

// mentions says whether the expression reads the variable.
func mentions(e *ir.Expr, name string) bool {
	if e == nil {
		return false
	}
	if e.Op == "var" && e.Var == name {
		return true
	}
	for _, x := range e.Args {
		if mentions(x, name) {
			return true
		}
	}
	return false
}

// TestPORReadsGeneratorsMakeTheirShapes is the calibration of genReads and
// genAtomicReads by their own statistics: each shape must occur in a good share
// of the models. The base generators are counted for comparison (the figures are
// logged): they make the shapes rarely or never. Two counters say what the
// effect-index shape is made of: it is the process's own array (b<p>, where no
// other process conflicts by a write and a footprint that forgot the read would
// expand the step alone) or the shared array a (where the whole-array write
// conflicts anyway and the read cannot show); and the assert on t is counted,
// because only models of at most three processes can take it.
func TestPORReadsGeneratorsMakeTheirShapes(t *testing.T) {
	const models = 600
	for _, name := range []string{"base", "atomic", "reads", "atomic-reads"} {
		g, _ := porGeneratorByName(name)
		var send, match, recvIndex, effectIndex, private, sharedOnly, assertT int
		for seed := g.first; seed < g.first+models; seed++ {
			n := readShapesOf(g.gen(newRand(seed)))
			if n.send {
				send++
			}
			if n.match {
				match++
			}
			if n.recvIndex {
				recvIndex++
			}
			if n.effectIndex {
				effectIndex++
			}
			if n.effectIndexPrivate {
				private++
			}
			if n.effectIndex && !n.effectIndexPrivate {
				sharedOnly++
			}
			if n.assertT {
				assertT++
			}
		}
		t.Logf("%-12s of %d models: %d have a send argument, %d a receive match, %d a receive index, %d an effect index that reads a scalar another process writes (%d on the process's own array, %d only on the shared array a); %d an assert on t",
			name, models, send, match, recvIndex, effectIndex, private, sharedOnly, assertT)
		if name != "reads" && name != "atomic-reads" {
			continue
		}
		for what, got := range map[string]int{"send argument": send, "receive match": match, "receive index": recvIndex, "effect index": effectIndex,
			"effect index on the process's own array": private, "assert on t": assertT} {
			if got < models/20 {
				t.Errorf("%s: only %d of %d models have a %s that reads a scalar another process writes", name, got, models, what)
			}
		}
	}
}
