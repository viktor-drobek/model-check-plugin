package protocol

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"modelcheck/trace"
)

func TestProtocolEntryPointLogicalPaths(t *testing.T) {
	paths := []string{"/absolute", "../escape", "a/../escape", "./current", "a/./current", "a//b", "a/", "a\\b", "C:/host", "C:host", "\\\\server\\share", "https://host/file", "file?query", "file#fragment", " padded", "trailing ", "", strings.Repeat("a", 513), "a/\x00/b", "a/\u200b/b"}
	for _, logicalPath := range paths {
		t.Run(logicalPath, func(t *testing.T) {
			grammar := grammarTestInput("kaitai")
			grammar.Artifact.Path = logicalPath
			output, err := AnalyzeGrammar(grammar)
			requireGrammarRule(t, output, err, "metadata_invalid")
			reference := referenceTestInput()
			reference.Artifact.Path = logicalPath
			output, err = AnalyzeGrammarReference(reference)
			requireGrammarRule(t, output, err, "metadata_invalid")
			data := []byte(`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}]`)
			tshark := TSharkJSONInput{Artifact: Input{Path: logicalPath, Content: data}, CaptureID: "synthetic", TSharkVersion: "4.4.10", Profile: "tshark-4.4.10-core-1", ExportSHA256: grammarTestDigest(data), ExportStatus: "complete"}
			output, err = AnalyzeTShark(tshark)
			requireGrammarRule(t, output, err, "metadata_invalid")
		})
	}
	for _, suffix := range []string{".ksy", ".grammar-ref.json", ".json"} {
		logicalPath := strings.Repeat("a", 512-len(suffix)) + suffix
		switch suffix {
		case ".ksy":
			input := grammarTestInput("kaitai")
			input.Artifact.Path = logicalPath
			if _, err := AnalyzeGrammar(input); err != nil {
				t.Fatalf("inclusive path bound: %v", err)
			}
		case ".grammar-ref.json":
			input := referenceTestInput()
			input.Artifact.Path = logicalPath
			if _, err := AnalyzeGrammarReference(input); err != nil {
				t.Fatalf("inclusive path bound: %v", err)
			}
		default:
			data := []byte(`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}]`)
			if _, err := AnalyzeTShark(TSharkJSONInput{Artifact: Input{Path: logicalPath, Content: data}, CaptureID: "synthetic", TSharkVersion: "4.4.10", Profile: "tshark-4.4.10-core-1", ExportSHA256: grammarTestDigest(data), ExportStatus: "complete"}); err != nil {
				t.Fatalf("inclusive path bound: %v", err)
			}
		}
	}
}

func TestProtocolEntryPointAdmissionPrecedence(t *testing.T) {
	raw := []byte(" ")
	_, release, err := begin(Input{Path: "holder.ksy", Content: raw}, grammarTestDigest(raw), func() bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	calls := []func() ([]byte, error){
		func() ([]byte, error) {
			return AnalyzeGrammar(GrammarFileInput{Artifact: Input{Path: "../bad", Content: raw}})
		},
		func() ([]byte, error) {
			return AnalyzeGrammarReference(GrammarReferenceInput{Artifact: Input{Path: "../bad", Content: raw}})
		},
		func() ([]byte, error) {
			return AnalyzeTShark(TSharkJSONInput{Artifact: Input{Path: "../bad", Content: raw}})
		},
	}
	type outcome struct {
		output []byte
		err    error
	}
	for _, call := range calls {
		done := make(chan outcome, 1)
		go func() { output, err := call(); done <- outcome{output, err} }()
		select {
		case result := <-done:
			requireGrammarRule(t, result.output, result.err, "resource_admission_busy")
		case <-time.After(time.Second):
			t.Fatal("entry-point admission blocked instead of rejecting contention")
		}
	}
	output, err := AnalyzeGrammar(GrammarFileInput{})
	requireGrammarRule(t, output, err, "input_length")
	output, err = AnalyzeGrammarReference(GrammarReferenceInput{})
	requireGrammarRule(t, output, err, "input_length")
	output, err = AnalyzeTShark(TSharkJSONInput{})
	requireGrammarRule(t, output, err, "input_length")
}

func TestProtocolJSONScalarBoundary(t *testing.T) {
	for _, scalar := range []string{"1e9999", "-1e9999", "1234567890123456789012345678901234567890"} {
		input := referenceTestInput()
		input.Artifact.Content = bytes.Replace(input.Artifact.Content, []byte(`"vendor.packet"`), []byte(scalar), 1)
		input.ManifestSHA256 = grammarTestDigest(input.Artifact.Content)
		output, err := AnalyzeGrammarReference(input)
		requireGrammarRule(t, output, err, "schema_invalid")
	}
	value, err := decodeJSON([]byte(`{"value":"\uD840\uDC00"}`))
	if err != nil {
		t.Fatal(err)
	}
	object, ok := value.(map[string]any)
	if !ok || object["value"] != "\U00020000" {
		t.Fatalf("paired scalar decoded incorrectly: %#v", value)
	}
}

func TestProtocolOutputDoesNotAliasCallerBytes(t *testing.T) {
	input := grammarTestInput("kaitai")
	output, err := AnalyzeGrammar(input)
	if err != nil {
		t.Fatal(err)
	}
	original := bytes.Clone(output)
	originalDigest := input.GrammarSHA256
	copy(input.Artifact.Content, []byte("password"))
	document, err := trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, output) || document.Nodes[0].Provenance.ContentSHA256 != originalDigest {
		t.Fatal("post-return caller mutation changed the admitted output")
	}
	output, err = AnalyzeGrammar(input)
	requireGrammarRule(t, output, err, "digest_mismatch")
}
