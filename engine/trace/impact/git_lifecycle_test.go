package impact

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func init() {
	switch os.Getenv("MCD_GIT_LIFECYCLE_HELPER") {
	case "stderr-after-stdout":
		_ = os.Stdout.Close()
		_, _ = os.Stderr.Write([]byte(strings.Repeat("e", 4096)))
		time.Sleep(2 * time.Second)
		os.Exit(0)
	case "inherited-pipe-parent":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "MCD_GIT_LIFECYCLE_HELPER=inherited-pipe-child")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "inherited-pipe-child":
		time.Sleep(2 * time.Second)
		os.Exit(0)
	case "stdout-closed-sleeping-leader":
		_ = os.WriteFile(os.Getenv("MCD_GIT_LIFECYCLE_MARKER"), []byte("started"), 0o600)
		_ = os.Stdout.Close()
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "stdout-closed-exiting-leader":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "MCD_GIT_LIFECYCLE_HELPER=exiting-leader-child")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(1)
		}
		_ = os.WriteFile(os.Getenv("MCD_GIT_LIFECYCLE_MARKER"), []byte("started"), 0o600)
		_ = os.Stdout.Close()
		os.Exit(0)
	case "exiting-leader-child":
		_ = os.WriteFile(os.Getenv("MCD_GIT_LIFECYCLE_PID"), []byte(strconv.Itoa(os.Getpid())), 0o600)
		time.Sleep(5 * time.Second)
		os.Exit(0)
	case "normal-exit":
		os.Exit(0)
	case "normal-exit-descendant":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "MCD_GIT_LIFECYCLE_HELPER=normal-exit-descendant-child")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "normal-exit-descendant-child":
		_ = os.WriteFile(os.Getenv("MCD_GIT_LIFECYCLE_PID"), []byte(strconv.Itoa(os.Getpid())), 0o600)
		_ = os.Stdout.Close()
		_ = os.Stderr.Close()
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
}

func TestGitLifecycleRejectsStderrOverflowAfterStdoutEOF(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	installLifecycleGitHelper(t, "stderr-after-stdout")

	started := time.Now()
	_, _, err := runGit(context.Background(), t.TempDir(), 32, "status")
	if err == nil || !strings.Contains(err.Error(), "stderr exceeded") {
		t.Fatalf("runGit error = %v, want stderr limit error", err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("runGit took %s after stderr overflow", elapsed)
	}
}

func TestGitLifecycleHandlesParentExitWithInheritedOutputPipes(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	installLifecycleGitHelper(t, "inherited-pipe-parent")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	_, _, err := runGit(ctx, t.TempDir(), 4096, "status")
	if err != nil {
		t.Fatalf("runGit error = %v, want helper self-expiry to close inherited pipes", err)
	}
	if elapsed := time.Since(started); elapsed >= 5*time.Second {
		t.Fatalf("runGit took %s, want inherited pipes to close before the helper expiry bound", elapsed)
	}
}

func TestGitLifecycleCancelsLeaderAfterStdoutEOF(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("safe non-reaping exit observation is implemented on Linux only")
	}
	installLifecycleGitHelper(t, "stdout-closed-sleeping-leader")
	marker := filepath.Join(t.TempDir(), "started")
	t.Setenv("MCD_GIT_LIFECYCLE_MARKER", marker)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, err := runGit(ctx, t.TempDir(), 4096, "status")
		result <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lifecycle helper did not close stdout")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("runGit returned nil after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("runGit deadlocked after stdout EOF and cancellation")
	}
}

func TestGitLifecycleKillsDescendantAfterLeaderExit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("safe non-reaping exit observation is implemented on Linux only")
	}
	installLifecycleGitHelper(t, "stdout-closed-exiting-leader")
	marker := filepath.Join(t.TempDir(), "started")
	t.Setenv("MCD_GIT_LIFECYCLE_MARKER", marker)
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	// The helper's child writes its PID through the inherited test environment.
	t.Setenv("MCD_GIT_LIFECYCLE_PID", pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, err := runGit(ctx, t.TempDir(), 4096, "status")
		result <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lifecycle helper did not close stdout")
		}
		time.Sleep(time.Millisecond)
	}
	// The leader exits immediately; give the inherited child time to publish its PID.
	deadline = time.Now().Add(time.Second)
	var pid int
	for {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, err = strconv.Atoi(string(data))
			if err == nil && pid > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("lifecycle descendant did not publish its PID")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-result; err == nil {
		t.Fatal("runGit returned nil after cancellation")
	}
	if !waitForGitHelperExit(pid) {
		t.Fatalf("descendant %d survived after leader exit", pid)
	}
}

func TestGitLifecycleKillsDescendantOnNormalExit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Git process-tree containment is implemented on Linux only")
	}
	installLifecycleGitHelper(t, "normal-exit-descendant")
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("MCD_GIT_LIFECYCLE_PID", pidFile)
	_, _, err := runGit(context.Background(), t.TempDir(), 4096, "status")
	if err != nil {
		t.Fatalf("runGit error = %v, want normal success", err)
	}
	pid := readGitHelperPID(t, pidFile)
	if !waitForGitHelperExit(pid) {
		t.Fatalf("descendant %d survived normal Git exit", pid)
	}
}

func installLifecycleGitHelper(t *testing.T, mode string) {
	t.Helper()
	dir := t.TempDir()
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
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MCD_GIT_LIFECYCLE_HELPER", mode)
}
