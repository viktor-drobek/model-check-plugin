package promela

import "fmt"

// `inline` (plan 14 §5.2, v1 / G5).
//
// SPIN's `inline` is textual substitution, not a procedure call: the body
// replaces the call, the arguments replace the parameter names token by
// token, and the result is parsed as if it had been written there. That is
// exactly what this pass does, before the parser sees anything, which has
// three consequences worth naming:
//
//   - the statements of an expansion carry the *body's* source lines, which
//     is what `pan -d` prints for them (probe: CH3/inline.pml, lines 2–4 in
//     init), so the statement tables still match;
//   - an expansion is spliced in braces, whose position is the position of
//     the body's first token, so the first statement of an option that
//     starts with an inline call is attributed to the inline's own line —
//     again as pan does it (probe: CH2/prodcons2.pml, the inlined guard is
//     the option's first statement at line 7, the inline's line);
//   - the braces make the body a nested scope, and a declaration in a
//     nested scope is a step for SPIN: `int y;` inside an inline becomes
//     the transition `y = 0` (probe: CH3/inline2.pml and a two-call probe,
//     where SPIN keeps one variable and re-initialises it per expansion).
//     The lowering reproduces both halves of that rule.
//
// Labels are the one thing substitution cannot leave alone: two expansions
// of a body with a label `L` would give one process two locations called
// `L`. Every label defined in a body, and every use of it inside that body,
// is therefore renamed to `<inline>_<call number>_<label>` in the
// expansion. A label that is *not* defined in the body (a `goto` out of the
// inline into the enclosing proctype) keeps its name, because it names a
// location the enclosing process owns.

// maxInlineDepth bounds mutual/recursive expansion; SPIN has no recursion
// in inlines either, and a bound turns a mistake into a message rather than
// a hang.
const maxInlineDepth = 32

type inlineDef struct {
	name   string
	params []string
	body   []Token
	line   int
	col    int
}

// ExpandInlines removes the `inline` definitions from toks and replaces
// every call by its body with the arguments substituted.
func ExpandInlines(toks []Token, file string) ([]Token, *Error) {
	defs := map[string]*inlineDef{}
	rest, err := collectInlines(toks, file, defs)
	if err != nil {
		return nil, err
	}
	if len(defs) == 0 {
		return rest, nil
	}
	counter := 0
	return expandCalls(rest, file, defs, &counter, 0)
}

// collectInlines takes the definitions out of the stream.
func collectInlines(toks []Token, file string, defs map[string]*inlineDef) ([]Token, *Error) {
	var out []Token
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if !t.isIdent("inline") {
			out = append(out, t)
			continue
		}
		d := &inlineDef{line: t.Line, col: t.Col}
		i++
		if i >= len(toks) || toks[i].Kind != Ident {
			return nil, syntaxErr(file, t.Line, t.Col, "inline needs a name")
		}
		d.name = toks[i].Text
		if defs[d.name] != nil {
			return nil, semanticErr(file, toks[i].Line, toks[i].Col, "inline %s declared twice", d.name)
		}
		i++
		if i >= len(toks) || !toks[i].is("(") {
			return nil, syntaxErr(file, t.Line, t.Col, "inline %s needs a parameter list in parentheses", d.name)
		}
		i++
		for i < len(toks) && !toks[i].is(")") {
			if toks[i].is(",") {
				i++
				continue
			}
			if toks[i].Kind != Ident {
				return nil, syntaxErr(file, toks[i].Line, toks[i].Col, "inline %s: %s is not a parameter name", d.name, toks[i])
			}
			d.params = append(d.params, toks[i].Text)
			i++
		}
		if i >= len(toks) {
			return nil, syntaxErr(file, t.Line, t.Col, "inline %s: unterminated parameter list", d.name)
		}
		i++ // )
		if i >= len(toks) || !toks[i].is("{") {
			return nil, syntaxErr(file, t.Line, t.Col, "inline %s needs a body in braces", d.name)
		}
		depth := 0
		start := i + 1
		for ; i < len(toks); i++ {
			if toks[i].is("{") {
				depth++
			} else if toks[i].is("}") {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if depth != 0 {
			return nil, syntaxErr(file, t.Line, t.Col, "inline %s: unterminated body", d.name)
		}
		d.body = append([]Token(nil), toks[start:i]...)
		defs[d.name] = d
	}
	return out, nil
}

// expandCalls replaces `name(args)` for every known inline.
func expandCalls(toks []Token, file string, defs map[string]*inlineDef, counter *int, depth int) ([]Token, *Error) {
	if depth > maxInlineDepth {
		return nil, semanticErr(file, 0, 0, "inline expansion nested deeper than %d: inlines call each other in a cycle", maxInlineDepth)
	}
	var out []Token
	changed := false
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		d := defs[t.Text]
		if t.Kind != Ident || d == nil {
			out = append(out, t)
			continue
		}
		if i+1 >= len(toks) || !toks[i+1].is("(") {
			return nil, syntaxErr(file, t.Line, t.Col, "inline %s is used without an argument list", d.name)
		}
		args, next, err := inlineArgs(toks, i+1, file, t)
		if err != nil {
			return nil, err
		}
		if len(args) != len(d.params) {
			return nil, semanticErr(file, t.Line, t.Col, "inline %s: %d argument(s) for %d parameter(s)", d.name, len(args), len(d.params))
		}
		*counter++
		out = append(out, substitute(d, args, *counter)...)
		i = next - 1
		changed = true
	}
	if !changed {
		return out, nil
	}
	return expandCalls(out, file, defs, counter, depth+1)
}

// inlineArgs reads the argument token lists of a call whose "(" is at open;
// it returns them and the index just past the ")".
func inlineArgs(toks []Token, open int, file string, at Token) ([][]Token, int, *Error) {
	var args [][]Token
	var cur []Token
	depth := 0
	i := open
	for ; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.is("(") || t.is("[") || t.is("{"):
			if depth > 0 {
				cur = append(cur, t)
			}
			depth++
		case t.is(")") || t.is("]") || t.is("}"):
			depth--
			if depth == 0 {
				if len(cur) > 0 || len(args) > 0 {
					args = append(args, cur)
				}
				return args, i + 1, nil
			}
			cur = append(cur, t)
		case t.is(",") && depth == 1:
			args = append(args, cur)
			cur = nil
		case t.Kind == EOF:
			return nil, 0, syntaxErr(file, at.Line, at.Col, "unterminated argument list of inline %s", at.Text)
		default:
			cur = append(cur, t)
		}
	}
	return nil, 0, syntaxErr(file, at.Line, at.Col, "unterminated argument list of inline %s", at.Text)
}

// substitute builds one expansion: parameters replaced by arguments,
// labels renamed with the call number.
func substitute(d *inlineDef, args [][]Token, call int) []Token {
	param := map[string][]Token{}
	for i, p := range d.params {
		param[p] = args[i]
	}
	labels := labelsOf(d.body)
	var out []Token
	open, close := Token{Kind: Punct, Text: "{"}, Token{Kind: Punct, Text: "}"}
	if len(d.body) > 0 {
		open.Line, open.Col = d.body[0].Line, d.body[0].Col
		last := d.body[len(d.body)-1]
		close.Line, close.Col = last.Line, last.Col
	} else {
		open.Line, open.Col, close.Line, close.Col = d.line, d.col, d.line, d.col
	}
	out = append(out, open)
	for _, t := range d.body {
		if t.Kind == Ident {
			if a, ok := param[t.Text]; ok {
				for _, x := range a {
					y := x
					y.Line, y.Col = t.Line, t.Col
					out = append(out, y)
				}
				continue
			}
			if labels[t.Text] {
				y := t
				y.Text = fmt.Sprintf("%s_%d_%s", d.name, call, t.Text)
				out = append(out, y)
				continue
			}
		}
		out = append(out, t)
	}
	return append(out, close)
}

// labelsOf collects the labels defined in a body: an identifier followed by
// a single ":" (the "::" of an option is one token, so it cannot be
// mistaken for one).
func labelsOf(body []Token) map[string]bool {
	out := map[string]bool{}
	for i := 0; i+1 < len(body); i++ {
		if body[i].Kind == Ident && body[i+1].is(":") {
			out[body[i].Text] = true
		}
	}
	return out
}
