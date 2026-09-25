package modelcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/cli"
	"modelcheck/mcp"
	"modelcheck/tools/pandiff"
)

// g4World is the per-scenario state for features/g4-ltl.feature. CLI steps
// go through cli.Run in-process; MCP steps through an in-memory client of
// mcp.Server configured as `mcd serve` configures it (Promela frontend
// linked).
type g4World struct {
	model  string
	stdout bytes.Buffer
	stderr bytes.Buffer
	exit   int
	first  []byte // output of the first of two runs
	tmp    string

	// MCP
	srv    *mcp.Server
	cs     *sdk.ClientSession
	base   string
	sess   string
	out    map[string]any
	errTxt string

	// differential
	diffModel string
	triple    *pandiff.Triple
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG4Steps)
}

const testdataPromela = "testdata/promela"

func registerG4Steps(sc *godog.ScenarioContext) {
	w := &g4World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g4World{}
		if d, err := os.MkdirTemp("", "g4-"); err == nil {
			w.tmp = d
		}
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
	sc.Step(`^the corpus model "([^"]*)"$`, func(name string) error {
		p := filepath.Join(corpusDir, name)
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.model = p
		return nil
	})
	sc.Step(`^the test model "([^"]*)"$`, func(name string) error {
		p := filepath.Join(testdataPromela, name)
		if _, err := os.Stat(p); err != nil {
			return err
		}
		w.model = p
		return nil
	})
	sc.Step(`^the test model "([^"]*)" with the corpus claim "([^"]*)" appended$`, func(name, claim string) error {
		a, err := os.ReadFile(filepath.Join(testdataPromela, name))
		if err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(corpusDir, claim))
		if err != nil {
			return err
		}
		p := filepath.Join(w.tmp, name)
		if err := os.WriteFile(p, append(append(a, '\n'), b...), 0o644); err != nil {
			return err
		}
		w.model = p
		return nil
	})
	sc.Step(`^spin and gcc are installed$`, func() error {
		if !(pandiff.Tools{}).Available() {
			return godog.ErrSkip
		}
		return nil
	})
	sc.Step(`^the model "([^"]*)" for the differential check$`, func(name string) error {
		p := filepath.Join(corpusDir, name)
		if strings.HasPrefix(name, "testdata:") {
			p = filepath.Join(testdataPromela, strings.TrimPrefix(name, "testdata:"))
		}
		if _, err := os.Stat(p); err != nil {
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
		base, err := os.MkdirTemp("", "g4-mcp-")
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
		cs, err := sdk.NewClient(&sdk.Implementation{Name: "g4-steps", Version: "0"}, nil).Connect(ctx, ct, nil)
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
	parseInto := func(path string) error {
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := call("mc_parse", map[string]any{"promela": string(src)}); err != nil {
			return err
		}
		if w.out["outcome"] != "ir" {
			return fmt.Errorf("mc_parse outcome %v: %v", w.out["outcome"], w.out)
		}
		return nil
	}
	sc.Step(`^an MCP session with the test model "([^"]*)" parsed$`, func(name string) error {
		return parseInto(filepath.Join(testdataPromela, name))
	})
	sc.Step(`^an MCP session with the corpus model "([^"]*)" parsed$`, func(name string) error {
		return parseInto(filepath.Join(corpusDir, name))
	})
	sc.Step(`^an MCP server with the Promela frontend linked as mcd serve links it$`, func() error {
		return ensure()
	})
	sc.Step(`^I call mc_parse with the corpus Promela source "([^"]*)"$`, func(name string) error {
		src, err := os.ReadFile(filepath.Join(corpusDir, name))
		if err != nil {
			return err
		}
		return call("mc_parse", map[string]any{"promela": string(src)})
	})
	mcCheck := func(props *godog.DocString, extra map[string]any) error {
		var list []any
		if err := json.Unmarshal([]byte(props.Content), &list); err != nil {
			return fmt.Errorf("properties docstring: %v", err)
		}
		args := map[string]any{"session_id": w.sess, "properties": list, "no_timing": true}
		for k, v := range extra {
			args[k] = v
		}
		return call("mc_check", args)
	}
	sc.Step(`^I call mc_check with properties:$`, func(props *godog.DocString) error {
		return mcCheck(props, nil)
	})
	sc.Step(`^I call mc_check with fairness "([^"]*)" and properties:$`, func(fair string, props *godog.DocString) error {
		return mcCheck(props, map[string]any{"fairness": fair})
	})
	sc.Step(`^I call mc_check with budget states (\d+) and properties:$`, func(states int, props *godog.DocString) error {
		return mcCheck(props, map[string]any{"budget": map[string]any{"states": states}})
	})
	sc.Step(`^I call mc_explain for the counterexample of "([^"]*)"$`, func(id string) error {
		p, err := w.mcpProperty(id)
		if err != nil {
			return err
		}
		ce, ok := p["counterexample"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s has no counterexample", id)
		}
		return call("mc_explain", map[string]any{"session_id": w.sess, "counterexample_id": ce["id"]})
	})

	// --- When: CLI ---------------------------------------------------------------
	run := func(cmd string) error {
		args, err := w.args(cmd)
		if err != nil {
			return err
		}
		w.stdout.Reset()
		w.stderr.Reset()
		w.exit = cli.Run(args, &w.stdout, &w.stderr)
		return nil
	}
	sc.Step(`^I invoke "([^"]*)"$`, run)
	sc.Step(`^I invoke "([^"]*)" twice$`, func(cmd string) error {
		if err := run(cmd); err != nil {
			return err
		}
		w.first = append([]byte(nil), w.stdout.Bytes()...)
		return run(cmd)
	})
	sc.Step(`^I compare the engine with pan for formula "([^"]*)" with defines "([^"]*)", fairness "([^"]*)" and mode "([^"]*)"$`, func(formula, defs, fairness, mode string) error {
		var defines []string
		for _, d := range strings.Split(defs, ";") {
			if d = strings.TrimSpace(d); d != "" {
				defines = append(defines, d)
			}
		}
		t, err := pandiff.RunTriple(context.Background(), pandiff.Tools{}, w.diffModel, defines, formula, fairness, mode)
		if err != nil {
			return err
		}
		w.triple = t
		return nil
	})

	// --- Then: CLI --------------------------------------------------------------
	sc.Step(`^it exits with (\d+)$`, func(code int) error {
		if w.exit != code {
			return fmt.Errorf("exit %d, want %d\nstdout: %s\nstderr: %s", w.exit, code, w.stdout.String(), w.stderr.String())
		}
		return nil
	})
	sc.Step(`^the property "([^"]*)" is "([^"]*)" with evidence "([^"]*)"$`, func(id, status, evidence string) error {
		p, err := w.property(id)
		if err != nil {
			return err
		}
		if p["status"] != status || p["evidence"] != evidence {
			return fmt.Errorf("%s: status=%v evidence=%v reason=%v", id, p["status"], p["evidence"], p["reason"])
		}
		return nil
	})
	sc.Step(`^there is no property "([^"]*)"$`, func(id string) error {
		if _, err := w.property(id); err == nil {
			return fmt.Errorf("property %q is present", id)
		}
		return nil
	})
	sc.Step(`^the reason of property "([^"]*)" mentions "([^"]*)"$`, func(id, text string) error {
		p, err := w.property(id)
		if err != nil {
			return err
		}
		if !strings.Contains(fmt.Sprint(p["reason"]), text) {
			return fmt.Errorf("reason of %s is %q", id, p["reason"])
		}
		return nil
	})
	sc.Step(`^the reason for "([^"]*)" names "([^"]*)" as enabled throughout the loop and never moving$`, func(id, proc string) error {
		p, err := w.property(id)
		if err != nil {
			return err
		}
		want := proc + " is enabled throughout the loop and never moves"
		if !strings.Contains(fmt.Sprint(p["reason"]), want) {
			return fmt.Errorf("reason of %s is %q", id, p["reason"])
		}
		return nil
	})
	sc.Step(`^the counterexample of "([^"]*)" has a loop$`, func(id string) error {
		_, _, err := w.loop(id)
		return err
	})
	sc.Step(`^the counterexample of "([^"]*)" has no loop$`, func(id string) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		if _, ok := ce["loop"]; ok {
			return fmt.Errorf("%s has a loop: %v", id, ce["loop"])
		}
		return nil
	})
	sc.Step(`^every step of the loop of "([^"]*)" is by process "([^"]*)" or by the claim$`, func(id, proc string) error {
		_, loop, err := w.loop(id)
		if err != nil {
			return err
		}
		for _, st := range loop {
			pr := fmt.Sprint(st["process"])
			if pr != proc && !strings.HasPrefix(pr, "never") {
				return fmt.Errorf("loop step %v by %s", st["index"], pr)
			}
		}
		return nil
	})
	sc.Step(`^the loop of "([^"]*)" contains a step by process "([^"]*)"$`, func(id, proc string) error {
		_, loop, err := w.loop(id)
		if err != nil {
			return err
		}
		for _, st := range loop {
			if fmt.Sprint(st["process"]) == proc {
				return nil
			}
		}
		return fmt.Errorf("no loop step of %s is by %s", id, proc)
	})
	sc.Step(`^the prefix of "([^"]*)" contains a step "([^"]*)"$`, func(id, cmd string) error {
		prefix, _, err := w.loop(id)
		if err != nil {
			return err
		}
		for _, st := range prefix {
			if fmt.Sprint(st["command"]) == cmd {
				return nil
			}
		}
		return fmt.Errorf("no prefix step of %s is %q", id, cmd)
	})
	sc.Step(`^the loop of "([^"]*)" is closed: the state after the last step equals the state before the loop$`, func(id string) error {
		_, loop, err := w.loop(id)
		if err != nil {
			return err
		}
		// Every variable changed inside the loop must end where it started:
		// its first "before" equals its last "after".
		first := map[string]any{}
		last := map[string]any{}
		for _, st := range loop {
			for _, c := range asAny(st["changes"]) {
				cm := c.(map[string]any)
				v := fmt.Sprint(cm["var"])
				if _, ok := first[v]; !ok {
					first[v] = cm["before"]
				}
				last[v] = cm["after"]
			}
		}
		for v := range first {
			if fmt.Sprint(first[v]) != fmt.Sprint(last[v]) {
				return fmt.Errorf("%s: %v before the loop, %v after it", v, first[v], last[v])
			}
		}
		return nil
	})
	sc.Step(`^the final state of "([^"]*)" has "([^"]*)" equal to (-?\d+)$`, func(id, v string, val int) error {
		ce, err := w.trace(id)
		if err != nil {
			return err
		}
		for _, f := range asAny(ce["final_state"]) {
			fm := f.(map[string]any)
			if fm["var"] == v {
				if int(fm["value"].(float64)) != val {
					return fmt.Errorf("%s = %v", v, fm["value"])
				}
				return nil
			}
		}
		return fmt.Errorf("no variable %q in the final state", v)
	})
	sc.Step(`^the search for "([^"]*)" is complete$`, func(id string) error {
		p, err := w.property(id)
		if err != nil {
			return err
		}
		if p["complete"] != true {
			return fmt.Errorf("%s: complete=%v (%v)", id, p["complete"], p["reason"])
		}
		return nil
	})
	sc.Step(`^the search for "([^"]*)" is not complete$`, func(id string) error {
		p, err := w.property(id)
		if err != nil {
			return err
		}
		if p["complete"] != false {
			return fmt.Errorf("%s: complete=%v", id, p["complete"])
		}
		return nil
	})
	sc.Step(`^the state count of "([^"]*)" is (\d+)$`, func(id string, n int) error {
		p, err := w.property(id)
		if err != nil {
			return err
		}
		got := int(p["counters"].(map[string]any)["states"].(float64))
		if got != n {
			return fmt.Errorf("%s: %d states, want %d", id, got, n)
		}
		return nil
	})
	sc.Step(`^the property "([^"]*)" has formula "([^"]*)" and negation "([^"]*)"$`, func(id, f, neg string) error {
		tp, err := w.temporal(id)
		if err != nil {
			return err
		}
		if tp["formula"] != f || tp["negated"] != neg {
			return fmt.Errorf("%s: formula=%v negated=%v", id, tp["formula"], tp["negated"])
		}
		return nil
	})
	sc.Step(`^the atoms of "([^"]*)" are "([^"]*)"$`, func(id, atoms string) error {
		tp, err := w.temporal(id)
		if err != nil {
			return err
		}
		if got := strings.Join(asList(tp["atoms"]), ", "); got != atoms {
			return fmt.Errorf("atoms of %s: %q", id, got)
		}
		return nil
	})
	sc.Step(`^the property "([^"]*)" is stutter-invariant$`, func(id string) error {
		tp, err := w.temporal(id)
		if err != nil {
			return err
		}
		if tp["stutter_invariant"] != true {
			return fmt.Errorf("%s: stutter_invariant=%v", id, tp["stutter_invariant"])
		}
		return nil
	})
	sc.Step(`^the property "([^"]*)" is not stutter-invariant$`, func(id string) error {
		tp, err := w.temporal(id)
		if err != nil {
			return err
		}
		if tp["stutter_invariant"] != false {
			return fmt.Errorf("%s: stutter_invariant=%v", id, tp["stutter_invariant"])
		}
		return nil
	})
	sc.Step(`^the property "([^"]*)" records fairness "([^"]*)"$`, func(id, fair string) error {
		tp, err := w.temporal(id)
		if err != nil {
			return err
		}
		if tp["fairness"] != fair {
			return fmt.Errorf("%s: fairness=%v", id, tp["fairness"])
		}
		return nil
	})
	sc.Step(`^the input is rejected with kind "([^"]*)" and status "([^"]*)"$`, func(kind, status string) error {
		e, err := w.errorBody()
		if err != nil {
			return err
		}
		if e["kind"] != kind || e["status"] != status {
			return fmt.Errorf("rejection %v", e)
		}
		return nil
	})
	sc.Step(`^the rejection message mentions "([^"]*)"$`, func(text string) error {
		e, err := w.errorBody()
		if err != nil {
			return err
		}
		if !strings.Contains(fmt.Sprint(e["message"]), text) {
			return fmt.Errorf("rejection message %q", e["message"])
		}
		return nil
	})
	sc.Step(`^the report budget has states (\d+) and time_ms (\d+)$`, func(states, ms int) error {
		r, err := w.report()
		if err != nil {
			return err
		}
		b := r["search"].(map[string]any)["budget"].(map[string]any)
		if int(b["states"].(float64)) != states || int(b["time_ms"].(float64)) != ms {
			return fmt.Errorf("budget %v", b)
		}
		return nil
	})
	sc.Step(`^the two reports are byte-identical$`, func() error {
		if !bytes.Equal(w.first, w.stdout.Bytes()) {
			return fmt.Errorf("the two outputs differ")
		}
		if len(w.first) == 0 {
			return fmt.Errorf("empty output")
		}
		return nil
	})

	// --- Then: MCP ---------------------------------------------------------------
	sc.Step(`^the mc_parse outcome is "([^"]*)" with (\d+) processes$`, func(outcome string, n int) error {
		if w.out["outcome"] != outcome {
			return fmt.Errorf("outcome %v (%v)", w.out["outcome"], w.out["reason"])
		}
		irDoc, _ := w.out["ir"].(map[string]any)
		if got := len(asAny(irDoc["processes"])); got != n {
			return fmt.Errorf("%d processes, want %d", got, n)
		}
		return nil
	})
	sc.Step(`^the parsed model has the properties "([^"]*)"$`, func(list string) error {
		irDoc, _ := w.out["ir"].(map[string]any)
		var ids []string
		for _, p := range asAny(irDoc["properties"]) {
			ids = append(ids, fmt.Sprint(p.(map[string]any)["id"]))
		}
		if got := strings.Join(ids, ", "); got != list {
			return fmt.Errorf("properties %q, want %q", got, list)
		}
		return nil
	})
	sc.Step(`^the MCP property "([^"]*)" is "([^"]*)" with evidence "([^"]*)"$`, func(id, status, evidence string) error {
		p, err := w.mcpProperty(id)
		if err != nil {
			return err
		}
		if p["status"] != status || p["evidence"] != evidence {
			return fmt.Errorf("%s: status=%v evidence=%v reason=%v", id, p["status"], p["evidence"], p["reason"])
		}
		return nil
	})
	sc.Step(`^the MCP reason for "([^"]*)" mentions "([^"]*)"$`, func(id, text string) error {
		p, err := w.mcpProperty(id)
		if err != nil {
			return err
		}
		if !strings.Contains(fmt.Sprint(p["reason"]), text) {
			return fmt.Errorf("reason of %s is %q", id, p["reason"])
		}
		return nil
	})
	sc.Step(`^the MCP property "([^"]*)" has a counterexample with a loop$`, func(id string) error {
		p, err := w.mcpProperty(id)
		if err != nil {
			return err
		}
		ce, ok := p["counterexample"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s has no counterexample", id)
		}
		loop, ok := ce["loop"].(map[string]any)
		if !ok || loop["steps"].(float64) < 1 {
			return fmt.Errorf("%s: counterexample without loop: %v", id, ce)
		}
		return nil
	})
	sc.Step(`^the explanation has a non-empty loop and a prefix$`, func() error {
		loop := asAny(w.out["loop"])
		prefix := asAny(w.out["prefix"])
		if len(loop) == 0 {
			return fmt.Errorf("empty loop: %v", w.out["loop_note"])
		}
		if prefix == nil {
			return fmt.Errorf("no prefix field")
		}
		return nil
	})
	sc.Step(`^every loop step of the explanation is by process "([^"]*)" or by the claim$`, func(proc string) error {
		for _, st := range asAny(w.out["loop"]) {
			pr := fmt.Sprint(st.(map[string]any)["process"])
			if pr != proc && !strings.HasPrefix(pr, "never") {
				return fmt.Errorf("loop step by %s", pr)
			}
		}
		return nil
	})
	sc.Step(`^the MCP applied budget has states (\d+)$`, func(n int) error {
		s, _ := w.out["search"].(map[string]any)
		b, _ := s["budget_applied"].(map[string]any)
		if int(b["states"].(float64)) != n {
			return fmt.Errorf("applied budget %v", b)
		}
		return nil
	})

	// --- Then: differential ------------------------------------------------------
	sc.Step(`^the three verdicts agree$`, func() error {
		if w.triple == nil {
			return fmt.Errorf("no comparison was run")
		}
		if !w.triple.Agree {
			return fmt.Errorf("%s", w.triple.Row())
		}
		return nil
	})
}

// args splits a command line like a shell would for single-quoted words,
// substituting <model>.
func (w *g4World) args(cmd string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQuote, inWord := false, false
	for _, r := range cmd {
		switch {
		case r == '\'':
			inQuote = !inQuote
			inWord = true
		case r == ' ' && !inQuote:
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unbalanced quote in %q", cmd)
	}
	if inWord {
		out = append(out, cur.String())
	}
	for i, tok := range out {
		if tok == "<model>" {
			out[i] = w.model
		}
	}
	if len(out) > 0 && out[0] == "mcd" {
		out = out[1:] // cli.Run takes the arguments without the program name
	}
	return out, nil
}

func (w *g4World) report() (map[string]any, error) {
	var r map[string]any
	if err := json.Unmarshal(w.stdout.Bytes(), &r); err != nil {
		return nil, fmt.Errorf("stdout is not a JSON report (exit %d): %w\n%s%s", w.exit, err, w.stdout.String(), w.stderr.String())
	}
	if _, ok := r["properties"]; !ok {
		return nil, fmt.Errorf("stdout is not a report: %s", w.stdout.String())
	}
	return r, nil
}

func (w *g4World) property(id string) (map[string]any, error) {
	r, err := w.report()
	if err != nil {
		return nil, err
	}
	for _, p := range asAny(r["properties"]) {
		pm := p.(map[string]any)
		if pm["id"] == id {
			return pm, nil
		}
	}
	return nil, fmt.Errorf("no property %q in the report", id)
}

func (w *g4World) temporal(id string) (map[string]any, error) {
	p, err := w.property(id)
	if err != nil {
		return nil, err
	}
	tp, ok := p["temporal"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s has no temporal section", id)
	}
	return tp, nil
}

func (w *g4World) trace(id string) (map[string]any, error) {
	p, err := w.property(id)
	if err != nil {
		return nil, err
	}
	ce, ok := p["counterexample"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s has no counterexample (status %v)", id, p["status"])
	}
	return ce, nil
}

// loop splits the counterexample's steps at loop.start.
func (w *g4World) loop(id string) (prefix, loop []map[string]any, err error) {
	ce, err := w.trace(id)
	if err != nil {
		return nil, nil, err
	}
	lp, ok := ce["loop"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("%s: the counterexample has no loop (summary %q)", id, ce["summary"])
	}
	start := int(lp["start"].(float64))
	steps := asAny(ce["steps"])
	if start < 1 || start > len(steps) || int(lp["steps"].(float64)) != len(steps)-start+1 {
		return nil, nil, fmt.Errorf("%s: loop %v inconsistent with %d steps", id, lp, len(steps))
	}
	for i, st := range steps {
		sm := st.(map[string]any)
		if i+1 >= start {
			loop = append(loop, sm)
		} else {
			prefix = append(prefix, sm)
		}
	}
	return prefix, loop, nil
}

func (w *g4World) errorBody() (map[string]any, error) {
	var r map[string]any
	if err := json.Unmarshal(w.stdout.Bytes(), &r); err != nil {
		return nil, fmt.Errorf("stdout is not JSON: %s", w.stdout.String())
	}
	e, ok := r["error"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("no error object in %s", w.stdout.String())
	}
	return e, nil
}

func (w *g4World) mcpProperty(id string) (map[string]any, error) {
	if w.out == nil {
		return nil, fmt.Errorf("no MCP answer (last error: %s)", w.errTxt)
	}
	for _, p := range asAny(w.out["properties"]) {
		pm := p.(map[string]any)
		if pm["id"] == id {
			return pm, nil
		}
	}
	return nil, fmt.Errorf("no property %q in the answer (outcome %v, rejection %v)", id, w.out["outcome"], w.out["rejection"])
}
