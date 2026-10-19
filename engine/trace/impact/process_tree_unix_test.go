//go:build linux

package impact

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
)

func TestGitDiffSecurityKillsDescendantOnCancellation(t *testing.T) {
	dir := t.TempDir()
	installGitHelper(t, dir, "descendant")
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("MCD_GIT_HELPER_PID", pidFile)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

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
	if err == nil {
		t.Fatal("runGit returned nil after cancellation")
	}
	if !waitForGitHelperExit(pid) {
		t.Fatalf("descendant %d survived cancellation", pid)
	}
}

func processIsLive(pid int) bool {
	if runtime.GOOS != "linux" {
		return syscall.Kill(pid, 0) != syscall.ESRCH
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	closing := bytes.LastIndexByte(data, ')')
	if closing < 0 || len(data) <= closing+2 {
		return true
	}
	state := data[closing+2]
	if state == 'Z' || state == 'X' {
		return false
	}
	return true
}
