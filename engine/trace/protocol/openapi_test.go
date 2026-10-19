package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"modelcheck/trace"
)

func openAPIInput(source string) ContractInput {
	sum := sha256.Sum256([]byte(source))
	return ContractInput{
		Artifact:      Input{Path: "contracts/openapi.json", Content: []byte(source)},
		ContentSHA256: hex.EncodeToString(sum[:]),
	}
}

func requireOpenAPIRule(t *testing.T, output []byte, err error, code string) {
	t.Helper()
	if output != nil {
		t.Fatalf("output = %q, want nil", output)
	}
	re, ok := err.(*RuleError)
	if !ok || re.Code != "stage4."+code {
		t.Fatalf("error = %#v, want stage4.%s", err, code)
	}
}

func TestOpenAPIProducesContractDeclarationsOnly(t *testing.T) {
	input := openAPIInput(`{
		"openapi":"3.1.0",
		"info":{"title":"Example","version":"1"},
		"paths":{"/widgets":{"get":{"operationId":"listWidgets","responses":{"200":{"description":"ok"}}}}},
		"components":{"schemas":{"Widget":{"type":"object","properties":{"id":{"type":"string"}}}}}
	}`)
	output, err := AnalyzeOpenAPI(input)
	if err != nil {
		t.Fatal(err)
	}
	document, err := trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(document.Nodes))
	}
	if len(document.Edges) != 1 || document.Edges[0].Kind != trace.DerivedFrom {
		t.Fatalf("edges = %#v, want one containment edge", document.Edges)
	}
	for _, evidence := range document.Evidence {
		if evidence.Level != trace.Contract {
			t.Fatalf("evidence level = %q, want contract", evidence.Level)
		}
	}
	foundDeclarationsOnly := false
	for _, gap := range document.Gaps {
		if gap.Kind == "openapi.declarations_only" {
			foundDeclarationsOnly = true
			break
		}
	}
	if !foundDeclarationsOnly {
		t.Fatalf("gaps = %#v, want declarations_only", document.Gaps)
	}
}

func TestOpenAPIUsesSourcePointersForFactsAndGaps(t *testing.T) {
	output, err := AnalyzeOpenAPI(openAPIInput(`{
		"openapi":"3.1.0", "info":{"title":"Example","version":"1"},
		"paths":{"/widgets":{"get":{"responses":{"200":{}}}}},
		"components":{"schemas":{"Widget":{"properties":{"id":{"type":"string"}}}}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	document, err := trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	wantAnchors := map[string]bool{
		"contract:contracts%2Fopenapi.json#/paths/~1widgets/get":                          false,
		"contract:contracts%2Fopenapi.json#/components/schemas/Widget":                    false,
		"contract:contracts%2Fopenapi.json#/components/schemas/Widget/properties/id":      false,
		"contract:contracts%2Fopenapi.json#/paths/~1widgets/get/responses":                false,
		"contract:contracts%2Fopenapi.json#/components/schemas/Widget/properties/id/type": false,
	}
	for _, node := range document.Nodes {
		if _, ok := wantAnchors[node.Provenance.Anchor]; ok {
			wantAnchors[node.Provenance.Anchor] = true
		}
	}
	for _, gap := range document.Gaps {
		if _, ok := wantAnchors[gap.Provenance.Anchor]; ok {
			wantAnchors[gap.Provenance.Anchor] = true
		}
	}
	for anchor, found := range wantAnchors {
		if !found {
			t.Errorf("missing fact or gap anchored at %s", anchor)
		}
	}
}

func TestOpenAPIAcceptsSupportedVersions(t *testing.T) {
	for _, version := range []string{"3.0.3", "3.1.0"} {
		t.Run(version, func(t *testing.T) {
			input := openAPIInput(`{"openapi":"` + version + `","info":{"title":"x","version":"1"},"paths":{}}`)
			if _, err := AnalyzeOpenAPI(input); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenAPIRejectsInvalidDeclarations(t *testing.T) {
	cases := []struct{ name, source, code string }{
		{"unsupported version", `{"openapi":"3.0.2","info":{"title":"x","version":"1"},"paths":{}}`, "unsupported_version"},
		{"root unknown key", `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{},"x":1}`, "schema_invalid"},
		{"schema wrong type", `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{},"components":{"schemas":{"X":"bad"}}}`, "schema_invalid"},
		{"schema unknown keyword", `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{},"components":{"schemas":{"X":{"wat":true}}}}`, "schema_invalid"},
		{"schema key outside narrow profile", `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{},"components":{"schemas":{"X":{"$schema":"https://json-schema.org"}}}}`, "schema_invalid"},
		{"duplicate operation id", `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/a":{"get":{"operationId":"same","responses":{"200":{}}}},"/b":{"get":{"operationId":"same","responses":{"200":{}}}}}}`, "schema_invalid"},
		{"duplicate template shape", `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/a/{id}":{"get":{"responses":{"200":{}}}},"/a/{name}":{"get":{"responses":{"200":{}}}}}}`, "schema_invalid"},
		{"unsafe metadata path", `{"openapi":"3.1.0","info":{"title":"x","version":"1","description":"/home/user/key"},"paths":{}}`, "schema_invalid"},
		{"secret metadata", `{"openapi":"3.1.0","info":{"title":"x","version":"1","description":"Bearer abcdefghijklmnop"},"paths":{}}`, "secret_indicator"},
		{"dot route component", `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/a/./b":{}}}`, "schema_invalid"},
		{"backslash route", `{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/a\\b":{}}}`, "schema_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := AnalyzeOpenAPI(openAPIInput(tc.source))
			requireOpenAPIRule(t, output, err, tc.code)
		})
	}
}

func TestOpenAPIRejectsInvalidReferences(t *testing.T) {
	for _, ref := range []string{"#/components/schemas/Missing", "https://example.test/schema", "#/components/schemas/A"} {
		t.Run(ref, func(t *testing.T) {
			schemas := `{"A":{"$ref":"` + ref + `"}}`
			if ref == "#/components/schemas/A" {
				schemas = `{"A":{"$ref":"#/components/schemas/B"},"B":{"$ref":"#/components/schemas/A"}}`
			}
			output, err := AnalyzeOpenAPI(openAPIInput(`{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{},"components":{"schemas":` + schemas + `}}`))
			requireOpenAPIRule(t, output, err, "schema_invalid")
		})
	}
}

func TestOpenAPILoweredBudgetReturnsNil(t *testing.T) {
	input := openAPIInput(`{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{}}`)
	output, err := analyzeOpenAPIWithBudget(input, budget{})
	requireOpenAPIRule(t, output, err, "preflight_limit")
}

func TestOpenAPIContractNamesAreBounded(t *testing.T) {
	long := strings.Repeat("a", 257)
	output, err := AnalyzeOpenAPI(openAPIInput(`{"openapi":"3.1.0","info":{"title":"x","version":"1"},"paths":{"/` + long + `":{}}}`))
	requireOpenAPIRule(t, output, err, "schema_invalid")
}
