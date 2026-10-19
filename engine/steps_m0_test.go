package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/mcp"
)

const m0MatrixPath = "docs/capability-matrix.json"

type m0Matrix struct {
	Schema         string         `json:"schema"`
	Version        string         `json:"version"`
	ProductVersion string         `json:"product_version"`
	Capabilities   []m0Capability `json:"capabilities"`
}

type m0Capability struct {
	ID                 string                  `json:"id"`
	Category           string                  `json:"category"`
	Status             string                  `json:"status"`
	Semantics          string                  `json:"semantics"`
	SemanticsVersion   string                  `json:"semantics_version"`
	Limits             []string                `json:"limits"`
	Surfaces           m0Surfaces              `json:"surfaces"`
	RegressionFixtures []string                `json:"regression_fixtures"`
	NegativeFixture    string                  `json:"negative_fixture"`
	Experimental       *m0ExperimentalEvidence `json:"experimental"`
	Delegated          *m0DelegatedEvidence    `json:"delegated"`
}

type m0Surfaces struct {
	CLI      []string `json:"cli"`
	MCP      []string `json:"mcp"`
	Manifest []string `json:"manifest"`
}

type m0ExperimentalEvidence struct {
	DefaultOff    bool     `json:"default_off"`
	CLIMarkers    []string `json:"cli_markers"`
	MCPMarkers    []string `json:"mcp_markers"`
	ReportMarkers []string `json:"report_markers"`
}

type m0DelegatedEvidence struct {
	Backend            string   `json:"backend"`
	M30ContractVersion string   `json:"m3_0_contract_version"`
	Fixtures           []string `json:"fixtures"`
}

type m0World struct {
	plugin string
	matrix m0Matrix
}

func init() {
	stepRegistrars = append(stepRegistrars, registerM0Steps)
}

func m0PluginDir() (string, error) {
	return filepath.Abs("..")
}

func m0LoadMatrix(plugin string) (m0Matrix, error) {
	var matrix m0Matrix
	data, err := os.ReadFile(filepath.Join(plugin, m0MatrixPath))
	if err != nil {
		return matrix, err
	}
	if err := json.Unmarshal(data, &matrix); err != nil {
		return matrix, fmt.Errorf("parse %s: %w", m0MatrixPath, err)
	}
	return matrix, nil
}

func m0KnownMCPSurfaces() map[string]bool {
	known := make(map[string]bool, len(mcp.ToolNames))
	for _, tool := range mcp.ToolNames {
		known[tool] = true
	}
	return known
}

func m0KnownCLISurfaces() map[string]bool {
	return map[string]bool{
		"parse":            true,
		"catalog":          true,
		"validate-catalog": true,
		"check":            true,
		"check --estimate": true,
	}
}

func m0ReadFeature(plugin, fixture string) (string, error) {
	if !strings.HasPrefix(fixture, "features/") || !strings.HasSuffix(fixture, ".feature") {
		return "", fmt.Errorf("fixture %q is not a feature path", fixture)
	}
	data, err := os.ReadFile(filepath.Join(plugin, fixture))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type m0ScenarioEvidence [][]string

var m0FixtureEvidence = map[string]map[string]m0ScenarioEvidence{
	"frontend.promela-subset": {
		"features/g1-promela.feature": {{"Scenario: mutex_flaw — the assertion is violated and the counterexample reads as Promela"}},
	},
	"frontend.petri-pt": {
		"features/g0-engine.feature": {{"Scenario: petrinet1 hangs after t1, t4 (Holzmann §8.10, App_C/petrinet1)"}},
	},
	"frontend.canonical-ir": {
		"features/g0-engine.feature": {{"Scenario: the IR round-trips through JSON and checks identically"}},
	},
	"property.safety-and-reachability": {
		"features/g0-engine.feature": {{"Scenario Outline: exhausting a budget is inconclusive with the resource named"}},
	},
	"property.ltl": {
		"features/g4-ltl.feature": {{"Scenario: []p on CH4/prop.pml through the engine's own automaton — violated with prefix + loop"}},
	},
	"property.ctl": {
		"features/g5-ctl-v1.feature": {{"Scenario: E6 — AG EF idle on CH14/version1 through the CLI, with the witness rule stated"}},
	},
	"property.progress": {
		"features/g4-ltl.feature": {{"Scenario: CH4/dijkstra_progress.pml — no non-progress cycle, as pan -l"}},
	},
	"backend.explicit-state": {
		"features/g0-engine.feature": {{"Scenario: petrinet1 hangs after t1, t4 (Holzmann §8.10, App_C/petrinet1)"}},
	},
	"backend.parallel-safety": {
		"features/g1-promela.feature": {{"Scenario Outline: an atomic sequence that never stores a state still stops at the time budget"}},
		"features/g2-mcp.feature":     {{"Scenario: mc_check with workers runs the parallel search and says so"}},
	},
	"backend.parallel-ctl-graph": {
		"features/g8-parallel.feature": {{"Scenario: a CTL-only run builds its graph in parallel before labelling"}},
	},
	"reduction.partial-order": {
		"features/g7-por.feature": {{"Scenario: independent processes are explored one after the other"}},
	},
	"evidence.exhaustive-bounded-unknown": {
		"features/g0-engine.feature": {
			{"Scenario Outline: exhausting a budget is inconclusive with the resource named"},
			{"Scenario: exhausting the time budget is inconclusive with the resource named"},
		},
		"features/g8-parallel.feature": {{"Scenario: a memory budget stops the run on the estimate and never verifies"}},
	},
	"evidence.approximate-estimate": {
		"features/g2-mcp.feature":     {{"Scenario: mc_estimate reports the states visited within a time limit and a growth projection", "And the estimate projection carries evidence \"approximate\""}},
		"features/g1-promela.feature": {{"Scenario: the estimate stops at its time limit inside an atomic sequence", "And the CLI estimate projection evidence is \"approximate\""}},
	},
	"model.finite-untimed": {
		"features/g0-engine.feature": {{"Scenario: petrinet1 hangs after t1, t4 (Holzmann §8.10, App_C/petrinet1)"}},
	},
	"petri.inhibitor-arcs": {
		"features/g0-engine.feature": {{"Scenario: an inhibitor arc is rejected with an explanation"}},
	},
	"model.timed": {
		"features/m0-capability-matrix.feature": {{"Scenario: Boundary-only unsupported capabilities have no executable product surface"}},
	},
	"model.probabilistic": {
		"features/m0-capability-matrix.feature": {{"Scenario: Boundary-only unsupported capabilities have no executable product surface"}},
	},
	"model.symbolic": {
		"features/m0-capability-matrix.feature": {{"Scenario: Boundary-only unsupported capabilities have no executable product surface"}},
	},
	"model.unbounded": {
		"features/m0-capability-matrix.feature": {{"Scenario: Boundary-only unsupported capabilities have no executable product surface"}},
	},
	"backend.external-unintegrated": {
		"features/m0-capability-matrix.feature": {{"Scenario: Boundary-only unsupported capabilities have no executable product surface"}},
	},
}

var m0NegativeFixtureEvidence = map[string]m0ScenarioEvidence{
	"promela.embedded-c": {
		{"Scenario: a model with c_code is rejected naming the construct, file and line"},
	},
	"promela.unless-outside-subset": {
		{"Scenario: unless inside an enclosing d_step is rejected before exploration"},
		{"Scenario: chained unless is rejected as an unsupported construct"},
	},
	"petri.inhibitor-arcs": {
		{"Scenario: mc_parse rejects an unsupported construct with a structured rejection, not a tool error", "the rejection names the construct \"inhibitor\""},
	},
	"property.strong-fairness": {
		{"Scenario Outline: an assumption the engine cannot honour is not-executed, with a reason", "strong fairness"},
	},
}

var m0BoundaryOnlyEntries = map[string]struct{}{
	"model.timed":                   {},
	"model.probabilistic":           {},
	"model.symbolic":                {},
	"model.unbounded":               {},
	"backend.external-unintegrated": {},
}

type m0FeatureScenario struct {
	header           string
	body             string
	pending          bool
	isOutline        bool
	runnableExamples bool
}

func m0FeatureScenarios(body string) []m0FeatureScenario {
	var scenarios []m0FeatureScenario
	var current *m0FeatureScenario
	pendingTag := false
	featurePending := false
	rulePending := false
	docstringDelimiter := ""
	finish := func() {
		if current != nil {
			if current.isOutline && !current.runnableExamples {
				current.pending = true
			}
			scenarios = append(scenarios, *current)
			current = nil
		}
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if docstringDelimiter != "" {
			if current != nil {
				current.body += line + "\n"
			}
			if strings.HasPrefix(trimmed, docstringDelimiter) {
				docstringDelimiter = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, `"""`) {
			docstringDelimiter = `"""`
			if current != nil {
				current.body += line + "\n"
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			docstringDelimiter = "```"
			if current != nil {
				current.body += line + "\n"
			}
			continue
		}
		if strings.HasPrefix(trimmed, "@") {
			for _, tag := range strings.Fields(trimmed) {
				if tag == "@pending" {
					pendingTag = true
				}
			}
			continue
		}
		if strings.HasPrefix(trimmed, "Feature:") {
			featurePending = pendingTag
			pendingTag = false
			continue
		}
		if strings.HasPrefix(trimmed, "Rule:") {
			finish()
			rulePending = featurePending || pendingTag
			pendingTag = false
			continue
		}
		if strings.HasPrefix(trimmed, "Background:") {
			finish()
			pendingTag = false
			continue
		}
		if strings.HasPrefix(trimmed, "Examples:") || strings.HasPrefix(trimmed, "Scenarios:") {
			if current != nil && current.isOutline && !pendingTag {
				current.runnableExamples = true
			}
			pendingTag = false
			continue
		}
		if strings.HasPrefix(trimmed, "Scenario:") || strings.HasPrefix(trimmed, "Scenario Outline:") || strings.HasPrefix(trimmed, "Example:") || strings.HasPrefix(trimmed, "Scenario Template:") {
			finish()
			current = &m0FeatureScenario{header: trimmed, pending: featurePending || rulePending || pendingTag, isOutline: strings.HasPrefix(trimmed, "Scenario Outline:") || strings.HasPrefix(trimmed, "Scenario Template:")}
			pendingTag = false
			continue
		}
		if current != nil && !strings.HasPrefix(trimmed, "#") {
			current.body += line + "\n"
		}
	}
	finish()
	return scenarios
}

func TestM0FeatureScenarios(t *testing.T) {
	body := `Feature: fixture
  Rule: scoped behavior
    Background:
      Given a setup

    # not evidence
    @fast
    Scenario: exact scenario
      Given value "expected"
      # not evidence
      """
      Scenario: fake header
      @pending
      """
` + "      ```\n      Scenario: backtick fake header\n      @pending\n      ```\n" + `

    Scenario Template: template scenario
      Given value "<value>"

      Examples:
        | value |
        | next  |
`
	scenarios := m0FeatureScenarios(body)
	if len(scenarios) != 2 {
		t.Fatalf("scenario count = %d, want 2", len(scenarios))
	}
	if scenarios[0].header != "Scenario: exact scenario" || scenarios[0].pending {
		t.Fatalf("first scenario = %#v", scenarios[0])
	}
	if scenarios[1].header != "Scenario Template: template scenario" {
		t.Fatalf("second scenario = %#v", scenarios[1])
	}
	if m0HasScenarioEvidence(body, m0ScenarioEvidence{{"Scenario: fake header"}}) {
		t.Fatal("docstring content must not create a scenario")
	}
	if m0HasScenarioEvidence(body, m0ScenarioEvidence{{"Scenario: backtick fake header"}}) {
		t.Fatal("backtick docstring content must not create a scenario")
	}
	if m0HasScenarioEvidence(body, m0ScenarioEvidence{{"Scenario: exact scenario", "# not evidence"}}) {
		t.Fatal("comments must not satisfy scenario evidence")
	}
}

func TestM0FeatureScenariosPendingTags(t *testing.T) {
	body := `@slow @pending
Feature: pending feature
  Scenario: first
    Given a step
  Scenario: second
    Given a step
`
	for _, scenario := range m0FeatureScenarios(body) {
		if !scenario.pending {
			t.Fatalf("feature-pending scenario %q was accepted", scenario.header)
		}
	}

	body = `Feature: scoped pending
  @fast @pending
  Rule: pending rule
    Scenario: rule scenario
      Given a step
  Scenario Outline: example scenario
    Given value "<value>"
    @slow @pending
    Examples:
      | value |
      | next  |

  Scenario Outline: examples pending independently
    Given value "<value>"
    @slow @pending
    Scenarios:
      | value |
      | later |

  Rule: runnable rule
  Scenario Outline: mixed examples
    Given value "<value>"
    @pending
    Examples:
      | value |
      | skip  |
    Examples:
      | value |
      | run   |
`
	scenarios := m0FeatureScenarios(body)
	if len(scenarios) != 4 || !scenarios[0].pending || !scenarios[1].pending || !scenarios[2].pending || scenarios[3].pending {
		t.Fatalf("pending scopes = %#v", scenarios)
	}
}

func m0HasScenarioBodyEvidence(body string, evidence []string) bool {
	for _, scenario := range m0FeatureScenarios(body) {
		if scenario.pending {
			continue
		}
		matches := true
		for _, text := range evidence {
			if !strings.Contains(scenario.body, text) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func m0HasScenarioEvidence(body string, groups m0ScenarioEvidence) bool {
	scenarios := m0FeatureScenarios(body)
	for _, group := range groups {
		if len(group) == 0 {
			return false
		}
		found := false
		for _, scenario := range scenarios {
			if scenario.pending || scenario.header != group[0] {
				continue
			}
			matches := true
			for _, text := range group[1:] {
				if !strings.Contains(scenario.body, text) {
					matches = false
					break
				}
			}
			if matches {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func m0HasFixtureEvidence(entry m0Capability, fixture string, body string) error {
	entryEvidence, ok := m0FixtureEvidence[entry.ID]
	if ok {
		if need, ok := entryEvidence[fixture]; ok {
			if !m0HasScenarioEvidence(body, need) {
				return fmt.Errorf("entry %q fixture %q lacks scenario evidence", entry.ID, fixture)
			}
			return nil
		}
	}
	need, ok := m0NegativeFixtureEvidence[entry.ID]
	if !ok || entry.NegativeFixture != fixture {
		return fmt.Errorf("entry %q fixture %q has no fixture-evidence rule", entry.ID, fixture)
	}
	if !m0HasScenarioEvidence(body, need) {
		return fmt.Errorf("entry %q fixture %q lacks scenario evidence", entry.ID, fixture)
	}
	return nil
}

func m0FixtureBodies(plugin string, entry m0Capability) (map[string]string, error) {
	if len(entry.RegressionFixtures) == 0 {
		return nil, fmt.Errorf("entry %q has no regression fixture", entry.ID)
	}
	bodies := make(map[string]string, len(entry.RegressionFixtures))
	canonical := make(map[string]bool, len(entry.RegressionFixtures))
	for _, fixture := range entry.RegressionFixtures {
		canonical[fixture] = true
		body, err := m0ReadFeature(plugin, fixture)
		if err != nil {
			return nil, fmt.Errorf("entry %q: %w", entry.ID, err)
		}
		bodies[fixture] = body
	}
	if evidence, ok := m0FixtureEvidence[entry.ID]; ok {
		for fixture := range evidence {
			if !canonical[fixture] {
				return nil, fmt.Errorf("entry %q evidence fixture %q is not canonical", entry.ID, fixture)
			}
		}
	}
	if entry.NegativeFixture != "" && !canonical[entry.NegativeFixture] {
		return nil, fmt.Errorf("entry %q negative fixture %q is not canonical", entry.ID, entry.NegativeFixture)
	}
	return bodies, nil
}

func m0ContainsExperimentalMarker(markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(strings.ToLower(marker), "experimental") {
			return true
		}
	}
	return false
}

func m0BodiesContain(bodies map[string]string, text string) bool {
	for _, body := range bodies {
		if m0HasScenarioBodyEvidence(body, []string{text}) {
			return true
		}
	}
	return false
}

func m0ValidateExperimentalEvidence(entry m0Capability, bodies map[string]string) error {
	if entry.Experimental == nil {
		return fmt.Errorf("experimental entry %q has no experimental evidence", entry.ID)
	}
	if !entry.Experimental.DefaultOff {
		return fmt.Errorf("experimental entry %q is not off by default", entry.ID)
	}
	for surface, markers := range map[string][]string{
		"CLI":    entry.Experimental.CLIMarkers,
		"MCP":    entry.Experimental.MCPMarkers,
		"report": entry.Experimental.ReportMarkers,
	} {
		if !m0ContainsExperimentalMarker(markers) {
			return fmt.Errorf("experimental entry %q has no %s experimental marker", entry.ID, surface)
		}
		for _, marker := range markers {
			if !m0BodiesContain(bodies, marker) {
				return fmt.Errorf("experimental entry %q fixture lacks %s marker %q", entry.ID, surface, marker)
			}
		}
	}
	if !m0BodiesContain(bodies, "off by default") {
		return fmt.Errorf("experimental entry %q fixture does not assert off by default", entry.ID)
	}
	return nil
}

func m0ValidateDelegatedEvidence(plugin string, entry m0Capability) error {
	if entry.Delegated == nil {
		return fmt.Errorf("delegated entry %q has no delegated backend evidence", entry.ID)
	}
	if entry.Delegated.Backend == "" || entry.Delegated.M30ContractVersion == "" {
		return fmt.Errorf("delegated entry %q lacks backend or M3.0 contract version", entry.ID)
	}
	if len(entry.Delegated.Fixtures) == 0 {
		return fmt.Errorf("delegated entry %q has no backend evidence fixture", entry.ID)
	}
	regressionFixtures := make(map[string]bool, len(entry.RegressionFixtures))
	for _, fixture := range entry.RegressionFixtures {
		regressionFixtures[fixture] = true
	}
	for _, fixture := range entry.Delegated.Fixtures {
		if !regressionFixtures[fixture] {
			return fmt.Errorf("delegated entry %q evidence fixture %q is not a regression fixture", entry.ID, fixture)
		}
		body, err := m0ReadFeature(plugin, fixture)
		if err != nil {
			return fmt.Errorf("delegated entry %q: %w", entry.ID, err)
		}
		if !m0HasScenarioBodyEvidence(body, []string{entry.Delegated.Backend, entry.Delegated.M30ContractVersion}) {
			return fmt.Errorf("delegated entry %q fixture %q lacks scenario evidence", entry.ID, fixture)
		}
	}
	return nil
}

func m0ValidateExecutableEvidence(plugin string, entry m0Capability) error {
	bodies, err := m0FixtureBodies(plugin, entry)
	if err != nil {
		return err
	}
	if entry.Status == "supported" {
		for fixture, body := range bodies {
			if err := m0HasFixtureEvidence(entry, fixture, body); err != nil {
				return err
			}
		}
	}
	if entry.Status == "experimental" {
		if err := m0ValidateExperimentalEvidence(entry, bodies); err != nil {
			return err
		}
	}
	if entry.Status == "delegated-integrated" {
		if err := m0ValidateDelegatedEvidence(plugin, entry); err != nil {
			return err
		}
	}
	return nil
}

func m0ValidateEntry(entry m0Capability) error {
	if entry.ID == "" || entry.Category == "" || entry.Status == "" || entry.Semantics == "" || entry.SemanticsVersion == "" {
		return fmt.Errorf("entry has an empty required field: %#v", entry)
	}
	if len(entry.Limits) == 0 {
		return fmt.Errorf("entry %q has no limits", entry.ID)
	}
	if len(entry.RegressionFixtures) == 0 {
		return fmt.Errorf("entry %q has no regression fixture", entry.ID)
	}
	return nil
}

func m0ReadReconciliation(plugin string) (string, error) {
	data, err := os.ReadFile(filepath.Join(plugin, "steps", "issue-11-reconciliation.md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func registerM0Steps(sc *godog.ScenarioContext) {
	w := &m0World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = m0World{}
		return ctx, nil
	})

	sc.Step(`^the issue (\d+) capability matrix is loaded$`, func(issue int) error {
		if issue != 11 {
			return fmt.Errorf("issue = %d, want 11", issue)
		}
		plugin, err := m0PluginDir()
		if err != nil {
			return err
		}
		matrix, err := m0LoadMatrix(plugin)
		if err != nil {
			return err
		}
		w.plugin, w.matrix = plugin, matrix
		return nil
	})
	sc.Step(`^the matrix schema is "([^"]*)"$`, func(want string) error {
		if w.matrix.Schema != want {
			return fmt.Errorf("matrix schema = %q, want %q", w.matrix.Schema, want)
		}
		return nil
	})
	sc.Step(`^every matrix entry has an identifier, category, status, semantics version, and limits$`, func() error {
		if w.matrix.Version == "" || w.matrix.ProductVersion == "" || len(w.matrix.Capabilities) == 0 {
			return fmt.Errorf("matrix version, product version, or capabilities is empty")
		}
		seen := map[string]bool{}
		for _, entry := range w.matrix.Capabilities {
			if seen[entry.ID] {
				return fmt.Errorf("duplicate matrix identifier %q", entry.ID)
			}
			seen[entry.ID] = true
			if err := m0ValidateEntry(entry); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^every matrix entry cites an exercised feature fixture$`, func() error {
		for _, entry := range w.matrix.Capabilities {
			bodies, err := m0FixtureBodies(w.plugin, entry)
			if err != nil {
				return err
			}
			for fixture, body := range bodies {
				if err := m0HasFixtureEvidence(entry, fixture, body); err != nil {
					return err
				}
			}
		}
		return nil
	})
	sc.Step(`^the matrix includes frontend, property, backend, reduction, evidence, and model-class categories$`, func() error {
		need := map[string]bool{"frontend": false, "property": false, "backend": false, "reduction": false, "evidence": false, "model-class": false}
		for _, entry := range w.matrix.Capabilities {
			if _, ok := need[entry.Category]; ok {
				need[entry.Category] = true
			}
		}
		var missing []string
		for category, found := range need {
			if !found {
				missing = append(missing, category)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			return fmt.Errorf("matrix lacks categories %v", missing)
		}
		return nil
	})
	sc.Step(`^every matrix CLI surface names a documented mcd command or check flag$`, func() error {
		known := m0KnownCLISurfaces()
		for _, entry := range w.matrix.Capabilities {
			for _, surface := range entry.Surfaces.CLI {
				if !known[surface] {
					return fmt.Errorf("entry %q names unknown CLI surface %q", entry.ID, surface)
				}
			}
		}
		return nil
	})
	sc.Step(`^every matrix MCP surface is one of the seven registered MCP tools$`, func() error {
		known := m0KnownMCPSurfaces()
		for _, entry := range w.matrix.Capabilities {
			for _, surface := range entry.Surfaces.MCP {
				if !known[surface] {
					return fmt.Errorf("entry %q names unknown MCP surface %q", entry.ID, surface)
				}
			}
		}
		return nil
	})
	sc.Step(`^every matrix manifest surface is declared by the shipped plugin manifests$`, func() error {
		for _, name := range []string{"mcp.json", "mcp/servers.json"} {
			data, err := os.ReadFile(filepath.Join(w.plugin, name))
			if err != nil {
				return err
			}
			if !strings.Contains(string(data), `"model-check"`) {
				return fmt.Errorf("manifest %s does not declare model-check", name)
			}
		}
		for _, entry := range w.matrix.Capabilities {
			for _, surface := range entry.Surfaces.Manifest {
				if surface != "model-check" {
					return fmt.Errorf("entry %q names undeclared manifest surface %q", entry.ID, surface)
				}
			}
		}
		return nil
	})
	sc.Step(`^every supported matrix entry cites an exercised feature fixture$`, func() error {
		for _, entry := range w.matrix.Capabilities {
			if entry.Status != "supported" {
				continue
			}
			if err := m0ValidateExecutableEvidence(w.plugin, entry); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^every supported, experimental, or delegated-integrated entry cites exercised feature evidence$`, func() error {
		for _, entry := range w.matrix.Capabilities {
			if entry.Status != "supported" && entry.Status != "experimental" && entry.Status != "delegated-integrated" {
				continue
			}
			if err := m0ValidateExecutableEvidence(w.plugin, entry); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^every experimental entry declares default-off CLI, MCP, and report markers$`, func() error {
		for _, entry := range w.matrix.Capabilities {
			if entry.Status != "experimental" {
				continue
			}
			bodies, err := m0FixtureBodies(w.plugin, entry)
			if err != nil {
				return err
			}
			if err := m0ValidateExperimentalEvidence(entry, bodies); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^every delegated-integrated entry identifies its compliant backend and fixture evidence$`, func() error {
		for _, entry := range w.matrix.Capabilities {
			if entry.Status != "delegated-integrated" {
				continue
			}
			if err := m0ValidateDelegatedEvidence(w.plugin, entry); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^every unsupported matrix entry with an exposed surface cites an exercised negative feature fixture$`, func() error {
		for _, entry := range w.matrix.Capabilities {
			if entry.Status != "unsupported" {
				continue
			}
			exposed := len(entry.Surfaces.CLI)+len(entry.Surfaces.MCP)+len(entry.Surfaces.Manifest) > 0
			if !exposed {
				continue
			}
			if entry.NegativeFixture == "" {
				return fmt.Errorf("unsupported exposed entry %q has no negative fixture", entry.ID)
			}
			body, err := m0ReadFeature(w.plugin, entry.NegativeFixture)
			if err != nil {
				return fmt.Errorf("unsupported entry %q: %w", entry.ID, err)
			}
			need, ok := m0NegativeFixtureEvidence[entry.ID]
			if !ok {
				return fmt.Errorf("unsupported entry %q has no negative-evidence rule", entry.ID)
			}
			if !m0HasScenarioEvidence(body, need) {
				return fmt.Errorf("unsupported entry %q fixture %q lacks scenario evidence", entry.ID, entry.NegativeFixture)
			}
		}
		return nil
	})
	sc.Step(`^every boundary-only unsupported matrix entry has no product surface$`, func() error {
		for id := range m0BoundaryOnlyEntries {
			found := false
			for _, entry := range w.matrix.Capabilities {
				if entry.ID != id {
					continue
				}
				found = true
				if entry.Status != "unsupported" {
					return fmt.Errorf("boundary-only entry %q has status %q, want unsupported", id, entry.Status)
				}
				if len(entry.Surfaces.CLI)+len(entry.Surfaces.MCP)+len(entry.Surfaces.Manifest) != 0 {
					return fmt.Errorf("boundary-only entry %q exposes a product surface", id)
				}
				break
			}
			if !found {
				return fmt.Errorf("missing boundary-only entry %q", id)
			}
		}
		return nil
	})
	sc.Step(`^the matrix does not classify a runtime result status as a capability status$`, func() error {
		capabilityStatuses := map[string]bool{"supported": true, "unsupported": true, "experimental": true, "delegated-integrated": true}
		runtimeStatuses := map[string]bool{"verified": true, "violated": true, "inconclusive": true, "unknown": true, "not-executed": true, "invalid-model": true}
		for _, entry := range w.matrix.Capabilities {
			if runtimeStatuses[entry.Status] || !capabilityStatuses[entry.Status] {
				return fmt.Errorf("entry %q has invalid capability status %q", entry.ID, entry.Status)
			}
		}
		return nil
	})
	sc.Step(`^the README, skill workflow, and CLI and MCP reference link to the capability matrix$`, func() error {
		links := map[string]string{
			"README.md":                                     "[capability and semantics matrix](docs/capability-matrix.md)",
			"skills/model-check/SKILL.md":                   "[capability matrix](../../docs/capability-matrix.md)",
			"skills/model-check/references/workflow.md":     "[capability and semantics matrix](../../../docs/capability-matrix.md)",
			"skills/model-check/references/engine-tools.md": "[capability and semantics matrix](../../../docs/capability-matrix.md)",
		}
		for path, link := range links {
			data, err := os.ReadFile(filepath.Join(w.plugin, path))
			if err != nil {
				return err
			}
			if !strings.Contains(string(data), link) {
				return fmt.Errorf("%s does not contain capability matrix link %q", path, link)
			}
		}
		return nil
	})
	sc.Step(`^the issue (\d+) reconciliation table maps every roadmap obligation exactly once$`, func(issue int) error {
		if issue != 11 {
			return fmt.Errorf("issue = %d, want 11", issue)
		}
		table, err := m0ReadReconciliation(w.plugin)
		if err != nil {
			return err
		}
		if !strings.Contains(table, "issue-11-reconciliation/1") {
			return fmt.Errorf("reconciliation table has no schema identifier")
		}
		for _, id := range []string{"I11-1", "I11-2", "I11-3", "I11-4", "I11-5", "I11-6", "I11-7"} {
			if n := strings.Count(table, "| "+id+" "); n != 1 {
				return fmt.Errorf("reconciliation maps %s %d times, want once", id, n)
			}
		}
		return nil
	})
	sc.Step(`^the arithmetic alternative selection is recorded as pending owner decision$`, func() error {
		table, err := m0ReadReconciliation(w.plugin)
		if err != nil {
			return err
		}
		if !strings.Contains(table, "`pending-owner-decision`") {
			return fmt.Errorf("arithmetic alternative is not pending owner decision")
		}
		return nil
	})
	sc.Step(`^no reconciliation row is deferred or out of scope without a tracker-owner decision link$`, func() error {
		table, err := m0ReadReconciliation(w.plugin)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(table, "\n") {
			if !strings.Contains(line, "| deferred |") && !strings.Contains(line, "| out-of-scope |") {
				continue
			}
			if !strings.Contains(line, "https://github.com/viktor-drobek/model-check-plugin/issues/11#issuecomment-") {
				return fmt.Errorf("scope-reduced row lacks tracker-owner decision link: %s", line)
			}
		}
		return nil
	})
}
