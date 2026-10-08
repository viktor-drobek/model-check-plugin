package modelcheck_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"modelcheck/cli"
)

// Benchmarks of the parallel search (performance plan 5): the same model with
// the sequential depth-first search (workers 0) and with 1, 2, 4, 8 and 16
// workers, on the shapes the plan names. The state counts are checked, so a
// faster search that explores something else cannot pass. The big model is
// behind MCD_BENCH_BIG=1 (8 million states, 0.4 GB).
//
//	go test -run XXX -bench Parallel -benchtime 3x
//
// Speedup is only meaningful on a quiet machine (check `uptime`): see
// steps/perf5-confirmation.md for what was measured and under which load.
func benchParallel(b *testing.B, kind, file string, defines []string, wantStates int) {
	for _, w := range []int{0, 1, 2, 4, 8, 16} {
		b.Run(fmt.Sprintf("workers%d", w), func(b *testing.B) {
			args := []string{"check", "--" + kind, file, "--sweep", "--no-timing", "--unlimited"}
			for _, d := range defines {
				args = append(args, "-D", d)
			}
			if w > 0 {
				args = append(args, "--workers", fmt.Sprint(w))
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var out, errb bytes.Buffer
				if code := cli.Run(args, &out, &errb); code != 0 {
					b.Fatalf("exit %d: %s%s", code, out.String(), errb.String())
				}
				var r struct {
					Properties []struct {
						Counters struct {
							States int `json:"states"`
						} `json:"counters"`
					} `json:"properties"`
				}
				if err := json.Unmarshal(out.Bytes(), &r); err != nil {
					b.Fatal(err)
				}
				if got := r.Properties[len(r.Properties)-1].Counters.States; got != wantStates {
					b.Fatalf("states = %d, want %d", got, wantStates)
				}
			}
		})
	}
}

func BenchmarkParallelIndepN5(b *testing.B) {
	benchParallel(b, "promela", "testdata/promela/bench-indep.pml", []string{"N=5", "K=4"}, 579195)
}

func BenchmarkParallelCounters10x6(b *testing.B) {
	benchParallel(b, "ir", "testdata/ir/counters-10-6.json", nil, 1000000)
}

func BenchmarkParallelClientServer(b *testing.B) {
	benchParallel(b, "promela", corpusDir+"/CH15/client_server.pml", nil, 191200)
}

// two counters to 1000: cheap steps, thousands of layers (the moderate shape).
func BenchmarkParallelTwoCounters(b *testing.B) {
	benchParallel(b, "promela", "testdata/promela/par-two.pml", []string{"N=1000"}, 4010007)
}

// one counter to 10^6: a single path, one state per layer (the narrow shape).
func BenchmarkParallelChain(b *testing.B) {
	benchParallel(b, "promela", "testdata/promela/par-chain.pml", []string{"N=1000000"}, 2000003)
}

func BenchmarkParallelIndepN6(b *testing.B) {
	if os.Getenv("MCD_BENCH_BIG") == "" {
		b.Skip("set MCD_BENCH_BIG=1 for the 8-million-state model")
	}
	benchParallel(b, "promela", "testdata/promela/bench-indep.pml", []string{"N=6", "K=4"}, 8108731)
}
