package modelcheck_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/trace"
)

type g9TraceWorld struct {
	document *trace.Document
	err      error
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG9TraceSteps)
}

func TestG9TraceFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			for _, register := range stepRegistrars {
				register(sc)
			}
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../features/g9-trace-ledger.feature"},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run G9 trace-ledger features")
	}
}

func registerG9TraceSteps(sc *godog.ScenarioContext) {
	w := &g9TraceWorld{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g9TraceWorld{}
		return ctx, nil
	})

	sc.Step(`^the trace ledger fixture "([^"]*)"$`, func(name string) error {
		if filepath.Clean(name) != name || !strings.HasPrefix(name, "trace/") {
			return fmt.Errorf("invalid trace fixture %q", name)
		}
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			return err
		}
		w.document, w.err = trace.UnmarshalJSON(data)
		return nil
	})
	sc.Step(`^the trace ledger is accepted$`, func() error {
		if w.err != nil {
			return fmt.Errorf("trace ledger was rejected: %w", w.err)
		}
		if w.document == nil {
			return fmt.Errorf("trace ledger was not loaded")
		}
		return nil
	})
	sc.Step(`^the trace ledger is rejected with "([^"]*)"$`, func(want string) error {
		if w.err == nil {
			return fmt.Errorf("trace ledger was accepted")
		}
		if !strings.Contains(w.err.Error(), want) {
			return fmt.Errorf("rejection %q does not contain %q", w.err, want)
		}
		return nil
	})
	sc.Step(`^the trace ledger keeps evidence level "([^"]*)" distinct from "([^"]*)"$`, func(first, second string) error {
		if w.document == nil {
			return fmt.Errorf("trace ledger was not loaded")
		}
		levels := map[string]bool{}
		levelsBySubject := map[string]map[string]bool{}
		for _, evidence := range w.document.Evidence {
			level := string(evidence.Level)
			levels[level] = true
			if levelsBySubject[evidence.Subject] == nil {
				levelsBySubject[evidence.Subject] = map[string]bool{}
			}
			levelsBySubject[evidence.Subject][level] = true
		}
		if !levels[first] || !levels[second] {
			return fmt.Errorf("levels %q and %q are not both present: %v", first, second, levels)
		}
		if first == second {
			return fmt.Errorf("evidence levels must differ")
		}
		for subject, subjectLevels := range levelsBySubject {
			if subjectLevels[first] && subjectLevels[second] {
				return fmt.Errorf("levels %q and %q are attached to the same subject %q", first, second, subject)
			}
		}
		return nil
	})
	sc.Step(`^the trace report names the gap "([^"]*)"$`, func(gapID string) error {
		if w.document == nil {
			return fmt.Errorf("trace ledger was not loaded")
		}
		report, err := trace.Markdown(w.document)
		if err != nil {
			return err
		}
		if !strings.Contains(string(report), gapID) {
			return fmt.Errorf("trace report does not name %q", gapID)
		}
		return nil
	})
	sc.Step(`^the trace report states that a model verdict is not transferred to an implementation claim$`, func() error {
		if w.document == nil {
			return fmt.Errorf("trace ledger was not loaded")
		}
		report, err := trace.Markdown(w.document)
		if err != nil {
			return err
		}
		if !strings.Contains(string(report), "does not transfer a model verdict") {
			return fmt.Errorf("trace report does not state its implementation boundary")
		}
		return nil
	})
	sc.Step(`^the trace SARIF report preserves the reference "([^"]*)" without a region$`, func(reference string) error {
		if w.document == nil {
			return fmt.Errorf("trace ledger was not loaded")
		}
		report, err := trace.SARIF(w.document)
		if err != nil {
			return err
		}
		text := string(report)
		if !strings.Contains(text, `"uri": "`+reference+`"`) {
			return fmt.Errorf("trace SARIF report does not preserve reference %q:\n%s", reference, text)
		}
		if !strings.Contains(text, `"reference": "`+reference+`"`) {
			return fmt.Errorf("trace SARIF report does not preserve raw reference %q:\n%s", reference, text)
		}
		if strings.Contains(text, `"region":`) {
			return fmt.Errorf("trace SARIF report invents a region for reference %q:\n%s", reference, text)
		}
		return nil
	})
}
