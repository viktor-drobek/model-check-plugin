package protocol

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"modelcheck/trace"
)

// AnalyzeProtobuf accepts one bounded proto3 source using the declaration-only
// profile. It does not call protoc or resolve imports.
func AnalyzeProtobuf(input ContractInput) ([]byte, error) {
	return analyzeProtobuf(input, defaultBudget)
}
func analyzeProtobufWithBudget(input ContractInput, limits budget) ([]byte, error) {
	return analyzeProtobuf(input, limits)
}
func analyzeProtobuf(input ContractInput, limits budget) ([]byte, error) {
	return contractRun(input, "protobuf", limits, parseProtobufContract)
}

type protoToken struct {
	text   string
	quoted bool
}
type protoMessage struct {
	name   string
	fields []protoField
}
type protoField struct {
	typ, name string
	tag       int
	repeated  bool
}
type protoService struct {
	name string
	rpcs []protoRPC
}
type protoRPC struct{ name, in, out string }

func parseProtobufContract(data []byte) (contractPlan, error) {
	if !utf8.Valid(data) || (len(data) >= 3 && string(data[:3]) == "\xef\xbb\xbf") {
		return contractPlan{}, contractErr("schema_invalid")
	}
	if err := protobufRawSafety(string(data)); err != nil {
		return contractPlan{}, err
	}
	tokens, err := protobufLex(data)
	if err != nil {
		return contractPlan{}, err
	}
	p := protoParser{tokens: tokens}
	if !p.take("syntax") || !p.take("=") || !p.takeQuoted("proto3") || !p.take(";") {
		return contractPlan{}, contractErr("schema_invalid")
	}
	if p.take("package") {
		if !p.packageName() || !p.take(";") {
			return contractPlan{}, contractErr("schema_invalid")
		}
	}
	messages := []protoMessage{}
	services := []protoService{}
	names := map[string]bool{}
	for !p.end() {
		if p.peek("message") {
			m, e := p.message()
			if e != nil {
				return contractPlan{}, e
			}
			if protoScalarName(m.name) {
				return contractPlan{}, contractErr("schema_invalid")
			}
			if names[m.name] {
				return contractPlan{}, contractErr("schema_invalid")
			}
			names[m.name] = true
			messages = append(messages, m)
			continue
		}
		if p.peek("service") {
			s, e := p.service()
			if e != nil {
				return contractPlan{}, e
			}
			if protoScalarName(s.name) {
				return contractPlan{}, contractErr("schema_invalid")
			}
			if names[s.name] {
				return contractPlan{}, contractErr("schema_invalid")
			}
			names[s.name] = true
			services = append(services, s)
			continue
		}
		return contractPlan{}, contractErr("unsupported_declaration")
	}
	count := len(messages) + len(services)
	for _, m := range messages {
		count += len(m.fields)
	}
	for _, s := range services {
		count += len(s.rpcs)
	}
	if count > 1024 {
		return contractPlan{}, contractErrAt("preflight_limit", "budget")
	}
	messageNames := map[string]bool{}
	for _, m := range messages {
		messageNames[m.name] = true
	}
	graph := map[string][]string{}
	primitive := map[string]bool{"double": true, "float": true, "int32": true, "int64": true, "uint32": true, "uint64": true, "sint32": true, "sint64": true, "fixed32": true, "fixed64": true, "sfixed32": true, "sfixed64": true, "bool": true, "string": true, "bytes": true}
	plan := contractPlan{gaps: []contractGap{{key: "#", kind: "protobuf.declarations_only", reason: "declarations are not protocol execution"}}}
	ordinal := 0
	for _, m := range messages {
		mk := protoOrdinal(ordinal)
		ordinal++
		plan.facts = append(plan.facts, contractFact{key: mk, label: m.name, kind: trace.ProtocolMessage})
		tags := map[int]bool{}
		fields := map[string]bool{}
		jsonNames := map[string]bool{}
		for _, f := range m.fields {
			jsonName := protoJSONName(f.name)
			if fields[f.name] || jsonNames[jsonName] || tags[f.tag] || f.tag <= 0 || f.tag > 536870911 || (f.tag >= 19000 && f.tag <= 19999) {
				return contractPlan{}, contractErr("schema_invalid")
			}
			fields[f.name] = true
			jsonNames[jsonName] = true
			tags[f.tag] = true
			if !primitive[f.typ] && !messageNames[f.typ] {
				return contractPlan{}, contractErr("schema_invalid")
			}
			if f.repeated && messageNames[f.typ] {
				return contractPlan{}, contractErr("schema_invalid")
			}
			if messageNames[f.typ] {
				graph[m.name] = append(graph[m.name], f.typ)
			}
			plan.facts = append(plan.facts, contractFact{key: protoOrdinal(ordinal), label: f.name, kind: trace.BinaryField, parent: mk})
			ordinal++
		}
	}
	if protoCycle(graph) {
		return contractPlan{}, contractErr("schema_invalid")
	}
	for _, s := range services {
		seen := map[string]bool{}
		for _, r := range s.rpcs {
			if seen[r.name] || !messageNames[r.in] || !messageNames[r.out] {
				return contractPlan{}, contractErr("schema_invalid")
			}
			seen[r.name] = true
			plan.facts = append(plan.facts, contractFact{key: protoOrdinal(ordinal), label: r.name, kind: trace.Endpoint})
			ordinal++
		}
	}
	return plan, nil
}

func protoOrdinal(ordinal int) string { return fmt.Sprintf("#/%04d", ordinal) }

func protoJSONName(name string) string {
	var b strings.Builder
	capitalize := false
	for i := 0; i < len(name); i++ {
		if name[i] == '_' {
			capitalize = true
			continue
		}
		if capitalize && name[i] >= 'a' && name[i] <= 'z' {
			b.WriteByte(name[i] - ('a' - 'A'))
		} else {
			b.WriteByte(name[i])
		}
		capitalize = false
	}
	return b.String()
}

func protoScalarName(name string) bool {
	switch name {
	case "double", "float", "int32", "int64", "uint32", "uint64", "sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64", "bool", "string", "bytes":
		return true
	default:
		return false
	}
}

func protobufRawSafety(s string) error {
	lower := strings.ToLower(s)
	if secretValue(s) {
		return contractErr("secret_indicator")
	}
	if unsafeFileReference(lower) || strings.Contains(lower, "/home/") || strings.Contains(lower, "/root/") || strings.Contains(lower, "/tmp/") || strings.Contains(lower, "/etc/") || strings.Contains(lower, "/var/") || strings.Contains(lower, "/data/") || strings.Contains(s, "\\\\") {
		return contractErr("schema_invalid")
	}
	for i := 0; i+2 < len(s); i++ {
		if (i == 0 || !protoNameByte(s[i-1])) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')) && s[i+1] == ':' && (s[i+2] == '/' || s[i+2] == '\\') {
			return contractErr("schema_invalid")
		}
	}
	safe := []byte(s)
	for i := range safe {
		if safe[i] == '.' && protoQualifiedDot(safe, i) {
			safe[i] = '_'
		}
	}
	for _, component := range strings.FieldsFunc(string(safe), func(r rune) bool {
		return r == '/' || r == '\\' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	}) {
		if component == "." || component == ".." {
			return contractErr("schema_invalid")
		}
	}
	return nil
}

func protoQualifiedDot(s []byte, index int) bool {
	previous := index - 1
	for previous >= 0 && (s[previous] == ' ' || s[previous] == '\t' || s[previous] == '\r' || s[previous] == '\n') {
		previous--
	}
	next := index + 1
	for next < len(s) && (s[next] == ' ' || s[next] == '\t' || s[next] == '\r' || s[next] == '\n') {
		next++
	}
	return previous >= 0 && next < len(s) && protoNameByte(s[previous]) && protoNameByte(s[next])
}

func protoNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}
func protoCycle(g map[string][]string) bool {
	active := map[string]bool{}
	done := map[string]bool{}
	var visit func(string) bool
	visit = func(n string) bool {
		if active[n] {
			return true
		}
		if done[n] {
			return false
		}
		active[n] = true
		for _, v := range g[n] {
			if visit(v) {
				return true
			}
		}
		delete(active, n)
		done[n] = true
		return false
	}
	for n := range g {
		if visit(n) {
			return true
		}
	}
	return false
}

func protobufLex(data []byte) ([]protoToken, error) {
	s := string(data)
	out := make([]protoToken, 0, 64)
	appendToken := func(token protoToken) error {
		if len(out) >= 8192 {
			return contractErrAt("preflight_limit", "budget")
		}
		out = append(out, token)
		return nil
	}
	for i := 0; i < len(s); {
		c := s[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			i++
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '/' {
			i += 2
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return nil, contractErr("schema_invalid")
			}
			i += end + 4
			continue
		}
		if strings.ContainsRune("{}()=;,[].<>", rune(c)) {
			if err := appendToken(protoToken{text: string(c)}); err != nil {
				return nil, err
			}
			i++
			continue
		}
		if c == '"' {
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					return nil, contractErr("schema_invalid")
				}
				j++
			}
			if j >= len(s) {
				return nil, contractErr("schema_invalid")
			}
			if err := appendToken(protoToken{text: s[i+1 : j], quoted: true}); err != nil {
				return nil, err
			}
			i = j + 1
			continue
		}
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' || (c >= '0' && c <= '9') {
			j := i + 1
			for j < len(s) && ((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z') || (s[j] >= '0' && s[j] <= '9') || s[j] == '_') {
				j++
			}
			if err := appendToken(protoToken{text: s[i:j]}); err != nil {
				return nil, err
			}
			i = j
			continue
		}
		return nil, contractErr("schema_invalid")
	}
	return out, nil
}

type protoParser struct {
	tokens []protoToken
	pos    int
}

func (p *protoParser) end() bool { return p.pos == len(p.tokens) }
func (p *protoParser) peek(x string) bool {
	return !p.end() && !p.tokens[p.pos].quoted && p.tokens[p.pos].text == x
}
func (p *protoParser) take(x string) bool {
	if p.peek(x) {
		p.pos++
		return true
	}
	return false
}
func (p *protoParser) takeQuoted(x string) bool {
	if !p.end() && p.tokens[p.pos].quoted && p.tokens[p.pos].text == x {
		p.pos++
		return true
	}
	return false
}
func (p *protoParser) ident() bool {
	if p.end() || p.tokens[p.pos].quoted || !protoIdentifier(p.tokens[p.pos].text) {
		return false
	}
	p.pos++
	return true
}
func (p *protoParser) name() (string, bool) {
	if p.end() || p.tokens[p.pos].quoted || !protoIdentifier(p.tokens[p.pos].text) {
		return "", false
	}
	x := p.tokens[p.pos].text
	p.pos++
	return x, true
}
func (p *protoParser) packageName() bool {
	if !p.ident() {
		return false
	}
	for p.take(".") {
		if !p.ident() {
			return false
		}
	}
	return true
}
func protoIdentifier(s string) bool {
	if len(s) == 0 || len(s) > 200 {
		return false
	}
	for i, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
		if i == 0 && !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
			return false
		}
	}
	return true
}
func (p *protoParser) message() (protoMessage, error) {
	p.take("message")
	n, ok := p.name()
	if !ok || !p.take("{") {
		return protoMessage{}, contractErr("schema_invalid")
	}
	m := protoMessage{name: n}
	for !p.take("}") {
		if p.end() {
			return m, contractErr("schema_invalid")
		}
		if p.protoUnsupported() {
			return m, contractErr("unsupported_declaration")
		}
		repeated := p.take("repeated")
		if p.peek("map") {
			return m, contractErr("unsupported_declaration")
		}
		typ, ok := p.name()
		if !ok {
			return m, contractErr("schema_invalid")
		}
		field, ok := p.name()
		if !ok || !p.take("=") {
			return m, contractErr("schema_invalid")
		}
		if p.end() || p.tokens[p.pos].quoted {
			return m, contractErr("schema_invalid")
		}
		tagS := p.tokens[p.pos].text
		p.pos++
		if p.peek("[") {
			return m, contractErr("unsupported_declaration")
		}
		if !p.take(";") {
			return m, contractErr("schema_invalid")
		}
		if repeated && !p.primitiveType(typ) {
			return m, contractErr("schema_invalid")
		}
		tag := 0
		if len(tagS) > 1 && tagS[0] == '0' {
			return m, contractErr("schema_invalid")
		}
		for _, r := range tagS {
			if r < '0' || r > '9' {
				return m, contractErr("schema_invalid")
			}
			tag = tag*10 + int(r-'0')
			if tag > 536870911 {
				return m, contractErr("schema_invalid")
			}
		}
		m.fields = append(m.fields, protoField{typ: typ, name: field, tag: tag, repeated: repeated})
	}
	return m, nil
}

func (p *protoParser) primitiveType(typ string) bool {
	switch typ {
	case "double", "float", "int32", "int64", "uint32", "uint64", "sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64", "bool", "string", "bytes":
		return true
	default:
		return false
	}
}

func (p *protoParser) protoUnsupported() bool {
	if p.end() || p.tokens[p.pos].quoted {
		return false
	}
	switch p.tokens[p.pos].text {
	case "message", "enum", "oneof", "map", "option", "import", "extend", "extensions", "group", "reserved", "optional":
		return true
	default:
		return false
	}
}
func (p *protoParser) service() (protoService, error) {
	p.take("service")
	n, ok := p.name()
	if !ok || !p.take("{") {
		return protoService{}, contractErr("schema_invalid")
	}
	s := protoService{name: n}
	for !p.take("}") {
		if !p.take("rpc") {
			return s, contractErr("unsupported_declaration")
		}
		r, ok := p.name()
		if !ok || !p.take("(") {
			return s, contractErr("schema_invalid")
		}
		if p.take("stream") {
			return s, contractErr("unsupported_declaration")
		}
		in, ok := p.name()
		if !ok || !p.take(")") || !p.take("returns") || !p.take("(") {
			return s, contractErr("schema_invalid")
		}
		if p.take("stream") {
			return s, contractErr("unsupported_declaration")
		}
		out, ok := p.name()
		if !ok || !p.take(")") {
			return s, contractErr("schema_invalid")
		}
		if !p.take(";") {
			if p.peek("[") || p.peek("{") {
				return s, contractErr("unsupported_declaration")
			}
			return s, contractErr("schema_invalid")
		}
		s.rpcs = append(s.rpcs, protoRPC{name: r, in: in, out: out})
	}
	return s, nil
}
