package ir

import "testing"

// tableBase is two processes that read nothing the live-process table
// answers: neither creates a process, neither reads _nr_pr, none leaves.
func tableBase() *Model {
	edge := func() []Edge { return []Edge{{From: 0, To: 1, Effect: []Assign{{Var: "x", Value: Const(1)}}}} }
	return &Model{Schema: Schema, Name: "table",
		Globals: []Var{{Name: "x", Type: Byte}},
		Processes: []Process{
			{Name: "A", Locations: []Location{{}, {}}, Edges: edge()},
			{Name: "B", Locations: []Location{{}, {}}, Edges: edge()},
		},
	}
}

// TestPropertyCannotGiveTheModelATable: whether the vector carries the
// live-process table is decided by the model's processes alone. A property
// that reads _nr_pr, a pid or the youngest test is read after the processes
// were written, so nothing in them removes a process from a table made for
// the property: the count would stay at the number of processes started. The
// explorer refuses such a property (explore/tableread.go); the layout must
// not build the table for it, or the refused property would still enlarge the
// state vector of every property beside it.
func TestPropertyCannotGiveTheModelATable(t *testing.T) {
	for _, e := range []*Expr{NrPr(), PID(1), Youngest(1), Binary("ge", NrPr(), Const(0))} {
		m := tableBase()
		m.Properties = []Property{{ID: "p", Kind: KindInvariant, Expr: e}}
		if NeedsTable(m) {
			t.Errorf("a property reading %s makes NeedsTable true", e)
		}
		l, err := NewLayout(m)
		if err != nil {
			t.Fatal(err)
		}
		if l.HasTable() {
			t.Errorf("a property reading %s gives the layout a table", e)
		}
		plain, err := NewLayout(tableBase())
		if err != nil {
			t.Fatal(err)
		}
		if l.Size != plain.Size {
			t.Errorf("a property reading %s changes the state vector from %d to %d bytes", e, plain.Size, l.Size)
		}
	}
}

// TestProcessesGiveTheModelATable: the three things in a process that need the
// table still make it, each of them.
func TestProcessesGiveTheModelATable(t *testing.T) {
	cases := []struct {
		name string
		mut  func(m *Model)
	}{
		{"created by run", func(m *Model) { m.Processes[1].Dynamic = true }},
		{"read in a guard", func(m *Model) { m.Processes[0].Edges[0].Guard = Binary("ge", NrPr(), Const(0)) }},
		{"read in an assert", func(m *Model) { m.Processes[0].Edges[0].Assert = Binary("ge", NrPr(), Const(0)) }},
		{"read in an assignment", func(m *Model) { m.Processes[0].Edges[0].Effect[0].Value = NrPr() }},
		{"read in provided", func(m *Model) { m.Processes[0].Provided = Binary("ge", NrPr(), Const(0)) }},
		{"a pid", func(m *Model) { m.Processes[0].Edges[0].Guard = Binary("ge", PID(0), Const(0)) }},
		{"the youngest test", func(m *Model) { m.Processes[0].Edges[0].Guard = Youngest(0) }},
		// An edge that leaves the table presupposes it, whatever else the
		// model reads: a hand-written IR can put the leave on its end edges
		// and read the count only from a property.
		{"an edge that leaves the table", func(m *Model) { m.Processes[0].Edges[0].Leave = true }},
	}
	for _, c := range cases {
		m := tableBase()
		c.mut(m)
		if !NeedsTable(m) {
			t.Errorf("%s: NeedsTable is false", c.name)
			continue
		}
		l, err := NewLayout(m)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !l.HasTable() {
			t.Errorf("%s: the layout has no table", c.name)
		}
	}
	if NeedsTable(tableBase()) {
		t.Error("a model that reads nothing of the table needs one")
	}
}

// TestTableRead: the first read of the table in an expression, and none in
// one that does not read it.
func TestTableRead(t *testing.T) {
	if (*Expr)(nil).TableRead() != nil {
		t.Error("a nil expression reads the table")
	}
	if e := Binary("and", Ref("x"), Binary("or", Timeout(), Const(1))); e.TableRead() != nil {
		t.Errorf("%s reads the table", e)
	}
	for _, e := range []*Expr{NrPr(), PID(2), Youngest(3)} {
		wrapped := Binary("and", Ref("x"), Binary("ge", e, Const(0)))
		if wrapped.TableRead() != e {
			t.Errorf("%s: TableRead did not find %s", wrapped, e)
		}
	}
	// An index and a channel selector hold sub-expressions too.
	if CLen(NrPr()).TableRead() == nil || (&Expr{Op: "index", Var: "a", Args: []*Expr{NrPr()}}).TableRead() == nil {
		t.Error("a read of the table inside clen or an index was missed")
	}
}

// TestReadsState: an expression is constant only when nothing in it reads the
// state. A variable, an array element, a channel, a program counter, the
// timeout and the live-process table are all state; the lint of a property
// calls an expression that reads none of them a vacuity candidate, so a read
// that is missed here is a false "constant".
func TestReadsState(t *testing.T) {
	if (*Expr)(nil).ReadsState() {
		t.Error("a nil expression reads the state")
	}
	for _, e := range []*Expr{
		Const(1),
		Unary("neg", Const(1)),
		Unary("not", Const(0)),
		Binary("and", Const(1), Binary("lt", Binary("add", Const(1), Const(2)), Const(4))),
	} {
		if e.ReadsState() {
			t.Errorf("%s reads the state", e)
		}
	}
	reads := []*Expr{
		Ref("x"), Index("a", Const(0)), Len("c"), Timeout(), PC(0),
		CLen(Const(0)), CFull(Const(0)), NrPr(), PID(1), Youngest(1),
	}
	for _, e := range reads {
		if !e.ReadsState() {
			t.Errorf("%s does not read the state", e)
		}
		if w := Binary("eq", Const(2), e); !w.ReadsState() {
			t.Errorf("%s does not read the state", w)
		}
	}
	// Every op the language has is either one of the pure ones above or a read:
	// an op added later must be classified here, not fall silently into one.
	pure := map[string]bool{"const": true, "neg": true, "not": true, "add": true, "sub": true, "mul": true,
		"div": true, "mod": true, "eq": true, "ne": true, "lt": true, "le": true, "gt": true, "ge": true,
		"and": true, "or": true}
	for op, n := range arity {
		args := make([]*Expr, n)
		for i := range args {
			args[i] = Const(0)
		}
		if got := (&Expr{Op: op, Args: args}).ReadsState(); got == pure[op] {
			t.Errorf("op %q: ReadsState = %v, but it is %s", op, got, map[bool]string{true: "pure", false: "a state read"}[pure[op]])
		}
	}
}
