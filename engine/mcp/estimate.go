package mcp

import (
	"context"
	"errors"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/estimate"
	"modelcheck/explore"
)

// defaultEstimateMS is the time limit when the client gives none.
const defaultEstimateMS = 1000

// EstimateIn is the input of mc_estimate.
type EstimateIn struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"session holding the parsed model; omitted = a new session (then ir is required)"`
	IR        any    `json:"ir,omitempty" jsonschema:"IR JSON; omitted = the session's parsed model"`
	MS        int64  `json:"ms,omitempty" jsonschema:"time limit in milliseconds (default 1000; capped by the server's ms ceiling)"`
	// TargetDepth asks the growth model for one more number.
	TargetDepth int `json:"target_depth,omitempty" jsonschema:"depth to project the growth to; omitted = only the next level"`
}

// Level, Growth, Projection and Size are the estimate's own shapes; they
// come from package estimate so that mcd check --estimate and mc_estimate
// answer with the same numbers and the same words.
type Level = estimate.Level

// Growth is the per-level measurement and the fitted factor.
type Growth = estimate.Growth

// Projection is the extrapolation; `approximate` while it extrapolates,
// `exhaustive` only when the whole graph was expanded.
type Projection = estimate.Projection

// Size is the A4 size class of plan 14 §12 and the recommendation.
type Size = estimate.Size

// EstimateOut is the answer of mc_estimate.
type EstimateOut struct {
	SessionID       string     `json:"session_id"`
	TimeLimitMS     int64      `json:"time_limit_ms"`
	ElapsedMS       int64      `json:"elapsed_ms"`
	StatesVisited   int        `json:"states_visited" jsonschema:"states stored by the deepest run"`
	Transitions     int        `json:"transitions"`
	DepthReached    int        `json:"depth_reached"`
	StateBytes      int        `json:"state_bytes"`
	Complete        bool       `json:"complete" jsonschema:"true when a run expanded the whole reachable graph; then states_visited is exact"`
	StatesPerSecond float64    `json:"states_per_second"`
	Growth          Growth     `json:"growth" jsonschema:"states within each measured depth, and the growth factor fitted to the last of them"`
	Projection      Projection `json:"projection" jsonschema:"extrapolation of the factor to the next level and to target_depth"`
	Size            Size       `json:"size" jsonschema:"the A4 size class of plan 14 §12 (small | medium | large) and what to do about it"`
	Note            string     `json:"note" jsonschema:"this is an estimate, not a verification result: no property gets a status here"`
}

func (s *Server) estimate(ctx context.Context, req *sdk.CallToolRequest, in EstimateIn) (*sdk.CallToolResult, *EstimateOut, error) {
	if in.SessionID == "" && in.IR == nil {
		return nil, nil, errors.New("no model: pass `ir` inline or a session_id whose model was parsed with mc_parse")
	}
	limit := in.MS
	if limit <= 0 {
		limit = defaultEstimateMS
	}
	if s.cfg.Ceiling.MS > 0 && limit > s.cfg.Ceiling.MS {
		limit = s.cfg.Ceiling.MS
	}
	sess, err := s.session(in.SessionID, true)
	if err != nil {
		return nil, nil, err
	}
	timer := s.begin(ctx, sess, "mc_estimate")
	params := &Params{TimeLimitMS: limit, Search: "bfs"}
	defer func() { timer.end(err, params, nil) }()
	m, rej, err := s.modelFor(sess, in.IR)
	if err != nil {
		return nil, nil, err
	}
	if rej != nil {
		err = rejectedInput(rej)
		return nil, nil, err
	}
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	// States and memory are bounded by the server defaults (which lie within
	// the ceilings), as for a check whose client left those fields at 0.
	res, err := estimate.Run(ctx, m, estimate.Options{
		TimeLimitMS: limit,
		TargetDepth: in.TargetDepth,
		Budget:      explore.Budget{MaxStates: s.cfg.Default.States, MaxMemBytes: s.cfg.Default.MemoryMB << 20},
	})
	if err != nil {
		err = rejectedInput(&Rejection{Kind: "ir", Construct: "model", Reason: err.Error()})
		return nil, nil, err
	}
	return nil, &EstimateOut{
		SessionID: sess.ID, TimeLimitMS: res.TimeLimitMS, ElapsedMS: res.ElapsedMS,
		StatesVisited: res.StatesVisited, Transitions: res.Transitions, DepthReached: res.DepthReached,
		StateBytes: res.StateBytes, Complete: res.Complete, StatesPerSecond: res.StatesPerSecond,
		Growth: res.Growth, Projection: res.Projection, Size: res.Size, Note: res.Note,
	}, nil
}
