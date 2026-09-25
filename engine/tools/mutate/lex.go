// Package mutate generates first-order mutants of a Promela model for the
// mutation tests of plan 14 §8.1 (checkpoint K3): each mutant is the source
// with exactly one small change, so that a mutant whose verdict does not
// change tells us either that the change was semantically void or that the
// checker missed it. The point of the metric is the main risk of §11 — a
// silent, false `verified`.
//
// The tokenizer below is the mutator's own and deliberately NOT
// frontend/promela's:
//
//   - it must be total. A mutator has to work on text the frontend rejects
//     (the second corpus of §2.3 is full of such text) and on mutants that
//     may not parse; it therefore never fails, and any byte it does not
//     recognise becomes a one-byte punctuation token that no operator
//     touches.
//   - it keeps byte offsets, which the frontend's tokens do not, and the
//     mutation is a byte-range splice.
//   - it keeps this step independent of the frontend changes G5 makes in
//     parallel (plan §9, row G5).
//
// The price is that the mutator does not know the Promela subset: whether a
// mutant is inside it is decided by running the engine on the mutant, not by
// the mutator.
package mutate

import "strconv"

// Kind of a token.
type Kind int

// Token kinds.
const (
	EOF Kind = iota
	Ident
	Number
	String
	Punct
	// Directive is a whole `#…` logical line (with `\` continuations) as one
	// opaque token: no operator mutates inside a macro body or an #if.
	Directive
)

// Token is a lexical token with its byte range and position in the source.
// Off/End are byte offsets into the source; Line and Col are 1-based, Col in
// bytes from the start of the line.
type Token struct {
	Kind      Kind
	Text      string
	Val       int64 // Number only
	Off, End  int
	Line, Col int
}

func (t Token) is(text string) bool { return t.Kind == Punct && t.Text == text }
func (t Token) kw(text string) bool { return t.Kind == Ident && t.Text == text }
func (t Token) oneOf(ws ...string) bool {
	for _, w := range ws {
		if t.kw(w) {
			return true
		}
	}
	return false
}

// puncts is tried longest first; the same table as the frontend's, so that a
// mutation never splits a token the frontend would read as one.
var puncts = []string{
	"->", "::", "==", "!=", "<=", ">=", "&&", "||", "++", "--", "<<", ">>", "!!", "??",
	"!", "?", "+", "-", "*", "/", "%", "<", ">", "=", ";", ",", "(", ")", "[", "]", "{", "}", ":", ".", "@", "~", "&", "|", "^",
}

// Lex tokenises src. Comments and the contents of string literals are not
// returned as anything an operator can reach: a comment is skipped, a string
// is one String token whose Text includes its quotes. Lex never fails.
func Lex(src []byte) []Token {
	var out []Token
	line, lineStart := 1, 0
	i := 0
	bol := true // only blank space seen on this line so far
	col := func(p int) int { return p - lineStart + 1 }
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			i++
			line++
			lineStart = i
			bol = true
			continue
		case c == ' ' || c == '\t' || c == '\r' || c == '\f' || c == '\v':
			i++
			continue
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := i + 2
			for j+1 < len(src) && !(src[j] == '*' && src[j+1] == '/') {
				if src[j] == '\n' {
					line++
					lineStart = j + 1
				}
				j++
			}
			if j+1 < len(src) {
				j += 2
			} else {
				j = len(src)
			}
			i = j
			continue
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		start, startLine, startCol := i, line, col(i)
		switch {
		case bol && c == '#':
			// A directive runs to the end of the logical line.
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) && src[i+1] == '\n' {
					i += 2
					line++
					lineStart = i
					continue
				}
				if src[i] == '\n' {
					break
				}
				i++
			}
			out = append(out, Token{Kind: Directive, Text: string(src[start:i]), Off: start, End: i, Line: startLine, Col: startCol})
		case isIdentStart(c):
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			out = append(out, Token{Kind: Ident, Text: string(src[start:i]), Off: start, End: i, Line: startLine, Col: startCol})
		case c >= '0' && c <= '9':
			for i < len(src) && src[i] >= '0' && src[i] <= '9' {
				i++
			}
			text := string(src[start:i])
			v, _ := strconv.ParseInt(text, 10, 64)
			out = append(out, Token{Kind: Number, Text: text, Val: v, Off: start, End: i, Line: startLine, Col: startCol})
		case c == '"' || c == '\'':
			quote := c
			i++
			for i < len(src) && src[i] != quote {
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				if src[i] == '\n' {
					break // an unterminated literal stops at the line end
				}
				i++
			}
			if i < len(src) && src[i] == quote {
				i++
			}
			out = append(out, Token{Kind: String, Text: string(src[start:i]), Off: start, End: i, Line: startLine, Col: startCol})
		default:
			text := ""
			for _, p := range puncts {
				if len(src)-i >= len(p) && string(src[i:i+len(p)]) == p {
					text = p
					break
				}
			}
			if text == "" {
				// Anything unrecognised (a stray byte, a byte of a cyrillic
				// identifier) is one opaque punctuation token.
				text = string(src[i : i+1])
			}
			i += len(text)
			out = append(out, Token{Kind: Punct, Text: text, Off: start, End: i, Line: startLine, Col: startCol})
		}
		bol = false
	}
	out = append(out, Token{Kind: EOF, Off: len(src), End: len(src), Line: line, Col: col(len(src))})
	return out
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }
