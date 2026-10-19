package source

import (
	"strings"
	"testing"

	"modelcheck/trace"
)

var _ Adapter = GoAdapter{}

func TestAnalyzeGoRecordsOnlyUnambiguousDirectCalls(t *testing.T) {
	document, err := AnalyzeGo(Input{
		Path:    "example.go",
		Content: []byte("package example\nfunc helper() {}\nfunc caller() { helper() }\n"),
	})
	if err != nil {
		t.Fatalf("AnalyzeGo() error = %v", err)
	}
	if got := len(document.Nodes); got != 2 {
		t.Fatalf("node count = %d, want 2", got)
	}
	if got := len(document.Edges); got != 1 {
		t.Fatalf("edge count = %d, want 1", got)
	}
	if got := len(document.Gaps); got != 0 {
		t.Fatalf("gap count = %d, want 0", got)
	}
	if document.Edges[0].Kind != trace.Calls || document.Evidence[2].Level != trace.Syntactic {
		t.Fatalf("call evidence = %#v, want syntactic call", document.Evidence[2])
	}
}

func TestAnalyzeGoRecordsUnresolvedAndShadowedCallsAsGaps(t *testing.T) {
	document, err := AnalyzeGo(Input{
		Path:    "example.go",
		Content: []byte("package example\nfunc helper() {}\nfunc caller() { helper := func() {}; helper() }\nfunc external() { fmt.Println(\"x\") }\n"),
	})
	if err != nil {
		t.Fatalf("AnalyzeGo() error = %v", err)
	}
	if got := len(document.Edges); got != 0 {
		t.Fatalf("edge count = %d, want 0", got)
	}
	if got := len(document.Gaps); got != 2 {
		t.Fatalf("gap count = %d, want 2", got)
	}
	for _, gap := range document.Gaps {
		if !strings.Contains(gap.Reason, "not unambiguously resolved") {
			t.Fatalf("gap reason = %q", gap.Reason)
		}
	}
}

func TestAnalyzeGoFailsClosedForUncertainCalls(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		gapTarget string
	}{
		{
			name:      "dot import",
			content:   "package example\nimport . \"fmt\"\nfunc Println(...any) {}\nfunc caller() { Println(\"x\") }\n",
			gapTarget: `call target "Println"`,
		},
		{
			name:      "type parameter",
			content:   "package example\nfunc T(string) {}\nfunc caller[T ~string](value string) { _ = T(value) }\n",
			gapTarget: `call target "T"`,
		},
		{
			name:      "function literal",
			content:   "package example\nfunc caller() { func() { unknown() }() }\n",
			gapTarget: `call target "unknown"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := AnalyzeGo(Input{Path: "example.go", Content: []byte(test.content)})
			if err != nil {
				t.Fatalf("AnalyzeGo() error = %v", err)
			}
			if got := len(document.Edges); got != 0 {
				t.Fatalf("edge count = %d, want 0", got)
			}
			for _, gap := range document.Gaps {
				if strings.Contains(gap.Reason, test.gapTarget) {
					return
				}
			}
			t.Fatalf("gap target %q not found in %#v", test.gapTarget, document.Gaps)
		})
	}
}

func TestAnalyzeGoRejectsInvalidInput(t *testing.T) {
	if _, err := AnalyzeGo(Input{Content: []byte("package example")}); err == nil || !strings.Contains(err.Error(), "path is blank") {
		t.Fatalf("blank path error = %v", err)
	}
	if _, err := AnalyzeGo(Input{Path: "broken.go", Content: []byte("package example\nfunc broken( {")}); err == nil || !strings.Contains(err.Error(), "parse Go source") {
		t.Fatalf("syntax error = %v", err)
	}
}
