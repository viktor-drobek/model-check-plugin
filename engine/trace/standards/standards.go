// Package standards imports offline standards artefacts into trace-ledger facts.
//
// The adapters operate only on bytes supplied by their caller. They neither
// fetch documents nor open the supplied logical path. Their evidence describes
// the imported artefact and never establishes implementation correctness.
package standards

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"modelcheck/trace"
)

const (
	rfcAdapterName     = "mcd-rfc-text-adapter"
	rfcAdapterVersion  = "1"
	ieeeAdapterName    = "mcd-ieee-clause-manifest-adapter"
	ieeeAdapterVersion = "1"

	maxArtifactBytes       = 1 << 20
	maxJSONNestingDepth    = 64
	maxIEEEClauses         = 1000
	maxRFCCandidates       = 10000
	maxRFCNumberRunes      = 10
	maxRFCCategoryRunes    = 100
	maxRFCUpdates          = 100
	maxStandardIDRunes     = 200
	maxEditionRunes        = 100
	maxClauseAnchorRunes   = 200
	maxClauseSummaryRunes  = 500
	maxSourceRevisionRunes = 200
	maxLogicalPathRunes    = 512
	maxSourceURLRunes      = 2048
)

var (
	rfcNumberRE           = regexp.MustCompile(`^[1-9][0-9]*$`)
	rfcColumnSplitRE      = regexp.MustCompile(`[ \t]{2,}`)
	rfcUpdatesRE          = regexp.MustCompile(`^([0-9]+(?:[ \t]{0,2},[ \t]{0,2}[0-9]+)*(?:[ \t]{0,2},)?)`)
	rfcUpdatesListRE      = regexp.MustCompile(`^[0-9]+(?:[ \t]*,[ \t]*[0-9]+)*(?:[ \t]*,)?$`)
	rfcUpdatesTokenRE     = regexp.MustCompile(`^[0-9]+(?:[ \t]*,|[ \t]*$)`)
	rfcDateColumnRE       = regexp.MustCompile(`^[0-9]{1,2}[ \t]+[A-Za-z]+[ \t]+[0-9]{4}$`)
	logicalPathRE         = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
	rfcSourceURLRE        = regexp.MustCompile(`^https://www\.(?:rfc-editor\.org|ietf\.org)/rfc/rfc[1-9][0-9]*\.txt$`)
	bcpDeclarationRE      = regexp.MustCompile(`(?i)(?:^|[.;?!]\s+)(?:the\s+)?(?:key\s+words?|keywords)\b([^.;?!:]*)\bare\s+to\s+be\s+interpreted\s+as\s+described\s+in\s+(?:BCP\s*14\b|\[RFC\s*(?:2119|8174)\]|RFC\s*(?:2119|8174)\b)`)
	bcpQualifierRE        = regexp.MustCompile(`^(?:(?:[ \t]+)|(?:,|;)|(?:"[^"]*"|'[^']*'|“[^”]*”)|(?i:\b(?:MUST(?: NOT)?|REQUIRED|SHALL(?: NOT)?|SHOULD(?: NOT)?|NOT RECOMMENDED|RECOMMENDED|MAY|OPTIONAL|and|or|in|this|document)\b))*$`)
	bcpTemplateRE         = regexp.MustCompile(`(?i)(?:^|[.;?!]\s+)(?:authors?|editors?)(?:\s+(?:who|that)\b[^.!?]*?)?\s+(?-i:may|can|should)\s+(?:incorporate|include|use)\s+(?:this|the\s+following)\s+(?:phrase|text|boilerplate)\b`)
	bcpReplacementLabelRE = regexp.MustCompile(`(?i)(?:^|[.;?!]\s+)(?:old|new)(?:\s+text)?\s*:` + `|(?:^|[.;?!]\s+)(?:replacement|quoted(?:\s+text)?)\s*:`)
	bcpQuotationLeadInRE  = regexp.MustCompile(`(?i)(?:^|[^[:alnum:]_])(?:\[\s*)?(?:BCP\s*14|RFC\s*[1-9][0-9]*)(?:\s*\])?\s+(?:states|says|reads|defines|specifies)\s*:`)
	rfcPageFooterRE       = regexp.MustCompile(`^\S(?:.*\S)?[ \t]{2,}\[Page [1-9][0-9]*\]$`)
	rfcRunningHeaderRE    = regexp.MustCompile(`^(?:RFC\s+[1-9][0-9]*|Internet-Draft|draft-[a-z0-9._-]+)[ \t]{2,}\S(?:.*\S)?[ \t]{2,}(?:January|February|March|April|May|June|July|August|September|October|November|December) [0-9]{4}$`)
	bcpKeywordRE          = regexp.MustCompile(`\b(?:MUST NOT|SHALL NOT|SHOULD NOT|NOT RECOMMENDED|MUST|REQUIRED|SHALL|SHOULD|RECOMMENDED|MAY|OPTIONAL)\b`)
	ieeeAnchorRE          = regexp.MustCompile(`^[A-Za-z0-9._()\-]+$`)
	sha256RE              = regexp.MustCompile(`^[0-9a-f]{64}$`)
	rfcCategories         = map[string]struct{}{
		"Best Current Practice": {},
		"Experimental":          {},
		"FYI":                   {},
		"Historic":              {},
		"Informational":         {},
		"Standards Track":       {},
	}
)

// Input is a caller-provided standards artefact. Path is a logical, stable
// review anchor; the importers never open it. Each adapter records a SHA-256
// digest of the supplied artefact. SourceRevision may retain the revision
// selected by a caller.
type Input struct {
	Path           string
	Content        []byte
	SourceRevision string
}

// RFCInput is a pinned RFC text artefact and its declared publisher URL.
// ImportRFC accepts only HTTPS RFC Editor or IETF URLs and performs no fetch.
type RFCInput struct {
	Artifact  Input
	SourceURL string
}

// ImportRFC imports one caller-supplied RFC text artefact. It records its RFC
// number, publication category, optional Updates field, content hash, source
// URL, and line anchors. BCP 14 terms are emitted only as
// normative_candidate evidence when the input itself declares RFC 2119 or RFC
// 8174 terminology; a human review is still required to identify a requirement.
func ImportRFC(input RFCInput) (*trace.Document, error) {
	artifact, err := normalizeInput(input.Artifact)
	if err != nil {
		return nil, fmt.Errorf("RFC input: %w", err)
	}
	sourceURL, err := validateRFCSourceURL(input.SourceURL)
	if err != nil {
		return nil, err
	}

	content := string(artifact.Content)
	lines := strings.Split(normalizeLineEndings(content), "\n")
	number, status, updates, bodyStart, err := parseRFCHeaders(lines)
	if err != nil {
		return nil, err
	}
	if err := validateRFCSourceURLNumber(sourceURL, number); err != nil {
		return nil, err
	}

	digest := contentSHA256(artifact.Content)
	prefix := digest[:12]
	label := fmt.Sprintf("RFC %s, category %s", number, status)
	if updates != "" {
		label += ", updates " + updates
	}
	anchor := rfcAnchor(sourceURL, artifact.Path, 1)
	provenance := newProvenance(artifact.SourceRevision, digest, anchor, rfcAdapterName, rfcAdapterVersion)
	standardID := fmt.Sprintf("rfc.%s.%s", number, prefix)
	standardEvidenceID := "evidence." + standardID
	document := &trace.Document{
		Schema: trace.Schema,
		Nodes: []trace.Node{{
			ID:         standardID,
			Kind:       trace.StandardClause,
			Label:      label,
			Provenance: provenance,
			Evidence:   []string{standardEvidenceID},
		}},
		Edges: []trace.Edge{},
		Evidence: []trace.Evidence{{
			ID:         standardEvidenceID,
			Subject:    "node:" + standardID,
			Level:      trace.Syntactic,
			Provenance: provenance,
		}},
		Gaps: []trace.Gap{},
	}

	bodyLines := lines[bodyStart:]
	nonCandidateLines, firstCandidateLine := bcp14NonCandidateLines(bodyLines)
	if firstCandidateLine == -1 {
		if err := trace.Validate(document); err != nil {
			return nil, fmt.Errorf("validate RFC trace ledger: %w", err)
		}
		return document, nil
	}

	candidateCount := 0
	for bodyLineNumber, line := range bodyLines {
		if bodyLineNumber < firstCandidateLine || nonCandidateLines[bodyLineNumber] {
			continue
		}
		lineNumber := bodyStart + bodyLineNumber
		if !bcpKeywordRE.MatchString(line) {
			continue
		}
		normalized := strings.ReplaceAll(line, "\t", " ")
		if containsUnsafeText(normalized) {
			return nil, fmt.Errorf("RFC text candidate line contains unsafe characters")
		}
		trimmed := strings.TrimSpace(normalized)
		if trimmed == "" {
			continue
		}
		candidateCount++
		if candidateCount > maxRFCCandidates {
			return nil, fmt.Errorf("RFC text exceeds %d normative candidates", maxRFCCandidates)
		}
		candidateID := fmt.Sprintf("rfc.%s.%s.candidate.%d", number, prefix, lineNumber+1)
		candidateEvidenceID := "evidence." + candidateID
		candidateProvenance := newProvenance(
			artifact.SourceRevision,
			digest,
			rfcAnchor(sourceURL, artifact.Path, lineNumber+1),
			rfcAdapterName,
			rfcAdapterVersion,
		)
		document.Nodes = append(document.Nodes, trace.Node{
			ID:         candidateID,
			Kind:       trace.Requirement,
			Label:      trimmed,
			Provenance: candidateProvenance,
			Evidence:   []string{candidateEvidenceID},
		})
		document.Evidence = append(document.Evidence, trace.Evidence{
			ID:         candidateEvidenceID,
			Subject:    "node:" + candidateID,
			Level:      trace.NormativeCandidate,
			Provenance: candidateProvenance,
		})
		edgeID := fmt.Sprintf("rfc.%s.%s.derived.%d", number, prefix, lineNumber+1)
		edgeEvidenceID := "evidence." + edgeID
		document.Edges = append(document.Edges, trace.Edge{
			ID:         edgeID,
			Kind:       trace.DerivedFrom,
			Source:     candidateID,
			Target:     standardID,
			Provenance: candidateProvenance,
			Evidence:   []string{edgeEvidenceID},
		})
		document.Evidence = append(document.Evidence, trace.Evidence{
			ID:         edgeEvidenceID,
			Subject:    "edge:" + edgeID,
			Level:      trace.NormativeCandidate,
			Provenance: candidateProvenance,
		})
	}
	if err := trace.Validate(document); err != nil {
		return nil, fmt.Errorf("validate RFC trace ledger: %w", err)
	}
	return document, nil
}

// ImportIEEEManifest imports a user-provided clause manifest. The manifest
// contains an IEEE standard identifier, edition, licensed document SHA-256,
// and concise clause summaries. It deliberately accepts neither an IEEE PDF
// nor a document URL.
func ImportIEEEManifest(input Input) (*trace.Document, error) {
	artifact, err := normalizeInput(input)
	if err != nil {
		return nil, fmt.Errorf("IEEE clause manifest input: %w", err)
	}
	if strings.Contains(artifact.SourceRevision, "manifest-sha256:") {
		return nil, fmt.Errorf("IEEE clause manifest input: source revision must not contain manifest-sha256:")
	}
	if err := rejectDuplicateKeys(artifact.Content); err != nil {
		return nil, fmt.Errorf("IEEE clause manifest: %w", err)
	}
	manifest, err := decodeIEEEManifest(artifact.Content)
	if err != nil {
		return nil, err
	}
	if err := manifest.validate(); err != nil {
		return nil, err
	}

	manifestDigest := contentSHA256(artifact.Content)
	prefix := manifestDigest[:12]
	revision := "manifest-sha256:" + manifestDigest
	if artifact.SourceRevision != "" {
		revision += ";rev=" + artifact.SourceRevision
	}
	document := &trace.Document{
		Schema:   trace.Schema,
		Nodes:    []trace.Node{},
		Edges:    []trace.Edge{},
		Evidence: []trace.Evidence{},
		Gaps:     []trace.Gap{},
	}
	for index, clause := range manifest.Clauses {
		clauseID := fmt.Sprintf("ieee.%s.clause.%d", prefix, index+1)
		requirementID := fmt.Sprintf("ieee.%s.requirement.%d", prefix, index+1)
		provenance := newProvenance(
			revision,
			manifest.DocumentSHA256,
			fmt.Sprintf("%s#clause:%s", artifact.Path, clause.Anchor),
			ieeeAdapterName,
			ieeeAdapterVersion,
		)
		clauseEvidenceID := "evidence." + clauseID
		requirementEvidenceID := "evidence." + requirementID
		edgeID := fmt.Sprintf("ieee.%s.derived.%d", prefix, index+1)
		edgeEvidenceID := "evidence." + edgeID
		document.Nodes = append(document.Nodes,
			trace.Node{
				ID:         clauseID,
				Kind:       trace.StandardClause,
				Label:      fmt.Sprintf("%s-%s, clause %s", manifest.StandardID, manifest.Edition, clause.Anchor),
				Provenance: provenance,
				Evidence:   []string{clauseEvidenceID},
			},
			trace.Node{
				ID:         requirementID,
				Kind:       trace.Requirement,
				Label:      clause.Summary,
				Provenance: provenance,
				Evidence:   []string{requirementEvidenceID},
			},
		)
		document.Edges = append(document.Edges, trace.Edge{
			ID:         edgeID,
			Kind:       trace.DerivedFrom,
			Source:     requirementID,
			Target:     clauseID,
			Provenance: provenance,
			Evidence:   []string{edgeEvidenceID},
		})
		document.Evidence = append(document.Evidence,
			trace.Evidence{
				ID:         clauseEvidenceID,
				Subject:    "node:" + clauseID,
				Level:      trace.LicensedClauseReference,
				Provenance: provenance,
			},
			trace.Evidence{
				ID:         requirementEvidenceID,
				Subject:    "node:" + requirementID,
				Level:      trace.LicensedClauseReference,
				Provenance: provenance,
			},
			trace.Evidence{
				ID:         edgeEvidenceID,
				Subject:    "edge:" + edgeID,
				Level:      trace.LicensedClauseReference,
				Provenance: provenance,
			},
		)
	}
	if err := trace.Validate(document); err != nil {
		return nil, fmt.Errorf("validate IEEE clause trace ledger: %w", err)
	}
	return document, nil
}

type ieeeManifest struct {
	StandardID     string       `json:"standard_id"`
	Edition        string       `json:"edition"`
	DocumentSHA256 string       `json:"document_sha256"`
	Clauses        []ieeeClause `json:"clauses"`
}

type ieeeClause struct {
	Anchor  string `json:"anchor"`
	Summary string `json:"summary"`
}

func (m *ieeeManifest) validate() error {
	if containsUnsafeText(m.StandardID) || containsUnsafeText(m.Edition) {
		return fmt.Errorf("IEEE clause manifest standard_id and edition must not contain control characters")
	}
	if hasSurroundingWhitespace(m.StandardID) || hasSurroundingWhitespace(m.Edition) {
		return fmt.Errorf("IEEE clause manifest standard_id and edition must not have surrounding whitespace")
	}
	if strings.TrimSpace(m.StandardID) == "" {
		return fmt.Errorf("IEEE clause manifest standard_id is blank")
	}
	if utf8.RuneCountInString(m.StandardID) > maxStandardIDRunes {
		return fmt.Errorf("IEEE clause manifest standard_id exceeds %d characters", maxStandardIDRunes)
	}
	if strings.TrimSpace(m.Edition) == "" {
		return fmt.Errorf("IEEE clause manifest edition is blank")
	}
	if utf8.RuneCountInString(m.Edition) > maxEditionRunes {
		return fmt.Errorf("IEEE clause manifest edition exceeds %d characters", maxEditionRunes)
	}
	if !sha256RE.MatchString(m.DocumentSHA256) {
		return fmt.Errorf("IEEE clause manifest document_sha256 must be 64 lowercase hexadecimal characters")
	}
	if len(m.Clauses) == 0 {
		return fmt.Errorf("IEEE clause manifest clauses is empty")
	}
	if len(m.Clauses) > maxIEEEClauses {
		return fmt.Errorf("IEEE clause manifest exceeds %d clauses", maxIEEEClauses)
	}
	m.StandardID = strings.TrimSpace(m.StandardID)
	m.Edition = strings.TrimSpace(m.Edition)
	anchors := make(map[string]struct{}, len(m.Clauses))
	for index := range m.Clauses {
		clause := &m.Clauses[index]
		path := fmt.Sprintf("IEEE clause manifest clauses[%d]", index)
		if containsUnsafeText(clause.Anchor) {
			return fmt.Errorf("%s anchor contains unsupported characters", path)
		}
		if hasSurroundingWhitespace(clause.Anchor) {
			return fmt.Errorf("%s anchor must not have surrounding whitespace", path)
		}
		clause.Anchor = strings.TrimSpace(clause.Anchor)
		if clause.Anchor == "" {
			return fmt.Errorf("%s anchor is blank", path)
		}
		if utf8.RuneCountInString(clause.Anchor) > maxClauseAnchorRunes {
			return fmt.Errorf("%s anchor exceeds %d characters", path, maxClauseAnchorRunes)
		}
		if !ieeeAnchorRE.MatchString(clause.Anchor) {
			return fmt.Errorf("%s anchor contains unsupported characters", path)
		}
		if _, exists := anchors[clause.Anchor]; exists {
			return fmt.Errorf("%s anchor %q is duplicated", path, clause.Anchor)
		}
		anchors[clause.Anchor] = struct{}{}
		if containsUnsafeText(clause.Summary) {
			return fmt.Errorf("%s summary must not contain control characters", path)
		}
		if hasSurroundingWhitespace(clause.Summary) {
			return fmt.Errorf("%s summary must not have surrounding whitespace", path)
		}
		clause.Summary = strings.TrimSpace(clause.Summary)
		if clause.Summary == "" {
			return fmt.Errorf("%s summary is blank", path)
		}
		if utf8.RuneCountInString(clause.Summary) > maxClauseSummaryRunes {
			return fmt.Errorf("%s summary exceeds %d characters", path, maxClauseSummaryRunes)
		}
	}
	return nil
}

func normalizeInput(input Input) (Input, error) {
	if containsUnsafeText(input.Path) || input.Path != strings.TrimSpace(input.Path) {
		return Input{}, fmt.Errorf("path must be a clean relative logical anchor")
	}
	if containsUnsafeText(input.SourceRevision) || hasSurroundingWhitespace(input.SourceRevision) || utf8.RuneCountInString(input.SourceRevision) > maxSourceRevisionRunes {
		return Input{}, fmt.Errorf("source revision must be at most %d safe characters", maxSourceRevisionRunes)
	}
	input.SourceRevision = strings.TrimSpace(input.SourceRevision)
	if input.Path == "" {
		return Input{}, fmt.Errorf("path is blank")
	}
	if input.Path == "." || utf8.RuneCountInString(input.Path) > maxLogicalPathRunes || path.IsAbs(input.Path) || input.Path == ".." || strings.HasPrefix(input.Path, "../") || path.Clean(input.Path) != input.Path || !logicalPathRE.MatchString(input.Path) {
		return Input{}, fmt.Errorf("path must be a clean relative logical anchor")
	}
	if len(input.Content) == 0 {
		return Input{}, fmt.Errorf("content is empty")
	}
	if len(input.Content) > maxArtifactBytes {
		return Input{}, fmt.Errorf("content exceeds %d bytes", maxArtifactBytes)
	}
	if !utf8.Valid(input.Content) {
		return Input{}, fmt.Errorf("content is not valid UTF-8")
	}
	return input, nil
}

func validateRFCSourceURL(raw string) (*url.URL, error) {
	if raw != strings.TrimSpace(raw) || containsUnsafeText(raw) || utf8.RuneCountInString(raw) > maxSourceURLRunes || !rfcSourceURLRE.MatchString(raw) {
		return nil, fmt.Errorf("RFC source URL must be a canonical RFC URL from HTTPS RFC Editor or IETF")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return nil, fmt.Errorf("RFC source URL must be a canonical RFC URL from HTTPS RFC Editor or IETF")
	}
	return parsed, nil
}

func validateRFCSourceURLNumber(sourceURL *url.URL, number string) error {
	const prefix = "/rfc/rfc"
	if sourceURL.RawPath != "" || !strings.HasPrefix(sourceURL.Path, prefix) || !strings.HasSuffix(sourceURL.Path, ".txt") {
		return fmt.Errorf("RFC source URL must use the canonical RFC URL for RFC %s", number)
	}
	sourceNumber := strings.TrimSuffix(strings.TrimPrefix(sourceURL.Path, prefix), ".txt")
	if !rfcNumberRE.MatchString(sourceNumber) {
		return fmt.Errorf("RFC source URL must use the canonical RFC URL for RFC %s", number)
	}
	if sourceNumber != number {
		return fmt.Errorf("RFC source URL does not identify RFC %s", number)
	}
	return nil
}

func parseRFCHeaders(lines []string) (string, string, string, int, error) {
	var number, category, updates string
	var numberSeen, categorySeen, updatesSeen, headerBlockStarted bool
	bodyStart := len(lines)
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		rawLine := lines[lineIndex]
		line := strings.TrimSuffix(rawLine, "\r")
		if strings.TrimSpace(line) == "" {
			if headerBlockStarted {
				bodyStart = lineIndex + 1
				break
			}
			continue
		}
		headerBlockStarted = true
		if value, column, ok := rfcHeaderValue(line, "Request for Comments:"); ok {
			if numberSeen {
				return "", "", "", 0, fmt.Errorf("RFC text declares Request for Comments more than once")
			}
			numberSeen = true
			if !rfcNumberRE.MatchString(value) || utf8.RuneCountInString(value) > maxRFCNumberRunes || ambiguousRFCHeaderColumn(column) {
				return "", "", "", 0, fmt.Errorf("RFC text has an invalid Request for Comments number")
			}
			number = value
		}
		if value, column, ok := rfcHeaderValue(line, "Category:"); ok {
			if categorySeen {
				return "", "", "", 0, fmt.Errorf("RFC text declares Category more than once")
			}
			categorySeen = true
			if !validRFCCategory(value) || utf8.RuneCountInString(value) > maxRFCCategoryRunes || ambiguousRFCHeaderColumn(column) {
				return "", "", "", 0, fmt.Errorf("RFC text does not declare a publication category")
			}
			category = value
		}
		if value, terminatesAtColumn, ok := rfcUpdatesHeaderValue(line); ok {
			if updatesSeen {
				return "", "", "", 0, fmt.Errorf("RFC text declares Updates more than once")
			}
			updatesSeen = true
			if value == "" {
				return "", "", "", 0, fmt.Errorf("RFC text has a malformed Updates header")
			}
			for strings.HasSuffix(value, ",") {
				if terminatesAtColumn && (lineIndex+1 >= len(lines) || !startsWithRFCContinuation(lines[lineIndex+1])) {
					value = strings.TrimSuffix(value, ",")
					break
				}
				lineIndex++
				if lineIndex >= len(lines) || !startsWithRFCContinuation(lines[lineIndex]) {
					return "", "", "", 0, fmt.Errorf("RFC text has a malformed Updates header")
				}
				continuation, continuationTerminatesAtColumn, valid := rfcUpdatesContinuationValue(lines[lineIndex])
				if !valid {
					return "", "", "", 0, fmt.Errorf("RFC text has a malformed Updates header")
				}
				value += " " + continuation
				terminatesAtColumn = continuationTerminatesAtColumn
				if rfcUpdatesCount(value) > maxRFCUpdates {
					return "", "", "", 0, fmt.Errorf("RFC text has a malformed Updates header")
				}
			}
			if lineIndex+1 < len(lines) && looksLikeRFCUpdatesContinuation(lines[lineIndex+1]) {
				return "", "", "", 0, fmt.Errorf("RFC text has a malformed Updates header")
			}
			updates = value
		}
	}
	if number == "" {
		return "", "", "", 0, fmt.Errorf("RFC text does not declare a Request for Comments number")
	}
	if category == "" {
		return "", "", "", 0, fmt.Errorf("RFC text does not declare a publication category")
	}
	return number, category, updates, bodyStart, nil
}

func rfcHeaderValue(line, header string) (string, string, bool) {
	if !strings.HasPrefix(line, header) {
		return "", "", false
	}
	rawValue := strings.TrimPrefix(line, header)
	if containsUnsafeText(rawValue) || containsNonASCIIWhitespace(rawValue) || !hasCanonicalRFCHeaderValueLayout(rawValue) {
		return "", "", true
	}
	value := strings.TrimPrefix(rawValue, " ")
	column := ""
	if index := rfcColumnSplitRE.FindStringIndex(value); index != nil {
		column = trimRFCLayoutWhitespace(value[index[1]:])
		value = trimRFCLayoutWhitespace(value[:index[0]])
	}
	return value, column, true
}

func rfcUpdatesHeaderValue(line string) (string, bool, bool) {
	if !strings.HasPrefix(line, "Updates:") {
		return "", false, false
	}
	rawValue := strings.TrimPrefix(line, "Updates:")
	if containsUnsafeText(rawValue) || containsNonASCIIWhitespace(rawValue) || !hasCanonicalRFCHeaderValueLayout(rawValue) {
		return "", false, true
	}
	value, terminatesAtColumn, valid := parseRFCUpdatesValue(strings.TrimPrefix(rawValue, " "))
	if !valid {
		return "", false, true
	}
	return value, terminatesAtColumn, true
}

func hasCanonicalRFCHeaderValueLayout(rawValue string) bool {
	lastRune, lastRuneSize := utf8.DecodeLastRuneInString(rawValue)
	if lastRuneSize == 0 {
		return false
	}
	return strings.HasPrefix(rawValue, " ") && !strings.HasPrefix(rawValue, "  ") && !unicode.IsSpace(lastRune)
}

func rfcUpdatesContinuationValue(line string) (string, bool, bool) {
	if containsUnsafeText(line) || containsNonASCIIWhitespace(line) {
		return "", false, false
	}
	return parseRFCUpdatesValue(trimRFCLayoutWhitespace(line))
}

func rfcUpdatesValue(value string) (string, bool) {
	value, _, valid := parseRFCUpdatesValue(value)
	return value, valid
}

func parseRFCUpdatesValue(value string) (string, bool, bool) {
	if containsUnsafeText(value) || containsNonASCIIWhitespace(value) {
		return "", false, false
	}
	match := rfcUpdatesRE.FindStringSubmatchIndex(value)
	if len(match) != 4 {
		return "", false, false
	}
	matched := trimRFCLayoutWhitespace(value[match[2]:match[3]])
	trailing := value[match[3]:]
	terminatesAtColumn := false
	if trimRFCLayoutWhitespace(trailing) != "" {
		column := rfcColumnSplitRE.FindStringIndex(trailing)
		if column == nil || column[0] != 0 {
			return "", false, false
		}
		if !looksLikeRFCHeaderColumn(trailing) {
			return "", false, false
		}
		columnValue := trimRFCLayoutWhitespace(trailing)
		commaTerminated := strings.HasSuffix(matched, ",")
		if rfcDateColumnRE.MatchString(columnValue) {
			terminatesAtColumn = commaTerminated
		} else if startsWithRFCNumber(columnValue) || rfcUpdatesListRE.MatchString(columnValue) || strings.Contains(columnValue, ",") {
			return "", false, false
		}
	}
	commaTerminated := strings.HasSuffix(matched, ",")
	matched = strings.TrimSpace(strings.TrimSuffix(matched, ","))
	numbers := strings.Split(matched, ",")
	if len(numbers) == 0 || len(numbers) > maxRFCUpdates {
		return "", false, false
	}
	for index := range numbers {
		numbers[index] = strings.TrimSpace(numbers[index])
		if !rfcNumberRE.MatchString(numbers[index]) || utf8.RuneCountInString(numbers[index]) > maxRFCNumberRunes {
			return "", false, false
		}
	}
	value = strings.Join(numbers, ", ")
	if commaTerminated {
		value += ","
	}
	return value, terminatesAtColumn, true
}

func validRFCCategory(value string) bool {
	_, ok := rfcCategories[value]
	return ok
}

func ambiguousRFCHeaderColumn(value string) bool {
	return !rfcDateColumnRE.MatchString(value) && (startsWithRFCNumber(value) || validRFCCategory(value))
}

func startsWithRFCNumber(value string) bool {
	if index := strings.IndexAny(value, " \t"); index >= 0 {
		value = value[:index]
	}
	return rfcNumberRE.MatchString(value)
}

func hasSurroundingWhitespace(value string) bool {
	return value != strings.TrimSpace(value)
}

func looksLikeRFCHeaderColumn(value string) bool {
	return strings.HasPrefix(value, "   ") || strings.HasPrefix(value, "\t\t")
}

func rfcAnchor(sourceURL *url.URL, logicalPath string, line int) string {
	return fmt.Sprintf("%s#logical-path=%s&line=%d", sourceURL.String(), logicalPath, line)
}

func rfcUpdatesCount(value string) int {
	value = strings.TrimSpace(strings.TrimSuffix(value, ","))
	if value == "" {
		return 0
	}
	return len(strings.Split(value, ","))
}

func normalizeLineEndings(value string) string {
	return strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(value)
}

func startsWithRFCContinuation(line string) bool {
	return len(strings.TrimSpace(line)) > 0 && (line[0] == ' ' || line[0] == '\t')
}

func looksLikeRFCUpdatesContinuation(line string) bool {
	value := strings.TrimSpace(line)
	if value == "" {
		return false
	}
	return value[0] == ',' || rfcUpdatesTokenRE.MatchString(value)
}

func containsNonASCIIWhitespace(value string) bool {
	for _, r := range value {
		if unicode.IsSpace(r) && r != ' ' && r != '\t' {
			return true
		}
	}
	return false
}

func trimRFCLayoutWhitespace(value string) string {
	return strings.Trim(value, " \t")
}

func bcp14NonCandidateLines(lines []string) (map[int]bool, int) {
	nonCandidateLines := make(map[int]bool)
	firstCandidateLine := -1
	pendingTemplateDeclaration := false
	pendingQuotation := false
	replacementBlock := false
	for start := 0; start < len(lines); {
		for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
			start++
		}
		if start == len(lines) {
			break
		}
		end := start
		for end < len(lines) && strings.TrimSpace(lines[end]) != "" {
			end++
		}
		paragraph := strings.Join(strings.Fields(strings.Join(lines[start:end], " ")), " ")
		templateContext := bcpTemplateRE.MatchString(paragraph)
		replacementLabel := bcpReplacementLabelRE.MatchString(paragraph)
		quotationLeadIn := bcpQuotationLeadInRE.MatchString(paragraph)
		quoteContext := templateContext || replacementLabel || quotationLeadIn
		declaration := hasBCP14Declaration(paragraph)
		pageFurniture := isRFCPageFurniture(lines[start:end])
		if declaration || quoteContext || pageFurniture || replacementBlock || pendingTemplateDeclaration || pendingQuotation {
			for line := start; line < end; line++ {
				nonCandidateLines[line] = true
			}
		}
		if declaration && !pendingTemplateDeclaration && !pendingQuotation && !quoteContext && !pageFurniture && !replacementBlock {
			if firstCandidateLine == -1 {
				firstCandidateLine = end
			}
		}
		switch {
		case replacementLabel:
			replacementBlock = true
			pendingTemplateDeclaration = false
			pendingQuotation = false
		case replacementBlock:
			// An explicitly labelled replacement remains quoted through the document.
		case quotationLeadIn:
			pendingTemplateDeclaration = false
			pendingQuotation = true
		case declaration:
			pendingTemplateDeclaration = false
			pendingQuotation = false
		case templateContext:
			pendingTemplateDeclaration = true
			pendingQuotation = false
		case !pageFurniture:
			pendingTemplateDeclaration = false
			pendingQuotation = false
		}
		start = end
	}
	return nonCandidateLines, firstCandidateLine
}

func isRFCPageFurniture(lines []string) bool {
	if len(lines) != 1 {
		return false
	}
	return rfcPageFooterRE.MatchString(lines[0]) || rfcRunningHeaderRE.MatchString(lines[0])
}

func hasBCP14Declaration(paragraph string) bool {
	for _, match := range bcpDeclarationRE.FindAllStringSubmatch(paragraph, -1) {
		if bcpQualifierRE.MatchString(match[1]) {
			return true
		}
	}
	return false
}

func decodeIEEEManifest(data []byte) (ieeeManifest, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return ieeeManifest{}, fmt.Errorf("decode IEEE clause manifest: %w", err)
	}
	if err := validateExactJSONObject(object, "IEEE clause manifest", "standard_id", "edition", "document_sha256", "clauses"); err != nil {
		return ieeeManifest{}, err
	}
	var clauses []json.RawMessage
	if err := json.Unmarshal(object["clauses"], &clauses); err != nil {
		return ieeeManifest{}, fmt.Errorf("decode IEEE clause manifest clauses: %w", err)
	}
	for index, rawClause := range clauses {
		var clause map[string]json.RawMessage
		if err := json.Unmarshal(rawClause, &clause); err != nil {
			return ieeeManifest{}, fmt.Errorf("decode IEEE clause manifest clauses[%d]: %w", index, err)
		}
		if err := validateExactJSONObject(clause, fmt.Sprintf("IEEE clause manifest clauses[%d]", index), "anchor", "summary"); err != nil {
			return ieeeManifest{}, err
		}
	}
	var manifest ieeeManifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return ieeeManifest{}, fmt.Errorf("decode IEEE clause manifest: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return ieeeManifest{}, fmt.Errorf("IEEE clause manifest has trailing data")
	}
	return manifest, nil
}

func validateExactJSONObject(object map[string]json.RawMessage, path string, fields ...string) error {
	if object == nil {
		return fmt.Errorf("%s must be a JSON object", path)
	}
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	keys := make([]string, 0, len(object))
	for field := range object {
		keys = append(keys, field)
	}
	sort.Strings(keys)
	for _, field := range keys {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("%s contains unknown field %q", path, field)
		}
	}
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			return fmt.Errorf("%s is missing required field %q", path, field)
		}
	}
	return nil
}

func containsUnsafeText(value string) bool {
	for _, runeValue := range value {
		if unicode.IsControl(runeValue) || unicode.Is(unicode.Cf, runeValue) || unicode.Is(unicode.Zl, runeValue) || unicode.Is(unicode.Zp, runeValue) {
			return true
		}
	}
	return false
}

func contentSHA256(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

func newProvenance(sourceRevision, digest, anchor, producerName, producerVersion string) trace.Provenance {
	return trace.Provenance{
		SourceRevision: sourceRevision,
		ContentSHA256:  digest,
		Anchor:         anchor,
		Producer:       trace.Producer{Name: producerName, Version: producerVersion},
	}
}

func rejectDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(dec, 0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("trailing data after JSON document")
	}
	return nil
}

func scanJSONValue(dec *json.Decoder, depth int) error {
	if depth > maxJSONNestingDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels", maxJSONNestingDepth)
	}
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for dec.More() {
			name, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := name.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("unterminated JSON object")
		}
	case '[':
		for dec.More() {
			if err := scanJSONValue(dec, depth+1); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("unterminated JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}
