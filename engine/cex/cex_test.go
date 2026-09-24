package cex

import (
	"testing"

	"modelcheck/ir"
)

func model() *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "m",
		Globals: []ir.Var{{Name: "g", Type: ir.Byte, Init: []int64{1}}, {Name: "arr", Type: ir.Byte, Len: 2}},
		Processes: []ir.Process{{
			Name:      "P",
			Locals:    []ir.Var{{Name: "x", Type: ir.Short}},
			Locations: []ir.Location{{Name: "L0"}, {Name: "L1"}},
			Edges: []ir.Edge{
				{From: 0, To: 1, Guard: ir.Binary("gt", ir.Ref("g"), ir.Const(0)),
					Effect: []ir.Assign{{Var: "g", Value: ir.Binary("sub", ir.Ref("g"), ir.Const(1))},
						{Var: "x", Value: ir.Const(-3)}, {Var: "arr", Index: ir.Const(1), Value: ir.Const(9)}},
					Origin: &ir.Origin{File: "f.pml", Line: 4, Name: "take"}},
				{From: 1, To: 1, Text: "loop"},
			},
		}}}
}

func TestBuildDiffsAndNames(t *testing.T) {
	l, err := ir.NewLayout(model())
	if err != nil {
		t.Fatal(err)
	}
	s0 := l.Initial()
	s1 := append([]byte(nil), s0...)
	l.WritePC(s1, 0, 1)
	l.Resolve("g", -1).Write(s1, 0)
	l.Resolve("x", 0).Write(s1, -3)
	l.Resolve("arr", -1).At(1).Write(s1, 9)
	s2 := append([]byte(nil), s1...)

	tr := Build(l, [][]byte{s0, s1, s2}, []Ref{{0, 0}, {0, 1}})
	if len(tr.Steps) != 2 || tr.Summary != "g > 0 -> g = g - 1 x = -3 arr[1] = 9, loop" {
		t.Fatalf("summary %q", tr.Summary)
	}
	st := tr.Steps[0]
	if st.Index != 1 || st.Process != "P" || st.Location != "L1" || st.Origin == nil || st.Origin.Line != 4 {
		t.Fatalf("step %+v", st)
	}
	want := []Change{{"P.x", 0, -3}, {"g", 1, 0}, {"arr[1]", 0, 9}} // layout order: locals, then globals
	if len(st.Changes) != len(want) {
		t.Fatalf("changes %+v", st.Changes)
	}
	for i, c := range want {
		if st.Changes[i] != c {
			t.Fatalf("change %d = %+v, want %+v", i, st.Changes[i], c)
		}
	}
	if len(tr.Steps[1].Changes) != 0 {
		t.Fatal("loop changes nothing")
	}
	if got := tr.UserNames(); got[0] != "take" || got[1] != "loop" {
		t.Fatalf("user names %v", got)
	}
	if tr.NonZero() != "P.x=-3 arr[1]=9" {
		t.Fatalf("nonzero %q", tr.NonZero())
	}
	if len(tr.Final) != 4 { // x, g, arr[0], arr[1]
		t.Fatalf("final %v", tr.Final)
	}
}

func TestCommandTextFallbacks(t *testing.T) {
	if CommandText(&ir.Edge{}) != "skip" {
		t.Fatal("empty edge")
	}
	if got := CommandText(&ir.Edge{Assert: ir.Binary("eq", ir.Ref("c"), ir.Const(1))}); got != "assert(c == 1)" {
		t.Fatal(got)
	}
	if got := CommandText(&ir.Edge{Text: "t1", Guard: ir.Const(1)}); got != "t1" {
		t.Fatal(got)
	}
}
