package modelcheck_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/trace"
	"modelcheck/trace/protocol"
)

func init() { stepRegistrars = append(stepRegistrars, registerG9PactSteps) }

func TestG9PactFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: registerG9PactSteps,
		Options:             &godog.Options{Format: "pretty", Paths: []string{"../features/g9-pact.feature"}, Strict: true, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("Pact feature tests failed")
	}
}

func registerG9PactSteps(sc *godog.ScenarioContext) {
	var input protocol.ContractInput
	var first, second []byte
	var document *trace.Document
	var adapterErr error

	setInput := func(body string) {
		content := []byte(body)
		input = protocol.ContractInput{Artifact: protocol.Input{Path: "bdd/example.pact.json", Content: content}, ContentSHA256: fmt.Sprintf("%x", sha256.Sum256(content))}
	}
	sc.Step(`^a valid Pact v4 HTTP declaration$`, func() error {
		setInput(`{"consumer":{"name":"consumer"},"provider":{"name":"provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"find-widget","type":"Synchronous/HTTP","description":"find widget","request":{"method":"GET","path":"/widgets/1"},"response":{"status":200}}]}`)
		return nil
	})
	sc.Step(`^a valid Pact v4 message declaration$`, func() error {
		setInput(`{"consumer":{"name":"consumer"},"provider":{"name":"provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"widget-created","type":"Asynchronous/Messages","description":"widget created","contents":{"widget":"one"}}]}`)
		return nil
	})
	sc.Step(`^a Pact declaration with an unsafe route$`, func() error {
		setInput(`{"consumer":{"name":"consumer"},"provider":{"name":"provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"unsafe","type":"Synchronous/HTTP","description":"unsafe","request":{"method":"GET","path":"/../etc"},"response":{"status":200}}]}`)
		return nil
	})
	sc.Step(`^the Pact declaration is adapted twice$`, func() error {
		var err error
		first, err = protocol.AnalyzePact(input)
		if err != nil {
			return err
		}
		second, err = protocol.AnalyzePact(input)
		if err != nil {
			return err
		}
		document, err = trace.UnmarshalJSON(first)
		return err
	})
	sc.Step(`^the Pact declaration is adapted$`, func() error {
		first, adapterErr = protocol.AnalyzePact(input)
		if adapterErr == nil {
			document, adapterErr = trace.UnmarshalJSON(first)
		}
		return nil
	})
	sc.Step(`^the Pact ledgers are byte reproducible$`, func() error {
		if !bytes.Equal(first, second) {
			return fmt.Errorf("Pact output is not reproducible")
		}
		return nil
	})
	sc.Step(`^the Pact ledger contains one endpoint and two direct messages$`, func() error {
		var endpoints, messages int
		for _, node := range document.Nodes {
			if node.Kind == trace.Endpoint {
				endpoints++
			}
			if node.Kind == trace.ProtocolMessage {
				messages++
			}
		}
		if endpoints != 1 || messages != 2 {
			return fmt.Errorf("want 1 endpoint and 2 messages, got %d and %d", endpoints, messages)
		}
		return nil
	})
	sc.Step(`^all Pact evidence is contract only$`, func() error {
		for _, evidence := range document.Evidence {
			if evidence.Level != trace.Contract {
				return fmt.Errorf("non-contract evidence %q", evidence.Level)
			}
		}
		return nil
	})
	sc.Step(`^the Pact ledger contains no inferred message facts$`, func() error {
		if len(document.Nodes) != 0 || len(document.Edges) != 0 || len(document.Evidence) != 0 {
			return fmt.Errorf("message declaration produced facts")
		}
		return nil
	})
	sc.Step(`^the Pact ledger discloses the unmapped message declaration$`, func() error {
		for _, gap := range document.Gaps {
			if gap.Kind == "pact.unmapped_declaration" {
				return nil
			}
		}
		return fmt.Errorf("missing unmapped message gap")
	})
	sc.Step(`^the Pact declaration is rejected with "([^"]*)"$`, func(code string) error {
		if first != nil {
			return fmt.Errorf("unexpected output")
		}
		rule, ok := adapterErr.(*protocol.RuleError)
		if !ok || rule.Code != "stage4."+code {
			return fmt.Errorf("want stage4.%s, got %v", code, adapterErr)
		}
		return nil
	})
	sc.Step(`^the Pact adapter returns no partial ledger$`, func() error {
		if first != nil {
			return fmt.Errorf("partial ledger returned")
		}
		return nil
	})
}
