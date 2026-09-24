package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
