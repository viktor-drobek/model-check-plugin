package mutate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Operator is the name of a mutation operator (plan 14 §8.1).
type Operator string

// The operators, in the rank order that fixes the manifest's generation
// order. Adding one appends to this list; it never reorders it, because the
// mutant ids of a recorded run must keep meaning.
const (
	DropAtomic      Operator = "drop-atomic"
	DropDStep       Operator = "drop-dstep"
	ChanCapMinus    Operator = "chan-cap-minus"
	ChanCapZero     Operator = "chan-cap-zero"
	InvertGuard     Operator = "invert-guard"
	DropEndLabel    Operator = "drop-end-label"
	WeakenAssert    Operator = "weaken-assert"
	SwapRelop       Operator = "swap-relop"
	OffByOne        Operator = "off-by-one"
	DropAlternative Operator = "drop-alternative"
)

// Operators in rank order.
var Operators = []Operator{
	DropAtomic, DropDStep, ChanCapMinus, ChanCapZero, InvertGuard,
	DropEndLabel, WeakenAssert, SwapRelop, OffByOne, DropAlternative,
}

// ParseOperator maps a name to an operator.
func ParseOperator(name string) (Operator, error) {
	for _, op := range Operators {
		if string(op) == name {
			return op, nil
		}
	}
	return "", fmt.Errorf("unknown mutation operator %q (known: %s)", name, strings.Join(names(Operators), ", "))
}

func names(ops []Operator) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = string(op)
	}
	return out
}

// Entry is one line of the manifest: everything about a mutant except its
// text. `Original` is the replaced source text with surrounding whitespace
// trimmed; `Mutated` is its replacement and is empty for a deletion.
type Entry struct {
	ID       int    `json:"id"`
	Operator string `json:"operator"`
	File     string `json:"file"`
	Source   string `json:"source"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
}

// Mutant is an Entry together with the mutated source.
type Mutant struct {
	Entry
	Src []byte `json:"-"`
}

// edit is one byte-range splice of the source.
type edit struct {
	from, to  int
	repl      string
	line, col int
}

// Generate produces the mutants of src in manifest order: the operators in
// the order of Operators (restricted to ops when ops is not empty), and
// within one operator by source position. The ids are 1..n over the whole
// list. `source` is recorded in every entry and is otherwise unused.
func Generate(src []byte, source string, ops ...Operator) []Mutant {
	if len(ops) == 0 {
		ops = Operators
	}
	want := map[Operator]bool{}
	for _, op := range ops {
		want[op] = true
	}
	toks := Lex(src)
	var out []Mutant
	for _, op := range Operators { // rank order, not the caller's order
		if !want[op] {
			continue
		}
		edits := apply(op, src, toks)
		sort.SliceStable(edits, func(i, j int) bool { return edits[i].from < edits[j].from })
		for _, e := range edits {
			mutated := make([]byte, 0, len(src)-(e.to-e.from)+len(e.repl))
			mutated = append(mutated, src[:e.from]...)
			mutated = append(mutated, e.repl...)
			mutated = append(mutated, src[e.to:]...)
			out = append(out, Mutant{
				Entry: Entry{
					ID:       len(out) + 1,
					Operator: string(op),
					Source:   source,
					Line:     e.line,
					Col:      e.col,
					Original: strings.TrimSpace(string(src[e.from:e.to])),
					Mutated:  strings.TrimSpace(e.repl),
				},
				Src: mutated,
			})
		}
	}
	return out
}

func apply(op Operator, src []byte, toks []Token) []edit {
	switch op {
	case DropAtomic:
		return dropKeyword(src, toks, "atomic")
	case DropDStep:
		return dropKeyword(src, toks, "d_step")
	case ChanCapMinus:
		return chanCap(toks, false)
	case ChanCapZero:
		return chanCap(toks, true)
	case InvertGuard:
		return invertGuard(src, toks)
	case DropEndLabel:
		return dropEndLabel(toks)
	case WeakenAssert:
		return weakenAssert(toks)
	case SwapRelop:
		return swapRelop(toks)
	case OffByOne:
		return offByOne(toks)
	case DropAlternative:
		return dropAlternative(toks)
	}
	return nil
}

// dropKeyword removes an `atomic` / `d_step` keyword together with the space
// up to the `{` that follows it: the block stays, its atomicity goes.
func dropKeyword(src []byte, toks []Token, kw string) []edit {
	var out []edit
	for i, t := range toks {
		if !t.kw(kw) || i+1 >= len(toks) || !toks[i+1].is("{") {
			continue
		}
		out = append(out, edit{from: t.Off, to: toks[i+1].Off, repl: "", line: t.Line, col: t.Col})
	}
	return out
}

// chanCap rewrites the N of `[N] of {…}`. toZero replaces it by 0 (a
// rendezvous channel, only for N > 1, since N = 1 → 0 is already the
// chan-cap-minus mutant); otherwise N becomes N-1, for N > 0.
func chanCap(toks []Token, toZero bool) []edit {
	var out []edit
	for i, t := range toks {
		if t.Kind != Number || i == 0 || i+2 >= len(toks) {
			continue
		}
		if !toks[i-1].is("[") || !toks[i+1].is("]") || !toks[i+2].kw("of") {
			continue
		}
		var repl string
		switch {
		case toZero && t.Val > 1:
			repl = "0"
		case !toZero && t.Val > 0:
			repl = strconv.FormatInt(t.Val-1, 10)
		default:
			continue
		}
		out = append(out, edit{from: t.Off, to: t.End, repl: repl, line: t.Line, col: t.Col})
	}
	return out
}

// guardStoppers end the first statement of an option.
func isGuardStop(t Token) bool {
	return t.is("->") || t.is(";") || t.is("::") || t.kw("fi") || t.kw("od") ||
		t.is("}") || t.Kind == EOF || t.Kind == Directive
}

// invertGuard negates the first statement of an if/do option when that
// statement is a bare expression: the option becomes enabled exactly when it
// used to be blocked. Excluded (they are not expressions, or negating them is
// not a Promela expression): `else`, a send or receive, an assignment or
// ++/--, `run`, `atomic`/`d_step`/`{`, `goto`, `break`, `skip`, a nested
// `if`/`do`, a `printf`, an `assert`, and a label.
func invertGuard(src []byte, toks []Token) []edit {
	var out []edit
	for i, t := range toks {
		if !t.is("::") {
			continue
		}
		start := i + 1
		if start >= len(toks) {
			continue
		}
		depth := 0
		stop := start
		for ; stop < len(toks); stop++ {
			s := toks[stop]
			if s.is("(") || s.is("[") {
				depth++
				continue
			}
			if s.is(")") || s.is("]") {
				depth--
				continue
			}
			if depth == 0 && isGuardStop(s) {
				break
			}
		}
		if stop == start {
			continue
		}
		if !isBareExpression(toks[start:stop]) {
			continue
		}
		from, to := toks[start].Off, toks[stop-1].End
		out = append(out, edit{
			from: from, to: to,
			repl: "!(" + string(src[from:to]) + ")",
			line: toks[start].Line, col: toks[start].Col,
		})
	}
	return out
}

// isBareExpression reports whether the tokens are an expression that may be
// wrapped in `!( … )`.
func isBareExpression(ts []Token) bool {
	if len(ts) == 0 {
		return false
	}
	if ts[0].oneOf("else", "atomic", "d_step", "goto", "break", "run", "skip", "if", "do", "printf", "assert", "unless", "timeout") {
		return false
	}
	if ts[0].is("{") || ts[0].Kind == Directive {
		return false
	}
	if len(ts) >= 2 && ts[0].Kind == Ident && ts[1].is(":") {
		return false // a label
	}
	depth := 0
	for j, s := range ts {
		switch {
		case s.is("(") || s.is("["):
			depth++
		case s.is(")") || s.is("]"):
			depth--
		case depth != 0:
		case s.is("="), s.is("++"), s.is("--"):
			return false // assignment
		case s.is("!"), s.is("?"), s.is("!!"), s.is("??"):
			// A send or receive: the operator follows a channel name or an
			// indexed channel. As a prefix (`!x`, `?` never) it is negation.
			if j > 0 && (ts[j-1].Kind == Ident || ts[j-1].is("]")) {
				return false
			}
		case s.is("{"), s.is("}"):
			return false
		case s.Kind == Directive:
			return false
		}
	}
	return true
}

// dropEndLabel removes an `end…:` label. SPIN treats any label whose name
// starts with `end` as a valid end state; removing it turns a legitimate
// blocked state into an invalid end state, which is exactly the kind of
// mistake the deadlock check must catch.
func dropEndLabel(toks []Token) []edit {
	var out []edit
	for i, t := range toks {
		if t.Kind != Ident || !strings.HasPrefix(t.Text, "end") || i+1 >= len(toks) || !toks[i+1].is(":") {
			continue
		}
		if !atStatementStart(toks, i) {
			continue
		}
		to := toks[i+1].End
		if i+2 < len(toks) {
			to = toks[i+2].Off // take the space after the colon too
		}
		out = append(out, edit{from: t.Off, to: to, repl: "", line: t.Line, col: t.Col})
	}
	return out
}

// atStatementStart reports whether toks[i] can begin a statement (and so an
// `ident :` there is a label rather than part of an expression).
func atStatementStart(toks []Token, i int) bool {
	if i == 0 {
		return true
	}
	p := toks[i-1]
	return p.is("{") || p.is("}") || p.is(";") || p.is("->") || p.is("::") || p.is(":") ||
		p.kw("do") || p.kw("od") || p.kw("fi") || p.Kind == Directive
}

// weakenAssert replaces the asserted expression by `true`: the property stops
// being able to fail. A mutant that still reports `violated` on the same
// assert would mean the engine is not reading the assertion it prints.
func weakenAssert(toks []Token) []edit {
	var out []edit
	for i, t := range toks {
		if !t.kw("assert") || i+1 >= len(toks) || !toks[i+1].is("(") {
			continue
		}
		depth := 0
		end := -1
		for j := i + 1; j < len(toks); j++ {
			if toks[j].is("(") {
				depth++
			} else if toks[j].is(")") {
				depth--
				if depth == 0 {
					end = j
					break
				}
			}
		}
		if end < 0 {
			continue
		}
		if end == i+2 { // assert() — nothing to weaken
			continue
		}
		out = append(out, edit{from: t.Off, to: toks[end].End, repl: "assert(true)", line: t.Line, col: t.Col})
	}
	return out
}

var relopSwap = map[string]string{"<": "<=", "<=": "<", ">": ">=", ">=": ">"}

// swapRelop turns a strict comparison into a non-strict one and back: the
// classic off-by-one in a guard.
func swapRelop(toks []Token) []edit {
	var out []edit
	for _, t := range toks {
		if t.Kind != Punct {
			continue
		}
		if repl, ok := relopSwap[t.Text]; ok {
			out = append(out, edit{from: t.Off, to: t.End, repl: repl, line: t.Line, col: t.Col})
		}
	}
	return out
}

// offByOne increments a decimal constant. A constant directly inside `[ ]`
// is skipped: an array size, a channel capacity (which has its own
// operators), an `active [N]` process count or an index — mutating those
// changes the shape of the model rather than its arithmetic.
func offByOne(toks []Token) []edit {
	var out []edit
	for i, t := range toks {
		if t.Kind != Number {
			continue
		}
		if i > 0 && i+1 < len(toks) && toks[i-1].is("[") && toks[i+1].is("]") {
			continue
		}
		out = append(out, edit{from: t.Off, to: t.End, repl: strconv.FormatInt(t.Val+1, 10), line: t.Line, col: t.Col})
	}
	return out
}

// dropAlternative removes one `::` option from an if/do that has at least
// two, so that a behaviour disappears. Options are found by walking the
// if/do nesting: a `::` belongs to the innermost open `if`/`do`.
func dropAlternative(toks []Token) []edit {
	type frame struct {
		opts []int // indices of the `::` tokens of this construct
	}
	var stack []*frame
	var out []edit
	for i, t := range toks {
		switch {
		case t.kw("if") || t.kw("do"):
			stack = append(stack, &frame{})
		case t.kw("fi") || t.kw("od"):
			if len(stack) == 0 {
				continue
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(f.opts) < 2 {
				continue
			}
			for k, o := range f.opts {
				to := toks[i].Off // the last option runs up to fi/od
				if k+1 < len(f.opts) {
					to = toks[f.opts[k+1]].Off
				}
				out = append(out, edit{from: toks[o].Off, to: to, repl: "", line: toks[o].Line, col: toks[o].Col})
			}
		case t.is("::"):
			if len(stack) > 0 {
				f := stack[len(stack)-1]
				f.opts = append(f.opts, i)
			}
		}
	}
	return out
}

// FileName is the name a mutant is written under. It carries the id (which
// is the manifest order) and the operator, so that a path in a report is
// readable on its own.
func (m Mutant) FileName() string {
	return fmt.Sprintf("m%03d-%s.pml", m.ID, m.Operator)
}

// WriteAll writes every mutant into dir together with `manifest.json`, and
// returns the manifest path. The File field of each entry is set to the
// mutant's path (relative to dir) before the manifest is written, so that two
// runs into different directories still give byte-identical manifests.
func WriteAll(dir string, muts []Mutant) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	entries := make([]Entry, 0, len(muts))
	for _, m := range muts {
		name := m.FileName()
		if err := os.WriteFile(filepath.Join(dir, name), m.Src, 0o644); err != nil {
			return "", err
		}
		e := m.Entry
		e.File = name
		entries = append(entries, e)
	}
	path := filepath.Join(dir, "manifest.json")
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
