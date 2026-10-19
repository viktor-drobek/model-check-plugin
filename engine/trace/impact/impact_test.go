package impact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func completeInput() Input {
	return Input{
		Artifacts: []Artifact{
			{ID: "source", Kind: Source, Revision: "source-r1"},
			{ID: "model", Kind: Model, Revision: "model-r1"},
			{ID: "property", Kind: Property, Revision: "property-r1"},
			{ID: "environment", Kind: ModelledEnvironment, Revision: "environment-r1"},
			{ID: "adapter", Kind: Adapter, Revision: "adapter-r1", ContractVersion: "contract-r1"},
			{ID: "checker", Kind: Checker, Revision: "checker-r1", Toolchain: "go1.26.9"},
		},
		Evidence: []Evidence{{ID: "evidence", ContentSHA256: strings.Repeat("a", 64), Location: "src/a file.go:12", Dependencies: []Dependency{
			{ArtifactID: "source", Revision: "source-r1"},
			{ArtifactID: "model", Revision: "model-r1"},
			{ArtifactID: "property", Revision: "property-r1"},
			{ArtifactID: "environment", Revision: "environment-r1"},
			{ArtifactID: "adapter", Revision: "adapter-r1", ContractVersion: "contract-r1"},
			{ArtifactID: "checker", Revision: "checker-r1", Toolchain: "go1.26.9"},
		}}},
		Dossiers: []Dossier{{
			ID: "dossier", TargetRevision: "source-r1", ScopeIDs: []string{"source"}, EvidenceIDs: []string{"evidence"},
			Decision: ReviewerDecision{ID: "review", TargetRevision: "source-r1", ScopeIDs: []string{"source"}, Workflow: "two-person review", Rationale: "exact mapping reviewed", RenewOn: []RenewalRule{RenewOnDependencyChange}},
		}},
	}
}

func TestAnalyzeInvalidatesOnlyExactChangedDependencies(t *testing.T) {
	input := completeInput()
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("unchanged report = %#v, want no findings", report.Findings)
	}

	input.Changes = []Change{{ArtifactID: "source", Revision: "source-r2"}}
	report, err = Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "stale-evidence", "evidence") || !has(report, "review-renewal", "dossier") {
		t.Fatalf("changed report = %#v, want evidence invalidation and renewal", report.Findings)
	}
}

func TestAnalyzeInvalidatesAdapterContractAndCheckerToolchain(t *testing.T) {
	for _, change := range []Change{
		{ArtifactID: "adapter", Revision: "adapter-r1", ContractVersion: "contract-r2"},
		{ArtifactID: "checker", Revision: "checker-r1", Toolchain: "go1.27.0"},
	} {
		input := completeInput()
		input.Changes = []Change{change}
		report, err := Analyze(input)
		if err != nil {
			t.Fatal(err)
		}
		if !has(report, "stale-evidence", "evidence") {
			t.Fatalf("change %#v did not invalidate evidence: %#v", change, report.Findings)
		}
	}
}

func TestAnalyzeInvalidatesChangedEvidenceHash(t *testing.T) {
	input := completeInput()
	input.EvidenceChanges = []EvidenceChange{{EvidenceID: "evidence", ContentSHA256: strings.Repeat("b", 64)}}
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "stale-evidence", "evidence") || !has(report, "review-renewal", "dossier") {
		t.Fatalf("hash change did not invalidate exact evidence and dossier: %#v", report.Findings)
	}
}

func verdictInput() Input {
	input := completeInput()
	input.Artifacts = append(input.Artifacts, Artifact{ID: "registry.protocol", Kind: Model, Revision: "registry-r1"})
	input.RunRegistryRevision = "registry-r1"
	input.Verdicts = []ModelVerdict{{
		ID: "verdict.protocol", RunID: "run-1", RegistryRevision: "registry-r1",
		EngineBuildRevision: "checker-r1", Toolchain: "go1.26.9",
		CheckerArtifactID: "checker", RegistryArtifactID: "registry.protocol",
		EvidenceIDs: []string{"evidence"}, MappingApproved: true,
	}}
	return input
}

func TestAnalyzeInvalidatesModelVerdictProvenance(t *testing.T) {
	for _, change := range []Change{
		{ArtifactID: "checker", Revision: "checker-r2", Toolchain: "go1.26.9"},
		{ArtifactID: "checker", Revision: "checker-r1", Toolchain: "go1.27.0"},
	} {
		input := verdictInput()
		input.Changes = []Change{change}
		report, err := Analyze(input)
		if err != nil {
			t.Fatal(err)
		}
		if !has(report, "stale-verdict", "verdict.protocol") || !has(report, "verdict-unpromoted", "verdict.protocol") {
			t.Fatalf("change %#v report = %#v", change, report.Findings)
		}
	}
}

func TestAnalyzeInvalidatesModelVerdictWhenEvidenceChanges(t *testing.T) {
	input := verdictInput()
	input.EvidenceChanges = []EvidenceChange{{EvidenceID: "evidence", ContentSHA256: strings.Repeat("b", 64)}}
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "stale-verdict", "verdict.protocol") || !has(report, "verdict-unpromoted", "verdict.protocol") {
		t.Fatalf("report = %#v", report.Findings)
	}
}

func TestAnalyzeRejectsModelVerdictWithoutBuildProvenance(t *testing.T) {
	input := verdictInput()
	input.Verdicts[0].EngineBuildRevision = ""
	if _, err := Analyze(input); err == nil || !strings.Contains(err.Error(), "engine") {
		t.Fatalf("error = %v, want engine provenance rejection", err)
	}
}

func TestAnalyzeNeverPromotesUnapprovedModelVerdict(t *testing.T) {
	input := verdictInput()
	input.Verdicts[0].MappingApproved = false
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "stale-verdict", "verdict.protocol") || !has(report, "verdict-unpromoted", "verdict.protocol") {
		t.Fatalf("report = %#v", report.Findings)
	}
}

func TestAnalyzeDoesNotPromoteVerdictWithScopeMismatchedReview(t *testing.T) {
	input := verdictInput()
	input.Dossiers[0].Decision.ScopeIDs = []string{"different"}
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "stale-verdict", "verdict.protocol") || !has(report, "verdict-unpromoted", "verdict.protocol") {
		t.Fatalf("report = %#v, want stale and unpromoted verdict", report.Findings)
	}
}

func TestAnalyzeDoesNotPromoteVerdictWithWhitespaceReviewerID(t *testing.T) {
	input := verdictInput()
	input.Dossiers[0].Decision.ID = " \t"
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "stale-verdict", "verdict.protocol") || !has(report, "verdict-unpromoted", "verdict.protocol") {
		t.Fatalf("report = %#v, want stale and unpromoted verdict", report.Findings)
	}
}

func TestAnalyzeRequiresReviewCoverageForEveryVerdictEvidence(t *testing.T) {
	input := verdictInput()
	input.Evidence = append(input.Evidence, Evidence{ID: "evidence.other", ContentSHA256: strings.Repeat("c", 64), Location: "src/other.go:4", Dependencies: input.Evidence[0].Dependencies})
	input.Verdicts[0].EvidenceIDs = append(input.Verdicts[0].EvidenceIDs, "evidence.other")
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "stale-verdict", "verdict.protocol") {
		t.Fatalf("report = %#v, want stale verdict", report.Findings)
	}
	if !has(report, "verdict-unpromoted", "verdict.protocol") {
		t.Fatalf("report = %#v, want unpromoted verdict", report.Findings)
	}
}

func TestAnalyzeDoesNotPromoteVerdictWithIncompleteEvidenceDependencies(t *testing.T) {
	input := verdictInput()
	input.Evidence[0].Dependencies = input.Evidence[0].Dependencies[:5]
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "missing-dependency", "evidence") || !has(report, "stale-verdict", "verdict.protocol") || !has(report, "verdict-unpromoted", "verdict.protocol") {
		t.Fatalf("report = %#v, want missing dependency, stale and unpromoted verdict", report.Findings)
	}
}

func TestAnalyzeValidatesAllRenewalRulesBeforeApplicability(t *testing.T) {
	for _, edit := range []func(*Input){
		func(input *Input) { input.Dossiers[0].Decision.RenewOn = []RenewalRule{"unsupported"} },
		func(input *Input) {
			input.Dossiers[0].Decision.RenewOn = []RenewalRule{RenewOnDependencyChange, "unsupported"}
			input.Changes = []Change{{ArtifactID: "source", Revision: "source-r2"}}
		},
		func(input *Input) {
			input.Dossiers[0].Decision.RenewOn = []RenewalRule{"unsupported"}
			input.Dossiers[0].Decision.ScopeIDs = []string{"different"}
		},
	} {
		input := completeInput()
		edit(&input)
		if _, err := Analyze(input); err == nil || !strings.Contains(err.Error(), "unsupported renewal rule") {
			t.Fatalf("error = %v, want unsupported renewal rule", err)
		}
	}
}

func TestAnalyzeRejectsRepeatedEvidenceChanges(t *testing.T) {
	for _, hashes := range [][2]string{{"a", "b"}, {"b", "a"}, {"a", "a"}, {"b", "b"}} {
		t.Run(strings.Join(hashes[:], "-"), func(t *testing.T) {
			input := completeInput()
			for _, hash := range hashes {
				input.EvidenceChanges = append(input.EvidenceChanges, EvidenceChange{EvidenceID: "evidence", ContentSHA256: strings.Repeat(hash, 64)})
			}
			if _, err := Analyze(input); err == nil {
				t.Fatal("Analyze accepted repeated evidence changes")
			}
		})
	}
}

func TestAnalyzeFailsClosedOnAmbiguousOrInexactMappings(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"unknown dependency", func(input *Input) { input.Evidence[0].Dependencies[0].ArtifactID = "source-label" }},
		{"inexact revision", func(input *Input) { input.Evidence[0].Dependencies[0].Revision = "same-label-different-revision" }},
		{"duplicate dependency kind", func(input *Input) {
			input.Evidence[0].Dependencies = append(input.Evidence[0].Dependencies, input.Evidence[0].Dependencies[0])
		}},
		{"ambiguous changes", func(input *Input) {
			input.Changes = []Change{{ArtifactID: "source", Revision: "source-r2"}, {ArtifactID: "source", Revision: "source-r3"}}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := completeInput()
			test.edit(&input)
			if _, err := Analyze(input); err == nil {
				t.Fatal("Analyze() succeeded, want fail-closed error")
			}
		})
	}
}

func TestAnalyzeReportsMissingAndOrphanEvidenceDeterministically(t *testing.T) {
	input := completeInput()
	input.Evidence = append(input.Evidence, Evidence{ID: "orphan", ContentSHA256: strings.Repeat("c", 64), Location: "src/orphan.go:3", Dependencies: input.Evidence[0].Dependencies[:5]})
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "missing-dependency", "orphan") || !has(report, "orphan-evidence", "orphan") {
		t.Fatalf("report = %#v, want missing and orphan findings", report.Findings)
	}
	first, err := SARIF(report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SARIF(report)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || !strings.Contains(string(first), `"ruleId": "trace-impact/orphan-evidence"`) {
		t.Fatalf("SARIF is not deterministic or lacks stable rule:\n%s", first)
	}
}

func TestSARIFPreservesRootFileLine(t *testing.T) {
	input := completeInput()
	input.Evidence = append(input.Evidence, Evidence{ID: "orphan", ContentSHA256: strings.Repeat("c", 64), Location: "main.go:12", Dependencies: input.Evidence[0].Dependencies})
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	data, err := SARIF(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"uri": "main.go"`) || !strings.Contains(string(data), `"startLine": 12`) {
		t.Fatalf("SARIF lacks root-file region: %s", data)
	}
}

func TestReviewerDecisionApplicabilityAndRenewalAreIndependent(t *testing.T) {
	input := completeInput()
	input.Dossiers[0].Decision.ScopeIDs = []string{"different"}
	report, err := Analyze(input)
	if err != nil {
		t.Fatal(err)
	}
	if !has(report, "review-renewal", "dossier") {
		t.Fatalf("report = %#v, want renewal for scope mismatch", report.Findings)
	}
	input = completeInput()
	input.Dossiers[0].Decision.Workflow = ""
	if _, err := Analyze(input); err == nil {
		t.Fatal("Analyze() accepted a decision without review workflow")
	}
}

func has(report Report, kind, subject string) bool {
	for _, item := range report.Findings {
		if item.Kind == kind && item.Subject == subject {
			return true
		}
	}
	return false
}

func TestParseDiffPreservesMaliciousPathBytesWithoutWhitespaceSplitting(t *testing.T) {
	path := "dir/space name\tand\nnewline.go"
	data := []byte(":100644 100644 " + strings.Repeat("a", 40) + " " + strings.Repeat("b", 40) + " M\x00" + path + "\x00")
	changes, err := ParseDiff(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != path || changes[0].Status != 'M' {
		t.Fatalf("changes = %#v, want exact malicious path", changes)
	}
}

func TestParseDiffRejectsRenameAndSubmodule(t *testing.T) {
	for _, data := range [][]byte{
		[]byte(":100644 100644 " + strings.Repeat("a", 40) + " " + strings.Repeat("b", 40) + " R100\x00old name\x00new name\x00"),
		[]byte(":160000 160000 " + strings.Repeat("a", 40) + " " + strings.Repeat("b", 40) + " M\x00module\x00"),
	} {
		if _, err := ParseDiff(data); err == nil {
			t.Fatal("ParseDiff() accepted unsupported metadata")
		}
	}
}

func TestChangesFromGitTreatsRenameAsDeletionAndAddition(t *testing.T) {
	changes, err := ChangesFromGit([]GitChange{
		{Status: 'D', Path: "old.go"},
		{Status: 'A', Path: "new.go"},
	}, []PathMapping{
		{Path: "old.go", Change: Change{ArtifactID: "old-artifact", Removed: true}},
		{Path: "new.go", Change: Change{ArtifactID: "new-artifact", Revision: "new-r1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || !changes[0].Removed || changes[0].ArtifactID != "old-artifact" || changes[1].Removed || changes[1].ArtifactID != "new-artifact" {
		t.Fatalf("changes = %#v, want separate deletion and addition", changes)
	}
}

func TestChangesFromGitFailsClosedOnMissingOrAmbiguousMapping(t *testing.T) {
	for _, mappings := range [][]PathMapping{
		nil,
		{{Path: "a.go", Change: Change{ArtifactID: "one", Revision: "r1"}}, {Path: "a.go", Change: Change{ArtifactID: "two", Revision: "r2"}}},
	} {
		if _, err := ChangesFromGit([]GitChange{{Status: 'M', Path: "a.go"}}, mappings); err == nil {
			t.Fatal("ChangesFromGit() accepted missing or ambiguous path mapping")
		}
	}
}

func TestChangesFromGitRejectsMalformedDeletionMapping(t *testing.T) {
	for _, change := range []Change{
		{ArtifactID: "artifact", Revision: "rev"},
		{ArtifactID: "artifact", Removed: true, Revision: "rev"},
		{ArtifactID: "artifact", Removed: true, ContractVersion: "contract"},
		{ArtifactID: "artifact", Removed: true, Toolchain: "go"},
	} {
		if _, err := ChangesFromGit([]GitChange{{Status: 'D', Path: "gone.go"}}, []PathMapping{{Path: "gone.go", Change: change}}); err == nil {
			t.Fatalf("accepted malformed deletion mapping: %#v", change)
		}
	}
}

func TestGitChangesPinsCommandsAndRejectsDirtyAmbiguousAndCancelledInputs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	repository := t.TempDir()
	if err := os.Mkdir(filepath.Join(repository, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".git", "config"), []byte("[core]\n\tbare = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Repeat("a", 40)
	new := strings.Repeat("b", 40)
	original := runGit
	defer func() { runGit = original }()
	var calls [][]string
	runGit = func(ctx context.Context, repository string, limit int, args ...string) ([]byte, []byte, error) {
		if repository != root || limit != 1024 {
			t.Fatalf("Git invocation escaped canonical root/output bound: %q, %d", repository, limit)
		}
		calls = append(calls, append([]string(nil), args...))
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if args[0] == "ls-tree" || args[0] == "ls-files" {
			return nil, nil, nil
		}
		if args[0] == "rev-parse" {
			return []byte(strings.TrimSuffix(args[len(args)-1], "^{commit}") + "\n"), nil, nil
		}
		if reflect.DeepEqual(args, []string{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all"}) {
			return nil, nil, nil
		}
		return []byte(":100644 100644 " + old + " " + new + " M\x00file.go\x00"), nil, nil
	}
	changes, err := GitChanges(context.Background(), repository, old, new, GitOptions{MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("changes = %#v", changes)
	}
	wantCalls := [][]string{
		{"rev-parse", "--verify", "--quiet", old + "^{commit}"},
		{"rev-parse", "--verify", "--quiet", new + "^{commit}"},
		{"ls-tree", "-r", "-z", "--full-tree", old},
		{"ls-tree", "-r", "-z", "--full-tree", new},
		{"ls-tree", "-r", "-z", "--full-tree", "HEAD"},
		{"ls-files", "--stage", "-z"},
		{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all"},
		{"ls-files", "-v", "-z"},
		{"-c", "diff.external=", "-c", "diff.trustExitCode=false", "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=all", "--no-relative", "--no-abbrev", "--raw", "-z", old, new, "--"},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("git calls = %#v, want pinned sequence %#v", calls, wantCalls)
	}
	if _, err := GitChanges(context.Background(), repository, "HEAD", new, GitOptions{}); err == nil {
		t.Fatal("GitChanges accepted ambiguous ref")
	}

	runGit = func(_ context.Context, _ string, _ int, args ...string) ([]byte, []byte, error) {
		if args[0] == "ls-tree" || args[0] == "ls-files" {
			return nil, nil, nil
		}
		if args[0] == "rev-parse" {
			return []byte(strings.TrimSuffix(args[len(args)-1], "^{commit}") + "\n"), nil, nil
		}
		return []byte("?? untracked\x00"), nil, nil
	}
	if _, err := GitChanges(context.Background(), repository, old, new, GitOptions{}); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("dirty repository error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runGit = func(ctx context.Context, _ string, _ int, _ ...string) ([]byte, []byte, error) {
		return nil, nil, ctx.Err()
	}
	if _, err := GitChanges(ctx, repository, old, new, GitOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want context.Canceled", err)
	}
}
