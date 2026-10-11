package pandiff

import (
	"fmt"
	"os"
	"testing"

	"modelcheck/internal/tmpspace"
)

// TestMain refuses to start into a scratch directory without room: every test
// of this package compiles pan.c into TMPDIR, and the corpus comparison
// compiles several models in parallel. The amount is tmpspace.DefaultMB, or
// MCD_TMP_MIN_MB (0 switches the probe off); see modelcheck/internal/tmpspace
// for why free space alone is not checked.
func TestMain(m *testing.M) {
	if _, err := tmpspace.ProbeEnv(tmpspace.DefaultMB); err != nil {
		fmt.Fprintf(os.Stderr, "pandiff tests: %v\n", err)
		os.Exit(2)
	}
	os.Exit(m.Run())
}
