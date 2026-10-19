package ir

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestStaticCatalogReconstructsCanonicalIR(t *testing.T) {
	m := twoProcs()
	catalog, err := NewStaticCatalog(m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := catalog.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	read, err := ReadStaticCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := read.Reconstruct()
	if err != nil {
		t.Fatal(err)
	}
	want, err := MarshalJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := MarshalJSON(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("reconstructed IR differs:\n%s\nwant:\n%s", got, want)
	}
	read.IR[0] ^= 1
	if bytes.Equal(read.IR, catalog.IR) {
		t.Fatal("catalog IR aliases the reader result")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(catalog.IR, &fields); err != nil {
		t.Fatal(err)
	}
	canonicalKeys := []string{"schema", "name", "globals", "channels", "processes", "properties", "origin"}
	var keys []string
	for _, key := range canonicalKeys {
		if _, ok := fields[key]; ok {
			keys = append(keys, key)
		}
	}
	if len(fields) != len(keys) {
		t.Fatalf("IR has %d fields, expected only known model fields", len(fields))
	}
	for left, right := 0, len(keys)-1; left < right; left, right = left+1, right-1 {
		keys[left], keys[right] = keys[right], keys[left]
	}
	var reordered []byte
	reordered = append(reordered, '{')
	for i, key := range keys {
		value, ok := fields[key]
		if !ok {
			t.Fatalf("IR is missing field %q", key)
		}
		if i > 0 {
			reordered = append(reordered, ',')
		}
		reordered = append(reordered, '"')
		reordered = append(reordered, key...)
		reordered = append(reordered, '"', ':')
		reordered = append(reordered, value...)
	}
	reordered = append(reordered, '}')
	dec := json.NewDecoder(bytes.NewReader(reordered))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	for i, want := range keys {
		token, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		got, ok := token.(string)
		if !ok || got != want {
			t.Fatalf("top-level member %d = %q, want %q", i, got, want)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			t.Fatal(err)
		}
	}
	mutated := *catalog
	mutated.IR = reordered
	canonicalCatalog, err := mutated.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalCatalog, data) {
		t.Fatal("catalog marshal output is not canonical after valid IR whitespace mutation")
	}
}

func TestRejectDuplicateJSONMembersUsesSimpleFold(t *testing.T) {
	for _, data := range []string{
		`{"schema":1,"ſchema":2}`,
		`{"K":1,"K":2}`,
		`{"Å":1,"å":2}`,
	} {
		if err := rejectDuplicateJSONMembers([]byte(data)); err == nil {
			t.Fatalf("rejectDuplicateJSONMembers(%s) succeeded", data)
		}
	}
}

func TestReadStaticCatalogRejectsCorruption(t *testing.T) {
	catalog, err := NewStaticCatalog(twoProcs())
	if err != nil {
		t.Fatal(err)
	}
	data, err := catalog.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"digest", []byte(strings.Replace(string(data), catalog.CatalogRef.SHA256, strings.Repeat("0", 64), 1)), "digest mismatch"},
		{"trailing", append(append([]byte(nil), data...), []byte(" {}")...), "trailing data"},
		{"duplicate envelope", []byte(`{"schema":"bad",` + strings.TrimPrefix(string(data), "{")), `duplicate member "schema"`},
		{"case-variant duplicate envelope", []byte(`{"Schema":"bad",` + strings.TrimPrefix(string(data), "{")), `duplicate member "schema"`},
		{"duplicate IR", []byte(strings.Replace(string(data), `"name": "two"`, `"name":"two","name":"two"`, 1)), `duplicate member "name"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ReadStaticCatalog(tc.data); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReadStaticCatalogNormalizesDigestCase(t *testing.T) {
	catalog, err := NewStaticCatalog(twoProcs())
	if err != nil {
		t.Fatal(err)
	}
	data, err := catalog.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(catalog.CatalogRef.SHA256)
	read, err := ReadStaticCatalog([]byte(strings.Replace(string(data), catalog.CatalogRef.SHA256, upper, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if read.CatalogRef.SHA256 != catalog.CatalogRef.SHA256 {
		t.Fatalf("digest = %q, want %q", read.CatalogRef.SHA256, catalog.CatalogRef.SHA256)
	}
	if _, err := ReadOptimizedIRCatalog(optimizedJSON(t, twoProcs(), upper), read); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCatalogIsomorphism(t *testing.T) {
	source := twoProcs()
	catalog, err := NewStaticCatalog(source)
	if err != nil {
		t.Fatal(err)
	}
	validation, err := ValidateCatalogIsomorphism(source, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Schema != CatalogValidationSchema || validation.RelationKind != CatalogIsomorphismRelation || validation.CatalogRef != catalog.CatalogRef {
		t.Fatalf("validation = %#v", validation)
	}
	different := twoProcs()
	different.Name = "different"
	differentCatalog, err := NewStaticCatalog(different)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCatalogIsomorphism(source, differentCatalog); err == nil || !strings.Contains(err.Error(), "catalog structural mismatch") {
		t.Fatalf("mismatched catalog error = %v", err)
	}
	unreachable := twoProcs()
	unreachable.Processes[0].Locations = append(unreachable.Processes[0].Locations, Location{Name: "L2"})
	unreachable.Processes[0].Edges = append(unreachable.Processes[0].Edges, Edge{From: 2, To: 2, Text: "unreachable"})
	unreachableCatalog, err := NewStaticCatalog(unreachable)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCatalogIsomorphism(source, unreachableCatalog); err == nil || !strings.Contains(err.Error(), "catalog structural mismatch") {
		t.Fatalf("unreachable catalog error = %v", err)
	}
	corrupt := *catalog
	corrupt.CatalogRef.SHA256 = strings.Repeat("0", 64)
	if _, err := ValidateCatalogIsomorphism(source, &corrupt); err == nil || !strings.Contains(err.Error(), "catalog structural mismatch") {
		t.Fatalf("corrupt catalog error = %v", err)
	}
}

func optimizedJSON(t *testing.T, m *Model, digest string) []byte {
	t.Helper()
	return []byte(fmt.Sprintf(`{"schema":%q,"catalog_ref":{"schema":%q,"sha256":%q},"relation_kind":"property-slice","property_scope":["safe","deadlock"],"transfer_direction":"preserves-safety"}`,
		OptimizedSchema, Schema, digest))
}

func TestReadOptimizedIRRejectsTrailingOrIncompleteMetadata(t *testing.T) {
	m := twoProcs()
	canonical, err := MarshalJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	digestBytes := sha256.Sum256(canonical)
	digest := hex.EncodeToString(digestBytes[:])
	valid := string(optimizedJSON(t, m, digest))
	cases := []struct{ name, input, want string }{
		{"second JSON value", valid + ` {}`, "trailing data"},
		{"malformed suffix", valid + ` broken`, "trailing data"},
		{"empty scope", strings.Replace(valid, `["safe","deadlock"]`, `[]`, 1), "property_scope"},
		{"lossy lacks direction", strings.Replace(strings.Replace(valid, `"property-slice"`, `"lossy"`, 1), `,"property_scope":["safe","deadlock"],"transfer_direction":"preserves-safety"`, ``, 1), "transfer_direction"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadOptimizedIR([]byte(tc.input), m)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReadOptimizedIRBuildsLLMBrief(t *testing.T) {
	m := twoProcs()
	m.Processes[0].Edges[0].Origin = &Origin{File: "protocol.pml", Line: 12, Name: "advance"}
	canonical, err := MarshalJSON(m)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	brief, err := ReadOptimizedIR(optimizedJSON(t, m, hex.EncodeToString(digest[:])), m)
	if err != nil {
		t.Fatal(err)
	}
	if brief.RelationKind != "property-slice" || strings.Join(brief.PropertyScope, ",") != "deadlock,safe" {
		t.Fatalf("brief metadata = %#v", brief)
	}
	for _, want := range []string{"Canonical model \"two\"", "weaker relation", "advance (protocol.pml:12)"} {
		if !strings.Contains(brief.Text, want) {
			t.Fatalf("brief missing %q:\n%s", want, brief.Text)
		}
	}
}

func TestReadOptimizedIRRejectsCatalogMismatch(t *testing.T) {
	m := twoProcs()
	_, err := ReadOptimizedIR(optimizedJSON(t, m, strings.Repeat("0", 64)), m)
	if err == nil || !strings.Contains(err.Error(), "catalog digest mismatch") {
		t.Fatalf("error = %v, want digest mismatch", err)
	}
}

func TestReadOptimizedIRCatalogBindsImmutableCatalog(t *testing.T) {
	m := twoProcs()
	catalog, err := NewStaticCatalog(m)
	if err != nil {
		t.Fatal(err)
	}
	brief, err := ReadOptimizedIRCatalog(optimizedJSON(t, m, catalog.CatalogRef.SHA256), catalog)
	if err != nil {
		t.Fatal(err)
	}
	if brief.RelationKind != "property-slice" {
		t.Fatalf("relation kind = %q", brief.RelationKind)
	}
}
