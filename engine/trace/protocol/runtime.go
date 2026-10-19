package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"modelcheck/trace"
)

// RuntimeSpansInput supplies an offline projection and pins its exact export bytes.
type RuntimeSpansInput struct {
	Artifact     Input
	ExportSHA256 string
}

var runtimeIDRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]{0,63}$`)
var runtimeCollectorRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var runtimeRevisionRE = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var runtimeNumberRE = regexp.MustCompile(`^(?:0|[1-9][0-9]*)$`)

func runtimeRule(suffix string) error { return &RuleError{Code: "stage4." + suffix, Locator: "input"} }

func normalizeRuntimeError(err error) error {
	if err == nil {
		return nil
	}
	suffix := "ledger_invalid"
	var r *RuleError
	if errors.As(err, &r) && r != nil {
		candidate := strings.TrimPrefix(strings.TrimPrefix(r.Code, "stage3b."), "stage4.")
		if r.Code == "stage3b."+candidate || r.Code == "stage4."+candidate {
			switch candidate {
			case "input_length", "resource_admission_busy", "metadata_invalid", "secret_indicator", "digest_mismatch", "utf8_invalid", "json_syntax", "json_trailing_data", "json_duplicate_name", "json_depth", "schema_invalid", "preflight_limit":
				suffix = candidate
			case "span_limit", "identity_ambiguous", "ledger_invalid":
				if strings.HasPrefix(r.Code, "stage4.") {
					suffix = candidate
				}
			}
		}
	}
	locator := "input"
	if suffix == "utf8_invalid" || strings.HasPrefix(suffix, "json_") {
		locator = "json"
	}
	return &RuleError{Code: "stage4." + suffix, Locator: locator}
}

// AnalyzeRuntimeSpans records explicit observations without inferring communication.
func AnalyzeRuntimeSpans(input RuntimeSpansInput) ([]byte, error) {
	return analyzeRuntimeSpansWithBudget(input, defaultBudget)
}

func analyzeRuntimeSpansWithBudget(input RuntimeSpansInput, limits budget) (out []byte, err error) {
	defer func() {
		if err != nil {
			out = nil
			err = normalizeRuntimeError(err)
		}
	}()
	snapshot, release, err := begin(input.Artifact, input.ExportSHA256, func() bool { return len(input.Artifact.Path) <= 64 }, nil)
	if err != nil {
		return nil, err
	}
	defer release()
	value, err := decodeJSON(snapshot)
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok || len(root) != 2 {
		return nil, runtimeRule("schema_invalid")
	}
	schema, schemaOK := root["schema"].(string)
	entries, entriesOK := root["spans"].([]any)
	if !schemaOK || !entriesOK || schema != "mcd-runtime-span-export/1" {
		return nil, runtimeRule("schema_invalid")
	}
	if len(entries) < 1 || len(entries) > 1024 {
		return nil, runtimeRule("span_limit")
	}
	fields := []string{"trace_id", "content_sha256", "raw", "build_revision", "collector_name", "collector_version"}
	envelopes := make([]map[string]string, len(entries))
	for i, entry := range entries {
		m, valid := runtimeStrings(entry, fields)
		if !valid {
			return nil, runtimeRule("schema_invalid")
		}
		if !runtimeIDRE.MatchString(m["trace_id"]) || !validDigest(m["content_sha256"]) || m["build_revision"] != "" && !runtimeRevisionRE.MatchString(m["build_revision"]) {
			return nil, runtimeRule("metadata_invalid")
		}
		name, version := m["collector_name"], m["collector_version"]
		if (name == "") != (version == "") || name != "" && !runtimeCollectorRE.MatchString(name) || version != "" && !runtimeCollectorRE.MatchString(version) {
			return nil, runtimeRule("metadata_invalid")
		}
		if len(m["raw"]) < 1 || len(m["raw"]) > 4096 {
			return nil, runtimeRule("input_length")
		}
		envelopes[i] = m
	}
	type observation struct {
		envelope    map[string]string
		span        map[string]string
		digest      string
		occurrences int
	}
	observations := make([]observation, 0, len(entries))
	for _, envelope := range envelopes {
		raw := []byte(envelope["raw"])
		v, err := decodeJSON(raw)
		if err != nil {
			return nil, err
		}
		span, err := runtimeSpan(v)
		if err != nil {
			return nil, err
		}
		h := sha256.Sum256(raw)
		digest := hex.EncodeToString(h[:])
		if digest != envelope["content_sha256"] {
			return nil, runtimeRule("digest_mismatch")
		}
		if envelope["trace_id"] != span["trace_id"] {
			return nil, runtimeRule("identity_ambiguous")
		}
		for _, m := range []map[string]string{envelope, span} {
			for key, value := range m {
				if key != "raw" && secretValue(value) {
					return nil, runtimeRule("secret_indicator")
				}
			}
		}
		observations = append(observations, observation{envelope: envelope, span: span, digest: digest, occurrences: 1})
	}
	groups := make(map[string]int, len(observations))
	unique := make([]observation, 0, len(observations))
	for _, o := range observations {
		key := o.envelope["trace_id"] + "\x00" + o.digest
		if index, exists := groups[key]; exists {
			previous := &unique[index]
			for _, field := range []string{"build_revision", "collector_name", "collector_version"} {
				if previous.envelope[field] != o.envelope[field] {
					return nil, runtimeRule("identity_ambiguous")
				}
			}
			previous.occurrences++
		} else {
			groups[key] = len(unique)
			unique = append(unique, o)
		}
	}
	if err := checkBudget(5*uint64(len(entries)), 4096+16*uint64(len(snapshot))+16384*uint64(len(entries)), limits); err != nil {
		return nil, err
	}
	d := newDocument(trace.Schema)
	for _, o := range unique {
		m, s := o.envelope, o.span
		hash := tupleHash("runtime-span/1", input.Artifact.Path, m["trace_id"], o.digest)
		nodeID, evidenceID := "span."+hash, "evidence.span."+hash
		collector, version := m["collector_name"], m["collector_version"]
		if collector == "" {
			collector, version = "unavailable", "unavailable"
		}
		p := trace.Provenance{
			SourceRevision: m["build_revision"], ContentSHA256: o.digest,
			Anchor:   fmt.Sprintf("span:%s#trace=%s&span=%s&parent=%s&sha256=%s&occurrences=%d", input.Artifact.Path, m["trace_id"], s["span_id"], s["parent_id"], o.digest, o.occurrences),
			Producer: trace.Producer{Name: "mcd-runtime-span-adapter", Version: "1;collector=" + collector + "@" + version},
		}
		label := fmt.Sprintf("Observed span %s (%s, %s), %s..%s ns", s["name"], s["kind"], s["status"], s["start_ns"], s["end_ns"])
		if len(p.Anchor) > 512 || len(label) > 200 {
			return nil, runtimeRule("ledger_invalid")
		}
		d.Nodes = append(d.Nodes, trace.Node{ID: nodeID, Kind: trace.RuntimeSpan, Label: label, Provenance: p, Evidence: []string{evidenceID}})
		d.Evidence = append(d.Evidence, trace.Evidence{ID: evidenceID, Subject: "node:" + nodeID, Level: trace.Runtime, Provenance: p})
		addGap := func(suffix, kind, reason string) {
			d.Gaps = append(d.Gaps, trace.Gap{ID: "gap.span." + suffix + "." + hash, Kind: kind, Reference: "node:" + nodeID, Reason: reason, Provenance: p})
		}
		if m["build_revision"] == "" {
			addGap("build_missing", "span.provenance_missing", "producer build revision was not supplied")
		}
		if m["collector_name"] == "" {
			addGap("collector_missing", "span.provenance_missing", "collector name and version were not supplied")
		}
		if s["parent_id"] != "" {
			addGap("parent_unmapped", "span.parent_unmapped", "parent identity was recorded without inferring a communication relation")
		}
	}
	return finish(d)
}

func runtimeStrings(value any, fields []string) (map[string]string, bool) {
	object, ok := value.(map[string]any)
	if !ok || len(object) != len(fields) {
		return nil, false
	}
	result := make(map[string]string, len(fields))
	for _, field := range fields {
		scalar, ok := object[field].(string)
		if !ok {
			return nil, false
		}
		result[field] = scalar
	}
	return result, true
}

func runtimeSpan(value any) (map[string]string, error) {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 9 {
		return nil, runtimeRule("schema_invalid")
	}
	s := make(map[string]string, 9)
	for _, field := range []string{"schema", "trace_id", "span_id", "parent_id", "name", "kind", "status"} {
		scalar, ok := object[field].(string)
		if !ok {
			return nil, runtimeRule("schema_invalid")
		}
		s[field] = scalar
	}
	if s["schema"] != "mcd-runtime-span/1" || !runtimeIDRE.MatchString(s["trace_id"]) || !runtimeIDRE.MatchString(s["span_id"]) || !runtimeIDRE.MatchString(s["name"]) || s["parent_id"] != "" && !runtimeIDRE.MatchString(s["parent_id"]) {
		return nil, runtimeRule("schema_invalid")
	}
	switch s["kind"] {
	case "internal", "server", "client", "producer", "consumer":
	default:
		return nil, runtimeRule("schema_invalid")
	}
	switch s["status"] {
	case "unset", "ok", "error":
	default:
		return nil, runtimeRule("schema_invalid")
	}
	var times [2]int64
	for i, field := range []string{"start_ns", "end_ns"} {
		number, ok := object[field].(json.Number)
		if !ok || !runtimeNumberRE.MatchString(string(number)) {
			return nil, runtimeRule("schema_invalid")
		}
		parsed, err := strconv.ParseInt(string(number), 10, 64)
		if err != nil {
			return nil, runtimeRule("schema_invalid")
		}
		times[i], s[field] = parsed, string(number)
	}
	if times[1] < times[0] {
		return nil, runtimeRule("schema_invalid")
	}
	return s, nil
}
