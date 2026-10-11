package promela

import (
	"fmt"

	"modelcheck/ir"
)

// The frontend's resource limits (issue #9). Model text is untrusted input,
// and several of its shapes amplify: nesting and operator chains build a
// tree whose height the lowering recurses over, macro, inline and typedef
// definitions can double per line, an array can ask for a billion elements,
// and many deep expressions make an IR file of gigabytes. Each of these used
// to end the process with a fatal error that no recover sees (a stack
// overflow or an out-of-memory is not a panic), and a running `mcd serve`
// with it. The limits below are measured as the input is built, bottom-up,
// and answered as an `outside-subset` rejection that names the limit; every
// input within them behaves as before. They sit far above any real model:
// the corpus nests a few dozen deep, and the crash needs a hundred times the
// limit on a 64-bit stack (twenty-five times on 32-bit).
const (
	// maxHeight bounds the height of an expression tree (a leaf is 1): the
	// parser's own recursion, the lowering's and the IR compiler's all follow
	// it, and `ir.Compile` costs about a second at this height.
	maxHeight = 1000
	// maxDepth bounds the parser's recursion before a node is built: open
	// parentheses and prefix operators on the way down.
	maxDepth = 1000
	// maxBlockDepth bounds nested statement blocks: `{}`, `if`, `do`,
	// `atomic`, `d_step` and the bodies of their options.
	maxBlockDepth = 1000
	// maxExpansion bounds the tokens that macro expansion, and separately
	// inline expansion, may produce for one file; a megabyte of Promela is
	// about half a million tokens.
	maxExpansion = 4_000_000
	// maxExpansionDepth bounds the rescans a macro expansion nests.
	maxExpansionDepth = 1000
	// maxFields bounds the flattened fields of typedef instances over the
	// whole module: a typedef that doubles per level has 2^n of them.
	maxFields = 100_000
	// maxArrayLen is the IR's bound on an array; it is enforced here as soon
	// as the length is known, before any per-element work.
	maxArrayLen = 65535
	// maxChanArrayLen bounds an array of channels: a channel id is stored
	// in a byte, so the IR refuses more anyway, after creating them all.
	maxChanArrayLen = 255
)

// The IR weight (ir.MaxIRWeight) and the state-vector size
// (ir.MaxStateSlots) are the IR's own limits; the parser counts the slots
// as declarations are read, before the lowering builds anything per element,
// and the lowering checks the weight before the document is written.

// limitNote is the second half of every limit message.
const limitNote = "limit of the engine, not a syntax error"

// enter counts one level of the parser's own recursion (an open parenthesis,
// a prefix operator) and refuses the maxDepth-th plus one; every enter has
// its leave on the same path.
func (p *parser) enter(t Token) *Error {
	p.depth++
	if p.depth > maxDepth {
		return outside(p.file, t.Line, t.Col, fmt.Sprintf("an expression nested or chained deeper than %d levels", maxDepth), limitNote)
	}
	return nil
}

// declSlots is the state-vector slots one declaration takes: a scalar is
// one, an array one per element, and a channel also holds its field table
// and its buffer (message width times capacity plus one) per element.
func declSlots(d *VarDecl) int64 {
	per := int64(max(d.Len, 1))
	if d.Chan != nil {
		per *= 1 + int64(len(d.Chan.Fields))*int64(d.Chan.Cap+1)
	}
	return per
}

// addSlots counts the state-vector slots a declaration takes — for the
// globals of the module, or for one instance of the process being parsed —
// and refuses the model once the IR's bound is reached by either, before
// any per-element work. The lowering adds the instances up (lower.go).
func (p *parser) addSlots(t Token, n int64) *Error {
	total := &p.slots
	if p.curProc != nil {
		total = &p.curProc.Slots
	}
	*total += n
	if *total > ir.MaxStateSlots {
		return outside(p.file, t.Line, t.Col, fmt.Sprintf("the state vector of this model would have more than %d slots", ir.MaxStateSlots), limitNote)
	}
	return nil
}

// maxFieldNameBytes bounds the total length of the flattened field names of
// typedef instances: a chain of typedefs makes names that grow with the
// chain, quadratically in all.
const maxFieldNameBytes = 4_000_000

func (p *parser) leave() { p.depth-- }

// enterBlock and leaveBlock count nested statement blocks.
func (p *parser) enterBlock(t Token) *Error {
	p.blockDepth++
	if p.blockDepth > maxBlockDepth {
		return outside(p.file, t.Line, t.Col, fmt.Sprintf("statements nested deeper than %d levels", maxBlockDepth), limitNote)
	}
	return nil
}

func (p *parser) leaveBlock() { p.blockDepth-- }

// built records the height of a freshly constructed node — one more than
// its tallest kid — and refuses a tree above maxHeight at the token t. The
// loops of the binary operators pass their running left operand as a kid,
// so a chain of n operators is a tree of height n, however it is
// parenthesised; width is not summed. A kid that has children of its own
// but no recorded height is a construction site this bookkeeping missed: an
// internal error, reported rather than ignored.
func (p *parser) built(t Token, node Expr, kids ...Expr) (Expr, *Error) {
	h := 1
	for _, k := range kids {
		if k == nil {
			continue
		}
		kh, ok := p.heights[k]
		if !ok {
			if hasKids(k) {
				return nil, semanticErr(p.file, t.Line, t.Col, "internal: an expression node of type %T has no recorded height (a defect of mcd, not of the model)", k)
			}
			kh = 1
		}
		if kh+1 > h {
			h = kh + 1
		}
	}
	if h > maxHeight {
		return nil, outside(p.file, t.Line, t.Col, fmt.Sprintf("an expression nested or chained deeper than %d levels", maxHeight), limitNote)
	}
	p.heights[node] = h
	return node, nil
}

// hasKids reports whether an expression node has sub-expressions.
func hasKids(e Expr) bool {
	switch x := e.(type) {
	case *Unary, *Binary, *IndexExpr, *PCValueExpr:
		return true
	case *ChanExpr:
		return x.Index != nil
	}
	return false
}
