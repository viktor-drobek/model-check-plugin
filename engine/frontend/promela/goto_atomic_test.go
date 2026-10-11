package promela

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"modelcheck/ir"
)

// gotoAtomicSources are models SPIN refuses ("label placed incorrectly",
// "jump into d_step sequence") and this engine accepts with the documented
// semantics; they cannot be compared with pan, so they are not fixtures of
// testdata/promela/goto-atomic (the corpus comparisons walk it).
var gotoAtomicSources = map[string]string{
	// a goto that is the whole of a block, as a link of a chain inside
	// blocks: the lexical place of the goto counts for the chain
	"o1-goto-opens-block-in-chain.pml": `byte n;
active proctype A() {
  atomic { n = 1; goto L };
  n = 5;
  atomic { L: goto M };
  atomic { skip; M: n = 2; n = 3 }
}
active proctype B() { assert(n != 1) }
`,
	// a label on a d_step block itself: the jump lands in front of it
	"d1-label-on-dstep-block.pml": `byte n; byte m;
active proctype A() { goto L; n = 5; L: d_step { n = 2; n = 3 } }
active proctype B() { m = 1; m = 2 }
`,
}

// gotoAtomic lowers one fixture of testdata/promela/goto-atomic, or one of
// gotoAtomicSources.
func gotoAtomic(t *testing.T, name string) (*ir.Model, *Error) {
	t.Helper()
	src := []byte(gotoAtomicSources[name])
	if len(src) == 0 {
		var err error
		src, err = os.ReadFile(filepath.Join("..", "..", "testdata", "promela", "goto-atomic", name))
		if err != nil {
			t.Fatal(err)
		}
	}
	res, perr := Parse(src, name, nil)
	if perr != nil {
		return nil, perr
	}
	return res.Model, nil
}

// TestGotoAtomicRefusals (issue #6): the location of a label a forward goto
// pre-allocated takes the blocks of the position it stands for, so a loop at
// the start of an atomic block that shares its entry is refused with or
// without the goto, wherever the goto stands; a goto that keeps the control
// into the start of a block with a shared entry is refused the same way; a
// goto to a label on the first statement of an option with other
// alternatives is refused; a goto across a d_step boundary is refused as
// SPIN refuses it.
func TestGotoAtomicRefusals(t *testing.T) {
	for name, want := range map[string]string{
		"m1-issue.pml":                        "loop at the start of an atomic",
		"m6-goto-sibling-block.pml":           "loop at the start of an atomic",
		"m7-backward-goto-atomic-loop.pml":    "loop at the start of an atomic",
		"m9-gpt-inside-shared-head.pml":       "loop at the start of an atomic",
		"m22-goto-to-block-start-shared.pml":  "goto to the start of an atomic",
		"m33-goto-option-head-shared.pml":     "label on the first statement of an option",
		"m34-label-on-atomic-option-head.pml": "label on the first statement of an option",
		"m36-goto-if-option-head-shared.pml":  "label on the first statement of an option",
		"m37-backward-goto-option-head.pml":   "label on the first statement of an option",
	} {
		_, perr := gotoAtomic(t, name)
		if perr == nil || perr.Kind != KindOutside || !strings.Contains(perr.Message, want) {
			t.Errorf("%s: %v, want outside-subset mentioning %q", name, perr, want)
		}
	}
	for _, name := range []string{"m4-issue-dstep.pml", "m10-goto-out-of-dstep.pml", "m11-goto-into-dstep-own.pml",
		"m20-goto-into-dstep-first.pml", "m21-goto-into-dstep-first-loop.pml"} {
		_, perr := gotoAtomic(t, name)
		if perr == nil || perr.Kind != KindSemantic || !strings.Contains(perr.Message, "d_step boundary") {
			t.Errorf("%s: %v, want a semantic refusal of the jump across the d_step", name, perr)
		}
	}
}

// TestGotoEdgeKeepsTheControlByWhereItIsWritten (issue #6): the edge that
// ends in a goto keeps the exclusive control when the goto is written inside
// an atomic block and the location its label resolves to is inside one (or
// is the location in front of the label's block), and gives it up when the
// goto is outside — a jump into a block gets the control with the statement
// it lands on, as pan — or when the label leads out of the block; whether
// the goto opens an option (its own edge) or follows a statement (that
// statement's edge ends in it).
func TestGotoEdgeKeepsTheControlByWhereItIsWritten(t *testing.T) {
	cases := []struct {
		file   string
		atomic map[string]bool // edge text → keeps the control
		dstep  bool            // the control of a d_step (Edge.DStep), not of an atomic
	}{
		// a goto from outside into the middle of a block, as an option
		{"m3-goto-option-into-block.pml", map[string]bool{"goto L": false, "n = 1": true, "n = 2": true, "n = 3": false}, false},
		// the same goto after a statement: that statement's edge ends in it
		{"m16-nonfirst-goto-into-atomic.pml", map[string]bool{"n = 7": false, "n = 2": true, "n = 3": false}, false},
		// a goto out of a block, as an option
		{"m18-first-goto-out-of-atomic.pml", map[string]bool{"goto L": false, "n = 1": true, "n = 5": false}, false},
		// a goto after a statement from one block into another
		{"m19-nonfirst-goto-atomic-to-atomic.pml", map[string]bool{"n = 1": true, "n = 2": true}, false},
		// a backward goto, as an option, to the loop at the start of its block
		{"m14-backward-goto-first-option-inside.pml", map[string]bool{"goto L": true, "n++": true, "else": false}, false},
		// a backward goto after a statement to the loop at the start of its block
		{"m17-backward-goto-atomic-loop-own.pml", map[string]bool{"n = 0": true, "k++": true, "n = 1": true}, false},
		// a goto that is the whole of a block after a statement: the statement
		// stands in front of the block and gives the control up
		{"m23-goto-opens-own-atomic.pml", map[string]bool{"n = 7": false, "n = 2": true, "n = 3": false}, false},
		// a goto from outside to a label on a goto inside: every goto of the
		// chain has to be inside, so nothing is kept
		{"m24-goto-chain-label-on-goto.pml", map[string]bool{"n = 7": false, "n = 2": true}, false},
		// a chain of gotos inside blocks that lands on the first statement of
		// a third block keeps the control; one through a goto outside gives it up
		{"m35-goto-chain-into-block-entry.pml", map[string]bool{"n = 1": true, "n = 2": true}, false},
		{"m38-goto-chain-through-outside.pml", map[string]bool{"n = 1": false, "n = 2": true}, false},
		// a chain whose middle link alias compresses away (L: goto K after K:
		// goto M) still sees the link outside; a label on a block that
		// shares its node with the label on the block's first do keeps the
		// two apart (the block's label gives the control up)
		{"m40-goto-chain-alias-shortcut.pml", map[string]bool{"n = 1": false, "n = 2": true}, false},
		{"m39-label-on-block-and-inner-do.pml", map[string]bool{"n = 1": false, "n = 2": true}, false},
		// an edge that goes round a loop at the start of a block through a
		// chain with a goto written outside gives the control up there
		{"m43-keep-loop-through-outside-goto.pml", map[string]bool{"n = 1": false, "n = 0": false}, false},
		// a goto inside one d_step: no boundary crossed
		{"m41-goto-inside-same-dstep.pml", map[string]bool{"n = 1": true, "n = 5": true, "n = 2": false}, true},
		// SPIN refuses these; the engine lowers them by the documented rule
		{"o1-goto-opens-block-in-chain.pml", map[string]bool{"n = 1": true, "n = 2": true, "n = 3": false}, false},
		{"d1-label-on-dstep-block.pml", map[string]bool{"n = 2": true, "n = 3": false}, true},
		// a backward goto past a nested block to the loop at the start of its block
		{"m26-nested-atomic-backward-goto.pml", map[string]bool{"n = 0": true, "n = 1": true}, false},
		// a label on a goto out of the block, on a break out of it, on a plain
		// block that jumps out, and on an atomic block itself: the goto lands
		// outside and gives the control up
		{"m29-goto-chain-leaves-block.pml", map[string]bool{"n = 1": false}, false},
		{"m30-label-on-break-exit-outside.pml", map[string]bool{"n = 1": false, "n = 3": false}, false},
		{"m31-plain-block-chain-out.pml", map[string]bool{"n = 1": false}, false},
		{"m32-label-on-atomic-block.pml", map[string]bool{"n = 1": false, "n = 2": true, "n = 3": false}, false},
	}
	for _, c := range cases {
		m, perr := gotoAtomic(t, c.file)
		if perr != nil {
			t.Errorf("%s: %v", c.file, perr)
			continue
		}
		seen := map[string]bool{}
		for _, p := range m.Processes {
			for _, e := range p.Edges {
				want, ok := c.atomic[e.Text]
				if !ok {
					continue
				}
				if seen[e.Text] {
					t.Errorf("%s: the text %q names more than one edge", c.file, e.Text)
				}
				seen[e.Text] = true
				got := e.Atomic
				if c.dstep {
					got = e.DStep
				}
				if got != want {
					t.Errorf("%s: the edge %q keeps the control: %v, want %v", c.file, e.Text, got, want)
				}
			}
		}
		for text := range c.atomic {
			if !seen[text] {
				t.Errorf("%s: no edge %q", c.file, text)
			}
		}
	}
}
