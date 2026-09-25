package modelcheck_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cucumber/godog"

	"modelcheck/cli"
	"modelcheck/frontend/petri"
	"modelcheck/skillcheck"
)

// g3World is the per-scenario state for features/g3-skill-package.feature and
// features/g3-align.feature. Most checks are pure file/regex/JSON checks over
// skills/model-check; the alignment scenarios also call the engine's Petri
// frontend and the in-process CLI, so that the skill package is checked against
// the engine that exists rather than against a description of it.
type g3World struct {
	skillDir  string   // absolute path of the skill package
	pluginDir string   // absolute path of model-check-plugin
	repoDir   string   // absolute path of the repository root
	workDir   string   // absolute path of an evals-workspace run directory
	statuses  []string // list captured by "lists exactly these statuses"
	report    map[string]any
	property  map[string]any
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG3Steps)
}

func tableColumn(t *godog.Table) []string {
	var out []string
	for _, row := range t.Rows {
		if len(row.Cells) > 0 {
			out = append(out, strings.TrimSpace(row.Cells[0].Value))
		}
	}
	return out
}

var (
	jsonBlockRe    = regexp.MustCompile("(?s)```json\\s*\n(.*?)\n```")
	backtickSpanRe = regexp.MustCompile("`([^`\n]+)`")
	sha256Re       = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// resolveRef follows a local "$ref": "#/$defs/x" inside schema, if node is one.
func resolveRef(schema, node any) (any, bool) {
	ref, ok := skillcheck.Get(node, "$ref")
	if !ok {
		return node, true
	}
	s, _ := ref.(string)
	if !strings.HasPrefix(s, "#/") {
		return nil, false
	}
	return skillcheck.Get(schema, strings.Split(strings.TrimPrefix(s, "#/"), "/")...)
}

// jsonKeys returns the set of object keys used anywhere in v.
func jsonKeys(v any, into map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			into[k] = true
			jsonKeys(e, into)
		}
	case []any:
		for _, e := range t {
			jsonKeys(e, into)
		}
	}
}

func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// hashesUnder returns sha256 → relative path for every regular file under root.
func hashesUnder(root string) (map[string]string, error) {
	files, err := skillcheck.WalkFiles(root)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range files {
		h, err := fileSHA256(filepath.Join(root, f))
		if err != nil {
			return nil, err
		}
		out[h] = f
	}
	return out, nil
}

// sentenceWith reports whether s matches re; used for rules that must be
// stated as a sentence, not merely have their words somewhere in the file.
func sentenceWith(s string, re *regexp.Regexp) bool {
	return re.MatchString(s)
}

// phrase turns a literal phrase (with regex fragments allowed) into a pattern
// in which every space also matches a markdown line wrap.
func phrase(p string) *regexp.Regexp {
	return regexp.MustCompile(strings.ReplaceAll(p, " ", `\s+`))
}

func registerG3Steps(sc *godog.ScenarioContext) {
	w := &g3World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g3World{}
		return ctx, nil
	})

	read := func(rel string) (string, error) {
		b, err := os.ReadFile(filepath.Join(w.skillDir, rel))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	frontmatter := func() (map[string]string, string, error) {
		s, err := read("SKILL.md")
		if err != nil {
			return nil, "", err
		}
		return skillcheck.Frontmatter(s)
	}
	allSkillFiles := func() (map[string]string, error) {
		files, err := skillcheck.WalkFiles(w.skillDir)
		if err != nil {
			return nil, err
		}
		out := map[string]string{}
		for _, f := range files {
			s, err := read(f)
			if err != nil {
				return nil, err
			}
			out[f] = s
		}
		return out, nil
	}
	referenceFiles := func() (map[string]string, error) {
		names, err := skillcheck.ListDir(filepath.Join(w.skillDir, "references"))
		if err != nil {
			return nil, err
		}
		out := map[string]string{}
		for _, n := range names {
			s, err := read(filepath.Join("references", n))
			if err != nil {
				return nil, err
			}
			out[n] = s
		}
		return out, nil
	}
	mentionsEach := func(rel string, t *godog.Table) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		var missing []string
		for _, p := range tableColumn(t) {
			if !strings.Contains(s, p) {
				missing = append(missing, p)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s does not mention: %v", rel, missing)
		}
		return nil
	}
	statesRule := func(rel string, re *regexp.Regexp, what string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		if !sentenceWith(s, re) {
			return fmt.Errorf("%s does not state that %s (pattern %s)", rel, what, re)
		}
		return nil
	}

	// ---------------------------------------------------------- background
	sc.Step(`^the skill directory "([^"]+)"$`, func(rel string) error {
		plugin, err := filepath.Abs("..")
		if err != nil {
			return err
		}
		w.pluginDir = plugin
		w.repoDir = filepath.Dir(plugin)
		w.skillDir = filepath.Join(plugin, rel)
		if st, err := os.Stat(w.skillDir); err != nil || !st.IsDir() {
			return fmt.Errorf("skill directory %s missing", w.skillDir)
		}
		return nil
	})

	// ------------------------------------------------------------ SKILL.md
	sc.Step(`^the file "([^"]+)" exists$`, func(rel string) error {
		_, err := os.Stat(filepath.Join(w.skillDir, rel))
		return err
	})
	sc.Step(`^"([^"]+)" exists$`, func(rel string) error {
		_, err := os.Stat(filepath.Join(w.skillDir, rel))
		return err
	})
	sc.Step(`^the frontmatter field "([^"]+)" equals "([^"]+)"$`, func(field, want string) error {
		f, _, err := frontmatter()
		if err != nil {
			return err
		}
		if f[field] != want {
			return fmt.Errorf("frontmatter %s = %q, want %q", field, f[field], want)
		}
		return nil
	})
	sc.Step(`^the frontmatter field "([^"]+)" is non-empty$`, func(field string) error {
		f, _, err := frontmatter()
		if err != nil {
			return err
		}
		if strings.TrimSpace(f[field]) == "" {
			return fmt.Errorf("frontmatter %s is empty", field)
		}
		return nil
	})
	sc.Step(`^the frontmatter field "([^"]+)" contains each of:$`, func(field string, t *godog.Table) error {
		f, _, err := frontmatter()
		if err != nil {
			return err
		}
		var missing []string
		for _, phrase := range tableColumn(t) {
			if !strings.Contains(f[field], phrase) {
				missing = append(missing, phrase)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s lacks phrases: %q", field, missing)
		}
		return nil
	})
	sc.Step(`^the frontmatter field "([^"]+)" mentions the built-in engine$`, func(field string) error {
		f, _, err := frontmatter()
		if err != nil {
			return err
		}
		d := strings.ToLower(f[field])
		if !strings.Contains(d, "built-in") || !strings.Contains(d, "engine") {
			return fmt.Errorf("%s does not mention the built-in engine", field)
		}
		return nil
	})
	sc.Step(`^the body of "([^"]+)" has at most (\d+) lines$`, func(rel string, max int) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		_, body, err := skillcheck.Frontmatter(s)
		if err != nil {
			return err
		}
		if n := skillcheck.LineCount(body); n > max {
			return fmt.Errorf("%s body has %d lines, limit %d", rel, n, max)
		}
		return nil
	})
	sc.Step(`^every relative path referenced from "([^"]+)" exists in the skill directory$`, func(rel string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		var missing []string
		for _, p := range skillcheck.SkillRelativePaths(s) {
			if _, err := os.Stat(filepath.Join(w.skillDir, p)); err != nil {
				missing = append(missing, p)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s references missing files: %v", rel, missing)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" references each of:$`, mentionsEach)
	sc.Step(`^"([^"]+)" mentions each of:$`, mentionsEach)
	sc.Step(`^"([^"]+)" names each of:$`, mentionsEach)

	// -------------------------------------------------------- traceability
	sc.Step(`^every "FR-" or "NFR-" identifier used in the skill package exists in "([^"]+)"$`, func(noteRel string) error {
		noteB, err := os.ReadFile(filepath.Join(w.repoDir, noteRel))
		if err != nil {
			return err
		}
		files, err := allSkillFiles()
		if err != nil {
			return err
		}
		var bad []string
		for f, s := range files {
			for _, id := range skillcheck.RequirementIDs(s) {
				if !skillcheck.DefinesRequirement(string(noteB), id) {
					bad = append(bad, f+": "+id)
				}
			}
		}
		if len(bad) > 0 {
			return fmt.Errorf("unknown requirement ids: %v", bad)
		}
		return nil
	})
	sc.Step(`^every file under "references" names its source notes within its first (\d+) lines$`, func(n int) error {
		refs, err := referenceFiles()
		if err != nil {
			return err
		}
		var bad []string
		for name, s := range refs {
			if !skillcheck.CitesSources(s, n) {
				bad = append(bad, name)
			}
		}
		if len(bad) > 0 {
			return fmt.Errorf("references without a Sources line naming model-check-skill-notes/: %v", bad)
		}
		return nil
	})

	// ------------------------------------------------------------ statuses
	sc.Step(`^"([^"]+)" lists exactly these statuses:$`, func(rel string, t *godog.Table) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		want := tableColumn(t)
		got := skillcheck.TokensOnLine(s, "Status vocabulary:")
		if strings.Join(got, ",") != strings.Join(want, ",") {
			return fmt.Errorf("Status vocabulary line lists %v, want %v", got, want)
		}
		w.statuses = want
		return nil
	})
	sc.Step(`^no file in the skill package uses a status token outside that list$`, func() error {
		if w.statuses == nil {
			return fmt.Errorf("status list not captured by a previous step")
		}
		allowed := map[string]bool{}
		for _, s := range w.statuses {
			allowed[s] = true
		}
		files, err := allSkillFiles()
		if err != nil {
			return err
		}
		var bad []string
		for f, s := range files {
			for _, tok := range skillcheck.StatusTokens(s) {
				if !allowed[tok] {
					bad = append(bad, f+": `"+tok+"`")
				}
			}
		}
		if len(bad) > 0 {
			return fmt.Errorf("status tokens outside the vocabulary: %v", bad)
		}
		return nil
	})
	sc.Step(`^no file in the skill package contains a misspelt status such as "([^"]+)", "([^"]+)", "([^"]+)" or "([^"]+)" as a status$`, func(a, b, c, d string) error {
		blacklist := []string{a, b, c, d,
			"notexecuted", "not-run", "invalid model", "invalidmodel", "proven", "passed", "failed", "pass", "fail", "unsat", "no errors", "error-free"}
		files, err := allSkillFiles()
		if err != nil {
			return err
		}
		var bad []string
		for f, s := range files {
			for _, hit := range skillcheck.BacktickedIn(s, blacklist) {
				bad = append(bad, f+": `"+hit+"`")
			}
		}
		if len(bad) > 0 {
			return fmt.Errorf("misspelt or forbidden status-like tokens in backticks: %v", bad)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" lists exactly these evidence levels:$`, func(rel string, t *godog.Table) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		want := tableColumn(t)
		got := skillcheck.TokensOnLine(s, "Evidence levels:")
		if strings.Join(got, ",") != strings.Join(want, ",") {
			return fmt.Errorf("Evidence levels line lists %v, want %v", got, want)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" states that statistical evidence is not produced$`, func(rel string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		re := regexp.MustCompile(`(?i)statistical[^.\n]*not produced`)
		if !re.MatchString(s) {
			return fmt.Errorf("%s lacks a sentence 'statistical … not produced'", rel)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" has a section of forbidden phrasings that includes "([^"]+)" and "([^"]+)"$`, func(rel, p1, p2 string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		for _, sec := range skillcheck.Sections(s, 2) {
			if strings.Contains(strings.ToLower(sec.Title), "forbidden") {
				if strings.Contains(sec.Body, p1) && strings.Contains(sec.Body, p2) {
					return nil
				}
				return fmt.Errorf("forbidden section lacks %q or %q", p1, p2)
			}
		}
		return fmt.Errorf("%s has no level-2 section with 'forbidden' in its title", rel)
	})
	sc.Step(`^"([^"]+)" states that strong fairness is not supported by the engine$`, func(rel string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		re := regexp.MustCompile(`(?i)strong fairness is (not supported|unsupported)`)
		if !re.MatchString(s) {
			return fmt.Errorf("%s lacks 'strong fairness is not supported'", rel)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" states that weak fairness is supported$`, func(rel string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		re := regexp.MustCompile(`(?i)weak fairness is supported`)
		if !re.MatchString(s) {
			return fmt.Errorf("%s lacks 'weak fairness is supported'", rel)
		}
		return nil
	})

	// ----------------------------------------------------------- structure
	sc.Step(`^every file under "references" with more than (\d+) lines has a table of contents in its first (\d+) lines$`, func(limit, head int) error {
		refs, err := referenceFiles()
		if err != nil {
			return err
		}
		var bad []string
		for name, s := range refs {
			if skillcheck.LineCount(s) > limit && !skillcheck.HasTOC(s, head) {
				bad = append(bad, name)
			}
		}
		if len(bad) > 0 {
			return fmt.Errorf("long references without a table of contents: %v", bad)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" contains a markdown table whose header includes "([^"]+)" and "([^"]+)"$`, func(rel, a, b string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		if !skillcheck.TableHeaderContains(s, []string{a, b}) {
			return fmt.Errorf("%s has no table header with %q and %q", rel, a, b)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" has at least (\d+) anti-pattern entries$`, func(rel string, n int) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		if got := len(skillcheck.NumberedSections(s)); got < n {
			return fmt.Errorf("%s has %d numbered '### N.' entries, want at least %d", rel, got, n)
		}
		return nil
	})
	sc.Step(`^every anti-pattern entry in "([^"]+)" names a corpus path or says "([^"]+)"$`, func(rel, phrase string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		var bad []string
		for _, sec := range skillcheck.NumberedSections(s) {
			if !skillcheck.NamesCorpusPath(sec.Body) && !strings.Contains(strings.ToLower(sec.Body), strings.ToLower(phrase)) {
				bad = append(bad, sec.Title)
			}
		}
		if len(bad) > 0 {
			return fmt.Errorf("entries without a corpus path or %q: %v", phrase, bad)
		}
		return nil
	})

	// -------------------------------------------------------------- assets
	sc.Step(`^"([^"]+)" has these headings in this order:$`, func(rel string, t *godog.Table) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		return skillcheck.HeadingsInOrder(s, tableColumn(t))
	})
	sc.Step(`^"([^"]+)" has top-level keys:$`, func(rel string, t *godog.Table) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, k := range skillcheck.TopLevelYAMLKeys(s) {
			have[k] = true
		}
		var missing []string
		for _, k := range tableColumn(t) {
			if !have[k] {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s lacks top-level keys %v", rel, missing)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" parses as JSON$`, func(rel string) error {
		_, err := skillcheck.ParseJSONFile(filepath.Join(w.skillDir, rel))
		return err
	})
	schema := func() (any, error) {
		return skillcheck.ParseJSONFile(filepath.Join(w.skillDir, "assets/petri-net.schema.json"))
	}
	sc.Step(`^the schema declares "\$schema" as JSON Schema draft 2020-12$`, func() error {
		v, err := schema()
		if err != nil {
			return err
		}
		got, _ := skillcheck.Get(v, "$schema")
		if s, _ := got.(string); !strings.Contains(s, "json-schema.org/draft/2020-12/schema") {
			return fmt.Errorf("$schema = %v", got)
		}
		return nil
	})
	sc.Step(`^the schema defines "places" items with an integer "initial" and an optional integer "capacity" defaulting to (\d+)$`, func(def int) error {
		v, err := schema()
		if err != nil {
			return err
		}
		items, ok := skillcheck.Get(v, "properties", "places", "items")
		if !ok {
			return fmt.Errorf("no properties.places.items")
		}
		if items, ok = resolveRef(v, items); !ok {
			return fmt.Errorf("unresolved $ref in places.items")
		}
		if t, _ := skillcheck.Get(items, "properties", "initial", "type"); t != "integer" {
			return fmt.Errorf("places.items.initial.type = %v", t)
		}
		if t, _ := skillcheck.Get(items, "properties", "capacity", "type"); t != "integer" {
			return fmt.Errorf("places.items.capacity.type = %v", t)
		}
		if d, _ := skillcheck.Get(items, "properties", "capacity", "default"); d != float64(def) {
			return fmt.Errorf("places.items.capacity.default = %v, want %d", d, def)
		}
		req, _ := skillcheck.Get(items, "required")
		for _, r := range req.([]any) {
			if r == "capacity" {
				return fmt.Errorf("capacity must be optional, but it is required")
			}
		}
		return nil
	})
	sc.Step(`^the schema defines "transitions" items with "inputs" and "outputs" arcs whose "([^"]+)" has minimum (\d+)$`, func(field string, min int) error {
		v, err := schema()
		if err != nil {
			return err
		}
		items, ok := skillcheck.Get(v, "properties", "transitions", "items")
		if !ok {
			return fmt.Errorf("no properties.transitions.items")
		}
		if items, ok = resolveRef(v, items); !ok {
			return fmt.Errorf("unresolved $ref in transitions.items")
		}
		for _, side := range []string{"inputs", "outputs"} {
			arc, ok := skillcheck.Get(items, "properties", side, "items")
			if !ok {
				return fmt.Errorf("no transitions.items.properties.%s.items", side)
			}
			if arc, ok = resolveRef(v, arc); !ok {
				return fmt.Errorf("unresolved $ref in %s arc", side)
			}
			m, _ := skillcheck.Get(arc, "properties", field, "minimum")
			if m != float64(min) {
				return fmt.Errorf("%s arc %s.minimum = %v, want %d", side, field, m, min)
			}
		}
		return nil
	})
	sc.Step(`^every object in the schema sets "additionalProperties" to false$`, func() error {
		v, err := schema()
		if err != nil {
			return err
		}
		if open := skillcheck.ObjectsWithoutClosedProperties(v); len(open) > 0 {
			return fmt.Errorf("object schemas without additionalProperties=false: %v", open)
		}
		return nil
	})
	sc.Step(`^the schema's only key mentioning "([^"]+)" is the arc flag whose description says the engine rejects it$`, func(word string) error {
		v, err := schema()
		if err != nil {
			return err
		}
		hits := skillcheck.KeysOrEnumsMentioning(v, word)
		if len(hits) != 1 || hits[0] != "/$defs/arc/properties/"+word {
			return fmt.Errorf("keys mentioning %q: %v, want exactly /$defs/arc/properties/%s", word, hits, word)
		}
		d, _ := skillcheck.Get(v, "$defs", "arc", "properties", word, "description")
		if s, _ := d.(string); !regexp.MustCompile(`(?i)reject|never translates`).MatchString(s) {
			return fmt.Errorf("%s description does not say the engine rejects it: %q", word, s)
		}
		return nil
	})

	// ---------------------------------------------------- schema identity
	sc.Step(`^"([^"]+)" is byte-for-byte identical to the engine file "([^"]+)"$`, func(skillRel, engineRel string) error {
		a, err := os.ReadFile(filepath.Join(w.skillDir, skillRel))
		if err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(w.pluginDir, "engine", engineRel))
		if err != nil {
			return err
		}
		if !bytes.Equal(a, b) {
			return fmt.Errorf("%s differs from engine/%s: copy the engine's schema over the asset (cp engine/%s skills/model-check/%s)", skillRel, engineRel, engineRel, skillRel)
		}
		return nil
	})
	sc.Step(`^every "json" code block in "([^"]+)" that has a "places" key parses with the engine's Petri frontend$`, func(rel string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		n := 0
		for _, m := range jsonBlockRe.FindAllStringSubmatch(s, -1) {
			if !strings.Contains(m[1], `"places"`) {
				continue
			}
			n++
			if _, err := petri.Parse([]byte(m[1]), "example"); err != nil {
				return fmt.Errorf("%s: json example %d rejected by the Petri frontend: %v", rel, n, err)
			}
		}
		if n == 0 {
			return fmt.Errorf("%s has no json code block with a places key", rel)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" uses the schema keys "([^"]+)" and "([^"]+)" and not "([^"]+)" or "([^"]+)" in its JSON examples$`, func(rel, k1, k2, bad1, bad2 string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		keys := map[string]bool{}
		for _, m := range jsonBlockRe.FindAllStringSubmatch(s, -1) {
			var v any
			if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
				return fmt.Errorf("%s: json block does not parse: %v", rel, err)
			}
			jsonKeys(v, keys)
		}
		for _, k := range []string{k1, k2} {
			if !keys[k] {
				return fmt.Errorf("%s: no JSON example uses key %q", rel, k)
			}
		}
		for _, k := range []string{bad1, bad2} {
			if keys[k] {
				return fmt.Errorf("%s: a JSON example still uses the sketch key %q", rel, k)
			}
		}
		return nil
	})

	// -------------------------------------------------------- engine-tools
	sc.Step(`^"([^"]+)" describes exit codes 0, 1 and 2 each with a meaning$`, func(rel string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		for _, code := range []string{"0", "1", "2"} {
			re := regexp.MustCompile(`(?i)exit code ` + code + `\b[^\n]{20,}`)
			if !re.MatchString(s) {
				return fmt.Errorf("%s does not explain exit code %s", rel, code)
			}
		}
		return nil
	})
	// G1 and G2 are built, so these read as facts, not as arrivals: the three
	// steps replace "arrives with G1/G2" and "until then the CLI is the only
	// path" (steps/g3-evals-logika.md, finding 10).
	sc.Step(`^"([^"]+)" says that the flag "([^"]+)" is built in "([^"]+)"$`, func(rel, flag, step string) error {
		return statesRule(rel, regexp.MustCompile(`(?i)`+regexp.QuoteMeta(flag)+`[^\n]{0,80}is built[^\n]{0,20}`+step), "the flag "+flag+" is built in "+step)
	})
	sc.Step(`^"([^"]+)" says that the MCP layer is built in "([^"]+)"$`, func(rel, step string) error {
		return statesRule(rel, phrase(`(?i)MCP layer is built,? `+step), "the MCP layer is built in "+step)
	})
	sc.Step(`^"([^"]+)" says that the CLI and the MCP layer reach the same engine$`, func(rel string) error {
		return statesRule(rel, phrase(`(?i)CLI and the MCP layer reach the same engine`), "the CLI and the MCP layer reach the same engine")
	})
	sc.Step(`^"([^"]+)" does not call the report fields illustrative$`, func(rel string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(s), "illustrative") {
			return fmt.Errorf("%s still calls something illustrative", rel)
		}
		return nil
	})

	// ------------------------------------------------------- status rules
	sc.Step(`^"([^"]+)" states that violated carries evidence exhaustive$`, func(rel string) error {
		return statesRule(rel, phrase("`violated` carries evidence `exhaustive`"), "violated carries evidence exhaustive")
	})
	sc.Step(`^"([^"]+)" states that verified requires complete true except for reach$`, func(rel string) error {
		return statesRule(rel, phrase("`verified` requires `complete` = true[^.]*except for `reach`"), "verified requires complete = true except for reach")
	})
	sc.Step(`^"([^"]+)" states that reach is verified by a witness$`, func(rel string) error {
		return statesRule(rel, phrase("`reach`[^.]{0,20}is verified by a witness"), "reach is verified by a witness")
	})
	sc.Step(`^"([^"]+)" states that budget exhaustion gives inconclusive naming the exhausted resource$`, func(rel string) error {
		return statesRule(rel, phrase("(?i)budget exhaustion gives `inconclusive`[^.]*naming the exhausted resource"), "budget exhaustion gives inconclusive naming the exhausted resource")
	})
	sc.Step(`^"([^"]+)" states that a construct outside the subset gives not-executed$`, func(rel string) error {
		return statesRule(rel, phrase("(?i)a construct outside the subset gives `not-executed`"), "a construct outside the subset gives not-executed")
	})
	sc.Step(`^"([^"]+)" states that a domain overflow gives invalid-model$`, func(rel string) error {
		return statesRule(rel, phrase("(?i)a domain overflow gives `invalid-model`"), "a domain overflow gives invalid-model")
	})
	sc.Step(`^"([^"]+)" lists the plan §6 aggregation priority in this order:$`, func(rel string, t *godog.Table) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		const marker = "Aggregation priority:"
		i := strings.Index(s, marker)
		if i < 0 {
			return fmt.Errorf("%s has no %q line", rel, marker)
		}
		line := s[i+len(marker):]
		if j := strings.Index(line, "\n"); j >= 0 {
			line = line[:j]
		}
		want := tableColumn(t)
		parts := strings.Split(line, ">")
		if len(parts) < len(want) {
			return fmt.Errorf("%s: priority line has %d '>'-separated items, want %d", rel, len(parts), len(want))
		}
		for k, tok := range want {
			m := backtickSpanRe.FindStringSubmatch(parts[k])
			if m == nil || m[1] != tok {
				return fmt.Errorf("%s: priority item %d is %q, want `%s`", rel, k+1, strings.TrimSpace(parts[k]), tok)
			}
		}
		return nil
	})

	// ---------------------------------------------------------- size bounds
	boundsRow := func(rel, row string) (string, error) {
		s, err := read(rel)
		if err != nil {
			return "", err
		}
		inTable := false
		for _, line := range strings.Split(s, "\n") {
			l := strings.TrimSpace(line)
			if strings.HasPrefix(l, "| Class") && strings.Contains(l, "States") {
				inTable = true
				continue
			}
			if inTable {
				if !strings.HasPrefix(l, "|") {
					break
				}
				if strings.HasPrefix(l, "| "+row+" ") {
					return l, nil
				}
			}
		}
		return "", fmt.Errorf("%s has no size-bounds table row %q", rel, row)
	}
	sc.Step(`^"([^"]+)" has a size-bounds table with rows "([^"]+)" and "([^"]+)"$`, func(rel, r1, r2 string) error {
		for _, r := range []string{r1, r2} {
			if _, err := boundsRow(rel, r); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" row of that table contains "([^"]+)" for states and depth$`, func(row, val string) error {
		l, err := boundsRow("references/evidence-and-status.md", row)
		if err != nil {
			return err
		}
		if strings.Count(l, val) < 2 {
			return fmt.Errorf("row %q does not carry %q twice (states and depth): %s", row, val, l)
		}
		return nil
	})
	sc.Step(`^the "([^"]+)" row of that table contains "([^"]+)", "([^"]+)" and "([^"]+)" and "([^"]+)"$`, func(row, a, b, c, d string) error {
		l, err := boundsRow("references/evidence-and-status.md", row)
		if err != nil {
			return err
		}
		for _, v := range []string{a, b, c, d} {
			if !strings.Contains(l, v) {
				return fmt.Errorf("row %q lacks %q: %s", row, v, l)
			}
		}
		return nil
	})
	sc.Step(`^"([^"]+)" states that bounded is used when the engine can name the bound and unknown when it cannot$`, func(rel string) error {
		return statesRule(rel, phrase("`bounded` is used when the engine can name the bound[^.]*`unknown` is used when it cannot"), "bounded is used when the engine can name the bound and unknown when it cannot")
	})

	// ----------------------------------------------------------- petri-nets
	sc.Step(`^"([^"]+)" says that no fire atom is exposed by the G0 frontend$`, func(rel string) error {
		return statesRule(rel, phrase("(?i)G0 frontend exposes no `fire\\(t\\)` atom"), "the G0 frontend exposes no fire(t) atom")
	})

	// --------------------------------------------------------------- evals
	evals := func() (map[string]any, []any, error) {
		v, err := skillcheck.ParseJSONFile(filepath.Join(w.skillDir, "evals/evals.json"))
		if err != nil {
			return nil, nil, err
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("evals.json is not an object")
		}
		list, _ := m["evals"].([]any)
		return m, list, nil
	}
	sc.Step(`^"([^"]+)" has "skill_name" equal to "([^"]+)"$`, func(_ string, want string) error {
		m, _, err := evals()
		if err != nil {
			return err
		}
		if m["skill_name"] != want {
			return fmt.Errorf("skill_name = %v", m["skill_name"])
		}
		return nil
	})
	sc.Step(`^"([^"]+)" has exactly (\d+) evals with ids 1 to (\d+)$`, func(_ string, n, last int) error {
		_, list, err := evals()
		if err != nil {
			return err
		}
		if len(list) != n {
			return fmt.Errorf("%d evals, want %d", len(list), n)
		}
		for i, e := range list {
			id, _ := skillcheck.Get(e, "id")
			if id != float64(i+1) {
				return fmt.Errorf("eval %d has id %v", i, id)
			}
		}
		if last != n {
			return fmt.Errorf("ids 1 to %d cannot number %d evals", last, n)
		}
		return nil
	})
	nonEmptyAssertions := func() error {
		_, list, err := evals()
		if err != nil {
			return err
		}
		for i, e := range list {
			p, _ := skillcheck.Get(e, "prompt")
			if s, _ := p.(string); strings.TrimSpace(s) == "" {
				return fmt.Errorf("eval %d has an empty prompt", i+1)
			}
			if _, has := skillcheck.Get(e, "expectations"); has {
				return fmt.Errorf("eval %d still has an 'expectations' key; plan §8.2 names the list 'assertions'", i+1)
			}
			a, ok := skillcheck.Get(e, "assertions")
			if !ok {
				return fmt.Errorf("eval %d has no assertions", i+1)
			}
			if arr, _ := a.([]any); len(arr) == 0 {
				return fmt.Errorf("eval %d has an empty assertions list", i+1)
			}
		}
		return nil
	}
	sc.Step(`^every eval in "([^"]+)" has a non-empty "prompt" and a non-empty "assertions" list$`, func(_ string) error { return nonEmptyAssertions() })
	sc.Step(`^every eval in "([^"]+)" has a non-empty "assertions" list and no "expectations" key$`, func(_ string) error { return nonEmptyAssertions() })
	sc.Step(`^every assertion in "([^"]+)" has a "text" and a "check" whose "type" is one of:$`, func(_ string, t *godog.Table) error {
		_, list, err := evals()
		if err != nil {
			return err
		}
		allowed := map[string]bool{}
		for _, k := range tableColumn(t) {
			allowed[k] = true
		}
		for i, e := range list {
			a, _ := skillcheck.Get(e, "assertions")
			for j, as := range a.([]any) {
				txt, _ := skillcheck.Get(as, "text")
				if s, _ := txt.(string); strings.TrimSpace(s) == "" {
					return fmt.Errorf("eval %d assertion %d has no text", i+1, j+1)
				}
				typ, _ := skillcheck.Get(as, "check", "type")
				ts, _ := typ.(string)
				if !allowed[ts] {
					return fmt.Errorf("eval %d assertion %d has check type %q, not in %v", i+1, j+1, ts, tableColumn(t))
				}
				switch ts {
				case "regex", "not_regex":
					if p, _ := skillcheck.Get(as, "check", "pattern"); p == nil {
						return fmt.Errorf("eval %d assertion %d: %s without pattern", i+1, j+1, ts)
					}
				case "regex_order":
					if p, _ := skillcheck.Get(as, "check", "patterns"); p == nil {
						return fmt.Errorf("eval %d assertion %d: regex_order without patterns", i+1, j+1)
					}
				}
			}
		}
		return nil
	})
	sc.Step(`^every eval in "([^"]+)" has a "runnable_from" that is one of:$`, func(_ string, t *godog.Table) error {
		_, list, err := evals()
		if err != nil {
			return err
		}
		allowed := map[string]bool{}
		for _, k := range tableColumn(t) {
			allowed[k] = true
		}
		for i, e := range list {
			r, _ := skillcheck.Get(e, "runnable_from")
			if s, _ := r.(string); !allowed[s] {
				return fmt.Errorf("eval %d runnable_from = %v, want one of %v", i+1, r, tableColumn(t))
			}
		}
		return nil
	})
	sc.Step(`^"([^"]+)" explains that fixtures reference corpus paths and hashes instead of copying files$`, func(rel string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		l := strings.ToLower(s)
		for _, want := range []string{"promela - examples/", "hash", "path"} {
			if !strings.Contains(l, want) {
				return fmt.Errorf("%s lacks %q", rel, want)
			}
		}
		if !strings.Contains(l, "not copied") && !strings.Contains(l, "no copies") && !strings.Contains(l, "instead of copying") {
			return fmt.Errorf("%s does not say that corpus files are not copied", rel)
		}
		return nil
	})
	notACopy := func(skillRelDir, corpusRel string) error {
		corpus, err := hashesUnder(filepath.Join(w.repoDir, corpusRel))
		if err != nil {
			return err
		}
		target := filepath.Join(w.skillDir, skillRelDir)
		st, err := os.Stat(target)
		if err != nil {
			return err
		}
		var files []string
		if st.IsDir() {
			files, err = skillcheck.WalkFiles(target)
			if err != nil {
				return err
			}
		} else {
			target, files = filepath.Dir(target), []string{filepath.Base(target)}
		}
		for _, f := range files {
			h, err := fileSHA256(filepath.Join(target, f))
			if err != nil {
				return err
			}
			if orig, dup := corpus[h]; dup {
				return fmt.Errorf("%s/%s is a copy of %s/%s", skillRelDir, f, corpusRel, orig)
			}
		}
		return nil
	}
	sc.Step(`^no file under "([^"]+)" is a copy of a file under "([^"]+)"$`, notACopy)
	sc.Step(`^"([^"]+)" is not a copy of any file under "([^"]+)"$`, notACopy)
	sc.Step(`^"([^"]+)" parses with the engine's Petri frontend$`, func(rel string) error {
		b, err := os.ReadFile(filepath.Join(w.skillDir, rel))
		if err != nil {
			return err
		}
		_, err = petri.Parse(b, "fixture")
		return err
	})
	sc.Step(`^running "([^"]+)" on "([^"]+)" exits (\d+)$`, func(cmd, rel string, want int) error {
		args := append(strings.Fields(cmd)[1:], filepath.Join(w.skillDir, rel))
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != want {
			return fmt.Errorf("%s exited %d, want %d; stderr: %s; stdout: %s", cmd, code, want, stderr.String(), stdout.String())
		}
		var rep map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
			return fmt.Errorf("stdout is not JSON: %v", err)
		}
		w.report = rep
		return nil
	})
	sc.Step(`^the report has property "([^"]+)" with status "([^"]+)" and evidence "([^"]+)" and complete (true|false)$`, func(id, status, evidence, complete string) error {
		if w.report == nil {
			return fmt.Errorf("no report captured")
		}
		props, _ := w.report["properties"].([]any)
		for _, p := range props {
			pm, _ := p.(map[string]any)
			if pm["id"] != id {
				continue
			}
			if pm["status"] != status || pm["evidence"] != evidence || pm["complete"] != (complete == "true") {
				return fmt.Errorf("property %s: status %v evidence %v complete %v", id, pm["status"], pm["evidence"], pm["complete"])
			}
			w.property = pm
			return nil
		}
		return fmt.Errorf("report has no property %q", id)
	})
	sc.Step(`^that property's counterexample summary is "([^"]+)" and its non-zero final state is "([^"]+)"$`, func(summary, nonzero string) error {
		if w.property == nil {
			return fmt.Errorf("no property captured")
		}
		cex, _ := w.property["counterexample"].(map[string]any)
		if cex == nil {
			return fmt.Errorf("property %v has no counterexample", w.property["id"])
		}
		if cex["summary"] != summary {
			return fmt.Errorf("summary = %v, want %q", cex["summary"], summary)
		}
		var parts []string
		for _, v := range cex["final_state"].([]any) {
			vm := v.(map[string]any)
			if val, _ := vm["value"].(float64); val != 0 {
				parts = append(parts, fmt.Sprintf("%s=%d", vm["var"], int64(val)))
			}
		}
		if got := strings.Join(parts, " "); got != nonzero {
			return fmt.Errorf("non-zero final state = %q, want %q", got, nonzero)
		}
		return nil
	})
	sc.Step(`^every SHA-256 row in "([^"]+)" matches the file it names, resolved from the repository root$`, func(rel string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		rows := 0
		for _, line := range strings.Split(s, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "|") {
				continue
			}
			cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
			var path, hash string
			for _, c := range cells {
				for _, m := range backtickSpanRe.FindAllStringSubmatch(c, -1) {
					if sha256Re.MatchString(m[1]) && hash == "" {
						hash = m[1]
					} else if path == "" && !sha256Re.MatchString(m[1]) {
						path = m[1]
					}
				}
			}
			if hash == "" || path == "" {
				continue
			}
			rows++
			got, err := fileSHA256(filepath.Join(w.repoDir, path))
			if err != nil {
				return fmt.Errorf("%s: row for %q: %v", rel, path, err)
			}
			if got != hash {
				return fmt.Errorf("%s: %s has sha256 %s, README says %s", rel, path, got, hash)
			}
		}
		if rows == 0 {
			return fmt.Errorf("%s has no table rows with a path and a SHA-256", rel)
		}
		return nil
	})

	// -------------------------------------------------------- E3 graded run
	sc.Step(`^the evals workspace "([^"]+)"$`, func(rel string) error {
		w.workDir = filepath.Join(w.pluginDir, rel)
		if st, err := os.Stat(w.workDir); err != nil || !st.IsDir() {
			return fmt.Errorf("evals workspace %s missing", w.workDir)
		}
		return nil
	})
	grading := func(rel string) ([]map[string]any, error) {
		v, err := skillcheck.ParseJSONFile(filepath.Join(w.workDir, rel))
		if err != nil {
			return nil, err
		}
		list, ok := skillcheck.Get(v, "expectations")
		if !ok {
			return nil, fmt.Errorf("%s has no expectations list", rel)
		}
		arr, _ := list.([]any)
		if len(arr) == 0 {
			return nil, fmt.Errorf("%s has an empty expectations list", rel)
		}
		var out []map[string]any
		for i, e := range arr {
			m, _ := e.(map[string]any)
			for _, k := range []string{"text", "passed", "evidence"} {
				if _, ok := m[k]; !ok {
					return nil, fmt.Errorf("%s entry %d lacks %q", rel, i+1, k)
				}
			}
			out = append(out, m)
		}
		return out, nil
	}
	sc.Step(`^"([^"]+)" has an "expectations" list where every entry has "text", "passed" and "evidence"$`, func(rel string) error {
		_, err := grading(rel)
		return err
	})
	sc.Step(`^every entry of "([^"]+)" has passed true$`, func(rel string) error {
		list, err := grading(rel)
		if err != nil {
			return err
		}
		var failed []string
		for _, e := range list {
			if e["passed"] != true {
				failed = append(failed, fmt.Sprint(e["text"]))
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("%s: %d assertion(s) not passed: %v", rel, len(failed), failed)
		}
		return nil
	})
	sc.Step(`^at least one entry of "([^"]+)" has passed false$`, func(rel string) error {
		list, err := grading(rel)
		if err != nil {
			return err
		}
		for _, e := range list {
			if e["passed"] == false {
				return nil
			}
		}
		return fmt.Errorf("%s: every assertion passed — the baseline is not distinguished from the skill", rel)
	})
	sc.Step(`^"([^"]+)" records tokens and duration for "([^"]+)" and "([^"]+)"$`, func(rel, a, b string) error {
		v, err := skillcheck.ParseJSONFile(filepath.Join(w.workDir, rel))
		if err != nil {
			return err
		}
		for _, run := range []string{a, b} {
			for _, k := range []string{"tokens", "duration_s"} {
				x, ok := skillcheck.Get(v, run, k)
				if !ok {
					return fmt.Errorf("%s: %s lacks %s", rel, run, k)
				}
				if _, isNum := x.(float64); !isNum {
					return fmt.Errorf("%s: %s.%s is not a number: %v", rel, run, k, x)
				}
			}
		}
		return nil
	})

	// ------------------------------------------- G3 evals, stage 2 (after G1)
	evalsList := func() ([]map[string]any, error) {
		v, err := skillcheck.ParseJSONFile(filepath.Join(w.skillDir, "evals", "evals.json"))
		if err != nil {
			return nil, err
		}
		arr, ok := skillcheck.Get(v, "evals")
		if !ok {
			return nil, fmt.Errorf("evals.json has no evals")
		}
		list, _ := arr.([]any)
		var out []map[string]any
		for _, e := range list {
			m, _ := e.(map[string]any)
			out = append(out, m)
		}
		return out, nil
	}
	evalID := func(e map[string]any) int {
		f, _ := e["id"].(float64)
		return int(f)
	}
	findEval := func(id int) (map[string]any, error) {
		list, err := evalsList()
		if err != nil {
			return nil, err
		}
		for _, e := range list {
			if evalID(e) == id {
				return e, nil
			}
		}
		return nil, fmt.Errorf("evals.json has no eval with id %d", id)
	}
	// Build steps in the order the plan delivers them; "at or before G1" is a
	// rank comparison, not a string comparison.
	stepRank := map[string]int{"G0": 0, "G1": 1, "G4": 4, "G5": 5}
	evalsUpTo := func(step string) ([]map[string]any, error) {
		list, err := evalsList()
		if err != nil {
			return nil, err
		}
		limit, ok := stepRank[step]
		if !ok {
			return nil, fmt.Errorf("unknown build step %q", step)
		}
		var out []map[string]any
		for _, e := range list {
			r, ok := stepRank[fmt.Sprint(e["runnable_from"])]
			if !ok {
				return nil, fmt.Errorf("eval %d: runnable_from %v is not a known build step", evalID(e), e["runnable_from"])
			}
			if r <= limit {
				out = append(out, e)
			}
		}
		return out, nil
	}
	sc.Step(`^the evals in "([^"]+)" with "runnable_from" at or before "([^"]+)" are exactly ids "([^"]+)"$`, func(_, step, want string) error {
		list, err := evalsUpTo(step)
		if err != nil {
			return err
		}
		var ids []string
		for _, e := range list {
			ids = append(ids, fmt.Sprint(evalID(e)))
		}
		if got := strings.Join(ids, ", "); got != want {
			return fmt.Errorf("evals runnable from %s or earlier: got ids %q, want %q", step, got, want)
		}
		return nil
	})
	// The workspace directory of one eval: eval-<id>-<name>, unless the eval
	// names its own in `dir` (E2b is a variant of eval 2 and lives in
	// eval-2b-starvation-loop, where "2b" is not an id).
	evalDir := func(workspace string, id int) (string, error) {
		base := filepath.Join(w.pluginDir, workspace)
		if e, err := findEval(id); err == nil {
			if own, _ := e["dir"].(string); own != "" {
				p := filepath.Join(base, own)
				if st, err := os.Stat(p); err == nil && st.IsDir() {
					return p, nil
				}
				return "", fmt.Errorf("eval %d names dir %q, which is not a directory under %s", id, own, workspace)
			}
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			return "", err
		}
		prefix := fmt.Sprintf("eval-%d-", id)
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
				return filepath.Join(base, e.Name()), nil
			}
		}
		return "", fmt.Errorf("no directory %s* under %s", prefix, workspace)
	}
	sc.Step(`^every eval in "([^"]+)" with "runnable_from" at or before "([^"]+)" has a graded run under the workspace "([^"]+)" with "([^"]+)" and "([^"]+)"$`, func(_, step, workspace, a, b string) error {
		list, err := evalsUpTo(step)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return fmt.Errorf("no eval is runnable from %s or earlier", step)
		}
		for _, e := range list {
			id := evalID(e)
			dir, err := evalDir(workspace, id)
			if err != nil {
				return err
			}
			for _, conf := range []string{a, b} {
				p := filepath.Join(dir, conf, "grading.json")
				v, err := skillcheck.ParseJSONFile(p)
				if err != nil {
					return fmt.Errorf("eval %d, %s: %v", id, conf, err)
				}
				if got, _ := skillcheck.Get(v, "eval_id"); got != float64(id) {
					return fmt.Errorf("%s grades eval %v, want %d", p, got, id)
				}
				exp, _ := skillcheck.Get(v, "expectations")
				if l, _ := exp.([]any); len(l) == 0 {
					return fmt.Errorf("%s has no expectations", p)
				}
			}
		}
		return nil
	})
	sc.Step(`^"([^"]+)" names eval id (\d+) and the prompt of that eval in "([^"]+)"$`, func(rel string, id int, _ string) error {
		v, err := skillcheck.ParseJSONFile(filepath.Join(w.workDir, rel))
		if err != nil {
			return err
		}
		if got, _ := skillcheck.Get(v, "eval_id"); got != float64(id) {
			return fmt.Errorf("%s: eval_id = %v, want %d", rel, got, id)
		}
		e, err := findEval(id)
		if err != nil {
			return err
		}
		if got, _ := skillcheck.Get(v, "prompt"); got != e["prompt"] {
			return fmt.Errorf("%s: prompt differs from evals.json eval %d", rel, id)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" grades every assertion of eval (\d+) in "([^"]+)"$`, func(rel string, id int, _ string) error {
		list, err := grading(rel)
		if err != nil {
			return err
		}
		e, err := findEval(id)
		if err != nil {
			return err
		}
		asserts, _ := e["assertions"].([]any)
		if len(asserts) != len(list) {
			return fmt.Errorf("%s grades %d expectations, eval %d has %d assertions", rel, len(list), id, len(asserts))
		}
		for i, a := range asserts {
			am, _ := a.(map[string]any)
			if fmt.Sprint(am["text"]) != fmt.Sprint(list[i]["text"]) {
				return fmt.Errorf("%s entry %d: text %q, assertion says %q", rel, i+1, list[i]["text"], am["text"])
			}
		}
		return nil
	})
	sc.Step(`^"([^"]+)" exists in the workspace$`, func(rel string) error {
		if _, err := os.Stat(filepath.Join(w.workDir, rel)); err != nil {
			return err
		}
		return nil
	})
	workspaceJSON := func(rel string) (any, []byte, error) {
		raw, err := os.ReadFile(filepath.Join(w.workDir, rel))
		if err != nil {
			return nil, nil, err
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, nil, fmt.Errorf("%s: %v", rel, err)
		}
		return v, raw, nil
	}
	sc.Step(`^the workspace file "([^"]+)" parses as JSON$`, func(rel string) error {
		_, _, err := workspaceJSON(rel)
		return err
	})
	sc.Step(`^in the workspace file "([^"]+)" the "run_summary" keys begin with "([^"]+)" then "([^"]+)"$`, func(rel, a, b string) error {
		_, raw, err := workspaceJSON(rel)
		if err != nil {
			return err
		}
		keys, err := objectKeysInOrder(raw, "run_summary")
		if err != nil {
			return fmt.Errorf("%s: %v", rel, err)
		}
		if len(keys) < 2 || keys[0] != a || keys[1] != b {
			return fmt.Errorf("%s: run_summary keys are %v, want %s then %s first", rel, keys, a, b)
		}
		return nil
	})
	runsOf := func(rel string) ([]map[string]any, error) {
		v, _, err := workspaceJSON(rel)
		if err != nil {
			return nil, err
		}
		arr, ok := skillcheck.Get(v, "runs")
		list, _ := arr.([]any)
		if !ok || len(list) == 0 {
			return nil, fmt.Errorf("%s has no runs", rel)
		}
		var out []map[string]any
		for _, r := range list {
			m, _ := r.(map[string]any)
			out = append(out, m)
		}
		return out, nil
	}
	sc.Step(`^in the workspace file "([^"]+)" the runs of "([^"]+)" come before the runs of "([^"]+)"$`, func(rel, a, b string) error {
		runs, err := runsOf(rel)
		if err != nil {
			return err
		}
		lastA, firstB := -1, -1
		for i, r := range runs {
			switch r["configuration"] {
			case a:
				lastA = i
			case b:
				if firstB < 0 {
					firstB = i
				}
			}
		}
		if lastA < 0 || firstB < 0 {
			return fmt.Errorf("%s: runs lack a %s or a %s configuration", rel, a, b)
		}
		if lastA > firstB {
			return fmt.Errorf("%s: a %s run (index %d) follows a %s run (index %d)", rel, a, lastA, b, firstB)
		}
		return nil
	})
	sc.Step(`^every run in the workspace file "([^"]+)" has a "configuration" of "([^"]+)" or "([^"]+)" and a "result" with "([^"]+)", "([^"]+)", "([^"]+)", "([^"]+)" and "([^"]+)"$`, func(rel, a, b, f1, f2, f3, f4, f5 string) error {
		runs, err := runsOf(rel)
		if err != nil {
			return err
		}
		for i, r := range runs {
			if r["configuration"] != a && r["configuration"] != b {
				return fmt.Errorf("%s: run %d has configuration %v", rel, i, r["configuration"])
			}
			res, _ := r["result"].(map[string]any)
			for _, f := range []string{f1, f2, f3, f4, f5} {
				if _, ok := res[f]; !ok {
					return fmt.Errorf("%s: run %d result lacks %q", rel, i, f)
				}
			}
		}
		return nil
	})
	sc.Step(`^the workspace file "([^"]+)" lists "evals_run" equal to "([^"]+)"$`, func(rel, want string) error {
		v, _, err := workspaceJSON(rel)
		if err != nil {
			return err
		}
		arr, _ := skillcheck.Get(v, "metadata", "evals_run")
		list, _ := arr.([]any)
		var ids []string
		for _, x := range list {
			f, _ := x.(float64)
			ids = append(ids, fmt.Sprint(int(f)))
		}
		if got := strings.Join(ids, ", "); got != want {
			return fmt.Errorf("%s: evals_run = %q, want %q", rel, got, want)
		}
		return nil
	})

	// ------------------------------------------------ CLI on corpus files
	sc.Step(`^running "([^"]+)" on the repository file "([^"]+)" exits (\d+)$`, func(cmd, rel string, want int) error {
		args := append(strings.Fields(cmd)[1:], filepath.Join(w.repoDir, rel))
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != want {
			return fmt.Errorf("%s exited %d, want %d; stderr: %s; stdout: %.300s", cmd, code, want, stderr.String(), stdout.String())
		}
		var doc map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
			return fmt.Errorf("stdout is not JSON: %v", err)
		}
		w.report = doc
		w.property = nil
		return nil
	})
	sc.Step(`^the rejection has kind "([^"]+)", status "([^"]+)", names construct "([^"]+)" and points to line (\d+) of "([^"]+)"$`, func(kind, status, construct string, line int, file string) error {
		if w.report == nil {
			return fmt.Errorf("no CLI output captured")
		}
		e, _ := w.report["error"].(map[string]any)
		if e == nil {
			return fmt.Errorf("stdout has no error object: %v", w.report)
		}
		if e["kind"] != kind || e["status"] != status {
			return fmt.Errorf("rejection kind/status = %v/%v, want %s/%s", e["kind"], e["status"], kind, status)
		}
		msg, path := fmt.Sprint(e["message"]), fmt.Sprint(e["path"])
		if !strings.Contains(msg, construct) {
			return fmt.Errorf("message %q does not name %s", msg, construct)
		}
		if !strings.Contains(path, fmt.Sprintf("%s:%d:", file, line)) {
			return fmt.Errorf("path %q does not point to %s line %d", path, file, line)
		}
		if !strings.Contains(msg, fmt.Sprintf("(%s, line %d)", file, line)) {
			return fmt.Errorf("message %q does not say (%s, line %d)", msg, file, line)
		}
		return nil
	})
	sc.Step(`^the report counters show (\d+) states and the counterexample ends at line (\d+)$`, func(states, line int) error {
		if w.property == nil {
			return fmt.Errorf("no property selected")
		}
		counters, _ := w.property["counters"].(map[string]any)
		if got, _ := counters["states"].(float64); int(got) != states {
			return fmt.Errorf("counters.states = %v, want %d", counters["states"], states)
		}
		cex, _ := w.property["counterexample"].(map[string]any)
		steps, _ := cex["steps"].([]any)
		if len(steps) == 0 {
			return fmt.Errorf("property %v has no counterexample steps", w.property["id"])
		}
		last, _ := steps[len(steps)-1].(map[string]any)
		origin, _ := last["origin"].(map[string]any)
		if got, _ := origin["line"].(float64); int(got) != line {
			return fmt.Errorf("last step origin line = %v, want %d", origin["line"], line)
		}
		return nil
	})

	// ------------------------------------------ reference wording (G1/G2)
	sc.Step(`^"([^"]+)" says that run is accepted only as a straight-line statement in init$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)`run[^\n]{0,120}only as a straight-line statement in `init`"), "run is accepted only as a straight-line statement in init")
	})
	sc.Step(`^"([^"]+)" says that a blocking statement inside d_step gives invalid-model$`, func(rel string) error {
		return statesRule(rel, regexp.MustCompile("(?is)block\\w*[^\n]{0,60}`d_step`[^\n]{0,80}`invalid-model`"), "a blocking statement inside d_step gives invalid-model")
	})
	sc.Step(`^"([^"]+)" says that a byte overflow gives invalid-model while pan wraps silently$`, func(rel string) error {
		return statesRule(rel, regexp.MustCompile("(?is)overflow[^\n]{0,300}`invalid-model`[^\n]{0,600}wraps silently"), "a byte overflow gives invalid-model while pan wraps silently")
	})
	sc.Step(`^"([^"]+)" explains the atomic storage rule$`, func(rel string) error {
		return statesRule(rel, regexp.MustCompile("(?is)`atomic` storage rule[^\n]{0,200}not stored"), "the atomic storage rule (intermediate states not stored) is explained")
	})
	sc.Step(`^"([^"]+)" says that xr and xs are accepted as hints$`, func(rel string) error {
		return statesRule(rel, regexp.MustCompile("(?is)`xr`[^\n]{0,60}`xs`[^\n]{0,120}hints?"), "xr and xs are accepted as hints")
	})
	sc.Step(`^"([^"]+)" lists these constructs as outside the subset:$`, func(rel string, t *godog.Table) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		var body string
		for _, sec := range skillcheck.Sections(s, 2) {
			if strings.Contains(strings.ToLower(sec.Title), "outside the subset") {
				body = sec.Body
			}
		}
		if body == "" {
			return fmt.Errorf("%s has no level-2 section titled 'Outside the subset'", rel)
		}
		var missing []string
		for _, c := range tableColumn(t) {
			if !strings.Contains(body, "`"+c+"`") && !strings.Contains(body, "`"+c+" ") && !strings.Contains(body, "`"+c+"(") {
				missing = append(missing, c)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s, section 'Outside the subset', does not list: %v", rel, missing)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" has at most (\d+) lines or a table of contents$`, func(rel string, max int) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		if n := skillcheck.LineCount(s); n > max && !skillcheck.HasTOC(s, 40) {
			return fmt.Errorf("%s has %d lines (> %d) and no table of contents in its first 40 lines", rel, n, max)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" mentions every flag that "mcd ([a-z]+)" accepts according to its usage text$`, func(rel, command string) error {
		s, err := read(rel)
		if err != nil {
			return err
		}
		var stdout, stderr bytes.Buffer
		cli.Run([]string{command, "-h"}, &stdout, &stderr)
		flagRe := regexp.MustCompile(`(?m)^\s+-([A-Za-z][A-Za-z0-9-]*)`)
		var flags, missing []string
		for _, m := range flagRe.FindAllStringSubmatch(stderr.String(), -1) {
			name := m[1]
			flags = append(flags, name)
			spelled := "--" + name
			if len(name) == 1 {
				spelled = "-" + name
			}
			if !strings.Contains(s, spelled) {
				missing = append(missing, spelled)
			}
		}
		if len(flags) == 0 {
			return fmt.Errorf("mcd %s -h printed no flags: %s", command, stderr.String())
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s does not mention the mcd %s flags %v (usage lists %v)", rel, command, missing, flags)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" says that property kind ctl is not-executed until G5$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)`ctl`[^.]{0,200}`not-executed`[^.]{0,200}G5|(?is)`not-executed`[^.]{0,120}`ctl`[^.]{0,200}G5"), "the property kind ctl is not-executed until G5")
	})
	sc.Step(`^"([^"]+)" states that an absent or zero MCP budget field means the server default$`, func(rel string) error {
		return statesRule(rel, phrase("(?i)absent or zero[^.]{0,60}server default"), "an absent or zero budget field means the server default")
	})
	sc.Step(`^"([^"]+)" says that the budget rule is the same in the CLI and in MCP$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)budget rule is[^.]{0,40}identical in the CLI and in MCP|(?is)budget rule[^.]{0,60}the same in both layers"), "the budget rule is the same in the CLI and in MCP")
	})
	sc.Step(`^"([^"]+)" says that --unlimited exists in the CLI only$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)`--unlimited`[^.]{0,120}CLI only|(?is)CLI only[^.]{0,120}`--unlimited`|(?is)`--unlimited`[^.]{0,120}no MCP (counterpart|equivalent)"), "--unlimited exists in the CLI only")
	})
	sc.Step(`^"([^"]+)" says that mcd serve links the Promela frontend$`, func(rel string) error {
		if err := statesRule(rel, phrase("(?is)`mcd serve` links the Promela frontend"), "mcd serve links the Promela frontend"); err != nil {
			return err
		}
		// and the old sentence must be gone, not merely contradicted elsewhere
		body, err := read(rel)
		if err != nil {
			return err
		}
		if phrase("(?is)does not link the Promela frontend").MatchString(body) {
			return fmt.Errorf("%s still says that mcd serve does not link the Promela frontend", rel)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" says that fairness strong gives not-executed$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)(`fairness: ?strong`|`fairness` `?strong`?|fairness: strong|`--fairness strong`|strong[^.]{0,20})[^.]{0,200}`not-executed`"), "fairness strong gives not-executed")
	})
	sc.Step(`^"([^"]+)" says that the MCP examples were copied from the recorded session$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)copied from one hand-driven stdio session|(?is)were copied from[^.]{0,80}recorded[^.]{0,40}session"), "the MCP examples were copied from the recorded session")
	})
	sc.Step(`^"([^"]+)" says that Promela input goes through the promela field of mc_parse or through "([^"]+)"$`, func(rel, cliForm string) error {
		if err := statesRule(rel, phrase("(?is)Promela input goes through the `promela` field of `mc_parse`"), "Promela input goes through the promela field of mc_parse"); err != nil {
			return err
		}
		s, err := read(rel)
		if err != nil {
			return err
		}
		if !strings.Contains(s, cliForm) {
			return fmt.Errorf("%s does not mention %q", rel, cliForm)
		}
		return nil
	})

	// ------------------------------------- reference wording added in G4
	sc.Step(`^"([^"]+)" says that fairness is reported as an assumption and not as a fact about the system$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)[Ff]airness is reported as an assumption, never as a fact about the system"), "fairness is reported as an assumption and not as a fact about the system")
	})
	sc.Step(`^"([^"]+)" says that alternatingbit is lock-step so its delivery does not depend on fairness$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)lock-?step[^.]{0,400}without any fairness assumption|(?is)lock-?step[^.]{0,400}does not depend on the fairness setting"), "alternatingbit is lock-step so its delivery does not depend on fairness")
	})
	sc.Step(`^"([^"]+)" says that loop.start is the 1-based index of the first loop step$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)`loop.start` is the 1-based index of the first step of the cycle"), "loop.start is the 1-based index of the first loop step")
	})
	sc.Step(`^"([^"]+)" says how to read a stuttering process in a lasso$`, func(rel string) error {
		body, err := read(rel)
		if err != nil {
			return err
		}
		// Both reasons for a `-` step must be distinguished: the stutter
		// extension of a stopped system and the weak-fairness null step.
		for _, re := range []*regexp.Regexp{
			phrase("(?is)[Rr]eading a stuttering process in a lasso"),
			phrase("(?is)stutter extension"),
			phrase("(?is)[Ww]eak-fairness bookkeeping|(?is)null step[^.]{0,200}fairness"),
		} {
			if !re.MatchString(body) {
				return fmt.Errorf("%s does not say how to read a stuttering process in a lasso (missing %s)", rel, re)
			}
		}
		return nil
	})
	sc.Step(`^"([^"]+)" says that a formula with X gives stutter_invariant false$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)`stutter_invariant`[\\s\\S]{0,300}`false` exactly when the formula uses `X`"), "a formula with X gives stutter_invariant false")
	})
	sc.Step(`^"([^"]+)" says that the engine checks asserts over the whole state space while pan -a checks them in claim scope$`, func(rel string) error {
		body, err := read(rel)
		if err != nil {
			return err
		}
		for _, re := range []*regexp.Regexp{
			phrase("(?is)`pan -a` evaluates `assert` statements only while a claim is in scope"),
			phrase("(?is)checks the model's `assert` statements over the\\s+\\*\\*whole\\*\\*\\s+state space"),
		} {
			if !re.MatchString(body) {
				return fmt.Errorf("%s does not contrast the engine's assert scope with pan -a (missing %s)", rel, re)
			}
		}
		return nil
	})
	sc.Step(`^"([^"]+)" says that a claim reaching its end is a violation on a finite prefix$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)claim reaching its end is a violation on a finite prefix"), "a claim reaching its end is a violation on a finite prefix")
	})
	sc.Step(`^"([^"]+)" says that label atoms are not accepted by this build$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)Control-label atoms \\(`proc@label`, `proc\\[i\\]@label`\\) are not accepted by this\\s+build"), "label atoms are not accepted by this build")
	})
	sc.Step(`^"([^"]+)" says that the progress property is added when the model has progress labels$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)`progress` property is added automatically when the model has progress\\s+labels"), "the progress property is added when the model has progress labels")
	})
	sc.Step(`^"([^"]+)" says that a states or depth budget stop is bounded and a time or memory stop is unknown$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)\\*\\*states\\*\\* or \\*\\*depth\\*\\* stop is[^.]{0,20}`bounded`[^§]{0,600}\\*\\*time\\*\\* or \\*\\*memory\\*\\* stop is `unknown`"), "a states or depth budget stop is bounded and a time or memory stop is unknown")
	})
	sc.Step(`^"([^"]+)" says that ltl results carry evidence exhaustive after the G4 oracle$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)LTL results therefore\\s+carry the ordinary evidence levels"), "ltl results carry the ordinary evidence levels after the G4 oracle")
	})
	sc.Step(`^"([^"]+)" says that ctl is not-executed until G5$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)`ctl`[^.]{0,200}`not-executed`[^.]{0,200}G5|(?is)`not-executed`[^.]{0,120}`ctl`[^.]{0,200}G5|(?is)`ctl` property comes back `not-executed`"), "ctl is not-executed until G5")
	})
	// The same check as "no file under X is a copy of …", but over the evals
	// workspace set by the Given step instead of a path inside the skill.
	sc.Step(`^no file under the workspace is a copy of a file under "([^"]+)"$`, func(corpusRel string) error {
		corpus, err := hashesUnder(filepath.Join(w.repoDir, corpusRel))
		if err != nil {
			return err
		}
		files, err := skillcheck.WalkFiles(w.workDir)
		if err != nil {
			return err
		}
		for _, rel := range files {
			h, err := fileSHA256(filepath.Join(w.workDir, rel))
			if err != nil {
				return err
			}
			if orig, dup := corpus[h]; dup {
				return fmt.Errorf("%s is a copy of %s/%s", rel, corpusRel, orig)
			}
		}
		return nil
	})
	sc.Step(`^"([^"]+)" says that strong fairness is unsupported$`, func(rel string) error {
		return statesRule(rel, phrase("(?is)\\*\\*strong fairness is unsupported\\*\\*"), "strong fairness is unsupported")
	})

	// ----------------------------------------- evals.json fields (stage 3)
	sc.Step(`^the eval with id (\d+) has "([^"]+)" equal to (\d+)$`, func(id int, field string, want int) error {
		e, err := findEval(id)
		if err != nil {
			return err
		}
		got, _ := e[field].(float64)
		if int(got) != want {
			return fmt.Errorf("eval %d: %s = %v, want %d", id, field, e[field], want)
		}
		return nil
	})
	sc.Step(`^the eval with id (\d+) has "([^"]+)" equal to "([^"]+)"$`, func(id int, field, want string) error {
		e, err := findEval(id)
		if err != nil {
			return err
		}
		if got := fmt.Sprint(e[field]); got != want {
			return fmt.Errorf("eval %d: %s = %q, want %q", id, field, got, want)
		}
		return nil
	})

	// --------------------------------- temporal goldens through the CLI
	runLTL := func(base, model, formula, fairness string, want int) error {
		args := strings.Fields(base)[1:]
		args = append(args, model, "--ltl", formula, "--fairness", fairness)
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != want {
			return fmt.Errorf("%v exited %d, want %d; stderr: %s; stdout: %.300s", args, code, want, stderr.String(), stdout.String())
		}
		var doc map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
			return fmt.Errorf("stdout is not JSON: %v", err)
		}
		w.report = doc
		w.property = nil
		return nil
	}
	sc.Step(`^running "([^"]+)" on the repository file "([^"]+)" with the ltl formula "([^"]+)" and fairness "([^"]+)" exits (\d+)$`, func(base, rel, formula, fairness string, want int) error {
		return runLTL(base, filepath.Join(w.repoDir, rel), formula, fairness, want)
	})
	sc.Step(`^running "([^"]+)" on the engine test model "([^"]+)" with the ltl formula "([^"]+)" and fairness "([^"]+)" exits (\d+)$`, func(base, name, formula, fairness string, want int) error {
		return runLTL(base, filepath.Join(w.pluginDir, "engine", "testdata", "promela", name), formula, fairness, want)
	})
	temporal := func() (map[string]any, error) {
		if w.property == nil {
			return nil, fmt.Errorf("no property selected")
		}
		t, _ := w.property["temporal"].(map[string]any)
		if t == nil {
			return nil, fmt.Errorf("property %v has no temporal record", w.property["id"])
		}
		return t, nil
	}
	sc.Step(`^that property's temporal record has fairness "([^"]+)" and stutter_invariant (true|false)$`, func(fairness, stutter string) error {
		t, err := temporal()
		if err != nil {
			return err
		}
		if fmt.Sprint(t["fairness"]) != fairness {
			return fmt.Errorf("temporal.fairness = %v, want %s", t["fairness"], fairness)
		}
		if got, _ := t["stutter_invariant"].(bool); got != (stutter == "true") {
			return fmt.Errorf("temporal.stutter_invariant = %v, want %s", t["stutter_invariant"], stutter)
		}
		return nil
	})
	sc.Step(`^that property's temporal record has source "([^"]+)" and claim "([^"]+)"$`, func(source, claim string) error {
		t, err := temporal()
		if err != nil {
			return err
		}
		if fmt.Sprint(t["source"]) != source || fmt.Sprint(t["claim"]) != claim {
			return fmt.Errorf("temporal source/claim = %v/%v, want %s/%s", t["source"], t["claim"], source, claim)
		}
		return nil
	})
	sc.Step(`^that property's counters show (\d+) states$`, func(states int) error {
		if w.property == nil {
			return fmt.Errorf("no property selected")
		}
		counters, _ := w.property["counters"].(map[string]any)
		if got, _ := counters["states"].(float64); int(got) != states {
			return fmt.Errorf("counters.states = %v, want %d", counters["states"], states)
		}
		return nil
	})
	cexLoop := func() (map[string]any, []any, error) {
		if w.property == nil {
			return nil, nil, fmt.Errorf("no property selected")
		}
		cex, _ := w.property["counterexample"].(map[string]any)
		if cex == nil {
			return nil, nil, fmt.Errorf("property %v has no counterexample", w.property["id"])
		}
		loop, _ := cex["loop"].(map[string]any)
		if loop == nil {
			return nil, nil, fmt.Errorf("property %v has a counterexample without a loop", w.property["id"])
		}
		steps, _ := cex["steps"].([]any)
		return loop, steps, nil
	}
	sc.Step(`^that property's counterexample has a loop starting at step (\d+) of (\d+) steps$`, func(start, n int) error {
		loop, _, err := cexLoop()
		if err != nil {
			return err
		}
		gotStart, _ := loop["start"].(float64)
		gotSteps, _ := loop["steps"].(float64)
		if int(gotStart) != start || int(gotSteps) != n {
			return fmt.Errorf("loop = {start: %v, steps: %v}, want {start: %d, steps: %d}", loop["start"], loop["steps"], start, n)
		}
		return nil
	})
	sc.Step(`^that property's counterexample has a loop of (\d+) steps$`, func(n int) error {
		loop, _, err := cexLoop()
		if err != nil {
			return err
		}
		if got, _ := loop["steps"].(float64); int(got) != n {
			return fmt.Errorf("loop.steps = %v, want %d", loop["steps"], n)
		}
		return nil
	})
	sc.Step(`^every loop step of that property is by process "([^"]+)" or by the claim$`, func(proc string) error {
		loop, steps, err := cexLoop()
		if err != nil {
			return err
		}
		start, _ := loop["start"].(float64)
		n, _ := loop["steps"].(float64)
		if int(start)+int(n)-1 > len(steps) {
			return fmt.Errorf("loop {start %v, steps %v} exceeds the %d recorded steps", loop["start"], loop["steps"], len(steps))
		}
		for i := int(start) - 1; i < int(start)-1+int(n); i++ {
			sm, _ := steps[i].(map[string]any)
			got := fmt.Sprint(sm["process"])
			if got != proc && !strings.HasPrefix(got, "never:") && got != "np_" {
				return fmt.Errorf("loop step %d is by process %q, want %q or the claim", i+1, got, proc)
			}
		}
		return nil
	})
	sc.Step(`^that property's reason names "([^"]+)" as enabled throughout the loop and never moving$`, func(proc string) error {
		if w.property == nil {
			return fmt.Errorf("no property selected")
		}
		reason := fmt.Sprint(w.property["reason"])
		want := proc + " is enabled throughout the loop and never moves"
		if !strings.Contains(reason, want) {
			return fmt.Errorf("reason %q does not say %q", reason, want)
		}
		return nil
	})

	// ------------------------------------------- the recorded MCP session
	pluginFile := func(rel string) (string, error) {
		b, err := os.ReadFile(filepath.Join(w.pluginDir, rel))
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	jsonBlocks := func(rel string) ([]any, error) {
		body, err := pluginFile(rel)
		if err != nil {
			return nil, err
		}
		var out []any
		for _, m := range jsonBlockRe.FindAllStringSubmatch(body, -1) {
			var v any
			if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
				continue // a deliberately truncated block; the file says so
			}
			out = append(out, v)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s has no parsable json block", rel)
		}
		return out, nil
	}
	sc.Step(`^the plugin file "([^"]+)" contains a json block with "([^"]+)" equal to "([^"]+)"$`, func(rel, key, want string) error {
		blocks, err := jsonBlocks(rel)
		if err != nil {
			return err
		}
		for _, b := range blocks {
			if got, ok := skillcheck.Get(b, key); ok && fmt.Sprint(got) == want {
				return nil
			}
		}
		return fmt.Errorf("%s has no json block with %q = %q", rel, key, want)
	})
	sc.Step(`^the plugin file "([^"]+)" contains a json block that has the keys "([^"]+)", "([^"]+)" and "([^"]+)"$`, func(rel, a, b, c string) error {
		blocks, err := jsonBlocks(rel)
		if err != nil {
			return err
		}
		for _, blk := range blocks {
			m, _ := blk.(map[string]any)
			if m == nil {
				continue
			}
			_, okA := m[a]
			_, okB := m[b]
			_, okC := m[c]
			if okA && okB && okC {
				return nil
			}
		}
		return fmt.Errorf("%s has no json block with the keys %q, %q and %q", rel, a, b, c)
	})
	sc.Step(`^the plugin file "([^"]+)" mentions each of:$`, func(rel string, t *godog.Table) error {
		body, err := pluginFile(rel)
		if err != nil {
			return err
		}
		var missing []string
		for _, p := range tableColumn(t) {
			if !strings.Contains(body, p) {
				missing = append(missing, p)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s does not mention: %v", rel, missing)
		}
		return nil
	})
}

// objectKeysInOrder returns the keys of the top-level object member `key` in
// the order they appear in raw — Go maps lose that order, and the benchmark
// contract says with_skill is listed before without_skill.
func objectKeysInOrder(raw []byte, key string) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	type frame struct{ obj, expectKey, collect bool }
	var stack []frame
	var keys []string
	pendingCollect := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{':
				stack = append(stack, frame{obj: true, expectKey: true, collect: pendingCollect})
			case '[':
				stack = append(stack, frame{})
			default:
				if len(stack) > 0 && stack[len(stack)-1].collect {
					return keys, nil
				}
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1].obj {
					stack[len(stack)-1].expectKey = true
				}
			}
			pendingCollect = false
			continue
		}
		if len(stack) == 0 {
			continue
		}
		top := &stack[len(stack)-1]
		if top.obj && top.expectKey {
			k, _ := tok.(string)
			if top.collect {
				keys = append(keys, k)
			}
			pendingCollect = len(stack) == 1 && k == key
			top.expectKey = false
		} else if top.obj {
			top.expectKey = true
			pendingCollect = false
		}
	}
	return nil, fmt.Errorf("key %q not found", key)
}
