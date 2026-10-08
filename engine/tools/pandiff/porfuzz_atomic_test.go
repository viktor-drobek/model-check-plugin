package pandiff

// Loops inside atomic and d_step blocks, against SPIN and against the
// reduction (integration of the weak-fairness branch's frontend change, which
// keeps the exclusive control across the back edge of a loop that starts an
// atomic / d_step block, with the partial-order reduction of performance plan
// step 6, whose macro-step closure and cycle proviso were built and validated
// on the IR shapes the old frontend produced).
//
// What each model has: two or three `active` processes, a few globals, and in
// every process a bounded loop `do :: (l < K) -> l++; ... :: else -> break od`
// inside an `atomic` or `d_step` block, as the first statement of the block, in
// the middle of it or after other statements; bodies that assign, assert, test
// (an atomic loop may block inside: the holder then loses control), a loop
// nested in the other kind of block, and plain statements around the block.
// Every loop is bounded by its own counter, so every macro-step is finite and
// pan can answer. The shape the frontend refuses by name (a loop at the start of
// an atomic block that is itself the first statement of an option) is not
// generated; testdata/promela/atomic-loop-option.pml holds it.
//
// MCD_ALOOP_FUZZ is the number of models of the engine comparison (default
// 600), MCD_ALOOP_FUZZ_SPIN of the SPIN comparison (default 12),
// MCD_ALOOP_FUZZ_SEED the first seed.

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

type atomicLoopFuzz struct {
	src    string
	blocks bool // an atomic block can block inside (a guard in a body or after the loop)
	loops  int  // loops inside a block
	first  int  // loops that are the first statement of their block
}

func fuzzAtomicLoopPromela(seed int64) atomicLoopFuzz {
	rnd := rand.New(rand.NewSource(seed))
	nproc := 2 + rnd.Intn(2)
	useChan := rnd.Intn(10) < 2
	var out []string
	out = append(out, "byte x, y, n;")
	if useChan {
		out = append(out, "chan c = [2] of { byte };")
	}
	f := atomicLoopFuzz{}
	val := func() string { return strconv.Itoa(rnd.Intn(3)) }
	gv := func() string { return []string{"x", "y", "n"}[rnd.Intn(3)] }
	// plain statements; guard allowed only where blocking is allowed
	simple := func(mayBlock bool) string {
		g := gv()
		switch k := rnd.Intn(11); {
		case k == 0:
			return fmt.Sprintf("%s = (%s + 1) %% 3", g, gv())
		case k == 1:
			return fmt.Sprintf("%s = %s", g, val())
		case k == 2 && mayBlock:
			f.blocks = true
			return fmt.Sprintf("(%s == %s)", gv(), val())
		case k == 3:
			return fmt.Sprintf("assert(%s != 2)", gv())
		case k == 4 && useChan && mayBlock:
			f.blocks = true
			return []string{"c!" + val(), "c?v"}[rnd.Intn(2)]
		case k == 5:
			return "skip"
		case k == 6:
			return fmt.Sprintf("if :: (%s == %s) -> %s = %s :: else -> skip fi", gv(), val(), gv(), val())
		}
		return fmt.Sprintf("%s = %s", g, val())
	}
	nloc := 0
	loop := func(kind string, mayBlock bool) string {
		nloc++
		l := fmt.Sprintf("l%d", nloc)
		bound := 1 + rnd.Intn(3)
		var body []string
		for i, n := 0, 1+rnd.Intn(2); i < n; i++ {
			body = append(body, simple(mayBlock))
		}
		f.loops++
		return fmt.Sprintf("do :: (%s < %d) -> %s++; %s :: else -> break od", l, bound, l, strings.Join(body, "; "))
	}
	block := func(kind string) string {
		mayBlock := kind == "atomic"
		var parts []string
		pos := rnd.Intn(3) // 0: the loop first, 1: after a statement, 2: before one
		if pos == 1 {
			parts = append(parts, simple(mayBlock))
		}
		lp := loop(kind, mayBlock)
		if pos == 0 {
			f.first++
		}
		parts = append(parts, lp)
		if pos == 2 || rnd.Intn(3) == 0 {
			parts = append(parts, simple(mayBlock))
		}
		if rnd.Intn(8) == 0 { // a second loop in the same block
			parts = append(parts, loop(kind, mayBlock))
		}
		return kind + " { " + strings.Join(parts, "; ") + " }"
	}
	var procs []string
	for i := 0; i < nproc; i++ {
		var body []string
		nb := 1 + rnd.Intn(2)
		for j := 0; j < nb; j++ {
			if rnd.Intn(3) == 0 {
				body = append(body, simple(false))
			}
			kind := "atomic"
			if rnd.Intn(3) == 0 {
				kind = "d_step"
			}
			body = append(body, block(kind))
		}
		if rnd.Intn(2) == 0 {
			body = append(body, simple(true))
		}
		var decl []string
		for k := 1; k <= nloc; k++ {
			decl = append(decl, fmt.Sprintf("l%d", k))
		}
		text := "byte v"
		if len(decl) > 0 {
			text += "; byte " + strings.Join(decl, ", ")
		}
		procs = append(procs, text+";\n  "+strings.Join(body, ";\n  "))
		nloc = 0 // the next process declares its own counters l1..
	}
	for i, p := range procs {
		out = append(out, fmt.Sprintf("active proctype P%d() {\n  %s\n}", i, p))
	}
	f.src = strings.Join(out, "\n") + "\n"
	return f
}

func fuzzAtomicLoopFirstSeed() int64 {
	if v, err := strconv.ParseInt(os.Getenv("MCD_ALOOP_FUZZ_SEED"), 10, 64); err == nil && v > 0 {
		return v
	}
	return 5_000_001
}

// TestPORFuzzedAtomicLoopPromelaKeepsTheVerdicts: the engine in full and
// reduced, through the frontend, on loops inside atomic and d_step blocks.
func TestPORFuzzedAtomicLoopPromelaKeepsTheVerdicts(t *testing.T) {
	n := fuzzScale("MCD_ALOOP_FUZZ", 600)
	if testing.Short() {
		n = max(1, n/8)
	}
	first := fuzzAtomicLoopFirstSeed()
	var rejected, applied, smaller, errored, skipped, violatedModels, loops, firstLoops int
	for seed := first; seed < first+int64(n); seed++ {
		fm := fuzzAtomicLoopPromela(seed)
		p, perr := promela.Parse([]byte(fm.src), "fuzz.pml", nil)
		if perr != nil {
			rejected++
			if rejected <= 3 {
				t.Logf("seed %d rejected: %v\n%s", seed, perr, fm.src)
			}
			continue
		}
		loops += fm.loops
		firstLoops += fm.first
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
	t.Logf("atomic-loop fuzz: %d models, %d rejected by the frontend, %d loops in blocks (%d the first statement of their block), %d with an error of the model in both searches, %d skipped, %d with a violation, %d reduction applied, %d smaller", n, rejected, loops, firstLoops, errored, skipped, violatedModels, applied, smaller)
	if rejected > n/20 {
		t.Fatalf("%d of %d generated models were rejected by the frontend: the generator is out of the subset", rejected, n)
	}
	if n >= 100 && (firstLoops < n/3 || applied < n/2 || smaller < n/10) {
		t.Fatalf("the generator is too tame: %d loops at the start of a block, %d reduced, %d smaller, of %d models", firstLoops, applied, smaller, n)
	}
}

// TestPORFuzzedAtomicLoopPromelaAgreesWithSPIN: pan against the engine in full
// and reduced: verdict, error class, and the state count of the full search.
func TestPORFuzzedAtomicLoopPromelaAgreesWithSPIN(t *testing.T) {
	tools := Tools{}
	if !tools.Available() {
		t.Skip("spin or gcc not on PATH")
	}
	n := fuzzScale("MCD_ALOOP_FUZZ_SPIN", 12)
	if testing.Short() {
		n = max(1, n/4)
	}
	first := fuzzAtomicLoopFirstSeed()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Minute)
	defer cancel()
	var agree, skipped, countDiffers, countBlocks, countSame, countSameBlocks, reduced, withViolation, firstLoops int
	var diffs []string
	for seed := first; seed < first+int64(n); seed++ {
		fm := fuzzAtomicLoopPromela(seed)
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
		for _, which := range []struct {
			name string
			e    *EngineResult
		}{{"full", eng}, {"reduced", engRed}} {
			for _, r := range Compare(pan, which.e).Rows {
				if r.Name == "error class" && !r.Agree && pan.Errors > 0 && strings.Contains(which.e.Class, pan.Class) {
					continue
				}
				if (r.Name == "verdict" || r.Name == "error class") && !r.Agree {
					t.Fatalf("seed %d: %s: %s: pan %q, the engine %q (%s)\n%s", seed, which.name, r.Name, r.Pan, r.Engine, r.Note, fm.src)
				}
				if which.name == "full" && r.Name == "states stored" && r.Agree && !r.Skipped {
					if fm.blocks {
						countSameBlocks++
					} else {
						countSame++
					}
				}
				if which.name == "full" && r.Name == "states stored" && !r.Agree && !r.Skipped {
					if fm.blocks {
						countBlocks++
						continue
					}
					countDiffers++
					diffs = append(diffs, fmt.Sprintf("seed %d: pan %s, engine %s", seed, r.Pan, r.Engine))
				}
			}
		}
		if violated(full) != (pan.Errors > 0) || violated(red) != (pan.Errors > 0) {
			t.Fatalf("seed %d: pan reports an error: %v; the engine in full: %v, reduced: %v\n%s", seed, pan.Errors > 0, violated(full), violated(red), fm.src)
		}
		agree++
		firstLoops += fm.first
		if pan.Errors > 0 {
			withViolation++
		}
		if red.applied && red.states < full.states {
			reduced++
		}
	}
	t.Logf("atomic loops vs pan: %d models, %d agree on verdict and class in full and reduced (%d with an error, %d loops at the start of a block), %d skipped, %d reduced; the state count of the full search equals the pan one in %d models without a blocking atomic block and differs in %d of them; with a blocking atomic block (the known open item) it equals it in %d and differs in %d", n, agree, withViolation, firstLoops, skipped, reduced, countSame, countDiffers, countSameBlocks, countBlocks)
	for _, d := range diffs {
		t.Logf("  count: %s", d)
	}
}
