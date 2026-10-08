package explore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"modelcheck/ir"
)

// O2, the semantic audit of the ample sets (performance plan, step 6, section
// 7.2).
//
// For every stored state of the FULL graph and every process the plan calls
// eligible there (the eligibility table and the channel requirement of pick,
// a move enabled; the cycle proviso is ignored, because it depends on the
// search stack and C0 to C2 depend on the state only), the audit RUNS the
// model and checks, for every sequence of up to K macro-steps of the other
// processes, the things the reduction relies on:
//
//   - the macro-steps of the process are the same sets of edge sequences
//     (a step of the process that has become an error or a failed assert
//     changes its signature, so the process's own errors are covered);
//   - each of them commutes with the sequence, up to the exclusive byte;
//   - an erroring step of the others still errs with the process's step first,
//     and a clean one stays clean;
//   - no macro-step of the process changes the value of an invariant or of a
//     reach expression.
//
// It reads nothing of the analysis but the eligibility table and the channel
// requirements. The macro-steps are enumerated by Stepper's Enabled and Apply
// with intermediate() deciding where a chain ends. So, on the shapes the
// generators make, a hole in the footprints (a cell nobody reads, a
// continuation nobody follows) shows as soon as one state exhibits it,
// including in states the reduced search never visits, where the verdict oracle
// sees it only when the hole changes a verdict. A hole in a clause whose shape
// no generator makes is invisible to it, and to the other oracles: such shapes
// are pinned by directed tests only (steps/perf6-confirmation.md, "What the
// oracles can and cannot see").
//
// What it does not do: it shares the engine's enabledness and firing code, so
// a misreading of the semantics shared by the full and the reduced search is
// invisible to it (that is what the SPIN comparison is for), and it checks C1
// to depth K on the states it visits, it does not prove it.

// macroPath is one macro-step: the micro-moves of one process from a stored
// state to the next stored state (more than one inside an atomic sequence).
type macroPath struct {
	moves []Move
	end   []byte
	bad   bool // an error or a failed assert on the way: no end state
}

func (p macroPath) sig() string {
	var b strings.Builder
	if p.bad {
		b.WriteString("!")
	}
	for _, m := range p.moves {
		fmt.Fprintf(&b, "%d/%d", m.Edge.Proc, m.Edge.Edge)
		if m.Partner != nil {
			fmt.Fprintf(&b, "~%d/%d", m.Partner.Proc, m.Partner.Edge)
		}
		b.WriteByte(',')
	}
	return b.String()
}

// auditMaxChain bounds a macro-step the audit follows: a chain that is longer
// is reported as bad, like an error.
const auditMaxChain = 200

type auditCtx struct {
	st     *Stepper
	canon  func([]byte) string
	budget int // sequences left for the state being audited
}

// paths lists the macro-steps from a stored state whose first move is made by
// a process in who. A step that ends in an error or a failed assert is listed
// too, marked bad (with the moves made so far): whether it is bad must not
// depend on the order in which the others moved either.
func (a *auditCtx) paths(state []byte, who func(proc int) bool) (out []macroPath, bad bool) {
	moves, err := a.st.Enabled(state)
	if err != nil {
		return []macroPath{{bad: true}}, true
	}
	var rec func(prefix []Move, from []byte, mv Move, depth int)
	rec = func(prefix []Move, from []byte, mv Move, depth int) {
		pre := append(append([]Move(nil), prefix...), mv)
		if depth > auditMaxChain {
			out = append(out, macroPath{moves: pre, bad: true})
			return
		}
		next, failed, err := a.st.Apply(from, mv)
		if err != nil || failed != nil {
			out = append(out, macroPath{moves: pre, bad: true})
			return
		}
		inter, err := a.st.s.intermediate(next)
		if err != nil {
			out = append(out, macroPath{moves: pre, bad: true})
			return
		}
		if !inter {
			out = append(out, macroPath{moves: pre, end: next})
			return
		}
		more, err := a.st.Enabled(next)
		if err != nil {
			out = append(out, macroPath{moves: pre, bad: true})
			return
		}
		for _, m2 := range more {
			rec(pre, next, m2, depth+1)
		}
	}
	for _, mv := range moves {
		if who(mv.Edge.Proc) {
			rec(nil, state, mv, 0)
		}
	}
	for _, p := range out {
		if p.bad {
			bad = true
		}
	}
	return out, bad
}

// replayPath applies the micro-moves of p, by edge, from state. ok is false
// when one of them is not enabled there; bad when the path ends in an error or
// a failed assert.
func (a *auditCtx) replayPath(state []byte, p macroPath) (end []byte, ok bool, bad bool) {
	cur := state
	for _, want := range p.moves {
		moves, err := a.st.Enabled(cur)
		if err != nil {
			return nil, true, true
		}
		var found *Move
		for i := range moves {
			if moves[i].Edge == want.Edge && samePartner(moves[i].Partner, want.Partner) {
				found = &moves[i]
				break
			}
		}
		if found == nil {
			return nil, false, false
		}
		next, failed, err := a.st.Apply(cur, *found)
		if err != nil || failed != nil {
			return nil, true, true
		}
		cur = next
	}
	return cur, true, false
}

func sigs(ps []macroPath) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.sig()
	}
	sort.Strings(s)
	return strings.Join(s, "|")
}

func seqSig(ps []macroPath) string {
	var s []string
	for _, p := range ps {
		s = append(s, "["+p.sig()+"]")
	}
	return strings.Join(s, "")
}

// auditState checks stored state s, in which process p would be expanded
// alone. K is how many macro-steps of the others are tried in a row; vis gives
// the value of every invariant and reach expression in a state.
func (a *auditCtx) auditState(s []byte, p int, K int, vis func([]byte) []bool) error {
	mine, bad := a.paths(s, func(q int) bool { return q == p })
	if bad {
		return nil // an error or a failed assert: pick never expands such a state alone (the proviso)
	}
	if len(mine) == 0 {
		return fmt.Errorf("process %d is expanded alone but has no macro-step", p)
	}
	want := sigs(mine)
	v0 := vis(s)
	for _, m := range mine {
		v1 := vis(m.end)
		for i := range v0 {
			if v0[i] != v1[i] {
				return fmt.Errorf("C2: the macro-step %s of the process expanded alone changes property %d", m.sig(), i)
			}
		}
	}
	var rec func(cur []byte, seq []macroPath, depth int) error
	rec = func(cur []byte, seq []macroPath, depth int) error {
		if a.budget--; a.budget < 0 {
			return nil
		}
		others, _ := a.paths(cur, func(q int) bool { return q != p })
		for _, o := range others {
			nseq := append(append([]macroPath(nil), seq...), o)
			if o.bad {
				// An error of the others must not depend on what p did first.
				for _, m := range mine {
					mid, ok, _ := a.replayPath(cur, m)
					if !ok {
						return fmt.Errorf("C1: %s is not enabled after %s", m.sig(), seqSig(seq))
					}
					_, ok2, bad2 := a.replayPath(mid, o)
					if !ok2 || !bad2 {
						return fmt.Errorf("C1: %s ends in an error, but not after %s", seqSig(nseq), m.sig())
					}
				}
				continue
			}
			next := o.end
			mp, _ := a.paths(next, func(q int) bool { return q == p })
			if got := sigs(mp); got != want {
				return fmt.Errorf("C1: after %s the macro-steps of the process expanded alone are {%s}, they were {%s}", seqSig(nseq), got, want)
			}
			for _, m := range mine {
				viaOthers, ok, _ := a.replayPath(next, m)
				if !ok {
					return fmt.Errorf("C1: %s is no longer enabled after %s", m.sig(), seqSig(nseq))
				}
				viaMine, ok, _ := a.replayPath(s, m)
				if !ok {
					return fmt.Errorf("C1: %s is not replayable", m.sig())
				}
				for _, q := range nseq {
					var bd bool
					viaMine, ok, bd = a.replayPath(viaMine, q)
					if !ok || bd {
						return fmt.Errorf("C1: %s is no longer executable after %s", seqSig(nseq), m.sig())
					}
				}
				if a.canon(viaOthers) != a.canon(viaMine) {
					return fmt.Errorf("C1: %s and %s do not commute", m.sig(), seqSig(nseq))
				}
			}
			if depth+1 < K {
				if err := rec(next, nseq, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return rec(s, nil, 0)
}

// auditModel runs the full search of m and audits every stored state against
// plan: every process the plan makes eligible there. It returns the number of
// (state, process) pairs audited, how many of those have the process standing
// at a run edge, and the first violation.
func auditModel(m *ir.Model, plan *porPlan, K int) (audited, atRun int, err error) {
	c, err := compile(m)
	if err != nil {
		return 0, 0, err
	}
	if plan == nil {
		plan = analyzePOR(c)
	}
	if plan.reason != "" || !plan.any {
		return 0, 0, nil
	}
	opt := Options{Sweep: true, Budget: Budget{MaxStates: 3000, MaxDepth: 3000}}
	var rec *recorder
	opt.NewVisited = func(n int) Visited { rec = &recorder{Visited: defaultVisited(n)}; return rec }
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	full, err := Run(ctx, m, opt)
	if err != nil || !full.Complete {
		return 0, 0, nil
	}
	st, err := NewStepper(m)
	if err != nil {
		return 0, 0, err
	}
	l := st.Layout()
	canon := func(b []byte) string {
		x := append([]byte(nil), b...)
		x[l.Excl] = 0 // two orders of commuting steps can leave the byte differently: the same state (Lemma E)
		return string(x)
	}
	cc, err := compile(m) // a layout of its own for the properties: Truth sets Timeout
	if err != nil {
		return 0, 0, err
	}
	vis := func(state []byte) []bool {
		var out []bool
		cc.layout.Timeout = false
		for _, ip := range cc.invs {
			ok, _ := ip.expr.Truth(state)
			out = append(out, ok)
		}
		for _, rp := range cc.reaches {
			ok, _ := rp.expr.Truth(state)
			out = append(out, ok)
		}
		return out
	}
	if K > 2 && len(rec.states) > 500 {
		K = 2 // depth 3 on the small models only: the cost grows with the branching
	}
	a := &auditCtx{st: st, canon: canon}
	s := &search{c: c, cur: make([]byte, l.Size), next: make([]byte, l.Size)}
	run := &porRun{plan: plan, noProviso: true}
	for _, state := range rec.states {
		copy(s.cur, state)
		for p := range c.procs {
			if c.procs[p].claim {
				continue
			}
			loc := l.ReadPC(state, p)
			if !plan.eligible[p][loc] || !run.channelsCanAct(s, plan.req[p][loc]) {
				continue
			}
			if mv, ok := run.movesOf(s, p); !ok || len(mv) == 0 {
				continue
			}
			audited++
			for _, ei := range c.procs[p].out[loc] {
				if c.procs[p].edge[ei].e.Run != nil {
					atRun++
					break
				}
			}
			a.budget = 20000
			if e := a.auditState(state, p, K, vis); e != nil {
				return audited, atRun, fmt.Errorf("state %x, process %d (%s) at location %d: %w", state, p, m.Processes[p].Name, loc, e)
			}
		}
	}
	return audited, atRun, nil
}

func porAuditK() int {
	if v, ok := envInt("MCD_POR_AUDIT_K"); ok {
		return int(v)
	}
	return 2
}

func TestPORAuditOfTheEligibleProcesses(t *testing.T) {
	for _, g := range porGeneratorsToRun() {
		t.Run(g.name, func(t *testing.T) {
			K := porAuditK()
			// The nrpr models are small but have many states per model (every end
			// edge writes the table, so little is eligible and the audit walks the
			// whole graph): 150 of them cost what 1 000 of the others do.
			models := 1000
			if g.name == "nrpr" {
				models = 150
			}
			tl := porForSeeds(t, "O2", g, models, func(m *ir.Model, tl *porTally) (porOutcome, error) {
				n, r, err := auditModel(m, nil, K)
				tl.audited += n
				tl.atRun += r
				return porUncounted, err
			})
			if n, ok := porAuditFloor[g.name]; ok && tl.failures == 0 && tl.audited < tl.models/n {
				t.Fatalf("only %d (state, process) pairs were audited in %d models: the generator no longer exercises the rules", tl.audited, tl.models)
			}
			if n, ok := porAtRunFloor[g.name]; ok && tl.failures == 0 && tl.models >= porAtRunMinModels && tl.atRun < tl.models/n {
				t.Fatalf("only %d pairs with the process at a run edge in %d models: the footprint of the run itself is not being tested", tl.atRun, tl.models)
			}
		})
	}
}

// porAuditFloor is, per generator, how few audited (state, process) pairs per
// model make the audit vacuous: one pair per n models. A generator whose rule
// is not implemented has no entry.
var porAuditFloor = map[string]int{"base": 1, "atomic": 1, "loop": 1, "run": 1, "run-atomic": 1, "reads": 1, "atomic-reads": 1, "nrpr": 1}

// porAtRunFloor is, per generator, how few (state, process) pairs with the
// process at a `run` edge make the audit blind to the footprint of the `run`
// itself: one pair per n models. A model in which a table is never left is the
// only way a `run` is expanded alone, and the figure is what told the author of
// the first generator that its models were not exercising it.
var porAtRunFloor = map[string]int{"run": 40, "run-atomic": 60}

// porAtRunMinModels is the smallest run that the at-run floor applies to. Only
// about one model in forty has a pair at a run edge (the process has to be
// eligible there, which needs a table that is never left), and the pairs come in
// bunches: 1 000 models give 94 to 589 of them (16 windows of go test -short, 125
// models, gave 1 to 58, so the floor of 3 and of 2 failed on a fresh seed); at the
// size of a short run the count says nothing, and the check is for the default size.
const porAtRunMinModels = 1000

// ---- hand-forced wrong plans -----------------------------------------------------

// forcedPlan makes every process in procs eligible at every location that has
// an edge, with no channel requirement. It is what a broken analysis would
// produce at the locations of a known trap, so the audit can be shown to see
// each one without any switch in the analysis itself.
func forcedPlan(t *testing.T, m *ir.Model, procs ...int) *porPlan {
	t.Helper()
	plan := &porPlan{eligible: make([][]bool, len(m.Processes)), req: make([][][]porChanReq, len(m.Processes)), any: true}
	for p := range m.Processes {
		plan.eligible[p] = make([]bool, len(m.Processes[p].Locations))
		plan.req[p] = make([][]porChanReq, len(m.Processes[p].Locations))
	}
	for _, p := range procs {
		for l := range m.Processes[p].Locations {
			plan.eligible[p][l] = hasOut(&m.Processes[p], l)
		}
	}
	return plan
}

// wantAuditFailure runs the audit with the process forced eligible and wants a
// violation that mentions want.
func wantAuditFailure(t *testing.T, name string, m *ir.Model, want string, procs ...int) {
	t.Helper()
	_, _, err := auditModel(m, forcedPlan(t, m, procs...), 2)
	if err == nil {
		t.Fatalf("%s: the audit passed a plan that expands %v alone, which is wrong", name, procs)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("%s: the audit failed for another reason than %q: %v", name, want, err)
	}
}

// The traps of the earlier steps, each as the plan a broken analysis would
// make. The audit must report every one; the analysis proper is not used.
func TestPORAuditSeesTheKnownTraps(t *testing.T) {
	t.Run("a step another process enables (step 2, first review)", func(t *testing.T) {
		wantAuditFailure(t, "enabling", porEnabling(), "C1", 0)
	})
	t.Run("a d_step whose continuation reads a program counter (step 2)", func(t *testing.T) {
		wantAuditFailure(t, "dstep", porDStepGuard(), "C1", 0)
	})
	t.Run("a send that only the receiver's pop enables (step 4)", func(t *testing.T) {
		wantAuditFailure(t, "full channel", fullChannelTrap(), "C1", 0)
		wantAuditFailure(t, "full channel, other order", swapped(fullChannelTrap()), "C1", 1)
	})
	t.Run("a receive that only the sender's push enables (step 4)", func(t *testing.T) {
		wantAuditFailure(t, "empty channel", emptyChannelTrap(), "C1", 1)
		wantAuditFailure(t, "empty channel, other order", swapped(emptyChannelTrap()), "C1", 0)
	})
	t.Run("a clear that overlaps the receive end (step 4)", func(t *testing.T) {
		wantAuditFailure(t, "clear", clearBeforeReceive(), "C1", 0)
		wantAuditFailure(t, "clear, other order", rotated(clearBeforeReceive()), "C1", 1)
	})
	t.Run("a write that a property reads (step 2)", func(t *testing.T) {
		wantAuditFailure(t, "visible", porVisible(), "C2", 0)
	})
	t.Run("a shared write", func(t *testing.T) {
		m := model([]ir.Var{byteVar("x")}, nil, nil,
			proc("P", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 1)}}),
			proc("Q", nil, 2, ir.Edge{From: 0, To: 1, Effect: []ir.Assign{set("x", 2)}}))
		wantAuditFailure(t, "shared write", m, "commute", 0)
	})
}

// A plan that is right passes: the same models with the processes the analysis
// really finds eligible.
func TestPORAuditPassesTheAnalysisOnTheKnownTraps(t *testing.T) {
	for name, m := range map[string]*ir.Model{
		"enabling":      porEnabling(),
		"dstep":         porDStepGuard(),
		"full channel":  fullChannelTrap(),
		"empty channel": emptyChannelTrap(),
		"clear":         clearBeforeReceive(),
		"visible":       porVisible(),
		"pipeline":      pipeline(3, 2, 2),
	} {
		if _, _, err := auditModel(m, nil, 2); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
