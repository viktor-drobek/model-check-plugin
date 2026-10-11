package modelcheck_test

import (
	"fmt"

	"modelcheck/internal/testgen"
)

// generatedModel is the Promela model of the resource-limit scenarios
// (issue #9): a shape that amplifies in the frontend, sized by n. The steps
// of g1, g4, g5 and g2 write it to their scratch directory.
func generatedModel(shape string, n int) (string, error) {
	switch shape {
	case "nested-parens":
		return testgen.NestedParens(n), nil
	case "plus-chain":
		return testgen.PlusChain(n), nil
	case "paren-chain":
		return testgen.ParenChain(n, n), nil
	case "wide-mixed":
		return testgen.WideMixed(n), nil
	case "nested-if":
		return testgen.NestedIf(n), nil
	case "if-parens":
		return testgen.IfParens(n), nil
	case "define-doubling":
		return testgen.DefineDoubling(n), nil
	case "define-chain":
		return testgen.DefineChain(n), nil
	case "define-fanout":
		return testgen.DefineFanout(n, n), nil
	case "inline-doubling":
		return testgen.InlineDoubling(n), nil
	case "typedef-fanout":
		return testgen.TypedefFanout(12, n), nil
	case "huge-array-init":
		return testgen.HugeArrayInit(), nil
	case "many-arrays":
		return testgen.ManyArrays(n), nil
	case "chains":
		return testgen.Chains(n, 999), nil
	case "unused-defines":
		return testgen.UnusedDefines(n), nil
	case "inline-fanout":
		return testgen.InlineFanout(n, n), nil
	case "index-nest":
		return testgen.IndexNest(n), nil
	case "chan-arrays":
		return testgen.ChanArrays(n), nil
	case "wide-chans":
		return testgen.WideChans(n, 1000), nil
	}
	return "", fmt.Errorf("no generated model shape %q", shape)
}

// generatedFormula is the LTL or CTL formula of the resource-limit
// scenarios, sized by n; every atom is over the variable x of the generated
// model.
func generatedFormula(shape string, n int) (string, error) {
	switch shape {
	case "parens":
		return testgen.FormulaParens(n), nil
	case "iff-chain":
		return testgen.IffChain(n), nil
	case "and-chain":
		return testgen.AndChain(n), nil
	case "impl-chain":
		return testgen.ImplChain(n), nil
	case "until-chain":
		return testgen.UntilChain(n), nil
	case "not-chain":
		return testgen.NotChain(n), nil
	case "long-atom":
		return testgen.LongAtom(n), nil
	case "au-nest":
		return testgen.AUNest(n, "x == 1"), nil
	case "au-long-atom":
		// An atom within the height limit whose text is long: the labelling
		// keys on the text, and A[..U..] uses its right side three times.
		return testgen.AUNest(n, testgen.LongAtom(900)), nil
	case "define-use-ltl":
		return fmt.Sprintf("[] a%d", n), nil
	case "define-use-ctl":
		return fmt.Sprintf("AG a%d", n), nil
	}
	return "", fmt.Errorf("no generated formula shape %q", shape)
}
