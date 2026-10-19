package impact

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cucumber/godog"
)

func init() {
	mode := os.Getenv("MCD_GIT_HELPER")
	if mode == "" {
		return
	}
	if mode == "marker" {
		_ = os.WriteFile(os.Getenv("MCD_GIT_MARKER"), []byte("ran"), 0o600)
		os.Exit(0)
	}
	if mode == "descendant" {
		child := os.Getenv("MCD_GIT_HELPER_CHILD")
		if child == "1" {
			_ = os.WriteFile(os.Getenv("MCD_GIT_HELPER_PID"), []byte(fmt.Sprint(os.Getpid())), 0o600)
			time.Sleep(5 * time.Second)
			os.Exit(0)
		}
		command := exec.Command(os.Args[0])
		command.Env = append(os.Environ(), "MCD_GIT_HELPER_CHILD=1")
		_ = command.Start()
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
	if mode == "stdout-closed-descendant" {
		child := os.Getenv("MCD_GIT_HELPER_CHILD")
		if child == "1" {
			_ = os.WriteFile(os.Getenv("MCD_GIT_HELPER_PID"), []byte(fmt.Sprint(os.Getpid())), 0o600)
			time.Sleep(5 * time.Second)
			os.Exit(0)
		}
		command := exec.Command(os.Args[0])
		command.Env = append(os.Environ(), "MCD_GIT_HELPER_CHILD=1")
		if err := command.Start(); err != nil {
			os.Exit(1)
		}
		if err := os.Stdout.Close(); err != nil {
			os.Exit(1)
		}
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
	if mode == "stdout" {
		_, _ = os.Stdout.WriteString("0123456789abcdef")
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
	if mode == "preflight-stdout" {
		_, _ = os.Stdout.WriteString("0123456789abcdef")
		os.Exit(0)
	}
	if mode == "args" {
		_ = os.WriteFile(os.Getenv("MCD_GIT_MARKER"), []byte(strings.Join(os.Args[1:], "\x00")), 0o600)
		os.Exit(0)
	}
}

func TestGitDiffSecurityRejectsInheritedRepositoryEnvironment(t *testing.T) {
	environment := []string{
		"PATH=/usr/bin",
		"GIT_DIR=/attacker/repository",
		"GIT_WORK_TREE=/attacker/worktree",
		"GIT_INDEX_FILE=/attacker/index",
		"GIT_COMMON_DIR=/attacker/common",
		"GIT_OBJECT_DIRECTORY=/attacker/objects",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=/attacker/alternates",
		"GIT_QUARANTINE_PATH=/attacker/quarantine",
		"GIT_NAMESPACE=attacker",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.fsmonitor",
		"GIT_CONFIG_VALUE_0=attacker",
		"GIT_EXTERNAL_DIFF=/attacker/diff",
		"gIt_DiR=/attacker/mixed-case",
	}
	sanitized := safeGitEnvironment(environment)
	for _, item := range sanitized {
		name, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(strings.ToUpper(name), "GIT_") && name != "GIT_CONFIG_NOSYSTEM" && name != "GIT_CONFIG_GLOBAL" && name != "GIT_NO_REPLACE_OBJECTS" && name != "GIT_OPTIONAL_LOCKS" && name != "GIT_NO_LAZY_FETCH" && name != "GIT_ATTR_NOSYSTEM" {
			t.Fatalf("inherited Git variable survived sanitization: %q", item)
		}
	}
}

func TestGitDiffSecurityKillsChildOnStdoutLimit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	dir := t.TempDir()
	installGitHelper(t, dir, "stdout")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	_, _, err := runGit(ctx, t.TempDir(), 4, "status")
	if err == nil || !strings.Contains(err.Error(), "stdout exceeded") {
		t.Fatalf("runGit error = %v, want stdout limit error", err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("runGit took %s after exceeding output cap", elapsed)
	}
}

func TestGitDiffSecurityKillsDescendantAfterStdoutCloses(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	dir := t.TempDir()
	installGitHelper(t, dir, "stdout-closed-descendant")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("MCD_GIT_HELPER_PID", pidFile)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, _, err := runGit(ctx, t.TempDir(), 4096, "status")
		result <- err
	}()
	pid := readGitHelperPID(t, pidFile)
	cancel()
	err := <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runGit error = %v, want context canceled", err)
	}
	if !waitForGitHelperExit(pid) {
		t.Fatalf("descendant %d survived after parent closed stdout", pid)
	}
}

func TestGitDiffSecurityUsesCaseInsensitiveRoutingFilter(t *testing.T) {
	for _, item := range safeGitEnvironment([]string{"gIt_DiR=/attacker", "PATH=/usr/bin"}) {
		name, _, _ := strings.Cut(item, "=")
		if strings.EqualFold(name, "GIT_DIR") {
			t.Fatalf("mixed-case Git routing variable survived: %q", item)
		}
	}
}

func TestGitDiffSecurityDisablesLazyFetch(t *testing.T) {
	for _, item := range safeGitEnvironment([]string{"PATH=/usr/bin"}) {
		if item == "GIT_NO_LAZY_FETCH=1" {
			return
		}
	}
	t.Fatal("Git lazy fetch was not disabled")
}

func TestGitRunDisablesUntrackedCache(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	dir := t.TempDir()
	installGitHelper(t, dir, "args")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	marker := filepath.Join(t.TempDir(), "args")
	t.Setenv("MCD_GIT_MARKER", marker)
	if _, _, err := runGit(context.Background(), t.TempDir(), 4096, "status"); err != nil {
		t.Fatalf("runGit: %v", err)
	}
	args, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-c\x00core.untrackedCache=false") || !strings.Contains(string(args), "-c\x00core.fileMode=true") {
		t.Fatalf("runGit args = %q, want untracked cache disabled and file mode forced", args)
	}
}

func TestGitChangesRejectsPromisorConfiguration(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git configuration inspection is implemented on Linux only")
	}
	dir := initGitRepository(t)
	config := filepath.Join(dir, ".git", "config")
	if err := os.WriteFile(config, []byte("[remote \"origin\"]\n\tpromisor = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitConfigSafe(context.Background(), dir, 4096); err == nil {
		t.Fatal("promisor repository configuration was accepted")
	}
}

func TestGitChangesRejectsCaseInsensitiveWorktreeConfiguration(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git configuration inspection is implemented on Linux only")
	}
	dir := initGitRepository(t)
	config := filepath.Join(dir, ".git", "config.worktree")
	if err := os.WriteFile(config, []byte("[FILTER \"evil\"]\n\tprocess = helper\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitConfigSafe(context.Background(), dir, 4096); err == nil {
		t.Fatal("case-insensitive executable worktree configuration was accepted")
	}
}

func TestCanonicalGitRepositoryRequiresRootAndCanonicalizesSymlink(t *testing.T) {
	dir := initGitRepository(t)
	if _, err := canonicalGitRepository(filepath.Join(dir, "child")); err == nil || !strings.Contains(err.Error(), "resolve Git repository symlinks") {
		t.Fatalf("repository subdirectory error = %v, want root diagnostic", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("creating test symlinks requires Windows developer privileges")
	}
	link := filepath.Join(t.TempDir(), "repository")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	root, err := canonicalGitRepository(link)
	if err != nil {
		t.Fatalf("canonicalize symlinked repository: %v", err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root != want {
		t.Fatalf("canonical root = %q, want %q", root, want)
	}
}

func TestGitChangesRejectsSubmodulesBeforeStatus(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	dir := initGitRepository(t)
	original := runGit
	defer func() { runGit = original }()
	statusCalled := false
	runGit = func(_ context.Context, _ string, _ int, args ...string) ([]byte, []byte, error) {
		command := ""
		for _, arg := range args {
			if !strings.HasPrefix(arg, "-") {
				command = arg
				break
			}
		}
		switch command {
		case "rev-parse":
			return []byte(strings.TrimSuffix(args[len(args)-1], "^{commit}") + "\n"), nil, nil
		case "ls-tree":
			return []byte("160000 commit " + strings.Repeat("0", 40) + "\tmodule\x00"), nil, nil
		case "status":
			statusCalled = true
			return nil, nil, nil
		default:
			return nil, nil, fmt.Errorf("unexpected Git command %q", command)
		}
	}
	if _, err := GitChanges(context.Background(), dir, strings.Repeat("a", 40), strings.Repeat("b", 40), GitOptions{}); err == nil || !strings.Contains(err.Error(), "submodules") {
		t.Fatalf("GitChanges submodule error = %v", err)
	}
	if statusCalled {
		t.Fatal("Git status ran after a submodule was detected")
	}
}

func TestGitChangesRejectsRealIncludedConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git configuration inspection is implemented on Linux only")
	}
	dir := t.TempDir()
	if output, err := isolatedGitCommand(t, dir, "init", "--initial-branch=main"); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	config := filepath.Join(dir, ".git", "config")
	if err := os.WriteFile(config, []byte("[include]\n\tpath = evil.cfg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "evil.cfg"), []byte("[filter \"x\"]\n\tprocess = sh -c cat\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureGitConfigSafe(context.Background(), dir, 4096); err == nil {
		t.Fatal("real included configuration was accepted")
	}
}

func TestCanonicalGitRepositoryRejectsRedirectMetadata(t *testing.T) {
	cases := []struct {
		name string
		path string
		data string
	}{
		{name: "commondir", path: "commondir", data: "../outside\n"},
		{name: "gitdir", path: "gitdir", data: "../outside/.git\n"},
		{name: "alternates", path: filepath.Join("objects", "info", "alternates"), data: "/outside/objects\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := initGitRepository(t)
			path := filepath.Join(dir, ".git", tc.path)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := canonicalGitRepository(dir); err == nil {
				t.Fatalf("repository with %s metadata was accepted", tc.name)
			}
		})
	}
}

func TestGitChangesRejectsInvalidNestedGitDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	parent := initGitRepository(t)
	nested := filepath.Join(parent, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := GitChanges(context.Background(), nested, strings.Repeat("a", 40), strings.Repeat("b", 40), GitOptions{MaxOutputBytes: 4096}); err == nil || !strings.Contains(err.Error(), "Git repository root is required") {
		t.Fatalf("nested repository error = %v, want root diagnostic", err)
	}
}

func installGitHelper(t *testing.T, dir, mode string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCD_GIT_HELPER", mode)
	return path
}

func TestGitDiffSecurityRejectsMalformedRawMetadata(t *testing.T) {
	fullID := strings.Repeat("a", 40)
	if changes, err := ParseDiff([]byte(":" + "100644 100644 " + fullID + " " + strings.Repeat("b", 40) + " M\x00file.go\x00")); err != nil || len(changes) != 1 {
		t.Fatalf("ParseDiff rejected valid full-ID record: changes=%#v err=%v", changes, err)
	}
	cases := [][]byte{
		[]byte(":999999 100644 aaaaaaa bbbbbbb M\x00file.go\x00"),
		[]byte(":100644 100644 not-a-hash bbbbbbb M\x00file.go\x00"),
		[]byte(":100644 100644 a bbbbbbb M\x00file.go\x00"),
		[]byte(":100644 100644 aaaaaaa bbbbbbb M\x00"),
		[]byte(":100644 100644 " + fullID + " " + strings.Repeat("b", 40) + " R\x00file.go\x00new.go\x00"),
		[]byte(":100644 100644 " + fullID + " " + strings.Repeat("b", 40) + " C\x00file.go\x00new.go\x00"),
		[]byte(":100644 100644 " + fullID + " " + strings.Repeat("b", 40) + " T\x00file.go\x00"),
		[]byte(":160000 160000 " + fullID + " " + strings.Repeat("b", 40) + " M\x00submodule\x00"),
	}
	for _, input := range cases {
		if _, err := ParseDiff(input); err == nil {
			t.Fatalf("ParseDiff accepted malformed input %q", input)
		}
	}
}

func TestGitStderrSuffixIsSmallAndSanitized(t *testing.T) {
	stderr := []byte("fatal: /srv/private/repository/.git/config\x00\n" + strings.Repeat("x", 2048))
	suffix := gitStderrSuffix(stderr)
	if len(suffix) > 516 {
		t.Fatalf("stderr suffix length = %d, want at most 516", len(suffix))
	}
	if strings.Contains(suffix, "/srv/private") || strings.ContainsAny(suffix, "\x00") {
		t.Fatalf("stderr suffix leaked path/control data: %q", suffix)
	}
}

func TestGitStatusIgnoresCallerGlobalIgnoreFile(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	dir := initGitRepository(t)
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "git")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "ignore"), []byte("hidden.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := os.WriteFile(filepath.Join(dir, "hidden.txt"), []byte("untracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureClean(context.Background(), dir, 4096); err == nil || !strings.Contains(err.Error(), "including untracked files") {
		t.Fatalf("global ignore hid untracked file: %v", err)
	}
}

func TestGitStderrSuffixRedactsControlsAndSpacedPaths(t *testing.T) {
	stderr := []byte("fatal: /home/John Smith/repo/.git/config\nC:\\Users\\Jane Doe\\repo\\config\r\x1b\x7f\x80")
	suffix := gitStderrSuffix(stderr)
	if strings.ContainsAny(suffix, "\x00\n\r\t\x1b\x7f") || strings.ContainsRune(suffix, '\u0080') {
		t.Fatalf("stderr suffix retained control data: %q", suffix)
	}
	if strings.Contains(suffix, "John Smith") || strings.Contains(suffix, "Jane Doe") {
		t.Fatalf("stderr suffix leaked spaced path: %q", suffix)
	}
	if len(suffix) > 514 || !utf8.ValidString(suffix) {
		t.Fatalf("stderr suffix is not bounded UTF-8: len=%d value=%q", len(suffix), suffix)
	}
}

func TestGitLifecycleReapsAfterGroupTermination(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("safe non-reaping exit observation is implemented on Linux only")
	}
	installLifecycleGitHelper(t, "normal-exit")
	if _, _, err := runGit(context.Background(), t.TempDir(), 4096, "status"); err != nil {
		t.Fatalf("runGit error = %v, want normal exit to be observed and reaped", err)
	}
}

func TestGitRunIgnoresCallerGlobalAttributes(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git acquisition is implemented on Linux only")
	}
	dir := initGitRepository(t)
	home := t.TempDir()
	attrs := filepath.Join(home, ".config", "git")
	if err := os.MkdirAll(attrs, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attrs, "attributes"), []byte("*.txt binary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	out, _, err := runGit(context.Background(), dir, 4096, "check-attr", "diff", "--", "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "unspecified") {
		t.Fatalf("global attributes applied: %q", out)
	}
}

func TestEnsureCleanRejectsHiddenIndexFlags(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git acquisition is implemented on Linux only")
	}
	for _, flag := range []string{"assume-unchanged", "skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			dir := initGitRepository(t)
			if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("before\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			gitTestCommand(t, dir, "add", "tracked.txt")
			gitTestCommand(t, dir, "update-index", "--"+flag, "tracked.txt")
			if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("after\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ensureClean(context.Background(), dir, 4096); err == nil || !strings.Contains(err.Error(), "index state") {
				t.Fatalf("ensureClean = %v", err)
			}
		})
	}
}

func TestEnsureCleanRejectsUnknownIndexTag(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	original := runGit
	defer func() { runGit = original }()
	runGit = func(_ context.Context, _ string, _ int, args ...string) ([]byte, []byte, error) {
		if args[0] == "status" {
			return nil, nil, nil
		}
		return []byte("X tracked.txt\x00"), nil, nil
	}
	if err := ensureClean(context.Background(), t.TempDir(), 4096); err == nil || !strings.Contains(err.Error(), "index state") {
		t.Fatalf("ensureClean = %v, want unknown index state rejection", err)
	}
}

func TestGitChangesRejectsTrackedModeChangeWithFileModeDisabled(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	dir := initGitRepository(t)
	path := filepath.Join(dir, "executable.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, dir, "add", "executable.sh")
	old := gitCommit(t, dir, "initial executable")
	if err := os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, dir, "add", "second.txt")
	newRevision := gitCommit(t, dir, "second commit")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, dir, "config", "core.fileMode", "false")
	if _, err := GitChanges(context.Background(), dir, old, newRevision, GitOptions{MaxOutputBytes: 4096}); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("GitChanges = %v, want dirty mode-change rejection", err)
	}
}

func TestGitChangesRealCommitsReturnExactAMD(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git acquisition is implemented on Linux only")
	}
	dir := initGitRepository(t)
	if err := os.WriteFile(filepath.Join(dir, "modified.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deleted.txt"), []byte("remove\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, dir, "add", ".")
	old := gitCommit(t, dir, "initial")
	if err := os.WriteFile(filepath.Join(dir, "modified.txt"), []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "added.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, dir, "add", "-A")
	newRevision := gitCommit(t, dir, "changes")
	changes, err := GitChanges(context.Background(), dir, old, newRevision, GitOptions{MaxOutputBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]byte{}
	for _, change := range changes {
		got[change.Path] = change.Status
		if len(change.OldID) != 40 || len(change.NewID) != 40 {
			t.Fatalf("non-full IDs: %#v", change)
		}
	}
	want := map[string]byte{"added.txt": 'A', "modified.txt": 'M', "deleted.txt": 'D'}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("changes = %#v, want %#v", got, want)
	}
}

func TestGitChangesRejectsValidRevisionFromNonRepositoryRoot(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git acquisition is implemented on Linux only")
	}
	parent := initGitRepository(t)
	if err := os.WriteFile(filepath.Join(parent, "root.txt"), []byte("root\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestCommand(t, parent, "add", ".")
	revision := gitCommit(t, parent, "root")
	nested := filepath.Join(parent, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := GitChanges(context.Background(), nested, revision, strings.Repeat("b", 40), GitOptions{MaxOutputBytes: 4096}); err == nil || !strings.Contains(err.Error(), "repository root is required") {
		t.Fatalf("nested GitChanges error = %v", err)
	}
}

func TestGitMetadataFilesRejectSymlinks(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git acquisition is implemented on Linux only")
	}
	for _, name := range []string{"HEAD", "index", "packed-refs", "exclude", "attributes"} {
		t.Run(name, func(t *testing.T) {
			dir := initGitRepository(t)
			parent := filepath.Join(dir, ".git")
			if name == "exclude" || name == "attributes" {
				parent = filepath.Join(parent, "info")
			}
			path := filepath.Join(parent, name)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), path); err != nil {
				t.Fatal(err)
			}
			if _, err := canonicalGitRepository(dir); err == nil || !strings.Contains(err.Error(), "Git metadata file") {
				t.Fatalf("canonicalGitRepository = %v", err)
			}
		})
	}
}

func TestCanonicalGitRepositorySanitizesPathErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := canonicalGitRepository(missing)
	if err == nil {
		t.Fatal("missing repository was accepted")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonicalGitRepository error = %v, want os.ErrNotExist", err)
	}
	if strings.Contains(err.Error(), missing) {
		t.Fatalf("canonicalGitRepository leaked absolute path: %v", err)
	}
}

func TestGitConfigRejectsCleanlinessHidingControls(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git acquisition is implemented on Linux only")
	}
	for _, config := range []string{
		"[core]\nignoreStat = true\n",
		"[core]\nsparseCheckout = true\n",
		"[core]\nsparseCheckoutCone = true\n",
		"[extensions]\nsparseIndex = true\n",
	} {
		t.Run(strings.ReplaceAll(strings.TrimSpace(config), "\n", "/"), func(t *testing.T) {
			dir := initGitRepository(t)
			if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ensureGitConfigSafe(context.Background(), dir, 4096); err == nil {
				t.Fatalf("accepted unsafe config %q", config)
			}
		})
	}
}

func gitTestCommand(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	out, err := isolatedGitCommand(t, dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return out
}

func gitTestCommandError(t *testing.T, dir string, args ...string) error {
	t.Helper()
	_, err := isolatedGitCommand(t, dir, args...)
	return err
}

func isolatedGitEnvironment() []string {
	environment := safeGitEnvironment(os.Environ())
	return append(environment,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.invalid",
	)
}

func isolatedGitCommand(t *testing.T, dir string, args ...string) ([]byte, error) {
	t.Helper()
	hooksPath := filepath.Join(dir, ".git", "hooks-isolated")
	gitArgs := append([]string{"-C", dir, "-c", "core.hooksPath=" + hooksPath, "-c", "commit.gpgSign=false", "-c", "tag.gpgSign=false"}, args...)
	isInit := len(args) > 0 && args[0] == "init"
	if len(args) > 0 && args[0] == "init" {
		gitArgs = append([]string{"-C", dir, "-c", "init.templateDir=", "-c", "core.hooksPath=" + hooksPath}, args...)
	} else if err := os.MkdirAll(hooksPath, 0o700); err != nil {
		return nil, err
	}
	cmd := exec.Command("git", gitArgs...)
	cmd.Env = isolatedGitEnvironment()
	output, err := cmd.CombinedOutput()
	if err == nil && isInit {
		if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o700); err != nil {
			return output, err
		}
		err = os.MkdirAll(hooksPath, 0o700)
	}
	return output, err
}

func gitCommit(t *testing.T, dir, message string) string {
	gitTestCommand(t, dir, "commit", "-m", message)
	return strings.TrimSpace(string(gitTestCommand(t, dir, "rev-parse", "HEAD")))
}

func initGitRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if output, err := isolatedGitCommand(t, dir, "init", "--initial-branch=main"); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	return dir
}

type gitDiffSecurityWorld struct {
	t                   *testing.T
	environment         []string
	err                 error
	gitPath             string
	oldPath             string
	capErr              error
	cancelErr           error
	configErr           error
	configReason        string
	tempDir             string
	pidFile             string
	previousPath        string
	hadPath             bool
	previousHelper      string
	hadHelper           bool
	previousPIDFile     string
	hadPIDFile          bool
	environmentCaptured bool
	cleaned             bool
	previousHome        string
	hadHome             bool
	previousXDG         string
	hadXDG              bool
	previousGitDir      string
	hadGitDir           bool
	previousGitWorkTree string
	hadGitWorkTree      bool
	previousGitGlobal   string
	hadGitGlobal        bool
	isolationDir        string
	markerPath          string
	isolationOutput     []byte
	isolationErr        error
	indexErr            error
	preflightErr        error
}

func TestG9ImpactGitSecurityFeatures(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			world := &gitDiffSecurityWorld{t: t}
			sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
				world.cleanup()
				return ctx, nil
			})
			sc.Step(`^an environment containing inherited Git repository routing variables$`, world.inheritedEnvironment)
			sc.Step(`^an environment containing caller locale variables$`, world.localeEnvironment)
			sc.Step(`^the Git environment is sanitized$`, world.sanitizeEnvironment)
			sc.Step(`^repository routing variables are absent$`, world.routingVariablesAbsent)
			sc.Step(`^the Git locale is fixed to C$`, world.localeFixed)
			sc.Step(`^a hostile caller Git environment and global hook configuration$`, world.hostileGitEnvironment)
			sc.Step(`^an isolated Git test repository is initialized and committed$`, world.isolatedRepository)
			sc.Step(`^the caller repository is untouched and no global hook ran$`, world.helperIsolationVerified)
			sc.Step(`^an unisolated Git test repository is initialized and committed$`, world.unisolatedRepository)
			sc.Step(`^the global pre-commit hook ran$`, world.globalHookRan)
			sc.Step(`^the hostile Git environment is restored$`, world.restoreHostileEnvironment)
			sc.Step(`^the hostile Git environment variables are restored$`, world.hostileEnvironmentRestored)
			sc.Step(`^a Git preflight listing child that exceeds the output cap$`, world.preflightListingChild)
			sc.Step(`^the Git tree preflight is checked$`, world.checkTreePreflight)
			sc.Step(`^a command-specific Git preflight output-limit error is returned$`, world.preflightError)
			sc.Step(`^a Git child that writes beyond the output cap$`, world.capChild)
			sc.Step(`^the bounded Git command is run$`, world.runBounded)
			sc.Step(`^the bounded Git command returns an output-limit error promptly$`, world.capError)
			sc.Step(`^a Git child that closes stdout with a surviving descendant$`, world.descendantChild)
			sc.Step(`^the bounded Git command is cancelled$`, world.cancelled)
			sc.Step(`^the Git descendant terminates promptly after cancellation$`, world.descendantTerminated)
			sc.Step(`^a repository with an included executable Git filter configuration$`, world.includedFilter)
			sc.Step(`^a repository with "([^"]+)" Git configuration$`, world.staticConfiguration)
			sc.Step(`^a Git configuration is replaced between inspection and opening$`, world.replaceConfiguration)
			sc.Step(`^the replacement Git configuration is rejected$`, world.configurationRejected)
			sc.Step(`^the repository configuration is inspected$`, world.inspectConfiguration)
			sc.Step(`^executable repository configuration is rejected$`, world.configurationRejected)
			sc.Step(`^a Git helper scenario acquires temporary resources$`, world.acquireResources)
			sc.Step(`^explicit helper scenario cleanup runs$`, world.runCleanup)
			sc.Step(`^the helper environment and temporary resources are restored$`, world.resourcesRestored)
			sc.Step(`^malformed raw Git metadata$`, world.malformedMetadata)
			sc.Step(`^the raw Git diff is parsed$`, world.parseMetadata)
			sc.Step(`^parsing fails closed$`, world.parseError)
			sc.Step(`^a repository and caller XDG global Git attributes$`, world.globalAttributesRepository)
			sc.Step(`^the bounded Git attributes query is run$`, world.globalAttributesQuery)
			sc.Step(`^caller global Git attributes are ignored$`, world.globalAttributesIgnored)
			sc.Step(`^a tracked repository file marked "([^"]+)"$`, world.flaggedRepository)
			sc.Step(`^repository cleanliness is checked$`, world.checkFlaggedCleanliness)
			sc.Step(`^the hidden tracked file is rejected$`, world.flaggedFileRejected)
			sc.Step(`^a tracked executable with core\.fileMode disabled$`, world.fileModeRepository)
			sc.Step(`^the tracked mode change is rejected$`, world.fileModeRejected)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../../features/g9-impact-git-security.feature"},
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run Git security features")
	}
}

func TestGitDiffSecurityCleanupAfterFailedGodogStep(t *testing.T) {
	feature := filepath.Join(t.TempDir(), "cleanup.feature")
	contents := `Feature: cleanup hook
  Scenario: cleanup runs after a failed step
    Given a Git helper scenario acquires temporary resources
    When a helper step fails for real
`
	if err := os.WriteFile(feature, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	world := &gitDiffSecurityWorld{}
	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
				world.cleanup()
				return ctx, nil
			})
			sc.Step(`^a Git helper scenario acquires temporary resources$`, world.acquireResources)
			sc.Step(`^a helper step fails for real$`, func() error {
				return errors.New("intentional nested Godog step failure")
			})
		},
		// Deliberately omit Options.TestingT: this nested suite must return the
		// expected failing status rather than fail the enclosing Go test.
		Options: &godog.Options{
			Format: "progress",
			Paths:  []string{feature},
			Strict: true,
		},
	}
	if status := suite.Run(); status == 0 {
		t.Fatal("nested Godog suite unexpectedly passed")
	}
	if !world.cleaned {
		t.Fatal("Godog After hook did not run after the failing step")
	}
	if world.tempDir != "" {
		if _, err := os.Stat(world.tempDir); !os.IsNotExist(err) {
			t.Fatalf("helper directory remains after Godog After hook: %v", err)
		}
	}
	helper, exists := os.LookupEnv("MCD_GIT_HELPER")
	if exists != world.hadHelper || helper != world.previousHelper {
		t.Fatal("helper environment was not restored by Godog After hook")
	}
}

func (w *gitDiffSecurityWorld) cleanup() {
	if w.environmentCaptured {
		if w.hadHelper {
			_ = os.Setenv("MCD_GIT_HELPER", w.previousHelper)
		} else {
			_ = os.Unsetenv("MCD_GIT_HELPER")
		}
		if w.hadPath {
			_ = os.Setenv("PATH", w.previousPath)
		} else {
			_ = os.Unsetenv("PATH")
		}
		if w.hadPIDFile {
			_ = os.Setenv("MCD_GIT_HELPER_PID", w.previousPIDFile)
		} else {
			_ = os.Unsetenv("MCD_GIT_HELPER_PID")
		}
		if w.hadHome {
			_ = os.Setenv("HOME", w.previousHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
		if w.hadXDG {
			_ = os.Setenv("XDG_CONFIG_HOME", w.previousXDG)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
		for _, item := range []struct {
			name   string
			value  string
			wasSet bool
		}{
			{"GIT_DIR", w.previousGitDir, w.hadGitDir},
			{"GIT_WORK_TREE", w.previousGitWorkTree, w.hadGitWorkTree},
			{"GIT_CONFIG_GLOBAL", w.previousGitGlobal, w.hadGitGlobal},
		} {
			if item.wasSet {
				_ = os.Setenv(item.name, item.value)
			} else {
				_ = os.Unsetenv(item.name)
			}
		}
	}
	if w.tempDir != "" {
		_ = os.RemoveAll(w.tempDir)
	}
	w.cleaned = true
}

func (w *gitDiffSecurityWorld) replaceConfiguration() error {
	dir, err := os.MkdirTemp("", "gitconfig-swap-")
	if err != nil {
		return err
	}
	w.tempDir = dir
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("[core]\n bare = false\n"), 0o600); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if err := os.Rename(path, path+".original"); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte("[core]\n bare = false\n"), 0o600); err != nil {
		return err
	}
	file, err := openGitConfig(path, info)
	if err == nil {
		if closeErr := file.Close(); closeErr != nil {
			return closeErr
		}
		return errors.New("replaced Git configuration was accepted")
	}
	w.configErr = err
	w.configReason = "changed during inspection"
	return nil
}

func (w *gitDiffSecurityWorld) captureEnvironment() {
	w.previousPath, w.hadPath = os.LookupEnv("PATH")
	w.previousHelper, w.hadHelper = os.LookupEnv("MCD_GIT_HELPER")
	w.previousPIDFile, w.hadPIDFile = os.LookupEnv("MCD_GIT_HELPER_PID")
	w.previousHome, w.hadHome = os.LookupEnv("HOME")
	w.previousXDG, w.hadXDG = os.LookupEnv("XDG_CONFIG_HOME")
	w.previousGitDir, w.hadGitDir = os.LookupEnv("GIT_DIR")
	w.previousGitWorkTree, w.hadGitWorkTree = os.LookupEnv("GIT_WORK_TREE")
	w.previousGitGlobal, w.hadGitGlobal = os.LookupEnv("GIT_CONFIG_GLOBAL")
	w.environmentCaptured = true
}

func (w *gitDiffSecurityWorld) inheritedEnvironment() {
	w.environment = []string{"GIT_DIR=/tmp/attacker", "gIt_Work_Tree=/tmp/worktree", "GIT_INDEX_FILE=/tmp/index", "GIT_COMMON_DIR=/tmp/common"}
}

func (w *gitDiffSecurityWorld) localeEnvironment() {
	w.environment = []string{"LANG=de_DE.UTF-8", "LANGUAGE=de", "LC_MESSAGES=de_DE.UTF-8", "LC_ALL=de_DE.UTF-8", "PATH=/usr/bin"}
}

func (w *gitDiffSecurityWorld) sanitizeEnvironment() {
	for _, item := range safeGitEnvironment(w.environment) {
		name, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(strings.ToUpper(name), "GIT_") && name != "GIT_CONFIG_NOSYSTEM" && name != "GIT_CONFIG_GLOBAL" && name != "GIT_NO_REPLACE_OBJECTS" && name != "GIT_OPTIONAL_LOCKS" && name != "GIT_NO_LAZY_FETCH" && name != "GIT_ATTR_NOSYSTEM" {
			w.err = fmt.Errorf("inherited Git variable survived: %s", name)
		}
	}
}

func (w *gitDiffSecurityWorld) routingVariablesAbsent() error {
	return w.err
}

func (w *gitDiffSecurityWorld) localeFixed() error {
	if w.err != nil {
		return w.err
	}
	for _, item := range safeGitEnvironment(w.environment) {
		name, _, _ := strings.Cut(item, "=")
		if item == "LC_ALL=C" {
			return nil
		}
		if name == "LANG" || name == "LANGUAGE" || strings.HasPrefix(name, "LC_") {
			return fmt.Errorf("caller locale survived: %s", item)
		}
	}
	return errors.New("fixed C locale is absent")
}

func (w *gitDiffSecurityWorld) hostileGitEnvironment() error {
	w.captureEnvironment()
	hostile := w.t.TempDir()
	marker := filepath.Join(w.t.TempDir(), "global-hook-ran")
	globalConfig := filepath.Join(w.t.TempDir(), "gitconfig")
	hook := filepath.Join(w.t.TempDir(), "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf ran > "+marker+"\nexit 1\n"), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(globalConfig, []byte("[core]\n\thooksPath = "+filepath.Dir(hook)+"\n"), 0o600); err != nil {
		return err
	}
	w.t.Setenv("GIT_DIR", filepath.Join(hostile, ".git"))
	w.t.Setenv("GIT_WORK_TREE", hostile)
	w.t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	w.t.Setenv("HOME", w.t.TempDir())
	w.isolationDir = hostile
	w.tempDir = hostile
	w.markerPath = marker
	return nil
}

func (w *gitDiffSecurityWorld) restoreHostileEnvironment() error {
	w.cleanup()
	return nil
}

func (w *gitDiffSecurityWorld) hostileEnvironmentRestored() error {
	for _, item := range []struct {
		name   string
		value  string
		wasSet bool
	}{
		{"GIT_DIR", w.previousGitDir, w.hadGitDir},
		{"GIT_WORK_TREE", w.previousGitWorkTree, w.hadGitWorkTree},
		{"GIT_CONFIG_GLOBAL", w.previousGitGlobal, w.hadGitGlobal},
	} {
		value, exists := os.LookupEnv(item.name)
		if exists != item.wasSet || exists && value != item.value {
			return fmt.Errorf("%s was not restored", item.name)
		}
	}
	return nil
}

func (w *gitDiffSecurityWorld) isolatedRepository() error {
	dir := initGitRepository(w.t)
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		return err
	}
	if output, err := isolatedGitCommand(w.t, dir, "add", "tracked.txt"); err != nil {
		return fmt.Errorf("git add: %w: %s", err, output)
	}
	if output, err := isolatedGitCommand(w.t, dir, "commit", "-m", "isolated helper commit"); err != nil {
		return fmt.Errorf("git commit: %w: %s", err, output)
	}
	return nil
}

func (w *gitDiffSecurityWorld) unisolatedRepository() error {
	dir := w.t.TempDir()
	environment := os.Environ()
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE"} {
		environment = removeEnvironmentVariable(environment, name)
	}
	environment = append(environment,
		"GIT_AUTHOR_NAME=Unisolated Test",
		"GIT_AUTHOR_EMAIL=unisolated@example.invalid",
		"GIT_COMMITTER_NAME=Unisolated Test",
		"GIT_COMMITTER_EMAIL=unisolated@example.invalid",
	)
	run := func(args ...string) error {
		command := exec.Command("git", args...)
		command.Env = environment
		return command.Run()
	}
	if err := run("-C", dir, "init", "--initial-branch=main"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		return err
	}
	if err := run("-C", dir, "add", "tracked.txt"); err != nil {
		return err
	}
	config := exec.Command("git", "-C", dir, "config", "core.hooksPath")
	config.Env = environment
	if output, err := config.Output(); err != nil || strings.TrimSpace(string(output)) == "" {
		return fmt.Errorf("global hook path was not visible to unisolated Git: %q: %w", output, err)
	}
	_ = run("-C", dir, "commit", "-m", "unisolated helper commit")
	return nil
}

func removeEnvironmentVariable(environment []string, name string) []string {
	filtered := environment[:0]
	for _, item := range environment {
		key, _, _ := strings.Cut(item, "=")
		if key != name {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (w *gitDiffSecurityWorld) globalHookRan() error {
	if _, err := os.Stat(w.markerPath); err != nil {
		return fmt.Errorf("global pre-commit hook did not run: %w", err)
	}
	return nil
}

func (w *gitDiffSecurityWorld) preflightListingChild() error {
	dir := w.t.TempDir()
	w.captureEnvironment()
	path := installGitHelper(w.t, dir, "preflight-stdout")
	w.t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_ = path
	w.preflightErr = nil
	err := ensureNoSubmodules(context.Background(), w.t.TempDir(), "HEAD", 4)
	w.preflightErr = err
	return nil
}

func (w *gitDiffSecurityWorld) checkTreePreflight() error {
	if w.preflightErr == nil {
		return errors.New("tree preflight unexpectedly succeeded")
	}
	return nil
}

func (w *gitDiffSecurityWorld) preflightError() error {
	if w.preflightErr == nil || !strings.Contains(w.preflightErr.Error(), "Git tree preflight failed") || !strings.Contains(w.preflightErr.Error(), "stdout exceeded") {
		return fmt.Errorf("tree preflight error = %v", w.preflightErr)
	}
	return nil
}

func (w *gitDiffSecurityWorld) helperIsolationVerified() error {
	if _, err := os.Stat(w.markerPath); err == nil {
		return errors.New("global hook ran")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(filepath.Join(w.isolationDir, ".git")); err == nil {
		return errors.New("caller Git directory was modified")
	}
	return nil
}
func (w *gitDiffSecurityWorld) capChild() error {
	dir, err := os.MkdirTemp("", "gitdiff-security-")
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	w.gitPath = filepath.Join(dir, name)
	w.tempDir = dir
	w.captureEnvironment()
	data, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	if err := os.WriteFile(w.gitPath, data, 0o700); err != nil {
		return err
	}
	if err := os.Setenv("MCD_GIT_HELPER", "stdout"); err != nil {
		return err
	}
	w.oldPath = w.previousPath
	return os.Setenv("PATH", dir+string(os.PathListSeparator)+w.oldPath)
}
func (w *gitDiffSecurityWorld) runBounded() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, w.capErr = runGit(ctx, os.TempDir(), 4, "status")
	return nil
}
func (w *gitDiffSecurityWorld) capError() error {
	if w.capErr == nil {
		return errors.New("bounded child unexpectedly returned no error")
	}
	if !strings.Contains(w.capErr.Error(), "stdout exceeded") {
		return fmt.Errorf("bounded child error = %v", w.capErr)
	}
	return nil
}

func (w *gitDiffSecurityWorld) descendantChild() error {
	dir, err := os.MkdirTemp("", "gitdiff-security-descendant-")
	if err != nil {
		return err
	}
	w.tempDir = dir
	w.captureEnvironment()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	w.gitPath = filepath.Join(dir, "git")
	data, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	if err := os.WriteFile(w.gitPath, data, 0o700); err != nil {
		return err
	}
	w.pidFile = filepath.Join(dir, "child.pid")
	if err := os.Setenv("MCD_GIT_HELPER", "stdout-closed-descendant"); err != nil {
		return err
	}
	if err := os.Setenv("MCD_GIT_HELPER_PID", w.pidFile); err != nil {
		return err
	}
	return os.Setenv("PATH", dir+string(os.PathListSeparator)+w.previousPath)
}

func (w *gitDiffSecurityWorld) cancelled() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, _, err := runGit(ctx, os.TempDir(), 4096, "status")
		result <- err
	}()
	if _, err := waitForGitHelperPID(w.pidFile); err != nil {
		return err
	}
	cancel()
	w.cancelErr = <-result
	if !errors.Is(w.cancelErr, context.Canceled) {
		return fmt.Errorf("cancelled Git command error = %v, want context canceled", w.cancelErr)
	}
	return nil
}

func (w *gitDiffSecurityWorld) descendantTerminated() error {
	child, err := waitForGitHelperPID(w.pidFile)
	if err != nil {
		return err
	}
	if !waitForGitHelperExit(child) {
		return fmt.Errorf("descendant %d survived cancellation", child)
	}
	return nil
}

func (w *gitDiffSecurityWorld) includedFilter() error {
	dir, err := os.MkdirTemp("", "gitdiff-security-config-")
	if err != nil {
		return err
	}
	w.tempDir = dir
	output, err := isolatedGitCommand(w.t, dir, "init", "--initial-branch=main")
	if err != nil {
		return fmt.Errorf("git init: %w: %s", err, output)
	}
	w.configReason = `unsafe control "include.path"`
	config := filepath.Join(dir, ".git", "config")
	if err := os.WriteFile(config, []byte("[include]\n\tpath = evil.cfg\n"), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".git", "evil.cfg"), []byte("[filter \"x\"]\n\tprocess = sh -c cat\n"), 0o600)
}

func (w *gitDiffSecurityWorld) inspectConfiguration() error {
	_, w.configErr = canonicalGitRepository(w.tempDir)
	if w.configErr == nil {
		w.configErr = ensureGitConfigSafe(context.Background(), w.tempDir, 4096)
	}
	return nil
}

func (w *gitDiffSecurityWorld) staticConfiguration(kind string) error {
	dir, err := os.MkdirTemp("", "gitdiff-security-static-config-")
	if err != nil {
		return err
	}
	w.tempDir = dir
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, ".git", "config")
	if err := os.WriteFile(path, []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"), 0o600); err != nil {
		return err
	}
	if _, err := canonicalGitRepository(dir); err != nil {
		return fmt.Errorf("baseline repository layout: %w", err)
	}
	if err := ensureGitConfigSafe(context.Background(), dir, 4096); err != nil {
		return fmt.Errorf("baseline configuration: %w", err)
	}
	reasons := map[string]string{
		"legacy filter section":     `unsafe control "filter.process"`,
		"same-line filter payload":  "text after section header",
		"worktree redirection":      `unsafe control "core.worktree"`,
		"common Git directory":      `metadata "commondir" is unsupported`,
		"Git directory redirect":    `metadata "gitdir" is unsupported`,
		"alternate object store":    "alternate object stores are unsupported",
		"oversized configuration":   "exceeded 4096-byte limit",
		"non-regular configuration": "must be a regular file",
	}
	w.configReason = reasons[kind]
	var data string
	switch kind {
	case "legacy filter section":
		data = "[FILTER.evil]\nprocess = helper\n"
	case "same-line filter payload":
		data = "[filter \"evil\"] process = helper\n"
	case "continued filter section", "quoted continuation", "CRLF continuation":
		data = "[filter \"evil\"]\ndummy = a\\\n[core]\nprocess = helper\n"
		if kind == "quoted continuation" {
			data = "[filter \"evil\"]\ndummy = \"a\\\n[core]\"\nprocess = helper\n"
		}
		if kind == "CRLF continuation" {
			data = strings.ReplaceAll(data, "\n", "\r\n")
		}
		w.configReason = "continuation"
	case "worktree redirection":
		data = "[core]\nworktree = ../../outside\n"
	case "common Git directory":
		return os.WriteFile(filepath.Join(dir, ".git", "commondir"), []byte("../../outside\n"), 0o600)
	case "Git directory redirect":
		return os.WriteFile(filepath.Join(dir, ".git", "gitdir"), []byte("../../outside/.git\n"), 0o600)
	case "alternate object store":
		if err := os.MkdirAll(filepath.Join(dir, ".git", "objects", "info"), 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, ".git", "objects", "info", "alternates"), []byte("../../outside\n"), 0o600)
	case "oversized configuration":
		data = "#" + strings.Repeat("x", 4096)
	case "non-regular configuration":
		if err := os.Remove(path); err != nil {
			return err
		}
		return os.Mkdir(path, 0o700)
	default:
		return fmt.Errorf("unknown static Git configuration case %q", kind)
	}
	return os.WriteFile(path, []byte(data), 0o600)
}

func (w *gitDiffSecurityWorld) configurationRejected() error {
	if w.configErr == nil {
		return errors.New("unsafe configuration was accepted")
	}
	if w.configReason == "" || !strings.Contains(w.configErr.Error(), w.configReason) {
		return fmt.Errorf("configuration rejection = %v, want %q", w.configErr, w.configReason)
	}
	return nil
}
func (w *gitDiffSecurityWorld) acquireResources() error {
	dir, err := os.MkdirTemp("", "gitdiff-security-bdd-")
	if err != nil {
		return err
	}
	w.tempDir = dir
	w.captureEnvironment()
	return os.Setenv("MCD_GIT_HELPER", "stdout")
}
func (w *gitDiffSecurityWorld) runCleanup() error { w.cleanup(); return nil }
func (w *gitDiffSecurityWorld) resourcesRestored() error {
	if !w.cleaned {
		return errors.New("helper cleanup was not run")
	}
	if _, err := os.Stat(w.tempDir); !os.IsNotExist(err) {
		return fmt.Errorf("helper directory remains: %v", err)
	}
	helper, exists := os.LookupEnv("MCD_GIT_HELPER")
	if exists != w.hadHelper || helper != w.previousHelper {
		return errors.New("helper environment was not restored")
	}
	if os.Getenv("PATH") != w.previousPath {
		return errors.New("PATH was not restored")
	}
	pidFile, pidFileExists := os.LookupEnv("MCD_GIT_HELPER_PID")
	if pidFileExists != w.hadPIDFile || pidFile != w.previousPIDFile {
		return errors.New("helper PID environment was not restored")
	}
	return nil
}
func (w *gitDiffSecurityWorld) malformedMetadata() {}
func (w *gitDiffSecurityWorld) parseMetadata() {
	_, w.err = ParseDiff([]byte(":100644 100644 aaaaaaa bbbbbbb M\x00file.go\x00"))
}

func waitForGitHelperPID(path string) (int, error) {
	deadline := time.Now().Add(time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(string(data))
			if err == nil && pid > 0 {
				return pid, nil
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return 0, fmt.Errorf("wait for descendant pid: %w", err)
			}
			return 0, fmt.Errorf("wait for valid descendant pid in %q", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func (w *gitDiffSecurityWorld) parseError() error {
	if w.err != nil {
		return nil
	}
	return errors.New("parsing unexpectedly succeeded")
}

func (w *gitDiffSecurityWorld) globalAttributesRepository() error {
	dir := initGitRepository(w.t)
	w.tempDir = dir
	home := w.t.TempDir()
	attrs := filepath.Join(home, "xdg", "git")
	if err := os.MkdirAll(attrs, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(attrs, "attributes"), []byte("*.txt binary\n"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data\n"), 0o600); err != nil {
		return err
	}
	w.captureEnvironment()
	if err := os.Setenv("HOME", home); err != nil {
		return err
	}
	return os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
}

func (w *gitDiffSecurityWorld) globalAttributesQuery() error {
	w.isolationOutput, _, w.isolationErr = runGit(context.Background(), w.tempDir, 4096, "check-attr", "diff", "--", "file.txt")
	return nil
}

func (w *gitDiffSecurityWorld) globalAttributesIgnored() error {
	if w.isolationErr != nil {
		return w.isolationErr
	}
	if !strings.Contains(string(w.isolationOutput), "unspecified") {
		return fmt.Errorf("global attributes were applied: %q", w.isolationOutput)
	}
	return nil
}

func (w *gitDiffSecurityWorld) flaggedRepository(flag string) error {
	dir := initGitRepository(w.t)
	w.tempDir = dir
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("before\n"), 0o600); err != nil {
		return err
	}
	gitTestCommand(w.t, dir, "add", "tracked.txt")
	gitTestCommand(w.t, dir, "update-index", "--"+flag, "tracked.txt")
	return os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("after\n"), 0o600)
}

func (w *gitDiffSecurityWorld) checkFlaggedCleanliness() error {
	w.indexErr = ensureClean(context.Background(), w.tempDir, 4096)
	return nil
}

func (w *gitDiffSecurityWorld) flaggedFileRejected() error {
	if w.indexErr == nil || !strings.Contains(w.indexErr.Error(), "index state") {
		return fmt.Errorf("flagged file was not rejected: %v", w.indexErr)
	}
	return nil
}

func (w *gitDiffSecurityWorld) fileModeRepository() error {
	dir := initGitRepository(w.t)
	w.tempDir = dir
	path := filepath.Join(dir, "executable.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		return err
	}
	gitTestCommand(w.t, dir, "add", "executable.sh")
	gitCommit(w.t, dir, "executable")
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return gitTestCommandError(w.t, dir, "config", "core.fileMode", "false")
}

func (w *gitDiffSecurityWorld) fileModeRejected() error {
	w.indexErr = ensureClean(context.Background(), w.tempDir, 4096)
	if w.indexErr == nil || !strings.Contains(w.indexErr.Error(), "dirty") {
		return fmt.Errorf("tracked mode change was not rejected: %v", w.indexErr)
	}
	return nil
}
