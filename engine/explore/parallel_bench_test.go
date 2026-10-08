package explore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"modelcheck/ir"
)

// Tuning benchmarks of the parallel search: the sizes that must not change a
// result (segment, group, inline threshold) at a few worker counts, on the
// models the plan names. Environment: MCD_TUNE_MODEL (indep5 | counters |
// two | chain), MCD_TUNE_W (comma-separated worker counts), MCD_TUNE_S,
// MCD_TUNE_G, MCD_TUNE_T (comma-separated sizes; 0 = the default). A run
// prints one line per combination. It is a tool for step 8 of the plan, not a
// test: it is skipped unless MCD_TUNE_MODEL is set.
func tuneModel(t *testing.T, name string) *ir.Model {
	switch name {
	case "indep5":
		m, _ := parseFile(t, filepath.Join("../testdata/promela", "bench-indep.pml"), "N=5", "K=4")
		return m
	case "indep6":
		m, _ := parseFile(t, filepath.Join("../testdata/promela", "bench-indep.pml"), "N=6", "K=4")
		return m
	case "two":
		m, _ := parseFile(t, filepath.Join("../testdata/promela", "par-two.pml"), "N=1000")
		return m
	case "chain":
		m, _ := parseFile(t, filepath.Join("../testdata/promela", "par-chain.pml"), "N=1000000")
		return m
	case "client":
		m, _ := parseFile(t, filepath.Join(corpus, "CH15", "client_server.pml"))
		return m
	case "counters":
		return counters(10, 6)
	}
	t.Fatalf("unknown model %q", name)
	return nil
}

func ints(s string, def []int) []int {
	if s == "" {
		return def
	}
	var out []int
	for _, f := range splitCSV(s) {
		n, err := strconv.Atoi(f)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(r)
		}
	}
	return append(out, cur)
}

func TestTuneParallel(t *testing.T) {
	name := os.Getenv("MCD_TUNE_MODEL")
	if name == "" {
		t.Skip("set MCD_TUNE_MODEL to tune the parallel search")
	}
	m := tuneModel(t, name)
	reps := ints(os.Getenv("MCD_TUNE_REPS"), []int{3})[0]
	for _, w := range ints(os.Getenv("MCD_TUNE_W"), []int{1, 8}) {
		for _, s := range ints(os.Getenv("MCD_TUNE_S"), []int{0}) {
			for _, g := range ints(os.Getenv("MCD_TUNE_G"), []int{0}) {
				for _, inl := range ints(os.Getenv("MCD_TUNE_T"), []int{0}) {
					var best, sum float64
					var states int
					for r := 0; r < reps; r++ {
						opt := Options{Sweep: true, Workers: w}
						if w == 0 {
							opt.Mode = DFS
						}
						if s != 0 || g != 0 || inl != 0 {
							opt.par = &parKnobs{segment: s, group: g, inline: inl}
						}
						res, err := Run(context.Background(), m, opt)
						if err != nil {
							t.Fatal(err)
						}
						el := res.Elapsed.Seconds()
						sum += el
						if r == 0 || el < best {
							best = el
						}
						states = res.States
					}
					fmt.Printf("%s w=%d S=%d G=%d T=%d: states %d best %.3fs mean %.3fs\n", name, w, s, g, inl, states, best, sum/float64(reps))
				}
			}
		}
	}
}
