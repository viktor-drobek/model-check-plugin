package ir

import (
	"strings"
	"testing"
)

func g1Base() *Model {
	return &Model{Schema: Schema, Name: "g1",
		Globals:  []Var{{Name: "x", Type: Byte}, {Name: "a", Type: Byte, Len: 2}},
		Channels: []Channel{{Name: "c", Capacity: 2, Fields: []Type{Byte, Short}}},
		Processes: []Process{
			{Name: "A", Locations: []Location{{}, {}}, Edges: []Edge{{From: 0, To: 1}}},
			{Name: "B", Params: 1, Dynamic: true, Locals: []Var{{Name: "p", Type: Byte}}, Locations: []Location{{}, {}}, Edges: []Edge{{From: 0, To: 1}}},
		},
	}
}

func TestValidateG1Constructs(t *testing.T) {
	cases := []struct {
		name string
		mut  func(m *Model)
		want string // "" = valid
	}{
		{"send ok", func(m *Model) {
			m.Processes[0].Edges[0].Send = &ChanOp{Chan: "c", Args: []*Expr{Const(1), Const(2)}}
		}, ""},
		{"send arity", func(m *Model) {
			m.Processes[0].Edges[0].Send = &ChanOp{Chan: "c", Args: []*Expr{Const(1)}}
		}, "1 argument(s) for 2 field(s)"},
		{"send unknown chan", func(m *Model) {
			m.Processes[0].Edges[0].Send = &ChanOp{Chan: "q", Args: []*Expr{Const(1), Const(2)}}
		}, "undeclared channel"},
		{"recv bind and match", func(m *Model) {
			m.Processes[0].Edges[0].Recv = &RecvOp{Chan: "c", Args: []RecvArg{{Match: Const(1)}, {Var: "x"}}}
		}, ""},
		{"recv bind undeclared", func(m *Model) {
			m.Processes[0].Edges[0].Recv = &RecvOp{Chan: "c", Args: []RecvArg{{Var: "nope"}, {}}}
		}, "undeclared variable"},
		{"recv array needs index", func(m *Model) {
			m.Processes[0].Edges[0].Recv = &RecvOp{Chan: "c", Args: []RecvArg{{Var: "a"}, {}}}
		}, "needs an index"},
		{"recv both", func(m *Model) {
			m.Processes[0].Edges[0].Recv = &RecvOp{Chan: "c", Args: []RecvArg{{Var: "x", Match: Const(1)}, {}}}
		}, "binds a variable and matches"},
		{"run ok", func(m *Model) {
			m.Processes[0].Edges[0].Run = &RunOp{Proc: 1, Entry: 0, Args: []*Expr{Const(3)}}
		}, ""},
		{"run of a static process", func(m *Model) {
			m.Processes[0].Edges[0].Run = &RunOp{Proc: 0, Entry: 0}
		}, "is not a dynamic instance"},
		{"run pool", func(m *Model) {
			m.Processes[0].Edges[0].Run = &RunOp{Proc: 1, Pool: []int{1}, Entry: 0, Args: []*Expr{Const(3)}}
		}, ""},
		{"run init in the target's scope", func(m *Model) {
			m.Processes[0].Edges[0].Run = &RunOp{Proc: 1, Entry: 0, Args: []*Expr{Const(3)},
				Init: []Assign{{Var: "p", Value: Binary("add", Ref("p"), Const(1))}}}
		}, ""},
		{"run init names an unknown variable", func(m *Model) {
			m.Processes[0].Edges[0].Run = &RunOp{Proc: 1, Entry: 0, Args: []*Expr{Const(3)},
				Init: []Assign{{Var: "zz", Value: Const(1)}}}
		}, "has no variable"},
		{"run args", func(m *Model) {
			m.Processes[0].Edges[0].Run = &RunOp{Proc: 1, Entry: 0}
		}, "0 argument(s) for 1 parameter(s)"},
		{"run entry", func(m *Model) {
			m.Processes[0].Edges[0].Run = &RunOp{Proc: 1, Entry: 7, Args: []*Expr{Const(3)}}
		}, "outside 0..1"},
		{"two ops", func(m *Model) {
			m.Processes[0].Edges[0].Send = &ChanOp{Chan: "c", Args: []*Expr{Const(1), Const(2)}}
			m.Processes[0].Edges[0].Run = &RunOp{Proc: 1, Entry: 0, Args: []*Expr{Const(3)}}
		}, "at most one of send, recv, run"},
		{"else with guard", func(m *Model) {
			m.Processes[0].Edges[0].Else = true
			m.Processes[0].Edges[0].Guard = Const(1)
		}, "else edge has no guard"},
		{"len ok", func(m *Model) { m.Processes[0].Edges[0].Guard = Binary("lt", Len("c"), Const(2)) }, ""},
		{"len unknown", func(m *Model) { m.Processes[0].Edges[0].Guard = Binary("lt", Len("zz"), Const(2)) }, "undeclared channel"},
		{"pc ok", func(m *Model) { m.Processes[0].Edges[0].Guard = Binary("eq", PC(1), Const(1)) }, ""},
		{"pc range", func(m *Model) { m.Processes[0].Edges[0].Guard = Binary("eq", PC(5), Const(1)) }, "no such process"},
		{"timeout ok", func(m *Model) { m.Processes[0].Edges[0].Guard = Timeout() }, ""},
		{"params range", func(m *Model) { m.Processes[1].Params = 3 }, "params"},
	}
	for _, c := range cases {
		m := g1Base()
		c.mut(m)
		err := Validate(m)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: unexpected %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

func TestChannelBufferHelpers(t *testing.T) {
	m := g1Base()
	l, err := NewLayout(m)
	if err != nil {
		t.Fatal(err)
	}
	s := l.Initial()
	ci, ok := l.ChanIndex("c")
	if !ok || l.ChanLen(s, ci) != 0 {
		t.Fatal("channel c not found or not empty")
	}
	l.ChanPush(s, ci, []int64{7, -300})
	l.ChanPush(s, ci, []int64{9, 5})
	if l.ChanLen(s, ci) != 2 || l.ChanField(s, ci, 0, 1) != -300 || l.ChanField(s, ci, 1, 0) != 9 {
		t.Fatalf("buffer %v", l.ChanMessages(s, ci))
	}
	l.ChanPop(s, ci)
	if l.ChanLen(s, ci) != 1 || l.ChanField(s, ci, 0, 0) != 9 || l.ChanField(s, ci, 0, 1) != 5 {
		t.Fatalf("after pop %v", l.ChanMessages(s, ci))
	}
	// the freed slot is zeroed so equal contents give equal vectors
	l.ChanPop(s, ci)
	if string(s) != string(l.Initial()) {
		t.Fatalf("emptied channel differs from the initial vector")
	}
	// compiled len / pc / timeout
	c, err := l.Compile(Binary("add", Len("c"), Binary("add", PC(1), Timeout())), 0)
	if err != nil {
		t.Fatal(err)
	}
	l.WritePC(s, 1, 1)
	l.Timeout = true
	if v, _ := c.Eval(s); v != 2 {
		t.Fatalf("len(c) + pc(1) + timeout = %d, want 0 + 1 + 1", v)
	}
	if c.String() != "len(c) + (pc(1) + timeout)" {
		t.Fatalf("text %q", c.String())
	}
}

func TestExprUses(t *testing.T) {
	e := Binary("and", Ref("x"), Binary("or", Timeout(), Const(1)))
	if !e.Uses("timeout") || e.Uses("pc") || (*Expr)(nil).Uses("timeout") {
		t.Fatal("Uses")
	}
}
