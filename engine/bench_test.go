package modelcheck_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"modelcheck/cli"
)

// Whole-engine benchmarks on the Promela fixtures of testdata/promela, through
// the same cli.Run the mcd binary calls. They are the before/after yardstick
// of the performance plan; the state counts are checked so that a faster
// engine that explores something else cannot pass.
//
//	go test -run XXX -bench Engine -benchmem
func benchCheck(b *testing.B, file string, defines []string, wantStates int, extra ...string) {
	b.Helper()
	args := []string{"check", "--promela", file, "--sweep", "--no-timing",
		"--budget-states", "0", "--budget-depth", "0", "--budget-ms", "600000", "--budget-mem-mb", "8192"}
	for _, d := range defines {
		args = append(args, "-D", d)
	}
	args = append(args, extra...)
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
}

func BenchmarkEngineIndepN5(b *testing.B) {
	benchCheck(b, "testdata/promela/bench-indep.pml", []string{"N=5", "K=4"}, 579195)
}

func BenchmarkEngineSymN6(b *testing.B) {
	benchCheck(b, "testdata/promela/bench-sym.pml", []string{"N=6"}, 543076)
}

// The same models with --por (performance plan, step 2). The independent
// processes collapse to one chain; the lock model has atomic sequences, is
// refused, and shows what a refused request costs (nothing measurable).
func BenchmarkEngineIndepN5POR(b *testing.B) {
	benchCheck(b, "testdata/promela/bench-indep.pml", []string{"N=5", "K=4"}, 71, "--por")
}

func BenchmarkEngineIndepN12POR(b *testing.B) {
	// 6^12 = 2 176 782 336 states in full: out of reach without the reduction.
	benchCheck(b, "testdata/promela/bench-indep.pml", []string{"N=12", "K=4"}, 169, "--por")
}

func BenchmarkEngineSymN6POR(b *testing.B) {
	benchCheck(b, "testdata/promela/bench-sym.pml", []string{"N=6"}, 543076, "--por")
}

// A pipeline over buffered channels (performance plan, step 4): the stages are
// explored one after the other, so the states grow with K, not with the
// interleavings of the stages.
func BenchmarkEnginePipelinePOR(b *testing.B) {
	benchCheck(b, "testdata/promela/por-pipeline.pml", []string{"K=10"}, 117, "--por")
}
