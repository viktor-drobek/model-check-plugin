package skillcheck

import (
	"reflect"
	"testing"
)

func TestFrontmatterFolded(t *testing.T) {
	src := "---\nname: model-check\ndescription: >\n  first line\n  second line «тупик»\n---\n# Body\nline\n"
	f, body, err := Frontmatter(src)
	if err != nil {
		t.Fatal(err)
	}
	if f["name"] != "model-check" {
		t.Errorf("name = %q", f["name"])
	}
	if f["description"] != "first line second line «тупик»" {
		t.Errorf("description = %q", f["description"])
	}
	if body != "# Body\nline\n" {
		t.Errorf("body = %q", body)
	}
}

func TestFrontmatterErrors(t *testing.T) {
	if _, _, err := Frontmatter("# no frontmatter"); err == nil {
		t.Error("expected error for missing frontmatter")
	}
	if _, _, err := Frontmatter("---\nname: x\n"); err == nil {
		t.Error("expected error for unclosed frontmatter")
	}
}

func TestLineCount(t *testing.T) {
	cases := map[string]int{"": 0, "a": 1, "a\n": 1, "a\nb": 2, "a\nb\n": 2}
	for in, want := range cases {
		if got := LineCount(in); got != want {
			t.Errorf("LineCount(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSkillRelativePaths(t *testing.T) {
	got := SkillRelativePaths("see `references/workflow.md` and assets/intake-card.yaml. Also `evals/evals.json`.")
	want := []string{"assets/intake-card.yaml", "evals/evals.json", "references/workflow.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestRequirementIDs(t *testing.T) {
	got := RequirementIDs("FR-001, NFR-007 and FR-001 again; FR-12 is not an id")
	want := []string{"FR-001", "NFR-007"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	note := "| NFR-001 | MUST | x |\n| FR-002 | MUST | y |"
	if DefinesRequirement(note, "FR-001") {
		t.Error("FR-001 must not be found inside NFR-001")
	}
	if !DefinesRequirement(note, "NFR-001") || !DefinesRequirement(note, "FR-002") {
		t.Error("expected NFR-001 and FR-002 to be defined")
	}
}

func TestStatusTokens(t *testing.T) {
	src := "the status is `verified`; Status: `not-executed`; statuses in `evidence-and-status.md`; status `exhaustive`"
	got := StatusTokens(src)
	want := []string{"verified", "not-executed", "exhaustive"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBacktickedIn(t *testing.T) {
	got := BacktickedIn("use `not executed` never; `verified` ok; `Proved`", []string{"not executed", "proved"})
	want := []string{"not executed", "Proved"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestTokensOnLine(t *testing.T) {
	src := "intro\nStatus vocabulary: `a`, `b-c`, `d`\nother: `x`"
	got := TokensOnLine(src, "Status vocabulary:")
	if !reflect.DeepEqual(got, []string{"a", "b-c", "d"}) {
		t.Errorf("got %v", got)
	}
	if TokensOnLine(src, "missing:") != nil {
		t.Error("expected nil for missing prefix")
	}
}

func TestHasTOC(t *testing.T) {
	if !HasTOC("# T\n\n## Contents\n- a", 40) {
		t.Error("expected TOC")
	}
	if !HasTOC("# T\n\n## Table of Contents\n", 40) {
		t.Error("expected TOC (table of contents)")
	}
	if HasTOC("# T\n\n## Context\n", 40) {
		t.Error("Context is not a TOC")
	}
	if HasTOC("a\nb\n## Contents", 2) {
		t.Error("TOC beyond maxLines must not count")
	}
}

func TestHeadingsInOrder(t *testing.T) {
	doc := "# R\n## 1. Summary\n## 2. Scope and assumptions\n## 3. Model\n"
	if err := HeadingsInOrder(doc, []string{"Summary", "Scope", "Model"}); err != nil {
		t.Error(err)
	}
	if err := HeadingsInOrder(doc, []string{"Model", "Summary"}); err == nil {
		t.Error("wrong order must fail")
	}
}

func TestSectionsAndNumbered(t *testing.T) {
	doc := "## A\n### 1. first\nbody CH2/mutex_flaw.pml\n### note\nx\n### 2. second\nno corpus model\n## B\n### 3. third\nApp_C/petrinet1\n"
	num := NumberedSections(doc)
	if len(num) != 3 {
		t.Fatalf("got %d numbered sections", len(num))
	}
	if !NamesCorpusPath(num[0].Body) || NamesCorpusPath(num[1].Body) || !NamesCorpusPath(num[2].Body) {
		t.Errorf("corpus path detection wrong: %+v", num)
	}
}

func TestTopLevelYAMLKeys(t *testing.T) {
	got := TopLevelYAMLKeys("# c\nsystem:\n  boundary: x\nstate:\n  vars: []\nbudget: 1\n")
	if !reflect.DeepEqual(got, []string{"system", "state", "budget"}) {
		t.Errorf("got %v", got)
	}
}

func TestTableHeaderContains(t *testing.T) {
	doc := "| Intent | LTL | CTL |\n|---|---|---|\n| a | b | c |\n"
	if !TableHeaderContains(doc, []string{"LTL", "CTL"}) {
		t.Error("expected header match")
	}
	if TableHeaderContains(doc, []string{"PCTL"}) {
		t.Error("unexpected match")
	}
}

func TestSchemaWalkers(t *testing.T) {
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"arcs": map[string]any{"type": "array", "items": map[string]any{
				"type":       "object",
				"properties": map[string]any{"inhibitor": map[string]any{"type": "boolean"}},
			}},
			"kind": map[string]any{"enum": []any{"normal", "Inhibitor"}},
		},
	}
	open := ObjectsWithoutClosedProperties(schema)
	if !reflect.DeepEqual(open, []string{"/properties/arcs/items"}) {
		t.Errorf("open objects = %v", open)
	}
	hits := KeysOrEnumsMentioning(schema, "inhibitor")
	want := []string{"/properties/arcs/items/properties/inhibitor", "/properties/kind/enum:Inhibitor"}
	if !reflect.DeepEqual(hits, want) {
		t.Errorf("hits = %v", hits)
	}
	if v, ok := Get(schema, "properties", "kind", "enum"); !ok || len(v.([]any)) != 2 {
		t.Error("Get failed")
	}
}
