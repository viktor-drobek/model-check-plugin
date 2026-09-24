package modelcheck_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"modelcheck/internal/spike"
)

// spikeWorld is the per-scenario state for features/spike-explorer.feature.
type spikeWorld struct {
	opts    spike.Options
	model   spike.Model
	res     *spike.Result
	stats   *spike.Stats
	reports [][]byte
}

func init() {
	stepRegistrars = append(stepRegistrars, registerSpikeSteps)
}

func registerSpikeSteps(sc *godog.ScenarioContext) {
	w := &spikeWorld{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = spikeWorld{}
		return ctx, nil
	})

	sc.Step(`^the spike explorer with a budget of (\d+) states and (\d+) seconds$`, func(states, secs int) error {
		w.opts = spike.Options{MaxStates: states, Timeout: time.Duration(secs) * time.Second}
		return nil
	})
	loadModel := func(name string) error {
		m, err := spike.ByName(name)
		if err != nil {
			return err
		}
		w.model = m
		return nil
	}
	sc.Step(`^the hard-coded model "petrinet1" with initial marking p1=1 p4=1$`, func() error {
		if err := loadModel("petrinet1"); err != nil {
			return err
		}
		if got := w.model.Describe(w.model.Initial()); got != "p1=1 p4=1" {
			return fmt.Errorf("initial marking is %q", got)
		}
		return nil
	})
	sc.Step(`^the hard-coded model "mutex_flaw" with two user processes$`, func() error {
		if err := loadModel("mutex_flaw"); err != nil {
			return err
		}
		vars := w.model.Vars(w.model.Initial())
		if len(vars) != 6 || vars[4].Name != "pc0" || vars[5].Name != "pc1" {
			return fmt.Errorf("expected two program counters, got %v", vars)
		}
		return nil
	})
	sc.Step(`^the hard-coded model "([^"]*)"$`, loadModel)

	sc.Step(`^I run the explorer$`, func() error {
		w.res, w.stats = spike.Explore(w.model, w.opts)
		return nil
	})
	sc.Step(`^I run the explorer continuing after violations$`, func() error {
		opts := w.opts
		opts.ContinueAfterViolation = true
		w.res, w.stats = spike.Explore(w.model, opts)
		return nil
	})
	sc.Step(`^the number of violations seen is (\d+)$`, func(n int) error {
		if w.res.Violations != n {
			return fmt.Errorf("violations=%d", w.res.Violations)
		}
		return nil
	})
	sc.Step(`^I run the explorer twice$`, func() error {
		for i := 0; i < 2; i++ {
			r, _ := spike.Explore(w.model, w.opts)
			w.reports = append(w.reports, r.JSON())
		}
		return nil
	})

	sc.Step(`^the status is "([^"]*)" with violation kind "([^"]*)"$`, func(status, kind string) error {
		if w.res.Status != status || w.res.Violation != kind {
			return fmt.Errorf("got status=%s violation=%s reason=%q", w.res.Status, w.res.Violation, w.res.Reason)
		}
		return nil
	})
	sc.Step(`^the status is "([^"]*)" with reason "([^"]*)"$`, func(status, reason string) error {
		if w.res.Status != status || w.res.Reason != reason {
			return fmt.Errorf("got status=%s reason=%q", w.res.Status, w.res.Reason)
		}
		return nil
	})
	sc.Step(`^the status is "([^"]*)"$`, func(status string) error {
		if w.res.Status != status {
			return fmt.Errorf("got status=%s violation=%s reason=%q", w.res.Status, w.res.Violation, w.res.Reason)
		}
		return nil
	})
	sc.Step(`^the evidence is "([^"]*)"$`, func(ev string) error {
		if w.res.Evidence != ev {
			return fmt.Errorf("got evidence=%s", w.res.Evidence)
		}
		return nil
	})
	sc.Step(`^the witness is the transition sequence "([^"]*)"$`, func(seq string) error {
		var labels []string
		for _, s := range w.res.Witness {
			labels = append(labels, s.Label)
		}
		if got := strings.Join(labels, ", "); got != seq {
			return fmt.Errorf("witness is %q", got)
		}
		return nil
	})
	sc.Step(`^the witness is not empty$`, func() error {
		if len(w.res.Witness) == 0 {
			return fmt.Errorf("empty witness")
		}
		return nil
	})
	sc.Step(`^the final state is described as "([^"]*)"$`, func(desc string) error {
		if w.res.FinalState != desc {
			return fmt.Errorf("final state is %q", w.res.FinalState)
		}
		return nil
	})
	sc.Step(`^the final state has variable "([^"]*)" equal to (\d+)$`, func(name string, val int) error {
		for _, v := range w.res.FinalVars {
			if v.Name == name {
				if v.Value != val {
					return fmt.Errorf("%s=%d", name, v.Value)
				}
				return nil
			}
		}
		return fmt.Errorf("no variable %q in %v", name, w.res.FinalVars)
	})
	sc.Step(`^the violated statement is "([^"]*)"$`, func(stmt string) error {
		if w.res.Statement != stmt {
			return fmt.Errorf("statement is %q", w.res.Statement)
		}
		return nil
	})
	sc.Step(`^the number of stored states is (\d+)$`, func(n int) error {
		if w.res.States != n {
			return fmt.Errorf("states=%d", w.res.States)
		}
		return nil
	})
	sc.Step(`^replaying the witness from the initial state reproduces the final state$`, func() error {
		s, err := spike.Replay(w.model, w.res.Witness)
		if err != nil {
			return err
		}
		if got := w.model.Describe(s); got != w.res.FinalState {
			return fmt.Errorf("replay reaches %q, report says %q", got, w.res.FinalState)
		}
		return nil
	})
	sc.Step(`^the run finished within the budget$`, func() error {
		if w.stats.BudgetHit {
			return fmt.Errorf("budget hit: %s", w.res.Reason)
		}
		if w.opts.Timeout > 0 && w.stats.Elapsed > w.opts.Timeout {
			return fmt.Errorf("elapsed %s > %s", w.stats.Elapsed, w.opts.Timeout)
		}
		return nil
	})
	sc.Step(`^both JSON reports are byte-identical$`, func() error {
		if len(w.reports) != 2 {
			return fmt.Errorf("have %d reports", len(w.reports))
		}
		if !bytes.Equal(w.reports[0], w.reports[1]) {
			return fmt.Errorf("reports differ:\n%s\n---\n%s", w.reports[0], w.reports[1])
		}
		return nil
	})
}
