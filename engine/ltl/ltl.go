// Package ltl parses linear temporal logic in SPIN's syntax and translates
// it to a Büchi automaton that the explorer runs as a never claim (plan 14
// §4.2, step G4).
//
// Syntax (SPIN's, precedence from tightest to loosest):
//
//	unary    []f  <>f  X f  !f
//	binary   f U g   f V g            (right-associative; V is release)
//	         f && g                   (also /\)
//	         f || g                   (also \/)
//	         f -> g   f <-> g         (right-associative)
//	atoms    true  false  identifier  (expression)
//
// An atom is an identifier (a global variable, read as "non-zero" in the
// Promela convention) or a boolean expression over the global state:
// comparisons and arithmetic over variables, array elements, constants,
// `len(ch)`, `empty(ch)`, `nempty(ch)`, `full(ch)`, `nfull(ch)`. Symbols
// `#define`d in a Promela input are expanded at the token level before
// parsing (object-like macros only), so `#define p (x != 0)` makes `[]p`
// mean `[](x != 0)`. The letters U, V and X are always operators, as in
// SPIN. `&&`, `||` and `!` inside a parenthesised expression are read as
// the temporal connectives over atoms, which has the same meaning.
//
// Direction of the negation, kept explicit throughout: Translate builds
// the automaton of the formula it is given; the explorer asks for the
// automaton of the NEGATED property, so that an accepting run of the
// product is a run of the model that satisfies !(φ), i.e. a
// counterexample to φ. Info.Negated records that formula in the report.
//
// A formula that contains X is not stutter-invariant in general (plan
// §4.2); Info.StutterInvariant is false for it. Nothing in G4 depends on
// the flag: it exists so that a later partial-order reduction can refuse
// the property.
package ltl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"modelcheck/ir"
)

// Op is a formula constructor.
type Op int

const (
	True Op = iota
	False
	AtomOp
	Not
	And
	Or
	Impl
	Iff
	Next
	Always
	Eventually
	Until
	Release
)

// Atom is an atomic proposition: a boolean state expression.
type Atom struct {
	// Text is the expression as the engine reads it (ir.Expr.String()).
	Text string
	Expr *ir.Expr
}

// Formula is a syntax tree. L is the operand of unary operators.
type Formula struct {
	Op   Op
	Atom *Atom
	L, R *Formula
}

// Constructors.

func TrueF() *Formula  { return &Formula{Op: True} }
func FalseF() *Formula { return &Formula{Op: False} }
func AtomF(text string, e *ir.Expr) *Formula {
	return &Formula{Op: AtomOp, Atom: &Atom{Text: text, Expr: e}}
}
func NotF(f *Formula) *Formula        { return &Formula{Op: Not, L: f} }
func AndF(a, b *Formula) *Formula     { return &Formula{Op: And, L: a, R: b} }
func OrF(a, b *Formula) *Formula      { return &Formula{Op: Or, L: a, R: b} }
func ImplF(a, b *Formula) *Formula    { return &Formula{Op: Impl, L: a, R: b} }
func IffF(a, b *Formula) *Formula     { return &Formula{Op: Iff, L: a, R: b} }
func NextF(f *Formula) *Formula       { return &Formula{Op: Next, L: f} }
func AlwaysF(f *Formula) *Formula     { return &Formula{Op: Always, L: f} }
func EventuallyF(f *Formula) *Formula { return &Formula{Op: Eventually, L: f} }
func UntilF(a, b *Formula) *Formula   { return &Formula{Op: Until, L: a, R: b} }
func ReleaseF(a, b *Formula) *Formula { return &Formula{Op: Release, L: a, R: b} }

// String renders f in SPIN syntax with full parenthesisation of binary
// operators, so that equal formulas have equal strings (the tableau uses
// the string as the formula's identity).
func (f *Formula) String() string {
	switch f.Op {
	case True:
		return "true"
	case False:
		return "false"
	case AtomOp:
		return f.Atom.Text
	case Not:
		return "!" + f.L.String()
	case Next:
		return "X " + f.L.String()
	case Always:
		return "[]" + f.L.String()
	case Eventually:
		return "<>" + f.L.String()
	}
	sym := map[Op]string{And: "&&", Or: "||", Impl: "->", Iff: "<->", Until: "U", Release: "V"}[f.Op]
	return "(" + f.L.String() + " " + sym + " " + f.R.String() + ")"
}

// HasNext reports whether f contains the X operator.
func (f *Formula) HasNext() bool {
	if f == nil {
		return false
	}
	return f.Op == Next || f.L.HasNext() || f.R.HasNext()
}

// Atoms lists the distinct atoms of f in order of first occurrence.
func (f *Formula) Atoms() []*Atom {
	var out []*Atom
	seen := map[string]bool{}
	var walk func(g *Formula)
	walk = func(g *Formula) {
		if g == nil {
			return
		}
		if g.Op == AtomOp && !seen[g.Atom.Text] {
			seen[g.Atom.Text] = true
			out = append(out, g.Atom)
		}
		walk(g.L)
		walk(g.R)
	}
	walk(f)
	return out
}

// Options configures Parse.
type Options struct {
	// Defines are object-like macros (name → body text) expanded before
	// parsing; the Promela frontend supplies its #define table.
	Defines map[string]string
	// Scope, when set, type-checks every atom (undeclared variable or
	// channel, array read without index …) and rejects the formula.
	Scope ir.Scope
}

// Error is a parse or resolution error with the position in the formula.
type Error struct {
	Pos int // byte offset in the formula text, 0-based
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("formula: %s (at offset %d)", e.Msg, e.Pos) }

// ---- lexer ----------------------------------------------------------------------

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tNum
	tOp // punctuation and multi-char operators
)

type token struct {
	kind tokKind
	text string
	pos  int
	num  int64
}

var multiOps = []string{"<->", "[]", "<>", "->", "&&", "||", "==", "!=", "<=", ">=", "/\\", "\\/"}

func lex(src string, base int) ([]token, *Error) {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case isIdentStart(c):
			j := i
			for j < len(src) && (isIdentStart(src[j]) || isDigit(src[j])) {
				j++
			}
			toks = append(toks, token{kind: tIdent, text: src[i:j], pos: base + i})
			i = j
		case isDigit(c):
			j := i
			for j < len(src) && isDigit(src[j]) {
				j++
			}
			n, err := strconv.ParseInt(src[i:j], 10, 64)
			if err != nil {
				return nil, &Error{base + i, "number too large: " + src[i:j]}
			}
			toks = append(toks, token{kind: tNum, text: src[i:j], pos: base + i, num: n})
			i = j
		default:
			matched := false
			for _, op := range multiOps {
				if strings.HasPrefix(src[i:], op) {
					text := op
					switch op {
					case "/\\":
						text = "&&"
					case "\\/":
						text = "||"
					}
					toks = append(toks, token{kind: tOp, text: text, pos: base + i})
					i += len(op)
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			if strings.ContainsRune("!()<>+-*/%[],", rune(c)) {
				toks = append(toks, token{kind: tOp, text: string(c), pos: base + i})
				i++
				continue
			}
			return nil, &Error{base + i, fmt.Sprintf("unexpected character %q", c)}
		}
	}
	toks = append(toks, token{kind: tEOF, pos: base + len(src)})
	return toks, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// expandDefines replaces every identifier that names a define by its body
// (recursively, with a cycle guard), keeping the position of the use.
func expandDefines(toks []token, defines map[string]string) ([]token, *Error) {
	if len(defines) == 0 {
		return toks, nil
	}
	var out []token
	var expand func(ts []token, active map[string]bool, depth int) *Error
	expand = func(ts []token, active map[string]bool, depth int) *Error {
		for _, t := range ts {
			body, ok := defines[t.text]
			if t.kind != tIdent || !ok || isKeyword(t.text) {
				out = append(out, t)
				continue
			}
			if active[t.text] || depth > 32 {
				return &Error{t.pos, "recursive #define " + t.text}
			}
			bt, err := lex(body, t.pos)
			if err != nil {
				return &Error{t.pos, fmt.Sprintf("in the body of #define %s: %s", t.text, err.Msg)}
			}
			bt = bt[:len(bt)-1] // drop EOF
			for i := range bt {
				bt[i].pos = t.pos
			}
			active[t.text] = true
			if err := expand(bt, active, depth+1); err != nil {
				return err
			}
			delete(active, t.text)
		}
		return nil
	}
	if err := expand(toks, map[string]bool{}, 0); err != nil {
		return nil, err
	}
	return out, nil
}

func isKeyword(s string) bool {
	switch s {
	case "U", "V", "X", "true", "false", "len", "empty", "nempty", "full", "nfull":
		return true
	}
	return false
}

// ---- parser ---------------------------------------------------------------------

type parser struct {
	toks  []token
	i     int
	scope ir.Scope
	atoms map[string]*Atom
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) next() token { t := p.toks[p.i]; p.i++; return t }
func (p *parser) isOp(s string) bool {
	t := p.peek()
	return t.kind == tOp && t.text == s
}
func (p *parser) isIdent(s string) bool {
	t := p.peek()
	return t.kind == tIdent && t.text == s
}

func (p *parser) fail(t token, format string, args ...any) *Error {
	return &Error{t.pos, fmt.Sprintf(format, args...)}
}

// Parse reads a formula in SPIN syntax.
func Parse(text string, opt Options) (*Formula, error) {
	if strings.TrimSpace(text) == "" {
		return nil, &Error{0, "empty formula"}
	}
	toks, err := lex(text, 0)
	if err != nil {
		return nil, err
	}
	toks, err = expandDefines(toks, opt.Defines)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, scope: opt.Scope, atoms: map[string]*Atom{}}
	f, err := p.formula()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, p.fail(t, "unexpected %q after the formula", t.text)
	}
	return f, nil
}

func (p *parser) formula() (*Formula, *Error) {
	l, err := p.disjunction()
	if err != nil {
		return nil, err
	}
	if p.isOp("->") || p.isOp("<->") {
		op := p.next().text
		r, err := p.formula()
		if err != nil {
			return nil, err
		}
		if op == "->" {
			return ImplF(l, r), nil
		}
		return IffF(l, r), nil
	}
	return l, nil
}

func (p *parser) disjunction() (*Formula, *Error) {
	l, err := p.conjunction()
	if err != nil {
		return nil, err
	}
	for p.isOp("||") {
		p.next()
		r, err := p.conjunction()
		if err != nil {
			return nil, err
		}
		l = OrF(l, r)
	}
	return l, nil
}

func (p *parser) conjunction() (*Formula, *Error) {
	l, err := p.until()
	if err != nil {
		return nil, err
	}
	for p.isOp("&&") {
		p.next()
		r, err := p.until()
		if err != nil {
			return nil, err
		}
		l = AndF(l, r)
	}
	return l, nil
}

func (p *parser) until() (*Formula, *Error) {
	l, err := p.unary()
	if err != nil {
		return nil, err
	}
	if p.isIdent("U") || p.isIdent("V") {
		op := p.next().text
		r, err := p.until()
		if err != nil {
			return nil, err
		}
		if op == "U" {
			return UntilF(l, r), nil
		}
		return ReleaseF(l, r), nil
	}
	return l, nil
}

func (p *parser) unary() (*Formula, *Error) {
	t := p.peek()
	switch {
	case p.isOp("[]"):
		p.next()
		f, err := p.unary()
		return AlwaysF(f), err
	case p.isOp("<>"):
		p.next()
		f, err := p.unary()
		return EventuallyF(f), err
	case p.isIdent("X"):
		p.next()
		f, err := p.unary()
		return NextF(f), err
	case p.isOp("!"):
		p.next()
		f, err := p.unary()
		return NotF(f), err
	case p.isOp("("):
		p.next()
		f, err := p.formula()
		if err != nil {
			return nil, err
		}
		if !p.isOp(")") {
			return nil, p.fail(p.peek(), "expected ) to close the ( at offset %d", t.pos)
		}
		p.next()
		return f, nil
	case p.isIdent("true"):
		p.next()
		return TrueF(), nil
	case p.isIdent("false"):
		p.next()
		return FalseF(), nil
	case t.kind == tIdent || t.kind == tNum || p.isOp("-"):
		return p.atom()
	case t.kind == tEOF:
		return nil, p.fail(t, "unexpected end of formula")
	}
	return nil, p.fail(t, "unexpected %q", t.text)
}

// atom parses a comparison or an arithmetic expression as a proposition.
func (p *parser) atom() (*Formula, *Error) {
	start := p.peek()
	l, err := p.arith()
	if err != nil {
		return nil, err
	}
	var e *ir.Expr = l
	if t := p.peek(); t.kind == tOp {
		op := map[string]string{"==": "eq", "!=": "ne", "<": "lt", "<=": "le", ">": "gt", ">=": "ge"}[t.text]
		if op != "" {
			p.next()
			r, err := p.arith()
			if err != nil {
				return nil, err
			}
			e = ir.Binary(op, l, r)
		}
	}
	if p.scope != nil {
		if _, err := ir.Check(e, p.scope); err != nil {
			return nil, p.fail(start, "atom %s: %v", e.String(), err)
		}
	}
	text := e.String()
	a, ok := p.atoms[text]
	if !ok {
		a = &Atom{Text: text, Expr: e}
		p.atoms[text] = a
	}
	return &Formula{Op: AtomOp, Atom: a}, nil
}

func (p *parser) arith() (*ir.Expr, *Error) {
	l, err := p.term()
	if err != nil {
		return nil, err
	}
	for p.isOp("+") || p.isOp("-") {
		op := "add"
		if p.next().text == "-" {
			op = "sub"
		}
		r, err := p.term()
		if err != nil {
			return nil, err
		}
		l = ir.Binary(op, l, r)
	}
	return l, nil
}

func (p *parser) term() (*ir.Expr, *Error) {
	l, err := p.factor()
	if err != nil {
		return nil, err
	}
	for p.isOp("*") || p.isOp("/") || p.isOp("%") {
		op := map[string]string{"*": "mul", "/": "div", "%": "mod"}[p.next().text]
		r, err := p.factor()
		if err != nil {
			return nil, err
		}
		l = ir.Binary(op, l, r)
	}
	return l, nil
}

func (p *parser) factor() (*ir.Expr, *Error) {
	t := p.next()
	switch {
	case t.kind == tNum:
		return ir.Const(t.num), nil
	case t.kind == tOp && t.text == "-":
		f, err := p.factor()
		if err != nil {
			return nil, err
		}
		return ir.Unary("neg", f), nil
	case t.kind == tOp && t.text == "(":
		e, err := p.arith()
		if err != nil {
			return nil, err
		}
		if !p.isOp(")") {
			return nil, p.fail(p.peek(), "expected ) in the expression")
		}
		p.next()
		return e, nil
	case t.kind == tIdent:
		switch t.text {
		case "len", "empty", "nempty", "full", "nfull":
			if !p.isOp("(") {
				return nil, p.fail(t, "%s needs a channel in parentheses", t.text)
			}
			p.next()
			ch := p.next()
			if ch.kind != tIdent {
				return nil, p.fail(ch, "%s needs a channel name", t.text)
			}
			if !p.isOp(")") {
				return nil, p.fail(p.peek(), "expected ) after the channel name")
			}
			p.next()
			return p.chanOp(t, ch.text)
		case "U", "V", "X", "true", "false":
			return nil, p.fail(t, "%s is an operator, not a variable", t.text)
		}
		if p.isOp("[") {
			p.next()
			idx, err := p.arith()
			if err != nil {
				return nil, err
			}
			if !p.isOp("]") {
				return nil, p.fail(p.peek(), "expected ] after the array index")
			}
			p.next()
			return ir.Index(t.text, idx), nil
		}
		return ir.Ref(t.text), nil
	case t.kind == tEOF:
		return nil, p.fail(t, "unexpected end of formula")
	}
	return nil, p.fail(t, "unexpected %q", t.text)
}

func (p *parser) chanOp(t token, ch string) (*ir.Expr, *Error) {
	l := ir.Len(ch)
	switch t.text {
	case "len":
		return l, nil
	case "empty":
		return ir.Binary("eq", l, ir.Const(0)), nil
	case "nempty":
		return ir.Binary("gt", l, ir.Const(0)), nil
	}
	if p.scope == nil {
		return nil, p.fail(t, "%s(%s) needs the model to know the channel capacity", t.text, ch)
	}
	c := p.scope.LookupChan(ch)
	if c == nil {
		return nil, p.fail(t, "undeclared channel %q", ch)
	}
	if t.text == "full" {
		return ir.Binary("eq", l, ir.Const(int64(c.Capacity))), nil
	}
	return ir.Binary("lt", l, ir.Const(int64(c.Capacity))), nil
}

// ---- negation normal form ---------------------------------------------------------

// NNF rewrites f so that negation applies to atoms only, `->` and `<->` are
// eliminated, and [] / <> are kept (the tableau reads them as false V f
// and true U f).
func NNF(f *Formula) *Formula {
	switch f.Op {
	case True, False, AtomOp:
		return f
	case Not:
		return negate(f.L)
	case And:
		return AndF(NNF(f.L), NNF(f.R))
	case Or:
		return OrF(NNF(f.L), NNF(f.R))
	case Impl:
		return OrF(negate(f.L), NNF(f.R))
	case Iff:
		return OrF(AndF(NNF(f.L), NNF(f.R)), AndF(negate(f.L), negate(f.R)))
	case Next:
		return NextF(NNF(f.L))
	case Always:
		return AlwaysF(NNF(f.L))
	case Eventually:
		return EventuallyF(NNF(f.L))
	case Until:
		return UntilF(NNF(f.L), NNF(f.R))
	case Release:
		return ReleaseF(NNF(f.L), NNF(f.R))
	}
	panic("ltl: unknown op")
}

// negate returns NNF(!f).
func negate(f *Formula) *Formula {
	switch f.Op {
	case True:
		return FalseF()
	case False:
		return TrueF()
	case AtomOp:
		return NotF(f)
	case Not:
		return NNF(f.L)
	case And:
		return OrF(negate(f.L), negate(f.R))
	case Or:
		return AndF(negate(f.L), negate(f.R))
	case Impl:
		return AndF(NNF(f.L), negate(f.R))
	case Iff:
		return OrF(AndF(NNF(f.L), negate(f.R)), AndF(negate(f.L), NNF(f.R)))
	case Next:
		return NextF(negate(f.L))
	case Always:
		return EventuallyF(negate(f.L))
	case Eventually:
		return AlwaysF(negate(f.L))
	case Until:
		return ReleaseF(negate(f.L), negate(f.R))
	case Release:
		return UntilF(negate(f.L), negate(f.R))
	}
	panic("ltl: unknown op")
}

// sortedKeys is the deterministic iteration order over formula sets.
func sortedKeys(m map[string]*Formula) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
