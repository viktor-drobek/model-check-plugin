package protocol

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// All JSON fixtures are generated synthetic admission probes.
func TestJSONOrderedAdmission(t *testing.T) {
	cases := []struct{ name, text, code string }{
		{"BOM", "\ufeff{}", "utf8_invalid"},
		{"invalid UTF8", string([]byte{0xff}), "utf8_invalid"},
		{"incomplete duplicate", `{"x":0,"x":`, "json_syntax"},
		{"trailing duplicate", `{"x":0,"x":1} false`, "json_trailing_data"},
		{"trailing surrogate", `{} "\ud800"`, "json_trailing_data"},
		{"trailing depth", strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17) + "x", "json_trailing_data"},
		{"depth first", `{"x":` + strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17) + `,"x":0}`, "json_depth"},
		{"duplicate first", `{"x":0,"x":` + strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17) + `}`, "json_duplicate_name"},
		{"escaped duplicate", `{"a":0,"\u0061":1}`, "json_duplicate_name"},
		{"surrogate", `"\uD800"`, "json_syntax"},
		{"malformed deep root", strings.Repeat("[", 17) + "0" + strings.Repeat("]", 16), "json_syntax"},
		{"very deep root", strings.Repeat("[", 12000) + "0" + strings.Repeat("]", 12000), "json_depth"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeJSON([]byte(tc.text))
			var r *RuleError
			if !errors.As(err, &r) || r.Code != "stage3b."+tc.code {
				t.Fatalf("got %v want %s", err, tc.code)
			}
		})
	}
	for _, s := range []string{`null`, `1e9999`, `{"a":"\uD840\uDC00"}`, strings.Repeat("[", 16) + "0" + strings.Repeat("]", 16), `{"password":"password"}`} {
		if _, err := decodeJSON([]byte(s)); err != nil {
			t.Fatalf("lexical-only decode %q: %v", s, err)
		}
	}
}

func TestStructuredSchemaBeforeSecret(t *testing.T) {
	for _, mutation := range []string{`"extra":"password"`, `"password":true`} {
		in := referenceTestInput()
		in.Artifact.Content = append(bytes.TrimSuffix(in.Artifact.Content, []byte("}")), []byte(","+mutation+"}")...)
		in.ManifestSHA256 = grammarTestDigest(in.Artifact.Content)
		out, err := AnalyzeGrammarReference(in)
		requireGrammarRule(t, out, err, "schema_invalid")
	}
}

func TestStructuredSecretExpressions(t *testing.T) {
	for _, s := range []string{"AUTHORIZATION", "cookie", "passwd", "api-key", "private_key", "credential", "bearer abcdefgh", "BASIC abcdefgh", "-----BEGIN RSA PRIVATE KEY-----", "AKIAABCDEFGHIJKLMNOP"} {
		if !secretValue(s) {
			t.Fatalf("missed synthetic marker %q", s)
		}
	}
	for _, s := range []string{"basic handshake", "bearer abcdefg", "akiaABCDEFGHIJKLMNOP", "ordinary"} {
		// The basic handshake string exceeds the eight-character value threshold.
		if s == "basic handshake" {
			if !secretValue(s) {
				t.Fatal("structured Basic rule missing")
			}
			continue
		}
		if secretValue(s) {
			t.Fatalf("overbroad value rule %q", s)
		}
	}
	// Member names use only the marker expression, not the value expressions.
	if err := scanStructured(map[string]any{"AKIAABCDEFGHIJKLMNOP": "ordinary"}); err != nil {
		t.Fatal(err)
	}
	if err := scanStructured(map[string]any{"pass\u0077ord": "ordinary"}); err == nil {
		t.Fatal("member marker missing")
	}
}

func TestSecretPrefiltersPreserveRegexpSemantics(t *testing.T) {
	words := []string{"authorization", "cookie", "password", "passwd", "secret", "token", "apikey", "api_key", "api-key", "privatekey", "private_key", "private-key", "credential", "bearer abcdefgh", "basic abcdefgh", "-----BEGIN RSA PRIVATE KEY-----", "AKIAABCDEFGHIJKLMNOP", "ordinary", "dns.qry.type"}
	for _, word := range words {
		for _, text := range []string{word, strings.ToUpper(word), "prefix." + word + ".suffix", strings.ReplaceAll(strings.ReplaceAll(word, "s", "ſ"), "k", "K")} {
			want := marker.MatchString(text) || bearer.MatchString(text) || pem.MatchString(text) || aws.MatchString(text)
			if got := secretValue(text); got != want {
				t.Errorf("secretValue(%q)=%v want %v", text, got, want)
			}
			if got, want := secretMarker(text), marker.MatchString(text); got != want {
				t.Errorf("secretMarker(%q)=%v want %v", text, got, want)
			}
		}
	}
}

func TestBudgetArithmetic(t *testing.T) {
	if got := uint64(2000*15*14 + 3); got != defaultBudget.records {
		t.Fatalf("record bound %d", got)
	}
	if got := 678 + 54 + 14 + 29; got != 775 {
		t.Fatal(got)
	}
	unlimited := ^uint64(0)
	for _, tc := range []struct {
		name            string
		records, output uint64
		b               budget
		reject          bool
	}{
		{"record max", 420003, 1, budget{420003, unlimited, unlimited}, false},
		{"record overflow", 420004, 1, budget{420003, unlimited, unlimited}, true},
		{"output max", 0, 512 << 20, budget{unlimited, 512 << 20, unlimited}, false},
		{"output overflow", 0, (512 << 20) + 1, budget{unlimited, 512 << 20, unlimited}, true},
		{"working max", 1, 100, budget{unlimited, unlimited, 200 + 256 + (2 << 20)}, false},
		{"working overflow", 1, 101, budget{unlimited, unlimited, 200 + 256 + (2 << 20)}, true},
		{"arithmetic overflow", unlimited, unlimited, budget{unlimited, unlimited, unlimited}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkBudget(tc.records, tc.output, tc.b)
			if (err != nil) != tc.reject {
				t.Fatalf("records=%d output=%d err=%v", tc.records, tc.output, err)
			}
		})
	}
}

func TestAdmissionReleasesOnRejection(t *testing.T) {
	for i := 0; i < 3; i++ {
		in := referenceTestInput()
		in.ManifestSHA256 = strings.Repeat("0", 64)
		out, err := AnalyzeGrammarReference(in)
		requireGrammarRule(t, out, err, "digest_mismatch")
		if _, err := AnalyzeGrammarReference(referenceTestInput()); err != nil {
			t.Fatalf("leaked admission %d: %v", i, err)
		}
	}
}

func TestJSONDecoderAgreesWithStandardValidScalars(t *testing.T) {
	for _, s := range []string{`"\\"`, `"\""`, `"\b\f\n\r\t\/"`, `-0.1e+20`, `[true,false,null,{},[]]`} {
		if _, err := decodeJSON([]byte(s)); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	for n := 1; n <= 16; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			if _, err := decodeJSON([]byte(strings.Repeat("{\"x\":", n) + "0" + strings.Repeat("}", n))); err != nil {
				t.Fatal(err)
			}
		})
	}
}
