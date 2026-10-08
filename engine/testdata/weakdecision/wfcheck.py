#!/usr/bin/env python3
"""Independent ground truth for weak process fairness on tiny hand-encoded Promela models.
Shares NOTHING with the engine, its oracle or pan. Every model below is the hand translation of the .pml file
in models/ (statement = one edge; `a; b` = two edges; a `do` option starts at the loop-head pc).

Semantics (Holzmann, Baier-Katoen):
 * a process at the end of its proctype has the executable step `-end-` (it dies) iff it has the highest live pid
   (pan's process stack); that is why the hand counts agree with pan's and the engine's state counts.
 * a process p is ENABLED in system state s iff it has an executable edge: a normal edge whose guard holds, or a
   `timeout` edge while no process has an executable NORMAL edge (timeout = no other statement is executable);
   a process whose `provided` clause is false has no executable edge.
 * a never claim moves first in lockstep: product edge (s,q)->(s',q') iff the claim has an edge q->q' whose
   condition holds in s and (s->s' is a system move, or s has no move and s'=s: stutter extension, when ON).
 * weak fairness (justice): a run is fair iff for every process p: if p is enabled from some point on, p moves infinitely often.
 * Decision (textbook): an accepting weakly fair run exists iff a reachable SCC (with an edge) contains an accepting
   state and, for every process p, has an internal edge moved by p or a state in which p is disabled.
Usage: wfcheck.py [name ...]   (no names = all). Writes graphs/<name>.txt."""
import sys, os, itertools
D = os.path.dirname(os.path.abspath(__file__))  # graphs/<name>.txt is written next to this script

class Edge:
    def __init__(s, label, guard, eff, npc, timeout=False):
        s.label, s.guard, s.eff, s.npc, s.timeout = label, guard, eff, npc, timeout
class Proc:
    def __init__(s, name, edges, provided=None, accept=(), end=(), progress=(), atomic=()):
        s.name, s.edges, s.provided, s.accept, s.progress, s.atomic = name, edges, provided, set(accept), set(progress), set(atomic)  # edges: {pc: [Edge]}
class Model:
    def __init__(s, name, gnames, g0, procs, claim=None, qacc=(), q0=None, mode="claim"):
        s.name, s.gn, s.g0, s.procs, s.claim, s.qacc, s.q0, s.mode = name, gnames, g0, procs, claim, set(qacc), q0, mode
    def gs(s, g): return dict(zip(s.gn, g))

T = lambda g: True
def setg(m, g, **kw):
    d = dict(zip(m.gn, g)); d.update(kw); return tuple(d[n] for n in m.gn)

def moves(m, g, pcs):
    """Return (list of (p, g', pcs'), enabled-set). Real Promela semantics incl. timeout."""
    G = m.gs(g); normal, tmo = [], []
    for p, pr in enumerate(m.procs):
        if pcs[p] == -1: continue                       # dead process (removed after its -end- step)
        if pr.provided is not None and not pr.provided(G): continue
        es = pr.edges.get(pcs[p], [])
        if not es:                                      # at its end: pan's `-end-` step, only for the highest live pid
            if all(pcs[q] == -1 for q in range(p + 1, len(pcs))):
                normal.append((p, Edge("-end-", T, None, -1)))
            continue
        for e in es:
            if not e.guard(G): continue
            (tmo if e.timeout else normal).append((p, e))
    holder = [p for p, _ in normal if pcs[p] in m.procs[p].atomic]       # a process inside an atomic block keeps the lock
    if holder:                                                          # while it can move: nobody else is executable
        normal = [(p, e) for p, e in normal if p == holder[0]]; tmo = []
    use = normal if normal else tmo          # timeout is true iff no other statement is executable
    res = []
    for p, e in use:
        g2 = e.eff(g, m) if e.eff else g
        np_ = list(pcs); np_[p] = e.npc
        res.append((p, g2, tuple(np_)))
    return res, {p for p, _ in use}

def build(m, stutter):
    init = (m.g0, tuple(0 for _ in m.procs), m.q0)
    idx, order, edges, dis = {init: 0}, [init], [], {}
    i = 0
    while i < len(order):
        g, pcs, q = order[i]; G = m.gs(g)
        mv, en = moves(m, g, pcs)
        dis[i] = set(range(len(m.procs))) - en
        succ_sys = [(p, g2, pcs2) for p, g2, pcs2 in mv]
        if not succ_sys and stutter: succ_sys = [(None, g, pcs)]       # stutter extension
        cl = [(None, q)] if m.claim is None else [(c, q2) for c, q2 in m.claim.get(q, []) if c(G)]
        for _, q2 in cl:
            for p, g2, pcs2 in succ_sys:
                t = (g2, pcs2, q2)
                if t not in idx: idx[t] = len(order); order.append(t)
                edges.append((i, idx[t], p))
        i += 1
    return order, edges, dis

def accepting(m, st):
    g, pcs, q = st
    if m.mode == "np": return not any(pcs[p] in m.procs[p].progress for p in range(len(m.procs)))
    if m.mode == "claim": return q in m.qacc
    return any(pcs[p] in m.procs[p].accept for p in range(len(m.procs)))

def sccs(n, adj):
    idx, low, onst, st, out, c = {}, {}, set(), [], [], [0]
    sys.setrecursionlimit(100000)
    def sc(v):
        idx[v] = low[v] = c[0]; c[0] += 1; st.append(v); onst.add(v)
        for w in adj[v]:
            if w not in idx: sc(w); low[v] = min(low[v], low[w])
            elif w in onst: low[v] = min(low[v], idx[w])
        if low[v] == idx[v]:
            comp = []
            while True:
                w = st.pop(); onst.discard(w); comp.append(w)
                if w == v: break
            out.append(comp)
    for v in range(n):
        if v not in idx: sc(v)
    return out

def decide(m, stutter, dump=None):
    if m.mode == "np": stutter = False         # pan -l has no stutter extension
    order, edges, dis = build(m, stutter)
    adj = [[] for _ in order]
    for a, b, p in edges:
        if m.mode == "np" and not (accepting(m, order[a]) and accepting(m, order[b])): continue
        adj[a].append(b)
    comp_of = {}
    comps = sccs(len(order), adj)
    for ci, comp in enumerate(comps):
        for v in comp: comp_of[v] = ci
    witness, why = None, []
    for ci, comp in enumerate(comps):
        cs = set(comp)
        if m.mode == "np" and not all(accepting(m, order[v]) for v in comp): continue   # a non-progress cycle avoids progress states
        inner = [(a, b, p) for a, b, p in edges if a in cs and b in cs]
        if m.mode == "np" and len(comp) == 1 and not any(a == b for a, b, _ in inner): inner = []
        if not inner: continue                                  # no edge: not a cycle
        if not any(accepting(m, order[v]) for v in comp): continue
        movers = {p for _, _, p in inner if p is not None}
        unfair = [m.procs[p].name for p in range(len(m.procs))
                  if p not in movers and not any(p in dis[v] for v in comp)]
        why.append((sorted(comp), unfair))
        if not unfair and witness is None: witness = sorted(comp)
    if dump is not None:
        dump.write(f"## stutter_extension={'ON' if stutter else 'OFF'} states={len(order)} edges={len(edges)}\n")
        for i, st in enumerate(order):
            g, pcs, q = st
            dump.write(f"S{i}: {m.gs(g)} pcs={pcs} claim={q} acc={accepting(m, st)} disabled={[m.procs[p].name for p in sorted(dis[i])]}\n")
        for a, b, p in edges:
            dump.write(f"  S{a} -[{'stutter' if p is None else m.procs[p].name}]-> S{b}\n")
        dump.write(f"  accepting SCCs (comp, unfair-procs): {why}\n  fair accepting SCC: {witness}\n")
    return ("violated" if witness is not None else "verified"), len(order), len(edges), why

# ---------------------------------------------------------------- models
E = Edge
def eff(**kw):   # assignment of constants / expressions: kw name -> value or fn(G)
    def f(g, m):
        G = m.gs(g); d = dict(G)
        for k, v in kw.items(): d[k] = v(G) if callable(v) else v
        return tuple(d[n] for n in m.gn)
    return f
MODELS = {}
def reg(f): MODELS[f.__name__] = f; return f

@reg
def d1_ev_always():            # system blocked at a==0; claim = spin -f '!(<>[] p)', p = (a==1)
    P = Proc("P", {0: [E("a==1", lambda G: G["a"] == 1, None, 1)]})
    cl = {"T0": [(lambda G: not G["a"] == 1, "A9"), (T, "T0")], "A9": [(T, "T0")]}
    return Model("d1_ev_always", ["a"], (0,), [P], cl, {"A9"}, "T0")
@reg
def d1_alt():
    P = Proc("P", {0: [E("a==1", lambda G: G["a"] == 1, None, 1)]})
    cl = {"S0": [(T, "S1")], "S1": [(T, "S0")]}
    return Model("d1_alt", ["a"], (0,), [P], cl, {"S0"}, "S0")
@reg
def d1_stay():
    P = Proc("P", {0: [E("a==1", lambda G: G["a"] == 1, None, 1)]})
    return Model("d1_stay", ["a"], (0,), [P], {"S0": [(T, "S0")]}, {"S0"}, "S0")

def tog(v): return lambda G: 1 - G[v]
@reg
def d2_pf_a():                  # P0 provided(false) {skip}; P1 {do :: accept: y = 1-y od}
    P0 = Proc("P0", {0: [E("skip", T, None, 1)]}, provided=lambda G: False)
    P1 = Proc("P1", {0: [E("y=1-y", T, eff(y=tog("y")), 0)]}, accept={0})
    return Model("d2_pf_a", ["y"], (0,), [P0, P1], mode="acc")
@reg
def d2_pf_b():
    P0 = Proc("P0", {0: [E("y=1-y", T, eff(y=tog("y")), 0)]}, accept={0})
    P1 = Proc("P1", {0: [E("skip", T, None, 1)]}, provided=lambda G: False)
    return Model("d2_pf_b", ["y"], (0,), [P0, P1], mode="acc")
@reg
def d2_pb_prov():
    P0 = Proc("P0", {0: [E("x=0", T, eff(x=0), 0)]}, provided=lambda G: G["b"] == 0)
    P1 = Proc("P1", {0: [E("b=1", T, eff(b=1), 1)], 1: [E("y=1-y", T, eff(y=tog("y")), 1)]}, accept={1})
    return Model("d2_pb_prov", ["x", "y", "b"], (0, 0, 0), [P0, P1], mode="acc")
@reg
def d2_pb_guard():
    P0 = Proc("P0", {0: [E("b==0", lambda G: G["b"] == 0, None, 1)], 1: [E("x=0", T, eff(x=0), 0)]})
    P1 = Proc("P1", {0: [E("b=1", T, eff(b=1), 1)], 1: [E("y=1-y", T, eff(y=tog("y")), 1)]}, accept={1})
    return Model("d2_pb_guard", ["x", "y", "b"], (0, 0, 0), [P0, P1], mode="acc")
@reg
def d2_pf_claim():
    P0 = Proc("P0", {0: [E("skip", T, None, 1)]}, provided=lambda G: False)
    P1 = Proc("P1", {0: [E("y=1-y", T, eff(y=tog("y")), 0)]})
    return Model("d2_pf_claim", ["y"], (0,), [P0, P1], {"c": [(T, "c")]}, {"c"}, "c")

def q_b0(): return {"c": [(lambda G: G["b"] == 0, "c")]}
@reg
def d3_ctl_split():             # P1: do :: timeout -> a=1-a od ; P2: timeout -> b=1
    P1 = Proc("P1", {0: [E("timeout", T, None, 1, True)], 1: [E("a=1-a", T, eff(a=tog("a")), 0)]})
    P2 = Proc("P2", {0: [E("timeout", T, None, 1, True)], 1: [E("b=1", T, eff(b=1), 2)]})
    return Model("d3_ctl_split", ["a", "b"], (0, 0), [P1, P2], q_b0(), {"c"}, "c")
@reg
def d3_ctl_skip():              # P: do :: timeout -> skip od (skip is a statement of its own)
    P = Proc("P", {0: [E("timeout", T, None, 1, True)], 1: [E("skip", T, None, 0)]})
    Q = Proc("Q", {0: [E("timeout", T, None, 1, True)], 1: [E("b=1", T, eff(b=1), 2)]})
    return Model("d3_ctl_skip", ["b"], (0,), [P, Q], q_b0(), {"c"}, "c")
@reg
def d3_one():                   # P: do :: timeout od (one statement per step)
    P = Proc("P", {0: [E("timeout", T, None, 0, True)]})
    Q = Proc("Q", {0: [E("timeout", T, None, 1, True)], 1: [E("b=1", T, eff(b=1), 2)]})
    return Model("d3_one", ["b"], (0,), [P, Q], q_b0(), {"c"}, "c")
@reg
def to1():                      # the cross-review's to1.pml: the same model as d3_ctl_split (claim label `accept` instead of `accept_c`)
    P1 = Proc("P1", {0: [E("timeout", T, None, 1, True)], 1: [E("a=1-a", T, eff(a=tog("a")), 0)]})
    P2 = Proc("P2", {0: [E("timeout", T, None, 1, True)], 1: [E("b=1", T, eff(b=1), 2)]})
    return Model("to1", ["a", "b"], (0, 0), [P1, P2], q_b0(), {"c"}, "c")
@reg
def to2():                      # the cross-review's to2.pml: P: do :: timeout -> skip od ; Q: do :: timeout -> b = 1 od
    P = Proc("P", {0: [E("timeout", T, None, 1, True)], 1: [E("skip", T, None, 0)]})
    Q = Proc("Q", {0: [E("timeout", T, None, 1, True)], 1: [E("b=1", T, eff(b=1), 0)]})
    return Model("to2", ["b"], (0,), [P, Q], q_b0(), {"c"}, "c")
@reg
def d3_mixed():                 # P1 always executable: timeout never true, P2 never enabled
    P1 = Proc("P1", {0: [E("a=1-a", T, eff(a=tog("a")), 0)]})
    P2 = Proc("P2", {0: [E("timeout", T, None, 1, True)], 1: [E("b=1", T, eff(b=1), 2)]})
    return Model("d3_mixed", ["a", "b"], (0, 0), [P1, P2], q_b0(), {"c"}, "c")

@reg
def d3_ltl():                   # claim = spin -f '!(<> q)', q = (b==1)
    P = Proc("P", {0: [E("timeout", T, None, 0, True)]})
    Q = Proc("Q", {0: [E("timeout", T, None, 1, True)], 1: [E("b=1", T, eff(b=1), 2)]})
    cl = {"A": [(lambda G: not G["b"] == 1, "A")]}
    return Model("d3_ltl", ["b"], (0,), [P, Q], cl, {"A"}, "A")
@reg
def d3_two_loop():
    P = Proc("P", {0: [E("timeout", T, None, 0, True)]})
    Q = Proc("Q", {0: [E("timeout", T, None, 0, True)]})
    return Model("d3_two_loop", ["b"], (0,), [P, Q], q_b0(), {"c"}, "c")

@reg
def d3_np():                    # --progress: P: do :: timeout od ; Q: timeout -> progress: do :: skip od
    P = Proc("P", {0: [E("timeout", T, None, 0, True)]})
    Q = Proc("Q", {0: [E("timeout", T, None, 1, True)], 1: [E("y=1-y", T, eff(y=tog("y")), 1)]}, progress={1})
    return Model("d3_np", ["y"], (0,), [P, Q], mode="np")
@reg
def d3_np_flip():              # same but Q has an ordinary guard instead of timeout: Q disabled when it must be (control)
    P = Proc("P", {0: [E("timeout", T, None, 0, True)]})
    Q = Proc("Q", {0: [E("false", lambda G: False, None, 1)], 1: [E("y=1-y", T, eff(y=tog("y")), 1)]}, progress={1})
    return Model("d3_np_flip", ["y"], (0,), [P, Q], mode="np")

@reg
def c5_m31278():                # acc mode: accept label at P1 pc1; globals a,b,d
    mod3 = lambda G: (G["d"] + 1) % 3
    P0 = Proc("P0", {0: [E("a=1-a", T, eff(a=tog("a")), 1), E("b=1", T, eff(b=1), 3)],
                     1: [E("d==2", lambda G: G["d"] == 2, None, 2)],
                     2: [E("a!=1", lambda G: G["a"] != 1, None, 0)],
                     3: [E("d=(d+1)%3", T, eff(d=mod3), 0)]})
    P1 = Proc("P1", {0: [E("skip", T, None, 1)], 1: [E("d=(d+1)%3", T, eff(d=mod3), 2)]}, accept={1})
    P2 = Proc("P2", {0: [E("d!=1", lambda G: G["d"] != 1, None, 1)], 1: [E("a=1-a", T, eff(a=tog("a")), 0)]})
    P3 = Proc("P3", {0: [E("d=(d+1)%3", T, eff(d=mod3), 1)], 1: [E("d!=0", lambda G: G["d"] != 0, None, 2)],
                     2: [E("a==0", lambda G: G["a"] == 0, None, 3)]})
    return Model("c5_m31278", ["a", "b", "d"], (0, 0, 0), [P0, P1, P2, P3], mode="acc")

@reg
def c5_min3():                  # 3-process reduction of m31278 (pan -a -f: accept stutter; NOSTUTTER: 0)
    mod3 = lambda G: (G["d"] + 1) % 3
    P0 = Proc("P0", {0: [E("a=1-a", T, eff(a=tog("a")), 1), E("d=(d+1)%3", T, eff(d=mod3), 0)],
                     1: [E("d==2", lambda G: G["d"] == 2, None, 2)],
                     2: [E("a!=1", lambda G: G["a"] != 1, None, 0)]})
    P1 = Proc("P1", {0: [E("skip", T, None, 1)], 1: [E("d=(d+1)%3", T, eff(d=mod3), 2)]}, accept={1})
    P3 = Proc("P3", {0: [E("d=(d+1)%3", T, eff(d=mod3), 1)], 1: [E("d!=0", lambda G: G["d"] != 0, None, 2)],
                     2: [E("a==0", lambda G: G["a"] == 0, None, 3)]})
    return Model("c5_min3", ["a", "b", "d"], (0, 0, 0), [P0, P1, P3], mode="acc")

@reg
def new_atomic_hold():          # P: atomic { do :: x = 1-x od } ; Q: y = 1 ; claim accepting while y == 0
    P = Proc("P", {0: [E("x=1-x", T, eff(x=tog("x")), 1)], 1: [E("x=1-x", T, eff(x=tog("x")), 1)]}, atomic={1})
    Q = Proc("Q", {0: [E("y=1", T, eff(y=1), 1)]})
    return Model("new_atomic_hold", ["x", "y"], (0, 0), [P, Q], {"c": [(lambda G: G["y"] == 0, "c")]}, {"c"}, "c")
@reg
def new_atomic_split():         # P: do :: atomic { x = 1-x; x = 1-x } od  (one step per block) ; Q: y = 1
    P = Proc("P", {0: [E("atomic{x=1-x;x=1-x}", T, None, 0)]})
    Q = Proc("Q", {0: [E("y=1", T, eff(y=1), 1)]})
    return Model("new_atomic_split", ["x", "y"], (0, 0), [P, Q], {"c": [(lambda G: G["y"] == 0, "c")]}, {"c"}, "c")

if __name__ == "__main__":
    names = sys.argv[1:] or list(MODELS)
    os.makedirs(D + "/graphs", exist_ok=True)
    print(f"{'model':14s} {'states':>6s} {'edges':>5s} | truth(stutter ON) | truth(stutter OFF) | unfair-procs of accepting SCCs (ON)")
    for n in names:
        m = MODELS[n]()
        with open(f"{D}/graphs/{n}.txt", "w") as fh:
            on = decide(m, True, fh); off = decide(m, False, fh)
        print(f"{n:14s} {on[1]:6d} {on[2]:5d} | {on[0]:17s} | {off[0]:18s} | {[w[1] for w in on[3]]} (OFF states={off[1]})")
