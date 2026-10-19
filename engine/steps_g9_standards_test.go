package modelcheck_test

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/trace"
	"modelcheck/trace/standards"
)

type g9StandardsWorld struct {
	fixture   string
	document  *trace.Document
	documents []*trace.Document
	err       error
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG9StandardsSteps)
}

func TestG9StandardsImportFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			for _, register := range stepRegistrars {
				register(sc)
			}
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../features/g9-standards-imports.feature"},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run G9 standards-import features")
	}
}

func registerG9StandardsSteps(sc *godog.ScenarioContext) {
	w := &g9StandardsWorld{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g9StandardsWorld{}
		return ctx, nil
	})

	sc.Step(`^the RFC import fixture "([^"]*)"$`, func(name string) error {
		return setStandardsFixture(w, name)
	})
	sc.Step(`^the IEEE clause manifest fixture "([^"]*)"$`, func(name string) error {
		return setStandardsFixture(w, name)
	})
	sc.Step(`^the standards adapter imports the RFC from "([^"]*)"$`, func(sourceURL string) error {
		data, err := os.ReadFile(w.fixture)
		if err != nil {
			return err
		}
		w.document, w.err = standards.ImportRFC(standards.RFCInput{
			Artifact:  standards.Input{Path: filepath.ToSlash(w.fixture), Content: data},
			SourceURL: sourceURL,
		})
		return nil
	})
	sc.Step(`^the standards adapter imports the RFC fixture with logical paths "([^"]*)" and "([^"]*)" from "([^"]*)"$`, func(firstPath, secondPath, sourceURL string) error {
		data, err := os.ReadFile(w.fixture)
		if err != nil {
			return err
		}
		w.documents = make([]*trace.Document, 0, 2)
		for _, logicalPath := range []string{firstPath, secondPath} {
			document, importErr := standards.ImportRFC(standards.RFCInput{
				Artifact:  standards.Input{Path: logicalPath, Content: data},
				SourceURL: sourceURL,
			})
			if importErr != nil {
				w.err = importErr
				return nil
			}
			w.documents = append(w.documents, document)
		}
		return nil
	})
	sc.Step(`^the standards adapter imports the IEEE clause manifest$`, func() error {
		data, err := os.ReadFile(w.fixture)
		if err != nil {
			return err
		}
		w.document, w.err = standards.ImportIEEEManifest(standards.Input{
			Path:    filepath.ToSlash(w.fixture),
			Content: data,
		})
		return nil
	})
	registerStandardsAssertions(sc, w)
}

func setStandardsFixture(w *g9StandardsWorld, name string) error {
	if path.Clean(name) != name || !strings.HasPrefix(name, "standards/") {
		return fmt.Errorf("invalid standards fixture %q", name)
	}
	w.fixture = filepath.Join("testdata", "trace", filepath.FromSlash(name))
	return nil
}

func registerStandardsAssertions(sc *godog.ScenarioContext, w *g9StandardsWorld) {
	sc.Step(`^the standards trace ledger is accepted$`, func() error {
		if w.err != nil {
			return fmt.Errorf("standards trace ledger was rejected: %w", w.err)
		}
		if w.document == nil {
			return fmt.Errorf("standards trace ledger was not produced")
		}
		return nil
	})
	sc.Step(`^the standards trace ledger is rejected with "([^"]*)"$`, func(want string) error {
		if w.err == nil {
			return fmt.Errorf("standards trace ledger was accepted")
		}
		if !strings.Contains(w.err.Error(), want) {
			return fmt.Errorf("rejection %q does not contain %q", w.err, want)
		}
		return nil
	})
	sc.Step(`^the standards trace ledger contains the StandardClause "([^"]*)"$`, func(want string) error {
		if w.document == nil {
			return fmt.Errorf("standards trace ledger was not produced")
		}
		for _, node := range w.document.Nodes {
			if node.Kind == trace.StandardClause && node.Label == want {
				return nil
			}
		}
		return fmt.Errorf("standards trace ledger does not contain StandardClause %q", want)
	})
	sc.Step(`^the standards trace ledger contains the normative candidate "([^"]*)"$`, func(want string) error {
		if w.document == nil {
			return fmt.Errorf("standards trace ledger was not produced")
		}
		for _, evidence := range w.document.Evidence {
			if evidence.Level != trace.NormativeCandidate {
				continue
			}
			for _, node := range w.document.Nodes {
				if node.ID == strings.TrimPrefix(evidence.Subject, "node:") && strings.Contains(node.Label, want) {
					return nil
				}
			}
		}
		return fmt.Errorf("standards trace ledger does not contain normative candidate %q", want)
	})
	sc.Step(`^the standards trace ledger does not contain the normative candidate "([^"]*)"$`, func(want string) error {
		if w.document == nil {
			return fmt.Errorf("standards trace ledger was not produced")
		}
		for _, evidence := range w.document.Evidence {
			if evidence.Level != trace.NormativeCandidate || !strings.HasPrefix(evidence.Subject, "node:") {
				continue
			}
			for _, node := range w.document.Nodes {
				if node.ID == strings.TrimPrefix(evidence.Subject, "node:") && strings.Contains(node.Label, want) {
					return fmt.Errorf("standards trace ledger unexpectedly contains normative candidate %q", want)
				}
			}
		}
		return nil
	})
	sc.Step(`^the standards trace ledger has no normative candidates$`, func() error {
		if w.document == nil {
			return fmt.Errorf("standards trace ledger was not produced")
		}
		for _, evidence := range w.document.Evidence {
			if evidence.Level == trace.NormativeCandidate {
				return fmt.Errorf("standards trace ledger unexpectedly contains normative candidate %q", evidence.ID)
			}
		}
		return nil
	})
	sc.Step(`^the standards trace ledger contains licensed clause reference evidence$`, func() error {
		if w.document == nil {
			return fmt.Errorf("standards trace ledger was not produced")
		}
		for _, evidence := range w.document.Evidence {
			if evidence.Level == trace.LicensedClauseReference {
				return nil
			}
		}
		return fmt.Errorf("standards trace ledger does not contain licensed clause reference evidence")
	})
	sc.Step(`^the RFC trace ledger anchors distinguish the logical input paths$`, func() error {
		if w.err != nil {
			return fmt.Errorf("standards trace ledger was rejected: %w", w.err)
		}
		if len(w.documents) != 2 || len(w.documents[0].Nodes) == 0 || len(w.documents[1].Nodes) == 0 {
			return fmt.Errorf("expected two RFC trace ledgers with a root node")
		}
		first := w.documents[0].Nodes[0].Provenance.Anchor
		second := w.documents[1].Nodes[0].Provenance.Anchor
		if first == second {
			return fmt.Errorf("RFC trace ledger anchors are identical: %q", first)
		}
		if !strings.HasSuffix(first, "#logical-path=one/rfc-8174.txt&line=1") || !strings.HasSuffix(second, "#logical-path=two/rfc-8174.txt&line=1") {
			return fmt.Errorf("RFC trace ledger anchors do not preserve their logical paths: %q, %q", first, second)
		}
		return nil
	})
}
