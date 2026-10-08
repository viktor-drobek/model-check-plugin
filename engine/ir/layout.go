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
//  2. when the processes of the model ask for it (NeedsTable): some Process
//     is Dynamic (created by `run`), some Edge leaves the table
//     (Edge.Leave), or some expression of a process reads `nrpr` / `pid` /
//     `youngest`; a property never gives the model a table. The
//     live-process table is one byte holding the number of live processes
//     and one byte per process holding, at position k, the index of the
//     process whose pid is k. Positions from the count on are zero. The
//     table is what makes the vector a faithful image of pan's process
//     stack: creation pushes, and only the youngest process may pop;
//  3. for each process: its program counter (1 byte if the process has at
//     most 256 locations, else 2), then its locals;
//  4. the globals;
//  5. for each channel: one length byte, then Capacity × message width
//     bytes of buffer (rendezvous channels, capacity 0, take one byte).
//
// Scalars occupy Type.Width() bytes little-endian; arrays are contiguous.
// The layout is a pure function of the Model, so equal models give equal
// vectors and state counts are comparable across runs and versions.
//
// A model whose processes need no table (NeedsTable is false) has none, so its
// vector — and every count derived from it — is exactly the one G1 fixed
// against pan.
type Layout struct {
	Model  *Model
	Size   int
	Excl   int   // offset of the exclusive-control byte
	NrOff  int   // offset of the live-process count byte, -1 when there is no table
	TabOff int   // offset of the table, -1 when there is none
	PC     []int // per process: offset of its program counter
	PCW    []int // per process: width of the program counter (1 or 2)
	// Slots lists every scalar slot in layout order (array elements are
	// separate slots sharing one Var). Counterexample diffs walk this list.
	Slots []*Slot
	// Chans describes each channel's room in the vector.
	Chans []ChanLayout
	// Timeout is the value the `timeout` op reads. The explorer sets it
	// while it evaluates the timeout alternatives of a state; it is not
	// part of the state vector (as in pan).
	Timeout bool

	globals    map[string]*Slot
	locals     []map[string]*Slot
	chans      map[string]int
	staticLive int // non-claim processes, for a model without a table
}

// ChanLayout is a channel's place in the vector: the length byte at Off,
// then Capacity messages of Width bytes each, field f at FieldOff[f] within
// a message.
type ChanLayout struct {
	Chan     *Channel
	Off      int
	Width    int
	FieldOff []int
}

// ChanIndex returns the index of channel name, or false.
func (l *Layout) ChanIndex(name string) (int, bool) {
	i, ok := l.chans[name]
	return i, ok
}

// ChanID is the value a channel-typed variable holds for channel index ci:
// ci + 1, so that 0 stays Promela's null channel.
func ChanID(ci int) int64 { return int64(ci) + 1 }

// ChanByID maps a channel id back to an index; ok is false for the null
// channel and for an id outside the table.
func (l *Layout) ChanByID(id int64) (int, bool) {
	if id <= 0 || id > int64(len(l.Chans)) {
		return 0, false
	}
	return int(id) - 1, true
}

// HasTable reports whether the vector carries the live-process table.
func (l *Layout) HasTable() bool { return l.TabOff >= 0 }

// NrPr is the number of live processes. Without a table every non-claim
// process is live by construction (nothing is ever created or removed from
// the table), so the static count is the answer. A claim process is never
// counted, with a table or without one; pan counts it (BASE 1 in its pan.h),
// so under a never claim or an ltl formula a verdict that reads `_nr_pr` can
// differ from pan's (steps/fix-nrpr-confirmation.md).
func (l *Layout) NrPr(state []byte) int {
	if l.TabOff < 0 {
		return l.staticLive
	}
	return int(state[l.NrOff])
}

// PIDAt returns the index of the process whose pid is k (k < NrPr).
func (l *Layout) PIDAt(state []byte, k int) int {
	if l.TabOff < 0 {
		return k
	}
	return int(state[l.TabOff+k])
}

// PIDOf is the pid of process p, or -1 when p is not live.
func (l *Layout) PIDOf(state []byte, p int) int {
	if l.TabOff < 0 {
		return p
	}
	n := int(state[l.NrOff])
	for k := 0; k < n; k++ {
		if int(state[l.TabOff+k]) == p {
			return k
		}
	}
	return -1
}

// Youngest reports whether p is the youngest live process — SPIN's
// condition for a process to leave the vector.
func (l *Layout) Youngest(state []byte, p int) bool {
	if l.TabOff < 0 {
		return false
	}
	n := int(state[l.NrOff])
	return n > 0 && int(state[l.TabOff+n-1]) == p
}

// Enter appends p to the live-process table; ok is false when the table is
// full (which the caller reports as an exhausted bound, not as a verdict).
func (l *Layout) Enter(state []byte, p int) bool {
	if l.TabOff < 0 {
		return false
	}
	n := int(state[l.NrOff])
	if n >= len(l.Model.Processes) {
		return false
	}
	state[l.TabOff+n] = byte(p)
	state[l.NrOff] = byte(n + 1)
	return true
}

// Leave removes the youngest live process (the caller has checked that it
// is the one that is leaving) and zeroes the freed slot, so that equal
// tables give equal vectors.
func (l *Layout) Leave(state []byte) {
	if l.TabOff < 0 {
		return
	}
	n := int(state[l.NrOff])
	if n == 0 {
		return
	}
	state[l.TabOff+n-1] = 0
	state[l.NrOff] = byte(n - 1)
}

// ClearChan empties channel ci, zeroing the whole buffer.
func (l *Layout) ClearChan(state []byte, ci int) {
	c := &l.Chans[ci]
	for k := c.Off; k < c.Off+1+c.Chan.Capacity*c.Width; k++ {
		state[k] = 0
	}
}

// ChanLen is the number of messages in channel ci.
func (l *Layout) ChanLen(state []byte, ci int) int { return int(state[l.Chans[ci].Off]) }

// ChanField reads field f of message i (0 = head) of channel ci.
func (l *Layout) ChanField(state []byte, ci, i, f int) int64 {
	c := &l.Chans[ci]
	return readInt(state[c.Off+1+i*c.Width+c.FieldOff[f]:], c.Chan.Fields[f].Width())
}

// ChanPush appends a message (the caller has checked the capacity).
func (l *Layout) ChanPush(state []byte, ci int, vals []int64) {
	c := &l.Chans[ci]
	n := int(state[c.Off])
	base := c.Off + 1 + n*c.Width
	for f, v := range vals {
		writeInt(state[base+c.FieldOff[f]:], c.Chan.Fields[f].Width(), v)
	}
	state[c.Off] = byte(n + 1)
}

// ChanPop removes the head message, shifting the rest down and zeroing the
// freed slot so that equal contents give equal vectors.
func (l *Layout) ChanPop(state []byte, ci int) {
	c := &l.Chans[ci]
	n := int(state[c.Off])
	buf := state[c.Off+1 : c.Off+1+c.Chan.Capacity*c.Width]
	copy(buf, buf[c.Width:n*c.Width])
	for k := (n - 1) * c.Width; k < n*c.Width; k++ {
		buf[k] = 0
	}
	state[c.Off] = byte(n - 1)
}

// ChanMessages decodes the whole buffer of channel ci (for reports).
func (l *Layout) ChanMessages(state []byte, ci int) [][]int64 {
	n := l.ChanLen(state, ci)
	out := make([][]int64, n)
	for i := 0; i < n; i++ {
		msg := make([]int64, len(l.Chans[ci].Chan.Fields))
		for f := range msg {
			msg[f] = l.ChanField(state, ci, i, f)
		}
		out[i] = msg
	}
	return out
}

func readInt(b []byte, width int) int64 {
	switch width {
	case 1:
		return int64(b[0])
	case 2:
		return int64(int16(binary.LittleEndian.Uint16(b)))
	default:
		return int64(int32(binary.LittleEndian.Uint32(b)))
	}
}

func writeInt(b []byte, width int, v int64) {
	switch width {
	case 1:
		b[0] = byte(v)
	case 2:
		binary.LittleEndian.PutUint16(b, uint16(v))
	default:
		binary.LittleEndian.PutUint32(b, uint32(v))
	}
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
	l := &Layout{Model: m, globals: map[string]*Slot{}, chans: map[string]int{}, NrOff: -1, TabOff: -1}
	for p := range m.Processes {
		if !m.Processes[p].Claim {
			l.staticLive++
		}
	}
	off := 0
	l.Excl = off
	off++
	if NeedsTable(m) {
		l.NrOff = off
		off++
		l.TabOff = off
		off += len(m.Processes)
	}
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
		cl := ChanLayout{Chan: ch, Off: off}
		for _, f := range ch.Fields {
			cl.FieldOff = append(cl.FieldOff, cl.Width)
			cl.Width += f.Width()
		}
		l.Chans = append(l.Chans, cl)
		l.chans[ch.Name] = i
		off += 1 + ch.Capacity*cl.Width
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

func (s layoutScope) LookupChan(name string) *Channel {
	if i, ok := s.l.chans[name]; ok {
		return &s.l.Model.Channels[i]
	}
	return nil
}

func (s layoutScope) ProcessCount() int { return len(s.l.Model.Processes) }

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

// NeedsTable reports whether m's vector must carry the live-process table.
// The processes decide, and only they: some instance is created by `run` at
// runtime, some edge leaves the table when its process ends, or some
// expression of a process asks a question the table alone answers (`_nr_pr`,
// a runtime pid, "is this the youngest process").
//
// A property is deliberately not asked. It is read after the processes were
// written, so none of them removes a process from a table made for the
// property: `_nr_pr` would stay at the number of processes started and the
// verdict would describe another model. The explorer refuses such a property
// instead (explore/tableread.go), and a refused property must not enlarge the
// vector of the ones beside it.
func NeedsTable(m *Model) bool {
	for i := range m.Processes {
		if m.Processes[i].Dynamic {
			return true
		}
		for j := range m.Processes[i].Edges {
			if m.Processes[i].Edges[j].Leave {
				return true
			}
		}
	}
	found := false
	walkProcessExprs(m, func(e *Expr) {
		if isTableOp(e.Op) {
			found = true
		}
	})
	return found
}

// walkProcessExprs visits every expression of m's processes (properties are
// not processes: see NeedsTable).
func walkProcessExprs(m *Model, f func(*Expr)) {
	var walk func(e *Expr)
	walk = func(e *Expr) {
		if e == nil {
			return
		}
		f(e)
		for _, a := range e.Args {
			walk(a)
		}
	}
	for p := range m.Processes {
		pr := &m.Processes[p]
		walk(pr.Provided)
		for i := range pr.Edges {
			e := &pr.Edges[i]
			walk(e.Guard)
			walk(e.Assert)
			for _, a := range e.Effect {
				walk(a.Index)
				walk(a.Value)
			}
			if e.Send != nil {
				walk(e.Send.Sel)
				for _, a := range e.Send.Args {
					walk(a)
				}
			}
			if e.Recv != nil {
				walk(e.Recv.Sel)
				for _, a := range e.Recv.Args {
					walk(a.Index)
					walk(a.Match)
				}
			}
			if e.Run != nil {
				for _, a := range e.Run.Args {
					walk(a)
				}
				for _, a := range e.Run.Init {
					walk(a.Index)
					walk(a.Value)
				}
			}
		}
	}
}

// Initial builds the initial state vector: initial locations, Init values,
// empty channels, no exclusive holder, and the live-process table holding
// the processes that exist from the start (every non-claim, non-dynamic
// instance, in declaration order — SPIN's pid order).
func (l *Layout) Initial() []byte {
	s := make([]byte, l.Size)
	for p := range l.Model.Processes {
		l.WritePC(s, p, l.Model.Processes[p].Initial)
	}
	if l.TabOff >= 0 {
		n := 0
		for p := range l.Model.Processes {
			pr := &l.Model.Processes[p]
			if pr.Claim || pr.Dynamic {
				continue
			}
			s[l.TabOff+n] = byte(p)
			n++
		}
		s[l.NrOff] = byte(n)
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
	l := &Layout{Model: m, globals: map[string]*Slot{}, chans: map[string]int{}}
	for i := range m.Globals {
		l.place(&m.Globals[i], -1, 0, l.globals)
	}
	for i := range m.Channels {
		l.chans[m.Channels[i].Name] = i
	}
	pnames := map[string]bool{}
	// Every process's locals are placed before any edge is checked: a `run`
	// carries initialisers evaluated in the *target's* scope, so the scopes
	// of later processes must already exist when an earlier one is checked.
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
	}
	for p := range m.Processes {
		pr := &m.Processes[p]
		if pr.Params < 0 || pr.Params > len(pr.Locals) {
			return fmt.Errorf("processes[%d].params: %d outside 0..%d", p, pr.Params, len(pr.Locals))
		}
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
		if pr.Provided != nil {
			if _, err := Check(pr.Provided, l.scope(p)); err != nil {
				return fmt.Errorf("processes[%d].provided: %w", p, err)
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
			if e.Else && e.Guard != nil {
				return fmt.Errorf("%s: an else edge has no guard", path)
			}
			for j, ci := range e.ClearChans {
				if ci < 0 || ci >= len(m.Channels) {
					return fmt.Errorf("%s.clear_chans[%d]: channel %d outside 0..%d", path, j, ci, len(m.Channels)-1)
				}
			}
			ops := 0
			if e.Send != nil {
				ops++
				if e.Send.Sel != nil {
					if e.Send.Chan != "" {
						return fmt.Errorf("%s.send: a dynamic channel (sel) has no name", path)
					}
					if _, err := Check(e.Send.Sel, l.scope(p)); err != nil {
						return fmt.Errorf("%s.send.sel: %w", path, err)
					}
				} else {
					ch := l.scope(p).LookupChan(e.Send.Chan)
					if ch == nil {
						return fmt.Errorf("%s.send: undeclared channel %q", path, e.Send.Chan)
					}
					if len(e.Send.Args) != len(ch.Fields) {
						return fmt.Errorf("%s.send: %d argument(s) for %d field(s) of %s", path, len(e.Send.Args), len(ch.Fields), ch.Name)
					}
				}
				for j, a := range e.Send.Args {
					if _, err := Check(a, l.scope(p)); err != nil {
						return fmt.Errorf("%s.send.args[%d]: %w", path, j, err)
					}
				}
			}
			if e.Recv != nil {
				ops++
				if e.Recv.Sel != nil {
					if e.Recv.Chan != "" {
						return fmt.Errorf("%s.recv: a dynamic channel (sel) has no name", path)
					}
					if _, err := Check(e.Recv.Sel, l.scope(p)); err != nil {
						return fmt.Errorf("%s.recv.sel: %w", path, err)
					}
				} else {
					ch := l.scope(p).LookupChan(e.Recv.Chan)
					if ch == nil {
						return fmt.Errorf("%s.recv: undeclared channel %q", path, e.Recv.Chan)
					}
					if len(e.Recv.Args) != len(ch.Fields) {
						return fmt.Errorf("%s.recv: %d argument(s) for %d field(s) of %s", path, len(e.Recv.Args), len(ch.Fields), ch.Name)
					}
				}
				for j, a := range e.Recv.Args {
					apath := fmt.Sprintf("%s.recv.args[%d]", path, j)
					if a.Var != "" && a.Match != nil {
						return fmt.Errorf("%s: binds a variable and matches a value at once", apath)
					}
					if a.Var != "" {
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
					}
					if a.Match != nil {
						if _, err := Check(a.Match, l.scope(p)); err != nil {
							return fmt.Errorf("%s.match: %w", apath, err)
						}
					}
				}
			}
			if e.Run != nil {
				ops++
				r := e.Run
				targets := r.Targets()
				for _, q := range targets {
					if q < 0 || q >= len(m.Processes) {
						return fmt.Errorf("%s.run: process %d is not a process of the model", path, q)
					}
					if !m.Processes[q].Dynamic {
						return fmt.Errorf("%s.run: process %d (%s) is not a dynamic instance", path, q, m.Processes[q].Name)
					}
				}
				target := &m.Processes[targets[0]]
				if r.Entry < 0 || r.Entry >= len(target.Locations) {
					return fmt.Errorf("%s.run.entry: %d outside 0..%d", path, r.Entry, len(target.Locations)-1)
				}
				if len(r.Args) != target.Params {
					return fmt.Errorf("%s.run: %d argument(s) for %d parameter(s) of %s", path, len(r.Args), target.Params, target.Name)
				}
				for j, a := range r.Args {
					if _, err := Check(a, l.scope(p)); err != nil {
						return fmt.Errorf("%s.run.args[%d]: %w", path, j, err)
					}
				}
				for _, q := range targets {
					for j, a := range r.Init {
						if _, err := Check(a.Value, l.scope(q)); err != nil {
							return fmt.Errorf("%s.run.init[%d].value: %w", path, j, err)
						}
						if a.Index != nil {
							if _, err := Check(a.Index, l.scope(q)); err != nil {
								return fmt.Errorf("%s.run.init[%d].index: %w", path, j, err)
							}
						}
						if l.scope(q).LookupVar(a.Var) == nil {
							return fmt.Errorf("%s.run.init[%d]: %s has no variable %q", path, j, m.Processes[q].Name, a.Var)
						}
					}
				}
			}
			if ops > 1 {
				return fmt.Errorf("%s: an edge carries at most one of send, recv, run", path)
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
