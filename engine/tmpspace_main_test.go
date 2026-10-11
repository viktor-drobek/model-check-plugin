package modelcheck_test

import (
	"fmt"
	"os"
	"testing"

	"modelcheck/internal/tmpspace"
)

// TestMain refuses to start the suite into a scratch directory that cannot
// hold its temporary files: the SPIN comparisons, the mutation campaign and
// the godog scenarios compile pan.c into TMPDIR, several at a time under -p.
// A quota on a tmpfs is invisible to df and statfs, so the probe writes the
// amount it asks for (tmpspace.DefaultMB, or MCD_TMP_MIN_MB; 0 switches it
// off); the failure then names the directory, the numbers, the cause and the
// remedy instead of surfacing minutes later as a gcc error inside a scenario.
func TestMain(m *testing.M) {
	if _, err := tmpspace.ProbeEnv(tmpspace.DefaultMB); err != nil {
		fmt.Fprintf(os.Stderr, "modelcheck tests: %v\n", err)
		os.Exit(2)
	}
	os.Exit(m.Run())
}
