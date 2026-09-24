package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve maps a client-supplied relative path onto the session directory
// dir and refuses anything that would leave it (NFR-004). It is the only
// place where a path under the session directory is formed; every read and
// write of session files goes through it.
//
// Rules, in order:
//
//  1. rel must be non-empty and relative. An absolute path is refused even
//     if it happens to lie under dir, because the client is never meant to
//     know or choose absolute locations.
//  2. The lexical join filepath.Join(dir, rel) is cleaned, so `a/../../x`
//     is already `<parent of dir>/x` here — and refused by rule 4.
//  3. Symlinks are followed: the longest existing prefix of the joined path
//     is resolved with filepath.EvalSymlinks (a symlink inside the session
//     directory pointing outside is thereby exposed), the not-yet-existing
//     suffix is appended unchanged (it cannot contain a symlink).
//  4. filepath.Rel(realDir, realPath) must not start with "..": the resolved
//     target lies inside the resolved session directory.
//
// The returned path is the lexical join (inside dir), not the resolved one,
// so that files are created where the caller expects them; the check itself
// is on the resolved form.
func Resolve(dir, rel string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("session directory %q is not absolute", dir)
	}
	if rel == "" {
		return "", fmt.Errorf("refused: empty path is not inside the session directory")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("refused: %q is absolute; only paths inside the session directory are allowed", rel)
	}
	joined := filepath.Join(dir, rel)
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("session directory: %w", err)
	}
	realPath, err := resolveExisting(joined)
	if err != nil {
		return "", err
	}
	r, err := filepath.Rel(realDir, realPath)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refused: %q resolves outside the session directory", rel)
	}
	return joined, nil
}

// resolveExisting evaluates symlinks in the longest existing prefix of p and
// re-attaches the rest.
func resolveExisting(p string) (string, error) {
	existing := p
	var rest []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("refused: no existing ancestor for %q", p)
		}
		rest = append([]string{filepath.Base(existing)}, rest...)
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("refused: cannot resolve %q: %v", p, err)
	}
	return filepath.Join(append([]string{real}, rest...)...), nil
}
