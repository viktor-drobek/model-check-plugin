package modelcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cucumber/godog"

	"modelcheck/cex"
	"modelcheck/cli"
	"modelcheck/ir"
)

// g8Run is one `mcd` invocation and its parsed report.
type g8Run struct {
	stdout, stderr bytes.Buffer
	exit           int
	rep            map[string]any
	raw            []byte
	name           string
}

// g8World is the per-scenario state of features/g8-parallel.feature. Every
// step goes through cli.Run, the same code path as the mcd binary. A scenario
// holds one model and up to six runs of it, each kept under a name: the
// sequential run, the breadth-first run, the parallel run and the second,
// third and repeated parallel runs.
type g8World struct {
	file    string
	defines []string
	runs    map[string]*g8Run
	last    *g8Run
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG8Steps)
}

// g8Words splits a command line the way a shell does for the one thing the
// scenarios need: single quotes around an argument with spaces in it.
func g8Words(s string) []string {
	var out []string
	var cur strings.Builder
	inWord, quoted := false, false
	for _, r := range s {
		switch {
		case r == '\'':
			quoted, inWord = !quoted, true
		case (r == ' ' || r == '\t') && !quoted:
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

// the names of the runs a step may mean
const (
	g8Seq = "the sequential run"
	g8BFS = "the breadth-first run"
	g8Par = "the parallel run"
	g8Two = "the second parallel run"
	g8Thr = "the third parallel run"
	g8Rep = "the repeated run"
)

func registerG8Steps(sc *godog.ScenarioContext) {
	w := &g8World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g8World{runs: map[string]*g8Run{}}
		return ctx, nil
	})

	flagFor := func() string {
		switch filepath.Ext(w.file) {
		case ".json":
			if strings.Contains(w.file, "/petri/") {
				return "--petri"
			}
			return "--ir"
		}
		return "--promela"
	}
	run := func(name string, argv []string) (*g8Run, error) {
		r := &g8Run{name: name}
		r.exit = cli.Run(argv, &r.stdout, &r.stderr)
		r.raw = append([]byte(nil), r.stdout.Bytes()...)
		if r.stdout.Len() > 0 && r.stdout.Bytes()[0] == '{' {
			if err := json.Unmarshal(r.stdout.Bytes(), &r.rep); err != nil {
				return nil, fmt.Errorf("stdout is not JSON (exit %d): %v\n%s%s", r.exit, err, r.stdout.String(), r.stderr.String())
			}
		}
		w.runs[name], w.last = r, r
		return r, nil
	}
	check := func(name, args string, workers int) (*g8Run, error) {
		words := g8Words(args)
		for i, a := range words {
			if a == "-D" && i+1 < len(words) {
				w.defines = append(w.defines, words[i+1])
			}
		}
		argv := append([]string{"check", flagFor(), w.file}, words...)
		if workers > 0 {
			argv = append(argv, "--workers", fmt.Sprint(workers))
		}
		return run(name, argv)
	}
	get := func(name string) (*g8Run, error) {
		r := w.runs[name]
		if r == nil {
			return nil, fmt.Errorf("the scenario has not made %s yet", name)
		}
		return r, nil
	}
	search := func(r *g8Run) map[string]any {
		s, _ := r.rep["search"].(map[string]any)
		return s
	}
	parallelOf := func(r *g8Run) (map[string]any, error) {
		p, ok := search(r)["parallel"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("the report of %s has no search.parallel object: %s", r.name, r.stdout.String())
		}
		return p, nil
	}
	props := func(r *g8Run) (map[string]map[string]any, error) {
		list, ok := r.rep["properties"].([]any)
		if !ok {
			return nil, fmt.Errorf("the report of %s has no properties (exit %d): %s%s", r.name, r.exit, r.stdout.String(), r.stderr.String())
		}
		out := map[string]map[string]any{}
		for _, p := range list {
			pm := p.(map[string]any)
			out[pm["id"].(string)] = pm
		}
		return out, nil
	}
	counter := func(r *g8Run, key string) (float64, error) {
		list, ok := r.rep["properties"].([]any)
		if !ok || len(list) == 0 {
			return 0, fmt.Errorf("the report of %s has no properties", r.name)
		}
		c := list[0].(map[string]any)["counters"].(map[string]any)
		return c[key].(float64), nil
	}
	prop := func(r *g8Run, id string) (map[string]any, error) {
		ps, err := props(r)
		if err != nil {
			return nil, err
		}
		p, ok := ps[id]
		if !ok {
			return nil, fmt.Errorf("the report of %s has no property %q", r.name, id)
		}
		return p, nil
	}
	steps := func(r *g8Run, id, field string) (int, error) {
		p, err := prop(r, id)
		if err != nil {
			return 0, err
		}
		ce, ok := p[field].(map[string]any)
		if !ok {
			return 0, fmt.Errorf("property %q of %s has no %s", id, r.name, field)
		}
		st, _ := ce["steps"].([]any)
		return len(st), nil
	}
	// withoutWorkerFields is the report minus the three fields that may differ
	// between worker counts.
	withoutWorkerFields := func(r *g8Run) (string, error) {
		var rep map[string]any
		if err := json.Unmarshal(r.raw, &rep); err != nil {
			return "", err
		}
		if s, ok := rep["search"].(map[string]any); ok {
			if p, ok := s["parallel"].(map[string]any); ok {
				delete(p, "requested_workers")
				delete(p, "workers")
				delete(p, "worker_bytes_est")
			}
		}
		b, err := json.Marshal(rep)
		return string(b), err
	}
	model := func() (*ir.Model, error) {
		switch filepath.Ext(w.file) {
		case ".json":
			data, err := os.ReadFile(w.file)
			if err != nil {
				return nil, err
			}
			return ir.UnmarshalJSON(data)
		}
		src, err := os.ReadFile(w.file)
		if err != nil {
			return nil, err
		}
		p, rej := cli.ParsePromela(src, w.file, w.defines, 0)
		if rej != nil {
			return nil, fmt.Errorf("the model does not parse: %s", rej.Message)
		}
		return p.Model, nil
	}

	sc.Step(`^the model under parallel test "([^"]*)"$`, func(path string) error {
		w.file = path
		return nil
	})
	sc.Step(`^I check it sequentially with "([^"]*)"$`, func(args string) error {
		_, err := check(g8Seq, args, 0)
		return err
	})
	sc.Step(`^I check it sequentially with "([^"]*)" as the breadth-first run$`, func(args string) error {
		_, err := check(g8BFS, args, 0)
		return err
	})
	sc.Step(`^I check it in parallel with (\d+) workers and "([^"]*)"$`, func(n int, args string) error {
		_, err := check(g8Par, args, n)
		return err
	})
	for suffix, name := range map[string]string{"second parallel run": g8Two, "third parallel run": g8Thr, "repeated run": g8Rep} {
		name := name
		sc.Step(`^I check it in parallel with (\d+) workers and "([^"]*)" as the `+suffix+`$`, func(n int, args string) error {
			_, err := check(name, args, n)
			return err
		})
	}
	sc.Step(`^I run mcd with "([^"]*)"$`, func(args string) error {
		_, err := run("the last run", g8Words(args))
		return err
	})
	sc.Step(`^the last run exits with (\d+)$`, func(code int) error {
		if w.last == nil || w.last.exit != code {
			return fmt.Errorf("exit code %v, want %d", w.last, code)
		}
		return nil
	})
	sc.Step(`^the check exits with (\d+) in the parallel run$`, func(code int) error {
		r, err := get(g8Par)
		if err != nil {
			return err
		}
		if r.exit != code {
			return fmt.Errorf("exit code %d, want %d\n%s%s", r.exit, code, r.stdout.String(), r.stderr.String())
		}
		return nil
	})
	sc.Step(`^the standard error of the last run mentions "([^"]*)"$`, func(s string) error {
		if w.last == nil || !strings.Contains(w.last.stderr.String(), s) {
			return fmt.Errorf("standard error %q does not mention %q", w.last.stderr.String(), s)
		}
		return nil
	})

	// ---- what ran
	sc.Step(`^the parallel search was applied with (\d+) workers$`, func(n int) error {
		r, err := get(g8Par)
		if err != nil {
			return err
		}
		p, err := parallelOf(r)
		if err != nil {
			return err
		}
		if p["applied"] != true || int(p["workers"].(float64)) != n || int(p["requested_workers"].(float64)) != n {
			return fmt.Errorf("search.parallel is %v, want applied with %d workers", p, n)
		}
		return nil
	})
	sc.Step(`^the parallel search was refused with a reason mentioning "([^"]*)"$`, func(s string) error {
		r, err := get(g8Par)
		if err != nil {
			return err
		}
		p, err := parallelOf(r)
		if err != nil {
			return err
		}
		if p["applied"] != false {
			return fmt.Errorf("search.parallel applied: %v", p)
		}
		if reason, _ := p["reason"].(string); !strings.Contains(reason, s) {
			return fmt.Errorf("reason %q does not mention %q", reason, s)
		}
		return nil
	})
	sc.Step(`^the report of the sequential run has no parallel object$`, func() error {
		r, err := get(g8Seq)
		if err != nil {
			return err
		}
		if _, ok := search(r)["parallel"]; ok {
			return fmt.Errorf("the report carries search.parallel: %v", search(r)["parallel"])
		}
		return nil
	})
	sc.Step(`^the sequential run reports the search mode "([^"]*)"$`, func(mode string) error {
		r, err := get(g8Seq)
		if err != nil {
			return err
		}
		if search(r)["mode"] != mode {
			return fmt.Errorf("search.mode is %v, want %s", search(r)["mode"], mode)
		}
		return nil
	})
	sc.Step(`^the parallel run reports the search mode "([^"]*)"$`, func(mode string) error {
		r, err := get(g8Par)
		if err != nil {
			return err
		}
		if search(r)["mode"] != mode {
			return fmt.Errorf("search.mode is %v, want %s", search(r)["mode"], mode)
		}
		return nil
	})
	sc.Step(`^the second parallel run reports the search mode "([^"]*)"$`, func(mode string) error {
		r, err := get(g8Two)
		if err != nil {
			return err
		}
		if search(r)["mode"] != mode {
			return fmt.Errorf("search.mode of the second parallel run is %v, want %s", search(r)["mode"], mode)
		}
		return nil
	})
	sc.Step(`^the report of the sequential run equals the recorded report "([^"]*)"$`, func(path string) error {
		r, err := get(g8Seq)
		if err != nil {
			return err
		}
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(r.raw, want) {
			return fmt.Errorf("the report differs from %s (%d bytes against %d)", path, len(r.raw), len(want))
		}
		return nil
	})
	sc.Step(`^the report of the parallel run equals the sequential report except for the parallel object$`, func() error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		b, err := get(g8Seq)
		if err != nil {
			return err
		}
		strip := func(r *g8Run) (string, error) {
			var rep map[string]any
			if err := json.Unmarshal(r.raw, &rep); err != nil {
				return "", err
			}
			if s, ok := rep["search"].(map[string]any); ok {
				delete(s, "parallel")
			}
			out, err := json.Marshal(rep)
			return string(out), err
		}
		x, err := strip(a)
		if err != nil {
			return err
		}
		y, err := strip(b)
		if err != nil {
			return err
		}
		if x != y {
			return fmt.Errorf("the refused run differs from the sequential one:\n%s\n---\n%s", x, y)
		}
		return nil
	})
	sc.Step(`^the reduction was applied in the parallel run$`, func() error {
		r, err := get(g8Par)
		if err != nil {
			return err
		}
		red, ok := search(r)["reduction"].(map[string]any)
		if !ok || red["applied"] != true {
			return fmt.Errorf("search.reduction is %v, want applied", search(r)["reduction"])
		}
		return nil
	})
	sc.Step(`^the reduction was not applied in the parallel run$`, func() error {
		r, err := get(g8Par)
		if err != nil {
			return err
		}
		red, ok := search(r)["reduction"].(map[string]any)
		if !ok || red["applied"] != false {
			return fmt.Errorf("search.reduction is %v, want not applied", search(r)["reduction"])
		}
		return nil
	})

	// ---- counts
	sc.Step(`^the parallel run stores (\d+) states$`, func(n int) error {
		r, err := get(g8Par)
		if err != nil {
			return err
		}
		got, err := counter(r, "states")
		if err != nil {
			return err
		}
		if int(got) != n {
			return fmt.Errorf("the parallel run stores %v states, want %d", got, n)
		}
		return nil
	})
	sc.Step(`^the parallel run stores at most (\d+) states$`, func(n int) error {
		r, err := get(g8Par)
		if err != nil {
			return err
		}
		got, err := counter(r, "states")
		if err != nil {
			return err
		}
		if int(got) > n {
			return fmt.Errorf("the parallel run stores %v states, at most %d wanted", got, n)
		}
		return nil
	})
	// sameCounts compares `states` and `transitions` of two runs by name.
	sameCounts := func(an, bn string) error {
		a, err := get(an)
		if err != nil {
			return err
		}
		b, err := get(bn)
		if err != nil {
			return err
		}
		for _, k := range []string{"states", "transitions"} {
			x, err := counter(a, k)
			if err != nil {
				return err
			}
			y, err := counter(b, k)
			if err != nil {
				return err
			}
			if x != y {
				return fmt.Errorf("%s: %v in %s, %v in %s", k, x, an, y, bn)
			}
		}
		return nil
	}
	// countsAre checks the states and transitions of a named run against figures.
	countsAre := func(name string, states, transitions int) error {
		r, err := get(name)
		if err != nil {
			return err
		}
		for _, c := range []struct {
			key  string
			want int
		}{{"states", states}, {"transitions", transitions}} {
			got, err := counter(r, c.key)
			if err != nil {
				return err
			}
			if int(got) != c.want {
				return fmt.Errorf("%s has %v %s, want %d", name, got, c.key, c.want)
			}
		}
		return nil
	}
	sc.Step(`^the breadth-first run has (\d+) states and (\d+) transitions$`, func(s, t int) error { return countsAre(g8BFS, s, t) })
	sc.Step(`^the parallel run has (\d+) states and (\d+) transitions$`, func(s, t int) error { return countsAre(g8Par, s, t) })
	sc.Step(`^both runs have the same states and transitions$`, func() error { return sameCounts(g8Par, g8Seq) })
	sc.Step(`^the breadth-first run and the parallel run have the same states and transitions$`, func() error { return sameCounts(g8BFS, g8Par) })
	sc.Step(`^the parallel depth equals the breadth-first depth$`, func() error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		b, err := get(g8BFS)
		if err != nil {
			return err
		}
		x, err := counter(a, "depth")
		if err != nil {
			return err
		}
		y, err := counter(b, "depth")
		if err != nil {
			return err
		}
		if x != y {
			return fmt.Errorf("depth %v in parallel, %v breadth-first", x, y)
		}
		return nil
	})
	sc.Step(`^the parallel run has as many layers as states$`, func() error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		p, err := parallelOf(a)
		if err != nil {
			return err
		}
		n, err := counter(a, "states")
		if err != nil {
			return err
		}
		if p["layers"] != n {
			return fmt.Errorf("%v layers for %v states", p["layers"], n)
		}
		return nil
	})
	sc.Step(`^the parallel run stops with the reason of the breadth-first run$`, func() error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		b, err := get(g8BFS)
		if err != nil {
			return err
		}
		if search(a)["stop"] != search(b)["stop"] {
			return fmt.Errorf("stop %q in parallel, %q breadth-first", search(a)["stop"], search(b)["stop"])
		}
		return nil
	})
	depthIs := func(name string, n int) error {
		r, err := get(name)
		if err != nil {
			return err
		}
		got, err := counter(r, "depth")
		if err != nil {
			return err
		}
		if int(got) != n {
			return fmt.Errorf("the depth of %s is %v, want %d", name, got, n)
		}
		return nil
	}
	sc.Step(`^the parallel run has depth (\d+)$`, func(n int) error { return depthIs(g8Par, n) })
	sc.Step(`^the sequential run has depth (\d+)$`, func(n int) error { return depthIs(g8Seq, n) })
	sc.Step(`^the breadth-first run has depth (\d+)$`, func(n int) error { return depthIs(g8BFS, n) })
	// statusIs checks the status of one property of a named run.
	statusIs := func(name, id, status string) error {
		a, err := get(name)
		if err != nil {
			return err
		}
		p, err := prop(a, id)
		if err != nil {
			return err
		}
		if p["status"] != status {
			return fmt.Errorf("property %q is %v in %s, want %s: %v", id, p["status"], name, status, p["reason"])
		}
		return nil
	}
	sc.Step(`^property "([^"]*)" is "([^"]*)" in the breadth-first run$`, func(id, status string) error { return statusIs(g8BFS, id, status) })
	sc.Step(`^property "([^"]*)" is "([^"]*)" in the second parallel run$`, func(id, status string) error { return statusIs(g8Two, id, status) })
	// stopStartsWith checks the stop reason of a named run.
	stopStartsWith := func(name, prefix string) error {
		a, err := get(name)
		if err != nil {
			return err
		}
		if stop, _ := search(a)["stop"].(string); !strings.HasPrefix(stop, prefix) {
			return fmt.Errorf("stop of %s is %q, which does not start with %q", name, stop, prefix)
		}
		return nil
	}
	sc.Step(`^the breadth-first run stops with a reason starting "([^"]*)"$`, func(prefix string) error { return stopStartsWith(g8BFS, prefix) })
	sc.Step(`^the parallel run stops with the reason of the sequential run$`, func() error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		b, err := get(g8Seq)
		if err != nil {
			return err
		}
		if search(a)["stop"] != search(b)["stop"] {
			return fmt.Errorf("stop %q in parallel, %q sequentially", search(a)["stop"], search(b)["stop"])
		}
		return nil
	})
	sc.Step(`^the parallel run stops with a reason starting "([^"]*)"$`, func(prefix string) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		if stop, _ := search(a)["stop"].(string); !strings.HasPrefix(stop, prefix) {
			return fmt.Errorf("stop %q does not start with %q", stop, prefix)
		}
		return nil
	})
	sc.Step(`^the parallel run is complete$`, func() error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		if search(a)["complete"] != true {
			return fmt.Errorf("search.complete is %v, stop %v", search(a)["complete"], search(a)["stop"])
		}
		return nil
	})
	sc.Step(`^the parallel run is not complete$`, func() error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		if search(a)["complete"] != false {
			return fmt.Errorf("search.complete is %v", search(a)["complete"])
		}
		return nil
	})
	sc.Step(`^the warnings of both runs are equal$`, func() error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		b, err := get(g8Seq)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(a.rep["warnings"], b.rep["warnings"]) {
			return fmt.Errorf("warnings %v in parallel, %v sequentially", a.rep["warnings"], b.rep["warnings"])
		}
		return nil
	})

	// ---- verdicts
	sc.Step(`^every property has the same status and evidence in the parallel and the sequential run$`, func() error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		b, err := get(g8Seq)
		if err != nil {
			return err
		}
		pa, err := props(a)
		if err != nil {
			return err
		}
		pb, err := props(b)
		if err != nil {
			return err
		}
		if len(pa) != len(pb) {
			return fmt.Errorf("%d properties against %d", len(pa), len(pb))
		}
		for id, x := range pa {
			y, ok := pb[id]
			if !ok {
				return fmt.Errorf("property %q missing in the sequential run", id)
			}
			if x["status"] != y["status"] || x["evidence"] != y["evidence"] {
				return fmt.Errorf("property %q: parallel %v/%v, sequential %v/%v", id, x["status"], x["evidence"], y["status"], y["evidence"])
			}
		}
		return nil
	})
	sc.Step(`^property "([^"]*)" is "([^"]*)" in the parallel run$`, func(id, status string) error { return statusIs(g8Par, id, status) })
	sc.Step(`^every property of the parallel run is "([^"]*)"$`, func(status string) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		ps, err := props(a)
		if err != nil {
			return err
		}
		for id, p := range ps {
			if p["status"] != status {
				return fmt.Errorf("property %q is %v, want %s", id, p["status"], status)
			}
		}
		return nil
	})
	sc.Step(`^every property of the parallel run has evidence "([^"]*)"$`, func(ev string) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		ps, err := props(a)
		if err != nil {
			return err
		}
		for id, p := range ps {
			if p["evidence"] != ev {
				return fmt.Errorf("property %q has evidence %v, want %s", id, p["evidence"], ev)
			}
		}
		return nil
	})
	sc.Step(`^no property of the parallel run is "([^"]*)"$`, func(status string) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		ps, err := props(a)
		if err != nil {
			return err
		}
		for id, p := range ps {
			if p["status"] == status && p["kind"] != "reach" {
				return fmt.Errorf("property %q is %s", id, status)
			}
		}
		return nil
	})
	sc.Step(`^no property of the parallel run is "([^"]*)" unless the parallel run is complete$`, func(status string) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		if search(a)["complete"] == true {
			return nil
		}
		ps, err := props(a)
		if err != nil {
			return err
		}
		for id, p := range ps {
			if p["status"] == status && p["kind"] != "reach" {
				return fmt.Errorf("property %q is %s on a run that is not complete (stop %v)", id, status, search(a)["stop"])
			}
		}
		return nil
	})
	sc.Step(`^property "([^"]*)" in the parallel run is "([^"]*)" or "([^"]*)" with a counterexample that replays$`, func(id, s1, s2 string) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		p, err := prop(a, id)
		if err != nil {
			return err
		}
		if p["status"] != s1 && p["status"] != s2 {
			return fmt.Errorf("property %q is %v, want %s or %s", id, p["status"], s1, s2)
		}
		return replays(a, p, model)
	})

	// ---- traces
	sc.Step(`^the counterexample of "([^"]*)" in the parallel run is as long as the breadth-first one$`, func(id string) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		b, err := get(g8BFS)
		if err != nil {
			return err
		}
		x, err := steps(a, id, "counterexample")
		if err != nil {
			return err
		}
		y, err := steps(b, id, "counterexample")
		if err != nil {
			return err
		}
		if x != y {
			return fmt.Errorf("the counterexample has %d steps in parallel, %d breadth-first", x, y)
		}
		return nil
	})
	sc.Step(`^the counterexample of "([^"]*)" in the parallel run has (\d+) steps$`, func(id string, n int) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		got, err := steps(a, id, "counterexample")
		if err != nil {
			return err
		}
		if got != n {
			return fmt.Errorf("the counterexample of %s has %d steps, want %d", id, got, n)
		}
		return nil
	})
	sc.Step(`^the witness of "([^"]*)" in the parallel run has (\d+) steps$`, func(id string, n int) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		got, err := steps(a, id, "witness")
		if err != nil {
			return err
		}
		if got != n {
			return fmt.Errorf("the witness of %s has %d steps, want %d", id, got, n)
		}
		return nil
	})
	sc.Step(`^the counterexample of "([^"]*)" in the parallel run replays as a run of the model$`, func(id string) error {
		a, err := get(g8Par)
		if err != nil {
			return err
		}
		p, err := prop(a, id)
		if err != nil {
			return err
		}
		return replays(a, p, model)
	})

	// ---- determinism
	sc.Step(`^the reports of all parallel runs are equal except for the worker-count fields$`, func() error {
		base, err := get(g8Par)
		if err != nil {
			return err
		}
		want, err := withoutWorkerFields(base)
		if err != nil {
			return err
		}
		n := 0
		for _, name := range []string{g8Two, g8Thr, g8Rep} {
			r := w.runs[name]
			if r == nil {
				continue
			}
			n++
			got, err := withoutWorkerFields(r)
			if err != nil {
				return err
			}
			if got != want {
				return fmt.Errorf("the report of %s differs from that of the parallel run:\n%s\n---\n%s", name, got, want)
			}
		}
		if n == 0 {
			return fmt.Errorf("the scenario has only one parallel run to compare")
		}
		return nil
	})
	identical := func(x, y string) error {
		a, err := get(x)
		if err != nil {
			return err
		}
		b, err := get(y)
		if err != nil {
			return err
		}
		if !bytes.Equal(a.raw, b.raw) {
			return fmt.Errorf("the reports of %s and %s are not byte-identical", x, y)
		}
		return nil
	}
	sc.Step(`^the reports of the third parallel run and the repeated run are byte-identical$`, func() error { return identical(g8Thr, g8Rep) })
	sc.Step(`^the reports of the second parallel run and the repeated run are byte-identical$`, func() error { return identical(g8Two, g8Rep) })
}

// replays checks that the counterexample of a property in a run is a run of the
// model.
func replays(r *g8Run, p map[string]any, model func() (*ir.Model, error)) error {
	ce, ok := p["counterexample"]
	if !ok {
		return fmt.Errorf("property %v has no counterexample", p["id"])
	}
	b, err := json.Marshal(ce)
	if err != nil {
		return err
	}
	var tr cex.Trace
	if err := json.Unmarshal(b, &tr); err != nil {
		return err
	}
	m, err := model()
	if err != nil {
		return err
	}
	if err := replayTrace(m, &tr, p["status"] == "invalid-model"); err != nil {
		return fmt.Errorf("the counterexample of %v does not replay: %v\n%s", p["id"], err, tr.Summary)
	}
	return nil
}
