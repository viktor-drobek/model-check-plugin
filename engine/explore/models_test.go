package explore

import (
	"fmt"

	"modelcheck/ir"
)

// Hand-built IR models for the explorer tests. They mirror the spike's
// hard-coded models so that the state counts checked there (and against
// pan) carry over: petrinet1 = 6 markings, mutex_flaw = 429 states,
// counters = K^N.

type pt struct {
	name    string
	in, out []string
}

// petriIR encodes a P/T net the way frontend/petri does (one looping
// process, one edge per transition, guard = inputs > 0, effect = decrements
// then increments). Kept here so explore tests do not depend on the
// frontend.
func petriIR(name string, places []string, marking map[string]int64, ts []pt, extra ...ir.Property) *ir.Model {
	m := &ir.Model{Schema: ir.Schema, Name: name}
	for _, p := range places {
		m.Globals = append(m.Globals, ir.Var{Name: p, Type: ir.Byte, Init: []int64{marking[p]}})
	}
	pr := ir.Process{Name: "init", Locations: []ir.Location{{Name: "do"}}}
	for _, t := range ts {
		var guards []*ir.Expr
		var eff []ir.Assign
		for _, p := range t.in {
			guards = append(guards, ir.Binary("gt", ir.Ref(p), ir.Const(0)))
			eff = append(eff, ir.Assign{Var: p, Value: ir.Binary("sub", ir.Ref(p), ir.Const(1))})
		}
		for _, p := range t.out {
			eff = append(eff, ir.Assign{Var: p, Value: ir.Binary("add", ir.Ref(p), ir.Const(1))})
		}
		pr.Edges = append(pr.Edges, ir.Edge{Guard: ir.And(guards...), Effect: eff, Text: t.name, Origin: &ir.Origin{Name: t.name}})
	}
	m.Processes = []ir.Process{pr}
	m.Properties = append([]ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}}, extra...)
	return m
}

func petrinet1() *ir.Model {
	return petriIR("petrinet1",
		[]string{"p1", "p2", "p3", "p4", "p5", "p6"},
		map[string]int64{"p1": 1, "p4": 1},
		[]pt{
			{"t1", []string{"p1"}, []string{"p2"}},
			{"t2", []string{"p2", "p4"}, []string{"p3"}},
			{"t3", []string{"p3"}, []string{"p1", "p4"}},
			{"t4", []string{"p4"}, []string{"p5"}},
			{"t5", []string{"p1", "p5"}, []string{"p6"}},
			{"t6", []string{"p6"}, []string{"p4", "p1"}},
		})
}

// mutexFlaw is CH2/mutex_flaw.pml as in the spike: locations L1..L9 (index
// 1..9, index 0 unused so that pc values read like the labels).
func mutexFlaw() *ir.Model {
	m := &ir.Model{Schema: ir.Schema, Name: "mutex_flaw",
		Globals: []ir.Var{{Name: "cnt", Type: ir.Byte}, {Name: "x", Type: ir.Byte}, {Name: "y", Type: ir.Byte}, {Name: "z", Type: ir.Byte}}}
	for pid := 0; pid < 2; pid++ {
		me := ir.Const(int64(pid + 1))
		pr := ir.Process{Name: fmt.Sprintf("user%d", pid), Initial: 1}
		for i := 0; i < 10; i++ {
			pr.Locations = append(pr.Locations, ir.Location{Name: fmt.Sprintf("L%d", i)})
		}
		set := func(v string) []ir.Assign { return []ir.Assign{{Var: v, Value: me}} }
		pr.Edges = []ir.Edge{
			{From: 1, To: 2, Effect: set("x"), Text: "x = me"},
			{From: 2, To: 1, Guard: ir.Binary("and", ir.Binary("ne", ir.Ref("y"), ir.Const(0)), ir.Binary("ne", ir.Ref("y"), me)), Text: "(y != 0 && y != me) -> goto L1"},
			{From: 2, To: 3, Guard: ir.Binary("or", ir.Binary("eq", ir.Ref("y"), ir.Const(0)), ir.Binary("eq", ir.Ref("y"), me)), Text: "(y == 0 || y == me)"},
			{From: 3, To: 4, Effect: set("z"), Text: "z = me"},
			{From: 4, To: 1, Guard: ir.Binary("ne", ir.Ref("x"), me), Text: "(x != me) -> goto L1"},
			{From: 4, To: 5, Guard: ir.Binary("eq", ir.Ref("x"), me), Text: "(x == me)"},
			{From: 5, To: 6, Effect: set("y"), Text: "y = me"},
			{From: 6, To: 1, Guard: ir.Binary("ne", ir.Ref("z"), me), Text: "(z != me) -> goto L1"},
			{From: 6, To: 7, Guard: ir.Binary("eq", ir.Ref("z"), me), Text: "(z == me)"},
			{From: 7, To: 8, Effect: []ir.Assign{{Var: "cnt", Value: ir.Binary("add", ir.Ref("cnt"), ir.Const(1))}}, Text: "cnt++"},
			{From: 8, To: 9, Assert: ir.Binary("eq", ir.Ref("cnt"), ir.Const(1)), Text: "assert(cnt == 1)"},
			{From: 9, To: 1, Effect: []ir.Assign{{Var: "cnt", Value: ir.Binary("sub", ir.Ref("cnt"), ir.Const(1))}}, Text: "cnt--"},
		}
		m.Processes = append(m.Processes, pr)
	}
	m.Properties = []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}, {ID: "assert", Kind: ir.KindAssert}}
	return m
}

// counters is the synthetic K^N model: N processes each with a byte counter
// incremented modulo K.
func counters(k, n int) *ir.Model {
	m := &ir.Model{Schema: ir.Schema, Name: fmt.Sprintf("counters(K=%d, N=%d)", k, n),
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}}}
	for i := 0; i < n; i++ {
		m.Processes = append(m.Processes, ir.Process{
			Name:      fmt.Sprintf("P%d", i),
			Locals:    []ir.Var{{Name: "c", Type: ir.Byte}},
			Locations: []ir.Location{{Name: "do"}},
			Edges: []ir.Edge{{Effect: []ir.Assign{{Var: "c",
				Value: ir.Binary("mod", ir.Binary("add", ir.Ref("c"), ir.Const(1)), ir.Const(int64(k)))}}, Text: "c = (c + 1) % K"}},
		})
	}
	return m
}

// porVisible is the model of testdata/ir/por-visible.json (the G7 feature):
// A writes the global x twice (x = 1, then x = 2) while B and C each count a
// private byte to 3. The invariant x != 2 is violated by A's second write; the
// reach property x == 1 holds. x is read by both properties, so A's writes
// are visible and must never be postponed behind B's and C's steps.
func porVisible() *ir.Model {
	counter := func(name string) ir.Process {
		return ir.Process{
			Name:      name,
			Locals:    []ir.Var{{Name: "c", Type: ir.Byte}},
			Locations: []ir.Location{{Name: "count"}, {Name: "done"}},
			Edges: []ir.Edge{
				{From: 0, To: 0, Guard: ir.Binary("lt", ir.Ref("c"), ir.Const(3)),
					Effect: []ir.Assign{{Var: "c", Value: ir.Binary("add", ir.Ref("c"), ir.Const(1))}}, Text: "c < 3 -> c++"},
				{From: 0, To: 1, Guard: ir.Binary("ge", ir.Ref("c"), ir.Const(3)), Text: "c >= 3"},
			},
		}
	}
	return &ir.Model{Schema: ir.Schema, Name: "por-visible",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte}},
		Processes: []ir.Process{
			{Name: "A", Locations: []ir.Location{{Name: "a0"}, {Name: "a1"}, {Name: "a2"}}, Edges: []ir.Edge{
				{From: 0, To: 1, Effect: []ir.Assign{{Var: "x", Value: ir.Const(1)}}, Text: "x = 1"},
				{From: 1, To: 2, Effect: []ir.Assign{{Var: "x", Value: ir.Const(2)}}, Text: "x = 2"},
			}},
			counter("B"), counter("C"),
		},
		Properties: []ir.Property{
			{ID: "deadlock", Kind: ir.KindDeadlock},
			{ID: "inv", Kind: ir.KindInvariant, Expr: ir.Binary("ne", ir.Ref("x"), ir.Const(2)), Text: "x != 2"},
			{ID: "can1", Kind: ir.KindReach, Expr: ir.Binary("eq", ir.Ref("x"), ir.Const(1)), Text: "x == 1"},
		},
	}
}

// porEnabling is testdata/ir/por-enabling.json: P at location 0 has a free
// edge `f` (0 -> 1) and an edge `e` (0 -> 2) guarded by pc(Q) == 1; Q has one
// step, 0 -> 1. In the full graph Q's step enables `e`, and P then goes on to
// 2 where it waits for x == 5, which nobody ever writes: a deadlock reached
// only through `e`. Expanding `f` alone would never reach it. (Found by
// cross-review of the first version of the pc refinement.)
func porEnabling() *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "por-enabling",
		Globals: []ir.Var{byteVar("x")},
		Processes: []ir.Process{
			{Name: "P", Locations: locs("l0", "l1", "l2", "l3"), Edges: []ir.Edge{
				{From: 0, To: 1, Text: "f"},
				{From: 0, To: 2, Guard: ir.Binary("eq", ir.PC(1), ir.Const(1)), Text: "e: pc(Q) == 1"},
				{From: 2, To: 3, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(5)), Text: "x == 5"},
			}},
			{Name: "Q", Locations: locs("d", "c"), Edges: []ir.Edge{{From: 0, To: 1, Text: "enter c"}}},
		},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}},
	}
}

// porDStepGuard is testdata/ir/por-dstep.json: the same trap one step
// removed. P's first edge is a d_step; the edge it goes on into is chosen
// when the step is made, and one of the candidates is guarded by pc(Q) == 1,
// so the result of the step depends on whether Q has entered location 1.
func porDStepGuard() *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "por-dstep",
		Globals: []ir.Var{byteVar("x")},
		Processes: []ir.Process{
			{Name: "P", Locations: locs("l0", "l1", "l2", "l3", "l4"), Edges: []ir.Edge{
				{From: 0, To: 1, DStep: true, Text: "d_step start"},
				{From: 1, To: 2, Guard: ir.Binary("eq", ir.PC(1), ir.Const(1)), Text: "pc(Q) == 1"},
				{From: 1, To: 3, Text: "otherwise"},
				{From: 2, To: 4, Guard: ir.Binary("eq", ir.Ref("x"), ir.Const(5)), Text: "x == 5"},
			}},
			{Name: "Q", Locations: locs("d", "c"), Edges: []ir.Edge{{From: 0, To: 1, Text: "enter c"}}},
		},
		Properties: []ir.Property{{ID: "deadlock", Kind: ir.KindDeadlock}},
	}
}
