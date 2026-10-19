package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"modelcheck/trace"
)

// Input holds caller-supplied bytes and a logical review path, never opened here.
// The caller must not mutate Content until the adapter returns.
type Input struct {
	Path    string
	Content []byte
}

// RuleError exposes only a stable code and a non-secret locator.
type RuleError struct {
	Code    string
	Locator string
}

func (e *RuleError) Error() string { return e.Code }
func rule(code, locator string) error {
	return &RuleError{Code: "stage3b." + code, Locator: safeLocator(locator)}
}
func safeLocator(s string) string {
	if strings.HasPrefix(s, "byte:") {
		for _, c := range s[5:] {
			if c < '0' || c > '9' {
				return "input"
			}
		}
		return s
	}
	switch s {
	case "input", "metadata", "manifest", "json", "path", "digest", "schema":
		return s
	}
	return "input"
}

var admission = make(chan struct{}, 1)
var defaultBudget = budget{records: 420003, output: 512 << 20, working: 768 << 20}

type budget struct{ records, output, working uint64 }

func begin(in Input, digest string, metadataValid func() bool, metadata []string) ([]byte, func(), error) {
	if len(in.Content) < 1 || len(in.Content) > 1<<20 {
		return nil, func() {}, rule("input_length", "input")
	}
	select {
	case admission <- struct{}{}:
	default:
		return nil, func() {}, rule("resource_admission_busy", "input")
	}
	release := func() {
		select {
		case <-admission:
		default:
		}
	}
	snap := bytes.Clone(in.Content)
	if !validPath(in.Path) || !metadataValid() || !validDigest(digest) {
		release()
		return nil, func() {}, rule("metadata_invalid", "metadata")
	}
	vals := append([]string{in.Path, digest}, metadata...)
	for _, v := range vals {
		if secretValue(v) {
			release()
			return nil, func() {}, rule("secret_indicator", "metadata")
		}
	}
	h := sha256.Sum256(snap)
	if digest != hex.EncodeToString(h[:]) {
		release()
		return nil, func() {}, rule("digest_mismatch", "digest")
	}
	return snap, release, nil
}
func validPath(s string) bool {
	if len(s) == 0 || len(s) > 512 || path.Clean(s) != s || strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") || strings.Contains(s, "//") {
		return false
	}
	for _, b := range []byte(s) {
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || strings.ContainsRune("._/-", rune(b))) {
			return false
		}
	}
	for _, p := range strings.Split(s, "/") {
		if p == "." || p == ".." || p == "" {
			return false
		}
	}
	return true
}
func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, b := range []byte(s) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}

var marker = regexp.MustCompile(`(?i)authorization|cookie|password|passwd|secret|token|api[_-]?key|private[_-]?key|credential`)
var bearer = regexp.MustCompile(`(?i)\b(?:bearer|basic)[ \t]+[A-Za-z0-9._~+/=-]{8,}`)
var pem = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
var aws = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)

// secretMarker avoids allocating the regexp backtracking bitmap for ordinary
// names. Go's case-insensitive ASCII literals also fold long s and Kelvin sign.
func secretMarker(s string) bool {
	folded := strings.ReplaceAll(strings.ToLower(s), "ſ", "s")
	for _, literal := range []string{"authorization", "cookie", "password", "passwd", "secret", "token", "apikey", "api_key", "api-key", "privatekey", "private_key", "private-key", "credential"} {
		if strings.Contains(folded, literal) {
			return marker.MatchString(s)
		}
	}
	return false
}

func secretValue(s string) bool {
	return secretMarker(s) ||
		(strings.ContainsAny(s, "bB") && bearer.MatchString(s)) ||
		(strings.Contains(s, "-----BEGIN ") && pem.MatchString(s)) ||
		(strings.Contains(s, "AKIA") && aws.MatchString(s))
}
func tupleHash(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func checkBudget(records, output uint64, b budget) error {
	const overhead = 2 << 20
	max := ^uint64(0)
	if records > b.records || output > b.output || output > (max-overhead)/2 || records > (max-overhead-2*output)/256 || 2*output+256*records+overhead > b.working {
		return rule("preflight_limit", "budget")
	}
	return nil
}
func newDocument(schema string) *trace.Document {
	return &trace.Document{Schema: schema, Nodes: []trace.Node{}, Edges: []trace.Edge{}, Evidence: []trace.Evidence{}, Gaps: []trace.Gap{}}
}
func finish(d *trace.Document) ([]byte, error) {
	sort.Slice(d.Nodes, func(i, j int) bool { return d.Nodes[i].ID < d.Nodes[j].ID })
	sort.Slice(d.Edges, func(i, j int) bool { return d.Edges[i].ID < d.Edges[j].ID })
	sort.Slice(d.Evidence, func(i, j int) bool { return d.Evidence[i].ID < d.Evidence[j].ID })
	sort.Slice(d.Gaps, func(i, j int) bool { return d.Gaps[i].ID < d.Gaps[j].ID })
	if err := trace.Validate(d); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "{\n  \"schema\": %q", d.Schema)
	// Encode one bounded record at a time. Encoding the entire document with
	// json.Encoder would retain a second full output buffer alongside b.
	sections := []struct {
		name  string
		count int
		at    func(int) any
	}{
		{"nodes", len(d.Nodes), func(i int) any { return d.Nodes[i] }},
		{"edges", len(d.Edges), func(i int) any { return d.Edges[i] }},
		{"evidence", len(d.Evidence), func(i int) any { return d.Evidence[i] }},
		{"gaps", len(d.Gaps), func(i int) any { return d.Gaps[i] }},
	}
	var record bytes.Buffer
	encoder := json.NewEncoder(&record)
	encoder.SetIndent("    ", "  ")
	encoder.SetEscapeHTML(false)
	// Count using the same encoder before reserving the final buffer. Repeated
	// growth would copy the entire accumulated ledger and exceed TotalAlloc's
	// preflight bound even when the live heap fits comfortably.
	size := b.Len() + len("\n}\n")
	for _, section := range sections {
		size += len(fmt.Sprintf(",\n  %q: [", section.name)) + 1
		if section.count != 0 {
			size += len("\n  ") + section.count - 1
		}
		for i := 0; i < section.count; i++ {
			record.Reset()
			if err := encoder.Encode(section.at(i)); err != nil {
				return nil, fmt.Errorf("size protocol ledger record: %w", err)
			}
			size += len("\n    ") + record.Len() - 1
		}
	}
	b.Grow(size - b.Len())
	for _, section := range sections {
		fmt.Fprintf(&b, ",\n  %q: [", section.name)
		for i := 0; i < section.count; i++ {
			if i != 0 {
				b.WriteByte(',')
			}
			b.WriteString("\n    ")
			record.Reset()
			if err := encoder.Encode(section.at(i)); err != nil {
				return nil, fmt.Errorf("encode protocol ledger record: %w", err)
			}
			b.Write(bytes.TrimSuffix(record.Bytes(), []byte{'\n'}))
		}
		if section.count != 0 {
			b.WriteString("\n  ")
		}
		b.WriteByte(']')
	}
	b.WriteString("\n}\n")
	return b.Bytes(), nil
}
