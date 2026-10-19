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

// All exports in this file are synthetic. No producer build, collector or
// capture data from any third party is embedded.

func runtimeDigest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func rawSpanJSON(fields string) string {
	return `{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1,` + strings.TrimSuffix(fields, ",") + `}`
}

func runtimeRaw(spanID, endNS, parentID string) []byte {
	raw := `{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"` + spanID + `","parent_id":"` + parentID + `","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":` + endNS + `}`
	return []byte(raw)
}

// runtimeEntry builds one export entry; declared is the raw digest unless "".
func runtimeEntry(raw []byte, declared string) map[string]any {
	if declared == "" {
		declared = runtimeDigest(raw)
	}
	return map[string]any{
		"trace_id":          "trace.demo",
		"content_sha256":    declared,
		"raw":               string(raw),
		"build_revision":    "",
		"collector_name":    "",
		"collector_version": "",
	}
}

// runtimeExport builds a canonical export object around the given entries.
func runtimeExport(entries ...map[string]any) []byte {
	out := map[string]any{"schema": "mcd-runtime-span-export/1", "spans": entries}
	b, err := json.Marshal(out)
	if err != nil {
		panic(fmt.Sprintf("marshal synthetic runtime export: %v", err))
	}
	return b
}

func runtimeInput(content []byte) RuntimeSpansInput {
	return RuntimeSpansInput{Artifact: Input{Path: "exports/runtime.json", Content: content}, ExportSHA256: runtimeDigest(content)}
}

func requireRuntimeRule(t *testing.T, output []byte, err error, code, locator string) {
	t.Helper()
	var e *RuleError
	if output != nil || !errors.As(err, &e) || e.Code != "stage4."+code || e.Locator != locator {
		t.Fatalf("output=%v error=%v; want nil and stage4.%s locator %q", output, err, code, locator)
	}
}

func TestRuntimeSpanProjection(t *testing.T) {
	content := runtimeExport(runtimeEntry(runtimeRaw("span.one", "1", ""), ""))
	output, err := AnalyzeRuntimeSpans(runtimeInput(content))
	if err != nil {
		t.Fatal(err)
	}
	document, err := trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if document.Schema != trace.Schema {
		t.Fatalf("schema %q", document.Schema)
	}
	canonical, err := trace.JSON(document)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, canonical) {
		t.Fatal("output is not canonical")
	}
	if len(document.Nodes) != 1 || document.Nodes[0].Kind != trace.RuntimeSpan {
		t.Fatalf("nodes=%v", document.Nodes)
	}
	node := document.Nodes[0]
	if want := "Observed span accept (server, ok), 0..1 ns"; node.Label != want {
		t.Fatalf("label %q, want %q", node.Label, want)
	}
	anchor := "span:exports/runtime.json#trace=trace.demo&span=span.one&parent=&sha256=" + runtimeDigest(runtimeRaw("span.one", "1", "")) + "&occurrences=1"
	if node.Provenance.Anchor != anchor {
		t.Fatalf("anchor %q", node.Provenance.Anchor)
	}
	if node.ID != "span."+tupleHash("runtime-span/1", "exports/runtime.json", "trace.demo", runtimeDigest(runtimeRaw("span.one", "1", ""))) {
		t.Fatalf("node id %q", node.ID)
	}
	if len(document.Evidence) != 1 || document.Evidence[0].Subject != "node:"+node.ID || document.Evidence[0].Level != trace.Runtime || document.Evidence[0].ID != "evidence.span."+node.ID[len("span."):len(node.ID)] {
		t.Fatalf("evidence=%v", document.Evidence)
	}
	if len(document.Evidence[0].ID) != 0 && document.Evidence[0].ID != "evidence.span."+strings.TrimPrefix(node.ID, "span.") {
		t.Fatalf("evidence id %q", document.Evidence[0].ID)
	}
	if node.Provenance.ContentSHA256 != runtimeDigest(runtimeRaw("span.one", "1", "")) {
		t.Fatalf("provenance digest %q", node.Provenance.ContentSHA256)
	}
	if node.Provenance.SourceRevision != "" {
		t.Fatalf("source revision %q", node.Provenance.SourceRevision)
	}
	if got := node.Provenance.Producer; got.Name != "mcd-runtime-span-adapter" || got.Version != "1;collector=unavailable@unavailable" {
		t.Fatalf("producer %v", got)
	}
	if len(document.Gaps) != 2 {
		t.Fatalf("gaps=%v", document.Gaps)
	}
	buildGap := "gap.span.build_missing." + strings.TrimPrefix(node.ID, "span.")
	collectorGap := "gap.span.collector_missing." + strings.TrimPrefix(node.ID, "span.")
	seen := map[string]bool{}
	for _, gap := range document.Gaps {
		if gap.Reference != "node:"+node.ID {
			t.Fatalf("gap reference %q", gap.Reference)
		}
		if gap.Provenance != node.Provenance {
			t.Fatalf("gap provenance differs")
		}
		seen[gap.ID] = true
	}
	if !seen[buildGap] || !seen[collectorGap] {
		t.Fatalf("gap ids=%v, want %q and %q", seen, buildGap, collectorGap)
	}
	again, err := AnalyzeRuntimeSpans(runtimeInput(content))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, again) {
		t.Fatal("re-run output differs")
	}
}

func TestRuntimeSpanMetadataVariants(t *testing.T) {
	build := strings.Repeat("a", 40)
	raw := runtimeRaw("span.one", "1", "")

	buildCase := runtimeInput(buildRuntimeEntry(raw, build, "", ""))
	output, err := AnalyzeRuntimeSpans(buildCase)
	if err != nil {
		t.Fatal(err)
	}
	document, err := trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Nodes) != 1 || document.Nodes[0].Provenance.SourceRevision != build {
		t.Fatalf("build provenance %v", document.Nodes)
	}
	if len(document.Gaps) != 1 || document.Gaps[0].Kind != "span.provenance_missing" {
		t.Fatalf("gaps=%v", document.Gaps)
	}

	collectorCase := runtimeInput(buildRuntimeEntry(raw, "", "collector", "1.0.0"))
	output, err = AnalyzeRuntimeSpans(collectorCase)
	if err != nil {
		t.Fatal(err)
	}
	document, err = trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if got := document.Nodes[0].Provenance.Producer; got.Version != "1;collector=collector@1.0.0" {
		t.Fatalf("collector producer %q", got.Version)
	}
	if len(document.Gaps) != 1 || document.Gaps[0].Kind != "span.provenance_missing" || document.Gaps[0].Reason != "producer build revision was not supplied" {
		t.Fatalf("gaps=%v", document.Gaps)
	}

	full := runtimeInput(buildRuntimeEntry(raw, build, "collector", "1.0.0"))
	output, err = AnalyzeRuntimeSpans(full)
	if err != nil {
		t.Fatal(err)
	}
	document, err = trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Gaps) != 0 {
		t.Fatalf("full metadata gaps=%v", document.Gaps)
	}
}

func buildRuntimeEntry(raw []byte, build, collector, version string) []byte {
	entries := []map[string]any{
		{
			"trace_id":          "trace.demo",
			"content_sha256":    runtimeDigest(raw),
			"raw":               string(raw),
			"build_revision":    build,
			"collector_name":    collector,
			"collector_version": version,
		},
	}
	return runtimeExport(entries...)
}

func TestRuntimeSpanParentGap(t *testing.T) {
	raw := runtimeRaw("span.one", "1", "span.parent")
	output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(raw, ""))))
	if err != nil {
		t.Fatal(err)
	}
	document, err := trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Nodes) != 1 || len(document.Edges) != 0 {
		t.Fatalf("nodes=%v edges=%v", document.Nodes, document.Edges)
	}
	kind := map[string]int{}
	for _, gap := range document.Gaps {
		kind[gap.Kind]++
		if gap.Reference != "node:"+document.Nodes[0].ID {
			t.Fatalf("gap reference %q", gap.Reference)
		}
	}
	if kind["span.provenance_missing"] != 2 || kind["span.parent_unmapped"] != 1 {
		t.Fatalf("gap kinds=%v", kind)
	}
	for _, gap := range document.Gaps {
		if gap.Kind == "span.parent_unmapped" && gap.Reason != "parent identity was recorded without inferring a communication relation" {
			t.Fatalf("parent gap reason %q", gap.Reason)
		}
	}
	if anchor := document.Nodes[0].Provenance.Anchor; !strings.Contains(anchor, "parent=span.parent") {
		t.Fatalf("anchor %q", anchor)
	}
}

func TestRuntimeSpanDeduplication(t *testing.T) {
	raw := runtimeRaw("span.one", "1", "")
	output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(raw, ""), runtimeEntry(raw, ""))))
	if err != nil {
		t.Fatal(err)
	}
	document, err := trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Nodes) != 1 || len(document.Evidence) != 1 {
		t.Fatalf("dedup nodes=%d evidence=%d", len(document.Nodes), len(document.Evidence))
	}
	if anchor := document.Nodes[0].Provenance.Anchor; !strings.Contains(anchor, "occurrences=2") {
		t.Fatalf("anchor %q", anchor)
	}

	// Same trace, different bytes: distinct nodes even for the same span_id.
	other := runtimeRaw("span.one", "2", "")
	output, err = AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(raw, ""), runtimeEntry(other, ""))))
	if err != nil {
		t.Fatal(err)
	}
	document, err = trace.UnmarshalJSON(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Nodes) != 2 {
		t.Fatalf("nodes=%d", len(document.Nodes))
	}
	for i, node := range document.Nodes {
		if i != 0 && node.Provenance.Anchor == document.Nodes[0].Provenance.Anchor {
			t.Fatalf("identical anchors across distinct spans")
		}
	}

	// Reordering distinguishable records, including an occurrence group, must
	// not change canonical output.
	rawB := runtimeRaw("span.two", "2", "")
	ordered := runtimeInput(runtimeExport(runtimeEntry(raw, ""), runtimeEntry(rawB, "")))
	reversed := runtimeInput(runtimeExport(runtimeEntry(rawB, ""), runtimeEntry(raw, "")))
	orderedOutput, err := AnalyzeRuntimeSpans(ordered)
	if err != nil {
		t.Fatal(err)
	}
	reversedOutput, err := AnalyzeRuntimeSpans(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(orderedOutput, reversedOutput) {
		t.Fatal("distinct-span reorder changed output")
	}
	grouped := runtimeInput(runtimeExport(runtimeEntry(raw, ""), runtimeEntry(rawB, ""), runtimeEntry(raw, "")))
	permutedGrouped := runtimeInput(runtimeExport(runtimeEntry(rawB, ""), runtimeEntry(raw, ""), runtimeEntry(raw, "")))
	groupedOutput, err := AnalyzeRuntimeSpans(grouped)
	if err != nil {
		t.Fatal(err)
	}
	permutedGroupedOutput, err := AnalyzeRuntimeSpans(permutedGrouped)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(groupedOutput, permutedGroupedOutput) {
		t.Fatal("occurrence-group reorder changed output")
	}
}

func TestRuntimeSpanMetadataPrecedenceAcrossEntries(t *testing.T) {
	malformed := runtimeEntry([]byte("{"), "")
	badMetadata := runtimeEntry(runtimeRaw("span.two", "1", ""), "")
	badMetadata["build_revision"] = strings.Repeat("A", 40)
	output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(malformed, badMetadata)))
	requireRuntimeRule(t, output, err, "metadata_invalid", "input")
}

func TestRuntimeSpanConflictingMetadata(t *testing.T) {
	raw := runtimeRaw("span.one", "1", "")
	entries := []map[string]any{
		{
			"trace_id":          "trace.demo",
			"content_sha256":    runtimeDigest(raw),
			"raw":               string(raw),
			"build_revision":    strings.Repeat("a", 40),
			"collector_name":    "",
			"collector_version": "",
		},
		{
			"trace_id":          "trace.demo",
			"content_sha256":    runtimeDigest(raw),
			"raw":               string(raw),
			"build_revision":    strings.Repeat("b", 64),
			"collector_name":    "",
			"collector_version": "",
		},
	}
	output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(entries...)))
	requireRuntimeRule(t, output, err, "identity_ambiguous", "input")
}

func TestRuntimeSpanDigestMismatchPrecedence(t *testing.T) {
	raw := runtimeRaw("span.one", "1", "")
	// Well-formed declared digest that does not match the raw bytes.
	output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(raw, strings.Repeat("0", 64)))))
	requireRuntimeRule(t, output, err, "digest_mismatch", "input")

	// A valid first occurrence must not bypass digest validation of a later
	// duplicate. The whole export fails atomically, not identity_ambiguous.
	output, err = AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(raw, ""), runtimeEntry(raw, strings.Repeat("0", 64)))))
	requireRuntimeRule(t, output, err, "digest_mismatch", "input")

	// Envelope digest mismatch wins over raw content.
	content := runtimeExport(runtimeEntry(raw, ""))
	output, err = AnalyzeRuntimeSpans(RuntimeSpansInput{Artifact: Input{Path: "exports/runtime.json", Content: content}, ExportSHA256: strings.Repeat("0", 64)})
	requireRuntimeRule(t, output, err, "digest_mismatch", "input")

	// Raw trace_id agreement fails only after the declared digest verifies.
	otherTrace := []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.other","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
	output, err = AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(otherTrace, ""))))
	requireRuntimeRule(t, output, err, "identity_ambiguous", "input")
}

func TestRuntimeSpanBounds(t *testing.T) {
	// Entry count above 1024.
	entries := make([]map[string]any, 0, 1025)
	for i := 0; i < 1025; i++ {
		entries = append(entries, runtimeEntry(runtimeRaw(fmt.Sprintf("span.%03d", i), "1", ""), ""))
	}
	output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(entries...)))
	requireRuntimeRule(t, output, err, "span_limit", "input")

	// Decoded raw above 4096 bytes with a correct digest.
	longName := strings.Repeat("n", 4080)
	longRaw := []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"` + longName + `","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
	output, err = AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(longRaw, ""))))
	requireRuntimeRule(t, output, err, "input_length", "input")

	// Export above 1 MiB.
	big := runtimeRaw("span.one", "1", "")
	base := runtimeExport(runtimeEntry(big, ""))
	base = append(base, bytes.Repeat([]byte(" "), (1<<20+1)-len(base))...)
	if len(base) <= 1<<20 {
		t.Fatalf("test setup: export %d bytes", len(base))
	}
	output, err = AnalyzeRuntimeSpans(runtimeInput(base))
	requireRuntimeRule(t, output, err, "input_length", "input")

	// 65-byte path and 65-byte ID/name scalars.
	longPath := runtimeInput(runtimeExport(runtimeEntry(runtimeRaw("span.one", "1", ""), "")))
	longPath.Artifact.Path = strings.Repeat("a", 65)
	output, err = AnalyzeRuntimeSpans(longPath)
	requireRuntimeRule(t, output, err, "metadata_invalid", "input")

	badEntry := runtimeEntry(runtimeRaw("span.one", "1", ""), "")
	badEntry["trace_id"] = strings.Repeat("a", 65)
	output, err = AnalyzeRuntimeSpans(runtimeInput(runtimeExport(badEntry)))
	requireRuntimeRule(t, output, err, "metadata_invalid", "input")
}

func TestRuntimeSpanClosedBoundaries(t *testing.T) {
	raw := runtimeRaw(strings.Repeat("s", 64), "1", strings.Repeat("p", 64))
	raw = []byte(strings.Replace(string(raw), `"name":"accept"`, `"name":"`+strings.Repeat("n", 64)+`"`, 1))
	validEntry := runtimeEntry(raw, "")
	validEntry["build_revision"] = strings.Repeat("a", 64)
	validEntry["collector_name"] = strings.Repeat("c", 64)
	validEntry["collector_version"] = strings.Repeat("v", 64)
	input := runtimeInput(runtimeExport(validEntry))
	if output, err := AnalyzeRuntimeSpans(input); err != nil || output == nil {
		t.Fatalf("exact scalar bounds rejected: output=%v error=%v", output, err)
	}

	pathInput := input
	pathInput.Artifact.Path = strings.Repeat("a", 64)
	if output, err := AnalyzeRuntimeSpans(pathInput); err != nil || output == nil {
		t.Fatalf("exact path bound rejected: output=%v error=%v", output, err)
	}

	baseRaw := runtimeRaw("span.one", "1", "")
	rawAtLimit := append(append([]byte(nil), baseRaw...), bytes.Repeat([]byte(" "), 4096-len(baseRaw))...)
	output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(rawAtLimit, ""))))
	if err != nil || output == nil {
		t.Fatalf("exact raw bound rejected: output=%v error=%v", output, err)
	}

	noncanonical := map[string]string{
		"-0":           `"start_ns":-0,"end_ns":1`,
		"fraction":     `"start_ns":0.0,"end_ns":1`,
		"exponent":     `"start_ns":0e0,"end_ns":1`,
		"string":       `"start_ns":"0","end_ns":1`,
		"leading-zero": `"start_ns":01,"end_ns":1`,
	}
	for name, numbers := range noncanonical {
		t.Run(name, func(t *testing.T) {
			raw := []byte(strings.Replace(string(runtimeRaw("span.one", "1", "")), `"start_ns":0,"end_ns":1`, numbers, 1))
			output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(raw, ""))))
			code := "schema_invalid"
			locator := "input"
			if name == "leading-zero" {
				code = "json_syntax"
				locator = "json"
			}
			requireRuntimeRule(t, output, err, code, locator)
		})
	}

	for name, mutate := range map[string]func(map[string]any){
		"collector-name-alphabet":    func(e map[string]any) { e["collector_name"], e["collector_version"] = "collector/name", "1" },
		"collector-version-alphabet": func(e map[string]any) { e["collector_name"], e["collector_version"] = "collector", "version 1" },
		"envelope-secret":            func(e map[string]any) { e["collector_name"], e["collector_version"] = "token", "1" },
	} {
		t.Run(name, func(t *testing.T) {
			e := runtimeEntry(runtimeRaw("span.one", "1", ""), "")
			mutate(e)
			output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(e)))
			code := "metadata_invalid"
			if name == "envelope-secret" {
				code = "secret_indicator"
			}
			requireRuntimeRule(t, output, err, code, "input")
		})
	}

	for field, replacement := range map[string]string{
		"span_id":   strings.Repeat("s", 65),
		"parent_id": strings.Repeat("p", 65),
		"name":      strings.Repeat("n", 65),
	} {
		t.Run("overlength-"+field, func(t *testing.T) {
			raw := runtimeRaw("span.one", "1", "")
			quoted := `"` + field + `":"` + map[string]string{"span_id": "span.one", "parent_id": "", "name": "accept"}[field] + `"`
			raw = []byte(strings.Replace(string(raw), quoted, `"`+field+`":"`+replacement+`"`, 1))
			output, err := AnalyzeRuntimeSpans(runtimeInput(runtimeExport(runtimeEntry(raw, ""))))
			requireRuntimeRule(t, output, err, "schema_invalid", "input")
		})
	}
}

func TestRuntimeSpanScalarAndFormatRules(t *testing.T) {
	cases := map[string]func() []byte{
		"runtime-bad-build": func() []byte {
			raw := runtimeRaw("span.one", "1", "")
			return buildRuntimeEntry(raw, strings.Repeat("A", 40), "", "")
		},
		"runtime-partial-collector": func() []byte {
			raw := runtimeRaw("span.one", "1", "")
			return buildRuntimeEntry(raw, "", "collector", "")
		},
		"runtime-secret-name": func() []byte {
			return []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"auth_token","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
		},
		"runtime-unknown-field": func() []byte {
			return []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1,"note":"x"}`)
		},
		"runtime-entry-extra-field": func() []byte {
			entries := []map[string]any{runtimeEntry(runtimeRaw("span.one", "1", ""), "")}
			entries[0]["vendor"] = "x"
			return runtimeExport(entries...)
		},
		"runtime-bad-schema": func() []byte {
			return []byte(`{"schema":"mcd-runtime-span/2","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
		},
		"runtime-bad-kind": func() []byte {
			return []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"worker","status":"ok","start_ns":0,"end_ns":1}`)
		},
		"runtime-bad-status": func() []byte {
			return []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"done","start_ns":0,"end_ns":1}`)
		},
		"runtime-long-raw-trace-id": func() []byte {
			return []byte(`{"schema":"mcd-runtime-span/1","trace_id":"` + strings.Repeat("a", 65) + `","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
		},
		"runtime-number-bound": func() []byte {
			return []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":9223372036854775808,"end_ns":9223372036854775808}`)
		},
		"runtime-negative-number": func() []byte {
			return []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":-1,"end_ns":1}`)
		},
		"runtime-end-before-start": func() []byte {
			raw := []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":2,"end_ns":1}`)
			entries := []map[string]any{runtimeEntry(raw, "")}
			return runtimeExport(entries...)
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			content := build()
			if name != "runtime-bad-build" && name != "runtime-partial-collector" && name != "runtime-entry-extra-field" && name != "runtime-end-before-start" {
				content = runtimeExport(runtimeEntry(content, ""))
			}
			output, err := AnalyzeRuntimeSpans(runtimeInput(content))
			code := "schema_invalid"
			if name == "runtime-bad-build" || name == "runtime-partial-collector" {
				code = "metadata_invalid"
			}
			if name == "runtime-secret-name" {
				code = "secret_indicator"
			}
			requireRuntimeRule(t, output, err, code, "input")
		})
	}
}

func TestRuntimeSpanJSONSyntaxRules(t *testing.T) {
	bom := append([]byte{0xEF, 0xBB, 0xBF}, runtimeExport(runtimeEntry(runtimeRaw("span.one", "1", ""), ""))...)
	output, err := AnalyzeRuntimeSpans(runtimeInput(bom))
	requireRuntimeRule(t, output, err, "utf8_invalid", "json")

	malformed := runtimeExport(runtimeEntry(runtimeRaw("span.one", "1", ""), ""))
	malformed = append([]byte("\xff"), malformed...)
	output, err = AnalyzeRuntimeSpans(runtimeInput(malformed))
	requireRuntimeRule(t, output, err, "utf8_invalid", "json")

	entries := []map[string]any{runtimeEntry([]byte(`"\ud800"`), "")}
	output, err = AnalyzeRuntimeSpans(runtimeInput(runtimeExport(entries...)))
	requireRuntimeRule(t, output, err, "json_syntax", "json")

	duplicate := []byte(`{"schema":"mcd-runtime-span-export/1","spans":{"trace_id":"a","\u0074race_id":"a","content_sha256":"` + strings.Repeat("a", 64) + `","raw":"{}","build_revision":"","collector_name":"","collector_version":""}}`)
	output, err = AnalyzeRuntimeSpans(runtimeInput(duplicate))
	requireRuntimeRule(t, output, err, "json_duplicate_name", "json")

	trailing := append(runtimeExport(runtimeEntry(runtimeRaw("span.one", "1", ""), "")), 'x')
	output, err = AnalyzeRuntimeSpans(runtimeInput(trailing))
	requireRuntimeRule(t, output, err, "json_trailing_data", "json")

	deepRaw := strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17)
	entries = []map[string]any{runtimeEntry([]byte(deepRaw), "")}
	output, err = AnalyzeRuntimeSpans(runtimeInput(runtimeExport(entries...)))
	requireRuntimeRule(t, output, err, "json_depth", "json")
}

func TestRuntimeSpanErrorNormalization(t *testing.T) {
	// Non-rule errors normalize to ledger_invalid with constant input data.
	normalize := func(err error) *RuleError {
		normalized := normalizeRuntimeError(err)
		var rule *RuleError
		if !errors.As(normalized, &rule) {
			t.Fatalf("want RuleError, got %v", normalized)
		}
		return rule
	}
	rule := normalize(errors.New("boom"))
	if rule.Code != "stage4.ledger_invalid" || rule.Locator != "input" {
		t.Fatalf("ordinary error %v", rule)
	}
	rule = normalize(ruleErr("stage3b.utf8_invalid", "byte:7"))
	if rule.Code != "stage4.utf8_invalid" || rule.Locator != "json" {
		t.Fatalf("remapped utf8 %v", rule)
	}
	rule = normalize(ruleErr("stage3b.secret_indicator", "metadata"))
	if rule.Code != "stage4.secret_indicator" || rule.Locator != "input" {
		t.Fatalf("remapped secret %v", rule)
	}
	rule = normalize(ruleErr("stage4.span_limit", "input"))
	if rule.Code != "stage4.span_limit" || rule.Locator != "input" {
		t.Fatalf("native stage4 code %v", rule)
	}
	rule = normalize(ruleErr("stage4.weird_code", "json"))
	if rule.Code != "stage4.ledger_invalid" || rule.Locator != "input" {
		t.Fatalf("unrecognized stage4 code %v", rule)
	}
	rule = normalize(ruleErr("stage5.other", "digest"))
	if rule.Code != "stage4.ledger_invalid" || rule.Locator != "input" {
		t.Fatalf("foreign prefix %v", rule)
	}
}

func ruleErr(code, locator string) *RuleError { return &RuleError{Code: code, Locator: locator} }

func TestRuntimeSpanBudgetSeam(t *testing.T) {
	content := runtimeExport(runtimeEntry(runtimeRaw("span.one", "1", ""), ""))
	input := runtimeInput(content)
	output, err := AnalyzeRuntimeSpans(input)
	if err != nil {
		t.Fatal(err)
	}
	limits := defaultBudget
	limits.records = 0
	rejected, err := analyzeRuntimeSpansWithBudget(input, limits)
	requireRuntimeRule(t, rejected, err, "preflight_limit", "input")

	// Admission was released, so a subsequent valid call succeeds.
	again, err := AnalyzeRuntimeSpans(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output, again) {
		t.Fatal("budget failure changed the output")
	}
}
