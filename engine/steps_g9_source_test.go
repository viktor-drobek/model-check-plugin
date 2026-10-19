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
	"modelcheck/trace/source"
)

type g9SourceWorld struct {
	fixture  string
	document *trace.Document
	err      error
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG9SourceSteps)
}

func TestG9SourceAdapterFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			for _, register := range stepRegistrars {
				register(sc)
			}
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../features/g9-source-adapters.feature"},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run G9 source-adapter features")
	}
}

func registerG9SourceSteps(sc *godog.ScenarioContext) {
	w := &g9SourceWorld{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g9SourceWorld{}
		return ctx, nil
	})

	sc.Step(`^the Go source adapter fixture "([^"]*)"$`, func(name string) error {
		if filepath.Clean(name) != name || !strings.HasPrefix(name, "source/go/") {
			return fmt.Errorf("invalid Go source fixture %q", name)
		}
		w.fixture = filepath.Join("testdata", name)
		return nil
	})
	sc.Step(`^the Go source adapter analyses the fixture$`, func() error {
		data, err := os.ReadFile(w.fixture)
		if err != nil {
			return err
		}
		w.document, w.err = source.AnalyzeGo(source.Input{
			Path:    filepath.ToSlash(w.fixture),
			Content: data,
		})
		return nil
	})
	sc.Step(`^the source trace ledger is accepted$`, func() error {
		if w.err != nil {
			return fmt.Errorf("source trace ledger was rejected: %w", w.err)
		}
		if w.document == nil {
			return fmt.Errorf("source trace ledger was not produced")
		}
		return nil
	})
	sc.Step(`^the source trace ledger is rejected with "([^"]*)"$`, func(want string) error {
		if w.err == nil {
			return fmt.Errorf("source trace ledger was accepted")
		}
		if !strings.Contains(w.err.Error(), want) {
			return fmt.Errorf("rejection %q does not contain %q", w.err, want)
		}
		return nil
	})
	sc.Step(`^the source trace ledger contains the SourceSymbol "([^"]*)"$`, func(want string) error {
		for _, node := range w.document.Nodes {
			if node.Kind == trace.SourceSymbol && node.Label == want {
				return nil
			}
		}
		return fmt.Errorf("source trace ledger does not contain SourceSymbol %q", want)
	})
	sc.Step(`^the source trace ledger contains the syntactic call "([^"]*)" to "([^"]*)"$`, func(caller, callee string) error {
		for _, edge := range w.document.Edges {
			if edge.Kind != trace.Calls {
				continue
			}
			if sourceLabel(w.document, edge.Source) == caller && sourceLabel(w.document, edge.Target) == callee {
				return nil
			}
		}
		return fmt.Errorf("source trace ledger does not contain syntactic call %q to %q", caller, callee)
	})
	sc.Step(`^the source trace ledger does not contain a syntactic call "([^"]*)" to "([^"]*)"$`, func(caller, callee string) error {
		for _, edge := range w.document.Edges {
			if edge.Kind == trace.Calls && sourceLabel(w.document, edge.Source) == caller && sourceLabel(w.document, edge.Target) == callee {
				return fmt.Errorf("source trace ledger unexpectedly contains syntactic call %q to %q", caller, callee)
			}
		}
		return nil
	})
	sc.Step(`^the source trace ledger contains a gap with "([^"]*)"$`, func(want string) error {
		for _, gap := range w.document.Gaps {
			if strings.Contains(gap.Reason, want) {
				return nil
			}
		}
		return fmt.Errorf("source trace ledger does not contain a gap with %q", want)
	})
	sc.Step(`^the source trace ledger contains a gap for the call "([^"]*)"$`, func(name string) error {
		want := fmt.Sprintf("call target %q", name)
		for _, gap := range w.document.Gaps {
			if strings.Contains(gap.Reason, want) {
				return nil
			}
		}
		return fmt.Errorf("source trace ledger does not contain a gap for call %q", name)
	})
	sc.Step(`^the source trace ledger has no gaps$`, func() error {
		if len(w.document.Gaps) != 0 {
			return fmt.Errorf("source trace ledger has %d gaps", len(w.document.Gaps))
		}
		return nil
	})
}

func sourceLabel(document *trace.Document, id string) string {
	for _, node := range document.Nodes {
		if node.ID == id {
			return node.Label
		}
	}
	return ""
}
