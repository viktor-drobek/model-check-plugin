package explore

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"modelcheck/frontend/promela"
	"modelcheck/ir"
)

// atomicModel is the model of issue #8: 2^40 paths through one atomic block
// and nothing stored inside it, so a budget that fires on stored states
// never fires.
func atomicModel(t *testing.T) *ir.Model {
	t.Helper()
	src, err := os.ReadFile("../testdata/budget/time-budget.pml")
	if err != nil {
		t.Fatal(err)
	}
	res, perr := promela.Parse(src, "time-budget.pml", nil)
	if perr != nil {
		t.Fatal(perr)
	}
	return res.Model
}

// TestTimeBudgetHoldsInsideAnAtomicDescent: every search mode stops at the
// deadline on the atomic model, within a small multiple of the budget, and
// reports the stop as the time budget.
func TestTimeBudgetHoldsInsideAnAtomicDescent(t *testing.T) {
	ltlOnly := func(m *ir.Model) *ir.Model {
		mm := *m
		mm.Properties = []ir.Property{{ID: "ltl1", Kind: ir.KindLTL, Formula: "[] (i < 41)", Text: "[] (i < 41)"}}
		return &mm
	}
	ctlOnly := func(m *ir.Model) *ir.Model {
		mm := *m
		mm.Properties = []ir.Property{{ID: "ctl1", Kind: ir.KindCTL, Formula: "AG (i < 41)", Text: "AG (i < 41)"}}
		return &mm
	}
	progressOnly := func(m *ir.Model) *ir.Model {
		mm := *m
		mm.Properties = []ir.Property{{ID: "progress", Kind: ir.KindProgress}}
		return &mm
	}
	estimate := func(m *ir.Model) *ir.Model {
		mm := *m
		mm.Properties = nil
		return &mm
	}
	cases := []struct {
		name  string
		opt   Options
		model func(*ir.Model) *ir.Model
	}{
		{"dfs", Options{}, nil},
		{"bfs", Options{Mode: BFS}, nil},
		{"por", Options{POR: true}, nil},
		{"parallel-4", Options{Workers: 4}, nil},
		{"ltl-cycle-search", Options{}, ltlOnly},
		{"progress", Options{}, progressOnly},
		{"ctl-graph", Options{}, ctlOnly},
		{"estimate-bfs-sweep", Options{Mode: BFS, Sweep: true, Watch: []*ir.Expr{}}, estimate},
	}
	base := atomicModel(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := base
			if c.model != nil {
				m = c.model(base)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			r, err := Run(ctx, m, c.opt)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			if elapsed > 2*time.Second {
				t.Fatalf("the run took %s under a 100 ms budget", elapsed)
			}
			// A run of temporal properties only keeps its stop in the
			// outcomes (checked below); a safety run or an estimate keeps it
			// in the summary too.
			if r.Complete || (r.Stop != "" && !strings.Contains(r.Stop, "time budget")) {
				t.Fatalf("complete=%v stop=%q, want a stop on the time budget", r.Complete, r.Stop)
			}
			if len(r.Outcomes) == 0 && !strings.Contains(r.Stop, "time budget") {
				t.Fatalf("stop=%q, want the time budget named", r.Stop)
			}
			for _, o := range r.Outcomes {
				if o.Status != Inconclusive || o.Evidence != EvUnknown || !strings.Contains(o.Reason, "time budget") {
					t.Errorf("%s: %s / %s: %s", o.Property.ID, o.Status, o.Evidence, o.Reason)
				}
			}
		})
	}
}

// TestTimeBudgetCountsDStepSteps: a d_step of tens of thousands of steps is
// that much work for the clock, so a budget inside a run of long d_steps
// stops within a small multiple of the budget and not after a thousand of
// them.
func TestTimeBudgetCountsDStepSteps(t *testing.T) {
	src := "int i; int n;\nactive proctype P(){ do :: n < 3000 -> d_step { i = 0; do :: i < 45000 -> i++ :: else -> break od }; n++ :: else -> break od }\n"
	res, perr := promela.Parse([]byte(src), "dstep.pml", nil)
	if perr != nil {
		t.Fatal(perr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, err := Run(ctx, res.Model, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second || !strings.Contains(r.Stop, "time budget") {
		t.Fatalf("took %s, stop %q", elapsed, r.Stop)
	}
}
