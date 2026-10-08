#!/usr/bin/env python3
"""Mutation test of the weak-fairness search (steps/fix-weakfairness-confirmation.md).

Run from anywhere inside the repository:

    python3 model-check-plugin/steps/fix-weakfairness-mutants.py [M1 M5 ...]

It copies model-check-plugin/engine (without bin/) to a temporary directory, applies one
textual mutation at a time to a source file there, runs the tests that must notice it and
prints whether the mutant was killed. The working tree is never touched. A mutant that
SURVIVES is a hole in the tests. The baseline (no mutation) must be green first: a failing
baseline would count every mutant as killed.

Mutants M1-M11 are the copies construction (explore/cycle.go); M12-M32 are the rules added
by the second round (explore/explore.go, explore/cycle.go, explore/step.go,
frontend/promela/lower.go): `provided` on both sides of a rendezvous, a pending `timeout`
as a move, the k-th non-claim process, the timeout moves of every claim edge, the Stepper's
timeout gate, the loop at the start of an atomic block, and `else` in a never claim.
"""
import os, shutil, subprocess, sys, tempfile

ROOT = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True, check=True).stdout.strip()
SRC = os.path.join(ROOT, "model-check-plugin", "engine")
TMP = tempfile.mkdtemp(prefix="wf-mutants-")
ENG = os.path.join(TMP, "model-check-plugin", "engine")
shutil.copytree(SRC, ENG, ignore=shutil.ignore_patterns("bin"))
# the explore tests read the SPIN corpus; link it into the copy
for extra in ("Promela - examples",):
    p = os.path.join(ROOT, extra)
    if os.path.exists(p):
        os.symlink(p, os.path.join(TMP, extra))

# what every mutant is run against: (package, -run pattern)
TESTS = [("./explore", "TestWeakFairness|TestProvided|TestStepper"), ("./frontend/promela", "TestLoopAtTheStart|TestBreakOfALoop")]

def read(f):
    return open(os.path.join(ENG, f)).read()

def write(f, s):
    open(os.path.join(ENG, f), "w").write(s)

def run():
    out = ""
    for pkg, pat in TESTS:
        r = subprocess.run(["go", "test", "-count=1", "-run", pat, pkg], cwd=ENG, capture_output=True, text=True, timeout=1800)
        out += r.stdout + r.stderr
        if r.returncode != 0:
            return r.returncode, out
    return 0, out

CY, EX, ST, LW = "explore/cycle.go", "explore/explore.go", "explore/step.go", "frontend/promela/lower.go"
# (name, [(file, old, new), ...])
M = []
M.append(("M1 null step closes the round again (the original defect)", [(CY,
'''			case k == cs.nproc+1:
				// Every process has moved or been blocked once. No null step
				// back to copy 0: the copy is left by the next step of the
				// product (apply), so that no cycle consists of null steps.
''','''			case k == cs.nproc+1:
				return productMove{null: true, toCopy: 0, claim: -1}, nil
'''), (CY,
'''		if k == 0 && cs.acceptingSys(sys) {
			return productMove{none: true}, nil
		}''','''		if (k == 0 && cs.acceptingSys(sys)) || k == cs.nproc+1 {
			return productMove{none: true}, nil
		}'''), (CY,
'''		case pm.toCopy == 1:
			note += ", leaving an accepting state)"
		default:''','''		case pm.toCopy == 1:
			note += ", leaving an accepting state)"
		case pm.toCopy == 0:
			note += ", every process has moved or been blocked once)"
		default:''')]))
M.append(("M2 a process is never blocked (no null step for a blocked process)", [(CY,
'''				if b {
					return productMove{null: true, toCopy: k + 1, claim: -1}, nil''','''				if b && false {
					return productMove{null: true, toCopy: k + 1, claim: -1}, nil''')]))
M.append(("M3 a process is always blocked (every copy advances by a null step)", [(CY,
'''				if b {
					return productMove{null: true, toCopy: k + 1, claim: -1}, nil''','''				if b || true {
					return productMove{null: true, toCopy: k + 1, claim: -1}, nil''')]))
M.append(("M4 a rendezvous partner does not count as moved", [(CY,
'''			moved := pm.m.e.proc == cs.sys[k-1] || (pm.m.partner != nil && pm.m.partner.proc == cs.sys[k-1])''','''			moved := pm.m.e.proc == cs.sys[k-1]''')]))
M.append(("M5 the round is never closed by a real step (copy stays n+1)", [(CY,
'''		case k == cs.nproc+1:
			cs.pnext[cs.size] = 0 // a step of the product closes the round''','''		case k == cs.nproc+1:
			cs.pnext[cs.size] = byte(k)''')]))
M.append(("M6 a stutter step does not close the round", [(CY,
'''		if cs.fair && int(cs.pcur[cs.size]) == cs.nproc+1 {
			cs.pnext[cs.size] = 0 // a step of the product closes the round
		}
		return nil''','''		return nil''')]))
M.append(("M7 the null step out of an accepting copy-0 state is missing", [(CY,
'''				if cs.acceptingSys(sys) {
					return productMove{null: true, toCopy: 1, claim: -1}, nil
				}''','''				if cs.acceptingSys(sys) && false {
					return productMove{null: true, toCopy: 1, claim: -1}, nil
				}''')]))
M.append(("M8 a move of the wrong process advances the copy", [(CY,
'''			moved := pm.m.e.proc == cs.sys[k-1] || (pm.m.partner != nil && pm.m.partner.proc == cs.sys[k-1])''','''			moved := pm.m.e.proc == cs.sys[k%cs.nproc] || (pm.m.partner != nil && pm.m.partner.proc == cs.sys[k%cs.nproc])''')]))
M.append(("M9 blocked is judged on the wrong process", [(CY,
'''				b, err := cs.blocked(cs.sys[k-1], sys)''','''				b, err := cs.blocked(cs.sys[k%cs.nproc], sys)''')]))
M.append(("M10 accepting states of any copy count", [(CY,
'''	if cs.fair && st[cs.size] != 0 {
		return false
	}
	return cs.acceptingSys(st[:cs.size])''','''	return cs.acceptingSys(st[:cs.size])''')]))
M.append(("M11 stutter extension off for the LTL/accept product too", [(CY,
'''		noStutter: prop.Kind == KindProgress,''','''		noStutter: true,''')]))
M.append(("M12 `provided` is not asked of the initiating side of a rendezvous", [(EX,
'''			if ok, err := s.providedHolds(e.proc, state); err != nil || !ok {
				if err != nil {
					return move{}, false, err
				}
				continue
			}
			ok, err := e.guard.Truth(state)''','''			ok, err := e.guard.Truth(state)''')]))
M.append(("M13 `provided` is not asked of the receiving side of a rendezvous", [(EX,
'''	if ok, err := s.providedHolds(r.proc, state); err != nil || !ok {
		return false, err
	}
	ok, err := r.guard.Truth(state)''','''	ok, err := r.guard.Truth(state)''')]))
M.append(("M14 blocked ignores `timeout` (hasEnabled again)", [(CY,
'''	ok, err := cs.s.executable(p, sys)
	return !ok, err''','''	ok, err := cs.s.hasEnabled(p, sys)
	return !ok, err''')]))
M.append(("M15 a timeout edge counts in every state, not only in a timeout state", [(EX,
'''	for q := range s.c.procs {
		if s.c.procs[q].claim {
			continue
		}
		if ok, err := s.hasEnabled(q, state); err != nil || ok {
			return false, err // not a timeout state: another statement is executable
		}
	}
	s.c.layout.Timeout = true''','''	s.c.layout.Timeout = true''')]))
M.append(("M16 only edges that do not use `timeout` count in a timeout state", [(EX,
'''		e := &s.c.procs[p].edge[ei]
		if !e.usesTimeout {
			continue
		}
		if ok, err := s.enabled(e, state); err != nil || ok {
			return ok, err
		}''','''		e := &s.c.procs[p].edge[ei]
		if e.usesTimeout {
			continue
		}
		if ok, err := s.enabled(e, state); err != nil || ok {
			return ok, err
		}''')]))
M.append(("M17 blocked is judged on process index k-1 (the claim is assumed to be last)", [(CY,
'''				b, err := cs.blocked(cs.sys[k-1], sys)''','''				b, err := cs.blocked(k-1, sys)''')]))
M.append(("M18 the mover is compared with process index k-1", [(CY,
'''			moved := pm.m.e.proc == cs.sys[k-1] || (pm.m.partner != nil && pm.m.partner.proc == cs.sys[k-1])''','''			moved := pm.m.e.proc == k-1 || (pm.m.partner != nil && pm.m.partner.proc == k-1)''')]))
M.append(("M19 the null-step note names process index copy-2 (the claim is assumed to be last)", [(CY,
'''cs.s.c.m.Processes[cs.sys[pm.toCopy-2]].Name''','''cs.s.c.m.Processes[pm.toCopy-2].Name''')]))
M.append(("M20 Stepper.Enabled lists timeout moves next to ordinary ones", [(ST,
'''		f.enabled++
		out = append(out, toMove(m))''','''		out = append(out, toMove(m))''')]))
M.append(("M21 Stepper.Apply accepts a timeout move while another process can move", [(ST,
'''		f.enabled++ // a move was found: the timeout phase is not entered
''','''''')]))
M.append(("M22 the move counter of a claim edge is not reset when the next edge is chosen", [(CY,
'''						f.proc = -1
						f.sysMoves = 0
''','''						f.proc = -1
''')]))
M.append(("M23 the timeout phase is gated by the frame counter again (claim branch)", [(CY,
'''			f.enabled = f.sysMoves
			m, ok, err := s.nextEnabled(f, sys)
			if err != nil {
				return productMove{}, err
			}
			if ok {
				f.sysSeen = true
				f.sysMoves++
				return productMove{claim: f.cedge''','''			m, ok, err := s.nextEnabled(f, sys)
			if err != nil {
				return productMove{}, err
			}
			if ok {
				f.sysSeen = true
				f.sysMoves++
				return productMove{claim: f.cedge''')]))
M.append(("M24 the timeout phase is gated by the frame counter again (no-claim branch)", [(CY,
'''		f.enabled = f.sysMoves
		m, ok, err := s.nextEnabled(f, sys)
		if err != nil {
			return productMove{}, err
		}
		if ok {
			f.sysSeen = true
			f.sysMoves++
			return productMove{claim: -1, m: m}, nil''','''		m, ok, err := s.nextEnabled(f, sys)
		if err != nil {
			return productMove{}, err
		}
		if ok {
			f.sysSeen = true
			f.sysMoves++
			return productMove{claim: -1, m: m}, nil''')]))
M.append(("M25 the back edge of a loop at the start of an atomic block gives up the control", [(LW,
'''				if len(l.curAtomic) > 0 {
					in.keepAtomic[i] = true
				}''','''				if len(l.curAtomic) > 0 && false {
					in.keepAtomic[i] = true
				}''')]))
M.append(("M26 the `break` of the loop keeps the control as well", [(LW,
'''				if b, ok := in.breakOf[i]; ok && b == id {
					continue
				}''','''				if _, ok := in.breakOf[i]; ok && false {
					continue
				}''')]))
M.append(("M27 an atomic loop that shares its entry with another alternative is accepted", [(LW,
'''			if (i < ol.lo || i >= ol.hi) && in.find(in.edges[i].From) == h {''','''			if false && (i < ol.lo || i >= ol.hi) && in.find(in.edges[i].From) == h {''')]))
M.append(("M28 the loop of a d_step block is not kept together", [(LW,
'''				if len(l.curDStep) > 0 {
					in.keepDStep[i] = true
				}''','''				if len(l.curDStep) > 0 && false {
					in.keepDStep[i] = true
				}''')]))
M.append(("M29 the note of a loop found without fairness ignores pending timeouts", [(CY,
'''			ok, err := cs.s.executable(p, st)''','''			ok, err := cs.s.hasEnabled(p, st)''')]))

M.append(("M30 an `else` edge of the claim is enabled although an ordinary edge is", [(CY,
'''						if f.cany {
							break
						}''','''						if f.cany && false {
							break
						}''')]))
M.append(("M31 the claim's ordinary edges never mark the location as having an enabled edge", [(CY,
'''						if ok {
							f.cany = true
						}''','''''')]))
M.append(("M32 after an atomic claim edge an `else` is taken in its textual order, whatever the other edges say", [(CY,
'''			if o.e.Else {
				if els == nil {
					els = o
				}
				continue
			}
''','''''')]))
only = sys.argv[1:]
orig = {}
rc, out = run()
if rc != 0:
    print("BASELINE FAILS, no mutant result is meaningful:\n" + out[:3000]); sys.exit(1)
print("baseline: green")
survived = 0
for name, reps in M:
    if only and not any(name.split()[0] == o for o in only): continue
    touched = {}
    for f, old, new in reps:
        if f not in touched:
            touched[f] = read(f)
        s = read(f)
        assert s.count(old) == 1, (name, f, old[:60], s.count(old))
        write(f, s.replace(old, new))
    rc, out = run()
    for f, src in touched.items():
        write(f, src)
    if rc == 0:
        print(name, "-> SURVIVED"); survived += 1
    else:
        kinds = []
        if "--- FAIL: TestWeakFairnessMatchesTheSCCOracle" in out: kinds.append("oracle (%d disagreeing rows)" % out.count("seed "))
        for line in out.splitlines():
            if line.startswith("--- FAIL: "):
                t = line.split()[2]
                if t != "TestWeakFairnessMatchesTheSCCOracle" and t not in kinds: kinds.append(t)
        if not kinds: kinds.append("build or crash: " + out[:160].replace("\n", " "))
        print(name, "-> killed by", ", ".join(kinds))
shutil.rmtree(TMP, ignore_errors=True)
sys.exit(1 if survived else 0)
