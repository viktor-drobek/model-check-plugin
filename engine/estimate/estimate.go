// Package estimate is the growth model behind `mc_estimate` and
// `mcd check --estimate` (plan 14 §6, FR-022): how big is this state space,
// and is it a size this engine can finish?
//
// The answer is measured, then extrapolated, and the two are kept apart.
// Measured: one breadth-first search under the time budget, reporting the
// number of states within each depth it finished expanding.
// Extrapolated: a growth factor fitted to the last levels (the geometric
// mean of their ratios) and, from it, a projection to a target depth. The
// projection carries evidence `approximate`; only a run that expanded the
// whole reachable graph carries `exhaustive`, and then nothing is
// extrapolated at all.
//
// Size classes are the A4 bounds of plan 14 §12, fixed at checkpoint K1 in
// steps/spike-confirmation.md. Two of those bounds decide the class here —
// the number of states and the width of the state vector:
//
//	small    up to 1e5 states, state vector up to 128 bytes
//	medium   up to 1e6 states, state vector up to 128 bytes
//	large    beyond them — the plan's instruction for such a model is to say
//	         so in advance instead of waiting silently, so the recommendation
//	         names what to do (bound the model, or accept `inconclusive`).
//
// The other A4 bounds — depth 1e6, 60 s, 1 GiB — are budgets of a run, not
// properties of the model, so they are not part of the classification; the
// depth actually reached is reported separately as depth_reached, and the
// recommendation of each class names the budgets it expects to fit.
//
// An estimate is never a verification result: no property gets a status
// here, and the caller is told so in Result.Note.
package estimate

import (
	"context"
	"fmt"
	"math"
	"time"

	"modelcheck/explore"
	"modelcheck/ir"
)

// A4 bounds of plan 14 §12.
const (
	SmallStates   = 100_000
	MediumStates  = 1_000_000
	MediumDepth   = 1_000_000
	MediumVector  = 128
	MediumSeconds = 60
	MediumMiB     = 1024
)

// Level is the number of states within a depth.
type Level = explore.Level

// Growth is the per-level measurement and the factor fitted to it.
type Growth struct {
	PerLevel  []Level `json:"per_level"`
	Rate      float64 `json:"rate"`
	RateBasis string  `json:"rate_basis"`
}

// Projection extrapolates the growth to a target depth.
type Projection struct {
	Evidence          string `json:"evidence"`
	TargetDepth       int    `json:"target_depth,omitempty"`
	StatesAtTarget    int    `json:"states_at_target,omitempty"`
	StatesAtNextLevel int    `json:"states_at_next_level"`
	Note              string `json:"note"`
}

// Size is the A4 size class and what to do about it.
type Size struct {
	Class          string `json:"class"`
	Basis          string `json:"basis"`
	Recommendation string `json:"recommendation"`
}

// Result is the whole estimate.
type Result struct {
	TimeLimitMS     int64      `json:"time_limit_ms"`
	ElapsedMS       int64      `json:"elapsed_ms"`
	StatesVisited   int        `json:"states_visited"`
	Transitions     int        `json:"transitions"`
	DepthReached    int        `json:"depth_reached"`
	StateBytes      int        `json:"state_bytes"`
	Complete        bool       `json:"complete"`
	StatesPerSecond float64    `json:"states_per_second"`
	Growth          Growth     `json:"growth"`
	Projection      Projection `json:"projection"`
	Size            Size       `json:"size"`
	Note            string     `json:"note"`
}

// Options configures Run.
type Options struct {
	// TimeLimitMS bounds the whole measurement.
	TimeLimitMS int64
	// TargetDepth, when > 0, is the depth the projection is asked about.
	TargetDepth int
	// Budget bounds each partial run (states and memory); its MaxDepth is
	// set per level and is ignored here.
	Budget explore.Budget
}

// Note is the sentence every estimate carries.
const Note = "estimate of the state space by partial breadth-first exploration; not a verification result — no property gets a status from it"

// Run measures m under the options and returns the estimate.
//
// One breadth-first search does the measuring: it reports the number of
// states within each depth it finished expanding, so the levels are exact
// and cost one pass rather than one pass per level. Whatever the time
// budget stops is simply the last depth reported.
func Run(ctx context.Context, m *ir.Model, opt Options) (*Result, error) {
	limit := opt.TimeLimitMS
	if limit <= 0 {
		limit = 1000
	}
	start := time.Now()
	runCtx, cancel := context.WithDeadline(ctx, start.Add(time.Duration(limit)*time.Millisecond))
	defer cancel()
	out := &Result{TimeLimitMS: limit, Growth: Growth{PerLevel: []Level{}}, Note: Note}
	// The estimate measures the state space, not the properties: a copy
	// without them keeps a temporal property from starting a product search
	// that has nothing to do with the question asked here.
	mm := *m
	mm.Properties = nil
	res, err := explore.Run(runCtx, &mm, explore.Options{Mode: explore.BFS, Budget: opt.Budget, Sweep: true, Watch: []*ir.Expr{}})
	if err != nil {
		return nil, err
	}
	out.StatesVisited, out.Transitions, out.DepthReached = res.States, res.Transitions, res.MaxDepth
	out.StateBytes, out.Complete = res.StateBytes, res.Complete
	if len(res.Levels) > 0 {
		out.Growth.PerLevel = res.Levels
	}
	elapsed := time.Since(start)
	out.ElapsedMS = elapsed.Milliseconds()
	if elapsed > 0 {
		out.StatesPerSecond = math.Round(float64(out.StatesVisited)/elapsed.Seconds()*10) / 10
	}
	out.Growth.Rate, out.Growth.RateBasis = growthRate(out.Growth.PerLevel)
	out.Projection = project(out, opt.TargetDepth)
	out.Size = classify(out)
	return out, nil
}

// growthRate is the geometric mean of the last (up to three) level ratios.
// It is a fit, not a law: the note says over which levels it was taken, so
// that a reader can judge it.
func growthRate(levels []Level) (float64, string) {
	if len(levels) < 2 {
		return 0, "fewer than two levels were measured: no ratio can be formed"
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
		return 0, "no usable ratio (a measured level had no states)"
	}
	rate := math.Pow(prod, 1/float64(n))
	return math.Round(rate*1000) / 1000, fmt.Sprintf("geometric mean of %d ratio(s) states(d+1)/states(d) over levels %d..%d", n, levels[from].Depth, levels[len(levels)-1].Depth)
}

// projCap bounds an extrapolation so that a runaway factor cannot overflow
// the answer; a projection at the cap is reported as "at least".
const projCap = 1 << 40

func project(r *Result, target int) Projection {
	p := Projection{Evidence: string(explore.Approximate)}
	switch {
	case r.Complete:
		p.Evidence = string(explore.Exhaustive)
		p.StatesAtNextLevel = r.StatesVisited
		p.StatesAtTarget = r.StatesVisited
		p.TargetDepth = target
		p.Note = fmt.Sprintf("the reachable graph was expanded completely: %d states is exact, there is nothing to extrapolate", r.StatesVisited)
		if target > 0 && len(r.Growth.PerLevel) > 0 && target < r.Growth.PerLevel[len(r.Growth.PerLevel)-1].Depth {
			p.StatesAtTarget = statesWithin(r.Growth.PerLevel, target)
			p.Note += fmt.Sprintf("; within depth %d there are %d of them, measured, not projected", target, p.StatesAtTarget)
		}
		return p
	case r.Growth.Rate <= 0:
		p.Note = "fewer than two levels were measured within the time limit: no growth factor, and therefore no projection"
		return p
	}
	last := r.Growth.PerLevel[len(r.Growth.PerLevel)-1]
	p.StatesAtNextLevel = extrapolate(last.States, r.Growth.Rate, 1)
	p.Note = fmt.Sprintf("extrapolation of the fitted factor %.3f: within depth %d about %d states", r.Growth.Rate, last.Depth+1, p.StatesAtNextLevel)
	if target > last.Depth {
		p.TargetDepth = target
		p.StatesAtTarget = extrapolate(last.States, r.Growth.Rate, target-last.Depth)
		p.Note += fmt.Sprintf("; within the requested depth %d about %d states, if the factor holds — it is a fit over the measured levels, not a law of the model", target, p.StatesAtTarget)
	} else if target > 0 {
		p.TargetDepth = target
		p.StatesAtTarget = statesWithin(r.Growth.PerLevel, target)
		p.Note += fmt.Sprintf("; depth %d was measured, not projected: %d states", target, p.StatesAtTarget)
	}
	return p
}

func statesWithin(levels []Level, depth int) int {
	best := 0
	for _, l := range levels {
		if l.Depth <= depth {
			best = l.States
		}
	}
	return best
}

func extrapolate(from int, rate float64, steps int) int {
	v := float64(from) * math.Pow(rate, float64(steps))
	if math.IsInf(v, 1) || v > projCap {
		return projCap
	}
	return int(math.Round(v))
}

// classify places the estimate in the A4 size classes. The number it
// classifies is the exact count when the graph was expanded completely and
// the projection otherwise, and the basis says which of the two it was.
func classify(r *Result) Size {
	states, how := r.StatesVisited, "the exact number of reachable states"
	if !r.Complete {
		if n := r.Projection.StatesAtTarget; n > states {
			states, how = n, fmt.Sprintf("the projection to depth %d", r.Projection.TargetDepth)
		} else if n := r.Projection.StatesAtNextLevel; n > states {
			states, how = n, fmt.Sprintf("the projection to depth %d", r.Growth.PerLevel[len(r.Growth.PerLevel)-1].Depth+1)
		} else {
			how = "the states seen so far (no larger projection)"
		}
	}
	s := Size{Basis: fmt.Sprintf("%s (%d) against the A4 bounds of plan 14 §12, which the class reads off two numbers: small up to %d states, medium up to %d states, both with a state vector up to %d bytes; the state vector here is %d bytes, and the depth reached (%d) is reported separately because it is a bound of the run, not of the model",
		how, states, SmallStates, MediumStates, MediumVector, r.StateBytes, r.DepthReached)}
	switch {
	case states <= SmallStates && r.StateBytes <= MediumVector:
		s.Class = "small"
		s.Recommendation = fmt.Sprintf("small (plan 14 §12, A4): an exhaustive check fits well inside the default budgets (%d states, %d ms, %d MiB); run mcd check or mc_check as it is", MediumStates, MediumSeconds*1000, MediumMiB)
	case states <= MediumStates && r.StateBytes <= MediumVector:
		s.Class = "medium"
		s.Recommendation = fmt.Sprintf("medium (plan 14 §12, A4): an exhaustive check is expected to fit the default budgets (%d states, depth %d, %d ms, %d MiB), but with little room; raise the time budget before a full run and keep the state vector below %d bytes",
			MediumStates, MediumDepth, MediumSeconds*1000, MediumMiB, MediumVector)
	default:
		s.Class = "large"
		s.Recommendation = fmt.Sprintf("large (plan 14 §12, A4: beyond %d states or a state vector above %d bytes): this engine version has no state compression or partial-order reduction, so an exhaustive check is likely to end as inconclusive on a budget rather than as a verdict; bound the model (fewer processes, smaller channel capacities, smaller constants) or accept an inconclusive result on a declared bound",
			MediumStates, MediumVector)
	}
	return s
}
