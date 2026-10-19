package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"modelcheck/ir"
)

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

const petriDir = "../testdata/petri/"

func TestExitCodes(t *testing.T) {
	cases := []struct {
		args []string
		code int
	}{
		{[]string{}, ExitTool},
		{[]string{"frobnicate"}, ExitTool},
		{[]string{"version"}, ExitOK},
		{[]string{"check"}, ExitTool},                              // neither --petri nor --ir
		{[]string{"check", "--petri", "a", "--ir", "b"}, ExitTool}, // both
		{[]string{"check", "--petri", petriDir + "missing.json"}, ExitTool},
		{[]string{"check", "--bogus"}, ExitTool},
		{[]string{"parse", "--petri", petriDir + "inhibitor.json"}, ExitRejected},
		{[]string{"check", "--petri", petriDir + "inhibitor.json"}, ExitRejected},
		{[]string{"parse", "--petri", petriDir + "bad-weight.json"}, ExitRejected},
		{[]string{"check", "--petri", petriDir + "overflow.json"}, ExitOK}, // invalid-model is a verdict, not an error
		{[]string{"check", "--petri", petriDir + "petrinet1.json"}, ExitOK},
		{[]string{"parse", "--petri", petriDir + "petrinet1.json"}, ExitOK},
	}
	for _, c := range cases {
		code, out, errs := runCLI(t, c.args...)
		if code != c.code {
			t.Errorf("%v: exit %d, want %d\n%s%s", c.args, code, c.code, out, errs)
		}
	}
}

func TestRejectionIsJSONOnStdout(t *testing.T) {
	code, out, _ := runCLI(t, "parse", "--petri", petriDir+"inhibitor.json")
	if code != ExitRejected {
		t.Fatal(code)
	}
	var r struct {
		Error struct{ Kind, Path, Message string }
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if r.Error.Kind != "unsupported-input" || !strings.Contains(r.Error.Message, "Holzmann") || strings.Contains(out, "\\u003e") {
		t.Fatalf("%+v", r)
	}
}

func TestInvalidIRIsRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.json")
	os.WriteFile(p, []byte(`{"schema":"mcd-ir/1","name":"x","processes":[]}`), 0o644)
	code, out, _ := runCLI(t, "check", "--ir", p)
	if code != ExitRejected || !strings.Contains(out, `"kind": "ir"`) {
		t.Fatalf("exit %d: %s", code, out)
	}
}

func TestCatalogIsDeterministicAndReconstructible(t *testing.T) {
	code, first, stderr := runCLI(t, "catalog", "--petri", petriDir+"petrinet1.json")
	if code != ExitOK || stderr != "" {
		t.Fatalf("first catalog: exit %d, stderr %q", code, stderr)
	}
	code, second, stderr := runCLI(t, "catalog", "--petri", petriDir+"petrinet1.json")
	if code != ExitOK || stderr != "" {
		t.Fatalf("second catalog: exit %d, stderr %q", code, stderr)
	}
	if first != second {
		t.Fatal("catalog output is not deterministic")
	}
	catalog, err := ir.ReadStaticCatalog([]byte(first))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Reconstruct(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCatalogRejectsPositionalArguments(t *testing.T) {
	missing := t.TempDir() + "/does-not-exist.json"
	code, out, stderr := runCLI(t,
		"validate-catalog", "--catalog", "catalog.json", "--petri", missing,
		"unexpected", "--petri", petriDir+"petrinet2.json")
	if code != ExitTool || out != "" || !strings.Contains(stderr, "unexpected positional arguments") || strings.Contains(stderr, missing) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out, stderr)
	}
}

func TestInputByteLimitIsRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oversized.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte(" "), int(MaxInputBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runCLI(t, "parse", "--petri", path)
	if code != ExitRejected || errs != "" || !strings.Contains(out, `"kind": "input-too-large"`) || !strings.Contains(out, `"path": `) || !strings.Contains(out, "input limit") {
		t.Fatalf("code %d, stderr %q, output %s", code, errs, out)
	}
	code, out, errs = runCLI(t, "check", "--petri", path)
	if code != ExitRejected || errs != "" || !strings.Contains(out, `"kind": "input-too-large"`) {
		t.Fatalf("check: code %d, stderr %q, output %s", code, errs, out)
	}
}

func TestReadInputFileAcceptsExactLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exact.pml")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 64), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := ReadInputFile(path, 64)
	if err != nil || len(data) != 64 {
		t.Fatalf("ReadInputFile exact limit = %d bytes, %v", len(data), err)
	}
}

func TestCheckReportShape(t *testing.T) {
	code, out, errs := runCLI(t, "check", "--petri", petriDir+"petrinet1.json", "--budget-states", "10", "--budget-ms", "1000", "--no-timing")
	if code != ExitOK {
		t.Fatal(errs)
	}
	var r map[string]any
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	b := r["search"].(map[string]any)["budget"].(map[string]any)
	if b["states"].(float64) != 10 || b["time_ms"].(float64) != 1000 || b["depth"].(float64) != DefaultDepth {
		t.Fatalf("budget echo %v", b)
	}
	if r["search"].(map[string]any)["mode"] != "dfs" {
		t.Fatal("mode")
	}
	if strings.Contains(out, `"time_ms": 0`) || strings.Count(out, "time_ms") != 1 {
		t.Fatalf("--no-timing leaked timing:\n%s", out)
	}
	in := r["inputs"].([]any)[0].(map[string]any)
	if in["kind"] != "petri" || in["path"] != petriDir+"petrinet1.json" || len(in["sha256"].(string)) != 64 {
		t.Fatalf("inputs %v", in)
	}
}

func TestBFSFlag(t *testing.T) {
	_, out, _ := runCLI(t, "check", "--petri", petriDir+"bfs-shortest.json", "--bfs")
	if !strings.Contains(out, `"mode": "bfs"`) || !strings.Contains(out, `"summary": "t4"`) {
		t.Fatalf("%s", out)
	}
}
