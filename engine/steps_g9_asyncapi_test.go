package modelcheck_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/trace"
	"modelcheck/trace/protocol"
)

func TestG9AsyncAPIFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: registerG9AsyncAPISteps,
		Options:             &godog.Options{Format: "pretty", Paths: []string{"../features/g9-asyncapi.feature"}, Strict: true, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run AsyncAPI feature tests")
	}
}

func init() { stepRegistrars = append(stepRegistrars, registerG9AsyncAPISteps) }

func registerG9AsyncAPISteps(sc *godog.ScenarioContext) {
	var ledger *trace.Document
	var output []byte
	var analysisErr error
	input := func(data string) error {
		content := []byte(data)
		digest := sha256.Sum256(content)
		output, analysisErr = protocol.AnalyzeAsyncAPI(protocol.ContractInput{Artifact: protocol.Input{Path: "features/asyncapi.json", Content: content}, ContentSHA256: hex.EncodeToString(digest[:])})
		return analysisErr
	}
	checkLedger := func() error {
		var err error
		ledger, err = trace.UnmarshalJSON(output)
		return err
	}
	sc.Step(`^a valid AsyncAPI 2.6 declaration document$`, func() error {
		return input(`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"publish":{"message":{"name":"Notice","payload":{"type":"object","properties":{"value":{"type":"string"}}}}}}}}`)
	})
	sc.Step(`^a valid AsyncAPI 3 declaration with an action$`, func() error {
		return input(`{"asyncapi":"3.0.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"messages":{"notice":{"name":"Notice"}}}},"operations":{"receive":{"action":"receive","channel":{"$ref":"#/channels/updates"}}}}`)
	})
	sc.Step(`^an AsyncAPI 3 declaration with a reference-only schema child "([^"]*)"$`, func(keyword string) error {
		return input(`{"asyncapi":"3.0.0","info":{"title":"Demo","version":"1"},"channels":{},"components":{"schemas":{"List":{"` + keyword + `":{"$ref":"#/components/schemas/Item"}},"Item":{"type":"string"}}}}`)
	})
	sc.Step(`^an AsyncAPI 2.6 declaration with duplicate operation IDs$`, func() error {
		content := `{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"a":{"publish":{"operationId":"same","message":{"name":"A"}}},"b":{"publish":{"operationId":"same","message":{"name":"B"}}}}}`
		sum := sha256.Sum256([]byte(content))
		output, analysisErr = protocol.AnalyzeAsyncAPI(protocol.ContractInput{Artifact: protocol.Input{Path: "features/asyncapi.json", Content: []byte(content)}, ContentSHA256: hex.EncodeToString(sum[:])})
		return nil
	})
	sc.Step(`^I analyse the AsyncAPI declaration$`, func() error {
		if analysisErr != nil {
			return nil
		}
		return checkLedger()
	})
	sc.Step(`^the AsyncAPI ledger contains only declaration containment facts$`, func() error {
		for _, edge := range ledger.Edges {
			if edge.Kind != trace.DerivedFrom {
				return fmt.Errorf("unexpected edge kind %q", edge.Kind)
			}
		}
		return nil
	})
	sc.Step(`^the AsyncAPI ledger has a declarations-only gap$`, func() error { return asyncAPIFeatureGap(ledger, "asyncapi.declarations_only") })
	sc.Step(`^the AsyncAPI ledger has an operation-unmapped gap$`, func() error { return asyncAPIFeatureGap(ledger, "asyncapi.operation_unmapped") })
	sc.Step(`^the AsyncAPI ledger has no duplicate gaps$`, func() error {
		if analysisErr != nil {
			return analysisErr
		}
		seen := map[string]bool{}
		for _, gap := range ledger.Gaps {
			key := gap.Provenance.Anchor + "\x00" + gap.Kind
			if seen[key] {
				return fmt.Errorf("duplicate gap %s", key)
			}
			seen[key] = true
		}
		return nil
	})
	sc.Step(`^AsyncAPI analysis fails with rule "([^"]*)"$`, func(code string) error {
		var rule *protocol.RuleError
		if output != nil || !errors.As(analysisErr, &rule) || rule.Code != code {
			return fmt.Errorf("output=%d err=%v want=%s", len(output), analysisErr, code)
		}
		return nil
	})
	sc.Step(`^the AsyncAPI ledger contains no behavior edge$`, func() error {
		for _, edge := range ledger.Edges {
			if edge.Kind == trace.Sends || edge.Kind == trace.Receives {
				return fmt.Errorf("inferred behavior edge %q", edge.ID)
			}
		}
		return nil
	})
}

func asyncAPIFeatureGap(document *trace.Document, kind string) error {
	for _, gap := range document.Gaps {
		if gap.Kind == kind {
			return nil
		}
	}
	return fmt.Errorf("missing %s", kind)
}
