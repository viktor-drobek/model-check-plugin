package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"modelcheck/cex"
	"modelcheck/explore"
	"modelcheck/ir"
)

func model() *ir.Model {
	return &ir.Model{Schema: ir.Schema, Name: "m",
		Globals:   []ir.Var{{Name: "x", Type: ir.Byte}},
		Processes: []ir.Process{{Name: "P", Locations: []ir.Location{{}}, Edges: []ir.Edge{{}}}}}
}

func result(outcomes ...explore.Outcome) *explore.Result {
	return &explore.Result{Outcomes: outcomes, States: 3, Transitions: 4, MaxDepth: 2, MemBytes: 100,
		Elapsed: 7 * time.Millisecond, StateBytes: 3, Complete: true, Stop: "complete"}
}

func prop(id, kind string) ir.Property { return ir.Property{ID: id, Kind: kind} }

func TestBuildAcceptsEveryStatusOfThePartition(t *testing.T) {
	tr := &cex.Trace{Summary: "t1"}
	res := result(
		explore.Outcome{Property: prop("a", ir.KindDeadlock), Status: explore.Verified, Evidence: explore.Exhaustive},
		explore.Outcome{Property: prop("b", ir.KindInvariant), Status: explore.Violated, Evidence: explore.Exhaustive, Trace: tr},
		explore.Outcome{Property: prop("c", "ltl"), Status: explore.NotExecuted, Evidence: explore.EvUnknown, Reason: "ltl"},
		explore.Outcome{Property: prop("d", ir.KindReach), Status: explore.Verified, Evidence: explore.Exhaustive, Trace: tr},
	)
	r, err := Build(model(), res, Meta{Mode: explore.DFS})
	if err != nil {
		t.Fatal(err)
	}
	if r.Properties[1].Counterexample != tr || r.Properties[3].Witness != tr || r.Properties[3].Counterexample != nil {
		t.Fatal("trace placement")
	}
	if r.Properties[0].Counters.TimeMS == nil || *r.Properties[0].Counters.TimeMS != 7 {
		t.Fatal("time_ms")
	}
	b, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"engine", "inputs", "model", "search", "properties"} {
		if _, ok := back[k]; !ok {
			t.Fatalf("missing %s", k)
		}
	}
	if !strings.Contains(string(b), `"version": "`+EngineVersion+`"`) {
		t.Fatal("engine version")
	}
}

func TestBuildRefusesVerifiedWhenIncomplete(t *testing.T) {
	res := result(explore.Outcome{Property: prop("a", ir.KindDeadlock), Status: explore.Verified, Evidence: explore.Exhaustive})
	res.Complete = false
	if _, err := Build(model(), res, Meta{}); err == nil || !strings.Contains(err.Error(), "not complete") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func TestBuildRefusesWrongEvidence(t *testing.T) {
	cases := []explore.Outcome{
		{Property: prop("a", ir.KindDeadlock), Status: explore.Verified, Evidence: explore.Bounded},
		{Property: prop("a", ir.KindDeadlock), Status: explore.Violated, Evidence: explore.Bounded},
		{Property: prop("a", ir.KindDeadlock), Status: explore.InvalidModel, Evidence: explore.Exhaustive, Reason: "x"},
		{Property: prop("a", ir.KindDeadlock), Status: explore.Unknown, Evidence: explore.EvUnknown},
		{Property: prop("a", ir.KindDeadlock), Status: "maybe"},
	}
	for _, o := range cases {
		if _, err := Build(model(), result(o), Meta{}); err == nil {
			t.Errorf("%s/%s accepted", o.Status, o.Evidence)
		}
	}
	// inconclusive on a complete search is contradictory too
	res := result(explore.Outcome{Property: prop("a", ir.KindDeadlock), Status: explore.Inconclusive, Evidence: explore.Bounded, Reason: "budget"})
	if _, err := Build(model(), res, Meta{}); err == nil {
		t.Error("inconclusive with complete=true accepted")
	}
	res.Complete = false
	if _, err := Build(model(), res, Meta{}); err != nil {
		t.Error(err)
	}
}

func TestNoTimingIsDeterministic(t *testing.T) {
	mk := func(d time.Duration) []byte {
		res := result(explore.Outcome{Property: prop("a", ir.KindDeadlock), Status: explore.Verified, Evidence: explore.Exhaustive})
		res.Elapsed = d
		r, err := Build(model(), res, Meta{NoTiming: true, Inputs: []Input{{Kind: "ir", Path: "p", SHA256: "00"}}})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := r.JSON()
		return b
	}
	a, b := mk(time.Millisecond), mk(time.Second)
	if !bytes.Equal(a, b) {
		t.Fatal("reports differ with --no-timing")
	}
	if bytes.Contains(a, []byte("time_ms\": ")) && !bytes.Contains(a, []byte(`"time_ms": 60000`)) {
		// only the budget echo may mention time_ms
		if strings.Count(string(a), "time_ms") != 1 {
			t.Fatalf("time_ms leaked into counters:\n%s", a)
		}
	}
}
