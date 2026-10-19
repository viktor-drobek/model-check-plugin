//go:build windows

package impact

import (
	"errors"
	"os/exec"
	"testing"
)

func processIsLive(_ int) bool { return false }

func TestGitProcessTreeUnsupported(t *testing.T) {
	if err := configureGitProcess(exec.Command("git")); !errors.Is(err, errGitProcessTreeUnsupported) {
		t.Fatalf("configureGitProcess error = %v, want unsupported", err)
	}
}
