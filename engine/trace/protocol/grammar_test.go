package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"modelcheck/trace"
)

// All artifacts in this file are synthetic and contain no third-party grammar.
func grammarTestDigest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func grammarTestInput(language string) GrammarFileInput {
	version, ext, text := "0.10", ".ksy", "meta:\n  id: packet\nseq: []\n"
	if language == "spicy" {
		version, ext, text = "1.12.0", ".spicy", "module Packet;\ntype Packet = unit {};\n"
	}
	data := []byte(text)
	return GrammarFileInput{Artifact: Input{Path: "grammars/packet" + ext, Content: data}, Language: language, LanguageVersion: version, DeclaredAnchor: "root", GrammarSHA256: grammarTestDigest(data)}
}
func referenceTestInput() GrammarReferenceInput {
	data := []byte(`{"schema":"mcd-grammar-reference/1","language":"kaitai","language_version":"0.10","target_id":"vendor.packet","target_sha256":"` + strings.Repeat("a", 64) + `","declared_anchor":"root"}`)
	return GrammarReferenceInput{Artifact: Input{Path: "grammars/packet.grammar-ref.json", Content: data}, ManifestSHA256: grammarTestDigest(data)}
}
func requireGrammarRule(t *testing.T, output []byte, err error, code string) {
	t.Helper()
	var e *RuleError
	if len(output) != 0 || !errors.As(err, &e) || e.Code != "stage3b."+code {
		t.Fatalf("output bytes=%d error=%v; want stage3b.%s and no output", len(output), err, code)
	}
}
func TestGrammarFileRegistration(t *testing.T) {
	for _, lang := range []string{"kaitai", "spicy"} {
		t.Run(lang, func(t *testing.T) {
			input := grammarTestInput(lang)
			got, err := AnalyzeGrammar(input)
			if err != nil {
				t.Fatal(err)
			}
			again, err := AnalyzeGrammar(input)
			if err != nil || !bytes.Equal(got, again) {
				t.Fatalf("nondeterministic: %v", err)
			}
			doc, err := trace.UnmarshalJSON(got)
			if err != nil {
				t.Fatal(err)
			}
			if doc.Schema != trace.SchemaV2 || len(doc.Nodes) != 1 || len(doc.Edges) != 0 || len(doc.Evidence) != 1 || len(doc.Gaps) != 1 {
				t.Fatalf("unexpected registration: %+v", doc)
			}
			id := grammarTestDigest([]byte(strings.Join([]string{"grammar-artifact/1", lang, input.LanguageVersion, input.Artifact.Path, input.DeclaredAnchor, input.GrammarSHA256}, "\x00")))
			node := doc.Nodes[0]
			ev := doc.Evidence[0]
			gap := doc.Gaps[0]
			anchor := "grammar:" + lang + "@" + input.LanguageVersion + ":" + input.Artifact.Path + "#declared=root"
			if node.ID != "grammar."+id || node.Kind != trace.GrammarArtifact || ev.ID != "evidence.grammar."+id || ev.Subject != "node:"+node.ID || ev.Level != trace.Grammar || gap.ID != "gap.grammar.anchor_unverified."+id || gap.Reference != "node:"+node.ID || gap.Reason != "the declared grammar anchor was not syntax-verified" {
				t.Fatalf("wrong identity/evidence/gap: %+v", doc)
			}
			if node.Provenance.Anchor != anchor || node.Provenance.ContentSHA256 != input.GrammarSHA256 || node.Provenance.Producer != (trace.Producer{Name: "mcd-grammar-adapter", Version: "1"}) || node.Provenance != ev.Provenance || node.Provenance != gap.Provenance {
				t.Fatal("wrong provenance")
			}
			canon, err := trace.JSON(doc)
			if err != nil || !bytes.Equal(got, canon) {
				t.Fatalf("canonical mismatch %v", err)
			}
			input.Artifact.Path = "other/packet" + map[string]string{"kaitai": ".ksy", "spicy": ".spicy"}[lang]
			other, err := AnalyzeGrammar(input)
			if err != nil || bytes.Equal(got, other) {
				t.Fatalf("path not in identity: %v", err)
			}
		})
	}
}
func TestGrammarFileMetadataAndDigest(t *testing.T) {
	tests := []struct {
		name, code string
		mutate     func(*GrammarFileInput)
	}{
		{"digest", "digest_mismatch", func(i *GrammarFileInput) { i.GrammarSHA256 = strings.Repeat("0", 64) }},
		{"digest syntax before secret", "metadata_invalid", func(i *GrammarFileInput) { i.GrammarSHA256 = "secret"; i.DeclaredAnchor = "password" }},
		{"unknown language", "metadata_invalid", func(i *GrammarFileInput) { i.Language = "other" }},
		{"future version rejected by adapter", "metadata_invalid", func(i *GrammarFileInput) { i.LanguageVersion = "0.11" }},
		{"version range", "metadata_invalid", func(i *GrammarFileInput) { i.LanguageVersion = "latest" }},
		{"wrong extension", "metadata_invalid", func(i *GrammarFileInput) { i.Artifact.Path = "grammars/packet.spicy" }},
		{"stemless", "metadata_invalid", func(i *GrammarFileInput) { i.Artifact.Path = "grammars/.ksy" }},
		{"absolute", "metadata_invalid", func(i *GrammarFileInput) { i.Artifact.Path = "/packet.ksy" }},
		{"escape", "metadata_invalid", func(i *GrammarFileInput) { i.Artifact.Path = "a/../packet.ksy" }},
		{"anchor URI", "metadata_invalid", func(i *GrammarFileInput) { i.DeclaredAnchor = "https://host/root" }},
		{"long anchor", "metadata_invalid", func(i *GrammarFileInput) { i.DeclaredAnchor = strings.Repeat("x", 201) }},
		{"blank anchor", "metadata_invalid", func(i *GrammarFileInput) { i.DeclaredAnchor = "" }},
		{"secret anchor before digest", "secret_indicator", func(i *GrammarFileInput) { i.DeclaredAnchor = "auth_token"; i.GrammarSHA256 = strings.Repeat("0", 64) }},
		{"secret path", "secret_indicator", func(i *GrammarFileInput) { i.Artifact.Path = "grammars/credential.ksy" }},
		{"empty", "input_length", func(i *GrammarFileInput) { i.Artifact.Content = nil }},
		{"oversize", "input_length", func(i *GrammarFileInput) { i.Artifact.Content = bytes.Repeat([]byte{'x'}, (1<<20)+1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := grammarTestInput("kaitai")
			tt.mutate(&i)
			out, err := AnalyzeGrammar(i)
			requireGrammarRule(t, out, err, tt.code)
		})
	}
}
func TestGrammarRawAdmission(t *testing.T) {
	invalid := []string{
		"auth_token: value", `"password hint": value`, "? password\n: value", "local db_password\n = x;", "token # comment\n = 1;", "- id: auth_token", `- id: "auth token"`, "1: db_password", "0x01: db_password", "type E = enum { TOKEN, OTHER };", "type TokenPayload = unit {};", "x: |\n  password", "# credential", "-----BEGIN PRIVATE KEY-----", "AKIAABCDEFGHIJKLMNOP", "a\\b", "meta:\r\n", "\x00", "\x01", "\x7f", "\u0085", "\ufeff", "\u200b", "\u2028", "\u2029", string([]byte{0xff}), string([]byte{0xed, 0xa0, 0x80}),
	}
	for index, text := range invalid {
		for _, lang := range []string{"kaitai", "spicy"} {
			t.Run(fmt.Sprintf("%s/%d", lang, index), func(t *testing.T) {
				i := grammarTestInput(lang)
				i.Artifact.Content = []byte(text)
				i.GrammarSHA256 = grammarTestDigest(i.Artifact.Content)
				out, err := AnalyzeGrammar(i)
				requireGrammarRule(t, out, err, "grammar_source_invalid")
				if strings.Contains(err.Error(), text) && len(text) > 4 {
					t.Fatal("raw input leaked")
				}
			})
		}
	}
	for _, tt := range []struct{ lang, text string }{
		{"kaitai", "imports: []"}, {"kaitai", "{ imports: [] }"}, {"kaitai", `"imports"`}, {"kaitai", "# imports"}, {"spicy", "import X;"}, {"spicy", "include X;"}, {"spicy", `"include"`}, {"spicy", "# import"},
	} {
		t.Run(tt.lang+tt.text, func(t *testing.T) {
			i := grammarTestInput(tt.lang)
			i.Artifact.Content = []byte(tt.text)
			i.GrammarSHA256 = grammarTestDigest(i.Artifact.Content)
			out, err := AnalyzeGrammar(i)
			requireGrammarRule(t, out, err, "grammar_source_invalid")
		})
	}
	for _, text := range []string{"basic handshake", "important includes", "Imports import include", "\tmeta:\n", "external-type-without-syntax-claim"} {
		i := grammarTestInput("kaitai")
		i.Artifact.Content = []byte(text)
		i.GrammarSHA256 = grammarTestDigest(i.Artifact.Content)
		if _, err := AnalyzeGrammar(i); err != nil {
			t.Fatalf("control %q rejected: %v", text, err)
		}
	}
	for _, text := range []string{"\\password", "password\\", "\x00imports", "imports\x00", "a\xffpassword"} {
		i := grammarTestInput("kaitai")
		i.Artifact.Content = []byte(text)
		i.GrammarSHA256 = grammarTestDigest(i.Artifact.Content)
		out, err := AnalyzeGrammar(i)
		requireGrammarRule(t, out, err, "grammar_source_invalid")
		var e *RuleError
		errors.As(err, &e)
		want := 0
		if text[0] == 'a' {
			want = 1
		}
		if e.Locator != fmt.Sprintf("byte:%d", want) {
			t.Fatalf("locator %q want byte:%d", e.Locator, want)
		}
	}
	i := grammarTestInput("kaitai")
	i.Artifact.Content = bytes.Repeat([]byte{' '}, 1<<20)
	i.GrammarSHA256 = grammarTestDigest(i.Artifact.Content)
	if _, err := AnalyzeGrammar(i); err != nil {
		t.Fatalf("inclusive byte max: %v", err)
	}
}
func TestGrammarRawControlsAndOffsets(t *testing.T) {
	for value := 0; value <= 0x9f; value++ {
		if value > 0x1f && value < 0x7f || value == '\n' || value == '\t' {
			continue
		}
		source := []byte("meta:\n  id: packet\n" + string(rune(value)) + "\n")
		input := grammarTestInput("kaitai")
		input.Artifact.Content = source
		input.GrammarSHA256 = grammarTestDigest(source)
		output, err := AnalyzeGrammar(input)
		requireGrammarRule(t, output, err, "grammar_source_invalid")
		var rejection *RuleError
		if !errors.As(err, &rejection) || rejection.Locator != "byte:19" {
			t.Fatalf("control %02x: %v", value, err)
		}
	}
	cases := []struct {
		source string
		offset int
	}{
		{"abc\npassword\\", 4}, {"abc\n\\password", 4},
		{"ok\nimports\x00", 3}, {"ok\n\x00imports", 3},
		{"\U00020000\npassword", 5}, {"ok\n\xffpassword", 3},
	}
	for _, tt := range cases {
		input := grammarTestInput("kaitai")
		input.Artifact.Content = []byte(tt.source)
		input.GrammarSHA256 = grammarTestDigest(input.Artifact.Content)
		output, err := AnalyzeGrammar(input)
		requireGrammarRule(t, output, err, "grammar_source_invalid")
		var rejection *RuleError
		if !errors.As(err, &rejection) || rejection.Locator != fmt.Sprintf("byte:%d", tt.offset) {
			t.Fatalf("wrong raw byte offset: %v", err)
		}
	}
	for _, lang := range []string{"kaitai", "spicy"} {
		input := grammarTestInput(lang)
		input.Artifact.Content = []byte("# \U00020000\n\timportant includes\n")
		input.GrammarSHA256 = grammarTestDigest(input.Artifact.Content)
		if _, err := AnalyzeGrammar(input); err != nil {
			t.Fatalf("valid scalar source: %v", err)
		}
	}
}

func TestGrammarReferenceManifest(t *testing.T) {
	input := referenceTestInput()
	got, err := AnalyzeGrammarReference(input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := AnalyzeGrammarReference(input)
	if err != nil || !bytes.Equal(got, again) {
		t.Fatalf("not deterministic: %v", err)
	}
	doc, err := trace.UnmarshalJSON(got)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Schema != trace.Schema || len(doc.Nodes) != 0 || len(doc.Edges) != 0 || len(doc.Evidence) != 0 || len(doc.Gaps) != 1 {
		t.Fatalf("asserted bytes became evidence: %+v", doc)
	}
	id := grammarTestDigest([]byte(strings.Join([]string{"grammar-reference/1", input.Artifact.Path, input.ManifestSHA256, "kaitai", "0.10", "vendor.packet", strings.Repeat("a", 64), "root"}, "\x00")))
	gap := doc.Gaps[0]
	if gap.ID != "gap.grammar.reference_unresolved."+id || gap.Reference != "grammar-reference:kaitai@0.10:vendor.packet#sha256="+strings.Repeat("a", 64)+"&declared=root" || gap.Reason != "grammar bytes were not supplied for the asserted reference" || gap.Provenance.ContentSHA256 != input.ManifestSHA256 || gap.Provenance.Anchor != "grammar-reference:"+input.Artifact.Path+"#declared=root" || gap.Provenance.Producer != (trace.Producer{Name: "mcd-grammar-reference-adapter", Version: "1"}) {
		t.Fatalf("wrong gap: %+v", gap)
	}
}
func TestGrammarInclusiveByteLimit(t *testing.T) {
	input := grammarTestInput("kaitai")
	input.Artifact.Content = bytes.Repeat([]byte{' '}, 1<<20)
	input.GrammarSHA256 = grammarTestDigest(input.Artifact.Content)
	output, err := AnalyzeGrammar(input)
	if err != nil || len(output) == 0 {
		t.Fatalf("inclusive raw byte limit: %v", err)
	}
	input.Artifact.Content = append(input.Artifact.Content, ' ')
	output, err = AnalyzeGrammar(input)
	requireGrammarRule(t, output, err, "input_length")
}

func TestGrammarReferencePermittedPunctuation(t *testing.T) {
	input := referenceTestInput()
	input.Artifact.Content = bytes.Replace(input.Artifact.Content, []byte("vendor.packet"), []byte("vendor..packet"), 1)
	input.ManifestSHA256 = grammarTestDigest(input.Artifact.Content)
	output, err := AnalyzeGrammarReference(input)
	if err != nil {
		t.Fatal(err)
	}
	document, err := trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Gaps) != 1 || !strings.Contains(document.Gaps[0].Reference, ":vendor..packet#") {
		t.Fatal("permitted target identity not retained")
	}
}

func TestGrammarReferenceRejections(t *testing.T) {
	tests := []struct {
		name, code string
		mutate     func(*GrammarReferenceInput)
	}{
		{"digest", "digest_mismatch", func(i *GrammarReferenceInput) { i.ManifestSHA256 = strings.Repeat("0", 64) }},
		{"path", "metadata_invalid", func(i *GrammarReferenceInput) { i.Artifact.Path = "/a.grammar-ref.json" }},
		{"stemless", "metadata_invalid", func(i *GrammarReferenceInput) { i.Artifact.Path = ".grammar-ref.json" }},
		{"size", "input_length", func(i *GrammarReferenceInput) { i.Artifact.Content = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := referenceTestInput()
			tt.mutate(&i)
			out, err := AnalyzeGrammarReference(i)
			requireGrammarRule(t, out, err, tt.code)
		})
	}
	jsonCases := []struct{ old, new, code string }{
		{`"target_id":"vendor.packet"`, `"target_id":"password"`, "secret_indicator"},
		{`"target_id":"vendor.packet"`, `"target_id":"pass\u0077ord"`, "secret_indicator"},
		{`"target_id":"vendor.packet"`, `"target_id":"/root/file"`, "schema_invalid"},
		{`"target_id":"vendor.packet"`, `"target_id":"id#fragment"`, "schema_invalid"},
		{`"0.10"`, `"0.11"`, "schema_invalid"},
		{`"language":"kaitai"`, `"language":"spicy"`, "schema_invalid"},
		{`"declared_anchor":"root"`, `"declared_anchor":"auth_token"`, "secret_indicator"},
		{`"declared_anchor":"root"`, `"declared_anchor":"root/path"`, "schema_invalid"},
		{`"target_sha256":"` + strings.Repeat("a", 64) + `"`, `"target_sha256":"` + strings.Repeat("A", 64) + `"`, "schema_invalid"},
		{`"target_id":"vendor.packet"`, `"target_id":"` + strings.Repeat("a", 201) + `"`, "schema_invalid"},
		{`"target_id":"vendor.packet"`, `"target_id":"https://host"`, "schema_invalid"},

		{`"target_id":"vendor.packet"`, `"target_id":null`, "schema_invalid"},
		{`"target_id":"vendor.packet"`, `"target_id":"vendor.packet","extra":"x"`, "schema_invalid"},
		{`"target_id":"vendor.packet"`, `"target_id":"vendor.packet","target_\u0069d":"x"`, "json_duplicate_name"},
		{`"target_id":"vendor.packet"`, `"target_id":"\ud800"`, "json_syntax"},
	}
	for n, tt := range jsonCases {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			i := referenceTestInput()
			i.Artifact.Content = []byte(strings.Replace(string(i.Artifact.Content), tt.old, tt.new, 1))
			i.ManifestSHA256 = grammarTestDigest(i.Artifact.Content)
			out, err := AnalyzeGrammarReference(i)
			requireGrammarRule(t, out, err, tt.code)
		})
	}
	// Escaped legitimate values use the structured policy, not the raw-source backslash ban.
	i := referenceTestInput()
	i.Artifact.Content = bytes.Replace(i.Artifact.Content, []byte("kaitai"), []byte(`kait\u0061i`), 1)
	i.ManifestSHA256 = grammarTestDigest(i.Artifact.Content)
	if _, err := AnalyzeGrammarReference(i); err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(i.Artifact.Content, &m); err != nil {
		t.Fatal(err)
	}
}
