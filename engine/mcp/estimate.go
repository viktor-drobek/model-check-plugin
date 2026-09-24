package mcp

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/explore"
)

// defaultEstimateMS is the time limit when the client gives none.
const defaultEstimateMS = 1000

// EstimateIn is the input of mc_estimate.
type EstimateIn struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"session holding the parsed model; omitted = a new session (then ir is required)"`
	IR        any    `json:"ir,omitempty" jsonschema:"IR JSON; omitted = the session's parsed model"`
	MS        int64  `json:"ms,omitempty" jsonschema:"time limit in milliseconds (default 1000; capped by the server's ms ceiling)"`
}

// Level is the number of states within a depth.
type Level struct {
	Depth  int `json:"depth" jsonschema:"states at distance ≤ depth from the initial state"`
	States int `json:"states"`
}

// Growth is the per-level growth observed.
type Growth struct {
	PerLevel  []Level `json:"per_level"`
	Rate      float64 `json:"rate" jsonschema:"geometric mean of the last ratios states(d+1)/states(d); 1.0 when the graph stopped growing; 0 when fewer than two levels were measured"`
	RateBasis string  `json:"rate_basis"`
}

// Projection extrapolates the growth. It is marked approximate and is not a
// verification result.
type Projection struct {
	Evidence          string `json:"evidence" jsonschema:"always approximate"`
	StatesAtNextLevel int    `json:"states_at_next_level" jsonschema:"states(last depth) × rate, rounded; 0 when no rate"`
	Note              string `json:"note"`
}

// EstimateOut is the answer of mc_estimate.
type EstimateOut struct {
	SessionID       string     `json:"session_id"`
	TimeLimitMS     int64      `json:"time_limit_ms"`
	ElapsedMS       int64      `json:"elapsed_ms"`
	StatesVisited   int        `json:"states_visited" jsonschema:"states stored by the deepest run"`
	Transitions     int        `json:"transitions"`
	DepthReached    int        `json:"depth_reached"`
	Complete        bool       `json:"complete" jsonschema:"true when a run expanded the whole reachable graph; then states_visited is exact"`
	StatesPerSecond float64    `json:"states_per_second"`
	Growth          Growth     `json:"growth"`
	Projection      Projection `json:"projection"`
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
	timer := begin(sess, "mc_estimate")
	params := &Params{TimeLimitMS: limit, Search: "bfs"}
	defer func() { timer.end(err, params, nil) }()
	m, rej, err := s.modelFor(sess, in.IR)
	if err != nil {
		return nil, nil, err
	}
	if rej != nil {
		err = fmt.Errorf("ir rejected: %s", rej.Reason)
		return nil, nil, err
	}
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	// Breadth-first runs with growing depth budgets, each within the time
	// left. A run with MaxDepth = d stores the states at distance ≤ d+1 (the
	// frontier is stored, not expanded), so level d+1 is what it counts.
	start := time.Now()
	deadline := start.Add(time.Duration(limit) * time.Millisecond)
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	out := &EstimateOut{SessionID: sess.ID, TimeLimitMS: limit, Growth: Growth{PerLevel: []Level{}},
		Note: "estimate of the state space by partial breadth-first exploration; not a verification result"}
	budget := explore.Budget{MaxStates: s.cfg.Ceiling.States, MaxMemBytes: s.cfg.Ceiling.MemoryMB << 20}
	for d := 1; ; d++ {
		if time.Now().After(deadline) {
			break
		}
		budget.MaxDepth = d
		res, e := explore.Run(runCtx, m, explore.Options{Mode: explore.BFS, Budget: budget})
		if e != nil {
			err = fmt.Errorf("ir rejected: %v", e)
			return nil, nil, err
		}
		out.StatesVisited, out.Transitions, out.DepthReached = res.States, res.Transitions, res.MaxDepth
		if runCtx.Err() != nil {
			// The time ran out inside this run: its counts are partial and
			// are not a level.
			break
		}
		out.Growth.PerLevel = append(out.Growth.PerLevel, Level{Depth: d + 1, States: res.States})
		if res.Complete {
			out.Complete = true
			break
		}
		if res.Stop != "" && res.Stop != "complete" && !isDepthStop(res.Stop) {
			// states or memory ceiling: deeper runs would only repeat it.
			break
		}
	}
	elapsed := time.Since(start)
	out.ElapsedMS = elapsed.Milliseconds()
	if elapsed > 0 {
		out.StatesPerSecond = math.Round(float64(out.StatesVisited)/elapsed.Seconds()*10) / 10
	}
	out.Growth.Rate, out.Growth.RateBasis = growthRate(out.Growth.PerLevel)
	out.Projection = Projection{Evidence: string(explore.Approximate)}
	switch {
	case out.Complete:
		out.Projection.Note = fmt.Sprintf("the reachable graph was expanded completely: %d states is exact, no projection needed", out.StatesVisited)
		out.Projection.StatesAtNextLevel = out.StatesVisited
	case out.Growth.Rate > 0:
		last := out.Growth.PerLevel[len(out.Growth.PerLevel)-1]
		out.Projection.StatesAtNextLevel = int(math.Round(float64(last.States) * out.Growth.Rate))
		out.Projection.Note = fmt.Sprintf("extrapolation of the last levels' ratio; states within depth %d ≈ %d if the growth continues", last.Depth+1, out.Projection.StatesAtNextLevel)
	default:
		out.Projection.Note = "fewer than two levels were measured within the time limit; no growth rate can be given"
	}
	return nil, out, nil
}

func isDepthStop(stop string) bool {
	return len(stop) >= 5 && stop[:5] == "depth"
}

// growthRate is the geometric mean of the last (up to three) level ratios.
func growthRate(levels []Level) (float64, string) {
	if len(levels) < 2 {
		return 0, "fewer than two levels measured"
	}
	from := len(levels) - 4
	if from < 0 {
		from = 0
	}
	prod, n := 1.0, 0
	for i := from + 1; i < len(levels); i++ {
		if levels[i-1].States > 0 {
			prod *= float64(levels[i].States) / float64(levels[i-1].States)
			n++
		}
	}
	if n == 0 {
		return 0, "no usable ratio"
	}
	rate := math.Pow(prod, 1/float64(n))
	return math.Round(rate*1000) / 1000, fmt.Sprintf("geometric mean of %d ratio(s) over levels %d..%d", n, levels[from].Depth, levels[len(levels)-1].Depth)
}
