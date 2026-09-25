package ctl

import (
	"strings"
	"testing"

	"modelcheck/ir"
)

// testEnv resolves a tiny model: globals x and y, a channel c, two
// processes with named locations.
type testEnv struct{}

func (testEnv) LookupVar(name string) *ir.Var {
	switch name {
	case "x", "y", "p":
		return &ir.Var{Name: name, Type: ir.Byte}
	case "a":
		return &ir.Var{Name: "a", Type: ir.Byte, Len: 3}
	}
	return nil
}

func (testEnv) LookupChan(name string) *ir.Channel {
	if name == "c" {
		return &ir.Channel{Name: "c", Capacity: 2, Fields: []ir.Type{ir.Byte}}
	}
	return nil
}

func (testEnv) ProcessCount() int { return 2 }

func (testEnv) Process(name string) (int, bool) {
	switch name {
	case "P", "P:0":
		return 0, true
	case "Q", "Q:1":
		return 1, true
	}
	return 0, false
}

func (testEnv) Location(p int, label string) (int, bool) {
	if p == 0 && label == "Idle" {
		return 3, true
	}
	if p == 1 && label == "Busy" {
		return 7, true
	}
	return 0, false
}

func parse(t *testing.T, text string) *Formula {
	t.Helper()
	f, err := Parse(text, Options{Env: testEnv{}})
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return f
}

func TestParseAndPrint(t *testing.T) {
	cases := []struct{ in, want string }{
		{"AG p", "AG p"},
		{"AG EF p", "AG EF p"},
		{"EF (x == 1)", "EF (x == 1)"},
		{"E[p U (x > 2)]", "E[p U (x > 2)]"},
		{"A[p U (x == 0)]", "A[p U (x == 0)]"},
		{"!AX p", "!AX p"},
		{"p && (x == 1)", "(p && (x == 1))"},
		{"p -> (x == 1)", "(p -> (x == 1))"},
		{"true", "true"},
		{"EX EX p", "EX EX p"},
		{"len(c) > 1", "(len(c) > 1)"},
		{"empty(c)", "(len(c) == 0)"},
		{"full(c)", "(len(c) == 2)"},
		{"a[1] == 2", "(a[1] == 2)"},
		{"pc_value(1) == 4", "(pc(1) == 4)"},
		{"P@Idle", "(pc(0) == 3)"},
		{"Q:1@Busy", "(pc(1) == 7)"},
		{"_nr_pr == 2", "(_nr_pr == 2)"},
	}
	for _, c := range cases {
		if got := parse(t, c.in).String(); got != c.want {
			t.Errorf("Parse(%q).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "empty formula"},
		{"AG U x", "U is a CTL operator"},
		{"[]p", "not a CTL operator"},
		{"<>p", "not a CTL operator"},
		{"E[p q]", "expected U inside E[ … U … ]"},
		{"A[p U (x == 1)", "expected ] to close A["},
		{"AG zz", "undeclared variable"},
		{"R@Idle", "no process called R"},
		{"P@Nowhere", "has no control location labelled Nowhere"},
		{"AG p q", "unexpected"},
		{"pc_value(x)", "constant process number"},
	}
	for _, c := range cases {
		_, err := Parse(c.in, Options{Env: testEnv{}})
		if err == nil {
			t.Errorf("%q: accepted", c.in)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: %v, want it to mention %q", c.in, err, c.want)
		}
	}
}

func TestDefinesAreExpanded(t *testing.T) {
	f, err := Parse("AG EF idle", Options{Env: testEnv{}, Defines: map[string]string{"idle": "(x == 0)"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.String(); got != "AG EF (x == 0)" {
		t.Fatalf("got %q", got)
	}
	if _, err := Parse("AG loop", Options{Env: testEnv{}, Defines: map[string]string{"loop": "loop"}}); err == nil ||
		!strings.Contains(err.Error(), "recursive") {
		t.Fatalf("a recursive define must be refused, got %v", err)
	}
}

func TestNormaliseIsInTheBasis(t *testing.T) {
	cases := []struct{ in, want string }{
		{"EF p", "E[true U p]"},
		{"AG p", "!E[true U !p]"},
		{"AF p", "!EG !p"},
		{"AX p", "!EX !p"},
		{"EX p", "EX p"},
		{"EG p", "EG p"},
		{"E[p U (x == 1)]", "E[p U (x == 1)]"},
		{"A[p U (x == 1)]", "(!E[!(x == 1) U (!p && !(x == 1))] && !EG !(x == 1))"},
		{"AG EF p", "!E[true U !E[true U p]]"},
		{"p -> (x == 1)", "(!p || (x == 1))"},
		{"!!p", "p"},
		{"AG true", "!E[true U false]"},
	}
	for _, c := range cases {
		n := Normalise(parse(t, c.in))
		if got := n.String(); got != c.want {
			t.Errorf("Normalise(%q) = %q, want %q", c.in, got, c.want)
		}
		if !IsNormal(n) {
			t.Errorf("Normalise(%q) = %q is not in the EX/EU/EG basis", c.in, n)
		}
	}
}

// ---- labelling ------------------------------------------------------------------

// smallGraph is the graph the labelling tests run on; it is the textbook
// three-state Kripke structure of notes 05 ch. 6 (Clarke et al.):
//
//	s0 --> s1 --> s2 --> s2      (s2 is a sink with a self-loop)
//	s0 --> s0
//
// with p true in s0 and s1, q true only in s2.
type smallGraph struct {
	succ [][]int32
	pred [][]int32
}

func newGraph(succ [][]int32) *smallGraph {
	g := &smallGraph{succ: succ, pred: make([][]int32, len(succ))}
	for i, ss := range succ {
		for _, t := range ss {
			g.pred[t] = append(g.pred[t], int32(i))
		}
	}
	return g
}

func (g *smallGraph) Len() int           { return len(g.succ) }
func (g *smallGraph) Succ(i int) []int32 { return g.succ[i] }
func (g *smallGraph) Pred(i int) []int32 { return g.pred[i] }

func valuation(sets map[string][]bool) Valuation {
	return func(a *Atom) ([]bool, error) {
		s, ok := sets[a.Text]
		if !ok {
			return nil, &Error{0, "no valuation for " + a.Text}
		}
		return s, nil
	}
}

func labelOf(t *testing.T, g Graph, val Valuation, text string) ([]bool, *Labelling, *Formula) {
	t.Helper()
	f, err := Parse(text, Options{})
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	n := Normalise(f)
	l, err := Label(g, n, val)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	sat, err := l.Eval(n)
	if err != nil {
		t.Fatal(err)
	}
	return sat, l, f
}

func TestLabellingOnTheTextbookGraph(t *testing.T) {
	g := newGraph([][]int32{{0, 1}, {2}, {2}})
	val := valuation(map[string][]bool{
		"p": {true, true, false},
		"q": {false, false, true},
	})
	cases := []struct {
		formula string
		want    []bool
	}{
		{"p", []bool{true, true, false}},
		{"q", []bool{false, false, true}},
		{"EX q", []bool{false, true, true}},
		{"AX q", []bool{false, true, true}},
		{"EF q", []bool{true, true, true}},
		{"AF q", []bool{false, true, true}}, // s0 can loop on itself forever
		{"EG p", []bool{true, false, false}},
		{"AG p", []bool{false, false, false}},
		{"E[p U q]", []bool{true, true, true}},
		{"A[p U q]", []bool{false, true, true}},
		{"AG EF q", []bool{true, true, true}},
		{"!EF q", []bool{false, false, false}},
		{"EF (p && q)", []bool{false, false, false}},
	}
	for _, c := range cases {
		sat, _, _ := labelOf(t, g, val, c.formula)
		for i := range c.want {
			if sat[i] != c.want[i] {
				t.Errorf("%s: sat = %v, want %v", c.formula, sat, c.want)
				break
			}
		}
	}
}

// TestLabellingOnATrap checks EG and AF where a state can never come back:
//
//	s0 -> s1 -> s2 (self-loop), p true in s0 and s1 only.
func TestLabellingOnATrap(t *testing.T) {
	g := newGraph([][]int32{{1}, {2}, {2}})
	val := valuation(map[string][]bool{"p": {true, true, false}})
	cases := []struct {
		formula string
		want    []bool
	}{
		{"EG p", []bool{false, false, false}}, // no cycle inside p
		{"EF p", []bool{true, true, false}},
		{"AG EF p", []bool{false, false, false}},
		{"AF !p", []bool{true, true, true}},
	}
	for _, c := range cases {
		sat, _, _ := labelOf(t, g, val, c.formula)
		for i := range c.want {
			if sat[i] != c.want[i] {
				t.Errorf("%s: sat = %v, want %v", c.formula, sat, c.want)
				break
			}
		}
	}
}

// TestEGOnACycleThatIsNotASink: a two-state cycle inside p reached through
// a state outside it — only the cycle and what reaches it *within* p
// satisfies EG p.
func TestEGOnACycleThatIsNotASink(t *testing.T) {
	// s0 -> s1 -> s2 -> s1 ; p in s1, s2 only.
	g := newGraph([][]int32{{1}, {2}, {1}})
	val := valuation(map[string][]bool{"p": {false, true, true}})
	sat, _, _ := labelOf(t, g, val, "EG p")
	want := []bool{false, true, true}
	for i := range want {
		if sat[i] != want[i] {
			t.Fatalf("EG p: %v, want %v", sat, want)
		}
	}
}

func TestLabelRefusesADenormalisedFormula(t *testing.T) {
	g := newGraph([][]int32{{0}})
	val := valuation(map[string][]bool{"p": {true}})
	f, err := Parse("AG p", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Label(g, f, val); err == nil || !strings.Contains(err.Error(), "EX/EU/EG basis") {
		t.Fatalf("Label must refuse a formula that is not normalised, got %v", err)
	}
}

// ---- witnesses -------------------------------------------------------------------

func TestWitnessDivisionIsExhaustive(t *testing.T) {
	g := newGraph([][]int32{{0, 1}, {2}, {2}})
	val := valuation(map[string][]bool{
		"p": {true, true, false},
		"q": {false, false, true},
	})
	cases := []struct {
		formula string
		kind    string
		why     string
	}{
		{"EF q", "path", ""},
		{"EX p", "path", ""},
		{"E[p U q]", "path", ""},
		{"EG p", "lasso", ""},
		{"AG p", "path", ""},  // fails: a path to the state where p is false
		{"AF q", "lasso", ""}, // fails: the self-loop of s0
		{"AX q", "path", ""},  // fails: s0 -> s0
		{"AG EF q", "none", whyUniversalHolds},
		{"EF (p && q)", "none", whyExistentialFails},
		{"EG q", "none", whyExistentialFails},
		{"p", "none", whyBoolean},
		{"p && q", "none", whyBoolean},
		{"!EF q", "none", whyBoolean},
	}
	for _, c := range cases {
		sat, lab, f := labelOf(t, g, val, c.formula)
		w, err := BuildWitness(g, f, lab, sat[0])
		if err != nil {
			t.Fatalf("%s: %v", c.formula, err)
		}
		if w.Kind != c.kind {
			t.Errorf("%s: witness kind %q, want %q (why: %s)", c.formula, w.Kind, c.kind, w.Why)
			continue
		}
		if c.kind == "none" && w.Why != c.why {
			t.Errorf("%s: why %q, want %q", c.formula, w.Why, c.why)
		}
		if c.kind == "path" && (len(w.Path) == 0 || w.Path[0] != 0 || w.Loop != -1) {
			t.Errorf("%s: path %v loop %d", c.formula, w.Path, w.Loop)
		}
		if c.kind == "lasso" {
			if w.Loop < 0 || len(w.Path) < 2 || w.Path[0] != 0 {
				t.Errorf("%s: lasso %v loop %d", c.formula, w.Path, w.Loop)
			} else if w.Path[len(w.Path)-1] != w.Path[w.Loop] {
				t.Errorf("%s: the lasso does not close: %v loop %d", c.formula, w.Path, w.Loop)
			}
		}
	}
}

// TestAUHasTwoWitnessShapes: !A[f U g] is a disjunction, so the case has
// two shapes — a finite path when a run fails f before g ever holds, and a
// lasso when a run never reaches g. Both refute the property, and the
// feature file promises both.
func TestAUHasTwoWitnessShapes(t *testing.T) {
	// s0 -> s1 -> s1 : p holds in s0 only, q nowhere. A[p U q] fails
	// because s1 has neither p nor q — a finite path refutes it.
	g := newGraph([][]int32{{1}, {1}})
	val := valuation(map[string][]bool{"p": {true, false}, "q": {false, false}})
	sat, lab, f := labelOf(t, g, val, "A[p U q]")
	w, err := BuildWitness(g, f, lab, sat[0])
	if err != nil {
		t.Fatal(err)
	}
	if w.Kind != "path" {
		t.Fatalf("witness kind %q (%s), want a finite path to the state where p failed before q held", w.Kind, w.Why)
	}

	// s0 -> s0 : p always holds, q never. A[p U q] fails because q never
	// comes — the refutation is a lasso.
	g2 := newGraph([][]int32{{0}})
	val2 := valuation(map[string][]bool{"p": {true}, "q": {false}})
	sat2, lab2, f2 := labelOf(t, g2, val2, "A[p U q]")
	w2, err := BuildWitness(g2, f2, lab2, sat2[0])
	if err != nil {
		t.Fatal(err)
	}
	if w2.Kind != "lasso" {
		t.Fatalf("witness kind %q (%s), want a lasso on which q never comes", w2.Kind, w2.Why)
	}
}

func TestWitnessPathsAreRealPaths(t *testing.T) {
	g := newGraph([][]int32{{0, 1}, {2}, {2}})
	val := valuation(map[string][]bool{"p": {true, true, false}, "q": {false, false, true}})
	sat, lab, f := labelOf(t, g, val, "EF q")
	w, err := BuildWitness(g, f, lab, sat[0])
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(w.Path); i++ {
		ok := false
		for _, s := range g.Succ(w.Path[i]) {
			if int(s) == w.Path[i+1] {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("%v is not a path of the graph", w.Path)
		}
	}
	if last := w.Path[len(w.Path)-1]; last != 2 {
		t.Fatalf("the witness of EF q ends at %d, want the q state 2", last)
	}
}

// ---- vacuity material -------------------------------------------------------------

func TestStatsAndAntecedents(t *testing.T) {
	g := newGraph([][]int32{{0, 1}, {2}, {2}})
	val := valuation(map[string][]bool{
		"p": {true, true, true},    // never false
		"q": {false, false, false}, // never true
	})
	f, err := Parse("AG (q -> p)", Options{})
	if err != nil {
		t.Fatal(err)
	}
	st, err := Stats(g, f, val)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 2 {
		t.Fatalf("%d atoms, want 2", len(st))
	}
	byName := map[string]AtomStat{}
	for _, s := range st {
		byName[s.Atom.Text] = s
	}
	if byName["q"].EverTrue || !byName["q"].EverFalse {
		t.Errorf("q: %+v, want never true", byName["q"])
	}
	if !byName["p"].EverTrue || byName["p"].EverFalse {
		t.Errorf("p: %+v, want never false", byName["p"])
	}
	if got := Antecedents(f); len(got) != 1 || got[0] != "q" {
		t.Errorf("antecedents %v, want [q]", got)
	}
	if got := Antecedents(parse(t, "AG p")); len(got) != 0 {
		t.Errorf("a formula without an implication has antecedents %v", got)
	}
}

func TestModelEnvResolvesProcessesAndLabels(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "m",
		Globals: []ir.Var{{Name: "x", Type: ir.Byte}},
		Processes: []ir.Process{
			{Name: "sub:0", Locations: []ir.Location{{Name: "Idle"}, {Name: "Busy"}}, Edges: []ir.Edge{{From: 0, To: 1}, {From: 1, To: 0}}},
		}}
	l, err := ir.NewLayout(m)
	if err != nil {
		t.Fatal(err)
	}
	env := ModelEnv(l)
	if p, ok := env.Process("sub"); !ok || p != 0 {
		t.Fatalf("Process(\"sub\") = %d, %v", p, ok)
	}
	if p, ok := env.Process("sub:0"); !ok || p != 0 {
		t.Fatalf("Process(\"sub:0\") = %d, %v", p, ok)
	}
	if loc, ok := env.Location(0, "Busy"); !ok || loc != 1 {
		t.Fatalf("Location(0, Busy) = %d, %v", loc, ok)
	}
	f, err := Parse("AG EF sub@Idle", Options{Env: env})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.String(); got != "AG EF (pc(0) == 0)" {
		t.Fatalf("got %q", got)
	}
}
