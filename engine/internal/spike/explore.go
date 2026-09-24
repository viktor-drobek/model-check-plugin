package spike

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Options bounds a run. Zero values mean "no limit" except VisitedSet.
type Options struct {
	MaxStates int           // state budget; exceeding it → inconclusive
	Timeout   time.Duration // wall-clock budget; exceeding it → inconclusive
	// ContinueAfterViolation keeps exploring after the first violation so that
	// the stored-state count covers the whole reachable graph (pan -c0). The
	// first violation found is still the one reported.
	ContinueAfterViolation bool
	// CompactSet selects the arena hash table instead of map[string]struct{}.
	CompactSet bool
}

// Status values are the subset of the 11 §14 vocabulary a bounded explicit
// search can produce on its own: verified (exhaustive, no violation),
// violated (a violation was reached), inconclusive (a budget ran out first).
const (
	StatusVerified     = "verified"
	StatusViolated     = "violated"
	StatusInconclusive = "inconclusive"
)

// Evidence values (11 §14). Exhaustive: the result rests on an exact search —
// for verified, the whole reachable graph was visited; for violated, the
// witness is an exact run of the model, so stopping at the first violation
// loses nothing about the verdict. Bounded: a budget stopped the search
// before either held, so the verdict is inconclusive. Result.Complete says
// separately whether the sweep covered the whole graph, because a violated
// run may also have been cut short (ContinueAfterViolation plus a budget).
const (
	EvidenceExhaustive = "exhaustive"
	EvidenceBounded    = "bounded"
)

// Violation kinds.
const (
	KindDeadlock  = "deadlock"
	KindAssertion = "assertion"
)

// Step is one transition of a witness.
type Step struct {
	Label string `json:"label"`
	State string `json:"state"`
}

// Result is the deterministic part of a run: no timings, no memory figures.
type Result struct {
	Model      string `json:"model"`
	Status     string `json:"status"`
	Evidence   string `json:"evidence"`
	Reason     string `json:"reason,omitempty"`    // for inconclusive
	Violation  string `json:"violation,omitempty"` // deadlock | assertion
	Statement  string `json:"statement,omitempty"` // violated assertion text
	Witness    []Step `json:"witness,omitempty"`
	FinalState string `json:"final_state,omitempty"`
	FinalVars  []Var  `json:"final_vars,omitempty"`
	// Complete is true when the search visited the whole reachable graph, so
	// that States is the size of the graph. False when a budget stopped the
	// search or when the run stopped at the first violation.
	Complete    bool `json:"complete"`
	States      int  `json:"states_stored"`
	Transitions int  `json:"transitions"`
	MaxDepth    int  `json:"max_depth"`
	Violations  int  `json:"violations_seen"`
}

// Stats is the non-deterministic part: timing and memory.
type Stats struct {
	Elapsed      time.Duration
	StatesPerSec float64
	StateLen     int
	BudgetHit    bool
	// Visited is the set of stored states, returned so that a measurement
	// harness can hold it alive across a GC and read the retained heap.
	Visited Visited
	// PeakStack is the largest number of DFS frames held at once.
	PeakStack int
}

// JSON renders the result deterministically (fixed field order, no maps).
func (r *Result) JSON() []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

type frame struct {
	state []byte
	// succ is the list of successors of state, materialised on push so that
	// the model's reusable next-buffer is never held across calls.
	succ []edge
	next int    // index of the next successor to try
	via  string // label of the transition that led here (empty for initial)
}

type edge struct {
	label    string
	state    []byte
	violated string
}

// Explore runs a deterministic DFS from the model's initial state.
//
// DFS with an explicit stack: the witness is the stack contents at the moment
// a violation is found, so it is a valid run but not necessarily the shortest
// (plan 14 §4.1). Deadlock is "no enabled transition" (see feature header for
// why that equals SPIN's invalid end state on these models).
func Explore(m Model, opt Options) (*Result, *Stats) {
	start := time.Now()
	var deadline time.Time
	if opt.Timeout > 0 {
		deadline = start.Add(opt.Timeout)
	}
	init := append([]byte(nil), m.Initial()...)
	var visited Visited
	if opt.CompactSet {
		visited = newCompactVisited(len(init), 1024)
	} else {
		visited = newMapVisited(1024)
	}
	res := &Result{Model: m.Name(), Status: StatusVerified, Evidence: EvidenceExhaustive}
	stats := &Stats{StateLen: len(init), Visited: visited}

	successors := func(s []byte) []edge {
		var out []edge
		m.Successors(s, func(label string, next []byte, violated string) {
			out = append(out, edge{label, append([]byte(nil), next...), violated})
		})
		return out
	}

	visited.Add(init)
	res.States = 1
	stack := []frame{{state: init, succ: successors(init)}}

	record := func(kind, statement string, s []byte) {
		res.Violations++
		if res.Status == StatusViolated {
			return // keep the first violation
		}
		res.Status = StatusViolated
		res.Violation = kind
		res.Statement = statement
		res.FinalState = m.Describe(s)
		res.FinalVars = m.Vars(s)
		for _, f := range stack[1:] {
			res.Witness = append(res.Witness, Step{Label: f.via, State: m.Describe(f.state)})
		}
	}

	checked := 0
	for len(stack) > 0 {
		if len(stack) > res.MaxDepth {
			res.MaxDepth = len(stack)
		}
		top := &stack[len(stack)-1]
		if top.next == 0 && len(top.succ) == 0 {
			// Only just arrived and nothing is enabled: deadlock.
			record(KindDeadlock, "", top.state)
			if !opt.ContinueAfterViolation {
				break
			}
		}
		if top.next >= len(top.succ) {
			stack = stack[:len(stack)-1]
			continue
		}
		e := top.succ[top.next]
		top.next++
		res.Transitions++
		if e.violated != "" {
			// The violating step is part of the witness; push it so record
			// sees it on the stack, then decide whether to go on.
			stack = append(stack, frame{state: e.state, via: e.label})
			record(KindAssertion, e.violated, e.state)
			if !opt.ContinueAfterViolation {
				break
			}
			stack = stack[:len(stack)-1]
			// pan -c0 semantics: the assertion is treated as passed and the
			// target state is explored like any other.
		}
		if visited.Has(e.state) {
			continue
		}
		// A genuinely new state: it consumes budget, so the budget is checked
		// before storing it. Stored states are therefore never more than
		// MaxStates, and a violation in the last state within budget is still
		// examined (the check above runs on the next iteration).
		if opt.MaxStates > 0 && res.States >= opt.MaxStates {
			budget(res, stats, "state budget exhausted")
			break
		}
		visited.Add(e.state)
		res.States++
		stack = append(stack, frame{state: e.state, via: e.label, succ: successors(e.state)})

		checked++
		if checked&1023 == 0 && !deadline.IsZero() && time.Now().After(deadline) {
			budget(res, stats, "time budget exhausted")
			break
		}
	}
	res.Complete = len(stack) == 0 && !stats.BudgetHit
	stats.PeakStack = res.MaxDepth
	stats.Elapsed = time.Since(start)
	if stats.Elapsed > 0 {
		stats.StatesPerSec = float64(res.States) / stats.Elapsed.Seconds()
	}
	return res, stats
}

func budget(res *Result, stats *Stats, reason string) {
	stats.BudgetHit = true
	if res.Status == StatusViolated {
		return // a violation found before the budget ran out stands
	}
	res.Status = StatusInconclusive
	res.Evidence = EvidenceBounded
	res.Reason = reason
}

// Replay re-executes a witness from the initial state by label and returns
// the state reached. It fails if some label is not enabled where the witness
// claims it is. Used by tests to check that a reported witness is a real run.
func Replay(m Model, w []Step) ([]byte, error) {
	s := append([]byte(nil), m.Initial()...)
	for i, st := range w {
		var found []byte
		m.Successors(s, func(label string, next []byte, _ string) {
			if found == nil && label == st.Label {
				found = append([]byte(nil), next...)
			}
		})
		if found == nil {
			return nil, fmt.Errorf("step %d: transition %q not enabled in %s", i, st.Label, m.Describe(s))
		}
		s = found
	}
	return s, nil
}

// ErrUnknownModel is returned by ByName for names it does not know.
var ErrUnknownModel = errors.New("unknown model")

// ByName builds a hard-coded model from its feature-file name.
func ByName(name string) (Model, error) {
	switch name {
	case "petrinet1":
		return NewPetrinet1(), nil
	case "mutex_flaw":
		return NewMutexFlaw(), nil
	}
	var k, n int
	if _, err := fmt.Sscanf(name, "counters(K=%d, N=%d)", &k, &n); err == nil {
		return NewCounters(k, n)
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownModel, name)
}
