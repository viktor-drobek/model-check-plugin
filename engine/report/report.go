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
//   - `inconclusive` carries a `reason` naming the exhausted resource and
//     evidence `bounded` when a declared states or depth limit was reached,
//     `unknown` when time or memory ran out (plan 14 §6); complete is
//     false.
//   - `invalid-model` and `not-executed` carry `unknown` and a `reason`; an
//     invalid-model result attaches the run to the offending step as its
//     counterexample.
//   - `unknown` is in the vocabulary but the engine never produces it.
//
// An ltl / progress property (G4) has its own search: its counters and
// `complete` are those of the product search, and `temporal` describes the
// claim (formula, negation, atoms, stutter invariance, automaton size,
// fairness). A cycle counterexample carries `loop` (see package cex).
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
	// Warnings are frontend notes that do not change the verdict but that
	// a reader must know to interpret it (G1: `printf` output is not
	// produced; a never claim was parsed and not executed). Deterministic
	// order: the frontend's textual order.
	Warnings []string `json:"warnings,omitempty"`
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

// Search describes the run's safety search (deadlock, invariant, reach,
// assert), or the first temporal search when there is no safety property.
// An ltl / progress property has its own product search: read its own
// `complete` and counters, not these.
type Search struct {
	Mode     string `json:"mode"`
	Budget   Budget `json:"budget"`
	Stop     string `json:"stop"`
	Complete bool   `json:"complete"`
}

// Budget echoes the limits the run was given (0 = no limit, which the CLI
// sets only under --unlimited).
type Budget struct {
	States   int   `json:"states"`
	Depth    int   `json:"depth"`
	TimeMS   int64 `json:"time_ms"`
	MemBytes int64 `json:"mem_bytes"`
}

// Explore converts the echoed budget to the explorer's (0 = no limit in
// both).
func (b Budget) Explore() explore.Budget {
	return explore.Budget{MaxStates: b.States, MaxDepth: b.Depth, MaxMemBytes: b.MemBytes}
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
	// Temporal is present for ltl, progress and ctl properties.
	Temporal *Temporal `json:"temporal,omitempty"`
	// Warnings are hints about this property that do not change its verdict
	// (the vacuity hints of FR-011). They are repeated in Report.Warnings.
	Warnings []string `json:"warnings,omitempty"`
}

// Temporal describes how an ltl / progress / ctl property was checked.
type Temporal struct {
	// Logic is "ltl" (an automaton in synchronous product: kinds ltl and
	// progress) or "ctl" (graph labelling: kind ctl). A ctl property is
	// never answered by an LTL automaton and an ltl property never by CTL
	// labelling, and this field is how a caller checks that.
	Logic string `json:"logic"`
	// Source: formula | never-claim | accept-labels | np
	Source  string `json:"source"`
	Formula string `json:"formula,omitempty"`
	// Negated is the LTL formula whose automaton was run as the claim; an
	// acceptance cycle satisfies it and violates Formula.
	Negated string `json:"negated,omitempty"`
	// Normalised is the CTL formula in the EX/EU/EG basis that was labelled.
	Normalised       string   `json:"normalised,omitempty"`
	Atoms            []string `json:"atoms,omitempty"`
	StutterInvariant *bool    `json:"stutter_invariant,omitempty"`
	AutomatonStates  int      `json:"automaton_states,omitempty"`
	AutomatonTrans   int      `json:"automaton_transitions,omitempty"`
	AutomatonAccept  int      `json:"automaton_accepting,omitempty"`
	Fairness         string   `json:"fairness"`
	Claim            string   `json:"claim,omitempty"`
	// Note records a semantic decision needed to read the verdict.
	Note string `json:"note,omitempty"`
	// WitnessNote says, when the verdict carries no run, which rule of the
	// witness division applies — the engine's honest "not available".
	WitnessNote string `json:"witness_note,omitempty"`
	// Vacuous marks the FR-011 hint that the implication's antecedent named
	// in VacuousAtom is never true in a reachable state. It is a hint: the
	// status and evidence above are what the search found.
	Vacuous     bool   `json:"vacuous,omitempty"`
	VacuousAtom string `json:"vacuous_atom,omitempty"`
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
	Warnings []string
}

// Build turns an explorer result into a report, enforcing the status rules.
func Build(m *ir.Model, res *explore.Result, meta Meta) (*Report, error) {
	r := &Report{
		Engine:   Engine{Name: EngineName, Version: EngineVersion, IRSchema: ir.Schema, ReportSchema: ReportSchema},
		Inputs:   meta.Inputs,
		Model:    ModelInfo{Name: m.Name, StateBytes: res.StateBytes, Processes: len(m.Processes), Variables: countVars(m)},
		Search:   Search{Mode: string(meta.Mode), Budget: meta.Budget, Stop: res.Stop, Complete: res.Complete},
		Warnings: meta.Warnings,
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
		counters := Counters{States: res.States, Transitions: res.Transitions, Depth: res.MaxDepth, TimeMS: tm, MemoryBytesEst: res.MemBytes}
		complete := res.Complete
		if st := o.Stats; st != nil {
			counters = Counters{States: st.States, Transitions: st.Transitions, Depth: st.MaxDepth, TimeMS: tm, MemoryBytesEst: st.MemBytes}
			if !meta.NoTiming {
				ms := st.Elapsed.Milliseconds()
				counters.TimeMS = &ms
			}
			complete = st.Complete
		}
		p := Property{
			ID: o.Property.ID, Kind: o.Property.Kind, Text: o.Property.Text,
			Status: string(o.Status), Evidence: string(o.Evidence),
			Counters: counters, Complete: complete, Reason: o.Reason,
		}
		if ti := o.Temporal; ti != nil {
			p.Temporal = &Temporal{Logic: ti.Logic, Source: ti.Source, Formula: ti.Formula, Negated: ti.Negated,
				Normalised: ti.Normalised, Atoms: ti.Atoms,
				StutterInvariant: ti.StutterInvariant, AutomatonStates: ti.AutomatonStates, AutomatonTrans: ti.AutomatonTrans,
				AutomatonAccept: ti.AutomatonAccept, Fairness: ti.Fairness, Claim: ti.Claim,
				Note: ti.Note, WitnessNote: ti.WitnessNote, Vacuous: ti.Vacuous, VacuousAtom: ti.VacuousAtom}
			if p.Temporal.Atoms == nil {
				p.Temporal.Atoms = []string{}
			}
		}
		p.Warnings = o.Warnings
		r.Warnings = append(r.Warnings, o.Warnings...)
		switch o.Status {
		case explore.Verified:
			switch o.Property.Kind {
			case ir.KindReach:
				// The witness is an exact run, so completeness is not needed.
				p.Witness = o.Trace
			case ir.KindCTL:
				// A CTL verdict is only ever given on a complete graph, and
				// an existential formula that holds brings a witness with it.
				p.Witness = o.Trace
				if !complete {
					return nil, fmt.Errorf("report: property %q is verified but the search is not complete", o.Property.ID)
				}
			default:
				if !complete {
					return nil, fmt.Errorf("report: property %q is verified but the search is not complete", o.Property.ID)
				}
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
			if (o.Evidence != explore.Bounded && o.Evidence != explore.EvUnknown) || o.Reason == "" || complete {
				return nil, fmt.Errorf("report: inconclusive %q needs bounded or unknown evidence, a reason and complete=false", o.Property.ID)
			}
		case explore.InvalidModel:
			p.Counterexample = o.Trace
			fallthrough
		case explore.NotExecuted:
			if o.Evidence != explore.EvUnknown || o.Reason == "" {
				return nil, fmt.Errorf("report: %s %q needs unknown evidence and a reason", o.Status, o.Property.ID)
			}
		case explore.Unknown:
			return nil, fmt.Errorf("report: the engine does not produce status unknown (%q)", o.Property.ID)
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
