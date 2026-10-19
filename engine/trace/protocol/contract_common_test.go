package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"modelcheck/trace"
)

func contractTestInput(body string) ContractInput {
	data := []byte(body)
	digest := sha256.Sum256(data)
	return ContractInput{Artifact: Input{Path: "contracts/example.json", Content: data}, ContentSHA256: hex.EncodeToString(digest[:])}
}

func requireContractRule(t *testing.T, output []byte, err error, code string) {
	t.Helper()
	var got *RuleError
	if len(output) != 0 || !errors.As(err, &got) || got == nil || got.Code != "stage4."+code {
		t.Fatalf("output=%d error=%v; want stage4.%s and no output", len(output), err, code)
	}
	switch got.Locator {
	case "input", "metadata", "digest", "json", "schema", "budget":
	default:
		t.Fatalf("unsafe locator %q", got.Locator)
	}
}

func TestContractHelpersObjectNameAndPointer(t *testing.T) {
	object, err := contractObject(map[string]any{"name": "packet"}, "name")
	if err != nil || object["name"] != "packet" {
		t.Fatalf("contractObject: object=%#v err=%v", object, err)
	}
	if _, err := contractObject(map[string]any{"extra": true}, "name"); err == nil {
		t.Fatal("unknown key accepted")
	}
	for _, name := range []string{"A", "packet_1", "with.dot", "colon:ok", "dash-ok"} {
		if !contractName(name) {
			t.Fatalf("valid contract name rejected: %q", name)
		}
	}
	for _, name := range []string{"", " bad", "../bad", "a/b", string(bytes.Repeat([]byte("a"), 201))} {
		if contractName(name) {
			t.Fatalf("unsafe contract name accepted: %q", name)
		}
	}
	if got, want := contractPointer("components", "a/b", "x~y"), "#/components/a~1b/x~0y"; got != want {
		t.Fatalf("pointer=%q want %q", got, want)
	}
}

func TestContractJSONRejectsUnsafeValuesAndReferences(t *testing.T) {
	cases := []string{
		`{"metadata":{"api_key":"value"}}`,
		`{"metadata":"/home/tester/private"}`,
		`{"metadata":"C:\\secret"}`,
		`{"metadata":"file:///etc/passwd"}`,
		`{"value":{"$ref":"https://example.invalid/schema"}}`,
		`{"value":{"$ref":"#/missing"}}`,
		`{"a":{"$ref":"#/b"},"b":{"$ref":"#/a"}}`,
		`{"a":{"$ref":"#/a/value"},"value":1}`,
	}
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			if _, err := contractJSON([]byte(body)); err == nil {
				t.Fatal("unsafe JSON accepted")
			} else {
				var rule *RuleError
				if !errors.As(err, &rule) || rule.Code[:7] != "stage4." {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
	if _, err := contractJSON([]byte(`{"route":"/v1/foo","components":{"packet":{"type":"object"}},"use":{"$ref":"#/components/packet"}}`)); err != nil {
		t.Fatalf("safe local reference rejected: %v", err)
	}
}

func TestContractRunEmitsDeterministicContractLedger(t *testing.T) {
	input := contractTestInput(`{"openapi":"3.0.3"}`)
	parse := func([]byte) (contractPlan, error) {
		return contractPlan{
			facts: []contractFact{
				{key: "message", label: "Packet", kind: trace.ProtocolMessage},
				{key: "field", label: "Version", parent: "message", kind: trace.BinaryField},
				{key: "endpoint", label: "GET /v1/packets", kind: trace.Endpoint},
			},
			gaps: []contractGap{{key: "#", kind: "openapi.declarations_only", reason: "declarations are not execution evidence"}},
		}, nil
	}
	first, err := contractRun(input, "openapi", defaultBudget, parse)
	if err != nil {
		t.Fatal(err)
	}
	second, err := contractRun(input, "openapi", defaultBudget, parse)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("nondeterministic output: %v", err)
	}
	document, err := trace.UnmarshalJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	if document.Schema != trace.Schema || len(document.Nodes) != 3 || len(document.Edges) != 1 || len(document.Evidence) != 4 || len(document.Gaps) != 1 {
		t.Fatalf("unexpected ledger counts: %+v", document)
	}
	for _, evidence := range document.Evidence {
		if evidence.Level != trace.Contract || evidence.Provenance.Producer != (trace.Producer{Name: "mcd-openapi-adapter", Version: "1"}) {
			t.Fatalf("bad evidence: %+v", evidence)
		}
	}
	if canonical, err := trace.JSON(document); err != nil || !bytes.Equal(first, canonical) {
		t.Fatalf("non-canonical ledger: %v", err)
	}
}

func TestContractRunFailsClosedForPlanAndBudget(t *testing.T) {
	input := contractTestInput(`{"openapi":"3.0.3"}`)
	for _, plan := range []contractPlan{
		{facts: []contractFact{{key: "same", label: "one", kind: trace.Endpoint}, {key: "same", label: "two", kind: trace.Endpoint}}, gaps: []contractGap{{key: "#", kind: "openapi.declarations_only", reason: "declarations only"}}},
		{facts: []contractFact{{key: "child", label: "child", parent: "missing", kind: trace.BinaryField}}, gaps: []contractGap{{key: "#", kind: "openapi.declarations_only", reason: "declarations only"}}},
		{facts: []contractFact{{key: "bad", label: "bad", kind: trace.SourceSymbol}}, gaps: []contractGap{{key: "#", kind: "openapi.declarations_only", reason: "declarations only"}}},
		{facts: []contractFact{{key: "x", label: string(bytes.Repeat([]byte("x"), 257)), kind: trace.Endpoint}}, gaps: []contractGap{{key: "#", kind: "openapi.declarations_only", reason: "declarations only"}}},
	} {
		output, err := contractRun(input, "openapi", defaultBudget, func([]byte) (contractPlan, error) { return plan, nil })
		requireContractRule(t, output, err, "schema_invalid")
	}
	plan := func([]byte) (contractPlan, error) {
		return contractPlan{facts: []contractFact{{key: "endpoint", label: "Endpoint", kind: trace.Endpoint}}, gaps: []contractGap{{key: "#", kind: "openapi.declarations_only", reason: "declarations only"}}}, nil
	}
	limits := defaultBudget
	limits.records = 1
	output, err := contractRun(input, "openapi", limits, plan)
	requireContractRule(t, output, err, "preflight_limit")
}

func TestContractRunPreflightsEscapedPointerAnchors(t *testing.T) {
	input := contractTestInput(`{"openapi":"3.0.3"}`)
	key := "#/" + string(bytes.Repeat([]byte{`"`[0]}, 2046))
	limits := defaultBudget
	limits.output = 1024 + 3*4096
	output, err := contractRun(input, "openapi", limits, func([]byte) (contractPlan, error) {
		return contractPlan{
			facts: []contractFact{{key: key, label: "Pointer", kind: trace.Endpoint}},
			gaps:  []contractGap{{key: "#", kind: "openapi.declarations_only", reason: "declarations only"}},
		}, nil
	})
	requireContractRule(t, output, err, "preflight_limit")
}

func TestContractRunRejectsSecretMetadataAndNilParserOutput(t *testing.T) {
	input := contractTestInput(`{"metadata":{"token":"not-a-real-secret"}}`)
	output, err := contractRun(input, "pact", defaultBudget, func(data []byte) (contractPlan, error) {
		_, err := contractJSON(data)
		return contractPlan{}, err
	})
	requireContractRule(t, output, err, "secret_indicator")

	input = contractTestInput(`{"openapi":"3.0.3"}`)
	output, err = contractRun(input, "pact", defaultBudget, nil)
	requireContractRule(t, output, err, "schema_invalid")
}

func TestContractJSONRejectsNestedReferenceCyclesAndUnsafePathField(t *testing.T) {
	for _, source := range []string{
		`{"components":{"A":{"properties":{"next":{"$ref":"#/components/B"}}},"B":{"properties":{"next":{"$ref":"#/components/A"}}}}}`,
		`{"metadata":{"path":"/home/example/secret"}}`,
		`{"value":{"$ref":"#/components/.."},"components":{}}`,
	} {
		if _, err := contractJSON([]byte(source)); err == nil {
			t.Fatalf("unsafe JSON accepted: %s", source)
		}
	}
}

func TestContractRunUsesStage4IDTuplesAndDeclarationGapLocator(t *testing.T) {
	input := contractTestInput(`{"openapi":"3.1.0"}`)
	plan := func([]byte) (contractPlan, error) {
		return contractPlan{
			facts: []contractFact{
				{key: "#/components/schemas/Packet", label: "Packet", kind: trace.ProtocolMessage},
				{key: "#/components/schemas/Packet/properties/id", label: "id", parent: "#/components/schemas/Packet", kind: trace.BinaryField},
			},
			gaps: []contractGap{{key: "#", kind: "openapi.declarations_only", reason: "declarations only"}},
		}, nil
	}
	output, err := contractRun(input, "openapi", defaultBudget, plan)
	if err != nil {
		t.Fatal(err)
	}
	document, err := trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	nodeLocator := "#/components/schemas/Packet"
	nodeID := "contract-node-" + tupleHash("stage4", "openapi", "3.1.0", "node", string(trace.ProtocolMessage), input.Artifact.Path, input.ContentSHA256, nodeLocator)
	if document.Nodes[0].ID != nodeID && document.Nodes[1].ID != nodeID {
		t.Fatalf("missing exact node ID %q: %#v", nodeID, document.Nodes)
	}
	var declarationGap trace.Gap
	for _, gap := range document.Gaps {
		if gap.Kind == "openapi.declarations_only" {
			declarationGap = gap
		}
	}
	if declarationGap.ID == "" || declarationGap.Provenance.Anchor == "" {
		t.Fatal("missing declaration-only gap")
	}
	gapID := "contract-gap-" + tupleHash("stage4", "openapi", "3.1.0", "gap", declarationGap.Kind, declarationGap.Reference, declarationGap.Reason, input.Artifact.Path, input.ContentSHA256, "#")
	if declarationGap.ID != gapID {
		t.Fatalf("gap ID=%q want %q", declarationGap.ID, gapID)
	}
}

func TestContractRunRequiresOneDeclarationsOnlyGapAtRoot(t *testing.T) {
	input := contractTestInput(`{"openapi":"3.0.3"}`)
	for _, gaps := range [][]contractGap{
		nil,
		{{key: "document", kind: "openapi.declarations_only", reason: "wrong locator"}},
		{{key: "#", kind: "openapi.declarations_only", reason: "one"}, {key: "#/other", kind: "openapi.declarations_only", reason: "two"}},
	} {
		output, err := contractRun(input, "openapi", defaultBudget, func([]byte) (contractPlan, error) {
			return contractPlan{gaps: gaps}, nil
		})
		requireContractRule(t, output, err, "schema_invalid")
	}
}
