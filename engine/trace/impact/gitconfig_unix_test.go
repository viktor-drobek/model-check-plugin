//go:build linux

package impact

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestGitConfigOpenFileTypes(t *testing.T) {
	if os.Getenv("MCD_GITCONFIG_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGitConfigOpenFileTypes$", "-test.v")
		command.Env = append(os.Environ(), "MCD_GITCONFIG_CHILD=1")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("config-open regression: %v (context: %v): %s", err, ctx.Err(), output)
		}
		return
	}
	for _, kind := range []string{"regular", "replacement", "symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(path, []byte("[core]\n bare = false\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if kind != "regular" {
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "replacement":
					err = os.WriteFile(path, []byte("[core]\n bare = false\n"), 0o600)
				case "symlink":
					err = os.Symlink(path+".original", path)
				case "fifo":
					err = syscall.Mkfifo(path, 0o600)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			file, err := openGitConfig(path, info)
			if err == nil {
				if closeErr := file.Close(); closeErr != nil {
					t.Fatal(closeErr)
				}
			}
			if (err == nil) != (kind == "regular") {
				t.Fatalf("openGitConfig(%s) error = %v", kind, err)
			}
		})
	}
}
