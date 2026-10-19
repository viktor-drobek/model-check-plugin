package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
)

// Markdown renders a deterministic review report. It is intentionally a
// traceability summary, not a verification or implementation-correctness
// report.
func Markdown(d *Document) ([]byte, error) {
	canonical, err := Canonical(d)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# Trace ledger\n\nSchema: `%s`\n\n", canonical.Schema)
	fmt.Fprintln(&buf, "> This report records traceability evidence and explicit gaps. It does not transfer a model verdict to an implementation claim.")
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, "## Nodes")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "| ID | Kind | Label | Evidence |")
	fmt.Fprintln(&buf, "| --- | --- | --- | --- |")
	for _, node := range canonical.Nodes {
		fmt.Fprintf(&buf, "| `%s` | `%s` | %s | %s |\n", node.ID, node.Kind, markdownCell(node.Label), joinedIDs(node.Evidence))
	}
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, "## Relations")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "| ID | Kind | Source | Target | Evidence |")
	fmt.Fprintln(&buf, "| --- | --- | --- | --- | --- |")
	for _, edge := range canonical.Edges {
		fmt.Fprintf(&buf, "| `%s` | `%s` | `%s` | `%s` | %s |\n", edge.ID, edge.Kind, edge.Source, edge.Target, joinedIDs(edge.Evidence))
	}
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, "## Evidence ledger")
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "| ID | Subject | Level | Anchor | Producer |")
	fmt.Fprintln(&buf, "| --- | --- | --- | --- | --- |")
	for _, evidence := range canonical.Evidence {
		fmt.Fprintf(&buf, "| `%s` | `%s` | `%s` | %s | %s %s |\n", evidence.ID, evidence.Subject, evidence.Level, markdownCell(evidence.Provenance.Anchor), markdownCell(evidence.Provenance.Producer.Name), markdownCell(evidence.Provenance.Producer.Version))
	}
	fmt.Fprintln(&buf)

	fmt.Fprintln(&buf, "## Open gaps")
	fmt.Fprintln(&buf)
	if len(canonical.Gaps) == 0 {
		fmt.Fprintln(&buf, "None.")
	} else {
		fmt.Fprintln(&buf, "| ID | Kind | Reference | Reason |")
		fmt.Fprintln(&buf, "| --- | --- | --- | --- |")
		for _, gap := range canonical.Gaps {
			fmt.Fprintf(&buf, "| `%s` | %s | %s | %s |\n", gap.ID, markdownCell(gap.Kind), markdownCell(gap.Reference), markdownCell(gap.Reason))
		}
	}
	return buf.Bytes(), nil
}

func joinedIDs(ids []string) string {
	if len(ids) == 0 {
		return "—"
	}
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = "`" + id + "`"
	}
	return strings.Join(quoted, ", ")
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = html.EscapeString(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "\n", "<br>")
}

// SARIF returns SARIF 2.1.0 findings for explicit gaps. Existing graph facts
// are deliberately not emitted as warnings: only an unresolved obligation is a
// review finding.
func SARIF(d *Document) ([]byte, error) {
	canonical, err := Canonical(d)
	if err != nil {
		return nil, err
	}
	rules := make([]sarifRule, 0, len(canonical.Gaps))
	results := make([]sarifResult, 0, len(canonical.Gaps))
	knownRules := make(map[string]bool, len(canonical.Gaps))
	for _, gap := range canonical.Gaps {
		ruleID := "trace/" + gap.Kind
		if !knownRules[ruleID] {
			rules = append(rules, sarifRule{ID: ruleID, Name: gap.Kind, ShortDescription: sarifMessage{Text: "Trace-ledger obligation: " + gap.Kind}})
			knownRules[ruleID] = true
		}
		results = append(results, sarifResult{
			RuleID:  ruleID,
			Level:   "warning",
			Message: sarifMessage{Text: gap.Reason},
			Properties: map[string]string{
				"gapId":     gap.ID,
				"reference": gap.Reference,
			},
			Locations: []sarifLocation{{
				PhysicalLocation: sarifLocationForReference(gap.Reference),
			}},
		})
	}
	report := sarifLog{
		Version: "2.1.0",
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Runs: []sarifRun{{
			Tool:    sarifTool{Driver: sarifDriver{Name: "mcd-trace-ledger", Version: "1", Rules: rules, Properties: map[string]string{"schema": canonical.Schema}}},
			Results: results,
		}},
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(report); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sarifLocationForReference(reference string) sarifPhysicalLocation {
	location := sarifPhysicalLocation{ArtifactLocation: sarifArtifactLocation{URI: sarifURI(reference)}}
	path, line, ok := sarifSourceLine(reference)
	if !ok {
		return location
	}
	location.ArtifactLocation.URI = sarifPathURI(path)
	location.Region = &sarifRegion{StartLine: line}
	return location
}

func sarifSourceLine(reference string) (string, int, bool) {
	if strings.ContainsAny(reference, "?#") || strings.HasPrefix(reference, "//") {
		return "", 0, false
	}
	separator := strings.LastIndexByte(reference, ':')
	if separator <= 0 || separator == len(reference)-1 || strings.Contains(reference[:separator], ":") {
		return "", 0, false
	}
	line, err := strconv.Atoi(reference[separator+1:])
	if err != nil || line < 1 {
		return "", 0, false
	}
	path := reference[:separator]
	if strings.HasPrefix(path, "/") || !strings.Contains(path, "/") {
		return "", 0, false
	}
	parsed, err := url.Parse(reference)
	if err == nil && (parsed.IsAbs() || parsed.Host != "") {
		return "", 0, false
	}
	return path, line, true
}

func sarifURI(reference string) string {
	if isWindowsAbsolutePath(reference) {
		if relative, err := url.Parse("./" + reference); err == nil {
			return relative.String()
		}
	}
	parsed, err := url.Parse(reference)
	if err == nil {
		return parsed.String()
	}
	return (&url.URL{Path: reference}).String()
}

func sarifPathURI(path string) string {
	return (&url.URL{Path: path}).String()
}

func isWindowsAbsolutePath(reference string) bool {
	return len(reference) >= 3 && isASCIIAlpha(reference[0]) && reference[1] == ':' && (reference[2] == '/' || reference[2] == '\\')
}

func isASCIIAlpha(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Rules      []sarifRule       `json:"rules"`
	Properties map[string]string `json:"properties"`
}

type sarifRule struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	ShortDescription sarifMessage `json:"shortDescription"`
}

type sarifResult struct {
	RuleID     string            `json:"ruleId"`
	Level      string            `json:"level"`
	Message    sarifMessage      `json:"message"`
	Locations  []sarifLocation   `json:"locations"`
	Properties map[string]string `json:"properties"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}
