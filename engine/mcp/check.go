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
	"modelcheck/explore"
	"modelcheck/ir"
	"modelcheck/report"
)

// Property kinds the tool interface accepts (plan 14 §6). `assert` is the
// IR's own implicit kind and is added by the engine, not requested.
var checkKinds = map[string]bool{"invariant": true, "deadlock": true, "reach": true, "ltl": true, "ctl": true, "progress": true}

// notExecutedReason names, per kind, the missing capability and the step
// that brings it. It is the only text the server writes for these kinds.
var notExecutedReason = map[string]string{
	"ltl":      "not implemented until G4: LTL model checking (LTL → Büchi translation, product with the model, nested DFS for acceptance cycles) is not part of this engine version",
	"progress": "not implemented until G4: non-progress cycle detection (progress labels, nested DFS) is not part of this engine version",
	"ctl":      "not implemented until G5: CTL model checking (graph labelling for EX/EU/EG) is not part of this engine version",
}

// PropertyIn is one property to check.
type PropertyIn struct {
	ID   string `json:"id"`
	Kind string `json:"kind" jsonschema:"invariant | deadlock | reach | ltl | ctl | progress"`
	Expr any    `json:"expr,omitempty" jsonschema:"boolean state expression: IR expression JSON ({op, args, var, value}) or a bare variable name; required for invariant, reach, ltl, ctl"`
	Text string `json:"text,omitempty" jsonschema:"the user's statement of the property"`
}

// CheckIn is the input of mc_check.
type CheckIn struct {
	SessionID  string       `json:"session_id,omitempty" jsonschema:"session holding the parsed model; omitted = a new session (then ir is required)"`
	IR         any          `json:"ir,omitempty" jsonschema:"IR JSON to check; omitted = the session's parsed model"`
	Properties []PropertyIn `json:"properties,omitempty" jsonschema:"properties to check; they replace the model's own properties when given, omitted = the model's own"`
	Fairness   string       `json:"fairness,omitempty" jsonschema:"none | weak (default none); weak fairness applies to ltl and progress only and therefore has no effect in G2"`
	Budget     *Budget      `json:"budget,omitempty" jsonschema:"limits; fields at 0 take the server default; fields above the server ceiling are clamped"`
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
	UserNames []string `json:"user_names" jsonschema:"per step, the user's name for the command when the frontend recorded one"`
}

// PropertyOut is the result for one property.
type PropertyOut struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Text           string          `json:"text,omitempty"`
	Status         string          `json:"status" jsonschema:"verified | violated | inconclusive | unknown | not-executed | invalid-model"`
	Evidence       string          `json:"evidence" jsonschema:"exhaustive | bounded | approximate | unknown"`
	Complete       bool            `json:"complete"`
	Reason         string          `json:"reason,omitempty" jsonschema:"inconclusive: the exhausted resource; not-executed: the missing capability; invalid-model: the offending step"`
	Counters       report.Counters `json:"counters"`
	Counterexample *TraceRef       `json:"counterexample,omitempty"`
	Witness        *TraceRef       `json:"witness,omitempty"`
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
	case "", "none", "weak":
	default:
		return nil, nil, fmt.Errorf("fairness must be none or weak, got %q", in.Fairness)
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
			if e == nil && (p.Kind == "invariant" || p.Kind == "reach" || p.Kind == "ltl" || p.Kind == "ctl") {
				return nil, nil, fmt.Errorf("properties[%d] (%s): kind %s requires expr", i, p.ID, p.Kind)
			}
			props = append(props, ir.Property{ID: p.ID, Kind: p.Kind, Expr: e, Text: p.Text})
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
	if in.Fairness == "weak" {
		hasTemporal := false
		for _, p := range m.Properties {
			if p.Kind == "ltl" || p.Kind == "progress" {
				hasTemporal = true
			}
		}
		if !hasTemporal {
			out.Warnings = append(out.Warnings, "fairness weak has no effect: it applies to ltl and progress properties only, and none was given")
		} else {
			out.Warnings = append(out.Warnings, "fairness weak is recorded but not applied: ltl and progress are not executed until G4")
		}
	}

	var requested Budget
	if in.Budget != nil {
		requested = *in.Budget
	}
	applied, notes := Clamp(requested, s.cfg.Default, s.cfg.Ceiling)
	fairness := in.Fairness
	if fairness == "" {
		fairness = "none"
	}
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
	}})
	if runErr != nil {
		// Validated IR that does not compile (e.g. an undeclared variable in
		// a property expression): the input is refused, nothing is claimed.
		out.Outcome = "rejected"
		out.Rejection = &Rejection{Kind: "ir", Construct: "expression", Reason: runErr.Error()}
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
	out.Search = &SearchOut{Mode: string(mode), BudgetRequested: requested, BudgetApplied: applied, BudgetNotes: notes, Stop: res.Stop, Complete: res.Complete}
	for _, p := range rep.Properties {
		po := PropertyOut{ID: p.ID, Kind: p.Kind, Text: p.Text, Status: p.Status, Evidence: p.Evidence, Complete: p.Complete, Reason: p.Reason, Counters: p.Counters}
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
	ref := &TraceRef{ID: id, Path: p, Summary: t.Summary, Steps: len(t.Steps), UserNames: t.UserNames()}
	if ref.UserNames == nil {
		ref.UserNames = []string{}
	}
	return ref, rel, nil
}

// aggregate applies the fixed priority of plan 14 §6.
func aggregate(props []PropertyOut) *Aggregate {
	order := []string{"invalid-model", "not-executed", "violated", "inconclusive", "unknown", "verified"}
	for _, st := range order {
		for _, p := range props {
			if p.Status == st {
				return &Aggregate{Status: st, Basis: "priority invalid-model > not-executed > violated > inconclusive > unknown > verified (plan 14 §6); property " + p.ID}
			}
		}
	}
	return &Aggregate{Status: "unknown", Basis: "no properties"}
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
	Prefix     []ExplainStep `json:"prefix"`
	Loop       []ExplainStep `json:"loop" jsonschema:"the repeated part of a lasso; empty in G2"`
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
		LoopNote:   "loop counterexamples (prefix + cycle, for ltl and progress) arrive with G4; in G2 every run is finite and the loop is empty",
		FinalState: t.Final, Summary: t.Summary, UserNames: t.UserNames(),
	}
	names := t.UserNames()
	for i, st := range t.Steps {
		es := ExplainStep{Index: st.Index, Process: st.Process, Command: st.Command, UserName: names[i], Location: st.Location, Changes: st.Changes, Origin: st.Origin}
		if es.Changes == nil {
			es.Changes = []cex.Change{}
		}
		out.Prefix = append(out.Prefix, es)
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
