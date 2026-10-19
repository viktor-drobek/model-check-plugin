// Package impact analyses trace-evidence dependencies after an explicitly
// identified change. It records review obligations and never concludes that an
// implementation conforms to a requirement.
package impact

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ArtifactKind names the only artifact classes that can support impact analysis.
type ArtifactKind string

const (
	Source              ArtifactKind = "source"
	Model               ArtifactKind = "model"
	Property            ArtifactKind = "property"
	ModelledEnvironment ArtifactKind = "modelled-environment"
	Adapter             ArtifactKind = "adapter"
	Checker             ArtifactKind = "checker"
)

var requiredKinds = []ArtifactKind{Source, Model, Property, ModelledEnvironment, Adapter, Checker}

var sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Artifact is the current immutable identity of a dependency. Adapter identity
// includes both its implementation revision and its contract version; checker
// identity includes both its build revision and toolchain.
type Artifact struct {
	ID              string
	Kind            ArtifactKind
	Revision        string
	ContractVersion string
	Toolchain       string
}

// Dependency is the exact immutable artifact identity consumed by one evidence
// record. IDs are resolved by exact equality only.
type Dependency struct {
	ArtifactID      string
	Revision        string
	ContractVersion string
	Toolchain       string
}

// Evidence is an evidence record whose dependencies are explicit and immutable.
type Evidence struct {
	ID            string
	ContentSHA256 string
	Location      string
	Dependencies  []Dependency
}

// EvidenceChange supplies the new immutable digest for one exact evidence ID.
type EvidenceChange struct {
	EvidenceID    string
	ContentSHA256 string
}

// ModelVerdict records the provenance required for an executed model result.
// It is never an implementation-conformance conclusion. A verdict is
// promotable only while its run, registry, engine build, toolchain, mapping,
// and dependent evidence remain exact and current.
type ModelVerdict struct {
	ID                  string
	RunID               string
	RegistryRevision    string
	EngineBuildRevision string
	Toolchain           string
	EvidenceIDs         []string
	MappingApproved     bool
	CheckerArtifactID   string
	RegistryArtifactID  string
}

// RenewalRule explains when a review decision must be renewed.
type RenewalRule string

const (
	RenewOnDependencyChange RenewalRule = "dependency-change"
	RenewOnScopeChange      RenewalRule = "scope-change"
)

// ReviewerDecision records a review workflow without upgrading the underlying
// evidence. It applies only to the exact dossier target revision and scope.
type ReviewerDecision struct {
	ID             string
	TargetRevision string
	ScopeIDs       []string
	Workflow       string
	Rationale      string
	RenewOn        []RenewalRule
}

// Dossier groups evidence for a defined target and review decision.
type Dossier struct {
	ID             string
	TargetRevision string
	ScopeIDs       []string
	EvidenceIDs    []string
	Decision       ReviewerDecision
}

// Change replaces the current identity for one exact artifact ID. An omitted
// field retains the previous value; a changed adapter or checker identity must
// explicitly contain its required companion field.
type Change struct {
	ArtifactID      string
	Revision        string
	ContractVersion string
	Toolchain       string
	Removed         bool
}

// Input is a complete, offline impact-analysis snapshot.
type Input struct {
	Artifacts           []Artifact
	Evidence            []Evidence
	Dossiers            []Dossier
	Changes             []Change
	EvidenceChanges     []EvidenceChange
	Verdicts            []ModelVerdict
	RunRegistryRevision string
}

// Finding is a deterministic unresolved impact obligation.
type Finding struct {
	ID       string
	Kind     string
	Subject  string
	Location string
	Message  string
}

// Report records every open impact obligation in stable order.
type Report struct {
	Findings []Finding
}

// Analyze validates the complete snapshot and computes orphan, dependency-gap,
// stale-evidence, and reviewer-renewal obligations deterministically.
func Analyze(input Input) (Report, error) {
	artifacts, err := indexArtifacts(input.Artifacts)
	if err != nil {
		return Report{}, err
	}
	changed, err := applyChanges(artifacts, input.Changes)
	if err != nil {
		return Report{}, err
	}
	evidence, err := indexEvidence(input.Evidence)
	if err != nil {
		return Report{}, err
	}
	changedEvidence, err := applyEvidenceChanges(evidence, input.EvidenceChanges)
	if err != nil {
		return Report{}, err
	}
	dossiers, referenced, err := indexDossiers(input.Dossiers, evidence)
	if err != nil {
		return Report{}, err
	}

	findings := make([]Finding, 0)
	stale := make(map[string]bool, len(evidence))
	incomplete := make(map[string]bool, len(evidence))
	for _, ev := range evidence {
		missing, changedDependency, err := dependencyState(ev, artifacts, changed)
		if err != nil {
			return Report{}, err
		}
		if len(missing) > 0 {
			incomplete[ev.ID] = true
			findings = append(findings, finding("missing-dependency", ev.ID, ev.Location, "evidence is missing required exact dependencies: "+strings.Join(missing, ", ")))
		}
		if len(changedDependency) > 0 || changedEvidence[ev.ID] {
			stale[ev.ID] = true
			message := "evidence content hash changed"
			if len(changedDependency) > 0 {
				message = "evidence depends on changed immutable artifact identities: " + strings.Join(changedDependency, ", ")
			}
			findings = append(findings, finding("stale-evidence", ev.ID, ev.Location, message))
		}
		if !referenced[ev.ID] {
			findings = append(findings, finding("orphan-evidence", ev.ID, ev.Location, "evidence is not referenced by a dossier"))
		}
	}

	for _, dossier := range dossiers {
		decisionStale, reason, err := reviewState(dossier, stale)
		if err != nil {
			return Report{}, err
		}
		if decisionStale {
			findings = append(findings, finding("review-renewal", dossier.ID, dossierLocation(dossier, evidence), reason))
		}
	}
	verdicts, err := indexVerdicts(input.Verdicts, evidence)
	if err != nil {
		return Report{}, err
	}
	for _, verdict := range verdicts {
		staleVerdict, reason := verdictState(verdict, artifacts, changed, changedEvidence, stale, incomplete, dossiers, input.RunRegistryRevision)
		if staleVerdict {
			findings = append(findings, finding("stale-verdict", verdict.ID, "verdict:"+verdict.ID, reason))
		}
		if !verdictPromotable(verdict, staleVerdict, incomplete, dossiers) {
			findings = append(findings, finding("verdict-unpromoted", verdict.ID, "verdict:"+verdict.ID, "model verdict is not promoted because its exact provenance or review mapping is not current"))
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Kind != findings[j].Kind {
			return findings[i].Kind < findings[j].Kind
		}
		return findings[i].Subject < findings[j].Subject
	})
	return Report{Findings: findings}, nil
}

func indexVerdicts(items []ModelVerdict, evidence map[string]Evidence) ([]ModelVerdict, error) {
	seen := make(map[string]bool, len(items))
	for _, verdict := range items {
		if strings.TrimSpace(verdict.ID) == "" || strings.TrimSpace(verdict.RunID) == "" || strings.TrimSpace(verdict.RegistryRevision) == "" || strings.TrimSpace(verdict.EngineBuildRevision) == "" || strings.TrimSpace(verdict.Toolchain) == "" || strings.TrimSpace(verdict.CheckerArtifactID) == "" || strings.TrimSpace(verdict.RegistryArtifactID) == "" {
			return nil, fmt.Errorf("impact: model verdict %q needs run, registry, engine build, and toolchain provenance", verdict.ID)
		}
		if seen[verdict.ID] {
			return nil, fmt.Errorf("impact: duplicate model verdict ID %q", verdict.ID)
		}
		seen[verdict.ID] = true
		if len(verdict.EvidenceIDs) == 0 {
			return nil, fmt.Errorf("impact: model verdict %q needs dependent evidence", verdict.ID)
		}
		for _, id := range verdict.EvidenceIDs {
			if _, ok := evidence[id]; !ok {
				return nil, fmt.Errorf("impact: model verdict %q names unknown evidence %q", verdict.ID, id)
			}
		}
	}
	return items, nil
}

func verdictState(verdict ModelVerdict, artifacts, changed map[string]Artifact, changedEvidence map[string]bool, stale, incomplete map[string]bool, dossiers []Dossier, currentRegistry string) (bool, string) {
	for _, evidenceID := range verdict.EvidenceIDs {
		if incomplete[evidenceID] {
			return true, "model verdict depends on evidence with incomplete exact dependencies"
		}
		if stale[evidenceID] || changedEvidence[evidenceID] {
			return true, "model verdict depends on stale evidence"
		}
	}
	checker, ok := changed[verdict.CheckerArtifactID]
	if !ok || checker.Kind != Checker {
		return true, "model verdict depends on a changed or missing checker build"
	}
	if checker.Revision != verdict.EngineBuildRevision || checker.Toolchain != verdict.Toolchain {
		return true, "model verdict engine build or toolchain provenance does not match the current checker"
	}
	if !verdict.MappingApproved {
		return true, "model verdict mapping is not approved"
	}
	registry, ok := changed[verdict.RegistryArtifactID]
	if !ok || registry.Revision != verdict.RegistryRevision || (currentRegistry != "" && currentRegistry != verdict.RegistryRevision) {
		return true, "model verdict registry provenance is stale"
	}
	if !verdictReviewCoverage(verdict, dossiers) {
		return true, "model verdict review mapping is not current for all evidence"
	}
	return false, ""
}

func verdictPromotable(verdict ModelVerdict, stale bool, incomplete map[string]bool, dossiers []Dossier) bool {
	if stale || !verdict.MappingApproved {
		return false
	}
	for _, evidenceID := range verdict.EvidenceIDs {
		if incomplete[evidenceID] {
			return false
		}
	}
	return verdictReviewCoverage(verdict, dossiers)
}

func verdictReviewCoverage(verdict ModelVerdict, dossiers []Dossier) bool {
	for _, evidenceID := range verdict.EvidenceIDs {
		covered := false
		for _, dossier := range dossiers {
			if !containsID(dossier.EvidenceIDs, evidenceID) {
				continue
			}
			if decisionHasRequiredFields(dossier.Decision) && dossier.Decision.TargetRevision == dossier.TargetRevision && sameIDs(dossier.Decision.ScopeIDs, dossier.ScopeIDs) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func decisionHasRequiredFields(decision ReviewerDecision) bool {
	return strings.TrimSpace(decision.ID) != "" &&
		strings.TrimSpace(decision.Workflow) != "" &&
		strings.TrimSpace(decision.Rationale) != "" &&
		len(decision.RenewOn) > 0
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func indexArtifacts(items []Artifact) (map[string]Artifact, error) {
	out := make(map[string]Artifact, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Revision) == "" {
			return nil, fmt.Errorf("impact: artifact needs non-blank ID and revision")
		}
		if !isRequiredKind(item.Kind) {
			return nil, fmt.Errorf("impact: artifact %q has unsupported kind %q", item.ID, item.Kind)
		}
		if item.Kind == Adapter && strings.TrimSpace(item.ContractVersion) == "" {
			return nil, fmt.Errorf("impact: adapter %q needs a contract version", item.ID)
		}
		if item.Kind == Checker && strings.TrimSpace(item.Toolchain) == "" {
			return nil, fmt.Errorf("impact: checker %q needs a toolchain", item.ID)
		}
		if _, exists := out[item.ID]; exists {
			return nil, fmt.Errorf("impact: duplicate artifact ID %q", item.ID)
		}
		out[item.ID] = item
	}
	return out, nil
}

func indexEvidence(items []Evidence) (map[string]Evidence, error) {
	out := make(map[string]Evidence, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Location) == "" || !sha256RE.MatchString(item.ContentSHA256) {
			return nil, fmt.Errorf("impact: evidence needs non-blank ID and location plus lowercase SHA-256 content hash")
		}
		if _, exists := out[item.ID]; exists {
			return nil, fmt.Errorf("impact: duplicate evidence ID %q", item.ID)
		}
		out[item.ID] = item
	}
	return out, nil
}

func applyEvidenceChanges(evidence map[string]Evidence, changes []EvidenceChange) (map[string]bool, error) {
	changed := make(map[string]bool, len(changes))
	for _, change := range changes {
		if strings.TrimSpace(change.EvidenceID) == "" || !sha256RE.MatchString(change.ContentSHA256) {
			return nil, fmt.Errorf("impact: evidence change needs exact ID and lowercase SHA-256 content hash")
		}
		record, ok := evidence[change.EvidenceID]
		if !ok {
			return nil, fmt.Errorf("impact: evidence change names unknown evidence %q", change.EvidenceID)
		}
		if _, seen := changed[change.EvidenceID]; seen {
			return nil, fmt.Errorf("impact: ambiguous multiple changes for evidence %q", change.EvidenceID)
		}
		changed[change.EvidenceID] = record.ContentSHA256 != change.ContentSHA256
	}
	return changed, nil
}

func indexDossiers(items []Dossier, evidence map[string]Evidence) ([]Dossier, map[string]bool, error) {
	ids := make(map[string]bool, len(items))
	referenced := make(map[string]bool, len(evidence))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.TargetRevision) == "" || len(item.ScopeIDs) == 0 {
			return nil, nil, fmt.Errorf("impact: dossier needs non-blank ID, target revision, and scope")
		}
		if ids[item.ID] {
			return nil, nil, fmt.Errorf("impact: duplicate dossier ID %q", item.ID)
		}
		ids[item.ID] = true
		for _, evidenceID := range item.EvidenceIDs {
			if _, ok := evidence[evidenceID]; !ok {
				return nil, nil, fmt.Errorf("impact: dossier %q names unknown evidence %q", item.ID, evidenceID)
			}
			referenced[evidenceID] = true
		}
	}
	return items, referenced, nil
}

func applyChanges(current map[string]Artifact, changes []Change) (map[string]Artifact, error) {
	out := make(map[string]Artifact, len(current))
	for id, artifact := range current {
		out[id] = artifact
	}
	seen := make(map[string]bool, len(changes))
	for _, change := range changes {
		if strings.TrimSpace(change.ArtifactID) == "" {
			return nil, fmt.Errorf("impact: change has blank artifact ID")
		}
		if seen[change.ArtifactID] {
			return nil, fmt.Errorf("impact: ambiguous multiple changes for artifact %q", change.ArtifactID)
		}
		seen[change.ArtifactID] = true
		artifact, ok := out[change.ArtifactID]
		if !ok {
			return nil, fmt.Errorf("impact: change names unknown artifact %q", change.ArtifactID)
		}
		if change.Removed {
			if change.Revision != "" || change.ContractVersion != "" || change.Toolchain != "" {
				return nil, fmt.Errorf("impact: removed artifact %q must not carry a replacement identity", change.ArtifactID)
			}
			delete(out, change.ArtifactID)
			continue
		}
		if strings.TrimSpace(change.Revision) == "" {
			return nil, fmt.Errorf("impact: change for %q needs an immutable revision", change.ArtifactID)
		}
		artifact.Revision = change.Revision
		if change.ContractVersion != "" {
			artifact.ContractVersion = change.ContractVersion
		}
		if change.Toolchain != "" {
			artifact.Toolchain = change.Toolchain
		}
		if artifact.Kind == Adapter && strings.TrimSpace(artifact.ContractVersion) == "" {
			return nil, fmt.Errorf("impact: changed adapter %q has no contract version", artifact.ID)
		}
		if artifact.Kind == Checker && strings.TrimSpace(artifact.Toolchain) == "" {
			return nil, fmt.Errorf("impact: changed checker %q has no toolchain", artifact.ID)
		}
		out[artifact.ID] = artifact
	}
	return out, nil
}

func dependencyState(ev Evidence, base, changed map[string]Artifact) ([]string, []string, error) {
	seenKinds := make(map[ArtifactKind]bool, len(ev.Dependencies))
	changedIDs := make([]string, 0)
	for _, dep := range ev.Dependencies {
		artifact, ok := base[dep.ArtifactID]
		if !ok {
			return nil, nil, fmt.Errorf("impact: evidence %q names unknown artifact %q", ev.ID, dep.ArtifactID)
		}
		if seenKinds[artifact.Kind] {
			return nil, nil, fmt.Errorf("impact: evidence %q has ambiguous %s dependency mapping", ev.ID, artifact.Kind)
		}
		seenKinds[artifact.Kind] = true
		if !sameIdentity(dep, artifact) {
			return nil, nil, fmt.Errorf("impact: evidence %q dependency %q does not exactly match its declared artifact identity", ev.ID, dep.ArtifactID)
		}
		changedArtifact, stillPresent := changed[artifact.ID]
		if !stillPresent || !sameArtifact(artifact, changedArtifact) {
			changedIDs = append(changedIDs, dep.ArtifactID)
		}
	}
	missing := make([]string, 0)
	for _, kind := range requiredKinds {
		if !seenKinds[kind] {
			missing = append(missing, string(kind))
		}
	}
	sort.Strings(changedIDs)
	return missing, changedIDs, nil
}

func sameIdentity(dep Dependency, artifact Artifact) bool {
	return dep.Revision == artifact.Revision && dep.ContractVersion == artifact.ContractVersion && dep.Toolchain == artifact.Toolchain
}

func sameArtifact(left, right Artifact) bool {
	return left.ID == right.ID && left.Kind == right.Kind && left.Revision == right.Revision && left.ContractVersion == right.ContractVersion && left.Toolchain == right.Toolchain
}

func reviewState(dossier Dossier, stale map[string]bool) (bool, string, error) {
	decision := dossier.Decision
	for _, rule := range decision.RenewOn {
		switch rule {
		case RenewOnDependencyChange, RenewOnScopeChange:
		default:
			return false, "", fmt.Errorf("impact: reviewer decision %q has unsupported renewal rule %q", decision.ID, rule)
		}
	}
	if strings.TrimSpace(decision.ID) == "" {
		return true, "dossier has no applicable reviewer decision", nil
	}
	if strings.TrimSpace(decision.Workflow) == "" || strings.TrimSpace(decision.Rationale) == "" || len(decision.RenewOn) == 0 {
		return false, "", fmt.Errorf("impact: reviewer decision %q needs workflow, rationale, and renewal semantics", decision.ID)
	}
	if decision.TargetRevision != dossier.TargetRevision || !sameIDs(decision.ScopeIDs, dossier.ScopeIDs) {
		return true, "review decision does not exactly match dossier target revision and scope", nil
	}
	for _, rule := range decision.RenewOn {
		switch rule {
		case RenewOnDependencyChange:
			for _, evidenceID := range dossier.EvidenceIDs {
				if stale[evidenceID] {
					return true, "review decision must be renewed after dependent evidence became stale", nil
				}
			}
		case RenewOnScopeChange:
			// Exact scope matching above is the observable scope-change rule.
		}
	}
	return false, "", nil
}

func sameIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for i := range leftCopy {
		if leftCopy[i] == "" || rightCopy[i] == "" || leftCopy[i] != rightCopy[i] {
			return false
		}
	}
	return true
}

func dossierLocation(dossier Dossier, evidence map[string]Evidence) string {
	if len(dossier.EvidenceIDs) == 0 {
		return "dossier:" + dossier.ID
	}
	locations := make([]string, 0, len(dossier.EvidenceIDs))
	for _, id := range dossier.EvidenceIDs {
		locations = append(locations, evidence[id].Location)
	}
	sort.Strings(locations)
	return locations[0]
}

func finding(kind, subject, location, message string) Finding {
	return Finding{ID: "impact." + kind + "." + subject, Kind: kind, Subject: subject, Location: location, Message: message}
}

func isRequiredKind(kind ArtifactKind) bool {
	for _, candidate := range requiredKinds {
		if kind == candidate {
			return true
		}
	}
	return false
}
