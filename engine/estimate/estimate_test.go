package estimate

import (
	"context"
	"strings"
	"testing"

	"modelcheck/explore"
	"modelcheck/ir"
)

func levels(pairs ...int) []Level {
	var out []Level
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Level{Depth: pairs[i], States: pairs[i+1]})
	}
	return out
}

func TestGrowthRate(t *testing.T) {
	cases := []struct {
		name  string
		lv    []Level
		want  float64
		basis string
	}{
		{"one level", levels(0, 1), 0, "fewer than two levels"},
		{"doubling", levels(0, 1, 1, 2, 2, 4, 3, 8), 2, "geometric mean of 3"},
		{"saturated", levels(0, 100, 1, 100, 2, 100), 1, "geometric mean of 2"},
		{"last four only", levels(0, 1, 1, 10, 2, 20, 3, 40, 4, 80), 2, "levels 1..4"},
	}
	for _, c := range cases {
		got, basis := growthRate(c.lv)
		if got != c.want {
			t.Errorf("%s: rate %v, want %v", c.name, got, c.want)
		}
		if !strings.Contains(basis, c.basis) {
			t.Errorf("%s: basis %q, want it to mention %q", c.name, basis, c.basis)
		}
	}
}

func TestProjection(t *testing.T) {
	// A complete graph is not extrapolated at all, and a target depth inside
	// the measured range is answered with the measurement, not a projection.
	done := &Result{Complete: true, StatesVisited: 100,
		Growth: Growth{PerLevel: levels(0, 1, 1, 10, 2, 100), Rate: 1}}
	p := project(done, 1)
	if p.Evidence != string(explore.Exhaustive) {
		t.Errorf("a complete graph must carry exhaustive evidence, got %q", p.Evidence)
	}
	if p.StatesAtTarget != 10 {
		t.Errorf("states within depth 1 = %d, want the measured 10", p.StatesAtTarget)
	}
	if !strings.Contains(p.Note, "measured, not projected") {
		t.Errorf("note %q must say the number was measured", p.Note)
	}

	// A partial graph is extrapolated, and the note says so.
	part := &Result{Growth: Growth{PerLevel: levels(0, 1, 1, 2, 2, 4), Rate: 2}}
	p = project(part, 5)
	if p.Evidence != string(explore.Approximate) {
		t.Errorf("an extrapolation must carry approximate evidence, got %q", p.Evidence)
	}
	if p.StatesAtNextLevel != 8 {
		t.Errorf("next level %d, want 8", p.StatesAtNextLevel)
	}
	if p.StatesAtTarget != 32 { // 4 * 2^3
		t.Errorf("states at depth 5 = %d, want 32", p.StatesAtTarget)
	}
	if !strings.Contains(p.Note, "not a law of the model") {
		t.Errorf("note %q must say the factor is a fit", p.Note)
	}

	// Without two levels there is no factor and therefore no projection.
	none := &Result{Growth: Growth{PerLevel: levels(0, 1)}}
	p = project(none, 5)
	if p.StatesAtNextLevel != 0 || !strings.Contains(p.Note, "no growth factor") {
		t.Errorf("projection without a factor: %+v", p)
	}
}

func TestProjectionIsCapped(t *testing.T) {
	r := &Result{Growth: Growth{PerLevel: levels(0, 1, 1, 1000), Rate: 1000}}
	p := project(r, 40)
	if p.StatesAtTarget != projCap {
		t.Fatalf("a runaway factor must be capped, got %d", p.StatesAtTarget)
	}
}

func TestClassifyUsesStatesAndVector(t *testing.T) {
	cases := []struct {
		name   string
		r      *Result
		want   string
		inRec  string
		inBase string
	}{
		{"exact and small", &Result{Complete: true, StatesVisited: 1000, StateBytes: 20}, "small",
			"fits well inside the default budgets", "the exact number of reachable states"},
		{"exact at the small bound", &Result{Complete: true, StatesVisited: SmallStates, StateBytes: 20}, "small", "", ""},
		{"one over the small bound", &Result{Complete: true, StatesVisited: SmallStates + 1, StateBytes: 20}, "medium",
			"with little room", ""},
		{"over the medium bound", &Result{Complete: true, StatesVisited: MediumStates + 1, StateBytes: 20}, "large",
			"bound the model", ""},
		{"wide vector", &Result{Complete: true, StatesVisited: 10, StateBytes: MediumVector + 1}, "large",
			"state vector above", ""},
		{"projected", &Result{StatesVisited: 10, StateBytes: 20,
			Growth:     Growth{PerLevel: levels(0, 1, 1, 10), Rate: 2},
			Projection: Projection{StatesAtNextLevel: MediumStates + 5}}, "large",
			"", "the projection to depth"},
	}
	for _, c := range cases {
		s := classify(c.r)
		if s.Class != c.want {
			t.Errorf("%s: class %q, want %q (basis: %s)", c.name, s.Class, c.want, s.Basis)
		}
		if c.inRec != "" && !strings.Contains(s.Recommendation, c.inRec) {
			t.Errorf("%s: recommendation %q must mention %q", c.name, s.Recommendation, c.inRec)
		}
		if c.inBase != "" && !strings.Contains(s.Basis, c.inBase) {
			t.Errorf("%s: basis %q must mention %q", c.name, s.Basis, c.inBase)
		}
		if !strings.Contains(s.Recommendation, "plan 14 §12") || !strings.Contains(s.Basis, "plan 14 §12") {
			t.Errorf("%s: the class must name the bounds it comes from", c.name)
		}
	}
}

// TestRunOnACompleteModel: a graph that fits in the budget is measured, not
// projected, and every level is exact.
func TestRunOnACompleteModel(t *testing.T) {
	// A single process counting 0..4, so the reachable graph is a chain of
	// five states and the levels are 1, 2, 3, 4, 5.
	m := &ir.Model{Schema: ir.Schema, Name: "chain",
		Globals: []ir.Var{{Name: "c", Type: ir.Byte, Max: max4()}},
		Processes: []ir.Process{{
			Name:      "P",
			Locations: []ir.Location{{}},
			Edges: []ir.Edge{{From: 0, To: 0, Guard: ir.Binary("lt", ir.Ref("c"), ir.Const(4)),
				Effect: []ir.Assign{{Var: "c", Value: ir.Binary("add", ir.Ref("c"), ir.Const(1))}}}},
		}}}
	res, err := Run(context.Background(), m, Options{TimeLimitMS: 5000, TargetDepth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Complete || res.StatesVisited != 5 {
		t.Fatalf("complete=%v states=%d, want a complete graph of 5 states", res.Complete, res.StatesVisited)
	}
	want := []Level{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}}
	if len(res.Growth.PerLevel) != len(want) {
		t.Fatalf("levels %v, want %v", res.Growth.PerLevel, want)
	}
	for i, l := range res.Growth.PerLevel {
		if l != want[i] {
			t.Fatalf("levels %v, want %v", res.Growth.PerLevel, want)
		}
	}
	if res.Projection.Evidence != string(explore.Exhaustive) {
		t.Errorf("projection evidence %q", res.Projection.Evidence)
	}
	if res.Projection.StatesAtTarget != 3 {
		t.Errorf("states within depth 2 = %d, want 3", res.Projection.StatesAtTarget)
	}
	if res.Size.Class != "small" {
		t.Errorf("size class %q", res.Size.Class)
	}
	if !strings.Contains(res.Note, "not a verification result") {
		t.Errorf("note %q", res.Note)
	}
}

// TestRunIgnoresTheModelsProperties: the estimate measures the state space,
// so a temporal property must not start a product search of its own.
func TestRunIgnoresTheModelsProperties(t *testing.T) {
	m := &ir.Model{Schema: ir.Schema, Name: "p",
		Globals:   []ir.Var{{Name: "c", Type: ir.Bit}},
		Processes: []ir.Process{{Name: "P", Locations: []ir.Location{{}}, Edges: []ir.Edge{{From: 0, To: 0}}}},
		Properties: []ir.Property{
			{ID: "bad", Kind: ir.KindLTL, Formula: "[] nosuchvariable"},
		}}
	res, err := Run(context.Background(), m, Options{TimeLimitMS: 1000})
	if err != nil {
		t.Fatalf("an unresolvable formula must not reach the estimate: %v", err)
	}
	if res.StatesVisited != 1 {
		t.Fatalf("states %d, want the single state of the model", res.StatesVisited)
	}
}

func max4() *int64 { v := int64(4); return &v }
