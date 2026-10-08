package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/explore"
	"modelcheck/frontend/promela"
	"modelcheck/ir"
)

// mc_simulate on models that read the live-process table (the simulate
// campaign of the 0.3.0 integration).
//
// Each generated model snapshots `_nr_pr` into the globals s0..s2 while its
// processes (active ones and some started by `run`) end in different orders.
// The ground truth is the engine's own exhaustive search of the same model: for
// every s_i and value c, "s_i == c" is reachable or not, and a deadlock or a
// failing assert is reachable or not. The simulation goes through the real
// server (mc_parse, then mc_simulate in random mode over many seeds). A run is
// wrong when it shows a snapshot value no run of the model can produce, stops
// with "deadlock" or "assert failed" where the search says that cannot happen,
// takes more steps than it reports, or answers differently for the same seed.
//
// MCD_SIM_MODELS (default 25), MCD_SIM_SEEDS per model (default 40) and
// MCD_SIM_FIRST (first model seed) scale it.

func simEnv(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// simModel makes the Promela text of one model of the family.
func simModel(rnd *rand.Rand) string {
	var b strings.Builder
	b.WriteString("byte s0, s1, s2, g;\n")
	stmt := func() string {
		switch rnd.Intn(8) {
		case 0, 1, 2:
			return fmt.Sprintf("s%d = _nr_pr", rnd.Intn(3))
		case 3:
			return "g = (g + 1) % 3"
		case 4:
			return fmt.Sprintf("(_nr_pr >= %d)", rnd.Intn(4))
		case 5:
			return fmt.Sprintf("(_nr_pr <= %d)", 1+rnd.Intn(3))
		case 6:
			return fmt.Sprintf("if :: _nr_pr == %d -> g = 1 :: else -> skip fi", rnd.Intn(4))
		}
		return "skip"
	}
	body := func() string {
		n := 1 + rnd.Intn(3)
		parts := make([]string, n)
		for i := range parts {
			parts[i] = stmt()
		}
		return strings.Join(parts, "; ")
	}
	nw := rnd.Intn(3)
	if nw > 0 {
		fmt.Fprintf(&b, "proctype W() { %s }\n", body())
	}
	for i, n := 0, 1+rnd.Intn(3); i < n; i++ {
		fmt.Fprintf(&b, "active proctype P%d() { %s }\n", i, body())
	}
	if nw > 0 {
		b.WriteString("init { ")
		for i := 0; i < nw; i++ {
			fmt.Fprintf(&b, "run W(); ")
		}
		fmt.Fprintf(&b, "%s }\n", body())
	}
	return b.String()
}

type simTruth struct {
	reach    map[string]bool // "s1==2" -> reachable
	deadlock bool
	assert   bool
}

func simTruthOf(m *ir.Model) (*simTruth, error) {
	m2 := *m
	m2.Properties = append([]ir.Property(nil), m.Properties...) // the frontend's deadlock and assert properties stay
	for i := 0; i < 3; i++ {
		for c := int64(0); c < 10; c++ {
			m2.Properties = append(m2.Properties, ir.Property{
				ID: fmt.Sprintf("s%d==%d", i, c), Kind: ir.KindReach,
				Expr: ir.Binary("eq", ir.Ref(fmt.Sprintf("s%d", i)), ir.Const(c))})
		}
	}
	res, err := explore.Run(context.Background(), &m2, explore.Options{Sweep: true, Budget: explore.Budget{MaxStates: 200000, MaxDepth: 100000}})
	if err != nil {
		return nil, err
	}
	if !res.Complete {
		return nil, fmt.Errorf("incomplete search: %s", res.Stop)
	}
	tr := &simTruth{reach: map[string]bool{}}
	for _, o := range res.Outcomes {
		if os.Getenv("MCD_SIM_DEBUG") != "" && !strings.Contains(o.Property.ID, "==") {
			fmt.Printf("outcome %q kind %q status %s\n", o.Property.ID, o.Property.Kind, o.Status)
		}
		switch o.Property.ID {
		case "deadlock":
			tr.deadlock = o.Status == explore.Violated
		case "assert":
			tr.assert = o.Status == explore.Violated
		default:
			tr.reach[o.Property.ID] = o.Status == explore.Verified
		}
	}
	return tr, nil
}

func simCall(t *testing.T, cs *sdk.ClientSession, tool string, args map[string]any, out any) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil || res.IsError {
		msg := ""
		if res != nil {
			for _, c := range res.Content {
				if tc, ok := c.(*sdk.TextContent); ok {
					msg += tc.Text
				}
			}
		}
		t.Fatalf("%s: %v %s", tool, err, msg)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
}

func TestSimulateOnModelsWithTheProcessTable(t *testing.T) {
	models, seeds := simEnv("MCD_SIM_MODELS", 25), simEnv("MCD_SIM_SEEDS", 40)
	first := int64(simEnv("MCD_SIM_FIRST", 1))
	var withTable, observed, possible, runs, stops = 0, 0, 0, 0, map[string]int{}
	failures := 0
	for k := int64(0); k < int64(models) && failures < 5; k++ {
		k := k
		t.Run(fmt.Sprint(first+k), func(t *testing.T) {
			src := simModel(rand.New(rand.NewSource(first + k)))
			pr, perr := promela.Parse([]byte(src), "sim.pml", nil)
			if perr != nil {
				t.Fatalf("model %d does not parse: %v\n%s", first+k, perr, src)
			}
			truth, err := simTruthOf(pr.Model)
			if err != nil {
				return // too big for the ground truth: not compared
			}
			withTable++
			_, cs := connect(t, Config{Promela: PromelaViaCLI}) // a session directory of its own, removed with the subtest
			var parsed ParseOut
			simCall(t, cs, "mc_parse", map[string]any{"promela": src}, &parsed)
			if parsed.Outcome != "ir" {
				t.Fatalf("mc_parse of model %d: %s %+v %s\n%s", first+k, parsed.Outcome, parsed.Rejection, parsed.Reason, src)
			}
			seen := map[string]bool{}
			for s := int64(1); s <= int64(seeds); s++ {
				var a, b SimulateOut
				args := map[string]any{"session_id": parsed.SessionID, "mode": "random", "seed": s, "steps": 50}
				simCall(t, cs, "mc_simulate", args, &a)
				simCall(t, cs, "mc_simulate", args, &b)
				runs++
				stops[a.Stopped]++
				fail := func(format string, v ...any) {
					failures++
					t.Errorf("model %d seed %d: %s\n%s", first+k, s, fmt.Sprintf(format, v...), src)
				}
				if a.Summary != b.Summary || a.Stopped != b.Stopped || a.StepsTaken != b.StepsTaken {
					fail("the same seed gave two answers: %q/%s vs %q/%s", a.Summary, a.Stopped, b.Summary, b.Stopped)
				}
				switch a.Stopped {
				case "deadlock":
					if !truth.deadlock {
						fail("stopped with a deadlock, the search finds none")
					}
				case "assert failed":
					if !truth.assert {
						fail("stopped on a failing assert, the search finds none")
					}
				case "steps", "terminated":
				default:
					fail("unexpected stop %q (%s)", a.Stopped, a.StopReason)
				}
				if a.Trace == nil {
					fail("no embedded trace for a run of %d steps", a.StepsTaken)
					continue
				}
				if len(a.Trace.Steps) > a.StepsTaken {
					fail("the trace has %d steps, the answer says %d", len(a.Trace.Steps), a.StepsTaken)
				}
				note := func(v string, val int64) {
					key := fmt.Sprintf("%s==%d", v, val)
					if val > 0 && !truth.reach[key] {
						fail("%s observed, the exhaustive search says it is unreachable", key)
					}
					seen[key] = true
				}
				for _, st := range a.Trace.Steps {
					for _, ch := range st.Changes {
						if strings.HasPrefix(ch.Var, "s") {
							note(ch.Var, ch.After)
						}
					}
				}
				for _, v := range a.Trace.Final {
					if strings.HasPrefix(v.Var, "s") {
						note(v.Var, v.Value)
					}
				}
			}
			for key, ok := range truth.reach {
				if ok && !strings.HasSuffix(key, "==0") {
					possible++
					if seen[key] {
						observed++
					}
				}
			}
		})
	}
	t.Logf("simulate on table models: %d models compared, %d runs, stops %v, %d of %d reachable nonzero snapshot values seen, %d failures",
		withTable, runs, stops, observed, possible, failures)
	// A ground truth that cannot be computed skips the model; too many skips
	// would let the campaign pass having compared almost nothing.
	if withTable < models/2 {
		t.Fatalf("only %d of %d models had a complete ground truth", withTable, models)
	}
}
