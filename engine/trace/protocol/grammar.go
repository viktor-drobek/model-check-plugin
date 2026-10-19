package protocol

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"modelcheck/trace"
)

const grammarAdapterVersion = "1"

var (
	declaredAnchorRE   = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,200}$`)
	grammarTargetRE    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]{0,199}$`)
	rawGrammarMarkerRE = regexp.MustCompile(`(?i:authorization|cookie|password|passwd|secret|token|api[_-]?key|private[_-]?key|credential)|-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----|\bAKIA[0-9A-Z]{16}\b`)
)

// GrammarFileInput registers exact caller-supplied grammar bytes, not parsed syntax.
// DeclaredAnchor is an assertion and is never resolved against a declaration.
type GrammarFileInput struct {
	Artifact        Input
	Language        string
	LanguageVersion string
	DeclaredAnchor  string
	GrammarSHA256   string
}

// GrammarReferenceInput supplies a manifest when the asserted grammar bytes are absent.
type GrammarReferenceInput struct {
	Artifact       Input
	ManifestSHA256 string
}

func admittedGrammarPair(language, version string) bool {
	return language == "kaitai" && version == "0.10" || language == "spicy" && version == "1.12.0"
}

func hasStem(logicalPath, suffix string) bool {
	base := path.Base(logicalPath)
	return strings.HasSuffix(base, suffix) && len(base) > len(suffix)
}

// AnalyzeGrammar returns canonical v2 registration bytes. It neither opens paths
// nor invokes a compiler, parses grammar syntax, or resolves imports or anchors.
func AnalyzeGrammar(input GrammarFileInput) ([]byte, error) {
	return analyzeGrammarWithBudget(input, defaultBudget)
}

func analyzeGrammarWithBudget(input GrammarFileInput, limits budget) ([]byte, error) {
	snapshot, release, err := begin(input.Artifact, input.GrammarSHA256, func() bool {
		extension := ".ksy"
		if input.Language == "spicy" {
			extension = ".spicy"
		}
		return admittedGrammarPair(input.Language, input.LanguageVersion) && hasStem(input.Artifact.Path, extension) && declaredAnchorRE.MatchString(input.DeclaredAnchor)
	}, []string{input.Language, input.LanguageVersion, input.DeclaredAnchor})
	if err != nil {
		return nil, err
	}
	defer release()
	if err := validateRawGrammar(snapshot, input.Language); err != nil {
		return nil, err
	}
	identity := tupleHash("grammar-artifact/1", input.Language, input.LanguageVersion, input.Artifact.Path, input.DeclaredAnchor, input.GrammarSHA256)
	nodeID := "grammar." + identity
	evidenceID := "evidence.grammar." + identity
	gapID := "gap.grammar.anchor_unverified." + identity
	anchor := "grammar:" + input.Language + "@" + input.LanguageVersion + ":" + input.Artifact.Path + "#declared=" + input.DeclaredAnchor
	label := "Registered Kaitai grammar artifact"
	if input.Language == "spicy" {
		label = "Registered Spicy grammar artifact"
	}
	reason := "the declared grammar anchor was not syntax-verified"
	// This conservative bound accounts for every repeated string and JSON framing
	// before any graph record is allocated. All admitted output scalars are ASCII.
	bound := uint64(2048 + 3*(len(anchor)+len(input.GrammarSHA256)+len("mcd-grammar-adapter")+len(grammarAdapterVersion)) + 4*len(nodeID) + 3*len(evidenceID) + len(gapID) + len(label) + len(reason))
	if err := checkBudget(3, bound, limits); err != nil {
		return nil, err
	}
	provenance := trace.Provenance{ContentSHA256: input.GrammarSHA256, Anchor: anchor, Producer: trace.Producer{Name: "mcd-grammar-adapter", Version: grammarAdapterVersion}}
	document := newDocument(trace.SchemaV2)
	document.Nodes = append(document.Nodes, trace.Node{ID: nodeID, Kind: trace.GrammarArtifact, Label: label, Provenance: provenance, Evidence: []string{evidenceID}})
	document.Evidence = append(document.Evidence, trace.Evidence{ID: evidenceID, Subject: "node:" + nodeID, Level: trace.Grammar, Provenance: provenance})
	document.Gaps = append(document.Gaps, trace.Gap{ID: gapID, Kind: "grammar.anchor_unverified", Reference: "node:" + nodeID, Reason: reason, Provenance: provenance})
	return finish(document)
}

func validateRawGrammar(source []byte, language string) error {
	first := len(source)
	for offset := 0; offset < len(source); {
		value, size := utf8.DecodeRune(source[offset:])
		invalid := value == utf8.RuneError && size == 1 || value == '\\' || value == '\ufeff' || unicode.Is(unicode.Cf, value) || unicode.Is(unicode.Zl, value) || unicode.Is(unicode.Zp, value) || unicode.IsControl(value) && value != '\n' && value != '\t'
		if invalid && offset < first {
			first = offset
		}
		offset += size
	}
	if match := rawGrammarMarkerRE.FindIndex(source); match != nil && match[0] < first {
		first = match[0]
	}
	for offset := 0; offset < len(source); {
		if !grammarWordByte(source[offset]) {
			offset++
			continue
		}
		start := offset
		for offset < len(source) && grammarWordByte(source[offset]) {
			offset++
		}
		word := string(source[start:offset])
		if language == "kaitai" && word == "imports" || language == "spicy" && (word == "import" || word == "include") {
			if start < first {
				first = start
			}
		}
	}
	if first < len(source) {
		return rule("grammar_source_invalid", fmt.Sprintf("byte:%d", first))
	}
	return nil
}

func grammarWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

// AnalyzeGrammarReference returns a canonical v1 ledger containing only an
// unresolved-reference gap. The target digest remains an unverified assertion.
func AnalyzeGrammarReference(input GrammarReferenceInput) ([]byte, error) {
	return analyzeGrammarReferenceWithBudget(input, defaultBudget)
}

func analyzeGrammarReferenceWithBudget(input GrammarReferenceInput, limits budget) ([]byte, error) {
	snapshot, release, err := begin(input.Artifact, input.ManifestSHA256, func() bool { return hasStem(input.Artifact.Path, ".grammar-ref.json") }, nil)
	if err != nil {
		return nil, err
	}
	defer release()
	value, err := decodeJSON(snapshot)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 6 {
		return nil, rule("schema_invalid", "manifest")
	}
	fields := []string{"schema", "language", "language_version", "target_id", "target_sha256", "declared_anchor"}
	stringsByName := make(map[string]string, len(fields))
	for _, field := range fields {
		scalar, ok := object[field].(string)
		if !ok {
			return nil, rule("schema_invalid", field)
		}
		stringsByName[field] = scalar
	}
	m := stringsByName
	if m["schema"] != "mcd-grammar-reference/1" || !admittedGrammarPair(m["language"], m["language_version"]) || !grammarTargetRE.MatchString(m["target_id"]) || !validDigest(m["target_sha256"]) || !declaredAnchorRE.MatchString(m["declared_anchor"]) {
		return nil, rule("schema_invalid", "manifest")
	}
	if err := scanStructured(value); err != nil {
		return nil, err
	}
	identity := tupleHash("grammar-reference/1", input.Artifact.Path, input.ManifestSHA256, m["language"], m["language_version"], m["target_id"], m["target_sha256"], m["declared_anchor"])
	id := "gap.grammar.reference_unresolved." + identity
	reference := "grammar-reference:" + m["language"] + "@" + m["language_version"] + ":" + m["target_id"] + "#sha256=" + m["target_sha256"] + "&declared=" + m["declared_anchor"]
	anchor := "grammar-reference:" + input.Artifact.Path + "#declared=" + m["declared_anchor"]
	reason := "grammar bytes were not supplied for the asserted reference"
	bound := uint64(1024 + len(id) + len(reference) + len(anchor) + len(input.ManifestSHA256) + len(reason) + len("mcd-grammar-reference-adapter"))
	if err := checkBudget(1, bound, limits); err != nil {
		return nil, err
	}
	document := newDocument(trace.Schema)
	document.Gaps = append(document.Gaps, trace.Gap{ID: id, Kind: "grammar.reference_unresolved", Reference: reference, Reason: reason, Provenance: trace.Provenance{ContentSHA256: input.ManifestSHA256, Anchor: anchor, Producer: trace.Producer{Name: "mcd-grammar-reference-adapter", Version: grammarAdapterVersion}}})
	return finish(document)
}
