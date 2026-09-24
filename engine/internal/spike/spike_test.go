package spike

import (
	"bytes"
	"testing"
	"time"
)

var testOpts = Options{MaxStates: 2_000_000, Timeout: 60 * time.Second}

func TestPetrinet1Deadlock(t *testing.T) {
	m := NewPetrinet1()
	res, _ := Explore(m, testOpts)
	if res.Status != StatusViolated || res.Violation != KindDeadlock {
		t.Fatalf("status=%s violation=%s, want violated/deadlock", res.Status, res.Violation)
	}
	if res.Evidence != EvidenceExhaustive {
		t.Errorf("evidence=%s", res.Evidence)
	}
	if len(res.Witness) != 2 || res.Witness[0].Label != "t1" || res.Witness[1].Label != "t4" {
		t.Errorf("witness=%v, want t1,t4", res.Witness)
	}
	if res.FinalState != "p2=1 p5=1" {
		t.Errorf("final=%q", res.FinalState)
	}
	// Full sweep (pan -c0 semantics): 6 markings, matching pan's 8 stored
	// minus the 2 initialisation states of the Promela init process.
	full, _ := Explore(m, Options{ContinueAfterViolation: true})
	if full.States != 6 {
		t.Errorf("states=%d, want 6", full.States)
	}
	if full.Violations != 1 {
		t.Errorf("violations=%d, want 1 (only p2=p5=1 hangs)", full.Violations)
	}
	got, err := Replay(m, res.Witness)
	if err != nil || m.Describe(got) != res.FinalState {
		t.Errorf("replay: %v / %s", err, m.Describe(got))
	}
}

func TestMutexFlawAssertion(t *testing.T) {
	m := NewMutexFlaw()
	res, _ := Explore(m, testOpts)
	if res.Status != StatusViolated || res.Violation != KindAssertion {
		t.Fatalf("status=%s violation=%s", res.Status, res.Violation)
	}
	if res.Statement != "assert(cnt == 1)" {
		t.Errorf("statement=%q", res.Statement)
	}
	if res.FinalVars[0].Name != "cnt" || res.FinalVars[0].Value != 2 {
		t.Errorf("final vars=%v, want cnt=2", res.FinalVars)
	}
	got, err := Replay(m, res.Witness)
	if err != nil || m.Describe(got) != res.FinalState {
		t.Errorf("replay: %v / %s vs %s", err, m.Describe(got), res.FinalState)
	}
	// Full sweep must equal pan -c0 on the same file: 429 states stored
	// (spin 6.5.2, -DNOREDUCE, with and without -o1 -o2 -o3).
	full, _ := Explore(m, Options{ContinueAfterViolation: true})
	if full.States != 429 {
		t.Errorf("states=%d, want 429 (pan -c0)", full.States)
	}
	if full.Violations == 0 {
		t.Errorf("no violations in full sweep")
	}
}

func TestCountersExactSize(t *testing.T) {
	for _, tc := range []struct{ k, n int }{{2, 3}, {10, 3}, {7, 4}} {
		m, err := NewCounters(tc.k, tc.n)
		if err != nil {
			t.Fatal(err)
		}
		for _, compact := range []bool{false, true} {
			res, _ := Explore(m, Options{CompactSet: compact})
			if res.Status != StatusVerified || res.Evidence != EvidenceExhaustive {
				t.Errorf("K=%d N=%d compact=%v: status=%s/%s", tc.k, tc.n, compact, res.Status, res.Evidence)
			}
			if want := m.ExpectedStates(); res.States != want {
				t.Errorf("K=%d N=%d compact=%v: states=%d want %d", tc.k, tc.n, compact, res.States, want)
			}
			// every state has N successors
			if res.Transitions != res.States*tc.n {
				t.Errorf("transitions=%d want %d", res.Transitions, res.States*tc.n)
			}
		}
	}
	if _, err := NewCounters(1, 3); err == nil {
		t.Error("K=1 accepted")
	}
}

func TestBudgetIsInconclusive(t *testing.T) {
	m, _ := NewCounters(10, 5)
	res, st := Explore(m, Options{MaxStates: 1000})
	if res.Status != StatusInconclusive || res.Evidence != EvidenceBounded || res.Reason != "state budget exhausted" {
		t.Errorf("got %s/%s/%q", res.Status, res.Evidence, res.Reason)
	}
	if res.States != 1000 || !st.BudgetHit {
		t.Errorf("states=%d budgetHit=%v", res.States, st.BudgetHit)
	}
	// petrinet1's deadlock is the 4th stored state: with budget 4 it is
	// examined and reported; with budget 3 the run is inconclusive.
	pn, _ := Explore(NewPetrinet1(), Options{MaxStates: 4})
	if pn.Status != StatusViolated || pn.States != 4 {
		t.Errorf("petrinet1 budget 4: %s (%s) states=%d", pn.Status, pn.Reason, pn.States)
	}
	pn3, _ := Explore(NewPetrinet1(), Options{MaxStates: 3})
	if pn3.Status != StatusInconclusive || pn3.States != 3 {
		t.Errorf("petrinet1 budget 3: %s states=%d", pn3.Status, pn3.States)
	}
	// A violation found before the budget runs out is not downgraded.
	mx, _ := Explore(NewMutexFlaw(), Options{MaxStates: 100000})
	if mx.Status != StatusViolated {
		t.Errorf("mutex_flaw: %s", mx.Status)
	}
}

func TestDeterministicJSON(t *testing.T) {
	for _, name := range []string{"petrinet1", "mutex_flaw", "counters(K=10, N=4)"} {
		m, err := ByName(name)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := Explore(m, testOpts)
		b, _ := Explore(m, testOpts)
		if !bytes.Equal(a.JSON(), b.JSON()) {
			t.Errorf("%s: two runs differ", name)
		}
		c, _ := Explore(m, Options{CompactSet: true, MaxStates: testOpts.MaxStates})
		if !bytes.Equal(a.JSON(), c.JSON()) {
			t.Errorf("%s: visited-set implementation changed the report", name)
		}
	}
	if _, err := ByName("nonsense"); err == nil {
		t.Error("unknown model accepted")
	}
}

func TestCompactVisited(t *testing.T) {
	v := newCompactVisited(3, 1)
	seen := 0
	for a := 0; a < 20; a++ {
		for b := 0; b < 20; b++ {
			if v.Add([]byte{byte(a), byte(b), 7}) {
				seen++
			}
		}
	}
	if seen != 400 || v.Len() != 400 {
		t.Fatalf("seen=%d len=%d", seen, v.Len())
	}
	if v.Add([]byte{3, 4, 7}) {
		t.Error("duplicate accepted")
	}
	if !v.Add([]byte{3, 4, 8}) {
		t.Error("new state rejected")
	}
}

func BenchmarkCounters10x5Map(b *testing.B)     { benchCounters(b, 10, 5, false) }
func BenchmarkCounters10x5Compact(b *testing.B) { benchCounters(b, 10, 5, true) }

func benchCounters(b *testing.B, k, n int, compact bool) {
	m, _ := NewCounters(k, n)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		res, _ := Explore(m, Options{CompactSet: compact})
		if res.States != m.ExpectedStates() {
			b.Fatal("wrong size")
		}
	}
}
