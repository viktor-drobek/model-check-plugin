//go:build !windows && !linux

package impact

import (
	"os"
	"os/exec"
)

func openGitConfigFile(_ string) (*os.File, error) {
	return nil, errGitProcessTreeUnsupported
}

func configureGitProcess(_ *exec.Cmd) error {
	return errGitProcessTreeUnsupported
}

func waitForGitProcessExit(_ int) error {
	return errGitProcessTreeUnsupported
}

func terminateGitProcessTree(_ *exec.Cmd, _ *gitProcessLifecycle) error {
	return errGitProcessTreeUnsupported
}

func terminateGitProcessTreeLocked(_ *exec.Cmd) error {
	return errGitProcessTreeUnsupported
}
