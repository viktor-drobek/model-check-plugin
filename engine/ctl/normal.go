package ctl

// Normalisation to the EX / EU / EG basis. The equivalences below are the
// standard ones of CTL (notes 05 "Principles of Model Checking" ch. 6.4 and
// notes 03); they are identities of the semantics, not approximations, and
// they are listed here so that the basis is a stated ground rather than an
// assumption. Every CTL
// formula is equivalent to one built from atoms, `!`, `&&`, `||`, `EX`,
// `E[· U ·]` and `EG` by these equivalences, each of which is an identity of
// CTL semantics, not an approximation:
//
//	EF f      = E[true U f]
//	AX f      = ! EX ! f
//	AF f      = ! EG ! f
//	AG f      = ! E[true U ! f]
//	A[f U g]  = ! E[!g U (!f && !g)] && ! EG !g
//	f -> g    = !f || g
//	f <-> g   = (f && g) || (!f && !g)
//
// The basis is three operators because three fixed points are all the
// labeller needs: EX is one backward step, E[·U·] a least fixed point and
// EG a greatest one. Keeping the normalisation visible (it is printed in
// the report as `temporal.normalised`) is what lets a reader check that the
// engine answered the question that was asked.
//
// Normalise does *not* simplify beyond folding `!true`, `!false` and double
// negation: a smaller formula would be harder to match against the original.

// Normalise rewrites f into the EX / EU / EG basis.
func Normalise(f *Formula) *Formula {
	if f == nil {
		return TrueF()
	}
	switch f.Op {
	case True, False, AtomOp:
		return f
	case Not:
		return neg(Normalise(f.L))
	case And:
		return AndF(Normalise(f.L), Normalise(f.R))
	case Or:
		return OrF(Normalise(f.L), Normalise(f.R))
	case Impl:
		return OrF(neg(Normalise(f.L)), Normalise(f.R))
	case Iff:
		l, r := Normalise(f.L), Normalise(f.R)
		return OrF(AndF(l, r), AndF(neg(l), neg(r)))
	case EX:
		return Unary(EX, Normalise(f.L))
	case EG:
		return Unary(EG, Normalise(f.L))
	case EF:
		return Until(EU, TrueF(), Normalise(f.L))
	case EU:
		return Until(EU, Normalise(f.L), Normalise(f.R))
	case AX:
		return neg(Unary(EX, neg(Normalise(f.L))))
	case AF:
		return neg(Unary(EG, neg(Normalise(f.L))))
	case AG:
		return neg(Until(EU, TrueF(), neg(Normalise(f.L))))
	case AU:
		l, r := Normalise(f.L), Normalise(f.R)
		return AndF(
			neg(Until(EU, neg(r), AndF(neg(l), neg(r)))),
			neg(Unary(EG, neg(r))))
	}
	panic("ctl: unknown operator in Normalise")
}

// neg is `!g` with the three foldings that keep the text readable.
func neg(g *Formula) *Formula {
	switch g.Op {
	case True:
		return FalseF()
	case False:
		return TrueF()
	case Not:
		return g.L
	}
	return NotF(g)
}

// IsNormal reports whether f is already in the basis (used by the labeller
// to refuse a formula it was not given in normal form, so that the two
// stages cannot silently drift apart).
func IsNormal(f *Formula) bool {
	if f == nil {
		return false
	}
	switch f.Op {
	case True, False, AtomOp:
		return true
	case Not, EX, EG:
		return IsNormal(f.L)
	case And, Or, EU:
		return IsNormal(f.L) && IsNormal(f.R)
	}
	return false
}
