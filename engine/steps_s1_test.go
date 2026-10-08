package modelcheck_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cucumber/godog"
)

// Step definitions for features/s1-security.feature.
//
// The scenarios read files at the monorepo root and run
// scripts/security-scan.sh only with a setting the script must reject before
// it touches any scanner, so no scanner, docker image or network is needed.

func init() {
	stepRegistrars = append(stepRegistrars, registerS1Steps)
}

type s1World struct {
	root     string
	exitCode int
	output   string
}

func s1Root() (string, error) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, "scripts", "security-scan.sh")); err != nil {
		return "", fmt.Errorf("repository root %s has no scripts/security-scan.sh: %w", root, err)
	}
	return root, nil
}

func (w *s1World) read(rel string) (string, error) {
	b, err := os.ReadFile(filepath.Join(w.root, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", rel, err)
	}
	return string(b), nil
}

// s1Body drops the YAML frontmatter of a rule file.
func s1Body(text string) string {
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	rest := text[4:]
	i := strings.Index(rest, "\n---\n")
	if i < 0 {
		return text
	}
	return strings.TrimLeft(rest[i+5:], "\n")
}

func registerS1Steps(sc *godog.ScenarioContext) {
	w := &s1World{}

	sc.Step(`^the repository root$`, func() error {
		root, err := s1Root()
		if err != nil {
			return err
		}
		w.root = root
		return nil
	})

	sc.Step(`^the repository file "([^"]+)" exists$`, func(rel string) error {
		if _, err := os.Stat(filepath.Join(w.root, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		return nil
	})

	sc.Step(`^the repository file "([^"]+)" exists and is executable$`, func(rel string) error {
		fi, err := os.Stat(filepath.Join(w.root, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		if fi.Mode()&0o111 == 0 {
			return fmt.Errorf("%s is not executable (mode %v)", rel, fi.Mode())
		}
		return nil
	})

	sc.Step(`^the security gate scans with "([^"]+)" by default$`, func(want string) error {
		text, err := w.read("scripts/security-scan.sh")
		if err != nil {
			return err
		}
		re := regexp.MustCompile(`(?m)^SEC_SCANNERS="\$\{SEC_SCANNERS:-([^}]*)\}"`)
		m := re.FindStringSubmatch(text)
		if m == nil {
			return errors.New("scripts/security-scan.sh sets no default for SEC_SCANNERS")
		}
		if m[1] != want {
			return fmt.Errorf("default SEC_SCANNERS is %q, want %q", m[1], want)
		}
		return nil
	})

	sc.Step(`^the security gate pins the image "([A-Z_]+)" by digest$`, func(name string) error {
		text, err := w.read("scripts/security-scan.sh")
		if err != nil {
			return err
		}
		re := regexp.MustCompile(`(?m)^` + name + `="[^"@]+:[^"@]+@sha256:[0-9a-f]{64}"$`)
		if !re.MatchString(text) {
			return fmt.Errorf("%s is not pinned as name:tag@sha256:<64 hex>", name)
		}
		return nil
	})

	// ---- the build container (features/s3-build-container.feature)
	dockerGo := func(rel string) (string, error) {
		text, err := w.read(rel)
		if err != nil {
			return "", err
		}
		m := regexp.MustCompile(`(?m)^FROM golang:(\d+\.\d+\.\d+)-[a-z0-9]+@sha256:[0-9a-f]{64}(?:\s+AS\s+\w+)?\s*$`).FindStringSubmatch(text)
		if m == nil {
			return "", fmt.Errorf("%s has no line FROM golang:<version>-<variant>@sha256:<64 hex>", rel)
		}
		return m[1], nil
	}
	sc.Step(`^the base image of "([^"]+)" is pinned by digest$`, func(rel string) error {
		_, err := dockerGo(rel)
		return err
	})
	sc.Step(`^the Go version of the base image of "([^"]+)" equals the go line of "([^"]+)"$`, func(rel, gomod string) error {
		v, err := dockerGo(rel)
		if err != nil {
			return err
		}
		mod, err := w.read(gomod)
		if err != nil {
			return err
		}
		m := regexp.MustCompile(`(?m)^go (\d+\.\d+\.\d+)$`).FindStringSubmatch(mod)
		if m == nil {
			return fmt.Errorf("%s has no go line with a patch version", gomod)
		}
		if m[1] != v {
			return fmt.Errorf("%s is based on Go %s, %s requires Go %s", rel, v, gomod, m[1])
		}
		return nil
	})
	sc.Step(`^the Go version of the base image of "([^"]+)" equals the one "([^"]+)" requires$`, func(rel, script string) error {
		v, err := dockerGo(rel)
		if err != nil {
			return err
		}
		text, err := w.read(script)
		if err != nil {
			return err
		}
		m := regexp.MustCompile(`REQUIRED_GO_VERSION="\$\{MCD_GO_VERSION:-go(\d+\.\d+\.\d+)\}"`).FindStringSubmatch(text)
		if m == nil {
			return fmt.Errorf("%s names no required Go version", script)
		}
		if m[1] != v {
			return fmt.Errorf("%s is based on Go %s, %s requires Go %s", rel, v, script, m[1])
		}
		return nil
	})
	sc.Step(`^the security gate Go image has the same minor version as "([^"]+)"$`, func(gomod string) error {
		script, err := w.read("scripts/security-scan.sh")
		if err != nil {
			return err
		}
		mod, err := w.read(gomod)
		if err != nil {
			return err
		}
		img := regexp.MustCompile(`(?m)^GO_IMAGE="golang:(\d+\.\d+)[.\-"]`).FindStringSubmatch(script)
		if img == nil {
			return errors.New("cannot read the Go version of GO_IMAGE")
		}
		line := regexp.MustCompile(`(?m)^go (\d+\.\d+)`).FindStringSubmatch(mod)
		if line == nil {
			return fmt.Errorf("cannot read the go line of %s", gomod)
		}
		if img[1] != line[1] {
			return fmt.Errorf("GO_IMAGE is Go %s, %s requires Go %s", img[1], gomod, line[1])
		}
		return nil
	})

	sc.Step(`^the security gate runs with ([A-Z_]+) set to "([^"]*)"$`, func(variable, value string) error {
		cmd := exec.Command("bash", "scripts/security-scan.sh")
		cmd.Dir = w.root
		cmd.Env = append(os.Environ(), variable+"="+value)
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		err := cmd.Run()
		w.output = buf.String()
		var ee *exec.ExitError
		switch {
		case err == nil:
			w.exitCode = 0
		case errors.As(err, &ee):
			w.exitCode = ee.ExitCode()
		default:
			return fmt.Errorf("run scripts/security-scan.sh: %w", err)
		}
		return nil
	})

	sc.Step(`^the security gate exits with status (\d+)$`, func(want int) error {
		if w.exitCode != want {
			return fmt.Errorf("exit status %d, want %d; output:\n%s", w.exitCode, want, w.output)
		}
		return nil
	})

	sc.Step(`^the security gate output mentions "([^"]+)"$`, func(s string) error {
		if !strings.Contains(w.output, s) {
			return fmt.Errorf("output does not mention %q:\n%s", s, w.output)
		}
		return nil
	})

	sc.Step(`^the workflow "([^"]+)" runs "([^"]+)"$`, func(rel, cmd string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		if !regexp.MustCompile(`(?m)^\s+run:\s+` + regexp.QuoteMeta(cmd) + `\s*$`).MatchString(text) {
			return fmt.Errorf("%s has no step running %q", rel, cmd)
		}
		return nil
	})

	sc.Step(`^the workflow "([^"]+)" is triggered by "([^"]+)"$`, func(rel, event string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		on := text
		if i := strings.Index(text, "\njobs:"); i >= 0 {
			on = text[:i]
		}
		if !regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(event) + `:`).MatchString(on) {
			return fmt.Errorf("%s is not triggered by %s", rel, event)
		}
		return nil
	})

	sc.Step(`^the workflow "([^"]+)" has a job timeout$`, func(rel string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		if !regexp.MustCompile(`(?m)^    timeout-minutes:\s+\d+`).MatchString(text) {
			return fmt.Errorf("%s sets no timeout-minutes on its job", rel)
		}
		return nil
	})

	sc.Step(`^the workflow "([^"]+)" uploads SARIF only when "([^"]+)" is "([^"]+)"$`, func(rel, name, val string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		uploads := strings.Count(text, "upload-sarif@")
		gated := strings.Count(text, "vars."+name+" == '"+val+"'")
		if uploads == 0 {
			return fmt.Errorf("%s uploads no SARIF", rel)
		}
		if gated != uploads {
			return fmt.Errorf("%s has %d SARIF uploads but %d of them gated on vars.%s == '%s'", rel, uploads, gated, name, val)
		}
		return nil
	})

	sc.Step(`^no text file of "([^"]+)" matches "([^"]+)"$`, func(dir, pattern string) error {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return err
		}
		var hits []string
		root := filepath.Join(w.root, filepath.FromSlash(dir))
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "bin" || d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil || bytes.IndexByte(b, 0) >= 0 { // unreadable or binary
				return nil
			}
			if re.Match(b) {
				rel, _ := filepath.Rel(w.root, p)
				hits = append(hits, rel)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if len(hits) > 0 {
			return fmt.Errorf("%d file(s) of %s match %q: %v", len(hits), dir, pattern, hits[:min(len(hits), 5)])
		}
		return nil
	})
	sc.Step(`^"([^"]+)" is the same file as "([^"]+)"$`, func(a, b string) error {
		ta, err := w.read(a)
		if err != nil {
			return err
		}
		tb, err := w.read(b)
		if err != nil {
			return err
		}
		if ta != tb {
			return fmt.Errorf("%s and %s differ", a, b)
		}
		return nil
	})
	sc.Step(`^the repository file "([^"]+)" documents every SEC_ setting of the plugin's scan script$`, func(guide string) error {
		script, err := w.read("model-check-plugin/scripts/security-scan.sh")
		if err != nil {
			return err
		}
		doc, err := w.read(guide)
		if err != nil {
			return err
		}
		for _, m := range regexp.MustCompile(`\bSEC_[A-Z_]+\b`).FindAllString(script, -1) {
			if !strings.Contains(doc, m) {
				return fmt.Errorf("%s does not document %s", guide, m)
			}
		}
		return nil
	})
	sc.Step(`^the workflow "([^"]+)" does not build over the tracked "([^"]+)"$`, func(rel, target string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		if strings.Contains(text, "-o "+target) {
			return fmt.Errorf("%s builds over %s, the tracked file of the release (go refuses to overwrite a wrapper script)", rel, target)
		}
		return nil
	})
	sc.Step(`^the workflow "([^"]+)" lets every SARIF upload fail without failing the job$`, func(rel string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		uploads := strings.Count(text, "upload-sarif@")
		tolerant := len(regexp.MustCompile(`(?m)^\s+continue-on-error:\s+true\b`).FindAllString(text, -1))
		if uploads == 0 || tolerant != uploads {
			return fmt.Errorf("%s has %d SARIF uploads and %d steps with continue-on-error: true", rel, uploads, tolerant)
		}
		return nil
	})
	sc.Step(`^the workflow "([^"]+)" asks SPIN for its version with -V$`, func(rel string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		if !regexp.MustCompile(`(?m)^\s+spin -V\s*$`).MatchString(text) || strings.Contains(text, "spin --version") {
			return fmt.Errorf("%s must run `spin -V` (SPIN 6.5.2 has no --version) and not `spin --version`", rel)
		}
		return nil
	})
	sc.Step(`^the workflow "([^"]+)" skips the SARIF upload for fork pull requests$`, func(rel string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		uploads := strings.Count(text, "upload-sarif@")
		guards := strings.Count(text, "github.event.pull_request.head.repo.full_name != github.repository")
		if uploads == 0 || guards != uploads {
			return fmt.Errorf("%s has %d SARIF uploads and %d fork guards", rel, uploads, guards)
		}
		return nil
	})

	excludesNone := func(rel string, names []string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(text, "\n") {
			entry := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
			entry = strings.Trim(entry, `'"`)
			if entry == "" || strings.HasPrefix(entry, "#") {
				continue
			}
			for _, n := range names {
				if entry == n || strings.HasPrefix(strings.TrimPrefix(entry, "**/"), n) {
					return fmt.Errorf("%s excludes product path %q", rel, entry)
				}
			}
		}
		return nil
	}
	sc.Step(`^"([^"]+)" excludes none of ("[^"]+"(?: "[^"]+")*)$`, func(rel, list string) error {
		var names []string
		for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(list, -1) {
			names = append(names, m[1])
		}
		return excludesNone(rel, names)
	})

	sc.Step(`^the bodies of "([^"]+)" and "([^"]+)" are identical$`, func(a, b string) error {
		ta, err := w.read(a)
		if err != nil {
			return err
		}
		tb, err := w.read(b)
		if err != nil {
			return err
		}
		if s1Body(ta) != s1Body(tb) {
			return fmt.Errorf("the bodies of %s and %s differ", a, b)
		}
		return nil
	})

	sc.Step(`^the repository file "([^"]+)" mentions "([^"]+)"$`, func(rel, s string) error {
		text, err := w.read(rel)
		if err != nil {
			return err
		}
		if !strings.Contains(text, s) {
			return fmt.Errorf("%s does not mention %q", rel, s)
		}
		return nil
	})

	sc.Step(`^"([^"]+)" documents every SEC_ setting of the security gate$`, func(guide string) error {
		script, err := w.read("scripts/security-scan.sh")
		if err != nil {
			return err
		}
		doc, err := w.read(guide)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, m := range regexp.MustCompile(`\bSEC_[A-Z_]+\b`).FindAllString(script, -1) {
			if seen[m] {
				continue
			}
			seen[m] = true
			if !strings.Contains(doc, m) {
				return fmt.Errorf("%s does not document %s", guide, m)
			}
		}
		if len(seen) == 0 {
			return errors.New("scripts/security-scan.sh has no SEC_ settings")
		}
		return nil
	})
}
