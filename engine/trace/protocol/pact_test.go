package protocol

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"modelcheck/trace"
)

func pactInput(content string) ContractInput {
	data := []byte(content)
	return ContractInput{
		Artifact:      Input{Path: "contracts/example.pact.json", Content: data},
		ContentSHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
	}
}

func requirePactRule(t *testing.T, output []byte, err error, code string) {
	t.Helper()
	if output != nil {
		t.Fatalf("unexpected partial output: %s", output)
	}
	rule, ok := err.(*RuleError)
	if !ok || rule.Code != "stage4."+code {
		t.Fatalf("want stage4.%s, got %#v", code, err)
	}
}

func TestAnalyzePactHTTPDeclaration(t *testing.T) {
	input := pactInput(`{
  "consumer":{"name":"consumer"},
  "provider":{"name":"provider"},
  "metadata":{"pactSpecification":{"version":"4.0"}},
  "interactions":[{
    "key":"create-widget", "type":"Synchronous/HTTP", "description":"create a widget",
    "request":{"method":"POST","path":"/widgets"},
    "response":{"status":201}
  }]
}`)
	out, err := AnalyzePact(input)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Nodes) != 3 || len(doc.Edges) != 2 || len(doc.Evidence) != 5 {
		t.Fatalf("unexpected ledger counts: nodes=%d edges=%d evidence=%d", len(doc.Nodes), len(doc.Edges), len(doc.Evidence))
	}
	for _, evidence := range doc.Evidence {
		if evidence.Level != trace.Contract {
			t.Fatalf("non-contract evidence emitted: %q", evidence.Level)
		}
	}
	if len(doc.Gaps) != 1 || doc.Gaps[0].Kind != "pact.declarations_only" {
		t.Fatalf("missing declaration-only disclosure: %+v", doc.Gaps)
	}
	var endpoints, messages int
	for _, node := range doc.Nodes {
		switch node.Kind {
		case trace.Endpoint:
			endpoints++
		case trace.ProtocolMessage:
			messages++
		default:
			t.Fatalf("unexpected Pact node kind: %q", node.Kind)
		}
	}
	if endpoints != 1 || messages != 2 {
		t.Fatalf("want endpoint plus request/response messages, got endpoints=%d messages=%d", endpoints, messages)
	}
}

func TestAnalyzePactMessageIsGapOnly(t *testing.T) {
	input := pactInput(`{"consumer":{"name":"consumer"},"provider":{"name":"provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"event","type":"Asynchronous/Messages","description":"a message","contents":{"id":1}}]}`)
	out, err := AnalyzePact(input)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Nodes) != 0 || len(doc.Edges) != 0 || len(doc.Evidence) != 0 {
		t.Fatalf("message interaction fabricated facts: %+v", doc)
	}
	if len(doc.Gaps) != 2 {
		t.Fatalf("want declaration and message gaps, got %+v", doc.Gaps)
	}
	kinds := map[string]bool{}
	for _, gap := range doc.Gaps {
		kinds[gap.Kind] = true
	}
	if !kinds["pact.declarations_only"] || !kinds["pact.unmapped_declaration"] {
		t.Fatalf("want declaration and message gaps, got %+v", doc.Gaps)
	}
}

func TestAnalyzePactRejectsFailClosed(t *testing.T) {
	valid := `{"consumer":{"name":"consumer"},"provider":{"name":"provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"one","type":"Synchronous/HTTP","description":"one","request":{"method":"GET","path":"/one"},"response":{"status":200}}]}`
	cases := []struct {
		name string
		body string
		code string
	}{
		{"unsupported version", strings.Replace(valid, `"4.0"`, `"3.0"`, 1), "unsupported_version"},
		{"unknown root", strings.Replace(valid, `"interactions"`, `"extra":true,"interactions"`, 1), "schema_invalid"},
		{"duplicate key", strings.Replace(valid, `"key":"one"`, `"key":"one","key":"two"`, 1), "schema_invalid"},
		{"duplicate interaction key", strings.Replace(valid, `]}`, `,{"key":"one","type":"Synchronous/HTTP","description":"two","request":{"method":"GET","path":"/two"},"response":{"status":200}}]}`, 1), "schema_invalid"},
		{"malformed JSON", valid[:len(valid)-1], "schema_invalid"},
		{"bad method", strings.Replace(valid, `"GET"`, `"get"`, 1), "schema_invalid"},
		{"unsafe route", strings.Replace(valid, `"/one"`, `"/../etc"`, 1), "schema_invalid"},
		{"remote reference", strings.Replace(valid, `"description":"one"`, `"description":"one","$ref":"https://example.invalid/pact"`, 1), "schema_invalid"},
		{"reference cycle", strings.Replace(valid, `"request":{"method":"GET","path":"/one"}`, `"request":{"method":"GET","path":"/one","headers":{"$ref":"#/interactions/0/request/headers"}}`, 1), "schema_invalid"},
		{"secret in ignored metadata", strings.Replace(valid, `"metadata":{"pactSpecification":{"version":"4.0"}}`, `"metadata":{"pactSpecification":{"version":"4.0"},"token":"Bearer abcdefgh"}`, 1), "secret_indicator"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := AnalyzePact(pactInput(tc.body))
			requirePactRule(t, out, err, tc.code)
		})
	}
}

func TestAnalyzePactRejectsUnsafeMetadataAndBudget(t *testing.T) {
	input := pactInput(`{"consumer":{"name":"consumer"},"provider":{"name":"provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"one","type":"Synchronous/HTTP","description":"one","request":{"method":"GET","path":"/one"},"response":{"status":200}}]}`)
	input.Artifact.Path = "contracts/../host.pact.json"
	out, err := AnalyzePact(input)
	requirePactRule(t, out, err, "metadata_invalid")

	input = pactInput(`{"consumer":{"name":"consumer"},"provider":{"name":"provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"one","type":"Synchronous/HTTP","description":"one","request":{"method":"GET","path":"/one"},"response":{"status":200}}]}`)
	out, err = analyzePactWithBudget(input, budget{records: 0, output: 1 << 30, working: 1 << 30})
	requirePactRule(t, out, err, "preflight_limit")
}

func TestAnalyzePactProfileRegressions(t *testing.T) {
	valid := `{"consumer":{"name":"same party"},"provider":{"name":"same party"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"one","type":"Synchronous/HTTP","description":"one","request":{"method":"GET","path":"/one"},"response":{"status":200}}]}`
	input := pactInput(valid)
	out, err := AnalyzePact(input)
	if err != nil {
		t.Fatalf("equal party names are declarations, not an invalid relationship: %v", err)
	}
	document, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	foundDocumentGap := false
	foundEndpoint := false
	for _, gap := range document.Gaps {
		if gap.Kind == "pact.declarations_only" {
			foundDocumentGap = true
			if !strings.HasSuffix(gap.Provenance.Anchor, "#") {
				t.Fatalf("declarations-only gap must use document locator #: %q", gap.Provenance.Anchor)
			}
		}
	}
	if !foundDocumentGap {
		t.Fatal("missing pact declarations-only gap")
	}
	for _, node := range document.Nodes {
		if !strings.HasPrefix(node.ID, "contract-node-") {
			t.Fatalf("unexpected node id: %q", node.ID)
		}
		if node.Kind == trace.Endpoint {
			foundEndpoint = true
			expected := "contract-node-" + tupleHash("stage4", "pact", "4.0", "node", string(trace.Endpoint), input.Artifact.Path, input.ContentSHA256, "#/interactions/0")
			if node.ID != expected {
				t.Fatalf("Pact endpoint ID omitted exact profile or locator: got %q, want %q", node.ID, expected)
			}
		}
	}
	if !foundEndpoint {
		t.Fatal("missing Pact HTTP endpoint")
	}

	for _, tc := range []struct {
		name string
		body string
	}{
		{"HTTP contents", strings.Replace(valid, `"response":{"status":200}`, `"contents":{},"response":{"status":200}`, 1)},
		{"pending is not bool", strings.Replace(valid, `"description":"one"`, `"description":"one","pending":"yes"`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := AnalyzePact(pactInput(tc.body))
			requirePactRule(t, got, gotErr, "schema_invalid")
		})
	}

	long := strings.Repeat("a", 200)
	maxDescription := strings.Repeat("d", 256)
	longLabel := strings.Replace(valid, `"key":"one","type":"Synchronous/HTTP","description":"one"`, `"key":"`+long+`","type":"Synchronous/HTTP","description":"`+maxDescription+`"`, 1)
	if _, err := AnalyzePact(pactInput(longLabel)); err != nil {
		t.Fatalf("bounded key and description must not overflow ledger labels: %v", err)
	}
}

func TestAnalyzePactAllowsOrdinaryColonProse(t *testing.T) {
	input := pactInput(`{"consumer":{"name":"consumer"},"provider":{"name":"provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"one","type":"Synchronous/HTTP","description":"update profile: name","request":{"method":"GET","path":"/one"},"response":{"status":200}}]}`)
	if _, err := AnalyzePact(input); err != nil {
		t.Fatalf("ordinary description prose must be accepted: %v", err)
	}
}

func TestAnalyzePactAcceptsExactInteractionBoundary(t *testing.T) {
	var interactions strings.Builder
	interactions.WriteString(`{"consumer":{"name":"consumer"},"provider":{"name":"provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[`)
	for i := 0; i < 256; i++ {
		if i != 0 {
			interactions.WriteByte(',')
		}
		fmt.Fprintf(&interactions, `{"key":"call%d","type":"Synchronous/HTTP","description":"call","request":{"method":"GET","path":"/call%d"},"response":{"status":200}}`, i, i)
	}
	interactions.WriteString(`]}`)
	if out, err := AnalyzePact(pactInput(interactions.String())); err != nil || out == nil {
		t.Fatalf("exact 256-interaction boundary rejected: output=%d error=%v", len(out), err)
	}
}
