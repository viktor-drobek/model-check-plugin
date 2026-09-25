package ltl

import (
	"math/rand"
	"strings"
	"testing"

	"modelcheck/ir"
)

// ---- parsing ----------------------------------------------------------------------

func TestParsePrecedence(t *testing.T) {
	cases := map[string]string{
		"[]p":                       "[]p",
		"<>[]p":                     "<>[]p",
		"[]<>p":                     "[]<>p",
		"p U q":                     "(p U q)",
		"p U q U r":                 "(p U (q U r))",
		"p V q":                     "(p V q)",
		"X p":                       "X p",
		"!p":                        "!p",
		"p && q || r":               "((p && q) || r)",
		"p || q && r":               "(p || (q && r))",
		"p -> q -> r":               "(p -> (q -> r))",
		"p <-> q":                   "(p <-> q)",
		"[]p -> <>q":                "([]p -> <>q)",
		"p U q && r":                "((p U q) && r)",
		"[](p -> <>q)":              "[](p -> <>q)",
		"(x < 4)":                   "x < 4",
		"(x + 1 == y * 2)":          "(x + 1) == (y * 2)",
		"a[2] == 1":                 "a[2] == 1",
		"len(q) > 0":                "len(q) > 0",
		"empty(q)":                  "len(q) == 0",
		"nempty(q)":                 "len(q) > 0",
		"true U false":              "(true U false)",
		"p /\\ q":                   "(p && q)",
		"p \\/ q":                   "(p || q)",
		"[] (len(q) > 0 -> <> (x))": "[](len(q) > 0 -> <>x)",
		"!(x != 0)":                 "!x != 0",
	}
	for in, want := range cases {
		f, err := Parse(in, Options{})
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got := f.String(); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestParseDefines(t *testing.T) {
	defs := map[string]string{"p": "(x != 0)", "N": "4", "q": "<>p", "loop": "loop2", "loop2": "loop"}
	f, err := Parse("[]p && (y < N) && q", Options{Defines: defs})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.String(); got != "(([]x != 0 && y < 4) && <>x != 0)" {
		t.Errorf("got %q", got)
	}
	if _, err := Parse("<> loop", Options{Defines: defs}); err == nil || !strings.Contains(err.Error(), "recursive") {
		t.Errorf("recursive define not reported: %v", err)
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{"", "[] (p ->", "p q", "p U", "(p", "p )", "3 +", "X", "len p", "p @ q"}
	for _, in := range bad {
		if _, err := Parse(in, Options{}); err == nil {
			t.Errorf("%q: expected an error", in)
		}
	}
}

type testScope struct{}

func (testScope) LookupVar(name string) *ir.Var {
	switch name {
	case "x", "y", "done":
		return &ir.Var{Name: name, Type: ir.Byte}
	case "a":
		return &ir.Var{Name: name, Type: ir.Byte, Len: 3}
	}
	return nil
}
func (testScope) LookupChan(name string) *ir.Channel {
	if name == "q" {
		return &ir.Channel{Name: "q", Capacity: 2, Fields: []ir.Type{ir.Byte}}
	}
	return nil
}
func (testScope) ProcessCount() int { return 1 }

func TestParseScope(t *testing.T) {
	if _, err := Parse("<> nosuchvar", Options{Scope: testScope{}}); err == nil || !strings.Contains(err.Error(), "nosuchvar") {
		t.Errorf("undeclared atom accepted: %v", err)
	}
	if _, err := Parse("[] a", Options{Scope: testScope{}}); err == nil {
		t.Error("array read without index accepted")
	}
	f, err := Parse("full(q) || nfull(q) || a[1] > x", Options{Scope: testScope{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.String(); got != "((len(q) == 2 || len(q) < 2) || a[1] > x)" {
		t.Errorf("got %q", got)
	}
}

func TestNNF(t *testing.T) {
	cases := map[string]string{
		"!(p U q)":     "(!p V !q)",
		"!(p V q)":     "(!p U !q)",
		"![]p":         "<>!p",
		"!<>p":         "[]!p",
		"!X p":         "X !p",
		"!(p -> q)":    "(p && !q)",
		"p -> q":       "(!p || q)",
		"p <-> q":      "((p && q) || (!p && !q))",
		"!(p <-> q)":   "((p && !q) || (!p && q))",
		"!!p":          "p",
		"!(p && q)":    "(!p || !q)",
		"!(<>[]p)":     "[]<>!p",
		"!true":        "false",
		"[](p -> <>q)": "[](!p || <>q)",
	}
	for in, want := range cases {
		f, err := Parse(in, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if got := NNF(f).String(); got != want {
			t.Errorf("NNF(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---- brute-force semantics on lassos --------------------------------------------------

// lasso is an ultimately periodic word: valuations vals[0..n-1], with
// position n continuing at loop.
type lasso struct {
	vals []map[string]bool
	loop int
}

func (w lasso) next(i int) int {
	if i+1 < len(w.vals) {
		return i + 1
	}
	return w.loop
}

// holds evaluates f at every position of w by fixpoint iteration over the
// finitely many distinct suffixes (least fixpoint for U / <>, greatest for
// V / []).
func holds(f *Formula, w lasso) []bool {
	n := len(w.vals)
	out := make([]bool, n)
	switch f.Op {
	case True:
		for i := range out {
			out[i] = true
		}
	case False:
	case AtomOp:
		for i := range out {
			out[i] = w.vals[i][f.Atom.Text]
		}
	case Not:
		s := holds(f.L, w)
		for i := range out {
			out[i] = !s[i]
		}
	case And, Or, Impl, Iff:
		a, b := holds(f.L, w), holds(f.R, w)
		for i := range out {
			switch f.Op {
			case And:
				out[i] = a[i] && b[i]
			case Or:
				out[i] = a[i] || b[i]
			case Impl:
				out[i] = !a[i] || b[i]
			case Iff:
				out[i] = a[i] == b[i]
			}
		}
	case Next:
		s := holds(f.L, w)
		for i := range out {
			out[i] = s[w.next(i)]
		}
	case Until, Eventually:
		var a, b []bool
		if f.Op == Until {
			a, b = holds(f.L, w), holds(f.R, w)
		} else {
			a = make([]bool, n)
			for i := range a {
				a[i] = true
			}
			b = holds(f.L, w)
		}
		for iter := 0; iter <= n; iter++ {
			for i := 0; i < n; i++ {
				out[i] = b[i] || (a[i] && out[w.next(i)])
			}
		}
	case Release, Always:
		var a, b []bool
		if f.Op == Release {
			a, b = holds(f.L, w), holds(f.R, w)
		} else {
			a = make([]bool, n)
			b = holds(f.L, w)
		}
		for i := range out {
			out[i] = true
		}
		for iter := 0; iter <= n; iter++ {
			for i := 0; i < n; i++ {
				out[i] = b[i] && (a[i] || out[w.next(i)])
			}
		}
	}
	return out
}

// accepts reports whether a has an accepting run on w: a lasso-shaped
// search in the product of positions and states.
func accepts(a *Automaton, w lasso) bool {
	n := len(w.vals)
	type pq struct{ pos, q int }
	sat := func(l []Lit, v map[string]bool) bool {
		for _, x := range l {
			if v[a.Atoms[x.Atom].Text] == x.Neg {
				return false
			}
		}
		return true
	}
	// Build the product graph.
	adj := map[pq][]pq{}
	var nodes []pq
	seen := map[pq]bool{}
	stack := []pq{{0, 0}}
	seen[stack[0]] = true
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		nodes = append(nodes, x)
		for _, t := range a.States[x.q].Trans {
			if !sat(t.Label, w.vals[x.pos]) {
				continue
			}
			y := pq{w.next(x.pos), t.Target}
			adj[x] = append(adj[x], y)
			if !seen[y] {
				seen[y] = true
				stack = append(stack, y)
			}
		}
	}
	_ = n
	// Nested DFS: from every reachable accepting node, can we come back?
	for _, s := range nodes {
		if !a.States[s.q].Accepting {
			continue
		}
		vis := map[pq]bool{}
		st := []pq{s}
		for len(st) > 0 {
			x := st[len(st)-1]
			st = st[:len(st)-1]
			for _, y := range adj[x] {
				if y == s {
					return true
				}
				if !vis[y] {
					vis[y] = true
					st = append(st, y)
				}
			}
		}
	}
	return false
}

func mustParse(t *testing.T, s string) *Formula {
	t.Helper()
	f, err := Parse(s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func allLassos(atoms []string, maxLen int) []lasso {
	var out []lasso
	nv := 1 << len(atoms)
	var gen func(prefix []map[string]bool)
	gen = func(prefix []map[string]bool) {
		if len(prefix) > 0 {
			for loop := 0; loop < len(prefix); loop++ {
				out = append(out, lasso{vals: append([]map[string]bool(nil), prefix...), loop: loop})
			}
		}
		if len(prefix) == maxLen {
			return
		}
		for v := 0; v < nv; v++ {
			m := map[string]bool{}
			for i, a := range atoms {
				m[a] = v&(1<<i) != 0
			}
			gen(append(prefix, m))
		}
	}
	gen(nil)
	return out
}

// TestLanguage checks the automaton against the brute-force semantics on
// every lasso of length ≤ 3 over two atoms, for textbook formulas.
func TestLanguage(t *testing.T) {
	formulas := []string{
		"[]p", "<>p", "[]<>p", "<>[]p", "p U q", "p V q", "X p", "X X p", "p -> X q",
		"[](p -> <>q)", "[](p -> X q)", "<>p && <>q", "[]p || []q", "!(p U q)", "(p U q) U p",
		"[]<>p -> []<>q", "p <-> q", "<>(p && X !p)", "[](p U q)", "true", "false", "p", "!p",
	}
	lassos := allLassos([]string{"p", "q"}, 3)
	for _, s := range formulas {
		f := mustParse(t, s)
		a := Translate(f)
		neg := Translate(NotF(f))
		for _, w := range lassos {
			want := holds(f, w)[0]
			if got := accepts(a, w); got != want {
				t.Fatalf("%s on %v/%d: automaton %v, semantics %v", s, w.vals, w.loop, got, want)
			}
			if got := accepts(neg, w); got == want {
				t.Fatalf("!(%s) on %v/%d: negated automaton agrees with the formula", s, w.vals, w.loop)
			}
		}
	}
}

// TestLanguageRandom does the same for random formulas.
func TestLanguageRandom(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	atoms := []string{"p", "q"}
	var gen func(depth int) *Formula
	gen = func(depth int) *Formula {
		if depth == 0 || r.Intn(4) == 0 {
			switch r.Intn(4) {
			case 0:
				return TrueF()
			case 1:
				return AtomF("p", ir.Ref("p"))
			default:
				return AtomF("q", ir.Ref("q"))
			}
		}
		switch r.Intn(9) {
		case 0:
			return NotF(gen(depth - 1))
		case 1:
			return AndF(gen(depth-1), gen(depth-1))
		case 2:
			return OrF(gen(depth-1), gen(depth-1))
		case 3:
			return NextF(gen(depth - 1))
		case 4:
			return AlwaysF(gen(depth - 1))
		case 5:
			return EventuallyF(gen(depth - 1))
		case 6:
			return UntilF(gen(depth-1), gen(depth-1))
		case 7:
			return ReleaseF(gen(depth-1), gen(depth-1))
		default:
			return ImplF(gen(depth-1), gen(depth-1))
		}
	}
	lassos := allLassos(atoms, 3)
	for i := 0; i < 150; i++ {
		f := gen(3)
		a := Translate(f)
		for _, w := range lassos {
			want := holds(f, w)[0]
			if got := accepts(a, w); got != want {
				t.Fatalf("%s on %v/%d: automaton %v, semantics %v", f, w.vals, w.loop, got, want)
			}
		}
	}
}

// TestAutomatonSizes pins the sizes of textbook automata (init state
// included) so that a regression in the simplification is visible.
func TestAutomatonSizes(t *testing.T) {
	cases := []struct {
		f                 string
		states, accepting int
	}{
		// The init pseudo-state merges with the first tableau node whenever
		// they have the same transitions, as in SPIN's claims (T0_init
		// carries the true self-loop).
		{"[]<>p", 2, 1}, // T0_init (true loop), accept (p)
		{"<>[]p", 2, 1}, // T0_init (true loop, p → accept), accept (p loop)
		{"p U q", 2, 1}, // T0_init (p loop, q → accept), accept (true loop)
		{"X p", 3, 2},   // T0_init → T1 (true) → accept (p), accept (true loop)
		{"[]p", 2, 1},   // T0_init, accept (p loop)
		{"<>p", 2, 1},
	}
	for _, c := range cases {
		a := Translate(mustParse(t, c.f))
		if len(a.States) != c.states || a.Accepting() != c.accepting {
			var names []string
			for _, s := range a.States {
				names = append(names, s.Name)
			}
			t.Errorf("%s: %d states (%v), %d accepting; want %d / %d", c.f, len(a.States), names, a.Accepting(), c.states, c.accepting)
		}
	}
}

func TestForProperty(t *testing.T) {
	c, err := ForProperty("never:ltl1", "<>[]p", Options{Defines: map[string]string{"p": "(x < 4)"}, Scope: testScope{}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Info.Negated != "!(<>[]p)" || !c.Info.StutterInvariant || len(c.Info.Atoms) != 1 || c.Info.Atoms[0] != "x < 4" {
		t.Errorf("info: %+v", c.Info)
	}
	if !c.Process.Claim || c.Process.Locations[0].Name != "T0_init" {
		t.Errorf("process: %+v", c.Process)
	}
	acc := 0
	for _, l := range c.Process.Locations {
		for _, lb := range l.Labels {
			if lb == ir.Accept {
				acc++
			}
		}
	}
	if acc != c.Info.Accepting {
		t.Errorf("accept labels %d, info %d", acc, c.Info.Accepting)
	}
	x, err := ForProperty("n", "X p", Options{Defines: map[string]string{"p": "x"}, Scope: testScope{}})
	if err != nil {
		t.Fatal(err)
	}
	if x.Info.StutterInvariant {
		t.Error("X p flagged stutter-invariant")
	}
}
