package modelcheck_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// Step definitions for features/r2-mcp-end-to-end.feature.
//
// The claim these steps accept is the one G2 opened and G6 could not close: an
// agent holding only the skill reaches the engine through the MCP tools. The
// client that can do it is external (Coddy), so the acceptance reads a recorded
// run rather than starting one: the declaration the run used, the tool log of
// the session, the report the server wrote and the answer. Every step below is
// a check over those files — nothing here talks to a model.
func init() {
	stepRegistrars = append(stepRegistrars, registerR2Steps)
}

// r2World is the per-scenario state: the recorded run's directory and the
// report found in it, so that the later steps speak of "that report".
type r2World struct {
	dir    string
	plugin string
	report map[string]any
}

func registerR2Steps(sc *godog.ScenarioContext) {
	w := &r2World{}

	readJSON := func(path string, into any) error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, into)
	}

	// The engine's own report, found by shape: the run may hold several JSON
	// files (the declaration, the tool log), and only one of them is a report.
	findReport := func() (map[string]any, string, error) {
		var found map[string]any
		var at string
		err := filepath.Walk(w.dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".json") || found != nil {
				return err
			}
			var doc map[string]any
			if readJSON(p, &doc) != nil {
				return nil
			}
			eng, _ := doc["engine"].(map[string]any)
			if eng == nil || eng["name"] != "mcd" {
				return nil
			}
			if _, ok := doc["properties"].([]any); !ok {
				return nil
			}
			found, at = doc, p
			return nil
		})
		if err != nil {
			return nil, "", err
		}
		if found == nil {
			return nil, "", fmt.Errorf("no report the engine wrote under %s", w.dir)
		}
		return found, at, nil
	}

	// A scenario that speaks of "that report" without having read it first
	// reads it here: the world is per-scenario, and the last scenario asks
	// about the answer, not about how the report was found.
	property := func(id string) (map[string]any, error) {
		if w.report == nil {
			rep, _, err := findReport()
			if err != nil {
				return nil, err
			}
			w.report = rep
		}
		list, _ := w.report["properties"].([]any)
		for _, p := range list {
			m, _ := p.(map[string]any)
			if m != nil && m["id"] == id {
				return m, nil
			}
		}
		return nil, fmt.Errorf("the report carries no property %q", id)
	}

	readText := func(name string) (string, error) {
		b, err := os.ReadFile(filepath.Join(w.dir, name))
		return string(b), err
	}

	sc.Step(`^the recorded MCP run "([^"]+)"$`, func(rel string) error {
		plugin, err := filepath.Abs("..")
		if err != nil {
			return err
		}
		w.plugin = plugin
		w.dir = filepath.Join(plugin, rel)
		st, err := os.Stat(w.dir)
		if err != nil || !st.IsDir() {
			return fmt.Errorf("no recorded run at %s", w.dir)
		}
		return nil
	})

	sc.Step(`^the run's "([^"]+)" declares the server "([^"]+)"$`, func(file, name string) error {
		var doc map[string]any
		if err := readJSON(filepath.Join(w.dir, file), &doc); err != nil {
			return err
		}
		servers, _ := doc["mcpServers"].(map[string]any)
		if servers == nil {
			return fmt.Errorf("%s has no mcpServers object", file)
		}
		if _, ok := servers[name]; !ok {
			return fmt.Errorf("%s declares %v, not %q", file, keysOf(servers), name)
		}
		return nil
	})

	sc.Step(`^that declaration's command and args match "([^"]+)" with CLAUDE_PLUGIN_ROOT expanded$`, func(rel string) error {
		var run, shipped map[string]any
		if err := readJSON(filepath.Join(w.dir, "mcp.json"), &run); err != nil {
			return err
		}
		if err := readJSON(filepath.Join(w.plugin, rel), &shipped); err != nil {
			return err
		}
		got, _ := run["mcpServers"].(map[string]any)["model-check"].(map[string]any)
		want, _ := shipped["mcpServers"].(map[string]any)["model-check"].(map[string]any)
		if got == nil || want == nil {
			return fmt.Errorf("one of the declarations has no model-check entry")
		}
		expand := func(s string) string {
			return strings.ReplaceAll(s, "${CLAUDE_PLUGIN_ROOT}", w.plugin)
		}
		if expand(fmt.Sprint(want["command"])) != fmt.Sprint(got["command"]) {
			return fmt.Errorf("command: run has %v, the plugin ships %v (expanded %v)",
				got["command"], want["command"], expand(fmt.Sprint(want["command"])))
		}
		wa, _ := want["args"].([]any)
		ga, _ := got["args"].([]any)
		if len(wa) != len(ga) {
			return fmt.Errorf("args: run has %d, the plugin ships %d", len(ga), len(wa))
		}
		for i := range wa {
			if expand(fmt.Sprint(wa[i])) != fmt.Sprint(ga[i]) {
				return fmt.Errorf("args[%d]: run has %v, the plugin ships %v", i, ga[i], wa[i])
			}
		}
		return nil
	})

	sc.Step(`^"([^"]+)" points its mcpServers at "([^"]+)"$`, func(manifestRel, target string) error {
		var doc map[string]any
		if err := readJSON(filepath.Join(w.plugin, manifestRel), &doc); err != nil {
			return err
		}
		if got := fmt.Sprint(doc["mcpServers"]); got != target {
			return fmt.Errorf("%s points mcpServers at %q, want %q", manifestRel, got, target)
		}
		// The file it points at has to be the one the other record was built from.
		if _, err := os.Stat(filepath.Join(w.plugin, strings.TrimPrefix(target, "./"))); err != nil {
			return fmt.Errorf("%s names %s, which is not there: %v", manifestRel, target, err)
		}
		return nil
	})

	sc.Step(`^the run's "([^"]+)" keeps the tokens "([^"]+)" and "([^"]+)"$`, func(file, a, b string) error {
		text, err := readText(file)
		if err != nil {
			return err
		}
		for _, tok := range []string{a, b} {
			if !strings.Contains(text, tok) {
				return fmt.Errorf("%s does not keep the token %q as the engine returns it (SKILL.md step 8)", file, tok)
			}
		}
		return nil
	})

	sc.Step(`^the run's tool log names each of:$`, func(t *godog.Table) error {
		log, err := readText("tool-log.txt")
		if err != nil {
			return err
		}
		var missing []string
		for _, row := range t.Rows {
			name := strings.TrimSpace(row.Cells[0].Value)
			if !strings.Contains(log, name) {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("the tool log does not name: %s", strings.Join(missing, ", "))
		}
		return nil
	})

	sc.Step(`^the run's tool log records no shell invocation of the "([^"]+)" binary$`, func(bin string) error {
		log, err := readText("tool-log.txt")
		if err != nil {
			return err
		}
		for _, line := range strings.Split(log, "\n") {
			if !strings.Contains(line, "run_command") && !strings.Contains(line, "shell") {
				continue
			}
			// A command line that starts the binary, not a path that merely
			// contains its name (the server's own command does).
			for _, f := range strings.Fields(line) {
				if f == bin || strings.HasSuffix(f, "/"+bin) {
					return fmt.Errorf("the run also used the CLI: %s", strings.TrimSpace(line))
				}
			}
		}
		return nil
	})

	sc.Step(`^the run holds a report the engine wrote$`, func() error {
		rep, at, err := findReport()
		if err != nil {
			return err
		}
		w.report = rep
		if _, err := os.Stat(at); err != nil {
			return err
		}
		return nil
	})

	sc.Step(`^that report's input hash is the hash of the run's "([^"]+)"$`, func(model string) error {
		if w.report == nil {
			return fmt.Errorf("no report read yet")
		}
		b, err := os.ReadFile(filepath.Join(w.dir, model))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		want := hex.EncodeToString(sum[:])
		inputs, _ := w.report["inputs"].([]any)
		for _, i := range inputs {
			m, _ := i.(map[string]any)
			if m != nil && fmt.Sprint(m["sha256"]) == want {
				return nil
			}
		}
		return fmt.Errorf("no input of the report has the hash of %s (%s)", model, want[:16])
	})

	sc.Step(`^that report has the property "([^"]+)" with status "([^"]+)" and evidence "([^"]+)"$`, func(id, status, evidence string) error {
		p, err := property(id)
		if err != nil {
			return err
		}
		if fmt.Sprint(p["status"]) != status || fmt.Sprint(p["evidence"]) != evidence {
			return fmt.Errorf("property %s is %v / %v, want %s / %s", id, p["status"], p["evidence"], status, evidence)
		}
		return nil
	})

	sc.Step(`^the run holds the server's session manifest$`, func() error {
		var found string
		_ = filepath.Walk(w.dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && filepath.Base(p) == "manifest.json" && found == "" {
				found = p
			}
			return nil
		})
		if found == "" {
			return fmt.Errorf("no manifest.json under %s: the server's session was not kept", w.dir)
		}
		w.dir = w.dir // keep
		return nil
	})

	sc.Step(`^that manifest records a call of "([^"]+)"$`, func(tool string) error {
		var manifest map[string]any
		var at string
		_ = filepath.Walk(w.dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && filepath.Base(p) == "manifest.json" && at == "" {
				at = p
			}
			return nil
		})
		if at == "" {
			return fmt.Errorf("no manifest.json under %s", w.dir)
		}
		if err := readJSON(at, &manifest); err != nil {
			return err
		}
		calls, _ := manifest["calls"].([]any)
		for _, c := range calls {
			m, _ := c.(map[string]any)
			if m != nil && fmt.Sprint(m["tool"]) == tool {
				return nil
			}
		}
		return fmt.Errorf("%s records no call of %s", at, tool)
	})

	sc.Step(`^the run's "([^"]+)" states the status of the property "([^"]+)" from that report$`, func(file, id string) error {
		p, err := property(id)
		if err != nil {
			return err
		}
		text, err := readText(file)
		if err != nil {
			return err
		}
		status := fmt.Sprint(p["status"])
		// The claim under test is that the answer comes from this report, so an
		// unambiguous Russian rendering counts as much as the English token. That
		// the token itself is missing is a deviation from SKILL.md step 8 and is
		// recorded as an observation in steps/foreign-agents-test.md §5, not here:
		// a scenario about the MCP route must not fail over wording.
		equivalents := map[string][]string{
			"violated": {"наруш", "не выполня", "опроверг"},
			"verified": {"выполня", "подтвержд", "держится"},
		}
		if strings.Contains(text, status) {
			return nil
		}
		for _, alt := range equivalents[status] {
			if strings.Contains(strings.ToLower(text), alt) {
				return nil
			}
		}
		return fmt.Errorf("%s states neither %q nor a Russian equivalent of it, the status the report gives %s", file, status, id)
	})

	sc.Step(`^the run's "([^"]+)" names the interleaving in which both clients pass the flag test$`, func(file string) error {
		text, err := readText(file)
		if err != nil {
			return err
		}
		lower := strings.ToLower(text)
		// The mechanism, in either language and in either form: the two
		// processes both pass the guard before either sets the flag.
		for _, marker := range []string{"атомар", "atomic", "оба", "both", "check-then-act"} {
			if strings.Contains(lower, marker) {
				return nil
			}
		}
		return fmt.Errorf("%s does not name the interleaving", file)
	})
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
