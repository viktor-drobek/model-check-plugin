package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/cex"
	"modelcheck/explore"
)

// inlineStepLimit bounds the trace embedded in the answer; longer traces are
// only in the session file.
const inlineStepLimit = 50

// defaultSimSteps is the step limit when the client gives none.
const defaultSimSteps = 100

// SimulateIn is the input of mc_simulate.
type SimulateIn struct {
	SessionID string   `json:"session_id,omitempty" jsonschema:"session holding the parsed model; omitted = a new session (then ir is required)"`
	IR        any      `json:"ir,omitempty" jsonschema:"IR JSON; omitted = the session's parsed model"`
	Seed      int64    `json:"seed,omitempty" jsonschema:"random mode: seed of the generator; the same seed gives the same run"`
	Steps     int      `json:"steps,omitempty" jsonschema:"maximum number of steps (default 100; capped by the server's depth ceiling)"`
	Mode      string   `json:"mode" jsonschema:"random | guided"`
	Edges     []string `json:"edges,omitempty" jsonschema:"guided mode: edge ids in order; an id is process/index (e.g. init/3), the edge text, or the user's name recorded in its origin; a rendezvous step is named by its sending edge"`
}

// SimulateOut is the answer of mc_simulate.
type SimulateOut struct {
	SessionID     string     `json:"session_id"`
	Mode          string     `json:"mode"`
	Seed          int64      `json:"seed"`
	StepsTaken    int        `json:"steps_taken"`
	Stopped       string     `json:"stopped" jsonschema:"steps | deadlock | terminated | edge not enabled | edges exhausted | assert failed | invalid-model — exactly one"`
	StopReason    string     `json:"stop_reason"`
	Summary       string     `json:"summary" jsonschema:"commands taken, comma-separated"`
	TracePath     string     `json:"trace_path" jsonschema:"session file with the full trace"`
	Trace         *cex.Trace `json:"trace,omitempty" jsonschema:"embedded when the run has at most 50 steps"`
	EnabledAtStop []string   `json:"enabled_at_stop" jsonschema:"edge ids enabled in the final state"`
}

func edgeID(st *explore.Stepper, ref explore.EdgeRef) string {
	return st.Layout().Model.Processes[ref.Proc].Name + "/" + strconv.Itoa(ref.Edge)
}

// nameMatches reports whether the client's name denotes ref. Texts and
// origin names need not be unique across the model (two processes may
// both have an edge "x++"); when several enabled moves answer to a name,
// guided mode takes the first in the explorer's order. process/index is
// always unique.
func nameMatches(st *explore.Stepper, name string, ref explore.EdgeRef) bool {
	e := st.Edge(ref)
	if edgeID(st, ref) == name || (e.Text != "" && e.Text == name) {
		return true
	}
	return e.Origin != nil && e.Origin.Name != "" && e.Origin.Name == name
}

// knownEdge reports whether any edge of the model answers to name.
func knownEdge(st *explore.Stepper, name string) bool {
	m := st.Layout().Model
	for p := range m.Processes {
		for i := range m.Processes[p].Edges {
			if nameMatches(st, name, explore.EdgeRef{Proc: p, Edge: i}) {
				return true
			}
		}
	}
	return false
}

func (s *Server) simulate(ctx context.Context, req *sdk.CallToolRequest, in SimulateIn) (*sdk.CallToolResult, *SimulateOut, error) {
	if in.SessionID == "" && in.IR == nil {
		return nil, nil, errors.New("no model: pass `ir` inline or a session_id whose model was parsed with mc_parse")
	}
	switch in.Mode {
	case "random", "guided":
	default:
		return nil, nil, fmt.Errorf("mode must be random or guided, got %q", in.Mode)
	}
	if in.Mode == "guided" && len(in.Edges) == 0 {
		return nil, nil, errors.New("guided mode requires a non-empty edges list")
	}
	steps := in.Steps
	if steps <= 0 {
		steps = defaultSimSteps
	}
	if s.cfg.Ceiling.Depth > 0 && steps > s.cfg.Ceiling.Depth {
		steps = s.cfg.Ceiling.Depth
	}
	sess, err := s.session(in.SessionID, true)
	if err != nil {
		return nil, nil, err
	}
	timer := s.begin(ctx, sess, "mc_simulate")
	seed := in.Seed
	params := &Params{Mode: in.Mode, Steps: steps}
	if in.Mode == "random" {
		params.Seed = &seed
	}
	var artifacts []string
	defer func() { timer.end(err, params, artifacts) }()

	m, rej, err := s.modelFor(sess, in.IR)
	if err != nil {
		return nil, nil, err
	}
	if rej != nil {
		err = rejectedInput(rej)
		return nil, nil, err
	}
	st, err := explore.NewStepper(m)
	if err != nil {
		err = rejectedInput(&Rejection{Kind: "ir", Construct: "model", Reason: err.Error()})
		return nil, nil, err
	}
	// Unknown guided ids are a client mistake, found before running.
	if in.Mode == "guided" {
		for i, name := range in.Edges {
			if !knownEdge(st, name) {
				err = fmt.Errorf("edges[%d]: no edge named %q (use process/index, the edge text, or the origin name)", i, name)
				return nil, nil, err
			}
		}
	}

	rng := rand.New(rand.NewPCG(uint64(seed), 0x6d63645f73696d)) // "mcd_sim"
	states := [][]byte{st.Initial()}
	var refs []cex.Ref
	out := &SimulateOut{SessionID: sess.ID, Mode: in.Mode, Seed: seed, EnabledAtStop: []string{}}
	cur := states[0]
	for {
		enabled, e := st.Enabled(cur)
		if e != nil {
			out.Stopped, out.StopReason = "invalid-model", e.Error()
			break
		}
		out.EnabledAtStop = out.EnabledAtStop[:0]
		for _, mv := range enabled {
			out.EnabledAtStop = append(out.EnabledAtStop, edgeID(st, mv.Edge))
		}
		// Exactly one stop condition applies per iteration, tested in this
		// order: no move (deadlock/terminated), step limit, guidance.
		if len(enabled) == 0 {
			if st.Terminated(cur) {
				out.Stopped, out.StopReason = "terminated", "every process is at an end location or has no outgoing edges"
			} else {
				out.Stopped, out.StopReason = "deadlock", "no move is enabled and not every process is terminated"
			}
			break
		}
		if len(refs) >= steps {
			out.Stopped, out.StopReason = "steps", fmt.Sprintf("the step limit %d was reached", steps)
			break
		}
		var next explore.Move
		if in.Mode == "random" {
			next = enabled[rng.IntN(len(enabled))]
		} else {
			if len(refs) >= len(in.Edges) {
				out.Stopped, out.StopReason = "edges exhausted", "every edge of the list was taken"
				break
			}
			name := in.Edges[len(refs)]
			found := false
			for _, mv := range enabled {
				if nameMatches(st, name, mv.Edge) {
					next, found = mv, true
					break
				}
			}
			if !found {
				out.Stopped, out.StopReason = "edge not enabled", fmt.Sprintf("edge %q is not enabled after step %d", name, len(refs))
				break
			}
		}
		succ, failed, e := st.Apply(cur, next)
		if e != nil {
			out.Stopped, out.StopReason = "invalid-model", e.Error()
			break
		}
		refs = append(refs, next.Ref())
		states = append(states, succ)
		cur = succ
		if failed != nil {
			out.Stopped, out.StopReason = "assert failed", "assert failed in step "+cex.CommandText(failed)
			out.EnabledAtStop = out.EnabledAtStop[:0]
			break
		}
	}
	trace := cex.Build(st.Layout(), states, refs)
	out.StepsTaken = len(trace.Steps)
	out.Summary = trace.Summary
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	sess.mu.Lock()
	rel := sess.next("sim", ".json")
	sess.mu.Unlock()
	if out.TracePath, err = sess.WriteFile(rel, append(data, '\n')); err != nil {
		return nil, nil, err
	}
	artifacts = []string{rel}
	if len(trace.Steps) <= inlineStepLimit {
		out.Trace = trace
	}
	if out.Stopped == "" {
		err = errors.New("internal: simulation ended without a stop reason")
		return nil, nil, err
	}
	return nil, out, nil
}
