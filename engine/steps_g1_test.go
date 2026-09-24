package modelcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"modelcheck/cli"
	"modelcheck/tools/pandiff"
)

// corpusDir is "Promela - examples" relative to the engine directory.
const corpusDir = "../../Promela - examples"

// g1World is the per-scenario state for features/g1-promela.feature. Every
// mcd step goes through cli.Run, the same code path as the binary.
type g1World struct {
	model  string
	stdout bytes.Buffer
	stderr bytes.Buffer
	exit   int
	kept   map[string][]byte
	tmp    string
	cmp    *pandiff.Comparison
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG1Steps)
}

func registerG1Steps(sc *godog.ScenarioContext) {
	w := &g1World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g1World{kept: map[string][]byte{}}
		w.tmp = os.TempDir()
		if d, err := os.MkdirTemp("", "g1-"); err == nil {
			w.tmp = d
		}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if strings.HasPrefix(filepath.Base(w.tmp), "g1-") {
			os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// --- Given -------------------------------------------------------------
	sc.Step(`^the Promela model "([^"]*)" from the corpus$`, func(name string) error {
		path := filepath.Join(corpusDir, name)
		if _, err := os.Stat(path); err != nil {
			return err
		}
		w.model = path
		return nil
	})
	sc.Step(`^the Promela file "([^"]*)"$`, func(path string) error {
		if _, err := os.Stat(path); err != nil {
			return err
		}
		w.model = path
		return nil
	})
	sc.Step(`^spin is installed$`, func() error {
		if !(pandiff.Tools{}).Available() {
			return godog.ErrSkip
		}
		return nil
	})

	// --- When ----------------------------------------------------------------
	run := func(cmd string) {
		w.stdout.Reset()
		w.stderr.Reset()
		w.exit = cli.Run(w.args(cmd), &w.stdout, &w.stderr)
	}
	sc.Step(`^I execute "mcd ([^"]*)"$`, func(cmd string) error {
		run(cmd)
		return nil
	})
	sc.Step(`^I execute "mcd ([^"]*)" and keep the output as "([^"]*)"$`, func(cmd, name string) error {
		run(cmd)
		if w.exit != 0 {
			return fmt.Errorf("exit %d: %s%s", w.exit, w.stdout.String(), w.stderr.String())
		}
		out := append([]byte(nil), w.stdout.Bytes()...)
		w.kept[name] = out
		return os.WriteFile(filepath.Join(w.tmp, name), out, 0o644)
	})
	sc.Step(`^I run pandiff on the model$`, func() error {
		cmp, _, _, err := pandiff.Run(context.Background(), pandiff.Tools{}, w.model, nil, false)
		if err != nil {
			return err
		}
		w.cmp = cmp
		return nil
	})

	// --- Then: exit code, rejections, warnings ----------------------------------
	sc.Step(`^the command exits with (\d+)$`, func(code int) error {
		if w.exit != code {
			return fmt.Errorf("exit code %d, want %d\nstdout: %s\nstderr: %s", w.exit, code, w.stdout.String(), w.stderr.String())
		}
		return nil
	})
	sc.Step(`^the rejection has kind "([^"]*)" and status "([^"]*)"$`, func(kind, status string) error {
		e, err := w.errorBody()
		if err != nil {
			return err
		}
		if e["kind"] != kind || e["status"] != status {
			return fmt.Errorf("rejection kind=%v status=%v, want %s/%s (%v)", e["kind"], e["status"], kind, status, e["message"])
		}
		return nil
	})
	sc.Step(`^the rejection mentions "([^"]*)"$`, func(s string) error {
		e, err := w.errorBody()
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("%v %v", e["path"], e["message"])
		if !strings.Contains(msg, s) {
			return fmt.Errorf("rejection %q does not mention %q", msg, s)
		}
		return nil
	})
	sc.Step(`^the warnings mention "([^"]*)"$`, func(s string) error {
		if strings.Contains(w.stderr.String(), s) {
			return nil
		}
		return fmt.Errorf("stderr %q does not mention %q", w.stderr.String(), s)
	})

	// --- Then: report -----------------------------------------------------------
	sc.Step(`^property "([^"]*)" is "([^"]*)" with evidence "([^"]*)"$`, func(id, status, ev string) error {
		p, err := w.property(id)
		if err != nil {
			return err
		}
		if p["status"] != status || p["evidence"] != ev {
			return fmt.Errorf("%s: status=%v evidence=%v reason=%v", id, p["status"], p["evidence"], p["reason"])
		}
		return nil
	})
	sc.Step(`^the reason for "([^"]*)" mentions "([^"]*)"$`, func(id, s string) error {
		p, err := w.property(id)
		if err != nil {
			return err
		}
		reason, _ := p["reason"].(string)
		if !strings.Contains(reason, s) {
			return fmt.Errorf("%s: reason %q does not mention %q", id, reason, s)
		}
		return nil
	})
	complete := func(want bool) error {
		r, err := w.report()
		if err != nil {
			return err
		}
		if got := r["search"].(map[string]any)["complete"]; got != want {
			return fmt.Errorf("search.complete = %v, want %v (stop: %v)", got, want, r["search"].(map[string]any)["stop"])
		}
		return nil
	}
	sc.Step(`^the search is complete$`, func() error { return complete(true) })
	sc.Step(`^the search is not complete$`, func() error { return complete(false) })
	sc.Step(`^the state count is (\d+)$`, func(n int) error {
		r, err := w.report()
		if err != nil {
			return err
		}
		p := r["properties"].([]any)[0].(map[string]any)
		if got := int(p["counters"].(map[string]any)["states"].(float64)); got != n {
			return fmt.Errorf("states = %d, want %d", got, n)
		}
		return nil
	})
	sc.Step(`^the pan error class is "([^"]*)"$`, func(class string) error {
		r, err := w.report()
		if err != nil {
			return err
		}
		got := "no error"
		for _, p := range r["properties"].([]any) {
			pm := p.(map[string]any)
			if pm["status"] == "violated" {
				switch pm["kind"] {
				case "assert":
					got = "assertion violated"
				case "deadlock":
					if got == "no error" {
						got = "invalid end state"
					}
				}
			}
			if pm["status"] == "invalid-model" {
				got = "invalid-model: " + pm["reason"].(string)
			}
		}
		if got != class {
			return fmt.Errorf("error class %q, want %q", got, class)
		}
		return nil
	})

	// --- Then: counterexamples ----------------------------------------------------
	sc.Step(`^the counterexample of "([^"]*)" ends with "([^"]*)" equal to (-?\d+)$`, func(id, name string, val int) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		for _, v := range ce["final_state"].([]any) {
			vm := v.(map[string]any)
			if vm["var"] == name {
				if int(vm["value"].(float64)) != val {
					return fmt.Errorf("%s = %v in the final state, want %d", name, vm["value"], val)
				}
				return nil
			}
		}
		return fmt.Errorf("no variable %q in the final state", name)
	})
	sc.Step(`^the counterexample of "([^"]*)" passes through the labels "([^"]*)"$`, func(id, labels string) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, s := range ce["steps"].([]any) {
			if loc, ok := s.(map[string]any)["location"].(string); ok {
				seen[loc] = true
			}
		}
		for _, l := range strings.Split(labels, ",") {
			l = strings.TrimSpace(l)
			if !seen[l] {
				return fmt.Errorf("label %s is not a location of any step (seen %v)", l, seen)
			}
		}
		return nil
	})
	sc.Step(`^every step of the counterexample of "([^"]*)" names a process, a source line and the statement text$`, func(id string) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		steps := ce["steps"].([]any)
		if len(steps) == 0 {
			return fmt.Errorf("empty counterexample")
		}
		for _, s := range steps {
			st := s.(map[string]any)
			o, _ := st["origin"].(map[string]any)
			if st["process"] == "" || o == nil || o["line"] == nil || o["line"].(float64) <= 0 || o["name"] == nil || o["name"] == "" || st["command"] == "" {
				return fmt.Errorf("step %v lacks process/line/text", st)
			}
			if !strings.HasSuffix(o["file"].(string), filepath.Base(w.model)) {
				return fmt.Errorf("step origin file %v is not %s", o["file"], w.model)
			}
		}
		return nil
	})
	sc.Step(`^the last step of the counterexample of "([^"]*)" is "([^"]*)" at line (\d+)$`, func(id, text string, line int) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		steps := ce["steps"].([]any)
		st := steps[len(steps)-1].(map[string]any)
		o, _ := st["origin"].(map[string]any)
		if st["command"] != text || o == nil || int(o["line"].(float64)) != line {
			return fmt.Errorf("last step is %v at %v, want %q at line %d", st["command"], o, text, line)
		}
		return nil
	})
	sc.Step(`^the counterexample of "([^"]*)" contains the step "([^"]*)"$`, func(id, text string) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		for _, s := range ce["steps"].([]any) {
			if s.(map[string]any)["command"] == text {
				return nil
			}
		}
		return fmt.Errorf("no step %q in %v", text, ce["summary"])
	})
	sc.Step(`^the step "([^"]*)" of the counterexample of "([^"]*)" has the partner "([^"]*)" in process "([^"]*)"$`, func(text, id, ptext, proc string) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		for _, s := range ce["steps"].([]any) {
			st := s.(map[string]any)
			if st["command"] != text {
				continue
			}
			p, ok := st["partner"].(map[string]any)
			if !ok {
				return fmt.Errorf("step %q has no partner", text)
			}
			if p["command"] != ptext || p["process"] != proc {
				return fmt.Errorf("partner is %v/%v, want %s/%s", p["process"], p["command"], proc, ptext)
			}
			return nil
		}
		return fmt.Errorf("no step %q", text)
	})
	sc.Step(`^the counterexample of "([^"]*)" changes "([^"]*)" from (-?\d+) to (-?\d+)$`, func(id, name string, before, after int) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		for _, s := range ce["steps"].([]any) {
			chs, _ := s.(map[string]any)["changes"].([]any)
			for _, c := range chs {
				cm := c.(map[string]any)
				if cm["var"] == name && int(cm["before"].(float64)) == before && int(cm["after"].(float64)) == after {
					return nil
				}
			}
		}
		return fmt.Errorf("no step changes %s from %d to %d", name, before, after)
	})

	// --- Then: IR ---------------------------------------------------------------------
	claim := func() (map[string]any, error) {
		m, err := w.irDoc()
		if err != nil {
			return nil, err
		}
		for _, p := range m["processes"].([]any) {
			if pm := p.(map[string]any); pm["claim"] == true {
				return pm, nil
			}
		}
		return nil, fmt.Errorf("no claim process in the IR")
	}
	sc.Step(`^the IR has a claim process whose locations carry the label "([^"]*)"$`, func(label string) error {
		pm, err := claim()
		if err != nil {
			return err
		}
		for _, l := range pm["locations"].([]any) {
			for _, lb := range asList(l.(map[string]any)["labels"]) {
				if lb == label {
					return nil
				}
			}
		}
		return fmt.Errorf("no location of the claim carries %q", label)
	})
	sc.Step(`^the IR has a claim process whose locations carry no label$`, func() error {
		pm, err := claim()
		if err != nil {
			return err
		}
		for _, l := range pm["locations"].([]any) {
			if len(asList(l.(map[string]any)["labels"])) > 0 {
				return fmt.Errorf("claim location %v carries labels", l)
			}
		}
		return nil
	})
	sc.Step(`^the IR has a claim process with an edge whose text is "([^"]*)"$`, func(text string) error {
		pm, err := claim()
		if err != nil {
			return err
		}
		var texts []string
		for _, e := range pm["edges"].([]any) {
			t, _ := e.(map[string]any)["text"].(string)
			if t == text {
				return nil
			}
			texts = append(texts, t)
		}
		return fmt.Errorf("claim edges %v, none is %q", texts, text)
	})
	sc.Step(`^the IR channel "([^"]*)" has the hints xs "([^"]*)" and xr "([^"]*)"$`, func(ch, xs, xr string) error {
		m, err := w.irDoc()
		if err != nil {
			return err
		}
		for _, c := range m["channels"].([]any) {
			cm := c.(map[string]any)
			if cm["name"] != ch {
				continue
			}
			if strings.Join(asList(cm["xs"]), ",") != xs || strings.Join(asList(cm["xr"]), ",") != xr {
				return fmt.Errorf("channel %s hints xs=%v xr=%v", ch, cm["xs"], cm["xr"])
			}
			return nil
		}
		return fmt.Errorf("no channel %q", ch)
	})
	sc.Step(`^the kept outputs "([^"]*)" and "([^"]*)" are byte-identical$`, func(a, b string) error {
		if !bytes.Equal(w.kept[a], w.kept[b]) {
			return fmt.Errorf("%s and %s differ", a, b)
		}
		return nil
	})
	sc.Step(`^the kept outputs "([^"]*)" and "([^"]*)" are identical except for the inputs section$`, func(a, b string) error {
		strip := func(name string) ([]byte, error) {
			var m map[string]any
			if err := json.Unmarshal(w.kept[name], &m); err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			delete(m, "inputs")
			return json.Marshal(m)
		}
		ja, err := strip(a)
		if err != nil {
			return err
		}
		jb, err := strip(b)
		if err != nil {
			return err
		}
		if !bytes.Equal(ja, jb) {
			return fmt.Errorf("%s and %s differ beyond inputs:\n%s\n---\n%s", a, b, ja, jb)
		}
		return nil
	})
	sc.Step(`^the IR names the processes "([^"]*)"$`, func(names string) error {
		m, err := w.irDoc()
		if err != nil {
			return err
		}
		var got []string
		for _, p := range m["processes"].([]any) {
			got = append(got, p.(map[string]any)["name"].(string))
		}
		if strings.Join(got, ", ") != names {
			return fmt.Errorf("processes %v, want %q", got, names)
		}
		return nil
	})
	sc.Step(`^every edge of the IR has an origin with the file, a positive line and the statement text$`, func() error {
		m, err := w.irDoc()
		if err != nil {
			return err
		}
		for _, p := range m["processes"].([]any) {
			for _, e := range p.(map[string]any)["edges"].([]any) {
				em := e.(map[string]any)
				o, ok := em["origin"].(map[string]any)
				if !ok || o["file"] == nil || o["line"] == nil || o["line"].(float64) <= 0 || o["name"] == nil || o["name"] == "" || em["text"] == nil {
					return fmt.Errorf("edge %v has an incomplete origin", em)
				}
			}
		}
		return nil
	})
	sc.Step(`^the local "([^"]*)" of process "([^"]*)" is initialised to (\d+) and of "([^"]*)" to (\d+)$`, func(local, p1 string, v1 int, p2 string, v2 int) error {
		m, err := w.irDoc()
		if err != nil {
			return err
		}
		want := map[string]int{p1: v1, p2: v2}
		for _, p := range m["processes"].([]any) {
			pm := p.(map[string]any)
			v, ok := want[pm["name"].(string)]
			if !ok {
				continue
			}
			found := false
			for _, l := range asAny(pm["locals"]) {
				lm := l.(map[string]any)
				if lm["name"] == local {
					found = true
					init := asAny(lm["init"])
					if len(init) != 1 || int(init[0].(float64)) != v {
						return fmt.Errorf("%s.%s init %v, want %d", pm["name"], local, lm["init"], v)
					}
				}
			}
			if !found {
				return fmt.Errorf("%s has no local %s", pm["name"], local)
			}
			delete(want, pm["name"].(string))
		}
		if len(want) > 0 {
			return fmt.Errorf("processes not found: %v", want)
		}
		return nil
	})

	// --- Then: pandiff ------------------------------------------------------------------
	sc.Step(`^pandiff reports agreement on the verdict, the error class and the state count$`, func() error {
		if w.cmp == nil {
			return fmt.Errorf("pandiff did not run")
		}
		if !w.cmp.Agree {
			return fmt.Errorf("disagreement:\n%s", w.cmp.Table(w.model))
		}
		return nil
	})
	sc.Step(`^pandiff reports the statement table of every proctype as matching pan -d$`, func() error {
		if w.cmp == nil {
			return fmt.Errorf("pandiff did not run")
		}
		if !w.cmp.StmtAgree {
			return fmt.Errorf("statement tables differ:\n%s", w.cmp.Table(w.model))
		}
		return nil
	})
}

func asAny(x any) []any {
	l, _ := x.([]any)
	return l
}

func asList(x any) []string {
	var out []string
	for _, v := range asAny(x) {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

// args splits a command line, substituting <model> and kept-output names.
func (w *g1World) args(cmd string) []string {
	var out []string
	for _, tok := range strings.Fields(cmd) {
		if tok == "<model>" {
			tok = w.model
		} else if _, ok := w.kept[tok]; ok {
			tok = filepath.Join(w.tmp, tok)
		}
		out = append(out, tok)
	}
	return out
}

func (w *g1World) report() (map[string]any, error) {
	var r map[string]any
	if err := json.Unmarshal(w.stdout.Bytes(), &r); err != nil {
		return nil, fmt.Errorf("stdout is not a JSON report (exit %d): %w\n%s%s", w.exit, err, w.stdout.String(), w.stderr.String())
	}
	if _, ok := r["properties"]; !ok {
		return nil, fmt.Errorf("stdout is not a report: %s", w.stdout.String())
	}
	return r, nil
}

func (w *g1World) irDoc() (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(w.stdout.Bytes(), &m); err != nil {
		return nil, fmt.Errorf("stdout is not JSON (exit %d): %w\n%s%s", w.exit, err, w.stdout.String(), w.stderr.String())
	}
	if _, ok := m["processes"]; !ok {
		return nil, fmt.Errorf("stdout is not an IR document: %s", w.stdout.String())
	}
	return m, nil
}

func (w *g1World) property(id string) (map[string]any, error) {
	r, err := w.report()
	if err != nil {
		return nil, err
	}
	for _, p := range r["properties"].([]any) {
		pm := p.(map[string]any)
		if pm["id"] == id {
			return pm, nil
		}
	}
	return nil, fmt.Errorf("no property %q in the report", id)
}

func (w *g1World) trace(id string) (map[string]any, error) {
	p, err := w.property(id)
	if err != nil {
		return nil, err
	}
	ce, ok := p["counterexample"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s has no counterexample (status %v)", id, p["status"])
	}
	return ce, nil
}

func (w *g1World) errorBody() (map[string]any, error) {
	var r map[string]any
	if err := json.Unmarshal(w.stdout.Bytes(), &r); err != nil {
		return nil, fmt.Errorf("stdout is not JSON: %s", w.stdout.String())
	}
	e, ok := r["error"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("no error object in %s", w.stdout.String())
	}
	return e, nil
}
