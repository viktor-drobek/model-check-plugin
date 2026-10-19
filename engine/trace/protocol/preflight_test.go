package protocol

import (
	"testing"

	"modelcheck/trace"
)

func TestTSharkPreflightCountsEveryExportStatus(t *testing.T) {
	for _, status := range []string{"complete", "truncated", "unknown"} {
		t.Run(status, func(t *testing.T) {
			input := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"dns"}},{"name":"dns","fields":{"dns.flags.response":"1","dns.qry.type":"1","dns.resp.type":"1"}}]}]`)
			input.ExportStatus = status
			encoded, err := AnalyzeTShark(input)
			if err != nil {
				t.Fatal(err)
			}
			document, err := trace.UnmarshalJSON(encoded)
			if err != nil {
				t.Fatal(err)
			}
			actual := uint64(len(document.Nodes) + len(document.Edges) + len(document.Evidence) + len(document.Gaps))
			packets, err := planTShark(mustDecodeTShark(t, input.Artifact.Content))
			if err != nil {
				t.Fatal(err)
			}
			inputID := tupleHash("tshark-input/1", input.Artifact.Path, input.ExportSHA256, input.CaptureID, input.TSharkVersion, input.Profile)
			estimated, output := estimateTSharkPlan(input, packets, inputID)
			if estimated != actual {
				t.Errorf("record estimate=%d, emitted=%d", estimated, actual)
			}
			if uint64(len(encoded)) > output {
				t.Errorf("output bound=%d, emitted=%d", output, len(encoded))
			}
			limits := defaultBudget
			limits.records = actual - 1
			got, err := analyzeTSharkWithBudget(input, limits)
			requireGrammarRule(t, got, err, "preflight_limit")
			limits.records = actual
			if _, err := analyzeTSharkWithBudget(input, limits); err != nil {
				t.Fatalf("exact record budget rejected: %v", err)
			}
		})
	}
}
