package pandiff

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const corpus = "../../../../Promela - examples"

const c0Sample = `pan:1: assertion violated (cnt==1) (at depth 53)
	assertion violations	+
	invalid end states	+
State-vector 28 byte, depth reached 65, errors: 8
      429 states, stored
      430 states, matched
      859 transitions (= stored+matched)
`

const dSample = `proctype user
	state   1 -(tr   3)-> state   5  [id   0 tp   2] [----G] model.pml:5 => x = me
	state   5 -(tr   4)-> state   1  [id   1 tp   2] [----G] model.pml:7 => (((y!=0)&&(y!=me)))
	state  21 -(tr  14)-> state   1  [id  20 tp   2] [----G] model.pml:24 => cnt = (cnt-1)
init
	state   1 -(tr   3)-> state   2  [id  11 tp   2] [----L] model.pml:12 => (run Euclid(36,12))
	state   2 -(tr   4)-> state   0  [id  12 tp 3500] [--e-L] model.pml:12 => -end-
claim never_0
	state   1 -(tr   1)-> state   1  [id   0 tp   2] [----G] model.pml:17 => ((x!=0))

Transition Type: A=atomic; D=d_step; L=local; G=global
`

func TestParsePanOutputs(t *testing.T) {
	stored, matched, errs, err := ParseC0(c0Sample)
	if err != nil || stored != 429 || matched != 430 || errs != 8 {
		t.Fatalf("%d %d %d %v", stored, matched, errs, err)
	}
	if _, class := ParseFirst(c0Sample, 8); class != "assertion violated" {
		t.Fatalf("class %q", class)
	}
	if first, class := ParseFirst("pan:1: invalid end state (at depth 10)\n", 1); class != "invalid end state" || first != "invalid end state" {
		t.Fatalf("%q %q", first, class)
	}
	if _, class := ParseFirst("no pan line", 0); class != "no error" {
		t.Fatalf("class %q", class)
	}
	if _, class := ParseFirst("pan:1: block in d_step seq (at depth 2)\n", 1); class != "block in d_step seq" {
		t.Fatalf("class %q", class)
	}
	stmts, order := ParseD(dSample)
	if len(order) != 3 || order[0] != "user" || order[1] != ":init:" || order[2] != "claim never_0" {
		t.Fatalf("order %v", order)
	}
	if len(stmts["user"]) != 3 || stmts["user"][1].Line != 7 || stmts["user"][1].Text != "(((y!=0)&&(y!=me)))" || stmts["user"][2].From != 21 {
		t.Fatalf("user %+v", stmts["user"])
	}
	if stmts[":init:"][1].Text != "-end-" || stmts[":init:"][1].Flags != "--e-L" {
		t.Fatalf("init %+v", stmts[":init:"])
	}
}

func TestCompareRowsAndStatements(t *testing.T) {
	pan := &PanResult{Stored: 5, Errors: 1, Class: "invalid end state",
		Statements: map[string][]PanTransition{"A": {{Line: 2, Text: "x = 1"}, {Line: 3, Text: "-end-"}}}, Order: []string{"A"}}
	eng := &EngineResult{States: 5, Complete: true, Class: "invalid end state",
		Statements: map[string][]EngineEdge{"A": {{Line: 2, Text: "x = 1"}, {Line: 3, Text: "-end-"}}}, Order: []string{"A"}}
	c := Compare(pan, eng)
	if !c.Agree || !c.StmtAgree {
		t.Fatalf("should agree:\n%s", c.Table("m"))
	}
	eng.States = 6
	eng.Statements["A"] = append(eng.Statements["A"], EngineEdge{Line: 9, Text: "extra"})
	c = Compare(pan, eng)
	if c.Agree || c.StmtAgree || len(c.Stmts[0].OnlyEngine) != 1 || c.Stmts[0].OnlyEngine[0] != 9 {
		t.Fatalf("should disagree:\n%s", c.Table("m"))
	}
	eng.Complete = false
	c = Compare(pan, eng)
	if !c.Rows[2].Agree {
		t.Fatalf("state count is not compared when the engine's search was not complete")
	}
}

// differentialFiles are the corpus files whose verdict, error class and state
// count TestDifferentialCorpus compares with pan's.
var differentialFiles = []string{
	"CH2/mutex_flaw.pml", "CH2/peterson.pml", "CH2/prodcons.pml", "CH3/alternatingbit.pml",
	"CH2/peterson2.pml", "CH2/mutex.pml", "CH2/protocol", "CH2/protocol2", "CH2/false.pml",
	"CH2/hello.pml", "CH2/hello2.pml", "CH3/alternatingbit2.pml", "CH3/counter3.pml", "CH3/counter4.pml",
	"CH3/euclid.pml", "CH3/macro.pml", "CH3/mtype.pml", "CH3/rendezvous.pml", "CH3/send_recv.pml",
	"CH3/you_run.pml", "CH3/you_run2.pml",
	"App_C/petrinet1", "App_C/petrinet2",
	"../model-check-plugin/engine/testdata/promela/atomic-t1.pml",
	"../model-check-plugin/engine/testdata/promela/atomic-t3.pml",
	"../model-check-plugin/engine/testdata/promela/atomic-t4.pml",
	"../model-check-plugin/engine/testdata/promela/atomic-t5.pml",
	"../model-check-plugin/engine/testdata/promela/atomic-t6.pml",
	"../model-check-plugin/engine/testdata/promela/atomic-at.pml",
}

// Every remaining corpus file the frontend accepts and whose verdict a
// plain `pan -c0` can answer. A model with a never claim or accept
// labels is not here: plain pan does not look for acceptance cycles, so
// the two sides would be answering different questions — those files are
// compared by the differential triples of G4 (`pandiff -mode a`,
// TestDifferentialTriples). The rest of the exclusions are named in
// steps/g5-confirmation.md §3.2.
//
// Unlocked by the v1 subset of G5. CH15/client_server.pml predates
// SPIN 6, where `return` became a reserved word: without the rename
// `spin -a` refuses the file, so both sides get the same rename and
// compare the same model.
// stmtExempt: the verdict, the error class and the state count are
// compared for every file; the statement table is not compared for
// CH9/leader.pml, where pan emits two control transitions of its own
// for a `break` that ends an option *inside* an `atomic` sequence
// ("goto :b1" at the break's line and "break" at the do's line). They
// are the same class of artefact as the ".(goto)" join the parser
// already drops — unstored atomic steps with no statement behind them —
// but they cannot be told apart from the transitions pan emits for a
// *lone* `:: break` option, which the engine does produce. The counts
// agree (41692 = pan), so the difference is in the diagnostic table
// only; see steps/g5-confirmation.md.
type differentialEntry struct {
	file       string
	defines    []string
	stmtExempt bool
}

// differentialV1 are the files the v1 subset of G5 unlocked.
var differentialV1 = []differentialEntry{
	{file: "CH2/prodcons2.pml"},
	{file: "CH3/inline.pml"},
	{file: "CH3/inline2.pml"},
	{file: "CH3/typedef.pml"},
	{file: "CH3/toggle.pml"},
	{file: "CH3/rendezvous2.pml"},
	{file: "CH4/dijkstra.pml"},
	{file: "CH4/dijkstra_progress.pml"},
	{file: "CH4/fair.pml"},
	{file: "CH4/true.pml"},
	{file: "CH4/false.pml"},
	{file: "CH8/example.pml"},
	{file: "CH9/merging.pml"},
	{file: "CH14/v14_4.pml"},
	{file: "CH5/pathfinder.pml"},
	{file: "CH5/diskhead.pml"},
	{file: "CH9/leader.pml", stmtExempt: true},
	{file: "CH14/version1"},
	{file: "CH14/version2"},
	{file: "CH14/version3"},
	{file: "CH14/version4"},
	{file: "CH15/client_server.pml", defines: []string{"return=ret_"}},
}

// Differential test on the chapter 2–3 corpus (plan 14 §8.1). Skipped when
// spin or gcc is missing.
func TestDifferentialCorpus(t *testing.T) {
	tools := Tools{}
	if !tools.Available() {
		t.Skip("spin/gcc not on PATH")
	}
	if _, err := os.Stat(corpus); err != nil {
		t.Skip("corpus not found")
	}
	files := differentialFiles
	v1 := differentialV1
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	for _, f := range files {
		cmp, _, _, err := Run(ctx, tools, filepath.Join(corpus, f), nil, false)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if !cmp.Agree || !cmp.StmtAgree {
			t.Errorf("%s:\n%s", f, cmp.Table(f))
		}
	}
	for _, c := range v1 {
		cmp, _, _, err := Run(ctx, tools, filepath.Join(corpus, c.file), c.defines, false)
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		if !cmp.Agree || (!cmp.StmtAgree && !c.stmtExempt) {
			t.Errorf("%s:\n%s", c.file, cmp.Table(c.file))
		}
	}
}

// The parallel search against pan (performance plan 5): for the models of the
// chapter 2-3 corpus, the atomic fixtures and a few of the larger ones, the
// number of stored states of the parallel search, at 1 and 4 workers, is the
// sequential engine's and so pan's `pan -c0`, with the same error class and the
// same completeness. Skipped when spin or gcc is missing.
func TestDifferentialCorpusParallel(t *testing.T) {
	tools := Tools{}
	if !tools.Available() {
		t.Skip("spin/gcc not on PATH")
	}
	if _, err := os.Stat(corpus); err != nil {
		t.Skip("corpus not found")
	}
	files := []struct {
		file    string
		defines []string
	}{
		{file: "CH2/mutex_flaw.pml"}, {file: "CH2/peterson.pml"}, {file: "CH2/prodcons.pml"}, {file: "CH3/alternatingbit.pml"},
		{file: "CH2/peterson2.pml"}, {file: "CH2/mutex.pml"}, {file: "CH3/alternatingbit2.pml"}, {file: "CH3/counter3.pml"},
		{file: "CH3/euclid.pml"}, {file: "CH3/rendezvous.pml"}, {file: "CH3/you_run.pml"}, {file: "App_C/petrinet1"}, {file: "App_C/petrinet2"},
		{file: "../model-check-plugin/engine/testdata/promela/atomic-t3.pml"},
		{file: "../model-check-plugin/engine/testdata/promela/atomic-t4.pml"},
		{file: "../model-check-plugin/engine/testdata/promela/atomic-t6.pml"},
		{file: "CH3/rendezvous2.pml"}, {file: "CH8/example.pml"}, {file: "CH9/merging.pml"}, {file: "CH9/leader.pml"},
		{file: "CH5/pathfinder.pml"}, {file: "CH15/client_server.pml", defines: []string{"return=ret_"}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	compared := 0
	for _, f := range files {
		path := filepath.Join(corpus, f.file)
		dir := t.TempDir()
		pan, err := tools.RunSpin(ctx, path, f.defines, dir)
		if err != nil {
			t.Errorf("%s: %v", f.file, err)
			continue
		}
		seq, err := RunEngine(ctx, path, f.defines, 0)
		if err != nil {
			t.Errorf("%s: %v", f.file, err)
			continue
		}
		if !Compare(pan, seq).Agree {
			continue // the sequential engine disagrees with pan: the existing differential test says so
		}
		compared++
		for _, w := range []int{1, 4} {
			par, err := RunEngineWorkers(ctx, path, f.defines, 0, w)
			if err != nil {
				t.Errorf("%s with %d workers: %v", f.file, w, err)
				continue
			}
			if par.States != seq.States || par.Class != seq.Class || par.Complete != seq.Complete || par.States != pan.Stored {
				t.Errorf("%s with %d workers: %d states, class %q, complete %v; sequential %d, %q, %v; pan stores %d", f.file, w, par.States, par.Class, par.Complete, seq.States, seq.Class, seq.Complete, pan.Stored)
			}
		}
	}
	t.Logf("%d of %d models compared with pan and the sequential engine", compared, len(files))
	if compared < len(files)-3 {
		t.Fatalf("only %d of %d models were compared", compared, len(files))
	}
}
