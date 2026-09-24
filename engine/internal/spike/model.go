// Package spike is the time-boxed calibration explorer of plan 14 §9.
//
// It is deliberately throwaway: models are hard-coded in Go, there is no IR,
// no parser and no counterexample mapping. Its only purpose is to obtain the
// numbers the K1 checkpoint asks for (states/s and bytes per state against
// SPIN's pan) on an engine that is at least demonstrably correct on
// petrinet1 and mutex_flaw. Nothing in this package is imported by the
// product engine.
package spike

// Model is a hard-coded transition system over a flat byte-vector state.
//
// Successors must enumerate the enabled transitions of s in a fixed order
// that depends only on s (determinism, NFR-006). The next slice passed to fn
// may be reused by the model after fn returns; callers copy what they keep.
// A non-empty violated string means that the step itself violated an
// assertion (SPIN semantics: assert is a statement, so the violation belongs
// to the transition, and the target state is the one in which the failed
// assertion was evaluated).
type Model interface {
	Name() string
	Initial() []byte
	Successors(s []byte, fn func(label string, next []byte, violated string))
	// Describe renders s for reports; it must not depend on anything but s.
	Describe(s []byte) string
	// Vars returns selected variables of s by name for tests and reports.
	Vars(s []byte) []Var
}

// Var is a named byte value from a state, used in reports.
type Var struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}
