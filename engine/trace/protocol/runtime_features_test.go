package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"modelcheck/trace"
)

type runtimeFeatureState struct {
	input  RuntimeSpansInput
	output []byte
	err    error
}

func TestRuntimeSpanFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			state := &runtimeFeatureState{}
			sc.Step(`^the synthetic runtime span case "([^"]*)"$`, state.caseInput)
			sc.Step(`^the runtime span adapter analyses the export$`, state.analyze)
			sc.Step(`^the runtime span adapter analyses the export under the reduced budget$`, state.analyzeReducedBudget)
			sc.Step(`^the runtime span adapter holds the admission slot$`, state.holdAdmission)
			sc.Step(`^a second runtime admission holder reports contention promptly$`, state.admissionContention)
			sc.Step(`^the runtime span output is a deterministic ledger$`, state.deterministic)
			sc.Step(`^the runtime span output is unchanged after reversing the export entries$`, state.reordered)
			sc.Step(`^the runtime span output has (\d+) runtime span node$`, state.nodes)
			sc.Step(`^the runtime span output has (\d+) gaps of kind "([^"]*)"$`, state.gapsOfKind)
			sc.Step(`^the runtime span output has (\d+) gap of kind "([^"]*)"$`, state.gapsOfKind)
			sc.Step(`^the runtime span output has a gap with reason "([^"]*)"$`, state.gapReason)
			sc.Step(`^the runtime span output contains an anchor with "([^"]*)"$`, state.anchor)
			sc.Step(`^the runtime span output has no span\.parent_unmapped gap$`, state.noParentGap)
			sc.Step(`^the runtime span output records the source revision$`, state.sourceRevision)
			sc.Step(`^the runtime span output has no gaps$`, state.noGaps)
			sc.Step(`^the runtime span output is absent with rule "([^"]*)"$`, state.rule)
			sc.Step(`^a later runtime span call admits again$`, state.admitsAgain)
		},
		Options: &godog.Options{Strict: true, Format: "pretty", Paths: []string{filepath.Join("..", "..", "..", "features", "g9-runtime-spans.feature")}, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("runtime span feature suite failed")
	}
}

func (s *runtimeFeatureState) caseInput(name string) error {
	input, err := runtimeSpanCase(name)
	if err != nil {
		return err
	}
	s.input, s.output, s.err = input, nil, nil
	return nil
}

func (s *runtimeFeatureState) analyze() { s.output, s.err = AnalyzeRuntimeSpans(s.input) }

func (s *runtimeFeatureState) analyzeReducedBudget() {
	limits := defaultBudget
	limits.records = 0
	s.output, s.err = analyzeRuntimeSpansWithBudget(s.input, limits)
}

var errRuntimeAdmissionHeld = errors.New("runtime admission test slot is already held")

func (s *runtimeFeatureState) holdAdmission() error {
	select {
	case admission <- struct{}{}:
	default:
		return errRuntimeAdmissionHeld
	}
	defer func() { <-admission }()
	s.output, s.err = AnalyzeRuntimeSpans(s.input)
	return nil
}

func (s *runtimeFeatureState) admissionContention() error {
	select {
	case admission <- struct{}{}:
	default:
		return errors.New("cannot arrange runtime admission contention: slot is already held")
	}
	defer func() { <-admission }()
	if err := s.holdAdmission(); !errors.Is(err, errRuntimeAdmissionHeld) {
		return fmt.Errorf("second runtime admission holder returned %v, want contention", err)
	}
	return nil
}

func (s *runtimeFeatureState) document() (*trace.Document, error) {
	if s.err != nil {
		return nil, s.err
	}
	return trace.UnmarshalJSON(s.output)
}

func (s *runtimeFeatureState) deterministic() error {
	document, err := s.document()
	if err != nil {
		return err
	}
	canonical, err := trace.JSON(document)
	if err != nil {
		return err
	}
	if !bytes.Equal(s.output, canonical) {
		return errors.New("runtime output is not canonical")
	}
	again, err := AnalyzeRuntimeSpans(s.input)
	if err != nil {
		return err
	}
	if !bytes.Equal(s.output, again) {
		return errors.New("runtime output is not deterministic")
	}
	return nil
}

func (s *runtimeFeatureState) nodes(want int) error {
	document, err := s.document()
	if err != nil {
		return err
	}
	got := 0
	for _, node := range document.Nodes {
		if node.Kind == trace.RuntimeSpan {
			got++
		}
	}
	if got != want {
		return fmt.Errorf("runtime nodes=%d, want %d", got, want)
	}
	return nil
}

func (s *runtimeFeatureState) reordered() error {
	if err := s.deterministic(); err != nil {
		return err
	}
	var export map[string]json.RawMessage
	if err := json.Unmarshal(s.input.Artifact.Content, &export); err != nil {
		return err
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(export["spans"], &entries); err != nil {
		return err
	}
	before, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	after, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		return errors.New("runtime permutation fixture did not change entry order")
	}
	export["spans"] = after
	content, err := json.Marshal(export)
	if err != nil {
		return err
	}
	input := s.input
	input.Artifact.Content = content
	input.ExportSHA256 = runtimeDigest(content)
	output, err := AnalyzeRuntimeSpans(input)
	if err != nil {
		return err
	}
	if !bytes.Equal(s.output, output) {
		return errors.New("reordered runtime export changed the ledger")
	}
	return nil
}

func (s *runtimeFeatureState) gapsOfKind(want int, kind string) error {
	document, err := s.document()
	if err != nil {
		return err
	}
	got := 0
	for _, gap := range document.Gaps {
		if gap.Kind == kind {
			got++
		}
	}
	if got != want {
		return fmt.Errorf("gaps of kind %q=%d, want %d", kind, got, want)
	}
	return nil
}

func (s *runtimeFeatureState) gapReason(want string) error {
	document, err := s.document()
	if err != nil {
		return err
	}
	for _, gap := range document.Gaps {
		if gap.Reason == want {
			return nil
		}
	}
	return fmt.Errorf("missing gap reason %q", want)
}

func (s *runtimeFeatureState) anchor(want string) error {
	document, err := s.document()
	if err != nil {
		return err
	}
	for _, node := range document.Nodes {
		if strings.Contains(node.Provenance.Anchor, want) {
			return nil
		}
	}
	return fmt.Errorf("missing anchor fragment %q", want)
}

func (s *runtimeFeatureState) noParentGap() error { return s.gapsOfKind(0, "span.parent_unmapped") }

func (s *runtimeFeatureState) sourceRevision() error {
	document, err := s.document()
	if err != nil {
		return err
	}
	if len(document.Nodes) != 1 || document.Nodes[0].Provenance.SourceRevision != strings.Repeat("a", 40) {
		return errors.New("runtime output did not preserve the source revision")
	}
	return nil
}

func (s *runtimeFeatureState) noGaps() error {
	document, err := s.document()
	if err != nil {
		return err
	}
	if len(document.Gaps) != 0 {
		return fmt.Errorf("unexpected gaps: %v", document.Gaps)
	}
	return nil
}

func (s *runtimeFeatureState) rule(want string) error {
	var rule *RuleError
	if s.output != nil || !errors.As(s.err, &rule) || rule.Code != want {
		return fmt.Errorf("output=%v error=%v, want nil and %s", s.output, s.err, want)
	}
	return nil
}

func (s *runtimeFeatureState) admitsAgain() error {
	output, err := AnalyzeRuntimeSpans(s.input)
	if err != nil {
		return err
	}
	if len(output) == 0 {
		return errors.New("valid runtime call returned no output")
	}
	return nil
}

func runtimeSpanCase(name string) (RuntimeSpansInput, error) {
	raw := runtimeRaw("span.one", "1", "")
	entry := func(r []byte) map[string]any { return runtimeEntry(r, "") }
	valid := func(entries ...map[string]any) RuntimeSpansInput { return runtimeInput(runtimeExport(entries...)) }
	switch name {
	case "runtime":
		return valid(entry(raw)), nil
	case "runtime-with-build":
		return runtimeInput(buildRuntimeEntry(raw, strings.Repeat("a", 40), "", "")), nil
	case "runtime-with-collector":
		return runtimeInput(buildRuntimeEntry(raw, "", "collector", "1.0.0")), nil
	case "runtime-full":
		return runtimeInput(buildRuntimeEntry(raw, strings.Repeat("a", 40), "collector", "1.0.0")), nil
	case "runtime-parent":
		return valid(entry(runtimeRaw("span.one", "1", "span.parent"))), nil
	case "runtime-dedup":
		return valid(entry(raw), entry(raw)), nil
	case "runtime-distinct-occurrence-group":
		other := runtimeRaw("span.two", "2", "")
		return valid(entry(raw), entry(raw), entry(other)), nil
	case "runtime-same-trace-different-bytes":
		return valid(entry(raw), entry(runtimeRaw("span.one", "2", ""))), nil
	case "runtime-envelope-precedence":
		malformed := entry([]byte("{"))
		badMetadata := entry(runtimeRaw("span.two", "1", ""))
		badMetadata["build_revision"] = strings.Repeat("A", 40)
		return valid(malformed, badMetadata), nil
	case "runtime-bad-export-digest":
		input := valid(entry(raw))
		input.ExportSHA256 = strings.Repeat("0", 64)
		return input, nil
	case "runtime-raw-digest-mismatch", "runtime-raw-digest-late-duplicate":
		bad := runtimeEntry(raw, strings.Repeat("0", 64))
		if name == "runtime-raw-digest-late-duplicate" {
			return valid(entry(raw), bad), nil
		}
		return valid(bad), nil
	case "runtime-identity-ambiguous":
		other := []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.other","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
		return valid(entry(other)), nil
	case "runtime-dedup-conflict":
		first := runtimeEntry(raw, "")
		first["build_revision"] = strings.Repeat("a", 40)
		second := runtimeEntry(raw, "")
		second["build_revision"] = strings.Repeat("b", 64)
		return valid(first, second), nil
	case "runtime-span-limit":
		entries := make([]map[string]any, 1025)
		for i := range entries {
			entries[i] = entry(runtimeRaw(fmt.Sprintf("span.%d", i), "1", ""))
		}
		return valid(entries...), nil
	case "runtime-oversized-export":
		content := runtimeExport(entry(raw))
		content = append(content, bytes.Repeat([]byte(" "), (1<<20+1)-len(content))...)
		return runtimeInput(content), nil
	case "runtime-oversized-raw":
		long := []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"` + strings.Repeat("n", 4080) + `","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
		return valid(entry(long)), nil
	case "runtime-bad-path":
		input := valid(entry(raw))
		input.Artifact.Path = strings.Repeat("a", 65)
		return input, nil
	case "runtime-scalar-bound":
		bad := []byte(`{"schema":"mcd-runtime-span/1","trace_id":"` + strings.Repeat("a", 65) + `","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
		return valid(entry(bad)), nil
	case "runtime-bad-build":
		return runtimeInput(buildRuntimeEntry(raw, strings.Repeat("A", 40), "", "")), nil
	case "runtime-partial-collector":
		return runtimeInput(buildRuntimeEntry(raw, "", "collector", "")), nil
	case "runtime-secret-name":
		secret := []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"auth_token","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
		return valid(entry(secret)), nil
	case "runtime-unknown-field":
		unknown := []byte(`{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1,"note":"x"}`)
		return valid(entry(unknown)), nil
	case "runtime-entry-extra-field":
		e := entry(raw)
		e["vendor"] = "x"
		return valid(e), nil
	case "runtime-extra-export-field":
		content := runtimeExport(entry(raw))
		var export map[string]any
		if err := json.Unmarshal(content, &export); err != nil {
			return RuntimeSpansInput{}, err
		}
		export["vendor"] = "x"
		content, err := json.Marshal(export)
		if err != nil {
			return RuntimeSpansInput{}, err
		}
		return runtimeInput(content), nil
	case "runtime-bad-schema":
		bad := []byte(`{"schema":"mcd-runtime-span/2","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1}`)
		return valid(entry(bad)), nil
	case "runtime-bad-export-schema":
		content := runtimeExport(entry(raw))
		var export map[string]any
		if err := json.Unmarshal(content, &export); err != nil {
			return RuntimeSpansInput{}, err
		}
		export["schema"] = "mcd-runtime-span-export/2"
		content, err := json.Marshal(export)
		if err != nil {
			return RuntimeSpansInput{}, err
		}
		return runtimeInput(content), nil
	case "runtime-bad-kind", "runtime-bad-status", "runtime-number-bound", "runtime-negative-number", "runtime-end-before-start":
		replacements := map[string]string{"runtime-bad-kind": `"kind":"worker"`, "runtime-bad-status": `"status":"done"`, "runtime-number-bound": `"start_ns":9223372036854775808`, "runtime-negative-number": `"start_ns":-1`, "runtime-end-before-start": `"start_ns":2`}
		needle := strings.Split(replacements[name], ":")[0]
		value := strings.SplitN(replacements[name], ":", 2)[1]
		mutated := strings.Replace(string(raw), needle+":"+map[string]string{"runtime-bad-kind": `"server"`, "runtime-bad-status": `"ok"`, "runtime-number-bound": "0", "runtime-negative-number": "0", "runtime-end-before-start": "0"}[name], needle+":"+value, 1)
		return valid(entry([]byte(mutated))), nil
	case "runtime-malformed-utf8":
		input := valid(entry(raw))
		input.Artifact.Content = append([]byte{0xff}, input.Artifact.Content...)
		input.ExportSHA256 = runtimeDigest(input.Artifact.Content)
		return input, nil
	case "runtime-bom":
		input := valid(entry(raw))
		input.Artifact.Content = append([]byte{0xef, 0xbb, 0xbf}, input.Artifact.Content...)
		input.ExportSHA256 = runtimeDigest(input.Artifact.Content)
		return input, nil
	case "runtime-surrogate":
		return valid(runtimeEntry([]byte(`"\ud800"`), "")), nil
	case "runtime-duplicate-key":
		content := []byte(`{"schema":"mcd-runtime-span-export/1","spans":[{"trace_id":"trace.demo","\u0074race_id":"trace.demo","content_sha256":"` + strings.Repeat("a", 64) + `","raw":"{}","build_revision":"","collector_name":"","collector_version":""}]}`)
		return runtimeInput(content), nil
	case "runtime-trailing":
		input := valid(entry(raw))
		input.Artifact.Content = append(input.Artifact.Content, 'x')
		input.ExportSHA256 = runtimeDigest(input.Artifact.Content)
		return input, nil
	case "runtime-depth":
		return valid(runtimeEntry([]byte(strings.Repeat("[", 17)+"0"+strings.Repeat("]", 17)), "")), nil
	default:
		return RuntimeSpansInput{}, fmt.Errorf("unknown runtime span case %q", name)
	}
}
