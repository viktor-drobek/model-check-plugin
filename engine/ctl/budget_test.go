package ctl

import (
	"context"
	"errors"
	"testing"
)

// TestLabellingStopsWithTheContext: an evaluation, each fixed point and each
// witness search end with ErrBudget when the context has ended (issue #8),
// and a live context changes nothing.
func TestLabellingStopsWithTheContext(t *testing.T) {
	g := newGraph([][]int32{{0, 1}, {2}, {2}})
	p := []bool{true, true, false}
	q := []bool{false, false, true}
	val := valuation(map[string][]bool{"p": p, "q": q})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	// Every formula here holds or fails in a way that asks for a run, so
	// BuildWitness searches (an EX that fails returns without one).
	for _, text := range []string{"EF q", "EG p", "E[p U q]", "AG p", "AF q"} {
		f, err := Parse(text, Options{})
		if err != nil {
			t.Fatal(err)
		}
		nf := Normalise(f)
		if _, err := LabelCtx(cancelled, g, nf, val); !errors.Is(err, ErrBudget) || !errors.Is(err, context.Canceled) {
			t.Errorf("%s: LabelCtx under a cancelled context: %v, want ErrBudget wrapping context.Canceled", text, err)
		}
		// A labelling built under a live context whose context then ends:
		// the witness search stops too.
		live, cancelLive := context.WithCancel(context.Background())
		lab, err := LabelCtx(live, g, nf, val)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		sat, err := lab.Eval(nf)
		if err != nil {
			t.Fatalf("%s: %v", text, err)
		}
		cancelLive()
		// Every witness search looks at the context on its first step, so
		// the search that the verdict asks for stops at once.
		if _, err := BuildWitness(g, f, lab, sat[0]); !errors.Is(err, ErrBudget) {
			t.Errorf("%s: BuildWitness after the context ended: %v, want ErrBudget", text, err)
		}
	}
	// The fixed points themselves, on a cancelled context.
	if _, err := euSat(cancelled, g, p, q); !errors.Is(err, ErrBudget) {
		t.Errorf("euSat: %v", err)
	}
	if _, err := egSat(cancelled, g, p); !errors.Is(err, ErrBudget) {
		t.Errorf("egSat: %v", err)
	}
	// Label (no context) is LabelCtx under a background context.
	f, _ := Parse("EF q", Options{})
	lab, err := Label(g, Normalise(f), val)
	if err != nil {
		t.Fatal(err)
	}
	if s := lab.Sat(Normalise(f)); !s[0] {
		t.Error("EF q does not hold in s0")
	}
}
