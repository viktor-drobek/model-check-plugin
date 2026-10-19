package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"modelcheck/trace"
)

func protobufInput(source string) ContractInput {
	data := []byte(source)
	sum := sha256.Sum256(data)
	return ContractInput{Artifact: Input{Path: "fixtures/demo.proto", Content: data}, ContentSHA256: hex.EncodeToString(sum[:])}
}

func requireProtobufRule(t *testing.T, source, code string) {
	t.Helper()
	out, err := AnalyzeProtobuf(protobufInput(source))
	var rule *RuleError
	if out != nil || !errors.As(err, &rule) || rule.Code != code {
		t.Fatalf("output=%q error=%v, want %s", out, err, code)
	}
}

func TestAnalyzeProtobufDeclarations(t *testing.T) {
	source := `syntax = "proto3";
package demo.v1;
message Request { string id = 1; repeated int32 values = 2; }
message Reply { Request echo = 1; }
service Echo { rpc Call (Request) returns (Reply); }`
	out, err := AnalyzeProtobuf(protobufInput(source))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	var messages, fields, endpoints int
	for _, node := range doc.Nodes {
		switch node.Kind {
		case trace.ProtocolMessage:
			messages++
		case trace.BinaryField:
			fields++
		case trace.Endpoint:
			endpoints++
		}
		if node.Provenance.Anchor == "" {
			t.Fatal("declaration has no stable anchor")
		}
	}
	if messages != 2 || fields != 3 || endpoints != 1 {
		t.Fatalf("messages=%d fields=%d endpoints=%d", messages, fields, endpoints)
	}
	if len(doc.Gaps) == 0 || doc.Gaps[0].Kind != "protobuf.declarations_only" {
		t.Fatalf("scope gap missing: %#v", doc.Gaps)
	}
	if !strings.HasSuffix(doc.Gaps[0].Provenance.Anchor, "#") {
		t.Fatalf("scope gap anchor=%q, want locator #", doc.Gaps[0].Provenance.Anchor)
	}
	anchors := map[string]bool{}
	for _, node := range doc.Nodes {
		if anchors[node.Provenance.Anchor] {
			t.Fatalf("duplicate declaration ordinal %q", node.Provenance.Anchor)
		}
		anchors[node.Provenance.Anchor] = true
	}
}

func TestAnalyzeProtobufRejectsUnsafeAndUnsupportedSyntax(t *testing.T) {
	cases := []struct{ name, source, code string }{
		{"import", `syntax = "proto3"; import "x.proto";`, "stage4.unsupported_declaration"},
		{"reserved declaration", `syntax = "proto3"; message A { reserved 4; }`, "stage4.unsupported_declaration"},
		{"comment declaration", "// message Fake { string ignored = 1; }\nsyntax = \"proto3\"; message Real { string value = 1; }", ""},
		{"unknown type", `syntax = "proto3"; message Real { Missing value = 1; }`, "stage4.schema_invalid"},
		{"cycle", `syntax = "proto3"; message A { B b = 1; } message B { A a = 1; }`, "stage4.schema_invalid"},
		{"reserved tag", `syntax = "proto3"; message A { string x = 19000; }`, "stage4.schema_invalid"},
		{"duplicate rpc", `syntax = "proto3"; message A{} service S { rpc X(A) returns(A); rpc X(A) returns(A); }`, "stage4.schema_invalid"},
		{"secret comment", `// token=abcdef0123456789abcdef
syntax = "proto3"; message A { string x = 1; }`, "stage4.secret_indicator"},
		{"host path string", `syntax = "/tmp/data"; message A { string x = 1; }`, "stage4.schema_invalid"},
		{"traversal comment", "// ../private\nsyntax = \"proto3\"; message A { string x = 1; }", "stage4.schema_invalid"},
		{"windows comment", "// C:\\private\\key\nsyntax = \"proto3\"; message A { string x = 1; }", "stage4.schema_invalid"},
		{"unc comment", "// \\\\server\\share\nsyntax = \"proto3\"; message A { string x = 1; }", "stage4.schema_invalid"},
	}
	cases = append(cases,
		struct{ name, source, code string }{"map syntax", `syntax = "proto3"; message A { map<string, int32> values = 1; }`, "stage4.unsupported_declaration"},
		struct{ name, source, code string }{"field option", `syntax = "proto3"; message A { string x = 1 [deprecated = true]; }`, "stage4.unsupported_declaration"},
		struct{ name, source, code string }{"leading zero tag alias", `syntax = "proto3"; message A { string a = 8; string b = 010; }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"leading zero reserved alias", `syntax = "proto3"; message A { string a = 045250; }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"repeated local message", `syntax = "proto3"; message B {} message A { repeated B values = 1; }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"empty package segment", `syntax = "proto3"; package demo.; message A { string x = 1; }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"dotted declaration name", `syntax = "proto3"; message A.B { string x = 1; }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"default JSON name collision", `syntax = "proto3"; message A { string foo_bar = 1; string fooBar = 2; }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"trailing underscore JSON collision", `syntax = "proto3"; message A { string foo_ = 1; string foo = 2; }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"consecutive underscore JSON collision", `syntax = "proto3"; message A { string foo__bar = 1; string fooBar = 2; }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"scalar message name", `syntax = "proto3"; message string { string value = 1; }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"scalar service name", `syntax = "proto3"; message A {} service string { rpc Call(A) returns(A); }`, "stage4.schema_invalid"},
		struct{ name, source, code string }{"spaced package separator", `syntax = "proto3"; package demo . v1; message A { string x = 1; }`, ""},
		struct{ name, source, code string }{"URL in comment", "// see https://example.com/docs\nsyntax = \"proto3\"; message A { string x = 1; }", ""},
		struct{ name, source, code string }{"profile prose", "// profile: v1\nsyntax = \"proto3\"; message A { string x = 1; }", ""},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.code == "" {
				out, err := AnalyzeProtobuf(protobufInput(tc.source))
				if err != nil || out == nil {
					t.Fatalf("comments must be ignored: %v", err)
				}
				return
			}
			requireProtobufRule(t, tc.source, tc.code)
		})
	}
}

func TestAnalyzeProtobufRejectsInvalidUTF8AndBOM(t *testing.T) {
	for _, source := range []string{
		"\xef\xbb\xbfsyntax = \"proto3\"; message A {}",
		"syntax = \"proto3\"; \xff",
	} {
		requireProtobufRule(t, source, "stage4.schema_invalid")
	}
}

func TestAnalyzeProtobufAcceptsLeadingUnderscoreIdentifiers(t *testing.T) {
	source := `syntax = "proto3"; package _demo.v1; message _Request { string _id = 1; }`
	if out, err := AnalyzeProtobuf(protobufInput(source)); err != nil || out == nil {
		t.Fatalf("leading underscore identifiers rejected: output=%q err=%v", out, err)
	}
}

func TestAnalyzeProtobufBudgetErrorsUseBudgetLocator(t *testing.T) {
	var declarations strings.Builder
	declarations.WriteString(`syntax = "proto3"; `)
	for i := 0; i < 1025; i++ {
		fmt.Fprintf(&declarations, "message A%d{} ", i)
	}
	out, err := AnalyzeProtobuf(protobufInput(declarations.String()))
	var rule *RuleError
	if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.preflight_limit" || rule.Locator != "budget" {
		t.Fatalf("declaration budget output=%q err=%v want preflight_limit at budget", out, err)
	}

	tooManyTokens := `syntax = "proto3"; ` + strings.Repeat("{} ", 4100)
	out, err = AnalyzeProtobuf(protobufInput(tooManyTokens))
	if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.preflight_limit" || rule.Locator != "budget" {
		t.Fatalf("token budget output=%q err=%v want preflight_limit at budget", out, err)
	}
}

func TestAnalyzeProtobufLoweredBudgetIsAtomic(t *testing.T) {
	in := protobufInput(`syntax = "proto3"; message A { string x = 1; }`)
	limits := defaultBudget
	limits.records = 0
	out, err := analyzeProtobufWithBudget(in, limits)
	var rule *RuleError
	if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.preflight_limit" {
		t.Fatalf("output=%q err=%v", out, err)
	}
}

func TestAnalyzeProtobufAcceptsExactDeclarationBoundary(t *testing.T) {
	var source strings.Builder
	source.WriteString(`syntax = "proto3"; `)
	for i := 0; i < 1024; i++ {
		fmt.Fprintf(&source, "message A%d{} ", i)
	}
	out, err := AnalyzeProtobuf(protobufInput(source.String()))
	if err != nil || out == nil {
		t.Fatalf("exact 1024-message boundary rejected: output=%d error=%v", len(out), err)
	}
}
