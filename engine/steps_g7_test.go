package modelcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"modelcheck/cli"
)

// g7Run is one `mcd check` invocation and its parsed report.
type g7Run struct {
	stdout, stderr bytes.Buffer
	exit           int
	rep            map[string]any
}

// g7World is the per-scenario state for features/g7-por.feature. Every step
// goes through cli.Run, the same code path as the mcd binary. A scenario
// holds up to two runs of one model: `main` (the one under test, usually with
// --por) and `base` (the same model without the reduction).
type g7World struct {
	file       string
	main, base *g7Run
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG7Steps)
}

func registerG7Steps(sc *godog.ScenarioContext) {
	w := &g7World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g7World{}
		return ctx, nil
	})

	check := func(args string) (*g7Run, error) {
		flag := "--promela"
		switch filepath.Ext(w.file) {
		case ".json":
			flag = "--ir"
		}
		r := &g7Run{}
		argv := append([]string{"check", flag, w.file}, strings.Fields(args)...)
		r.exit = cli.Run(argv, &r.stdout, &r.stderr)
		if err := json.Unmarshal(r.stdout.Bytes(), &r.rep); err != nil {
			return nil, fmt.Errorf("stdout is not JSON (exit %d): %v\n%s%s", r.exit, err, r.stdout.String(), r.stderr.String())
		}
		return r, nil
	}

	bothStates := func() (a, b int, err error) {
		if a, err = statesOf(w.main); err != nil {
			return 0, 0, err
		}
		b, err = statesOf(w.base)
		return a, b, err
	}

	sc.Step(`^the model under reduction "([^"]*)"$`, func(path string) error {
		w.file = path
		return nil
	})
	sc.Step(`^I check it with "([^"]*)"$`, func(args string) (err error) {
		w.main, err = check(args)
		return err
	})
	sc.Step(`^I check it without reduction with "([^"]*)"$`, func(args string) (err error) {
		w.base, err = check(args)
		return err
	})

	sc.Step(`^the check exits with (\d+)$`, func(code int) error {
		if w.main == nil || w.main.exit != code {
			return fmt.Errorf("exit code %v, want %d", w.main, code)
		}
		return nil
	})
	sc.Step(`^the reduction object is absent$`, func() error {
		if _, ok := searchOf(w.main)["reduction"]; ok {
			return fmt.Errorf("the report carries a reduction object: %v", searchOf(w.main)["reduction"])
		}
		return nil
	})
	sc.Step(`^the reduction was applied$`, func() error {
		red, err := reductionOf(w.main)
		if err != nil {
			return err
		}
		if red["applied"] != true {
			return fmt.Errorf("reduction not applied: %v", red)
		}
		if red["kind"] != "partial-order" {
			return fmt.Errorf("reduction kind %v, want partial-order", red["kind"])
		}
		return nil
	})
	sc.Step(`^the reduction reduced at least one state$`, func() error {
		red, err := reductionOf(w.main)
		if err != nil {
			return err
		}
		if n, _ := red["reduced_states"].(float64); n < 1 {
			return fmt.Errorf("reduced_states = %v: %v", red["reduced_states"], red)
		}
		return nil
	})
	sc.Step(`^the reduction was not applied and its reason mentions "([^"]*)"$`, func(s string) error {
		red, err := reductionOf(w.main)
		if err != nil {
			return err
		}
		if red["applied"] != false {
			return fmt.Errorf("reduction applied: %v", red)
		}
		if reason, _ := red["reason"].(string); !strings.Contains(reason, s) {
			return fmt.Errorf("reason %q does not mention %q", reason, s)
		}
		return nil
	})

	sc.Step(`^the reduced run stores at most (\d+) states$`, func(n int) error {
		got, err := statesOf(w.main)
		if err != nil {
			return err
		}
		if got > n {
			return fmt.Errorf("the reduced run stores %d states, want at most %d", got, n)
		}
		return nil
	})
	sc.Step(`^the baseline run stores (\d+) states$`, func(n int) error {
		got, err := statesOf(w.base)
		if err != nil {
			return err
		}
		if got != n {
			return fmt.Errorf("the baseline run stores %d states, want %d", got, n)
		}
		return nil
	})
	sc.Step(`^the reduced run stores fewer states than the baseline run$`, func() error {
		a, b, err := bothStates()
		if err != nil {
			return err
		}
		if a >= b {
			return fmt.Errorf("reduced %d states, baseline %d: not fewer", a, b)
		}
		return nil
	})
	sc.Step(`^the reduced run stores as many states as the baseline run$`, func() error {
		a, b, err := bothStates()
		if err != nil {
			return err
		}
		if a != b {
			return fmt.Errorf("reduced %d states, baseline %d: not equal", a, b)
		}
		return nil
	})
	sc.Step(`^every property has the same status and evidence in both runs$`, func() error {
		a, err := propsOf(w.main)
		if err != nil {
			return err
		}
		b, err := propsOf(w.base)
		if err != nil {
			return err
		}
		if len(a) != len(b) {
			return fmt.Errorf("%d properties against %d", len(a), len(b))
		}
		for id, pa := range a {
			pb, ok := b[id]
			if !ok {
				return fmt.Errorf("property %q missing in the baseline run", id)
			}
			if pa["status"] != pb["status"] || pa["evidence"] != pb["evidence"] {
				return fmt.Errorf("property %q: reduced %v/%v, baseline %v/%v", id, pa["status"], pa["evidence"], pb["status"], pb["evidence"])
			}
		}
		return nil
	})
	sc.Step(`^property "([^"]*)" is "([^"]*)" in the reduced run$`, func(id, status string) error {
		props, err := propsOf(w.main)
		if err != nil {
			return err
		}
		p, ok := props[id]
		if !ok {
			return fmt.Errorf("no property %q", id)
		}
		if p["status"] != status {
			return fmt.Errorf("property %q is %v, want %s: %v", id, p["status"], status, p["reason"])
		}
		return nil
	})
	sc.Step(`^property "([^"]*)" is "([^"]*)" in the baseline run$`, func(id, status string) error {
		props, err := propsOf(w.base)
		if err != nil {
			return err
		}
		p, ok := props[id]
		if !ok {
			return fmt.Errorf("no property %q", id)
		}
		if p["status"] != status {
			return fmt.Errorf("property %q is %v in the baseline run, want %s: %v", id, p["status"], status, p["reason"])
		}
		return nil
	})
	sc.Step(`^the reduced run has a counterexample for "([^"]*)"$`, func(id string) error {
		props, err := propsOf(w.main)
		if err != nil {
			return err
		}
		ce, ok := props[id]["counterexample"].(map[string]any)
		if !ok {
			return fmt.Errorf("property %q has no counterexample", id)
		}
		if steps, _ := ce["steps"].([]any); len(steps) == 0 {
			return fmt.Errorf("the counterexample of %q has no steps", id)
		}
		return nil
	})

}

func searchOf(r *g7Run) map[string]any {
	if r == nil {
		return nil
	}
	s, _ := r.rep["search"].(map[string]any)
	return s
}

func reductionOf(r *g7Run) (map[string]any, error) {
	red, ok := searchOf(r)["reduction"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("the report has no search.reduction object")
	}
	return red, nil
}

func propsOf(r *g7Run) (map[string]map[string]any, error) {
	if r == nil {
		return nil, fmt.Errorf("the scenario has not run that check yet")
	}
	list, ok := r.rep["properties"].([]any)
	if !ok {
		return nil, fmt.Errorf("the report has no properties: %s", r.stdout.String())
	}
	out := map[string]map[string]any{}
	for _, p := range list {
		pm := p.(map[string]any)
		out[pm["id"].(string)] = pm
	}
	return out, nil
}

// statesOf is the state count of a run: the counters every state property
// of a report shares.
func statesOf(r *g7Run) (int, error) {
	if r == nil {
		return 0, fmt.Errorf("the scenario has not run that check yet")
	}
	list, ok := r.rep["properties"].([]any)
	if !ok || len(list) == 0 {
		return 0, fmt.Errorf("the report has no properties: %s", r.stdout.String())
	}
	c := list[0].(map[string]any)["counters"].(map[string]any)
	return int(c["states"].(float64)), nil
}
