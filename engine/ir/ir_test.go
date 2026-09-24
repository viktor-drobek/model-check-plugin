package ir

import (
	"bytes"
	"strings"
	"testing"
)

func i64(v int64) *int64 { return &v }

// twoProcs is a small model with a global array, a short, process locals and
// a channel, exercising every layout rule.
func twoProcs() *Model {
	return &Model{
		Schema: Schema,
		Name:   "two",
		Globals: []Var{
			{Name: "a", Type: Byte, Len: 3, Init: []int64{1, 2, 3}},
			{Name: "s", Type: Short, Init: []int64{-5}},
			{Name: "cap", Type: Byte, Max: i64(2)},
		},
		Channels: []Channel{{Name: "q", Capacity: 2, Fields: []Type{Byte, Short}}},
		Processes: []Process{
			{
				Name:      "P",
				Locals:    []Var{{Name: "x", Type: Int, Init: []int64{-100000}}},
				Locations: []Location{{Name: "L0"}, {Name: "L1", Labels: []Label{End}}},
				Initial:   0,
				Edges: []Edge{
					{From: 0, To: 1, Guard: Binary("gt", Index("a", Const(1)), Const(1)),
						Effect: []Assign{{Var: "x", Value: Binary("add", Ref("x"), Const(1))}}, Text: "step"},
				},
			},
			{
				Name:      "Q",
				Locations: []Location{{Name: "M0"}},
				Initial:   0,
				Edges:     []Edge{{From: 0, To: 0, Effect: []Assign{{Var: "cap", Value: Binary("add", Ref("cap"), Const(1))}}}},
			},
		},
		Properties: []Property{
			{ID: "deadlock", Kind: KindDeadlock},
			{ID: "inv", Kind: KindInvariant, Expr: Binary("le", Ref("cap"), Const(2))},
		},
	}
}

func TestLayoutOrderAndSize(t *testing.T) {
	l, err := NewLayout(twoProcs())
	if err != nil {
		t.Fatal(err)
	}
	// excl(1) + P: pc(1) + x(4) + Q: pc(1) + a(3) + s(2) + cap(1) + q: 1 + 2*(1+2)
	want := 1 + 1 + 4 + 1 + 3 + 2 + 1 + 1 + 6
	if l.Size != want {
		t.Fatalf("size %d, want %d", l.Size, want)
	}
	if l.Excl != 0 || l.PC[0] != 1 || l.PC[1] != 6 {
		t.Fatalf("offsets excl=%d pc=%v", l.Excl, l.PC)
	}
	s := l.Initial()
	if l.ReadPC(s, 0) != 0 || l.ReadPC(s, 1) != 0 {
		t.Fatal("initial pcs")
	}
	if got := l.Resolve("x", 0).Read(s); got != -100000 {
		t.Fatalf("x = %d", got)
	}
	if got := l.Resolve("s", -1).Read(s); got != -5 {
		t.Fatalf("s = %d", got)
	}
	if got := l.Resolve("a", 1).At(2).Read(s); got != 3 {
		t.Fatalf("a[2] = %d", got)
	}
	if l.Resolve("x", 1) != nil {
		t.Fatal("local x of P visible from Q")
	}
	c := l.Resolve("cap", -1)
	if c.Min != 0 || c.Max != 2 || c.InDomain(3) {
		t.Fatalf("cap domain [%d,%d]", c.Min, c.Max)
	}
}

func TestEvalArithmeticAndBool(t *testing.T) {
	l, err := NewLayout(twoProcs())
	if err != nil {
		t.Fatal(err)
	}
	s := l.Initial()
	cases := []struct {
		e    *Expr
		want int64
	}{
		{Binary("add", Index("a", Const(0)), Ref("s")), -4},
		{Binary("mul", Ref("s"), Const(-2)), 10},
		{Binary("div", Const(-7), Const(2)), -3},
		{Binary("mod", Const(-7), Const(2)), -1},
		{Binary("and", Binary("gt", Ref("s"), Const(-10)), Binary("lt", Ref("s"), Const(0))), 1},
		{Binary("or", Const(0), Binary("eq", Index("a", Const(2)), Const(3))), 1},
		{Unary("not", Const(5)), 0},
		{Unary("neg", Ref("s")), 5},
		{Binary("ne", Ref("cap"), Const(0)), 0},
	}
	for _, c := range cases {
		ce, err := l.Compile(c.e, -1)
		if err != nil {
			t.Fatalf("%s: %v", c.e, err)
		}
		got, err := ce.Eval(s)
		if err != nil || got != c.want {
			t.Errorf("%s = %d (%v), want %d", c.e, got, err, c.want)
		}
	}
}

func TestEvalErrorsAreInvalidModel(t *testing.T) {
	l, _ := NewLayout(twoProcs())
	s := l.Initial()
	for _, e := range []*Expr{
		Binary("div", Const(1), Ref("cap")),
		Binary("mod", Const(1), Ref("cap")),
		Index("a", Const(3)),
		Index("a", Const(-1)),
	} {
		ce, err := l.Compile(e, -1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ce.Eval(s); err == nil {
			t.Errorf("%s: expected an evaluation error", e)
		} else if _, ok := err.(*EvalError); !ok {
			t.Errorf("%s: %T is not *EvalError", e, err)
		}
	}
}

func TestCheckRejectsIllTyped(t *testing.T) {
	l, _ := NewLayout(twoProcs())
	for _, e := range []*Expr{
		Ref("a"),                             // array without index
		Index("s", Const(0)),                 // scalar indexed
		Ref("nope"),                          // undeclared
		{Op: "add", Args: []*Expr{Const(1)}}, // wrong arity
		{Op: "xor", Args: []*Expr{Const(1), Const(2)}},
	} {
		if _, err := l.Compile(e, -1); err == nil {
			t.Errorf("%v: expected a type error", e)
		}
	}
}

func TestValidateFindsProblems(t *testing.T) {
	cases := []struct {
		mut  func(m *Model)
		want string
	}{
		{func(m *Model) { m.Schema = "x" }, "schema"},
		{func(m *Model) { m.Globals[0].Init = []int64{1} }, "initial values"},
		{func(m *Model) { m.Globals[2].Max = i64(300) }, "outside the byte domain"},
		{func(m *Model) { m.Processes[0].Initial = 5 }, "initial"},
		{func(m *Model) { m.Processes[0].Edges[0].To = 9 }, "from/to"},
		{func(m *Model) { m.Processes[0].Edges[0].Effect[0].Var = "a" }, "needs an index"},
		{func(m *Model) { m.Properties[1].Expr = nil }, "needs expr"},
		{func(m *Model) { m.Properties[1].ID = "deadlock" }, "duplicate id"},
		{func(m *Model) { m.Processes[1].Name = "P" }, "duplicate name"},
		{func(m *Model) { m.Processes[0].Locations[1].Labels = []Label{"finish"} }, "unknown label"},
		{func(m *Model) { m.Processes = nil }, "at least one process"},
	}
	for _, c := range cases {
		m := twoProcs()
		c.mut(m)
		err := Validate(m)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("want error containing %q, got %v", c.want, err)
		}
	}
	if err := Validate(twoProcs()); err != nil {
		t.Fatal(err)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	m := twoProcs()
	b1, err := MarshalJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := UnmarshalJSON(b1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := MarshalJSON(m2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("round trip differs:\n%s\n---\n%s", b1, b2)
	}
	l1, _ := NewLayout(m)
	l2, _ := NewLayout(m2)
	if !bytes.Equal(l1.Initial(), l2.Initial()) || l1.Size != l2.Size {
		t.Fatal("layouts differ after round trip")
	}
}

func TestUnmarshalRejectsUnknownField(t *testing.T) {
	_, err := UnmarshalJSON([]byte(`{"schema":"mcd-ir/1","name":"m","processes":[],"gaurd":1}`))
	if err == nil || !strings.Contains(err.Error(), "gaurd") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestExprString(t *testing.T) {
	e := Binary("and", Binary("gt", Ref("p1"), Const(0)), Binary("gt", Index("a", Const(1)), Const(0)))
	if got := e.String(); got != "(p1 > 0) && (a[1] > 0)" {
		t.Fatalf("got %q", got)
	}
}
