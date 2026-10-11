package ctl

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"modelcheck/internal/testgen"
)

func parseMsg(text string, opt Options) string {
	_, err := Parse(text, opt)
	if err == nil {
		return "ok"
	}
	return err.Error()
}

func doublingDefines(n int) map[string]string {
	d := map[string]string{"a0": "(x == 1 || x == 1)"}
	for i := 1; i <= n; i++ {
		d[fmt.Sprintf("a%d", i)] = fmt.Sprintf("(a%d || a%d)", i-1, i-1)
	}
	return d
}

// TestLimitsCrashScale: the formulas that used to overflow the stack or the
// memory (issue #9), in a child process with a memory cap.
func TestLimitsCrashScale(t *testing.T) {
	testgen.RunInChild(t, "TestLimitsCrashScale", 6000, 90*time.Second, []testgen.Case{
		{Name: "parens-600000", Want: "deeper than 1000", Run: func() string { return parseMsg(testgen.FormulaParens(600000), Options{}) }},
		{Name: "and-chain-1500000", Want: "deeper than 1000", Run: func() string { return parseMsg(testgen.AndChain(1500000), Options{}) }},
		{Name: "impl-chain-1500000", Want: "deeper than 1000", Run: func() string { return parseMsg(testgen.ImplChain(1500000), Options{}) }},
		{Name: "not-chain-3000000", Want: "deeper than 1000", Run: func() string { return parseMsg(testgen.NotChain(3000000), Options{}) }},
		{Name: "au-nest-5000", Want: "deeper than 1000", Run: func() string { return parseMsg(testgen.AUNest(5000, "x == 1"), Options{}) }},
		{Name: "long-atom-1000000", Want: "deeper than 1000", Run: func() string { return parseMsg(testgen.LongAtom(1000000), Options{}) }},
		{Name: "index-nest-600000", Want: "deeper than 1000", Run: func() string { return parseMsg(testgen.FormulaIndexNest(600000), Options{}) }},
		{Name: "not-not-iff-40", Want: "normal form", Run: func() string { return parseMsg("!!("+testgen.IffChain(40)+")", Options{}) }},
		{Name: "not-ax-iff-40", Want: "normal form", Run: func() string { return parseMsg("! AX ("+testgen.IffChain(40)+")", Options{}) }},
		{Name: "iff-40", Want: "normal form", Run: func() string { return parseMsg(testgen.IffChain(40), Options{}) }},
		{Name: "au-nest-14", Want: "normal form", Run: func() string { return parseMsg(testgen.AUNest(14, "x == 1"), Options{}) }},
		{Name: "au-long-atom-10", Want: "normal form", Run: func() string { return parseMsg(testgen.AUNest(10, testgen.LongAtom(900)), Options{}) }},
		{Name: "defines-28", Want: "#define expansion", Run: func() string { return parseMsg("AG a28", Options{Defines: doublingDefines(28)}) }},
	})
}

// TestLimitsBoundary pins the limits: the last accepted size parses and
// normalises, the next one is refused with the limit named.
func TestLimitsBoundary(t *testing.T) {
	// A leaf is height 1: 999 prefix operators over an atom are height
	// 1000, and an atom's comparison is one level over its arithmetic.
	ok := []string{testgen.FormulaParens(1000), testgen.AndChain(1000), testgen.ImplChain(999), testgen.NotChain(999), testgen.LongAtom(998), testgen.IffChain(8), testgen.AUNest(6, "x == 1")}
	for _, f := range ok {
		fm, err := Parse(f, Options{})
		if err != nil {
			t.Errorf("%.40s…: refused: %v", f, err)
			continue
		}
		Normalise(fm)
	}
	refused := []struct{ f, want string }{
		{testgen.FormulaParens(1001), "formula nested or chained deeper than 1000"},
		{testgen.AndChain(1001), "formula nested or chained deeper than 1000"},
		{testgen.ImplChain(1001), "formula nested or chained deeper than 1000"},
		{testgen.NotChain(1001), "formula nested or chained deeper than 1000"},
		{testgen.LongAtom(1001), "atom expression nested or chained deeper than 1000"},
		{testgen.IffChain(20), "normal form"},
		{testgen.AUNest(14, "x == 1"), "normal form"},
		// A negation around a saturated form must not bring it back under
		// the limit (neg folds a Not away).
		{"!!(" + testgen.IffChain(20) + ")", "normal form"},
		{"! AG (" + testgen.IffChain(20) + ")", "normal form"},
	}
	for _, c := range refused {
		got := parseMsg(c.f, Options{})
		if !strings.Contains(got, c.want) || !strings.Contains(got, "limit") {
			t.Errorf("%.40s…: %q, want %q and the limit named", c.f, got, c.want)
		}
	}
}

// TestNormalFormSizeIsExact: the size the parser estimates equals the
// weighted size of Normalise(f), over every formula of height up to three
// on the operators of the grammar.
func TestNormalFormSizeIsExact(t *testing.T) {
	atoms := []*Formula{TrueF(), FalseF(), {Op: AtomOp, Atom: &Atom{Text: "p"}}, {Op: AtomOp, Atom: &Atom{Text: "x + 1 > 0"}}}
	unary := []func(*Formula) *Formula{NotF}
	for _, op := range []Op{AG, AF, AX, EG, EF, EX} {
		op := op
		unary = append(unary, func(f *Formula) *Formula { return Unary(op, f) })
	}
	binary := []func(*Formula, *Formula) *Formula{
		AndF, OrF,
		func(a, b *Formula) *Formula { return &Formula{Op: Impl, L: a, R: b} },
		func(a, b *Formula) *Formula { return &Formula{Op: Iff, L: a, R: b} },
		func(a, b *Formula) *Formula { return Until(AU, a, b) },
		func(a, b *Formula) *Formula { return Until(EU, a, b) },
	}
	level := [][]*Formula{atoms}
	for h := 1; h <= 3; h++ {
		var next []*Formula
		prev := level[h-1]
		for _, f := range prev {
			for _, u := range unary {
				next = append(next, u(f))
			}
		}
		// Pairs over a window of three neighbours keep the enumeration in
		// the tens of thousands while every operator meets every shape.
		for i, f := range prev {
			for j := max(0, i-2); j <= i; j++ {
				for _, b := range binary {
					next = append(next, b(f, prev[j]))
				}
			}
		}
		level = append(level, next)
	}
	n := 0
	for _, fs := range level {
		for _, f := range fs {
			if est, got := normalFormSize(f), weightedSize(Normalise(f)); est != got {
				t.Errorf("%s: Normalise size %d, estimated %d", f, got, est)
			}
			n++
		}
	}
	if n < 1000 {
		t.Errorf("only %d formulas enumerated", n)
	}
}
