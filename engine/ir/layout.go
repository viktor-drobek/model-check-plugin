package ir

import (
	"encoding/binary"
	"fmt"
)

// Layout is the flat byte-vector state layout of a Model (14 §4.1), in this
// order:
//
//  1. one byte Excl: 0 = no process holds exclusive control, p+1 = process p
//     is inside an atomic sequence (see Edge.Atomic);
//  2. for each process: its program counter (1 byte if the process has at
//     most 256 locations, else 2), then its locals;
//  3. the globals;
//  4. for each channel: one length byte, then Capacity × message width
//     bytes of buffer (rendezvous channels, capacity 0, take one byte).
//
// Scalars occupy Type.Width() bytes little-endian; arrays are contiguous.
// The layout is a pure function of the Model, so equal models give equal
// vectors and state counts are comparable across runs and versions.
type Layout struct {
	Model *Model
	Size  int
	Excl  int   // offset of the exclusive-control byte
	PC    []int // per process: offset of its program counter
	PCW   []int // per process: width of the program counter (1 or 2)
	// Slots lists every scalar slot in layout order (array elements are
	// separate slots sharing one Var). Counterexample diffs walk this list.
	Slots []*Slot
	// Chans gives each channel's offset (length byte first).
	Chans []int

	globals map[string]*Slot
	locals  []map[string]*Slot
}

// Slot is one scalar cell of the state vector.
type Slot struct {
	Var    *Var
	Proc   int // -1 for a global
	Index  int // element index within an array, 0 for scalars
	Offset int
	Width  int
	Min    int64
	Max    int64
}

// Name is the qualified name used in reports: "x", "x[2]", "P.y".
func (s *Slot) Name() string {
	n := s.Var.Name
	if s.Var.Len > 0 {
		n = fmt.Sprintf("%s[%d]", n, s.Index)
	}
	return n
}

// At returns the slot of element i of the same array (i is checked by the
// caller).
func (s *Slot) At(i int) *Slot {
	if i == s.Index {
		return s
	}
	c := *s
	c.Index = i
	c.Offset = s.Offset + (i-s.Index)*s.Width
	return &c
}

// Read decodes the slot's value (sign-extended for short/int).
func (s *Slot) Read(state []byte) int64 {
	b := state[s.Offset:]
	switch s.Width {
	case 1:
		return int64(b[0]) // bit, bool, byte: unsigned
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(b)))
	default:
		return int64(int32(binary.LittleEndian.Uint32(b)))
	}
}

// Write stores v; the caller has already checked v against [Min, Max].
func (s *Slot) Write(state []byte, v int64) {
	b := state[s.Offset:]
	switch s.Width {
	case 1:
		b[0] = byte(v)
	case 2:
		binary.LittleEndian.PutUint16(b, uint16(v))
	default:
		binary.LittleEndian.PutUint32(b, uint32(v))
	}
}

// InDomain reports whether v may be stored in s.
func (s *Slot) InDomain(v int64) bool { return v >= s.Min && v <= s.Max }

// NewLayout validates m (see Validate) and computes its layout.
func NewLayout(m *Model) (*Layout, error) {
	if err := Validate(m); err != nil {
		return nil, err
	}
	l := &Layout{Model: m, globals: map[string]*Slot{}}
	off := 0
	l.Excl = off
	off++
	for p := range m.Processes {
		pr := &m.Processes[p]
		w := 1
		if len(pr.Locations) > 256 {
			w = 2
		}
		l.PC = append(l.PC, off)
		l.PCW = append(l.PCW, w)
		off += w
		loc := map[string]*Slot{}
		for i := range pr.Locals {
			off = l.place(&pr.Locals[i], p, off, loc)
		}
		l.locals = append(l.locals, loc)
	}
	for i := range m.Globals {
		off = l.place(&m.Globals[i], -1, off, l.globals)
	}
	for i := range m.Channels {
		ch := &m.Channels[i]
		l.Chans = append(l.Chans, off)
		w := 0
		for _, f := range ch.Fields {
			w += f.Width()
		}
		off += 1 + ch.Capacity*w
	}
	l.Size = off
	return l, nil
}

func (l *Layout) place(v *Var, proc, off int, into map[string]*Slot) int {
	min, max := v.Domain()
	w := v.Type.Width()
	var first *Slot
	for i := 0; i < v.Count(); i++ {
		s := &Slot{Var: v, Proc: proc, Index: i, Offset: off, Width: w, Min: min, Max: max}
		if first == nil {
			first = s
		}
		l.Slots = append(l.Slots, s)
		off += w
	}
	into[v.Name] = first
	return off
}

// Resolve returns the first slot of name as seen from process proc (-1 =
// global scope): locals shadow globals. nil if undeclared.
func (l *Layout) Resolve(name string, proc int) *Slot {
	if proc >= 0 {
		if s, ok := l.locals[proc][name]; ok {
			return s
		}
	}
	return l.globals[name]
}

type layoutScope struct {
	l    *Layout
	proc int
}

func (s layoutScope) LookupVar(name string) *Var {
	if sl := s.l.Resolve(name, s.proc); sl != nil {
		return sl.Var
	}
	return nil
}

func (l *Layout) scope(proc int) Scope { return layoutScope{l, proc} }

// Scope exposes name resolution for process proc (-1 = global) to callers
// that type-check expressions themselves.
func (l *Layout) Scope(proc int) Scope { return l.scope(proc) }

// ReadPC returns the control location of process p in state.
func (l *Layout) ReadPC(state []byte, p int) int {
	if l.PCW[p] == 1 {
		return int(state[l.PC[p]])
	}
	return int(binary.LittleEndian.Uint16(state[l.PC[p]:]))
}

// WritePC sets the control location of process p.
func (l *Layout) WritePC(state []byte, p, loc int) {
	if l.PCW[p] == 1 {
		state[l.PC[p]] = byte(loc)
		return
	}
	binary.LittleEndian.PutUint16(state[l.PC[p]:], uint16(loc))
}

// Initial builds the initial state vector: initial locations, Init values,
// empty channels, no exclusive holder.
func (l *Layout) Initial() []byte {
	s := make([]byte, l.Size)
	for p := range l.Model.Processes {
		l.WritePC(s, p, l.Model.Processes[p].Initial)
	}
	for _, sl := range l.Slots {
		if sl.Index < len(sl.Var.Init) {
			sl.Write(s, sl.Var.Init[sl.Index])
		}
	}
	return s
}

// Validate checks the static well-formedness of m: schema, unique names,
// known types, initial values inside domains, location and edge indices in
// range, expressions type-correct in their scope, properties complete. It
// returns the first problem found, with the element's path.
func Validate(m *Model) error {
	if m == nil {
		return fmt.Errorf("nil model")
	}
	if m.Schema != Schema {
		return fmt.Errorf("schema: want %q, got %q", Schema, m.Schema)
	}
	seen := map[string]bool{}
	for i := range m.Globals {
		if err := validateVar(&m.Globals[i], seen); err != nil {
			return fmt.Errorf("globals[%d]: %w", i, err)
		}
	}
	for i := range m.Channels {
		ch := &m.Channels[i]
		if ch.Name == "" || seen[ch.Name] {
			return fmt.Errorf("channels[%d]: missing or duplicate name %q", i, ch.Name)
		}
		seen[ch.Name] = true
		if ch.Capacity < 0 || ch.Capacity > 255 {
			return fmt.Errorf("channels[%d]: capacity %d outside 0..255", i, ch.Capacity)
		}
		for j, f := range ch.Fields {
			if _, _, ok := f.Bounds(); !ok {
				return fmt.Errorf("channels[%d].fields[%d]: unknown type %q", i, j, f)
			}
		}
	}
	if len(m.Processes) == 0 {
		return fmt.Errorf("processes: a model needs at least one process")
	}
	if len(m.Processes) > 254 {
		return fmt.Errorf("processes: %d exceed the 254 the exclusive-control byte can name", len(m.Processes))
	}
	// A scratch layout for expression scopes; built without re-validating.
	l := &Layout{Model: m, globals: map[string]*Slot{}}
	for i := range m.Globals {
		l.place(&m.Globals[i], -1, 0, l.globals)
	}
	pnames := map[string]bool{}
	for p := range m.Processes {
		pr := &m.Processes[p]
		if pr.Name == "" || pnames[pr.Name] {
			return fmt.Errorf("processes[%d]: missing or duplicate name %q", p, pr.Name)
		}
		pnames[pr.Name] = true
		lseen := map[string]bool{}
		loc := map[string]*Slot{}
		for i := range pr.Locals {
			if err := validateVar(&pr.Locals[i], lseen); err != nil {
				return fmt.Errorf("processes[%d].locals[%d]: %w", p, i, err)
			}
			l.place(&pr.Locals[i], p, 0, loc)
		}
		l.locals = append(l.locals, loc)
		n := len(pr.Locations)
		if n == 0 {
			return fmt.Errorf("processes[%d]: no locations", p)
		}
		if n > 65535 {
			return fmt.Errorf("processes[%d]: %d locations exceed 65535", p, n)
		}
		if pr.Initial < 0 || pr.Initial >= n {
			return fmt.Errorf("processes[%d].initial: %d outside 0..%d", p, pr.Initial, n-1)
		}
		for i, lc := range pr.Locations {
			for _, lb := range lc.Labels {
				if lb != End && lb != Progress && lb != Accept {
					return fmt.Errorf("processes[%d].locations[%d]: unknown label %q", p, i, lb)
				}
			}
		}
		for i := range pr.Edges {
			e := &pr.Edges[i]
			path := fmt.Sprintf("processes[%d].edges[%d]", p, i)
			if e.From < 0 || e.From >= n || e.To < 0 || e.To >= n {
				return fmt.Errorf("%s: from/to %d/%d outside 0..%d", path, e.From, e.To, n-1)
			}
			if e.Guard != nil {
				if _, err := Check(e.Guard, l.scope(p)); err != nil {
					return fmt.Errorf("%s.guard: %w", path, err)
				}
			}
			if e.Assert != nil {
				if _, err := Check(e.Assert, l.scope(p)); err != nil {
					return fmt.Errorf("%s.assert: %w", path, err)
				}
			}
			for j, a := range e.Effect {
				apath := fmt.Sprintf("%s.effect[%d]", path, j)
				v := l.scope(p).LookupVar(a.Var)
				if v == nil {
					return fmt.Errorf("%s: undeclared variable %q", apath, a.Var)
				}
				if (v.Len > 0) != (a.Index != nil) {
					return fmt.Errorf("%s: %q needs an index iff it is an array", apath, a.Var)
				}
				if a.Index != nil {
					if _, err := Check(a.Index, l.scope(p)); err != nil {
						return fmt.Errorf("%s.index: %w", apath, err)
					}
				}
				if a.Value == nil {
					return fmt.Errorf("%s: missing value", apath)
				}
				if _, err := Check(a.Value, l.scope(p)); err != nil {
					return fmt.Errorf("%s.value: %w", apath, err)
				}
			}
		}
	}
	ids := map[string]bool{}
	for i, pr := range m.Properties {
		if pr.ID == "" || ids[pr.ID] {
			return fmt.Errorf("properties[%d]: missing or duplicate id %q", i, pr.ID)
		}
		ids[pr.ID] = true
		switch pr.Kind {
		case KindInvariant, KindReach:
			if pr.Expr == nil {
				return fmt.Errorf("properties[%d] (%s): kind %q needs expr", i, pr.ID, pr.Kind)
			}
			if _, err := Check(pr.Expr, l.scope(-1)); err != nil {
				return fmt.Errorf("properties[%d] (%s).expr: %w", i, pr.ID, err)
			}
		case KindDeadlock, KindAssert:
		default:
			// Unknown kinds are legal in the IR (a later engine version may
			// execute them); the explorer reports them not-executed.
		}
	}
	return nil
}

func validateVar(v *Var, seen map[string]bool) error {
	if v.Name == "" || seen[v.Name] {
		return fmt.Errorf("missing or duplicate name %q", v.Name)
	}
	seen[v.Name] = true
	tmin, tmax, ok := v.Type.Bounds()
	if !ok {
		return fmt.Errorf("%s: unknown type %q", v.Name, v.Type)
	}
	if v.Len < 0 || v.Len > 65535 {
		return fmt.Errorf("%s: array length %d outside 0..65535", v.Name, v.Len)
	}
	if v.Min != nil && (*v.Min < tmin || *v.Min > tmax) {
		return fmt.Errorf("%s: min %d outside the %s domain", v.Name, *v.Min, v.Type)
	}
	if v.Max != nil && (*v.Max < tmin || *v.Max > tmax) {
		return fmt.Errorf("%s: max %d outside the %s domain", v.Name, *v.Max, v.Type)
	}
	min, max := v.Domain()
	if min > max {
		return fmt.Errorf("%s: empty domain [%d, %d]", v.Name, min, max)
	}
	if len(v.Init) > 0 && len(v.Init) != v.Count() {
		return fmt.Errorf("%s: %d initial values for %d element(s)", v.Name, len(v.Init), v.Count())
	}
	for i, x := range v.Init {
		if x < min || x > max {
			return fmt.Errorf("%s: initial value %d (element %d) outside [%d, %d]", v.Name, x, i, min, max)
		}
	}
	return nil
}
