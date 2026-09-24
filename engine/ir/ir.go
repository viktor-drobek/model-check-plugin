// Package ir is the backend-neutral intermediate representation of plan 14
// §5.1: variables with finite domains, processes as control-flow graphs whose
// edges are guarded commands, channels, labels on control locations, and the
// mapping of every element back to the user's file, line and name.
//
// The IR is what every frontend produces (Petri JSON in G0, the Promela
// subset in G1) and what the explorer runs. Its JSON form is the exchange
// format of `mcd parse` / `mcd check --ir`. The state layout derived from it
// (see Layout) is fixed now so that G1 can target the IR without changing how
// states are stored or counted.
//
// Design decisions recorded here because later steps depend on them:
//
//   - Every Process is one instance. `active [N]` is expanded by the frontend.
//   - An Edge is one indivisible step: guard evaluated in the source state,
//     then the assert, then the effect applied sequentially (each assignment
//     reads the partially updated state, as `x--; y++` does in Promela).
//   - Edge.Atomic means "the process keeps exclusive control after this
//     edge", i.e. the edge is a statement inside a Promela `atomic {}`
//     sequence that has further statements after it. The last statement of a
//     sequence, and any single-edge command, has Atomic = false: there is
//     nothing left to run exclusively. A blocked process inside a sequence
//     loses exclusivity (Promela semantics, 14 §5.2). The holder of exclusive
//     control is part of the state vector (Layout.Excl).
//   - Origin is embedded in each element instead of a separate table keyed by
//     element paths: the "mapping table" of §5.1 is then the set of Origin
//     fields, and a consumer holding an element never has to look it up.
//   - Properties travel with the model so that a serialised IR is a complete
//     verification case (12 §2.1).
package ir

// Schema is the identifier written into Model.Schema. It changes only when
// the JSON shape or the state layout changes incompatibly.
const Schema = "mcd-ir/1"

// Type is a scalar variable type. All are integers with a fixed domain.
type Type string

const (
	Bit   Type = "bit"
	Bool  Type = "bool"
	Byte  Type = "byte"
	Short Type = "short"
	Int   Type = "int"
)

// Bounds returns the domain of t. ok is false for an unknown type.
func (t Type) Bounds() (min, max int64, ok bool) {
	switch t {
	case Bit, Bool:
		return 0, 1, true
	case Byte:
		return 0, 255, true
	case Short:
		return -32768, 32767, true
	case Int:
		return -2147483648, 2147483647, true
	}
	return 0, 0, false
}

// Width is the number of bytes one element of type t occupies in the state
// vector. Values are stored little-endian, two's complement for signed types.
func (t Type) Width() int {
	switch t {
	case Short:
		return 2
	case Int:
		return 4
	}
	return 1
}

// Origin maps an IR element back to the user's artefact (14 §5.1, FR-015).
type Origin struct {
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
	Name string `json:"name,omitempty"` // the user's own name for the element
}

// Var is a scalar or fixed-length array variable.
type Var struct {
	Name string `json:"name"`
	Type Type   `json:"type"`
	// Len > 0 declares an array of Len elements; 0 declares a scalar.
	Len int `json:"len,omitempty"`
	// Min/Max narrow the type's domain (e.g. a Petri place with capacity 3 is
	// a byte with Max = 3). Leaving a variable's value outside [Min, Max] is a
	// domain overflow, reported as invalid-model.
	Min *int64 `json:"min,omitempty"`
	Max *int64 `json:"max,omitempty"`
	// Init holds the initial value(s): one for a scalar, Len for an array.
	// Missing means all zero.
	Init   []int64 `json:"init,omitempty"`
	Origin *Origin `json:"origin,omitempty"`
}

// Domain returns the effective [min, max] of v: the type bounds narrowed by
// Min/Max.
func (v *Var) Domain() (min, max int64) {
	min, max, _ = v.Type.Bounds()
	if v.Min != nil && *v.Min > min {
		min = *v.Min
	}
	if v.Max != nil && *v.Max < max {
		max = *v.Max
	}
	return min, max
}

// Count is the number of scalar slots v occupies (1 for a scalar).
func (v *Var) Count() int {
	if v.Len > 0 {
		return v.Len
	}
	return 1
}

// Channel is a message channel. Capacity 0 is a rendezvous channel. Its
// room in the state vector is one length byte plus Capacity messages of
// the summed field widths (a rendezvous channel takes the length byte
// only: a handshake is a single step and never leaves a message stored).
// Operations are Edge.Send and Edge.Recv (G1).
type Channel struct {
	Name     string  `json:"name"`
	Capacity int     `json:"capacity"`
	Fields   []Type  `json:"fields"`
	Origin   *Origin `json:"origin,omitempty"`
	// XR / XS record Promela `xr` / `xs` hints: the process types that
	// alone receive from / send to the channel. They are stored for the
	// partial-order reduction of a later version and change nothing in the
	// search (plan 14 §5.2).
	XR []string `json:"xr,omitempty"`
	XS []string `json:"xs,omitempty"`
}

// ChanOp is a send: Args, one per channel field, are evaluated in the
// source state (after the edge's Run, before its Effect) and appended to
// the buffer. On a buffered channel the edge is enabled only when the
// buffer is not full; on a rendezvous channel (capacity 0) only when some
// other process has, at its current location, an enabled Recv edge on the
// same channel whose Match values equal the sent values — the two edges are
// then one indivisible step (the handshake) in which the receiver's
// variables are bound and both processes advance.
type ChanOp struct {
	Chan string  `json:"chan"`
	Args []*Expr `json:"args"`
}

// RecvOp is a receive: on a buffered channel the edge is enabled when the
// buffer is not empty and the head message satisfies every Match; taking
// it binds each Var to the corresponding field and dequeues the head. On a
// rendezvous channel a Recv edge is never taken on its own: it is the
// partner half of a Send (see ChanOp).
type RecvOp struct {
	Chan string    `json:"chan"`
	Args []RecvArg `json:"args"`
}

// RecvArg is one field of a receive: Var (with Index for an array element)
// binds the field, Match requires the field to equal the expression's
// value, neither means the field is ignored (Promela `_`).
type RecvArg struct {
	Var   string `json:"var,omitempty"`
	Index *Expr  `json:"index,omitempty"`
	Match *Expr  `json:"match,omitempty"`
}

// RunOp starts process Proc (Promela `run`): its program counter moves
// from its Initial (dormant) location to Entry, and Args, evaluated in the
// source state, are written to its first len(Args) locals (the
// parameters). The frontend pre-instantiates every process a `run` can
// create, so Proc is a static index; the explorer reports an already
// started target as an evaluation error.
type RunOp struct {
	Proc  int     `json:"proc"`
	Entry int     `json:"entry"`
	Args  []*Expr `json:"args,omitempty"`
}

// Label marks a control location.
type Label string

const (
	// End marks a location where a process may legitimately stop; a system
	// state with no enabled transition is a deadlock only if some process is
	// neither at an End location nor at a location without outgoing edges.
	End Label = "end"
	// Progress and Accept are declared now for G4 (non-progress and
	// acceptance cycles); G0 records them and does nothing with them.
	Progress Label = "progress"
	Accept   Label = "accept"
)

// Location is a node of a process's control-flow graph.
type Location struct {
	Name   string  `json:"name,omitempty"`
	Labels []Label `json:"labels,omitempty"`
	Origin *Origin `json:"origin,omitempty"`
}

// Assign is one assignment of an effect. Index is nil for a scalar target.
type Assign struct {
	Var   string `json:"var"`
	Index *Expr  `json:"index,omitempty"`
	Value *Expr  `json:"value"`
}

// Edge is a guarded command `guard -> effect` between two locations.
type Edge struct {
	From int `json:"from"`
	To   int `json:"to"`
	// Guard nil means always enabled. A guard of kind Int is enabled when
	// non-zero (Promela convention); a guard of kind Bool when true.
	Guard *Expr `json:"guard,omitempty"`
	// Assert, if present, is evaluated in the source state once the guard
	// holds; a false assert is a violation of the model's "assert" property
	// and the edge itself is the last step of the witness.
	Assert *Expr    `json:"assert,omitempty"`
	Effect []Assign `json:"effect,omitempty"`
	// Send / Recv / Run are the channel and process-creation operations
	// (G1). An edge carries at most one of them; the Promela frontend gives
	// each statement its own edge. Order within a step: guard (and the
	// operation's own enabling condition), assert, Run, Send or Recv,
	// Effect.
	Send *ChanOp `json:"send,omitempty"`
	Recv *RecvOp `json:"recv,omitempty"`
	Run  *RunOp  `json:"run,omitempty"`
	// Else marks a Promela `else`: the edge is enabled iff no other edge
	// out of the same location is enabled. Guard must be nil.
	Else bool `json:"else,omitempty"`
	// DStep: after this edge the same process continues at once with the
	// first enabled edge of its new location, inside the same step, and so
	// on while the edges taken carry DStep; intermediate states are not
	// stored (Promela `d_step`). No enabled continuation is a model error
	// ("block in d_step seq", invalid-model), as in SPIN.
	DStep bool `json:"dstep,omitempty"`
	// Atomic: the process keeps exclusive control after this edge (see the
	// package comment). False for single-step commands such as a Petri
	// transition.
	Atomic bool `json:"atomic,omitempty"`
	// Text is the command as the user would recognise it (e.g. "t1" or
	// "(x > 0) -> x--"). It is what a counterexample step shows.
	Text   string  `json:"text,omitempty"`
	Origin *Origin `json:"origin,omitempty"`
}

// Process is one process instance: a control-flow graph over Locations with
// its own Locals. Locals shadow Globals of the same name inside the process.
type Process struct {
	Name   string `json:"name"`
	Locals []Var  `json:"locals,omitempty"`
	// Params is the number of leading Locals that are parameters set by a
	// RunOp (0 for processes that start with the system).
	Params    int        `json:"params,omitempty"`
	Locations []Location `json:"locations"`
	Initial   int        `json:"initial"`
	Edges     []Edge     `json:"edges"`
	// Claim marks a Promela never claim. G1 stores it and does not execute
	// it: the explorer never takes its edges and ignores it in the deadlock
	// rule; its synchronous product with the system is G4.
	Claim  bool    `json:"claim,omitempty"`
	Origin *Origin `json:"origin,omitempty"`
}

// Property kinds the IR can carry. The explorer executes Deadlock,
// Invariant, Reach and Assert in G0; other kinds are reported not-executed.
const (
	KindDeadlock  = "deadlock"  // no reachable deadlock (definition: explore package)
	KindInvariant = "invariant" // Expr holds in every reachable state
	KindReach     = "reach"     // some reachable state satisfies Expr
	KindAssert    = "assert"    // no edge assert fails on any reachable step
)

// Property is a verification question about the model.
type Property struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	// Expr is required for invariant and reach, ignored for the others.
	Expr *Expr `json:"expr,omitempty"`
	// Text is the user-facing statement of the property.
	Text   string  `json:"text,omitempty"`
	Origin *Origin `json:"origin,omitempty"`
}

// Model is a complete verification case: the system and its properties.
type Model struct {
	Schema     string     `json:"schema"`
	Name       string     `json:"name"`
	Globals    []Var      `json:"globals,omitempty"`
	Channels   []Channel  `json:"channels,omitempty"`
	Processes  []Process  `json:"processes"`
	Properties []Property `json:"properties,omitempty"`
	Origin     *Origin    `json:"origin,omitempty"`
}
