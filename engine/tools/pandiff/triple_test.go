package pandiff

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// tripleTable mirrors the differential outline of features/g4-ltl.feature;
// the rows are printed (go test -v) for steps/g4-confirmation.md.
var tripleTable = []struct {
	model, formula, defines, fairness, mode string
}{
	{"CH4/prop.pml", "[]p", "", "none", "a"},
	{"CH4/prop.pml", "<>[]p", "", "none", "a"},
	{"CH4/prop.pml", "[]<>p", "", "none", "a"},
	{"CH4/prop.pml", "<>!p", "", "none", "a"},
	{"App_A/example", "<>[]p", "", "none", "a"},
	{"App_A/example", "[]<>p", "", "none", "a"},
	{"App_A/example", "[]<>!p", "", "none", "a"},
	{"App_A/example", "X p", "", "none", "a"},
	{"App_A/example", "X X p", "", "none", "a"},
	{"App_A/example", "p U (x == 1)", "", "none", "a"},
	{"App_A/example", "", "", "none", "a"},
	{"CH8/trivial.pml", "", "", "none", "a"},
	{"CH8/trivial.pml", "", "", "weak", "a"},
	{"CH8/trivial.pml", "[]<>x", "", "none", "a"},
	{"CH8/trivial.pml", "[]<>x", "", "weak", "a"},
	{"CH8/trivial.pml", "[]<>!x", "", "weak", "a"},
	{"CH8/fairness.pml", "", "", "none", "a"},
	{"CH8/fairness.pml", "", "", "weak", "a"},
	{"CH8/example.pml", "", "", "none", "a"},
	{"CH4/fair_accept.pml", "", "", "none", "a"},
	{"CH4/fair_accept.pml", "", "", "weak", "a"},
	{"CH4/fair.pml", "[]<>(x == 1)", "", "none", "a"},
	{"CH4/fair.pml", "[]<>(x == 1)", "", "weak", "a"},
	{"CH4/fair.pml", "<>[](x == 1)", "", "none", "a"},
	{"CH4/fair.pml", "", "", "none", "l"},
	{"CH4/fair.pml", "", "", "weak", "l"},
	{"CH4/dijkstra_progress.pml", "", "", "none", "l"},
	{"CH4/dijkstra_progress.pml", "", "", "weak", "l"},
	{"CH4/dijkstra_progress.pml", "[]<>(len(sema) == 0)", "", "none", "a"},
	{"CH4/true.pml", "[]true", "", "none", "a"},
	{"CH4/false.pml", "[]true", "", "none", "a"},
	{"CH3/alternatingbit.pml", "[] (full1 -> <> empty1)", "full1=(len(to_rcvr) > 0); empty1=(len(to_rcvr) == 0)", "none", "a"},
	{"CH3/alternatingbit.pml", "[] (full1 -> <> empty1)", "full1=(len(to_rcvr) > 0); empty1=(len(to_rcvr) == 0)", "weak", "a"},
	{"CH3/alternatingbit.pml", "[]<> two", "two=(len(to_rcvr) == 2)", "none", "a"},
	{"testdata:starvation.pml", "<>done", "", "none", "a"},
	{"testdata:starvation.pml", "<>done", "", "weak", "a"},
	{"testdata:starvation.pml", "done U (done)", "", "none", "a"},
	{"testdata:starvation.pml", "[]!done", "", "none", "a"},
	{"testdata:leader3.pml", "<>[]oneLeader", "oneLeader=(nr_leaders == 1)", "none", "a"},
	{"testdata:leader3.pml", "<>[]oneLeader", "oneLeader=(nr_leaders == 1)", "weak", "a"},
	{"testdata:leader3.pml", "[]noLeader", "noLeader=(nr_leaders == 0)", "none", "a"},
	{"testdata:leader3.pml", "<>elected", "elected=(nr_leaders > 0)", "none", "a"},
	{"testdata:leader3.pml", "<>elected", "elected=(nr_leaders > 0)", "weak", "a"},
}

func TestDifferentialTriples(t *testing.T) {
	tools := Tools{}
	if !tools.Available() {
		t.Skip("spin or gcc not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var rows []string
	for _, c := range tripleTable {
		model := "../../../../Promela - examples/" + c.model
		if strings.HasPrefix(c.model, "testdata:") {
			model = "../../testdata/promela/" + strings.TrimPrefix(c.model, "testdata:")
		}
		if _, err := os.Stat(model); err != nil {
			t.Fatalf("%s: %v", c.model, err)
		}
		var defines []string
		for _, d := range strings.Split(c.defines, ";") {
			if d = strings.TrimSpace(d); d != "" {
				defines = append(defines, d)
			}
		}
		tr, err := RunTriple(ctx, tools, model, defines, c.formula, c.fairness, c.mode)
		if err != nil {
			t.Fatalf("%s %q: %v", c.model, c.formula, err)
		}
		rows = append(rows, tr.Row())
		if !tr.Agree {
			t.Errorf("DISAGREE: %s", tr.Row())
		}
	}
	t.Log("\n" + strings.Join(rows, "\n"))
}

func TestStripNever(t *testing.T) {
	src := "byte x;\n#ifdef PHI\nnever { accept: do :: (x) od }\n#else\nnever {\n do :: !x -> break :: true od\n}\n#endif\ninit { x = 1 }\n"
	got := string(stripNever([]byte(src)))
	if strings.Contains(got, "never") || !strings.Contains(got, "init { x = 1 }") || !strings.Contains(got, "#ifdef PHI") {
		t.Fatalf("got %q", got)
	}
}
