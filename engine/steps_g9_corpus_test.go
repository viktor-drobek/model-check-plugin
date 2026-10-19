package modelcheck_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"modelcheck/trace"
	"modelcheck/trace/dossier"
	"modelcheck/trace/protocol"
	"modelcheck/trace/source"
)

func init() { stepRegistrars = append(stepRegistrars, registerG9CorpusSteps) }

func TestG9TraceCorpusFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: registerG9CorpusSteps,
		Options:             &godog.Options{Format: "pretty", Paths: []string{"../features/g9-trace-corpus.feature"}, Strict: true, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("trace corpus features failed")
	}
}

func registerG9CorpusSteps(sc *godog.ScenarioContext) {
	var artifacts map[string][]byte
	var first, second []byte
	var document *trace.Document
	sc.Step(`^the synthetic incomplete trace corpus$`, func() error {
		artifacts = make(map[string][]byte)
		for _, name := range []string{"api.json", "handler.go", "span.json"} {
			data, err := os.ReadFile("testdata/trace/end-to-end/" + name)
			if err != nil {
				return err
			}
			artifacts[name] = data
		}
		return nil
	})
	sc.Step(`^the corpus is imported twice through its adapters$`, func() error {
		var err error
		first, err = importTraceCorpus(artifacts)
		if err != nil {
			return err
		}
		second, err = importTraceCorpus(artifacts)
		if err != nil {
			return err
		}
		document, err = trace.UnmarshalJSON(first)
		return err
	})
	sc.Step(`^the corpus imports are byte reproducible$`, func() error {
		if !bytes.Equal(first, second) || len(first) == 0 {
			return fmt.Errorf("corpus imports differ or are empty")
		}
		return nil
	})
	sc.Step(`^contract syntactic and runtime evidence remain separate$`, func() error {
		levels := map[trace.EvidenceLevel]bool{}
		for _, evidence := range document.Evidence {
			switch evidence.Level {
			case trace.Contract, trace.Syntactic, trace.Runtime:
				levels[evidence.Level] = true
			default:
				return fmt.Errorf("unexpected authority %q", evidence.Level)
			}
		}
		if len(levels) != 3 {
			return fmt.Errorf("missing evidence authority: %v", levels)
		}
		for _, edge := range document.Edges {
			if edge.Kind != trace.Calls && edge.Kind != trace.DerivedFrom {
				return fmt.Errorf("inferred cross-artifact relation %q", edge.Kind)
			}
		}
		return nil
	})
	sc.Step(`^the corpus disagreement remains an explicit SARIF obligation$`, func() error {
		data, err := trace.SARIF(document)
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), "trace/corpus.contract_runtime_disagreement") || !strings.Contains(string(data), "gap.corpus.disagreement") {
			return fmt.Errorf("SARIF lost the explicit disagreement")
		}
		missing := 0
		for _, gap := range document.Gaps {
			if gap.Kind == "span.provenance_missing" {
				missing++
			}
		}
		if missing != 2 {
			return fmt.Errorf("want both missing runtime provenance gaps, got %d", missing)
		}
		return nil
	})
	sc.Step(`^the corpus dossier retains the disagreement without an implementation claim$`, func() error {
		input := corpusDossierInput(document)
		doc, err := dossier.Build(input)
		if err != nil {
			return err
		}
		if doc.Result.ImplementationConclusion != "not_assessed" || doc.Result.Status != dossier.StatusNotExecuted || doc.Result.Verdict != "" {
			return fmt.Errorf("corpus dossier invented an implementation or model conclusion")
		}
		for _, gap := range doc.Gaps {
			if gap.ID == "gap.corpus.disagreement" {
				return nil
			}
		}
		return fmt.Errorf("corpus dossier lost the unresolved disagreement")
	})
	sc.Step(`^unrelated revision and scope decisions cannot resolve the corpus review$`, func() error {
		for _, mismatch := range []string{"revision", "scope", "target"} {
			input := corpusDossierInput(document)
			decision := corpusReviewDecision(input.Brief)
			switch mismatch {
			case "revision":
				decision.TargetRevision = "sha256:" + strings.Repeat("b", 64)
			case "scope":
				decision.ScopeIDs = []string{"scope.other"}
			case "target":
				decision.TargetIDs = []string{"target.other"}
			}
			input.Decisions = []dossier.ReviewerDecision{decision}
			doc, err := dossier.Build(input)
			if err != nil {
				return err
			}
			missing, disagreement := false, false
			for _, gap := range doc.Gaps {
				missing = missing || gap.Kind == "review_missing"
				disagreement = disagreement || gap.ID == "gap.corpus.disagreement"
			}
			if len(doc.ReviewerDecisions) != 0 || !missing || !disagreement || doc.Result.ImplementationConclusion != "not_assessed" {
				return fmt.Errorf("%s mismatch incorrectly resolved corpus review", mismatch)
			}
		}
		return nil
	})
	sc.Step(`^an exact corpus review is recorded without erasing evidence or claiming conformance$`, func() error {
		input := corpusDossierInput(document)
		decision := corpusReviewDecision(input.Brief)
		input.Decisions = []dossier.ReviewerDecision{decision}
		input.Links = []dossier.Link{{ID: "link.corpus.review", Kind: "review_target", Source: decision.ID, Target: input.Brief.ID}}
		doc, err := dossier.Build(input)
		if err != nil {
			return err
		}
		if len(doc.ReviewerDecisions) != 1 || doc.ReviewerDecisions[0].ID != decision.ID || doc.Result.ImplementationConclusion != "not_assessed" {
			return fmt.Errorf("exact review missing or conformance inferred")
		}
		disagreement := false
		for _, gap := range doc.Gaps {
			if gap.Kind == "review_missing" {
				return fmt.Errorf("exact review was not applied")
			}
			disagreement = disagreement || gap.ID == "gap.corpus.disagreement"
		}
		if !disagreement {
			return fmt.Errorf("review erased original evidence obligation")
		}
		again, err := dossier.Build(input)
		if err != nil {
			return err
		}
		left, err := dossier.JSON(doc)
		if err != nil {
			return err
		}
		right, err := dossier.JSON(again)
		if err != nil {
			return err
		}
		if !bytes.Equal(left, right) {
			return fmt.Errorf("reviewed corpus is not reproducible")
		}
		return nil
	})
}

func corpusReviewDecision(brief dossier.Brief) dossier.ReviewerDecision {
	return dossier.ReviewerDecision{ID: "review.corpus", TargetRevision: brief.TargetRevision, ScopeIDs: append([]string(nil), brief.ScopeIDs...), TargetIDs: append([]string(nil), brief.TargetIDs...), Disposition: "The drop observation does not establish the Accept obligation; keep the mismatch open."}
}

func importTraceCorpus(artifacts map[string][]byte) ([]byte, error) {
	digest := func(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
	contractBytes, err := protocol.AnalyzeOpenAPI(protocol.ContractInput{
		Artifact:      protocol.Input{Path: "corpus/api.json", Content: artifacts["api.json"]},
		ContentSHA256: digest(artifacts["api.json"]),
	})
	if err != nil {
		return nil, err
	}
	contract, err := trace.UnmarshalJSON(contractBytes)
	if err != nil {
		return nil, err
	}
	syntax, err := source.AnalyzeGo(source.Input{Path: "corpus/handler.go", Content: artifacts["handler.go"]})
	if err != nil {
		return nil, err
	}
	export, err := json.Marshal(map[string]any{
		"schema": "mcd-runtime-span-export/1",
		"spans": []map[string]string{{
			"trace_id": "trace.demo", "content_sha256": digest(artifacts["span.json"]),
			"raw": string(artifacts["span.json"]), "build_revision": "", "collector_name": "", "collector_version": "",
		}},
	})
	if err != nil {
		return nil, err
	}
	runtimeBytes, err := protocol.AnalyzeRuntimeSpans(protocol.RuntimeSpansInput{
		Artifact: protocol.Input{Path: "corpus/spans.json", Content: export}, ExportSHA256: digest(export),
	})
	if err != nil {
		return nil, err
	}
	runtimeDoc, err := trace.UnmarshalJSON(runtimeBytes)
	if err != nil {
		return nil, err
	}
	merged := &trace.Document{Schema: trace.Schema, Nodes: []trace.Node{}, Edges: []trace.Edge{}, Evidence: []trace.Evidence{}, Gaps: []trace.Gap{}}
	for _, doc := range []*trace.Document{contract, syntax, runtimeDoc} {
		merged.Nodes = append(merged.Nodes, doc.Nodes...)
		merged.Edges = append(merged.Edges, doc.Edges...)
		merged.Evidence = append(merged.Evidence, doc.Evidence...)
		merged.Gaps = append(merged.Gaps, doc.Gaps...)
	}
	// This authored obligation is explicit; labels never establish a mapping.
	merged.Gaps = append(merged.Gaps, trace.Gap{
		ID: "gap.corpus.disagreement", Kind: "corpus.contract_runtime_disagreement", Reference: "corpus:accept-obligation",
		Reason:     "The declared Accept obligation and the observed drop span have no reviewed correspondence.",
		Provenance: trace.Provenance{ContentSHA256: digest(artifacts["api.json"]), Anchor: "corpus/api.json#/paths/~1accept/post", Producer: trace.Producer{Name: "synthetic-corpus", Version: "1"}},
	})
	return trace.JSON(merged)
}

func corpusDossierInput(document *trace.Document) dossier.Input {
	input := dossier.Input{
		Brief: dossier.Brief{
			ID: "brief.corpus", TargetRevision: "sha256:" + strings.Repeat("a", 64),
			ScopeIDs: []string{"scope.accept"}, TargetIDs: []string{"target.demo"},
			Requirements: []dossier.Reference{{ID: "requirement.accept", Anchor: "corpus:accept-obligation"}},
			TraceNodes:   []dossier.Reference{{ID: document.Nodes[0].ID, Anchor: document.Nodes[0].Provenance.Anchor}},
			Algorithm:    "One bounded request followed by one local handler action",
			Protocol:     "Synthetic request handling",
		},
		Intake: dossier.ModelIntake{
			ID: "intake.corpus", StateBounds: []dossier.Bound{{Name: "requests", Limit: 1}},
			Atomicity: []string{"one local action per transition"}, Environment: []string{"one request"},
			SchedulerFairness: []string{"none"}, PropertyScope: []string{"requirement.accept"},
			ModelTransitions: []dossier.Reference{{ID: "transition.accept", Anchor: "model:accept"}},
			Properties:       []dossier.Reference{{ID: "property.accept", Anchor: "model:accept-property"}},
			Assumptions:      []dossier.Reference{{ID: "assumption.single", Anchor: "intake:single-request"}},
		},
		Result: dossier.ModelResult{Status: dossier.StatusNotExecuted},
	}
	for _, gap := range document.Gaps {
		input.Gaps = append(input.Gaps, dossier.Gap{ID: gap.ID, Kind: gap.Kind, Reference: gap.Reference, Reason: gap.Reason})
	}
	return input
}
