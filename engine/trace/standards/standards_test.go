package standards

import (
	"strings"
	"testing"

	"modelcheck/trace"
)

func TestImportRFCEmitsCandidatesOnlyAfterBCP14Declaration(t *testing.T) {
	withDeclaration := []byte(`Request for Comments: 9000
Category: Standards Track

An introduction MUST NOT be emitted before the declaration.

The key words "MUST" and "SHOULD" are to be interpreted as described in [RFC8174].

An implementation MUST preserve the source revision.
`)
	document, err := ImportRFC(RFCInput{
		Artifact:  Input{Path: "rfc-9000.txt", Content: withDeclaration},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt",
	})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if err := trace.Validate(document); err != nil {
		t.Fatalf("trace.Validate() error = %v", err)
	}
	if got := normativeCandidates(document); got != 1 {
		t.Fatalf("normative candidate count = %d, want 1", got)
	}

	withIncidentalReference := []byte(`Request for Comments: 9002
Category: Informational

Key words are discussed in this historical overview.

Reference: RFC 8174.

An implementation MAY preserve the source revision.
`)
	document, err = ImportRFC(RFCInput{
		Artifact:  Input{Path: "rfc-9002.txt", Content: withIncidentalReference},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9002.txt",
	})
	if err != nil {
		t.Fatalf("ImportRFC() with incidental reference error = %v", err)
	}
	if got := normativeCandidates(document); got != 0 {
		t.Fatalf("normative candidate count with incidental reference = %d, want 0", got)
	}

	withoutDeclaration := []byte(`Request for Comments: 9001
Category: Informational

An implementation MUST preserve the source revision.
`)
	document, err = ImportRFC(RFCInput{
		Artifact:  Input{Path: "rfc-9001.txt", Content: withoutDeclaration},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9001.txt",
	})
	if err != nil {
		t.Fatalf("ImportRFC() without declaration error = %v", err)
	}
	if got := normativeCandidates(document); got != 0 {
		t.Fatalf("normative candidate count without declaration = %d, want 0", got)
	}
}

func TestImportRFCRejectsUntrustedSourceURL(t *testing.T) {
	for _, sourceURL := range []string{
		"https://example.invalid/rfc-9000.txt",
		"http://www.rfc-editor.org/rfc/rfc9000.txt",
		"https://user:token@www.ietf.org/rfc/rfc9000.txt",
		"https://www.rfc-editor.org/rfc/rfc9000.txt?format=text",
		"https://www.rfc-editor.org/rfc/rfc9000.txt#section-1",
	} {
		t.Run(sourceURL, func(t *testing.T) {
			_, err := ImportRFC(RFCInput{
				Artifact:  Input{Path: "rfc-9000.txt", Content: []byte("Request for Comments: 9000\nCategory: Informational\n")},
				SourceURL: sourceURL,
			})
			if err == nil || !strings.Contains(err.Error(), "RFC Editor or IETF") {
				t.Fatalf("ImportRFC() error = %v, want RFC Editor or IETF rejection", err)
			}
		})
	}
}

func TestImportRFCRejectsSourceURLForAnotherRFC(t *testing.T) {
	_, err := ImportRFC(RFCInput{
		Artifact:  Input{Path: "rfc-9000.txt", Content: []byte("Request for Comments: 9000\nCategory: Informational\n")},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9001.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "does not identify RFC 9000") {
		t.Fatalf("ImportRFC() error = %v, want source URL number rejection", err)
	}
}

func TestImportRFCRejectsNoncanonicalSourceURLForms(t *testing.T) {
	for _, sourceURL := range []string{
		"https://www.rfc-editor.org/rfc/rfc9000.txt/",
		"https://www.rfc-editor.org:/rfc/rfc9000.txt",
		"https://www.rfc-editor.org/errata/rfc9000",
		"https://mailarchive.ietf.org/arch/msg/rfc9000",
		"https://www.rfc-editor.org/rfc/../../x/rfc9000.txt",
		"https://www.rfc-editor.org/rfc/rfc%39000.txt",
		"https://WWW.RFC-EDITOR.ORG/rfc/rfc9000.txt",
		"https://www.rfc-editor.org/rfc/rfc9000",
		" HTTPS://www.rfc-editor.org/rfc/rfc9000.txt",
		"https://www.rfc-editor.org/rfc/rfc9000.txt ",
		"https://www.rfc-editor.org/rfc/rfc9000.txt#",
		"HTTPS://www.rfc-editor.org/rfc/rfc9000.txt",
	} {
		t.Run(sourceURL, func(t *testing.T) {
			_, err := ImportRFC(RFCInput{
				Artifact:  Input{Path: "rfc-9000.txt", Content: []byte("Request for Comments: 9000\nCategory: Informational\n")},
				SourceURL: sourceURL,
			})
			if err == nil {
				t.Fatal("ImportRFC() accepted a noncanonical source URL")
			}
		})
	}
}

func TestImportRFCAcceptsCanonicalPublisherURLs(t *testing.T) {
	for _, sourceURL := range []string{
		"https://www.rfc-editor.org/rfc/rfc9000.txt",
		"https://www.ietf.org/rfc/rfc9000.txt",
	} {
		t.Run(sourceURL, func(t *testing.T) {
			_, err := ImportRFC(RFCInput{
				Artifact:  Input{Path: "rfc-9000.txt", Content: []byte("Request for Comments: 9000\nCategory: Informational\n")},
				SourceURL: sourceURL,
			})
			if err != nil {
				t.Fatalf("ImportRFC() error = %v", err)
			}
		})
	}
}

func TestImportRFCPreservesLogicalInputPathInProvenanceAnchors(t *testing.T) {
	content := []byte("Request for Comments: 9000\nCategory: Informational\n")
	first, err := ImportRFC(RFCInput{Artifact: Input{Path: "one/rfc-9000.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt"})
	if err != nil {
		t.Fatalf("first ImportRFC() error = %v", err)
	}
	second, err := ImportRFC(RFCInput{Artifact: Input{Path: "two/rfc-9000.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt"})
	if err != nil {
		t.Fatalf("second ImportRFC() error = %v", err)
	}
	firstAnchor := first.Nodes[0].Provenance.Anchor
	secondAnchor := second.Nodes[0].Provenance.Anchor
	if firstAnchor == secondAnchor {
		t.Fatalf("anchors are identical: %q", firstAnchor)
	}
	if !strings.HasSuffix(firstAnchor, "#logical-path=one/rfc-9000.txt&line=1") {
		t.Fatalf("first anchor = %q", firstAnchor)
	}
	if !strings.HasSuffix(secondAnchor, "#logical-path=two/rfc-9000.txt&line=1") {
		t.Fatalf("second anchor = %q", secondAnchor)
	}
}

func TestImportRFCRejectsMalformedUpdatesHeader(t *testing.T) {
	for _, header := range []string{
		"Updates:",
		"Updates: 2119,",
		"Updates: 2119abc",
		"Updates: 1111 2222",
		"Updates: 1111;2222",
		"Updates: 1111   2222",
		"Updates: 1111                              2222 3333",
		"Updates: 1111,   2222, 3333",
	} {
		t.Run(header, func(t *testing.T) {
			_, err := ImportRFC(RFCInput{
				Artifact:  Input{Path: "rfc-9000.txt", Content: []byte("Request for Comments: 9000\nCategory: Informational\n" + header + "\n")},
				SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt",
			})
			if err == nil || !strings.Contains(err.Error(), "malformed Updates") {
				t.Fatalf("ImportRFC() error = %v, want malformed Updates rejection", err)
			}
		})
	}
}

func TestImportRFCRejectsUnsafeMetadataAndRevision(t *testing.T) {
	for _, test := range []struct {
		name     string
		artifact Input
		content  string
	}{
		{"unsafe category", Input{Path: "rfc-9000.txt"}, "Request for Comments: 9000\nCategory: Standards\x1b Track\n"},
		{"unsafe category before trimming", Input{Path: "rfc-9000.txt"}, "Request for Comments: 9000\nCategory: Informational\v\n"},
		{"overlong category", Input{Path: "rfc-9000.txt"}, "Request for Comments: 9000\nCategory: " + strings.Repeat("x", maxRFCCategoryRunes+1) + "\n"},
		{"unsafe source revision", Input{Path: "rfc-9000.txt", SourceRevision: "git:\u202eabc"}, "Request for Comments: 9000\nCategory: Informational\n"},
		{"source revision unsafe before trimming", Input{Path: "rfc-9000.txt", SourceRevision: "git:abc\v"}, "Request for Comments: 9000\nCategory: Informational\n"},
		{"source revision surrounding whitespace", Input{Path: "rfc-9000.txt", SourceRevision: " git:abc "}, "Request for Comments: 9000\nCategory: Informational\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.artifact.Content = []byte(test.content)
			_, err := ImportRFC(RFCInput{Artifact: test.artifact, SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt"})
			if err == nil {
				t.Fatal("ImportRFC() accepted unsafe metadata")
			}
		})
	}
}

func TestImportRFCRejectsUnrecognizedCategory(t *testing.T) {
	_, err := ImportRFC(RFCInput{
		Artifact:  Input{Path: "rfc-9021.txt", Content: []byte("Request for Comments: 9021\nCategory: Best  Current Practice\n")},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9021.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "publication category") {
		t.Fatalf("ImportRFC() error = %v, want publication category rejection", err)
	}
}

func TestImportRFCReadsWrappedUpdatesHeader(t *testing.T) {
	document, err := ImportRFC(RFCInput{
		Artifact:  Input{Path: "rfc-9000.txt", Content: []byte("Request for Comments: 9000\nUpdates: 1111,  2222,                    Example Org\n         3333, 4444\nCategory: Informational\n")},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt",
	})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got, want := document.Nodes[0].Label, "RFC 9000, category Informational, updates 1111, 2222, 3333, 4444"; got != want {
		t.Fatalf("standard label = %q, want %q", got, want)
	}
}

func TestImportRFCRecognizesKeywordsDeclaration(t *testing.T) {
	document, err := ImportRFC(RFCInput{
		Artifact: Input{Path: "rfc-9000.txt", Content: []byte(`Request for Comments: 9000
Category: Informational

The keywords "MUST" and "SHOULD" are to be interpreted as described in RFC 8174.

An implementation MUST preserve the source revision.
`)},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt",
	})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got := normativeCandidates(document); got != 1 {
		t.Fatalf("normative candidate count = %d, want 1", got)
	}
}

func TestImportRFCDoesNotTreatAClausallyDisconnectedBCPPredicateAsADeclaration(t *testing.T) {
	content := []byte("Request for Comments: 9016\nCategory: Informational\n\nThe key words are merely examples, while status codes are to be interpreted as described in RFC 8174.\n\nAn implementation MUST not become a candidate.\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9016.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9016.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got := normativeCandidates(document); got != 0 {
		t.Fatalf("normative candidate count = %d, want 0", got)
	}
}

func TestImportIEEEManifestRejectsDuplicateKeys(t *testing.T) {
	_, err := ImportIEEEManifest(Input{
		Path: "manifest.json",
		Content: []byte(`{
  "standard_id": "IEEE 802.1Q",
  "standard_id": "IEEE 802.1D",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [{"anchor": "8.6.8", "summary": "A concise permitted summary."}]
}`),
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("ImportIEEEManifest() error = %v, want duplicate-key rejection", err)
	}
}

func TestImportIEEEManifestRejectsCaseVariantKeys(t *testing.T) {
	_, err := ImportIEEEManifest(Input{
		Path: "manifest.json",
		Content: []byte(`{
  "standard_id": "IEEE 802.1Q",
  "STANDARD_ID": "IEEE 802.1D",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [{"anchor": "8.6.8", "summary": "A concise permitted summary."}]
}`),
	})
	if err == nil || !strings.Contains(err.Error(), `unknown field "STANDARD_ID"`) {
		t.Fatalf("ImportIEEEManifest() error = %v, want exact-key rejection", err)
	}
}

func TestImportIEEEManifestRejectsCaseVariantClauseKeys(t *testing.T) {
	_, err := ImportIEEEManifest(Input{
		Path: "manifest.json",
		Content: []byte(`{
  "standard_id": "IEEE 802.1Q",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [{"anchor": "8.6.8", "SUMMARY": "A concise permitted summary."}]
}`),
	})
	if err == nil || !strings.Contains(err.Error(), `unknown field "SUMMARY"`) {
		t.Fatalf("ImportIEEEManifest() error = %v, want exact nested-key rejection", err)
	}
}

func TestImportIEEEManifestUnknownFieldErrorIsDeterministic(t *testing.T) {
	input := Input{Path: "manifest.json", Content: []byte(`{
  "A_extra": true,
  "B_extra": true,
  "standard_id": "IEEE 802.1Q",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [{"anchor": "8.6.8", "summary": "A concise permitted summary."}]
}`)}
	for range 20 {
		_, err := ImportIEEEManifest(input)
		if err == nil || !strings.Contains(err.Error(), `unknown field "A_extra"`) {
			t.Fatalf("ImportIEEEManifest() error = %v, want deterministic first unknown field", err)
		}
	}
}

func TestImportIEEEManifestRejectsUnsafeOrDuplicateAnchors(t *testing.T) {
	for _, clauses := range []string{
		`[{"anchor": "8.6.8\n", "summary": "A concise permitted summary."}]`,
		`[{"anchor": "8.6.8", "summary": "One."}, {"anchor": " 8.6.8", "summary": "Two."}]`,
	} {
		t.Run(clauses, func(t *testing.T) {
			_, err := ImportIEEEManifest(Input{Path: "manifest.json", Content: []byte(`{
  "standard_id": "IEEE 802.1Q",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": ` + clauses + `
}`)})
			if err == nil {
				t.Fatal("ImportIEEEManifest() accepted an unsafe or duplicate anchor")
			}
		})
	}
}

func TestStandardsImportsRejectInvalidUTF8(t *testing.T) {
	_, err := ImportRFC(RFCInput{
		Artifact:  Input{Path: "rfc-9000.txt", Content: []byte{'R', 'F', 'C', 0xff}},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "content is not valid UTF-8") {
		t.Fatalf("ImportRFC() error = %v, want invalid UTF-8 rejection", err)
	}
}

func TestImportIEEEManifestRejectsOverlongMetadata(t *testing.T) {
	standardID := strings.Repeat("x", maxStandardIDRunes+1)
	_, err := ImportIEEEManifest(Input{
		Path: "manifest.json",
		Content: []byte(`{
  "standard_id": "` + standardID + `",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [{"anchor": "8.6.8", "summary": "A concise permitted summary."}]
}`),
	})
	if err == nil || !strings.Contains(err.Error(), "standard_id exceeds") {
		t.Fatalf("ImportIEEEManifest() error = %v, want metadata length rejection", err)
	}
}

func TestStandardsImportsRejectOversizedContentAndUnsafePaths(t *testing.T) {
	for _, test := range []struct {
		input Input
		want  string
	}{
		{Input{Path: "manifest.json", Content: []byte(strings.Repeat("x", maxArtifactBytes+1))}, "content exceeds"},
		{Input{Path: "/tmp/manifest.json", Content: []byte("{}")}, "path must be a clean relative logical anchor"},
		{Input{Path: "../manifest.json", Content: []byte("{}")}, "path must be a clean relative logical anchor"},
		{Input{Path: "a#fragment/manifest.json", Content: []byte("{}")}, "path must be a clean relative logical anchor"},
		{Input{Path: "C:/manifest.json", Content: []byte("{}")}, "path must be a clean relative logical anchor"},
		{Input{Path: ".", Content: []byte("{}")}, "path must be a clean relative logical anchor"},
	} {
		_, err := ImportIEEEManifest(test.input)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("ImportIEEEManifest(%q) error = %v, want %q", test.input.Path, err, test.want)
		}
	}
}

func TestStandardsImportsRejectWhitespacePaddedLogicalPaths(t *testing.T) {
	for _, path := range []string{" manifest.json", "manifest.json ", "\tmanifest.json", "manifest.json\n"} {
		t.Run(path, func(t *testing.T) {
			_, err := ImportRFC(RFCInput{
				Artifact:  Input{Path: path, Content: []byte("Request for Comments: 9000\nCategory: Informational\n")},
				SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt",
			})
			if err == nil || !strings.Contains(err.Error(), "path must be a clean relative logical anchor") {
				t.Fatalf("ImportRFC() error = %v, want path rejection", err)
			}
			_, err = ImportIEEEManifest(Input{Path: path, Content: []byte(`{"standard_id":"IEEE 802.1Q","edition":"2022","document_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","clauses":[{"anchor":"8.6.8","summary":"A concise permitted summary."}]}`)})
			if err == nil || !strings.Contains(err.Error(), "path must be a clean relative logical anchor") {
				t.Fatalf("ImportIEEEManifest() error = %v, want path rejection", err)
			}
		})
	}
}

func TestImportIEEEManifestRejectsFailClosedBoundaries(t *testing.T) {
	const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const summary = "A concise permitted summary."
	valid := `{"standard_id":"IEEE 802.1Q","edition":"2022","document_sha256":"` + digest + `","clauses":[{"anchor":"8.6.8","summary":"` + summary + `"}]}`
	for _, test := range []struct {
		name    string
		content string
	}{
		{"trailing JSON", valid + ` null`},
		{"uppercase hash", strings.Replace(valid, digest, strings.ToUpper(digest), 1)},
		{"illegal anchor", strings.Replace(valid, `"8.6.8"`, `"8/6/8"`, 1)},
		{"overlong edition", strings.Replace(valid, `"2022"`, `"`+strings.Repeat("x", maxEditionRunes+1)+`"`, 1)},
		{"overlong summary", strings.Replace(valid, summary, strings.Repeat("x", maxClauseSummaryRunes+1), 1)},
		{"document URL", strings.Replace(valid, `,"clauses"`, `,"document_url":"https://example.invalid/document.pdf","clauses"`, 1)},
		{"PDF field", strings.Replace(valid, `,"clauses"`, `,"pdf":"document.pdf","clauses"`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ImportIEEEManifest(Input{Path: "manifest.json", Content: []byte(test.content)})
			if err == nil {
				t.Fatal("ImportIEEEManifest() accepted a fail-closed boundary violation")
			}
		})
	}

	tooDeep := strings.Repeat("[", maxJSONNestingDepth+2) + "0" + strings.Repeat("]", maxJSONNestingDepth+2)
	if _, err := ImportIEEEManifest(Input{Path: "manifest.json", Content: []byte(tooDeep)}); err == nil {
		t.Fatal("ImportIEEEManifest() accepted excessive JSON nesting")
	}
}

func TestStandardsImportsRejectExcessiveCardinality(t *testing.T) {
	clauses := strings.TrimSuffix(strings.Repeat(`{"anchor":"8.6.8","summary":"A concise permitted summary."},`, maxIEEEClauses+1), ",")
	_, err := ImportIEEEManifest(Input{Path: "manifest.json", Content: []byte(`{
  "standard_id": "IEEE 802.1Q",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [` + clauses + `]
}`)})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("ImportIEEEManifest() error = %v, want clause-count rejection", err)
	}

	content := "Request for Comments: 9000\nCategory: Informational\n\nThe keywords MUST are to be interpreted as described in BCP 14.\n\n" + strings.Repeat("An implementation MUST preserve the source revision.\n", maxRFCCandidates+1)
	_, err = ImportRFC(RFCInput{
		Artifact:  Input{Path: "rfc-9000.txt", Content: []byte(content)},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "normative candidates") {
		t.Fatalf("ImportRFC() error = %v, want candidate-count rejection", err)
	}
}

func TestImportIEEEManifestPinsLicensedDocumentHash(t *testing.T) {
	const documentSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	document, err := ImportIEEEManifest(Input{
		Path: "manifest.json",
		Content: []byte(`{
  "standard_id": "IEEE 802.1Q",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [{"anchor": "8.6.8", "summary": "A concise permitted summary."}]
}`),
	})
	if err != nil {
		t.Fatalf("ImportIEEEManifest() error = %v", err)
	}
	if err := trace.Validate(document); err != nil {
		t.Fatalf("trace.Validate() error = %v", err)
	}
	if len(document.Nodes) != 2 || len(document.Edges) != 1 || len(document.Evidence) != 3 {
		t.Fatalf("document shape = nodes:%d edges:%d evidence:%d, want 2:1:3", len(document.Nodes), len(document.Edges), len(document.Evidence))
	}
	for _, evidence := range document.Evidence {
		if evidence.Level != trace.LicensedClauseReference {
			t.Fatalf("evidence level = %q, want %q", evidence.Level, trace.LicensedClauseReference)
		}
		if evidence.Provenance.ContentSHA256 != documentSHA256 {
			t.Fatalf("evidence content SHA-256 = %q, want %q", evidence.Provenance.ContentSHA256, documentSHA256)
		}
	}
}

func TestImportIEEEManifestPinsManifestDigestWithSourceRevision(t *testing.T) {
	input := Input{Path: "manifest.json", SourceRevision: "git:abc", Content: []byte(`{
  "standard_id": "IEEE 802.1Q",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [{"anchor": "8.6.8", "summary": "A concise permitted summary."}]
}`)}
	document, err := ImportIEEEManifest(input)
	if err != nil {
		t.Fatalf("ImportIEEEManifest() error = %v", err)
	}
	want := "manifest-sha256:" + contentSHA256(input.Content) + ";rev=git:abc"
	for _, evidence := range document.Evidence {
		if evidence.Provenance.SourceRevision != want {
			t.Fatalf("source revision = %q, want %q", evidence.Provenance.SourceRevision, want)
		}
	}
}

func TestImportIEEEManifestRejectsReservedSourceRevisionMarker(t *testing.T) {
	_, err := ImportIEEEManifest(Input{Path: "manifest.json", SourceRevision: "manifest-sha256:abc", Content: []byte(`{
  "standard_id": "IEEE 802.1Q",
  "edition": "2022",
  "document_sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "clauses": [{"anchor": "8.6.8", "summary": "A concise permitted summary."}]
}`)})
	if err == nil || !strings.Contains(err.Error(), "source revision") {
		t.Fatalf("ImportIEEEManifest() error = %v, want reserved source revision rejection", err)
	}
}

func TestRFCUpdatesValueRejectsMalformedAndUnboundedLists(t *testing.T) {
	for _, value := range []string{
		"1111  2222",
		strings.Repeat("1,", maxRFCUpdates) + "1",
		strings.Repeat("1", maxRFCNumberRunes+1),
		"0001",
	} {
		if _, ok := rfcUpdatesValue(value); ok {
			t.Fatalf("rfcUpdatesValue(%q) accepted malformed or unbounded value", value)
		}
	}
}

func TestImportRFCRejectsUpdatesThatExceedTheHeaderWideLimit(t *testing.T) {
	firstLine := strings.TrimSuffix(strings.Repeat("1,", maxRFCUpdates-1), ",") + ","
	content := []byte("Request for Comments: 9009\nUpdates: " + firstLine + "\n  2, 3\nCategory: Informational\n")
	_, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9009.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9009.txt"})
	if err == nil || !strings.Contains(err.Error(), "malformed Updates") {
		t.Fatalf("ImportRFC() error = %v, want malformed Updates header", err)
	}
}

func TestImportRFCRejectsUnexpectedIndentedUpdatesContinuation(t *testing.T) {
	content := []byte("Request for Comments: 9010\nUpdates: 2119\n  2222, 3333\nCategory: Informational\n")
	_, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9010.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9010.txt"})
	if err == nil || !strings.Contains(err.Error(), "malformed Updates") {
		t.Fatalf("ImportRFC() error = %v, want malformed Updates header", err)
	}
}

func TestImportRFCRejectsNonnumericWrappedUpdatesContinuation(t *testing.T) {
	content := []byte("Request for Comments: 9010\nUpdates: 2119,\n  not a number\nCategory: Informational\n")
	_, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9010.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9010.txt"})
	if err == nil || !strings.Contains(err.Error(), "malformed Updates") {
		t.Fatalf("ImportRFC() error = %v, want malformed Updates header", err)
	}
}

func TestImportRFCAcceptsCRLineEndings(t *testing.T) {
	content := []byte("Request for Comments: 9011\rCategory: Informational\r\rThe key words in this document are to be interpreted as described in RFC 8174.\r\rAn implementation MUST preserve CR line endings.\r")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9011.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9011.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	candidates := 0
	for _, evidence := range document.Evidence {
		if evidence.Level == trace.NormativeCandidate {
			candidates++
		}
	}
	if candidates != 2 {
		t.Fatalf("normative evidence records = %d, want 2 for one requirement and its edge", candidates)
	}
}

func TestImportRFCAcceptsCRLFLineEndings(t *testing.T) {
	content := []byte("Request for Comments: 9011\r\nCategory: Informational\r\n\r\nThe key words in this document are to be interpreted as described in RFC 8174.\r\n\r\nAn implementation MUST preserve CRLF line endings.\r\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9011.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9011.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got := normativeCandidates(document); got != 1 {
		t.Fatalf("normative candidate count = %d, want 1", got)
	}
}

func TestImportRFCRejectsUnsafeCandidateText(t *testing.T) {
	for _, candidate := range []string{
		"An implementation MUST preserve \u202eunsafe text.",
		"\vAn implementation MUST preserve a revision.",
	} {
		content := []byte("Request for Comments: 9011\nCategory: Informational\n\nThe key words in this document are to be interpreted as described in RFC 8174.\n\n" + candidate + "\n")
		_, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9011.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9011.txt"})
		if err == nil || !strings.Contains(err.Error(), "candidate line contains unsafe characters") {
			t.Fatalf("ImportRFC() error = %v, want unsafe candidate rejection", err)
		}
	}
}

func TestImportRFCAcceptsTabsInCandidateLines(t *testing.T) {
	content := []byte("Request for Comments: 9013\nCategory: Informational\n\nThe key words in this document are to be interpreted as described in RFC 8174.\n\nAn implementation MUST\tpreserve tab-separated source text.\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9013.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9013.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	for _, node := range document.Nodes {
		if node.Kind == trace.Requirement && strings.Contains(node.Label, "MUST preserve") {
			return
		}
	}
	t.Fatal("ImportRFC() did not preserve the tab-normalized candidate")
}

func TestImportRFCAcceptsRightHandHeaderColumnAfterUpdates(t *testing.T) {
	content := []byte("Request for Comments: 9014\nUpdates: 2119\n                                                  G. Camarillo\nCategory: Informational\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9014.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9014.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if document.Nodes[0].Label != "RFC 9014, category Informational, updates 2119" {
		t.Fatalf("root label = %q", document.Nodes[0].Label)
	}
}

func TestImportRFCAcceptsRightHandDateColumnAfterUpdates(t *testing.T) {
	content := []byte("Request for Comments: 9014\nUpdates: 2119\n                                                  1 April 2026\nCategory: Informational\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9014.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9014.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if document.Nodes[0].Label != "RFC 9014, category Informational, updates 2119" {
		t.Fatalf("root label = %q", document.Nodes[0].Label)
	}
}

func TestImportRFCAcceptsSameLineRightHandColumnsAfterUpdates(t *testing.T) {
	for _, column := range []string{"1 April 2026", "3GPP"} {
		t.Run(column, func(t *testing.T) {
			content := []byte("Request for Comments: 9014\nUpdates: 2119                                                  " + column + "\nCategory: Informational\n")
			document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9014.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9014.txt"})
			if err != nil {
				t.Fatalf("ImportRFC() error = %v", err)
			}
			if document.Nodes[0].Label != "RFC 9014, category Informational, updates 2119" {
				t.Fatalf("root label = %q", document.Nodes[0].Label)
			}
		})
	}
}

func TestImportRFCAcceptsDateRightHandColumnAfterCommaTerminatedUpdates(t *testing.T) {
	content := []byte("Request for Comments: 9017\nUpdates: 1111,                              1 April 2026\nCategory: Informational\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9017.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9017.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got, want := document.Nodes[0].Label, "RFC 9017, category Informational, updates 1111"; got != want {
		t.Fatalf("root label = %q, want %q", got, want)
	}
}

func TestImportRFCRejectsAmbiguousNumericRightHandColumnAfterUpdates(t *testing.T) {
	content := []byte("Request for Comments: 9017\nUpdates: 1111,                              2026\nCategory: Informational\n")
	_, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9017.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9017.txt"})
	if err == nil || !strings.Contains(err.Error(), "malformed Updates") {
		t.Fatalf("ImportRFC() error = %v, want malformed Updates rejection", err)
	}
}

func TestImportRFCRejectsWrappedNumericRightHandColumnAfterUpdates(t *testing.T) {
	content := []byte("Request for Comments: 9025\nUpdates: 1111,\n         2222                            3333 4444\nCategory: Informational\n")
	_, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9025.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9025.txt"})
	if err == nil || !strings.Contains(err.Error(), "malformed Updates") {
		t.Fatalf("ImportRFC() error = %v, want malformed Updates rejection", err)
	}
}

func TestImportRFCKeepsAWrappedUpdatesListAfterADateColumn(t *testing.T) {
	content := []byte("Request for Comments: 9022\nUpdates: 1111,                              1 April 2026\n         2222\nCategory: Informational\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9022.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9022.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got, want := document.Nodes[0].Label, "RFC 9022, category Informational, updates 1111, 2222"; got != want {
		t.Fatalf("root label = %q, want %q", got, want)
	}
}

func TestImportRFCAcceptsReservedSourceRevisionMarker(t *testing.T) {
	document, err := ImportRFC(RFCInput{
		Artifact: Input{
			Path:           "rfc-9000.txt",
			SourceRevision: "manifest-sha256:abc",
			Content:        []byte("Request for Comments: 9000\nCategory: Informational\n"),
		},
		SourceURL: "https://www.rfc-editor.org/rfc/rfc9000.txt",
	})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got := document.Nodes[0].Provenance.SourceRevision; got != "manifest-sha256:abc" {
		t.Fatalf("source revision = %q, want RFC caller revision", got)
	}
}

func TestImportRFCDoesNotTreatAnUnrelatedRFCReferenceAsBCPDeclaration(t *testing.T) {
	content := []byte("Request for Comments: 9008\nCategory: Informational\n\nThe key words in the glossary are to be interpreted as described in Section 4; RFC 2119 is not used.\n\nAn implementation MUST reject this false declaration.\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9008.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9008.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	for _, evidence := range document.Evidence {
		if evidence.Level == trace.NormativeCandidate {
			t.Fatalf("ImportRFC() emitted false normative candidate %q", evidence.ID)
		}
	}
}

func TestImportRFCExcludesTheBCPDeclarationParagraph(t *testing.T) {
	content := []byte("Request for Comments: 9008\nCategory: Informational\n\nThe key words in this document are to be interpreted as described in RFC 8174. An implementation MUST not become a candidate in the declaration paragraph.\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9008.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9008.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got := normativeCandidates(document); got != 0 {
		t.Fatalf("normative candidate count = %d, want 0", got)
	}
}

func TestImportRFCDoesNotTreatHeaderTextAsABCPDeclaration(t *testing.T) {
	content := []byte("Request for Comments: 9015\nCategory: Informational\nKeywords: key words are to be interpreted as described in BCP 14\n\nAn implementation MUST not be authorized by header text.\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9015.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9015.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got := normativeCandidates(document); got != 0 {
		t.Fatalf("normative candidate count = %d, want 0", got)
	}
}

func TestImportRFCDoesNotTreatNegatedBCPDeclarationAsAffirmative(t *testing.T) {
	content := []byte("Request for Comments: 9023\nCategory: Informational\n\nIt is false that the key words \"MUST\" are to be interpreted as described in RFC 8174.\n\nAn implementation MUST not become a candidate.\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9023.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9023.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got := normativeCandidates(document); got != 0 {
		t.Fatalf("normative candidate count = %d, want 0", got)
	}
}

func TestImportRFCDoesNotTreatBCPTemplateAsAnAffirmativeDeclaration(t *testing.T) {
	content := []byte("Request for Comments: 9026\nCategory: Informational\n\nAuthors may incorporate this phrase:\n\nThe key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\nAn implementation MUST not become a candidate.\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9026.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9026.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got := normativeCandidates(document); got != 0 {
		t.Fatalf("normative candidate count = %d, want 0", got)
	}
}

func TestImportRFCDoesNotTreatQuotedBCPDeclarationAsAffirmative(t *testing.T) {
	for _, test := range []struct {
		name   string
		leadIn string
	}{
		{name: "unbracketed RFC 2119", leadIn: "RFC 2119 states:"},
		{name: "bracketed RFC 2119", leadIn: "[RFC2119] states:"},
		{name: "bracketed arbitrary RFC", leadIn: "[RFC9000] says:"},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := []byte("Request for Comments: 9027\nCategory: Informational\n\n" + test.leadIn + "\n\nThe key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\nAn implementation MUST not become a candidate.\n")
			document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9027.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9027.txt"})
			if err != nil {
				t.Fatalf("ImportRFC() error = %v", err)
			}
			if got := normativeCandidates(document); got != 0 {
				t.Fatalf("normative candidate count = %d, want 0", got)
			}
		})
	}
}

func TestImportRFCKeepsRFCBodyProseAndOperationalTextAsCandidates(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{
			name: "RFC body prose",
			body: "RFC 9000 endpoints MUST discard such packets.\n",
		},
		{
			name: "RFC body prose with a column-sized space gap",
			body: "RFC 9000  endpoints MUST discard such packets.\n",
		},
		{
			name: "RFC body prose with multiple column-sized space gaps",
			body: "RFC 9000  endpoints MUST discard such packets.  Clients MUST validate the source.\n",
		},
		{
			name: "page-number body prose",
			body: "Servers MUST return page 1\n",
		},
		{
			name: "include this text operational requirement",
			body: "Clients MUST include this text in the Reason header.\n",
		},
		{
			name: "use following text operational requirement",
			body: "Senders MUST use the following text encoding: UTF-8.\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := []byte("Request for Comments: 9028\nCategory: Informational\n\nThe key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" + test.body)
			document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9028.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9028.txt"})
			if err != nil {
				t.Fatalf("ImportRFC() error = %v", err)
			}
			if got := normativeCandidates(document); got != 1 {
				t.Fatalf("normative candidate count = %d, want 1", got)
			}
		})
	}
}

func TestImportRFCKeepsUppercaseTemplateLikeOperationalTextAsCandidates(t *testing.T) {
	content := []byte("Request for Comments: 9052\nCategory: Informational\n\nThe key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\nEditors SHOULD include this text in the Reason header.\nClients MUST validate the source.\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9052.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9052.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got := normativeCandidates(document); got != 2 {
		t.Fatalf("normative candidate count = %d, want 2", got)
	}
}

func TestImportRFCHandlesBCPTemplateDeclarationBoundaries(t *testing.T) {
	for _, test := range []struct {
		name           string
		body           string
		wantCandidates int
	}{
		{
			name: "two blank lines before template boilerplate",
			body: "Authors may incorporate this phrase:\n\n\n" +
				"The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"An implementation MUST not become a candidate.\n",
			wantCandidates: 0,
		},
		{
			name: "relative clause before template instruction",
			body: "Authors who follow these guidelines should incorporate this phrase:\n\n" +
				"The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"An implementation MUST not become a candidate.\n",
			wantCandidates: 0,
		},
		{
			name: "template instruction after ordinary sentence",
			body: "This introduction is not a declaration. Authors may incorporate this phrase:\n\n" +
				"The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"An implementation MUST not become a candidate.\n",
			wantCandidates: 0,
		},
		{
			name: "genuine declaration after template text",
			body: "Authors may incorporate this phrase:\n\n" +
				"The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"This paragraph separates the template from this document's declaration.\n\n" +
				"The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"An implementation MUST become a candidate.\n",
			wantCandidates: 1,
		},
		{
			name: "template instruction and boilerplate in one paragraph",
			body: "Authors may incorporate this phrase. The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"An implementation MUST not become a candidate.\n",
			wantCandidates: 0,
		},
		{
			name: "page footer between template instruction and boilerplate",
			body: "Authors may incorporate this phrase:\n\n" +
				"Example Author                                                [Page 2]\n\n" +
				"The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"An implementation MUST not become a candidate.\n",
			wantCandidates: 0,
		},
		{
			name: "replacement quotation",
			body: "OLD:\n\n" +
				"The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"An implementation MUST not become a candidate.\n",
			wantCandidates: 0,
		},
		{
			name: "page footer and running header before template boilerplate",
			body: "Authors may incorporate this phrase:\n\n" +
				"Example Author                                                [Page 2]\n\f\n" +
				"RFC 9035                 Example Protocol              October 2026\n\n" +
				"The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"An implementation MUST not become a candidate.\n",
			wantCandidates: 0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := []byte("Request for Comments: 9029\nCategory: Informational\n\n" + test.body)
			document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9029.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9029.txt"})
			if err != nil {
				t.Fatalf("ImportRFC() error = %v", err)
			}
			if got := normativeCandidates(document); got != test.wantCandidates {
				t.Fatalf("normative candidate count = %d, want %d", got, test.wantCandidates)
			}
		})
	}
}

func TestImportRFCExcludesQuotedBCPParagraphsFromCandidates(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{
			name: "template declaration after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"Authors may incorporate this phrase. The key words \"MUST\" in this document are to be interpreted as described in RFC 8174. This MUST not become a candidate.\n",
		},
		{
			name: "replacement requirement after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"OLD:\n\n" +
				"A quoted requirement MUST not become a candidate.\n",
		},
		{
			name: "RFC-like replacement requirement after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"OLD:\n\n" +
				"RFC 2119 senders MUST not become a candidate.\n",
		},
		{
			name: "multi-paragraph replacement requirement after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"OLD:\n\n" +
				"First replacement paragraph.\n\n" +
				"Another replacement requirement MUST not become a candidate.\n",
		},
		{
			name: "inline replacement requirement after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"OLD: An inline replacement requirement MUST not become a candidate.\n",
		},
		{
			name: "replacement requirement after ordinary prose",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"Section change. OLD: A quoted requirement MUST not become a candidate.\n",
		},
		{
			name: "extended replacement label after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"New text:\n\n" +
				"Senders MUST reject the value.\n",
		},
		{
			name: "RFC quotation lead-in after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"RFC 2119 states:\n\n" +
				"Senders MUST reject the value.\n",
		},
		{
			name: "RFC quotation lead-in with declaration after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"The key words \"MUST\" are to be interpreted as described in RFC 8174. RFC 2119 says:\n\n" +
				"A quoted requirement MUST not become a candidate.\n",
		},
		{
			name: "running header after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"RFC 9999   TLS MUST-Staple   May 2026\n",
		},
		{
			name: "RFC-like template payload after genuine declaration",
			body: "The key words \"MUST\" in this document are to be interpreted as described in RFC 8174.\n\n" +
				"A genuine requirement MUST become a candidate.\n\n" +
				"Authors may incorporate this phrase:\n\n" +
				"RFC 2119 template payload MUST not become a candidate.\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := []byte("Request for Comments: 9036\nCategory: Informational\n\n" + test.body)
			document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9036.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9036.txt"})
			if err != nil {
				t.Fatalf("ImportRFC() error = %v", err)
			}
			if got := normativeCandidates(document); got != 1 {
				t.Fatalf("normative candidate count = %d, want 1", got)
			}
		})
	}
}

func TestImportRFCIgnoresWhitespaceOnlyUpdatesContinuation(t *testing.T) {
	content := []byte("Request for Comments: 9032\nCategory: Informational\nUpdates: 2119\n   \n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9032.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9032.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	if got, want := document.Nodes[0].Label, "RFC 9032, category Informational, updates 2119"; got != want {
		t.Fatalf("root label = %q, want %q", got, want)
	}
}

func TestImportRFCRejectsAmbiguousMandatoryHeaderColumns(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		source  string
		want    string
	}{
		{
			name:    "request number",
			content: "Request for Comments: 9027  9028\nCategory: Informational\n",
			source:  "9027",
			want:    "Request for Comments number",
		},
		{
			name:    "category",
			content: "Request for Comments: 9028\nCategory: Informational   Historic\n",
			source:  "9028",
			want:    "publication category",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ImportRFC(RFCInput{
				Artifact:  Input{Path: "rfc-" + test.source + ".txt", Content: []byte(test.content)},
				SourceURL: "https://www.rfc-editor.org/rfc/rfc" + test.source + ".txt",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ImportRFC() error = %v, want %q rejection", err, test.want)
			}
		})
	}
}

func TestImportRFCRejectsNonCanonicalMandatoryHeaderWhitespace(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "extra space after category separator",
			content: "Request for Comments: 9038\nCategory:    Informational\n",
			want:    "publication category",
		},
		{
			name:    "trailing category whitespace",
			content: "Request for Comments: 9038\nCategory: Informational   \n",
			want:    "publication category",
		},
		{
			name:    "missing request number separator space",
			content: "Request for Comments:9038\nCategory: Informational\n",
			want:    "Request for Comments number",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ImportRFC(RFCInput{
				Artifact:  Input{Path: "rfc-9038.txt", Content: []byte(test.content)},
				SourceURL: "https://www.rfc-editor.org/rfc/rfc9038.txt",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ImportRFC() error = %v, want %q rejection", err, test.want)
			}
		})
	}
}

func TestImportRFCRejectsUnicodeWhitespaceInRFCMetadata(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "non-breaking space in request-number column boundary",
			content: "Request for Comments: 9038\u00a0   Example Org\nCategory: Informational\n",
			want:    "Request for Comments",
		},
		{
			name:    "em space in category column boundary",
			content: "Request for Comments: 9038\nCategory: Informational\u2003   Example Org\n",
			want:    "publication category",
		},
		{
			name:    "non-breaking space in Updates continuation",
			content: "Request for Comments: 9038\nCategory: Informational\nUpdates: 2119,\n \u00a02222\n",
			want:    "Updates",
		},
		{
			name:    "non-breaking space at end of Updates value",
			content: "Request for Comments: 9038\nCategory: Informational\nUpdates: 2119\u00a0\n",
			want:    "Updates",
		},
		{
			name:    "em space at end of Updates value",
			content: "Request for Comments: 9038\nCategory: Informational\nUpdates: 2119\u2003\n",
			want:    "Updates",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ImportRFC(RFCInput{
				Artifact: Input{
					Path:    "rfc-9038.txt",
					Content: []byte(test.content),
				},
				SourceURL: "https://www.rfc-editor.org/rfc/rfc9038.txt",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ImportRFC() error = %v, want %q rejection", err, test.want)
			}
		})
	}
}

func TestImportRFCAcceptsDateRightHandColumnsInMandatoryHeaders(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
	}{
		{"request number", "Request for Comments: 9030  1 April 2026\nCategory: Informational\n"},
		{"category", "Request for Comments: 9030\nCategory: Informational  1 April 2026\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ImportRFC(RFCInput{
				Artifact:  Input{Path: "rfc-9030.txt", Content: []byte(test.content)},
				SourceURL: "https://www.rfc-editor.org/rfc/rfc9030.txt",
			})
			if err != nil {
				t.Fatalf("ImportRFC() error = %v", err)
			}
		})
	}
}

func TestImportIEEEManifestRejectsWhitespacePaddedMetadata(t *testing.T) {
	const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	valid := `{"standard_id":"IEEE 802.1Q","edition":"2022","document_sha256":"` + digest + `","clauses":[{"anchor":"8.6.8","summary":"A concise permitted summary."}]}`
	for _, test := range []struct {
		name    string
		content string
	}{
		{"standard id", strings.Replace(valid, `"IEEE 802.1Q"`, `" IEEE 802.1Q"`, 1)},
		{"edition", strings.Replace(valid, `"2022"`, `"2022 "`, 1)},
		{"anchor", strings.Replace(valid, `"8.6.8"`, `" 8.6.8"`, 1)},
		{"summary", strings.Replace(valid, `"A concise permitted summary."`, `"A concise permitted summary. "`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ImportIEEEManifest(Input{Path: "manifest.json", Content: []byte(test.content)})
			if err == nil || !strings.Contains(err.Error(), "must not have surrounding whitespace") {
				t.Fatalf("ImportIEEEManifest() error = %v, want surrounding-whitespace rejection", err)
			}
		})
	}
}

func TestImportRFCDoesNotJoinSentencesIntoABCPDeclaration(t *testing.T) {
	content := []byte("Request for Comments: 9012\nCategory: Informational\n\nThis memo discusses key words. Error codes are to be interpreted as described in RFC 2119.\n\nAn implementation MUST reject this false declaration.\n")
	document, err := ImportRFC(RFCInput{Artifact: Input{Path: "rfc-9012.txt", Content: content}, SourceURL: "https://www.rfc-editor.org/rfc/rfc9012.txt"})
	if err != nil {
		t.Fatalf("ImportRFC() error = %v", err)
	}
	for _, evidence := range document.Evidence {
		if evidence.Level == trace.NormativeCandidate {
			t.Fatalf("ImportRFC() emitted false normative candidate %q", evidence.ID)
		}
	}
}

func normativeCandidates(document *trace.Document) int {
	count := 0
	for _, evidence := range document.Evidence {
		if evidence.Level == trace.NormativeCandidate && strings.HasPrefix(evidence.Subject, "node:") {
			count++
		}
	}
	return count
}
