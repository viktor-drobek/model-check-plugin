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
	for _, root := range []string{"testdata/promela", "testdata/corpus2", corpusDir} {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".pml") {
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
}
