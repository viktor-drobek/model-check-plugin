package modelcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/cli"
	"modelcheck/tools/mutate"
	"modelcheck/tools/pandiff"
)

// k3World is the per-scenario state for features/k3-mutation.feature.
//
// The mutation steps call the mutator's package directly (it is this step's
// own code). The verdict steps go through mutate.CompareOne, which runs the
// engine as the `mcd` BINARY and pan through tools/pandiff — the same path
// the campaign uses, so that a scenario cannot pass by a route the
// measurement does not take.
type k3World struct {
	source   string // path of the corpus model under test
	muts     []mutate.Mutant
	dir      string // where the mutants were written
	manifest []byte
	second   []byte

	base    mutate.CheckResult
	mutant  mutate.CheckResult
	mutIdx  int
	class   string
	listing string // corpus2 listing path

	stdout bytes.Buffer
	stderr bytes.Buffer
	exit   int
}

func init() {
	stepRegistrars = append(stepRegistrars, registerK3Steps)
}

const (
	k3Corpus2 = "testdata/corpus2"
	k3Steps   = "../steps"
)

// k3MCD builds the engine's CLI once per test binary and returns its path.
// The feature must exercise the same binary the campaign used; building it
// here (rather than calling cli.Run in-process) keeps that true.
var (
	k3MCDOnce sync.Once
	k3MCDPath string
	k3MCDErr  error
)

func k3mcd() (string, error) {
	k3MCDOnce.Do(func() {
		dir, err := os.MkdirTemp("", "k3-mcd-")
		if err != nil {
			k3MCDErr = err
			return
		}
		out := filepath.Join(dir, "mcd")
		cmd := exec.Command("go", "build", "-o", out, "./cmd/mcd")
		if b, err := cmd.CombinedOutput(); err != nil {
			k3MCDErr = fmt.Errorf("go build ./cmd/mcd: %v: %s", err, b)
			return
		}
		k3MCDPath = out
	})
	return k3MCDPath, k3MCDErr
}

func registerK3Steps(sc *godog.ScenarioContext) {
	w := &k3World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = k3World{}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.dir != "" {
			os.RemoveAll(w.dir)
		}
		return ctx, nil
	})

	// --- Given ---------------------------------------------------------------

	sc.Step(`^the K3 mutator built from "([^"]*)"$`, func(pkg string) error {
		// The mutator is linked into this test binary; the step records which
		// package the scenarios are about and fails if it has moved.
		if _, err := os.Stat(filepath.Join("..", pkg)); err != nil {
			return fmt.Errorf("the K3 mutator is expected in %s: %w", pkg, err)
		}
		if len(mutate.Operators) != 10 {
			return fmt.Errorf("the feature describes 10 operators, the mutator has %d", len(mutate.Operators))
		}
		return nil
	})

	sc.Step(`^the K3 corpus model "([^"]*)"$`, func(name string) error {
		p := filepath.Join(corpusDir, name)
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.source = p
		return nil
	})

	sc.Step(`^spin and gcc are available for K3$`, func() error {
		if !(pandiff.Tools{}).Available() {
			return godog.ErrSkip
		}
		if _, err := k3mcd(); err != nil {
			return godog.ErrSkip
		}
		return nil
	})

	sc.Step(`^the corpus2 listing "([^"]*)"$`, func(name string) error {
		p := filepath.Join(k3Corpus2, name)
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.listing = p
		return nil
	})

	sc.Step(`^the corpus2 directory "([^"]*)"$`, func(dir string) error {
		p := strings.TrimPrefix(dir, "engine/")
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.listing = p
		return nil
	})

	sc.Step(`^the K3 mutation results "([^"]*)"$`, func(path string) error {
		p := filepath.Join("..", path)
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.listing = p
		return nil
	})

	// --- When ----------------------------------------------------------------

	generate := func(ops ...mutate.Operator) error {
		src, err := os.ReadFile(w.source)
		if err != nil {
			return err
		}
		w.muts = mutate.Generate(src, w.source, ops...)
		dir, err := os.MkdirTemp("", "k3-mutants-")
		if err != nil {
			return err
		}
		w.dir = dir
		mp, err := mutate.WriteAll(dir, w.muts)
		if err != nil {
			return err
		}
		w.manifest, err = os.ReadFile(mp)
		return err
	}

	sc.Step(`^I generate the K3 mutants with the operator "([^"]*)"$`, func(name string) error {
		op, err := mutate.ParseOperator(name)
		if err != nil {
			return err
		}
		return generate(op)
	})

	sc.Step(`^I generate all K3 mutants$`, func() error { return generate() })

	sc.Step(`^I generate all K3 mutants twice$`, func() error {
		if err := generate(); err != nil {
			return err
		}
		first := w.manifest
		dir := w.dir
		if err := generate(); err != nil {
			return err
		}
		os.RemoveAll(dir)
		w.second = w.manifest
		w.manifest = first
		return nil
	})

	sc.Step(`^I run "mcd parse" on the corpus2 listing$`, func() error {
		w.stdout.Reset()
		w.stderr.Reset()
		w.exit = cli.Run([]string{"parse", "--promela", w.listing}, &w.stdout, &w.stderr)
		return nil
	})

	sc.Step(`^I compare the engine with pan on the original and on K3 mutant (\d+)$`, func(n int) error {
		if n < 1 || n > len(w.muts) {
			return fmt.Errorf("mutant %d of %d", n, len(w.muts))
		}
		mcd, err := k3mcd()
		if err != nil {
			return err
		}
		w.mutIdx = n
		opts := mutate.CampaignOptions{MCD: mcd, TimeoutSec: 180}
		check := mutate.Check{Name: "safety"}
		w.base = mutate.CompareOne(context.Background(), opts, w.source, nil, check)
		w.mutant = mutate.CompareOne(context.Background(), opts, filepath.Join(w.dir, w.muts[n-1].FileName()), nil, check)
		w.class = mutate.Classify(w.base, w.mutant)
		return nil
	})

	// --- Then: generation ----------------------------------------------------

	sc.Step(`^at least (\d+) mutant is generated$`, func(n int) error {
		if len(w.muts) < n {
			return fmt.Errorf("%d mutants of %s, want at least %d", len(w.muts), w.source, n)
		}
		return nil
	})

	sc.Step(`^(\d+) mutants are generated$`, func(n int) error {
		if len(w.muts) != n {
			return fmt.Errorf("%d mutants of %s, want %d", len(w.muts), w.source, n)
		}
		return nil
	})

	sc.Step(`^every K3 mutant parses with the engine's Promela frontend$`, func() error {
		for _, m := range w.muts {
			var out, errb bytes.Buffer
			p := filepath.Join(w.dir, m.FileName())
			if code := cli.Run([]string{"parse", "--promela", p}, &out, &errb); code != 0 {
				return fmt.Errorf("%s (%s at %d:%d, %q → %q): exit %d, %s",
					m.FileName(), m.Operator, m.Line, m.Col, m.Original, m.Mutated, code, strings.TrimSpace(out.String()))
			}
		}
		return nil
	})

	sc.Step(`^every K3 manifest entry has the operator "([^"]*)", a line, a column, an original text and a mutated text$`, func(op string) error {
		entries, err := k3entries(w.manifest)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return fmt.Errorf("the manifest is empty")
		}
		for _, e := range entries {
			switch {
			case e.Operator != op:
				return fmt.Errorf("entry %d has operator %q, want %q", e.ID, e.Operator, op)
			case e.Line == 0 || e.Col == 0:
				return fmt.Errorf("entry %d has no position", e.ID)
			case e.Original == "":
				return fmt.Errorf("entry %d has no original text", e.ID)
			case e.File == "" || e.Source == "":
				return fmt.Errorf("entry %d does not name its files", e.ID)
			}
			// A deletion has an empty replacement; the tables write it
			// "(removed)", and that is the only case where Mutated is empty.
			if e.Mutated == "" && !k3deletes(e.Operator) {
				return fmt.Errorf("entry %d (%s) has no mutated text", e.ID, e.Operator)
			}
		}
		return nil
	})

	sc.Step(`^the K3 manifest entry (\d+) replaces "([^"]*)" by "([^"]*)"$`, func(n int, original, mutated string) error {
		entries, err := k3entries(w.manifest)
		if err != nil {
			return err
		}
		if n < 1 || n > len(entries) {
			return fmt.Errorf("entry %d of %d", n, len(entries))
		}
		e := entries[n-1]
		want := mutated
		if want == "(removed)" {
			want = ""
		}
		if e.Original != original || e.Mutated != want {
			return fmt.Errorf("entry %d replaces %q by %q, want %q by %q", n, e.Original, e.Mutated, original, want)
		}
		return nil
	})

	sc.Step(`^the two K3 manifests are byte-identical$`, func() error {
		if !bytes.Equal(w.manifest, w.second) {
			return fmt.Errorf("the two manifests differ")
		}
		return nil
	})

	sc.Step(`^the K3 manifest entries are ordered by operator rank then by line and column$`, func() error {
		entries, err := k3entries(w.manifest)
		if err != nil {
			return err
		}
		rank := map[string]int{}
		for i, op := range mutate.Operators {
			rank[string(op)] = i
		}
		for i := 1; i < len(entries); i++ {
			p, c := entries[i-1], entries[i]
			if c.ID != p.ID+1 {
				return fmt.Errorf("entry %d has id %d after %d", i+1, c.ID, p.ID)
			}
			switch {
			case rank[c.Operator] < rank[p.Operator]:
				return fmt.Errorf("%s came after %s", c.Operator, p.Operator)
			case rank[c.Operator] == rank[p.Operator] && (c.Line < p.Line || (c.Line == p.Line && c.Col < p.Col)):
				return fmt.Errorf("%s at %d:%d came after %d:%d", c.Operator, c.Line, c.Col, p.Line, p.Col)
			}
		}
		return nil
	})

	sc.Step(`^spin -a accepts every K3 mutant$`, func() error {
		tools := pandiff.Tools{}
		for _, m := range w.muts {
			dir, err := os.MkdirTemp("", "k3-spin-")
			if err != nil {
				return err
			}
			src, err := os.ReadFile(filepath.Join(w.dir, m.FileName()))
			if err != nil {
				os.RemoveAll(dir)
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, "m.pml"), src, 0o644); err != nil {
				os.RemoveAll(dir)
				return err
			}
			cmd := exec.Command("spin", "-a", "-o1", "-o2", "-o3", "m.pml")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			os.RemoveAll(dir)
			_ = tools
			if err != nil || bytes.Contains(out, []byte("Error")) {
				return fmt.Errorf("spin rejects %s (%s at %d:%d): %s", m.FileName(), m.Operator, m.Line, m.Col, strings.TrimSpace(string(out)))
			}
		}
		return nil
	})

	// --- Then: verdicts ------------------------------------------------------

	sc.Step(`^the engine verdict of the original is "([^"]*)" and of the mutant "([^"]*)"$`, func(orig, mut string) error {
		if w.base.Engine != orig || w.mutant.Engine != mut {
			return fmt.Errorf("engine: original %q, mutant %q; want %q and %q (notes: %q / %q)",
				w.base.Engine, w.mutant.Engine, orig, mut, w.base.Note, w.mutant.Note)
		}
		return nil
	})

	sc.Step(`^the pan verdict of the original is "([^"]*)" and of the mutant "([^"]*)"$`, func(orig, mut string) error {
		got, want := k3pan(w.base.Pan), orig
		if got != want {
			return fmt.Errorf("pan on the original: %q, want %q", got, want)
		}
		if got, want := k3pan(w.mutant.Pan), mut; got != want {
			return fmt.Errorf("pan on the mutant: %q, want %q", got, want)
		}
		return nil
	})

	sc.Step(`^the K3 mutant is classified "([^"]*)"$`, func(class string) error {
		if !strings.HasPrefix(w.class, class) && !strings.Contains(w.class, class) {
			return fmt.Errorf("classified %q, want %q (engine %s→%s, pan %s→%s)",
				w.class, class, w.base.Engine, w.mutant.Engine, w.base.Pan, w.mutant.Pan)
		}
		return nil
	})

	sc.Step(`^the state count of the original is (\d+) for both the engine and pan$`, func(n int) error {
		if w.base.EngineStates != n || w.base.PanStates != n {
			return fmt.Errorf("original: engine %d, pan %d, want %d for both", w.base.EngineStates, w.base.PanStates, n)
		}
		return nil
	})

	sc.Step(`^the state count of the mutant is larger than (\d+) and equal for the engine and pan$`, func(n int) error {
		if w.mutant.EngineStates <= n {
			return fmt.Errorf("the mutant's engine state count is %d, not larger than %d", w.mutant.EngineStates, n)
		}
		if w.mutant.EngineStates != w.mutant.PanStates {
			return fmt.Errorf("mutant: engine %d states, pan %d", w.mutant.EngineStates, w.mutant.PanStates)
		}
		return nil
	})

	// --- Then: corpus2 -------------------------------------------------------

	sc.Step(`^it is rejected with kind "([^"]*)" and status "([^"]*)"$`, func(kind, status string) error {
		if w.exit != 2 {
			return fmt.Errorf("exit %d, want 2 (a rejection); stdout %s", w.exit, strings.TrimSpace(w.stdout.String()))
		}
		var rej struct {
			Error struct {
				Kind, Status, Message string
			} `json:"error"`
		}
		if err := json.Unmarshal(w.stdout.Bytes(), &rej); err != nil {
			return err
		}
		if rej.Error.Kind != kind || rej.Error.Status != status {
			return fmt.Errorf("kind %q status %q, want %q and %q", rej.Error.Kind, rej.Error.Status, kind, status)
		}
		return nil
	})

	sc.Step(`^the K3 rejection message mentions "([^"]*)"$`, func(text string) error {
		if !strings.Contains(w.stdout.String(), text) {
			return fmt.Errorf("the rejection does not mention %q: %s", text, strings.TrimSpace(w.stdout.String()))
		}
		return nil
	})

	sc.Step(`^spin -a rejects the corpus2 listing with a syntax error$`, func() error {
		dir, err := os.MkdirTemp("", "k3-spin-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		src, err := os.ReadFile(w.listing)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "m.pml"), src, 0o644); err != nil {
			return err
		}
		cmd := exec.Command("spin", "-a", "-o1", "-o2", "-o3", "m.pml")
		cmd.Dir = dir
		out, _ := cmd.CombinedOutput()
		if !bytes.Contains(out, []byte("syntax error")) {
			return fmt.Errorf("spin did not report a syntax error: %s", strings.TrimSpace(string(out)))
		}
		return nil
	})

	sc.Step(`^every "([^"]*)" file under it has a row in its README table$`, func(ext string) error {
		rows, err := k3readme(w.listing)
		if err != nil {
			return err
		}
		files, err := k3listings(w.listing, ext)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return fmt.Errorf("no %s file under %s", ext, w.listing)
		}
		for _, f := range files {
			if _, ok := rows[f]; !ok {
				return fmt.Errorf("%s has no row in the README table", f)
			}
		}
		for f := range rows {
			found := false
			for _, g := range files {
				if f == g {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("the README has a row for %s, which does not exist", f)
			}
		}
		return nil
	})

	sc.Step(`^every row names an engine outcome that is one of (.+)$`, func(list string) error {
		return k3column(w.listing, 3, list)
	})

	sc.Step(`^every row names a pan outcome that is one of (.+)$`, func(list string) error {
		return k3column(w.listing, 4, list)
	})

	sc.Step(`^every listing starts with a header comment citing its source lines$`, func() error {
		files, err := k3listings(w.listing, ".pml")
		if err != nil {
			return err
		}
		cite := regexp.MustCompile(`\* Source: \S+, lines \d+`)
		for _, f := range files {
			src, err := os.ReadFile(filepath.Join(w.listing, f))
			if err != nil {
				return err
			}
			head := src
			if len(head) > 600 {
				head = head[:600]
			}
			if !bytes.HasPrefix(src, []byte("/*")) || !cite.Match(head) {
				return fmt.Errorf("%s does not start with a header comment citing its source lines", f)
			}
		}
		return nil
	})

	// --- Then: the report ----------------------------------------------------

	sc.Step(`^every mutant of class "([^"]*)" records a mutant path, the engine verdict and the pan verdict$`, func(class string) error {
		res, err := k3results(w.listing)
		if err != nil {
			return err
		}
		n := 0
		for _, m := range res.Models {
			for _, mu := range m.Mutants {
				if !strings.HasPrefix(mu.Class, class) {
					continue
				}
				n++
				if mu.File == "" {
					return fmt.Errorf("a %s mutant of %s has no file", class, m.Model)
				}
				for _, c := range mu.Checks {
					if c.Class == mutate.ClassDisagree && (c.Engine == "" || c.Pan == "") {
						return fmt.Errorf("%s: a disagreeing check records no verdict pair", mu.File)
					}
				}
			}
		}
		if n != res.Counts.Disagreement {
			return fmt.Errorf("%d mutants of class %s, the counts say %d", n, class, res.Counts.Disagreement)
		}
		// Every disagreement must also be in the top-level list, with both
		// verdicts: that list is what a bug report is cut from.
		for _, d := range res.Disagreements {
			if d.Mutant == "" || d.Engine == "" || d.Pan == "" {
				return fmt.Errorf("incomplete disagreement record: %+v", d)
			}
		}
		return nil
	})

	sc.Step(`^every mutant of class "([^"]*)" records a changed verdict for both the engine and pan$`, func(class string) error {
		res, err := k3results(w.listing)
		if err != nil {
			return err
		}
		for _, m := range res.Models {
			for _, mu := range m.Mutants {
				if !strings.HasPrefix(mu.Class, class) {
					continue
				}
				moved := false
				for i, c := range mu.Checks {
					if c.Class != mutate.ClassDetected {
						continue
					}
					b := m.Baseline[i]
					if c.Engine == b.Engine || c.Pan == b.Pan {
						return fmt.Errorf("%s: class (i) but a verdict did not move (engine %s→%s, pan %s→%s)",
							mu.File, b.Engine, c.Engine, b.Pan, c.Pan)
					}
					if c.Engine != c.Pan {
						return fmt.Errorf("%s: class (i) but the engine and pan ended up different (%s vs %s)", mu.File, c.Engine, c.Pan)
					}
					moved = true
				}
				if !moved {
					return fmt.Errorf("%s: class (i) with no check of class (i)", mu.File)
				}
			}
		}
		return nil
	})

	sc.Step(`^the agreement rate equals \(i\) divided by \(i\) plus \(iii\) over the recorded counts$`, func() error {
		res, err := k3results(w.listing)
		if err != nil {
			return err
		}
		var c mutate.Counts
		for _, m := range res.Models {
			for _, mu := range m.Mutants {
				switch mu.Class {
				case mutate.ClassDetected:
					c.Detected++
				case mutate.ClassEquivalent:
					c.Equivalent++
				case mutate.ClassDisagree:
					c.Disagreement++
				default:
					c.NotComparabl++
				}
			}
		}
		if c != res.Counts {
			return fmt.Errorf("the recorded counts %+v do not match the mutants %+v", res.Counts, c)
		}
		rate, ok := c.Rate()
		if !ok {
			return fmt.Errorf("no mutant moved a verdict: there is no detection rate to report")
		}
		want := float64(c.Detected) / float64(c.Detected+c.Disagreement)
		if rate != want {
			return fmt.Errorf("rate %v, want %v", rate, want)
		}
		return nil
	})

	sc.Step(`^the share of detected mutants is reported separately as \(i\) over all mutants$`, func() error {
		res, err := k3results(w.listing)
		if err != nil {
			return err
		}
		share, ok := res.Counts.DetectedShare()
		if !ok {
			return fmt.Errorf("no mutants: there is no share to report")
		}
		want := float64(res.Counts.Detected) / float64(res.Counts.Total())
		if share != want {
			return fmt.Errorf("share %v, want %v", share, want)
		}
		// The two numbers must be different quantities, not the same one
		// under two names (steps/k3-logika.md, finding 1): the report has to
		// print both, and the denominators must differ whenever some mutant
		// did not move a verdict.
		rate, rateOK := res.Counts.Rate()
		if rateOK && res.Counts.Equivalent+res.Counts.NotComparabl > 0 && rate == share {
			return fmt.Errorf("the agreement rate and the detected share came out equal (%v) although %d mutants moved no verdict: one of the two denominators is wrong",
				rate, res.Counts.Equivalent+res.Counts.NotComparabl)
		}
		md, err := os.ReadFile(filepath.Join("..", "steps", "k3-mutation-report.md"))
		if err != nil {
			return err
		}
		for _, want := range []string{"Agreement rate on verdict-moving mutants", "Share of detected mutants"} {
			if !bytes.Contains(md, []byte(want)) {
				return fmt.Errorf("the report does not print %q", want)
			}
		}
		return nil
	})

	sc.Step(`^the report "([^"]*)" names every mutant of class "([^"]*)"$`, func(path, class string) error {
		res, err := k3results(w.listing)
		if err != nil {
			return err
		}
		md, err := os.ReadFile(filepath.Join("..", path))
		if err != nil {
			return err
		}
		for _, d := range res.Disagreements {
			if !bytes.Contains(md, []byte(d.Mutant)) {
				return fmt.Errorf("%s does not name the %s mutant %s", path, class, d.Mutant)
			}
		}
		if len(res.Disagreements) == 0 && !bytes.Contains(md, []byte("None.")) {
			return fmt.Errorf("%s does not say that there is no mutant of class %s", path, class)
		}
		return nil
	})
}

// k3deletes reports whether the operator's replacement is empty by design.
func k3deletes(op string) bool {
	switch mutate.Operator(op) {
	case mutate.DropAtomic, mutate.DropDStep, mutate.DropEndLabel, mutate.DropAlternative:
		return true
	}
	return false
}

// k3pan renders a pan verdict in the words the feature uses.
func k3pan(v string) string {
	switch v {
	case "violated":
		return "error found"
	case "verified":
		return "no error"
	}
	return v
}

func k3entries(data []byte) ([]mutate.Entry, error) {
	var entries []mutate.Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func k3results(path string) (*mutate.Results, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var res mutate.Results
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// k3listings lists the corpus2 files with the extension, as paths relative to
// the corpus2 root and in a fixed order.
func k3listings(root, ext string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(p) != ext {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

// k3readme reads the corpus2 README table: a row per listing, keyed by the
// file name in the first column (with any markdown backticks or link syntax
// stripped).
func k3readme(root string) (map[string][]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return nil, err
	}
	rows := map[string][]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		name := strings.Trim(cells[0], "`")
		if !strings.HasSuffix(name, ".pml") {
			continue // the header row and the separator
		}
		rows[name] = cells
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s/README.md has no table rows naming a .pml file", root)
	}
	return rows, nil
}

func k3column(root string, col int, list string) error {
	rows, err := k3readme(root)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, w := range strings.Split(list, ",") {
		allowed[strings.TrimSpace(strings.Trim(strings.TrimSpace(w), `"`))] = true
	}
	names := make([]string, 0, len(rows))
	for k := range rows {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		cells := rows[name]
		if col >= len(cells) {
			return fmt.Errorf("%s: the row has %d cells, no column %d", name, len(cells), col+1)
		}
		word := strings.Fields(strings.Trim(cells[col], "`*"))
		if len(word) == 0 {
			return fmt.Errorf("%s: column %d is empty", name, col+1)
		}
		if !allowed[strings.Trim(word[0], "`*,.:")] {
			return fmt.Errorf("%s: column %d says %q, which is not one of %s", name, col+1, cells[col], list)
		}
	}
	return nil
}

// TestK3MutatorOnCorpus is the unit-level guard behind the feature's first
// section: every mutant of every model the campaign touches must still be a
// Promela program the frontend accepts, or the mutation rate would measure
// the mutator rather than the engine. It runs without spin.
func TestK3MutatorOnCorpus(t *testing.T) {
	for _, name := range []string{
		"CH2/mutex_flaw.pml", "CH2/peterson.pml", "CH3/euclid.pml",
		"CH3/alternatingbit2.pml", "CH4/dijkstra.pml", "CH8/trivial.pml", "App_C/petrinet1",
	} {
		src, err := os.ReadFile(filepath.Join(corpusDir, name))
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		muts := mutate.Generate(src, name)
		if len(muts) == 0 {
			t.Errorf("%s: no mutants", name)
			continue
		}
		if _, err := mutate.WriteAll(dir, muts); err != nil {
			t.Fatal(err)
		}
		for _, m := range muts {
			var out, errb bytes.Buffer
			p := filepath.Join(dir, m.FileName())
			if code := cli.Run([]string{"parse", "--promela", p}, &out, &errb); code != 0 {
				t.Errorf("%s %s at %d:%d (%q → %q): exit %d, %s",
					name, m.Operator, m.Line, m.Col, m.Original, m.Mutated, code, strings.TrimSpace(out.String()))
			}
		}
	}
}

// TestCorpus2FidelityToSource checks the one claim the second corpus rests on
// (plan 14 §2.3, and the README of testdata/corpus2): the extraction
// reconstructed OCR line breaks and changed NOTHING else. The check is
// mechanical and does not trust the extraction's own report.
//
// Method: strip the header comment, decode the HTML entities and markdown
// escapes the README records, remove all whitespace from both the listing and
// the markdown lines it cites, and verify that the listing is a SUBSEQUENCE of
// the source — that is, that it can be obtained from the cited text by
// deleting characters only. Deletions are expected and documented (the slide
// annotation glued onto the end of a listing, and the gaps between joined
// pieces); an INSERTION would mean a token was invented or altered, and that
// is what this test forbids.
//
// Karpov's listings carry printed line numbers, which the extraction removed
// as the line-break markers they are; digits are therefore ignored for those
// files, which makes the check weaker for them and is stated as such here and
// in steps/k3-logika.md.
func TestCorpus2FidelityToSource(t *testing.T) {
	const booksDir = "../../books-md"
	if _, err := os.Stat(booksDir); err != nil {
		t.Skip("books-md not present")
	}
	files, err := k3listings(k3Corpus2, ".pml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no listings under " + k3Corpus2)
	}
	header := regexp.MustCompile(`(?s)^/\*.*?\*/\n`)
	source := regexp.MustCompile(`\* Source: (\S+), lines? (\d+)(?:-(\d+))?`)
	anyNum := regexp.MustCompile(`\b\d{3,4}\b`)
	cache := map[string][]string{}
	for _, name := range files {
		raw, err := os.ReadFile(filepath.Join(k3Corpus2, name))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		head := header.FindString(text)
		if head == "" {
			t.Errorf("%s: no header comment", name)
			continue
		}
		m := source.FindStringSubmatch(head)
		if m == nil {
			t.Errorf("%s: the header does not cite a source line range", name)
			continue
		}
		path := filepath.Join("..", "..", m[1])
		lines, ok := cache[path]
		if !ok {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			lines = strings.Split(string(data), "\n")
			cache[path] = lines
		}
		// Every line the header cites: the range, plus the duplicate and
		// joined-piece line numbers it lists after "Source:".
		want := map[int]bool{}
		from, _ := strconv.Atoi(m[2])
		to := from
		if m[3] != "" {
			to, _ = strconv.Atoi(m[3])
		}
		for n := from; n <= to; n++ {
			want[n] = true
		}
		if i := strings.Index(head, "Source:"); i >= 0 {
			for _, s := range anyNum.FindAllString(head[i:], -1) {
				if n, err := strconv.Atoi(s); err == nil {
					want[n] = true
				}
			}
		}
		nums := make([]int, 0, len(want))
		for n := range want {
			nums = append(nums, n)
		}
		sort.Ints(nums)
		var src strings.Builder
		for _, n := range nums {
			if n >= 1 && n <= len(lines) {
				src.WriteString(lines[n-1])
				src.WriteByte(' ')
			}
		}
		dropDigits := strings.HasPrefix(name, "karpov/")
		body := k3normalise(strings.TrimPrefix(text, head), dropDigits)
		cited := k3normalise(src.String(), dropDigits)
		if body == "" {
			t.Errorf("%s: empty listing", name)
			continue
		}
		if !k3isSubsequence(body, cited) {
			t.Errorf("%s: the listing is not obtainable from %s lines %v by deleting characters — "+
				"something was inserted or altered", name, m[1], nums)
		}
	}
}

// k3normalise applies the documented repairs and removes all whitespace, so
// that only the sequence of non-blank characters is compared.
func k3normalise(s string, dropDigits bool) string {
	r := strings.NewReplacer(
		"&amp;", "&", "&gt;", ">", "&lt;", "<", "&quot;", `"`, "&#39;", "'", `\_`, "_",
	)
	s = r.Replace(s)
	var b strings.Builder
	for _, c := range s {
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
		case dropDigits && c >= '0' && c <= '9':
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// k3isSubsequence reports whether every rune of need appears in have, in
// order. Deletions from have are allowed; an insertion into need is not.
func k3isSubsequence(need, have string) bool {
	n := []rune(need)
	i := 0
	for _, c := range have {
		if i < len(n) && n[i] == c {
			i++
		}
	}
	return i == len(n)
}
