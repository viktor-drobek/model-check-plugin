package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"modelcheck/trace"
)

func tsharkDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func tsharkInput(t *testing.T, body string) TSharkJSONInput {
	t.Helper()
	b := []byte(body)
	return TSharkJSONInput{Artifact: Input{Path: "captures/synthetic.json", Content: b}, CaptureID: "capture-1", TSharkVersion: "4.4.10", Profile: "tshark-4.4.10-core-1", ExportSHA256: tsharkDigest(b), ExportStatus: "complete"}
}
func requireTsharkRule(t *testing.T, out []byte, err error, code string) {
	t.Helper()
	var r *RuleError
	if !errors.As(err, &r) || r == nil {
		t.Fatalf("out=%d err=%v want stage3b.%s", len(out), err, code)
	}
	if len(out) != 0 || r.Code != "stage3b."+code {
		t.Fatalf("out=%d err=%v locator=%q want stage3b.%s", len(out), err, r.Locator, code)
	}
}

func TestTSharkSyntheticProjection(t *testing.T) {
	in := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"eth:ip:tcp:tls"}},{"name":"eth","fields":{}},{"name":"ip","fields":{"ip.proto":"6"}},{"name":"tcp","fields":{"tcp.srcport":"443","tcp.dstport":"49152"}},{"name":"tls","fields":{}}]}]`)
	got, err := AnalyzeTShark(in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := AnalyzeTShark(in)
	if err != nil || !bytes.Equal(got, again) {
		t.Fatalf("nondeterministic: %v", err)
	}
	d, err := trace.UnmarshalJSON(got)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := trace.JSON(d)
	if err != nil || !bytes.Equal(got, canonical) {
		t.Fatalf("canonical encoding mismatch: %v", err)
	}
	if len(d.Nodes) != 6 || len(d.Edges) != 3 || len(d.Evidence) != 9 || len(d.Gaps) != 4 {
		t.Fatalf("records nodes=%d edges=%d evidence=%d gaps=%d", len(d.Nodes), len(d.Edges), len(d.Evidence), len(d.Gaps))
	}
	for _, n := range d.Nodes {
		if len(n.Evidence) != 1 || n.Provenance.Producer.Name != "mcd-tshark-projection-adapter" {
			t.Fatal("bad node provenance/evidence")
		}
	}
	for _, e := range d.Edges {
		if e.Kind != trace.DerivedFrom || !strings.HasPrefix(e.Source, "binary-field.") || !strings.HasPrefix(e.Target, "protocol-message.") {
			t.Fatal("wrong edge direction")
		}
	}
	for _, g := range d.Gaps {
		if g.Provenance.ContentSHA256 != in.ExportSHA256 {
			t.Fatal("gap provenance digest")
		}
	}
}

func TestTSharkRepeatedAndOmitted(t *testing.T) {
	in := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip:ip"}},{"name":"ip","fields":{}},{"name":"ip","fields":{"ip.proto":"4"}}]}]`)
	b, err := AnalyzeTShark(in)
	if err != nil {
		t.Fatal(err)
	}
	d, err := trace.UnmarshalJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Nodes) != 3 || len(d.Edges) != 1 || len(d.Gaps) != 3 {
		t.Fatalf("unexpected counts: nodes=%d edges=%d gaps=%d", len(d.Nodes), len(d.Edges), len(d.Gaps))
	}
	seen := map[string]bool{}
	for _, n := range d.Nodes {
		if seen[n.ID] {
			t.Fatal("duplicate node")
		}
		seen[n.ID] = true
	}
	found := false
	for _, g := range d.Gaps {
		if g.Kind == "tshark.permitted_fields_omitted" {
			found = true
			if !strings.Contains(g.Reference, "omitted=ip.proto") {
				t.Fatal(g.Reference)
			}
		}
	}
	if !found {
		t.Fatal("missing omission gap")
	}
}

func TestTSharkRejections(t *testing.T) {
	cases := []struct{ name, code, body string }{
		{"unknown layer", "schema_invalid", `[{"layers":[{"name":"frame","fields":{"frame.protocols":"bogus"}},{"name":"bogus","fields":{}}]}]`},
		{"bad value", "schema_invalid", `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"ip.proto":"01"}}]}]`},
		{"unsupported fields", "schema_invalid", `[{"layers":[{"name":"frame","fields":{"frame.protocols":"tls"}},{"name":"tls","fields":{"x":"y"}}]}]`},
		{"only frame", "schema_invalid", `[{"layers":[{"name":"frame","fields":{"frame.protocols":""}}]}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := tsharkInput(t, tc.body)
			o, e := AnalyzeTShark(i)
			requireTsharkRule(t, o, e, tc.code)
		})
	}
	i := tsharkInput(t, `[]`)
	i.ExportSHA256 = strings.Repeat("0", 64)
	o, e := AnalyzeTShark(i)
	requireTsharkRule(t, o, e, "digest_mismatch")
	i = tsharkInput(t, `[]`)
	i.TSharkVersion = "latest"
	o, e = AnalyzeTShark(i)
	requireTsharkRule(t, o, e, "metadata_invalid")
}

func TestTSharkProfileFieldDomainsAndBoundaries(t *testing.T) {
	body := `[{"layers":[{"name":"frame","fields":{"frame.protocols":"eth:ip:ipv6:tcp:udp:dns"}},{"name":"eth","fields":{"eth.type":"0x0000"}},{"name":"ip","fields":{"ip.proto":"255"}},{"name":"ipv6","fields":{"ipv6.nxt":"0"}},{"name":"tcp","fields":{"tcp.srcport":"0","tcp.dstport":"65535"}},{"name":"udp","fields":{"udp.srcport":"1","udp.dstport":"65535"}},{"name":"dns","fields":{"dns.flags.response":"1","dns.qry.type":"0","dns.resp.type":"65535"}}]}]`
	if _, err := AnalyzeTShark(tsharkInput(t, body)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ field, value string }{
		{"eth.type", "0X0800"}, {"ip.proto", "256"}, {"ipv6.nxt", "00"}, {"tcp.srcport", "65536"}, {"udp.dstport", "-1"}, {"dns.flags.response", "2"}, {"dns.qry.type", "65536"},
	} {
		layer := strings.Split(tc.field, ".")[0]
		body := `[{"layers":[{"name":"frame","fields":{"frame.protocols":"` + layer + `"}},{"name":"` + layer + `","fields":{"` + tc.field + `":"` + tc.value + `"}}]}]`
		o, err := AnalyzeTShark(tsharkInput(t, body))
		requireTsharkRule(t, o, err, "schema_invalid")
	}
}

func TestTSharkSchemaPrecedenceAndEnvelope(t *testing.T) {
	cases := []string{
		`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"ip.proto":null}}]}]`,
		`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"ip.proto":true}}]}]`,
		`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"ip.proto":1}}]}]`,
		`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"ip.proto":[]}}]}]`,
		`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"ip.proto":{}}}]}]`,
		`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"other":"4"}}]}]`,
		`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"ip.proto":"4"},"extra":1}]}]`,
		`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{"ip.proto":"4"}},{"name":"frame","fields":{}}]}]`,
		`[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip:tcp"}},{"name":"ip","fields":{"ip.proto":"4"}}]}]`,
	}
	for n, body := range cases {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			o, err := AnalyzeTShark(tsharkInput(t, body))
			requireTsharkRule(t, o, err, "schema_invalid")
		})
	}
	body := `[{"layers":[{"name":"frame","fields":{"frame.protocols":"bogus"}},{"name":"bogus","fields":{"token":"secret"}}]}]`
	o, err := AnalyzeTShark(tsharkInput(t, body))
	requireTsharkRule(t, o, err, "schema_invalid")
}

func TestTSharkMetadataAndStatus(t *testing.T) {
	cases := []struct {
		name, code string
		mutate     func(*TSharkJSONInput)
	}{
		{"capture", "metadata_invalid", func(i *TSharkJSONInput) { i.CaptureID = "bad/capture" }},
		{"version", "metadata_invalid", func(i *TSharkJSONInput) { i.TSharkVersion = "4.4" }},
		{"profile", "metadata_invalid", func(i *TSharkJSONInput) { i.Profile = "other" }},
		{"status", "metadata_invalid", func(i *TSharkJSONInput) { i.ExportStatus = "partial" }},
		{"digest", "digest_mismatch", func(i *TSharkJSONInput) { i.ExportSHA256 = strings.Repeat("0", 64) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}]`)
			tc.mutate(&i)
			o, err := AnalyzeTShark(i)
			requireTsharkRule(t, o, err, tc.code)
		})
	}
	for _, status := range []string{"truncated", "unknown"} {
		i := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}]`)
		i.ExportStatus = status
		out, err := AnalyzeTShark(i)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := trace.UnmarshalJSON(out)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		wantKind := map[string]string{"truncated": "tshark.export_truncated", "unknown": "tshark.export_status_unknown"}[status]
		for _, gap := range doc.Gaps {
			if gap.Kind == wantKind {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing status gap")
		}
	}
}

func TestTSharkLimitsAndInclusiveMaximums(t *testing.T) {
	one := `{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}`
	var packets strings.Builder
	packets.WriteByte('[')
	for i := 0; i < tsharkMaxPackets; i++ {
		if i != 0 {
			packets.WriteByte(',')
		}
		packets.WriteString(one)
	}
	packets.WriteByte(']')
	if _, err := AnalyzeTShark(tsharkInput(t, packets.String())); err != nil {
		t.Fatal("packet maximum rejected:", err)
	}
	layers := []string{`{"name":"frame","fields":{"frame.protocols":"ip:ip:ip:ip:ip:ip:ip:ip:ip:ip:ip:ip:ip:ip:ip"}}`}
	for i := 0; i < tsharkMaxLayers-1; i++ {
		layers = append(layers, `{"name":"ip","fields":{}}`)
	}
	if _, err := AnalyzeTShark(tsharkInput(t, `[{"layers":[`+strings.Join(layers, ",")+`]}]`)); err != nil {
		t.Fatal("layer maximum rejected:", err)
	}
	base := `[{"layers":[{"name":"frame","fields":{"frame.protocols":"ip"}},{"name":"ip","fields":{}}]}]`
	padded := base + strings.Repeat(" ", (1<<20)-len(base))
	if len(padded) != 1<<20 {
		t.Fatal("bad padding")
	}
	if _, err := AnalyzeTShark(tsharkInput(t, padded)); err != nil {
		t.Fatal("inclusive byte maximum rejected:", err)
	}
}

func TestTSharkPreflightIndependentBounds(t *testing.T) {
	i := tsharkInput(t, `[{"layers":[{"name":"frame","fields":{"frame.protocols":"dns"}},{"name":"dns","fields":{"dns.flags.response":"1","dns.qry.type":"1","dns.resp.type":"1"}}]}]`)
	for _, limits := range []budget{
		{records: 1, output: 1 << 30, working: 1 << 30},
		{records: tsharkMaxRecords, output: 1, working: 1 << 30},
		{records: tsharkMaxRecords, output: 1 << 30, working: 1},
	} {
		out, err := analyzeTSharkWithBudget(i, limits)
		requireTsharkRule(t, out, err, "preflight_limit")
	}
	packets, err := planTShark(mustDecodeTShark(t, i.Artifact.Content))
	if err != nil {
		t.Fatal(err)
	}
	records, out := estimateTSharkPlan(i, packets, tupleHash("tshark-input/1", i.Artifact.Path, i.ExportSHA256, i.CaptureID, i.TSharkVersion, i.Profile))
	if records != 16 {
		t.Fatalf("concrete record estimate=%d want 16", records)
	}
	if out == 0 || out > 512*1024*1024 {
		t.Fatalf("bad output estimate=%d", out)
	}
}

func TestTSharkLargestAcceptedAllocationEstimate(t *testing.T) {
	// The largest construction fixture combines 6,000 dense DNS occurrences
	// with the inclusive byte bound, rather than only padding a tiny ledger.
	layer := `{"name":"dns","fields":{"dns.flags.response":"1","dns.qry.type":"65535","dns.resp.type":"65535"}}`
	packet := `{"layers":[{"name":"frame","fields":{"frame.protocols":"` + strings.TrimSuffix(strings.Repeat("dns:", 15), ":") + `"}},` + strings.TrimSuffix(strings.Repeat(layer+",", 15), ",") + `]}`
	base := "[" + strings.TrimSuffix(strings.Repeat(packet+",", 400), ",") + "]"
	if len(base) > 1<<20 {
		t.Fatal("construction fixture exceeds byte limit")
	}
	input := tsharkInput(t, base+strings.Repeat(" ", (1<<20)-len(base)))
	packets, err := planTShark(mustDecodeTShark(t, input.Artifact.Content))
	if err != nil {
		t.Fatal(err)
	}
	records, output := estimateTSharkPlan(input, packets, tupleHash("tshark-input/1", input.Artifact.Path, input.ExportSHA256, input.CaptureID, input.TSharkVersion, input.Profile))
	if err := checkBudget(records, output, defaultBudget); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	encoded, err := AnalyzeTShark(input)
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if uint64(len(encoded)) > output {
		t.Fatal("canonical bytes exceeded preflight")
	}
	estimate := 2*output + 256*records + 2*1024*1024
	if delta := after.TotalAlloc - before.TotalAlloc; delta > estimate {
		t.Fatalf("allocation delta=%d exceeds working estimate=%d", delta, estimate)
	}
}

func mustDecodeTShark(t *testing.T, b []byte) any {
	t.Helper()
	v, err := decodeJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
