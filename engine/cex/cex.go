// Package cex builds counterexamples: a finite run of the model as a list
// of steps, each naming the process, the command taken, the variables it
// changed with before/after values, and the source mapping of the command
// (14 §4.1, FR-015, NFR-011). Loops (prefix + cycle) arrive with G4.
package cex

import (
	"bytes"
	"fmt"
	"strings"

	"modelcheck/ir"
)

// Trace is a finite run from the initial state.
type Trace struct {
	Steps []Step `json:"steps"`
	// Final lists every variable of the last state, including unchanged
	// ones, so that a reader can see the whole marking / valuation at the
	// point of the violation without replaying the steps.
	Final []Value `json:"final_state"`
	// FinalChannels lists the buffer of every channel in the last state.
	FinalChannels []ChanValue `json:"final_channels,omitempty"`
	// Summary is the step texts joined with ", ", e.g. "t1, t4".
	Summary string `json:"summary"`
}

// ChanValue is a channel and its buffer, head first; each message is one
// value per field.
type ChanValue struct {
	Chan     string    `json:"chan"`
	Messages [][]int64 `json:"messages"`
}

// Step is one transition of the run.
type Step struct {
	Index   int    `json:"index"` // 1-based position in the run
	Process string `json:"process"`
	// Command is the edge text (ir.Edge.Text) or, when the frontend left it
	// empty, the guard and effect rendered from the IR.
	Command string   `json:"command"`
	Changes []Change `json:"changes,omitempty"`
	// Channels lists the buffers that the step changed, as they are after
	// it (a send, a receive or a rendezvous handshake).
	Channels []ChanValue `json:"channels,omitempty"`
	// Location is the process's control location after the step, when the
	// location has a name.
	Location string     `json:"location,omitempty"`
	Origin   *ir.Origin `json:"origin,omitempty"`
	// Partner is present for a rendezvous handshake: the receiver moved in
	// the same step.
	Partner *Partner `json:"partner,omitempty"`
}

// Change is one variable that differs between the source and target state.
type Change struct {
	Var    string `json:"var"`
	Before int64  `json:"before"`
	After  int64  `json:"after"`
}

// Value is a variable and its value in a state.
type Value struct {
	Var   string `json:"var"`
	Value int64  `json:"value"`
}

// Ref identifies an edge taken from a state and, for a rendezvous
// handshake, the receiving edge taken in the same step.
type Ref struct {
	Proc int
	Edge int

	HasPartner  bool
	PartnerProc int
	PartnerEdge int
}

// Partner is the receiving half of a rendezvous handshake step.
type Partner struct {
	Process  string     `json:"process"`
	Command  string     `json:"command"`
	Location string     `json:"location,omitempty"`
	Origin   *ir.Origin `json:"origin,omitempty"`
}

// Build renders a run given the sequence of states s0..sn and the edges
// taken between them (len(refs) == len(states)-1). Variable names are
// qualified with the process name for locals ("P.x") and bare for globals.
func Build(l *ir.Layout, states [][]byte, refs []Ref) *Trace {
	t := &Trace{}
	var texts []string
	for i, r := range refs {
		pr := &l.Model.Processes[r.Proc]
		e := &pr.Edges[r.Edge]
		st := Step{Index: i + 1, Process: pr.Name, Command: CommandText(e), Origin: e.Origin}
		if name := pr.Locations[e.To].Name; name != "" {
			st.Location = name
		}
		if r.HasPartner {
			qr := &l.Model.Processes[r.PartnerProc]
			qe := &qr.Edges[r.PartnerEdge]
			st.Partner = &Partner{Process: qr.Name, Command: CommandText(qe), Origin: qe.Origin}
			if name := qr.Locations[qe.To].Name; name != "" {
				st.Partner.Location = name
			}
		}
		before, after := states[i], states[i+1]
		for _, sl := range l.Slots {
			b, a := sl.Read(before), sl.Read(after)
			if b != a {
				st.Changes = append(st.Changes, Change{Var: slotName(l, sl), Before: b, After: a})
			}
		}
		for ci := range l.Chans {
			c := &l.Chans[ci]
			end := c.Off + 1 + c.Chan.Capacity*c.Width
			if !bytes.Equal(before[c.Off:end], after[c.Off:end]) {
				st.Channels = append(st.Channels, ChanValue{Chan: c.Chan.Name, Messages: l.ChanMessages(after, ci)})
			}
		}
		t.Steps = append(t.Steps, st)
		texts = append(texts, st.Command)
	}
	last := states[len(states)-1]
	for _, sl := range l.Slots {
		t.Final = append(t.Final, Value{Var: slotName(l, sl), Value: sl.Read(last)})
	}
	for ci := range l.Chans {
		t.FinalChannels = append(t.FinalChannels, ChanValue{Chan: l.Chans[ci].Chan.Name, Messages: l.ChanMessages(last, ci)})
	}
	t.Summary = strings.Join(texts, ", ")
	return t
}

// CommandText is the user-facing text of an edge.
func CommandText(e *ir.Edge) string {
	if e.Text != "" {
		return e.Text
	}
	var parts []string
	if e.Guard != nil {
		parts = append(parts, e.Guard.String()+" ->")
	}
	if e.Else {
		parts = append(parts, "else")
	}
	if e.Assert != nil {
		parts = append(parts, "assert("+e.Assert.String()+")")
	}
	if e.Run != nil {
		var args []string
		for _, a := range e.Run.Args {
			args = append(args, a.String())
		}
		parts = append(parts, fmt.Sprintf("run #%d(%s)", e.Run.Proc, strings.Join(args, ", ")))
	}
	if e.Send != nil {
		var args []string
		for _, a := range e.Send.Args {
			args = append(args, a.String())
		}
		parts = append(parts, e.Send.Chan+"!"+strings.Join(args, ","))
	}
	if e.Recv != nil {
		var args []string
		for _, a := range e.Recv.Args {
			switch {
			case a.Var != "" && a.Index != nil:
				args = append(args, a.Var+"["+a.Index.String()+"]")
			case a.Var != "":
				args = append(args, a.Var)
			case a.Match != nil:
				args = append(args, a.Match.String())
			default:
				args = append(args, "_")
			}
		}
		parts = append(parts, e.Recv.Chan+"?"+strings.Join(args, ","))
	}
	for _, a := range e.Effect {
		target := a.Var
		if a.Index != nil {
			target += "[" + a.Index.String() + "]"
		}
		parts = append(parts, target+" = "+a.Value.String())
	}
	if len(parts) == 0 {
		return "skip"
	}
	return strings.Join(parts, " ")
}

// UserNames returns, per step, the user's name for the command taken:
// Origin.Name when the frontend recorded one, else the command text.
func (t *Trace) UserNames() []string {
	out := make([]string, len(t.Steps))
	for i, s := range t.Steps {
		if s.Origin != nil && s.Origin.Name != "" {
			out[i] = s.Origin.Name
		} else {
			out[i] = s.Command
		}
	}
	return out
}

// NonZero renders the final state as "p2=1 p5=1": the non-zero variables in
// layout order, which is the natural reading of a Petri-net marking.
func (t *Trace) NonZero() string {
	var parts []string
	for _, v := range t.Final {
		if v.Value != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", v.Var, v.Value))
		}
	}
	if len(parts) == 0 {
		return "empty"
	}
	return strings.Join(parts, " ")
}

func slotName(l *ir.Layout, sl *ir.Slot) string {
	if sl.Proc >= 0 {
		return l.Model.Processes[sl.Proc].Name + "." + sl.Name()
	}
	return sl.Name()
}
