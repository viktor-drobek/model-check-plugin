package modelcheck_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"modelcheck/cli"
	"modelcheck/explore"
)

// TestPORAgreesWithTheFullSearchOnTheCorpus runs every Promela model of the
// fixtures and of the SPIN textbook corpus that the frontend accepts, in full
// and with the partial-order reduction, and requires the same status and
// evidence for every property and no more states in the reduced graph. It is
// the check on real protocols that the random models of explore/por_random_test
// cannot be: the termination-order guards, channels, never claims and
// d_steps of the models the engine is actually used on.
func TestPORAgreesWithTheFullSearchOnTheCorpus(t *testing.T) {
	var files []string
	// MCD_CORPUS_ALL_FILES=1 also tries the corpus files without a .pml suffix
	// (App_A/example, CH15/uts_model, ...; the frontend rejects what is not
	// Promela, as it does for the .pml files). The default stays the .pml files:
	// the floors below are the figures of that walk.
	everyFile := os.Getenv("MCD_CORPUS_ALL_FILES") != ""
	for _, root := range []string{"testdata/promela", "testdata/corpus2", corpusDir} {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && (strings.HasSuffix(path, ".pml") || (everyFile && root == corpusDir)) {
				files = append(files, path)
			}
			return nil
		})
	}
	sort.Strings(files)
	if len(files) < 50 {
		t.Skipf("only %d Promela files found", len(files))
	}
	var parsed, compared, applied, reduced, refused int
	reasons := map[string]int{}
	sizes := map[string][2]int{} // the models the reduction is expected to shrink: reduced, full
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, rej := cli.ParsePromela(src, f, nil, 0)
		if rej != nil {
			continue // outside the supported subset
		}
		parsed++
		opt := explore.Options{Sweep: true, Budget: explore.Budget{MaxStates: 200000}}
		run := func(por bool) *explore.Result {
			o := opt
			o.POR, o.Defines = por, p.Defines
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			r, err := explore.Run(ctx, p.Model, o)
			if err != nil {
				return nil
			}
			return r
		}
		full, red := run(false), run(true)
		if full == nil || red == nil {
			continue
		}
		// Whether an error is reachable is compared before the completeness
		// skip: an error ends the search and leaves it incomplete.
		if !strings.Contains(full.Stop, "budget") && !strings.Contains(red.Stop, "budget") {
			if (full.Stop == "invalid model") != (red.Stop == "invalid model") {
				t.Errorf("%s: an error is reachable with the reduction: %v, without: %v", f, red.Stop == "invalid model", full.Stop == "invalid model")
			}
		}
		if !full.Complete || !red.Complete {
			continue
		}
		compared++
		if !red.Reduction.Applied {
			refused++
			key := red.Reduction.Reason
			if i := strings.IndexAny(key, ":("); i > 0 {
				key = key[:i]
			}
			reasons[key]++
			if red.States != full.States {
				t.Errorf("%s: refused (%s) but %d states against %d", f, red.Reduction.Reason, red.States, full.States)
			}
		} else {
			applied++
			if red.States < full.States {
				reduced++
			}
		}
		sizes[filepath.ToSlash(f)] = [2]int{red.States, full.States}
		if red.States > full.States {
			t.Errorf("%s: the reduced graph has %d states, the full one %d", f, red.States, full.States)
		}
		for i := range full.Outcomes {
			a, b := red.Outcomes[i], full.Outcomes[i]
			if a.Status != b.Status || a.Evidence != b.Evidence {
				t.Errorf("%s: property %s is %s/%s with the reduction and %s/%s without (%s | %s)",
					f, a.Property.ID, a.Status, a.Evidence, b.Status, b.Evidence, a.Reason, b.Reason)
			}
		}
	}
	t.Logf("%d Promela files, %d accepted, %d compared in full and reduced: %d with the reduction applied (%d of them smaller), %d refused", len(files), parsed, compared, applied, reduced, refused)
	var keys []string
	for k := range reasons {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("  refused %3d× — %s", reasons[k], k)
	}
	if compared < 30 {
		t.Fatalf("only %d models compared: the corpus is not being read", compared)
	}
	// What the reduction must keep covering: these figures are what the walk
	// gives on the integrated tree, with atomic sequences, `run` and the process
	// table reduced (performance plan, step 6) and the fixtures of the `_nr_pr`,
	// weak-fairness, lasso and parallel branches in testdata/promela: 190 files,
	// 137 accepted, 120 compared, 75 applied, 35 of them smaller, 45 refused (11
	// rendezvous, 2 channels named by a value, 2 timeout, 5 provided, 25
	// temporal). They may only go up. (Step 6 alone, on its own tree: 90
	// compared, 63 applied, 29 smaller, 27 refused.) Adding a fixture to the
	// corpus changes the counts: raise the floors to the new numbers.
	// MCD_CORPUS_ALL_FILES=1 adds the corpus files without a .pml suffix: 248
	// files, 150 accepted, 131 compared, 78 applied, 35 smaller, 53 refused.
	if applied < 75 || reduced < 35 {
		t.Errorf("the reduction applies to %d models and shrinks %d: it applied to 75 and shrank 35 when this was last pinned", applied, reduced)
	}
	for _, f := range []string{"testdata/promela/bench-sym.pml", "testdata/promela/leader3.pml", "testdata/promela/nrpr.pml"} {
		got, ok := sizes[f]
		switch {
		case !ok:
			t.Errorf("%s was not compared in full and reduced", f)
		case got[0] >= got[1]:
			t.Errorf("%s: %d states reduced against %d in full: the reduction no longer shrinks it", f, got[0], got[1])
		}
	}
}
