package impact_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/trace/impact"
)

func TestG9ImpactFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: registerG9ImpactSteps,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../../features/g9-impact.feature"},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run G9 impact feature tests")
	}
}

type impactWorld struct {
	input  impact.Input
	report impact.Report
	err    error
}

func registerG9ImpactSteps(sc *godog.ScenarioContext) {
	world := &impactWorld{}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*world = impactWorld{}
		return ctx, nil
	})
	sc.Step(`^an impact dossier with complete exact dependencies$`, world.completeDossier)
	sc.Step(`^the source artifact revision changes$`, world.changeSource)
	sc.Step(`^no artifact revision changes$`, world.noChanges)
	sc.Step(`^the impact report marks evidence "([^"]*)" stale$`, world.evidenceStale)
	sc.Step(`^the impact report marks dossier "([^"]*)" for renewal$`, world.dossierStale)
	sc.Step(`^the impact SARIF finding has rule "([^"]*)" at "([^"]*)"$`, world.sarifFinding)
	sc.Step(`^the impact report has no findings$`, world.noFindings)
	sc.Step(`^an impact dossier with an executed model verdict and exact provenance$`, world.executedVerdict)
	sc.Step(`^the engine build revision changes$`, world.changeEngine)
	sc.Step(`^the engine toolchain changes$`, world.changeToolchain)
	sc.Step(`^the evidence content hash changes$`, world.changeEvidenceHash)
	sc.Step(`^the review revision changes$`, world.changeReview)
	sc.Step(`^the verdict review scope does not match$`, world.changeReviewScope)
	sc.Step(`^a verdict dependency is missing$`, world.missingVerdictDependency)
	sc.Step(`^the impact dossier is analyzed$`, world.analyze)
	sc.Step(`^an impact dossier with an executed model verdict missing its build revision$`, world.missingBuild)
	sc.Step(`^the impact analysis is rejected with "([^"]*)"$`, world.rejected)
	sc.Step(`^the impact report marks verdict "([^"]*)" stale$`, world.verdictStale)
	sc.Step(`^the impact report does not promote verdict "([^"]*)"$`, world.verdictUnpromoted)
	sc.Step(`^the impact report carries no implementation pass$`, world.noImplementationPass)
	sc.Step(`^one evidence ID has an unchanged hash followed by a changed hash$`, world.repeatEvidenceChange)
	sc.Step(`^impact analysis rejects the ambiguous evidence changes$`, world.rejectAmbiguousChanges)
	sc.Step(`^the reviewer decision ID is whitespace only$`, world.whitespaceDecision)
	sc.Step(`^an impact dossier with complete exact dependencies at "([^"]*)"$`, world.completeDossierAt)
	sc.Step(`^the impact report is rendered as SARIF$`, world.renderSARIF)
}

func (w *impactWorld) whitespaceDecision() error {
	w.input.Dossiers[0].Decision.ID = " \t"
	return w.analyze()
}

func (w *impactWorld) completeDossierAt(location string) error {
	if err := w.completeDossier(); err != nil {
		return err
	}
	w.input.Evidence[0].Location = location
	w.input.Evidence = append(w.input.Evidence, impact.Evidence{
		ID: "evidence.orphan", ContentSHA256: strings.Repeat("b", 64), Location: location,
		Dependencies: w.input.Evidence[0].Dependencies,
	})
	return nil
}

func (w *impactWorld) renderSARIF() error {
	w.report, w.err = impact.Analyze(w.input)
	return w.err
}

func (w *impactWorld) completeDossier() error {
	w.input = impact.Input{
		Artifacts: []impact.Artifact{
			{ID: "source.protocol", Kind: impact.Source, Revision: "a1b2c3d4"},
			{ID: "model.protocol", Kind: impact.Model, Revision: "m1"},
			{ID: "property.safety", Kind: impact.Property, Revision: "p1"},
			{ID: "environment.network", Kind: impact.ModelledEnvironment, Revision: "e1"},
			{ID: "adapter.go", Kind: impact.Adapter, Revision: "adapter-build-1", ContractVersion: "contract-1"},
			{ID: "checker.mcd", Kind: impact.Checker, Revision: "mcd-build-1", Toolchain: "go1.26.9"},
		},
		Evidence: []impact.Evidence{{
			ID: "evidence.mapping", ContentSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Location: "src/protocol.go:12",
			Dependencies: []impact.Dependency{
				{ArtifactID: "source.protocol", Revision: "a1b2c3d4"},
				{ArtifactID: "model.protocol", Revision: "m1"},
				{ArtifactID: "property.safety", Revision: "p1"},
				{ArtifactID: "environment.network", Revision: "e1"},
				{ArtifactID: "adapter.go", Revision: "adapter-build-1", ContractVersion: "contract-1"},
				{ArtifactID: "checker.mcd", Revision: "mcd-build-1", Toolchain: "go1.26.9"},
			},
		}},
		Dossiers: []impact.Dossier{{
			ID: "dossier.protocol", TargetRevision: "a1b2c3d4", ScopeIDs: []string{"source.protocol"}, EvidenceIDs: []string{"evidence.mapping"},
			Decision: impact.ReviewerDecision{ID: "review.protocol", TargetRevision: "a1b2c3d4", ScopeIDs: []string{"source.protocol"}, Workflow: "independent review", Rationale: "mapping accepted", RenewOn: []impact.RenewalRule{impact.RenewOnDependencyChange}},
		}},
	}
	return nil
}

func (w *impactWorld) changeSource() error {
	w.input.Changes = []impact.Change{{ArtifactID: "source.protocol", Revision: "d4c3b2a1"}}
	w.report, w.err = impact.Analyze(w.input)
	return w.err
}

func (w *impactWorld) repeatEvidenceChange() error {
	w.input.EvidenceChanges = []impact.EvidenceChange{
		{EvidenceID: "evidence.mapping", ContentSHA256: strings.Repeat("a", 64)},
		{EvidenceID: "evidence.mapping", ContentSHA256: strings.Repeat("b", 64)},
	}
	w.report, w.err = impact.Analyze(w.input)
	return nil
}

func (w *impactWorld) rejectAmbiguousChanges() error {
	if w.err == nil || !strings.Contains(w.err.Error(), "ambiguous multiple changes") {
		return fmt.Errorf("expected ambiguous evidence changes error, got %v", w.err)
	}
	return nil
}

func (w *impactWorld) noChanges() error {
	w.report, w.err = impact.Analyze(w.input)
	return w.err
}

func (w *impactWorld) evidenceStale(id string) error {
	if !hasFinding(w.report, "stale-evidence", id) {
		return fmt.Errorf("stale finding for %q not found: %#v", id, w.report.Findings)
	}
	return nil
}

func (w *impactWorld) dossierStale(id string) error {
	if !hasFinding(w.report, "review-renewal", id) {
		return fmt.Errorf("renewal finding for %q not found: %#v", id, w.report.Findings)
	}
	return nil
}

func (w *impactWorld) sarifFinding(rule, location string) error {
	sarif, err := impact.SARIF(w.report)
	if err != nil {
		return err
	}
	var document struct {
		Runs []struct {
			Results []struct {
				RuleID    string `json:"ruleId"`
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(sarif, &document); err != nil {
		return err
	}
	for _, run := range document.Runs {
		for _, result := range run.Results {
			if result.RuleID != rule || len(result.Locations) == 0 {
				continue
			}
			physical := result.Locations[0].PhysicalLocation
			got := physical.ArtifactLocation.URI
			if physical.Region.StartLine > 0 {
				got = fmt.Sprintf("%s:%d", got, physical.Region.StartLine)
			}
			if got == location {
				return nil
			}
		}
	}
	return fmt.Errorf("SARIF lacks rule %q at %q: %s", rule, location, sarif)
}

func (w *impactWorld) noFindings() error {
	if len(w.report.Findings) != 0 {
		return fmt.Errorf("findings = %#v, want none", w.report.Findings)
	}
	return nil
}

func (w *impactWorld) executedVerdict() error {
	w.completeDossier()
	w.input.Artifacts = append(w.input.Artifacts, impact.Artifact{ID: "registry.protocol", Kind: impact.Model, Revision: "registry-r1"})
	w.input.RunRegistryRevision = "registry-r1"
	w.input.Verdicts = []impact.ModelVerdict{{ID: "verdict.protocol", RunID: "run-1", RegistryRevision: "registry-r1", EngineBuildRevision: "mcd-build-1", Toolchain: "go1.26.9", CheckerArtifactID: "checker.mcd", RegistryArtifactID: "registry.protocol", EvidenceIDs: []string{"evidence.mapping"}, MappingApproved: true}}
	return nil
}
func (w *impactWorld) changeEngine() error {
	w.input.Changes = []impact.Change{{ArtifactID: "checker.mcd", Revision: "mcd-build-2", Toolchain: "go1.26.9"}}
	return w.analyze()
}
func (w *impactWorld) changeToolchain() error {
	w.input.Changes = []impact.Change{{ArtifactID: "checker.mcd", Revision: "mcd-build-1", Toolchain: "go1.27.0"}}
	return w.analyze()
}
func (w *impactWorld) changeEvidenceHash() error {
	w.input.EvidenceChanges = []impact.EvidenceChange{{EvidenceID: "evidence.mapping", ContentSHA256: strings.Repeat("b", 64)}}
	return w.analyze()
}
func (w *impactWorld) changeReview() error {
	w.input.Dossiers[0].Decision.TargetRevision = "other-revision"
	return w.analyze()
}
func (w *impactWorld) changeReviewScope() error {
	w.executedVerdict()
	w.input.Dossiers[0].Decision.ScopeIDs = []string{"different.scope"}
	return w.analyze()
}
func (w *impactWorld) missingVerdictDependency() error {
	w.executedVerdict()
	w.input.Evidence[0].Dependencies = w.input.Evidence[0].Dependencies[:5]
	return w.analyze()
}
func (w *impactWorld) analyze() error { w.report, w.err = impact.Analyze(w.input); return nil }
func (w *impactWorld) missingBuild() error {
	w.executedVerdict()
	w.input.Verdicts[0].EngineBuildRevision = ""
	return nil
}
func (w *impactWorld) rejected(want string) error {
	if w.err == nil || !strings.Contains(w.err.Error(), want) {
		return fmt.Errorf("error = %v, want %q", w.err, want)
	}
	return nil
}
func (w *impactWorld) verdictStale(id string) error {
	if !hasFinding(w.report, "stale-verdict", id) {
		return fmt.Errorf("stale verdict %q not found: %#v", id, w.report.Findings)
	}
	return nil
}
func (w *impactWorld) verdictUnpromoted(id string) error {
	if !hasFinding(w.report, "verdict-unpromoted", id) {
		return fmt.Errorf("unpromoted verdict %q not found: %#v", id, w.report.Findings)
	}
	return nil
}
func (w *impactWorld) noImplementationPass() error {
	for _, f := range w.report.Findings {
		if strings.Contains(strings.ToLower(f.Message), "implementation") && strings.Contains(strings.ToLower(f.Message), "pass") {
			return fmt.Errorf("implementation pass finding: %#v", f)
		}
	}
	return nil
}

func hasFinding(report impact.Report, kind, subject string) bool {
	for _, finding := range report.Findings {
		if finding.Kind == kind && finding.Subject == subject {
			return true
		}
	}
	return false
}
