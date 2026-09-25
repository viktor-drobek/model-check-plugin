package promela

import (
	"strings"
	"testing"

	"modelcheck/ir"
)

// Unit tests for the v1 subset of plan 14 §5.2 (G5): inline substitution,
// typedef flattening, `provided`, channel values, and the instantiation
// order of a dynamic `run`. The numbers quoted as pan's come from SPIN
// 6.5.2 probes recorded in steps/g5-confirmation.md.

func lowerSrc(t *testing.T, src string) *ir.Model {
	t.Helper()
	m, _, err := parseSrc(t, src)
	if err != nil {
		t.Fatalf("%v\n%s", err, src)
	}
	return m
}

func edgeTexts(p *ir.Process) []string {
	var out []string
	for _, e := range p.Edges {
		out = append(out, e.Text)
	}
	return out
}

func edgeLines(p *ir.Process) []int {
	var out []int
	for _, e := range p.Edges {
		line := 0
		if e.Origin != nil {
			line = e.Origin.Line
		}
		out = append(out, line)
	}
	return out
}

func findProc(t *testing.T, m *ir.Model, name string) *ir.Process {
	t.Helper()
	for i := range m.Processes {
		if m.Processes[i].Name == name {
			return &m.Processes[i]
		}
	}
	t.Fatalf("no process %q in %v", name, procNames(m))
	return nil
}

func procNames(m *ir.Model) []string {
	var out []string
	for _, p := range m.Processes {
		out = append(out, p.Name)
	}
	return out
}

// TestInlineSubstitutesArgumentsAndKeepsBodyLines: the expansion carries
// the inline body's lines, which is what pan -d prints for it (probe on
// CH3/inline.pml: lines 2, 3, 4 inside init).
func TestInlineSubstitutesArgumentsAndKeepsBodyLines(t *testing.T) {
	src := "inline example(x, y) {\n\ty = a;\n\tx = b;\n\tassert(x)\n}\ninit {\n\tint a, b;\n\n\texample(a,b)\n}\n"
	m := lowerSrc(t, src)
	p := findProc(t, m, "init:0")
	got := edgeTexts(p)
	want := []string{"b = a", "a = b", "assert(a)", "-end-"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("edges %v, want %v", got, want)
	}
	if lines := edgeLines(p); lines[0] != 2 || lines[1] != 3 || lines[2] != 4 {
		t.Fatalf("edge lines %v, want the inline body's 2, 3, 4", lines)
	}
}

// TestInlineLabelsAreUniquePerCall: two expansions of a body with a label
// must not give the process two locations with the same name, and each
// `goto` must stay inside its own expansion.
func TestInlineLabelsAreUniquePerCall(t *testing.T) {
	src := "byte x;\ninline twice(v) {\nagain:\tv++;\n\tif\n\t:: v < 2 -> goto again\n\t:: else\n\tfi\n}\ninit {\n\ttwice(x);\n\ttwice(x)\n}\n"
	m := lowerSrc(t, src)
	p := findProc(t, m, "init:0")
	seen := map[string]int{}
	for _, loc := range p.Locations {
		if loc.Name != "" {
			seen[loc.Name]++
		}
	}
	if len(seen) == 0 {
		t.Fatal("the expansions produced no named location at all")
	}
	for name, n := range seen {
		if n > 1 {
			t.Fatalf("location %q appears %d times: labels of an inline must be renamed per call", name, n)
		}
	}
	var labels []string
	for name := range seen {
		if strings.Contains(name, "again") {
			labels = append(labels, name)
		}
	}
	if len(labels) != 2 {
		t.Fatalf("the two expansions gave %d renamed copies of `again`: %v", len(labels), seen)
	}
	for _, l := range labels {
		if !strings.HasPrefix(l, "twice_") {
			t.Fatalf("renamed label %q should carry the inline's name and call number", l)
		}
	}
}

// TestInlineDeclarationIsAStep: a declaration inside an inline body is a
// nested scope for SPIN, and SPIN generates an initialisation transition
// for it — "y = 0" in CH3/inline2.pml, which is why pan counts 6 states
// there and not 5.
func TestInlineDeclarationIsAStep(t *testing.T) {
	src := "inline thisworks(x) {\n\tint y;\n\n\ty = x;\n\tprintf(\"%d\\n\", y)\n}\ninit {\n\tint a;\n\ta = 34;\n\tthisworks(a)\n}\n"
	m := lowerSrc(t, src)
	p := findProc(t, m, "init:0")
	got := edgeTexts(p)
	want := []string{"a = 34", "y = 0", "y = a", "printf(\"%d\\n\", y)", "-end-"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("edges %v, want %v", got, want)
	}
	// pan attributes both the initialisation and the assignment to the line
	// of the statement that follows the declaration.
	if lines := edgeLines(p); lines[1] != 4 || lines[2] != 4 {
		t.Fatalf("edge lines %v, want the initialisation and the assignment at line 4", lines)
	}
}

// TestDeclarationAtTheTopOfABodyIsNotAStep keeps the G1 rule: at the
// outermost scope of a proctype a declaration only sets the variable's
// initial value.
func TestDeclarationAtTheTopOfABodyIsNotAStep(t *testing.T) {
	m := lowerSrc(t, "init {\n\tint a = 1;\n\ta = 2\n}\n")
	p := findProc(t, m, "init:0")
	if got := edgeTexts(p); strings.Join(got, "|") != "a = 2|-end-" {
		t.Fatalf("edges %v, want just the assignment and -end-", got)
	}
	if p.Locals[0].Init == nil || p.Locals[0].Init[0] != 1 {
		t.Fatalf("local a: %+v, want initial value 1", p.Locals[0])
	}
}

// TestRedeclarationInAnotherBlockKeepsOneVariable: the rule that a nested
// declaration is a step, and that a repeat of the name in another scope
// re-initialises the same variable, is SPIN's — probed on a plain `{ }`
// block as well as on an inline body, so the generalisation rests on both
// shapes of nested scope (SPIN 6.5.2 gives 6 states and the four
// assignments below).
func TestRedeclarationInAnotherBlockKeepsOneVariable(t *testing.T) {
	m := lowerSrc(t, "init {\n\t{ int y;\n\t  y = 1\n\t};\n\t{ int y;\n\t  y = 2\n\t}\n}\n")
	p := findProc(t, m, "init:0")
	if got := edgeTexts(p); strings.Join(got, "|") != "y = 0|y = 1|y = 0|y = 2|-end-" {
		t.Fatalf("edges %v, want the two initialisations and the two assignments", got)
	}
	n := 0
	for _, v := range p.Locals {
		if v.Name == "y" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d locals called y; SPIN keeps one and re-initialises it", n)
	}
	// Two different types under one name cannot share a slot.
	if _, _, err := parseSrc(t, "init { { int y; y++ }; { byte y; y++ } }\n"); err == nil ||
		!strings.Contains(err.Message, "different types") {
		t.Fatalf("a redeclaration with another type must be refused, got %v", err)
	}
}

// TestTypedefFlattening: a typedef instance becomes plain variables with
// dotted names, nested structs included, and SPIN's own flattening is
// reproduced name for name (probe: pan -d on CH3/typedef.pml prints "z.g",
// "foo.f", "foo.g").
func TestTypedefFlattening(t *testing.T) {
	src := "typedef Field {\n\tshort f = 3;\n\tbyte g\n};\n\ntypedef Record {\n\tbyte a[3];\n\tint fld1;\n\tField fld2;\n\tbit b\n};\n\nproctype me(Field z) {\n\tz.g = 12\n}\n\ninit {\n\tRecord goo;\n\tField foo;\n\n\trun me(foo)\n}\n"
	m := lowerSrc(t, src)
	init := findProc(t, m, "init:0")
	var names []string
	for _, v := range init.Locals {
		names = append(names, v.Name)
	}
	want := []string{"goo.a", "goo.fld1", "goo.fld2.f", "goo.fld2.g", "goo.b", "foo.f", "foo.g"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("init locals %v, want %v", names, want)
	}
	byName := map[string]ir.Var{}
	for _, v := range init.Locals {
		byName[v.Name] = v
	}
	if a := byName["goo.a"]; a.Len != 3 || a.Type != ir.Byte {
		t.Errorf("goo.a: %+v, want a byte array of 3", a)
	}
	if f := byName["goo.fld2.f"]; f.Type != ir.Short || len(f.Init) == 0 || f.Init[0] != 3 {
		t.Errorf("goo.fld2.f: %+v, want a short starting at 3 (the typedef's field initialiser)", f)
	}
	if b := byName["goo.b"]; b.Type != ir.Bit {
		t.Errorf("goo.b: %+v, want a bit", b)
	}
	// A struct parameter is passed field by field, and the field
	// initialiser of the typedef does not apply to a parameter.
	me := findProc(t, m, "me:1")
	if me.Params != 2 || me.Locals[0].Name != "z.f" || me.Locals[1].Name != "z.g" {
		t.Fatalf("me's parameters: %d, %v", me.Params, me.Locals)
	}
	if len(me.Locals[0].Init) != 0 {
		t.Errorf("parameter z.f carries an initialiser %v; a parameter's value comes from the call", me.Locals[0].Init)
	}
	run := init.Edges[0].Run
	if run == nil || len(run.Args) != 2 {
		t.Fatalf("the run edge passes %v, want the two fields of foo", run)
	}
	if run.Args[0].String() != "foo.f" || run.Args[1].String() != "foo.g" {
		t.Fatalf("run args %s, %s; want foo.f, foo.g", run.Args[0], run.Args[1])
	}
}

// TestProvidedIsAProcessLevelGuard: `provided` gates every transition of
// the process, `else` and `-end-` included, so it cannot be folded into
// the edges' own guards (an `else` edge has none).
func TestProvidedIsAProcessLevelGuard(t *testing.T) {
	m := lowerSrc(t, "bool toggle = true;\nactive proctype A() provided (toggle == true) {\n\tdo\n\t:: toggle = false\n\tod\n}\nactive proctype B() { skip }\n")
	a := findProc(t, m, "A:0")
	if a.Provided == nil || a.Provided.String() != "toggle == 1" {
		t.Fatalf("A.Provided = %v, want the clause", a.Provided)
	}
	for _, e := range a.Edges {
		if e.Guard != nil && strings.Contains(e.Guard.String(), "toggle") {
			t.Fatalf("the clause was folded into an edge guard (%s); it belongs to the process", e.Guard)
		}
	}
	if b := findProc(t, m, "B:1"); b.Provided != nil {
		t.Fatalf("B has no provided clause but carries %v", b.Provided)
	}
}

// TestRunAllocationOrderMatchesPan: every `run` draws from the pool of its
// proctype and the explorer takes the first instance still dormant, so the
// k-th live instance of a proctype is the k-th slot of its pool. That
// invariant is what keeps the vector an image of pan's process stack: when
// a process dies, pan frees its pid and the next `run` takes it back, and
// the pool does the same with its slot. A `run` taken at most once gets a
// pool of one; a `run` inside a loop gets DefaultMaxProcs.
func TestRunAllocationOrderMatchesPan(t *testing.T) {
	// Straight-line runs: as many instances as run statements, in textual
	// order, and both runs draw from the same pool.
	m := lowerSrc(t, "proctype E(int x) { x = 1 }\nactive proctype A() { skip }\ninit { run E(3); run E(5) }\n")
	if got := strings.Join(procNames(m), ","); got != "A:0,init:1,E:2,E:3" {
		t.Fatalf("processes %s; want the static ones first in textual order, then the pool", got)
	}
	init := findProc(t, m, "init:1")
	for i := range []int{0, 1} {
		pool := init.Edges[i].Run.Targets()
		if len(pool) != 2 || pool[0] != 2 || pool[1] != 3 {
			t.Fatalf("run %d draws from %v; both runs share the proctype's pool, in allocation order", i, pool)
		}
	}
	for _, name := range []string{"E:2", "E:3"} {
		p := findProc(t, m, name)
		if !p.Dynamic || p.Locations[p.Initial].Name != "-dormant-" {
			t.Errorf("%s: dynamic=%v initial=%q", name, p.Dynamic, p.Locations[p.Initial].Name)
		}
	}

	// A run inside a loop: a pool of DefaultMaxProcs, in allocation order.
	m2 := lowerSrc(t, "byte n;\nproctype W() { skip }\ninit {\n\tdo\n\t:: n < 2 -> n++; run W()\n\t:: else -> break\n\tod\n}\n")
	initP := findProc(t, m2, "init:0")
	var runEdge *ir.Edge
	for i := range initP.Edges {
		if initP.Edges[i].Run != nil {
			runEdge = &initP.Edges[i]
		}
	}
	if runEdge == nil {
		t.Fatal("no run edge in init")
	}
	pool := runEdge.Run.Targets()
	if len(pool) != DefaultMaxProcs {
		t.Fatalf("pool of %d instances, want DefaultMaxProcs = %d", len(pool), DefaultMaxProcs)
	}
	for i, q := range pool {
		if q != i+1 {
			t.Fatalf("pool %v: the instances must be consecutive and in allocation order", pool)
		}
	}
	// Every instance of the pool leaves the table only as the youngest
	// process, which is SPIN's rule.
	for _, q := range pool {
		p := &m2.Processes[q]
		end := p.Edges[len(p.Edges)-1]
		if !end.Leave || end.Guard == nil || !end.Guard.Uses("youngest") {
			t.Fatalf("%s's -end-: %+v", p.Name, end)
		}
	}
}

// TestMaxProcsIsHonoured: the pool size is the declared bound, and it is
// the flag that sets it.
func TestMaxProcsIsHonoured(t *testing.T) {
	src := "byte n;\nproctype W() { skip }\ninit {\n\tdo\n\t:: n < 9 -> n++; run W()\n\t:: else -> break\n\tod\n}\n"
	res, err := ParseWith([]byte(src), "t.pml", nil, Options{MaxProcs: 3})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, p := range res.Model.Processes {
		if p.Dynamic {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("%d dynamic instances with --max-procs 3", n)
	}
}

// TestDynamicLocalInitialisersRunAtCreation: a dynamic instance keeps
// everything at zero while dormant (so that a dormant slot and a slot
// whose process has died are the same state), and its initialisers run as
// part of the `run` step, in the new process's own scope — which is what
// lets `byte maximum = mynumber` read a parameter, as CH9/leader does.
func TestDynamicLocalInitialisersRunAtCreation(t *testing.T) {
	m := lowerSrc(t, "proctype node(byte mynumber) {\n\tbit Active = 1;\n\tbyte maximum = mynumber;\n\tmaximum = Active\n}\ninit { run node(4) }\n")
	node := findProc(t, m, "node:1")
	for _, v := range node.Locals {
		if len(v.Init) != 0 {
			t.Errorf("%s carries a static initial value %v; a dynamic instance starts at zero", v.Name, v.Init)
		}
	}
	run := findProc(t, m, "init:0").Edges[0].Run
	if run == nil || len(run.Init) != 2 {
		t.Fatalf("the run edge carries %v initialisers, want 2", run)
	}
	if run.Init[0].Var != "Active" || run.Init[0].Value.String() != "1" {
		t.Errorf("first initialiser %+v", run.Init[0])
	}
	if run.Init[1].Var != "maximum" || run.Init[1].Value.String() != "mynumber" {
		t.Errorf("second initialiser %+v; it must read the parameter", run.Init[1])
	}
}

// TestChannelArraysAndChannelValues: a channel array becomes one IR
// channel per element plus a byte array of channel ids, so that `q[i]`
// with a computed index is an ordinary array read that yields a channel;
// a channel used as a value is its id.
func TestChannelArraysAndChannelValues(t *testing.T) {
	m := lowerSrc(t, "chan q[3] = [2] of { byte };\nchan g = [0] of { chan };\nproctype node(chan in) { in?_ }\ninit { byte i; g!q[i]; run node(q[1]) }\n")
	var names []string
	for _, c := range m.Channels {
		names = append(names, c.Name)
	}
	if strings.Join(names, "|") != "q[0]|q[1]|q[2]|g" {
		t.Fatalf("channels %v", names)
	}
	var arr *ir.Var
	for i := range m.Globals {
		if m.Globals[i].Name == "q" {
			arr = &m.Globals[i]
		}
	}
	if arr == nil || arr.Len != 3 || arr.Type != ir.Byte {
		t.Fatalf("the channel array needs a byte array of ids, got %+v", arr)
	}
	if len(arr.Init) != 3 || arr.Init[0] != ir.ChanID(0) || arr.Init[2] != ir.ChanID(2) {
		t.Fatalf("q's ids %v, want %d..%d", arr.Init, ir.ChanID(0), ir.ChanID(2))
	}
	init := findProc(t, m, "init:0")
	send := init.Edges[0].Send
	if send == nil || send.Chan != "g" || send.Args[0].String() != "q[i]" {
		t.Fatalf("the send is %+v; it must put the id of q[i] on g", send)
	}
	// The receive of the parameter is on a channel known only in a state.
	node := findProc(t, m, "node:1")
	if node.Edges[0].Recv == nil || node.Edges[0].Recv.Sel == nil || node.Edges[0].Recv.Sel.String() != "in" {
		t.Fatalf("node's receive is %+v; it must select the channel from its parameter", node.Edges[0].Recv)
	}
}

// TestNrPrAndYoungestNeedTheTable: the live-process table appears exactly
// when the model needs it, so that a model without `run` keeps the vector
// G1 fixed against pan.
func TestNrPrAndYoungestNeedTheTable(t *testing.T) {
	with := lowerSrc(t, "proctype W() { skip }\ninit { run W() }\n")
	if !ir.NeedsTable(with) {
		t.Error("a model with run needs the live-process table")
	}
	without := lowerSrc(t, "active proctype A() { skip }\n")
	if ir.NeedsTable(without) {
		t.Error("a model without run must not carry the table")
	}
	nr := lowerSrc(t, "active proctype A() { _nr_pr > 0 }\n")
	if !ir.NeedsTable(nr) {
		t.Error("a model that reads _nr_pr needs the table")
	}
}

// TestPcValueIsTheEnginesOwnNumbering records the deviation: `pc_value`
// yields this engine's location index, not pan's state number, and the
// frontend says so rather than letting a reader assume they agree.
func TestPcValueIsTheEnginesOwnNumbering(t *testing.T) {
	m, warnings, err := parseSrc(t, "active proctype A() { skip; skip }\nactive proctype B() { pc_value(0) > 1 }\n")
	if err != nil {
		t.Fatal(err)
	}
	b := findProc(t, m, "B:1")
	if b.Edges[0].Guard == nil || !b.Edges[0].Guard.Uses("pc") {
		t.Fatalf("B's guard is %v, want a pc read", b.Edges[0].Guard)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "pc_value") && strings.Contains(w, "numbering") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings %v say nothing about the numbering", warnings)
	}
}

// TestUndefinedGotoIsRejected: a jump to a label no statement carries
// changes the model if it is dropped, so it is refused by name, file and
// line (SPIN reports the same).
func TestUndefinedGotoIsRejected(t *testing.T) {
	_, _, err := parseSrc(t, "byte x;\nactive proctype A() {\n\tx = 1;\n\tgoto Nowhere\n}\n")
	if err == nil {
		t.Fatal("an undefined goto label was accepted")
	}
	if err.Kind != KindSemantic || err.Line != 4 {
		t.Fatalf("%v: want a semantic error at line 4", err)
	}
	if !strings.Contains(err.Message, "undefined label Nowhere") {
		t.Fatalf("message %q must name the label", err.Message)
	}
	// A label defined later in the body is fine: the goto is forward.
	if _, _, err := parseSrc(t, "byte x;\nactive proctype A() {\n\tgoto L;\nL:\tx = 1\n}\n"); err != nil {
		t.Fatalf("a forward goto must be accepted: %v", err)
	}
}
