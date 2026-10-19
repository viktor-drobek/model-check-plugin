package trace

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

const grammarAnchorUnverifiedReason = "the declared grammar anchor was not syntax-verified"

var (
	grammarAnchorPattern  = regexp.MustCompile(`^grammar:(kaitai|spicy)@([^:]+):([^#]+)#declared=([A-Za-z0-9_.:-]{1,200})$`)
	grammarVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*)){1,2}$`)
	logicalPathPattern    = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,512}$`)
)

func selectsSchemaV2(data []byte) bool {
	pos := skipJSONWhitespace(data, 0)
	if pos >= len(data) || data[pos] != '{' {
		return false
	}
	pos = skipJSONWhitespace(data, pos+1)
	if !bytes.HasPrefix(data[pos:], []byte(`"schema"`)) {
		return false
	}
	pos = skipJSONWhitespace(data, pos+len(`"schema"`))
	if pos >= len(data) || data[pos] != ':' {
		return false
	}
	pos = skipJSONWhitespace(data, pos+1)
	if !bytes.HasPrefix(data[pos:], []byte(`"mcd-trace-ledger/2"`)) {
		return false
	}
	pos = skipJSONWhitespace(data, pos+len(`"mcd-trace-ledger/2"`))
	return pos < len(data) && (data[pos] == ',' || data[pos] == '}')
}

func skipJSONWhitespace(data []byte, pos int) int {
	for pos < len(data) {
		switch data[pos] {
		case ' ', '\t', '\n', '\r':
			pos++
		default:
			return pos
		}
	}
	return pos
}

func unmarshalV2JSON(data []byte) (*Document, error) {
	if err := validateStrictV2JSON(data); err != nil {
		return nil, fmt.Errorf("trace v2: %w", err)
	}
	if err := validateExactJSONKeys(data); err != nil {
		return nil, fmt.Errorf("trace v2: %w", err)
	}
	var document Document
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("trace v2: %w", err)
	}
	if err := Validate(&document); err != nil {
		return nil, fmt.Errorf("trace v2: %w", err)
	}
	return &document, nil
}

func validateV2GrammarArtifacts(d *Document, nodes map[string]Node, incidentNodes map[string]bool, evidence map[string]Evidence) error {
	for _, ev := range d.Evidence {
		if ev.Level != Grammar {
			continue
		}
		kind, id, _ := strings.Cut(ev.Subject, ":")
		node, ok := nodes[id]
		if kind != "node" || !ok || node.Kind != GrammarArtifact {
			return fmt.Errorf("grammar evidence %q must apply only to a GrammarArtifact node", ev.ID)
		}
	}

	gapsByNode := make(map[string][]Gap)
	for _, gap := range d.Gaps {
		if gap.Kind != "grammar.anchor_unverified" {
			continue
		}
		kind, id, ok := strings.Cut(gap.Reference, ":")
		if !ok || kind != "node" {
			return fmt.Errorf("grammar disclosure gap %q must reference a GrammarArtifact node", gap.ID)
		}
		node, exists := nodes[id]
		if !exists || node.Kind != GrammarArtifact {
			return fmt.Errorf("grammar disclosure gap %q must reference a GrammarArtifact node", gap.ID)
		}
		gapsByNode[id] = append(gapsByNode[id], gap)
	}

	for _, node := range d.Nodes {
		if node.Kind != GrammarArtifact {
			continue
		}
		id := node.ID
		if len(node.Evidence) != 1 {
			return fmt.Errorf("GrammarArtifact node %q must have exactly one grammar evidence", id)
		}
		ev, ok := evidence[node.Evidence[0]]
		if !ok {
			return fmt.Errorf("GrammarArtifact node %q references missing evidence %q", id, node.Evidence[0])
		}
		if ev.Level != Grammar {
			return fmt.Errorf("GrammarArtifact node %q must use grammar evidence", id)
		}

		identity, language, err := grammarArtifactIdentity(node.Provenance)
		if err != nil {
			return fmt.Errorf("GrammarArtifact node %q: %w", id, err)
		}
		if want := "grammar." + identity; node.ID != want {
			return fmt.Errorf("GrammarArtifact node ID = %q, want %q", node.ID, want)
		}
		if want := grammarArtifactLabel(language); node.Label != want {
			return fmt.Errorf("GrammarArtifact node %q label = %q, want %q", id, node.Label, want)
		}
		if want := "evidence.grammar." + identity; ev.ID != want {
			return fmt.Errorf("GrammarArtifact evidence ID = %q, want %q", ev.ID, want)
		}
		if want := "node:" + id; ev.Subject != want {
			return fmt.Errorf("GrammarArtifact evidence subject = %q, want %q", ev.Subject, want)
		}
		if ev.Provenance != node.Provenance {
			return fmt.Errorf("GrammarArtifact evidence provenance must equal node provenance")
		}
		if incidentNodes[id] {
			return fmt.Errorf("GrammarArtifact node %q must be isolated", id)
		}

		gaps := gapsByNode[id]
		if len(gaps) != 1 {
			return fmt.Errorf("GrammarArtifact node %q must have exactly one grammar.anchor_unverified gap", id)
		}
		gap := gaps[0]
		if want := "gap.grammar.anchor_unverified." + identity; gap.ID != want {
			return fmt.Errorf("GrammarArtifact disclosure gap ID = %q, want %q", gap.ID, want)
		}
		if want := "node:" + id; gap.Reference != want {
			return fmt.Errorf("GrammarArtifact disclosure gap reference = %q, want %q", gap.Reference, want)
		}
		if gap.Reason != grammarAnchorUnverifiedReason {
			return fmt.Errorf("GrammarArtifact grammar disclosure gap reason = %q, want %q", gap.Reason, grammarAnchorUnverifiedReason)
		}
		if gap.Provenance != node.Provenance {
			return fmt.Errorf("GrammarArtifact disclosure gap provenance must equal node provenance")
		}
	}
	return nil
}

func grammarArtifactIdentity(provenance Provenance) (string, string, error) {
	if provenance.ContentSHA256 == "" {
		return "", "", fmt.Errorf("provenance.content_sha256 is required")
	}
	matches := grammarAnchorPattern.FindStringSubmatch(provenance.Anchor)
	if matches == nil {
		return "", "", fmt.Errorf("provenance.anchor is not a grammar anchor")
	}
	language, version, logicalPath, declaredAnchor := matches[1], matches[2], matches[3], matches[4]
	if !grammarVersionPattern.MatchString(version) {
		return "", "", fmt.Errorf("grammar language version %q is invalid", version)
	}
	if err := validateGrammarLogicalPath(logicalPath, language); err != nil {
		return "", "", err
	}
	parts := []string{"grammar-artifact/1", language, version, logicalPath, declaredAnchor, provenance.ContentSHA256}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%x", digest), language, nil
}

func validateGrammarLogicalPath(logicalPath, language string) error {
	if !logicalPathPattern.MatchString(logicalPath) || strings.HasPrefix(logicalPath, "/") || strings.HasSuffix(logicalPath, "/") || strings.Contains(logicalPath, "//") || path.Clean(logicalPath) != logicalPath {
		return fmt.Errorf("grammar logical path %q is invalid", logicalPath)
	}
	for _, segment := range strings.Split(logicalPath, "/") {
		if segment == "." || segment == ".." {
			return fmt.Errorf("grammar logical path %q is invalid", logicalPath)
		}
	}
	base := path.Base(logicalPath)
	extension := map[string]string{"kaitai": ".ksy", "spicy": ".spicy"}[language]
	if !strings.HasSuffix(base, extension) || strings.TrimSuffix(base, extension) == "" {
		return fmt.Errorf("grammar logical path %q must end in %s with a nonempty stem", logicalPath, extension)
	}
	return nil
}

func grammarArtifactLabel(language string) string {
	if language == "spicy" {
		return "Registered Spicy grammar artifact"
	}
	return "Registered Kaitai grammar artifact"
}

func validateStrictV2JSON(data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("invalid UTF-8")
	}
	parser := strictJSONParser{data: data}
	parser.skipWhitespace()
	if err := parser.parseValue(0); err != nil {
		return err
	}
	parser.skipWhitespace()
	if parser.pos != len(data) {
		return fmt.Errorf("trailing data")
	}
	return nil
}

type strictJSONParser struct {
	data []byte
	pos  int
}

func (p *strictJSONParser) parseValue(depth int) error {
	if depth > 128 {
		return fmt.Errorf("JSON nesting exceeds 128 levels")
	}
	p.skipWhitespace()
	if p.pos >= len(p.data) {
		return fmt.Errorf("unexpected end of JSON")
	}
	switch p.data[p.pos] {
	case '{':
		return p.parseObject(depth + 1)
	case '[':
		return p.parseArray(depth + 1)
	case '"':
		_, err := p.parseString()
		return err
	case 't':
		return p.parseLiteral("true")
	case 'f':
		return p.parseLiteral("false")
	case 'n':
		return p.parseLiteral("null")
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return p.parseNumber()
	default:
		return fmt.Errorf("invalid JSON value")
	}
}

func (p *strictJSONParser) parseObject(depth int) error {
	p.pos++
	p.skipWhitespace()
	if p.consume('}') {
		return nil
	}
	seen := make(map[string]struct{})
	for {
		p.skipWhitespace()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return fmt.Errorf("object member name must be a string")
		}
		name, err := p.parseString()
		if err != nil {
			return err
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate JSON member %q", name)
		}
		seen[name] = struct{}{}
		p.skipWhitespace()
		if !p.consume(':') {
			return fmt.Errorf("object member is missing colon")
		}
		if err := p.parseValue(depth); err != nil {
			return err
		}
		p.skipWhitespace()
		if p.consume('}') {
			return nil
		}
		if !p.consume(',') {
			return fmt.Errorf("object members must be comma-separated")
		}
	}
}

func (p *strictJSONParser) parseArray(depth int) error {
	p.pos++
	p.skipWhitespace()
	if p.consume(']') {
		return nil
	}
	for {
		if err := p.parseValue(depth); err != nil {
			return err
		}
		p.skipWhitespace()
		if p.consume(']') {
			return nil
		}
		if !p.consume(',') {
			return fmt.Errorf("array values must be comma-separated")
		}
	}
}

func (p *strictJSONParser) parseString() (string, error) {
	p.pos++
	var value strings.Builder
	for p.pos < len(p.data) {
		valueByte := p.data[p.pos]
		switch valueByte {
		case '"':
			p.pos++
			return value.String(), nil
		case '\\':
			p.pos++
			if p.pos >= len(p.data) {
				return "", fmt.Errorf("unterminated JSON escape")
			}
			escaped := p.data[p.pos]
			p.pos++
			switch escaped {
			case '"', '\\', '/':
				value.WriteByte(escaped)
			case 'b':
				value.WriteByte('\b')
			case 'f':
				value.WriteByte('\f')
			case 'n':
				value.WriteByte('\n')
			case 'r':
				value.WriteByte('\r')
			case 't':
				value.WriteByte('\t')
			case 'u':
				runeValue, err := p.parseUnicodeEscape()
				if err != nil {
					return "", err
				}
				value.WriteRune(runeValue)
			default:
				return "", fmt.Errorf("invalid JSON escape")
			}
		case '\n', '\r':
			return "", fmt.Errorf("control character in JSON string")
		default:
			if valueByte < 0x20 {
				return "", fmt.Errorf("control character in JSON string")
			}
			if valueByte < utf8.RuneSelf {
				value.WriteByte(valueByte)
				p.pos++
				continue
			}
			runeValue, size := utf8.DecodeRune(p.data[p.pos:])
			value.WriteRune(runeValue)
			p.pos += size
		}
	}
	return "", fmt.Errorf("unterminated JSON string")
}

func (p *strictJSONParser) parseUnicodeEscape() (rune, error) {
	value, err := p.readHexQuad()
	if err != nil {
		return 0, err
	}
	if value >= 0xD800 && value <= 0xDBFF {
		if p.pos+2 > len(p.data) || p.data[p.pos] != '\\' || p.data[p.pos+1] != 'u' {
			return 0, fmt.Errorf("unpaired surrogate")
		}
		p.pos += 2
		low, err := p.readHexQuad()
		if err != nil {
			return 0, err
		}
		if low < 0xDC00 || low > 0xDFFF {
			return 0, fmt.Errorf("unpaired surrogate")
		}
		return 0x10000 + (rune(value)-0xD800)<<10 + rune(low-0xDC00), nil
	}
	if value >= 0xDC00 && value <= 0xDFFF {
		return 0, fmt.Errorf("unpaired surrogate")
	}
	return rune(value), nil
}

func (p *strictJSONParser) readHexQuad() (uint16, error) {
	if p.pos+4 > len(p.data) {
		return 0, fmt.Errorf("truncated JSON unicode escape")
	}
	var value uint16
	for _, digit := range p.data[p.pos : p.pos+4] {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value += uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value += uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value += uint16(digit-'A') + 10
		default:
			return 0, fmt.Errorf("invalid JSON unicode escape")
		}
	}
	p.pos += 4
	return value, nil
}

func (p *strictJSONParser) parseLiteral(literal string) error {
	if !bytes.HasPrefix(p.data[p.pos:], []byte(literal)) {
		return fmt.Errorf("invalid JSON literal")
	}
	p.pos += len(literal)
	return nil
}

func (p *strictJSONParser) parseNumber() error {
	if p.consume('-') && p.pos >= len(p.data) {
		return fmt.Errorf("invalid JSON number")
	}
	if p.consume('0') {
		if p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			return fmt.Errorf("invalid JSON number")
		}
	} else {
		if p.pos >= len(p.data) || p.data[p.pos] < '1' || p.data[p.pos] > '9' {
			return fmt.Errorf("invalid JSON number")
		}
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.consume('.') {
		if !p.consumeDigit() {
			return fmt.Errorf("invalid JSON number")
		}
		for p.consumeDigit() {
		}
	}
	if p.pos < len(p.data) && (p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.data) && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
			p.pos++
		}
		if !p.consumeDigit() {
			return fmt.Errorf("invalid JSON number")
		}
		for p.consumeDigit() {
		}
	}
	return nil
}

func (p *strictJSONParser) consumeDigit() bool {
	if p.pos >= len(p.data) || p.data[p.pos] < '0' || p.data[p.pos] > '9' {
		return false
	}
	p.pos++
	return true
}

func (p *strictJSONParser) consume(want byte) bool {
	if p.pos >= len(p.data) || p.data[p.pos] != want {
		return false
	}
	p.pos++
	return true
}

func (p *strictJSONParser) skipWhitespace() {
	p.pos = skipJSONWhitespace(p.data, p.pos)
}
