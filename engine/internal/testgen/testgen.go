// Package testgen builds the inputs of the resource-limit tests: Promela
// models and formulas whose size amplifies in the frontend (nesting, chains,
// macro, inline and typedef fan-out, normal-form blow-up). The tests and the
// godog steps generate them instead of keeping megabyte fixtures in the tree.
// The package is used by tests only; it has no dependency on the engine.
package testgen

import (
	"fmt"
	"strings"
)

// Model wraps an expression statement in the smallest model that parses.
func Model(stmt string) string {
	return "byte x;\nactive proctype P() { " + stmt + " }\n"
}

// NestedParens is `x = ((((1))))` with n parentheses.
func NestedParens(n int) string {
	return Model("x = " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n))
}

// PlusChain is `x = 1+1+…+1` with n terms: a left-deep tree of height n.
func PlusChain(n int) string {
	return Model("x = " + chain("1", "+", n))
}

// ParenChain is p nested groups, each adding k terms to the previous group:
// `(((x+1+…)+1+…)+…)`. Neither the nesting nor one chain is deep, but the
// height of the whole tree is about p·k.
func ParenChain(p, k int) string {
	e := "x"
	for i := 0; i < p; i++ {
		e = "(" + e + strings.Repeat("+1", k) + ")"
	}
	return Model("x = " + e)
}

// WideMixed is `x == a*b + a*b + … && x == a*b + …` with n products on each
// side: wide and of mixed precedence, but of height about n+3.
func WideMixed(n int) string {
	side := chain("x*x", "+", n)
	return Model("(x == " + side + " && x == " + side + ") -> x = 1")
}

// NestedIf is n nested `if :: true -> … fi` blocks.
func NestedIf(n int) string {
	return Model(strings.Repeat("if :: true -> ", n) + "x = 1" + strings.Repeat(" fi", n))
}

// IfParens is a model whose `#if` expression nests n parentheses.
func IfParens(n int) string {
	return "#if " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) + "\n#endif\n" + Model("x = 1")
}

// DefineDoubling defines a_0 … a_n, each doubling the previous one, and uses
// a_n: the expansion has 2^n leaves.
func DefineDoubling(n int) string {
	var b strings.Builder
	b.WriteString("#define a0 (x || x)\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "#define a%d (a%d || a%d)\n", i, i-1, i-1)
	}
	b.WriteString(Model(fmt.Sprintf("(a%d) -> x = 1", n)))
	return b.String()
}

// UnusedDefines is DefineDoubling without the use: the model itself expands
// nothing, a formula naming a_n does.
func UnusedDefines(n int) string {
	var b strings.Builder
	b.WriteString("#define a0 (x == 1 || x == 1)\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "#define a%d (a%d || a%d)\n", i, i-1, i-1)
	}
	b.WriteString(Model("x = 1"))
	return b.String()
}

// DefineChain defines A_0 … A_n with A_i = A_{i+1} and A_n = 1, and uses A_0:
// one use rescans n levels deep.
func DefineChain(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "#define A%d A%d\n", i, i+1)
	}
	fmt.Fprintf(&b, "#define A%d 1\n", n)
	b.WriteString(Model("x = A0"))
	return b.String()
}

// DefineFanout is `#define F(a) a+a+…+a` (reps times) applied to an argument
// of argTerms tokens: the expansion is reps·argTerms tokens without any
// nesting.
func DefineFanout(reps, argTerms int) string {
	return "#define F(a) " + chain("a", "+", reps) + "\n" + Model("x = F(("+chain("1", "+", argTerms)+"))")
}

// InlineDoubling defines f_0 … f_n where f_i calls f_{i-1} twice, and calls
// f_n: 2^n copies of the body.
func InlineDoubling(n int) string {
	var b strings.Builder
	b.WriteString("byte x;\ninline f0() { x = 1 }\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "inline f%d() { f%d(); f%d() }\n", i, i-1, i-1)
	}
	fmt.Fprintf(&b, "active proctype P() { f%d() }\n", n)
	return b.String()
}

// TypedefFanout nests levels typedefs, each with two fields of the previous
// one (2^levels leaf fields), and declares instances of the outermost.
func TypedefFanout(levels, instances int) string {
	var b strings.Builder
	b.WriteString("typedef T0 { byte a; byte b }\n")
	for i := 1; i <= levels; i++ {
		fmt.Fprintf(&b, "typedef T%d { T%d a; T%d b }\n", i, i-1, i-1)
	}
	for i := 0; i < instances; i++ {
		fmt.Fprintf(&b, "T%d v%d;\n", levels, i)
	}
	fmt.Fprintf(&b, "active proctype P() { v0%s = 1 }\n", strings.Repeat(".a", levels+1))
	return b.String()
}

// PrefixChain is `x = !-!-…1` with n prefix operators (`!!` would lex as
// the sorted send, so the two alternate).
func PrefixChain(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			b.WriteByte('!')
		} else {
			b.WriteByte('-')
		}
	}
	return Model("x = " + b.String() + "1")
}

// InlineFanout is one inline whose body uses its parameter reps times,
// called once with an argument of argTerms terms: reps·argTerms tokens from
// a single call, without any nesting.
func InlineFanout(reps, argTerms int) string {
	return "byte x;\ninline F(a) { x = " + chain("a", "+", reps) + " }\nactive proctype P() { F((" + chain("1", "+", argTerms) + ")) }\n"
}

// IndexNest is `a[a[a[…0…]]]` with n index levels.
func IndexNest(n int) string {
	return "byte x; byte a[4];\nactive proctype P() { x = " + strings.Repeat("a[", n) + "0" + strings.Repeat("]", n) + " }\n"
}

// PCValueNest is `pc_value(pc_value(…0…))` with n levels.
func PCValueNest(n int) string {
	return Model("x = " + strings.Repeat("pc_value(", n) + "0" + strings.Repeat(")", n))
}

// FormulaIndexNest is the atom `a[a[…0…]] > 0` with n index levels.
func FormulaIndexNest(n int) string {
	return strings.Repeat("a[", n) + "0" + strings.Repeat("]", n) + " > 0"
}

// ManyArraysInit declares n arrays of the largest allowed length, each
// with an initialiser (one assignment per element in the lowering).
func ManyArraysInit(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "int a%d[65535] = 1;\n", i)
	}
	b.WriteString("active proctype P() { a0[0] = 1 }\n")
	return b.String()
}

// RunLocalsInit is a proctype created by run with n initialised arrays as
// locals: their initialisers belong to the run step, element by element,
// and the process has a pool of instances.
func RunLocalsInit(n int) string {
	var b strings.Builder
	b.WriteString("proctype P() {\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  int a%d[65535] = 1;\n", i)
	}
	b.WriteString("  a0[0] = 2\n}\ninit { run P() }\n")
	return b.String()
}

// ChanArrays declares n arrays of 65535 channels each.
func ChanArrays(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "chan q%d[65535] = [0] of { byte };\n", i)
	}
	b.WriteString("active proctype P() { q0[0]!1 }\n")
	return b.String()
}

// WideChans declares n channels of capacity 255 whose message has fields
// integers.
func WideChans(n, fields int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "chan c%d = [255] of { %s };\n", i, chain("int", ",", fields))
	}
	b.WriteString("active proctype P() { skip }\n")
	return b.String()
}

// ActiveLongBody is `active [n] proctype` whose body has stmts statements:
// the lowering would copy the body per instance.
func ActiveLongBody(n, stmts int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "byte x;\nactive [%d] proctype P() {\n", n)
	for i := 0; i < stmts; i++ {
		b.WriteString("  x = 1;\n")
	}
	b.WriteString("  x = 0\n}\n")
	return b.String()
}

// RunInitLocals is a proctype with two initialised arrays as locals (just
// under the slot limit for a pool of eight), started n times by run: the
// instances multiply the locals.
func RunInitLocals(n int) string {
	var b strings.Builder
	b.WriteString("proctype P() { int a[65535] = 1; int b[60000] = 1; a[0] = b[0] }\ninit {\n")
	for i := 0; i < n; i++ {
		b.WriteString("  run P();\n")
	}
	b.WriteString("  skip\n}\n")
	return b.String()
}

// TypedefChain is n typedefs each wrapping the previous one in one field:
// one field per definition, but a flattened name that grows with the chain.
func TypedefChain(n int) string {
	var b strings.Builder
	b.WriteString("typedef T0 { byte a }\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "typedef T%d { T%d a }\n", i, i-1)
	}
	fmt.Fprintf(&b, "T%d v;\nactive proctype P() { skip }\n", n)
	return b.String()
}

// WideZeroChans is an array of n rendezvous channels whose message has
// fields integers: no buffer, but a field table per channel object.
func WideZeroChans(n, fields int) string {
	return fmt.Sprintf("chan q[%d] = [0] of { %s };\nactive proctype P() { skip }\n", n, chain("int", ",", fields))
}

// DefineWrappers is DefineDoubling(levels) reached through wrappers
// single-token `#define`s: the expansion is within the budget, the rescans
// are deep, and a rescan that copies its result per level is quadratic.
func DefineWrappers(levels, wrappers int) string {
	var b strings.Builder
	b.WriteString("#define a0 (x || x)\n")
	for i := 1; i <= levels; i++ {
		fmt.Fprintf(&b, "#define a%d (a%d || a%d)\n", i, i-1, i-1)
	}
	fmt.Fprintf(&b, "#define W0 a%d\n", levels)
	for i := 1; i <= wrappers; i++ {
		fmt.Fprintf(&b, "#define W%d W%d\n", i, i-1)
	}
	b.WriteString(Model(fmt.Sprintf("(W%d) -> x = 1", wrappers)))
	return b.String()
}

// HugeArrayInit declares an array far beyond the IR limit with an
// initialiser, so that the elements would be built before the limit is seen.
func HugeArrayInit() string {
	return "byte a[1000000000] = 1;\nactive proctype P() { a[0] = 1 }\n"
}

// ManyArrays declares n arrays of the largest allowed length each.
func ManyArrays(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "int a%d[65535];\n", i)
	}
	b.WriteString("active proctype P() { a0[0] = 1 }\n")
	return b.String()
}

// Chains is a model with n statements, each a `+` chain of the given height:
// every one within the height limit, together a heavy IR.
func Chains(n, height int) string {
	var b strings.Builder
	b.WriteString("byte x;\nactive proctype P() {\n")
	for i := 0; i < n; i++ {
		b.WriteString("  x = " + chain("1", "+", height) + ";\n")
	}
	b.WriteString("  x = 0\n}\n")
	return b.String()
}

// Formulas ------------------------------------------------------------------

// FormulaParens wraps the atom `x == 1` in n parentheses.
func FormulaParens(n int) string {
	return strings.Repeat("(", n) + "x == 1" + strings.Repeat(")", n)
}

// IffChain is `x == 1 <-> (x == 1 <-> (…))` with n `<->` operators.
func IffChain(n int) string {
	f := "x == 1"
	for i := 0; i < n; i++ {
		f = "x == 1 <-> (" + f + ")"
	}
	return f
}

// AndChain is `x == 1 && x == 1 && …` with n conjuncts.
func AndChain(n int) string { return chain("x == 1", "&&", n) }

// ImplChain is `x == 1 -> x == 1 -> …` with n implications (right-recursive).
func ImplChain(n int) string { return chain("x == 1", "->", n+1) }

// UntilChain is `x == 1 U x == 1 U …` with n U operators (right-recursive).
func UntilChain(n int) string { return chain("x == 1", "U", n+1) }

// NotChain is `!!!…(x == 1)` with n negations.
func NotChain(n int) string { return strings.Repeat("!", n) + "(x == 1)" }

// AUNest is n nested `A[ atom U … ]` with the given atom; the innermost
// right side is the atom.
func AUNest(n int, atom string) string {
	f := atom
	for i := 0; i < n; i++ {
		f = "A[ " + atom + " U (" + f + ") ]"
	}
	return f
}

// LongAtom is `x+1+1+…+1 > 0` with n additions: one atom whose text is long.
func LongAtom(n int) string { return chain("x", "+", 1) + strings.Repeat("+1", n) + " > 0" }

// chain joins n copies of term with op.
func chain(term, op string, n int) string {
	if n <= 0 {
		return term
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = term
	}
	return strings.Join(parts, " "+op+" ")
}
