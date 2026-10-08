package explore

import (
	"context"
	"path/filepath"
	"testing"
)

// `provided` gates every transition of a process, and a rendezvous is one
// step of two processes: a process whose clause is false can neither send into
// a handshake nor receive in one. The explorer checked the clause on every
// other step and on neither side of a handshake, so such a process took part
// in rendezvous it must not (features/g1-promela.feature, pan: 1 state, the
// invalid end state and no assertion violation on both models).
func TestProvidedGatesBothSidesOfARendezvous(t *testing.T) {
	for _, file := range []string{"provided-rv-send.pml", "provided-rv-recv.pml"} {
		t.Run(file, func(t *testing.T) {
			m, defs := parseFile(t, filepath.Join(weakfairTestdata, file))
			res, err := Run(context.Background(), m, Options{Sweep: true, Defines: defs})
			if err != nil {
				t.Fatal(err)
			}
			if o := outcomeOf(t, res, "deadlock"); o.Status != Violated {
				t.Errorf("deadlock: %s, want violated: the process that is not allowed to move leaves the other waiting for ever", o.Status)
			}
			if o := outcomeOf(t, res, "assert"); o.Status != Verified {
				t.Errorf("assert: %s (%s), want verified: the assertion is behind a handshake that cannot happen", o.Status, o.Reason)
			}
			if n := res.States; n != 1 {
				t.Errorf("%d states, pan stores 1", n)
			}
		})
	}
}
