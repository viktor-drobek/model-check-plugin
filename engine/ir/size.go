package ir

import "fmt"

// Size limits of a model (issue #9). An IR document is validated before it
// is laid out, compiled or written; these bounds keep every one of those
// steps within the stack and the memory, whatever the input.
const (
	// MaxExprHeight bounds the height of an expression (a leaf is 1). The
	// frontends enforce the same number while they build, so an IR they
	// produce never exceeds it; a hand-written IR is refused at validation.
	MaxExprHeight = 1000
	// MaxStateSlots bounds the slots of the state vector: every scalar and
	// every array element of every global and local is one, and a channel
	// is one plus its message width times its capacity plus one (the field
	// table of the object itself). The layout allocates a record per slot,
	// so the bound is checked from the declared lengths before anything is
	// allocated.
	MaxStateSlots = 1 << 20
	// MaxIRWeight bounds the sum over every expression node of its depth:
	// the indented JSON rendering of the model costs about 36 bytes per
	// unit (weight 5·10^6 was a 181 MB file, 2·10^7 died).
	MaxIRWeight = 4_000_000
)

// LimitError is a validation failure on one of the size limits above: the
// model is well-formed but too large for the engine, which frontends report
// as a refused shape rather than as an internal error.
type LimitError struct{ Msg string }

func (e *LimitError) Error() string { return e.Msg }

// ExprHeight is the height of e (nil = 0, a leaf = 1).
func ExprHeight(e *Expr) int {
	if e == nil {
		return 0
	}
	h := 0
	for _, a := range e.Args {
		if k := ExprHeight(a); k > h {
			h = k
		}
	}
	return h + 1
}

// Weight is the sum over every expression node of m of the node's depth
// (the root is 1), and the greatest expression height in m. The weight is
// what an indented JSON rendering of the model costs; the height is what
// every recursive walk over it costs.
func Weight(m *Model) (weight int64, height int) {
	var visit func(e *Expr, depth int)
	visit = func(e *Expr, depth int) {
		if e == nil {
			return
		}
		weight += int64(depth)
		if depth > height {
			height = depth
		}
		for _, a := range e.Args {
			visit(a, depth+1)
		}
	}
	walkExprs(m, func(e *Expr) { visit(e, 1) })
	return weight, height
}

// walkExprs calls fn on every top-level expression of m: guards, asserts,
// effects, channel operations, run arguments, provided clauses and
// properties.
func walkExprs(m *Model, fn func(*Expr)) {
	for p := range m.Processes {
		pr := &m.Processes[p]
		fn(pr.Provided)
		for i := range pr.Edges {
			e := &pr.Edges[i]
			fn(e.Guard)
			fn(e.Assert)
			for _, a := range e.Effect {
				fn(a.Index)
				fn(a.Value)
			}
			if e.Send != nil {
				fn(e.Send.Sel)
				for _, a := range e.Send.Args {
					fn(a)
				}
			}
			if e.Recv != nil {
				fn(e.Recv.Sel)
				for _, a := range e.Recv.Args {
					fn(a.Index)
					fn(a.Match)
				}
			}
			if e.Run != nil {
				for _, a := range e.Run.Args {
					fn(a)
				}
				for _, a := range e.Run.Init {
					fn(a.Index)
					fn(a.Value)
				}
			}
		}
	}
	for i := range m.Properties {
		fn(m.Properties[i].Expr)
	}
}

// stateSlots is the number of slots the variables of m declare.
func stateSlots(m *Model) int64 {
	var n int64
	count := func(vs []Var) {
		for i := range vs {
			if vs[i].Len > 0 {
				n += int64(vs[i].Len)
			} else {
				n++
			}
		}
	}
	count(m.Globals)
	for p := range m.Processes {
		count(m.Processes[p].Locals)
	}
	for i := range m.Channels {
		// A channel object carries its field table besides its buffer, so a
		// wide message costs its width even at capacity zero.
		ch := &m.Channels[i]
		n += 1 + int64(len(ch.Fields))*int64(ch.Capacity+1)
	}
	return n
}

// checkSize is the part of Validate that bounds the model's size: the
// state vector, the expression heights and the weight of the document.
func checkSize(m *Model) error {
	if n := stateSlots(m); n > MaxStateSlots {
		return &LimitError{fmt.Sprintf("state vector: %d slots exceed the limit of %d (limit of the engine)", n, MaxStateSlots)}
	}
	w, h := Weight(m)
	if h > MaxExprHeight {
		return &LimitError{fmt.Sprintf("an expression is nested or chained %d levels deep, the limit is %d (limit of the engine)", h, MaxExprHeight)}
	}
	if w > MaxIRWeight {
		return &LimitError{fmt.Sprintf("the IR weighs %d (expression nesting times count), the limit is %d (limit of the engine)", w, MaxIRWeight)}
	}
	return nil
}
