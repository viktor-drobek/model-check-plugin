package explore

import (
	"os"
	"testing"

	"modelcheck/ir"
)

// TestGenerateTestdata writes the IR-encoded counters models used by the
// feature file and by the throughput measurement, and the G7 models (por-visible, por-enabling, por-dstep, por-dormant). Run explicitly:
//
//	MCD_GEN_TESTDATA=1 go test ./explore -run TestGenerateTestdata
func TestGenerateTestdata(t *testing.T) {
	if os.Getenv("MCD_GEN_TESTDATA") == "" {
		t.Skip("set MCD_GEN_TESTDATA=1 to regenerate testdata/ir")
	}
	for _, c := range []struct {
		k, n int
		file string
	}{{10, 5, "counters-10-5.json"}, {10, 6, "counters-10-6.json"}} {
		b, err := ir.MarshalJSON(counters(c.k, c.n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("../testdata/ir/"+c.file, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for file, m := range map[string]*ir.Model{
		"por-visible.json":  porVisible(),
		"por-enabling.json": porEnabling(),
		"por-dstep.json":    porDStepGuard(),
		"por-dormant.json":  porDormantReentry(),
	} {
		b, err := ir.MarshalJSON(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile("../testdata/ir/"+file, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
