package pandiff

// Models that read `_nr_pr` and start their processes with `active`, against
// SPIN and against the reduction (integration of the `_nr_pr` fix, which makes
// the count fall in such a model as it does in pan, with the partial-order
// reduction of performance plan step 6, whose oracles had only generated table
// models through `run`). porfuzz_test.go left these models out of its SPIN
// comparison because the engine's `_nr_pr` did not fall in them.
//
// What each model has: two to four `active` processes (sometimes with an `init`
// that starts one more by `run`), a tiny body each, with guards, asserts and
// `if` choices on `_nr_pr` (equal, below, at least, not equal), blocking
// guards that wait for the count to fall, `atomic` and `d_step` blocks with such
// guards inside, a bounded loop and a buffered channel. Bodies of different
// length make the processes end in every order SPIN allows.
//
// MCD_NRPR_FUZZ is the number of models of the engine comparison (default 600),
// MCD_NRPR_FUZZ_SPIN of the SPIN comparison (default 12), MCD_NRPR_FUZZ_SEED
// the first seed.

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

	"modelcheck/frontend/promela"
)

type nrPrFuzz struct {
	src   string
	byRun bool // an init starts one process by run
	// blocks: an atomic block whose second or later statement can block (a
	// guard, a channel operation). pan lets the other processes run while the
	// holder is blocked, and the engine stores that state with its exclusive-control
	// byte set: the state counts differ, with the same verdicts (the
	// known open item "a blocking atomic: 5 states against pan's 4" of the record
	// of the cycle-lasso fix).
	blocks bool
}

func fuzzNrPrPromela(seed int64) nrPrFuzz {
	rnd := rand.New(rand.NewSource(seed))
	nproc := 2 + rnd.Intn(3)
	useChan := rnd.Intn(10) < 3
	byRun := rnd.Intn(5) == 0
	var out []string
	out = append(out, "byte x, y;")
	if useChan {
		out = append(out, fmt.Sprintf("chan c = [%d] of { byte };", 1+rnd.Intn(2)))
	}
	f := nrPrFuzz{byRun: byRun}
	val := func() string { return strconv.Itoa(rnd.Intn(3)) }
	gv := func() string { return []string{"x", "y"}[rnd.Intn(2)] }
	cmpNr := func() string {
		return fmt.Sprintf("_nr_pr %s %d", []string{"==", "<", ">=", "!=", ">", "<="}[rnd.Intn(6)], rnd.Intn(5))
	}
	simple := func() string {
		g := gv()
		switch k := rnd.Intn(12); {
		case k < 3:
			return fmt.Sprintf("(%s)", cmpNr()) // a guard on the count: may block
		case k == 3:
			return fmt.Sprintf("%s = (%s + 1) %% 3", g, gv())
		case k == 4:
			return fmt.Sprintf("%s = _nr_pr", g)
		case k == 5:
			return fmt.Sprintf("assert(%s)", cmpNr())
		case k == 6:
			return fmt.Sprintf("(%s == %s)", gv(), val())
		case k == 7 && useChan:
			return []string{"c!" + val(), "c?v"}[rnd.Intn(2)]
		case k == 8:
			return "skip"
		}
		return fmt.Sprintf("%s = %s", g, val())
	}
	var stmt func(depth int) string
	stmt = func(depth int) string {
		switch k := rnd.Intn(12); {
		case k < 5:
			return simple()
		case k == 5:
			var parts []string
			for i, n := 0, 1+rnd.Intn(3); i < n; i++ {
				parts = append(parts, simple())
			}
			s := "atomic { " + strings.Join(parts, "; ") + " }"
			for _, p := range parts[1:] {
				if strings.HasPrefix(p, "(") || strings.HasPrefix(p, "c!") || strings.HasPrefix(p, "c?") {
					f.blocks = true // a statement that can block, after the first one of the block
				}
			}
			return s
		case k == 6:
			return fmt.Sprintf("d_step { %s = %s; %s = %s }", gv(), val(), gv(), val())
		case k == 7 && depth < 1:
			return fmt.Sprintf("if :: (%s) -> %s :: else -> %s fi", cmpNr(), stmt(depth+1), stmt(depth+1))
		case k == 8 && depth < 1:
			return fmt.Sprintf("if :: %s :: %s fi", simple(), simple())
		}
		return simple()
	}
	var procs []string
	for i := 0; i < nproc; i++ {
		var body []string
		for j, n := 0, 1+rnd.Intn(4); j < n; j++ {
			body = append(body, stmt(0))
		}
		text := "byte l; byte v;\n  " + strings.Join(body, ";\n  ")
		if rnd.Intn(10) < 2 {
			first := body[:1]
			rest := body[1:]
			if len(rest) == 0 {
				rest = []string{"skip"}
			}
			text = "byte l; byte v;\n  do\n  :: (l < 2) -> l++; " + strings.Join(first, "; ") + "\n  :: else -> break\n  od;\n  " + strings.Join(rest, ";\n  ")
		}
		procs = append(procs, text)
	}
	for i, p := range procs {
		if byRun && i == nproc-1 {
			out = append(out, fmt.Sprintf("proctype Q() {\n  %s\n}", p))
			out = append(out, "init { run Q() }")
			continue
		}
		out = append(out, fmt.Sprintf("active proctype P%d() {\n  %s\n}", i, p))
	}
	f.src = strings.Join(out, "\n") + "\n"
	return f
}

// TestPORFuzzedNrPrPromelaKeepsTheVerdicts: the engine in full and reduced, on
// models that read _nr_pr and have no run (or one), through the frontend.
func TestPORFuzzedNrPrPromelaKeepsTheVerdicts(t *testing.T) {
	n := fuzzScale("MCD_NRPR_FUZZ", 600)
	if testing.Short() {
		n = max(1, n/8)
	}
	first := fuzzNrPrFirstSeed()
	var parsed, rejected, applied, smaller, errored, skipped, violatedModels, reads int
	for seed := first; seed < first+int64(n); seed++ {
		fm := fuzzNrPrPromela(seed)
		p, perr := promela.Parse([]byte(fm.src), "fuzz.pml", nil)
		if perr != nil {
			rejected++
			continue
		}
		parsed++
		if strings.Contains(fm.src, "_nr_pr") {
			reads++
		}
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
		if violated(full) {
			violatedModels++
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
	t.Logf("nrpr fuzz: %d models, %d rejected by the frontend, %d read _nr_pr, %d with an error of the model in both searches, %d skipped, %d with a violation, %d reduction applied, %d smaller", n, rejected, reads, errored, skipped, violatedModels, applied, smaller)
	if rejected > n/10 {
		t.Fatalf("%d of %d generated models were rejected by the frontend: the generator is out of the subset", rejected, n)
	}
	if n >= 100 && (violatedModels < n/10 || applied < n/2) {
		t.Fatalf("the generator is too tame: %d models with a violation, %d reduced, of %d", violatedModels, applied, n)
	}
}

func fuzzNrPrFirstSeed() int64 {
	if v, err := strconv.ParseInt(os.Getenv("MCD_NRPR_FUZZ_SEED"), 10, 64); err == nil && v > 0 {
		return v
	}
	return 3_000_001
}

// TestPORFuzzedNrPrPromelaAgreesWithSPIN: pan (no reduction of its own) against
// the engine in full and reduced, on the verdict, the error class and, for the
// full search, the number of stored states (pan -c0).
func TestPORFuzzedNrPrPromelaAgreesWithSPIN(t *testing.T) {
	tools := Tools{}
	if !tools.Available() {
		t.Skip("spin or gcc not on PATH")
	}
	n := fuzzScale("MCD_NRPR_FUZZ_SPIN", 12)
	if testing.Short() {
		n = max(1, n/4)
	}
	first := fuzzNrPrFirstSeed()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer cancel()
	var agree, skipped, countDiffers, countBlocks, countSame, countSameBlocks, reduced, withViolation int
	var diffs []string
	for seed := first; seed < first+int64(n); seed++ {
		fm := fuzzNrPrPromela(seed)
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
		path := filepath.Join(dir, "m.pml")
		if err := os.WriteFile(path, []byte(fm.src), 0o644); err != nil {
			t.Fatal(err)
		}
		pan, err := tools.RunSpin(ctx, path, nil, dir)
		if err != nil || pan.Incomplete {
			skipped++
			continue
		}
		eng, err := RunEngine(ctx, path, nil, 0)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		engRed, err := RunEngineReduced(ctx, path, nil, 0)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		for _, r := range Compare(pan, eng).Rows {
			// pan stops at the first error it finds and names that class; the
			// engine lists every kind of error the model has ("invalid end state +
			// assertion violated"), so the class is compared as "pan's class is one
			// of the engine's".
			if r.Name == "error class" && !r.Agree && pan.Errors > 0 && strings.Contains(eng.Class, pan.Class) {
				continue
			}
			if (r.Name == "verdict" || r.Name == "error class") && !r.Agree {
				t.Fatalf("seed %d: %s: pan %q, the engine %q (%s)\n%s", seed, r.Name, r.Pan, r.Engine, r.Note, fm.src)
			}
			if r.Name == "states stored" && r.Agree && !r.Skipped {
				if fm.blocks {
					countSameBlocks++
				} else {
					countSame++
				}
			}
			if r.Name == "states stored" && !r.Agree && !r.Skipped {
				if fm.blocks {
					countBlocks++
					continue
				}
				countDiffers++
				diffs = append(diffs, fmt.Sprintf("seed %d: pan %s, engine %s", seed, r.Pan, r.Engine))
			}
		}
		for _, r := range Compare(pan, engRed).Rows {
			if r.Name == "error class" && !r.Agree && pan.Errors > 0 && strings.Contains(engRed.Class, pan.Class) {
				continue
			}
			if (r.Name == "verdict" || r.Name == "error class") && !r.Agree {
				t.Fatalf("seed %d: %s: pan %q, the engine with the reduction %q (%s)\n%s", seed, r.Name, r.Pan, r.Engine, r.Note, fm.src)
			}
		}
		if violated(full) != (pan.Errors > 0) || violated(red) != (pan.Errors > 0) {
			t.Fatalf("seed %d: pan reports an error: %v; the engine in full: %v, reduced: %v\n%s", seed, pan.Errors > 0, violated(full), violated(red), fm.src)
		}
		agree++
		if pan.Errors > 0 {
			withViolation++
		}
		if red.applied && red.states < full.states {
			reduced++
		}
	}
	t.Logf("nrpr vs pan: %d models, %d agree on verdict and class in full and reduced (%d with an error), %d skipped, %d reduced; the state count of the full search equals the pan one in %d models without a blocking atomic block and differs in %d of them; with a blocking atomic block (the known open item) it equals it in %d and differs in %d", n, agree, withViolation, skipped, reduced, countSame, countDiffers, countSameBlocks, countBlocks)
	for _, d := range diffs {
		t.Logf("  count: %s", d)
	}
}

// TestPORNrPrFixturesAgreeWithSPIN: the fixtures of the `_nr_pr` fix and the
// `run` model of step 6 that reads the count, against pan, in full and with
// the reduction. The full search agrees with pan on verdict, error class and
// state count (the fix's own claim); the reduced search on verdict and class.
func TestPORNrPrFixturesAgreeWithSPIN(t *testing.T) {
	tools := Tools{}
	if !tools.Available() {
		t.Skip("spin or gcc not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	for _, f := range []string{"nrpr-active.pml", "nrpr-order.pml", "nrpr-youngest.pml", "nrpr-mixed.pml", "nrpr-unread.pml", "nrpr.pml"} {
		t.Run(f, func(t *testing.T) {
			path := filepath.Join("..", "..", "testdata", "promela", f)
			dir := t.TempDir()
			pan, err := tools.RunSpin(ctx, path, nil, dir)
			if err != nil {
				t.Fatal(err)
			}
			full, err := RunEngine(ctx, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			red, err := RunEngineReduced(ctx, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range Compare(pan, full).Rows {
				if (r.Name == "verdict" || r.Name == "error class" || r.Name == "states stored") && !r.Agree {
					t.Errorf("full: %s: pan %q, engine %q", r.Name, r.Pan, r.Engine)
				}
			}
			for _, r := range Compare(pan, red).Rows {
				if (r.Name == "verdict" || r.Name == "error class") && !r.Agree {
					t.Errorf("reduced: %s: pan %q, engine %q", r.Name, r.Pan, r.Engine)
				}
			}
			applied := red.Reduction != nil && red.Reduction.Applied
			t.Logf("%s: pan %d states (%s); engine full %d, reduced %d (applied %v)", f, pan.Stored, pan.Class, full.States, red.States, applied)
		})
	}
}
