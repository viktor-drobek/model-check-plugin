package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"modelcheck/trace"
)

func bufInput(data string) ContractInput {
	content := []byte(data)
	sum := sha256.Sum256(content)
	return ContractInput{Artifact: Input{Path: "fixtures/buf.json", Content: content}, ContentSHA256: hex.EncodeToString(sum[:])}
}

func TestAnalyzeBufModuleGaps(t *testing.T) {
	out, err := AnalyzeBuf(bufInput(`{"version":"v2","modules":[{"path":"api/v1"},{"path":"internal"}],"deps":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Nodes) != 0 {
		t.Fatalf("buf emitted declarations: %#v", doc.Nodes)
	}
	var modules, scope int
	moduleLocators := map[string]bool{}
	for _, gap := range doc.Gaps {
		if gap.Kind == "buf.module_unresolved" {
			modules++
			moduleLocators[gap.Provenance.Anchor] = true
		}
		if gap.Kind == "buf.declarations_only" {
			scope++
			if !strings.HasSuffix(gap.Provenance.Anchor, "#") {
				t.Fatalf("scope anchor=%q, want #", gap.Provenance.Anchor)
			}
		}
	}
	if modules != 2 || scope != 1 {
		t.Fatalf("module gaps=%d scope=%d", modules, scope)
	}
	for _, want := range []string{"#/modules/0", "#/modules/1"} {
		found := false
		for anchor := range moduleLocators {
			found = found || strings.HasSuffix(anchor, want)
		}
		if !found {
			t.Fatalf("module locator %q missing: %#v", want, moduleLocators)
		}
	}
}

func TestAnalyzeBufRejectsInvalidConfig(t *testing.T) {
	cases := []struct{ name, data, code string }{
		{"dependency", `{"version":"v2","modules":[{"path":"api"}],"deps":["buf.build/x/y"]}`, "stage4.unsupported_declaration"},
		{"duplicate path", `{"version":"v2","modules":[{"path":"api"},{"path":"api"}]}`, "stage4.schema_invalid"},
		{"unsafe path", `{"version":"v2","modules":[{"path":"../api"}]}`, "stage4.schema_invalid"},
		{"dot path", `{"version":"v2","modules":[{"path":"api/./v1"}]}`, "stage4.schema_invalid"},
		{"windows path", `{"version":"v2","modules":[{"path":"C:\\private\\api"}]}`, "stage4.schema_invalid"},
		{"unknown key", `{"version":"v2","modules":[{"path":"api","name":"bad"}]}`, "stage4.schema_invalid"},
		{"wrong version", `{"version":"v1","modules":[{"path":"api"}]}`, "stage4.unsupported_version"},
		{"secret metadata", `{"version":"v2","modules":[{"path":"api"}],"x":"token=abcdef0123456789abcdef"}`, "stage4.secret_indicator"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := AnalyzeBuf(bufInput(tc.data))
			var rule *RuleError
			if out != nil || !errors.As(err, &rule) || rule.Code != tc.code {
				t.Fatalf("out=%q err=%v want=%s", out, err, tc.code)
			}
		})
	}
}

func TestAnalyzeBufLoweredBudgetIsAtomic(t *testing.T) {
	limits := defaultBudget
	limits.records = 0
	out, err := analyzeBufWithBudget(bufInput(`{"version":"v2","modules":[{"path":"api"}]}`), limits)
	var rule *RuleError
	if out != nil || !errors.As(err, &rule) || rule.Code != "stage4.preflight_limit" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
