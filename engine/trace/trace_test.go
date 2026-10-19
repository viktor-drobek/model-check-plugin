package trace

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

const (
	schemaV2             = "mcd-trace-ledger/2"
	grammarIdentity      = "8621b1ba283b844d72b9256c2f51bb161b207ef964ece0c59e7dc3dfdbd46d20"
	spicyGrammarIdentity = "8103997f4019fa44121c2ee1105936184665f122da61be7590f7c2dec18706ed"
)

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
		Gaps: []Gap{{ID: "gap.timeout", Kind: "unmapped-operation", Reference: "./timeout.go:8", Reason: "No model transition is mapped", Provenance: provenance("timeout.go:8")}},
	}
}

func validV2Document() *Document {
	grammarID := "grammar." + grammarIdentity
	provenance := Provenance{
		ContentSHA256: digest,
		Anchor:        "grammar:kaitai@0.10:grammars/packet.ksy#declared=root",
		Producer:      Producer{Name: "mcd-grammar-adapter", Version: "1"},
	}
	return &Document{
		Schema: schemaV2,
		Nodes: []Node{{
			ID: grammarID, Kind: "GrammarArtifact", Label: "Registered Kaitai grammar artifact",
			Provenance: provenance, Evidence: []string{"evidence.grammar." + grammarIdentity},
		}},
		Edges: []Edge{},
		Evidence: []Evidence{{
			ID: "evidence.grammar." + grammarIdentity, Subject: "node:" + grammarID,
			Level: "grammar", Provenance: provenance,
		}},
		Gaps: []Gap{{
			ID:   "gap.grammar.anchor_unverified." + grammarIdentity,
			Kind: "grammar.anchor_unverified", Reference: "node:" + grammarID,
			Reason: "the declared grammar anchor was not syntax-verified", Provenance: provenance,
		}},
	}
}

func validSpicyV2Document() *Document {
	document := validV2Document()
	grammarID := "grammar." + spicyGrammarIdentity
	provenance := Provenance{
		ContentSHA256: digest,
		Anchor:        "grammar:spicy@1.2.3:grammars/protocol.spicy#declared=Protocol",
		Producer:      Producer{Name: "mcd-grammar-adapter", Version: "1"},
	}
	document.Nodes[0] = Node{
		ID: grammarID, Kind: GrammarArtifact, Label: "Registered Spicy grammar artifact",
		Provenance: provenance, Evidence: []string{"evidence.grammar." + spicyGrammarIdentity},
	}
	document.Evidence[0] = Evidence{
		ID: "evidence.grammar." + spicyGrammarIdentity, Subject: "node:" + grammarID,
		Level: Grammar, Provenance: provenance,
	}
	document.Gaps[0] = Gap{
		ID:   "gap.grammar.anchor_unverified." + spicyGrammarIdentity,
		Kind: "grammar.anchor_unverified", Reference: "node:" + grammarID,
		Reason: grammarAnchorUnverifiedReason, Provenance: provenance,
	}
	return document
}

func appendOrdinaryV2Node(document *Document) {
	provenance := provenance("requirements.md#r1")
	document.Nodes = append(document.Nodes, Node{
		ID: "requirement.r1", Kind: Requirement, Label: "Grammar consumers accept packets",
		Provenance: provenance, Evidence: []string{"evidence.requirement.r1"},
	})
	document.Evidence = append(document.Evidence, Evidence{
		ID: "evidence.requirement.r1", Subject: "node:requirement.r1", Level: Contract, Provenance: provenance,
	})
}

func appendOrdinaryV2Edge(document *Document) {
	sourceProvenance := provenance("source.go#send")
	document.Nodes = append(document.Nodes, Node{
		ID: "symbol.send", Kind: SourceSymbol, Label: "Send",
		Provenance: sourceProvenance, Evidence: []string{"evidence.symbol.send"},
	})
	document.Evidence = append(document.Evidence,
		Evidence{ID: "evidence.symbol.send", Subject: "node:symbol.send", Level: Syntactic, Provenance: sourceProvenance},
		Evidence{ID: "evidence.edge.implements", Subject: "edge:edge.implements", Level: Reviewed, Provenance: provenance("mapping.yaml#send")},
	)
	document.Edges = append(document.Edges, Edge{
		ID: "edge.implements", Kind: Implements, Source: "symbol.send", Target: "requirement.r1",
		Provenance: provenance("mapping.yaml#send"), Evidence: []string{"evidence.edge.implements"},
	})
}

func readV2Fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../testdata/trace/grammar-artifact-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func appendRootMember(data, member string) string {
	trimmed := strings.TrimSpace(data)
	return trimmed[:len(trimmed)-1] + "," + member + "}"
}

func TestUnmarshalJSONForwardsNonEligibleV2SelectorsToV1(t *testing.T) {
	v1, err := JSON(validDocument())
	if err != nil {
		t.Fatal(err)
	}
	v2 := string(readV2Fixture(t))
	cases := []struct {
		name       string
		data       string
		wantSchema string
		wantError  string
	}{
		{name: "v1 selector", data: string(v1), wantSchema: Schema},
		{name: "BOM-prefixed selector", data: "\ufeff" + v2, wantError: "invalid character"},
		{name: "escaped selector name", data: strings.Replace(v2, `"schema"`, `"sch\u0065ma"`, 1), wantError: "schema is \"mcd-trace-ledger/2\", want \"mcd-trace-ledger/1\""},
		{name: "escaped selector value", data: strings.Replace(v2, "mcd-trace-ledger/2", `mcd-trace-ledger/\u0032`, 1), wantError: "schema is \"mcd-trace-ledger/2\", want \"mcd-trace-ledger/1\""},
		{name: "truncated selector", data: `{"schema": "mcd-trace-ledger/2`, wantError: "unexpected EOF"},
		{name: "selector after another root member", data: `{"nodes":[],"schema":"mcd-trace-ledger/2","edges":[],"evidence":[],"gaps":[]}`, wantError: "schema is \"mcd-trace-ledger/2\", want \"mcd-trace-ledger/1\""},
		{name: "case-variant selector", data: strings.Replace(v2, `"schema"`, `"Schema"`, 1), wantError: "unknown field"},
		{name: "vertical-tab prefix", data: "\v" + v2, wantError: "invalid character"},
		{name: "invalid selector terminator", data: `{"schema":"mcd-trace-ledger/2"x}`, wantError: "invalid character"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := UnmarshalJSON([]byte(test.data))
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("UnmarshalJSON() error = %v, want substring %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalJSON() error = %v", err)
			}
			if got.Schema != test.wantSchema {
				t.Fatalf("schema = %q, want %q", got.Schema, test.wantSchema)
			}
		})
	}
}

func TestV1RejectsGrammarArtifactAfterV2SupportIsAdded(t *testing.T) {
	document := validDocument()
	document.Nodes[0].Kind = GrammarArtifact
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "vocabulary") {
		t.Fatalf("Validate() error = %v, want v1 closed-vocabulary rejection", err)
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
	for _, want := range []string{`"version": "2.1.0"`, `"$schema": "https://json.schemastore.org/sarif-2.1.0.json"`, `"ruleId": "trace/unmapped-operation"`, `"gapId": "gap.timeout"`, `"uri": "./timeout.go"`, `"startLine": 8`, `"schema": "mcd-trace-ledger/1"`} {
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
		{"explicit file line", "./timeout.go:8", "./timeout.go", `"startLine": 8`},
		{"slash path line", "docs/timeout.go:8", "docs/timeout.go", `"startLine": 8`},
		{"URL port", "https://example.org:443", "https://example.org:443", ""},
		{"opaque URN", "urn:ietf:rfc:2119", "urn:ietf:rfc:2119", ""},
		{"opaque mailto", "mailto:owner@example.org", "mailto:owner@example.org", ""},
		{"file URI", "file:/abs/path", "file:/abs/path", ""},
		{"dotted opaque URI", "iris.beep:opaque", "iris.beep:opaque", ""},
		{"dotted opaque URI numeric suffix", "iris.beep:123", "iris.beep:123", ""},
		{"extensionless ambiguous reference", "Makefile:12", "makefile:12", ""},
		{"drive relative ambiguous reference", `C:repo\x.go`, `c:repo\x.go`, ""},
		{"fragment", "requirements.md#r1", "requirements.md#r1", ""},
		{"query", "requirements.md?version=1", "requirements.md?version=1", ""},
		{"query line suffix", "timeout.go?rev=x:8", "timeout.go?rev=x:8", ""},
		{"fragment line suffix", "requirements.md#r1:2", "requirements.md#r1:2", ""},
		{"explicit path fragment line suffix", "./a.md#L:12", "./a.md#L:12", ""},
		{"slash path query line suffix", "docs/a.md?x=y:3", "docs/a.md?x=y:3", ""},
		{"file line column", "./timeout.go:8:3", "./timeout.go:8:3", ""},
		{"file line query with URL", "timeout.go:8?see=https://x", "timeout.go:8?see=https://x", ""},
		{"POSIX absolute path line", "/repo/timeout.go:8", "/repo/timeout.go:8", ""},
		{"percent path line", "docs/100%.md:5", "docs/100%25.md", `"startLine": 5`},
		{"encoded-looking percent path line", "docs/50%20off.md:5", "docs/50%2520off.md", `"startLine": 5`},
		{"hex-looking percent path line", "docs/100%41.md:5", "docs/100%2541.md", `"startLine": 5`},
		{"windows style path", "C:/repo/x.go", "./C:/repo/x.go", ""},
		{"windows style path with line", "C:/repo/x.go:12", "./C:/repo/x.go:12", ""},
		{"network path line", "//host.example/x.go:5", "//host.example/x.go:5", ""},
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
			if !containsSARIFString(text, "uri", tc.wantURI) {
				t.Fatalf("SARIF URI does not contain %q:\n%s", tc.wantURI, text)
			}
			if !containsSARIFString(text, "reference", tc.reference) {
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

func containsSARIFString(report, field, value string) bool {
	encoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return strings.Contains(report, `"`+field+`": `+string(encoded))
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

func TestJSONSchemaV2IsValidAndReturnedAsACopy(t *testing.T) {
	first := JSONSchemaV2()
	var schema map[string]any
	if err := json.Unmarshal(first, &schema); err != nil {
		t.Fatalf("schema v2 is not JSON: %v", err)
	}
	if got := schema["$id"]; got != "https://model-check.dev/schemas/mcd-trace-ledger-2.json" {
		t.Fatalf("schema v2 id = %v", got)
	}
	if got := schema["properties"].(map[string]any)["schema"].(map[string]any)["const"]; got != SchemaV2 {
		t.Fatalf("schema v2 version = %v, want %q", got, SchemaV2)
	}
	if !bytes.Contains(first, []byte(`"GrammarArtifact"`)) {
		t.Fatalf("schema v2 lacks GrammarArtifact:\n%s", first)
	}
	first[0] = '!'
	if bytes.Equal(first, JSONSchemaV2()) {
		t.Fatal("JSONSchemaV2 returned mutable package storage")
	}
}

func TestV2ReportsUseDocumentSchemaAndPreserveGrammarAnchor(t *testing.T) {
	document := validV2Document()
	markdown, err := Markdown(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Schema: `mcd-trace-ledger/2`",
		"grammar:kaitai@0.10:grammars/packet.ksy#declared=root",
		"the declared grammar anchor was not syntax-verified",
	} {
		if !strings.Contains(string(markdown), want) {
			t.Fatalf("Markdown lacks %q:\n%s", want, markdown)
		}
	}

	sarif, err := SARIF(document)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sarif), `"schema": "mcd-trace-ledger/2"`) {
		t.Fatalf("SARIF lacks v2 schema:\n%s", sarif)
	}
	if strings.Contains(string(sarif), Schema) {
		t.Fatalf("SARIF leaked v1 schema:\n%s", sarif)
	}
}

func TestV2GrammarArtifactValidatesAndCanonicalizesByteIdentically(t *testing.T) {
	document := validV2Document()
	if err := Validate(document); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	first, err := JSON(document)
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if !bytes.HasPrefix(first, []byte("{\n  \"schema\": \"mcd-trace-ledger/2\"")) {
		t.Fatalf("v2 canonical JSON must emit schema first:\n%s", first)
	}

	roundTripped, err := UnmarshalJSON(first)
	if err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if roundTripped.Schema != schemaV2 {
		t.Fatalf("round-tripped schema = %q, want %q", roundTripped.Schema, schemaV2)
	}
	second, err := JSON(roundTripped)
	if err != nil {
		t.Fatalf("JSON(round-tripped) error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("v2 canonical round-trip changed bytes:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestUnmarshalJSONSelectedV2RejectsStrictJSONViolations(t *testing.T) {
	valid := string(readV2Fixture(t))
	cases := []struct {
		name string
		data string
		want string
	}{
		{
			name: "invalid UTF-8",
			data: strings.Replace(valid, "Registered Kaitai grammar artifact", "\xff", 1),
			want: "invalid UTF-8",
		},
		{
			name: "malformed escape",
			data: strings.Replace(valid, "Registered Kaitai grammar artifact", `\q`, 1),
			want: "invalid JSON escape",
		},
		{
			name: "unpaired surrogate",
			data: strings.Replace(valid, "Registered Kaitai grammar artifact", `\uD800`, 1),
			want: "unpaired surrogate",
		},
		{
			name: "lone low surrogate",
			data: strings.Replace(valid, "Registered Kaitai grammar artifact", `\uDC00`, 1),
			want: "unpaired surrogate",
		},
		{
			name: "mismatched surrogate pair",
			data: strings.Replace(valid, "Registered Kaitai grammar artifact", `\uD800\u0041`, 1),
			want: "unpaired surrogate",
		},
		{
			name: "truncated Unicode escape",
			data: strings.Replace(valid, "Registered Kaitai grammar artifact", `\u12`, 1),
			want: "invalid JSON unicode escape",
		},
		{
			name: "raw control character in string",
			data: strings.Replace(valid, "Registered Kaitai grammar artifact", "bad\x01label", 1),
			want: "control character",
		},
		{
			name: "decoded duplicate root member",
			data: appendRootMember(valid, `"sch\u0065ma":"mcd-trace-ledger/2"`),
			want: "duplicate JSON member",
		},
		{
			name: "unknown case-variant root member",
			data: appendRootMember(valid, `"Schema":"mcd-trace-ledger/2"`),
			want: "unknown field",
		},
		{
			name: "trailing data",
			data: valid + " null",
			want: "trailing data",
		},
		{name: "malformed literal", data: `{"schema":"mcd-trace-ledger/2","nodes":nulx}`, want: "invalid JSON literal"},
		{name: "malformed number", data: `{"schema":"mcd-trace-ledger/2","nodes":1e}`, want: "invalid JSON number"},
		{name: "missing colon", data: `{"schema" "mcd-trace-ledger/2"}`, want: "invalid character"},
		{name: "trailing comma", data: `{"schema":"mcd-trace-ledger/2",}`, want: "object member name must be a string"},
		{
			name: "nested duplicate member",
			data: `{"schema":"mcd-trace-ledger/2","nodes":[{"id":"first","id":"second"}]}`,
			want: "duplicate JSON member",
		},
		{
			name: "nesting bound",
			data: `{"schema":"mcd-trace-ledger/2","nodes":` + strings.Repeat("[", 129) + `]` + strings.Repeat("]", 128) + `}`,
			want: "JSON nesting exceeds 128 levels",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := UnmarshalJSON([]byte(test.data))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("UnmarshalJSON() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestStrictJSONParserDecodesSupplementaryUnicodeEscapes(t *testing.T) {
	parser := strictJSONParser{data: []byte(`"\uD840\uDC00"`)}
	got, err := parser.parseString()
	if err != nil {
		t.Fatalf("parseString() error = %v", err)
	}
	if got != "\U00020000" {
		t.Fatalf("parseString() = %U, want U+20000", []rune(got)[0])
	}
}

func TestUnmarshalJSONSelectedV2AcceptsWhitespaceAroundSelector(t *testing.T) {
	data := strings.Replace(string(readV2Fixture(t)), `"schema": "mcd-trace-ledger/2"`, "\n\t\"schema\" \r : \n \"mcd-trace-ledger/2\" \t", 1)
	document, err := UnmarshalJSON([]byte(data))
	if err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if document.Schema != SchemaV2 {
		t.Fatalf("schema = %q, want %q", document.Schema, SchemaV2)
	}
}

func TestV2GrammarArtifactRejectsDisclosureAndApplicabilityViolations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Document)
		want   string
	}{
		{
			name: "missing sole evidence",
			mutate: func(document *Document) {
				document.Nodes[0].Evidence = []string{}
			},
			want: "exactly one grammar evidence",
		},
		{
			name: "non-grammar evidence level",
			mutate: func(document *Document) {
				document.Evidence[0].Level = "reviewed"
			},
			want: "must use grammar evidence",
		},
		{
			name: "non-deterministic node ID",
			mutate: func(document *Document) {
				id := "grammar." + strings.Repeat("0", 64)
				document.Nodes[0].ID = id
				document.Evidence[0].Subject = "node:" + id
				document.Gaps[0].Reference = "node:" + id
			},
			want: "GrammarArtifact node ID",
		},
		{
			name: "non-deterministic label",
			mutate: func(document *Document) {
				document.Nodes[0].Label = "A grammar artifact"
			},
			want: "GrammarArtifact node",
		},
		{
			name: "non-deterministic evidence ID",
			mutate: func(document *Document) {
				id := "evidence.grammar." + strings.Repeat("1", 64)
				document.Nodes[0].Evidence = []string{id}
				document.Evidence[0].ID = id
			},
			want: "GrammarArtifact evidence ID",
		},
		{
			name: "evidence subject belongs to another grammar artifact",
			mutate: func(document *Document) {
				spicy := validSpicyV2Document()
				document.Nodes = append(document.Nodes, spicy.Nodes[0])
				document.Evidence = append(document.Evidence, spicy.Evidence[0])
				document.Gaps = append(document.Gaps, spicy.Gaps[0])
				document.Evidence[0].Subject = "node:" + spicy.Nodes[0].ID
			},
			want: "GrammarArtifact evidence subject",
		},
		{
			name: "differently-provenanced grammar evidence",
			mutate: func(document *Document) {
				document.Evidence[0].Provenance.Anchor = "grammar:kaitai@0.10:grammars/other.ksy#declared=root"
			},
			want: "evidence provenance must equal node provenance",
		},
		{
			name: "missing grammar content digest",
			mutate: func(document *Document) {
				document.Nodes[0].Provenance.ContentSHA256 = ""
			},
			want: "content_sha256",
		},
		{
			name: "one-part grammar version",
			mutate: func(document *Document) {
				document.Nodes[0].Provenance.Anchor = "grammar:kaitai@1:grammars/packet.ksy#declared=root"
			},
			want: "grammar language version",
		},
		{
			name: "zero-padded grammar version",
			mutate: func(document *Document) {
				document.Nodes[0].Provenance.Anchor = "grammar:kaitai@01.2:grammars/packet.ksy#declared=root"
			},
			want: "grammar language version",
		},
		{
			name: "wrong grammar extension",
			mutate: func(document *Document) {
				document.Nodes[0].Provenance.Anchor = "grammar:kaitai@0.10:grammars/packet.spicy#declared=root"
			},
			want: "must end in .ksy",
		},
		{
			name: "missing disclosure gap",
			mutate: func(document *Document) {
				document.Gaps = []Gap{}
			},
			want: "exactly one grammar.anchor_unverified gap",
		},
		{
			name: "mismatched disclosure reason",
			mutate: func(document *Document) {
				document.Gaps[0].Reason = "unverified"
			},
			want: "grammar disclosure gap reason",
		},
		{
			name: "duplicate disclosure gap",
			mutate: func(document *Document) {
				document.Gaps = append(document.Gaps, document.Gaps[0])
			},
			want: "exactly one grammar.anchor_unverified gap",
		},
		{
			name: "detached disclosure gap",
			mutate: func(document *Document) {
				document.Gaps[0].Reference = "node:missing.grammar"
			},
			want: "must reference a GrammarArtifact node",
		},
		{
			name: "disclosure gap without node reference",
			mutate: func(document *Document) {
				document.Gaps[0].Reference = "grammar." + grammarIdentity
			},
			want: "must reference a GrammarArtifact node",
		},
		{
			name: "differently-provenanced disclosure gap",
			mutate: func(document *Document) {
				document.Gaps[0].Provenance.Anchor = "grammar:kaitai@0.10:grammars/other.ksy#declared=root"
			},
			want: "provenance must equal node provenance",
		},
		{
			name: "grammar evidence on another node",
			mutate: func(document *Document) {
				provenance := document.Nodes[0].Provenance
				document.Nodes = append(document.Nodes, Node{
					ID: "requirement.grammar", Kind: Requirement, Label: "A requirement",
					Provenance: provenance, Evidence: []string{"evidence.requirement.grammar"},
				})
				document.Evidence = append(document.Evidence, Evidence{
					ID: "evidence.requirement.grammar", Subject: "node:requirement.grammar",
					Level: Grammar, Provenance: provenance,
				})
			},
			want: "must apply only to a GrammarArtifact node",
		},
		{
			name: "grammar evidence on an edge",
			mutate: func(document *Document) {
				provenance := document.Nodes[0].Provenance
				document.Edges = []Edge{{
					ID: "edge.grammar", Kind: "derived_from", Source: document.Nodes[0].ID,
					Target: document.Nodes[0].ID, Provenance: provenance,
					Evidence: []string{"evidence.edge.grammar"},
				}}
				document.Evidence = append(document.Evidence, Evidence{
					ID: "evidence.edge.grammar", Subject: "edge:edge.grammar", Level: Grammar, Provenance: provenance,
				})
			},
			want: "must apply only to a GrammarArtifact node",
		},
		{
			name: "unsafe grammar anchor",
			mutate: func(document *Document) {
				document.Nodes[0].Provenance.Anchor = "grammar:kaitai@0.10:../packet.ksy#declared=root"
			},
			want: "grammar logical path",
		},
		{
			name: "incident edge",
			mutate: func(document *Document) {
				document.Edges = []Edge{{
					ID: "edge.grammar", Kind: "derived_from", Source: document.Nodes[0].ID,
					Target: document.Nodes[0].ID, Provenance: document.Nodes[0].Provenance,
					Evidence: []string{document.Nodes[0].Evidence[0]},
				}}
			},
			want: "must be isolated",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			document := validV2Document()
			test.mutate(document)
			err := Validate(document)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestV2GrammarArtifactAcceptsSpicyAndMixedDocuments(t *testing.T) {
	cases := []struct {
		name     string
		document *Document
	}{
		{name: "Spicy grammar artifact", document: validSpicyV2Document()},
		{
			name: "grammar artifact mixed with ordinary node",
			document: func() *Document {
				document := validV2Document()
				appendOrdinaryV2Node(document)
				return document
			}(),
		},
		{
			name: "two isolated grammar artifacts with ordinary edge",
			document: func() *Document {
				document := validV2Document()
				spicy := validSpicyV2Document()
				document.Nodes = append(document.Nodes, spicy.Nodes[0])
				document.Evidence = append(document.Evidence, spicy.Evidence[0])
				document.Gaps = append(document.Gaps, spicy.Gaps[0])
				appendOrdinaryV2Node(document)
				appendOrdinaryV2Edge(document)
				return document
			}(),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := Validate(test.document); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}
