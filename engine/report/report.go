// Package report defines the JSON result of a check (plan 14 §3, §6): per
// property a status from the 11 §14 vocabulary, an evidence level, counters,
// the completeness flag and, when there is one, the run that decides it;
// plus the engine version and the hashes of the inputs.
//
// The output is deterministic for a given input: fixed struct field order,
// no maps, and every field is a function of the input except `time_ms`,
// which callers may omit (Meta.NoTiming) for byte-for-byte comparison.
//
// Status/evidence rules enforced by Build (they are the K1 rules):
//
//   - `verified` carries evidence `exhaustive`. For deadlock, invariant and
//     assert it requires complete = true, and Build refuses it otherwise; for
//     `reach` it rests on the attached witness (an exact run), so complete
//     may be false.
//   - `violated` carries `exhaustive`: for deadlock, invariant and assert
//     because its counterexample is an exact run of the model, whatever
//     stopped the search afterwards; for `reach` because only a complete
//     search can establish that no state satisfies the condition.
//   - `inconclusive` carries `bounded` and a `reason` naming the exhausted
//     resource; complete is false.
//   - `invalid-model` and `not-executed` carry `unknown` and a `reason`; an
//     invalid-model result attaches the run to the offending step as its
//     counterexample.
//   - `unknown` is in the vocabulary but the G0 engine never produces it.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"

	"modelcheck/cex"
	"modelcheck/explore"
	"modelcheck/ir"
)

const (
	EngineName    = "mcd"
	EngineVersion = "0.1.0-g0"
	ReportSchema  = "mcd-report/1"
)

// Report is the whole document.
type Report struct {
	Engine     Engine     `json:"engine"`
	Inputs     []Input    `json:"inputs"`
	Model      ModelInfo  `json:"model"`
	Search     Search     `json:"search"`
	Properties []Property `json:"properties"`
}

type Engine struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	IRSchema     string `json:"ir_schema"`
	ReportSchema string `json:"report_schema"`
}

// Input is one file the run depended on.
type Input struct {
	Kind   string `json:"kind"` // petri | ir
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type ModelInfo struct {
	Name       string `json:"name"`
	StateBytes int    `json:"state_bytes"`
	Processes  int    `json:"processes"`
	Variables  int    `json:"variables"`
}

type Search struct {
	Mode     string `json:"mode"`
	Budget   Budget `json:"budget"`
	Stop     string `json:"stop"`
	Complete bool   `json:"complete"`
}

// Budget echoes the limits the run was given (0 = unlimited).
type Budget struct {
	States   int   `json:"states"`
	Depth    int   `json:"depth"`
	TimeMS   int64 `json:"time_ms"`
	MemBytes int64 `json:"mem_bytes"`
}

type Property struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Text     string   `json:"text,omitempty"`
	Status   string   `json:"status"`
	Evidence string   `json:"evidence"`
	Counters Counters `json:"counters"`
	Complete bool     `json:"complete"`
	// Counterexample: the run that violates the property (violated) or that
	// reaches the offending step (invalid-model).
	Counterexample *cex.Trace `json:"counterexample,omitempty"`
	// Witness: the run that reaches the condition of a verified `reach`.
	Witness *cex.Trace `json:"witness,omitempty"`
	Reason  string     `json:"reason,omitempty"`
}

// Counters describe the whole run (they are the same for every property of
// one report). Depth is the greatest depth, in transitions from the initial
// state, of a state that was expanded.
type Counters struct {
	States      int `json:"states"`
	Transitions int `json:"transitions"`
	Depth       int `json:"depth"`
	// TimeMS is omitted when the caller asked for a timing-free report.
	TimeMS         *int64 `json:"time_ms,omitempty"`
	MemoryBytesEst int64  `json:"memory_bytes_est"`
}

// Meta is what the caller knows and the explorer does not.
type Meta struct {
	Inputs   []Input
	Mode     explore.Mode
	Budget   Budget
	NoTiming bool
}

// Build turns an explorer result into a report, enforcing the status rules.
func Build(m *ir.Model, res *explore.Result, meta Meta) (*Report, error) {
	r := &Report{
		Engine: Engine{Name: EngineName, Version: EngineVersion, IRSchema: ir.Schema, ReportSchema: ReportSchema},
		Inputs: meta.Inputs,
		Model:  ModelInfo{Name: m.Name, StateBytes: res.StateBytes, Processes: len(m.Processes), Variables: countVars(m)},
		Search: Search{Mode: string(meta.Mode), Budget: meta.Budget, Stop: res.Stop, Complete: res.Complete},
	}
	if r.Inputs == nil {
		r.Inputs = []Input{}
	}
	var tm *int64
	if !meta.NoTiming {
		ms := res.Elapsed.Milliseconds()
		tm = &ms
	}
	for _, o := range res.Outcomes {
		p := Property{
			ID: o.Property.ID, Kind: o.Property.Kind, Text: o.Property.Text,
			Status: string(o.Status), Evidence: string(o.Evidence),
			Counters: Counters{States: res.States, Transitions: res.Transitions, Depth: res.MaxDepth, TimeMS: tm, MemoryBytesEst: res.MemBytes},
			Complete: res.Complete, Reason: o.Reason,
		}
		switch o.Status {
		case explore.Verified:
			if o.Property.Kind == ir.KindReach {
				p.Witness = o.Trace
			} else if !res.Complete {
				return nil, fmt.Errorf("report: property %q is verified but the search is not complete", o.Property.ID)
			}
			if o.Evidence != explore.Exhaustive {
				return nil, fmt.Errorf("report: verified %q with evidence %q", o.Property.ID, o.Evidence)
			}
		case explore.Violated:
			p.Counterexample = o.Trace
			if o.Evidence != explore.Exhaustive {
				return nil, fmt.Errorf("report: violated %q with evidence %q", o.Property.ID, o.Evidence)
			}
		case explore.Inconclusive:
			if o.Evidence != explore.Bounded || o.Reason == "" || res.Complete {
				return nil, fmt.Errorf("report: inconclusive %q needs bounded evidence, a reason and complete=false", o.Property.ID)
			}
		case explore.InvalidModel:
			p.Counterexample = o.Trace
			fallthrough
		case explore.NotExecuted:
			if o.Evidence != explore.EvUnknown || o.Reason == "" {
				return nil, fmt.Errorf("report: %s %q needs unknown evidence and a reason", o.Status, o.Property.ID)
			}
		case explore.Unknown:
			return nil, fmt.Errorf("report: the G0 engine does not produce status unknown (%q)", o.Property.ID)
		default:
			return nil, fmt.Errorf("report: status %q of %q is outside the vocabulary", o.Status, o.Property.ID)
		}
		r.Properties = append(r.Properties, p)
	}
	if r.Properties == nil {
		r.Properties = []Property{}
	}
	return r, nil
}

func countVars(m *ir.Model) int {
	n := len(m.Globals)
	for _, p := range m.Processes {
		n += len(p.Locals)
	}
	return n
}

// JSON renders r with two-space indentation and a trailing newline.
func (r *Report) JSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
