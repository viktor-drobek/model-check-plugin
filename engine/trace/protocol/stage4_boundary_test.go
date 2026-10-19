package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"modelcheck/trace"
)

func stage4BoundaryDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func stage4BoundaryCalls(t *testing.T) map[string]func([]byte) ([]byte, error) {
	t.Helper()
	contract := func(analyze func(ContractInput) ([]byte, error)) func([]byte) ([]byte, error) {
		return func(data []byte) ([]byte, error) {
			return analyze(ContractInput{Artifact: Input{Path: "fixtures/declaration.json", Content: data}, ContentSHA256: stage4BoundaryDigest(data)})
		}
	}
	return map[string]func([]byte) ([]byte, error){
		"openapi":  contract(AnalyzeOpenAPI),
		"asyncapi": contract(AnalyzeAsyncAPI),
		"protobuf": contract(AnalyzeProtobuf),
		"buf":      contract(AnalyzeBuf),
		"pact":     contract(AnalyzePact),
		"runtime": func(data []byte) ([]byte, error) {
			return AnalyzeRuntimeSpans(RuntimeSpansInput{Artifact: Input{Path: "fixtures/spans.json", Content: data}, ExportSHA256: stage4BoundaryDigest(data)})
		},
	}
}

func stage4BoundaryFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	raw := []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
	runtime, err := json.Marshal(map[string]any{
		"schema": "mcd-runtime-span-export/1",
		"spans": []map[string]string{{
			"trace_id": "trace.demo", "content_sha256": stage4BoundaryDigest(raw), "raw": string(raw),
			"build_revision": "", "collector_name": "", "collector_version": "",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{
		"openapi":  []byte(`{"openapi":"3.1.0","info":{"title":"Demo","version":"1"},"paths":{"/accept":{"post":{"operationId":"accept","responses":{"200":{"description":"Accepted"}}}}}}`),
		"asyncapi": []byte(`{"asyncapi":"2.6.0","info":{"title":"Demo","version":"1"},"channels":{"updates":{"publish":{"operationId":"publishUpdate","message":{"name":"Notice","payload":{"type":"object","properties":{"value":{"type":"string"}}}}}}}}`),
		"protobuf": []byte(`syntax = "proto3"; message Request { string value = 1; } service Demo { rpc Accept (Request) returns (Request); }`),
		"buf":      []byte(`{"version":"v2","modules":[{"path":"schemas"}],"deps":[]}`),
		"pact":     []byte(`{"consumer":{"name":"Consumer"},"provider":{"name":"Provider"},"metadata":{"pactSpecification":{"version":"4.0"}},"interactions":[{"key":"accept","type":"Synchronous/HTTP","description":"Accept","request":{"method":"POST","path":"/accept"},"response":{"status":200}}]}`),
		"runtime":  runtime,
	}
}

func TestStage4SharedAdmissionAcrossFamilies(t *testing.T) {
	fixtures := stage4BoundaryFixtures(t)
	for name, analyze := range stage4BoundaryCalls(t) {
		t.Run(name, func(t *testing.T) {
			data := fixtures[name]
			admission <- struct{}{}
			out, err := analyze(data)
			<-admission
			var ruleErr *RuleError
			if out != nil || !errors.As(err, &ruleErr) || ruleErr.Code != "stage4.resource_admission_busy" || ruleErr.Locator != "input" {
				t.Fatalf("shared admission: output=%d error=%#v", len(out), err)
			}
			first, err := analyze(data)
			if err != nil {
				t.Fatalf("valid call after busy rejection: %v", err)
			}
			second, err := analyze(data)
			if err != nil || !bytes.Equal(first, second) {
				t.Fatalf("repeated call is not deterministic: %v", err)
			}
			doc, err := trace.UnmarshalJSON(first)
			if err != nil {
				t.Fatal(err)
			}
			if doc.Schema != trace.Schema || len(doc.Gaps) == 0 {
				t.Fatal("bounded declaration/observation must retain its scope gap")
			}
			want := trace.Contract
			if name == "runtime" {
				want = trace.Runtime
			}
			if name != "buf" && len(doc.Evidence) == 0 {
				t.Fatal("direct declarations/observations lack evidence")
			}
			for _, evidence := range doc.Evidence {
				if evidence.Level != want {
					t.Fatalf("evidence authority upgraded: got %s want %s", evidence.Level, want)
				}
			}
		})
	}
}

func TestStage4RejectedInputsReleaseAdmissionAcrossFamilies(t *testing.T) {
	fixtures := stage4BoundaryFixtures(t)
	for name, analyze := range stage4BoundaryCalls(t) {
		t.Run(name, func(t *testing.T) {
			for _, data := range [][]byte{nil, []byte("{"), []byte("{} {}"), bytes.Repeat([]byte("x"), (1<<20)+1)} {
				out, err := analyze(data)
				var ruleErr *RuleError
				if out != nil || !errors.As(err, &ruleErr) {
					t.Fatalf("rejection must be atomic and normalized: output=%d error=%#v", len(out), err)
				}
				switch ruleErr.Locator {
				case "input", "metadata", "digest", "json", "schema", "budget":
				default:
					t.Fatalf("nonconstant error locator %q", ruleErr.Locator)
				}
				if len(admission) != 0 {
					t.Fatal("rejected input leaked admission")
				}
			}
			if _, err := analyze(fixtures[name]); err != nil {
				t.Fatalf("valid call after malformed inputs: %v", err)
			}
		})
	}
}
