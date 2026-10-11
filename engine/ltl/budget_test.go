package ltl

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestTranslateStopsWithTheContext: the tableau of a formula that takes
// minutes ends with ErrBudget when the context ends (issue #8), and an
// already-ended context stops it before any work.
func TestTranslateStopsWithTheContext(t *testing.T) {
	f, err := Parse(strings.Repeat("[]<>", 12)+"(x == 1)", Options{})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := TranslateCtx(cancelled, f); !errors.Is(err, ErrBudget) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context: err = %v, want ErrBudget wrapping context.Canceled", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("an already-ended context took %s to stop the translation", d)
	}
	ctx, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	start = time.Now()
	_, err = TranslateCtx(ctx, f)
	if d := time.Since(start); !errors.Is(err, ErrBudget) || d > 3*time.Second {
		t.Fatalf("200 ms budget: err = %v after %s, want ErrBudget within a few seconds", err, d)
	}
	if !strings.Contains(err.Error(), "automaton") {
		t.Errorf("the error does not name the phase: %v", err)
	}
	// Within the budget the automaton is the same as without one.
	g, err := Parse("[] (p -> <> q)", Options{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := TranslateCtx(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	if b := Translate(g); len(a.States) != len(b.States) {
		t.Errorf("TranslateCtx and Translate differ: %d vs %d states", len(a.States), len(b.States))
	}
}
