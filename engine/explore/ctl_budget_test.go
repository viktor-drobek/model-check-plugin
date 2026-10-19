package explore

import (
	"context"
	"strings"
	"testing"

	"modelcheck/ctl"
	"modelcheck/frontend/promela"
	"modelcheck/ir"
)

// TestCTLLabellingStopsAtTheTimeBudget: a deadline that passes after the
// graph is complete stops the labelling with the phase named, inconclusive
// and not complete (issue #8); one that passes after the labelling keeps
// the verdict and only leaves the witness out.
func TestCTLLabellingStopsAtTheTimeBudget(t *testing.T) {
	res, perr := promela.Parse([]byte("byte x;\nactive proctype P(){ do :: x = 1 :: x = 0 od }\n"), "tiny-loop.pml", nil)
	if perr != nil {
		t.Fatal(perr)
	}
	m := res.Model
	m.Properties = []ir.Property{{ID: "ctl1", Kind: ir.KindCTL, Formula: "AG (x <= 1)", Text: "AG (x <= 1)"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := Run(ctx, m, Options{ctlAfterGraph: cancel})
	if err != nil {
		t.Fatal(err)
	}
	o := outcome(t, r, "ctl1")
	if o.Status != Inconclusive || o.Evidence != EvUnknown || o.Reason != ctl.ErrBudget.Error() {
		t.Fatalf("%s / %s: %s", o.Status, o.Evidence, o.Reason)
	}
	if o.Stats == nil || o.Stats.Complete || o.Stats.Stop != ctl.ErrBudget.Error() {
		t.Fatalf("stats %+v, want complete false and the labelling named as the stop", o.Stats)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	m.Properties = []ir.Property{{ID: "ctl1", Kind: ir.KindCTL, Formula: "EF (x == 1)", Text: "EF (x == 1)"}}
	r, err = Run(ctx2, m, Options{ctlAfterLabel: cancel2})
	if err != nil {
		t.Fatal(err)
	}
	o = outcome(t, r, "ctl1")
	if o.Status != Verified || o.Evidence != Exhaustive {
		t.Fatalf("the verdict did not survive a witness cut by the budget: %s / %s: %s", o.Status, o.Evidence, o.Reason)
	}
	if o.Trace != nil || o.Temporal == nil || !strings.Contains(o.Temporal.WitnessNote, "witness not built") {
		t.Fatalf("trace %v, note %q: want no trace and the note", o.Trace, o.Temporal.WitnessNote)
	}
}

func TestCTLVacuityBudgetKeepsDecidedVerdict(t *testing.T) {
	res, perr := promela.Parse([]byte("byte x;\nactive proctype P(){ do :: x = 1 :: x = 0 od }\n"), "tiny-loop.pml", nil)
	if perr != nil {
		t.Fatal(perr)
	}
	m := res.Model
	m.Properties = []ir.Property{{ID: "ctl1", Kind: ir.KindCTL, Formula: "AG (x <= 1)", Text: "AG (x <= 1)"}}
	baseline, err := Run(context.Background(), m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := outcome(t, baseline, "ctl1")
	hasVacuityWarning := false
	for _, warning := range want.Warnings {
		if strings.HasPrefix(warning, "vacuity hint for") {
			hasVacuityWarning = true
			break
		}
	}
	if !hasVacuityWarning {
		t.Fatalf("baseline must produce a vacuity warning for this test; got %q", want.Warnings)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := Run(ctx, m, Options{ctlAfterWitness: cancel})
	if err != nil {
		t.Fatal(err)
	}
	o := outcome(t, r, "ctl1")
	if o.Status != want.Status || o.Evidence != want.Evidence {
		t.Fatalf("vacuity budget changed the verdict to %s / %s: %s", o.Status, o.Evidence, o.Reason)
	}
	if (o.Trace == nil) != (want.Trace == nil) {
		t.Fatalf("vacuity budget changed trace availability from %v to %v", want.Trace != nil, o.Trace != nil)
	}
	if len(o.Warnings) != 0 {
		t.Fatalf("vacuity warnings %v, want optional hints omitted after the budget", o.Warnings)
	}
}
