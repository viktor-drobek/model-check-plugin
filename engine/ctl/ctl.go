// Package ctl is computation tree logic over the reachable graph of a model
// (plan 14 §4.2, step G5): syntax, normalisation to the EX / EU / EG basis,
// and the labelling algorithm of Clarke, Emerson and Sistla — time linear
// in |graph| × |formula| (notes 05 ch. 6, 03).
//
// CTL and LTL are different logics and stay apart. This package has its own
// parser, its own formula type and its own decision procedure; nothing here
// translates a CTL formula into LTL, and nothing in package ltl translates
// the other way. The report says which logic answered a property
// (`temporal.logic`), so a caller can never be sold an LTL answer to a CTL
// question — that is exactly what eval E6 asks (plan 14 §8, row E6).
//
// # Syntax
//
//	unary     AG f  AF f  AX f  EG f  EF f  EX f  ! f
//	binary    A[f U g]   E[f U g]
//	          f && g   (also /\)
//	          f || g   (also \/)
//	          f -> g   f <-> g     (right-associative)
//	atoms     true  false  identifier  (expression)  pc_value(n)  P@label
//
// An atom is a boolean state expression over the *global* state: an
// identifier (read as "non-zero", the Promela convention), a comparison or
// arithmetic over global variables and array elements, `len(ch)`,
// `empty(ch)`, `nempty(ch)`, `full(ch)`, `nfull(ch)`, `_nr_pr`,
// `pc_value(n)` (the control location of the process with index n), or
// `P@label` — process P is at the location named `label`, which is SPIN's
// remote-reference notation and the readable way to ask about control
// state. P is a process instance name (`subscriber:0`) or a proctype name,
// which then means its first instance. Symbols `#define`d in a Promela
// input are expanded before parsing, so `#define idle (x == 0)` makes
// `AG EF idle` mean `AG EF (x == 0)`.
//
// The letters A and E are operators only in the combinations above: `AG`,
// `AF`, `AX`, `EG`, `EF`, `EX`, `A[`, `E[`. A bare `A` or `E` elsewhere is a
// variable name, as in Promela.
//
// # What is *not* here
//
// Fairness. CTL under fairness constraints needs a different fixed point
// (05 ch. 6.4) and is outside the plan's scope for v1; a `ctl` property
// asked with fairness is reported not-executed with that reason, never
// answered by quietly dropping the assumption.
package ctl

import (
	"fmt"
	"regexp"
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
	EX
	EF
	EG
	EU // E[L U R]
	AX
	AF
	AG
	AU // A[L U R]
)

// Atom is an atomic proposition: a boolean expression over the global state.
type Atom struct {
	// Text is the expression as the engine reads it (ir.Expr.String()).
	Text string
	// Src is the text the user wrote, when it differs (e.g. "P@Idle").
	Src  string
	Expr *ir.Expr
}

// Formula is a syntax tree; L is the operand of the unary operators.
type Formula struct {
	Op   Op
	Atom *Atom
	L, R *Formula
}

// Constructors.

func TrueF() *Formula  { return &Formula{Op: True} }
func FalseF() *Formula { return &Formula{Op: False} }
func NotF(f *Formula) *Formula {
	return &Formula{Op: Not, L: f}
}
func AndF(a, b *Formula) *Formula { return &Formula{Op: And, L: a, R: b} }
func OrF(a, b *Formula) *Formula  { return &Formula{Op: Or, L: a, R: b} }
func Unary(op Op, f *Formula) *Formula {
	return &Formula{Op: op, L: f}
}
func Until(op Op, a, b *Formula) *Formula { return &Formula{Op: op, L: a, R: b} }

var plainIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// String renders f so that equal formulas have equal strings: the labelling
// uses the string as a subformula's identity, so it must be injective on
// syntax trees. Atoms that are not plain identifiers are parenthesised.
func (f *Formula) String() string {
	if f == nil {
		return "true"
	}
	switch f.Op {
	case True:
		return "true"
	case False:
		return "false"
	case AtomOp:
		if plainIdent.MatchString(f.Atom.Text) {
			return f.Atom.Text
		}
		return "(" + f.Atom.Text + ")"
	case Not:
		return "!" + f.L.String()
	case EX, EF, EG, AX, AF, AG:
		return opName(f.Op) + " " + f.L.String()
	case EU:
		return "E[" + f.L.String() + " U " + f.R.String() + "]"
	case AU:
		return "A[" + f.L.String() + " U " + f.R.String() + "]"
	}
	sym := map[Op]string{And: "&&", Or: "||", Impl: "->", Iff: "<->"}[f.Op]
	return "(" + f.L.String() + " " + sym + " " + f.R.String() + ")"
}

func opName(op Op) string {
	return map[Op]string{EX: "EX", EF: "EF", EG: "EG", AX: "AX", AF: "AF", AG: "AG"}[op]
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

// Env resolves the names an atom may use: variables and channels (ir.Scope),
// processes and their named control locations.
type Env interface {
	ir.Scope
	// Process returns the index of the process called name, which is either
	// an instance name as the IR spells it or a proctype name (then the
	// first instance).
	Process(name string) (int, bool)
	// Location returns the index of the location of process p named label.
	Location(p int, label string) (int, bool)
}

// ModelEnv is the Env of a model's layout.
func ModelEnv(l *ir.Layout) Env { return modelEnv{l} }

type modelEnv struct{ l *ir.Layout }

func (e modelEnv) LookupVar(name string) *ir.Var      { return e.l.Scope(-1).LookupVar(name) }
func (e modelEnv) LookupChan(name string) *ir.Channel { return e.l.Scope(-1).LookupChan(name) }
func (e modelEnv) ProcessCount() int                  { return len(e.l.Model.Processes) }

func (e modelEnv) Process(name string) (int, bool) {
	for i := range e.l.Model.Processes {
		if e.l.Model.Processes[i].Name == name {
			return i, true
		}
	}
	for i := range e.l.Model.Processes {
		n := e.l.Model.Processes[i].Name
		if k := strings.IndexByte(n, ':'); k > 0 && n[:k] == name {
			return i, true
		}
	}
	return 0, false
}

func (e modelEnv) Location(p int, label string) (int, bool) {
	if p < 0 || p >= len(e.l.Model.Processes) {
		return 0, false
	}
	for i, loc := range e.l.Model.Processes[p].Locations {
		if loc.Name == label {
			return i, true
		}
	}
	return 0, false
}

// Options configures Parse.
type Options struct {
	// Defines are object-like macros (name → body text) expanded before
	// parsing; the Promela frontend supplies its #define table.
	Defines map[string]string
	// Env, when set, resolves and type-checks every atom; a formula whose
	// atom does not resolve is rejected rather than checked against a
	// guessed meaning.
	Env Env
}

// Error is a parse or resolution error with the position in the formula.
type Error struct {
	Pos int // byte offset in the formula text, 0-based
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("CTL formula: %s (at offset %d)", e.Msg, e.Pos) }

// ---- lexer ----------------------------------------------------------------------

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tNum
	tOp
)

type token struct {
	kind tokKind
	text string
	pos  int
	num  int64
}

var multiOps = []string{"<->", "->", "&&", "||", "==", "!=", "<=", ">=", "/\\", "\\/"}

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
			if strings.ContainsRune("!()<>+-*/%[],@:", rune(c)) {
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

// reserved names are never expanded as macros and never read as variables.
func reserved(s string) bool {
	switch s {
	case "AG", "AF", "AX", "EG", "EF", "EX", "A", "E", "U", "true", "false",
		"len", "empty", "nempty", "full", "nfull", "pc_value":
		return true
	}
	return false
}

// expandDefines replaces every identifier that names a define by its body,
// recursively, with a cycle guard; the position of the use is kept.
func expandDefines(toks []token, defines map[string]string) ([]token, *Error) {
	if len(defines) == 0 {
		return toks, nil
	}
	var out []token
	var expand func(ts []token, active map[string]bool, depth int) *Error
	expand = func(ts []token, active map[string]bool, depth int) *Error {
		for _, t := range ts {
			body, ok := defines[t.text]
			if t.kind != tIdent || !ok || reserved(t.text) {
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
			bt = bt[:len(bt)-1]
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

// ---- parser ---------------------------------------------------------------------

type parser struct {
	toks  []token
	i     int
	env   Env
	atoms map[string]*Atom
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) peekN(n int) token {
	if p.i+n < len(p.toks) {
		return p.toks[p.i+n]
	}
	return p.toks[len(p.toks)-1]
}
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

// Parse reads a CTL formula.
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
	p := &parser{toks: toks, env: opt.Env, atoms: map[string]*Atom{}}
	f, err := p.formula()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, p.fail(t, "unexpected %q after the formula; CTL has no %s here (LTL operators such as [], <> and U outside A[..] / E[..] are not CTL)", t.text, t.text)
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
			return &Formula{Op: Impl, L: l, R: r}, nil
		}
		return &Formula{Op: Iff, L: l, R: r}, nil
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
	l, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.isOp("&&") {
		p.next()
		r, err := p.unary()
		if err != nil {
			return nil, err
		}
		l = AndF(l, r)
	}
	return l, nil
}

var pathOps = map[string]Op{"AG": AG, "AF": AF, "AX": AX, "EG": EG, "EF": EF, "EX": EX}

func isPathOp(s string) bool { _, ok := pathOps[s]; return ok }

func (p *parser) unary() (*Formula, *Error) {
	t := p.peek()
	switch {
	case t.kind == tIdent && isPathOp(t.text):
		p.next()
		f, err := p.unary()
		if err != nil {
			return nil, err
		}
		return Unary(pathOps[t.text], f), nil
	case (p.isIdent("A") || p.isIdent("E")) && p.peekN(1).kind == tOp && p.peekN(1).text == "[":
		quant := p.next().text
		p.next() // [
		l, err := p.formula()
		if err != nil {
			return nil, err
		}
		if !p.isIdent("U") {
			return nil, p.fail(p.peek(), "expected U inside %s[ … U … ]", quant)
		}
		p.next()
		r, err := p.formula()
		if err != nil {
			return nil, err
		}
		if !p.isOp("]") {
			return nil, p.fail(p.peek(), "expected ] to close %s[", quant)
		}
		p.next()
		if quant == "A" {
			return Until(AU, l, r), nil
		}
		return Until(EU, l, r), nil
	case p.isOp("!"):
		p.next()
		f, err := p.unary()
		if err != nil {
			return nil, err
		}
		return NotF(f), nil
	case p.isOp("("):
		open := p.next()
		f, err := p.formula()
		if err != nil {
			return nil, err
		}
		if !p.isOp(")") {
			return nil, p.fail(p.peek(), "expected ) to close the ( at offset %d", open.pos)
		}
		p.next()
		return f, nil
	case p.isOp("["), p.isOp("<"):
		return nil, p.fail(t, "%q is not a CTL operator; CTL has AG/AF/AX/EG/EF/EX and A[..U..]/E[..U..], and LTL operators such as [] and <> belong to an ltl property", t.text)
	case t.kind == tIdent || t.kind == tNum || p.isOp("-"):
		return p.atom()
	case t.kind == tEOF:
		return nil, p.fail(t, "unexpected end of formula")
	}
	return nil, p.fail(t, "unexpected %q", t.text)
}

// atom parses a comparison or arithmetic expression as a proposition, or a
// remote location reference P@label.
func (p *parser) atom() (*Formula, *Error) {
	start := p.peek()
	if start.kind == tIdent && !reserved(start.text) {
		if f, err, ok := p.remote(); ok {
			return f, err
		}
	}
	if p.isIdent("true") {
		p.next()
		return TrueF(), nil
	}
	if p.isIdent("false") {
		p.next()
		return FalseF(), nil
	}
	l, err := p.arith()
	if err != nil {
		return nil, err
	}
	e := l
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
	return p.mkAtom(start, e, "")
}

func (p *parser) mkAtom(start token, e *ir.Expr, src string) (*Formula, *Error) {
	if p.env != nil {
		if _, err := ir.Check(e, p.env); err != nil {
			return nil, p.fail(start, "atom %s: %v", e.String(), err)
		}
	}
	text := e.String()
	a, ok := p.atoms[text]
	if !ok {
		a = &Atom{Text: text, Src: src, Expr: e}
		p.atoms[text] = a
	}
	return &Formula{Op: AtomOp, Atom: a}, nil
}

// remote parses `P@label` and `P:pid@label`; ok is false when the tokens are
// not a remote reference, and the parser has not advanced.
func (p *parser) remote() (*Formula, *Error, bool) {
	save := p.i
	name := p.next().text
	if p.isOp(":") && p.peekN(1).kind == tNum && p.peekN(2).kind == tOp && p.peekN(2).text == "@" {
		p.next()
		name += ":" + p.next().text
	}
	if !p.isOp("@") {
		p.i = save
		return nil, nil, false
	}
	at := p.next()
	lbl := p.peek()
	if lbl.kind != tIdent {
		return nil, p.fail(lbl, "expected a location label after @"), true
	}
	p.next()
	src := name + "@" + lbl.text
	if p.env == nil {
		return nil, p.fail(at, "%s needs the model to resolve the process and the label", src), true
	}
	proc, ok := p.env.Process(name)
	if !ok {
		return nil, p.fail(at, "%s: no process called %s in the model", src, name), true
	}
	loc, ok := p.env.Location(proc, lbl.text)
	if !ok {
		return nil, p.fail(at, "%s: process %s has no control location labelled %s", src, name, lbl.text), true
	}
	f, err := p.mkAtom(at, ir.Binary("eq", ir.PC(proc), ir.Const(int64(loc))), src)
	return f, err, true
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
			return p.chanOp(t)
		case "pc_value":
			return p.pcValue(t)
		case "_nr_pr":
			return ir.NrPr(), nil
		case "AG", "AF", "AX", "EG", "EF", "EX", "U", "true", "false":
			return nil, p.fail(t, "%s is a CTL operator, not a variable", t.text)
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

func (p *parser) pcValue(t token) (*ir.Expr, *Error) {
	if !p.isOp("(") {
		return nil, p.fail(t, "pc_value needs a process number in parentheses")
	}
	p.next()
	n := p.next()
	if n.kind != tNum {
		return nil, p.fail(n, "pc_value needs a constant process number")
	}
	if !p.isOp(")") {
		return nil, p.fail(p.peek(), "expected ) after the process number")
	}
	p.next()
	return ir.PC(int(n.num)), nil
}

func (p *parser) chanOp(t token) (*ir.Expr, *Error) {
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
	l := ir.Len(ch.text)
	switch t.text {
	case "len":
		return l, nil
	case "empty":
		return ir.Binary("eq", l, ir.Const(0)), nil
	case "nempty":
		return ir.Binary("gt", l, ir.Const(0)), nil
	}
	if p.env == nil {
		return nil, p.fail(t, "%s(%s) needs the model to know the channel capacity", t.text, ch.text)
	}
	c := p.env.LookupChan(ch.text)
	if c == nil {
		return nil, p.fail(t, "undeclared channel %q", ch.text)
	}
	if t.text == "full" {
		return ir.Binary("eq", l, ir.Const(int64(c.Capacity))), nil
	}
	return ir.Binary("lt", l, ir.Const(int64(c.Capacity))), nil
}
