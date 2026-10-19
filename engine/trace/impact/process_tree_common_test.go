package impact

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func waitForGitHelperExit(pid int) bool {
	deadline := time.Now().Add(time.Second)
	for processIsLive(pid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return true
}

func readGitHelperPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(string(data))
			if err == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("wait for descendant pid: %v", err)
			}
			t.Fatalf("wait for valid descendant pid in %q", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
