package promela

import (
	"strings"
)

// The parser accepts exactly the MVP subset of plan 14 §5.2 (as amended
// after K1) and rejects everything else by name. The division is
// exhaustive: a token sequence is either parsed into the AST, a syntax
// error (malformed subset construct), or an outside-subset rejection
// naming the construct — the tables below list every construct the parser
// knows to be outside the subset; an unknown keyword in statement position
// is a syntax error, never silently accepted.

// outsideKeywords maps a keyword that starts a construct outside the MVP
// subset to the construct's name and the plan's note.
var outsideKeywords = map[string][2]string{
	"c_code":       {"c_code", "embedded C is outside the subset"},
	"c_decl":       {"c_decl", "embedded C is outside the subset"},
	"c_state":      {"c_state", "embedded C is outside the subset"},
	"c_track":      {"c_track", "embedded C is outside the subset"},
	"c_expr":       {"c_expr", "embedded C is outside the subset"},
	"unless":       {"unless", "plan 14 §5.2: outside the subset"},
	"priority":     {"priority", "process priorities are outside the subset"},
	"ltl":          {"ltl", "inline LTL blocks are outside the subset; use never { }"},
	"trace":        {"trace", "event traces are outside the subset"},
	"notrace":      {"notrace", "event traces are outside the subset"},
	"select":       {"select", "outside the subset"},
	"for":          {"for", "outside the subset"},
	"hidden":       {"hidden", "variable qualifiers are outside the subset"},
	"show":         {"show", "variable qualifiers are outside the subset"},
	"local":        {"local", "variable qualifiers are outside the subset"},
	"unsigned":     {"unsigned", "outside the subset"},
	"eval":         {"eval", "plan 14 §5.2: not in the corpus, outside the subset"},
	"enabled":      {"enabled()", "outside the subset"},
	"_last":        {"_last", "outside the subset"},
	"np_":          {"np_", "outside the subset"},
	"get_priority": {"get_priority()", "outside the subset"},
	"set_priority": {"set_priority()", "outside the subset"},
}

var typeKeywords = map[string]bool{
	"bit": true, "bool": true, "byte": true, "short": true, "int": true, "mtype": true, "pid": true, "chan": true,
}

type parser struct {
	toks []Token
	i    int
	file string
	mod  *Module

	inInit    bool
	inNever   bool
	optDepth  int  // inside if/do options
	noCall    bool // parsing a channel argument: ident( is not a call
	procNames map[string]bool
	// typedefs maps a typedef name to its fields, already flattened: the
	// field name is the path under the instance ("fld2.f"), so declaring
	// `Record goo` declares "goo.fld2.f" and the rest (G5).
	typedefs map[string][]*VarDecl
	// structVars records the instances of a typedef, so that `run me(foo)`
	// can be expanded to the fields SPIN passes ("run me(foo.f,foo.g)").
	structVars map[string][]string
}

// parseModule builds the AST of preprocessed tokens.
func parseModule(toks []Token, file string) (*Module, *Error) {
	p := &parser{toks: toks, file: file, mod: &Module{Text: &TextSource{Toks: toks}}, procNames: map[string]bool{},
		typedefs: map[string][]*VarDecl{}, structVars: map[string][]string{}}
	if err := p.module(); err != nil {
		return nil, err
	}
	return p.mod, nil
}

func (p *parser) peek() Token { return p.toks[p.i] }
func (p *parser) peekN(n int) Token {
	if p.i+n < len(p.toks) {
		return p.toks[p.i+n]
	}
	return p.toks[len(p.toks)-1]
}
func (p *parser) next() Token {
	t := p.toks[p.i]
	if t.Kind != EOF {
		p.i++
	}
	return t
}

func (p *parser) errAt(t Token, format string, args ...any) *Error {
	return syntaxErr(p.file, t.Line, t.Col, format, args...)
}

func (p *parser) expect(text string) (Token, *Error) {
	t := p.peek()
	if !t.is(text) {
		return t, p.errAt(t, "expected %q, got %s", text, t)
	}
	return p.next(), nil
}

func (p *parser) expectIdent() (Token, *Error) {
	t := p.peek()
	if t.Kind != Ident {
		return t, p.errAt(t, "expected a name, got %s", t)
	}
	return p.next(), nil
}

func (p *parser) outsideAt(t Token) *Error {
	k := outsideKeywords[t.Text]
	return outside(p.file, t.Line, t.Col, k[0], k[1])
}

// ---- module -----------------------------------------------------------------

func (p *parser) module() *Error {
	for {
		t := p.peek()
		switch {
		case t.Kind == EOF:
			return nil
		case t.is(";"):
			p.next()
		case t.Kind == Ident && outsideKeywords[t.Text] != [2]string{}:
			return p.outsideAt(t)
		case t.isIdent("mtype") && (p.peekN(1).is("=") || p.peekN(1).is("{")):
			if err := p.mtypeDecl(); err != nil {
				return err
			}
		case t.isIdent("mtype") && p.peekN(1).is(":"):
			return outside(p.file, t.Line, t.Col, "typed mtype (mtype:name)", "outside the subset")
		case t.isIdent("typedef"):
			if err := p.typedefDecl(); err != nil {
				return err
			}
		case t.Kind == Ident && (typeKeywords[t.Text] || p.typedefs[t.Text] != nil):
			decls, err := p.varDecls(true)
			if err != nil {
				return err
			}
			p.mod.Globals = append(p.mod.Globals, decls...)
		case t.isIdent("active") || t.isIdent("proctype"):
			if err := p.proctype(); err != nil {
				return err
			}
		case t.isIdent("init"):
			if err := p.initOrNever(true); err != nil {
				return err
			}
		case t.isIdent("never"):
			if err := p.initOrNever(false); err != nil {
				return err
			}
		default:
			return p.errAt(t, "unexpected %s at the top level (expected a declaration, proctype, init or never)", t)
		}
	}
}

// typedefDecl reads `typedef Name { fields };` and stores the fields
// flattened: a nested struct contributes its own fields with a dotted path,
// so that the IR sees plain variables and pan's own flattening ("z.g",
// "goo.fld2.f") is reproduced name for name.
func (p *parser) typedefDecl() *Error {
	kw := p.next()
	name, err := p.expectIdent()
	if err != nil {
		return err
	}
	if p.typedefs[name.Text] != nil {
		return semanticErr(p.file, name.Line, name.Col, "typedef %s declared twice", name.Text)
	}
	if _, err := p.expect("{"); err != nil {
		return err
	}
	var fields []*VarDecl
	for !p.peek().is("}") {
		if p.peek().Kind == EOF {
			return syntaxErr(p.file, kw.Line, kw.Col, "unterminated typedef %s", name.Text)
		}
		if p.peek().is(";") {
			p.next()
			continue
		}
		t := p.peek()
		if t.Kind != Ident || (!typeKeywords[t.Text] && p.typedefs[t.Text] == nil) {
			if t.Kind == Ident && outsideKeywords[t.Text] != [2]string{} {
				return p.outsideAt(t)
			}
			return p.errAt(t, "expected a field type in typedef %s, got %s", name.Text, t)
		}
		decls, err := p.varDecls(true)
		if err != nil {
			return err
		}
		fields = append(fields, decls...)
	}
	if _, err := p.expect("}"); err != nil {
		return err
	}
	if p.peek().is(";") {
		p.next()
	}
	if len(fields) == 0 {
		return semanticErr(p.file, name.Line, name.Col, "typedef %s has no fields", name.Text)
	}
	p.typedefs[name.Text] = fields
	return nil
}

func (p *parser) mtypeDecl() *Error {
	p.next() // mtype
	if p.peek().is("=") {
		p.next()
	}
	if _, err := p.expect("{"); err != nil {
		return err
	}
	var names []string
	for {
		t, err := p.expectIdent()
		if err != nil {
			return err
		}
		names = append(names, t.Text)
		if p.peek().is(",") {
			p.next()
			if p.peek().is("}") { // trailing comma
				break
			}
			continue
		}
		break
	}
	if _, err := p.expect("}"); err != nil {
		return err
	}
	if p.peek().is(";") {
		p.next()
	}
	p.mod.Mtypes = append(p.mod.Mtypes, names)
	return nil
}

// varDecls parses "type name[len] [= init], …" up to (not including) the
// separator. global selects the channel rules.
func (p *parser) varDecls(global bool) ([]*VarDecl, *Error) {
	typ := p.next()
	fields := p.typedefs[typ.Text]
	var out []*VarDecl
	for {
		name, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		d := &VarDecl{Name: name.Text, Type: typ.Text, Pos: Pos{name.Line, name.Col}}
		if p.peek().is("[") {
			p.next()
			n, err := p.constExpr()
			if err != nil {
				return nil, err
			}
			if n <= 0 {
				return nil, semanticErr(p.file, name.Line, name.Col, "array %s needs a positive length, got %d", d.Name, n)
			}
			d.Len = int(n)
			if _, err := p.expect("]"); err != nil {
				return nil, err
			}
		}
		if fields != nil {
			if d.Len > 0 {
				return nil, outside(p.file, name.Line, name.Col, "array of structs", "arrays of typedef instances are outside the subset; declare the fields as arrays instead")
			}
			inst, err := p.instantiate(d.Name, fields, name)
			if err != nil {
				return nil, err
			}
			out = append(out, inst...)
			if p.peek().is(",") {
				p.next()
				continue
			}
			return out, nil
		}
		if typ.Text == "chan" {
			if p.peek().is("=") {
				p.next()
				ci, err := p.chanInit()
				if err != nil {
					return nil, err
				}
				d.Chan = ci
			}
			// Without `= [cap] of { … }` this declares a channel-typed
			// variable (or an array of them): it holds a channel id, which
			// the lowering stores as a byte.
		} else if p.peek().is("=") {
			p.next()
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			d.Init = e
		}
		out = append(out, d)
		if p.peek().is(",") {
			p.next()
			continue
		}
		return out, nil
	}
}

// instantiate expands a typedef instance into its flattened fields.
func (p *parser) instantiate(base string, fields []*VarDecl, at Token) ([]*VarDecl, *Error) {
	var out []string
	var decls []*VarDecl
	for _, f := range fields {
		c := *f
		c.Name = base + "." + f.Name
		c.Pos = Pos{at.Line, at.Col}
		c.FromStruct = true
		decls = append(decls, &c)
		out = append(out, c.Name)
	}
	p.structVars[base] = out
	return decls, nil
}

func (p *parser) chanInit() (*ChanInit, *Error) {
	if _, err := p.expect("["); err != nil {
		return nil, err
	}
	n, err := p.constExpr()
	if err != nil {
		return nil, err
	}
	if n < 0 || n > 255 {
		return nil, semanticErr(p.file, p.peek().Line, p.peek().Col, "channel capacity %d outside 0..255", n)
	}
	if _, err := p.expect("]"); err != nil {
		return nil, err
	}
	t := p.peek()
	if !t.isIdent("of") {
		return nil, p.errAt(t, "expected \"of\", got %s", t)
	}
	p.next()
	if _, err := p.expect("{"); err != nil {
		return nil, err
	}
	ci := &ChanInit{Cap: int(n)}
	for {
		t := p.next()
		if t.Kind != Ident {
			return nil, p.errAt(t, "expected a message field type, got %s", t)
		}
		switch t.Text {
		case "unsigned":
			return nil, outside(p.file, t.Line, t.Col, "unsigned", "outside the subset")
		case "bit", "bool", "byte", "short", "int", "mtype", "pid", "chan":
			// A `chan` field carries a channel id, stored as a byte (G5).
			ci.Fields = append(ci.Fields, t.Text)
		default:
			return nil, outside(p.file, t.Line, t.Col, "user-defined message field type "+t.Text,
				"a typedef instance cannot be a channel message field in this engine version; send its fields separately")
		}
		if p.peek().is(",") {
			p.next()
			continue
		}
		break
	}
	if _, err := p.expect("}"); err != nil {
		return nil, err
	}
	return ci, nil
}

// constExpr parses an expression and requires it to be a number constant
// (macros are already expanded).
func (p *parser) constExpr() (int64, *Error) {
	t := p.peek()
	e, err := p.expr()
	if err != nil {
		return 0, err
	}
	v, ok := constFold(e)
	if !ok {
		return 0, semanticErr(p.file, t.Line, t.Col, "a constant is required here")
	}
	return v, nil
}

// constFold evaluates a numeric expression without variables.
func constFold(e Expr) (int64, bool) {
	switch x := e.(type) {
	case *Num:
		return x.Val, true
	case *Unary:
		v, ok := constFold(x.X)
		if !ok {
			return 0, false
		}
		if x.Op == "!" {
			return b2i(v == 0), true
		}
		return -v, true
	case *Binary:
		a, ok := constFold(x.X)
		if !ok {
			return 0, false
		}
		b, ok := constFold(x.Y)
		if !ok {
			return 0, false
		}
		switch x.Op {
		case "+":
			return a + b, true
		case "-":
			return a - b, true
		case "*":
			return a * b, true
		case "/":
			if b == 0 {
				return 0, false
			}
			return a / b, true
		case "%":
			if b == 0 {
				return 0, false
			}
			return a % b, true
		case "==":
			return b2i(a == b), true
		case "!=":
			return b2i(a != b), true
		case "<":
			return b2i(a < b), true
		case "<=":
			return b2i(a <= b), true
		case ">":
			return b2i(a > b), true
		case ">=":
			return b2i(a >= b), true
		case "&&":
			return b2i(a != 0 && b != 0), true
		case "||":
			return b2i(a != 0 || b != 0), true
		}
	}
	return 0, false
}

func (p *parser) proctype() *Error {
	pt := &Proctype{}
	t := p.peek()
	pt.Pos = Pos{t.Line, t.Col}
	if t.isIdent("active") {
		p.next()
		pt.Active = 1
		if p.peek().is("[") {
			p.next()
			n, err := p.constExpr()
			if err != nil {
				return err
			}
			if n < 1 || n > 254 {
				return semanticErr(p.file, t.Line, t.Col, "active [%d]: instance count outside 1..254", n)
			}
			pt.Active = int(n)
			if _, err := p.expect("]"); err != nil {
				return err
			}
		}
	}
	kw := p.peek()
	if kw.isIdent("D_proctype") {
		return outside(p.file, kw.Line, kw.Col, "D_proctype", "outside the subset")
	}
	if !kw.isIdent("proctype") {
		return p.errAt(kw, "expected \"proctype\", got %s", kw)
	}
	p.next()
	name, err := p.expectIdent()
	if err != nil {
		return err
	}
	pt.Name = name.Text
	if p.procNames[pt.Name] {
		return semanticErr(p.file, name.Line, name.Col, "proctype %s declared twice", pt.Name)
	}
	p.procNames[pt.Name] = true
	if _, err := p.expect("("); err != nil {
		return err
	}
	for !p.peek().is(")") {
		if p.peek().Kind == EOF {
			return p.errAt(p.peek(), "unterminated parameter list of %s", pt.Name)
		}
		t := p.peek()
		if t.Kind != Ident || (!typeKeywords[t.Text] && p.typedefs[t.Text] == nil) {
			if t.Kind == Ident && outsideKeywords[t.Text] != [2]string{} {
				return p.outsideAt(t)
			}
			return p.errAt(t, "expected a parameter type, got %s", t)
		}
		decls, err := p.varDecls(false)
		if err != nil {
			return err
		}
		for _, d := range decls {
			if d.Init != nil && d.FromStruct {
				// A typedef field may carry an initialiser; as a parameter
				// its value comes from the call, so SPIN ignores it.
				d.Init = nil
			}
			if d.Init != nil {
				return semanticErr(p.file, d.Pos.Line, d.Pos.Col, "parameter %s cannot have an initialiser", d.Name)
			}
			if d.Len > 0 {
				return outside(p.file, d.Pos.Line, d.Pos.Col, "array parameter", "outside the subset")
			}
		}
		pt.Params = append(pt.Params, decls...)
		if p.peek().is(";") {
			p.next()
		}
	}
	p.next() // )
	if p.peek().isIdent("provided") {
		p.next()
		if _, err := p.expect("("); err != nil {
			return err
		}
		e, err := p.expr()
		if err != nil {
			return err
		}
		if _, err := p.expect(")"); err != nil {
			return err
		}
		pt.Provided = e
	}
	if t := p.peek(); t.Kind == Ident && outsideKeywords[t.Text] != [2]string{} {
		return p.outsideAt(t)
	}
	body, end, err := p.body()
	if err != nil {
		return err
	}
	pt.Body, pt.End = body, end
	p.mod.Procs = append(p.mod.Procs, pt)
	return nil
}

func (p *parser) initOrNever(isInit bool) *Error {
	t := p.next()
	pt := &Proctype{Pos: Pos{t.Line, t.Col}, IsInit: isInit, IsNever: !isInit}
	if isInit {
		pt.Name, pt.Active = "init", 1
		if p.mod.hasInit() {
			return semanticErr(p.file, t.Line, t.Col, "second init")
		}
	} else {
		pt.Name = "never"
		if p.mod.Never != nil {
			return outside(p.file, t.Line, t.Col, "second never claim", "one never claim per model")
		}
	}
	if isInit && p.peek().isIdent("priority") {
		return p.outsideAt(p.peek())
	}
	p.inInit, p.inNever = isInit, !isInit
	body, end, err := p.body()
	p.inInit, p.inNever = false, false
	if err != nil {
		return err
	}
	pt.Body, pt.End = body, end
	if isInit {
		p.mod.Procs = append(p.mod.Procs, pt)
	} else {
		p.mod.Never = pt
	}
	return nil
}

func (m *Module) hasInit() bool {
	for _, pt := range m.Procs {
		if pt.IsInit {
			return true
		}
	}
	return false
}

// body parses "{ seq }" and returns the position of the closing brace.
func (p *parser) body() (*Block, Pos, *Error) {
	open, err := p.expect("{")
	if err != nil {
		return nil, Pos{}, err
	}
	items, err := p.seq("}")
	if err != nil {
		return nil, Pos{}, err
	}
	closeTok, err := p.expect("}")
	if err != nil {
		return nil, Pos{}, err
	}
	return &Block{Items: items, Pos: Pos{open.Line, open.Col}}, Pos{closeTok.Line, closeTok.Col}, nil
}

// ---- statements ---------------------------------------------------------------

func isSep(t Token) bool { return t.is(";") || t.is("->") }

// seq parses statements up to one of the closers (not consumed).
func (p *parser) seq(closers ...string) ([]Item, *Error) {
	var items []Item
	for {
		for isSep(p.peek()) {
			p.next()
		}
		t := p.peek()
		if t.Kind == EOF {
			return nil, p.errAt(t, "unexpected end of file (missing %s)", strings.Join(closers, " or "))
		}
		for _, c := range closers {
			if t.is(c) || t.isIdent(c) {
				return items, nil
			}
		}
		if t.is("::") {
			return items, nil
		}
		it, err := p.item()
		if err != nil {
			return nil, err
		}
		items = append(items, it)
		t = p.peek()
		if t.isIdent("unless") {
			return nil, p.outsideAt(t)
		}
		// A separator is optional after a compound statement (SPIN accepts
		// "} stmt" and "fi stmt").
		switch it.Stmt.(type) {
		case *Block, *If, *Do:
			continue
		}
		if !isSep(t) && !t.is("::") && !t.is("}") && !t.isIdent("fi") && !t.isIdent("od") && t.Kind != EOF {
			return nil, p.errAt(t, "expected \";\" or \"->\" after a statement, got %s", t)
		}
	}
}

func (p *parser) item() (Item, *Error) {
	it := Item{From: p.i}
	// labels
	for p.peek().Kind == Ident && p.peekN(1).is(":") && !typeKeywords[p.peek().Text] {
		l := p.next()
		p.next() // :
		it.Labels = append(it.Labels, l.Text)
	}
	t := p.peek()
	it.Pos = Pos{t.Line, t.Col}
	it.From = p.i
	st, err := p.stmt()
	if err != nil {
		return it, err
	}
	it.Stmt = st
	it.To = p.i
	return it, nil
}

func (p *parser) stmt() (Stmt, *Error) {
	t := p.peek()
	switch {
	case t.is("{"):
		b, _, err := p.body()
		return b, err
	case t.isIdent("atomic") || t.isIdent("d_step"):
		p.next()
		b, _, err := p.body()
		if err != nil {
			return nil, err
		}
		b.Kind = t.Text
		return b, nil
	case t.isIdent("if") || t.isIdent("do"):
		return p.options()
	case t.isIdent("goto"):
		p.next()
		l, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		return &Goto{Label: l.Text}, nil
	case t.isIdent("break"):
		p.next()
		return &Break{}, nil
	case t.isIdent("else"):
		p.next()
		return &Else{}, nil
	case t.isIdent("assert"):
		p.next()
		if _, err := p.expect("("); err != nil {
			return nil, err
		}
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(")"); err != nil {
			return nil, err
		}
		return &Assert{Cond: e}, nil
	case t.isIdent("printf") || t.isIdent("printm"):
		return p.printf()
	case t.isIdent("run"):
		return p.run(nil)
	case t.isIdent("xr") || t.isIdent("xs"):
		p.next()
		x := &XrXs{Kind: t.Text}
		for {
			c, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			x.Chans = append(x.Chans, c.Text)
			if p.peek().is(",") {
				p.next()
				continue
			}
			break
		}
		return x, nil
	case t.Kind == Ident && (typeKeywords[t.Text] || p.typedefs[t.Text] != nil):
		if p.inNever {
			return nil, outside(p.file, t.Line, t.Col, "local variable in a never claim", "outside the subset")
		}
		decls, err := p.varDecls(false)
		if err != nil {
			return nil, err
		}
		if len(decls) == 1 {
			return &Decl{Var: decls[0]}, nil
		}
		// several names in one declaration: a "decls" block (no scope of
		// its own; the names belong to the enclosing block)
		b := &Block{Kind: "decls", Pos: Pos{t.Line, t.Col}}
		for _, d := range decls {
			b.Items = append(b.Items, Item{Stmt: &Decl{Var: d}, Pos: d.Pos, From: p.i, To: p.i})
		}
		return b, nil
	case t.Kind == Ident && outsideKeywords[t.Text] != [2]string{}:
		return nil, p.outsideAt(t)
	case p.chanOpKind() == "!!":
		return nil, outside(p.file, t.Line, t.Col, "sorted send (!!)", "outside the subset")
	case p.chanOpKind() == "??":
		return nil, outside(p.file, t.Line, t.Col, "random receive (??)", "outside the subset")
	case p.chanOpKind() == "?<":
		return nil, outside(p.file, t.Line, t.Col, "copy receive (?<…>)", "outside the subset")
	case p.chanOpKind() == "?[":
		return nil, outside(p.file, t.Line, t.Col, "channel poll (?[…])", "outside the subset")
	case p.chanOpKind() == "!":
		lv, err := p.chanTarget()
		if err != nil {
			return nil, err
		}
		p.next() // !
		args, err := p.chanArgs()
		if err != nil {
			return nil, err
		}
		return &Send{Chan: lv.Name, Index: lv.Index, Args: args, Pos: lv.Pos}, nil
	case p.chanOpKind() == "?":
		lv, err := p.chanTarget()
		if err != nil {
			return nil, err
		}
		p.next() // ?
		args, err := p.chanArgs()
		if err != nil {
			return nil, err
		}
		r := &Recv{Chan: lv.Name, Index: lv.Index, Pos: lv.Pos}
		for _, a := range args {
			ra := RecvArg{Pos: exprPos(a, t)}
			switch x := a.(type) {
			case *VarRef:
				if x.Name == "_" {
					ra.Ignore = true
				} else {
					ra.Var = &LValue{Name: x.Name, Pos: x.Pos}
				}
			case *IndexExpr:
				ra.Var = &LValue{Name: x.Name, Index: x.Index, Pos: x.Pos}
			default:
				if _, ok := constFold(a); ok {
					ra.Match = a
				} else {
					return nil, outside(p.file, ra.Pos.Line, ra.Pos.Col, "expression in a receive argument", "needs eval(), which is outside the subset")
				}
			}
			r.Args = append(r.Args, ra)
		}
		return r, nil
	case t.Kind == Ident && (p.peekN(1).is("=") || p.peekN(1).is("++") || p.peekN(1).is("--") || p.peekN(1).is("[") || p.peekN(1).is(".")):
		// assignment, or an indexed expression statement
		save := p.i
		lv, err := p.lvalue()
		if err != nil {
			return nil, err
		}
		op := p.peek()
		switch {
		case op.is("++") || op.is("--"):
			p.next()
			return &Assign{Target: lv, Op: op.Text}, nil
		case op.is("="):
			p.next()
			if p.peek().isIdent("run") {
				return p.run(lv)
			}
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			return &Assign{Target: lv, Op: "=", Value: e}, nil
		}
		p.i = save
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		return &ExprStmt{X: e}, nil
	}
	e, err := p.expr()
	if err != nil {
		return nil, err
	}
	return &ExprStmt{X: e}, nil
}

// chanOpKind looks ahead over a channel name — an identifier with optional
// struct fields (`r.c`) and an optional array index (`q[i]`) — and reports
// the channel operator that follows it, or "" when the statement is not a
// channel operation. The lookahead is what allows `q[proc-1]!one(x)` to be
// told apart from an assignment to an array element.
func (p *parser) chanOpKind() string {
	i := p.i
	if p.toks[i].Kind != Ident {
		return ""
	}
	i++
	for i < len(p.toks) {
		switch {
		case p.toks[i].is(".") && i+1 < len(p.toks) && p.toks[i+1].Kind == Ident:
			i += 2
		case p.toks[i].is("["):
			depth := 0
			for ; i < len(p.toks); i++ {
				if p.toks[i].is("[") {
					depth++
				} else if p.toks[i].is("]") {
					depth--
					if depth == 0 {
						i++
						break
					}
				} else if p.toks[i].Kind == EOF {
					return ""
				}
			}
		default:
			goto done
		}
	}
done:
	if i >= len(p.toks) {
		return ""
	}
	switch {
	case p.toks[i].is("!!"):
		return "!!"
	case p.toks[i].is("??"):
		return "??"
	case p.toks[i].is("?") && i+1 < len(p.toks) && p.toks[i+1].is("<"):
		return "?<"
	case p.toks[i].is("?") && i+1 < len(p.toks) && p.toks[i+1].is("["):
		return "?["
	case p.toks[i].is("!"):
		return "!"
	case p.toks[i].is("?"):
		return "?"
	}
	return ""
}

// chanTarget parses the channel name of a channel operation.
func (p *parser) chanTarget() (*LValue, *Error) {
	return p.lvalue()
}

func exprPos(e Expr, fallback Token) Pos {
	switch x := e.(type) {
	case *VarRef:
		return x.Pos
	case *IndexExpr:
		return x.Pos
	}
	return Pos{fallback.Line, fallback.Col}
}

func (p *parser) lvalue() (*LValue, *Error) {
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	lv := &LValue{Name: p.dotted(name), Pos: Pos{name.Line, name.Col}}
	if p.peek().is("[") {
		p.next()
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect("]"); err != nil {
			return nil, err
		}
		lv.Index = e
	}
	return lv, nil
}

// dotted reads the rest of a struct field path after first; the flattened
// name is the one the lowering declared ("goo.fld2.f").
func (p *parser) dotted(first Token) string {
	name := first.Text
	for p.peek().is(".") && p.peekN(1).Kind == Ident {
		p.next()
		name += "." + p.next().Text
	}
	return name
}

func (p *parser) options() (Stmt, *Error) {
	kw := p.next()
	closer := "fi"
	if kw.Text == "do" {
		closer = "od"
	}
	var opts [][]Item
	p.optDepth++
	defer func() { p.optDepth-- }()
	for {
		t := p.peek()
		if t.isIdent(closer) {
			p.next()
			break
		}
		if !t.is("::") {
			return nil, p.errAt(t, "expected \"::\" or %q in %s, got %s", closer, kw.Text, t)
		}
		p.next()
		items, err := p.seq(closer)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, p.errAt(t, "empty option in %s", kw.Text)
		}
		opts = append(opts, items)
	}
	if len(opts) == 0 {
		return nil, p.errAt(kw, "%s without options", kw.Text)
	}
	if kw.Text == "if" {
		return &If{Options: opts, Pos: Pos{kw.Line, kw.Col}}, nil
	}
	return &Do{Options: opts, Pos: Pos{kw.Line, kw.Col}}, nil
}

func (p *parser) printf() (Stmt, *Error) {
	kw := p.next()
	if _, err := p.expect("("); err != nil {
		return nil, err
	}
	if kw.Text == "printf" {
		if t := p.peek(); t.Kind != String {
			return nil, p.errAt(t, "printf needs a format string, got %s", t)
		}
		p.next()
	}
	pf := &Printf{}
	if kw.Text == "printm" {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		pf.Args = append(pf.Args, e)
	}
	for p.peek().is(",") {
		p.next()
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		pf.Args = append(pf.Args, e)
	}
	if _, err := p.expect(")"); err != nil {
		return nil, err
	}
	return pf, nil
}

// run parses "run P(args)" as a statement (target != nil for x = run …).
func (p *parser) run(target *LValue) (Stmt, *Error) {
	kw := p.next()
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect("("); err != nil {
		return nil, err
	}
	rs := &RunStmt{Proc: name.Text, Target: target, Pos: Pos{kw.Line, kw.Col}, InLoop: p.optDepth > 0}
	for !p.peek().is(")") {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		// A struct passed by value is passed field by field, as SPIN does
		// (probe: pan -d on CH3/typedef.pml prints "run me(foo.f,foo.g)").
		if v, ok := e.(*VarRef); ok {
			if fields := p.structVars[v.Name]; fields != nil {
				for _, f := range fields {
					rs.Args = append(rs.Args, &VarRef{Name: f, Pos: v.Pos})
				}
				if p.peek().is(",") {
					p.next()
					continue
				}
				if !p.peek().is(")") {
					return nil, p.errAt(p.peek(), "expected \",\" or \")\" in run arguments, got %s", p.peek())
				}
				continue
			}
		}
		rs.Args = append(rs.Args, e)
		if p.peek().is(",") {
			p.next()
			continue
		}
		if !p.peek().is(")") {
			return nil, p.errAt(p.peek(), "expected \",\" or \")\" in run arguments, got %s", p.peek())
		}
	}
	p.next()
	return rs, nil
}

// chanArgs parses "a, b, c" or "a(b, c)" after ! or ?.
func (p *parser) chanArgs() ([]Expr, *Error) {
	p.noCall = true
	defer func() { p.noCall = false }()
	first, err := p.expr()
	if err != nil {
		return nil, err
	}
	args := []Expr{first}
	if p.peek().is("(") {
		p.next()
		for {
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			args = append(args, e)
			if p.peek().is(",") {
				p.next()
				continue
			}
			break
		}
		if _, err := p.expect(")"); err != nil {
			return nil, err
		}
		return args, nil
	}
	for p.peek().is(",") {
		p.next()
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		args = append(args, e)
	}
	return args, nil
}

// ---- expressions --------------------------------------------------------------

func (p *parser) expr() (Expr, *Error) { return p.orExpr() }

func (p *parser) orExpr() (Expr, *Error) {
	x, err := p.andExpr()
	if err != nil {
		return nil, err
	}
	for p.peek().is("||") {
		p.next()
		y, err := p.andExpr()
		if err != nil {
			return nil, err
		}
		x = &Binary{Op: "||", X: x, Y: y}
	}
	return x, nil
}

func (p *parser) andExpr() (Expr, *Error) {
	x, err := p.eqExpr()
	if err != nil {
		return nil, err
	}
	for p.peek().is("&&") {
		p.next()
		y, err := p.eqExpr()
		if err != nil {
			return nil, err
		}
		x = &Binary{Op: "&&", X: x, Y: y}
	}
	return x, nil
}

func (p *parser) bitwise(t Token) *Error {
	return outside(p.file, t.Line, t.Col, "bitwise operator "+t.Text, "outside the subset")
}

func (p *parser) eqExpr() (Expr, *Error) {
	x, err := p.relExpr()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.is("|") || t.is("&") || t.is("^") {
			return nil, p.bitwise(t)
		}
		if !t.is("==") && !t.is("!=") {
			return x, nil
		}
		p.next()
		y, err := p.relExpr()
		if err != nil {
			return nil, err
		}
		x = &Binary{Op: t.Text, X: x, Y: y}
	}
}

func (p *parser) relExpr() (Expr, *Error) {
	x, err := p.addExpr()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.is("<<") || t.is(">>") {
			return nil, p.bitwise(t)
		}
		if !t.is("<") && !t.is("<=") && !t.is(">") && !t.is(">=") {
			return x, nil
		}
		p.next()
		y, err := p.addExpr()
		if err != nil {
			return nil, err
		}
		x = &Binary{Op: t.Text, X: x, Y: y}
	}
}

func (p *parser) addExpr() (Expr, *Error) {
	x, err := p.mulExpr()
	if err != nil {
		return nil, err
	}
	for p.peek().is("+") || p.peek().is("-") {
		t := p.next()
		y, err := p.mulExpr()
		if err != nil {
			return nil, err
		}
		x = &Binary{Op: t.Text, X: x, Y: y}
	}
	return x, nil
}

func (p *parser) mulExpr() (Expr, *Error) {
	x, err := p.unaryExpr()
	if err != nil {
		return nil, err
	}
	for p.peek().is("*") || p.peek().is("/") || p.peek().is("%") {
		t := p.next()
		y, err := p.unaryExpr()
		if err != nil {
			return nil, err
		}
		x = &Binary{Op: t.Text, X: x, Y: y}
	}
	return x, nil
}

func (p *parser) unaryExpr() (Expr, *Error) {
	t := p.peek()
	switch {
	case t.is("!"):
		p.next()
		x, err := p.unaryExpr()
		if err != nil {
			return nil, err
		}
		return &Unary{Op: "!", X: x}, nil
	case t.is("-"):
		p.next()
		x, err := p.unaryExpr()
		if err != nil {
			return nil, err
		}
		if n, ok := x.(*Num); ok {
			return &Num{Val: -n.Val}, nil
		}
		return &Unary{Op: "-", X: x}, nil
	case t.is("~"):
		return nil, p.bitwise(t)
	}
	return p.primary()
}

func (p *parser) primary() (Expr, *Error) {
	t := p.next()
	switch {
	case t.Kind == Number:
		return &Num{Val: t.Val}, nil
	case t.Kind == String:
		return nil, p.errAt(t, "a string is allowed only in printf")
	case t.is("("):
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		if p.peek().is("->") {
			return nil, outside(p.file, t.Line, t.Col, "conditional expression (c -> a : b)", "outside the subset")
		}
		if _, err := p.expect(")"); err != nil {
			return nil, err
		}
		return e, nil
	case t.Kind == Ident:
		switch t.Text {
		case "true", "skip":
			return &Num{Val: 1}, nil
		case "false":
			return &Num{Val: 0}, nil
		case "timeout":
			return &TimeoutExpr{}, nil
		case "run":
			return nil, outside(p.file, t.Line, t.Col, "run inside an expression", "run is accepted as a statement, on its own or as `pid = run P(...)`, not inside a larger expression")
		case "len", "empty", "nempty", "full", "nfull":
			if _, err := p.expect("("); err != nil {
				return nil, err
			}
			c, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			ce := &ChanExpr{Fn: t.Text, Chan: p.dotted(c), Pos: Pos{t.Line, t.Col}}
			if p.peek().is("[") {
				p.next()
				idx, err := p.expr()
				if err != nil {
					return nil, err
				}
				if _, err := p.expect("]"); err != nil {
					return nil, err
				}
				ce.Index = idx
			}
			if _, err := p.expect(")"); err != nil {
				return nil, err
			}
			return ce, nil
		case "_nr_pr":
			return &NrPrExpr{Pos: Pos{t.Line, t.Col}}, nil
		case "pc_value":
			if _, err := p.expect("("); err != nil {
				return nil, err
			}
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(")"); err != nil {
				return nil, err
			}
			return &PCValueExpr{Proc: e, Pos: Pos{t.Line, t.Col}}, nil
		}
		if outsideKeywords[t.Text] != [2]string{} {
			return nil, p.outsideAt(t)
		}
		n := p.peek()
		switch {
		case n.is("@"):
			return nil, outside(p.file, t.Line, t.Col, "remote reference (P@label)", "outside the subset")
		case n.is(":") && !p.peekN(1).is(":"):
			return nil, outside(p.file, t.Line, t.Col, "remote variable reference (P:x)", "outside the subset")
		case n.is("?") && p.peekN(1).is("["):
			return nil, outside(p.file, t.Line, t.Col, "channel poll (?[…])", "outside the subset")
		case n.is("??"):
			return nil, outside(p.file, t.Line, t.Col, "random receive (??)", "outside the subset")
		case n.is("!!"):
			return nil, outside(p.file, t.Line, t.Col, "sorted send (!!)", "outside the subset")
		case n.is("["):
			p.next()
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect("]"); err != nil {
				return nil, err
			}
			return &IndexExpr{Name: t.Text, Index: e, Pos: Pos{t.Line, t.Col}}, nil
		case n.is("(") && !p.noCall:
			return nil, p.errAt(t, "%s is not a variable, a macro or an operator of the subset", t.Text)
		}
		return &VarRef{Name: t.Text, Pos: Pos{t.Line, t.Col}}, nil
	}
	return nil, p.errAt(t, "unexpected %s in expression", t)
}

// ---- statement text -------------------------------------------------------------

// Render reconstructs source-like text for the tokens [from, to).
func (ts *TextSource) Render(from, to int) string {
	var b strings.Builder
	var prev, prev2 *Token
	for i := from; i < to && i < len(ts.Toks); i++ {
		t := &ts.Toks[i]
		if prev != nil && needSpace(prev2, prev, t) {
			b.WriteByte(' ')
		}
		if t.Kind == String {
			b.WriteString(`"` + t.Text + `"`)
		} else {
			b.WriteString(t.Text)
		}
		prev2, prev = prev, t
	}
	return b.String()
}

// isOperand: a token that can end an operand (so a following - is binary).
func isOperand(t *Token) bool {
	return t != nil && (t.Kind == Number || t.Kind == Ident || t.is(")") || t.is("]"))
}

// needSpace decides the spacing between a and b given the token before a.
func needSpace(beforeA, a, b *Token) bool {
	switch {
	case b.is(")") || b.is("]") || b.is(",") || b.is(";") || b.is("(") && a.Kind == Ident || b.is("[") && a.Kind == Ident:
		return false
	case a.is("(") || a.is("[") || a.is("!") || a.is("?") || b.is("!") || b.is("?"):
		return false
	case b.is("++") || b.is("--"):
		return false
	case a.is("-") && !isOperand(beforeA):
		return false // unary minus is tight
	}
	return true
}
