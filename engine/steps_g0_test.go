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
)

// g0World is the per-scenario state for features/g0-engine.feature. Every
// step goes through cli.Run, i.e. the same code path as the mcd binary.
type g0World struct {
	file    string
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	exit    int
	outputs [][]byte
	saved   map[string][]byte
	tmp     string
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG0Steps)
}

func registerG0Steps(sc *godog.ScenarioContext) {
	w := &g0World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g0World{saved: map[string][]byte{}}
		w.tmp = os.TempDir()
		if d, err := os.MkdirTemp("", "g0-"); err == nil {
			w.tmp = d
		}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if strings.HasPrefix(filepath.Base(w.tmp), "g0-") {
			os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// --- Given -------------------------------------------------------------
	given := func(path string) error {
		if _, err := os.Stat(path); err != nil {
			return err
		}
		w.file = path
		return nil
	}
	sc.Step(`^the Petri net file "([^"]*)"$`, given)
	sc.Step(`^the IR file "([^"]*)"$`, given)

	// --- When ----------------------------------------------------------------
	run := func(cmd string) {
		w.stdout.Reset()
		w.stderr.Reset()
		w.exit = cli.Run(w.args(cmd), &w.stdout, &w.stderr)
	}
	sc.Step(`^I run "mcd ([^"]*)"$`, func(cmd string) error {
		run(cmd)
		return nil
	})
	sc.Step(`^I run "mcd ([^"]*)" twice$`, func(cmd string) error {
		for i := 0; i < 2; i++ {
			run(cmd)
			w.outputs = append(w.outputs, append([]byte(nil), w.stdout.Bytes()...))
		}
		return nil
	})
	sc.Step(`^I run "mcd ([^"]*)" and save the output as "([^"]*)"$`, func(cmd, name string) error {
		run(cmd)
		if w.exit != 0 {
			return fmt.Errorf("exit %d: %s%s", w.exit, w.stdout.String(), w.stderr.String())
		}
		out := append([]byte(nil), w.stdout.Bytes()...)
		w.saved[name] = out
		return os.WriteFile(filepath.Join(w.tmp, name), out, 0o644)
	})

	// --- Then: exit code and plain output -----------------------------------------
	sc.Step(`^the exit code is (\d+)$`, func(code int) error {
		if w.exit != code {
			return fmt.Errorf("exit code %d, want %d\nstdout: %s\nstderr: %s", w.exit, code, w.stdout.String(), w.stderr.String())
		}
		return nil
	})
	sc.Step(`^the output mentions "([^"]*)"$`, func(s string) error {
		if !strings.Contains(w.stdout.String(), s) {
			return fmt.Errorf("output %q does not mention %q", w.stdout.String(), s)
		}
		return nil
	})
	sc.Step(`^both outputs are byte-identical$`, func() error {
		if len(w.outputs) != 2 {
			return fmt.Errorf("have %d outputs", len(w.outputs))
		}
		if !bytes.Equal(w.outputs[0], w.outputs[1]) {
			return fmt.Errorf("outputs differ:\n%s\n---\n%s", w.outputs[0], w.outputs[1])
		}
		return nil
	})
	sc.Step(`^the output equals the golden file "([^"]*)"$`, func(path string) error {
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(want, w.stdout.Bytes()) {
			return fmt.Errorf("output differs from %s:\n%s", path, w.stdout.String())
		}
		return nil
	})

	// --- Then: report assertions --------------------------------------------------
	sc.Step(`^the property "([^"]*)" has status "([^"]*)" with evidence "([^"]*)"$`, func(id, status, ev string) error {
		p, err := w.property(id)
		if err != nil {
			return err
		}
		if p["status"] != status || p["evidence"] != ev {
			return fmt.Errorf("%s: status=%v evidence=%v reason=%v", id, p["status"], p["evidence"], p["reason"])
		}
		return nil
	})
	sc.Step(`^the counterexample of "([^"]*)" fires the transitions "([^"]*)"$`, func(id, seq string) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		if ce["summary"] != seq {
			return fmt.Errorf("%s: counterexample is %q", id, ce["summary"])
		}
		return nil
	})
	sc.Step(`^the final marking of the counterexample of "([^"]*)" is "([^"]*)"$`, func(id, want string) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		var parts []string
		for _, v := range ce["final_state"].([]any) {
			val := v.(map[string]any)
			if val["value"].(float64) != 0 {
				parts = append(parts, fmt.Sprintf("%s=%d", val["var"], int(val["value"].(float64))))
			}
		}
		if got := strings.Join(parts, " "); got != want {
			return fmt.Errorf("final marking %q, want %q", got, want)
		}
		return nil
	})
	sc.Step(`^the counterexample of "([^"]*)" maps its steps to the user names "([^"]*)"$`, func(id, want string) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		var names []string
		for _, s := range ce["steps"].([]any) {
			st := s.(map[string]any)
			o, ok := st["origin"].(map[string]any)
			if !ok || o["name"] == nil {
				return fmt.Errorf("step %v has no origin name", st["command"])
			}
			names = append(names, o["name"].(string))
		}
		if got := strings.Join(names, ", "); got != want {
			return fmt.Errorf("user names %q, want %q", got, want)
		}
		return nil
	})
	sc.Step(`^the reason of "([^"]*)" mentions "([^"]*)"$`, func(id, s string) error {
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
		for _, p := range r["properties"].([]any) {
			pm := p.(map[string]any)
			if pm["complete"] != want {
				return fmt.Errorf("property %v complete=%v, want %v", pm["id"], pm["complete"], want)
			}
		}
		return nil
	}
	sc.Step(`^the report is complete$`, func() error { return complete(true) })
	sc.Step(`^the report is not complete$`, func() error { return complete(false) })
	sc.Step(`^the report counts (\d+) states$`, func(n int) error {
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
	sc.Step(`^the report names the engine and its version$`, func() error {
		r, err := w.report()
		if err != nil {
			return err
		}
		e := r["engine"].(map[string]any)
		if e["name"] != "mcd" || e["version"] == "" || e["ir_schema"] == "" {
			return fmt.Errorf("engine block %v", e)
		}
		return nil
	})
	sc.Step(`^the report lists (\d+) input with a sha256 hash$`, func(n int) error {
		r, err := w.report()
		if err != nil {
			return err
		}
		in := r["inputs"].([]any)
		if len(in) != n {
			return fmt.Errorf("%d inputs", len(in))
		}
		for _, i := range in {
			h, _ := i.(map[string]any)["sha256"].(string)
			if len(h) != 64 {
				return fmt.Errorf("sha256 %q", h)
			}
		}
		return nil
	})

	// --- Then: rejections ---------------------------------------------------------
	sc.Step(`^the error kind is "([^"]*)"$`, func(kind string) error {
		e, err := w.errorBody()
		if err != nil {
			return err
		}
		if e["kind"] != kind {
			return fmt.Errorf("error kind %v, want %s", e["kind"], kind)
		}
		return nil
	})
	sc.Step(`^the error message mentions "([^"]*)"$`, func(s string) error {
		e, err := w.errorBody()
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("%v %v", e["path"], e["message"])
		if !strings.Contains(msg, s) {
			return fmt.Errorf("error %q does not mention %q", msg, s)
		}
		return nil
	})

	// --- Then: IR -------------------------------------------------------------------
	sc.Step(`^the saved outputs "([^"]*)" and "([^"]*)" are byte-identical$`, func(a, b string) error {
		if !bytes.Equal(w.saved[a], w.saved[b]) {
			return fmt.Errorf("%s and %s differ", a, b)
		}
		return nil
	})
	sc.Step(`^the saved outputs "([^"]*)" and "([^"]*)" are identical except for the inputs section$`, func(a, b string) error {
		strip := func(name string) ([]byte, error) {
			var m map[string]any
			if err := json.Unmarshal(w.saved[name], &m); err != nil {
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
	sc.Step(`^the saved output "([^"]*)" declares (\d+) global variables of type "([^"]*)" and (\d+) process with (\d+) edges$`,
		func(name string, nvars int, typ string, nprocs, nedges int) error {
			var m map[string]any
			if err := json.Unmarshal(w.saved[name], &m); err != nil {
				return err
			}
			globals := m["globals"].([]any)
			if len(globals) != nvars {
				return fmt.Errorf("%d globals", len(globals))
			}
			for _, g := range globals {
				if g.(map[string]any)["type"] != typ {
					return fmt.Errorf("global %v is not %s", g, typ)
				}
			}
			procs := m["processes"].([]any)
			if len(procs) != nprocs {
				return fmt.Errorf("%d processes", len(procs))
			}
			if edges := procs[0].(map[string]any)["edges"].([]any); len(edges) != nedges {
				return fmt.Errorf("%d edges", len(edges))
			}
			return nil
		})
	sc.Step(`^every global variable and every edge of the IR has an origin with a user name$`, func() error {
		var m map[string]any
		if err := json.Unmarshal(w.stdout.Bytes(), &m); err != nil {
			return err
		}
		check := func(what string, x any) error {
			o, ok := x.(map[string]any)["origin"].(map[string]any)
			if !ok || o["name"] == "" || o["name"] == nil {
				return fmt.Errorf("%s %v has no origin name", what, x.(map[string]any)["name"])
			}
			return nil
		}
		for _, g := range m["globals"].([]any) {
			if err := check("global", g); err != nil {
				return err
			}
		}
		for _, p := range m["processes"].([]any) {
			for _, e := range p.(map[string]any)["edges"].([]any) {
				if err := check("edge", e); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// args splits a command line, substituting <file> and saved-output names.
func (w *g0World) args(cmd string) []string {
	var out []string
	for _, tok := range strings.Fields(cmd) {
		if tok == "<file>" {
			tok = w.file
		} else if _, ok := w.saved[tok]; ok {
			tok = filepath.Join(w.tmp, tok)
		}
		out = append(out, tok)
	}
	return out
}

func (w *g0World) report() (map[string]any, error) {
	var r map[string]any
	if err := json.Unmarshal(w.stdout.Bytes(), &r); err != nil {
		return nil, fmt.Errorf("stdout is not a JSON report (exit %d): %w\n%s%s", w.exit, err, w.stdout.String(), w.stderr.String())
	}
	if _, ok := r["properties"]; !ok {
		return nil, fmt.Errorf("stdout is not a report: %s", w.stdout.String())
	}
	return r, nil
}

func (w *g0World) property(id string) (map[string]any, error) {
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

func (w *g0World) trace(id string) (map[string]any, error) {
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

func (w *g0World) errorBody() (map[string]any, error) {
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
