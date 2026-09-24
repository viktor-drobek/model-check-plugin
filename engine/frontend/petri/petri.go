// Package petri translates a P/T Petri net given as JSON (schema.json,
// JSON Schema draft 2020-12) into the IR and the standard property set of
// plan 14 §5.3.
//
// Encoding, the same as Promela - examples/App_C/petrinet1 for everything
// that is stored in a state, so that state counts are comparable with pan
// up to the two states pan stores for the `init` assignments of the initial
// marking (the IR has the marking as initial values, not as steps): every
// place is a global byte variable
// (Max = capacity when it is below 255), the net is one process with a
// single control location and one self-loop edge per transition; the edge's
// guard is the conjunction of `place >= weight` over the input arcs (written
// `place > 0` for weight 1, as the Promela `inp` macros do), its effect first
// subtracts the input weights and then adds the output weights (`x--; y++`).
// A transition is a single indivisible step, so Edge.Atomic is false: there
// is nothing after it that must run exclusively (see package ir).
//
// Generated properties: `deadlock` (Holzmann's hang: no transition enabled),
// `safe` (invariant: every place ≤ 1). Capacity is not a property: exceeding
// it makes every property invalid-model (14 §11).
//
// Rejections: inhibitor arcs (with the reason from Holzmann §8.10) and any
// deviation from schema.json, reported as *Error with the JSON path.
package petri

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"

	"modelcheck/ir"
)

// Error is a rejection of the input. Kind is "schema" for a structural
// violation of schema.json and "unsupported-input" for a well-formed net
// outside the class the engine translates.
type Error struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s: %s", e.Kind, e.Path, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

// Net is the validated input.
type Net struct {
	Name        string
	Places      []Place
	Transitions []Transition
}

type Place struct {
	Name     string
	Initial  int64
	Capacity int64
	Line     int
}

type Arc struct {
	Place     string
	Weight    int64
	Inhibitor bool
}

type Transition struct {
	Name    string
	Inputs  []Arc
	Outputs []Arc
	Line    int
}

const (
	defaultCapacity = 255
	holzmannNote    = "Holzmann, Design and Validation of Computer Protocols §8.10: a P/T net cannot express negation — a transition is enabled by the presence of tokens, never by their absence — and the engine encodes exactly that class (place = byte, transition = guarded command with guards `place > 0`, as in App_C/petrinet1). Rewrite the net without inhibitor arcs — for a place bounded by a known capacity k this is possible with a complementary place holding k minus its marking, so that \"empty\" becomes \"complement holds k tokens\" — or model the system in the Promela subset."
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Parse validates data against schema.json (hand-written checks, one per
// schema clause) and returns the net.
func Parse(data []byte, defaultName string) (*Net, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, &Error{Kind: "schema", Message: "not valid JSON: " + err.Error()}
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, &Error{Kind: "schema", Message: "trailing data after the document"}
	}
	v := &validator{}
	obj := v.object(root, "$", []string{"places", "transitions"}, []string{"name", "places", "transitions"})
	if v.err != nil {
		return nil, v.err
	}
	net := &Net{Name: defaultName}
	if n, ok := obj["name"]; ok {
		net.Name = v.str(n, "$.name")
	}
	seenPlaces := map[string]int{}
	for i, p := range v.array(obj["places"], "$.places", 1) {
		path := fmt.Sprintf("places[%d]", i)
		po := v.object(p, path, []string{"name"}, []string{"name", "initial", "capacity", "line"})
		if v.err != nil {
			return nil, v.err
		}
		pl := Place{Name: v.ident(po["name"], path+".name"), Capacity: defaultCapacity}
		if x, ok := po["initial"]; ok {
			pl.Initial = v.integer(x, path+".initial", 0, math.MaxInt64)
		}
		if x, ok := po["capacity"]; ok {
			pl.Capacity = v.integer(x, path+".capacity", 1, 255)
		}
		if x, ok := po["line"]; ok {
			pl.Line = int(v.integer(x, path+".line", 1, math.MaxInt32))
		}
		if v.err != nil {
			return nil, v.err
		}
		if pl.Initial > pl.Capacity {
			return nil, &Error{Kind: "schema", Path: path + ".initial", Message: fmt.Sprintf("initial marking %d exceeds capacity %d", pl.Initial, pl.Capacity)}
		}
		if _, dup := seenPlaces[pl.Name]; dup {
			return nil, &Error{Kind: "schema", Path: path + ".name", Message: fmt.Sprintf("duplicate place name %q", pl.Name)}
		}
		seenPlaces[pl.Name] = i
		net.Places = append(net.Places, pl)
	}
	seenTrans := map[string]bool{}
	var inhibitors []string
	for i, t := range v.array(obj["transitions"], "$.transitions", 1) {
		path := fmt.Sprintf("transitions[%d]", i)
		to := v.object(t, path, []string{"name"}, []string{"name", "inputs", "outputs", "line"})
		if v.err != nil {
			return nil, v.err
		}
		tr := Transition{Name: v.ident(to["name"], path+".name")}
		if x, ok := to["line"]; ok {
			tr.Line = int(v.integer(x, path+".line", 1, math.MaxInt32))
		}
		if v.err != nil {
			return nil, v.err
		}
		if _, isPlace := seenPlaces[tr.Name]; isPlace || seenTrans[tr.Name] {
			return nil, &Error{Kind: "schema", Path: path + ".name", Message: fmt.Sprintf("name %q is already used", tr.Name)}
		}
		seenTrans[tr.Name] = true
		for _, side := range []string{"inputs", "outputs"} {
			x, ok := to[side]
			if !ok {
				continue
			}
			for j, a := range v.array(x, path+"."+side, 0) {
				apath := fmt.Sprintf("%s.%s[%d]", path, side, j)
				ao := v.object(a, apath, []string{"place"}, []string{"place", "weight", "inhibitor"})
				if v.err != nil {
					return nil, v.err
				}
				arc := Arc{Place: v.ident(ao["place"], apath+".place"), Weight: 1}
				if w, ok := ao["weight"]; ok {
					arc.Weight = v.integer(w, apath+".weight", 1, 255)
				}
				if inh, ok := ao["inhibitor"]; ok {
					arc.Inhibitor = v.boolean(inh, apath+".inhibitor")
				}
				if v.err != nil {
					return nil, v.err
				}
				if _, known := seenPlaces[arc.Place]; !known {
					return nil, &Error{Kind: "schema", Path: apath + ".place", Message: fmt.Sprintf("unknown place %q", arc.Place)}
				}
				if arc.Inhibitor {
					if side == "outputs" {
						return nil, &Error{Kind: "schema", Path: apath + ".inhibitor", Message: "an output arc cannot be an inhibitor arc"}
					}
					inhibitors = append(inhibitors, fmt.Sprintf("transition %q, input place %q (%s)", tr.Name, arc.Place, apath))
				}
				if side == "inputs" {
					tr.Inputs = append(tr.Inputs, arc)
				} else {
					tr.Outputs = append(tr.Outputs, arc)
				}
			}
		}
		net.Transitions = append(net.Transitions, tr)
	}
	if v.err != nil {
		return nil, v.err
	}
	if len(inhibitors) > 0 {
		return nil, &Error{Kind: "unsupported-input", Path: "transitions",
			Message: fmt.Sprintf("inhibitor arcs are not supported: %s. %s", strings.Join(inhibitors, "; "), holzmannNote)}
	}
	return net, nil
}

// ToIR encodes the net (see the package comment) with source mapping to
// file, the optional line numbers and the user's names.
func (n *Net) ToIR(file string) *ir.Model {
	m := &ir.Model{Schema: ir.Schema, Name: n.Name, Origin: &ir.Origin{File: file, Name: n.Name}}
	var safe []*ir.Expr
	for _, p := range n.Places {
		v := ir.Var{Name: p.Name, Type: ir.Byte, Init: []int64{p.Initial}, Origin: &ir.Origin{File: file, Line: p.Line, Name: p.Name}}
		if p.Capacity < defaultCapacity {
			c := p.Capacity
			v.Max = &c
		}
		if p.Initial == 0 {
			v.Init = nil
		}
		m.Globals = append(m.Globals, v)
		safe = append(safe, ir.Binary("le", ir.Ref(p.Name), ir.Const(1)))
	}
	pr := ir.Process{Name: "init", Locations: []ir.Location{{Name: "do"}}, Initial: 0,
		Origin: &ir.Origin{File: file, Name: n.Name}}
	for _, t := range n.Transitions {
		var guards []*ir.Expr
		var eff []ir.Assign
		for _, a := range t.Inputs {
			if a.Weight == 1 {
				guards = append(guards, ir.Binary("gt", ir.Ref(a.Place), ir.Const(0)))
			} else {
				guards = append(guards, ir.Binary("ge", ir.Ref(a.Place), ir.Const(a.Weight)))
			}
			eff = append(eff, ir.Assign{Var: a.Place, Value: ir.Binary("sub", ir.Ref(a.Place), ir.Const(a.Weight))})
		}
		for _, a := range t.Outputs {
			eff = append(eff, ir.Assign{Var: a.Place, Value: ir.Binary("add", ir.Ref(a.Place), ir.Const(a.Weight))})
		}
		pr.Edges = append(pr.Edges, ir.Edge{
			From: 0, To: 0,
			Guard:  ir.And(guards...),
			Effect: eff,
			Atomic: false, // one indivisible step; nothing follows it exclusively
			Text:   t.Name,
			Origin: &ir.Origin{File: file, Line: t.Line, Name: t.Name},
		})
	}
	m.Processes = []ir.Process{pr}
	m.Properties = []ir.Property{
		{ID: "deadlock", Kind: ir.KindDeadlock,
			Text: "hang (Holzmann §8.10): a reachable marking in which no transition is enabled", Origin: &ir.Origin{File: file, Name: "hang"}},
		{ID: "safe", Kind: ir.KindInvariant, Expr: ir.And(safe...),
			Text: "safe (Holzmann §8.10): every place holds at most one token in every reachable marking", Origin: &ir.Origin{File: file, Name: "safe"}},
	}
	return m
}

// ---- hand-written structural validation ---------------------------------

type validator struct{ err *Error }

func (v *validator) fail(path, msg string) {
	if v.err == nil {
		v.err = &Error{Kind: "schema", Path: path, Message: msg}
	}
}

func (v *validator) object(x any, path string, required, allowed []string) map[string]any {
	if v.err != nil {
		return nil
	}
	o, ok := x.(map[string]any)
	if !ok {
		v.fail(path, "expected an object")
		return nil
	}
	for _, r := range required {
		if _, ok := o[r]; !ok {
			v.fail(path, fmt.Sprintf("missing required property %q", r))
			return nil
		}
	}
	var extra []string
	for k := range o {
		known := false
		for _, a := range allowed {
			if a == k {
				known = true
			}
		}
		if !known {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		v.fail(path, fmt.Sprintf("unknown property %q (additionalProperties: false)", extra[0]))
		return nil
	}
	return o
}

func (v *validator) array(x any, path string, minItems int) []any {
	if v.err != nil {
		return nil
	}
	a, ok := x.([]any)
	if !ok {
		v.fail(path, "expected an array")
		return nil
	}
	if len(a) < minItems {
		v.fail(path, fmt.Sprintf("needs at least %d item(s)", minItems))
		return nil
	}
	return a
}

func (v *validator) str(x any, path string) string {
	if v.err != nil {
		return ""
	}
	s, ok := x.(string)
	if !ok {
		v.fail(path, "expected a string")
	}
	return s
}

func (v *validator) ident(x any, path string) string {
	s := v.str(x, path)
	if v.err == nil && !identRe.MatchString(s) {
		v.fail(path, fmt.Sprintf("%q is not an identifier (pattern %s)", s, identRe))
	}
	return s
}

func (v *validator) integer(x any, path string, min, max int64) int64 {
	if v.err != nil {
		return 0
	}
	num, ok := x.(json.Number)
	if !ok {
		v.fail(path, "expected an integer")
		return 0
	}
	n, err := num.Int64()
	if err != nil {
		v.fail(path, fmt.Sprintf("expected an integer, got %s", num))
		return 0
	}
	if n < min || n > max {
		v.fail(path, fmt.Sprintf("%d outside [%d, %d]", n, min, max))
	}
	return n
}

func (v *validator) boolean(x any, path string) bool {
	if v.err != nil {
		return false
	}
	b, ok := x.(bool)
	if !ok {
		v.fail(path, "expected a boolean")
	}
	return b
}
