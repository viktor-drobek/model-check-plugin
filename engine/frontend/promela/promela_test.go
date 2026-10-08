package promela

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"modelcheck/explore"
	"modelcheck/ir"
)

const corpus = "../../../../Promela - examples"

func lexAll(t *testing.T, src string) []Token {
	t.Helper()
	toks, err := Lex(src, "t.pml")
	if err != nil {
		t.Fatalf("lex: %v", err)
	}
	return toks
}

func texts(toks []Token) string {
	var parts []string
	for _, t := range toks {
		if t.Kind == EOF {
			break
		}
		parts = append(parts, t.Text)
	}
	return strings.Join(parts, " ")
}

func TestPreprocessMacros(t *testing.T) {
	src := "#define N 3\n#define inc(x, y) x = x + y\n#define K (N + 1)\nbyte a[N];\ninc(a[0],\n K)\n"
	toks, err := Preprocess(lexAll(t, src), nil, "t.pml")
	if err != nil {
		t.Fatal(err)
	}
	got := texts(toks)
	want := "byte a [ 3 ] ; a [ 0 ] = a [ 0 ] + ( 3 + 1 )"
	if got != want {
		t.Fatalf("expansion %q, want %q", got, want)
	}
	// expanded tokens carry the line of the invocation
	for _, tk := range toks {
		if tk.Kind == EOF {
			break
		}
		if tk.Text == "+" && tk.Line != 5 {
			t.Fatalf("token + at line %d, want 5 (invocation line)", tk.Line)
		}
	}
}

func TestPreprocessContinuationAndRecursionGuard(t *testing.T) {
	src := "#define example(x, y) \\\n\ty = a;\t\\\n\tx = b\n#define a a\nexample(p, q)\n"
	toks, err := Preprocess(lexAll(t, src), nil, "t.pml")
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(toks); got != "q = a ; p = b" {
		t.Fatalf("got %q", got)
	}
}

func TestPreprocessConditionals(t *testing.T) {
	src := "#ifdef PHI\nA\n#else\nB\n#endif\n#if 0\nC\n#endif\n#if 1\nD\n#endif\n#ifndef PHI\nE\n#elif defined(X) && X == 2\nF\n#else\nG\n#endif\n"
	cases := []struct {
		defs []string
		want string
	}{
		{nil, "B D E"},
		{[]string{"PHI"}, "A D G"},
		{[]string{"PHI", "X=2"}, "A D F"},
		{[]string{"PHI", "X"}, "A D G"},
	}
	for _, c := range cases {
		toks, err := Preprocess(lexAll(t, src), c.defs, "t.pml")
		if err != nil {
			t.Fatalf("%v: %v", c.defs, err)
		}
		if got := texts(toks); got != c.want {
			t.Errorf("-D %v: %q, want %q", c.defs, got, c.want)
		}
	}
}

func TestPreprocessErrors(t *testing.T) {
	cases := []struct {
		src  string
		kind string
		line int
		msg  string
	}{
		{"byte x;\n#endif\n", KindSyntax, 2, "#endif without #if"},
		{"#ifdef A\nbyte x;\n", KindSyntax, 1, "#if without #endif"},
		{"#include \"x.h\"\n", KindOutside, 1, "#include"},
		{"#define f(x) x\nbyte y;\nf(1, 2)\n", KindSyntax, 3, "takes 1 argument"},
		{"#if 1 /\n#endif\n", KindSyntax, 1, "unexpected"},
	}
	for _, c := range cases {
		_, err := Preprocess(lexAll(t, c.src), nil, "t.pml")
		if err == nil {
			t.Errorf("%q: no error", c.src)
			continue
		}
		if err.Kind != c.kind || err.Line != c.line || !strings.Contains(err.Message, c.msg) {
			t.Errorf("%q: got %v, want %s line %d %q", c.src, err, c.kind, c.line, c.msg)
		}
	}
}

func parseSrc(t *testing.T, src string) (*ir.Model, []string, *Error) {
	t.Helper()
	res, err := Parse([]byte(src), "t.pml", nil)
	if err != nil {
		return nil, nil, err
	}
	return res.Model, res.Warnings, nil
}

func TestRejectionPositions(t *testing.T) {
	cases := []struct {
		name string
		src  string
		kind string
		line int
		col  int
		msg  string
	}{
		{"syntax", "byte x;\nactive proctype A()\n{\tx = = 1\n}\n", KindSyntax, 3, 7, "unexpected ="},
		{"c_code", "c_code { int x; }\n", KindOutside, 1, 1, "c_code"},
		{"unless", "active proctype A() { { skip } unless { skip } }\n", KindOutside, 1, 32, "unless"},
		{"inline call arity", "inline f(x) { x = 1 }\nbyte a;\ninit { f(a, a) }\n", KindSemantic, 3, 8, "1 parameter(s)"},
		{"struct array", "typedef T { byte a }\nT v[2];\ninit { skip }\n", KindOutside, 2, 3, "array of structs"},
		{"bitwise", "byte x;\nactive proctype A() { x = x & 1 }\n", KindOutside, 2, 29, "bitwise operator &"},
		{"run in expr", "proctype B() { skip }\ninit { !run B() }\n", KindOutside, 2, 9, "run inside an expression"},
		{"chan in a dynamic process", "proctype B() { chan c = [0] of { byte }; skip }\ninit { run B() }\n", KindOutside, 1, 21, "channel declared inside a process created by run"},
		{"struct message field", "typedef T { byte a }\nchan c = [0] of { T };\ninit { skip }\n", KindOutside, 2, 19, "user-defined message field type T"},
		{"eval", "chan c = [1] of { byte }; byte x;\nactive proctype A() { c?eval(x) }\n", KindOutside, 2, 25, "eval"},
		{"poll", "chan c = [1] of { byte };\nactive proctype A() { c?[1] }\n", KindOutside, 2, 23, "channel poll"},
		{"sorted send", "chan c = [1] of { byte };\nactive proctype A() { c!!1 }\n", KindOutside, 2, 23, "sorted send"},
		{"ternary", "byte x;\nactive proctype A() { x = (x -> 1 : 2) }\n", KindOutside, 2, 27, "conditional expression"},
		{"remote ref", "active proctype A() { L: skip; A@L }\n", KindOutside, 1, 32, "remote reference"},
		{"random receive", "chan c = [1] of { byte };\nactive proctype A() { c??1 }\n", KindOutside, 2, 23, "random receive"},
		{"pc_value of a computed process", "byte x;\nactive proctype A() { pc_value(x) > 0 }\n", KindOutside, 2, 23, "pc_value with a computed process number"},
		{"undeclared", "active proctype A() { y = 1 }\n", KindSemantic, 1, 23, "undeclared variable y"},
		{"undeclared in printf", "active proctype A() { printf(\"%d\", y) }\n", KindSemantic, 1, 36, "undeclared variable y"},
		{"block scope", "init { { int y; y++ } y = 1 }\n", KindSemantic, 1, 23, "undeclared variable y"},
		{"redeclared with another type", "init { { int y; y++ }; { byte y; y++ } }\n", KindSemantic, 1, 31, "different types"},
		{"send arity", "chan c = [1] of { byte, byte };\nactive proctype A() { c!1 }\n", KindSemantic, 2, 23, "1 value(s) for 2 field(s)"},
		{"break outside do", "active proctype A() { break }\n", KindSemantic, 1, 23, "break outside"},
		{"rendezvous in d_step", "chan c = [0] of { byte };\nactive proctype A() { d_step { c!1; skip } }\nactive proctype B() { c?_ }\n", KindOutside, 2, 32, "rendezvous operation inside d_step"},
		{"no process", "byte x;\n", KindSemantic, 0, 0, "no active process"},
		{"non-constant init", "byte x; byte y = x;\n", KindOutside, 1, 18, "non-constant initialiser"},
	}
	for _, c := range cases {
		_, _, err := parseSrc(t, c.src)
		if err == nil {
			t.Errorf("%s: accepted", c.name)
			continue
		}
		if err.Kind != c.kind || err.Line != c.line || (c.col > 0 && err.Col != c.col) || !strings.Contains(err.Message, c.msg) {
			t.Errorf("%s: got %s %d:%d %q, want %s %d:%d %q", c.name, err.Kind, err.Line, err.Col, err.Message, c.kind, c.line, c.col, c.msg)
		}
	}
}

func TestRenderStatementText(t *testing.T) {
	src := "#define p (x != 0)\nmtype = { msgtype };\nchan name = [0] of { mtype, byte };\nbyte x, cnt; byte count;\nactive proctype A() {\n\tassert ( cnt == 1 );\n\tname ! msgtype ( 124 );\n\tcount -- ;\n\t!p;\n\tx = - 1 + cnt * 2;\n\tprintf(\"a %d\\n\", x)\n}\nactive proctype B() { byte s; name?msgtype(s) }\n"
	m, _, err := parseSrc(t, src)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range m.Processes[0].Edges {
		got = append(got, e.Text)
	}
	want := []string{"assert(cnt == 1)", "name!msgtype(124)", "count--", "!(x != 0)", "x = -1 + cnt * 2", `printf("a %d\n", x)`, "-end-"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("texts %q, want %q", got, want)
	}
}

func TestMtypeNumbering(t *testing.T) {
	src := "mtype = { a, b };\nmtype { c, d };\nmtype v = c; mtype w = b;\nactive proctype A() { v = a }\n"
	m, _, err := parseSrc(t, src)
	if err != nil {
		t.Fatal(err)
	}
	if m.Globals[0].Init[0] != 4 || m.Globals[1].Init[0] != 1 {
		t.Fatalf("mtype values v=%v w=%v, want 4 and 1 (SPIN numbers each declaration backwards)", m.Globals[0].Init, m.Globals[1].Init)
	}
	if e := m.Processes[0].Edges[0]; e.Effect[0].Value.Value != 2 {
		t.Fatalf("a = %d, want 2", e.Effect[0].Value.Value)
	}
}

func TestAtomicDStepElseTimeoutFlags(t *testing.T) {
	src := "byte x, y;\nactive proctype A() {\n\tatomic { x = 1; y == 1; x = 2 };\n\td_step { x = 3; x = 4 };\n\tif :: x > 0 -> skip :: else -> skip fi;\n\ttimeout -> x = 0\n}\n"
	m, _, err := parseSrc(t, src)
	if err != nil {
		t.Fatal(err)
	}
	p := m.Processes[0]
	type flags struct{ a, d, e, to bool }
	var got []flags
	for _, e := range p.Edges {
		got = append(got, flags{e.Atomic, e.DStep, e.Else, e.Guard.Uses("timeout")})
	}
	want := []flags{
		{true, false, false, false},  // x = 1
		{true, false, false, false},  // y == 1
		{false, false, false, false}, // x = 2 (last of atomic)
		{false, true, false, false},  // x = 3
		{false, false, false, false}, // x = 4 (last of d_step)
		{false, false, false, false}, // x > 0
		{false, false, false, false}, // skip
		{false, false, true, false},  // else
		{false, false, false, false}, // skip
		{false, false, false, true},  // timeout
		{false, false, false, false}, // x = 0
		{false, false, false, false}, // -end-
	}
	if len(got) != len(want) {
		t.Fatalf("%d edges, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("edge %d (%s): flags %+v, want %+v", i, p.Edges[i].Text, got[i], want[i])
		}
	}
}

// A loop inside an atomic / d_step block stays inside it: the edge that goes
// round the loop keeps the exclusive control (is atomic / part of the d_step),
// the edge that leaves it by `break` does not. When the loop is the first
// statement of the block its head is the location in front of the block, which
// is outside it, so the flags come from the loop and not from the target.
func TestLoopAtTheStartOfAnAtomicBlockKeepsControl(t *testing.T) {
	for _, kind := range []string{"atomic", "d_step"} {
		src := "byte n;\nactive proctype A() {\n\t" + kind + " { do :: n < 3 -> n++ :: else -> break od };\n\tn = 9\n}\n"
		m, _, err := parseSrc(t, src)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		type flags struct{ a, d bool }
		want := map[string]flags{
			"n < 3": {kind == "atomic", kind == "d_step"}, // enters the block
			"n++":   {kind == "atomic", kind == "d_step"}, // goes round the loop: keeps control
			"else":  {false, false},                       // leaves the loop: gives it up
			"n = 9": {false, false},
		}
		for _, e := range m.Processes[0].Edges {
			w, ok := want[e.Text]
			if !ok {
				continue
			}
			if got := (flags{e.Atomic, e.DStep}); got != w {
				t.Errorf("%s: edge %q has atomic=%v d_step=%v, want atomic=%v d_step=%v", kind, e.Text, got.a, got.d, w.a, w.d)
			}
			delete(want, e.Text)
		}
		if len(want) > 0 {
			t.Errorf("%s: edges not found: %v", kind, want)
		}
	}
}

// A `break` that is the whole of an option leaves the loop: its edge goes to the
// location after the loop, which is the head of the outer loop here (the inner
// loop is the only option of the outer one, so the two heads are one location).
// The edge that goes round the inner loop keeps the control, the one that
// leaves it must not, although both end at that location.
func TestBreakOfALoopAtTheStartOfAnAtomicBlockGivesUpControl(t *testing.T) {
	src := "byte n;\nactive proctype A() {\n\tdo :: atomic { do :: n < 3 -> n++ :: break od } od\n}\n"
	m, _, err := parseSrc(t, src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range m.Processes[0].Edges {
		want := e.Text != "break" && e.Text != "-end-"
		if e.Atomic != want {
			t.Errorf("edge %q: atomic=%v, want %v", e.Text, e.Atomic, want)
		}
	}
}

// The loop that opens an atomic block is only exact when nothing else can be
// done at its head: the lock bit separates "inside" from "outside" but cannot
// keep a sibling alternative out of the block. The shape is refused by name;
// with a statement in front of the loop, or with the loop alone in its option,
// it is accepted.
func TestLoopAtTheStartOfAnAtomicBlockSharedWithAnAlternative(t *testing.T) {
	shared := "byte n;\nactive proctype A() {\n\tdo :: atomic { do :: n < 3 -> n++ :: else -> break od } :: skip -> n = 0 od\n}\n"
	if _, _, err := parseSrc(t, shared); err == nil || !strings.Contains(err.Error(), "shares its entry with another alternative") {
		t.Fatalf("an atomic loop that shares its entry with another option: error %v, want a refusal by name", err)
	}
	for name, src := range map[string]string{
		"statement in front":  "byte n;\nactive proctype A() {\n\tdo :: atomic { n = 0; do :: n < 3 -> n++ :: else -> break od } :: skip -> n = 0 od\n}\n",
		"alone in its option": "byte n;\nactive proctype A() {\n\tdo :: atomic { do :: n < 3 -> n++ :: else -> break od } od\n}\n",
	} {
		if _, _, err := parseSrc(t, src); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRunInstancesAndEndGuards(t *testing.T) {
	src := "proctype E(int x, y) { x = y }\nactive proctype A() { skip }\ninit { run E(3, 4); run E(5, 6) }\n"
	m, _, err := parseSrc(t, src)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range m.Processes {
		names = append(names, p.Name)
	}
	if got := strings.Join(names, ","); got != "A:0,init:1,E:2,E:3" {
		t.Fatalf("processes %s (pids: active and init in textual order, then run-created)", got)
	}
	e := m.Processes[2]
	if e.Params != 2 || len(e.Locals) != 2 || e.Locations[e.Initial].Name != "-dormant-" {
		t.Fatalf("run-created process: params %d locals %d initial %q", e.Params, len(e.Locals), e.Locations[e.Initial].Name)
	}
	initP := m.Processes[1]
	// Both runs draw from the same pool of E instances; the explorer takes
	// the first slot still dormant, which is what frees a slot again when a
	// process dies, as pan frees its pid (G5 addendum).
	for i := 0; i < 2; i++ {
		r := initP.Edges[i].Run
		if r == nil || len(r.Args) != 2 {
			t.Fatalf("init run edge %d: %+v", i, r)
		}
		if got := r.Targets(); len(got) != 2 || got[0] != 2 || got[1] != 3 {
			t.Fatalf("init run edge %d draws from %v, want the pool [2 3]", i, got)
		}
	}
	// With dynamic processes the model carries the live-process table, and
	// SPIN's rule is stated once: only the youngest live process may leave.
	for k, p := range m.Processes {
		end := p.Edges[len(p.Edges)-1]
		if end.Text != "-end-" || !end.Leave || end.Guard == nil || !end.Guard.Uses("youngest") {
			t.Fatalf("%s's -end- edge: %+v", p.Name, end)
		}
		if end.Guard.String() != "youngest("+strconv.Itoa(k)+")" {
			t.Fatalf("%s's -end- guard is %s", p.Name, end.Guard)
		}
	}
	// A dynamic instance returns to its dormant location, so the pool slot
	// can be started again.
	endE := e.Edges[len(e.Edges)-1]
	if e.Locations[endE.To].Name != "-dormant-" {
		t.Fatalf("E:2's -end- goes to %q, want -dormant-", e.Locations[endE.To].Name)
	}
}

// TestStaticModelHasNoProcessTable: a model without `run` keeps exactly the
// encoding G1 fixed against pan — no table, and the -end- guard written as
// the conjunction over the younger processes.
func TestStaticModelHasNoProcessTable(t *testing.T) {
	m, _, err := parseSrc(t, "active proctype A() { skip }\nactive proctype B() { skip }\n")
	if err != nil {
		t.Fatal(err)
	}
	if ir.NeedsTable(m) {
		t.Fatal("a model without run must not carry the live-process table")
	}
	endA := m.Processes[0].Edges[len(m.Processes[0].Edges)-1]
	if endA.Leave || endA.Guard == nil || !endA.Guard.Uses("pc") {
		t.Fatalf("A's -end- edge: %+v", endA)
	}
}

func TestLabelsGotoBreakLowering(t *testing.T) {
	src := "byte x;\nactive proctype A() {\nL1:\tx = 1;\nend:\tdo\n\t:: x > 0 -> x = 0\n\t:: x == 0 -> goto L1\n\t:: break\n\tod;\n\tif :: goto L1 :: x = 2 fi\n}\n"
	m, _, err := parseSrc(t, src)
	if err != nil {
		t.Fatal(err)
	}
	p := m.Processes[0]
	var l1, doHead int = -1, -1
	for i, loc := range p.Locations {
		if loc.Name == "L1" {
			l1 = i
		}
		if loc.Name == "end" {
			doHead = i
			if len(loc.Labels) != 1 || loc.Labels[0] != ir.End {
				t.Fatalf("end label not recorded: %+v", loc)
			}
		}
	}
	if l1 < 0 || doHead < 0 {
		t.Fatalf("labels not found: %+v", p.Locations)
	}
	find := func(text string) ir.Edge {
		for _, e := range p.Edges {
			if e.Text == text {
				return e
			}
		}
		t.Fatalf("no edge %q", text)
		return ir.Edge{}
	}
	if e := find("x == 0"); e.To != l1 {
		t.Errorf("guard followed by goto L1 must target L1: %+v", e)
	}
	if e := find("break"); e.From != doHead || e.To == doHead {
		t.Errorf("lone break is its own edge out of the loop: %+v", e)
	}
	if e := find("goto L1"); e.To != l1 {
		t.Errorf("lone goto option is its own edge: %+v", e)
	}
	if p.Initial != l1 {
		t.Errorf("the label on the first statement is the initial location")
	}
}

func TestNeverClaimAndWarnings(t *testing.T) {
	src := "byte x;\ninit { do :: x = 1 :: printf(\"%d\", x) od }\nnever { accept: do :: x == 1 od }\n"
	m, warnings, err := parseSrc(t, src)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Processes) != 2 || !m.Processes[1].Claim || m.Processes[0].Claim {
		t.Fatalf("claim flags: %+v", m.Processes)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "printf") {
		t.Fatalf("warnings %v", warnings)
	}
	if n := len(m.Properties); n != 2 || m.Properties[1].ID != "never" || m.Properties[1].Kind != ir.KindLTL {
		t.Fatalf("properties %+v", m.Properties)
	}
	res, err2 := explore.Run(context.Background(), m, explore.Options{})
	if err2 != nil {
		t.Fatal(err2)
	}
	// The safety search ignores the claim (2 states); the claim's own
	// product search decides the `never` property (G4).
	if res.States != 2 || res.Outcomes[0].Status != explore.Verified {
		t.Fatalf("claim must be ignored by the safety search: %d states, deadlock %s", res.States, res.Outcomes[0].Status)
	}
	// x is 0 initially, so the claim's only edge (x == 1) is blocked at
	// the first claim step: the claim accepts no run — verified.
	if res.Outcomes[1].Status != explore.Verified {
		t.Fatalf("never: %s %q", res.Outcomes[1].Status, res.Outcomes[1].Reason)
	}
}

// pan numbers: SPIN 6.5.2, spin -a -o1 -o2 -o3; gcc -O2 -DNOREDUCE; pan -c0.
var corpusCounts = []struct {
	file   string
	states int
	class  string
}{
	{"CH2/mutex_flaw.pml", 429, "assert"},
	{"CH2/peterson.pml", 74, ""},
	{"CH2/prodcons.pml", 6, ""},
	{"CH3/alternatingbit.pml", 8, ""},
	{"CH2/peterson2.pml", 42, "deadlock"},
	{"CH2/mutex.pml", 190, ""},
	{"CH2/protocol", 32, "deadlock"},
	{"CH2/protocol2", 25, "deadlock"},
	{"CH2/false.pml", 3, "assert"},
	{"CH2/hello.pml", 3, ""},
	{"CH2/hello2.pml", 3, ""},
	{"CH3/alternatingbit2.pml", 16, ""},
	{"CH3/counter3.pml", 3, ""},
	{"CH3/counter4.pml", 3, ""},
	{"CH3/euclid.pml", 10, ""},
	{"CH3/macro.pml", 5, "assert"},
	{"CH3/mtype.pml", 3, ""},
	{"CH3/rendezvous.pml", 3, "deadlock"},
	{"CH3/send_recv.pml", 10, "deadlock"},
	{"CH3/you_run.pml", 7, ""},
	{"CH3/you_run2.pml", 14, ""},
	// atomic sequences: pan stores no intermediate state while the holder
	// can move, and stores the state when the sequence is interrupted
	{"App_C/petrinet1", 8, "deadlock"},
	{"App_C/petrinet2", 22, "deadlock"},
	{"../model-check-plugin/engine/testdata/promela/atomic-t1.pml", 10, ""},
	{"../model-check-plugin/engine/testdata/promela/atomic-t3.pml", 17, ""},
	{"../model-check-plugin/engine/testdata/promela/atomic-t4.pml", 9, ""},
	{"../model-check-plugin/engine/testdata/promela/atomic-t5.pml", 4, ""},
	{"../model-check-plugin/engine/testdata/promela/atomic-t6.pml", 10, ""},
	{"../model-check-plugin/engine/testdata/promela/atomic-at.pml", 11, ""},
}

func TestCorpusStateCountsMatchPan(t *testing.T) {
	if _, err := os.Stat(corpus); err != nil {
		t.Skip("corpus not found")
	}
	for _, c := range corpusCounts {
		for _, mode := range []explore.Mode{explore.DFS, explore.BFS} {
			path := filepath.Join(corpus, c.file)
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			res, perr := Parse(src, path, nil)
			if perr != nil {
				t.Errorf("%s: %v", c.file, perr)
				continue
			}
			r, err := explore.Run(context.Background(), res.Model, explore.Options{Mode: mode, Sweep: true})
			if err != nil {
				t.Errorf("%s: %v", c.file, err)
				continue
			}
			if !r.Complete || r.States != c.states {
				t.Errorf("%s (%s): %d states complete=%v, pan has %d", c.file, mode, r.States, r.Complete, c.states)
			}
			violated := ""
			for _, o := range r.Outcomes {
				if o.Status == explore.Violated {
					violated = o.Property.ID
				}
			}
			if violated != c.class {
				t.Errorf("%s (%s): violated %q, pan class %q", c.file, mode, violated, c.class)
			}
		}
	}
}

func TestCorpusRejections(t *testing.T) {
	if _, err := os.Stat(corpus); err != nil {
		t.Skip("corpus not found")
	}
	cases := []struct {
		file, kind, construct string
		line                  int
	}{
		{"CH3/pots.pml", KindOutside, "unless", 20},
		{"CH14/version5", KindOutside, "random receive", 211},
		{"CH14/version6", KindOutside, "remote reference", 231},
		{"CH3/notpossible.pml", KindOutside, "run inside an expression", 3},
		{"CH3/scope.pml", KindSemantic, "undeclared variable y", 11},
		{"CH17/simple1.pr", KindOutside, "c_code", 1},
	}
	for _, c := range cases {
		path := filepath.Join(corpus, c.file)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		_, perr := Parse(src, path, nil)
		if perr == nil {
			t.Errorf("%s: accepted", c.file)
			continue
		}
		if perr.Kind != c.kind || perr.Line != c.line || !strings.Contains(perr.Message, c.construct) {
			t.Errorf("%s: %v, want %s %q line %d", c.file, perr, c.kind, c.construct, c.line)
		}
	}
}

func TestByteOverflowIsInvalidModel(t *testing.T) {
	if _, err := os.Stat(corpus); err != nil {
		t.Skip("corpus not found")
	}
	for _, f := range []string{"CH3/counter.pml", "CH3/counter2.pml", "CH3/xr.pml"} {
		path := filepath.Join(corpus, f)
		src, _ := os.ReadFile(path)
		res, perr := Parse(src, path, nil)
		if perr != nil {
			t.Fatalf("%s: %v", f, perr)
		}
		r, err := explore.Run(context.Background(), res.Model, explore.Options{Sweep: true})
		if err != nil {
			t.Fatal(err)
		}
		if r.Outcomes[0].Status != explore.InvalidModel || !strings.Contains(r.Outcomes[0].Reason, "domain overflow") {
			t.Errorf("%s: %s %q (pan wraps bytes silently; the plan makes overflow invalid-model)", f, r.Outcomes[0].Status, r.Outcomes[0].Reason)
		}
	}
}
