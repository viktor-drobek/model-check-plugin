package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runG1(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestPromelaParseAndCheck(t *testing.T) {
	code, out, errs := runG1(t, "parse", "--promela", "../testdata/promela/local-assert.pml")
	if code != ExitOK {
		t.Fatalf("parse exit %d: %s %s", code, out, errs)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	procs := m["processes"].([]any)
	if len(procs) != 2 || procs[0].(map[string]any)["name"] != "P:0" || procs[1].(map[string]any)["name"] != "P:1" {
		t.Fatalf("processes %v", procs)
	}
	code, out, _ = runG1(t, "check", "--promela", "../testdata/promela/local-assert.pml", "--no-timing")
	if code != ExitOK || !strings.Contains(out, `"status": "violated"`) || !strings.Contains(out, `"P:1.k"`) {
		t.Fatalf("check exit %d: %s", code, out)
	}
}

func TestPromelaRejectionIsNotExecuted(t *testing.T) {
	code, out, _ := runG1(t, "check", "--promela", "../testdata/promela/syntax-error.pml")
	if code != ExitRejected {
		t.Fatalf("exit %d: %s", code, out)
	}
	var r map[string]map[string]any
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	e := r["error"]
	if e["kind"] != "syntax" || e["status"] != "not-executed" || !strings.Contains(e["message"].(string), "line 3") {
		t.Fatalf("rejection %v", e)
	}
}

func TestPromelaDefinesAndWarnings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.pml")
	src := "byte x;\n#ifdef TWO\nactive [2] proctype A() { printf(\"%d\", x) }\n#else\nactive proctype A() { skip }\n#endif\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runG1(t, "parse", "--promela", path, "-D", "TWO")
	if code != ExitOK || strings.Count(out, `"name": "A:`) != 2 || !strings.Contains(errs, "warning: printf") {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	code, out, errs = runG1(t, "parse", "--promela", path)
	if code != ExitOK || strings.Count(out, `"name": "A:`) != 1 || errs != "" {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	code, out, _ = runG1(t, "check", "--promela", path, "-D", "TWO", "--no-timing", "--sweep")
	if code != ExitOK || !strings.Contains(out, `"warnings": [`) {
		t.Fatalf("report lacks warnings: %s", out)
	}
	if _, _, errs := runG1(t, "check", "--promela", path, "--ir", path); !strings.Contains(errs, "exactly one of") {
		t.Fatalf("two inputs accepted: %s", errs)
	}
}
