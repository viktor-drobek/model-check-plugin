package spike

import "fmt"

// Counters is the synthetic throughput model: N independent processes, each
// owning one byte counter that it increments modulo K. The reachable graph is
// exactly K^N states, every state has N enabled transitions, there is no
// deadlock and no assertion, so a run is a full sweep of a graph of known
// size. Promela equivalent (see steps/spike-confirmation.md):
//
//	active [N] proctype P() { byte c; do :: c = (c + 1) % K od }
type Counters struct {
	K, N   int
	name   string
	labels []string // precomputed: fmt on the hot path would distort states/s
}

// NewCounters requires 2 ≤ K ≤ 256 and 1 ≤ N ≤ 32 so that K^N stays a
// meaningful, byte-representable model.
func NewCounters(k, n int) (*Counters, error) {
	if k < 2 || k > 256 || n < 1 || n > 32 {
		return nil, fmt.Errorf("counters: K=%d N=%d out of range (2≤K≤256, 1≤N≤32)", k, n)
	}
	c := &Counters{K: k, N: n, name: fmt.Sprintf("counters(K=%d, N=%d)", k, n)}
	for i := 0; i < n; i++ {
		c.labels = append(c.labels, fmt.Sprintf("P[%d]:c++", i))
	}
	return c, nil
}

// ExpectedStates is K^N, or -1 if it does not fit in an int.
func (c *Counters) ExpectedStates() int {
	total := 1
	for i := 0; i < c.N; i++ {
		if total > (1<<62)/c.K {
			return -1
		}
		total *= c.K
	}
	return total
}

func (c *Counters) Name() string { return c.name }

func (c *Counters) Initial() []byte { return make([]byte, c.N) }

func (c *Counters) Successors(s []byte, fn func(string, []byte, string)) {
	next := make([]byte, c.N)
	for i := 0; i < c.N; i++ {
		copy(next, s)
		next[i] = byte((int(s[i]) + 1) % c.K)
		fn(c.labels[i], next, "")
	}
}

func (c *Counters) Describe(s []byte) string { return fmt.Sprintf("c=%v", s) }

func (c *Counters) Vars(s []byte) []Var {
	vs := make([]Var, c.N)
	for i, v := range s {
		vs[i] = Var{Name: fmt.Sprintf("c%d", i), Value: int(v)}
	}
	return vs
}
