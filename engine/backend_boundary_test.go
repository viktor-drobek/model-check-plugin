package modelcheck_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMCDDoesNotLinkBackendContract keeps the M3.0a contract kernel out of
// the production CLI until an approved integration increment adds it.
func TestMCDDoesNotLinkBackendContract(t *testing.T) {
	targets := []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64"}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			parts := strings.Split(target, "/")
			command := exec.Command("go", "list", "-deps", "./cmd/mcd")
			command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+parts[0], "GOARCH="+parts[1])
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("go list mcd dependencies: %v\n%s", err, output)
			}
			dependencies := strings.Fields(string(output))
			if len(dependencies) == 0 {
				t.Fatal("go list returned no dependencies")
			}
			for _, dependency := range dependencies {
				if dependency == "modelcheck/backend" {
					t.Fatal("cmd/mcd links modelcheck/backend before an approved integration increment")
				}
			}
		})
	}
	command := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "./backend")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list backend dependencies: %v\n%s", err, stderr.String())
	}
	dependencies := strings.Fields(string(output))
	want := map[string]bool{"modelcheck/backend": true, "modelcheck/ir": true}
	for _, dependency := range dependencies {
		if !want[dependency] {
			t.Fatalf("backend links forbidden nonstandard dependency %q", dependency)
		}
		delete(want, dependency)
	}
	for dependency := range want {
		t.Fatalf("backend does not link required dependency %q", dependency)
	}
}
