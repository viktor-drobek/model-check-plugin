// Package skillcheck holds pure file/regex/JSON checks used by the G3
// Cucumber steps (features/g3-skill-package.feature) to verify the shape of the
// skills/model-check package. It has no engine dependencies and is only used
// from tests.
package skillcheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Frontmatter parses a YAML frontmatter block delimited by "---" lines.
// It supports "key: value" and "key: >" / "key: |" multi-line scalars, which is
// all a SKILL.md needs. It returns the fields and the body after the block.
func Frontmatter(content string) (map[string]string, string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", fmt.Errorf("no frontmatter: first line is not ---")
	}
	fields := map[string]string{}
	var key string
	var multi []string
	flush := func() {
		if key != "" && multi != nil {
			fields[key] = strings.TrimSpace(strings.Join(multi, " "))
		}
		key, multi = "", nil
	}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			flush()
			return fields, strings.Join(lines[i+1:], "\n"), nil
		}
		if multi != nil && (strings.HasPrefix(line, "  ") || strings.TrimSpace(line) == "") {
			multi = append(multi, strings.TrimSpace(line))
			continue
		}
		flush()
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("frontmatter line %d is not key: value", i+1)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if v == ">" || v == "|" {
			key, multi = k, []string{}
			continue
		}
		fields[k] = strings.Trim(v, `"'`)
	}
	return nil, "", fmt.Errorf("frontmatter not closed with ---")
}

// LineCount counts lines the way `wc -l` does for text ending in a newline,
// and counts a trailing partial line as a line.
func LineCount(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

var skillPathRe = regexp.MustCompile(`(?:references|assets|evals)/[A-Za-z0-9_./-]+`)

// SkillRelativePaths returns the distinct references/, assets/ and evals/ paths
// mentioned in a markdown text, sorted.
func SkillRelativePaths(md string) []string {
	seen := map[string]bool{}
	for _, m := range skillPathRe.FindAllString(md, -1) {
		m = strings.TrimRight(m, ".")
		seen[m] = true
	}
	return sortedKeys(seen)
}

var reqIDRe = regexp.MustCompile(`\bN?FR-\d{3}\b`)

// RequirementIDs returns the distinct FR-/NFR- identifiers in a text, sorted.
func RequirementIDs(text string) []string {
	seen := map[string]bool{}
	for _, m := range reqIDRe.FindAllString(text, -1) {
		seen[m] = true
	}
	return sortedKeys(seen)
}

// DefinesRequirement reports whether the requirements note defines id, i.e. the
// id occurs not as a suffix of a longer id (FR-001 inside NFR-001 does not count).
func DefinesRequirement(note, id string) bool {
	re := regexp.MustCompile(`(^|[^A-Z])` + regexp.QuoteMeta(id) + `\b`)
	return re.MatchString(note)
}

// CitesSources reports whether the first maxLines lines name the notes directory
// under a "Sources" lead-in.
func CitesSources(content string, maxLines int) bool {
	head := strings.Join(firstLines(content, maxLines), "\n")
	return strings.Contains(head, "Sources") && strings.Contains(head, "model-check-skill-notes/")
}

var statusTokenRe = regexp.MustCompile("(?i)status[^`\n]{0,30}`([a-z][a-z-]*)`")

// StatusTokens returns backticked lower-case tokens that appear within 30
// characters after the word "status" on the same line — the places where a
// document names a status value.
func StatusTokens(content string) []string {
	var out []string
	for _, m := range statusTokenRe.FindAllStringSubmatch(content, -1) {
		out = append(out, m[1])
	}
	return out
}

var backtickRe = regexp.MustCompile("`([^`\n]+)`")

// BacktickedIn returns the backticked spans of content whose text, lower-cased,
// equals one of the blacklist entries.
func BacktickedIn(content string, blacklist []string) []string {
	bad := map[string]bool{}
	for _, b := range blacklist {
		bad[strings.ToLower(b)] = true
	}
	var out []string
	for _, m := range backtickRe.FindAllStringSubmatch(content, -1) {
		if bad[strings.ToLower(strings.TrimSpace(m[1]))] {
			out = append(out, m[1])
		}
	}
	return out
}

// TokensOnLine finds the first line starting with prefix and returns the
// backticked tokens on it, in order.
func TokensOnLine(content, prefix string) []string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) {
			var out []string
			for _, m := range backtickRe.FindAllStringSubmatch(line, -1) {
				out = append(out, m[1])
			}
			return out
		}
	}
	return nil
}

var tocRe = regexp.MustCompile(`(?im)^#{1,3}\s+(table of )?contents\b`)

// HasTOC reports whether a heading named "Contents" or "Table of contents"
// occurs within the first maxLines lines.
func HasTOC(content string, maxLines int) bool {
	return tocRe.MatchString(strings.Join(firstLines(content, maxLines), "\n"))
}

var headingRe = regexp.MustCompile(`(?m)^(#{1,6})\s+(.*)$`)

// Headings returns all markdown headings (any level) in order.
func Headings(content string) []string {
	var out []string
	for _, m := range headingRe.FindAllStringSubmatch(content, -1) {
		out = append(out, strings.TrimSpace(m[2]))
	}
	return out
}

// HeadingsInOrder checks that headings containing each wanted word
// (case-insensitive) occur as a subsequence of the document's headings.
func HeadingsInOrder(content string, wanted []string) error {
	hs := Headings(content)
	i := 0
	for _, w := range wanted {
		found := false
		for ; i < len(hs); i++ {
			if strings.Contains(strings.ToLower(hs[i]), strings.ToLower(w)) {
				found = true
				i++
				break
			}
		}
		if !found {
			return fmt.Errorf("heading containing %q not found in order; headings: %v", w, hs)
		}
	}
	return nil
}

// Section is a heading with the text up to the next heading of the same or a
// higher level.
type Section struct {
	Title string
	Body  string
}

// Sections splits content at headings of exactly the given level.
func Sections(content string, level int) []Section {
	marker := strings.Repeat("#", level) + " "
	var out []Section
	var cur *Section
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, marker) {
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &Section{Title: strings.TrimSpace(strings.TrimPrefix(line, marker))}
			continue
		}
		if cur != nil {
			if isHeadingAtMost(line, level-1) {
				out = append(out, *cur)
				cur = nil
				continue
			}
			cur.Body += line + "\n"
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

func isHeadingAtMost(line string, level int) bool {
	if level < 1 || !strings.HasPrefix(line, "#") {
		return false
	}
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	return n <= level && n < len(line) && line[n] == ' '
}

var numberedRe = regexp.MustCompile(`^\d+\.`)

// NumberedSections returns the level-3 sections whose title starts with "N.".
func NumberedSections(content string) []Section {
	var out []Section
	for _, s := range Sections(content, 3) {
		if numberedRe.MatchString(s.Title) {
			out = append(out, s)
		}
	}
	return out
}

var corpusPathRe = regexp.MustCompile(`\b(CH\d+|App_[A-Z])/[A-Za-z0-9_.]+`)

// NamesCorpusPath reports whether text contains a corpus path such as
// CH2/mutex_flaw.pml or App_C/petrinet1.
func NamesCorpusPath(text string) bool {
	return corpusPathRe.MatchString(text)
}

var yamlKeyRe = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_-]*):`)

// TopLevelYAMLKeys returns keys at column 0 of a YAML text, in order.
func TopLevelYAMLKeys(content string) []string {
	var out []string
	for _, m := range yamlKeyRe.FindAllStringSubmatch(content, -1) {
		out = append(out, m[1])
	}
	return out
}

// TableHeaderContains reports whether some markdown table header row (a line
// starting with "|" followed by a "|---" separator line) contains every word.
func TableHeaderContains(content string, words []string) bool {
	lines := strings.Split(content, "\n")
	for i := 0; i+1 < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
			continue
		}
		sep := strings.TrimSpace(lines[i+1])
		if !strings.HasPrefix(sep, "|") || !strings.Contains(sep, "---") {
			continue
		}
		ok := true
		for _, w := range words {
			if !strings.Contains(lines[i], w) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ParseJSONFile reads and parses a JSON file into a generic value.
func ParseJSONFile(path string) (any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return v, nil
}

// Get walks a parsed JSON value along object keys.
func Get(v any, path ...string) (any, bool) {
	for _, p := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok = m[p]
		if !ok {
			return nil, false
		}
	}
	return v, true
}

// ObjectsWithoutClosedProperties walks a JSON Schema and returns the paths of
// every subschema with "type": "object" that does not set
// "additionalProperties": false.
func ObjectsWithoutClosedProperties(schema any) []string {
	var out []string
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch t := v.(type) {
		case map[string]any:
			if typ, _ := t["type"].(string); typ == "object" {
				if ap, ok := t["additionalProperties"].(bool); !ok || ap {
					out = append(out, path)
				}
			}
			for _, k := range sortedKeys(t) {
				walk(t[k], path+"/"+k)
			}
		case []any:
			for i, e := range t {
				walk(e, fmt.Sprintf("%s/%d", path, i))
			}
		}
	}
	walk(schema, "")
	return out
}

// KeysOrEnumsMentioning returns JSON paths of object keys, or of enum entries,
// that contain word (case-insensitive).
func KeysOrEnumsMentioning(v any, word string) []string {
	word = strings.ToLower(word)
	var out []string
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch t := v.(type) {
		case map[string]any:
			for _, k := range sortedKeys(t) {
				if strings.Contains(strings.ToLower(k), word) {
					out = append(out, path+"/"+k)
				}
				if k == "enum" {
					if arr, ok := t[k].([]any); ok {
						for _, e := range arr {
							if s, ok := e.(string); ok && strings.Contains(strings.ToLower(s), word) {
								out = append(out, path+"/enum:"+s)
							}
						}
					}
				}
				walk(t[k], path+"/"+k)
			}
		case []any:
			for i, e := range t {
				walk(e, fmt.Sprintf("%s/%d", path, i))
			}
		}
	}
	walk(v, "")
	return out
}

// ListDir returns the sorted base names of regular files directly under dir.
func ListDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// WalkFiles returns all regular files under root (relative paths, sorted).
func WalkFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func firstLines(s string, n int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return lines
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
