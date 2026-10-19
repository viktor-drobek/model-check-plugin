package modelcheck_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"modelcheck/trace"
	"modelcheck/trace/protocol"
)

func init() { stepRegistrars = append(stepRegistrars, registerG9ProtocolSteps) }

func registerG9ProtocolSteps(sc *godog.ScenarioContext) {
	var run func() ([]byte, error)
	var output []byte
	var adapterErr error
	var document *trace.Document
	sc.Step(`^the synthetic offline protocol case "([^"]*)"$`, func(name string) error {
		var err error
		run, err = offlineProtocolCase(name)
		output, adapterErr, document = nil, nil, nil
		return err
	})
	sc.Step(`^the offline protocol adapter consumes the supplied bytes$`, func() error {
		if run == nil {
			return fmt.Errorf("offline protocol case was not configured")
		}
		output, adapterErr = run()
		return nil
	})
	sc.Step(`^the offline protocol output is deterministic schema "([^"]*)"$`, func(schema string) error {
		if adapterErr != nil {
			return adapterErr
		}
		again, err := run()
		if err != nil {
			return err
		}
		if !bytes.Equal(output, again) {
			return fmt.Errorf("offline output is not repeatable")
		}
		document, err = trace.UnmarshalJSON(output)
		if err != nil {
			return err
		}
		if document.Schema != schema {
			return fmt.Errorf("schema %s, want %s", document.Schema, schema)
		}
		canonical, err := trace.JSON(document)
		if err != nil {
			return err
		}
		if !bytes.Equal(output, canonical) {
			return fmt.Errorf("offline output is not canonical JSON")
		}
		for _, ev := range document.Evidence {
			if ev.Level != trace.Decoded && ev.Level != trace.Grammar {
				return fmt.Errorf("unexpected evidence authority %s", ev.Level)
			}
		}
		if schema == trace.SchemaV2 && (len(document.Nodes) != 1 || document.Nodes[0].Kind != trace.GrammarArtifact || len(document.Edges) != 0) {
			return fmt.Errorf("grammar registration was not isolated")
		}
		return nil
	})
	sc.Step(`^the offline protocol output retains the disclosure "([^"]*)"$`, func(kind string) error {
		if document == nil {
			return fmt.Errorf("offline document is absent")
		}
		for _, gap := range document.Gaps {
			if gap.Kind == kind {
				return nil
			}
		}
		return fmt.Errorf("missing offline disclosure %s", kind)
	})
	sc.Step(`^the offline protocol output is absent with rule "([^"]*)"$`, func(code string) error {
		var rejection *protocol.RuleError
		if len(output) != 0 || !errors.As(adapterErr, &rejection) || rejection.Code != code {
			return fmt.Errorf("output bytes=%d, error=%v; want %s", len(output), adapterErr, code)
		}
		return nil
	})
	sc.Step(`^the offline protocol rejection locator is "([^"]*)"$`, func(want string) error {
		var rejection *protocol.RuleError
		if !errors.As(adapterErr, &rejection) {
			return fmt.Errorf("want a protocol RuleError, got %v", adapterErr)
		}
		if rejection.Locator != want {
			return fmt.Errorf("want rejection locator %q, got %q", want, rejection.Locator)
		}
		return nil
	})
}

func offlineProtocolDigest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

// Every case below is synthetic; no capture or third-party grammar is embedded.
func offlineProtocolCase(name string) (func() ([]byte, error), error) {
	if strings.HasPrefix(name, "tshark") {
		input := protocol.TSharkJSONInput{Artifact: protocol.Input{Path: "exports/packet.json"}, CaptureID: "synthetic", TSharkVersion: "4.4.10", Profile: "tshark-4.4.10-core-1", ExportStatus: "complete"}
		text := `[{"layers":[{"name":"frame","fields":{"frame.protocols":"tcp"}},{"name":"tcp","fields":{"tcp.srcport":"443","tcp.dstport":"49152"}}]}]`
		switch name {
		case "tshark":
		case "tshark-unsupported":
			text = strings.ReplaceAll(text, `"tcp"`, `"tls"`)
			text = strings.ReplaceAll(text, `{"tcp.srcport":"443","tcp.dstport":"49152"}`, `{}`)
		case "tshark-omitted":
			text = strings.ReplaceAll(text, `"tcp.srcport":"443","tcp.dstport":"49152"`, ``)
		case "tshark-truncated":
			input.ExportStatus = "truncated"
		case "tshark-unknown":
			input.ExportStatus = "unknown"
		case "tshark-bad-digest":
		case "tshark-bad-version":
			input.TSharkVersion = "4.4.11"
		case "tshark-bad-path":
			input.Artifact.Path = "../packet.json"
		case "tshark-secret-capture":
			input.CaptureID = "auth_token"
		case "tshark-unknown-field":
			text = strings.ReplaceAll(text, "tcp.srcport", "tcp.unknown")
		case "tshark-wrong-layer":
			text = strings.ReplaceAll(text, "tcp.srcport", "ip.proto")
		case "tshark-chain-mismatch":
			text = strings.Replace(text, `frame.protocols":"tcp`, `frame.protocols":"udp`, 1)
		case "tshark-dns-overflow":
			text = `[{"layers":[{"name":"frame","fields":{"frame.protocols":"dns"}},{"name":"dns","fields":{"dns.qry.type":"65536"}}]}]`
		case "tshark-unsupported-fields":
			text = strings.ReplaceAll(text, `"tcp"`, `"tls"`)
		case "tshark-number":
			text = strings.Replace(text, `"443"`, `443`, 1)
		case "tshark-late-invalid":
			text = strings.TrimSuffix(text, "]") + `,{"layers":[]}]`
		case "tshark-escaped-duplicate":
			text = `{"layers":[],"\u006cayers":[]}`
		case "tshark-malformed-duplicate":
			text = `{"x":0,"x":`
		case "tshark-trailing-duplicate":
			text = `{"x":0,"x":1} false`
		case "tshark-trailing-depth":
			text = strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17) + "x"
		case "tshark-duplicate-depth":
			text = `{"x":0,"x":` + strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17) + "}"
		case "tshark-depth-duplicate":
			text = `{"x":` + strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17) + `,"x":0}`
		case "tshark-empty":
			text = `[]`
		case "tshark-only-frame":
			text = `[{"layers":[{"name":"frame","fields":{"frame.protocols":""}}]}]`
		case "tshark-missing-layers":
			text = `[{}]`
		case "tshark-surrogate":
			text = `["\ud800"]`
		case "tshark-bom":
			text = "\ufeff" + text
		case "tshark-oversized":
			text = strings.Repeat(" ", (1<<20)+1)
		default:
			return nil, fmt.Errorf("unknown synthetic case %q", name)
		}
		input.Artifact.Content = []byte(text)
		input.ExportSHA256 = offlineProtocolDigest(input.Artifact.Content)
		if name == "tshark-bad-digest" {
			input.ExportSHA256 = strings.Repeat("0", 64)
		}
		return func() ([]byte, error) { return protocol.AnalyzeTShark(input) }, nil
	}
	if strings.HasPrefix(name, "grammar-reference") {
		input := protocol.GrammarReferenceInput{Artifact: protocol.Input{Path: "grammars/packet.grammar-ref.json"}}
		text := `{"schema":"mcd-grammar-reference/1","language":"kaitai","language_version":"0.10","target_id":"vendor.packet","target_sha256":"` + strings.Repeat("a", 64) + `","declared_anchor":"root"}`
		switch name {
		case "grammar-reference", "grammar-reference-bad-digest":
		case "grammar-reference-duplicate":
			text = strings.Replace(text, `"schema":`, `"schema":"mcd-grammar-reference/1","sch\u0065ma":`, 1)
		case "grammar-reference-secret":
			text = strings.ReplaceAll(text, "vendor.packet", `auth_\u0074oken`)
		case "grammar-reference-unknown":
			text = strings.TrimSuffix(text, "}") + `,"unknown":true}`
		case "grammar-reference-bad-path":
			input.Artifact.Path = "C:/packet.grammar-ref.json"
		default:
			return nil, fmt.Errorf("unknown synthetic case %q", name)
		}
		input.Artifact.Content = []byte(text)
		input.ManifestSHA256 = offlineProtocolDigest(input.Artifact.Content)
		if name == "grammar-reference-bad-digest" {
			input.ManifestSHA256 = strings.Repeat("0", 64)
		}
		return func() ([]byte, error) { return protocol.AnalyzeGrammarReference(input) }, nil
	}
	if strings.HasPrefix(name, "kaitai") || strings.HasPrefix(name, "spicy") {
		input := protocol.GrammarFileInput{Artifact: protocol.Input{Path: "grammars/packet.ksy"}, Language: "kaitai", LanguageVersion: "0.10", DeclaredAnchor: "root"}
		text := "meta:\n  id: packet\nseq: []\n"
		if strings.HasPrefix(name, "spicy") {
			input.Artifact.Path = "grammars/packet.spicy"
			input.Language = "spicy"
			input.LanguageVersion = "1.12.0"
			text = "module Packet;\ntype Packet = unit {};\n"
		}
		switch name {
		case "kaitai", "spicy", "kaitai-bad-digest":
		case "kaitai-imports":
			text = "{ imports: [] }"
		case "kaitai-marker":
			text = "- id: auth_token"
		case "kaitai-reverse-solidus":
			text = "a\\b"
		case "kaitai-crlf":
			text = "meta:\r\n"
		case "kaitai-bad-anchor":
			input.DeclaredAnchor = "root/../name"
		case "kaitai-bad-path":
			input.Artifact.Path = "/packet.ksy"
		case "spicy-include":
			text = "include Packet;"
		case "spicy-comment-marker":
			text = "module Packet;\n# password hint"
		case "kaitai-first-import":
			text = "x imports password\\\xff"
		case "kaitai-first-marker":
			text = "password imports\\\xff"
		case "kaitai-first-backslash":
			text = "a\\password imports\xff"
		case "kaitai-first-utf8":
			text = "ab\xff imports password\\"
		case "spicy-first-include":
			text = "xx include password\\\xff"
		case "spicy-first-control":
			text = "\tX\x00password include\\"
		default:
			return nil, fmt.Errorf("unknown synthetic case %q", name)
		}
		input.Artifact.Content = []byte(text)
		input.GrammarSHA256 = offlineProtocolDigest(input.Artifact.Content)
		if name == "kaitai-bad-digest" {
			input.GrammarSHA256 = strings.Repeat("0", 64)
		}
		return func() ([]byte, error) { return protocol.AnalyzeGrammar(input) }, nil
	}
	return nil, fmt.Errorf("unknown synthetic case %q", name)
}
