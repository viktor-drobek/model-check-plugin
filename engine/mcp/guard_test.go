package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolve covers the four rules of Resolve, including the symlink
// escape (rule 3) and a symlink that stays inside (allowed).
func TestResolve(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "session")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{dir, outside, filepath.Join(dir, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link-out")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "sub"), filepath.Join(dir, "link-in")); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		rel  string
		ok   bool
		want string // suffix of the returned path when ok
	}{
		{"a.json", true, "session/a.json"},
		{"sub/b.json", true, "session/sub/b.json"},
		{"sub/new/deep/c.json", true, "session/sub/new/deep/c.json"}, // non-existing dirs
		{"link-in/d.json", true, "session/link-in/d.json"},           // symlink that stays inside
		{"", false, ""},
		{"../x.json", false, ""},
		{"sub/../../x.json", false, ""},
		{"..", false, ""},
		{filepath.Join(dir, "a.json"), false, ""}, // absolute, even if inside
		{"/tmp/x.json", false, ""},
		{"link-out/e.json", false, ""},        // symlink escape
		{"link-out/deep/f.json", false, ""},   // symlink escape, deeper
		{"sub/../link-out/g.json", false, ""}, // lexical clean then symlink escape
	}
	for _, c := range cases {
		got, err := Resolve(dir, c.rel)
		if c.ok {
			if err != nil {
				t.Errorf("Resolve(%q): unexpected error %v", c.rel, err)
				continue
			}
			if !strings.HasSuffix(got, c.want) {
				t.Errorf("Resolve(%q) = %q, want suffix %q", c.rel, got, c.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("Resolve(%q) = %q, want refusal", c.rel, got)
		} else if !strings.Contains(err.Error(), "refused") {
			t.Errorf("Resolve(%q): error %q does not say refused", c.rel, err)
		}
	}
	if _, err := Resolve("relative/dir", "a"); err == nil {
		t.Error("relative session dir accepted")
	}
}

// TestWriteFileRefusesEscape: the single writer never creates a file outside
// the session directory, whatever the path form.
func TestWriteFileRefusesEscape(t *testing.T) {
	base := t.TempDir()
	ss, err := NewSessions(base, false, ServerParams{})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := ss.New()
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "elsewhere")
	os.MkdirAll(outside, 0o755)
	os.Symlink(outside, filepath.Join(sess.Dir, "link"))
	for _, rel := range []string{"../escape.json", "link/escape.json", "/tmp/escape.json", "a/../../escape.json"} {
		if _, err := sess.WriteFile(rel, []byte("x")); err == nil {
			t.Errorf("WriteFile(%q) succeeded", rel)
		} else if !strings.Contains(err.Error(), "session directory") {
			t.Errorf("WriteFile(%q): %v does not name the session directory", rel, err)
		}
	}
	found := false
	filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Name() == "escape.json" {
			found = true
		}
		return nil
	})
	if found {
		t.Error("an escape.json was written somewhere under the base")
	}
	if p, err := sess.WriteFile("ok/inside.json", []byte("y")); err != nil || !strings.HasPrefix(p, sess.Dir) {
		t.Errorf("WriteFile inside: %q %v", p, err)
	}
}

// TestSessionsUniqueAndCleanup: ids are distinct, directories exist, and
// cleanup is opt-in.
func TestSessionsUniqueAndCleanup(t *testing.T) {
	base := t.TempDir()
	ss, _ := NewSessions(base, true, ServerParams{})
	a, _ := ss.New()
	b, _ := ss.New()
	if a.ID == b.ID {
		t.Fatal("duplicate session id")
	}
	if !fileExists(a.Dir) || !fileExists(b.Dir) {
		t.Fatal("session directories missing")
	}
	if _, err := ss.Get("nope"); err == nil || !strings.Contains(err.Error(), "unknown session") {
		t.Errorf("Get unknown: %v", err)
	}
	ss.Close()
	if fileExists(a.Dir) || fileExists(b.Dir) {
		t.Error("cleanup did not remove the session directories")
	}
	keep, _ := NewSessions(t.TempDir(), false, ServerParams{})
	c, _ := keep.New()
	keep.Close()
	if !fileExists(c.Dir) {
		t.Error("cleanup ran although it was not requested")
	}
}
