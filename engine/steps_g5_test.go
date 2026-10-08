package modelcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/cli"
	"modelcheck/mcp"
	"modelcheck/tools/pandiff"
)

// g5World is the per-scenario state for features/g5-ctl-v1.feature. CLI
// steps go through cli.Run in-process (the same code path as the binary);
// MCP steps through an in-memory client of mcp.Server configured as
// `mcd serve` configures it.
//
// Every step text of this file is unique in the suite: godog runs strict,
// so a text shared with another feature's steps would be ambiguous, not
// shared. That is why they all read "G5 …" or a phrase no other feature
// uses.
type g5World struct {
	model  string
	isIR   bool
	stdout bytes.Buffer
	stderr bytes.Buffer
	exit   int

	srv    *mcp.Server
	cs     *sdk.ClientSession
	base   string
	sess   string
	out    map[string]any
	errTxt string

	diffModel string
	tmp       string
	cmp       *pandiff.Comparison
	triple    *pandiff.Triple
}

// g5SetDefine rewrites an object-like `#define NAME value` line.
func g5SetDefine(src, name string, value int) (string, bool) {
	lines := strings.Split(src, "\n")
	done := false
	for i, l := range lines {
		f := strings.Fields(l)
		if len(f) >= 3 && f[0] == "#define" && f[1] == name {
			lines[i] = fmt.Sprintf("#define %s\t%d", name, value)
			done = true
		}
	}
	return strings.Join(lines, "\n"), done
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG5Steps)
}

// g5PanDefines are the preprocessor symbols a model needs on the pan side.
// CH15/client_server.pml predates SPIN 6, where `return` became a reserved
// word: without the rename `spin -a` refuses the file, so the differential
// run gives both sides the same rename and compares the same model.
func g5PanDefines(model string) []string {
	if strings.HasSuffix(model, "client_server.pml") {
		return []string{"return=ret_"}
	}
	return nil
}

func registerG5Steps(sc *godog.ScenarioContext) {
	w := &g5World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g5World{}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.cs != nil {
			w.cs.Close()
		}
		if w.srv != nil {
			w.srv.Close()
		}
		if w.base != "" {
			os.RemoveAll(w.base)
		}
		if w.tmp != "" {
			os.RemoveAll(w.tmp)
		}
		return ctx, nil
	})

	// --- Given -------------------------------------------------------------

	sc.Step(`^the model "([^"]*)" of the corpus$`, func(name string) error {
		p := filepath.Join(corpusDir, name)
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.model, w.isIR = p, false
		return nil
	})
	// A name with a directory in it is taken relative to testdata/ (the saved
	// mutants live under testdata/mutate/…); a bare name is a Promela model
	// of testdata/promela/.
	sc.Step(`^the model "([^"]*)" of the engine testdata$`, func(name string) error {
		p := filepath.Join("testdata/promela", name)
		if strings.Contains(name, "/") {
			p = filepath.Join("testdata", name)
		}
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.model, w.isIR = p, false
		return nil
	})
	sc.Step(`^the IR "([^"]*)" of the engine testdata$`, func(name string) error {
		if _, err := os.Stat(name); err != nil {
			return err
		}
		w.model, w.isIR = name, true
		return nil
	})
	sc.Step(`^spin and gcc are on PATH$`, func() error {
		if !(pandiff.Tools{}).Available() {
			return godog.ErrSkip
		}
		return nil
	})
	sc.Step(`^the model "([^"]*)" of the corpus for the G5 differential check$`, func(name string) error {
		p := filepath.Join(corpusDir, name)
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.diffModel = p
		return nil
	})
	sc.Step(`^the model "([^"]*)" of the engine testdata for the G5 differential check$`, func(name string) error {
		p := filepath.Join("testdata", name)
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.diffModel = p
		return nil
	})
	// The corpus file writes `#define N 7` outright, so a -D on the command
	// line is overridden by the file itself. Cutting the ring to three nodes
	// means rewriting that one line; nothing else of the model is touched.
	sc.Step(`^the model "([^"]*)" of the corpus with N cut to (\d+) for the G5 differential check$`, func(name string, n int) error {
		src, err := os.ReadFile(filepath.Join(corpusDir, name))
		if err != nil {
			return err
		}
		out, replaced := g5SetDefine(string(src), "N", n)
		if !replaced {
			return fmt.Errorf("%s has no `#define N` line to cut", name)
		}
		dir, err := os.MkdirTemp("", "g5-model-")
		if err != nil {
			return err
		}
		w.tmp = dir
		p := filepath.Join(dir, filepath.Base(name)+".pml")
		if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
			return err
		}
		w.diffModel = p
		return nil
	})

	// --- MCP plumbing ------------------------------------------------------

	ensure := func() error {
		if w.cs != nil {
			return nil
		}
		base, err := os.MkdirTemp("", "g5-mcp-")
		if err != nil {
			return err
		}
		w.base = base
		srv, err := mcp.New(mcp.Config{SessionBase: base, Promela: mcp.PromelaViaCLI})
		if err != nil {
			return err
		}
		ct, st := sdk.NewInMemoryTransports()
		ctx := context.Background()
		if _, err := srv.Connect(ctx, st); err != nil {
			return err
		}
		cs, err := sdk.NewClient(&sdk.Implementation{Name: "g5-steps", Version: "0"}, nil).Connect(ctx, ct, nil)
		if err != nil {
			return err
		}
		w.srv, w.cs = srv, cs
		return nil
	}
	call := func(tool string, args map[string]any) error {
		if err := ensure(); err != nil {
			return err
		}
		res, err := w.cs.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			return fmt.Errorf("protocol error calling %s: %w", tool, err)
		}
		w.out, w.errTxt = nil, ""
		if res.IsError {
			for _, c := range res.Content {
				if t, ok := c.(*sdk.TextContent); ok {
					w.errTxt += t.Text
				}
			}
			return fmt.Errorf("%s: tool error: %s", tool, w.errTxt)
		}
		data, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &w.out); err != nil {
			return fmt.Errorf("%s: structured content is not an object: %v", tool, err)
		}
		if id, _ := w.out["session_id"].(string); id != "" {
			w.sess = id
		}
		return nil
	}

	sc.Step(`^a G5 session with the corpus model "([^"]*)" parsed$`, func(name string) error {
		src, err := os.ReadFile(filepath.Join(corpusDir, name))
		if err != nil {
			return err
		}
		return call("mc_parse", map[string]any{"promela": string(src)})
	})
	sc.Step(`^a G5 session with the engine testdata model "([^"]*)" parsed$`, func(name string) error {
		src, err := os.ReadFile(filepath.Join("testdata/promela", name))
		if err != nil {
			return err
		}
		return call("mc_parse", map[string]any{"promela": string(src)})
	})
	sc.Step(`^a G5 session with the IR "([^"]*)" parsed$`, func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		return call("mc_parse", map[string]any{"ir": v})
	})
	sc.Step(`^a G5 session with the Petri net "([^"]*)" parsed$`, func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		return call("mc_parse", map[string]any{"petri": v})
	})

	// --- When: the CLI -----------------------------------------------------

	run := func(args ...string) {
		w.stdout.Reset()
		w.stderr.Reset()
		w.exit = cli.Run(args, &w.stdout, &w.stderr)
	}
	input := func() []string {
		if w.isIR {
			return []string{"--ir", w.model}
		}
		return []string{"--promela", w.model}
	}
	sc.Step(`^I check it with the CTL formula "([^"]*)"$`, func(f string) error {
		run(append([]string{"check"}, append(input(), "--ctl", f, "--no-timing")...)...)
		return nil
	})
	sc.Step(`^I check it with the CTL formula "([^"]*)" under fairness "([^"]*)"$`, func(f, fair string) error {
		run(append([]string{"check"}, append(input(), "--ctl", f, "--fairness", fair, "--no-timing")...)...)
		return nil
	})
	sc.Step(`^I check the IR with the CTL formula "([^"]*)" and a state budget of (\d+)$`, func(f string, n int) error {
		run(append([]string{"check"}, append(input(), "--ctl", f, "--budget-states", strconv.Itoa(n), "--no-timing")...)...)
		return nil
	})
	sc.Step(`^I check it with the LTL formula "([^"]*)"$`, func(f string) error {
		run(append([]string{"check"}, append(input(), "--ltl", f, "--no-timing")...)...)
		return nil
	})
	sc.Step(`^I check it sweeping the whole graph$`, func() error {
		run(append([]string{"check"}, append(input(), "--sweep", "--no-timing")...)...)
		return nil
	})
	sc.Step(`^I check it sweeping the whole graph with no budget$`, func() error {
		run(append([]string{"check"}, append(input(), "--sweep", "--unlimited", "--no-timing")...)...)
		return nil
	})
	sc.Step(`^I check it sweeping the whole graph with max-procs (\d+)$`, func(n int) error {
		run(append([]string{"check"}, append(input(), "--sweep", "--max-procs", strconv.Itoa(n), "--no-timing")...)...)
		return nil
	})
	sc.Step(`^I check it for non-progress cycles$`, func() error {
		run(append([]string{"check"}, append(input(), "--progress", "--sweep", "--no-timing")...)...)
		return nil
	})
	sc.Step(`^I parse it$`, func() error {
		run(append([]string{"parse"}, input()...)...)
		return nil
	})
	sc.Step(`^I estimate the IR with (\d+) ms and target depth (\d+)$`, func(ms, depth int) error {
		run(append([]string{"check"}, append(input(), "--estimate",
			"--estimate-ms", strconv.Itoa(ms), "--target-depth", strconv.Itoa(depth))...)...)
		return nil
	})

	// --- When: MCP ---------------------------------------------------------

	sc.Step(`^I call mc_check in the G5 session with properties:$`, func(t *godog.Table) error {
		props, err := g5Props(t)
		if err != nil {
			return err
		}
		return call("mc_check", map[string]any{"session_id": w.sess, "properties": props, "no_timing": true})
	})
	sc.Step(`^I call mc_lint_property in the G5 session with kind "([^"]*)" and formula "([^"]*)"$`, func(kind, f string) error {
		return call("mc_lint_property", map[string]any{"session_id": w.sess, "kind": kind, "formula": f})
	})
	sc.Step(`^I call mc_lint_property in the G5 session with kind "([^"]*)" and expr (.*)$`, func(kind, expr string) error {
		var e any
		if err := json.Unmarshal([]byte(expr), &e); err != nil {
			return fmt.Errorf("the expr is not JSON: %w", err)
		}
		return call("mc_lint_property", map[string]any{"session_id": w.sess, "kind": kind, "expr": e})
	})
	sc.Step(`^I call mc_estimate in the G5 session with (\d+) ms and target depth (\d+)$`, func(ms, depth int) error {
		return call("mc_estimate", map[string]any{"session_id": w.sess, "ms": ms, "target_depth": depth})
	})

	// --- When: differential ------------------------------------------------

	sc.Step(`^I compare it with pan$`, func() error {
		cmp, _, _, err := pandiff.Run(context.Background(), pandiff.Tools{}, w.diffModel, g5PanDefines(w.diffModel), false)
		if err != nil {
			return err
		}
		w.cmp = cmp
		return nil
	})
	sc.Step(`^I compare the engine, the SPIN claim and pan for "([^"]*)" under fairness "([^"]*)"$`,
		func(formula, fairness string) error {
			tr, err := pandiff.RunTriple(context.Background(), pandiff.Tools{}, w.diffModel,
				g5PanDefines(w.diffModel), formula, fairness, "a")
			if err != nil {
				return err
			}
			w.triple = tr
			return nil
		})

	// --- Then: the CLI report ----------------------------------------------

	sc.Step(`^the run exits with (\d+)$`, func(code int) error {
		if w.exit != code {
			return fmt.Errorf("exit %d, want %d\nstdout: %s\nstderr: %s", w.exit, code, w.stdout.String(), w.stderr.String())
		}
		return nil
	})
	prop := func(id string) (map[string]any, error) {
		var rep struct {
			Properties []map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(w.stdout.Bytes(), &rep); err != nil {
			return nil, fmt.Errorf("report: %v (%s)", err, w.stdout.String())
		}
		for _, p := range rep.Properties {
			if p["id"] == id {
				return p, nil
			}
		}
		return nil, fmt.Errorf("no property %q in %s", id, w.stdout.String())
	}
	temporal := func(id string) (map[string]any, error) {
		p, err := prop(id)
		if err != nil {
			return nil, err
		}
		t, _ := p["temporal"].(map[string]any)
		if t == nil {
			return nil, fmt.Errorf("property %q has no temporal block", id)
		}
		return t, nil
	}
	traceOf := func(id string) (map[string]any, error) {
		p, err := prop(id)
		if err != nil {
			return nil, err
		}
		for _, k := range []string{"witness", "counterexample"} {
			if t, ok := p[k].(map[string]any); ok {
				return t, nil
			}
		}
		return nil, fmt.Errorf("property %q carries no run", id)
	}

	sc.Step(`^the G5 property "([^"]*)" is "([^"]*)" with evidence "([^"]*)"$`, func(id, status, ev string) error {
		p, err := prop(id)
		if err != nil {
			return err
		}
		if p["status"] != status || p["evidence"] != ev {
			return fmt.Errorf("property %s is %v/%v, want %s/%s (reason: %v)", id, p["status"], p["evidence"], status, ev, p["reason"])
		}
		return nil
	})
	sc.Step(`^the G5 property "([^"]*)" reports logic "([^"]*)"$`, func(id, logic string) error {
		t, err := temporal(id)
		if err != nil {
			return err
		}
		if t["logic"] != logic {
			return fmt.Errorf("property %s reports logic %v, want %s", id, t["logic"], logic)
		}
		return nil
	})
	sc.Step(`^the G5 property "([^"]*)" reports the normalised formula "([^"]*)"$`, func(id, want string) error {
		t, err := temporal(id)
		if err != nil {
			return err
		}
		if t["normalised"] != want {
			return fmt.Errorf("property %s normalises to %v, want %s", id, t["normalised"], want)
		}
		return nil
	})
	sc.Step(`^the G5 property "([^"]*)" carries no run$`, func(id string) error {
		p, err := prop(id)
		if err != nil {
			return err
		}
		if p["counterexample"] != nil || p["witness"] != nil {
			return fmt.Errorf("property %s carries a run", id)
		}
		return nil
	})
	sc.Step(`^the G5 property "([^"]*)" carries no run and says so$`, func(id string) error {
		p, err := prop(id)
		if err != nil {
			return err
		}
		if p["counterexample"] != nil || p["witness"] != nil {
			return fmt.Errorf("property %s carries a run", id)
		}
		t, err := temporal(id)
		if err != nil {
			return err
		}
		note, _ := t["witness_note"].(string)
		if !strings.Contains(note, "not available") {
			return fmt.Errorf("property %s witness_note is %q, want it to say \"not available\"", id, note)
		}
		return nil
	})
	sc.Step(`^the note of the G5 property "([^"]*)" says that (.+)$`, func(id, phrase string) error {
		t, err := temporal(id)
		if err != nil {
			return err
		}
		note, _ := t["witness_note"].(string)
		key := g5NoteKey(phrase)
		if key == "" {
			return fmt.Errorf("the step does not know the phrase %q", phrase)
		}
		if !strings.Contains(note, key) {
			return fmt.Errorf("witness_note of %s is %q, want it to contain %q", id, note, key)
		}
		return nil
	})
	sc.Step(`^the reason of the G5 property "([^"]*)" mentions "([^"]*)"$`, func(id, text string) error {
		p, err := prop(id)
		if err != nil {
			return err
		}
		reason, _ := p["reason"].(string)
		if !strings.Contains(reason, text) {
			return fmt.Errorf("reason of %s is %q, want it to mention %q", id, reason, text)
		}
		return nil
	})
	sc.Step(`^the G5 property "([^"]*)" counts (\d+) states$`, func(id string, n int) error {
		p, err := prop(id)
		if err != nil {
			return err
		}
		c, _ := p["counters"].(map[string]any)
		if got := g5Int(c["states"]); got != n {
			return fmt.Errorf("property %s counted %d states, want %d", id, got, n)
		}
		return nil
	})
	sc.Step(`^the run of the G5 property "([^"]*)" is finite and ends with "([^"]*)" equal to (\d+)$`, func(id, v string, want int) error {
		t, err := traceOf(id)
		if err != nil {
			return err
		}
		if t["loop"] != nil {
			return fmt.Errorf("the run of %s has a loop", id)
		}
		return g5FinalIs(t, v, want)
	})
	sc.Step(`^the run of the G5 property "([^"]*)" is a lasso$`, func(id string) error {
		t, err := traceOf(id)
		if err != nil {
			return err
		}
		if t["loop"] == nil {
			return fmt.Errorf("the run of %s has no loop", id)
		}
		return nil
	})
	sc.Step(`^the lasso of the G5 property "([^"]*)" is closed$`, func(id string) error {
		t, err := traceOf(id)
		if err != nil {
			return err
		}
		loop, _ := t["loop"].(map[string]any)
		if loop == nil {
			return fmt.Errorf("the run of %s has no loop", id)
		}
		start, steps := g5Int(loop["start"]), g5Int(loop["steps"])
		all, _ := t["steps"].([]any)
		if start < 1 || steps < 1 || start+steps-1 != len(all) {
			return fmt.Errorf("loop start %d, steps %d, run of %d steps: the loop must be the suffix", start, steps, len(all))
		}
		return nil
	})
	sc.Step(`^every value "([^"]*)" takes on the run of the G5 property "([^"]*)" is below (\d+)$`, func(v, id string, limit int) error {
		t, err := traceOf(id)
		if err != nil {
			return err
		}
		vals, err := g5Values(t, v)
		if err != nil {
			return err
		}
		for _, x := range vals {
			if x >= limit {
				return fmt.Errorf("%s reaches %d on the run of %s, which is not below %d", v, x, id, limit)
			}
		}
		if len(vals) == 0 {
			return fmt.Errorf("the run of %s never mentions %s", id, v)
		}
		return nil
	})
	sc.Step(`^step (\d+) of the run of the G5 property "([^"]*)" is at line (\d+), inside the inline body$`, func(n int, id string, line int) error {
		t, err := traceOf(id)
		if err != nil {
			return err
		}
		all, _ := t["steps"].([]any)
		if n < 1 || n > len(all) {
			return fmt.Errorf("the run of %s has %d steps", id, len(all))
		}
		st, _ := all[n-1].(map[string]any)
		org, _ := st["origin"].(map[string]any)
		if got := g5Int(org["line"]); got != line {
			return fmt.Errorf("step %d of %s is at line %d, want %d", n, id, got, line)
		}
		return nil
	})
	sc.Step(`^the G5 property "([^"]*)" is marked vacuous naming the atom "([^"]*)"$`, func(id, atom string) error {
		t, err := temporal(id)
		if err != nil {
			return err
		}
		if t["vacuous"] != true || t["vacuous_atom"] != atom {
			return fmt.Errorf("property %s: vacuous=%v atom=%v, want true/%s", id, t["vacuous"], t["vacuous_atom"], atom)
		}
		return nil
	})
	sc.Step(`^the G5 property "([^"]*)" is not marked vacuous$`, func(id string) error {
		t, err := temporal(id)
		if err != nil {
			return err
		}
		if t["vacuous"] == true {
			return fmt.Errorf("property %s is marked vacuous", id)
		}
		return nil
	})
	sc.Step(`^the G5 warnings mention "([^"]*)"$`, func(text string) error {
		var rep struct {
			Warnings []string `json:"warnings"`
		}
		if err := json.Unmarshal(w.stdout.Bytes(), &rep); err != nil {
			return err
		}
		for _, x := range rep.Warnings {
			if strings.Contains(x, text) {
				return nil
			}
		}
		return fmt.Errorf("warnings %q do not mention %q", rep.Warnings, text)
	})
	sc.Step(`^the G5 state count is (\d+)$`, func(n int) error {
		var rep struct {
			Properties []struct {
				Counters struct {
					States int `json:"states"`
				} `json:"counters"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(w.stdout.Bytes(), &rep); err != nil {
			return err
		}
		if len(rep.Properties) == 0 {
			return fmt.Errorf("the report has no properties")
		}
		if got := rep.Properties[0].Counters.States; got != n {
			return fmt.Errorf("state count %d, want %d", got, n)
		}
		return nil
	})
	sc.Step(`^no G5 property is violated$`, func() error {
		var rep struct {
			Properties []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(w.stdout.Bytes(), &rep); err != nil {
			return err
		}
		for _, p := range rep.Properties {
			if p.Status == "violated" {
				return fmt.Errorf("property %s is violated", p.ID)
			}
		}
		return nil
	})
	// The scenario has just run `check`, whose stdout is a report, so the IR
	// is fetched by parsing the same model again — the same frontend, one
	// call further on.
	sc.Step(`^the parsed IR declares the variables "([^"]*)"$`, func(list string) error {
		return g5ParseIRNames(w.model, list)
	})
	sc.Step(`^the parsed IR variable "([^"]*)" starts at (\d+)$`, func(name string, want int) error {
		return g5ParseIRInit(w.model, name, want)
	})

	// --- Then: rejections --------------------------------------------------

	rejection := func() (map[string]any, error) {
		var body struct {
			Error map[string]any `json:"error"`
		}
		if err := json.Unmarshal(w.stdout.Bytes(), &body); err != nil || body.Error == nil {
			return nil, fmt.Errorf("stdout is not a rejection: %s", w.stdout.String())
		}
		return body.Error, nil
	}
	sc.Step(`^the G5 rejection has kind "([^"]*)" and status "([^"]*)"$`, func(kind, status string) error {
		e, err := rejection()
		if err != nil {
			return err
		}
		if e["kind"] != kind || e["status"] != status {
			return fmt.Errorf("rejection is %v/%v, want %s/%s", e["kind"], e["status"], kind, status)
		}
		return nil
	})
	sc.Step(`^the G5 rejection mentions "([^"]*)"$`, func(text string) error {
		e, err := rejection()
		if err != nil {
			return err
		}
		msg, _ := e["message"].(string)
		if !strings.Contains(msg, text) {
			return fmt.Errorf("rejection message %q does not mention %q", msg, text)
		}
		return nil
	})
	sc.Step(`^the G5 rejection points at line (\d+)$`, func(line int) error {
		e, err := rejection()
		if err != nil {
			return err
		}
		path, _ := e["path"].(string)
		parts := strings.Split(path, ":")
		if len(parts) < 3 || parts[len(parts)-2] != strconv.Itoa(line) {
			return fmt.Errorf("rejection path %q does not point at line %d", path, line)
		}
		return nil
	})

	// --- Then: MCP ---------------------------------------------------------

	answerProp := func(id string) (map[string]any, error) {
		props, _ := w.out["properties"].([]any)
		for _, p := range props {
			m, _ := p.(map[string]any)
			if m != nil && m["id"] == id {
				return m, nil
			}
		}
		return nil, fmt.Errorf("no answer property %q in %v", id, w.out)
	}
	sc.Step(`^the G5 call is not an error$`, func() error {
		if w.out == nil {
			return fmt.Errorf("no answer (error: %s)", w.errTxt)
		}
		return nil
	})
	sc.Step(`^the G5 answer property "([^"]*)" is "([^"]*)" with evidence "([^"]*)"$`, func(id, status, ev string) error {
		p, err := answerProp(id)
		if err != nil {
			return err
		}
		if p["status"] != status || p["evidence"] != ev {
			return fmt.Errorf("answer property %s is %v/%v, want %s/%s (%v)", id, p["status"], p["evidence"], status, ev, p["reason"])
		}
		return nil
	})
	sc.Step(`^the G5 answer property "([^"]*)" reports logic "([^"]*)"$`, func(id, logic string) error {
		p, err := answerProp(id)
		if err != nil {
			return err
		}
		t, _ := p["temporal"].(map[string]any)
		if t == nil || t["logic"] != logic {
			return fmt.Errorf("answer property %s temporal %v, want logic %s", id, t, logic)
		}
		return nil
	})
	sc.Step(`^the G5 answer reason of "([^"]*)" mentions "([^"]*)"$`, func(id, text string) error {
		p, err := answerProp(id)
		if err != nil {
			return err
		}
		reason, _ := p["reason"].(string)
		if !strings.Contains(reason, text) {
			return fmt.Errorf("answer property %s: reason %q does not mention %q", id, reason, text)
		}
		return nil
	})
	sc.Step(`^the G5 answer property "([^"]*)" counts (\d+) states$`, func(id string, n int) error {
		p, err := answerProp(id)
		if err != nil {
			return err
		}
		c, _ := p["counters"].(map[string]any)
		if got := g5Int(c["states"]); got != n {
			return fmt.Errorf("answer property %s counts %d states, want %d", id, got, n)
		}
		return nil
	})
	sc.Step(`^the G5 report has a state vector of (\d+) bytes$`, func(n int) error {
		path, _ := w.out["report_path"].(string)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var rep struct {
			Model struct {
				StateBytes int `json:"state_bytes"`
			} `json:"model"`
		}
		if err := json.Unmarshal(data, &rep); err != nil {
			return err
		}
		if rep.Model.StateBytes != n {
			return fmt.Errorf("the report has a state vector of %d bytes, want %d", rep.Model.StateBytes, n)
		}
		return nil
	})
	sc.Step(`^the G5 answer is rejected with a reason that mentions "([^"]*)"$`, func(text string) error {
		if w.out == nil {
			return fmt.Errorf("no answer (error: %s)", w.errTxt)
		}
		if w.out["outcome"] != "rejected" {
			return fmt.Errorf("the answer is not a rejection: %v", w.out["outcome"])
		}
		rej, _ := w.out["rejection"].(map[string]any)
		if reason, _ := rej["reason"].(string); !strings.Contains(reason, text) {
			return fmt.Errorf("rejection reason %q does not mention %q", reason, text)
		}
		return nil
	})
	sc.Step(`^the G5 answer property "([^"]*)" has no witness$`, func(id string) error {
		p, err := answerProp(id)
		if err != nil {
			return err
		}
		if p["witness"] != nil {
			return fmt.Errorf("answer property %s has a witness", id)
		}
		return nil
	})
	sc.Step(`^the G5 answer property "([^"]*)" has no temporal field "([^"]*)"$`, func(id, field string) error {
		p, err := answerProp(id)
		if err != nil {
			return err
		}
		t, _ := p["temporal"].(map[string]any)
		if t == nil {
			return fmt.Errorf("answer property %s has no temporal block", id)
		}
		if _, ok := t[field]; ok {
			return fmt.Errorf("answer property %s carries temporal field %q = %v", id, field, t[field])
		}
		return nil
	})
	sc.Step(`^the G5 lint reports logic "([^"]*)"$`, func(logic string) error {
		if w.out["logic"] != logic {
			return fmt.Errorf("lint logic %v, want %s", w.out["logic"], logic)
		}
		return nil
	})
	sc.Step(`^the G5 lint atoms are "([^"]*)"$`, func(list string) error {
		var got []string
		for _, a := range w.out["atoms"].([]any) {
			got = append(got, fmt.Sprint(a))
		}
		want := strings.Split(list, ", ")
		if strings.Join(got, "|") != strings.Join(want, "|") {
			return fmt.Errorf("atoms %v, want %v", got, want)
		}
		return nil
	})
	sc.Step(`^the G5 lint notes mention "([^"]*)"$`, func(text string) error {
		notes, _ := w.out["notes"].([]any)
		for _, n := range notes {
			if strings.Contains(fmt.Sprint(n), text) {
				return nil
			}
		}
		return fmt.Errorf("lint notes %v do not mention %q", notes, text)
	})
	sc.Step(`^the G5 lint says the expression is (constant|not constant)$`, func(which string) error {
		got, ok := w.out["constant"].(bool)
		if !ok {
			return fmt.Errorf("the lint has no constant field: %v", w.out)
		}
		if got != (which == "constant") {
			return fmt.Errorf("lint constant = %v, want %s (notes %v)", got, which, w.out["notes"])
		}
		return nil
	})
	sc.Step(`^the G5 lint notes do not mention "([^"]*)"$`, func(text string) error {
		notes, _ := w.out["notes"].([]any)
		for _, n := range notes {
			if strings.Contains(fmt.Sprint(n), text) {
				return fmt.Errorf("lint note %q mentions %q", n, text)
			}
		}
		return nil
	})
	sc.Step(`^the G5 lint reports a type error mentioning "([^"]*)"$`, func(text string) error {
		msg, _ := w.out["type_error"].(string)
		if !strings.Contains(msg, text) {
			return fmt.Errorf("lint type_error %q does not mention %q", msg, text)
		}
		return nil
	})

	// --- Then: estimate ----------------------------------------------------

	levels := func() ([]any, error) {
		g, _ := w.out["growth"].(map[string]any)
		if g == nil {
			return nil, fmt.Errorf("the answer has no growth block: %v", w.out)
		}
		l, _ := g["per_level"].([]any)
		return l, nil
	}
	sc.Step(`^the G5 estimate has at least (\d+) growth levels$`, func(n int) error {
		l, err := levels()
		if err != nil {
			return err
		}
		if len(l) < n {
			return fmt.Errorf("%d growth levels, want at least %d", len(l), n)
		}
		return nil
	})
	sc.Step(`^the G5 estimate levels grow from one depth to the next$`, func() error {
		l, err := levels()
		if err != nil {
			return err
		}
		prev := -1
		for _, x := range l {
			m, _ := x.(map[string]any)
			s := g5Int(m["states"])
			if s < prev {
				return fmt.Errorf("level counts are not monotone: %v", l)
			}
			prev = s
		}
		if len(l) < 2 || g5Int(l[len(l)-1].(map[string]any)["states"]) <= g5Int(l[0].(map[string]any)["states"]) {
			return fmt.Errorf("the levels do not grow: %v", l)
		}
		return nil
	})
	sc.Step(`^the G5 estimate growth rate names the levels it was fitted over$`, func() error {
		g, _ := w.out["growth"].(map[string]any)
		basis, _ := g["rate_basis"].(string)
		if !strings.Contains(basis, "levels") {
			return fmt.Errorf("rate basis %q does not name the levels", basis)
		}
		if _, ok := g["rate"]; !ok {
			return fmt.Errorf("the growth block has no rate")
		}
		return nil
	})
	sc.Step(`^the G5 estimate projects a state count for depth (\d+)$`, func(d int) error {
		p, _ := w.out["projection"].(map[string]any)
		if p == nil {
			return fmt.Errorf("no projection")
		}
		if g5Int(p["target_depth"]) != d || g5Int(p["states_at_target"]) <= 0 {
			return fmt.Errorf("projection %v does not give a count for depth %d", p, d)
		}
		return nil
	})
	sc.Step(`^the G5 estimate size class is "([^"]*)"$`, func(class string) error {
		s, _ := w.out["size"].(map[string]any)
		if s == nil || s["class"] != class {
			return fmt.Errorf("size %v, want class %s", s, class)
		}
		return nil
	})
	sc.Step(`^the G5 estimate recommendation mentions "([^"]*)"$`, func(text string) error {
		s, _ := w.out["size"].(map[string]any)
		rec, _ := s["recommendation"].(string)
		if !strings.Contains(rec, text) {
			return fmt.Errorf("recommendation %q does not mention %q", rec, text)
		}
		return nil
	})
	sc.Step(`^the G5 estimate says it is not a verification result$`, func() error {
		note, _ := w.out["note"].(string)
		if !strings.Contains(note, "not a verification result") {
			return fmt.Errorf("note %q does not say it is not a verification result", note)
		}
		return nil
	})
	sc.Step(`^the G5 estimate is complete$`, func() error {
		if w.out["complete"] != true {
			return fmt.Errorf("the estimate is not complete: %v", w.out["complete"])
		}
		return nil
	})
	sc.Step(`^the G5 estimate projection evidence is "([^"]*)"$`, func(ev string) error {
		p, _ := w.out["projection"].(map[string]any)
		if p == nil || p["evidence"] != ev {
			return fmt.Errorf("projection %v, want evidence %s", p, ev)
		}
		return nil
	})
	sc.Step(`^the G5 estimate output names the size class "([^"]*)"$`, func(class string) error {
		var doc map[string]any
		if err := json.Unmarshal(w.stdout.Bytes(), &doc); err != nil {
			return fmt.Errorf("stdout is not an estimate: %s", w.stdout.String())
		}
		s, _ := doc["size"].(map[string]any)
		if s == nil || s["class"] != class {
			return fmt.Errorf("size %v, want class %s", s, class)
		}
		return nil
	})
	sc.Step(`^the G5 estimate output has a growth rate and per-level counts$`, func() error {
		var doc map[string]any
		if err := json.Unmarshal(w.stdout.Bytes(), &doc); err != nil {
			return err
		}
		g, _ := doc["growth"].(map[string]any)
		if g == nil {
			return fmt.Errorf("no growth block")
		}
		l, _ := g["per_level"].([]any)
		if len(l) < 2 {
			return fmt.Errorf("%d per-level counts", len(l))
		}
		if _, ok := g["rate"]; !ok {
			return fmt.Errorf("no growth rate")
		}
		return nil
	})

	// --- Then: differential ------------------------------------------------

	sc.Step(`^pan and the engine agree on the verdict, the error class and the state count$`, func() error {
		if w.cmp == nil {
			return fmt.Errorf("no comparison was run")
		}
		if !w.cmp.Agree {
			return fmt.Errorf("disagreement:\n%s", w.cmp.Table(w.diffModel))
		}
		return nil
	})
	sc.Step(`^the comparison reports the verdict "([^"]*)" for pan and "([^"]*)" for the engine$`, func(pan, engine string) error {
		if w.cmp == nil {
			return fmt.Errorf("no comparison was run")
		}
		for _, r := range w.cmp.Rows {
			if r.Name != "verdict" {
				continue
			}
			if r.Pan != pan || r.Engine != engine {
				return fmt.Errorf("verdict: pan %q, engine %q, want pan %q, engine %q\n%s", r.Pan, r.Engine, pan, engine, w.cmp.Table(w.diffModel))
			}
			return nil
		}
		return fmt.Errorf("the comparison has no verdict row")
	})
	sc.Step(`^for the formula the engine says "([^"]*)" and pan says "([^"]*)"$`, func(engine, pan string) error {
		if w.triple == nil {
			return fmt.Errorf("no triple was run")
		}
		if w.triple.Engine != engine || w.triple.Pan != pan {
			return fmt.Errorf("engine %q, pan %q, want engine %q, pan %q: %s", w.triple.Engine, w.triple.Pan, engine, pan, w.triple.Row())
		}
		return nil
	})
	sc.Step(`^the three G5 verdicts agree$`, func() error {
		if w.triple == nil {
			return fmt.Errorf("no triple was run")
		}
		if !w.triple.Agree {
			return fmt.Errorf("triple disagrees: %s", w.triple.Row())
		}
		return nil
	})
}

// g5NoteKey maps the phrase a scenario uses to the substring the engine
// writes, so that the feature can say it in English and the test still
// checks the engine's own words.
func g5NoteKey(phrase string) string {
	switch phrase {
	case "a universal property that holds is justified by the complete graph":
		return "a universal property that holds is justified by the complete reachable graph"
	case "a failing existential property has no run to show":
		return "a failing existential property has no run to show"
	case "the nested subformula's failure is not a finite run":
		return "why the nested subformula fails there is not a finite run"
	}
	return ""
}

func g5Props(t *godog.Table) ([]map[string]any, error) {
	if len(t.Rows) < 2 {
		return nil, fmt.Errorf("the properties table needs a header and at least one row")
	}
	var head []string
	for _, c := range t.Rows[0].Cells {
		head = append(head, strings.TrimSpace(c.Value))
	}
	var out []map[string]any
	for _, r := range t.Rows[1:] {
		p := map[string]any{}
		for i, c := range r.Cells {
			v := strings.TrimSpace(c.Value)
			if v == "" {
				continue
			}
			if head[i] == "expr" && strings.HasPrefix(v, "{") {
				var e any
				if err := json.Unmarshal([]byte(v), &e); err != nil {
					return nil, fmt.Errorf("the expr cell %s is not JSON: %w", v, err)
				}
				p["expr"] = e
				continue
			}
			p[head[i]] = v
		}
		out = append(out, p)
	}
	return out, nil
}

func g5Int(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	}
	return 0
}

// g5FinalIs checks the value of a variable in the final state of a run.
func g5FinalIs(trace map[string]any, name string, want int) error {
	final, _ := trace["final_state"].([]any)
	for _, v := range final {
		m, _ := v.(map[string]any)
		if m != nil && m["var"] == name {
			if got := g5Int(m["value"]); got != want {
				return fmt.Errorf("%s is %d in the final state, want %d", name, got, want)
			}
			return nil
		}
	}
	return fmt.Errorf("the final state has no variable %q", name)
}

// g5Values reconstructs every value a variable takes along a run: the
// before/after pairs of the steps that changed it, plus its value in the
// final state.
func g5Values(trace map[string]any, name string) ([]int, error) {
	var out []int
	steps, _ := trace["steps"].([]any)
	for _, s := range steps {
		m, _ := s.(map[string]any)
		changes, _ := m["changes"].([]any)
		for _, c := range changes {
			cm, _ := c.(map[string]any)
			if cm != nil && cm["var"] == name {
				out = append(out, g5Int(cm["before"]), g5Int(cm["after"]))
			}
		}
	}
	final, _ := trace["final_state"].([]any)
	for _, v := range final {
		m, _ := v.(map[string]any)
		if m != nil && m["var"] == name {
			out = append(out, g5Int(m["value"]))
		}
	}
	return out, nil
}

// g5ParseIRNames re-parses the model and checks that the named variables
// are declared, as globals or as locals of some process (a typedef instance
// declared inside init becomes locals of init).
func g5ParseIRNames(model, list string) error {
	var buf, errs bytes.Buffer
	if code := cli.Run([]string{"parse", "--promela", model}, &buf, &errs); code != 0 {
		return fmt.Errorf("parse exited %d: %s", code, buf.String())
	}
	var m struct {
		Globals []struct {
			Name string `json:"name"`
			Len  int    `json:"len"`
		} `json:"globals"`
		Processes []struct {
			Locals []struct {
				Name string `json:"name"`
			} `json:"locals"`
		} `json:"processes"`
	}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		return err
	}
	have := map[string]bool{}
	var names []string
	for _, g := range m.Globals {
		have[g.Name] = true
		names = append(names, g.Name)
	}
	for _, p := range m.Processes {
		for _, l := range p.Locals {
			have[l.Name] = true
			names = append(names, l.Name)
		}
	}
	for _, want := range strings.Split(list, ", ") {
		if !have[strings.TrimSpace(want)] {
			return fmt.Errorf("the IR has no global %q; it has %v", want, names)
		}
	}
	return nil
}

func g5ParseIRInit(model, name string, want int) error {
	var buf, errs bytes.Buffer
	if code := cli.Run([]string{"parse", "--promela", model}, &buf, &errs); code != 0 {
		return fmt.Errorf("parse exited %d: %s", code, buf.String())
	}
	var m struct {
		Globals []struct {
			Name string  `json:"name"`
			Init []int64 `json:"init"`
		} `json:"globals"`
		Processes []struct {
			Locals []struct {
				Name string  `json:"name"`
				Init []int64 `json:"init"`
			} `json:"locals"`
		} `json:"processes"`
	}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		return err
	}
	check := func(n string, init []int64) error {
		if n != name {
			return nil
		}
		if len(init) == 0 || init[0] != int64(want) {
			return fmt.Errorf("%s starts at %v, want %d", name, init, want)
		}
		return errFound
	}
	for _, g := range m.Globals {
		if err := check(g.Name, g.Init); err != nil {
			if err == errFound {
				return nil
			}
			return err
		}
	}
	for _, p := range m.Processes {
		for _, l := range p.Locals {
			if err := check(l.Name, l.Init); err != nil {
				if err == errFound {
					return nil
				}
				return err
			}
		}
	}
	return fmt.Errorf("the IR has no variable %q", name)
}

// errFound is the sentinel g5ParseIRInit uses to stop at a match.
var errFound = fmt.Errorf("found")
