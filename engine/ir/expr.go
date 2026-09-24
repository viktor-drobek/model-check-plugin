package ir

import (
	"fmt"
	"strconv"
)

// Expr is a node of the small typed expression language: integer
// arithmetic, comparisons, boolean connectives, variable and array reads.
//
// Ops and their arity:
//
//	const            Value                     kind Int
//	var              Var                       kind Int (scalar read)
//	index            Var, Args[0]              kind Int (array read)
//	neg              Args[0]                   Int → Int
//	add sub mul div mod  Args[0], Args[1]      Int × Int → Int
//	eq ne lt le gt ge    Args[0], Args[1]      Int × Int → Bool
//	not              Args[0]                   Bool → Bool
//	and or           Args[0], Args[1]          Bool × Bool → Bool
//
// bit/bool variables are integers 0/1 (Promela convention); where a Bool is
// required, an Int operand is accepted and read as "non-zero". Where an Int
// is required, a Bool is accepted as 0/1. Division and modulo truncate
// towards zero like Go and C; division by zero is an evaluation error and
// therefore invalid-model.
type Expr struct {
	Op    string  `json:"op"`
	Value int64   `json:"value,omitempty"`
	Var   string  `json:"var,omitempty"`
	Args  []*Expr `json:"args,omitempty"`
}

// Kind is the static type of an expression.
type Kind int

const (
	KInt Kind = iota
	KBool
)

func (k Kind) String() string {
	if k == KBool {
		return "bool"
	}
	return "int"
}

// Constructors used by frontends and tests.

func Const(v int64) *Expr   { return &Expr{Op: "const", Value: v} }
func Ref(name string) *Expr { return &Expr{Op: "var", Var: name} }
func Index(name string, i *Expr) *Expr {
	return &Expr{Op: "index", Var: name, Args: []*Expr{i}}
}
func Unary(op string, a *Expr) *Expr     { return &Expr{Op: op, Args: []*Expr{a}} }
func Binary(op string, a, b *Expr) *Expr { return &Expr{Op: op, Args: []*Expr{a, b}} }

// And folds a conjunction; an empty list is the constant true.
func And(xs ...*Expr) *Expr {
	if len(xs) == 0 {
		return Const(1)
	}
	e := xs[0]
	for _, x := range xs[1:] {
		e = Binary("and", e, x)
	}
	return e
}

var arity = map[string]int{
	"const": 0, "var": 0, "index": 1,
	"neg": 1, "not": 1,
	"add": 2, "sub": 2, "mul": 2, "div": 2, "mod": 2,
	"eq": 2, "ne": 2, "lt": 2, "le": 2, "gt": 2, "ge": 2,
	"and": 2, "or": 2,
}

// Scope resolves variable names for type checking and compilation.
type Scope interface {
	// LookupVar returns the declaration of name, or nil.
	LookupVar(name string) *Var
}

// Check type-checks e against scope and returns its kind.
func Check(e *Expr, scope Scope) (Kind, error) {
	if e == nil {
		return KBool, fmt.Errorf("nil expression")
	}
	n, ok := arity[e.Op]
	if !ok {
		return KInt, fmt.Errorf("unknown op %q", e.Op)
	}
	if len(e.Args) != n {
		return KInt, fmt.Errorf("op %q takes %d argument(s), got %d", e.Op, n, len(e.Args))
	}
	switch e.Op {
	case "const":
		return KInt, nil
	case "var", "index":
		v := scope.LookupVar(e.Var)
		if v == nil {
			return KInt, fmt.Errorf("undeclared variable %q", e.Var)
		}
		if e.Op == "var" && v.Len > 0 {
			return KInt, fmt.Errorf("array %q read without index", e.Var)
		}
		if e.Op == "index" {
			if v.Len == 0 {
				return KInt, fmt.Errorf("scalar %q indexed", e.Var)
			}
			if _, err := Check(e.Args[0], scope); err != nil {
				return KInt, err
			}
		}
		return KInt, nil
	}
	for _, a := range e.Args {
		if _, err := Check(a, scope); err != nil {
			return KInt, err
		}
	}
	switch e.Op {
	case "not", "and", "or", "eq", "ne", "lt", "le", "gt", "ge":
		return KBool, nil
	}
	return KInt, nil
}

// String renders e in infix form for messages and counterexample texts.
func (e *Expr) String() string {
	if e == nil {
		return "true"
	}
	switch e.Op {
	case "const":
		return strconv.FormatInt(e.Value, 10)
	case "var":
		return e.Var
	case "index":
		return e.Var + "[" + e.Args[0].String() + "]"
	case "neg":
		return "-" + paren(e.Args[0])
	case "not":
		return "!" + paren(e.Args[0])
	}
	sym := map[string]string{
		"add": "+", "sub": "-", "mul": "*", "div": "/", "mod": "%",
		"eq": "==", "ne": "!=", "lt": "<", "le": "<=", "gt": ">", "ge": ">=",
		"and": "&&", "or": "||",
	}[e.Op]
	if sym == "" || len(e.Args) != 2 {
		return "<bad expr " + e.Op + ">"
	}
	return paren(e.Args[0]) + " " + sym + " " + paren(e.Args[1])
}

func paren(e *Expr) string {
	s := e.String()
	if e != nil && len(e.Args) > 0 && e.Op != "index" {
		return "(" + s + ")"
	}
	return s
}

// EvalError is an evaluation failure that makes the model invalid.
type EvalError struct {
	Msg string
}

func (e *EvalError) Error() string { return e.Msg }

// Compiled is an expression with variable references resolved to state
// slots; evaluation is deterministic and allocation-free.
type Compiled struct {
	op   string
	val  int64
	slot *Slot // for var/index: the first slot of the variable
	args []*Compiled
	text string
}

// Compile resolves e against layout within process proc (-1 for global
// scope) after type-checking it.
func (l *Layout) Compile(e *Expr, proc int) (*Compiled, error) {
	if e == nil {
		return nil, nil
	}
	if _, err := Check(e, l.scope(proc)); err != nil {
		return nil, err
	}
	return l.compile(e, proc), nil
}

func (l *Layout) compile(e *Expr, proc int) *Compiled {
	c := &Compiled{op: e.Op, val: e.Value, text: e.String()}
	if e.Op == "var" || e.Op == "index" {
		c.slot = l.Resolve(e.Var, proc)
	}
	for _, a := range e.Args {
		c.args = append(c.args, l.compile(a, proc))
	}
	return c
}

// Eval evaluates c on state. Bool results are 0/1.
func (c *Compiled) Eval(state []byte) (int64, error) {
	switch c.op {
	case "const":
		return c.val, nil
	case "var":
		return c.slot.Read(state), nil
	case "index":
		i, err := c.args[0].Eval(state)
		if err != nil {
			return 0, err
		}
		if i < 0 || i >= int64(c.slot.Var.Len) {
			return 0, &EvalError{fmt.Sprintf("index %d out of range for %s[%d]", i, c.slot.Var.Name, c.slot.Var.Len)}
		}
		return c.slot.At(int(i)).Read(state), nil
	case "neg":
		a, err := c.args[0].Eval(state)
		return -a, err
	case "not":
		a, err := c.args[0].Eval(state)
		return b2i(a == 0), err
	case "and":
		a, err := c.args[0].Eval(state)
		if err != nil || a == 0 {
			return 0, err
		}
		b, err := c.args[1].Eval(state)
		return b2i(b != 0), err
	case "or":
		a, err := c.args[0].Eval(state)
		if err != nil {
			return 0, err
		}
		if a != 0 {
			return 1, nil
		}
		b, err := c.args[1].Eval(state)
		return b2i(b != 0), err
	}
	a, err := c.args[0].Eval(state)
	if err != nil {
		return 0, err
	}
	b, err := c.args[1].Eval(state)
	if err != nil {
		return 0, err
	}
	switch c.op {
	case "add":
		return a + b, nil
	case "sub":
		return a - b, nil
	case "mul":
		return a * b, nil
	case "div":
		if b == 0 {
			return 0, &EvalError{"division by zero in " + c.text}
		}
		return a / b, nil
	case "mod":
		if b == 0 {
			return 0, &EvalError{"modulo by zero in " + c.text}
		}
		return a % b, nil
	case "eq":
		return b2i(a == b), nil
	case "ne":
		return b2i(a != b), nil
	case "lt":
		return b2i(a < b), nil
	case "le":
		return b2i(a <= b), nil
	case "gt":
		return b2i(a > b), nil
	case "ge":
		return b2i(a >= b), nil
	}
	return 0, &EvalError{"unknown op " + c.op}
}

// Truth evaluates c as a guard: nil is true, otherwise non-zero is true.
func (c *Compiled) Truth(state []byte) (bool, error) {
	if c == nil {
		return true, nil
	}
	v, err := c.Eval(state)
	return v != 0, err
}

// String returns the source-like text of the compiled expression.
func (c *Compiled) String() string {
	if c == nil {
		return "true"
	}
	return c.text
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
