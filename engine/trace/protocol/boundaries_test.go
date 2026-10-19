package protocol

import (
	"bytes"
	"strings"
	"testing"

	"modelcheck/trace"
)

func TestTSharkRejectsOneOverPacketAndLayerLimits(t *testing.T) {
	packet := `{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}`
	body := "[" + strings.TrimSuffix(strings.Repeat(packet+",", tsharkMaxPackets+1), ",") + "]"
	out, err := AnalyzeTShark(tsharkInput(t, body))
	requireGrammarRule(t, out, err, "schema_invalid")
	layers := `{"name":"frame","fields":{"frame.protocols":"` + strings.TrimSuffix(strings.Repeat("ip:", tsharkMaxLayers), ":") + `"}},` + strings.TrimSuffix(strings.Repeat(`{"name":"ip","fields":{}},`, tsharkMaxLayers), ",")
	out, err = AnalyzeTShark(tsharkInput(t, `[{"layers":[`+layers+`]}]`))
	requireGrammarRule(t, out, err, "schema_invalid")
}

func TestTSharkMetadataBoundaries(t *testing.T) {
	for _, id := range []string{"", strings.Repeat("a", 129)} {
		in := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}]`)
		in.CaptureID = id
		out, err := AnalyzeTShark(in)
		requireGrammarRule(t, out, err, "metadata_invalid")
	}
	for _, status := range []string{"", strings.Repeat("a", 129)} {
		in := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}]`)
		in.ExportStatus = status
		out, err := AnalyzeTShark(in)
		requireGrammarRule(t, out, err, "metadata_invalid")
	}
	in := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}]`)
	in.CaptureID = strings.Repeat("a", 128)
	if _, err := AnalyzeTShark(in); err != nil {
		t.Fatalf("inclusive capture identity bound rejected: %v", err)
	}
	in.CaptureID = "synthetic-token"
	in.ExportSHA256 = strings.Repeat("0", 64)
	out, err := AnalyzeTShark(in)
	requireGrammarRule(t, out, err, "secret_indicator")
	in.CaptureID = "synthetic"
	in.Artifact.Path = "captures/credential.json"
	out, err = AnalyzeTShark(in)
	requireTsharkRule(t, out, err, "secret_indicator")
}

func TestTSharkLogicalPathSeparatesContentIdentity(t *testing.T) {
	in := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"ip.proto":"6"}}]}]`)
	first, err := AnalyzeTShark(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Artifact.Path = "captures/another.json"
	second, err := AnalyzeTShark(in)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("logical path did not participate in identity")
	}
	a, err := trace.UnmarshalJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := trace.UnmarshalJSON(second)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, node := range a.Nodes {
		ids[node.ID] = true
	}
	for _, node := range b.Nodes {
		if ids[node.ID] || node.Provenance.ContentSHA256 != in.ExportSHA256 || !strings.Contains(node.Provenance.Anchor, in.Artifact.Path) {
			t.Fatal("path identity, anchor or digest was lost")
		}
	}
	gapIDs := map[string]bool{}
	for _, gap := range a.Gaps {
		gapIDs[gap.ID] = true
	}
	if len(a.Gaps) == 0 || len(a.Gaps) != len(b.Gaps) {
		t.Fatal("expected matching disclosures for both paths")
	}
	for _, gap := range b.Gaps {
		if gapIDs[gap.ID] || gap.Provenance.ContentSHA256 != in.ExportSHA256 || !strings.Contains(gap.Reference, in.Artifact.Path) {
			t.Fatal("gap path identity, reference or digest was lost")
		}
	}
}

func TestTSharkRepeatedFieldsBelongToTheirLayerOccurrence(t *testing.T) {
	in := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip:ip"}},{"name":"ip","fields":{"ip.proto":"4"}},{"name":"ip","fields":{"ip.proto":"4"}}]}]`)
	out, err := AnalyzeTShark(in)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := trace.UnmarshalJSON(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Nodes) != 4 || len(doc.Edges) != 2 || len(doc.Evidence) != 6 {
		t.Fatalf("unexpected occurrence counts: %+v", doc)
	}
	nodes := map[string]trace.Node{}
	for _, node := range doc.Nodes {
		if _, exists := nodes[node.ID]; exists {
			t.Fatal("repeated field or layer identity collided")
		}
		nodes[node.ID] = node
	}
	for _, edge := range doc.Edges {
		field, fieldOK := nodes[edge.Source]
		layer, layerOK := nodes[edge.Target]
		if !fieldOK || !layerOK || field.Kind != trace.BinaryField || layer.Kind != trace.ProtocolMessage || edge.Kind != trace.DerivedFrom || field.Provenance.Anchor != layer.Provenance.Anchor+"&field=ip.proto" {
			t.Fatalf("field connected to wrong layer occurrence: %+v", edge)
		}
	}
}

func TestGrammarAdaptersRejectEachPreflightBudget(t *testing.T) {
	for _, limits := range []budget{
		{records: 0, output: 1 << 30, working: 1 << 30},
		{records: 100, output: 1, working: 1 << 30},
		{records: 100, output: 1 << 30, working: 1},
	} {
		out, err := analyzeGrammarWithBudget(grammarTestInput("kaitai"), limits)
		requireGrammarRule(t, out, err, "preflight_limit")
		out, err = analyzeGrammarReferenceWithBudget(referenceTestInput(), limits)
		requireGrammarRule(t, out, err, "preflight_limit")
	}
}
