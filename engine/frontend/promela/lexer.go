// Package promela is the frontend for the Promela subset of plan 14 §5.2
// (as amended after K1): a C-like preprocessor, a parser producing an AST
// with source positions, and a lowering to the IR of package ir whose
// state counts agree with SPIN's pan (optimisations disabled) — see
// lower.go for the encoding decisions and steps/g1-confirmation.md for the
// differential evidence.
//
// Every input is either accepted — an ir.Model plus warnings — or rejected
// with one *Error naming the kind (syntax, semantic, outside-subset), the
// file, the line and, for outside-subset, the construct. Nothing that is
// rejected has been executed; the CLI reports a rejection as status
// not-executed, never invalid-model (that status belongs to execution).
package promela

import (
	"fmt"
	"strings"
)

// Error kinds.
const (
	KindSyntax   = "syntax"
	KindSemantic = "semantic"
	KindOutside  = "outside-subset"
)

// Error is a rejection of the input.
type Error struct {
	Kind      string
	File      string
	Line, Col int
	// Construct names the rejected construct for KindOutside.
	Construct string
	Message   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s:%d:%d: %s", e.Kind, e.File, e.Line, e.Col, e.Message)
}

func outside(file string, line, col int, construct, note string) *Error {
	msg := "construct outside subset: " + construct
	if note != "" {
		msg += " (" + note + ")"
	}
	return &Error{Kind: KindOutside, File: file, Line: line, Col: col, Construct: construct, Message: msg}
}

func syntaxErr(file string, line, col int, format string, args ...any) *Error {
	return &Error{Kind: KindSyntax, File: file, Line: line, Col: col, Message: fmt.Sprintf(format, args...)}
}

func semanticErr(file string, line, col int, format string, args ...any) *Error {
	return &Error{Kind: KindSemantic, File: file, Line: line, Col: col, Message: fmt.Sprintf(format, args...)}
}

// Kind of a token.
type Kind int

const (
	EOF Kind = iota
	Ident
	Number
	String
	Punct
	// Directive is a whole preprocessor line: Text is the directive name
	// ("define", "ifdef", …), Args the tokens after it.
	Directive
)

// Token is a lexical token with its position in the original file. Tokens
// produced by macro expansion carry the line of the expansion site.
type Token struct {
	Kind Kind
	Text string
	Val  int64 // Number: the value
	Line int
	Col  int
	Args []Token // Directive only
}

func (t Token) String() string {
	switch t.Kind {
	case EOF:
		return "end of file"
	case String:
		return `"` + t.Text + `"`
	case Directive:
		return "#" + t.Text
	}
	return t.Text
}

func (t Token) is(text string) bool { return t.Kind == Punct && t.Text == text }
func (t Token) isIdent(text string) bool {
	return t.Kind == Ident && t.Text == text
}

// puncts is tried longest first.
var puncts = []string{
	"->", "::", "==", "!=", "<=", ">=", "&&", "||", "++", "--", "<<", ">>", "!!", "??",
	"!", "?", "+", "-", "*", "/", "%", "<", ">", "=", ";", ",", "(", ")", "[", "]", "{", "}", ":", ".", "@", "~", "&", "|", "^",
}

type lexer struct {
	src  string
	file string
	pos  int
	line int
	col  int
	// bol: only whitespace seen so far on this line (a '#' here starts a
	// directive).
	bol bool
}

// Lex tokenises src. Comments are skipped; a '#' at the start of a line
// yields one Directive token spanning the logical line (with '\'
// continuations).
func Lex(src, file string) ([]Token, *Error) {
	lx := &lexer{src: src, file: file, line: 1, col: 1, bol: true}
	var out []Token
	for {
		if err := lx.skipSpace(); err != nil {
			return nil, err
		}
		if lx.pos >= len(lx.src) {
			out = append(out, Token{Kind: EOF, Line: lx.line, Col: lx.col})
			return out, nil
		}
		if lx.bol && lx.src[lx.pos] == '#' {
			d, err := lx.directive()
			if err != nil {
				return nil, err
			}
			out = append(out, d)
			continue
		}
		t, err := lx.token()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
}

func (lx *lexer) advance(n int) {
	for i := 0; i < n && lx.pos < len(lx.src); i++ {
		if lx.src[lx.pos] == '\n' {
			lx.line++
			lx.col = 1
			lx.bol = true
		} else {
			lx.col++
		}
		lx.pos++
	}
}

// skipSpace skips whitespace and comments; it stops at a newline only when
// stopAtNL is set (directive lexing).
func (lx *lexer) skipSpaceUntilNL(stopAtNL bool) *Error {
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		switch {
		case c == '\n':
			if stopAtNL {
				return nil
			}
			lx.advance(1)
		case c == ' ' || c == '\t' || c == '\r' || c == '\f':
			lx.advance(1)
		case c == '\\' && stopAtNL && lx.pos+1 < len(lx.src) && lx.src[lx.pos+1] == '\n':
			lx.advance(2)
		case c == '\\' && stopAtNL && lx.pos+2 < len(lx.src) && lx.src[lx.pos+1] == '\r' && lx.src[lx.pos+2] == '\n':
			lx.advance(3)
		case strings.HasPrefix(lx.src[lx.pos:], "//"):
			for lx.pos < len(lx.src) && lx.src[lx.pos] != '\n' {
				lx.advance(1)
			}
		case strings.HasPrefix(lx.src[lx.pos:], "/*"):
			line, col := lx.line, lx.col
			end := strings.Index(lx.src[lx.pos+2:], "*/")
			if end < 0 {
				return syntaxErr(lx.file, line, col, "unterminated comment")
			}
			lx.advance(end + 4)
		default:
			return nil
		}
	}
	return nil
}

func (lx *lexer) skipSpace() *Error { return lx.skipSpaceUntilNL(false) }

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func (lx *lexer) token() (Token, *Error) {
	lx.bol = false
	c := lx.src[lx.pos]
	t := Token{Line: lx.line, Col: lx.col}
	switch {
	case isIdentStart(c):
		start := lx.pos
		for lx.pos < len(lx.src) && (isIdentStart(lx.src[lx.pos]) || isDigit(lx.src[lx.pos])) {
			lx.advance(1)
		}
		t.Kind, t.Text = Ident, lx.src[start:lx.pos]
		return t, nil
	case isDigit(c):
		start := lx.pos
		for lx.pos < len(lx.src) && (isDigit(lx.src[lx.pos]) || lx.src[lx.pos] == 'x' || lx.src[lx.pos] == 'X' ||
			(lx.src[lx.pos] >= 'a' && lx.src[lx.pos] <= 'f') || (lx.src[lx.pos] >= 'A' && lx.src[lx.pos] <= 'F')) {
			lx.advance(1)
		}
		text := lx.src[start:lx.pos]
		var v int64
		if _, err := fmt.Sscan(text, &v); err != nil {
			return t, syntaxErr(lx.file, t.Line, t.Col, "bad number %q", text)
		}
		t.Kind, t.Text, t.Val = Number, text, v
		return t, nil
	case c == '\'':
		// character constant
		start := lx.pos
		lx.advance(1)
		var v int64
		if lx.pos < len(lx.src) && lx.src[lx.pos] == '\\' && lx.pos+1 < len(lx.src) {
			switch lx.src[lx.pos+1] {
			case 'n':
				v = '\n'
			case 't':
				v = '\t'
			case 'r':
				v = '\r'
			case '0':
				v = 0
			case '\\':
				v = '\\'
			case '\'':
				v = '\''
			default:
				return t, syntaxErr(lx.file, t.Line, t.Col, "bad escape in character constant")
			}
			lx.advance(2)
		} else if lx.pos < len(lx.src) {
			v = int64(lx.src[lx.pos])
			lx.advance(1)
		}
		if lx.pos >= len(lx.src) || lx.src[lx.pos] != '\'' {
			return t, syntaxErr(lx.file, t.Line, t.Col, "unterminated character constant")
		}
		lx.advance(1)
		t.Kind, t.Text, t.Val = Number, lx.src[start:lx.pos], v
		return t, nil
	case c == '"':
		lx.advance(1)
		start := lx.pos
		for lx.pos < len(lx.src) && lx.src[lx.pos] != '"' {
			if lx.src[lx.pos] == '\\' {
				lx.advance(1)
			}
			if lx.pos < len(lx.src) && lx.src[lx.pos] == '\n' {
				return t, syntaxErr(lx.file, t.Line, t.Col, "unterminated string")
			}
			lx.advance(1)
		}
		if lx.pos >= len(lx.src) {
			return t, syntaxErr(lx.file, t.Line, t.Col, "unterminated string")
		}
		t.Kind, t.Text = String, lx.src[start:lx.pos]
		lx.advance(1)
		return t, nil
	}
	for _, p := range puncts {
		if strings.HasPrefix(lx.src[lx.pos:], p) {
			lx.advance(len(p))
			t.Kind, t.Text = Punct, p
			return t, nil
		}
	}
	return t, syntaxErr(lx.file, t.Line, t.Col, "unexpected character %q", string(c))
}

// directive lexes "#name args…" up to the end of the logical line.
func (lx *lexer) directive() (Token, *Error) {
	d := Token{Kind: Directive, Line: lx.line, Col: lx.col}
	lx.advance(1) // '#'
	lx.bol = false
	if err := lx.skipSpaceUntilNL(true); err != nil {
		return d, err
	}
	if lx.pos >= len(lx.src) || !isIdentStart(lx.src[lx.pos]) {
		return d, syntaxErr(lx.file, d.Line, d.Col, "preprocessor directive without a name")
	}
	name, err := lx.token()
	if err != nil {
		return d, err
	}
	d.Text = name.Text
	for {
		if err := lx.skipSpaceUntilNL(true); err != nil {
			return d, err
		}
		if lx.pos >= len(lx.src) || lx.src[lx.pos] == '\n' {
			break
		}
		t, err := lx.token()
		if err != nil {
			return d, err
		}
		d.Args = append(d.Args, t)
	}
	return d, nil
}
