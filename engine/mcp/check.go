package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/cex"
	"modelcheck/cli"
	"modelcheck/explore"
	"modelcheck/ir"
	"modelcheck/report"
)

// Property kinds the tool interface accepts (plan 14 §6). `assert` is the
// IR's own implicit kind and is added by the engine, not requested.
var checkKinds = map[string]bool{"invariant": true, "deadlock": true, "reach": true, "ltl": true, "ctl": true, "progress": true}

// notExecutedReason names, per kind, the missing capability and the step
// that brings it. Every kind of plan 14 §6 is executed since G5 (ltl and
// progress in G4, ctl here), so the table is empty: a `not-executed` now
// comes from the engine itself, with the engine's own reason (strong
// fairness for ltl, any fairness for ctl, a kind the IR carries that this
// version does not know).
var notExecutedReason = map[string]string{}

// PropertyIn is one property to check.
type PropertyIn struct {
	ID      string `json:"id"`
	Kind    string `json:"kind" jsonschema:"invariant | deadlock | reach | ltl | ctl | progress"`
	Expr    any    `json:"expr,omitempty" jsonschema:"boolean state expression: IR expression JSON ({op, args, var, value}) or a bare variable name; required for invariant and reach"`
	Formula string `json:"formula,omitempty" jsonschema:"ltl: the formula in SPIN syntax ([] <> U V X ! && || -> <->); omitted = the model's own never claim, else its accept labels (SPIN pan -a). ctl (required): the formula in CTL syntax (AG AF AX EG EF EX, A[f U g], E[f U g]); atoms of both are global variables, parenthesised comparisons, len/empty/full of a channel, pc_value(n) or P@label, with the #define symbols of a Promela model parsed in this session expanded. CTL is decided by graph labelling and LTL by an automaton; neither is rewritten into the other"`
	Text    string `json:"text,omitempty" jsonschema:"the user's statement of the property"`
}

// CheckIn is the input of mc_check.
type CheckIn struct {
	SessionID  string       `json:"session_id,omitempty" jsonschema:"session holding the parsed model; omitted = a new session (then ir is required)"`
	IR         any          `json:"ir,omitempty" jsonschema:"IR JSON to check; omitted = the session's parsed model"`
	Properties []PropertyIn `json:"properties,omitempty" jsonschema:"properties to check; they replace the model's own properties when given, omitted = the model's own; in both cases the engine adds its implicit property 'assert' when some edge carries an assert, so that asserts are never checked silently"`
	Fairness   string       `json:"fairness,omitempty" jsonschema:"none | weak | strong (default none); applies to ltl and progress: weak = every continuously enabled process eventually moves (pan -f, n+2 copies); strong is not executed and makes those properties not-executed with a reason (FR-008)"`
	Budget     *Budget      `json:"budget,omitempty" jsonschema:"limits; an absent or 0 field takes the server default (the same reading as mcd check); fields above the server ceiling are clamped"`
	Search     string       `json:"search,omitempty" jsonschema:"dfs | bfs (default dfs; bfs gives shortest counterexamples)"`
	NoTiming   bool         `json:"no_timing,omitempty" jsonschema:"omit time_ms from the report so that reports are byte-for-byte reproducible"`
	Aggregate  bool         `json:"aggregate,omitempty" jsonschema:"also return one aggregate status by the fixed priority invalid-model > not-executed > violated > inconclusive > unknown > verified"`
}

// SearchOut describes the run.
type SearchOut struct {
	Mode            string   `json:"mode"`
	BudgetRequested Budget   `json:"budget_requested"`
	BudgetApplied   Budget   `json:"budget_applied" jsonschema:"after defaults and ceiling"`
	BudgetNotes     []string `json:"budget_notes" jsonschema:"one note per clamped field; empty when nothing was clamped"`
	Stop            string   `json:"stop" jsonschema:"why the search ended"`
	Complete        bool     `json:"complete" jsonschema:"true only when the whole reachable graph was expanded"`
}

// TraceRef points at a stored run.
type TraceRef struct {
	ID        string   `json:"id" jsonschema:"pass to mc_explain"`
	Path      string   `json:"path" jsonschema:"session file with the full trace"`
	Summary   string   `json:"summary" jsonschema:"the commands taken, comma-separated"`
	Steps     int      `json:"steps"`
	UserNames []string `json:"user_names" jsonschema:"per step, the user's name for the command when the frontend recorded one, else the command text"`
	// Loop is present for a cycle counterexample: the steps from Start
	// (1-based) on repeat forever.
	Loop *cex.Loop `json:"loop,omitempty" jsonschema:"cycle counterexamples: start (1-based index of the first loop step) and steps (loop length)"`
}

// PropertyOut is the result for one property.
type PropertyOut struct {
	ID             string           `json:"id"`
	Kind           string           `json:"kind"`
	Text           string           `json:"text,omitempty"`
	Status         string           `json:"status" jsonschema:"verified | violated | inconclusive | unknown | not-executed | invalid-model"`
	Evidence       string           `json:"evidence" jsonschema:"exhaustive | bounded | approximate | unknown"`
	Complete       bool             `json:"complete"`
	Reason         string           `json:"reason,omitempty" jsonschema:"inconclusive: the exhausted resource; not-executed: the missing capability; invalid-model: the offending step"`
	Counters       report.Counters  `json:"counters"`
	Counterexample *TraceRef        `json:"counterexample,omitempty"`
	Witness        *TraceRef        `json:"witness,omitempty"`
	Temporal       *report.Temporal `json:"temporal,omitempty" jsonschema:"ltl / progress: the claim used (logic ltl, formula, negation, atoms, stutter invariance, automaton size, fairness); ctl: logic ctl, the formula, its normalisation into the EX/EU/EG basis, the atoms, the vacuity hint and, when the verdict carries no run, why"`
	// Warnings are hints about this property that do not change its
	// verdict — the vacuity hints of FR-011.
	Warnings []string `json:"warnings,omitempty"`
}

// Aggregate is the optional single status.
type Aggregate struct {
	Status string `json:"status"`
	Basis  string `json:"basis"`
}

// CheckOut is the answer of mc_check.
type CheckOut struct {
	SessionID  string        `json:"session_id"`
	Outcome    string        `json:"outcome" jsonschema:"report | rejected"`
	ReportPath string        `json:"report_path,omitempty" jsonschema:"session file with the full report (mcd-report/1)"`
	Search     *SearchOut    `json:"search,omitempty"`
	Properties []PropertyOut `json:"properties,omitempty"`
	Warnings   []string      `json:"warnings"`
	Aggregate  *Aggregate    `json:"aggregate,omitempty"`
	Rejection  *Rejection    `json:"rejection,omitempty"`
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// exprFrom accepts an IR expression object or a bare variable name.
func exprFrom(v any) (*ir.Expr, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case string:
		if !identRe.MatchString(x) {
			return nil, fmt.Errorf("expr %q: a string expression must be a variable name; use IR expression JSON for anything else", x)
		}
		return ir.Ref(x), nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var e ir.Expr
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return nil, fmt.Errorf("expr: %v", err)
	}
	return &e, nil
}

func (s *Server) check(ctx context.Context, req *sdk.CallToolRequest, in CheckIn) (*sdk.CallToolResult, *CheckOut, error) {
	if in.SessionID == "" && in.IR == nil {
		return nil, nil, errors.New("no model: pass `ir` inline or a session_id whose model was parsed with mc_parse")
	}
	mode := explore.DFS
	switch in.Search {
	case "", "dfs":
	case "bfs":
		mode = explore.BFS
	default:
		return nil, nil, fmt.Errorf("search must be dfs or bfs, got %q", in.Search)
	}
	switch in.Fairness {
	case "", cli.FairnessNone, cli.FairnessWeak, cli.FairnessStrong:
	default:
		return nil, nil, fmt.Errorf("fairness must be none, weak or strong, got %q", in.Fairness)
	}
	// Properties are validated before any session work, so that a client
	// mistake is a tool error and leaves no trace in the session.
	var props []ir.Property
	if in.Properties != nil {
		seen := map[string]bool{}
		for i, p := range in.Properties {
			if p.ID == "" {
				return nil, nil, fmt.Errorf("properties[%d]: id is required", i)
			}
			if seen[p.ID] {
				return nil, nil, fmt.Errorf("properties[%d]: duplicate id %q", i, p.ID)
			}
			seen[p.ID] = true
			if !checkKinds[p.Kind] {
				return nil, nil, fmt.Errorf("properties[%d] (%s): kind must be one of invariant, deadlock, reach, ltl, ctl, progress; got %q", i, p.ID, p.Kind)
			}
			e, err := exprFrom(p.Expr)
			if err != nil {
				return nil, nil, fmt.Errorf("properties[%d] (%s): %v", i, p.ID, err)
			}
			if e == nil && (p.Kind == "invariant" || p.Kind == "reach") {
				return nil, nil, fmt.Errorf("properties[%d] (%s): kind %s requires expr", i, p.ID, p.Kind)
			}
			if (p.Kind == "ltl" || p.Kind == "progress" || p.Kind == "ctl") && e != nil {
				return nil, nil, fmt.Errorf("properties[%d] (%s): kind %s takes formula, not expr", i, p.ID, p.Kind)
			}
			if p.Kind == "ctl" && p.Formula == "" {
				return nil, nil, fmt.Errorf("properties[%d] (%s): kind ctl requires formula (there is no \"the model as written\" reading of CTL: that reading belongs to a never claim, which is an ltl property)", i, p.ID)
			}
			if p.Kind != "ltl" && p.Kind != "ctl" && p.Formula != "" {
				return nil, nil, fmt.Errorf("properties[%d] (%s): formula is for kinds ltl and ctl only", i, p.ID)
			}
			text := p.Text
			if text == "" && p.Formula != "" {
				text = p.Formula
			}
			props = append(props, ir.Property{ID: p.ID, Kind: p.Kind, Expr: e, Formula: p.Formula, Text: text})
		}
	}

	sess, err := s.session(in.SessionID, true)
	if err != nil {
		return nil, nil, err
	}
	timer := begin(sess, "mc_check")
	out := &CheckOut{SessionID: sess.ID, Warnings: []string{}}
	var params *Params
	var artifacts []string
	defer func() { timer.end(err, params, artifacts) }()

	m, rej, err := s.modelFor(sess, in.IR)
	if err != nil {
		return nil, nil, err
	}
	if rej != nil {
		out.Outcome, out.Rejection = "rejected", rej
		return nil, out, nil
	}
	if props != nil {
		mm := *m
		mm.Properties = props
		m = &mm
	}
	if len(m.Properties) == 0 && !hasAsserts(m) {
		// Nothing to check is a client mistake, not a result with no
		// properties (an empty result would invite an aggregate over
		// nothing, and no status of the vocabulary describes "nothing was
		// asked").
		err = errors.New("no properties: the model declares none and none were given; pass `properties`")
		return nil, nil, err
	}
	fairness := in.Fairness
	if fairness == "" {
		fairness = cli.FairnessNone
	}
	if fairness != cli.FairnessNone {
		hasTemporal := false
		for _, p := range m.Properties {
			if p.Kind == "ltl" || p.Kind == "progress" {
				hasTemporal = true
			}
		}
		if !hasTemporal {
			out.Warnings = append(out.Warnings, "fairness "+fairness+" has no effect: it applies to ltl and progress properties only, and none was given (a ctl property asked with fairness is not executed, because fairness for CTL is out of scope — plan 14 §4.2)")
		}
	}
	sess.mu.Lock()
	defines := sess.defines
	sess.mu.Unlock()
	if in.IR != nil {
		defines = nil // an inline IR carries no #define table
	}

	var requested Budget
	if in.Budget != nil {
		requested = *in.Budget
	}
	applied, notes := Clamp(requested, s.cfg.Default, s.cfg.Ceiling)
	params = &Params{Search: string(mode), Fairness: fairness, BudgetApplied: &applied}

	release, err := s.acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	runCtx := ctx
	if applied.MS > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(applied.MS)*time.Millisecond)
		defer cancel()
	}
	res, runErr := explore.Run(runCtx, m, explore.Options{Mode: mode, Budget: explore.Budget{
		MaxStates: applied.States, MaxDepth: applied.Depth, MaxMemBytes: applied.MemoryMB << 20,
	}, Fairness: fairness, Defines: defines})
	if runErr != nil {
		// Validated IR that does not compile (an undeclared variable in a
		// property expression, a malformed or unresolvable LTL formula): the
		// input is refused, nothing is claimed.
		out.Outcome = "rejected"
		out.Rejection = &Rejection{Kind: "ir", Construct: "expression", Reason: runErr.Error()}
		var fe *explore.FormulaError
		if errors.As(runErr, &fe) {
			out.Rejection = &Rejection{Kind: "ltl", Construct: "formula", Reason: runErr.Error()}
		}
		return nil, out, nil
	}

	sess.mu.Lock()
	inputs := []report.Input{{Kind: sess.modelInput.Kind, Path: sess.modelInput.Source, SHA256: sess.modelInput.SHA256}}
	sess.mu.Unlock()
	rep, err := report.Build(m, res, report.Meta{
		Inputs: inputs, Mode: mode, NoTiming: in.NoTiming,
		Budget: report.Budget{States: applied.States, Depth: applied.Depth, TimeMS: applied.MS, MemBytes: applied.MemoryMB << 20},
	})
	if err != nil {
		err = fmt.Errorf("internal: %v", err)
		return nil, nil, err
	}
	for i := range rep.Properties {
		if r, ok := notExecutedReason[rep.Properties[i].Kind]; ok && rep.Properties[i].Status == string(explore.NotExecuted) {
			rep.Properties[i].Reason = r
		}
	}
	data, err := rep.JSON()
	if err != nil {
		return nil, nil, err
	}
	sess.mu.Lock()
	rel := sess.next("check", ".json")
	sess.mu.Unlock()
	if out.ReportPath, err = sess.WriteFile(rel, data); err != nil {
		return nil, nil, err
	}
	artifacts = append(artifacts, rel)

	out.Outcome = "report"
	out.Warnings = append(out.Warnings, rep.Warnings...)
	out.Search = &SearchOut{Mode: string(mode), BudgetRequested: requested, BudgetApplied: applied, BudgetNotes: notes, Stop: res.Stop, Complete: res.Complete}
	for _, p := range rep.Properties {
		po := PropertyOut{ID: p.ID, Kind: p.Kind, Text: p.Text, Status: p.Status, Evidence: p.Evidence, Complete: p.Complete, Reason: p.Reason, Counters: p.Counters, Temporal: p.Temporal, Warnings: p.Warnings}
		if p.Counterexample != nil {
			ref, rel, e := s.storeTrace(sess, p.ID, "counterexample", p.Counterexample)
			if e != nil {
				err = e
				return nil, nil, err
			}
			po.Counterexample = ref
			artifacts = append(artifacts, rel)
		}
		if p.Witness != nil {
			ref, rel, e := s.storeTrace(sess, p.ID, "witness", p.Witness)
			if e != nil {
				err = e
				return nil, nil, err
			}
			po.Witness = ref
			artifacts = append(artifacts, rel)
		}
		out.Properties = append(out.Properties, po)
	}
	if out.Properties == nil {
		out.Properties = []PropertyOut{}
	}
	if in.Aggregate {
		out.Aggregate = aggregate(out.Properties)
	}
	return nil, out, nil
}

// storeTrace writes a run as cex/<id>.json and registers the id. It returns
// the reference for the answer and the session-relative path for the
// manifest.
func (s *Server) storeTrace(sess *Session, property, role string, t *cex.Trace) (*TraceRef, string, error) {
	sess.mu.Lock()
	id := sess.next("cex", "")
	rel := filepath.Join("cex", id+".json")
	sess.cexs = append(sess.cexs, cexEntry{ID: id, Property: property, Role: role, Path: rel})
	sess.mu.Unlock()
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return nil, "", err
	}
	p, err := sess.WriteFile(rel, append(data, '\n'))
	if err != nil {
		return nil, "", err
	}
	ref := &TraceRef{ID: id, Path: p, Summary: t.Summary, Steps: len(t.Steps), UserNames: t.UserNames(), Loop: t.Loop}
	if ref.UserNames == nil {
		ref.UserNames = []string{}
	}
	return ref, rel, nil
}

// hasAsserts reports whether the engine will add its implicit `assert`
// property (some edge carries an assert).
func hasAsserts(m *ir.Model) bool {
	for _, p := range m.Processes {
		for _, e := range p.Edges {
			if e.Assert != nil {
				return true
			}
		}
	}
	return false
}

// aggregate applies the fixed priority of plan 14 §6. The list is never
// empty here (check refuses a call with nothing to check), so the aggregate
// is always one of the properties' own statuses; nil is returned rather than
// inventing a status for an empty list.
func aggregate(props []PropertyOut) *Aggregate {
	order := []string{"invalid-model", "not-executed", "violated", "inconclusive", "unknown", "verified"}
	for _, st := range order {
		for _, p := range props {
			if p.Status == st {
				return &Aggregate{Status: st, Basis: "priority invalid-model > not-executed > violated > inconclusive > unknown > verified (plan 14 §6); property " + p.ID}
			}
		}
	}
	return nil
}

// --- mc_explain -----------------------------------------------------------------

// ExplainIn is the input of mc_explain.
type ExplainIn struct {
	SessionID        string `json:"session_id"`
	CounterexampleID string `json:"counterexample_id" jsonschema:"id from mc_check (counterexample.id or witness.id)"`
}

// ExplainStep is one step with its diff and the user's name for it.
type ExplainStep struct {
	Index    int          `json:"index"`
	Process  string       `json:"process"`
	Command  string       `json:"command"`
	UserName string       `json:"user_name" jsonschema:"Origin.Name when recorded, else the command text"`
	Location string       `json:"location,omitempty"`
	Changes  []cex.Change `json:"changes" jsonschema:"variables that changed, before/after"`
	Origin   *ir.Origin   `json:"origin,omitempty"`
}

// ExplainOut is the answer of mc_explain.
type ExplainOut struct {
	SessionID  string        `json:"session_id"`
	ID         string        `json:"id"`
	PropertyID string        `json:"property_id"`
	Role       string        `json:"role" jsonschema:"counterexample | witness"`
	Path       string        `json:"path"`
	Prefix     []ExplainStep `json:"prefix" jsonschema:"the steps before the loop (all steps of a finite run)"`
	Loop       []ExplainStep `json:"loop" jsonschema:"the repeated part of a lasso (ltl, progress): after its last step the state equals the state before its first; empty for a finite run"`
	LoopNote   string        `json:"loop_note"`
	FinalState []cex.Value   `json:"final_state"`
	Summary    string        `json:"summary"`
	UserNames  []string      `json:"user_names"`
}

func (s *Server) explain(ctx context.Context, req *sdk.CallToolRequest, in ExplainIn) (*sdk.CallToolResult, *ExplainOut, error) {
	sess, err := s.session(in.SessionID, false)
	if err != nil {
		return nil, nil, err
	}
	timer := begin(sess, "mc_explain")
	defer func() { timer.end(err, nil, nil) }()
	if in.CounterexampleID == "" {
		err = errors.New("counterexample_id is required")
		return nil, nil, err
	}
	// The id becomes a path; the guard decides before any lookup.
	rel := filepath.Join("cex", in.CounterexampleID+".json")
	if _, err = sess.Path(rel); err != nil {
		return nil, nil, err
	}
	sess.mu.Lock()
	var entry *cexEntry
	for i := range sess.cexs {
		if sess.cexs[i].ID == in.CounterexampleID {
			entry = &sess.cexs[i]
		}
	}
	sess.mu.Unlock()
	if entry == nil {
		err = fmt.Errorf("unknown counterexample id %q in session %s", in.CounterexampleID, sess.ID)
		return nil, nil, err
	}
	data, err := sess.ReadFile(entry.Path)
	if err != nil {
		return nil, nil, err
	}
	var t cex.Trace
	if err = json.Unmarshal(data, &t); err != nil {
		return nil, nil, err
	}
	out := &ExplainOut{
		SessionID: sess.ID, ID: entry.ID, PropertyID: entry.Property, Role: entry.Role,
		Path: filepath.Join(sess.Dir, entry.Path), Prefix: []ExplainStep{}, Loop: []ExplainStep{},
		LoopNote:   "finite run: the loop is empty (a cycle counterexample of an ltl or progress property has a non-empty loop, the steps that repeat forever)",
		FinalState: t.Final, Summary: t.Summary, UserNames: t.UserNames(),
	}
	if t.Loop != nil {
		out.LoopNote = fmt.Sprintf("lasso: steps 1..%d are the prefix, steps %d..%d the loop, which repeats forever — after step %d the state equals the state before step %d (claim moves are steps of the claim process; a step of process \"-\" is a weak-fairness null step)",
			t.Loop.Start-1, t.Loop.Start, len(t.Steps), len(t.Steps), t.Loop.Start)
	}
	names := t.UserNames()
	for i, st := range t.Steps {
		es := ExplainStep{Index: st.Index, Process: st.Process, Command: st.Command, UserName: names[i], Location: st.Location, Changes: st.Changes, Origin: st.Origin}
		if es.Changes == nil {
			es.Changes = []cex.Change{}
		}
		if t.Loop != nil && st.Index >= t.Loop.Start {
			out.Loop = append(out.Loop, es)
		} else {
			out.Prefix = append(out.Prefix, es)
		}
	}
	if out.FinalState == nil {
		out.FinalState = []cex.Value{}
	}
	if out.UserNames == nil {
		out.UserNames = []string{}
	}
	return nil, out, nil
}

// --- mc_manifest ----------------------------------------------------------------

// ManifestIn is the input of mc_manifest.
type ManifestIn struct {
	SessionID string `json:"session_id"`
}

// ManifestOut is the answer of mc_manifest.
type ManifestOut struct {
	SessionID string   `json:"session_id"`
	Path      string   `json:"path" jsonschema:"session file manifest.json"`
	Manifest  Manifest `json:"manifest"`
}

func (s *Server) manifest(ctx context.Context, req *sdk.CallToolRequest, in ManifestIn) (*sdk.CallToolResult, *ManifestOut, error) {
	sess, err := s.session(in.SessionID, false)
	if err != nil {
		return nil, nil, err
	}
	timer := begin(sess, "mc_manifest")
	defer func() { timer.end(err, nil, nil) }()
	m, _ := sess.snapshot()
	return nil, &ManifestOut{SessionID: sess.ID, Path: filepath.Join(sess.Dir, "manifest.json"), Manifest: m}, nil
}
