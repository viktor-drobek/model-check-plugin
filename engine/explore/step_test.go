package explore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// The Stepper lists the moves of a state as the searches do. `timeout` is true
// only when no statement of any process is executable, so the timeout moves of
// a state are listed only when it has no other move. The Stepper used to list
// them always (last), which let a simulated run take a timeout while another
// process could move, and gave the SCC oracle of the weak-fairness tests a
// graph with timeout moves that the engine never takes.
func TestStepperOffersTimeoutOnlyWhenNothingElseCanMove(t *testing.T) {
	m, _ := parseFile(t, filepath.Join(weakfairTestdata, "timeout-gate.pml"))
	st, err := NewStepper(m)
	if err != nil {
		t.Fatal(err)
	}
	names := func(s []byte) []string {
		mv, err := st.Enabled(s)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, x := range mv {
			out = append(out, m.Processes[x.Edge.Proc].Name+":"+st.Edge(x.Edge).Text)
		}
		return out
	}
	init := st.Initial()
	if got := strings.Join(names(init), ", "); got != "P1:0:timeout, P2:1:timeout" {
		t.Fatalf("initial state: %s, want both timeouts (nothing else is executable)", got)
	}
	mv, _ := st.Enabled(init)
	afterP1, _, err := st.Apply(init, mv[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(names(afterP1), ", "); got != "P1:0:a = 1 - a" {
		t.Fatalf("after P1's timeout: %s, want only P1's assignment: P2's timeout is false while P1 can move", got)
	}
	p2timeout := mv[1]
	if _, _, err := st.Apply(afterP1, p2timeout); !errors.Is(err, ErrNotEnabled) {
		t.Fatalf("Apply of P2's timeout while P1 can move: %v, want ErrNotEnabled", err)
	}
}
