package modelcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"modelcheck/cli"
	"modelcheck/mcp"
)

// g2World is the per-scenario state for features/g2-mcp.feature. Every tool
// call goes through an MCP client connected to the server over the SDK's
// in-memory transport — the same code path as `mcd serve` minus stdio.
type g2World struct {
	cfg      mcp.Config
	srv      *mcp.Server
	cs       *sdk.ClientSession
	tools    []*sdk.Tool
	res      *sdk.CallToolResult
	out      map[string]any // structured content of the last call
	errText  string         // text of the last isError result
	session  string
	reports  []string // report paths of the "twice" scenario
	sims     []map[string]any
	writeErr error
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG2Steps)
}

func registerG2Steps(sc *godog.ScenarioContext) {
	w := &g2World{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = g2World{}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.cs != nil {
			w.cs.Close()
		}
		if w.srv != nil {
			w.srv.Close()
		}
		if w.cfg.SessionBase != "" {
			os.RemoveAll(w.cfg.SessionBase)
		}
		return ctx, nil
	})

	// ensure starts the server lazily so that Given-steps can still adjust
	// the configuration.
	ensure := func() error {
		if w.cs != nil {
			return nil
		}
		srv, err := mcp.New(w.cfg)
		if err != nil {
			return err
		}
		ct, st := sdk.NewInMemoryTransports()
		ctx := context.Background()
		if _, err := srv.Connect(ctx, st); err != nil {
			return err
		}
		cs, err := sdk.NewClient(&sdk.Implementation{Name: "g2-steps", Version: "0"}, nil).Connect(ctx, ct, nil)
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
		if args == nil {
			args = map[string]any{}
		}
		res, err := w.cs.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			return fmt.Errorf("protocol error calling %s: %w", tool, err)
		}
		w.res, w.out, w.errText = res, nil, ""
		if res.IsError {
			for _, c := range res.Content {
				if t, ok := c.(*sdk.TextContent); ok {
					w.errText += t.Text
				}
			}
			return nil
		}
		data, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &w.out); err != nil {
			return fmt.Errorf("%s: structured content is not an object: %v", tool, err)
		}
		if id, _ := w.out["session_id"].(string); id != "" {
			w.session = id
		}
		return nil
	}
	inSession := func(args map[string]any) map[string]any {
		if args == nil {
			args = map[string]any{}
		}
		args["session_id"] = w.session
		return args
	}
	loadJSON := func(path string) (map[string]any, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		return m, json.Unmarshal(data, &m)
	}
	sessionDir := func() string { return filepath.Join(w.cfg.SessionBase, w.session) }
	insideSession := func(p string) error {
		if p == "" {
			return fmt.Errorf("empty path")
		}
		rel, err := filepath.Rel(sessionDir(), p)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("%s is not inside the session directory %s", p, sessionDir())
		}
		if _, err := os.Stat(p); err != nil {
			return err
		}
		return nil
	}
	property := func(id string) (map[string]any, error) {
		props, _ := w.out["properties"].([]any)
		for _, p := range props {
			pm := p.(map[string]any)
			if pm["id"] == id {
				return pm, nil
			}
		}
		return nil, fmt.Errorf("no property %q in the answer: %v", id, w.out)
	}
	str := func(m map[string]any, key string) string {
		s, _ := m[key].(string)
		return s
	}
	num := func(m map[string]any, key string) int {
		f, _ := m[key].(float64)
		return int(f)
	}
	propsFromTable := func(t *godog.Table) ([]map[string]any, error) {
		if len(t.Rows) < 2 {
			return nil, fmt.Errorf("empty properties table")
		}
		var out []map[string]any
		for _, row := range t.Rows[1:] {
			p := map[string]any{}
			for i, h := range t.Rows[0].Cells {
				v := strings.TrimSpace(row.Cells[i].Value)
				if v == "" {
					continue
				}
				if h.Value == "expr" && strings.HasPrefix(v, "{") {
					var e any
					if err := json.Unmarshal([]byte(v), &e); err != nil {
						return nil, err
					}
					p["expr"] = e
					continue
				}
				p[h.Value] = v
			}
			out = append(out, p)
		}
		return out, nil
	}

	// --- Given ------------------------------------------------------------------
	sc.Step(`^an MCP server with a fresh session base directory and ceiling states (\d+), depth (\d+), ms (\d+), memory_mb (\d+)$`, func(states, depth, ms, mem int) error {
		base, err := os.MkdirTemp("", "g2-sessions-")
		if err != nil {
			return err
		}
		w.cfg = mcp.Config{SessionBase: base, Ceiling: mcp.Budget{States: states, Depth: depth, MS: int64(ms), MemoryMB: int64(mem)}}
		return nil
	})
	sc.Step(`^the server was started with --allow-read "([^"]*)"$`, func(dir string) error {
		if w.cs != nil {
			return fmt.Errorf("server already started")
		}
		w.cfg.AllowRead = append(w.cfg.AllowRead, dir)
		return nil
	})
	parsed := func(kind, path string) error {
		m, err := loadJSON(path)
		if err != nil {
			return err
		}
		if err := call("mc_parse", map[string]any{kind: m}); err != nil {
			return err
		}
		if w.res.IsError || w.out["outcome"] != "ir" {
			return fmt.Errorf("mc_parse of %s did not yield an IR: %s %v", path, w.errText, w.out)
		}
		return nil
	}
	sc.Step(`^a session in which the Petri net "([^"]*)" was parsed$`, func(p string) error { return parsed("petri", p) })
	sc.Step(`^a session in which the IR "([^"]*)" was parsed$`, func(p string) error { return parsed("ir", p) })
	sc.Step(`^a symlink "([^"]*)" inside the session directory pointing outside the session base directory$`, func(name string) error {
		outside, err := os.MkdirTemp("", "g2-outside-")
		if err != nil {
			return err
		}
		return os.Symlink(outside, filepath.Join(sessionDir(), name))
	})

	// --- When: tool list --------------------------------------------------------
	sc.Step(`^I list the tools$`, func() error {
		if err := ensure(); err != nil {
			return err
		}
		res, err := w.cs.ListTools(context.Background(), nil)
		if err != nil {
			return err
		}
		w.tools = res.Tools
		return nil
	})
	sc.Step(`^the tool names are exactly "([^"]*)"$`, func(list string) error {
		want := strings.Split(list, ", ")
		var got []string
		for _, t := range w.tools {
			got = append(got, t.Name)
		}
		sort.Strings(want)
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			return fmt.Errorf("tools %v, want %v", got, want)
		}
		return nil
	})
	sc.Step(`^every tool has an input schema of type object and an output schema of type object$`, func() error {
		for _, t := range w.tools {
			for _, s := range []any{t.InputSchema, t.OutputSchema} {
				m, ok := s.(map[string]any)
				if !ok || m["type"] != "object" {
					return fmt.Errorf("%s: schema %v is not an object", t.Name, s)
				}
			}
		}
		return nil
	})
	sc.Step(`^every tool has a non-empty description$`, func() error {
		for _, t := range w.tools {
			if strings.TrimSpace(t.Description) == "" {
				return fmt.Errorf("%s has no description", t.Name)
			}
		}
		return nil
	})

	// --- When: mc_parse ------------------------------------------------------------
	sc.Step(`^I call "mc_parse" with the Petri net "([^"]*)" inline$`, func(path string) error {
		m, err := loadJSON(path)
		if err != nil {
			return err
		}
		return call("mc_parse", map[string]any{"petri": m})
	})
	sc.Step(`^I call "mc_parse" with the Promela source "([^"]*)"$`, func(src string) error {
		return call("mc_parse", map[string]any{"promela": src})
	})
	sc.Step(`^I call "mc_parse" with the file path "([^"]*)" of kind "([^"]*)"$`, func(path, kind string) error {
		return call("mc_parse", map[string]any{"file": map[string]any{"kind": kind, "path": path}})
	})
	sc.Step(`^I call "(mc_[a-z_]+)" with no input$`, func(tool string) error { return call(tool, nil) })

	// --- When: mc_check ---------------------------------------------------------------
	sc.Step(`^I call "mc_check" in that session with the model's own properties$`, func() error {
		return call("mc_check", inSession(nil))
	})
	sc.Step(`^I call "mc_check" in that session with the model's own properties and budget states (\d+), depth (\d+), ms (\d+), memory_mb (\d+)$`, func(states, depth, ms, mem int) error {
		return call("mc_check", inSession(map[string]any{"budget": map[string]any{"states": states, "depth": depth, "ms": ms, "memory_mb": mem}}))
	})
	sc.Step(`^I call "mc_check" in that session with the model's own properties and only budget states (\d+)$`, func(states int) error {
		return call("mc_check", inSession(map[string]any{"budget": map[string]any{"states": states}}))
	})
	sc.Step(`^I call "mc_check" in that session with properties:$`, func(t *godog.Table) error {
		props, err := propsFromTable(t)
		if err != nil {
			return err
		}
		return call("mc_check", inSession(map[string]any{"properties": props}))
	})
	sc.Step(`^I call "mc_check" in that session with aggregate requested and properties:$`, func(t *godog.Table) error {
		props, err := propsFromTable(t)
		if err != nil {
			return err
		}
		return call("mc_check", inSession(map[string]any{"properties": props, "aggregate": true}))
	})
	sc.Step(`^I call "mc_check" in that session with the model's own properties without timings, twice$`, func() error {
		w.reports = nil
		for i := 0; i < 2; i++ {
			if err := call("mc_check", inSession(map[string]any{"no_timing": true})); err != nil {
				return err
			}
			if w.res.IsError {
				return fmt.Errorf("mc_check failed: %s", w.errText)
			}
			w.reports = append(w.reports, str(w.out, "report_path"))
		}
		return nil
	})

	// --- When: other tools -------------------------------------------------------------
	sc.Step(`^I call "mc_explain" for the counterexample of "([^"]*)"$`, func(id string) error {
		p, err := property(id)
		if err != nil {
			return err
		}
		c, _ := p["counterexample"].(map[string]any)
		if c == nil {
			return fmt.Errorf("property %q has no counterexample", id)
		}
		return call("mc_explain", inSession(map[string]any{"counterexample_id": c["id"]}))
	})
	sc.Step(`^I call "mc_explain" in that session for the counterexample id "([^"]*)"$`, func(id string) error {
		return call("mc_explain", inSession(map[string]any{"counterexample_id": id}))
	})
	sc.Step(`^I call "mc_simulate" in that session with seed (\d+), (\d+) steps and mode "([^"]*)", twice$`, func(seed, steps int, mode string) error {
		w.sims = nil
		for i := 0; i < 2; i++ {
			if err := call("mc_simulate", inSession(map[string]any{"seed": seed, "steps": steps, "mode": mode})); err != nil {
				return err
			}
			if w.res.IsError {
				return fmt.Errorf("mc_simulate failed: %s", w.errText)
			}
			w.sims = append(w.sims, w.out)
		}
		return nil
	})
	sc.Step(`^I call "mc_simulate" in that session guided by the edges "([^"]*)"$`, func(list string) error {
		return call("mc_simulate", inSession(map[string]any{"mode": "guided", "edges": strings.Split(list, ", ")}))
	})
	sc.Step(`^I call "mc_lint_property" in that session with kind "([^"]*)" and expr (.*)$`, func(kind, expr string) error {
		var e any
		if err := json.Unmarshal([]byte(expr), &e); err != nil {
			return err
		}
		return call("mc_lint_property", inSession(map[string]any{"kind": kind, "expr": e}))
	})
	sc.Step(`^I call "mc_estimate" in that session with a time limit of (\d+) ms$`, func(ms int) error {
		return call("mc_estimate", inSession(map[string]any{"ms": ms}))
	})
	sc.Step(`^I call "mc_manifest" in that session$`, func() error { return call("mc_manifest", inSession(nil)) })
	sc.Step(`^I call "mc_manifest" for the session id "([^"]*)"$`, func(id string) error {
		return call("mc_manifest", map[string]any{"session_id": id})
	})
	sc.Step(`^the session is asked to write "([^"]*)"$`, func(rel string) error {
		sess, err := w.srv.Sessions().Get(w.session)
		if err != nil {
			return err
		}
		_, w.writeErr = sess.WriteFile(rel, []byte("{}"))
		return nil
	})

	// --- Then: call outcome ------------------------------------------------------------
	sc.Step(`^the call is not an error$`, func() error {
		if w.res == nil {
			return fmt.Errorf("no call was made")
		}
		if w.res.IsError {
			return fmt.Errorf("call returned isError: %s", w.errText)
		}
		return nil
	})
	sc.Step(`^the call is an error whose message mentions "([^"]*)"$`, func(s string) error {
		if w.res == nil || !w.res.IsError {
			return fmt.Errorf("expected an isError result, got %v", w.out)
		}
		if w.res.StructuredContent != nil {
			return fmt.Errorf("an error result must not carry structured content: %v", w.res.StructuredContent)
		}
		if !strings.Contains(w.errText, s) {
			return fmt.Errorf("error %q does not mention %q", w.errText, s)
		}
		return nil
	})
	sc.Step(`^the tool error message also mentions "([^"]*)"$`, func(s string) error {
		if !strings.Contains(w.errText, s) {
			return fmt.Errorf("error %q does not mention %q", w.errText, s)
		}
		return nil
	})
	sc.Step(`^the answer has a session id$`, func() error {
		if str(w.out, "session_id") == "" {
			return fmt.Errorf("no session_id in %v", w.out)
		}
		return nil
	})
	sc.Step(`^the answer field "([^"]*)" is "([^"]*)"$`, func(key, want string) error {
		if got := fmt.Sprint(w.out[key]); got != want {
			return fmt.Errorf("%s = %q, want %q", key, got, want)
		}
		return nil
	})
	sc.Step(`^the answer field "([^"]*)" mentions "([^"]*)"$`, func(key, s string) error {
		if got := fmt.Sprint(w.out[key]); !strings.Contains(got, s) {
			return fmt.Errorf("%s = %q does not mention %q", key, got, s)
		}
		return nil
	})
	sc.Step(`^the answer has no field "([^"]*)"$`, func(key string) error {
		if _, ok := w.out[key]; ok {
			return fmt.Errorf("field %s present: %v", key, w.out[key])
		}
		return nil
	})
	sc.Step(`^the answer field "([^"]*)" names a file inside the session directory$`, func(key string) error {
		return insideSession(str(w.out, key))
	})

	// --- Then: mc_parse ---------------------------------------------------------------
	sc.Step(`^the answer IR declares (\d+) global variables and (\d+) process with (\d+) edges$`, func(globals, procs, edges int) error {
		irm, _ := w.out["ir"].(map[string]any)
		if irm == nil {
			return fmt.Errorf("no ir in the answer")
		}
		g, _ := irm["globals"].([]any)
		ps, _ := irm["processes"].([]any)
		if len(g) != globals || len(ps) != procs {
			return fmt.Errorf("got %d globals, %d processes", len(g), len(ps))
		}
		es, _ := ps[0].(map[string]any)["edges"].([]any)
		if len(es) != edges {
			return fmt.Errorf("got %d edges", len(es))
		}
		return nil
	})
	sc.Step(`^that IR file equals the output of "mcd ([^"]*)"$`, func(cmd string) error {
		var stdout, stderr bytes.Buffer
		if code := cli.Run(strings.Fields(cmd), &stdout, &stderr); code != 0 {
			return fmt.Errorf("mcd %s: exit %d: %s", cmd, code, stderr.String())
		}
		file, err := os.ReadFile(str(w.out, "ir_path"))
		if err != nil {
			return err
		}
		// The CLI records the file path as origin, the server "inline:petri";
		// compare with that one field neutralised.
		norm := func(b []byte) string {
			return strings.ReplaceAll(strings.ReplaceAll(string(b), "testdata/petri/petrinet1.json", "X"), "inline:petri", "X")
		}
		if norm(file) != norm(stdout.Bytes()) {
			return fmt.Errorf("IR file differs from mcd parse output")
		}
		return nil
	})
	sc.Step(`^the answer lists (\d+) origins with user names "([^"]*)" among them$`, func(n int, names string) error {
		origins, _ := w.out["origins"].([]any)
		have := map[string]bool{}
		for _, o := range origins {
			have[str(o.(map[string]any), "name")] = true
		}
		for _, want := range strings.Split(names, ", ") {
			if !have[want] {
				return fmt.Errorf("origin name %q missing among %d origins", want, len(origins))
			}
		}
		if len(have) < n {
			return fmt.Errorf("only %d distinct origin names", len(have))
		}
		return nil
	})
	sc.Step(`^the rejection names the construct "([^"]*)" and its reason mentions "([^"]*)"$`, func(construct, reason string) error {
		rej, _ := w.out["rejection"].(map[string]any)
		if rej == nil {
			return fmt.Errorf("no rejection in %v", w.out)
		}
		if !strings.Contains(str(rej, "construct"), construct) || !strings.Contains(str(rej, "reason"), reason) {
			return fmt.Errorf("rejection %v", rej)
		}
		return nil
	})
	sc.Step(`^the rejection reason mentions "([^"]*)"$`, func(s string) error {
		rej, _ := w.out["rejection"].(map[string]any)
		if rej == nil || !strings.Contains(str(rej, "reason"), s) {
			return fmt.Errorf("rejection %v does not mention %q", rej, s)
		}
		return nil
	})

	// --- Then: mc_check -----------------------------------------------------------------
	sc.Step(`^the answer property "([^"]*)" has status "([^"]*)" with evidence "([^"]*)"$`, func(id, status, evidence string) error {
		p, err := property(id)
		if err != nil {
			return err
		}
		if str(p, "status") != status || str(p, "evidence") != evidence {
			return fmt.Errorf("property %s: status %q evidence %q (reason %q)", id, str(p, "status"), str(p, "evidence"), str(p, "reason"))
		}
		return nil
	})
	sc.Step(`^the answer says the search is (complete|not complete)$`, func(which string) error {
		search, _ := w.out["search"].(map[string]any)
		complete, _ := search["complete"].(bool)
		if complete != (which == "complete") {
			return fmt.Errorf("search.complete = %v", complete)
		}
		return nil
	})
	sc.Step(`^the counterexample of "([^"]*)" has summary "([^"]*)" and (\d+) steps$`, func(id, summary string, steps int) error {
		p, err := property(id)
		if err != nil {
			return err
		}
		c, _ := p["counterexample"].(map[string]any)
		if c == nil || str(c, "summary") != summary || num(c, "steps") != steps {
			return fmt.Errorf("counterexample of %s: %v", id, c)
		}
		return nil
	})
	sc.Step(`^the counterexample of "([^"]*)" is a file inside the session directory$`, func(id string) error {
		p, err := property(id)
		if err != nil {
			return err
		}
		c, _ := p["counterexample"].(map[string]any)
		return insideSession(str(c, "path"))
	})
	sc.Step(`^that report file is a report whose property "([^"]*)" has status "([^"]*)"$`, func(id, status string) error {
		rep, err := loadJSON(str(w.out, "report_path"))
		if err != nil {
			return err
		}
		for _, p := range rep["properties"].([]any) {
			pm := p.(map[string]any)
			if pm["id"] == id {
				if pm["status"] != status {
					return fmt.Errorf("report property %s status %v", id, pm["status"])
				}
				return nil
			}
		}
		return fmt.Errorf("report has no property %s", id)
	})
	sc.Step(`^the reason of "([^"]*)" names the exhausted resource "([^"]*)"$`, func(id, resource string) error {
		p, err := property(id)
		if err != nil {
			return err
		}
		if !strings.Contains(str(p, "reason"), resource) {
			return fmt.Errorf("reason %q does not name %q", str(p, "reason"), resource)
		}
		return nil
	})
	sc.Step(`^the answer reason of "([^"]*)" mentions "([^"]*)"$`, func(id, s string) error {
		p, err := property(id)
		if err != nil {
			return err
		}
		if !strings.Contains(str(p, "reason"), s) {
			return fmt.Errorf("reason %q does not mention %q", str(p, "reason"), s)
		}
		return nil
	})
	sc.Step(`^the applied budget has (states|depth|ms|memory_mb) (\d+)$`, func(field string, want int) error {
		search, _ := w.out["search"].(map[string]any)
		applied, _ := search["budget_applied"].(map[string]any)
		if num(applied, field) != want {
			return fmt.Errorf("budget_applied.%s = %v, want %d", field, applied[field], want)
		}
		return nil
	})
	sc.Step(`^the budget notes mention "([^"]*)" clamped to (\d+)$`, func(field string, ceiling int) error {
		search, _ := w.out["search"].(map[string]any)
		notes, _ := search["budget_notes"].([]any)
		for _, n := range notes {
			s := n.(string)
			if strings.HasPrefix(s, field+":") && strings.Contains(s, "clamped to "+strconv.Itoa(ceiling)) {
				return nil
			}
		}
		return fmt.Errorf("no note for %s in %v", field, notes)
	})
	sc.Step(`^the budget notes are empty$`, func() error {
		search, _ := w.out["search"].(map[string]any)
		notes, _ := search["budget_notes"].([]any)
		if len(notes) != 0 {
			return fmt.Errorf("notes %v", notes)
		}
		return nil
	})
	sc.Step(`^the aggregate status is "([^"]*)"$`, func(want string) error {
		agg, _ := w.out["aggregate"].(map[string]any)
		if agg == nil || str(agg, "status") != want {
			return fmt.Errorf("aggregate %v", agg)
		}
		return nil
	})
	sc.Step(`^the two report files are byte-identical$`, func() error {
		if len(w.reports) != 2 {
			return fmt.Errorf("need two reports")
		}
		a, err := os.ReadFile(w.reports[0])
		if err != nil {
			return err
		}
		b, err := os.ReadFile(w.reports[1])
		if err != nil {
			return err
		}
		if !bytes.Equal(a, b) {
			return fmt.Errorf("reports differ:\n%s\n---\n%s", a, b)
		}
		return nil
	})
	sc.Step(`^the two report files differ from each other only in name$`, func() error {
		if w.reports[0] == w.reports[1] {
			return fmt.Errorf("the two calls wrote the same file")
		}
		return nil
	})

	// --- Then: mc_explain ---------------------------------------------------------------
	sc.Step(`^the explanation has (\d+) prefix steps and an empty loop$`, func(n int) error {
		prefix, _ := w.out["prefix"].([]any)
		loop, _ := w.out["loop"].([]any)
		if len(prefix) != n || len(loop) != 0 {
			return fmt.Errorf("prefix %d, loop %d", len(prefix), len(loop))
		}
		return nil
	})
	sc.Step(`^the explanation step (\d+) changes "([^"]*)" from (\d+) to (\d+) and "([^"]*)" from (\d+) to (\d+)$`, func(i int, v1 string, b1, a1 int, v2 string, b2, a2 int) error {
		prefix, _ := w.out["prefix"].([]any)
		if i < 1 || i > len(prefix) {
			return fmt.Errorf("no step %d", i)
		}
		changes, _ := prefix[i-1].(map[string]any)["changes"].([]any)
		want := map[string][2]int{v1: {b1, a1}, v2: {b2, a2}}
		for _, c := range changes {
			cm := c.(map[string]any)
			if w, ok := want[str(cm, "var")]; ok && num(cm, "before") == w[0] && num(cm, "after") == w[1] {
				delete(want, str(cm, "var"))
			}
		}
		if len(want) != 0 {
			return fmt.Errorf("changes %v lack %v", changes, want)
		}
		return nil
	})
	sc.Step(`^the explanation maps its steps to the user names "([^"]*)"$`, func(names string) error {
		got, _ := w.out["user_names"].([]any)
		var gs []string
		for _, g := range got {
			gs = append(gs, g.(string))
		}
		if strings.Join(gs, ", ") != names {
			return fmt.Errorf("user names %v", gs)
		}
		return nil
	})
	sc.Step(`^the explanation says that loop counterexamples arrive with G4$`, func() error {
		if !strings.Contains(str(w.out, "loop_note"), "G4") {
			return fmt.Errorf("loop_note %q", str(w.out, "loop_note"))
		}
		return nil
	})

	// --- Then: guard ----------------------------------------------------------------------
	sc.Step(`^the write is refused with a message that mentions "([^"]*)"$`, func(s string) error {
		if w.writeErr == nil {
			return fmt.Errorf("the write was not refused")
		}
		if !strings.Contains(w.writeErr.Error(), s) {
			return fmt.Errorf("refusal %q does not mention %q", w.writeErr, s)
		}
		return nil
	})
	sc.Step(`^no file named "([^"]*)" exists anywhere under the session base directory$`, func(name string) error {
		var found []string
		filepath.Walk(w.cfg.SessionBase, func(p string, info os.FileInfo, err error) error {
			if err == nil && info.Name() == name {
				found = append(found, p)
			}
			return nil
		})
		// Also the parent of the base, where "../x" would land.
		if _, err := os.Stat(filepath.Join(filepath.Dir(w.cfg.SessionBase), name)); err == nil {
			found = append(found, filepath.Join(filepath.Dir(w.cfg.SessionBase), name))
		}
		if len(found) > 0 {
			return fmt.Errorf("found %v", found)
		}
		return nil
	})

	// --- Then: mc_simulate ----------------------------------------------------------------
	sc.Step(`^the two simulation answers are identical except for the trace path$`, func() error {
		if len(w.sims) != 2 {
			return fmt.Errorf("need two simulations")
		}
		var norm [2]string
		for i, s := range w.sims {
			c := map[string]any{}
			for k, v := range s {
				if k != "trace_path" {
					c[k] = v
				}
			}
			b, _ := json.Marshal(c)
			norm[i] = string(b)
		}
		if norm[0] != norm[1] {
			return fmt.Errorf("simulations differ:\n%s\n%s", norm[0], norm[1])
		}
		return nil
	})
	sc.Step(`^the simulation stopped because of one of "([^"]*)"$`, func(list string) error {
		for _, want := range strings.Split(list, ", ") {
			if str(w.out, "stopped") == want {
				return nil
			}
		}
		return fmt.Errorf("stopped = %q (%s), want one of %q", str(w.out, "stopped"), str(w.out, "stop_reason"), list)
	})
	sc.Step(`^the simulation trace file is inside the session directory$`, func() error {
		return insideSession(str(w.out, "trace_path"))
	})
	sc.Step(`^the simulation took (\d+) steps with summary "([^"]*)"$`, func(n int, summary string) error {
		if num(w.out, "steps_taken") != n || str(w.out, "summary") != summary {
			return fmt.Errorf("steps %v summary %q", w.out["steps_taken"], str(w.out, "summary"))
		}
		return nil
	})
	sc.Step(`^the simulation names the enabled edges at the stop as "([^"]*)"$`, func(list string) error {
		got, _ := w.out["enabled_at_stop"].([]any)
		var gs []string
		for _, g := range got {
			gs = append(gs, g.(string))
		}
		if strings.Join(gs, ", ") != list {
			return fmt.Errorf("enabled_at_stop %v, want %q", gs, list)
		}
		return nil
	})

	// --- Then: mc_lint_property --------------------------------------------------------------
	strList := func(key string) string {
		got, _ := w.out[key].([]any)
		var gs []string
		for _, g := range got {
			gs = append(gs, fmt.Sprint(g))
		}
		return strings.Join(gs, ", ")
	}
	sc.Step(`^the lint lists the atoms "([^"]*)"$`, func(list string) error {
		if strList("atoms") != list {
			return fmt.Errorf("atoms %q", strList("atoms"))
		}
		return nil
	})
	sc.Step(`^the lint lists the undefined atoms "([^"]*)"$`, func(list string) error {
		if strList("undefined") != list {
			return fmt.Errorf("undefined %q", strList("undefined"))
		}
		return nil
	})
	sc.Step(`^the lint class is "([^"]*)"$`, func(class string) error {
		if str(w.out, "class") != class {
			return fmt.Errorf("class %q", str(w.out, "class"))
		}
		return nil
	})
	sc.Step(`^the lint says the expression is X-free and not temporal$`, func() error {
		if w.out["x_free"] != true || w.out["temporal"] != false {
			return fmt.Errorf("x_free %v temporal %v", w.out["x_free"], w.out["temporal"])
		}
		return nil
	})
	sc.Step(`^the lint notes mention "([^"]*)"$`, func(s string) error {
		if !strings.Contains(strList("notes"), s) {
			return fmt.Errorf("notes %q", strList("notes"))
		}
		return nil
	})

	// --- Then: mc_estimate ---------------------------------------------------------------------
	sc.Step(`^the estimate reports at least (\d+) states visited and a positive states-per-second rate$`, func(n int) error {
		sps, _ := w.out["states_per_second"].(float64)
		if num(w.out, "states_visited") < n || sps <= 0 {
			return fmt.Errorf("states_visited %v, states_per_second %v", w.out["states_visited"], sps)
		}
		return nil
	})
	sc.Step(`^the estimate has a growth table with at least (\d+) levels and a growth rate$`, func(n int) error {
		growth, _ := w.out["growth"].(map[string]any)
		levels, _ := growth["per_level"].([]any)
		rate, _ := growth["rate"].(float64)
		if len(levels) < n || rate <= 0 {
			return fmt.Errorf("growth %v", growth)
		}
		return nil
	})
	sc.Step(`^the estimate projection carries evidence "([^"]*)"$`, func(ev string) error {
		proj, _ := w.out["projection"].(map[string]any)
		if str(proj, "evidence") != ev {
			return fmt.Errorf("projection %v", proj)
		}
		return nil
	})
	sc.Step(`^the estimate is not a verification result and says so$`, func() error {
		if _, has := w.out["properties"]; has {
			return fmt.Errorf("estimate carries properties")
		}
		if !strings.Contains(str(w.out, "note"), "not a verification result") {
			return fmt.Errorf("note %q", str(w.out, "note"))
		}
		return nil
	})

	// --- Then: mc_manifest ------------------------------------------------------------------------
	manifest := func() map[string]any {
		m, _ := w.out["manifest"].(map[string]any)
		return m
	}
	sc.Step(`^the manifest names the engine "([^"]*)" with a version and the schemas "([^"]*)" and "([^"]*)"$`, func(name, irs, reps string) error {
		eng, _ := manifest()["engine"].(map[string]any)
		if str(eng, "name") != name || str(eng, "version") == "" || str(eng, "ir_schema") != irs || str(eng, "report_schema") != reps {
			return fmt.Errorf("engine %v", eng)
		}
		return nil
	})
	sc.Step(`^the manifest lists (\d+) input with a sha256 hash$`, func(n int) error {
		inputs, _ := manifest()["inputs"].([]any)
		if len(inputs) != n {
			return fmt.Errorf("%d inputs", len(inputs))
		}
		for _, in := range inputs {
			if len(str(in.(map[string]any), "sha256")) != 64 {
				return fmt.Errorf("input without sha256: %v", in)
			}
		}
		return nil
	})
	sc.Step(`^the manifest lists the calls "([^"]*)" in this order$`, func(list string) error {
		calls, _ := manifest()["calls"].([]any)
		var tools []string
		for _, c := range calls {
			tools = append(tools, str(c.(map[string]any), "tool"))
		}
		if strings.Join(tools, ", ") != list {
			return fmt.Errorf("calls %v", tools)
		}
		return nil
	})
	sc.Step(`^every manifest call has a duration and an outcome of "ok" or "error"$`, func() error {
		calls, _ := manifest()["calls"].([]any)
		for _, c := range calls {
			cm := c.(map[string]any)
			if _, ok := cm["duration_ms"]; !ok {
				return fmt.Errorf("call without duration: %v", cm)
			}
			if o := str(cm, "outcome"); o != "ok" && o != "error" {
				return fmt.Errorf("call outcome %q", o)
			}
		}
		return nil
	})
	sc.Step(`^the manifest records the applied budget of the check$`, func() error {
		calls, _ := manifest()["calls"].([]any)
		for _, c := range calls {
			cm := c.(map[string]any)
			if str(cm, "tool") == "mc_check" {
				params, _ := cm["params"].(map[string]any)
				if b, _ := params["budget_applied"].(map[string]any); b == nil || num(b, "states") <= 0 {
					return fmt.Errorf("mc_check params %v", params)
				}
				return nil
			}
		}
		return fmt.Errorf("no mc_check call in the manifest")
	})
	sc.Step(`^the manifest is a file inside the session directory$`, func() error {
		return insideSession(str(w.out, "path"))
	})
}
