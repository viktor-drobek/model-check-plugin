package ctl

import (
	"fmt"

	"modelcheck/ir"
)

// The parser's resource limits (issue #9), the counterparts of ltl's: a
// formula is untrusted input; its height, the expansion of the model's
// `#define`s inside it and the size of its normal form are bounded while it
// is built and answered as a parse error — a rejected input of kind ctl —
// that names the limit. Normalise keeps its contract and is called on
// formulas that Parse accepted.
const (
	maxHeight         = 1000
	maxDepth          = 1000
	maxFormulaDefines = 100_000
	// maxNormalForm bounds the weighted size of Normalise(f): an operator
	// weighs 1, an atom the length of its text, because the labelling keys
	// its table on the text of every sub-formula. Every `<->` doubles the
	// form and every A[ p U q ] uses q three times in it.
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

// mk records the height of a formula node just built and refuses a tree
// above maxHeight; see ltl.mk.
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

// nfKind is what the top node of Normalise(f) is, which decides what neg
// costs on it: neg folds True/False and a Not, and wraps anything else.
type nfKind uint8

const (
	nfOther nfKind = iota
	nfBool
	nfNot
)

// nfSize is the weighted size of Normalise(f), by the same case analysis as
// Normalise and neg, without building it, saturating at maxNormalForm+1.
type nfSize struct {
	size int64
	kind nfKind
}

func normalFormSize(f *Formula) int64 {
	return nfSizeOf(f, map[*Formula]nfSize{}).size
}

// negCost is the size of neg(g) for a normalised g of the given size and
// kind, and the kind of the result.
func negCost(g nfSize) nfSize {
	// Saturation is sticky: a size already over the limit stays over it,
	// whatever the negation folds (a `!!` around a huge form must not bring
	// it back under the limit).
	if g.size > maxNormalForm {
		return nfSize{maxNormalForm + 1, nfOther}
	}
	switch g.kind {
	case nfBool:
		return nfSize{1, nfBool}
	case nfNot:
		return nfSize{g.size - 1, nfOther}
	}
	return nfSize{g.size + 1, nfNot}
}

func nfSizeOf(f *Formula, memo map[*Formula]nfSize) nfSize {
	if f == nil {
		return nfSize{1, nfBool}
	}
	if r, ok := memo[f]; ok {
		return r
	}
	sat := func(r nfSize) nfSize {
		if r.size > maxNormalForm {
			r.size = maxNormalForm + 1
		}
		return r
	}
	var r nfSize
	switch f.Op {
	case True, False:
		r = nfSize{1, nfBool}
	case AtomOp:
		w := int64(len(f.Atom.Text))
		if w < 1 {
			w = 1
		}
		r = nfSize{w, nfOther}
	case Not:
		r = negCost(nfSizeOf(f.L, memo))
	case And, Or:
		l, rr := nfSizeOf(f.L, memo), nfSizeOf(f.R, memo)
		r = nfSize{1 + l.size + rr.size, nfOther}
	case Impl:
		l, rr := nfSizeOf(f.L, memo), nfSizeOf(f.R, memo)
		r = nfSize{1 + negCost(l).size + rr.size, nfOther}
	case Iff:
		l, rr := nfSizeOf(f.L, memo), nfSizeOf(f.R, memo)
		r = nfSize{1 + (1 + l.size + rr.size) + (1 + negCost(l).size + negCost(rr).size), nfOther}
	case EX, EG:
		r = nfSize{1 + nfSizeOf(f.L, memo).size, nfOther}
	case EF:
		r = nfSize{2 + nfSizeOf(f.L, memo).size, nfOther}
	case EU:
		l, rr := nfSizeOf(f.L, memo), nfSizeOf(f.R, memo)
		r = nfSize{1 + l.size + rr.size, nfOther}
	case AX, AF:
		// neg(EX/EG(neg X))
		inner := nfSize{1 + negCost(nfSizeOf(f.L, memo)).size, nfOther}
		r = negCost(inner)
	case AG:
		// neg(EU(true, neg X))
		inner := nfSize{2 + negCost(nfSizeOf(f.L, memo)).size, nfOther}
		r = negCost(inner)
	case AU:
		// And(neg(EU(neg r, And(neg l, neg r))), neg(EG(neg r)))
		nl, nr := negCost(nfSizeOf(f.L, memo)), negCost(nfSizeOf(f.R, memo))
		eu := nfSize{1 + nr.size + (1 + nl.size + nr.size), nfOther}
		eg := nfSize{1 + nr.size, nfOther}
		r = nfSize{1 + negCost(eu).size + negCost(eg).size, nfOther}
	default:
		r = nfSize{1, nfBool}
	}
	r = sat(r)
	memo[f] = r
	return r
}

// weightedSize is the weight normalFormSize counts, measured on a built
// formula (tests compare the two on Normalise(f)).
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

func limitError(pos int) *Error {
	return &Error{pos, fmt.Sprintf("the normal form of this formula would have more than %d nodes (each <-> doubles it, each A[..U..] triples its right side; %s)", maxNormalForm, limitNote)}
}
