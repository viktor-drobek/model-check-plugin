package modelcheck_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/trace"
	"modelcheck/trace/protocol"
)

func TestG9OpenAPIFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: registerG9OpenAPISteps,
		Options: &godog.Options{
			Format: "pretty", Paths: []string{"../features/g9-openapi.feature"}, Strict: true, TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run G9 OpenAPI features")
	}
}

type g9OpenAPIState struct {
	input  protocol.ContractInput
	output []byte
	err    error
}

func init() { stepRegistrars = append(stepRegistrars, registerG9OpenAPISteps) }

func registerG9OpenAPISteps(sc *godog.ScenarioContext) {
	state := &g9OpenAPIState{}
	sc.Step(`^a supported OpenAPI declaration document$`, func() error {
		state.input = g9OpenAPIInput(`{"openapi":"3.1.0","info":{"title":"G9","version":"1"},"paths":{"/health":{"get":{"responses":{"200":{}}}}}}`)
		return nil
	})
	sc.Step(`^an OpenAPI declaration document with version "([^"]*)"$`, func(version string) error {
		state.input = g9OpenAPIInput(fmt.Sprintf(`{"openapi":%q,"info":{"title":"G9","version":"1"},"paths":{}}`, version))
		return nil
	})
	sc.Step(`^an OpenAPI declaration with aliased path templates$`, func() error {
		state.input = g9OpenAPIInput(`{"openapi":"3.1.0","info":{"title":"G9","version":"1"},"paths":{"/a/{id}":{"get":{"responses":{"200":{}}}},"/a/{name}":{"get":{"responses":{"200":{}}}}}}`)
		return nil
	})
	sc.Step(`^I analyze the OpenAPI declaration document$`, func() {
		state.output, state.err = protocol.AnalyzeOpenAPI(state.input)
	})
	sc.Step(`^the OpenAPI ledger contains declaration facts only$`, func() error {
		if state.err != nil {
			return state.err
		}
		document, err := trace.UnmarshalJSON(state.output)
		if err != nil {
			return err
		}
		if len(document.Nodes) != 1 || len(document.Evidence) != 1 || document.Evidence[0].Level != trace.Contract {
			return fmt.Errorf("ledger does not contain endpoint contract evidence only")
		}
		return nil
	})
	sc.Step(`^OpenAPI analysis fails with rule "([^"]*)"$`, func(code string) error {
		if state.output != nil {
			return fmt.Errorf("analysis output was not nil")
		}
		if state.err == nil || state.err.Error() != code {
			return fmt.Errorf("error = %v, want %s", state.err, code)
		}
		return nil
	})
}

func g9OpenAPIInput(source string) protocol.ContractInput {
	sum := sha256.Sum256([]byte(source))
	return protocol.ContractInput{
		Artifact:      protocol.Input{Path: "features/openapi.json", Content: []byte(source)},
		ContentSHA256: hex.EncodeToString(sum[:]),
	}
}
