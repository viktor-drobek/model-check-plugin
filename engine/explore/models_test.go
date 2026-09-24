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
