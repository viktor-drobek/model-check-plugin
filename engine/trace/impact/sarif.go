package impact

import (
	"bytes"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// SARIF renders the deterministic impact obligations as SARIF 2.1.0. Findings
// describe required review work, not an implementation-correctness conclusion.
func SARIF(report Report) ([]byte, error) {
	findings := append([]Finding(nil), report.Findings...)
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		return findings[i].Subject < findings[j].Subject
	})
	rules := make([]sarifRule, 0, len(findings))
	results := make([]sarifResult, 0, len(findings))
	known := make(map[string]bool, len(findings))
	for _, item := range findings {
		ruleID := "trace-impact/" + item.Kind
		if !known[ruleID] {
			rules = append(rules, sarifRule{ID: ruleID, Name: item.Kind, ShortDescription: sarifMessage{Text: "Trace impact obligation: " + item.Kind}})
			known[ruleID] = true
		}
		results = append(results, sarifResult{
			RuleID: ruleID, Level: "warning", Message: sarifMessage{Text: item.Message},
			Properties: map[string]string{"findingId": item.ID, "subject": item.Subject},
			Locations:  []sarifLocation{{PhysicalLocation: location(item.Location)}},
		})
	}
	output := sarifLog{Version: "2.1.0", Schema: "https://json.schemastore.org/sarif-2.1.0.json", Runs: []sarifRun{{
		Tool: sarifTool{Driver: sarifDriver{Name: "mcd-trace-impact", Version: "1", Rules: rules}}, Results: results,
	}}}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func location(reference string) sarifPhysicalLocation {
	result := sarifPhysicalLocation{ArtifactLocation: sarifArtifactLocation{URI: uri(reference)}}
	path, line, ok := sourceLine(reference)
	if !ok {
		return result
	}
	result.ArtifactLocation.URI = (&url.URL{Path: path}).String()
	result.Region = &sarifRegion{StartLine: line}
	return result
}

func sourceLine(reference string) (string, int, bool) {
	if strings.ContainsAny(reference, "?#") || strings.HasPrefix(reference, "/") || strings.HasPrefix(reference, "//") {
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
	// A repository-root file has no slash but is still an unambiguous relative
	// source location. Reject only anchor-like names, not all top-level files.
	if path == "." || path == ".." || strings.ContainsAny(path, "\\\x00") {
		return "", 0, false
	}
	return path, line, true
}

func uri(reference string) string {
	parsed, err := url.Parse(reference)
	if err == nil {
		return parsed.String()
	}
	return (&url.URL{Path: reference}).String()
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
	Name    string      `json:"name"`
	Version string      `json:"version"`
	Rules   []sarifRule `json:"rules"`
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
