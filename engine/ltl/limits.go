package ltl

import (
	"fmt"

	"modelcheck/ir"
)

// The parser's resource limits (issue #9). A formula is untrusted input:
// deep parentheses or long chains build a tree whose height the negation
// normal form and the tableau recurse over, a `#define` of the model that
// doubles per line expands to 2^n tokens when a formula names it, and every
// `<->` doubles the normal form itself. Each limit is measured as the
// formula is built and answered as a parse error — a rejected input of kind
// ltl — that names the limit. The public NNF keeps its contract: it is
// called on formulas that Parse accepted.
const (
	// maxHeight bounds the height of the formula tree and, separately, of
	// every atom's expression tree (a leaf is 1).
	maxHeight = 1000
	// maxDepth bounds the parser's recursion before a node is built: open
	// parentheses, prefix operators and the right-recursive `->`, `<->`,
	// `U`, `V`.
	maxDepth = 1000
	// maxFormulaDefines bounds the tokens that expanding the model's
	// `#define`s inside the formula may produce.
	maxFormulaDefines = 100_000
	// maxNormalForm bounds the weighted size of the negation normal form the
	// tableau reads: an operator weighs 1, an atom the length of its text
	// (the tableau keys its nodes on the text), and the formula is negated
	// first, so the larger of the two forms counts.
	maxNormalForm = 100_000
)

const limitNote = "limit of the engine"

func (p *parser) enter(t token) *Error {
	p.depth++
	if p.depth > maxDepth {
		return p.fail(t, "formula nested or chained deeper than %d levels (%s)", maxDepth, limitNote)
	}
	return nil
}

func (p *parser) leave() { p.depth-- }

// mk records the height of a formula node just built, one more than its
// tallest kid, and refuses a tree above maxHeight. A kid with sub-formulas
// but no recorded height is a construction site this bookkeeping missed.
func (p *parser) mk(t token, f *Formula, kids ...*Formula) (*Formula, *Error) {
	h := 1
	for _, k := range kids {
		if k == nil {
			continue
		}
		kh, ok := p.heights[k]
		if !ok {
			if k.L != nil || k.R != nil {
				return nil, p.fail(t, "internal: a formula node without a recorded height (a defect of mcd, not of the formula)")
			}
			kh = 1
		}
		if kh+1 > h {
			h = kh + 1
		}
	}
	if h > maxHeight {
		return nil, p.fail(t, "formula nested or chained deeper than %d levels (%s)", maxHeight, limitNote)
	}
	p.heights[f] = h
	return f, nil
}

// mkE is mk for the expression of an atom.
func (p *parser) mkE(t token, e *ir.Expr, kids ...*ir.Expr) (*ir.Expr, *Error) {
	h := 1
	for _, k := range kids {
		if k == nil {
			continue
		}
		kh, ok := p.eh[k]
		if !ok {
			if len(k.Args) > 0 {
				return nil, p.fail(t, "internal: an atom expression node without a recorded height (a defect of mcd, not of the formula)")
			}
			kh = 1
		}
		if kh+1 > h {
			h = kh + 1
		}
	}
	if h > maxHeight {
		return nil, p.fail(t, "atom expression nested or chained deeper than %d levels (%s)", maxHeight, limitNote)
	}
	p.eh[e] = h
	return e, nil
}

// normalFormSize is the weighted size of NNF(f) and of NNF(!f) — the larger
// of the two, since the claim negates the formula before the translation —
// computed by the same case analysis as NNF and negate, without building
// either, and saturating at maxNormalForm+1.
func normalFormSize(f *Formula) int64 {
	memo := map[*Formula][2]int64{}
	pos, neg := nfSizes(f, memo)
	return max(pos, neg)
}

// nfSizes returns (|NNF f|, |negate f|) for the weights of normalFormSize.
func nfSizes(f *Formula, memo map[*Formula][2]int64) (int64, int64) {
	if r, ok := memo[f]; ok {
		return r[0], r[1]
	}
	sat := func(v int64) int64 {
		if v > maxNormalForm {
			return maxNormalForm + 1
		}
		return v
	}
	var pos, neg int64
	switch f.Op {
	case True, False:
		pos, neg = 1, 1
	case AtomOp:
		w := int64(len(f.Atom.Text))
		if w < 1 {
			w = 1
		}
		pos, neg = w, w+1 // negate wraps the atom in Not
	case Not:
		pl, nl := nfSizes(f.L, memo)
		pos, neg = nl, pl
	case And, Or, Until, Release:
		pl, nl := nfSizes(f.L, memo)
		pr, nr := nfSizes(f.R, memo)
		pos, neg = 1+pl+pr, 1+nl+nr
	case Impl:
		pl, nl := nfSizes(f.L, memo)
		pr, nr := nfSizes(f.R, memo)
		pos, neg = 1+nl+pr, 1+pl+nr
	case Iff:
		pl, nl := nfSizes(f.L, memo)
		pr, nr := nfSizes(f.R, memo)
		both := 3 + pl + pr + nl + nr
		pos, neg = both, both
	case Next, Always, Eventually:
		pl, nl := nfSizes(f.L, memo)
		pos, neg = 1+pl, 1+nl
	default:
		pos, neg = 1, 1
	}
	pos, neg = sat(pos), sat(neg)
	memo[f] = [2]int64{pos, neg}
	return pos, neg
}

// weightedSize is the weight normalFormSize counts, measured on a built
// formula (tests compare the two).
func weightedSize(f *Formula) int64 {
	if f == nil {
		return 0
	}
	if f.Op == AtomOp {
		w := int64(len(f.Atom.Text))
		if w < 1 {
			w = 1
		}
		return w
	}
	return 1 + weightedSize(f.L) + weightedSize(f.R)
}

// limitError is the error of a formula whose normal form is too large.
func limitError(pos int) *Error {
	return &Error{pos, fmt.Sprintf("the normal form of this formula would have more than %d nodes (each <-> doubles it; %s)", maxNormalForm, limitNote)}
}
