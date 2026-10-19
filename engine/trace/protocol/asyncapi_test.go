package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"modelcheck/trace"
)

func asyncAPIInput(data string) ContractInput {
	content := []byte(data)
	digest := sha256.Sum256(content)
	return ContractInput{
		Artifact:      Input{Path: "fixtures/asyncapi.json", Content: content},
		ContentSHA256: hex.EncodeToString(digest[:]),
	}
}

func TestAsyncAPIDirectDeclarationsOnly(t *testing.T) {
	in := asyncAPIInput(`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"publish":{"operationId":"publishUpdate","message":{"name":"Notice","payload":{"type":"object","properties":{"value":{"type":"string"}}}}}}},"components":{"messages":{"ComponentNotice":{"name":"ComponentNotice","payload":{"type":"object","properties":{"id":{"type":"integer"}}}}}}}`)
	out, err := AnalyzeAsyncAPI(in)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	var endpoints, messages, fields, sends int
	for _, node := range doc.Nodes {
		switch node.Kind {
		case trace.Endpoint:
			endpoints++
		case trace.ProtocolMessage:
			messages++
		case trace.BinaryField:
			fields++
		}
	}
	for _, edge := range doc.Edges {
		if edge.Kind == trace.Sends || edge.Kind == trace.Receives {
			sends++
		}
	}
	if endpoints != 1 || messages != 2 || fields != 2 || sends != 0 {
		t.Fatalf("nodes endpoint=%d message=%d field=%d behavior edges=%d", endpoints, messages, fields, sends)
	}
	for _, evidence := range doc.Evidence {
		if evidence.Level != trace.Contract {
			t.Fatalf("evidence level=%q", evidence.Level)
		}
	}
	if !hasAsyncAPIGap(doc, "asyncapi.declarations_only") {
		t.Fatal("missing declarations-only gap")
	}
	if !hasAsyncAPIGapAt(doc, "asyncapi.operation_unmapped", "contract:fixtures%2Fasyncapi.json#/channels/updates/publish") {
		t.Fatal("missing v2 operation gap at the operation pointer")
	}
}

func TestAsyncAPIProfileSchemaConstraintsAreExplicit(t *testing.T) {
	for _, schema := range []string{
		`{"type":["string","null"]}`,
		`{"$ref":"#/components/schemas/Item","description":"opaque"}`,
	} {
		in := asyncAPIInput(`{"asyncapi":"3.0.0","info":{"title":"Demo","version":"1"},"channels":{},"components":{"schemas":{"Item":{"type":"string"},"Object":` + schema + `}}}`)
		out, err := AnalyzeAsyncAPI(in)
		var rule *RuleError
		if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" {
			t.Fatalf("schema constraint %s: output=%q error=%v", schema, out, err)
		}
	}
}

func TestAsyncAPIv3ActionsAreGapsNotBehaviour(t *testing.T) {
	in := asyncAPIInput(`{"asyncapi":"3.0.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"address":"updates","messages":{"notice":{"name":"Notice","payload":{"type":"object","properties":{"value":{"type":"string"}}}}}}},"operations":{"receiveNotice":{"action":"receive","channel":{"$ref":"#/channels/updates"}}}}`)
	out, err := AnalyzeAsyncAPI(in)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAsyncAPIGap(doc, "asyncapi.operation_unmapped") {
		t.Fatal("missing v3 operation gap")
	}
	for _, edge := range doc.Edges {
		if edge.Kind == trace.Sends || edge.Kind == trace.Receives {
			t.Fatalf("inferred behaviour edge %#v", edge)
		}
	}
}

func TestAsyncAPIRejectsUnsupportedShapes(t *testing.T) {
	cases := []string{
		`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{},"operations":{}}`,
		`{"asyncapi":"3.0.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"publish":{}}}}`,
		`{"asyncapi":"2.6.1","info":{"title":"Demo","version":"1"},"channels":{}}`,
		`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"publish":{"message":{"$ref":"https://example.invalid/message"}}}}}`,
		`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"publish":{"message":{"name":"N","payload":{"type":"object","properties":{"value":{"type":"string","unexpected":true}}}}}}}}`,
	}
	for _, data := range cases {
		t.Run(data[:20], func(t *testing.T) {
			out, err := AnalyzeAsyncAPI(asyncAPIInput(data))
			var ruleErr *RuleError
			if out != nil || !errors.As(err, &ruleErr) || !strings.HasPrefix(ruleErr.Code, "stage4.") {
				t.Fatalf("output=%q err=%#v", out, err)
			}
		})
	}
}

func TestAsyncAPIBudgetFailureIsAtomic(t *testing.T) {
	in := asyncAPIInput(`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"publish":{"message":{"name":"Notice"}}}}}`)
	limits := defaultBudget
	limits.records = 1
	out, err := analyzeAsyncAPIWithBudget(in, limits)
	var ruleErr *RuleError
	if out != nil || !errors.As(err, &ruleErr) || ruleErr.Code != "stage4.preflight_limit" || ruleErr.Locator != "budget" {
		t.Fatalf("output=%q error=%#v", out, err)
	}
}

func TestAsyncAPICanonicalPointersAndProfileGap(t *testing.T) {
	in := asyncAPIInput(`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"order/created":{"publish":{"message":{"name":"Notice","payload":{"type":"object","properties":{"value":{"type":"string","description":"opaque"}}}}}}}}`)
	out, err := AnalyzeAsyncAPI(in)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	wantChannel := "contract:fixtures%2Fasyncapi.json#/channels/order~1created"
	wantField := wantChannel + "/publish/message/payload/properties/value"
	var channel, field bool
	for _, node := range doc.Nodes {
		switch node.Provenance.Anchor {
		case wantChannel:
			channel = node.Kind == trace.Endpoint
		case wantField:
			field = node.Kind == trace.BinaryField
		}
	}
	if !channel || !field {
		t.Fatalf("missing canonical source anchors channel=%v field=%v", channel, field)
	}
	if !hasAsyncAPIGapAt(doc, "asyncapi.declarations_only", "contract:fixtures%2Fasyncapi.json#") {
		t.Fatal("missing root declarations-only scope gap")
	}
	if !hasAsyncAPIGapAt(doc, "asyncapi.unmapped_declaration", wantField+"/description") {
		t.Fatal("schema metadata gap was not anchored at its exact source pointer")
	}
}

func TestAsyncAPIDoesNotCreateNestedPropertyFacts(t *testing.T) {
	in := asyncAPIInput(`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"publish":{"message":{"payload":{"type":"object","properties":{"outer":{"type":"object","properties":{"inner":{"type":"string"}}}}}}}}}}`)
	out, err := AnalyzeAsyncAPI(in)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	fields := 0
	for _, node := range doc.Nodes {
		if node.Kind == trace.BinaryField {
			fields++
		}
	}
	if fields != 1 {
		t.Fatalf("want only direct payload property, got %d fields", fields)
	}
}

func TestAsyncAPIRejectsNonProfileSchemaAndV3ChannelTarget(t *testing.T) {
	cases := []string{
		`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"publish":{"message":{"payload":{"type":"string","unknownSchemaKeyword":["x"]}}}}}}`,
		`{"asyncapi":"3.0.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{}},"components":{"schemas":{"notAChannel":{"type":"object"}}},"operations":{"receive":{"action":"receive","channel":{"$ref":"#/components/schemas/notAChannel"}}}}`,
	}
	for _, data := range cases {
		out, err := AnalyzeAsyncAPI(asyncAPIInput(data))
		var ruleErr *RuleError
		if out != nil || !errors.As(err, &ruleErr) || ruleErr.Code != "stage4.schema_invalid" {
			t.Fatalf("output=%q error=%#v", out, err)
		}
	}
}

func TestAsyncAPIReferenceSchemaChildrenHaveUniqueGaps(t *testing.T) {
	for _, keyword := range []string{"items", "additionalProperties", "not"} {
		t.Run(keyword, func(t *testing.T) {
			in := asyncAPIInput(`{"asyncapi":"3.0.0","info":{"title":"Demo","version":"1"},"channels":{},"components":{"schemas":{"List":{"` + keyword + `":{"$ref":"#/components/schemas/Item"}},"Item":{"type":"string"}}}}`)
			out, err := AnalyzeAsyncAPI(in)
			if err != nil {
				t.Fatalf("reference-only %s rejected: %v", keyword, err)
			}
			doc, err := trace.UnmarshalJSON(out)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, gap := range doc.Gaps {
				key := gap.Provenance.Anchor + "\x00" + gap.Kind
				if seen[key] {
					t.Fatalf("duplicate gap %q", key)
				}
				seen[key] = true
			}
		})
	}
}

func TestAsyncAPIRejectsDuplicateV2OperationIDs(t *testing.T) {
	in := asyncAPIInput(`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"a":{"publish":{"operationId":"same","message":{"name":"A"}}},"b":{"publish":{"operationId":"same","message":{"name":"B"}}}}}`)
	out, err := AnalyzeAsyncAPI(in)
	var rule *RuleError
	if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" {
		t.Fatalf("duplicate operationId output=%q err=%v", out, err)
	}
}

func hasAsyncAPIGap(doc *trace.Document, kind string) bool {
	for _, gap := range doc.Gaps {
		if gap.Kind == kind {
			return true
		}
	}
	return false
}

func hasAsyncAPIGapAt(doc *trace.Document, kind, anchor string) bool {
	for _, gap := range doc.Gaps {
		if gap.Kind == kind && gap.Provenance.Anchor == anchor {
			return true
		}
	}
	return false
}
