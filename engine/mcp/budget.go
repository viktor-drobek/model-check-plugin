package mcp

import (
	"fmt"

	"modelcheck/cli"
)

// Budget is the client-facing budget of plan 14 §6: states, depth,
// milliseconds of wall time and MiB of estimated memory. Every field is
// optional in a request: absent or 0 means "server default"; in a ceiling
// it means "no ceiling" for that field. (The CLI reads 0 as "unlimited";
// the MCP interface deliberately does not — a client cannot lift a limit,
// only ask for one within the ceiling.) In an answer (budget_applied) a
// field is omitted only when it is 0, i.e. neither a default nor a ceiling
// bounds it. The names are the JSON names the skill uses.
type Budget struct {
	States   int   `json:"states,omitempty" jsonschema:"maximum number of stored states; absent or 0 = server default"`
	Depth    int   `json:"depth,omitempty" jsonschema:"maximum search depth in transitions; absent or 0 = server default"`
	MS       int64 `json:"ms,omitempty" jsonschema:"wall-clock limit in milliseconds; absent or 0 = server default"`
	MemoryMB int64 `json:"memory_mb,omitempty" jsonschema:"memory estimate limit in MiB; absent or 0 = server default"`
}

// DefaultBudget is the "medium model" of plan 14 §12 (A4), the same numbers
// the CLI uses.
var DefaultBudget = Budget{States: cli.DefaultStates, Depth: cli.DefaultDepth, MS: cli.DefaultMS, MemoryMB: cli.DefaultMemMB}

// Clamp applies the server policy to a client request: a field at 0 takes
// the default; a field above a non-zero ceiling is clamped to the ceiling,
// and a note names the field, the request and the ceiling. The default is
// assumed to lie within the ceiling (Config.normalize guarantees it), so a
// defaulted field never produces a note.
func Clamp(req, def, ceiling Budget) (Budget, []string) {
	var notes []string
	clampInt := func(name string, v, d, c int) int {
		if v == 0 {
			return d
		}
		if c > 0 && v > c {
			notes = append(notes, fmt.Sprintf("%s: requested %d exceeds the server ceiling %d; clamped to %d", name, v, c, c))
			return c
		}
		return v
	}
	clamp64 := func(name string, v, d, c int64) int64 {
		if v == 0 {
			return d
		}
		if c > 0 && v > c {
			notes = append(notes, fmt.Sprintf("%s: requested %d exceeds the server ceiling %d; clamped to %d", name, v, c, c))
			return c
		}
		return v
	}
	out := Budget{
		States:   clampInt("states", req.States, def.States, ceiling.States),
		Depth:    clampInt("depth", req.Depth, def.Depth, ceiling.Depth),
		MS:       clamp64("ms", req.MS, def.MS, ceiling.MS),
		MemoryMB: clamp64("memory_mb", req.MemoryMB, def.MemoryMB, ceiling.MemoryMB),
	}
	if notes == nil {
		notes = []string{}
	}
	return out, notes
}

// within clamps def into ceiling without notes (server start-up).
func within(def, ceiling Budget) Budget {
	if ceiling.States > 0 && (def.States == 0 || def.States > ceiling.States) {
		def.States = ceiling.States
	}
	if ceiling.Depth > 0 && (def.Depth == 0 || def.Depth > ceiling.Depth) {
		def.Depth = ceiling.Depth
	}
	if ceiling.MS > 0 && (def.MS == 0 || def.MS > ceiling.MS) {
		def.MS = ceiling.MS
	}
	if ceiling.MemoryMB > 0 && (def.MemoryMB == 0 || def.MemoryMB > ceiling.MemoryMB) {
		def.MemoryMB = ceiling.MemoryMB
	}
	return def
}
