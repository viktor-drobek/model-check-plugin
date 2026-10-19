package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"modelcheck/trace"

	"github.com/cucumber/godog"
)

func contractPointerSafety() error {
	for _, body := range []string{
		`{"items":[{"name":"entry"}],"use":{"$ref":"#/items/0"}}`,
		`{"items":[{"a/b":{"x~y":{}}}],"use":{"$ref":"#/items/0/a~1b/x~0y"}}`,
	} {
		if _, err := contractJSON([]byte(body)); err != nil {
			return fmt.Errorf("canonical array reference rejected: %w", err)
		}
	}
	for _, body := range []string{
		`{"items":[{}],"use":{"$ref":"#/items/00"}}`,
		`{"items":[{}],"use":{"$ref":"#/items/-1"}}`,
		`{"items":[{}],"use":{"$ref":"#/items/1"}}`,
		`{"items":[{}],"use":{"$ref":"#/items/999999999999999999999999"}}`,
		`{"items":[{"$ref":"#/items/1"},{"$ref":"#/items/0"}]}`,
	} {
		if _, err := contractJSON([]byte(body)); err == nil {
			return fmt.Errorf("invalid array reference accepted")
		}
	}
	return nil
}

func contractScopeSafety() error {
	pact := `{"consumer":{"name":"client"},"provider":{"name":"server"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"call","type":"Synchronous/HTTP","description":"call","request":{"method":"GET","path":"/data/items"},"response":{"status":200}}]}`
	openapi := `{"openapi":"3.0.3","info":{"title":"API","version":"1"},"paths":{"/data/items":{"get":{"responses":{"200":{}}}}}}`
	for _, tc := range []struct {
		name, body string
		analyze    func(ContractInput) ([]byte, error)
		code       string
	}{
		{"Pact route", pact, AnalyzePact, ""},
		{"OpenAPI route", openapi, AnalyzeOpenAPI, ""},
		{"opaque path", bytesToStringReplace(pact, `"status":200`, `"status":200,"body":{"path":"/data/items"}`), AnalyzePact, "schema_invalid"},
		{"party URL", bytesToStringReplace(pact, `"client"`, `"https://example.invalid/x"`), AnalyzePact, "schema_invalid"},
		{"gap URL", bytesToStringReplace(pact, `"status":200`, `"status":200,"body":{"url":"https://example.invalid/x"}`), AnalyzePact, ""},
		{"traversal route", bytesToStringReplace(pact, `/data/items`, `/data/../items`), AnalyzePact, "schema_invalid"},
		{"query route", bytesToStringReplace(openapi, `/data/items`, `/data/items?q=1`), AnalyzeOpenAPI, "schema_invalid"},
		{"file route", bytesToStringReplace(openapi, `/data/items`, `/file:/home/u/x`), AnalyzeOpenAPI, "schema_invalid"},
		{"drive route", bytesToStringReplace(openapi, `/data/items`, `/C:/Users/alice/x`), AnalyzeOpenAPI, "schema_invalid"},
		{"bearer-shaped route key", bytesToStringReplace(openapi, `/data/items`, `/x Bearer abcdefghijkl`), AnalyzeOpenAPI, "secret_indicator"},
		{"AWS-shaped component key", `{"openapi":"3.1.0","info":{"title":"API","version":"1"},"paths":{},"components":{"schemas":{"AKIAABCDEFGHIJKLMNOP":{}}}}`, AnalyzeOpenAPI, "secret_indicator"},
		{"arbitrary path key", bytesToStringReplace(openapi, `"version":"1"`, `"version":"1","description":"/data/items"`), AnalyzeOpenAPI, "schema_invalid"},
	} {
		out, err := tc.analyze(contractTestInput(tc.body))
		if tc.code == "" {
			if err != nil || len(out) == 0 {
				return fmt.Errorf("%s: output=%d err=%v", tc.name, len(out), err)
			}
			continue
		}
		var rule *RuleError
		if out != nil || !errors.As(err, &rule) || rule.Code != "stage4."+tc.code {
			return fmt.Errorf("%s: output=%d err=%v", tc.name, len(out), err)
		}
	}
	return nil
}

func bytesToStringReplace(body, from, to string) string {
	return string(bytes.ReplaceAll([]byte(body), []byte(from), []byte(to)))
}

func contractStableDiagnostics() error {
	body := []byte(`{"opaque":{"a":"Bearer abcdefgh1","b":"/tmp/x"}}`)
	var first string
	for i := 0; i < 512; i++ {
		_, err := contractJSON(body)
		var rule *RuleError
		if !errors.As(err, &rule) {
			return fmt.Errorf("missing normalized diagnostic: %v", err)
		}
		got := rule.Code + ":" + rule.Locator
		if i == 0 {
			first = got
		}
		if got != first {
			return fmt.Errorf("diagnostic changed: %s to %s", first, got)
		}
	}
	return nil
}

func apiSchemaDocument(family, schema string) (ContractInput, func(ContractInput) ([]byte, error)) {
	if family == "openapi" {
		return contractTestInput(`{"openapi":"3.1.0","info":{"title":"API","version":"1"},"paths":{},"components":{"schemas":{"Object":` + schema + `}}}`), AnalyzeOpenAPI
	}
	return contractTestInput(`{"asyncapi":"3.0.0","info":{"title":"API","version":"1"},"channels":{},"components":{"schemas":{"Object":` + schema + `}}}`), AnalyzeAsyncAPI
}

func testAPIKeywords() error {
	keywords := []string{"title", "description", "type", "format", "enum", "const", "default", "examples", "nullable", "deprecated", "readOnly", "writeOnly", "required", "items", "additionalProperties", "allOf", "anyOf", "oneOf", "not", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf", "minLength", "maxLength", "pattern", "minItems", "maxItems", "uniqueItems", "minProperties", "maxProperties", "discriminator", "xml", "externalDocs"}
	for _, family := range []string{"openapi", "asyncapi"} {
		for _, keyword := range keywords {
			value := `"opaque"`
			switch keyword {
			case "items", "additionalProperties", "not":
				value = `{}`
			case "allOf", "anyOf", "oneOf":
				value = `[{}]`
			}
			in, analyze := apiSchemaDocument(family, fmt.Sprintf(`{%q:%s}`, keyword, value))
			out, err := analyze(in)
			if err != nil {
				return fmt.Errorf("%s keyword %s: %w", family, keyword, err)
			}
			doc, err := trace.UnmarshalJSON(out)
			if err != nil {
				return err
			}
			count := 0
			for _, gap := range doc.Gaps {
				if strings.HasSuffix(gap.Provenance.Anchor, "#/components/schemas/Object/"+keyword) && gap.Kind == family+".unmapped_declaration" {
					count++
				}
			}
			if count != 1 {
				return fmt.Errorf("%s keyword %s: want one exact gap, got %d", family, keyword, count)
			}
		}
	}
	return nil
}

func testAPINestedProperties() error {
	for _, family := range []string{"openapi", "asyncapi"} {
		in, analyze := apiSchemaDocument(family, `{"properties":{"outer":{"properties":{"inner":{"type":"string"}}}}}`)
		out, err := analyze(in)
		if err != nil {
			return err
		}
		doc, err := trace.UnmarshalJSON(out)
		if err != nil {
			return err
		}
		fields, gaps := 0, 0
		for _, node := range doc.Nodes {
			if node.Kind == trace.BinaryField {
				fields++
			}
		}
		for _, gap := range doc.Gaps {
			if strings.HasSuffix(gap.Provenance.Anchor, "#/components/schemas/Object/properties/outer/properties") && gap.Kind == family+".unmapped_declaration" {
				gaps++
			}
		}
		if fields != 1 || gaps != 1 {
			return fmt.Errorf("%s: fields=%d nested properties gaps=%d", family, fields, gaps)
		}

		for _, schema := range []string{
			`{"items":{"unknown":true}}`,
			`{"additionalProperties":{"unknown":true}}`,
			`{"allOf":[{"unknown":true}]}`,
			`{"anyOf":[{"unknown":true}]}`,
			`{"oneOf":[{"unknown":true}]}`,
			`{"not":{"unknown":true}}`,
			`{"items":false}`,
		} {
			in, analyze = apiSchemaDocument(family, schema)
			out, err = analyze(in)
			var rule *RuleError
			if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" {
				return fmt.Errorf("%s nested schema position %s: output=%d err=%v", family, schema, len(out), err)
			}
		}
	}
	return nil
}

func testAPILabelBoundary() error {
	for _, length := range []int{256, 257} {
		for _, family := range []string{"openapi", "asyncapi"} {
			name := "a" + strings.Repeat("b", length-1)
			analyze := AnalyzeAsyncAPI
			body := fmt.Sprintf(`{"asyncapi":"3.0.0","info":{"title":"API","version":"1"},"channels":{%q:{}}}`, name)
			if family == "openapi" {
				name = "/" + name[1:]
				analyze = AnalyzeOpenAPI
				body = fmt.Sprintf(`{"openapi":"3.1.0","info":{"title":"API","version":"1"},"paths":{%q:{"get":{"responses":{"200":{}}}}}}`, name)
			}
			out, err := analyze(contractTestInput(body))
			if length == 256 {
				if err != nil || len(out) == 0 {
					return fmt.Errorf("%s boundary rejected: %v", family, err)
				}
			} else {
				var rule *RuleError
				if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" {
					return fmt.Errorf("%s over-boundary: output=%d err=%v", family, len(out), err)
				}
			}
		}
	}
	return nil
}

func testAPIFailures() error {
	for _, family := range []string{"openapi", "asyncapi"} {
		for _, schema := range []string{`{"externalDocs":{"description":"see FiLe:///opt/x"}}`, `{"title":"https://example.invalid"}`} {
			in, analyze := apiSchemaDocument(family, schema)
			// Schema title is explicitly gap-only and may contain a URL.
			if strings.Contains(schema, "https:") {
				in = contractTestInput(strings.Replace(string(in.Artifact.Content), `"title":"API"`, `"title":"https://example.invalid"`, 1))
			}
			out, err := analyze(in)
			var rule *RuleError
			if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" || rule.Locator != "json" {
				return fmt.Errorf("%s safety failure: output=%d err=%v", family, len(out), err)
			}
		}
	}
	out, err := AnalyzeAsyncAPI(contractTestInput(`{"asyncapi":2,"info":{"title":"API","version":"1"},"channels":{}}`))
	var rule *RuleError
	if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" || rule.Locator != "schema" {
		return fmt.Errorf("version type: output=%d err=%v", len(out), err)
	}
	return nil
}

func contractEncodedRecordAllowance() error {
	input := contractTestInput(`{"openapi":"3.1.0"}`)
	// validPath permits 512 bytes; this shape maximizes escaped '/' bytes
	// without introducing empty or traversal components.
	input.Artifact.Path = "aa/" + strings.Repeat("a/", 254) + "a"
	key := "#/" + strings.Repeat("\x01", 2046)
	plan := contractPlan{
		facts: []contractFact{
			{key: "parent", label: strings.Repeat("\x01", 256), kind: trace.ProtocolMessage},
			{key: key, label: strings.Repeat("\x01", 256), parent: "parent", kind: trace.BinaryField},
		},
		gaps: []contractGap{
			{key: "#", kind: "openapi.declarations_only", reason: "declarations only"},
			{key: key, kind: "openapi.unmapped_declaration", reason: strings.Repeat("\x01", 256)},
		},
	}
	// Node, edge, evidence and gap records all count toward the preflight.
	records := uint64(2 + 1 + 3 + 2)
	limits := defaultBudget
	limits.output = 1024 + records*contractMaxEncodedRecord
	out, err := contractRun(input, "openapi", limits, func([]byte) (contractPlan, error) { return plan, nil })
	if err != nil {
		return fmt.Errorf("maximum escaped plan: %w", err)
	}
	if uint64(len(out)) > limits.output {
		return fmt.Errorf("encoded output %d exceeds admitted budget %d", len(out), limits.output)
	}
	// Measure records directly from the canonical bytes, including indentation.
	var document map[string]json.RawMessage
	if err := json.Unmarshal(out, &document); err != nil {
		return err
	}
	for _, section := range []string{"nodes", "edges", "evidence", "gaps"} {
		var entries []json.RawMessage
		if err := json.Unmarshal(document[section], &entries); err != nil {
			return err
		}
		for _, entry := range entries {
			if uint64(len(entry)) > contractMaxEncodedRecord {
				return fmt.Errorf("encoded %s record %d exceeds allowance %d", section, len(entry), contractMaxEncodedRecord)
			}
		}
	}
	return nil
}

func contractURLGapBoundaries() error {
	for _, family := range []string{"openapi", "asyncapi"} {
		in, analyze := apiSchemaDocument(family, `{"title":"HtTpS://example.invalid/doc","description":{"url":"https://example.invalid/nested"}}`)
		out, err := analyze(in)
		if err != nil {
			return fmt.Errorf("%s exact schema gaps rejected: %w", family, err)
		}
		doc, err := trace.UnmarshalJSON(out)
		if err != nil {
			return err
		}
		for _, field := range []string{"title", "description"} {
			found := false
			for _, gap := range doc.Gaps {
				if gap.Kind == family+".unmapped_declaration" && strings.HasSuffix(gap.Provenance.Anchor, "#/components/schemas/Object/"+field) {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("%s missing exact %s gap", family, field)
			}
		}
		for _, field := range []string{"title", "version", "description"} {
			if family == "asyncapi" && field == "description" {
				continue // AsyncAPI retains this member as an exact metadata gap.
			}
			body := string(in.Artifact.Content)
			from := `"title":"API"`
			to := `"title":"API","` + field + `":"https://example.invalid/doc"`
			if field == "title" {
				to = `"title":"https://example.invalid/doc"`
			} else if field == "version" {
				from = `"version":"1"`
				to = `"version":"https://example.invalid/doc"`
			}
			out, err := analyze(contractTestInput(strings.Replace(body, from, to, 1)))
			var rule *RuleError
			if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" || rule.Locator != "json" {
				return fmt.Errorf("%s info.%s URL: output=%d err=%v", family, field, len(out), err)
			}
		}
	}
	for _, pointer := range []string{"#/metadata/titleNeighbor", "#/metadata", "#/elsewhere/title"} {
		err := validateContractURLs("https://example.invalid/doc", pointer, []contractGap{{key: "#", kind: "openapi.declarations_only"}, {key: "#/metadata/title", kind: "openapi.unmapped_declaration"}}, nil)
		if err == nil {
			return fmt.Errorf("URL gap exception leaked to %s", pointer)
		}
	}
	for _, value := range []string{"ftp://example.invalid/doc", "ws://example.invalid/doc", "//example.invalid/doc"} {
		if err := validateContractURLs(value, "#/metadata/title", []contractGap{{key: "#/metadata/title", kind: "openapi.unmapped_declaration"}}, nil); err != nil {
			return fmt.Errorf("narrow URL recognizer rejected %q: %w", value, err)
		}
	}
	return nil
}

func contractURLFactDescendants() error {
	base := `{"asyncapi":"2.6.0","info":{"title":"API","version":"1"},"channels":{"c":{"publish":{"message":%s,"summary":"https://example.invalid/metadata"}}}}`
	in := contractTestInput(fmt.Sprintf(base, `{"name":"message","payload":{"type":"string"}}`))
	positive, err := AnalyzeAsyncAPI(in)
	if err != nil || len(positive) == 0 {
		return fmt.Errorf("non-URL message with gap-only summary rejected: output=%d err=%v", len(positive), err)
	}
	positiveDoc, err := trace.UnmarshalJSON(positive)
	if err != nil {
		return err
	}
	foundSummaryGap := false
	for _, gap := range positiveDoc.Gaps {
		if gap.Kind == "asyncapi.unmapped_declaration" && gap.Provenance.Anchor == "contract:"+url.PathEscape(in.Artifact.Path)+"#/channels/c/publish/summary" {
			foundSummaryGap = true
			break
		}
	}
	if !foundSummaryGap {
		return fmt.Errorf("missing exact gap-only operation summary")
	}
	for _, message := range []string{
		`{"name":"https:evil.example","payload":{"type":"string"}}`,
		`{"payload":{"type":"object","properties":{"http:x":{"type":"string"}}}}`,
	} {
		out, err := AnalyzeAsyncAPI(contractTestInput(fmt.Sprintf(base, message)))
		var rule *RuleError
		if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" || rule.Locator != "json" {
			return fmt.Errorf("fact-producing URL descendant: output=%d err=%v", len(out), err)
		}
	}
	return nil
}

func contractOperationIdentifierURLs() error {
	for _, message := range []string{`{"$ref":"#/components/messages/M"}`, `{"name":"message"}`} {
		body := `{"asyncapi":"2.6.0","info":{"title":"API","version":"1"},"channels":{"c":{"publish":{"operationId":"https:evil.example","message":` + message + `}}},"components":{"messages":{"M":{"name":"M"}}}}`
		out, err := AnalyzeAsyncAPI(contractTestInput(body))
		var rule *RuleError
		if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" || rule.Locator != "json" {
			return fmt.Errorf("operationId with message %s: output=%d err=%v", message, len(out), err)
		}
		out, err = AnalyzeAsyncAPI(contractTestInput(strings.Replace(body, "https:evil.example", "publishM", 1)))
		if err != nil || len(out) == 0 {
			return fmt.Errorf("ordinary operationId rejected: output=%d err=%v", len(out), err)
		}
	}
	return nil
}

func contractChannelLabelURLs() error {
	for _, version := range []string{"2.6.0", "3.0.0"} {
		for _, name := range []string{"topic/http:x", "topic/HtTpS:x", " topic", "topic ", "\u2003topic"} {
			body := fmt.Sprintf(`{"asyncapi":%q,"info":{"title":"API","version":"1"},"channels":{%q:{}}}`, version, name)
			out, err := AnalyzeAsyncAPI(contractTestInput(body))
			var rule *RuleError
			if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" {
				return fmt.Errorf("channel %q version %s: output=%d err=%v", name, version, len(out), err)
			}
		}
	}
	return nil
}

func contractURLMapKeyScopes() error {
	for _, prefix := range []string{"http:", "HtTpS:"} {
		key := prefix + "evil.example"
		for _, body := range []string{
			fmt.Sprintf(`{"asyncapi":"3.0.0","info":{"title":"API","version":"1"},"channels":{"c":{}},"operations":{%q:{"action":"send","channel":{"$ref":"#/channels/c"}}}}`, key),
			fmt.Sprintf(`{"asyncapi":"3.0.0","info":{"title":"API","version":"1"},"channels":{"c":{"messages":{%q:{"$ref":"#/components/messages/M"}}}},"components":{"messages":{"M":{"name":"M"}}}}`, key),
			fmt.Sprintf(`{"asyncapi":"3.0.0","info":{"title":"API","version":"1"},"channels":{},"components":{"messages":{%q:{"$ref":"#/components/messages/M"},"M":{"name":"M"}}}}`, key),
		} {
			out, err := AnalyzeAsyncAPI(contractTestInput(body))
			var rule *RuleError
			if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" || rule.Locator != "json" {
				return fmt.Errorf("URL declaration key inherited its own gap: output=%d err=%v", len(out), err)
			}
		}
		// A key nested inside an opaque metadata gap remains eligible.
		body := fmt.Sprintf(`{"asyncapi":"3.0.0","info":{"title":"API","version":"1"},"channels":{"c":{"description":{%q:"metadata"}}}}`, key)
		if out, err := AnalyzeAsyncAPI(contractTestInput(body)); err != nil || len(out) == 0 {
			return fmt.Errorf("URL key beneath metadata gap rejected: output=%d err=%v", len(out), err)
		}
	}
	return nil
}

func contractV3OperationGaps() error {
	body := `{"asyncapi":"3.0.0","info":{"title":"API","version":"1"},"channels":{"c":{"messages":{"M":{"name":"M"}}}},"operations":{"sendM":{"action":"send","channel":{"$ref":"#/channels/c"},"title":"title","summary":"https://example.invalid/doc","description":"description","tags":[],"externalDocs":{},"bindings":{},"traits":[],"security":[],"messages":[{"$ref":"#/channels/c/messages/M"}]}}}`
	in := contractTestInput(body)
	out, err := AnalyzeAsyncAPI(in)
	if err != nil {
		return fmt.Errorf("valid v3 operation rejected: %w", err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		return err
	}
	for _, member := range []string{"title", "summary", "description", "tags", "externalDocs", "bindings", "traits", "security", "messages", "messages/0"} {
		anchor := "contract:" + url.PathEscape(in.Artifact.Path) + "#/operations/sendM/" + member
		count := 0
		for _, gap := range doc.Gaps {
			if gap.Kind == "asyncapi.unmapped_declaration" && gap.Provenance.Anchor == anchor {
				count++
			}
		}
		if count != 1 {
			return fmt.Errorf("v3 %s: want one exact gap, got %d", member, count)
		}
	}
	for _, messages := range []string{`{}`, `[{}]`, `[{"name":"M"}]`, `[{"$ref":"#/channels/c/messages/M","name":"M"}]`} {
		invalid := strings.Replace(body, `[{"$ref":"#/channels/c/messages/M"}]`, messages, 1)
		out, err := AnalyzeAsyncAPI(contractTestInput(invalid))
		var rule *RuleError
		if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" {
			return fmt.Errorf("v3 malformed messages %s: output=%d err=%v", messages, len(out), err)
		}
	}
	return nil
}

func contractNestedPropertyURLs() error {
	for _, version := range []string{"2.6.0", "3.0.0"} {
		for _, prefix := range []string{"http:", "HtTpS:"} {
			for _, schema := range []string{
				`{"properties":{"outer":{"properties":{"` + prefix + `evil.example":{"type":"string"}}}}}`,
				`{"items":{"properties":{"` + prefix + `evil.example":{"type":"string"}}}}`,
				`{"allOf":[{"properties":{"` + prefix + `evil.example":{"type":"string"}}}]}`,
			} {
				body := `{"asyncapi":"` + version + `","info":{"title":"API","version":"1"},"channels":{},"components":{"schemas":{"Object":` + schema + `}}}`
				out, err := AnalyzeAsyncAPI(contractTestInput(body))
				var rule *RuleError
				if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.schema_invalid" || rule.Locator != "json" {
					return fmt.Errorf("nested URL property %s %s: output=%d err=%v", version, schema, len(out), err)
				}
				out, err = AnalyzeAsyncAPI(contractTestInput(strings.ReplaceAll(body, prefix+"evil.example", "ordinary")))
				if err != nil || len(out) == 0 {
					return fmt.Errorf("ordinary nested property rejected: output=%d err=%v", len(out), err)
				}
			}
		}
	}
	return nil
}

func contractInfoDescriptionGaps() error {
	for _, version := range []string{"2.6.0", "3.0.0"} {
		for _, description := range []string{"documentation", "https://example.invalid/doc"} {
			in := contractTestInput(fmt.Sprintf(`{"asyncapi":%q,"info":{"title":"API","version":"1","description":%q},"channels":{}}`, version, description))
			out, err := AnalyzeAsyncAPI(in)
			if err != nil {
				return fmt.Errorf("description rejected: %w", err)
			}
			doc, err := trace.UnmarshalJSON(out)
			if err != nil {
				return err
			}
			anchor := "contract:" + url.PathEscape(in.Artifact.Path) + "#/info/description"
			count := 0
			for _, gap := range doc.Gaps {
				if gap.Kind == "asyncapi.unmapped_declaration" && gap.Provenance.Anchor == anchor {
					count++
				}
			}
			if count != 1 {
				return fmt.Errorf("%s info.description: want one exact gap, got %d", version, count)
			}
		}
	}
	return nil
}

func TestFeaturesG9ContractSafety(t *testing.T) {
	suite := godog.TestSuite{Name: "g9-contract-safety", ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^contract pointers support canonical array indices and reject cycles$`, contractPointerSafety)
		sc.Step(`^contract route exceptions and gap-only URLs respect structural scope$`, contractScopeSafety)
		sc.Step(`^contract safety errors are deterministic for mixed violations$`, contractStableDiagnostics)
		sc.Step(`^both API profiles admit all documented schema keywords with exact gaps$`, testAPIKeywords)
		sc.Step(`^both API profiles retain nested properties as schema-checked explicit gaps only$`, testAPINestedProperties)
		sc.Step(`^API routes and channel names accept the 256 byte boundary$`, testAPILabelBoundary)
		sc.Step(`^API version type errors and embedded file references fail atomically$`, testAPIFailures)
		sc.Step(`^maximally escaped contract records fit their output preflight allowance$`, contractEncodedRecordAllowance)
		sc.Step(`^API URL exceptions admit exact gaps and reject consumed neighbors$`, contractURLGapBoundaries)
		sc.Step(`^AsyncAPI v2 fact-producing descendants reject URL-shaped declarations$`, contractURLFactDescendants)
		sc.Step(`^AsyncAPI operation identifiers reject URLs independently of message facts$`, contractOperationIdentifierURLs)
		sc.Step(`^AsyncAPI channel labels reject embedded URLs and whitespace variants$`, contractChannelLabelURLs)
		sc.Step(`^URL declaration keys cannot inherit their own gap exception$`, contractURLMapKeyScopes)
		sc.Step(`^AsyncAPI rejects URL-shaped property names at every schema depth$`, contractNestedPropertyURLs)
		sc.Step(`^AsyncAPI info descriptions emit one exact unmapped gap$`, contractInfoDescriptionGaps)
		sc.Step(`^AsyncAPI v3 operations emit exact metadata gaps and check message references$`, contractV3OperationGaps)
	}, Options: &godog.Options{Format: "progress", Paths: []string{"../../../features/g9-contract-safety.feature"}, Strict: true, TestingT: t}}
	if suite.Run() != 0 {
		t.Fatal("contract safety scenarios failed")
	}
}
