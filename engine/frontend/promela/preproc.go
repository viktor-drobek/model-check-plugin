package promela

import (
	"strings"
)

// Macro is a #define.
type Macro struct {
	Name   string
	Params []string
	Func   bool // function-like: NAME( immediately after the name
	Body   []Token
	Line   int
}

type cond struct {
	active     bool // this branch is taken
	parentLive bool // the enclosing region is live
	taken      bool // some branch of this #if has been taken
	seenElse   bool
	line       int
}

type preprocessor struct {
	file   string
	macros map[string]*Macro
	conds  []cond
	out    []Token
}

// Preprocess expands the C-like preprocessor over toks: #define (object-
// and function-like, with '\' continuation), #undef, #ifdef, #ifndef,
// #if <constant expression>, #elif, #else, #endif, and -D symbols given as
// "NAME" (value 1) or "NAME=value". #include is outside the subset. Tokens
// produced by an expansion carry the line of the expansion site, as cpp
// and therefore SPIN's line numbers do.
func Preprocess(toks []Token, defines []string, file string) ([]Token, *Error) {
	pp := &preprocessor{file: file, macros: map[string]*Macro{}}
	for _, d := range defines {
		name, val := d, "1"
		if i := strings.IndexByte(d, '='); i >= 0 {
			name, val = d[:i], d[i+1:]
		}
		body, err := Lex(val, "-D "+name)
		if err != nil {
			return nil, syntaxErr(file, 0, 0, "-D %s: %s", d, err.Message)
		}
		body = body[:len(body)-1] // drop EOF
		for i := range body {
			body[i].Line, body[i].Col = 0, 0
		}
		pp.macros[name] = &Macro{Name: name, Body: body}
	}
	i := 0
	for i < len(toks) {
		t := toks[i]
		if t.Kind == Directive {
			if err := pp.directive(t); err != nil {
				return nil, err
			}
			i++
			continue
		}
		if t.Kind == EOF {
			if len(pp.conds) > 0 {
				return nil, syntaxErr(file, pp.conds[len(pp.conds)-1].line, 1, "#if without #endif")
			}
			pp.out = append(pp.out, t)
			break
		}
		if !pp.live() {
			i++
			continue
		}
		n, expanded, err := pp.expand(toks, i, nil)
		if err != nil {
			return nil, err
		}
		pp.out = append(pp.out, expanded...)
		i = n
	}
	return pp.out, nil
}

func (pp *preprocessor) live() bool {
	return len(pp.conds) == 0 || pp.conds[len(pp.conds)-1].active
}

func (pp *preprocessor) directive(d Token) *Error {
	switch d.Text {
	case "ifdef", "ifndef":
		if len(d.Args) != 1 || d.Args[0].Kind != Ident {
			return syntaxErr(pp.file, d.Line, d.Col, "#%s needs one identifier", d.Text)
		}
		_, defined := pp.macros[d.Args[0].Text]
		pp.push(d, (defined && d.Text == "ifdef") || (!defined && d.Text == "ifndef"))
	case "if":
		v, err := pp.evalIf(d)
		if err != nil {
			return err
		}
		pp.push(d, v != 0)
	case "elif":
		if len(pp.conds) == 0 {
			return syntaxErr(pp.file, d.Line, d.Col, "#elif without #if")
		}
		c := &pp.conds[len(pp.conds)-1]
		if c.seenElse {
			return syntaxErr(pp.file, d.Line, d.Col, "#elif after #else")
		}
		if c.taken || !c.parentLive {
			c.active = false
			return nil
		}
		v, err := pp.evalIf(d)
		if err != nil {
			return err
		}
		c.active = v != 0
		c.taken = c.active
	case "else":
		if len(pp.conds) == 0 {
			return syntaxErr(pp.file, d.Line, d.Col, "#else without #if")
		}
		c := &pp.conds[len(pp.conds)-1]
		if c.seenElse {
			return syntaxErr(pp.file, d.Line, d.Col, "second #else")
		}
		c.seenElse = true
		c.active = c.parentLive && !c.taken
		c.taken = true
	case "endif":
		if len(pp.conds) == 0 {
			return syntaxErr(pp.file, d.Line, d.Col, "#endif without #if")
		}
		pp.conds = pp.conds[:len(pp.conds)-1]
	case "define":
		if !pp.live() {
			return nil
		}
		return pp.define(d)
	case "undef":
		if !pp.live() {
			return nil
		}
		if len(d.Args) != 1 || d.Args[0].Kind != Ident {
			return syntaxErr(pp.file, d.Line, d.Col, "#undef needs one identifier")
		}
		delete(pp.macros, d.Args[0].Text)
	case "include":
		if !pp.live() {
			return nil
		}
		return outside(pp.file, d.Line, d.Col, "#include", "the model must be a single file")
	default:
		if !pp.live() {
			return nil
		}
		return syntaxErr(pp.file, d.Line, d.Col, "unknown preprocessor directive #%s", d.Text)
	}
	return nil
}

func (pp *preprocessor) push(d Token, v bool) {
	parent := pp.live()
	pp.conds = append(pp.conds, cond{active: parent && v, parentLive: parent, taken: v, line: d.Line})
}

func (pp *preprocessor) define(d Token) *Error {
	if len(d.Args) == 0 || d.Args[0].Kind != Ident {
		return syntaxErr(pp.file, d.Line, d.Col, "#define needs a name")
	}
	name := d.Args[0]
	m := &Macro{Name: name.Text, Line: d.Line}
	rest := d.Args[1:]
	if len(rest) > 0 && rest[0].is("(") && rest[0].Line == name.Line && rest[0].Col == name.Col+len(name.Text) {
		m.Func = true
		i := 1
		for i < len(rest) && !rest[i].is(")") {
			if rest[i].Kind != Ident {
				return syntaxErr(pp.file, rest[i].Line, rest[i].Col, "macro parameter expected, got %s", rest[i])
			}
			m.Params = append(m.Params, rest[i].Text)
			i++
			if i < len(rest) && rest[i].is(",") {
				i++
			}
		}
		if i >= len(rest) {
			return syntaxErr(pp.file, d.Line, d.Col, "unterminated macro parameter list")
		}
		rest = rest[i+1:]
	}
	m.Body = append([]Token(nil), rest...)
	pp.macros[m.Name] = m
	return nil
}

// expand handles toks[i]: if it is a macro invocation it returns the
// expansion (recursively expanded, with the invocation's line) and the
// index after it; otherwise the token itself. disabled guards recursion.
func (pp *preprocessor) expand(toks []Token, i int, disabled map[string]bool) (int, []Token, *Error) {
	t := toks[i]
	m := (*Macro)(nil)
	if t.Kind == Ident && !disabled[t.Text] {
		m = pp.macros[t.Text]
	}
	if m == nil {
		return i + 1, []Token{t}, nil
	}
	var body []Token
	next := i + 1
	if m.Func {
		if next >= len(toks) || !toks[next].is("(") {
			// A function-like macro name without arguments is an ordinary
			// identifier (cpp semantics).
			return i + 1, []Token{t}, nil
		}
		args, n, err := pp.collectArgs(toks, next, t)
		if err != nil {
			return 0, nil, err
		}
		if len(args) != len(m.Params) && !(len(m.Params) == 0 && len(args) == 1 && len(args[0]) == 0) {
			return 0, nil, syntaxErr(pp.file, t.Line, t.Col, "macro %s takes %d argument(s), got %d", m.Name, len(m.Params), len(args))
		}
		next = n
		for _, bt := range m.Body {
			if bt.Kind == Ident {
				if k := indexOf(m.Params, bt.Text); k >= 0 {
					body = append(body, args[k]...)
					continue
				}
			}
			body = append(body, bt)
		}
	} else {
		body = append(body, m.Body...)
	}
	for k := range body {
		body[k].Line, body[k].Col = t.Line, t.Col
	}
	// Rescan the expansion with this macro disabled.
	dis := map[string]bool{m.Name: true}
	for k := range disabled {
		dis[k] = true
	}
	var out []Token
	for k := 0; k < len(body); {
		n, ex, err := pp.expand(body, k, dis)
		if err != nil {
			return 0, nil, err
		}
		out = append(out, ex...)
		k = n
	}
	return next, out, nil
}

// collectArgs reads "( a, b, … )" starting at toks[open]; arguments are
// comma-separated at nesting depth 0.
func (pp *preprocessor) collectArgs(toks []Token, open int, at Token) ([][]Token, int, *Error) {
	depth := 0
	var args [][]Token
	var cur []Token
	for i := open; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.Kind == EOF || t.Kind == Directive:
			return nil, 0, syntaxErr(pp.file, at.Line, at.Col, "unterminated macro arguments")
		case t.is("("):
			depth++
			if depth > 1 {
				cur = append(cur, t)
			}
		case t.is(")"):
			depth--
			if depth == 0 {
				args = append(args, cur)
				return args, i + 1, nil
			}
			cur = append(cur, t)
		case t.is(",") && depth == 1:
			args = append(args, cur)
			cur = nil
		default:
			cur = append(cur, t)
		}
	}
	return nil, 0, syntaxErr(pp.file, at.Line, at.Col, "unterminated macro arguments")
}

func indexOf(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}

// evalIf evaluates the constant expression of #if / #elif after macro
// expansion: integers, defined(X), ! - ( ), * / %, + -, comparisons, &&,
// ||. Undefined identifiers are 0, as in cpp.
func (pp *preprocessor) evalIf(d Token) (int64, *Error) {
	// Expand macros, keeping defined(...) intact.
	var toks []Token
	for i := 0; i < len(d.Args); {
		t := d.Args[i]
		if t.isIdent("defined") {
			j := i + 1
			paren := j < len(d.Args) && d.Args[j].is("(")
			if paren {
				j++
			}
			if j >= len(d.Args) || d.Args[j].Kind != Ident {
				return 0, syntaxErr(pp.file, d.Line, d.Col, "defined needs an identifier")
			}
			_, ok := pp.macros[d.Args[j].Text]
			v := int64(0)
			if ok {
				v = 1
			}
			toks = append(toks, Token{Kind: Number, Text: "1", Val: v, Line: t.Line, Col: t.Col})
			j++
			if paren {
				if j >= len(d.Args) || !d.Args[j].is(")") {
					return 0, syntaxErr(pp.file, d.Line, d.Col, "defined( without )")
				}
				j++
			}
			i = j
			continue
		}
		n, ex, err := pp.expand(d.Args, i, nil)
		if err != nil {
			return 0, err
		}
		toks = append(toks, ex...)
		i = n
	}
	toks = append(toks, Token{Kind: EOF, Line: d.Line, Col: d.Col})
	ev := &ifEval{toks: toks, file: pp.file, d: d}
	v := ev.or()
	if ev.err != nil {
		return 0, ev.err
	}
	if ev.toks[ev.i].Kind != EOF {
		return 0, syntaxErr(pp.file, d.Line, d.Col, "unexpected %s in #%s expression", ev.toks[ev.i], d.Text)
	}
	return v, nil
}

type ifEval struct {
	toks []Token
	i    int
	file string
	d    Token
	err  *Error
}

func (e *ifEval) peek() Token { return e.toks[e.i] }
func (e *ifEval) next() Token { t := e.toks[e.i]; e.i++; return t }

func (e *ifEval) or() int64 {
	v := e.and()
	for e.peek().is("||") {
		e.next()
		w := e.and()
		v = b2i(v != 0 || w != 0)
	}
	return v
}

func (e *ifEval) and() int64 {
	v := e.cmp()
	for e.peek().is("&&") {
		e.next()
		w := e.cmp()
		v = b2i(v != 0 && w != 0)
	}
	return v
}

func (e *ifEval) cmp() int64 {
	v := e.add()
	for {
		t := e.peek()
		if t.Kind != Punct {
			return v
		}
		switch t.Text {
		case "==", "!=", "<", "<=", ">", ">=":
			e.next()
			w := e.add()
			switch t.Text {
			case "==":
				v = b2i(v == w)
			case "!=":
				v = b2i(v != w)
			case "<":
				v = b2i(v < w)
			case "<=":
				v = b2i(v <= w)
			case ">":
				v = b2i(v > w)
			case ">=":
				v = b2i(v >= w)
			}
		default:
			return v
		}
	}
}

func (e *ifEval) add() int64 {
	v := e.mul()
	for e.peek().is("+") || e.peek().is("-") {
		op := e.next()
		w := e.mul()
		if op.Text == "+" {
			v += w
		} else {
			v -= w
		}
	}
	return v
}

func (e *ifEval) mul() int64 {
	v := e.unary()
	for e.peek().is("*") || e.peek().is("/") || e.peek().is("%") {
		op := e.next()
		w := e.unary()
		switch {
		case op.Text == "*":
			v *= w
		case w == 0:
			e.fail("division by zero in #%s", e.d.Text)
		case op.Text == "/":
			v /= w
		default:
			v %= w
		}
	}
	return v
}

func (e *ifEval) unary() int64 {
	t := e.peek()
	switch {
	case t.is("!"):
		e.next()
		return b2i(e.unary() == 0)
	case t.is("-"):
		e.next()
		return -e.unary()
	case t.is("("):
		e.next()
		v := e.or()
		if !e.peek().is(")") {
			e.fail("missing ) in #%s expression", e.d.Text)
			return 0
		}
		e.next()
		return v
	case t.Kind == Number:
		e.next()
		return t.Val
	case t.Kind == Ident:
		e.next()
		return 0 // undefined identifier
	}
	e.fail("unexpected %s in #%s expression", t, e.d.Text)
	return 0
}

func (e *ifEval) fail(format string, args ...any) {
	if e.err == nil {
		e.err = syntaxErr(e.file, e.d.Line, e.d.Col, format, args...)
	}
	// Skip to the end so that callers terminate.
	for e.toks[e.i].Kind != EOF {
		e.i++
	}
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
