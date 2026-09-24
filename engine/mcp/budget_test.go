package mcp

import (
	"strings"
	"testing"
)

func TestClamp(t *testing.T) {
	def := Budget{States: 1000, Depth: 1000, MS: 5000, MemoryMB: 256}
	ceil := Budget{States: 1000, Depth: 1000, MS: 5000, MemoryMB: 256}

	// Zero fields take the default, no notes.
	got, notes := Clamp(Budget{}, def, ceil)
	if got != def || len(notes) != 0 {
		t.Errorf("zero request: %+v %v", got, notes)
	}

	// Within the ceiling: kept, no notes.
	got, notes = Clamp(Budget{States: 10, Depth: 5, MS: 20, MemoryMB: 1}, def, ceil)
	if got != (Budget{States: 10, Depth: 5, MS: 20, MemoryMB: 1}) || len(notes) != 0 {
		t.Errorf("within: %+v %v", got, notes)
	}

	// Above the ceiling: clamped, one note per field naming request and ceiling.
	got, notes = Clamp(Budget{States: 1_000_000, Depth: 5000, MS: 60000, MemoryMB: 4096}, def, ceil)
	if got != ceil {
		t.Errorf("clamped: %+v", got)
	}
	if len(notes) != 4 {
		t.Fatalf("notes: %v", notes)
	}
	for i, field := range []string{"states", "depth", "ms", "memory_mb"} {
		if !strings.HasPrefix(notes[i], field+":") || !strings.Contains(notes[i], "ceiling") {
			t.Errorf("note %d = %q", i, notes[i])
		}
	}
	if !strings.Contains(notes[0], "1000000") || !strings.Contains(notes[0], "clamped to 1000") {
		t.Errorf("states note = %q", notes[0])
	}

	// No ceiling on a field: any value passes.
	got, notes = Clamp(Budget{States: 5_000_000}, def, Budget{})
	if got.States != 5_000_000 || len(notes) != 0 {
		t.Errorf("no ceiling: %+v %v", got, notes)
	}

	// Mixed: only the exceeding field is noted.
	_, notes = Clamp(Budget{States: 2000, Depth: 10}, def, ceil)
	if len(notes) != 1 || !strings.HasPrefix(notes[0], "states:") {
		t.Errorf("mixed: %v", notes)
	}
}

func TestWithin(t *testing.T) {
	ceil := Budget{States: 1000, Depth: 1000, MS: 5000, MemoryMB: 256}
	got := within(DefaultBudget, ceil)
	if got != ceil {
		t.Errorf("defaults above ceiling not clamped: %+v", got)
	}
	got = within(Budget{States: 10}, ceil)
	if got.States != 10 || got.Depth != 1000 {
		t.Errorf("within with zero fields: %+v", got)
	}
	got = within(DefaultBudget, Budget{})
	if got != DefaultBudget {
		t.Errorf("no ceiling changed defaults: %+v", got)
	}
}
