package pormut

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The mutants are exact textual replacements, so they go stale when the code
// they patch moves. This test is what keeps the data honest: every anchor must
// occur exactly once in the current source, and the list must load.
func TestMutantAnchorsOccurExactlyOnce(t *testing.T) {
	ms, err := LoadMutants("mutants.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 {
		t.Fatal("no mutants")
	}
	if err := CheckAnchors(filepath.Join("..", ".."), ms); err != nil {
		t.Fatal(err)
	}
}

// A layer is a regular expression of exact test names, and CheckLayers only sees
// that the expression matches at least one: a renamed oracle would leave the
// layer green with the others. Every oracle the layers name must exist.
func TestEveryOracleTheLayersNameExists(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "explore", "*_test.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no tests of the explore package: %v", err)
	}
	var src strings.Builder
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src.Write(b)
	}
	for _, name := range strings.Split(oracleTests, "|") {
		if !strings.Contains(src.String(), "\nfunc "+name+"(t *testing.T)") {
			t.Errorf("the oracle %s is named by the layers and no longer exists", name)
		}
	}
}

func TestClassify(t *testing.T) {
	green := map[string]Verdict{"O1": Green, "O2": Green, "O3": Green, "directed": Green}
	with := func(layer string, v Verdict) map[string]Verdict {
		out := map[string]Verdict{}
		for k, x := range green {
			out[k] = x
		}
		out[layer] = v
		return out
	}
	for _, c := range []struct {
		name    string
		m       Mutant
		v       map[string]Verdict
		outcome string
		bad     bool
	}{
		{"killed by the audit", Mutant{}, with("O2", Red), "killed", false},
		{"killed by a directed test only", Mutant{}, with("directed", Red), "directed only", true},
		{"survivor", Mutant{}, green, "SURVIVED", true},
		{"equivalent survives", Mutant{Expect: "equivalent", Note: "x"}, green, "survived (equivalent)", false},
		{"equivalent killed", Mutant{Expect: "equivalent", Note: "x"}, with("O1", Red), "SURPRISE (expected to survive: equivalent)", true},
		{"pinned and killed by a directed test", Mutant{Expect: "pinned", Note: "x"}, with("directed", Red), "pinned by a directed test", false},
		{"pinned and killed by an oracle", Mutant{Expect: "pinned", Note: "x"}, with("O1", Red), "SURPRISE (killed by an oracle: not merely pinned)", true},
		{"pinned and not killed", Mutant{Expect: "pinned", Note: "x"}, green, "SURVIVED (a pinned mutant must be killed by its directed test)", true},
		{"a mutant that does not build is not a kill", Mutant{}, with("O1", BuildFailed), "build failed", true},
	} {
		got, bad := Classify(c.m, c.v)
		if got != c.outcome || bad != c.bad {
			t.Errorf("%s: %q %v, want %q %v", c.name, got, bad, c.outcome, c.bad)
		}
	}
}

func TestApplyNeedsAnExactAnchor(t *testing.T) {
	m := Mutant{ID: "x", Old: "a", New: "b"}
	if _, err := m.Apply([]byte("aa")); err == nil {
		t.Fatal("an anchor that occurs twice must be refused")
	}
	if _, err := m.Apply([]byte("c")); err == nil {
		t.Fatal("an anchor that does not occur must be refused")
	}
	if out, err := m.Apply([]byte("ac")); err != nil || string(out) != "bc" {
		t.Fatalf("%q %v", out, err)
	}
}

// ---- Run refuses a broken harness before its baseline starts ---------------------------

// fakeEngine writes a tiny module that has the shape the harness expects (an
// explore package with a file that mutants patch, and two tests that fail when
// both variables are 2) and a mutant list, and returns the engine directory and
// the list's path. The harness runs go test in a copy of the directory, so Run
// can be tested without the real engine.
func fakeEngine(t *testing.T, mutants []Mutant) (engineDir, mutantsPath string) {
	t.Helper()
	engineDir = t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(engineDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module fake\n\ngo 1.21\n")
	write("explore/por.go", "package explore\n\nvar a = 1\nvar b = 1\n")
	write("explore/por_test.go", `package explore

import "testing"

func TestPORGreen(t *testing.T) {
	if a == 2 && b == 2 {
		t.Fatal("mutated")
	}
}

func TestPORDirected(t *testing.T) {
	if a == 2 && b == 2 {
		t.Fatal("mutated")
	}
}
`)
	b, err := json.Marshal(mutants)
	if err != nil {
		t.Fatal(err)
	}
	mutantsPath = filepath.Join(t.TempDir(), "mutants.json")
	if err := os.WriteFile(mutantsPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return engineDir, mutantsPath
}

// withLayers replaces the layers of the harness for one test.
func withLayers(t *testing.T, ls ...Layer) {
	t.Helper()
	saved := Layers
	Layers = ls
	t.Cleanup(func() { Layers = saved })
}

var (
	oracleLayer   = Layer{"O1", []string{"-run", "^TestPORGreen$"}}
	directedLayer = Layer{"directed", []string{"-run", "^TestPOR", "-skip", "^TestPORGreen$"}}
)

func fakeConfig() Config {
	return Config{Models: 1, Workers: 1, Timeout: 2 * time.Minute}
}

// An anchor goes stale when the code it patches moves. The harness used to apply
// the anchors one mutant at a time, after the baseline and only for the mutants
// selected: a stale anchor of an unselected mutant went unseen (b4 and a7b stayed
// broken for three commits), and a selected one aborted the run after the whole
// baseline with partial results. Every stale id must be reported at once, and
// before anything is copied or run.
func TestRunRefusesEveryStaleAnchorBeforeTheBaseline(t *testing.T) {
	withLayers(t, oracleLayer, directedLayer)
	engineDir, path := fakeEngine(t, []Mutant{
		{ID: "fine", Old: "var a = 1", New: "var a = 2"},
		{ID: "unselected-and-fine", Old: "var b = 1", New: "var b = 2"},
		{ID: "gone", Old: "var c = 1", New: "var c = 2"},               // the anchor does not occur
		{ID: "twice", Old: "var", New: "const"},                        // it occurs more than once
		{ID: "nofile", File: "explore/missing.go", Old: "x", New: "y"}, // the file is gone
		{ID: "combo-with-a-stale-part", With: []string{"fine", "gone"}},
		{ID: "combo-that-cannot-compose", With: []string{"fine", "same"}}, // the anchor of the second is gone once the first is applied
		{ID: "same", Old: "var a = 1", New: "var a = 3"},
		{ID: "combo-fine", With: []string{"fine", "unselected-and-fine"}},
	})
	var log bytes.Buffer
	scratch := t.TempDir()
	res, err := Run(context.Background(), engineDir, path, scratch, map[string]bool{"fine": true}, fakeConfig(), &log)
	if err == nil {
		t.Fatalf("a list with stale anchors was accepted: %d results, log:\n%s", len(res), log.String())
	}
	// One line per stale mutant, each starting with its id: the stale ones are
	// all named, and none of the others.
	lines := map[string]bool{}
	for _, l := range strings.Split(err.Error(), "\n")[1:] {
		l = strings.TrimSpace(l)
		l = strings.TrimPrefix(strings.TrimPrefix(l, "mutant "), "combination ")
		lines[strings.SplitN(l, ":", 2)[0]] = true
	}
	for _, id := range []string{"gone", "twice", "nofile", "combo-with-a-stale-part", "combo-that-cannot-compose"} {
		if !lines[id] {
			t.Errorf("the stale mutant %q is not named: %v", id, err)
		}
	}
	for _, id := range []string{"fine", "unselected-and-fine", "same", "combo-fine"} {
		if lines[id] {
			t.Errorf("%q is not stale and is named: %v", id, err)
		}
	}
	if strings.Contains(log.String(), "baseline") || len(res) != 0 {
		t.Errorf("the baseline started before the anchors were checked: %d results, log:\n%s", len(res), log.String())
	}
	if entries, _ := os.ReadDir(scratch); len(entries) != 0 {
		t.Errorf("the engine was copied before the anchors were checked: %d entries in the scratch directory", len(entries))
	}
}

func TestRunRefusesAnUnknownOnlyId(t *testing.T) {
	withLayers(t, oracleLayer, directedLayer)
	engineDir, path := fakeEngine(t, []Mutant{{ID: "fine", Old: "var a = 1", New: "var a = 2"}})
	var log bytes.Buffer
	_, err := Run(context.Background(), engineDir, path, t.TempDir(), map[string]bool{"fine": true, "tpyo": true}, fakeConfig(), &log)
	if err == nil || !strings.Contains(err.Error(), "tpyo") {
		t.Fatalf("an -only id that is not in the list must be refused by name: %v", err)
	}
	if strings.Contains(log.String(), "baseline") {
		t.Errorf("the baseline started: %s", log.String())
	}
}

// A layer whose -run regex matches no test is "ok [no tests to run]" for go test,
// which the harness counted as green: the layer killed nothing and said nothing.
// The three survivors a4, a12 and r13 were all that would have hidden it.
func TestRunRefusesALayerThatMatchesNoTest(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go tool")
	}
	for name, l := range map[string]Layer{
		"a regex that matches nothing":     {"EMPTY", []string{"-run", "^TestNoSuchTest$"}},
		"everything skipped by the -skip":  {"SKIPPED", []string{"-run", "^TestPOR", "-skip", "^TestPOR"}},
		"a -run regex that does not parse": {"BROKEN", []string{"-run", "^(TestPOR"}},
	} {
		t.Run(name, func(t *testing.T) {
			withLayers(t, oracleLayer, l, directedLayer)
			engineDir, path := fakeEngine(t, []Mutant{{ID: "fine", Old: "var a = 1", New: "var a = 2"}})
			var log bytes.Buffer
			res, err := Run(context.Background(), engineDir, path, t.TempDir(), nil, fakeConfig(), &log)
			if err == nil {
				t.Fatalf("the layer %s was accepted: %d results, log:\n%s", l.Name, len(res), log.String())
			}
			if !strings.Contains(err.Error(), l.Name) || strings.Contains(err.Error(), "O1") || strings.Contains(err.Error(), "directed") {
				t.Errorf("the error must name the layer %s and only that one: %v", l.Name, err)
			}
			if strings.Contains(log.String(), "baseline") {
				t.Errorf("the baseline started before the layers were checked: %s", log.String())
			}
		})
	}
}

// The checks must not refuse a good list: the whole run, on layers that have
// tests, kills the mutant that breaks them and lets the harmless one survive.
// Two mutants that do nothing alone and break the tests together stand for the
// double mutant of the real list: the combination applies both to the same copy,
// and the copy is whole again afterwards (the next mutant is clean).
func TestRunStillRunsAGoodListAndAppliesACombination(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go tool")
	}
	withLayers(t, oracleLayer, directedLayer)
	engineDir, path := fakeEngine(t, []Mutant{
		{ID: "a2", Old: "var a = 1", New: "var a = 2", Expect: "equivalent", Note: "alone it changes nothing the tests see"},
		{ID: "both", With: []string{"a2", "b2"}},
		{ID: "b2", Old: "var b = 1", New: "var b = 2", Expect: "equivalent", Note: "alone it changes nothing the tests see"},
	})
	var log bytes.Buffer
	res, err := Run(context.Background(), engineDir, path, t.TempDir(), nil, fakeConfig(), &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	got := map[string]string{}
	for _, r := range res {
		got[r.Mutant.ID] = r.Outcome
	}
	want := map[string]string{"a2": "survived (equivalent)", "both": "killed", "b2": "survived (equivalent)"}
	if len(got) != len(want) {
		t.Fatalf("results %v, want %v\n%s", got, want, log.String())
	}
	for id, outcome := range want {
		if got[id] != outcome {
			t.Errorf("%s: %q, want %q\n%s", id, got[id], outcome, log.String())
		}
	}
}

func TestLoadMutantsChecksCombinations(t *testing.T) {
	load := func(ms ...Mutant) error {
		b, err := json.Marshal(ms)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "m.json")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err = LoadMutants(path)
		return err
	}
	x, y := Mutant{ID: "x", Old: "a", New: "b"}, Mutant{ID: "y", Old: "c", New: "d"}
	if err := load(x, y, Mutant{ID: "xy", With: []string{"x", "y"}}); err != nil {
		t.Errorf("a combination of two mutants was refused: %v", err)
	}
	for name, m := range map[string]Mutant{
		"one part only":       {ID: "xy", With: []string{"x"}},
		"an unknown part":     {ID: "xy", With: []string{"x", "z"}},
		"itself":              {ID: "xy", With: []string{"x", "xy"}},
		"a part twice":        {ID: "xy", With: []string{"x", "x"}},
		"a replacement too":   {ID: "xy", With: []string{"x", "y"}, Old: "a", New: "b"},
		"a combination of it": {ID: "xy", With: []string{"x", "y"}},
	} {
		list := []Mutant{x, y, m}
		if name == "a combination of it" {
			list = []Mutant{x, y, m, {ID: "xyx", With: []string{"xy", "x"}}}
		}
		if err := load(list...); err == nil {
			t.Errorf("%s: the combination was accepted", name)
		}
	}
}
