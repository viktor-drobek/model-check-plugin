package impact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const defaultGitOutputBytes = 8 << 20
const defaultGitTimeout = 30 * time.Second
const gitWaitDelay = 250 * time.Millisecond

var errGitProcessTreeUnsupported = errors.New("impact: Git process-tree containment is unsupported on this platform")

type gitProcessLifecycle struct {
	mu      sync.Mutex
	reaping bool
	reaped  bool
}

func (lifecycle *gitProcessLifecycle) wait(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	observeErr := waitForGitProcessExit(command.Process.Pid)
	lifecycle.mu.Lock()
	lifecycle.reaping = true
	terminationErr := terminateGitProcessTreeLocked(command)
	lifecycle.mu.Unlock()
	err := command.Wait()
	lifecycle.mu.Lock()
	lifecycle.reaped = true
	lifecycle.reaping = false
	lifecycle.mu.Unlock()
	return errors.Join(observeErr, terminationErr, err)
}

var fullRevisionRE = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var rawModeRE = regexp.MustCompile(`^[0-7]{6}$`)
var rawObjectIDRE = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var gitDiagnosticPathRE = regexp.MustCompile(`(?:[A-Za-z]:[\\/]|/)[^\s]+`)

// GitChange is one deletion, addition, or modification reported by a pinned
// Git diff. A path rename is intentionally rejected rather than matched by
// name similarity; callers must model it as deletion plus addition.
type GitChange struct {
	Status  byte
	Path    string
	OldMode string
	NewMode string
	OldID   string
	NewID   string
}

// PathMapping is an explicit, exact association from one Git path to an
// artifact replacement identity. The mapping is caller-supplied because Git
// paths cannot establish artifact continuity by themselves.
type PathMapping struct {
	Path   string
	Change Change
}

// ChangesFromGit converts parsed path changes into analyzer changes using only
// exact path mappings. Every changed path must have exactly one mapping. A
// deletion removes its mapped artifact; an addition or modification needs the
// replacement immutable identity supplied in the mapping.
func ChangesFromGit(changes []GitChange, mappings []PathMapping) ([]Change, error) {
	byPath := make(map[string]Change, len(mappings))
	for _, mapping := range mappings {
		if mapping.Path == "" || mapping.Change.ArtifactID == "" {
			return nil, fmt.Errorf("impact: path mapping needs exact path and artifact ID")
		}
		if _, exists := byPath[mapping.Path]; exists {
			return nil, fmt.Errorf("impact: ambiguous multiple mappings for path %q", mapping.Path)
		}
		byPath[mapping.Path] = mapping.Change
	}
	result := make([]Change, 0, len(changes))
	seenArtifacts := make(map[string]bool, len(changes))
	for _, gitChange := range changes {
		change, ok := byPath[gitChange.Path]
		if !ok {
			return nil, fmt.Errorf("impact: changed path %q has no exact artifact mapping", gitChange.Path)
		}
		if seenArtifacts[change.ArtifactID] {
			return nil, fmt.Errorf("impact: ambiguous Git mapping changes artifact %q more than once", change.ArtifactID)
		}
		seenArtifacts[change.ArtifactID] = true
		switch gitChange.Status {
		case 'D':
			if !change.Removed || change.Revision != "" || change.ContractVersion != "" || change.Toolchain != "" {
				return nil, fmt.Errorf("impact: deletion mapping for path %q must be explicitly removed", gitChange.Path)
			}
			change.Removed = true
			change.Revision = ""
			change.ContractVersion = ""
			change.Toolchain = ""
		case 'A', 'M':
			if change.Removed || change.Revision == "" {
				return nil, fmt.Errorf("impact: changed path %q needs a replacement immutable artifact identity", gitChange.Path)
			}
		default:
			return nil, fmt.Errorf("impact: unsupported parsed Git status %q", gitChange.Status)
		}
		result = append(result, change)
	}
	return result, nil
}

// GitOptions bounds the wrapper's captured output. Zero selects a conservative
// default; values below one byte are rejected.
type GitOptions struct {
	MaxOutputBytes int
}

// GitChanges obtains changes between two full immutable revisions in a clean
// repository. It does not accept refs, paths, command fragments, or rename
// inference, and it rejects submodule changes.
func GitChanges(ctx context.Context, repository, oldRevision, newRevision string, options GitOptions) ([]GitChange, error) {
	if ctx == nil {
		return nil, fmt.Errorf("impact: Git requires a non-nil context")
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultGitTimeout)
		defer cancel()
	}
	if !fullRevisionRE.MatchString(oldRevision) || !fullRevisionRE.MatchString(newRevision) || oldRevision == newRevision {
		return nil, fmt.Errorf("impact: Git requires two distinct full immutable revision IDs")
	}
	repository, err := canonicalGitRepository(repository)
	if err != nil {
		return nil, err
	}
	limit, err := gitOutputLimit(options)
	if err != nil {
		return nil, err
	}
	if err := ensureGitConfigSafe(ctx, repository, limit); err != nil {
		return nil, err
	}
	if err := verifyImmutableRevision(ctx, repository, oldRevision, limit); err != nil {
		return nil, err
	}
	if err := verifyImmutableRevision(ctx, repository, newRevision, limit); err != nil {
		return nil, err
	}
	if err := ensureNoSubmodules(ctx, repository, oldRevision, limit); err != nil {
		return nil, err
	}
	if err := ensureNoSubmodules(ctx, repository, newRevision, limit); err != nil {
		return nil, err
	}
	if err := ensureNoSubmodules(ctx, repository, "HEAD", limit); err != nil {
		return nil, err
	}
	if err := ensureNoIndexSubmodules(ctx, repository, limit); err != nil {
		return nil, err
	}
	if err := ensureClean(ctx, repository, limit); err != nil {
		return nil, err
	}
	stdout, stderr, err := runGit(ctx, repository, limit, "-c", "diff.external=", "-c", "diff.trustExitCode=false", "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=all", "--no-relative", "--no-abbrev", "--raw", "-z", oldRevision, newRevision, "--")
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("impact: git diff failed: %w%s", err, gitStderrSuffix(stderr))
	}
	return ParseDiff(stdout)
}

func verifyImmutableRevision(ctx context.Context, repository, revision string, limit int) error {
	stdout, stderr, err := runGit(ctx, repository, limit, "rev-parse", "--verify", "--quiet", revision+"^{commit}")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("impact: Git revision %q is not an immutable commit%s", revision, gitStderrSuffix(stderr))
	}
	if strings.TrimSpace(string(stdout)) != revision {
		return fmt.Errorf("impact: Git revision %q did not resolve exactly", revision)
	}
	return nil
}

func gitOutputLimit(options GitOptions) (int, error) {
	if options.MaxOutputBytes == 0 {
		return defaultGitOutputBytes, nil
	}
	if options.MaxOutputBytes < 1 {
		return 0, fmt.Errorf("impact: Git output limit must be positive")
	}
	return options.MaxOutputBytes, nil
}

func ensureClean(ctx context.Context, repository string, limit int) error {
	stdout, stderr, err := runGit(ctx, repository, limit, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("impact: git status failed: %w%s", err, gitStderrSuffix(stderr))
	}
	if err := ensureNoHiddenIndexState(ctx, repository, limit); err != nil {
		return err
	}
	if len(stdout) != 0 {
		return fmt.Errorf("impact: Git repository is dirty (including untracked files)")
	}
	return nil
}

func ensureNoHiddenIndexState(ctx context.Context, repository string, limit int) error {
	stdout, stderr, err := runGit(ctx, repository, limit, "ls-files", "-v", "-z")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("impact: Git index-state preflight failed (the complete ls-files listing is capped separately): %w%s", err, gitStderrSuffix(stderr))
	}
	for len(stdout) > 0 {
		record, rest, ok := cutNUL(stdout)
		if !ok || len(record) < 2 || record[1] != ' ' {
			return fmt.Errorf("impact: malformed Git index state listing")
		}
		tag := record[0]
		if tag != 'H' {
			return fmt.Errorf("impact: Git index state hides tracked file")
		}
		stdout = rest
	}
	return nil
}

func ensureGitConfigSafe(ctx context.Context, repository string, limit int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	gitDirectory := filepath.Join(repository, ".git")
	for _, name := range []string{"config", "config.worktree"} {
		path := filepath.Join(gitDirectory, name)
		if err := inspectGitConfig(path, limit); err != nil {
			if errors.Is(err, os.ErrNotExist) && name == "config.worktree" {
				continue
			}
			return err
		}
	}
	return nil
}

func canonicalGitRepository(repository string) (string, error) {
	if repository == "" || strings.HasPrefix(repository, "-") {
		return "", fmt.Errorf("impact: Git repository must be an exact directory path")
	}
	abs, err := filepath.Abs(repository)
	if err != nil {
		return "", fmt.Errorf("impact: resolve Git repository path: %w", err)
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("impact: resolve Git repository symlinks: %w", safePathError("repository", err))
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("impact: inspect Git repository: %w", safePathError("repository", err))
	}
	if !info.IsDir() {
		return "", fmt.Errorf("impact: Git repository must be a directory")
	}
	gitDirectory := filepath.Join(root, ".git")
	gitInfo, err := os.Lstat(gitDirectory)
	if err != nil {
		return "", fmt.Errorf("impact: Git repository root is required: %w", safePathError(".git", err))
	}
	if gitInfo.Mode()&os.ModeSymlink != 0 || !gitInfo.IsDir() {
		return "", fmt.Errorf("impact: linked and worktree Git directories are unsupported")
	}
	for _, name := range []string{"commondir", "gitdir"} {
		if _, err := os.Lstat(filepath.Join(gitDirectory, name)); err == nil {
			return "", fmt.Errorf("impact: Git directory metadata %q is unsupported", name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("impact: inspect Git directory metadata %q: %w", name, safePathError(filepath.Join(".git", name), err))
		}
	}
	for _, name := range []string{"objects", "refs", "info", "logs"} {
		path := filepath.Join(gitDirectory, name)
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("impact: inspect Git metadata directory %q: %w", name, safePathError(filepath.Join(".git", name), err))
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("impact: Git metadata directory %q must be a real directory", name)
		}
	}
	for _, metadata := range []struct {
		name string
		path string
	}{
		{"HEAD", filepath.Join(gitDirectory, "HEAD")},
		{"index", filepath.Join(gitDirectory, "index")},
		{"packed-refs", filepath.Join(gitDirectory, "packed-refs")},
		{"info/exclude", filepath.Join(gitDirectory, "info", "exclude")},
		{"info/attributes", filepath.Join(gitDirectory, "info", "attributes")},
	} {
		path := metadata.path
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("impact: inspect Git metadata file %q: %w", metadata.name, safePathError(filepath.Join(".git", metadata.name), err))
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("impact: Git metadata file %q must be a regular non-symlink file", metadata.name)
		}
	}
	alternates := filepath.Join(gitDirectory, "objects", "info", "alternates")
	if info, err := os.Lstat(alternates); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("impact: Git alternate object metadata must be a regular file")
		}
		return "", fmt.Errorf("impact: Git alternate object stores are unsupported")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("impact: inspect Git alternate object metadata: %w", safePathError(filepath.Join(".git", "objects", "info", "alternates"), err))
	}
	return root, nil
}

func safePathError(label string, err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return &os.PathError{Op: pathErr.Op, Path: label, Err: pathErr.Err}
	}
	return err
}

func inspectGitConfig(path string, limit int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("impact: inspect repository Git configuration: %w", safePathError(filepath.Join(".git", filepath.Base(path)), err))
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("impact: repository Git configuration must be a regular file")
	}
	if info.Size() > int64(limit) {
		return fmt.Errorf("impact: repository Git configuration exceeded %d-byte limit", limit)
	}
	file, err := openGitConfig(path, info)
	if err != nil {
		return fmt.Errorf("impact: open repository Git configuration: %w", safePathError(filepath.Join(".git", filepath.Base(path)), err))
	}
	data, readErr := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		return fmt.Errorf("impact: read repository Git configuration: %w", safePathError(filepath.Join(".git", filepath.Base(path)), err))
	}
	if len(data) > limit {
		return fmt.Errorf("impact: repository Git configuration exceeded %d-byte limit", limit)
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasSuffix(strings.TrimSpace(line), "\\") {
			return fmt.Errorf("impact: repository Git configuration continuation is unsupported")
		}
	}
	section := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			closing := strings.IndexByte(line, ']')
			if closing < 0 {
				return fmt.Errorf("impact: malformed repository Git configuration")
			}
			if strings.TrimSpace(line[closing+1:]) != "" {
				return fmt.Errorf("impact: repository Git configuration contains text after section header")
			}
			section = normalizeGitConfigName(line[1:closing])
			section, _, _ = strings.Cut(section, ".")
			if section == "" {
				return fmt.Errorf("impact: malformed repository Git configuration")
			}
			continue
		}
		key, _, found := strings.Cut(line, "=")
		if !found || section == "" {
			return fmt.Errorf("impact: malformed repository Git configuration")
		}
		if unsafeGitConfigKey(section, normalizeGitConfigName(key)) {
			return fmt.Errorf("impact: repository Git configuration contains unsafe control %q", section+"."+normalizeGitConfigName(key))
		}
	}
	return nil
}

func normalizeGitConfigName(value string) string {
	value = strings.TrimSpace(value)
	if boundary := strings.IndexAny(value, " \t\""); boundary >= 0 {
		value = value[:boundary]
	}
	return strings.ToLower(value)
}

func unsafeGitConfigKey(section, key string) bool {
	if section == "include" || section == "includeif" || section == "core" && (key == "fsmonitor" || key == "worktree" || key == "excludesfile" || key == "attributesfile" || key == "ignorestat" || key == "sparsecheckout" || key == "sparsecheckoutcone") || section == "diff" && (key == "external" || key == "textconv") || section == "extensions" && (key == "worktreeconfig" || key == "sparseindex") || section == "filter" && (key == "clean" || key == "process") {
		return true
	}
	return strings.Contains(section, "promisor") || strings.Contains(key, "promisor") || strings.Contains(section, "partialclone") || strings.Contains(key, "partialclone")
}

func ensureNoSubmodules(ctx context.Context, repository, revision string, limit int) error {
	stdout, stderr, err := runGit(ctx, repository, limit, "ls-tree", "-r", "-z", "--full-tree", revision)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("impact: Git tree preflight failed (the complete ls-tree listing is capped separately): %w%s", err, gitStderrSuffix(stderr))
	}
	for len(stdout) > 0 {
		record, rest, ok := cutNUL(stdout)
		if !ok {
			return fmt.Errorf("impact: malformed Git tree listing")
		}
		metadata, _, found := bytes.Cut(record, []byte{'\t'})
		if !found {
			return fmt.Errorf("impact: malformed Git tree listing")
		}
		if bytes.HasPrefix(metadata, []byte("160000 ")) {
			return fmt.Errorf("impact: Git submodules are unsupported")
		}
		stdout = rest
	}
	return nil
}

func ensureNoIndexSubmodules(ctx context.Context, repository string, limit int) error {
	stdout, stderr, err := runGit(ctx, repository, limit, "ls-files", "--stage", "-z")
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("impact: Git index-submodule preflight failed (the complete ls-files listing is capped separately): %w%s", err, gitStderrSuffix(stderr))
	}
	for len(stdout) > 0 {
		record, rest, ok := cutNUL(stdout)
		if !ok {
			return fmt.Errorf("impact: malformed Git index listing")
		}
		if bytes.HasPrefix(record, []byte("160000 ")) {
			return fmt.Errorf("impact: Git submodules are unsupported")
		}
		stdout = rest
	}
	return nil
}

// ParseDiff parses only Git's --raw -z output from a no-renames invocation.
// It never splits a path on whitespace or line endings.
func ParseDiff(data []byte) ([]GitChange, error) {
	changes := make([]GitChange, 0)
	for len(data) > 0 {
		metadata, rest, ok := cutNUL(data)
		if !ok {
			return nil, fmt.Errorf("impact: malformed Git raw diff: metadata lacks NUL delimiter")
		}
		path, rest, ok := cutNUL(rest)
		if !ok {
			return nil, fmt.Errorf("impact: malformed Git raw diff: path lacks NUL delimiter")
		}
		change, err := parseRawRecord(metadata, path)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
		data = rest
	}
	return changes, nil
}

func cutNUL(data []byte) ([]byte, []byte, bool) {
	index := bytes.IndexByte(data, 0)
	if index < 0 {
		return nil, nil, false
	}
	return data[:index], data[index+1:], true
}

func parseRawRecord(metadata, path []byte) (GitChange, error) {
	if len(path) == 0 {
		return GitChange{}, fmt.Errorf("impact: malformed Git raw diff: blank path")
	}
	parts := strings.Split(string(metadata), " ")
	if len(parts) != 5 || !strings.HasPrefix(parts[0], ":") {
		return GitChange{}, fmt.Errorf("impact: malformed Git raw diff metadata")
	}
	oldMode, newMode := strings.TrimPrefix(parts[0], ":"), parts[1]
	if !rawModeRE.MatchString(oldMode) || !rawModeRE.MatchString(newMode) {
		return GitChange{}, fmt.Errorf("impact: malformed Git raw diff mode")
	}
	if !rawObjectIDRE.MatchString(parts[2]) || !rawObjectIDRE.MatchString(parts[3]) {
		return GitChange{}, fmt.Errorf("impact: malformed Git raw diff object ID")
	}
	if oldMode == "160000" || newMode == "160000" {
		return GitChange{}, fmt.Errorf("impact: Git submodule change is unsupported")
	}
	if len(parts[4]) != 1 {
		return GitChange{}, fmt.Errorf("impact: Git rename or copy metadata is unsupported")
	}
	status := parts[4][0]
	if status != 'A' && status != 'M' && status != 'D' {
		return GitChange{}, fmt.Errorf("impact: Git change status %q is unsupported", parts[4])
	}
	return GitChange{Status: status, Path: string(path), OldMode: oldMode, NewMode: newMode, OldID: parts[2], NewID: parts[3]}, nil
}

func gitStderrSuffix(stderr []byte) string {
	if len(stderr) == 0 {
		return ""
	}
	const maxDiagnosticBytes = 512
	diagnostic := sanitizeGitDiagnostic(stderr)
	if len(diagnostic) > maxDiagnosticBytes {
		end := maxDiagnosticBytes - len("...")
		for !utf8.RuneStart(diagnostic[end]) {
			end--
		}
		diagnostic = diagnostic[:end] + "..."
	}
	if diagnostic == "" {
		return ""
	}
	return ": " + diagnostic
}

func sanitizeGitDiagnostic(stderr []byte) string {
	lines := strings.Split(strings.TrimSpace(string(stderr)), "\n")
	for i, line := range lines {
		line = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || r == '\x7f' {
				return ' '
			}
			return r
		}, line)
		if gitDiagnosticPathRE.MatchString(line) {
			line = "<path-bearing diagnostic>"
		}
		lines[i] = line
	}
	return strings.Join(lines, " ")
}

var runGit = func(ctx context.Context, repository string, limit int, args ...string) (output []byte, stderrOutput []byte, resultErr error) {
	gitArgs := append([]string{"--git-dir=" + filepath.Join(repository, ".git"), "--work-tree=" + repository, "--no-lazy-fetch", "--no-replace-objects", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.fileMode=true", "-c", "core.excludesFile=" + os.DevNull, "-c", "core.attributesFile=" + os.DevNull}, args...)
	command := exec.Command("git", gitArgs...)
	command.WaitDelay = gitWaitDelay
	command.Dir = repository
	command.Env = safeGitEnvironment(os.Environ())
	lifecycle := &gitProcessLifecycle{}
	if err := configureGitProcess(command); err != nil {
		return nil, nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	var stderr limitedBuffer
	stderr.limit = limit
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return nil, nil, err
	}
	terminate := func(err error) error {
		return errors.Join(err, terminateGitProcessTree(command, lifecycle))
	}
	waited := false
	waitStarted := false
	var waitResults chan error
	defer func() {
		var cleanupErr error
		if waitStarted {
			if !waited {
				cleanupErr = <-waitResults
				waited = true
			}
		} else {
			waitErr := lifecycle.wait(command)
			waited = true
			cleanupErr = waitErr
		}
		_ = stdout.Close()
		if cleanupErr != nil {
			if resultErr == nil {
				resultErr = cleanupErr
			} else {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
		if contextErr := ctx.Err(); contextErr != nil {
			if resultErr == nil {
				resultErr = contextErr
			} else if !errors.Is(resultErr, contextErr) {
				resultErr = errors.Join(contextErr, resultErr)
			}
		}
		stderrOutput = stderr.Bytes()
	}()
	type readResult struct {
		output []byte
		err    error
	}
	readResults := make(chan readResult, 1)
	go func() {
		output, err := readBounded(stdout, limit)
		readResults <- readResult{output: output, err: err}
	}()

	var result readResult
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case result = <-readResults:
			goto readComplete
		case <-ctx.Done():
			return nil, nil, terminate(ctx.Err())
		case <-ticker.C:
			if stderr.Exceeded() {
				return nil, nil, terminate(fmt.Errorf("impact: Git stderr exceeded %d-byte limit", limit))
			}
		}
	}

readComplete:
	if result.err != nil || stderr.Exceeded() {
		if result.err != nil {
			return nil, nil, terminate(result.err)
		}
		return nil, nil, terminate(fmt.Errorf("impact: Git stderr exceeded %d-byte limit", limit))
	}
	waitStarted = true
	waitResults = make(chan error, 1)
	go func() {
		waitErr := lifecycle.wait(command)
		waitResults <- waitErr
	}()
	for {
		select {
		case waitErr := <-waitResults:
			waited = true
			if stderr.Exceeded() {
				return nil, nil, fmt.Errorf("impact: Git stderr exceeded %d-byte limit", limit)
			}
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			if waitErr != nil {
				return nil, nil, waitErr
			}
			return result.output, nil, nil
		case <-ctx.Done():
			return nil, nil, terminate(ctx.Err())
		case <-ticker.C:
			if stderr.Exceeded() {
				return nil, nil, terminate(fmt.Errorf("impact: Git stderr exceeded %d-byte limit", limit))
			}
		}
	}
}

func safeGitEnvironment(environment []string) []string {
	out := make([]string, 0, len(environment)+5)
	for _, item := range environment {
		name, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(strings.ToUpper(name), "GIT_") {
			continue
		}
		upperName := strings.ToUpper(name)
		if upperName == "LANG" || upperName == "LANGUAGE" || strings.HasPrefix(upperName, "LC_") {
			continue
		}
		out = append(out, item)
	}
	out = append(out,
		"LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_NO_LAZY_FETCH=1",
	)
	return out
}

func readBounded(reader io.Reader, limit int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("impact: Git stdout exceeded %d-byte limit", limit)
	}
	return data, nil
}

type limitedBuffer struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if buffer.buffer.Len()+len(data) > buffer.limit {
		remaining := buffer.limit - buffer.buffer.Len()
		if remaining > 0 {
			_, _ = buffer.buffer.Write(data[:remaining])
		}
		buffer.exceeded = true
		return len(data), nil
	}
	return buffer.buffer.Write(data)
}

func (buffer *limitedBuffer) Bytes() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return append([]byte(nil), buffer.buffer.Bytes()...)
}

func (buffer *limitedBuffer) Exceeded() bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.exceeded
}
