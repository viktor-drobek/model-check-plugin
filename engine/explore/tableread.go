package explore

import (
	"fmt"

	"modelcheck/ctl"
	"modelcheck/ir"
)

// A property that reads the live-process table.
//
// `_nr_pr` (and, in a hand-written IR, a runtime pid or the youngest-process
// test) is answered by the table in the state vector, and the table is only
// right when the processes keep it: `run` enters a process, and the `-end-`
// edge of every process leaves it (ir.Edge.Leave), which is what makes the
// count fall. The Promela frontend writes those edges when a process reads
// `_nr_pr` or creates one with `run`; whether the vector carries a table is
// decided by the model's processes alone (ir.NeedsTable).
//
// A property is read after the processes were written, so it cannot ask for
// them to be written differently. Over a model whose processes keep no table
// `_nr_pr` is a count that never falls, and a verdict about it — `verified`
// for `invariant _nr_pr == 2`, `violated` for `reach _nr_pr == 0` — would
// describe another model, with exhaustive evidence. Such a property is refused,
// each one by itself: `not-executed`, evidence `unknown`, with the way out.
// It is not evaluated, it adds nothing to the vector, and it does not decide
// whether the others are refused.

// tableReadReason is the answer for a property whose `what` (the formula of a
// CTL property, the expression of an invariant or a reach) reads `read`
// although the model keeps no live-process table.
func tableReadReason(what string, read *ir.Expr) string {
	return fmt.Sprintf("the %s reads %s, which only the live-process table answers, but this model keeps no such table: its processes do not leave a table when they end, so the value would stay at the number of processes started and the verdict would not describe the model; a model that reads _nr_pr itself (or creates processes with run) keeps the table, so put a read of _nr_pr into the model, for instance assert(_nr_pr >= 0) in one process, and check the property again",
		what, read)
}

// tableReadRefusal returns the reason the property p must be refused, or ""
// when it may be checked: its expression (invariant, reach) reads the table
// while the layout carries none. Temporal properties are asked where their
// formula is read (runCTL; an LTL atom cannot read the table at all).
func tableReadRefusal(l *ir.Layout, p *ir.Property) string {
	if l.HasTable() || (p.Kind != ir.KindInvariant && p.Kind != ir.KindReach) {
		return ""
	}
	if read := p.Expr.TableRead(); read != nil {
		return tableReadReason("expression", read)
	}
	return ""
}

// formulaTableRefusal returns the reason a CTL formula must be refused, or ""
// when it may be checked: one of its atoms reads the table while the layout
// carries none.
func formulaTableRefusal(l *ir.Layout, f *ctl.Formula) string {
	if l.HasTable() {
		return ""
	}
	for _, a := range f.Atoms() {
		if read := a.Expr.TableRead(); read != nil {
			return tableReadReason("formula", read)
		}
	}
	return ""
}

// TableReadRefusal is the reason a check over the layout l refuses the
// invariant or reach property p for reading the live-process table, or "".
// mc_lint_property asks it, so that its note and the answer of mc_check come
// from one decision.
func TableReadRefusal(l *ir.Layout, p *ir.Property) string { return tableReadRefusal(l, p) }

// TableReadRefusalCTL is the same for a parsed CTL formula.
func TableReadRefusalCTL(l *ir.Layout, f *ctl.Formula) string { return formulaTableRefusal(l, f) }
