package modelcheck_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/cucumber/godog"
)

// Step definitions for features/g6-package.feature.
//
// Three things here are shared between scenarios instead of repeated, because
// each costs a minute or a copy of the whole plugin: the cross-platform build
// (g6Build), the Go-less sandbox (g6Sandbox) and the sandbox's server session.
// Sharing is safe because no scenario writes to them; a scenario that needed a
// different build would have to say so and would get its own directory.
//
// Nothing here writes into the repository. The build goes to a temporary
// directory, so the committed engine/bin/SHA256SUMS keeps describing the
// release build rather than whatever a test run produced.

const g6TestVersion = "0.1.0-g6-test"

var g6Platforms = [][2]string{
	{"linux", "amd64"}, {"linux", "arm64"},
	{"darwin", "amd64"}, {"darwin", "arm64"},
	{"windows", "amd64"},
}

func init() {
	stepRegistrars = append(stepRegistrars, registerG6Steps)
}

// ------------------------------------------------------------------ shared

type g6BuildResult struct {
	dir      string
	exitCode int
	output   string
	err      error
}

var (
	g6BuildOnce sync.Once
	g6BuildRes  g6BuildResult

	g6SandboxOnce sync.Once
	g6SandboxDir  string   // temp root holding model-check-plugin/ and sandbox-bin/
	g6SandboxEnv  []string // environment with no Go toolchain
	g6SandboxErr  error
)

func g6PluginDir() (string, error) { return filepath.Abs("..") }

// g6RunBuild runs build.sh once per test binary into a fresh temporary
// directory and caches the result.
func g6RunBuild() g6BuildResult {
	g6BuildOnce.Do(func() {
		plugin, err := g6PluginDir()
		if err != nil {
			g6BuildRes.err = err
			return
		}
		dir, err := os.MkdirTemp("", "g6-build-")
		if err != nil {
			g6BuildRes.err = err
			return
		}
		cmd := exec.Command(filepath.Join(plugin, "build.sh"),
			"--version", g6TestVersion, "--out", dir)
		cmd.Dir = plugin
		out, err := cmd.CombinedOutput()
		g6BuildRes = g6BuildResult{dir: dir, output: string(out), exitCode: cmd.ProcessState.ExitCode()}
		if err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				g6BuildRes.err = err
			}
		}
	})
	return g6BuildRes
}

// g6GoFreeEnv builds a PATH holding every ordinary utility of this machine and
// no Go tool. Dropping whole PATH directories would be wrong: /usr/bin carries
// `go` and `uname` alike, and a sandbox without `uname` would test the absence
// of coreutils rather than the absence of Go.
func g6GoFreeEnv(root string) ([]string, error) {
	goTools := map[string]bool{"go": true, "gofmt": true, "godoc": true, "gccgo": true, "tinygo": true}
	binDir := filepath.Join(root, "sandbox-bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d == "" {
			continue
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if goTools[name] || seen[name] {
				continue
			}
			src := filepath.Join(d, name)
			st, err := os.Stat(src)
			if err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
				continue
			}
			if err := os.Symlink(src, filepath.Join(binDir, name)); err == nil {
				seen[name] = true
			}
		}
	}
	drop := map[string]bool{"GOROOT": true, "GOTOOLCHAIN": true, "GOPATH": true,
		"GOFLAGS": true, "GOBIN": true, "GOMODCACHE": true, "GOCACHE": true, "PATH": true}
	env := []string{"PATH=" + binDir}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i > 0 && !drop[kv[:i]] {
			env = append(env, kv)
		}
	}
	return env, nil
}

func g6CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(target, b, info.Mode().Perm())
		}
	})
}

func g6MakeSandbox() (string, []string, error) {
	g6SandboxOnce.Do(func() {
		plugin, err := g6PluginDir()
		if err != nil {
			g6SandboxErr = err
			return
		}
		root, err := os.MkdirTemp("", "g6-install-")
		if err != nil {
			g6SandboxErr = err
			return
		}
		if err := g6CopyTree(plugin, filepath.Join(root, "model-check-plugin")); err != nil {
			g6SandboxErr = err
			return
		}
		env, err := g6GoFreeEnv(root)
		if err != nil {
			g6SandboxErr = err
			return
		}
		g6SandboxDir, g6SandboxEnv = root, env
	})
	return g6SandboxDir, g6SandboxEnv, g6SandboxErr
}

// ------------------------------------------------------- tiny stdio client

// g6Client speaks JSON-RPC over the server's stdin/stdout directly rather than
// through the SDK: what is under test is the packaged binary as a client would
// launch it, so the transport is the plain one a client uses.
type g6Client struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	stderr *bytes.Buffer
	id     int

	// serverInfo from the one initialize handshake: the server answers
	// `initialize` once per connection, so a second call would fail and the
	// implementation name has to be kept from the first.
	serverInfo map[string]any
}

func g6Start(command string, args, env []string, dir string) (*g6Client, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = env
	cmd.Dir = dir
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &g6Client{cmd: cmd, in: in, out: bufio.NewReader(out), stderr: &errBuf}
	res, err := c.call("initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "g6-install-check", "version": "0"},
	})
	if err != nil {
		return nil, err
	}
	c.serverInfo, _ = res["serverInfo"].(map[string]any)
	if err := c.notify("notifications/initialized", map[string]any{}); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *g6Client) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(b, '\n'))
	return err
}

func (c *g6Client) notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *g6Client) call(method string, params any) (map[string]any, error) {
	c.id++
	id := c.id
	if err := c.write(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		line, err := c.out.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("%s: %w (server stderr: %s)", method, err, c.stderr.String())
		}
		var msg map[string]any
		if json.Unmarshal([]byte(line), &msg) != nil {
			continue
		}
		got, ok := msg["id"].(float64)
		if !ok || int(got) != id {
			continue
		}
		if e, ok := msg["error"]; ok {
			return nil, fmt.Errorf("%s: %v", method, e)
		}
		res, _ := msg["result"].(map[string]any)
		return res, nil
	}
}

func (c *g6Client) tool(name string, args map[string]any) (map[string]any, error) {
	res, err := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return nil, err
	}
	if isErr, _ := res["isError"].(bool); isErr {
		return nil, fmt.Errorf("%s returned a tool error: %v", name, res["content"])
	}
	sc, _ := res["structuredContent"].(map[string]any)
	if sc == nil {
		return nil, fmt.Errorf("%s returned no structured content", name)
	}
	return sc, nil
}

func (c *g6Client) close() {
	_ = c.in.Close()
	done := make(chan struct{})
	go func() { _ = c.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		_ = c.cmd.Process.Kill()
	}
}

// -------------------------------------------------------------- the world

type g6World struct {
	plugin string
	outDir string

	build g6BuildResult

	sandboxRoot string
	sandboxEnv  []string
	client      *g6Client

	serverInfo map[string]any
	tools      []string

	properties []map[string]any

	manifest map[string]any
	servers  map[string]string // server name -> the source that declared it, last wins

	jsonArray []map[string]any
	results   map[string]any
	skillDesc string
}

func g6HostBinary(dir string) string {
	name := fmt.Sprintf("mcd-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name)
}

func g6ReadJSON(path string, into any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, into)
}

func g6HasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// g6Description returns the `description:` block of a SKILL.md frontmatter,
// folded to one line, the same way the measurement harness reads it.
func g6Description(skillMD string) (string, error) {
	b, err := os.ReadFile(skillMD)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(b), "\n")
	var out []string
	in := false
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "description: >"):
			in = true
		case in && strings.HasPrefix(l, "  "):
			out = append(out, strings.TrimSpace(l))
		case in:
			in = false
		}
	}
	if len(out) == 0 {
		return "", fmt.Errorf("%s: no `description: >` block", skillMD)
	}
	return strings.Join(out, " "), nil
}

// g6MCPConfig returns the path of the MCP config the plugin's manifest names,
// so that the scenarios follow whatever plugin.json declares instead of a path
// written twice. A plugin-root .mcp.json would be read too — by a client, and
// as a *project* config — which is exactly what the packaging avoids, so this
// helper deliberately resolves only the declared pointer.
func g6MCPConfig(pluginDir string) (string, error) {
	var manifest map[string]any
	if err := g6ReadJSON(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), &manifest); err != nil {
		return "", err
	}
	decl, _ := manifest["mcpServers"].(string)
	if decl == "" {
		return "", fmt.Errorf("plugin.json declares no mcpServers path")
	}
	return filepath.Join(pluginDir, filepath.Clean(decl)), nil
}

// ---------------------------------------------------------------- the steps

func registerG6Steps(sc *godog.ScenarioContext) {
	w := &g6World{}

	must := func(cond bool, format string, a ...any) error {
		if cond {
			return nil
		}
		return fmt.Errorf(format, a...)
	}

	setPlugin := func() error {
		p, err := g6PluginDir()
		if err != nil {
			return err
		}
		w.plugin = p
		return nil
	}

	// ------------------------------------------------------------- build
	sc.Step(`^a clean build output directory$`, func() error {
		if err := setPlugin(); err != nil {
			return err
		}
		w.build = g6RunBuild()
		if w.build.err != nil {
			return w.build.err
		}
		w.outDir = w.build.dir
		return nil
	})

	sc.Step(`^the build script "([^"]+)" is run into it with version "([^"]+)"$`,
		func(script, version string) error {
			if version != g6TestVersion {
				return fmt.Errorf("the suite builds once with version %q; %q would need its own build",
					g6TestVersion, version)
			}
			if filepath.Base(script) != "build.sh" {
				return fmt.Errorf("unexpected script %q", script)
			}
			return nil
		})

	sc.Step(`^the script exits with code (\d+)$`, func(code int) error {
		return must(w.build.exitCode == code, "build.sh exited %d, want %d\n%s",
			w.build.exitCode, code, w.build.output)
	})

	sc.Step(`^the output directory contains a binary for each of these platforms:$`,
		func(t *godog.Table) error {
			want := map[string]bool{}
			for _, row := range t.Rows[1:] {
				goos, goarch := row.Cells[0].Value, row.Cells[1].Value
				name := fmt.Sprintf("mcd-%s-%s", goos, goarch)
				if goos == "windows" {
					name += ".exe"
				}
				want[name] = true
			}
			if len(want) != len(g6Platforms) {
				return fmt.Errorf("the table lists %d platforms, the build lists %d",
					len(want), len(g6Platforms))
			}
			for name := range want {
				st, err := os.Stat(filepath.Join(w.outDir, name))
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				if st.Size() < 1<<20 {
					return fmt.Errorf("%s is %d bytes — too small to be the engine", name, st.Size())
				}
			}
			return nil
		})

	sc.Step(`^every file in the output directory has an entry in "([^"]+)" whose digest matches it$`,
		func(sums string) error {
			lines, err := os.ReadFile(filepath.Join(w.outDir, sums))
			if err != nil {
				return err
			}
			listed := map[string]string{}
			for _, l := range strings.Split(string(lines), "\n") {
				f := strings.Fields(l)
				if len(f) == 2 {
					listed[strings.TrimPrefix(f[1], "*")] = f[0]
				}
			}
			entries, err := os.ReadDir(w.outDir)
			if err != nil {
				return err
			}
			for _, e := range entries {
				name := e.Name()
				if name == sums || name == "BUILD-INFO.json" {
					continue
				}
				digest, ok := listed[name]
				if !ok {
					return fmt.Errorf("%s has no entry in %s", name, sums)
				}
				b, err := os.ReadFile(filepath.Join(w.outDir, name))
				if err != nil {
					return err
				}
				sum := sha256.Sum256(b)
				if got := hex.EncodeToString(sum[:]); got != digest {
					return fmt.Errorf("%s: %s records %s, the file hashes to %s",
						name, sums, digest, got)
				}
			}
			return nil
		})

	sc.Step(`^"([^"]+)" has no entry for a file that is absent$`, func(sums string) error {
		lines, err := os.ReadFile(filepath.Join(w.outDir, sums))
		if err != nil {
			return err
		}
		for _, l := range strings.Split(string(lines), "\n") {
			f := strings.Fields(l)
			if len(f) != 2 {
				continue
			}
			name := strings.TrimPrefix(f[1], "*")
			if _, err := os.Stat(filepath.Join(w.outDir, name)); err != nil {
				return fmt.Errorf("%s lists %s, which is not there", sums, name)
			}
		}
		return nil
	})

	sc.Step(`^the build record says the build used CGO_ENABLED=0$`, func() error {
		var info map[string]any
		if err := g6ReadJSON(filepath.Join(w.outDir, "BUILD-INFO.json"), &info); err != nil {
			return err
		}
		return must(fmt.Sprint(info["cgo_enabled"]) == "0",
			"BUILD-INFO.json records cgo_enabled=%v", info["cgo_enabled"])
	})

	sc.Step(`^running the host binary with argument "([^"]+)" prints "([^"]+)"$`,
		func(arg, want string) error {
			out, err := exec.Command(g6HostBinary(w.outDir), arg).Output()
			if err != nil {
				return err
			}
			return must(strings.Contains(string(out), want),
				"`mcd %s` printed %q, which does not contain %q", arg, strings.TrimSpace(string(out)), want)
		})

	sc.Step(`^the version recorded in "([^"]+)" is "([^"]+)"$`, func(file, want string) error {
		var info map[string]any
		if err := g6ReadJSON(filepath.Join(w.outDir, file), &info); err != nil {
			return err
		}
		return must(info["version"] == want, "%s records version %v, want %s", file, info["version"], want)
	})

	// ---------------------------------------------- .mcp.json command on this host
	sc.Step(`^the packaged plugin directory$`, func() error {
		if err := setPlugin(); err != nil {
			return err
		}
		w.outDir = filepath.Join(w.plugin, "engine", "bin")
		if _, err := os.Stat(g6HostBinary(w.outDir)); err != nil {
			return fmt.Errorf("the plugin is not packaged: %w (run model-check-plugin/build.sh)", err)
		}
		return nil
	})

	sc.Step(`^the command in the plugin's declared MCP config resolves, on this host, to an executable file under the plugin directory$`,
		func() error {
			rel, err := g6MCPConfig(w.plugin)
			if err != nil {
				return err
			}
			var cfg struct {
				MCPServers map[string]struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"mcpServers"`
			}
			if err := g6ReadJSON(rel, &cfg); err != nil {
				return err
			}
			srv, ok := cfg.MCPServers["model-check"]
			if !ok {
				return fmt.Errorf("%s declares no server named model-check", rel)
			}
			cmd := strings.ReplaceAll(srv.Command, "${CLAUDE_PLUGIN_ROOT}", w.plugin)
			st, err := os.Stat(cmd)
			if err != nil {
				return err
			}
			if st.Mode()&0o111 == 0 {
				return fmt.Errorf("%s is not executable", cmd)
			}
			if !strings.HasPrefix(cmd, w.plugin+string(os.PathSeparator)) {
				return fmt.Errorf("%s lies outside the plugin directory", cmd)
			}
			w.results = map[string]any{"command": cmd}
			return nil
		})

	sc.Step(`^running that command with argument "([^"]+)" prints the version recorded in "([^"]+)"$`,
		func(arg, rel string) error {
			var info map[string]any
			if err := g6ReadJSON(filepath.Join(w.plugin, rel), &info); err != nil {
				return err
			}
			want, _ := info["version"].(string)
			out, err := exec.Command(w.results["command"].(string), arg).Output()
			if err != nil {
				return err
			}
			return must(strings.Contains(string(out), want),
				"the wrapper printed %q, which does not contain the packaged version %q",
				strings.TrimSpace(string(out)), want)
		})

	// -------------------------------------------------------- the sandbox
	sc.Step(`^a sandbox copy of the plugin in a temporary directory$`, func() error {
		root, env, err := g6MakeSandbox()
		if err != nil {
			return err
		}
		w.sandboxRoot, w.sandboxEnv = root, env
		return nil
	})

	sc.Step(`^no "([^"]+)" executable on PATH inside the sandbox$`, func(tool string) error {
		var path string
		for _, kv := range w.sandboxEnv {
			if strings.HasPrefix(kv, "PATH=") {
				path = kv[len("PATH="):]
			}
		}
		for _, d := range filepath.SplitList(path) {
			if st, err := os.Stat(filepath.Join(d, tool)); err == nil && st.Mode()&0o111 != 0 {
				return fmt.Errorf("%s/%s exists: the sandbox would prove nothing", d, tool)
			}
		}
		return must(path != "", "the sandbox environment carries no PATH at all")
	})

	sc.Step(`^the MCP server is started from the packaged binary over stdio inside the sandbox$`,
		func() error {
			root := filepath.Join(w.sandboxRoot, "model-check-plugin")
			cfgPath, err := g6MCPConfig(root)
			if err != nil {
				return err
			}
			var cfg struct {
				MCPServers map[string]struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"mcpServers"`
			}
			if err := g6ReadJSON(cfgPath, &cfg); err != nil {
				return err
			}
			srv := cfg.MCPServers["model-check"]
			cmd := strings.ReplaceAll(srv.Command, "${CLAUDE_PLUGIN_ROOT}", root)
			args := make([]string, 0, len(srv.Args))
			for _, a := range srv.Args {
				args = append(args, strings.ReplaceAll(a, "${CLAUDE_PLUGIN_ROOT}", root))
			}
			c, err := g6Start(cmd, args, w.sandboxEnv, w.sandboxRoot)
			if err != nil {
				return err
			}
			w.client = c
			return nil
		})

	sc.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if w.client != nil {
			w.client.close()
			w.client = nil
		}
		return ctx, nil
	})

	sc.Step(`^the client sends "initialize" and then "tools/list"$`, func() error {
		// `initialize` happened when the process started (a server answers it
		// once per connection); this asks for the tools and keeps what that
		// handshake reported.
		res, err := w.client.call("tools/list", map[string]any{})
		if err != nil {
			return err
		}
		list, _ := res["tools"].([]any)
		w.tools = nil
		for _, t := range list {
			m, _ := t.(map[string]any)
			if n, ok := m["name"].(string); ok {
				w.tools = append(w.tools, n)
			}
		}
		sort.Strings(w.tools)
		w.serverInfo = w.client.serverInfo
		return nil
	})

	sc.Step(`^the server reports its implementation name "([^"]+)"$`, func(name string) error {
		if w.serverInfo == nil {
			return fmt.Errorf("no serverInfo was captured")
		}
		return must(w.serverInfo["name"] == name,
			"serverInfo.name is %v, want %s", w.serverInfo["name"], name)
	})

	sc.Step(`^"tools/list" returns exactly these tools:$`, func(t *godog.Table) error {
		var want []string
		for _, row := range t.Rows[1:] {
			want = append(want, strings.TrimSpace(row.Cells[0].Value))
		}
		sort.Strings(want)
		if strings.Join(want, ",") != strings.Join(w.tools, ",") {
			return fmt.Errorf("tools/list returned [%s], want [%s]",
				strings.Join(w.tools, ", "), strings.Join(want, ", "))
		}
		return nil
	})

	sc.Step(`^no Go toolchain was reachable from the environment the server ran in$`, func() error {
		for _, kv := range w.sandboxEnv {
			for _, bad := range []string{"GOROOT=", "GOTOOLCHAIN=", "GOPATH=", "GOFLAGS="} {
				if strings.HasPrefix(kv, bad) {
					return fmt.Errorf("the server's environment carried %s", kv)
				}
			}
		}
		var path string
		for _, kv := range w.sandboxEnv {
			if strings.HasPrefix(kv, "PATH=") {
				path = kv[len("PATH="):]
			}
		}
		for _, tool := range []string{"go", "gofmt", "gccgo"} {
			for _, d := range filepath.SplitList(path) {
				if _, err := os.Stat(filepath.Join(d, tool)); err == nil {
					return fmt.Errorf("%s was reachable at %s/%s", tool, d, tool)
				}
			}
		}
		return nil
	})

	sc.Step(`^the client parses the petrinet1 net and checks "([^"]+)" on it$`, func(kind string) error {
		root := filepath.Join(w.sandboxRoot, "model-check-plugin")
		var net any
		if err := g6ReadJSON(filepath.Join(root, "engine/testdata/petri/petrinet1.json"), &net); err != nil {
			return err
		}
		parsed, err := w.client.tool("mc_parse", map[string]any{"petri": net})
		if err != nil {
			return err
		}
		if parsed["outcome"] != "ir" {
			return fmt.Errorf("mc_parse outcome %v, want ir", parsed["outcome"])
		}
		checked, err := w.client.tool("mc_check", map[string]any{
			"session_id": parsed["session_id"],
			"properties": []map[string]any{{"id": kind, "kind": kind}},
		})
		if err != nil {
			return err
		}
		w.properties = nil
		props, _ := checked["properties"].([]any)
		for _, p := range props {
			if m, ok := p.(map[string]any); ok {
				w.properties = append(w.properties, m)
			}
		}
		return must(len(w.properties) > 0, "mc_check returned no properties")
	})

	find := func(id string) map[string]any {
		for _, p := range w.properties {
			if p["id"] == id {
				return p
			}
		}
		return nil
	}

	sc.Step(`^the property "([^"]+)" has status "([^"]+)" and evidence "([^"]+)"$`,
		func(id, status, evidence string) error {
			p := find(id)
			if p == nil {
				return fmt.Errorf("no property %q in the answer", id)
			}
			if p["status"] != status || p["evidence"] != evidence {
				return fmt.Errorf("%s: status %v evidence %v, want %s / %s",
					id, p["status"], p["evidence"], status, evidence)
			}
			return nil
		})

	sc.Step(`^the counterexample summary is "([^"]+)"$`, func(want string) error {
		for _, p := range w.properties {
			cex, _ := p["counterexample"].(map[string]any)
			if cex != nil && cex["summary"] == want {
				return nil
			}
		}
		return fmt.Errorf("no property carries a counterexample with summary %q", want)
	})

	// ----------------------------------------------------- one registration
	sc.Step(`^the plugin manifest "([^"]+)"$`, func(rel string) error {
		if err := setPlugin(); err != nil {
			return err
		}
		return g6ReadJSON(filepath.Join(w.plugin, rel), &w.manifest)
	})

	sc.Step(`^the portable plugin manifest "([^"]+)"$`, func(rel string) error {
		if err := setPlugin(); err != nil {
			return err
		}
		return g6ReadJSON(filepath.Join(w.plugin, rel), &w.manifest)
	})

	sc.Step(`^its version matches "([^"]+)"$`, func(rel string) error {
		var other map[string]any
		if err := g6ReadJSON(filepath.Join(w.plugin, rel), &other); err != nil {
			return err
		}
		got, _ := w.manifest["version"].(string)
		want, _ := other["version"].(string)
		return must(got != "" && got == want,
			"portable manifest version %q does not match %s version %q", got, rel, want)
	})

	sc.Step(`^its MCP declaration "([^"]+)" matches "([^"]+)" for server "([^"]+)"$`,
		func(portableRel, claudeRel, name string) error {
			var portable, claude struct {
				MCPServers map[string]map[string]any `json:"mcpServers"`
			}
			if err := g6ReadJSON(filepath.Join(w.plugin, portableRel), &portable); err != nil {
				return err
			}
			if err := g6ReadJSON(filepath.Join(w.plugin, claudeRel), &claude); err != nil {
				return err
			}
			p, pok := portable.MCPServers[name]
			c, cok := claude.MCPServers[name]
			if !pok || !cok {
				return fmt.Errorf("server %q is missing from portable=%t Claude=%t declarations", name, pok, cok)
			}
			if len(portable.MCPServers) != 1 || len(claude.MCPServers) != 1 {
				return fmt.Errorf("declarations must contain exactly one server: portable=%d Claude=%d",
					len(portable.MCPServers), len(claude.MCPServers))
			}
			if len(p) != len(c) {
				return fmt.Errorf("server %q declarations have different key sets: portable=%v Claude=%v", name, p, c)
			}
			for key := range p {
				if _, ok := c[key]; !ok {
					return fmt.Errorf("server %q Claude declaration is missing key %q", name, key)
				}
			}
			for _, key := range []string{"type", "args"} {
				if !reflect.DeepEqual(p[key], c[key]) {
					return fmt.Errorf("server %q field %s differs: portable=%v Claude=%v", name, key, p[key], c[key])
				}
			}
			if p["command"] != "${PLUGIN_ROOT}/engine/bin/mcd" ||
				c["command"] != "${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd" ||
				p["cwd"] != "${PLUGIN_ROOT}" || c["cwd"] != "${CLAUDE_PLUGIN_ROOT}" {
				return fmt.Errorf("server %q has invalid root variables: portable=%v Claude=%v", name, p, c)
			}
			normalizeRoot := func(v any) string {
				s := fmt.Sprint(v)
				s = strings.ReplaceAll(s, "${PLUGIN_ROOT}", "${ROOT}")
				return strings.ReplaceAll(s, "${CLAUDE_PLUGIN_ROOT}", "${ROOT}")
			}
			for _, key := range []string{"command", "cwd"} {
				portableValue := normalizeRoot(p[key])
				claudeValue := normalizeRoot(c[key])
				if portableValue != claudeValue {
					return fmt.Errorf("server %q field %s differs after root normalization: portable=%q Claude=%q",
						name, key, portableValue, claudeValue)
				}
			}
			if normalizeRoot(p["command"]) != "${ROOT}/engine/bin/mcd" {
				return fmt.Errorf("server %q command must resolve to ${ROOT}/engine/bin/mcd: %v", name, p["command"])
			}
			return nil
		})

	sc.Step(`^the release contains the Windows binary alias "([^"]+)"$`, func(rel string) error {
		if err := setPlugin(); err != nil {
			return err
		}
		info, err := os.Stat(filepath.Join(w.plugin, rel))
		if err != nil {
			return err
		}
		return must(info.Mode()&0o111 != 0, "%s is not executable", rel)
	})

	sc.Step(`^the Windows binary "([^"]+)" exists$`, func(rel string) error {
		if err := setPlugin(); err != nil {
			return err
		}
		_, err := os.Stat(filepath.Join(w.plugin, rel))
		return err
	})

	sc.Step(`^the plugin file "([^"]+)" contains each of:$`, func(rel string, t *godog.Table) error {
		if err := setPlugin(); err != nil {
			return err
		}
		body, err := os.ReadFile(filepath.Join(w.plugin, rel))
		if err != nil {
			return err
		}
		text := string(body)
		var missing []string
		for _, phrase := range tableColumn(t) {
			if !strings.Contains(text, phrase) {
				missing = append(missing, phrase)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s does not contain: %v", rel, missing)
		}
		return nil
	})

	sc.Step(`^the Coddy MCP declaration uses the portable workspace command "([^"]+)"$`, func(want string) error {
		if err := setPlugin(); err != nil {
			return err
		}
		var cfg struct {
			MCPServers map[string]struct {
				Command string `json:"command"`
			} `json:"mcpServers"`
		}
		if err := g6ReadJSON(filepath.Join(w.plugin, ".coddy", "mcp.json"), &cfg); err != nil {
			return err
		}
		server, ok := cfg.MCPServers["model-check"]
		if !ok {
			return fmt.Errorf(".coddy/mcp.json has no model-check server")
		}
		return must(server.Command == want, ".coddy/mcp.json command is %q, want %q", server.Command, want)
	})

	sc.Step(`^the manifest is valid JSON with a kebab-case "name" and a "version"$`, func() error {
		name, _ := w.manifest["name"].(string)
		if name == "" {
			return fmt.Errorf("no name")
		}
		for _, r := range name {
			if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
				return fmt.Errorf("name %q is not kebab-case", name)
			}
		}
		v, _ := w.manifest["version"].(string)
		return must(v != "", "no version")
	})

	sc.Step(`^the MCP sources the plugin declares resolve to exactly one server named "([^"]+)"$`,
		func(name string) error {
			// The sources a client reads, in the documented order: the
			// plugin-root .mcp.json first, then what `mcpServers` declares.
			// The packaging leaves the first of those absent (see the next
			// step), so in practice there is one source; the loop still looks
			// for both, so that re-introducing the file fails here.
			w.servers = map[string]string{}
			add := func(path, source string) error {
				var cfg struct {
					MCPServers map[string]any `json:"mcpServers"`
				}
				if err := g6ReadJSON(path, &cfg); err != nil {
					return err
				}
				for n := range cfg.MCPServers {
					w.servers[n] = source
				}
				return nil
			}
			rootMCP := filepath.Join(w.plugin, ".mcp.json")
			if _, err := os.Stat(rootMCP); err == nil {
				if err := add(rootMCP, ".mcp.json at the plugin root"); err != nil {
					return err
				}
			}
			if decl, ok := w.manifest["mcpServers"].(string); ok {
				p := filepath.Join(w.plugin, filepath.Clean(decl))
				if err := add(p, "plugin.json mcpServers -> "+decl); err != nil {
					return err
				}

			}
			if len(w.servers) != 1 {
				return fmt.Errorf("the declared sources resolve to %d servers (%v), want exactly one",
					len(w.servers), w.servers)
			}
			if _, ok := w.servers[name]; !ok {
				return fmt.Errorf("the one server is not named %s: %v", name, w.servers)
			}
			return nil
		})

	sc.Step(`^the plugin root holds no "([^"]+)", which a client also reads as a project config$`,
		func(name string) error {
			p := filepath.Join(w.plugin, name)
			if _, err := os.Stat(p); err == nil {
				return fmt.Errorf("%s exists: a client reads it a second time as a project "+
					"config when the plugin directory is the working directory, and lists the "+
					"server twice (measured in steps/g6-confirmation.md §3)", p)
			} else if !os.IsNotExist(err) {
				return err
			}
			return nil
		})

	sc.Step(`^the repository root holds no "([^"]+)" that declares a server named "([^"]+)"$`,
		func(rel, name string) error {
			p := filepath.Join(filepath.Dir(w.plugin), rel)
			b, err := os.ReadFile(p)
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			var cfg struct {
				MCPServers map[string]any `json:"mcpServers"`
			}
			if json.Unmarshal(b, &cfg) != nil {
				return nil
			}
			if _, ok := cfg.MCPServers[name]; ok {
				return fmt.Errorf("%s also declares %s: a project-scoped registration beside the plugin's", p, name)
			}
			return nil
		})

	// ------------------------------------------------------ trigger eval set
	sc.Step(`^the file "([^"]+)"$`, func(rel string) error {
		if err := setPlugin(); err != nil {
			return err
		}
		p := filepath.Join(w.plugin, rel)
		switch {
		case strings.HasSuffix(rel, "trigger-eval.json"):
			return g6ReadJSON(p, &w.jsonArray)
		case strings.HasSuffix(rel, ".json"):
			return g6ReadJSON(p, &w.results)
		default:
			d, err := g6Description(p)
			if err != nil {
				return err
			}
			w.skillDesc = d
			return nil
		}
	})

	sc.Step(`^it is a JSON array of at least (\d+) entries$`, func(n int) error {
		return must(len(w.jsonArray) >= n, "the set has %d entries, want at least %d", len(w.jsonArray), n)
	})

	sc.Step(`^every entry has a non-empty "query" and a boolean "should_trigger"$`, func() error {
		for i, e := range w.jsonArray {
			q, _ := e["query"].(string)
			if strings.TrimSpace(q) == "" {
				return fmt.Errorf("entry %d has an empty query", i)
			}
			if _, ok := e["should_trigger"].(bool); !ok {
				return fmt.Errorf("entry %d: should_trigger is %T, want a boolean", i, e["should_trigger"])
			}
		}
		return nil
	})

	sc.Step(`^at least (\d+) entries have "should_trigger" (true|false)$`, func(n int, val string) error {
		want := val == "true"
		got := 0
		for _, e := range w.jsonArray {
			if b, _ := e["should_trigger"].(bool); b == want {
				got++
			}
		}
		return must(got >= n, "%d entries have should_trigger %s, want at least %d", got, val, n)
	})

	sc.Step(`^at least one entry that must trigger is written in (Russian|English)$`, func(lang string) error {
		for _, e := range w.jsonArray {
			b, _ := e["should_trigger"].(bool)
			q, _ := e["query"].(string)
			if !b {
				continue
			}
			if (lang == "Russian") == g6HasCyrillic(q) {
				return nil
			}
		}
		return fmt.Errorf("no must-trigger query is written in %s", lang)
	})

	sc.Step(`^no two entries have the same "query"$`, func() error {
		seen := map[string]bool{}
		for _, e := range w.jsonArray {
			q, _ := e["query"].(string)
			if seen[q] {
				return fmt.Errorf("duplicate query: %.40s…", q)
			}
			seen[q] = true
		}
		return nil
	})

	// ------------------------------------------------------ trigger results
	sc.Step(`^it names the split sizes and the seed used to make the split$`, func() error {
		split, _ := w.results["split"].(map[string]any)
		if split == nil {
			return fmt.Errorf("no split recorded")
		}
		for _, k := range []string{"train", "test"} {
			l, _ := split[k].([]any)
			if len(l) == 0 {
				return fmt.Errorf("the %s split is empty", k)
			}
		}
		if _, ok := w.results["seed"]; !ok {
			return fmt.Errorf("no seed recorded: the split could not be remade")
		}
		return nil
	})

	sc.Step(`^it carries a train score and a test score for every description variant measured$`,
		func() error {
			variants, _ := w.results["variants"].([]any)
			if len(variants) == 0 {
				return fmt.Errorf("no variants recorded")
			}
			for _, v := range variants {
				m, _ := v.(map[string]any)
				for _, k := range []string{"train_accuracy", "test_accuracy", "variant"} {
					if _, ok := m[k]; !ok {
						return fmt.Errorf("variant %v lacks %s", m["variant"], k)
					}
				}
			}
			return nil
		})

	sc.Step(`^the variant recorded as chosen is the one with the best test score$`, func() error {
		variants, _ := w.results["variants"].([]any)
		chosen, _ := w.results["chosen"].(string)
		best, bestName := -1.0, ""
		for _, v := range variants {
			m, _ := v.(map[string]any)
			acc, _ := m["test_accuracy"].(float64)
			if acc > best {
				best, bestName = acc, fmt.Sprint(m["variant"])
			}
		}
		if chosen == "" {
			return fmt.Errorf("no chosen variant recorded")
		}
		var chosenAcc float64
		for _, v := range variants {
			m, _ := v.(map[string]any)
			if fmt.Sprint(m["variant"]) == chosen {
				chosenAcc, _ = m["test_accuracy"].(float64)
			}
		}
		return must(chosenAcc >= best,
			"chosen variant %s scores %.4f on the held-out split, but %s scores %.4f",
			chosen, chosenAcc, bestName, best)
	})

	sc.Step(`^the description of "([^"]+)" equals the chosen variant$`, func(rel string) error {
		if err := setPlugin(); err != nil {
			return err
		}
		got, err := g6Description(filepath.Join(w.plugin, rel))
		if err != nil {
			return err
		}
		want, _ := w.results["chosen_description"].(string)
		if strings.Join(strings.Fields(got), " ") != strings.Join(strings.Fields(want), " ") {
			return fmt.Errorf("SKILL.md's description is not the one the results call chosen")
		}
		return nil
	})

	sc.Step(`^its description mentions "([^"]+)" and "([^"]+)" and "([^"]+)"$`,
		func(a, b, c string) error {
			for _, s := range []string{a, b, c} {
				if !strings.Contains(w.skillDesc, s) {
					return fmt.Errorf("the description does not mention %q", s)
				}
			}
			return nil
		})

	sc.Step(`^its description still says that no external model checker is needed$`, func() error {
		low := strings.ToLower(w.skillDesc)
		return must(strings.Contains(low, "no external") &&
			(strings.Contains(low, "spin") || strings.Contains(low, "model checker")),
			"the description no longer says that no external model checker is needed")
	})

	// --------------------------------------------------------- iteration-4
	sc.Step(`^the eval file "([^"]+)"$`, func(rel string) error {
		if err := setPlugin(); err != nil {
			return err
		}
		var doc struct {
			Evals []map[string]any `json:"evals"`
		}
		if err := g6ReadJSON(filepath.Join(w.plugin, rel), &doc); err != nil {
			return err
		}
		w.jsonArray = doc.Evals
		return nil
	})

	sc.Step(`^the workspace iteration "([^"]+)"$`, func(rel string) error {
		w.outDir = filepath.Join(w.plugin, rel)
		st, err := os.Stat(w.outDir)
		if err != nil || !st.IsDir() {
			return fmt.Errorf("%s is not a directory", rel)
		}
		return nil
	})

	// evalDirs maps eval id -> its directory in the iteration.
	evalDirs := func() (map[string]string, error) {
		out := map[string]string{}
		entries, err := os.ReadDir(w.outDir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			var meta map[string]any
			if err := g6ReadJSON(filepath.Join(w.outDir, e.Name(), "eval_metadata.json"), &meta); err != nil {
				continue
			}
			out[fmt.Sprint(meta["eval_id"])] = filepath.Join(w.outDir, e.Name())
		}
		return out, nil
	}

	runnableByG5 := func() []map[string]any {
		order := map[string]int{"G0": 0, "G1": 1, "G2": 2, "G3": 3, "G4": 4, "G5": 5, "G6": 6, "G7": 7}
		var out []map[string]any
		for _, e := range w.jsonArray {
			step, _ := e["runnable_from"].(string)
			if n, ok := order[step]; ok && n <= order["G5"] {
				out = append(out, e)
			}
		}
		return out
	}

	graded := func(config string) error {
		dirs, err := evalDirs()
		if err != nil {
			return err
		}
		for _, e := range runnableByG5() {
			id := fmt.Sprint(e["id"])
			d, ok := dirs[id]
			if !ok {
				return fmt.Errorf("eval %s (runnable from %v) has no directory in the iteration",
					id, e["runnable_from"])
			}
			var g map[string]any
			if err := g6ReadJSON(filepath.Join(d, config, "grading.json"), &g); err != nil {
				return fmt.Errorf("eval %s, %s: %w", id, config, err)
			}
			if _, ok := g["expectations"]; !ok {
				return fmt.Errorf("eval %s, %s: grading.json carries no expectations", id, config)
			}
		}
		return nil
	}

	sc.Step(`^every eval whose "runnable_from" is at most "G5" has a graded run with the skill$`,
		func() error {
			if len(runnableByG5()) == 0 {
				return fmt.Errorf("no eval is runnable by G5 — the eval file was read wrongly")
			}
			return graded("with_skill")
		})

	sc.Step(`^every such eval has a graded run without the skill$`, func() error {
		return graded("without_skill")
	})

	sc.Step(`^"([^"]+)" lists both configurations for each of them$`, func(rel string) error {
		var bench map[string]any
		if err := g6ReadJSON(filepath.Join(w.plugin, rel), &bench); err != nil {
			return err
		}
		runs, _ := bench["runs"].([]any)
		seen := map[string]map[string]bool{}
		for _, r := range runs {
			m, _ := r.(map[string]any)
			id := fmt.Sprint(m["eval_id"])
			if seen[id] == nil {
				seen[id] = map[string]bool{}
			}
			seen[id][fmt.Sprint(m["configuration"])] = true
		}
		for _, e := range runnableByG5() {
			id := fmt.Sprint(e["id"])
			for _, cfg := range []string{"with_skill", "without_skill"} {
				if !seen[id][cfg] {
					return fmt.Errorf("%s: eval %s has no %s run", rel, id, cfg)
				}
			}
		}
		return nil
	})

	sc.Step(`^the benchmark emits "([^"]+)" before "([^"]+)"$`, func(first, second string) error {
		var bench map[string]any
		if err := g6ReadJSON(filepath.Join(w.outDir, "benchmark.json"), &bench); err != nil {
			return err
		}
		runs, _ := bench["runs"].([]any)
		seenSecond := false
		for _, r := range runs {
			m, _ := r.(map[string]any)
			switch fmt.Sprint(m["configuration"]) {
			case second:
				seenSecond = true
			case first:
				if seenSecond {
					return fmt.Errorf("a %s run follows a %s run", first, second)
				}
			}
		}
		return nil
	})
}
