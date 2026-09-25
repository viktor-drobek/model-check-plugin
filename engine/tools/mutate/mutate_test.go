package mutate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// first returns the first mutant of one operator, or fails.
func first(t *testing.T, src, op string) Mutant {
	t.Helper()
	o, err := ParseOperator(op)
	if err != nil {
		t.Fatal(err)
	}
	ms := Generate([]byte(src), "t.pml", o)
	if len(ms) == 0 {
		t.Fatalf("%s: no mutant of\n%s", op, src)
	}
	return ms[0]
}

func TestDropAtomic(t *testing.T) {
	src := "init { atomic { x = 1; y = 2 } }\n"
	m := first(t, src, "drop-atomic")
	if got := string(m.Src); got != "init { { x = 1; y = 2 } }\n" {
		t.Fatalf("got %q", got)
	}
	if m.Original != "atomic" || m.Mutated != "" {
		t.Fatalf("manifest %+v", m.Entry)
	}
	if m.Line != 1 || m.Col != 8 {
		t.Fatalf("position %d:%d, want 1:8", m.Line, m.Col)
	}
	// `atomic` not followed by a block is an expression, not a wrapper.
	if ms := Generate([]byte("init { x = atomic }\n"), "t.pml", DropAtomic); len(ms) != 0 {
		t.Fatalf("mutated a non-wrapper atomic: %+v", ms)
	}
}

func TestDropDStep(t *testing.T) {
	src := "active proctype A() { do :: d_step { !x -> x=1 } od }\n"
	m := first(t, src, "drop-dstep")
	if !strings.Contains(string(m.Src), ":: { !x -> x=1 }") {
		t.Fatalf("got %q", string(m.Src))
	}
}

func TestChanCap(t *testing.T) {
	src := "chan c = [2] of { mtype, bit };\nchan r = [0] of { bit };\nbyte a[2];\n"
	minus := Generate([]byte(src), "t.pml", ChanCapMinus)
	if len(minus) != 1 {
		t.Fatalf("chan-cap-minus: %d mutants, want 1 (the [0] channel and the array must be skipped)", len(minus))
	}
	if minus[0].Original != "[2]" || minus[0].Mutated != "[1]" {
		t.Fatalf("manifest %+v", minus[0].Entry)
	}
	if !strings.Contains(string(minus[0].Src), "[1] of { mtype, bit }") {
		t.Fatalf("got %q", string(minus[0].Src))
	}
	zero := Generate([]byte(src), "t.pml", ChanCapZero)
	if len(zero) != 1 || zero[0].Mutated != "[0]" {
		t.Fatalf("chan-cap-zero: %+v", zero)
	}
	// Capacity 1: minus gives 0, and chan-cap-zero would repeat it, so it
	// must not fire.
	one := Generate([]byte("chan c = [1] of { bit };\n"), "t.pml", ChanCapZero)
	if len(one) != 0 {
		t.Fatalf("chan-cap-zero duplicated chan-cap-minus on capacity 1: %+v", one)
	}
}

func TestInvertGuard(t *testing.T) {
	src := "active proctype A() {\n\tif\n\t:: (y != 0 && y != me) -> goto L1\n\t:: else -> skip\n\tfi\n}\n"
	ms := Generate([]byte(src), "t.pml", InvertGuard)
	if len(ms) != 1 {
		t.Fatalf("%d mutants, want 1 (else must be skipped): %+v", len(ms), ms)
	}
	if ms[0].Original != "(y != 0 && y != me)" || ms[0].Mutated != "!((y != 0 && y != me))" {
		t.Fatalf("manifest %+v", ms[0].Entry)
	}
	if !strings.Contains(string(ms[0].Src), ":: !((y != 0 && y != me)) -> goto L1") {
		t.Fatalf("got %q", string(ms[0].Src))
	}
}

func TestInvertGuardSkipsNonExpressions(t *testing.T) {
	for _, src := range []string{
		"active proctype A() { do :: c!msg -> skip od }",    // send
		"active proctype A() { do :: c?msg -> skip od }",    // receive
		"active proctype A() { do :: x = 1 od }",            // assignment
		"active proctype A() { do :: x++ od }",              // increment
		"active proctype A() { do :: else -> skip od }",     // else
		"active proctype A() { do :: goto L od }",           // goto
		"active proctype A() { do :: break od }",            // break
		"active proctype A() { do :: run B() od }",          // run
		"active proctype A() { do :: atomic { x = 1 } od }", // atomic block
		"active proctype A() { do :: { x = 1 } od }",        // block
		"active proctype A() { do :: L: x = 1 od }",         // label
		"active proctype A() { do :: assert(x) od }",        // assert
		"active proctype A() { do :: printf(\"a\") od }",    // printf
	} {
		if ms := Generate([]byte(src), "t.pml", InvertGuard); len(ms) != 0 {
			t.Errorf("inverted a non-guard in %q: %+v", src, ms[0].Entry)
		}
	}
	// A negation at the head of the expression is not a send.
	if ms := Generate([]byte("active proctype A() { do :: !x -> skip od }"), "t.pml", InvertGuard); len(ms) != 1 {
		t.Errorf("did not invert `!x`: %+v", ms)
	}
}

func TestDropEndLabel(t *testing.T) {
	src := "active proctype A() {\nend:\tdo :: x = 1 od\n}\n"
	m := first(t, src, "drop-end-label")
	if m.Original != "end:" || m.Mutated != "" {
		t.Fatalf("manifest %+v", m.Entry)
	}
	if !strings.Contains(string(m.Src), "\ndo :: x = 1 od\n") {
		t.Fatalf("got %q", string(m.Src))
	}
	// endless is also an end label (SPIN reads the prefix), but `sender` is
	// not, and an expression `a : b` is not a label.
	if ms := Generate([]byte("active proctype A() {\nendloop: skip\n}\n"), "t.pml", DropEndLabel); len(ms) != 1 {
		t.Fatalf("endloop: %+v", ms)
	}
	if ms := Generate([]byte("active proctype A() {\nsend: skip\n}\n"), "t.pml", DropEndLabel); len(ms) != 0 {
		t.Fatalf("mutated a label that is not an end label: %+v", ms)
	}
}

func TestWeakenAssert(t *testing.T) {
	src := "active proctype A() { cnt++; assert(cnt == 1); cnt-- }\n"
	m := first(t, src, "weaken-assert")
	if m.Original != "assert(cnt == 1)" || m.Mutated != "assert(true)" {
		t.Fatalf("manifest %+v", m.Entry)
	}
	if !strings.Contains(string(m.Src), "assert(true); cnt--") {
		t.Fatalf("got %q", string(m.Src))
	}
	// Nested parentheses must not stop the scan early.
	m2 := first(t, "init { assert((a && b) || c) }\n", "weaken-assert")
	if m2.Original != "assert((a && b) || c)" {
		t.Fatalf("nested parens: %+v", m2.Entry)
	}
}

func TestSwapRelop(t *testing.T) {
	src := "init { if :: (x > y) -> skip :: (x <= y) -> skip fi }\n"
	ms := Generate([]byte(src), "t.pml", SwapRelop)
	if len(ms) != 2 {
		t.Fatalf("%d mutants, want 2: %+v", len(ms), ms)
	}
	if ms[0].Original != ">" || ms[0].Mutated != ">=" {
		t.Fatalf("first %+v", ms[0].Entry)
	}
	if ms[1].Original != "<=" || ms[1].Mutated != "<" {
		t.Fatalf("second %+v", ms[1].Entry)
	}
	// `->`, `<<` and `>>` are their own tokens and are not comparisons.
	if ms := Generate([]byte("init { x = y << 1; z = y >> 1 }\n"), "t.pml", SwapRelop); len(ms) != 0 {
		t.Fatalf("mutated a shift: %+v", ms)
	}
}

func TestOffByOne(t *testing.T) {
	src := "#define N 5\nbyte a[3];\nchan c = [2] of { bit };\nactive [2] proctype A() { x = 7; a[1] = 9 }\n"
	ms := Generate([]byte(src), "t.pml", OffByOne)
	var got []string
	for _, m := range ms {
		got = append(got, m.Original+"->"+m.Mutated)
	}
	want := []string{"7->8", "9->10"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v (the directive, the array size, the capacity, the process count and the index must be skipped)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestDropAlternative(t *testing.T) {
	src := "init {\n\tif\n\t:: x = 1\n\t:: x = 2\n\tfi\n}\n"
	ms := Generate([]byte(src), "t.pml", DropAlternative)
	if len(ms) != 2 {
		t.Fatalf("%d mutants, want 2: %+v", len(ms), ms)
	}
	if ms[0].Original != ":: x = 1" || ms[0].Mutated != "" {
		t.Fatalf("first %+v", ms[0].Entry)
	}
	if strings.Contains(string(ms[0].Src), "x = 1") {
		t.Fatalf("option not removed: %q", string(ms[0].Src))
	}
	if !strings.Contains(string(ms[1].Src), "x = 1") || strings.Contains(string(ms[1].Src), "x = 2") {
		t.Fatalf("second removed the wrong option: %q", string(ms[1].Src))
	}
	// A single-option if keeps its option: removing it would leave `if fi`.
	if ms := Generate([]byte("init { if :: x = 1 fi }\n"), "t.pml", DropAlternative); len(ms) != 0 {
		t.Fatalf("emptied a one-option if: %+v", ms)
	}
	// A `::` of a nested if belongs to the nested if.
	nested := "init {\n\tdo\n\t:: if :: a = 1 :: b = 1 fi\n\t:: c = 1\n\tod\n}\n"
	ms = Generate([]byte(nested), "t.pml", DropAlternative)
	if len(ms) != 4 {
		t.Fatalf("nested: %d mutants, want 4 (2 inner + 2 outer)", len(ms))
	}
}

func TestCommentsAndStringsAreNotMutated(t *testing.T) {
	src := "init { printf(\"x > 1 and 5\"); /* x > 1 and 5 */ skip }\n"
	for _, op := range []Operator{SwapRelop, OffByOne} {
		if ms := Generate([]byte(src), "t.pml", op); len(ms) != 0 {
			t.Errorf("%s mutated inside a string or comment: %+v", op, ms[0].Entry)
		}
	}
}

func TestDirectivesAreNotMutated(t *testing.T) {
	src := "#define inp1(x)\t(x>0) -> x--\n#define N 5\ninit { y = 1 }\n"
	ms := Generate([]byte(src), "t.pml")
	for _, m := range ms {
		if m.Line < 3 {
			t.Errorf("mutated a directive: %+v", m.Entry)
		}
	}
}

func TestGenerateIsDeterministicAndRanked(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "mutate", "sample.pml"))
	if err != nil {
		t.Skip("no sample")
	}
	a := Generate(src, "sample.pml")
	b := Generate(src, "sample.pml")
	ja, _ := json.Marshal(entriesOf(a))
	jb, _ := json.Marshal(entriesOf(b))
	if string(ja) != string(jb) {
		t.Fatal("two runs gave different manifests")
	}
	rank := map[string]int{}
	for i, op := range Operators {
		rank[string(op)] = i
	}
	for i := 1; i < len(a); i++ {
		p, c := a[i-1], a[i]
		if c.ID != p.ID+1 {
			t.Fatalf("ids are not sequential at %d", i)
		}
		if rank[c.Operator] < rank[p.Operator] {
			t.Fatalf("operator %s came after %s", c.Operator, p.Operator)
		}
		if rank[c.Operator] == rank[p.Operator] && (c.Line < p.Line || (c.Line == p.Line && c.Col < p.Col)) {
			t.Fatalf("positions out of order at %d: %d:%d after %d:%d", i, c.Line, c.Col, p.Line, p.Col)
		}
	}
}

func entriesOf(ms []Mutant) []Entry {
	out := make([]Entry, 0, len(ms))
	for _, m := range ms {
		e := m.Entry
		e.File = m.FileName()
		out = append(out, e)
	}
	return out
}

func TestWriteAll(t *testing.T) {
	dir := t.TempDir()
	src := "init { if :: x = 1 :: x = 2 fi; assert(x == 1) }\n"
	ms := Generate([]byte(src), "t.pml")
	path, err := WriteAll(dir, ms)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(ms) {
		t.Fatalf("manifest has %d entries, %d mutants", len(entries), len(ms))
	}
	for _, e := range entries {
		if e.File == "" || e.Line == 0 || e.Col == 0 || e.Original == "" {
			t.Fatalf("incomplete entry %+v", e)
		}
		if _, err := os.Stat(filepath.Join(dir, e.File)); err != nil {
			t.Fatalf("mutant %s: %v", e.File, err)
		}
	}
}
