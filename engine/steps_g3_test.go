package modelcheck_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cucumber/godog"

	"modelcheck/skillcheck"
)

// g3World is the per-scenario state for features/g3-skill-package.feature.
// All checks are pure file/regex/JSON checks over skills/model-check.
type g3World struct {
	skillDir  string   // absolute path of the skill package
	pluginDir string   // absolute path of model-check-plugin
	repoDir   string   // absolute path of the repository root
	statuses  []string // list captured by "lists exactly these statuses"
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
	sc.Step(`^"([^"]+)" references each of:$`, func(rel string, t *godog.Table) error {
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
			return fmt.Errorf("%s does not reference: %v", rel, missing)
		}
		return nil
	})
	sc.Step(`^"([^"]+)" mentions each of:$`, func(rel string, t *godog.Table) error {
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
	})

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
	sc.Step(`^the schema defines "transitions" items with "inputs" and "outputs" arcs whose "multiplicity" has minimum (\d+)$`, func(min int) error {
		v, err := schema()
		if err != nil {
			return err
		}
		for _, side := range []string{"inputs", "outputs"} {
			arc, ok := skillcheck.Get(v, "properties", "transitions", "items", "properties", side, "items")
			if !ok {
				return fmt.Errorf("no transitions.items.properties.%s.items", side)
			}
			// the arc schema may be inline or a $ref into $defs
			if ref, ok := skillcheck.Get(arc, "$ref"); ok {
				name := strings.TrimPrefix(ref.(string), "#/$defs/")
				arc, ok = skillcheck.Get(v, "$defs", name)
				if !ok {
					return fmt.Errorf("unresolved $ref %v", ref)
				}
			}
			m, _ := skillcheck.Get(arc, "properties", "multiplicity", "minimum")
			if m != float64(min) {
				return fmt.Errorf("%s arc multiplicity.minimum = %v, want %d", side, m, min)
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
	sc.Step(`^the schema contains no key or enum value mentioning "([^"]+)"$`, func(word string) error {
		v, err := schema()
		if err != nil {
			return err
		}
		if hits := skillcheck.KeysOrEnumsMentioning(v, word); len(hits) > 0 {
			return fmt.Errorf("schema mentions %q at %v", word, hits)
		}
		return nil
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
	sc.Step(`^every eval in "([^"]+)" has a non-empty "prompt" and an empty assertions list$`, func(_ string) error {
		_, list, err := evals()
		if err != nil {
			return err
		}
		for i, e := range list {
			p, _ := skillcheck.Get(e, "prompt")
			if s, _ := p.(string); strings.TrimSpace(s) == "" {
				return fmt.Errorf("eval %d has an empty prompt", i+1)
			}
			// skill-creator names the list "expectations"; the plan calls them assertions.
			a, ok := skillcheck.Get(e, "assertions")
			if !ok {
				a, ok = skillcheck.Get(e, "expectations")
			}
			if !ok {
				return fmt.Errorf("eval %d has neither assertions nor expectations", i+1)
			}
			if arr, _ := a.([]any); len(arr) != 0 {
				return fmt.Errorf("eval %d already has assertions; they are deferred to the evals half", i+1)
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
	sc.Step(`^no file under "([^"]+)" other than "([^"]+)" exists$`, func(dir, only string) error {
		files, err := skillcheck.WalkFiles(filepath.Join(w.skillDir, dir))
		if err != nil {
			return err
		}
		for _, f := range files {
			if f != only {
				return fmt.Errorf("unexpected file %s under %s", f, dir)
			}
		}
		return nil
	})
}
