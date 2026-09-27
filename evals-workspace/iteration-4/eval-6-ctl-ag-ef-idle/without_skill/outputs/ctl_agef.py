#!/usr/bin/env python3
"""
Explicit-state CTL check of  AG EF idle  for  'Promela - examples/CH14/version1'.

The two process-local automata below are transcribed verbatim from SPIN's own
state tables (`pan -D`, Spin 6.5.2), so the granularity of states matches the
graph SPIN explores.  idle == switch is at its label Idle (local state S4).

CTL is branching-time, so it is checked the branching-time way: by a
least-fixpoint (backward reachability) computation of EF, then a universal
test over the reachable states for AG.  Nothing is rewritten into LTL.
"""
from collections import deque
import json, sys

# ---- process-local transition relations, from `pan -D` -----------------------
# (source, label, target); label 's:<msg>' = send, 'r:<msg>' = receive,
# 'l:<text>' = local (printf/goto) step.
SUB_INIT = 1
SUB = [
    (1, 's:offhook', 6),
    (6, 's:digits',  6),
    (6, 's:onhook',  1),
]
SW_INIT = 4
SW = [
    (4,  'r:offhook', 2),
    (2,  'l:printf dialtone', 12),
    (12, 'r:digits',  7),
    (12, 'r:onhook',  10),
    (7,  'l:printf notone',  18),
    (18, 'l:printf ringtone', 24),
    (18, 'l:printf busytone', 26),
    (24, 'l:printf busytone', 26),
    (24, 'l:printf notone',   26),
    (26, 'r:onhook',  27),
    (27, 'l:printf notone',   4),
    (10, 'l:printf notone',   4),
]

# human-readable names for the switch's labelled control states
SW_LABEL = {4: 'Idle', 12: 'Dial', 18: 'Wait', 24: 'Connect', 26: 'Busy'}
SUB_LABEL = {1: 'Idle', 6: 'Busy'}

def sub_out(p):  return [(l, q) for (s, l, q) in SUB if s == p]
def sw_out(p):   return [(l, q) for (s, l, q) in SW  if s == p]

# ---- global transition relation: rendezvous (chan tpc = [0]) ------------------
def succs(st):
    sub, sw = st
    out = []
    # local (printf) steps of either process
    for (l, q) in sub_out(sub):
        if l.startswith('l:'):
            out.append(('sub ' + l[2:], (q, sw)))
    for (l, q) in sw_out(sw):
        if l.startswith('l:'):
            out.append(('sw ' + l[2:], (sub, q)))
    # rendezvous: a send in one process matched by a receive of the same
    # message in the other; both move in a single global step
    for (ls, qs) in sub_out(sub):
        if not ls.startswith('s:'):
            continue
        msg = ls[2:]
        for (lr, qr) in sw_out(sw):
            if lr == 'r:' + msg:
                out.append(('tpc!' + msg + ' / tpc?' + msg, (qs, qr)))
    for (ls, qs) in sw_out(sw):          # (switch never sends, kept for symmetry)
        if not ls.startswith('s:'):
            continue
        msg = ls[2:]
        for (lr, qr) in sub_out(sub):
            if lr == 'r:' + msg:
                out.append(('tpc!' + msg + ' / tpc?' + msg, (sub, qr)))
    return out

INIT = (SUB_INIT, SW_INIT)

# ---- forward reachability ----------------------------------------------------
R, order, edges = {INIT}, [INIT], {}
dq = deque([INIT])
while dq:
    s = dq.popleft()
    edges[s] = succs(s)
    for (_, t) in edges[s]:
        if t not in R:
            R.add(t); order.append(t); dq.append(t)

ntrans = sum(len(v) for v in edges.values())
deadlocks = [s for s in R if not edges[s]]

def name(s):
    sub, sw = s
    return "sub@%s(S%d), switch@%s(S%d)" % (
        SUB_LABEL.get(sub, '-'), sub, SW_LABEL.get(sw, '-'), sw)

# ---- the atomic proposition --------------------------------------------------
def idle(s):   return s[1] == SW_INIT          # switch at label Idle (S4)
SAT_idle = {s for s in R if idle(s)}

# ---- CTL:  EF phi  =  mu Z . phi | EX Z   (backward least fixpoint) ----------
pred = {s: [] for s in R}
for s in R:
    for (_, t) in edges[s]:
        pred[t].append(s)

def EF(sat):
    Z = set(sat & R)
    dq = deque(Z)
    while dq:
        t = dq.popleft()
        for s in pred[t]:
            if s not in Z:
                Z.add(s); dq.append(s)
    return Z

SAT_EF_idle = EF(SAT_idle)

# ---- CTL:  AG phi  =  nu Z . phi & AX Z  ; here AG(EF idle) ------------------
def AG(sat):
    # greatest fixpoint: drop any state that is not in sat or has a successor
    # outside the current set
    Z = set(R) & set(sat)
    changed = True
    while changed:
        changed = False
        for s in list(Z):
            if any(t not in Z for (_, t) in edges[s]):
                Z.discard(s); changed = True
    return Z

SAT_AG_EF_idle = AG(SAT_EF_idle)
HOLDS = INIT in SAT_AG_EF_idle
violating = sorted(R - SAT_EF_idle)

# ---- witnesses: for AG EF the witness is a *tree* of finite paths ------------
dist, via = {}, {}
for s in SAT_idle: dist[s] = 0
dq = deque(SAT_idle)
while dq:
    t = dq.popleft()
    for s in pred[t]:
        if s not in dist:
            dist[s] = dist[t] + 1; via[s] = (t, next(l for (l, u) in edges[s] if u == t)); dq.append(s)

witness = {}
for s in order:
    path, cur = [], s
    while dist.get(cur, None):
        t, lab = via[cur]
        path.append(lab); cur = t
    witness[name(s)] = {"steps_to_idle": dist[s], "witness_path": path}

# ---- contrast only: the LTL strengthening AGF idle (BSCC analysis) -----------
# every reachable bottom SCC containing no idle state would refute both AGF
# idle and AG EF idle; a non-bottom idle-free cycle would refute AGF idle only.
import itertools
index, low, onstk, stk, sccs, counter = {}, {}, set(), [], [], itertools.count()
def tarjan(root):
    work = [(root, 0)]
    while work:
        v, pi = work[-1]
        if pi == 0:
            index[v] = low[v] = next(counter); stk.append(v); onstk.add(v)
        recurse = False
        succ = [t for (_, t) in edges[v]]
        for i in range(pi, len(succ)):
            w = succ[i]
            if w not in index:
                work[-1] = (v, i + 1); work.append((w, 0)); recurse = True; break
            elif w in onstk:
                low[v] = min(low[v], index[w])
        if recurse: continue
        if low[v] == index[v]:
            comp = []
            while True:
                w = stk.pop(); onstk.discard(w); comp.append(w)
                if w == v: break
            sccs.append(comp)
        work.pop()
        if work:
            u = work[-1][0]; low[u] = min(low[u], low[v])
for s in order:
    if s not in index: tarjan(s)

def is_bottom(comp):
    cs = set(comp)
    return all(t in cs for v in comp for (_, t) in edges[v])

bsccs = [c for c in sccs if is_bottom(c) or len(c) == 1 and not edges[c[0]]]
bsccs_no_idle = [c for c in bsccs if not any(idle(s) for s in c)]
# an idle-free cycle is a cycle inside the subgraph induced by the NOT-idle
# states; SCCs of the whole graph are too coarse for that (this model is one
# big SCC that does contain idle).
sub_nodes = {s for s in R if not idle(s)}
sub_edges = {s: [t for (_, t) in edges[s] if t in sub_nodes] for s in sub_nodes}
sindex, slow, sonstk, sstk, ssccs = {}, {}, set(), [], []
scounter = itertools.count()
def tarjan2(root):
    work=[(root,0)]
    while work:
        v,pi=work[-1]
        if pi==0:
            sindex[v]=slow[v]=next(scounter); sstk.append(v); sonstk.add(v)
        rec=False; succ=sub_edges[v]
        for i in range(pi,len(succ)):
            w=succ[i]
            if w not in sindex:
                work[-1]=(v,i+1); work.append((w,0)); rec=True; break
            elif w in sonstk: slow[v]=min(slow[v],sindex[w])
        if rec: continue
        if slow[v]==sindex[v]:
            comp=[]
            while True:
                w=sstk.pop(); sonstk.discard(w); comp.append(w)
                if w==v: break
            ssccs.append(comp)
        work.pop()
        if work:
            u=work[-1][0]; slow[u]=min(slow[u],slow[v])
for s in order:
    if s in sub_nodes and s not in sindex: tarjan2(s)
nontrivial_idlefree_cycles = [sorted(c) for c in ssccs
    if len(c) > 1 or any(t == c[0] for t in sub_edges[c[0]])]

report = {
  "model": "Promela - examples/CH14/version1",
  "proposition_idle": "switch (pid 1) at label Idle == local state S4",
  "state_space": {
      "reachable_global_states": len(R),
      "global_transitions": ntrans,
      "deadlock_states": [name(s) for s in deadlocks],
  },
  "ctl": {
      "formula": "AG EF idle",
      "sat_idle": sorted(name(s) for s in SAT_idle),
      "num_states_satisfying_EF_idle": len(SAT_EF_idle),
      "reachable_states_violating_EF_idle": [name(s) for s in violating],
      "AG_EF_idle_holds_in_initial_state": HOLDS,
      "AG_EF_idle_holds_in_every_reachable_state": len(SAT_AG_EF_idle) == len(R),
      "fairness": "none assumed; EF/AG fixpoints range over all paths of the raw state graph",
      "witness_shape": "tree (one finite witness path per reachable state), not a single path",
  },
  "witness_tree": witness,
  "contrast_only_not_the_property_checked": {
      "reachable_bottom_SCCs": len(bsccs),
      "bottom_SCCs_without_idle": [[name(s) for s in c] for c in bsccs_no_idle],
      "idle_free_cycles_anywhere": [[name(s) for s in c] for c in nontrivial_idlefree_cycles],
      "note": "an idle-free cycle refutes the LTL formula []<>idle but NOT the CTL formula AG EF idle; an idle-free BOTTOM SCC would refute both",
  },
}
json.dump(report, sys.stdout, indent=2, ensure_ascii=False)
print()

with open(sys.argv[1] if len(sys.argv) > 1 else '/dev/null', 'w') as f:
    f.write('digraph global {\n  rankdir=LR;\n  node [shape=box,fontname="monospace"];\n')
    ids = {s: 'n%d' % i for i, s in enumerate(order)}
    for s in order:
        f.write('  %s [label="%s"%s];\n' % (ids[s], name(s),
                ',style=filled,fillcolor="#cfe8cf"' if idle(s) else ''))
    for s in order:
        for (l, t) in edges[s]:
            f.write('  %s -> %s [label="%s"];\n' % (ids[s], ids[t], l))
    f.write('}\n')
