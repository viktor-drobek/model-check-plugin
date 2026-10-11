package ir

import (
	"errors"
	"strings"
	"testing"
)

// chainExpr is `1+1+…+1` of the given height.
func chainExpr(height int) *Expr {
	e := Const(1)
	for i := 1; i < height; i++ {
		e = Binary("add", e, Const(1))
	}
	return e
}

// tinyModel is one process with one location and the given effects.
func tinyModel(effects []Assign, globals []Var, chans []Channel) *Model {
	return &Model{Schema: Schema, Name: "t", Globals: append([]Var{{Name: "x", Type: "byte"}}, globals...), Channels: chans,
		Processes: []Process{{Name: "P", Locations: []Location{{}}, Edges: []Edge{{From: 0, To: 0, Effect: effects}}}}}
}

// TestValidateRefusesTheSizeLimits: an IR document, hand-written or not,
// meets the same bounds as a lowered model, as a LimitError.
func TestValidateRefusesTheSizeLimits(t *testing.T) {
	limit := func(m *Model, want string) {
		t.Helper()
		err := Validate(m)
		var le *LimitError
		if !errors.As(err, &le) || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate = %v, want a LimitError mentioning %q", err, want)
		}
	}
	// weight: 100 effects of height 999 (about 5·10^7)
	var heavy []Assign
	for i := 0; i < 100; i++ {
		heavy = append(heavy, Assign{Var: "x", Value: chainExpr(999)})
	}
	limit(tinyModel(heavy, nil, nil), "the IR weighs")
	// height
	limit(tinyModel([]Assign{{Var: "x", Value: chainExpr(1001)}}, nil, nil), "nested or chained")
	// state slots: arrays
	var arrays []Var
	for i := 0; i < 17; i++ {
		arrays = append(arrays, Var{Name: "a" + strings.Repeat("a", i), Type: "int", Len: 65535})
	}
	limit(tinyModel(nil, arrays, nil), "state vector")
	// state slots: channel buffers count capacity times width
	var wide []Channel
	for i := 0; i < 5; i++ {
		fields := make([]Type, 1000)
		for j := range fields {
			fields[j] = "int"
		}
		wide = append(wide, Channel{Name: "c" + strings.Repeat("c", i), Capacity: 255, Fields: fields})
	}
	limit(tinyModel(nil, nil, wide), "state vector")
	// within the limits: 7 arrays (458 745) and 2 channels (510 002) stay
	// under 1 048 576 slots together
	if err := Validate(tinyModel([]Assign{{Var: "x", Value: chainExpr(1000)}}, arrays[:7], wide[:2])); err != nil {
		t.Errorf("a model within the limits is refused: %v", err)
	}
}
