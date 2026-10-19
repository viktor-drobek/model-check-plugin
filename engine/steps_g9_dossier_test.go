package modelcheck_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/trace/dossier"
)

type g9DossierWorld struct {
	input         dossier.Input
	result        *dossier.Document
	first         []byte
	second        []byte
	err           error
	validationErr error
}

const g9ImmutableRevision = "sha256:000000000000000000000000000000000000000000000000000000000000000a"

func init() { stepRegistrars = append(stepRegistrars, registerG9DossierSteps) }

func TestG9DossierFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: registerG9DossierSteps,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../features/g9-dossier.feature"},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run G9 dossier features")
	}
}

func registerG9DossierSteps(sc *godog.ScenarioContext) {
	w := &g9DossierWorld{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g9DossierWorld{}
		return ctx, nil
	})

	sc.Step(`^a complete conformance dossier intake$`, func() error {
		w.input = completeDossierInput()
		return nil
	})
	sc.Step(`^a complete conformance dossier intake with invalid UTF-8 free text$`, func() error {
		w.input = completeDossierInput()
		w.input.Brief.Algorithm = string([]byte{0xff})
		return nil
	})
	sc.Step(`^the completed conformance dossier gets an invalid generated gap reference of kind "([^"]*)"$`, func(kind string) error {
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		gaps := map[string]dossier.Gap{
			"requirement_trace_missing": {ID: "gap.invalid.requirement", Kind: "requirement_trace_missing", Reference: "req.unscoped", Reason: "regression"},
			"relation_missing":          {ID: "gap.invalid.relation", Kind: "relation_missing", Reference: "not_a_required_relation", Reason: "regression"},
			"review_missing":            {ID: "gap.invalid.review", Kind: "review_missing", Reference: "not-the-brief", Reason: "regression"},
			"review_target_mismatch":    {ID: "gap.invalid.review-target", Kind: "review_target_mismatch", Reference: "req.delivery", Reason: "regression"},
		}
		gap, ok := gaps[kind]
		if !ok {
			return fmt.Errorf("unknown generated gap kind %q", kind)
		}
		w.result.Gaps = append(w.result.Gaps, gap)
		return nil
	})
	sc.Step(`^a conformance dossier intake with a caller-supplied generated gap of kind "([^"]*)"$`, func(kind string) error {
		w.input = completeDossierInput()
		w.input.Gaps = append(w.input.Gaps, dossier.Gap{ID: "gap.caller.generated", Kind: kind, Reference: "req.delivery", Reason: "caller injection regression"})
		return nil
	})
	sc.Step(`^a complete conformance dossier with a stale generated gap of kind "([^"]*)"$`, func(kind string) error {
		w.result, w.err = dossier.Build(completeDossierInput())
		if w.err != nil {
			return w.err
		}
		gaps := map[string]dossier.Gap{
			"requirement_trace_missing": {ID: "gap.stale.requirement", Kind: "requirement_trace_missing", Reference: "req.delivery", Reason: "regression"},
			"relation_missing":          {ID: "gap.stale.relation", Kind: "relation_missing", Reference: "review_target", Reason: "regression"},
			"review_missing":            {ID: "gap.stale.review", Kind: "review_missing", Reference: "brief.abp", Reason: "regression"},
			"review_target_mismatch":    {ID: "gap.stale.review-target", Kind: "review_target_mismatch", Reference: "review.r1", Reason: "regression"},
		}
		gap, ok := gaps[kind]
		if !ok {
			return fmt.Errorf("unknown generated gap kind %q", kind)
		}
		w.result.Gaps = append(w.result.Gaps, gap)
		return nil
	})
	sc.Step(`^an underspecified conformance dossier intake without state bounds$`, func() error {
		w.input = completeDossierInput()
		w.input.Intake.StateBounds = nil
		return nil
	})
	sc.Step(`^a complete conformance dossier intake with a reviewer decision for another revision$`, func() error {
		w.input = completeDossierInput()
		w.input.Decisions[0].TargetRevision = "sha256:000000000000000000000000000000000000000000000000000000000000000b"
		w.input.Links = w.input.Links[:4]
		return nil
	})
	sc.Step(`^a complete conformance dossier intake with status "([^"]*)"$`, func(status string) error {
		w.input = completeDossierInput()
		w.input.Result.Status = dossier.Status(status)
		w.input.Result.Verdict = ""
		return nil
	})
	sc.Step(`^a complete conformance dossier intake without declared links$`, func() error {
		w.input = completeDossierInput()
		w.input.Links = nil
		return nil
	})
	sc.Step(`^a complete conformance dossier intake with a maximum-length requirement ID$`, func() error {
		w.input = completeDossierInput()
		id := "r" + strings.Repeat("x", 107)
		w.input.Brief.Requirements[0].ID = id
		w.input.Intake.PropertyScope = []string{id}
		w.input.Links[0].Source = id
		return nil
	})
	sc.Step(`^a complete conformance dossier intake with duplicate requirement-trace endpoints$`, func() error {
		w.input = completeDossierInput()
		w.input.Links = append(w.input.Links, dossier.Link{ID: "link.req.trace.duplicate", Kind: "requirement_trace", Source: "req.delivery", Target: "trace.send"})
		return nil
	})
	sc.Step(`^a conformance dossier intake with colliding derived review-gap identifiers$`, func() error {
		w.input = completeDossierInput()
		w.input.Links = w.input.Links[:4]
		w.input.Decisions = []dossier.ReviewerDecision{
			w.input.Decisions[0],
			{ID: "a1a5f90db5a6f9b41ecdaed028896d665a4f4cc3c1af132573f38b7135c40625", TargetRevision: "sha256:000000000000000000000000000000000000000000000000000000000000000b", ScopeIDs: []string{"scope.sender", "scope.receiver"}, TargetIDs: []string{"target.protocol.abp"}, Disposition: "accepted"},
		}
		w.input.Decisions[0].ID = "review.x0."
		w.input.Decisions[0].TargetRevision = "sha256:000000000000000000000000000000000000000000000000000000000000000b"
		return nil
	})
	sc.Step(`^the conformance dossier is built$`, func() error {
		w.result, w.err = dossier.Build(w.input)
		return nil
	})
	sc.Step(`^the conformance dossier is built twice$`, func() error {
		w.result, w.err = dossier.Build(w.input)
		if w.err != nil {
			return w.err
		}
		w.first, w.err = dossier.JSON(w.result)
		if w.err != nil {
			return w.err
		}
		second, err := dossier.Build(w.input)
		if err != nil {
			return err
		}
		w.second, w.err = dossier.JSON(second)
		return w.err
	})
	sc.Step(`^the conformance dossier is built twice with reversed reviewer decision order$`, func() error {
		first, err := dossier.Build(w.input)
		if err != nil {
			return err
		}
		w.first, w.err = dossier.JSON(first)
		if w.err != nil {
			return w.err
		}
		for left, right := 0, len(w.input.Decisions)-1; left < right; left, right = left+1, right-1 {
			w.input.Decisions[left], w.input.Decisions[right] = w.input.Decisions[right], w.input.Decisions[left]
		}
		second, err := dossier.Build(w.input)
		if err != nil {
			return err
		}
		w.second, w.err = dossier.JSON(second)
		return w.err
	})
	sc.Step(`^a conformance dossier intake with colliding derived coverage identifiers$`, func() error {
		w.input = completeDossierInput()
		longID := "r" + strings.Repeat("x", 107) + "8"
		hashedID := "b736ee5d1143384a2352460f8785e19158ff07daa0b7d1e04559931a9a4611d8"
		w.input.Brief.Requirements = []dossier.Reference{
			{ID: longID, Anchor: "requirements#long"},
			{ID: hashedID, Anchor: "requirements#hashed"},
		}
		w.input.Intake.PropertyScope = []string{longID, hashedID}
		w.input.Links[0].Source = longID
		w.input.Links = append(w.input.Links, dossier.Link{ID: "link.req.trace.hashed", Kind: "requirement_trace", Source: hashedID, Target: "trace.send"})
		return nil
	})
	sc.Step(`^a conformance dossier intake with a caller gap colliding with a generated relation gap$`, func() error {
		w.input = completeDossierInput()
		w.input.Links = w.input.Links[:4]
		w.input.Gaps = append(w.input.Gaps, dossier.Gap{ID: "gap.relation.review_target.missing", Kind: "caller_question", Reference: "review.r1", Reason: "caller-owned collision fixture"})
		return nil
	})
	sc.Step(`^a conformance dossier intake with one scoped requirement missing a trace relation$`, func() error {
		w.input = completeDossierInput()
		w.input.Brief.Requirements = append(w.input.Brief.Requirements, dossier.Reference{ID: "req.order", Anchor: "requirements#order"})
		w.input.Intake.PropertyScope = []string{"req.delivery", "req.order"}
		return nil
	})
	sc.Step(`^the conformance dossier is accepted$`, func() error {
		if w.err != nil {
			return fmt.Errorf("conformance dossier was rejected: %w", w.err)
		}
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		return nil
	})
	sc.Step(`^the conformance dossier is rejected with "([^"]*)"$`, func(want string) error {
		if w.err == nil {
			return fmt.Errorf("conformance dossier was accepted")
		}
		if !strings.Contains(w.err.Error(), want) {
			return fmt.Errorf("rejection %q does not contain %q", w.err, want)
		}
		return nil
	})
	sc.Step(`^the dossier contains the applicable reviewer decision "([^"]*)"$`, func(id string) error {
		for _, decision := range w.result.ReviewerDecisions {
			if decision.ID == id {
				return nil
			}
		}
		return fmt.Errorf("dossier contains no applicable reviewer decision %q", id)
	})
	sc.Step(`^the dossier does not contain the reviewer decision "([^"]*)"$`, func(id string) error {
		for _, decision := range w.result.ReviewerDecisions {
			if decision.ID == id {
				return fmt.Errorf("dossier incorrectly contains reviewer decision %q", id)
			}
		}
		return nil
	})
	sc.Step(`^the dossier names the gap "([^"]*)"$`, func(id string) error {
		for _, gap := range w.result.Gaps {
			if gap.ID == id {
				return nil
			}
		}
		return fmt.Errorf("dossier does not name gap %q", id)
	})
	sc.Step(`^the dossier names a gap of kind "([^"]*)"$`, func(kind string) error {
		for _, gap := range w.result.Gaps {
			if gap.Kind == kind {
				return nil
			}
		}
		return fmt.Errorf("dossier does not contain gap kind %q", kind)
	})
	sc.Step(`^the dossier contains two review-target mismatch gaps$`, func() error {
		count := 0
		for _, gap := range w.result.Gaps {
			if gap.Kind == "review_target_mismatch" {
				count++
			}
		}
		if count != 2 {
			return fmt.Errorf("review-target mismatch gaps = %d, want 2", count)
		}
		return nil
	})
	sc.Step(`^all generated dossier identifiers are bounded$`, func() error {
		for _, coverage := range w.result.Coverage {
			if len(coverage.ID) > 128 {
				return fmt.Errorf("coverage ID too long: %q", coverage.ID)
			}
		}
		for _, gap := range w.result.Gaps {
			if len(gap.ID) > 128 {
				return fmt.Errorf("gap ID too long: %q", gap.ID)
			}
		}
		return nil
	})
	sc.Step(`^requirement coverage has one related trace node$`, func() error {
		if len(w.result.Coverage) != 1 || len(w.result.Coverage[0].Related) != 1 {
			return fmt.Errorf("coverage = %#v", w.result.Coverage)
		}
		return nil
	})
	sc.Step(`^all coverage identifiers are unique$`, func() error {
		seen := map[string]bool{}
		for _, coverage := range w.result.Coverage {
			if seen[coverage.ID] {
				return fmt.Errorf("duplicate coverage identifier %q", coverage.ID)
			}
			seen[coverage.ID] = true
		}
		return nil
	})
	sc.Step(`^generated relation gaps avoid caller gap identifiers$`, func() error {
		if len(w.first) == 0 || len(w.second) == 0 || string(w.first) != string(w.second) {
			return fmt.Errorf("canonical dossier JSON differs between builds")
		}
		for _, gap := range w.result.Gaps {
			if gap.Kind == "relation_missing" && gap.ID == "gap.relation.review_target.missing" {
				return fmt.Errorf("generated relation gap reused caller gap ID %q", gap.ID)
			}
		}
		return nil
	})
	sc.Step(`^the dossier names a requirement-trace gap for "([^"]*)"$`, func(requirementID string) error {
		for _, gap := range w.result.Gaps {
			if gap.Kind == "requirement_trace_missing" && gap.Reference == requirementID {
				return nil
			}
		}
		return fmt.Errorf("dossier does not contain requirement-trace gap for %q", requirementID)
	})
	sc.Step(`^the dossier links requirements, trace nodes, transitions, properties, and assumptions$`, func() error {
		kinds := map[string]bool{}
		for _, link := range w.result.Links {
			kinds[link.Kind] = true
		}
		for _, want := range []string{"requirement_trace", "trace_transition", "transition_property", "assumption_transition"} {
			if !kinds[want] {
				return fmt.Errorf("dossier lacks %s link", want)
			}
		}
		return nil
	})
	sc.Step(`^the dossier states that a model result is finite-model-only$`, func() error {
		if !w.result.Result.FiniteModelOnly || w.result.Result.ImplementationConclusion != "not_assessed" {
			return fmt.Errorf("dossier result escapes finite-model boundary: %#v", w.result.Result)
		}
		return nil
	})
	sc.Step(`^the dossier has status "([^"]*)" without a model verdict$`, func(status string) error {
		if string(w.result.Result.Status) != status || w.result.Result.Verdict != "" {
			return fmt.Errorf("dossier result = %#v, want status %q with no verdict", w.result.Result, status)
		}
		return nil
	})
	sc.Step(`^the dossier has no fabricated relation links$`, func() error {
		if len(w.result.Links) != 0 {
			return fmt.Errorf("dossier fabricated links: %#v", w.result.Links)
		}
		return nil
	})
	sc.Step(`^the completed dossier loses its review-target link$`, func() error {
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		filtered := w.result.Links[:0]
		for _, link := range w.result.Links {
			if link.Kind != "review_target" {
				filtered = append(filtered, link)
			}
		}
		w.result.Links = filtered
		return nil
	})
	sc.Step(`^the completed dossier loses its coverage target$`, func() error {
		if w.result == nil || len(w.result.Coverage) == 0 {
			return fmt.Errorf("conformance dossier has no coverage to modify")
		}
		w.result.Coverage[0].Related = nil
		return nil
	})
	sc.Step(`^the completed dossier loses its reviewer mismatch gaps$`, func() error {
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		filtered := w.result.Gaps[:0]
		removed := 0
		for _, gap := range w.result.Gaps {
			if gap.Kind == "review_target_mismatch" {
				removed++
				continue
			}
			filtered = append(filtered, gap)
		}
		w.result.Gaps = filtered
		if removed == 0 {
			return fmt.Errorf("conformance dossier had no mismatch gaps")
		}
		return nil
	})
	sc.Step(`^the completed dossier replaces its applicable decision with a mismatch gap$`, func() error {
		if w.result == nil || len(w.result.ReviewerDecisions) != 1 {
			return fmt.Errorf("conformance dossier must have one applicable decision")
		}
		id := w.result.ReviewerDecisions[0].ID
		w.result.ReviewerDecisions = nil
		filtered := w.result.Gaps[:0]
		for _, gap := range w.result.Gaps {
			if gap.Kind != "relation_missing" || gap.Reference != "review_target" {
				filtered = append(filtered, gap)
			}
		}
		w.result.Gaps = append(filtered,
			dossier.Gap{ID: "gap.forged.mismatch", Kind: "review_target_mismatch", Reference: id, Reason: "regression"},
			dossier.Gap{ID: "gap.review.missing", Kind: "review_missing", Reference: w.result.Brief.ID, Reason: "no applicable review"},
		)
		return nil
	})
	sc.Step(`^the completed dossier mutates its applicable reviewer decision$`, func() error {
		if w.result == nil || len(w.result.ReviewerDecisions) != 1 {
			return fmt.Errorf("conformance dossier must have one applicable decision")
		}
		w.result.ReviewerDecisions[0].Disposition = "rejected"
		return nil
	})
	sc.Step(`^the completed dossier has oversized non-applicable provenance$`, func() error {
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		w.result.ReviewProvenance.NonApplicableIDs = make([]string, 129)
		for i := range w.result.ReviewProvenance.NonApplicableIDs {
			w.result.ReviewProvenance.NonApplicableIDs[i] = fmt.Sprintf("review.nonapp.%d", i)
		}
		return nil
	})
	sc.Step(`^the completed dossier has duplicate non-applicable provenance$`, func() error {
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		w.result.ReviewProvenance.NonApplicableIDs = []string{"review.nonapp.1", "review.nonapp.1"}
		return nil
	})
	sc.Step(`^the completed dossier has malformed non-applicable provenance$`, func() error {
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		w.result.ReviewProvenance.NonApplicableIDs = []string{"review nonapp"}
		return nil
	})
	sc.Step(`^the completed dossier has combined reviewer provenance populations over the cap$`, func() error {
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		w.result.ReviewProvenance.NonApplicableIDs = make([]string, 128)
		for i := range w.result.ReviewProvenance.NonApplicableIDs {
			w.result.ReviewProvenance.NonApplicableIDs[i] = fmt.Sprintf("review.nonapp.%d", i)
		}
		return nil
	})
	sc.Step(`^the completed dossier has an oversized reviewer decision field$`, func() error {
		if w.result == nil || len(w.result.ReviewerDecisions) == 0 {
			return fmt.Errorf("conformance dossier must have an applicable reviewer decision")
		}
		w.result.ReviewerDecisions[0].ScopeIDs = make([]string, 129)
		for i := range w.result.ReviewerDecisions[0].ScopeIDs {
			w.result.ReviewerDecisions[0].ScopeIDs[i] = fmt.Sprintf("scope.%d", i)
		}
		return nil
	})
	sc.Step(`^the completed dossier loses the generated gap of kind "([^"]*)"$`, func(kind string) error {
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		filtered := w.result.Gaps[:0]
		removed := false
		for _, gap := range w.result.Gaps {
			if !removed && gap.Kind == kind {
				removed = true
				continue
			}
			filtered = append(filtered, gap)
		}
		if !removed {
			return fmt.Errorf("conformance dossier had no generated gap of kind %q", kind)
		}
		w.result.Gaps = filtered
		return nil
	})
	sc.Step(`^validating the dossier is rejected with "([^"]*)"$`, func(want string) error {
		if w.result == nil {
			return fmt.Errorf("conformance dossier was not built")
		}
		w.validationErr = dossier.Validate(w.result)
		if w.validationErr == nil {
			return fmt.Errorf("dossier validation was accepted")
		}
		if !strings.Contains(w.validationErr.Error(), want) {
			return fmt.Errorf("validation rejection %q does not contain %q", w.validationErr, want)
		}
		return nil
	})
	sc.Step(`^the canonical conformance dossier JSON is identical$`, func() error {
		if len(w.first) == 0 || len(w.second) == 0 {
			return fmt.Errorf("canonical dossier JSON is empty")
		}
		if string(w.first) != string(w.second) {
			return fmt.Errorf("canonical dossier JSON differs between builds\nfirst: %s\nsecond: %s", w.first, w.second)
		}
		return nil
	})
}

func completeDossierInput() dossier.Input {
	return dossier.Input{
		Brief: dossier.Brief{
			ID:             "brief.abp",
			TargetRevision: g9ImmutableRevision,
			ScopeIDs:       []string{"scope.sender", "scope.receiver"},
			TargetIDs:      []string{"target.protocol.abp"},
			Requirements:   []dossier.Reference{{ID: "req.delivery", Anchor: "requirements#delivery"}},
			TraceNodes:     []dossier.Reference{{ID: "trace.send", Anchor: "ledger#trace.send"}},
			Algorithm:      "alternating-bit delivery",
			Protocol:       "data and acknowledgement exchange",
		},
		Intake: dossier.ModelIntake{
			ID:                "intake.abp",
			StateBounds:       []dossier.Bound{{Name: "sequence", Limit: 2}},
			Atomicity:         []string{"send transition is atomic"},
			Environment:       []string{"loss is nondeterministic"},
			SchedulerFairness: []string{"weak fairness for enabled retransmission"},
			PropertyScope:     []string{"req.delivery"},
			ModelTransitions:  []dossier.Reference{{ID: "model.send", Anchor: "model#send"}},
			Properties:        []dossier.Reference{{ID: "property.delivery", Anchor: "properties#delivery"}},
			Assumptions:       []dossier.Reference{{ID: "assumption.loss", Anchor: "assumptions#loss"}},
		},
		Decisions: []dossier.ReviewerDecision{{
			ID:             "review.r1",
			TargetRevision: g9ImmutableRevision,
			ScopeIDs:       []string{"scope.receiver", "scope.sender"},
			TargetIDs:      []string{"target.protocol.abp"},
			Disposition:    "accepted",
		}},
		Links: []dossier.Link{
			{ID: "link.req.trace", Kind: "requirement_trace", Source: "req.delivery", Target: "trace.send"},
			{ID: "link.trace.transition", Kind: "trace_transition", Source: "trace.send", Target: "model.send"},
			{ID: "link.transition.property", Kind: "transition_property", Source: "model.send", Target: "property.delivery"},
			{ID: "link.assumption.transition", Kind: "assumption_transition", Source: "assumption.loss", Target: "model.send"},
			{ID: "link.review.target", Kind: "review_target", Source: "review.r1", Target: "brief.abp"},
		},
		Gaps: []dossier.Gap{{
			ID: "gap.caller.open-question", Kind: "caller_question", Reference: "req.delivery", Reason: "loss-rate bound remains open",
		}},
		Result: dossier.ModelResult{Status: dossier.StatusVerified, Verdict: "property.delivery holds"},
	}
}
