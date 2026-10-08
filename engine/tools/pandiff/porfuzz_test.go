package pandiff

// Outside witnesses for the partial-order reduction (performance plan, step 6,
// oracles O5 and O6): small random Promela models, in the shapes the frontend
// really emits for the constructs the reduction handles, run
//
//   - through the frontend and the engine in full and with `--por` (O6: the two
//     must agree on every verdict and on whether an error of the model is
//     reachable), and
//   - against SPIN's pan (O5: `spin -a -o1 -o2 -o3`, `gcc -DNOREDUCE`), so that
//     a misreading of Promela that the full search and its reduction share is
//     seen by a third party.
//
// The shapes: `init { atomic { run P(); run Q() } }`, `init { run ... }` and
// `active` processes; `atomic` blocks with guards (a block that blocks inside),
// `d_step`, `if ... else`, bounded loops, `_nr_pr` guards, a buffered channel.
//
// MCD_POR_FUZZ is the number of models of the engine comparison (default 400),
// MCD_POR_FUZZ_SPIN of the SPIN comparison (default 10: each costs a `spin -a`
// and a `gcc` run, about a second), MCD_POR_FUZZ_SEED the first seed.

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"modelcheck/explore"
	"modelcheck/frontend/promela"
	"modelcheck/ir"
)

// fuzzModel is a generated Promela source and what matters about it.
type fuzzModel struct {
	src   string
	nrPr  bool // a statement reads _nr_pr
	byRun bool // the processes are started by `run`
}

func fuzzPromela(seed int64) fuzzModel {
	rnd := rand.New(rand.NewSource(seed))
	nproc := 2 + rnd.Intn(2)
	useChan := rnd.Intn(10) < 4
	useNrPr := rnd.Intn(10) < 3
	start := []string{"init_atomic", "init_plain", "active"}[rnd.Intn(3)]
	var out []string
	out = append(out, "byte x, y, z;")
	if useChan {
		out = append(out, fmt.Sprintf("chan c = [%d] of { byte };", 1+rnd.Intn(2)))
	}
	val := func() string { return strconv.Itoa(rnd.Intn(3)) }
	gv := func() string { return string("xyz"[rnd.Intn(3)]) }
	simple := func() string {
		g := gv()
		switch k := rnd.Intn(10); {
		case k == 0:
			return fmt.Sprintf("%s = %s", g, val())
		case k == 1:
			return fmt.Sprintf("%s = (%s + 1) %% 3", g, gv())
		case k == 2:
			return "l = (l + 1) % 3"
		case k == 3:
			return fmt.Sprintf("(%s == %s)", gv(), val())
		case k == 4:
			return fmt.Sprintf("(%s != %s)", gv(), val())
		case k == 5 && useNrPr:
			return fmt.Sprintf("(_nr_pr %s %d)", []string{"<", ">=", "=="}[rnd.Intn(3)], 1+rnd.Intn(4))
		case k == 6 && useChan:
			return []string{"c!" + val(), "c?v"}[rnd.Intn(2)]
		case k == 7:
			return "skip"
		}
		return fmt.Sprintf("%s = %s", g, val())
	}
	atomicBlock := func() string {
		var parts []string
		for i, n := 0, 1+rnd.Intn(3); i < n; i++ {
			parts = append(parts, simple())
		}
		return "atomic { " + strings.Join(parts, "; ") + " }"
	}
	var stmt func(depth int) string
	stmt = func(depth int) string {
		switch k := rnd.Intn(10); {
		case k < 4:
			return simple()
		case k < 6:
			return atomicBlock()
		case k == 6 && depth < 1:
			return fmt.Sprintf("if :: (%s == %s) -> %s :: else -> %s fi", gv(), val(), stmt(depth+1), stmt(depth+1))
		case k == 7:
			return fmt.Sprintf("d_step { %s = %s; %s = %s }", gv(), val(), gv(), val())
		case k == 8 && rnd.Intn(2) == 0:
			return fmt.Sprintf("assert(%s != 2)", gv())
		}
		return simple()
	}
	var procs []string
	for i := 0; i < nproc; i++ {
		var body []string
		for j, n := 0, 2+rnd.Intn(4); j < n; j++ {
			body = append(body, stmt(0))
		}
		text := "byte l; byte v;\n  " + strings.Join(body, ";\n  ")
		if rnd.Intn(10) < 3 {
			first := body[:2]
			rest := body[2:]
			if len(rest) == 0 {
				rest = []string{"skip"}
			}
			text = "byte l; byte v;\n  do\n  :: (l < 2) -> l++; " + strings.Join(first, "; ") + "\n  :: else -> break\n  od;\n  " + strings.Join(rest, ";\n  ")
		}
		procs = append(procs, fmt.Sprintf("proctype P%d() {\n  %s\n}", i, text))
	}
	m := fuzzModel{nrPr: useNrPr, byRun: start != "active"}
	if start == "active" {
		for _, p := range procs {
			out = append(out, "active "+p)
		}
	} else {
		out = append(out, procs...)
		var runs []string
		for i := 0; i < nproc; i++ {
			runs = append(runs, fmt.Sprintf("run P%d()", i))
		}
		body := strings.Join(runs, "; ")
		if start == "init_atomic" {
			body = "atomic { " + body + " }"
		}
		out = append(out, "init { "+body+" }")
	}
	m.src = strings.Join(out, "\n") + "\n"
	return m
}

// fuzzScale reads a count from the environment.
func fuzzScale(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

func fuzzFirstSeed() int64 {
	if v, err := strconv.ParseInt(os.Getenv("MCD_POR_FUZZ_SEED"), 10, 64); err == nil && v > 0 {
		return v
	}
	return 1_000_001
}

// engineRun is one search of a model: the status of every property, whether the
// search stopped on an error of the model, whether it finished, the states.
type engineRun struct {
	statuses []string
	errStop  bool
	budget   bool
	complete bool
	states   int
	applied  bool
	reason   string
}

func runEngineOf(m *ir.Model, defines map[string]string, por bool) (*engineRun, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, err := explore.Run(ctx, m, explore.Options{Sweep: true, POR: por, Defines: defines,
		Budget: explore.Budget{MaxStates: 30000, MaxDepth: 5000}})
	if err != nil {
		return nil, err
	}
	e := &engineRun{complete: r.Complete, states: r.States}
	for _, o := range r.Outcomes {
		e.statuses = append(e.statuses, fmt.Sprintf("%s=%s/%s", o.Property.ID, o.Status, o.Evidence))
	}
	e.errStop = r.Stop == "invalid model" || strings.HasPrefix(r.Stop, "process budget")
	e.budget = strings.Contains(r.Stop, "budget") && !strings.HasPrefix(r.Stop, "process budget")
	if r.Reduction != nil {
		e.applied, e.reason = r.Reduction.Applied, r.Reduction.Reason
	}
	return e, nil
}

func violated(e *engineRun) bool {
	for _, s := range e.statuses {
		if strings.Contains(s, "=violated/") {
			return true
		}
	}
	return false
}

// TestPORFuzzedPromelaKeepsTheVerdicts is O6: the engine in full and reduced, on
// models written as Promela, through the frontend.
func TestPORFuzzedPromelaKeepsTheVerdicts(t *testing.T) {
	n := fuzzScale("MCD_POR_FUZZ", 400)
	if testing.Short() {
		n = max(1, n/8)
	}
	first := fuzzFirstSeed()
	var parsed, rejected, applied, smaller, errored, skipped int
	for seed := first; seed < first+int64(n); seed++ {
		fm := fuzzPromela(seed)
		p, perr := promela.Parse([]byte(fm.src), "fuzz.pml", nil)
		if perr != nil {
			rejected++
			continue
		}
		parsed++
		full, err1 := runEngineOf(p.Model, p.Defines, false)
		red, err2 := runEngineOf(p.Model, p.Defines, true)
		if err1 != nil || err2 != nil {
			t.Fatalf("seed %d: %v %v\n%s", seed, err1, err2, fm.src)
		}
		if !full.budget && !red.budget && full.errStop != red.errStop {
			t.Fatalf("seed %d: an error is reachable with the reduction: %v, without: %v\n%s", seed, red.errStop, full.errStop, fm.src)
		}
		if full.errStop {
			errored++
			continue
		}
		if !full.complete || !red.complete {
			skipped++
			continue
		}
		if red.applied {
			applied++
			if red.states < full.states {
				smaller++
			}
		} else if red.states != full.states {
			t.Fatalf("seed %d: refused (%s) but %d states against %d\n%s", seed, red.reason, red.states, full.states, fm.src)
		}
		if red.states > full.states || strings.Join(red.statuses, " ") != strings.Join(full.statuses, " ") {
			t.Fatalf("seed %d: reduced %v (%d states), full %v (%d states)\n%s", seed, red.statuses, red.states, full.statuses, full.states, fm.src)
		}
	}
	t.Logf("por fuzz: %d models, %d rejected by the frontend, %d with an error of the model in both searches, %d skipped, %d reduction applied, %d smaller", n, rejected, errored, skipped, applied, smaller)
	if rejected > n/10 {
		t.Fatalf("%d of %d generated models were rejected by the frontend: the generator is out of the subset", rejected, n)
	}
}

// TestPORFuzzedPromelaAgreesWithSPIN is O5: pan (no reduction of its own) says
// whether an error exists; the engine in full and reduced must say the same.
//
// A model that reads _nr_pr and starts its processes with `active` used to be
// left out here: the engine's _nr_pr did not decrease when a process of such a
// model ended (steps/perf6-confirmation.md, "Found along the way"). The
// `_nr_pr` fix lowers such a model with end edges that leave the live-process
// table, so it is compared like any other; porfuzz_nrpr_test.go adds a
// generator that makes those models on purpose.
func TestPORFuzzedPromelaAgreesWithSPIN(t *testing.T) {
	tools := Tools{}
	if !tools.Available() {
		t.Skip("spin or gcc not on PATH")
	}
	n := fuzzScale("MCD_POR_FUZZ_SPIN", 10)
	if testing.Short() {
		n = max(1, n/8)
	}
	first := fuzzFirstSeed()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()
	var agree, skipped, reduced, nrPrActive int
	for seed := first; seed < first+int64(n); seed++ {
		fm := fuzzPromela(seed)
		p, perr := promela.Parse([]byte(fm.src), "fuzz.pml", nil)
		if perr != nil {
			skipped++
			continue
		}
		full, err1 := runEngineOf(p.Model, p.Defines, false)
		red, err2 := runEngineOf(p.Model, p.Defines, true)
		if err1 != nil || err2 != nil {
			t.Fatalf("seed %d: %v %v", seed, err1, err2)
		}
		if !full.complete || !red.complete || full.errStop || red.errStop {
			skipped++
			continue
		}
		dir := t.TempDir()
		path := dir + "/m.pml"
		if err := os.WriteFile(path, []byte(fm.src), 0o644); err != nil {
			t.Fatal(err)
		}
		pan, err := tools.RunSpin(ctx, path, nil, dir)
		if err != nil || pan.Incomplete {
			skipped++
			continue
		}
		wantErr := pan.Errors > 0
		if violated(full) != wantErr || violated(red) != wantErr {
			t.Fatalf("seed %d: pan reports an error: %v (%s); the engine in full: %v, reduced: %v\nfull %v\nreduced %v\n%s",
				seed, wantErr, pan.Class, violated(full), violated(red), full.statuses, red.statuses, fm.src)
		}
		agree++
		if fm.nrPr && !fm.byRun {
			nrPrActive++
		}
		if red.applied && red.states < full.states {
			reduced++
		}
	}
	t.Logf("pan: %d models, %d agree with the engine in full and reduced (%d of them read _nr_pr and start their processes with active), %d skipped, %d of the agreeing ones reduced", n, agree, nrPrActive, skipped, reduced)
}

// TestPORMatchesSPINOnTheDifferentialCorpus runs the engine with the reduction
// on the corpus files of TestDifferentialCorpus and sets its verdict and error
// class against pan's. A file on which the reduction is refused is the full
// search, which TestDifferentialCorpus already compares; pan is not run on it.
// By default the first 12 files on which the reduction applies are compared (each
// costs a `spin -a` and a `gcc` run); MCD_POR_CORPUS_SPIN=100 compares them all.
func TestPORMatchesSPINOnTheDifferentialCorpus(t *testing.T) {
	tools := Tools{}
	if !tools.Available() {
		t.Skip("spin or gcc not on PATH")
	}
	if _, err := os.Stat(corpus); err != nil {
		t.Skip("corpus not found")
	}
	type entry struct {
		file    string
		defines []string
	}
	var all []entry
	for _, f := range differentialFiles {
		all = append(all, entry{f, nil})
	}
	for _, c := range differentialV1 {
		all = append(all, entry{c.file, c.defines})
	}
	limit := fuzzScale("MCD_POR_CORPUS_SPIN", 12)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var compared, refused int
	for _, e := range all {
		if compared >= limit {
			break
		}
		path := filepath.Join(corpus, e.file)
		eng, err := RunEngineReduced(ctx, path, e.defines, 0)
		if err != nil {
			t.Errorf("%s: %v", e.file, err)
			continue
		}
		if eng.Reduction == nil || !eng.Reduction.Applied {
			refused++
			continue
		}
		dir := t.TempDir()
		pan, err := tools.RunSpin(ctx, path, e.defines, dir)
		if err != nil {
			t.Errorf("%s: %v", e.file, err)
			continue
		}
		cmp := Compare(pan, eng)
		for _, r := range cmp.Rows {
			if (r.Name == "verdict" || r.Name == "error class") && !r.Agree {
				t.Errorf("%s: %s: pan %q, the engine with the reduction %q (%s)", e.file, r.Name, r.Pan, r.Engine, r.Note)
			}
		}
		compared++
	}
	t.Logf("%d corpus files: %d compared with pan with the reduction applied, %d refused before the limit", len(all), compared, refused)
}
