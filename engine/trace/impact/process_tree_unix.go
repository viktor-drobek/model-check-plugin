//go:build linux

package impact

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func openGitConfigFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

func configureGitProcess(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func waitForGitProcessExit(pid int) error {
	deadline := time.Now().Add(30 * time.Second)
	var info unix.Siginfo
	for {
		info = unix.Siginfo{}
		if err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOHANG|unix.WNOWAIT, nil); err != nil {
			return fmt.Errorf("impact: observe Git process exit: %w", err)
		}
		if info.Signo != 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("impact: observe Git process exit: timed out after 30s")
		}
		time.Sleep(time.Millisecond)
	}
}

func terminateGitProcessTree(command *exec.Cmd, lifecycle *gitProcessLifecycle) error {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if lifecycle.reaping || lifecycle.reaped {
		return nil
	}
	return terminateGitProcessTreeLocked(command)
}

func terminateGitProcessTreeLocked(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("impact: terminate Git process group: %w", err)
	}
	return nil
}
