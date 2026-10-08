package explore

import (
	"context"
	"errors"
	"strings"
	"testing"

	"modelcheck/ir"
)

// lyingVisited is a visited set that, once armed, answers its next lookup
// with a state that the search's own bookkeeping calls "on the outer stack"
// although no frame of the outer stack holds it: the state in which a defect
// of the nested search would find itself. No model can bring the search there
// (that is what makes it a defect), so the test builds the search by hand and
// lets the set lie.
type lyingVisited struct {
	Visited
	cs    *cycleSearch
	armed bool
	lied  bool
}

func (v *lyingVisited) Has(state []byte) (int, bool) {
	if v.armed && !v.lied {
		v.lied = true
		v.cs.onStack = append(v.cs.onStack, true)
		v.cs.inner = append(v.cs.inner, false)
		return len(v.cs.onStack) - 1, true
	}
	return v.Visited.Has(state)
}

// neverClaimSearch builds the search that runCycle builds for the never claim
// of a model (no fairness), with a visited set the test controls, and returns
// it with the index of the property the search decides.
func neverClaimSearch(t *testing.T, path string, armed bool) (*cycleSearch, *lyingVisited) {
	t.Helper()
	m, _ := parseFile(t, path)
	var prop ir.Property
	for _, p := range m.Properties {
		if p.ID == "never" {
			prop = p
		}
	}
	if prop.ID == "" {
		t.Fatalf("%s has no property \"never\": %+v", path, m.Properties)
	}
	claimName := ""
	for _, p := range m.Processes {
		if p.Claim {
			claimName = p.Name
		}
	}
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	s := &search{c: c, ctx: context.Background(), res: &Result{StateBytes: c.layout.Size}}
	cs := &cycleSearch{s: s, kind: prop.Kind, claim: -1, size: c.layout.Size, pl: c.layout.Size,
		info: &TemporalInfo{Logic: "ltl", Fairness: "none", Source: "never-claim", Claim: claimName, Atoms: []string{}}}
	cs.res = &Stats{StateBytes: c.layout.Size}
	for p := range c.procs {
		if c.procs[p].claim && m.Processes[p].Name == claimName {
			cs.claim = p
		}
		if !c.procs[p].claim {
			cs.nproc++
		}
	}
	if cs.claim < 0 {
		t.Fatalf("%s has no never claim", path)
	}
	pr := &m.Processes[cs.claim]
	cs.cOut = c.procs[cs.claim].out
	cs.cAcc = make([]bool, len(pr.Locations))
	cs.cEnd = make([]bool, len(pr.Locations))
	for i, loc := range pr.Locations {
		for _, lb := range loc.Labels {
			if lb == ir.Accept {
				cs.cAcc[i] = true
			}
			if lb == ir.End {
				cs.cEnd[i] = true
			}
		}
	}
	cs.pcur = make([]byte, cs.pl)
	cs.pnext = make([]byte, cs.pl)
	s.cur = cs.pcur[:cs.size]
	s.next = cs.pnext[:cs.size]
	v := &lyingVisited{Visited: defaultVisited(cs.pl), cs: cs, armed: armed}
	s.visited = v
	s.res.Outcomes = []Outcome{{Property: prop}}
	s.undecided = 1
	return cs, v
}

// TestClosingStateMissingFromTheOuterStackIsAnInternalError: the inner search
// closes a cycle at a state its bookkeeping puts on the outer stack, and looks
// that state's position up there. A state the lookup cannot find is a defect
// of the engine: it is returned as ErrInternal (a tool failure, never a
// statement about the model), not recorded as an invalid-model verdict of the
// property, which a caller would read as a result.
func TestClosingStateMissingFromTheOuterStackIsAnInternalError(t *testing.T) {
	const model = "../testdata/promela/claim-atomic-loop.pml"

	// The same search with an honest set finds its acceptance cycle: the
	// forged set, and nothing else, is what puts the search into the defect.
	t.Run("control: an honest set finds the acceptance cycle", func(t *testing.T) {
		cs, v := neverClaimSearch(t, model, false)
		if err := cs.run(); err != nil {
			t.Fatalf("run: %v", err)
		}
		if v.lied {
			t.Fatal("the control must not lie")
		}
		o := cs.s.res.Outcomes[0]
		if o.Status != Violated || !strings.Contains(o.Reason, "acceptance cycle") {
			t.Fatalf("never: %s %q", o.Status, o.Reason)
		}
	})

	t.Run("a closing state the outer stack lacks is ErrInternal", func(t *testing.T) {
		cs, v := neverClaimSearch(t, model, true)
		err := cs.run()
		if !v.lied {
			t.Fatal("the inner search never asked the set for a closing state: the test does not reach the defect")
		}
		o := cs.s.res.Outcomes[0]
		if err == nil {
			t.Fatalf("run returned no error; the defect was recorded as the outcome %s %q", o.Status, o.Reason)
		}
		if !errors.Is(err, ErrInternal) || !strings.Contains(err.Error(), "closing state") || !strings.Contains(err.Error(), "not on the outer stack") {
			t.Fatalf("want an ErrInternal about the closing state, got: %v", err)
		}
		if o.Status == InvalidModel {
			t.Fatalf("the defect is also recorded as an invalid-model verdict: %q", o.Reason)
		}
	})
}
