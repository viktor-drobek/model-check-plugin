package dossier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func immutableRevision(label string) string {
	return "sha256:" + strings.Repeat("0", 63) + label[:1]
}

func TestBuildAcceptsCompleteFiniteModelIntake(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(document.ReviewerDecisions) != 1 || document.ReviewerDecisions[0].ID != "review.r1" {
		t.Fatalf("reviewer decisions = %#v, want applicable review.r1", document.ReviewerDecisions)
	}
	if !document.Result.FiniteModelOnly || document.Result.ImplementationConclusion != "not_assessed" {
		t.Fatalf("result boundary = %#v", document.Result)
	}
	if len(document.Links) == 0 || len(document.Coverage) == 0 {
		t.Fatalf("links = %#v, coverage = %#v, want explicit relations and coverage", document.Links, document.Coverage)
	}
	if len(document.Gaps) != 1 || document.Gaps[0].ID != "gap.caller.open-question" {
		t.Fatalf("gaps = %#v, want preserved caller gap", document.Gaps)
	}
}

func TestBuildRejectsMissingStateBounds(t *testing.T) {
	input := testInput()
	input.Intake.StateBounds = nil
	_, err := Build(input)
	if err == nil || !strings.Contains(err.Error(), "state_bounds") {
		t.Fatalf("Build() error = %v, want state_bounds rejection", err)
	}
}

func TestBuildRecordsMismatchedReviewerDecisionAsGap(t *testing.T) {
	input := testInput()
	input.Decisions[0].ScopeIDs = []string{"scope.sender"}
	input.Links = input.Links[:4]
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(document.ReviewerDecisions) != 0 {
		t.Fatalf("reviewer decisions = %#v, want no invented applicable decision", document.ReviewerDecisions)
	}
	if !hasGap(document.Gaps, "gap.review.review.r1.target_mismatch") {
		t.Fatalf("gaps = %#v, want explicit scope mismatch gap", document.Gaps)
	}
	if !hasGapKind(document.Gaps, "review_missing") {
		t.Fatalf("gaps = %#v, want review_missing alongside mismatch", document.Gaps)
	}
	for _, link := range document.Links {
		if link.Source == "review.r1" {
			t.Fatalf("link = %#v, mismatched reviewer decision must not be linked", link)
		}
	}
}

func TestBuildAcceptsMaximumLengthRequirementWithBoundedCoverageID(t *testing.T) {
	input := testInput()
	input.Brief.Requirements[0].ID = "r" + strings.Repeat("x", 107)
	input.Intake.PropertyScope = []string{input.Brief.Requirements[0].ID}
	input.Links[0].Source = input.Brief.Requirements[0].ID
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(document.Coverage) != 1 || len(document.Coverage[0].ID) > maxIDLength {
		t.Fatalf("coverage = %#v, want bounded generated ID", document.Coverage)
	}
}

func TestBuildBoundsTrailingOrLongMismatchGapIDs(t *testing.T) {
	input := testInput()
	input.Decisions[0].ID = "review." + strings.Repeat("x", 100) + "."
	input.Decisions[0].TargetRevision = immutableRevision("b")
	input.Links = input.Links[:4]
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(document.Gaps) == 0 {
		t.Fatal("Build() produced no gaps")
	}
	for _, gap := range document.Gaps {
		if gap.Kind == "review_target_mismatch" && len(gap.ID) > maxIDLength {
			t.Fatalf("gap = %#v, want bounded generated ID", gap)
		}
	}
}

func TestBuildDeduplicatesCoverageEndpoints(t *testing.T) {
	input := testInput()
	input.Links = append(input.Links, Link{ID: "link.req.trace.duplicate", Kind: "requirement_trace", Source: "req.delivery", Target: "trace.send"})
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := document.Coverage[0].Related; len(got) != 1 || got[0] != "trace.send" {
		t.Fatalf("coverage related = %#v, want one deduplicated trace", got)
	}
	if len(document.Links) != len(input.Links) {
		t.Fatalf("links = %d, want caller links retained", len(document.Links))
	}
}

func TestValidateRejectsDuplicateReviewerIDs(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	document.ReviewerDecisions = append(document.ReviewerDecisions, document.ReviewerDecisions[0])
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "review provenance") {
		t.Fatalf("Validate() error = %v, want strict review provenance rejection", err)
	}
}

func TestValidateRejectsCompletedDocumentMissingReviewTargetObligation(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	filtered := document.Links[:0]
	for _, link := range document.Links {
		if link.Kind != "review_target" {
			filtered = append(filtered, link)
		}
	}
	document.Links = filtered
	for name, check := range map[string]func(*Document) error{
		"Validate":  func(value *Document) error { return Validate(value) },
		"Canonical": func(value *Document) error { _, err := Canonical(value); return err },
		"JSON":      func(value *Document) error { _, err := JSON(value); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(document); err == nil || !strings.Contains(err.Error(), "review_target") {
				t.Fatalf("%s() error = %v, want missing review_target obligation", name, err)
			}
		})
	}
}

func TestValidateRejectsCompletedDocumentMissingCoverageTarget(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	document.Coverage[0].Related = nil
	for name, check := range map[string]func(*Document) error{
		"Validate":  func(value *Document) error { return Validate(value) },
		"Canonical": func(value *Document) error { _, err := Canonical(value); return err },
		"JSON":      func(value *Document) error { _, err := JSON(value); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(document); err == nil || !strings.Contains(err.Error(), "coverage") {
				t.Fatalf("%s() error = %v, want incomplete coverage rejection", name, err)
			}
		})
	}
}

func TestValidateRejectsIncompleteCompletedDocument(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	document.Links = nil
	document.Coverage = nil
	document.Gaps = nil
	document.ReviewerDecisions = nil
	if _, err := JSON(document); err == nil {
		t.Fatal("JSON() accepted incomplete completed document")
	}
}

func TestBuildAvoidsLiteralAndHashedReviewGapIDCollision(t *testing.T) {
	input := testInput()
	input.Links = input.Links[:4]
	input.Decisions = []ReviewerDecision{
		input.Decisions[0],
		{ID: "a1a5f90db5a6f9b41ecdaed028896d665a4f4cc3c1af132573f38b7135c40625", TargetRevision: immutableRevision("b"), ScopeIDs: []string{"scope.sender", "scope.receiver"}, TargetIDs: []string{"target.protocol.abp"}, Disposition: "accepted"},
	}
	input.Decisions[0].ID = "review.x0."
	input.Decisions[0].TargetRevision = immutableRevision("b")
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v, want collision-free generated IDs: %v", err, err)
	}
	if got := countGapKind(document.Gaps, "review_target_mismatch"); got != 2 {
		t.Fatalf("review-target mismatch gaps = %d, want 2 (%#v)", got, document.Gaps)
	}
	var literal, hashed Gap
	for _, gap := range document.Gaps {
		if gap.Kind != "review_target_mismatch" {
			continue
		}
		if gap.Reference == "review.x0." {
			hashed = gap
		} else if gap.Reference == "a1a5f90db5a6f9b41ecdaed028896d665a4f4cc3c1af132573f38b7135c40625" {
			literal = gap
		}
	}
	wantLiteral := "gap.review.a1a5f90db5a6f9b41ecdaed028896d665a4f4cc3c1af132573f38b7135c40625.target_mismatch"
	if literal.ID != wantLiteral {
		t.Fatalf("literal collision fixture gap = %#v, want ID %q", literal, wantLiteral)
	}
	if hashed.ID == literal.ID || hashed.ID == "gap.review.review.x0..target_mismatch" {
		t.Fatalf("derivedIDAvoiding did not resolve literal collision: hashed=%#v literal=%#v", hashed, literal)
	}
}

func TestBuildUsesOneEffectiveLinkBound(t *testing.T) {
	input := testInput()
	for i := len(input.Links); i <= maxItems; i++ {
		input.Links = append(input.Links, Link{ID: fmt.Sprintf("link.extra.%d", i), Kind: "trace_transition", Source: "trace.send", Target: "model.send"})
	}
	if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "links exceeds") {
		t.Fatalf("Build() error = %v, want effective link bound rejection", err)
	}
}

func TestBuildDocumentsCombinedCallerAndGeneratedGapBound(t *testing.T) {
	input := testInput()
	input.Links = nil
	input.Gaps = make([]Gap, maxItems)
	for i := range input.Gaps {
		input.Gaps[i] = Gap{ID: fmt.Sprintf("gap.caller.%d", i), Kind: "caller_question", Reference: "req.delivery", Reason: "open"}
	}
	if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "merged gaps") {
		t.Fatalf("Build() error = %v, want combined gap bound rejection", err)
	}
}

func TestBuildRejectsCallerGeneratedGapKinds(t *testing.T) {
	for _, kind := range []string{"requirement_trace_missing", "relation_missing", "review_missing", "review_target_mismatch"} {
		t.Run(kind, func(t *testing.T) {
			input := testInput()
			input.Gaps = append(input.Gaps, Gap{ID: "gap.caller.generated", Kind: kind, Reference: "req.delivery", Reason: "claimed"})
			if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "generated gap kind") {
				t.Fatalf("Build() error = %v, want reserved generated gap rejection", err)
			}
		})
	}
}

func TestValidateRejectsStaleGeneratedGaps(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	cases := []struct {
		name string
		gap  Gap
	}{
		{name: "requirement trace", gap: Gap{ID: "gap.requirement_trace.req.delivery.missing", Kind: "requirement_trace_missing", Reference: "req.delivery", Reason: "stale"}},
		{name: "relation", gap: Gap{ID: "gap.relation.review_target.missing", Kind: "relation_missing", Reference: "review_target", Reason: "stale"}},
		{name: "review", gap: Gap{ID: "gap.review.brief.abp.missing", Kind: "review_missing", Reference: "brief.abp", Reason: "stale"}},
		{name: "review target mismatch", gap: Gap{ID: "gap.review.review.r1.target_mismatch", Kind: "review_target_mismatch", Reference: "review.r1", Reason: "stale"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := *document
			candidate.Gaps = append(append([]Gap{}, document.Gaps...), tc.gap)
			if err := Validate(&candidate); err == nil || !strings.Contains(err.Error(), "stale generated") {
				t.Fatalf("Validate() error = %v, want stale generated gap rejection", err)
			}
		})
	}
}

func TestValidateRejectsGeneratedGapReferencesOutsideTheirDomains(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	cases := []struct {
		name string
		gap  Gap
		want string
	}{
		{name: "requirement trace", gap: Gap{ID: "gap.invalid.requirement", Kind: "requirement_trace_missing", Reference: "req.other", Reason: "regression"}, want: "requirement_trace_missing"},
		{name: "relation", gap: Gap{ID: "gap.invalid.relation", Kind: "relation_missing", Reference: "unknown_relation", Reason: "regression"}, want: "relation_missing"},
		{name: "review", gap: Gap{ID: "gap.invalid.review", Kind: "review_missing", Reference: "brief.other", Reason: "regression"}, want: "review_missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := *document
			candidate.Gaps = append(append([]Gap{}, document.Gaps...), tc.gap)
			if err := Validate(&candidate); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %s gap rejection", err, tc.want)
			}
		})
	}
}

func TestValidateRejectsWhitespaceVariantReviewTargetMismatchReference(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	document.Gaps = append(document.Gaps, Gap{
		ID:        "gap.invalid.review-target-whitespace",
		Kind:      "review_target_mismatch",
		Reference: "review.r1 ",
		Reason:    "regression",
	})
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "review_target_mismatch gap reference") {
		t.Fatalf("Validate() error = %v, want identifier validation for whitespace mismatch reference", err)
	}
}

func TestValidateRejectsNonDecisionReviewTargetMismatchReference(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	document.Gaps = append(document.Gaps, Gap{
		ID: "gap.invalid.review-target-entity", Kind: "review_target_mismatch",
		Reference: "req.delivery", Reason: "regression",
	})
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "unknown reviewer decision") {
		t.Fatalf("Validate() error = %v, want unknown reviewer decision rejection", err)
	}
}

func TestValidateRejectsDuplicateGeneratedGapObligations(t *testing.T) {
	input := testInput()
	input.Links = nil
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, gap := range document.Gaps {
		if gap.Kind != "requirement_trace_missing" {
			continue
		}
		document.Gaps = append(document.Gaps, Gap{
			ID:        "gap.duplicate.requirement-trace",
			Kind:      gap.Kind,
			Reference: gap.Reference,
			Reason:    "duplicate obligation regression",
		})
		if err := Validate(document); err == nil || !strings.Contains(err.Error(), "duplicates generated obligation") {
			t.Fatalf("Validate() error = %v, want duplicate generated obligation rejection", err)
		}
		return
	}
	t.Fatal("Build() produced no requirement_trace_missing gap to duplicate")
}

func TestJSONUsesEmptyArraysNotNull(t *testing.T) {
	input := testInput()
	input.Links = nil
	input.Gaps = nil
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	encoded, err := JSON(document)
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	text := string(encoded)
	for _, field := range []string{`"links": []`, `"related": []`} {
		if !strings.Contains(text, field) {
			t.Fatalf("JSON = %s, want non-null empty array %s", text, field)
		}
	}
}

func TestBuildRejectsNonDigestRevision(t *testing.T) {
	input := testInput()
	input.Brief.TargetRevision = "develop"
	input.Links = nil
	if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "immutable digest") {
		t.Fatalf("Build() error = %v, want immutable digest rejection", err)
	}
}

func TestBuildNotExecutedHasNoVerdict(t *testing.T) {
	input := testInput()
	input.Result = ModelResult{Status: StatusNotExecuted}
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if document.Result.Status != StatusNotExecuted || document.Result.Verdict != "" {
		t.Fatalf("result = %#v, want not-executed with no verdict", document.Result)
	}
}

func TestBuildDoesNotInventMissingRelations(t *testing.T) {
	input := testInput()
	input.Links = nil
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(document.Links) != 0 {
		t.Fatalf("links = %#v, want no fabricated links", document.Links)
	}
	for _, want := range []string{
		"gap.requirement_trace.req.delivery.missing",
		"gap.relation.trace_transition.missing",
		"gap.relation.transition_property.missing",
		"gap.relation.assumption_transition.missing",
		"gap.relation.review_target.missing",
	} {
		if !hasGap(document.Gaps, want) {
			t.Fatalf("gaps = %#v, want %q", document.Gaps, want)
		}
	}
}

func TestBuildRejectsLinkWithWrongDeclaredEndpointCategory(t *testing.T) {
	input := testInput()
	input.Links[0].Target = "model.send"
	_, err := Build(input)
	if err == nil || !strings.Contains(err.Error(), "requirement_trace") {
		t.Fatalf("Build() error = %v, want requirement_trace endpoint rejection", err)
	}
}

func TestBuildRejectsUnboundedOrSecretLikeID(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "too long", id: "a" + strings.Repeat("b", 128)},
		{name: "secret marker", id: "review.secret-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := testInput()
			input.Brief.ID = tt.id
			input.Links = nil
			_, err := Build(input)
			if err == nil || !strings.Contains(err.Error(), "brief.id") {
				t.Fatalf("Build() error = %v, want brief.id rejection", err)
			}
		})
	}
}

func TestBuildRejectsMutableTargetRevision(t *testing.T) {
	input := testInput()
	input.Brief.TargetRevision = "main"
	input.Links = nil
	_, err := Build(input)
	if err == nil || !strings.Contains(err.Error(), "target_revision") {
		t.Fatalf("Build() error = %v, want immutable target revision rejection", err)
	}
}

func TestBuildRejectsMutableReviewerTargetRevision(t *testing.T) {
	input := testInput()
	input.Decisions[0].TargetRevision = "main"
	input.Links = nil
	_, err := Build(input)
	if err == nil || !strings.Contains(err.Error(), "decisions[0].target_revision") {
		t.Fatalf("Build() error = %v, want immutable reviewer target revision rejection", err)
	}
}

func TestCoverageUsesOnlyExplicitRequirementTraceLinks(t *testing.T) {
	input := testInput()
	input.Brief.Requirements = []Reference{
		{ID: "req.delivery", Anchor: "requirements#delivery"},
		{ID: "req.order", Anchor: "requirements#order"},
	}
	input.Intake.PropertyScope = []string{"req.delivery"}
	input.Links = input.Links[:1]
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(document.Coverage) != 1 || document.Coverage[0].Subject != "req.delivery" {
		t.Fatalf("coverage = %#v, want only the explicitly scoped requirement", document.Coverage)
	}
	if len(document.Coverage[0].Related) != 1 || document.Coverage[0].Related[0] != "trace.send" {
		t.Fatalf("coverage = %#v, want only the explicit requirement-trace relation", document.Coverage)
	}
}

func TestReviewerGapsRetainMismatchesAlongsideApplicableDecision(t *testing.T) {
	input := testInput()
	input.Decisions = append(input.Decisions, ReviewerDecision{
		ID:             "review.r2",
		TargetRevision: immutableRevision("b"),
		ScopeIDs:       []string{"scope.sender", "scope.receiver"},
		TargetIDs:      []string{"target.protocol.abp"},
		Disposition:    "rejected",
	})
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !hasGap(document.Gaps, "gap.review.review.r2.target_mismatch") {
		t.Fatalf("gaps = %#v, want mismatch for non-applicable review alongside applicable review", document.Gaps)
	}
}

func TestJSONIsCanonicalAcrossInputOrder(t *testing.T) {
	first, err := Build(testInput())
	if err != nil {
		t.Fatalf("first Build() error = %v", err)
	}
	secondInput := testInput()
	secondInput.Brief.ScopeIDs = []string{"scope.receiver", "scope.sender"}
	secondInput.Intake.PropertyScope = []string{"req.delivery"}
	secondInput.Decisions[0].ScopeIDs = []string{"scope.receiver", "scope.sender"}
	second, err := Build(secondInput)
	if err != nil {
		t.Fatalf("second Build() error = %v", err)
	}
	firstJSON, err := JSON(first)
	if err != nil {
		t.Fatalf("first JSON() error = %v", err)
	}
	secondJSON, err := JSON(second)
	if err != nil {
		t.Fatalf("second JSON() error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical JSON differs\nfirst: %s\nsecond: %s", firstJSON, secondJSON)
	}
}

func TestCompletedDocumentRejectsMissingMismatchGaps(t *testing.T) {
	for _, applicable := range []bool{false, true} {
		for _, removeAll := range []bool{false, true} {
			t.Run(fmt.Sprintf("applicable=%t/all=%t", applicable, removeAll), func(t *testing.T) {
				input := testInput()
				input.Links = nil
				if !applicable {
					input.Decisions = nil
				}
				for _, id := range []string{"review.r2", "review.r3"} {
					input.Decisions = append(input.Decisions, ReviewerDecision{ID: id, TargetRevision: immutableRevision("b"), ScopeIDs: input.Brief.ScopeIDs, TargetIDs: input.Brief.TargetIDs, Disposition: "accepted"})
				}
				document, err := Build(input)
				if err != nil {
					t.Fatal(err)
				}
				filtered := document.Gaps[:0]
				for _, gap := range document.Gaps {
					if gap.Kind == "review_target_mismatch" && (removeAll || gap.Reference == "review.r2") {
						continue
					}
					filtered = append(filtered, gap)
				}
				document.Gaps = filtered
				assertDocumentRejected(t, document, "missing derived review_target_mismatch gap")
			})
		}
	}
}

func TestCompletedDocumentCannotForgeMismatchFromRemovedApplicableDecision(t *testing.T) {
	input := testInput()
	input.Links = nil
	document, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	document.ReviewerDecisions = nil
	filtered := document.Gaps[:0]
	for _, gap := range document.Gaps {
		if gap.Kind != "relation_missing" || gap.Reference != "review_target" {
			filtered = append(filtered, gap)
		}
	}
	document.Gaps = append(filtered,
		Gap{ID: "gap.forged.mismatch", Kind: "review_target_mismatch", Reference: "review.r1", Reason: "regression"},
		Gap{ID: "gap.review.missing", Kind: "review_missing", Reference: document.Brief.ID, Reason: "no applicable review"},
	)
	assertDocumentRejected(t, document, "review provenance")
}

func TestCompletedDocumentRejectsApplicableDecisionMutations(t *testing.T) {
	for _, mutate := range []struct {
		name string
		edit func(*Document)
	}{
		{name: "changed", edit: func(document *Document) { document.ReviewerDecisions[0].Disposition = "rejected" }},
		{name: "removed", edit: func(document *Document) { document.ReviewerDecisions = nil }},
		{name: "added", edit: func(document *Document) {
			document.ReviewerDecisions = append(document.ReviewerDecisions, document.ReviewerDecisions[0])
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			document, err := Build(testInput())
			if err != nil {
				t.Fatal(err)
			}
			mutate.edit(document)
			err = validateAllForms(document)
			if err == nil || (!strings.Contains(err.Error(), "review provenance") && !strings.Contains(err.Error(), "review_target endpoints") && !strings.Contains(err.Error(), "duplicates a decision")) {
				t.Fatalf("validation error = %v, want strict reviewer rejection", err)
			}
		})
	}
}

func TestMismatchProvenanceSurvivesJSONRoundTrip(t *testing.T) {
	input := testInput()
	input.Links = input.Links[:4]
	input.Decisions[0].ScopeIDs = []string{"scope.sender"}
	document, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := JSON(document)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Document
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := Validate(&decoded); err != nil {
		t.Fatalf("round-tripped Validate() error = %v", err)
	}
	for i, gap := range decoded.Gaps {
		if gap.Kind == "review_target_mismatch" {
			decoded.Gaps = append(decoded.Gaps[:i], decoded.Gaps[i+1:]...)
			break
		}
	}
	if err := Validate(&decoded); err == nil || !strings.Contains(err.Error(), "missing derived review_target_mismatch gap") {
		t.Fatalf("round-tripped Validate() error = %v, want missing generated mismatch gap", err)
	}
}

func TestApplicableProvenanceSurvivesJSONRoundTrip(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := JSON(document)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Document
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := Validate(&decoded); err != nil {
		t.Fatalf("round-tripped applicable dossier Validate() error = %v", err)
	}
}

func TestValidateRejectsUnboundedReviewProvenanceBeforeDerivation(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Document)
		want string
	}{
		{name: "oversized", edit: func(document *Document) {
			document.ReviewProvenance.NonApplicableIDs = make([]string, maxItems+1)
			for i := range document.ReviewProvenance.NonApplicableIDs {
				document.ReviewProvenance.NonApplicableIDs[i] = fmt.Sprintf("review.nonapp.%d", i)
			}
		}, want: "review provenance exceeds"},
		{name: "duplicate", edit: func(document *Document) {
			document.ReviewProvenance.NonApplicableIDs = []string{"review.nonapp.1", "review.nonapp.1"}
		}, want: "review provenance.non_applicable_ids contains duplicate"},
		{name: "malformed", edit: func(document *Document) {
			document.ReviewProvenance.NonApplicableIDs = []string{"review nonapp"}
		}, want: "review provenance"},
		{name: "combined", edit: func(document *Document) {
			document.ReviewProvenance.NonApplicableIDs = make([]string, maxItems)
			for i := range document.ReviewProvenance.NonApplicableIDs {
				document.ReviewProvenance.NonApplicableIDs[i] = fmt.Sprintf("review.nonapp.%d", i)
			}
		}, want: "review provenance population exceeds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			document, err := Build(testInput())
			if err != nil {
				t.Fatal(err)
			}
			tc.edit(document)
			if err := Validate(document); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateAdmitsReviewerDecisionFieldsBeforeProvenance(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ReviewerDecision)
		want string
	}{
		{name: "oversized scope IDs", edit: func(decision *ReviewerDecision) {
			decision.ScopeIDs = make([]string, maxItems+1)
			for i := range decision.ScopeIDs {
				decision.ScopeIDs[i] = fmt.Sprintf("scope.%d", i)
			}
		}, want: "reviewer_decisions[0].scope_ids exceeds"},
		{name: "oversized disposition", edit: func(decision *ReviewerDecision) {
			decision.Disposition = strings.Repeat("x", maxTextLength+1)
		}, want: "reviewer_decisions[0].disposition must be non-blank and at most"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document, err := Build(testInput())
			if err != nil {
				t.Fatal(err)
			}
			tc.edit(&document.ReviewerDecisions[0])
			err = Validate(document)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateRejectsExactGeneratedGapTampering(t *testing.T) {
	for _, kind := range []string{"requirement_trace_missing", "relation_missing", "review_missing"} {
		t.Run(kind, func(t *testing.T) {
			input := testInput()
			input.Links = nil
			if kind == "review_missing" {
				input.Decisions = nil
			}
			document, err := Build(input)
			if err != nil {
				t.Fatal(err)
			}
			for i := range document.Gaps {
				if document.Gaps[i].Kind == kind {
					document.Gaps[i].Reason = "tampered"
					break
				}
			}
			if err := Validate(document); err == nil || !strings.Contains(err.Error(), "does not match derived identity or reason") {
				t.Fatalf("Validate() error = %v, want exact generated-gap rejection", err)
			}
		})
	}
}

func TestValidateRejectsDerivedCoverageIDTampering(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatal(err)
	}
	document.Coverage[0].ID = "coverage.tampered"
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "does not match derived ID") {
		t.Fatalf("Validate() error = %v, want derived coverage ID rejection", err)
	}
}

func assertDocumentRejected(t *testing.T, document *Document, want string) {
	t.Helper()
	for name, check := range map[string]func(*Document) error{
		"Validate":  Validate,
		"Canonical": func(value *Document) error { _, err := Canonical(value); return err },
		"JSON":      func(value *Document) error { _, err := JSON(value); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := check(document); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s() error = %v, want %q", name, err, want)
			}
		})
	}
}

func validateAllForms(document *Document) error {
	if err := Validate(document); err != nil {
		return err
	}
	if _, err := Canonical(document); err != nil {
		return err
	}
	_, err := JSON(document)
	return err
}

func TestBuildRejectsInvalidUTF8BeforeDerivation(t *testing.T) {
	invalidValues := []struct {
		name  string
		apply func(*Input)
	}{
		{"algorithm", func(input *Input) { input.Brief.Algorithm = string([]byte{0xff}) }},
		{"disposition", func(input *Input) { input.Decisions[0].Disposition = string([]byte{0xfe}) }},
	}
	for _, tc := range invalidValues {
		t.Run(tc.name, func(t *testing.T) {
			input := testInput()
			tc.apply(&input)
			if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "invalid UTF-8") {
				t.Fatalf("Build() error = %v, want invalid UTF-8 rejection", err)
			}
		})
	}
}

func TestValidateAndJSONRejectInvalidUTF8FreeText(t *testing.T) {
	document, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	document.Brief.Algorithm = string([]byte{0xff})
	if err := Validate(document); err == nil || !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("Validate() error = %v, want invalid UTF-8 rejection", err)
	}
	if _, err := JSON(document); err == nil || !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("JSON() error = %v, want invalid UTF-8 rejection", err)
	}
}

func TestBuildRejectsDistinctInvalidUTF8Values(t *testing.T) {
	for _, value := range []string{string([]byte{0xff}), string([]byte{0xfe})} {
		input := testInput()
		input.Brief.Algorithm = value
		if _, err := Build(input); err == nil || !strings.Contains(err.Error(), "invalid UTF-8") {
			t.Fatalf("Build(%q) error = %v, want invalid UTF-8 rejection", value, err)
		}
	}
}

func TestValidUnicodeSurvivesJSONRoundTrip(t *testing.T) {
	input := testInput()
	input.Brief.Algorithm = "Протокол 🚦 — 状態"
	document, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	encoded, err := JSON(document)
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	var decoded Document
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if err := Validate(&decoded); err != nil {
		t.Fatalf("Validate() after Unicode round trip error = %v", err)
	}
	if decoded.Brief.Algorithm != input.Brief.Algorithm {
		t.Fatalf("round-tripped algorithm = %q, want %q", decoded.Brief.Algorithm, input.Brief.Algorithm)
	}
}

func testInput() Input {
	return Input{
		Brief: Brief{
			ID:             "brief.abp",
			TargetRevision: immutableRevision("a"),
			ScopeIDs:       []string{"scope.sender", "scope.receiver"},
			TargetIDs:      []string{"target.protocol.abp"},
			Requirements:   []Reference{{ID: "req.delivery", Anchor: "requirements#delivery"}},
			TraceNodes:     []Reference{{ID: "trace.send", Anchor: "ledger#trace.send"}},
			Algorithm:      "alternating-bit delivery",
			Protocol:       "data acknowledgement exchange",
		},
		Intake: ModelIntake{
			ID:                "intake.abp",
			StateBounds:       []Bound{{Name: "sequence", Limit: 2}},
			Atomicity:         []string{"send transition is atomic"},
			Environment:       []string{"loss is nondeterministic"},
			SchedulerFairness: []string{"weak fairness for enabled retransmission"},
			PropertyScope:     []string{"req.delivery"},
			ModelTransitions:  []Reference{{ID: "model.send", Anchor: "model#send"}},
			Properties:        []Reference{{ID: "property.delivery", Anchor: "properties#delivery"}},
			Assumptions:       []Reference{{ID: "assumption.loss", Anchor: "assumptions#loss"}},
		},
		Decisions: []ReviewerDecision{{
			ID:             "review.r1",
			TargetRevision: immutableRevision("a"),
			ScopeIDs:       []string{"scope.sender", "scope.receiver"},
			TargetIDs:      []string{"target.protocol.abp"},
			Disposition:    "accepted",
		}},
		Links: []Link{
			{ID: "link.req.trace", Kind: "requirement_trace", Source: "req.delivery", Target: "trace.send"},
			{ID: "link.trace.transition", Kind: "trace_transition", Source: "trace.send", Target: "model.send"},
			{ID: "link.transition.property", Kind: "transition_property", Source: "model.send", Target: "property.delivery"},
			{ID: "link.assumption.transition", Kind: "assumption_transition", Source: "assumption.loss", Target: "model.send"},
			{ID: "link.review.target", Kind: "review_target", Source: "review.r1", Target: "brief.abp"},
		},
		Gaps: []Gap{{
			ID: "gap.caller.open-question", Kind: "caller_question", Reference: "req.delivery", Reason: "loss-rate bound remains open",
		}},
		Result: ModelResult{Status: StatusVerified, Verdict: "property delivery holds"},
	}
}

func hasGap(gaps []Gap, id string) bool {
	for _, gap := range gaps {
		if gap.ID == id {
			return true
		}
	}
	return false
}

func hasGapKind(gaps []Gap, kind string) bool {
	for _, gap := range gaps {
		if gap.Kind == kind {
			return true
		}
	}
	return false
}

func countGapKind(gaps []Gap, kind string) int {
	count := 0
	for _, gap := range gaps {
		if gap.Kind == kind {
			count++
		}
	}
	return count
}
