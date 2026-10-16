package trace

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func provenance(anchor string) Provenance {
	return Provenance{
		ContentSHA256: digest,
		Anchor:        anchor,
		Producer:      Producer{Name: "fixture", Version: "1"},
	}
}

func validDocument() *Document {
	return &Document{
		Schema: Schema,
		Nodes: []Node{
			{ID: "requirement.r1", Kind: Requirement, Label: "Messages are acknowledged", Provenance: provenance("requirements.md#r1"), Evidence: []string{"ev.requirement"}},
			{ID: "symbol.send", Kind: SourceSymbol, Label: "Send", Provenance: provenance("send.go:10"), Evidence: []string{"ev.symbol"}},
		},
		Edges: []Edge{{
			ID: "edge.implements", Kind: Implements, Source: "symbol.send", Target: "requirement.r1",
			Provenance: provenance("mapping.yaml#send"), Evidence: []string{"ev.edge"},
		}},
		Evidence: []Evidence{
			{ID: "ev.requirement", Subject: "node:requirement.r1", Level: Contract, Provenance: provenance("requirements.md#r1")},
			{ID: "ev.symbol", Subject: "node:symbol.send", Level: Syntactic, Provenance: provenance("send.go:10")},
			{ID: "ev.edge", Subject: "edge:edge.implements", Level: Reviewed, Provenance: provenance("review.md#mapping")},
		},
		Gaps: []Gap{{ID: "gap.timeout", Kind: "unmapped-operation", Reference: "timeout.go:8", Reason: "No model transition is mapped", Provenance: provenance("timeout.go:8")}},
	}
}

func TestValidateAcceptsCompleteLedger(t *testing.T) {
	if err := Validate(validDocument()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsFailClosedViolations(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Document)
		want string
	}{
		{"wrong schema", func(d *Document) { d.Schema = "trace/0" }, "schema"},
		{"invalid node kind", func(d *Document) { d.Nodes[0].Kind = "guessed" }, "vocabulary"},
		{"invalid edge kind", func(d *Document) { d.Edges[0].Kind = "near" }, "vocabulary"},
		{"invalid evidence level", func(d *Document) { d.Evidence[0].Level = "trusted" }, "vocabulary"},
		{"dangling edge", func(d *Document) { d.Edges[0].Target = "similar.name" }, "does not name a node"},
		{"unknown evidence", func(d *Document) { d.Nodes[0].Evidence = []string{"missing"} }, "unknown evidence"},
		{"cross subject evidence", func(d *Document) { d.Nodes[0].Evidence = []string{"ev.symbol"} }, "references evidence"},
		{"orphan evidence", func(d *Document) {
			d.Evidence = append(d.Evidence, Evidence{ID: "ev.orphan", Subject: "node:symbol.send", Level: Runtime, Provenance: provenance("trace.json:1")})
		}, "is not listed by its subject"},
		{"source verified evidence", func(d *Document) { d.Evidence[1].Level = Verified }, "must apply only to a formal-model artifact"},
		{"empty node evidence", func(d *Document) { d.Nodes[0].Evidence = []string{} }, "must not be empty"},
		{"duplicate node", func(d *Document) { d.Nodes = append(d.Nodes, d.Nodes[0]) }, "duplicates a node"},
		{"malformed hash", func(d *Document) { d.Nodes[0].Provenance.ContentSHA256 = "ABC" }, "lowercase SHA-256"},
		{"no pinned source", func(d *Document) { d.Nodes[0].Provenance.ContentSHA256 = "" }, "source_revision or content_sha256"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validDocument()
			tc.mut(d)
			if err := Validate(d); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestUnmarshalRejectsUnknownDuplicateAndTrailingJSON(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{"unknown nested field", `{"schema":"mcd-trace-ledger/1","nodes":[],"edges":[],"evidence":[],"gaps":[],"unexpected":true}`, "unknown field"},
		{"case variant root field", `{"Schema":"mcd-trace-ledger/1","nodes":[],"edges":[],"evidence":[],"gaps":[]}`, "unknown field"},
		{"duplicate root key", `{"schema":"mcd-trace-ledger/1","schema":"mcd-trace-ledger/1","nodes":[],"edges":[],"evidence":[],"gaps":[]}`, "duplicate JSON key"},
		{"duplicate nested key", `{"schema":"mcd-trace-ledger/1","nodes":[{"id":"n","id":"n","kind":"Issue","label":"x","provenance":{"content_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","anchor":"a","producer":{"name":"p","version":"1"}},"evidence":[]}],"edges":[],"evidence":[],"gaps":[]}`, "duplicate JSON key"},
		{"trailing value", `{"schema":"mcd-trace-ledger/1","nodes":[],"edges":[],"evidence":[],"gaps":[]} null`, "trailing data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := UnmarshalJSON([]byte(tc.data)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestUnmarshalRejectsSchemaInvalidPresentProvenanceFields(t *testing.T) {
	encoded, err := JSON(validDocument())
	if err != nil {
		t.Fatal(err)
	}
	contentHash := `"content_sha256": "` + digest + `"`
	cases := []struct {
		name string
		data string
		want string
	}{
		{
			name: "blank source revision",
			data: strings.Replace(string(encoded), contentHash, `"source_revision": "   ", `+contentHash, 1),
			want: "provenance.source_revision is blank",
		},
		{
			name: "empty content hash",
			data: strings.Replace(string(encoded), contentHash, `"source_revision": "abc123", "content_sha256": ""`, 1),
			want: "provenance.content_sha256 must be a lowercase SHA-256",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := UnmarshalJSON([]byte(tc.data)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestUnmarshalRejectsExcessiveNesting(t *testing.T) {
	data := []byte(strings.Repeat("[", maxJSONNestingDepth+1) + "null" + strings.Repeat("]", maxJSONNestingDepth+1))
	if _, err := UnmarshalJSON(data); err == nil || !strings.Contains(err.Error(), "JSON nesting exceeds") {
		t.Fatalf("error = %v, want nesting rejection", err)
	}
}

func TestJSONIsDeterministicAndDoesNotMutateInput(t *testing.T) {
	d := validDocument()
	d.Nodes[0], d.Nodes[1] = d.Nodes[1], d.Nodes[0]
	d.Evidence[0], d.Evidence[2] = d.Evidence[2], d.Evidence[0]
	d.Nodes[0].Evidence = []string{"ev.symbol"}

	first, err := JSON(d)
	if err != nil {
		t.Fatal(err)
	}
	second, err := JSON(d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("canonical JSON differs:\n%s\n---\n%s", first, second)
	}
	if d.Nodes[0].ID != "symbol.send" {
		t.Fatal("JSON mutated caller node ordering")
	}
	if got := string(first); strings.Index(got, `"id": "requirement.r1"`) > strings.Index(got, `"id": "symbol.send"`) {
		t.Fatalf("nodes are not canonical:\n%s", first)
	}

	roundTrip, err := UnmarshalJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	third, err := JSON(roundTrip)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, third) {
		t.Fatalf("canonical round trip differs:\n%s\n---\n%s", first, third)
	}
}

func TestJSONRoundTripsEmptyCollections(t *testing.T) {
	d := &Document{
		Schema:   Schema,
		Nodes:    []Node{},
		Edges:    []Edge{},
		Evidence: []Evidence{},
		Gaps:     []Gap{},
	}
	encoded, err := JSON(d)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(": null")) {
		t.Fatalf("canonical JSON contains null collection:\n%s", encoded)
	}
	if _, err := UnmarshalJSON(encoded); err != nil {
		t.Fatalf("canonical JSON does not round trip: %v\n%s", err, encoded)
	}
}

func TestReportsAreDeterministicAndNameGaps(t *testing.T) {
	d := validDocument()
	markdown, err := Markdown(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Trace ledger", "does not transfer a model verdict", "gap.timeout", "No model transition is mapped"} {
		if !strings.Contains(string(markdown), want) {
			t.Fatalf("Markdown lacks %q:\n%s", want, markdown)
		}
	}
	sarif, err := SARIF(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"version": "2.1.0"`, `"$schema": "https://json.schemastore.org/sarif-2.1.0.json"`, `"ruleId": "trace/unmapped-operation"`, `"gapId": "gap.timeout"`, `"uri": "timeout.go"`, `"startLine": 8`, `"schema": "mcd-trace-ledger/1"`} {
		if !strings.Contains(string(sarif), want) {
			t.Fatalf("SARIF lacks %q:\n%s", want, sarif)
		}
	}
}

func TestMarkdownEscapesUntrustedCells(t *testing.T) {
	d := validDocument()
	d.Nodes[0].Label = "<img src=x onerror=alert(1)>\\|note\r\nnext"
	markdown, err := Markdown(d)
	if err != nil {
		t.Fatal(err)
	}
	text := string(markdown)
	for _, want := range []string{"&lt;img src=x onerror=alert(1)&gt;", strings.Repeat("\\", 3) + "|note<br>next"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Markdown lacks escaped %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\r") || strings.Contains(text, "<img src=") {
		t.Fatalf("Markdown did not safely normalise cells:\n%s", text)
	}
}

func TestSARIFUsesOneRuleDefinitionPerGapKind(t *testing.T) {
	d := validDocument()
	d.Gaps = append(d.Gaps, Gap{
		ID: "gap.timeout-2", Kind: "unmapped-operation", Reference: "retry.go:5",
		Reason: "No model transition is mapped", Provenance: provenance("retry.go:5"),
	})
	sarif, err := SARIF(d)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(sarif), `"id": "trace/unmapped-operation"`); got != 1 {
		t.Fatalf("SARIF defines %d duplicate rules:\n%s", got, sarif)
	}
	if got := strings.Count(string(sarif), `"ruleId": "trace/unmapped-operation"`); got != 2 {
		t.Fatalf("SARIF has %d results, want 2:\n%s", got, sarif)
	}
}

func TestSARIFParsesOnlyUnambiguousFileLineReferences(t *testing.T) {
	cases := []struct {
		name          string
		reference     string
		wantURI       string
		wantStartLine string
	}{
		{"file line", "timeout.go:8", "timeout.go", `"startLine": 8`},
		{"URL port", "https://example.org:443", "https://example.org:443", ""},
		{"file line column", "timeout.go:8:3", "timeout.go:8:3", ""},
		{"space in path", "docs/my file.md", "docs/my%20file.md", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validDocument()
			d.Gaps[0].Reference = tc.reference
			sarif, err := SARIF(d)
			if err != nil {
				t.Fatal(err)
			}
			text := string(sarif)
			if !strings.Contains(text, `"uri": "`+tc.wantURI+`"`) {
				t.Fatalf("SARIF URI does not contain %q:\n%s", tc.wantURI, text)
			}
			if !strings.Contains(text, `"reference": "`+tc.reference+`"`) {
				t.Fatalf("SARIF does not preserve reference %q:\n%s", tc.reference, text)
			}
			if tc.wantStartLine == "" && strings.Contains(text, `"startLine":`) {
				t.Fatalf("SARIF invents a line for %q:\n%s", tc.reference, text)
			}
			if tc.wantStartLine != "" && !strings.Contains(text, tc.wantStartLine) {
				t.Fatalf("SARIF lacks %s for %q:\n%s", tc.wantStartLine, tc.reference, text)
			}
		})
	}
}

func TestJSONSchemaIsValidAndReturnedAsACopy(t *testing.T) {
	first := JSONSchema()
	var schema map[string]any
	if err := json.Unmarshal(first, &schema); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	if got := schema["$id"]; got != "https://model-check.dev/schemas/mcd-trace-ledger-1.json" {
		t.Fatalf("schema id = %v", got)
	}
	if got := schema["properties"].(map[string]any)["schema"].(map[string]any)["const"]; got != Schema {
		t.Fatalf("schema version = %v, want %q", got, Schema)
	}
	first[0] = '!'
	if bytes.Equal(first, JSONSchema()) {
		t.Fatal("JSONSchema returned mutable package storage")
	}
}
