// Package dossier builds a bounded, deterministic conformance dossier for a
// declared finite model. It never reads files, uses a network, or starts a
// process; all facts in a dossier are supplied by its caller.
package dossier

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Schema identifies the JSON contract emitted by this package.
const Schema = "mcd-conformance-dossier/1"

const (
	maxItems      = 128
	maxIDLength   = 128
	maxTextLength = 512
	maxBound      = 1_000_000
)

var idRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]*$`)
var immutableRevisionRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Status retains the public mcd status vocabulary without changing its
// meaning. A dossier reports a status about the declared finite model only.
type Status string

const (
	StatusVerified     Status = "verified"
	StatusViolated     Status = "violated"
	StatusInconclusive Status = "inconclusive"
	StatusUnknown      Status = "unknown"
	StatusNotExecuted  Status = "not-executed"
	StatusInvalidModel Status = "invalid-model"
)

// Reference identifies an admitted artifact and its non-private stable anchor.
type Reference struct {
	ID     string `json:"id"`
	Anchor string `json:"anchor"`
}

// Bound declares one finite state-space limit used by the model.
type Bound struct {
	Name  string `json:"name"`
	Limit int    `json:"limit"`
}

// Brief describes the algorithm or protocol and the exact immutable target it
// is about. Requirements and trace nodes are caller-provided ledger IDs.
type Brief struct {
	ID             string      `json:"id"`
	TargetRevision string      `json:"target_revision"`
	ScopeIDs       []string    `json:"scope_ids"`
	TargetIDs      []string    `json:"target_ids"`
	Requirements   []Reference `json:"requirements"`
	TraceNodes     []Reference `json:"trace_nodes"`
	Algorithm      string      `json:"algorithm"`
	Protocol       string      `json:"protocol"`
}

// ModelIntake states every modelling assumption required for a hand-off.
type ModelIntake struct {
	ID                string      `json:"id"`
	StateBounds       []Bound     `json:"state_bounds"`
	Atomicity         []string    `json:"atomicity"`
	Environment       []string    `json:"environment"`
	SchedulerFairness []string    `json:"scheduler_fairness"`
	PropertyScope     []string    `json:"property_scope"`
	ModelTransitions  []Reference `json:"model_transitions"`
	Properties        []Reference `json:"properties"`
	Assumptions       []Reference `json:"assumptions"`
}

// ReviewerDecision is an immutable, caller-supplied record. Build copies only
// decisions whose revision, scope IDs, and target IDs exactly match the brief.
type ReviewerDecision struct {
	ID             string   `json:"id"`
	TargetRevision string   `json:"target_revision"`
	ScopeIDs       []string `json:"scope_ids"`
	TargetIDs      []string `json:"target_ids"`
	Disposition    string   `json:"disposition"`
}

// ModelResult reports a supplied finite-model result. It deliberately has no
// field for a source implementation conclusion.
type ModelResult struct {
	Status  Status `json:"status"`
	Verdict string `json:"verdict,omitempty"`
}

// Input is the complete in-memory hand-off supplied to Build.
type Input struct {
	Brief     Brief              `json:"brief"`
	Intake    ModelIntake        `json:"intake"`
	Decisions []ReviewerDecision `json:"decisions"`
	Links     []Link             `json:"links"`
	Gaps      []Gap              `json:"gaps"`
	Result    ModelResult        `json:"result"`
}

// Link is a caller-declared structural relation. Its kind and endpoints are
// checked against the declared dossier categories; Build never infers one.
type Link struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// Coverage makes the supplied scope explicit rather than inferring coverage.
type Coverage struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Subject string   `json:"subject"`
	Related []string `json:"related"`
}

// Gap records an unmet review obligation without inventing a relation.
type Gap struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	Reason    string `json:"reason"`
}

// Result is the externally visible status boundary of a dossier.
type Result struct {
	Status                   Status `json:"status"`
	Verdict                  string `json:"verdict,omitempty"`
	FiniteModelOnly          bool   `json:"finite_model_only"`
	ImplementationConclusion string `json:"implementation_conclusion"`
}

// DecisionFingerprint preserves the exact Build-time identity of an
// applicable reviewer decision without retaining its free-text fields twice.
type DecisionFingerprint struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}

// ReviewProvenance is the serialized Build-time reviewer ledger. Its presence
// is required so a decoded document cannot silently lose review obligations.
type ReviewProvenance struct {
	Applicable       []DecisionFingerprint `json:"applicable"`
	NonApplicableIDs []string              `json:"non_applicable_ids"`
}

// Document is a self-contained, canonicalizable dossier. It is distinct from
// trace ledger schemas and therefore leaves trace v1 behavior unchanged.
type Document struct {
	Schema            string             `json:"schema"`
	Brief             Brief              `json:"brief"`
	Intake            ModelIntake        `json:"intake"`
	ReviewerDecisions []ReviewerDecision `json:"reviewer_decisions"`
	ReviewProvenance  ReviewProvenance   `json:"review_provenance"`
	Links             []Link             `json:"links"`
	Coverage          []Coverage         `json:"coverage"`
	Gaps              []Gap              `json:"gaps"`
	Result            Result             `json:"result"`
}

// Build validates an in-memory intake and creates a deterministic dossier
// skeleton. It does not access the filesystem, environment, network, or any
// process. A non-applicable decision is recorded as a gap, not linked.
func Build(input Input) (*Document, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}

	brief := cloneBrief(input.Brief)
	intake := cloneIntake(input.Intake)
	sortBrief(&brief)
	sortIntake(&intake)
	decisions := applicableDecisions(brief, input.Decisions)
	provenance := buildReviewProvenance(decisions, input.Decisions)
	occupied := gapIDs(input.Gaps)
	gaps, err := mergeGaps(input.Gaps, reviewerGaps(brief, input.Decisions, decisions, occupied), missingRelationGaps(input.Links, decisions, intake.PropertyScope, occupied))
	if err != nil {
		return nil, err
	}
	doc := &Document{
		Schema:            Schema,
		Brief:             brief,
		Intake:            intake,
		ReviewerDecisions: decisions,
		ReviewProvenance:  provenance,
		Links:             cloneLinks(input.Links),
		Coverage:          skeletonCoverage(brief, intake, input.Links),
		Gaps:              gaps,
		Result: Result{
			Status:                   input.Result.Status,
			Verdict:                  input.Result.Verdict,
			FiniteModelOnly:          true,
			ImplementationConclusion: "not_assessed",
		},
	}
	return Canonical(doc)
}

// Canonical returns a sorted copy of a valid dossier without changing caller
// memory. Sorting makes the JSON result stable across repeated builds.
func Canonical(document *Document) (*Document, error) {
	if err := Validate(document); err != nil {
		return nil, err
	}
	out := *document
	out.Brief = cloneBrief(document.Brief)
	out.Intake = cloneIntake(document.Intake)
	out.ReviewerDecisions = cloneDecisions(document.ReviewerDecisions)
	out.ReviewProvenance = cloneReviewProvenance(document.ReviewProvenance)
	out.Links = append([]Link{}, document.Links...)
	out.Coverage = cloneCoverage(document.Coverage)
	out.Gaps = append([]Gap{}, document.Gaps...)
	sort.Strings(out.Brief.ScopeIDs)
	sort.Strings(out.Brief.TargetIDs)
	sortReferences(out.Brief.Requirements)
	sortReferences(out.Brief.TraceNodes)
	sort.Strings(out.Intake.Atomicity)
	sort.Strings(out.Intake.Environment)
	sort.Strings(out.Intake.SchedulerFairness)
	sort.Strings(out.Intake.PropertyScope)
	sort.Slice(out.Intake.StateBounds, func(i, j int) bool { return out.Intake.StateBounds[i].Name < out.Intake.StateBounds[j].Name })
	sortReferences(out.Intake.ModelTransitions)
	sortReferences(out.Intake.Properties)
	sortReferences(out.Intake.Assumptions)
	for i := range out.ReviewerDecisions {
		sort.Strings(out.ReviewerDecisions[i].ScopeIDs)
		sort.Strings(out.ReviewerDecisions[i].TargetIDs)
	}
	sort.Slice(out.ReviewerDecisions, func(i, j int) bool { return out.ReviewerDecisions[i].ID < out.ReviewerDecisions[j].ID })
	sort.Slice(out.Links, func(i, j int) bool { return out.Links[i].ID < out.Links[j].ID })
	for i := range out.Coverage {
		sort.Strings(out.Coverage[i].Related)
	}
	sort.Slice(out.Coverage, func(i, j int) bool { return out.Coverage[i].ID < out.Coverage[j].ID })
	sort.Slice(out.Gaps, func(i, j int) bool { return out.Gaps[i].ID < out.Gaps[j].ID })
	return &out, nil
}

// JSON encodes a valid dossier with stable indentation and a trailing newline.
func JSON(document *Document) ([]byte, error) {
	canonical, err := Canonical(document)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(canonical); err != nil {
		return nil, fmt.Errorf("dossier: encode JSON: %w", err)
	}
	return output.Bytes(), nil
}

// Validate checks the strict bounded data contract of a completed dossier.
func Validate(document *Document) error {
	if document == nil {
		return fmt.Errorf("dossier: document is nil")
	}
	if document.Schema != Schema {
		return fmt.Errorf("dossier: schema is %q, want %q", document.Schema, Schema)
	}
	if err := validateBrief(document.Brief); err != nil {
		return err
	}
	if err := validateIntake(document.Intake, document.Brief.Requirements); err != nil {
		return err
	}
	if err := validateResult(document.Result.Status, document.Result.Verdict); err != nil {
		return err
	}
	if !document.Result.FiniteModelOnly || document.Result.ImplementationConclusion != "not_assessed" {
		return fmt.Errorf("dossier: result must remain finite-model-only with implementation conclusion not_assessed")
	}
	if len(document.ReviewerDecisions) > maxItems || len(document.Links) > maxItems || len(document.Coverage) > maxItems || len(document.Gaps) > maxItems {
		return fmt.Errorf("dossier: output exceeds bounded item limit")
	}
	for i, decision := range document.ReviewerDecisions {
		if err := validateDecision(decision, fmt.Sprintf("reviewer_decisions[%d]", i)); err != nil {
			return err
		}
		if !sameSet(decision.ScopeIDs, document.Brief.ScopeIDs) || !sameSet(decision.TargetIDs, document.Brief.TargetIDs) || decision.TargetRevision != document.Brief.TargetRevision {
			return fmt.Errorf("dossier: reviewer_decisions[%d] is not applicable to the brief", i)
		}
	}
	if err := validateReviewProvenance(*document); err != nil {
		return err
	}
	if err := validateDecisionUniqueness(document.ReviewerDecisions); err != nil {
		return err
	}
	if err := validateLinks(document.Links, document.Brief, document.Intake, document.ReviewerDecisions); err != nil {
		return err
	}
	if err := validateCoverage(document); err != nil {
		return err
	}
	if err := validateCompleteness(*document); err != nil {
		return err
	}
	if err := validateGaps(document.Gaps); err != nil {
		return err
	}
	if err := validateGeneratedGapConsistency(*document); err != nil {
		return err
	}
	return nil
}

func validateInput(input Input) error {
	if err := validateBrief(input.Brief); err != nil {
		return err
	}
	if err := validateIntake(input.Intake, input.Brief.Requirements); err != nil {
		return err
	}
	if err := validateResult(input.Result.Status, input.Result.Verdict); err != nil {
		return err
	}
	if len(input.Decisions) > maxItems {
		return fmt.Errorf("dossier: decisions exceeds %d items", maxItems)
	}
	seen := map[string]bool{}
	for i, decision := range input.Decisions {
		path := fmt.Sprintf("decisions[%d]", i)
		if err := validateDecision(decision, path); err != nil {
			return err
		}
		if seen[decision.ID] {
			return fmt.Errorf("dossier: %s.id %q duplicates a decision", path, decision.ID)
		}
		seen[decision.ID] = true
	}
	if err := validateLinks(input.Links, input.Brief, input.Intake, applicableDecisions(input.Brief, input.Decisions)); err != nil {
		return err
	}
	if len(input.Links) > maxItems {
		return fmt.Errorf("dossier: links exceeds %d items", maxItems)
	}
	if err := validateGaps(input.Gaps); err != nil {
		return err
	}
	if err := validateCallerGaps(input.Gaps); err != nil {
		return err
	}
	return nil
}

func validateDecisionUniqueness(decisions []ReviewerDecision) error {
	seen := map[string]bool{}
	for _, decision := range decisions {
		if seen[decision.ID] {
			return fmt.Errorf("dossier: reviewer decision ID %q duplicates a decision", decision.ID)
		}
		seen[decision.ID] = true
	}
	return nil
}

func validateBrief(brief Brief) error {
	if err := validateID(brief.ID, "brief.id"); err != nil {
		return err
	}
	if err := validateRevision(brief.TargetRevision, "brief.target_revision"); err != nil {
		return err
	}
	if err := validateIDs(brief.ScopeIDs, "brief.scope_ids", true); err != nil {
		return err
	}
	if err := validateIDs(brief.TargetIDs, "brief.target_ids", true); err != nil {
		return err
	}
	if err := validateReferences(brief.Requirements, "brief.requirements", true); err != nil {
		return err
	}
	if err := validateReferences(brief.TraceNodes, "brief.trace_nodes", true); err != nil {
		return err
	}
	if err := validateText(brief.Algorithm, "brief.algorithm"); err != nil {
		return err
	}
	return validateText(brief.Protocol, "brief.protocol")
}

func validateIntake(intake ModelIntake, requirements []Reference) error {
	if err := validateID(intake.ID, "intake.id"); err != nil {
		return err
	}
	if err := validateBounds(intake.StateBounds); err != nil {
		return err
	}
	if err := validateTexts(intake.Atomicity, "intake.atomicity", true); err != nil {
		return err
	}
	if err := validateTexts(intake.Environment, "intake.environment", true); err != nil {
		return err
	}
	if err := validateTexts(intake.SchedulerFairness, "intake.scheduler_fairness", true); err != nil {
		return err
	}
	if err := validateIDs(intake.PropertyScope, "intake.property_scope", true); err != nil {
		return err
	}
	requirementIDs := map[string]bool{}
	for _, requirement := range requirements {
		requirementIDs[requirement.ID] = true
	}
	for _, id := range intake.PropertyScope {
		if !requirementIDs[id] {
			return fmt.Errorf("dossier: intake.property_scope %q is not a declared requirement", id)
		}
	}
	if err := validateReferences(intake.ModelTransitions, "intake.model_transitions", true); err != nil {
		return err
	}
	if err := validateReferences(intake.Properties, "intake.properties", true); err != nil {
		return err
	}
	return validateReferences(intake.Assumptions, "intake.assumptions", true)
}

func validateDecision(decision ReviewerDecision, path string) error {
	if err := validateID(decision.ID, path+".id"); err != nil {
		return err
	}
	if err := validateRevision(decision.TargetRevision, path+".target_revision"); err != nil {
		return err
	}
	if err := validateIDs(decision.ScopeIDs, path+".scope_ids", true); err != nil {
		return err
	}
	if err := validateIDs(decision.TargetIDs, path+".target_ids", true); err != nil {
		return err
	}
	return validateText(decision.Disposition, path+".disposition")
}

func validateResult(status Status, verdict string) error {
	switch status {
	case StatusVerified, StatusViolated, StatusInconclusive, StatusUnknown, StatusNotExecuted, StatusInvalidModel:
	default:
		return fmt.Errorf("dossier: result.status %q is outside mcd status vocabulary", status)
	}
	if status == StatusNotExecuted {
		if verdict != "" {
			return fmt.Errorf("dossier: not-executed result must not include a verdict")
		}
		return nil
	}
	return validateText(verdict, "result.verdict")
}

func validateLinks(links []Link, brief Brief, intake ModelIntake, decisions []ReviewerDecision) error {
	if len(links) > maxItems {
		return fmt.Errorf("dossier: links exceeds %d items", maxItems)
	}
	requirements := referencesByID(brief.Requirements)
	traceNodes := referencesByID(brief.TraceNodes)
	transitions := referencesByID(intake.ModelTransitions)
	properties := referencesByID(intake.Properties)
	assumptions := referencesByID(intake.Assumptions)
	reviews := decisionsByID(decisions)
	seen := map[string]bool{}
	for i, link := range links {
		path := fmt.Sprintf("links[%d]", i)
		if err := validateID(link.ID, path+".id"); err != nil {
			return err
		}
		if seen[link.ID] {
			return fmt.Errorf("dossier: %s.id %q duplicates a link", path, link.ID)
		}
		seen[link.ID] = true
		if err := validateID(link.Source, path+".source"); err != nil {
			return err
		}
		if err := validateID(link.Target, path+".target"); err != nil {
			return err
		}
		switch link.Kind {
		case "requirement_trace":
			if !requirements[link.Source] || !traceNodes[link.Target] {
				return fmt.Errorf("dossier: %s requirement_trace endpoints must be a declared requirement and trace node", path)
			}
		case "trace_transition":
			if !traceNodes[link.Source] || !transitions[link.Target] {
				return fmt.Errorf("dossier: %s trace_transition endpoints must be a declared trace node and model transition", path)
			}
		case "transition_property":
			if !transitions[link.Source] || !properties[link.Target] {
				return fmt.Errorf("dossier: %s transition_property endpoints must be a declared model transition and property", path)
			}
		case "assumption_transition":
			if !assumptions[link.Source] || !transitions[link.Target] {
				return fmt.Errorf("dossier: %s assumption_transition endpoints must be a declared assumption and model transition", path)
			}
		case "review_target":
			if !reviews[link.Source] || link.Target != brief.ID {
				return fmt.Errorf("dossier: %s review_target endpoints must be an applicable reviewer decision and the brief", path)
			}
		default:
			return fmt.Errorf("dossier: %s.kind %q is outside the relation vocabulary", path, link.Kind)
		}
	}
	return nil
}

func validateCoverage(document *Document) error {
	if len(document.Coverage) > maxItems {
		return fmt.Errorf("dossier: coverage exceeds %d items", maxItems)
	}
	requirements := referencesByID(document.Brief.Requirements)
	propertyScope := make(map[string]bool, len(document.Intake.PropertyScope))
	for _, id := range document.Intake.PropertyScope {
		propertyScope[id] = true
	}
	traces := referencesByID(document.Brief.TraceNodes)
	explicit := make(map[string]map[string]bool)
	for _, link := range document.Links {
		if link.Kind != "requirement_trace" {
			continue
		}
		if explicit[link.Source] == nil {
			explicit[link.Source] = map[string]bool{}
		}
		explicit[link.Source][link.Target] = true
	}
	seen := map[string]bool{}
	for i, coverage := range document.Coverage {
		path := fmt.Sprintf("coverage[%d]", i)
		if err := validateID(coverage.ID, path+".id"); err != nil {
			return err
		}
		if seen[coverage.ID] {
			return fmt.Errorf("dossier: %s.id %q duplicates coverage", path, coverage.ID)
		}
		seen[coverage.ID] = true
		if coverage.Kind != "requirement" {
			return fmt.Errorf("dossier: %s.kind %q is not requirement", path, coverage.Kind)
		}
		if !requirements[coverage.Subject] || !propertyScope[coverage.Subject] {
			return fmt.Errorf("dossier: %s.subject must be a declared property-scope requirement", path)
		}
		if len(coverage.Related) > maxItems {
			return fmt.Errorf("dossier: %s.related exceeds %d items", path, maxItems)
		}
		seenRelated := map[string]bool{}
		for _, related := range coverage.Related {
			if !traces[related] || !explicit[coverage.Subject][related] {
				return fmt.Errorf("dossier: %s.related contains a non-explicit requirement-trace relation %q", path, related)
			}
			if seenRelated[related] {
				return fmt.Errorf("dossier: %s.related contains duplicate identifier %q", path, related)
			}
			seenRelated[related] = true
		}
		if len(seenRelated) != len(explicit[coverage.Subject]) {
			return fmt.Errorf("dossier: %s.related must include every explicit requirement-trace target", path)
		}
		for related := range explicit[coverage.Subject] {
			if !seenRelated[related] {
				return fmt.Errorf("dossier: %s.related is missing explicit requirement-trace target %q", path, related)
			}
		}
		expected := skeletonCoverage(document.Brief, document.Intake, document.Links)
		for _, candidate := range expected {
			if candidate.Subject == coverage.Subject && coverage.ID != candidate.ID {
				return fmt.Errorf("dossier: coverage[%d].id %q does not match derived ID %q", i, coverage.ID, candidate.ID)
			}
		}
	}
	return nil
}

func validateGaps(gaps []Gap) error {
	if len(gaps) > maxItems {
		return fmt.Errorf("dossier: gaps exceeds %d items", maxItems)
	}
	seen := map[string]bool{}
	for i, gap := range gaps {
		path := fmt.Sprintf("gaps[%d]", i)
		if err := validateID(gap.ID, path+".id"); err != nil {
			return err
		}
		if seen[gap.ID] {
			return fmt.Errorf("dossier: %s.id %q duplicates a gap", path, gap.ID)
		}
		seen[gap.ID] = true
		if err := validateID(gap.Kind, path+".kind"); err != nil {
			return err
		}
		if err := validateText(gap.Reference, path+".reference"); err != nil {
			return err
		}
		if err := validateText(gap.Reason, path+".reason"); err != nil {
			return err
		}
	}
	return nil
}

var generatedGapKinds = map[string]bool{
	"requirement_trace_missing": true,
	"relation_missing":          true,
	"review_missing":            true,
	"review_target_mismatch":    true,
}

func validateCallerGaps(gaps []Gap) error {
	for i, gap := range gaps {
		if generatedGapKinds[gap.Kind] {
			return fmt.Errorf("dossier: gaps[%d].kind %q is a generated gap kind and cannot be supplied by callers", i, gap.Kind)
		}
	}
	return nil
}

func validateGeneratedGapConsistency(document Document) error {
	propertyScope := map[string]bool{}
	for _, requirementID := range document.Intake.PropertyScope {
		propertyScope[requirementID] = true
	}
	requiredRelations := map[string]bool{}
	for _, kind := range requiredRelationKinds(len(document.ReviewerDecisions) > 0) {
		requiredRelations[kind] = true
	}
	seenObligations := map[string]bool{}
	decisionIDs := decisionIDSet(document.ReviewerDecisions)
	for _, id := range document.ReviewProvenance.NonApplicableIDs {
		decisionIDs[id] = true
	}
	for _, gap := range document.Gaps {
		if generatedGapKinds[gap.Kind] {
			if err := validateID(gap.Reference, fmt.Sprintf("generated %s gap reference", gap.Kind)); err != nil {
				return err
			}
			obligation := gap.Kind + "\x00" + gap.Reference
			if seenObligations[obligation] {
				return fmt.Errorf("dossier: generated %s gap %q duplicates generated obligation %q", gap.Kind, gap.ID, gap.Reference)
			}
			seenObligations[obligation] = true
		}
		satisfied := false
		switch gap.Kind {
		case "requirement_trace_missing":
			if !propertyScope[gap.Reference] {
				return fmt.Errorf("dossier: generated requirement_trace_missing gap %q references requirement outside property scope", gap.ID)
			}
			satisfied = hasRequirementTrace(document.Links, gap.Reference)
		case "relation_missing":
			if !requiredRelations[gap.Reference] {
				return fmt.Errorf("dossier: generated relation_missing gap %q references a relation kind that is not required", gap.ID)
			}
			satisfied = hasLinkKind(document.Links, gap.Reference)
		case "review_missing":
			if gap.Reference != document.Brief.ID {
				return fmt.Errorf("dossier: generated review_missing gap %q must reference the brief", gap.ID)
			}
			satisfied = len(document.ReviewerDecisions) > 0
		case "review_target_mismatch":
			if !decisionIDs[gap.Reference] {
				return fmt.Errorf("dossier: generated review_target_mismatch gap reference %q: unknown reviewer decision", gap.Reference)
			}
			for _, decision := range document.ReviewerDecisions {
				if decision.ID == gap.Reference {
					return fmt.Errorf("dossier: stale generated review_target_mismatch gap %q", gap.ID)
				}
			}
		}
		if satisfied {
			return fmt.Errorf("dossier: stale generated %s gap %q", gap.Kind, gap.ID)
		}
	}
	callerGaps := make([]Gap, 0, len(document.Gaps))
	for _, gap := range document.Gaps {
		if !generatedGapKinds[gap.Kind] {
			callerGaps = append(callerGaps, gap)
		}
	}
	occupied := gapIDs(callerGaps)
	expected := append(reviewerGapsFromProvenance(&document, occupied), missingRelationGaps(document.Links, document.ReviewerDecisions, document.Intake.PropertyScope, occupied)...)
	expectedByObligation := make(map[string]Gap, len(expected))
	for _, gap := range expected {
		expectedByObligation[gap.Kind+"\x00"+gap.Reference] = gap
	}
	actualByObligation := make(map[string]Gap)
	for _, gap := range document.Gaps {
		if generatedGapKinds[gap.Kind] {
			actualByObligation[gap.Kind+"\x00"+gap.Reference] = gap
		}
	}
	if len(actualByObligation) != len(expectedByObligation) {
		return fmt.Errorf("dossier: generated gap set does not match derived obligations")
	}
	for obligation, want := range expectedByObligation {
		got, ok := actualByObligation[obligation]
		if !ok || got != want {
			return fmt.Errorf("dossier: generated %s gap %q does not match derived identity or reason", want.Kind, want.ID)
		}
	}
	return nil
}

func decisionIDSet(decisions []ReviewerDecision) map[string]bool {
	ids := make(map[string]bool, len(decisions))
	for _, decision := range decisions {
		ids[decision.ID] = true
	}
	return ids
}

func validateBounds(bounds []Bound) error {
	if len(bounds) == 0 {
		return fmt.Errorf("dossier: intake.state_bounds must not be empty")
	}
	if len(bounds) > maxItems {
		return fmt.Errorf("dossier: intake.state_bounds exceeds %d items", maxItems)
	}
	seen := map[string]bool{}
	for i, bound := range bounds {
		path := fmt.Sprintf("intake.state_bounds[%d]", i)
		if err := validateID(bound.Name, path+".name"); err != nil {
			return err
		}
		if bound.Limit < 1 || bound.Limit > maxBound {
			return fmt.Errorf("dossier: %s.limit must be between 1 and %d", path, maxBound)
		}
		if seen[bound.Name] {
			return fmt.Errorf("dossier: %s.name %q duplicates a bound", path, bound.Name)
		}
		seen[bound.Name] = true
	}
	return nil
}

func validateReferences(references []Reference, path string, required bool) error {
	if required && len(references) == 0 {
		return fmt.Errorf("dossier: %s must not be empty", path)
	}
	if len(references) > maxItems {
		return fmt.Errorf("dossier: %s exceeds %d items", path, maxItems)
	}
	seen := map[string]bool{}
	for i, reference := range references {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		if err := validateID(reference.ID, itemPath+".id"); err != nil {
			return err
		}
		if seen[reference.ID] {
			return fmt.Errorf("dossier: %s.id %q duplicates a reference", itemPath, reference.ID)
		}
		if err := validateAnchor(reference.Anchor, itemPath+".anchor"); err != nil {
			return err
		}
		seen[reference.ID] = true
	}
	return nil
}

func validateIDs(ids []string, path string, required bool) error {
	if required && len(ids) == 0 {
		return fmt.Errorf("dossier: %s must not be empty", path)
	}
	if len(ids) > maxItems {
		return fmt.Errorf("dossier: %s exceeds %d items", path, maxItems)
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if err := validateID(id, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("dossier: %s contains duplicate identifier %q", path, id)
		}
		seen[id] = true
	}
	return nil
}

func validateTexts(values []string, path string, required bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("dossier: %s must not be empty", path)
	}
	if len(values) > maxItems {
		return fmt.Errorf("dossier: %s exceeds %d items", path, maxItems)
	}
	seen := map[string]bool{}
	for i, value := range values {
		if err := validateText(value, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
		if seen[value] {
			return fmt.Errorf("dossier: %s contains duplicate declaration", path)
		}
		seen[value] = true
	}
	return nil
}

func validateID(value, path string) error {
	if len(value) == 0 || len(value) > maxIDLength {
		return fmt.Errorf("dossier: %s must be non-blank and at most %d bytes", path, maxIDLength)
	}
	if !idRE.MatchString(value) {
		return fmt.Errorf("dossier: %s %q is not a stable identifier", path, value)
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"password", "secret", "api_key", "authorization", "bearer"} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("dossier: %s contains disallowed secret-like metadata", path)
		}
	}
	if strings.HasPrefix(lower, "file:") || strings.Contains(value, "..") {
		return fmt.Errorf("dossier: %s contains an unsafe path-like value", path)
	}
	return nil
}

func validateRevision(value, path string) error {
	if err := validateText(value, path); err != nil {
		return err
	}
	if !immutableRevisionRE.MatchString(value) {
		return fmt.Errorf("dossier: %s must identify an immutable digest (sha256:<64 lowercase hex digits>)", path)
	}
	return nil
}

func validateAnchor(value, path string) error {
	if err := validateText(value, path); err != nil {
		return err
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") || strings.Contains(value, "..") || strings.Contains(value, `\\`) || strings.Contains(strings.ToLower(value), "file:") {
		return fmt.Errorf("dossier: %s is not a safe public anchor", path)
	}
	return nil
}

func validateText(value, path string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("dossier: %s contains invalid UTF-8", path)
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(value) > maxTextLength {
		return fmt.Errorf("dossier: %s must be non-blank and at most %d bytes", path, maxTextLength)
	}
	if strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return fmt.Errorf("dossier: %s contains a control character", path)
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "~") || strings.Contains(value, `\`) || strings.HasPrefix(trimmed, "file:") || (len(trimmed) > 2 && trimmed[1] == ':' && (trimmed[2] == '/' || trimmed[2] == '\\')) {
		return fmt.Errorf("dossier: %s contains an unsafe path-like value", path)
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"password", "secret", "api_key", "authorization", "bearer "} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("dossier: %s contains disallowed secret-like metadata", path)
		}
	}
	return nil
}

func applicableDecisions(brief Brief, decisions []ReviewerDecision) []ReviewerDecision {
	var out []ReviewerDecision
	for _, decision := range decisions {
		if decision.TargetRevision == brief.TargetRevision && sameSet(decision.ScopeIDs, brief.ScopeIDs) && sameSet(decision.TargetIDs, brief.TargetIDs) {
			out = append(out, cloneDecision(decision))
		}
	}
	return out
}

func buildReviewProvenance(applicable, supplied []ReviewerDecision) ReviewProvenance {
	provenance := ReviewProvenance{
		Applicable:       make([]DecisionFingerprint, 0, len(applicable)),
		NonApplicableIDs: make([]string, 0, len(supplied)-len(applicable)),
	}
	applicableIDs := decisionIDSet(applicable)
	for _, decision := range applicable {
		provenance.Applicable = append(provenance.Applicable, DecisionFingerprint{ID: decision.ID, Digest: decisionDigest(decision)})
	}
	for _, decision := range supplied {
		if !applicableIDs[decision.ID] {
			provenance.NonApplicableIDs = append(provenance.NonApplicableIDs, decision.ID)
		}
	}
	sort.Slice(provenance.Applicable, func(i, j int) bool { return provenance.Applicable[i].ID < provenance.Applicable[j].ID })
	sort.Strings(provenance.NonApplicableIDs)
	return provenance
}

func reviewerGapsFromProvenance(document *Document, occupied map[string]bool) []Gap {
	for _, decision := range document.ReviewerDecisions {
		occupied[decision.ID] = true
	}
	for _, id := range document.ReviewProvenance.NonApplicableIDs {
		occupied[id] = true
	}
	gaps := make([]Gap, 0, len(document.ReviewProvenance.NonApplicableIDs)+1)
	if len(document.ReviewerDecisions) == 0 {
		id := derivedIDAvoiding("gap.review.", "missing", "", occupied)
		occupied[id] = true
		gaps = append(gaps, Gap{ID: id, Kind: "review_missing", Reference: document.Brief.ID, Reason: "no reviewer decision exactly matches the dossier target revision, scope IDs, and target IDs"})
	}
	for _, id := range document.ReviewProvenance.NonApplicableIDs {
		gapID := derivedIDAvoiding("gap.review.", id, ".target_mismatch", occupied)
		occupied[gapID] = true
		gaps = append(gaps, Gap{ID: gapID, Kind: "review_target_mismatch", Reference: id, Reason: "reviewer decision does not exactly match the dossier target revision, scope IDs, and target IDs"})
	}
	return gaps
}

func validateReviewProvenance(document Document) error {
	if document.ReviewProvenance.Applicable == nil && document.ReviewProvenance.NonApplicableIDs == nil {
		return fmt.Errorf("dossier: review provenance is required")
	}
	if len(document.ReviewProvenance.Applicable) > maxItems {
		return fmt.Errorf("dossier: review provenance applicable exceeds %d items", maxItems)
	}
	if len(document.ReviewProvenance.NonApplicableIDs) > maxItems {
		return fmt.Errorf("dossier: review provenance exceeds %d items", maxItems)
	}
	if len(document.ReviewProvenance.Applicable)+len(document.ReviewProvenance.NonApplicableIDs) > maxItems || len(document.ReviewerDecisions)+len(document.ReviewProvenance.NonApplicableIDs) > maxItems {
		return fmt.Errorf("dossier: review provenance population exceeds %d items", maxItems)
	}
	if err := validateIDs(document.ReviewProvenance.NonApplicableIDs, "review provenance.non_applicable_ids", false); err != nil {
		return err
	}
	seen := make(map[string]bool, len(document.ReviewProvenance.Applicable))
	for i, fingerprint := range document.ReviewProvenance.Applicable {
		path := fmt.Sprintf("review provenance.applicable[%d]", i)
		if err := validateID(fingerprint.ID, path+".id"); err != nil {
			return err
		}
		if seen[fingerprint.ID] {
			return fmt.Errorf("dossier: review provenance applicable contains duplicate identifier %q", fingerprint.ID)
		}
		seen[fingerprint.ID] = true
		if err := validateRevision(fingerprint.Digest, path+".digest"); err != nil {
			return err
		}
	}
	current := buildReviewProvenance(document.ReviewerDecisions, append(cloneDecisions(document.ReviewerDecisions), decisionsForIDs(document.ReviewProvenance.NonApplicableIDs)...))
	if !equalFingerprints(current.Applicable, document.ReviewProvenance.Applicable) {
		return fmt.Errorf("dossier: review provenance does not match applicable reviewer decisions")
	}
	if !equalStrings(current.NonApplicableIDs, document.ReviewProvenance.NonApplicableIDs) {
		return fmt.Errorf("dossier: review provenance has invalid non-applicable decision IDs")
	}
	return nil
}

func equalFingerprints(left, right []DecisionFingerprint) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func decisionsForIDs(ids []string) []ReviewerDecision {
	decisions := make([]ReviewerDecision, 0, len(ids))
	for _, id := range ids {
		decisions = append(decisions, ReviewerDecision{ID: id})
	}
	return decisions
}

func decisionDigest(decision ReviewerDecision) string {
	value := cloneDecision(decision)
	sort.Strings(value.ScopeIDs)
	sort.Strings(value.TargetIDs)
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func reviewerGaps(brief Brief, supplied, applicable []ReviewerDecision, occupied map[string]bool) []Gap {
	gaps := make([]Gap, 0, len(supplied))
	for _, decision := range supplied {
		occupied[decision.ID] = true
	}
	if len(applicable) == 0 {
		id := derivedIDAvoiding("gap.review.", "missing", "", occupied)
		occupied[id] = true
		gaps = append(gaps, Gap{ID: id, Kind: "review_missing", Reference: brief.ID, Reason: "no reviewer decision exactly matches the dossier target revision, scope IDs, and target IDs"})
	}
	mismatches := make([]ReviewerDecision, 0, len(supplied))
	for _, decision := range supplied {
		if decision.TargetRevision == brief.TargetRevision && sameSet(decision.ScopeIDs, brief.ScopeIDs) && sameSet(decision.TargetIDs, brief.TargetIDs) {
			continue
		}
		mismatches = append(mismatches, decision)
	}
	sort.Slice(mismatches, func(i, j int) bool { return mismatches[i].ID < mismatches[j].ID })
	for _, decision := range mismatches {
		id := derivedIDAvoiding("gap.review.", decision.ID, ".target_mismatch", occupied)
		occupied[id] = true
		gaps = append(gaps, Gap{ID: id, Kind: "review_target_mismatch", Reference: decision.ID, Reason: "reviewer decision does not exactly match the dossier target revision, scope IDs, and target IDs"})
	}
	return gaps
}

func missingRelationGaps(links []Link, decisions []ReviewerDecision, propertyScope []string, occupied map[string]bool) []Gap {
	present := map[string]bool{}
	requirementTrace := map[string]bool{}
	for _, link := range links {
		present[link.Kind] = true
		if link.Kind == "requirement_trace" {
			requirementTrace[link.Source] = true
		}
	}
	required := requiredRelationKinds(len(decisions) > 0)
	gaps := make([]Gap, 0, len(required))
	for _, kind := range required {
		if !present[kind] {
			id := derivedIDAvoiding("gap.relation.", kind, ".missing", occupied)
			occupied[id] = true
			gaps = append(gaps, Gap{ID: id, Kind: "relation_missing", Reference: kind, Reason: "no caller-declared relation of this required kind"})
		}
	}
	requirements := append([]string{}, propertyScope...)
	sort.Strings(requirements)
	for _, requirementID := range requirements {
		if requirementTrace[requirementID] {
			continue
		}
		id := derivedIDAvoiding("gap.requirement_trace.", requirementID, ".missing", occupied)
		occupied[id] = true
		gaps = append(gaps, Gap{ID: id, Kind: "requirement_trace_missing", Reference: requirementID, Reason: "no caller-declared requirement_trace relation for this property-scope requirement"})
	}
	return gaps
}

func gapIDs(gaps []Gap) map[string]bool {
	occupied := make(map[string]bool, len(gaps))
	for _, gap := range gaps {
		occupied[gap.ID] = true
	}
	return occupied
}

func mergeGaps(groups ...[]Gap) ([]Gap, error) {
	var out []Gap
	seen := map[string]bool{}
	for _, group := range groups {
		for _, gap := range group {
			if seen[gap.ID] {
				return nil, fmt.Errorf("dossier: generated gap %q duplicates a caller or generated gap", gap.ID)
			}
			seen[gap.ID] = true
			out = append(out, gap)
		}
	}
	if len(out) > maxItems {
		return nil, fmt.Errorf("dossier: merged gaps exceeds %d items", maxItems)
	}
	return out, nil
}

func skeletonCoverage(brief Brief, intake ModelIntake, links []Link) []Coverage {
	tracesByRequirement := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, link := range links {
		if link.Kind == "requirement_trace" {
			if seen[link.Source] == nil {
				seen[link.Source] = map[string]bool{}
			}
			if seen[link.Source][link.Target] {
				continue
			}
			seen[link.Source][link.Target] = true
			tracesByRequirement[link.Source] = append(tracesByRequirement[link.Source], link.Target)
		}
	}
	coverage := make([]Coverage, 0, len(intake.PropertyScope))
	requirementIDs := append([]string{}, intake.PropertyScope...)
	sort.Strings(requirementIDs)
	occupied := make(map[string]bool, len(requirementIDs))
	for _, requirementID := range requirementIDs {
		id := derivedIDAvoiding("coverage.requirement.", requirementID, "", occupied)
		occupied[id] = true
		coverage = append(coverage, Coverage{ID: id, Kind: "requirement", Subject: requirementID, Related: append([]string{}, tracesByRequirement[requirementID]...)})
	}
	return coverage
}

func derivedID(prefix, value, suffix string) string {
	candidate := prefix + value + suffix
	if len(candidate) <= maxIDLength && idRE.MatchString(candidate) && !strings.Contains(candidate, "..") {
		return candidate
	}
	digest := sha256.Sum256([]byte(prefix + value + suffix))
	return prefix + hex.EncodeToString(digest[:]) + suffix
}

func derivedIDAvoiding(prefix, value, suffix string, occupied map[string]bool) string {
	id := derivedID(prefix, value, suffix)
	if !occupied[id] {
		return id
	}
	for nonce := 1; ; nonce++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d", prefix, value, suffix, nonce)))
		id = prefix + hex.EncodeToString(digest[:]) + suffix
		if !occupied[id] {
			return id
		}
	}
}

func requiredRelationKinds(hasReviewDecision bool) []string {
	required := []string{"trace_transition", "transition_property", "assumption_transition"}
	if hasReviewDecision {
		required = append(required, "review_target")
	}
	return required
}

func validateCompleteness(document Document) error {
	want := map[string]bool{}
	for _, requirementID := range document.Intake.PropertyScope {
		want[requirementID] = true
	}
	if len(document.Coverage) != len(want) {
		return fmt.Errorf("dossier: coverage must contain one entry for each property-scope requirement")
	}
	for _, coverage := range document.Coverage {
		if !want[coverage.Subject] {
			return fmt.Errorf("dossier: coverage subject %q is outside property scope", coverage.Subject)
		}
		delete(want, coverage.Subject)
	}
	if len(want) != 0 {
		return fmt.Errorf("dossier: coverage is missing a property-scope requirement")
	}
	for _, requirementID := range document.Intake.PropertyScope {
		if !hasRequirementTrace(document.Links, requirementID) && !hasGapKindReference(document.Gaps, "requirement_trace_missing", requirementID) {
			return fmt.Errorf("dossier: missing requirement_trace relation or gap for property-scope requirement %s", requirementID)
		}
	}
	for _, kind := range requiredRelationKinds(len(document.ReviewerDecisions) > 0) {
		if !hasLinkKind(document.Links, kind) && !hasGapKindReference(document.Gaps, "relation_missing", kind) {
			return fmt.Errorf("dossier: missing derived relation_missing gap for %s", kind)
		}
	}
	if len(document.ReviewerDecisions) == 0 && !hasGapKindReference(document.Gaps, "review_missing", "") {
		return fmt.Errorf("dossier: missing derived review_missing gap")
	}
	for _, id := range document.ReviewProvenance.NonApplicableIDs {
		if !hasGapKindReference(document.Gaps, "review_target_mismatch", id) {
			return fmt.Errorf("dossier: missing derived review_target_mismatch gap for %s", id)
		}
	}
	return nil
}

func hasRequirementTrace(links []Link, requirementID string) bool {
	for _, link := range links {
		if link.Kind == "requirement_trace" && link.Source == requirementID {
			return true
		}
	}
	return false
}

func hasLinkKind(links []Link, kind string) bool {
	for _, link := range links {
		if link.Kind == kind {
			return true
		}
	}
	return false
}

func hasGapKindReference(gaps []Gap, kind, reference string) bool {
	for _, gap := range gaps {
		if gap.Kind == kind && (reference == "" || gap.Reference == reference) {
			return true
		}
	}
	return false
}

func sameSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]bool, len(left))
	for _, value := range left {
		seen[value] = true
	}
	for _, value := range right {
		if !seen[value] {
			return false
		}
	}
	return true
}

func referencesByID(references []Reference) map[string]bool {
	out := make(map[string]bool, len(references))
	for _, reference := range references {
		out[reference.ID] = true
	}
	return out
}

func decisionsByID(decisions []ReviewerDecision) map[string]bool {
	out := make(map[string]bool, len(decisions))
	for _, decision := range decisions {
		out[decision.ID] = true
	}
	return out
}

func cloneBrief(value Brief) Brief {
	value.ScopeIDs = append([]string{}, value.ScopeIDs...)
	value.TargetIDs = append([]string{}, value.TargetIDs...)
	value.Requirements = append([]Reference{}, value.Requirements...)
	value.TraceNodes = append([]Reference{}, value.TraceNodes...)
	return value
}
func cloneIntake(value ModelIntake) ModelIntake {
	value.StateBounds = append([]Bound{}, value.StateBounds...)
	value.Atomicity = append([]string{}, value.Atomicity...)
	value.Environment = append([]string{}, value.Environment...)
	value.SchedulerFairness = append([]string{}, value.SchedulerFairness...)
	value.PropertyScope = append([]string{}, value.PropertyScope...)
	value.ModelTransitions = append([]Reference{}, value.ModelTransitions...)
	value.Properties = append([]Reference{}, value.Properties...)
	value.Assumptions = append([]Reference{}, value.Assumptions...)
	return value
}
func cloneDecision(value ReviewerDecision) ReviewerDecision {
	value.ScopeIDs = append([]string{}, value.ScopeIDs...)
	value.TargetIDs = append([]string{}, value.TargetIDs...)
	return value
}
func cloneDecisions(values []ReviewerDecision) []ReviewerDecision {
	out := make([]ReviewerDecision, len(values))
	for i, value := range values {
		out[i] = cloneDecision(value)
	}
	return out
}

func cloneReviewProvenance(value ReviewProvenance) ReviewProvenance {
	applicable := make([]DecisionFingerprint, len(value.Applicable))
	copy(applicable, value.Applicable)
	nonApplicableIDs := make([]string, len(value.NonApplicableIDs))
	copy(nonApplicableIDs, value.NonApplicableIDs)
	return ReviewProvenance{
		Applicable:       applicable,
		NonApplicableIDs: nonApplicableIDs,
	}
}
func cloneLinks(values []Link) []Link { return append([]Link{}, values...) }
func cloneCoverage(values []Coverage) []Coverage {
	out := make([]Coverage, len(values))
	for i, value := range values {
		out[i] = value
		out[i].Related = append([]string{}, value.Related...)
	}
	return out
}
func sortReferences(values []Reference) {
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
}
func sortBrief(value *Brief) {
	sort.Strings(value.ScopeIDs)
	sort.Strings(value.TargetIDs)
	sortReferences(value.Requirements)
	sortReferences(value.TraceNodes)
}
func sortIntake(value *ModelIntake) {
	sort.Strings(value.Atomicity)
	sort.Strings(value.Environment)
	sort.Strings(value.SchedulerFairness)
	sort.Strings(value.PropertyScope)
	sort.Slice(value.StateBounds, func(i, j int) bool { return value.StateBounds[i].Name < value.StateBounds[j].Name })
	sortReferences(value.ModelTransitions)
	sortReferences(value.Properties)
	sortReferences(value.Assumptions)
}
