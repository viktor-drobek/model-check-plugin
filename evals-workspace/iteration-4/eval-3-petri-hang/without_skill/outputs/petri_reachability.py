#!/usr/bin/env python3
"""Exhaustive reachability analysis of the Petri net from the task.

Places  : p1..p6            (index 0..5)
Trans.  : t1: p1 -> p2
          t2: p2,p4 -> p3
          t3: p3 -> p1,p4
          t4: p4 -> p5
          t5: p1,p5 -> p6
          t6: p6 -> p4,p1
M0      : p1=1, p4=1
Goal    : are there dead (deadlock) markings reachable from M0?
"""
from collections import deque

P = ["p1", "p2", "p3", "p4", "p5", "p6"]
IDX = {n: i for i, n in enumerate(P)}


def vec(**kw):
    v = [0] * 6
    for k, n in kw.items():
        v[IDX[k]] = n
    return tuple(v)


# (name, pre, post)
T = [
    ("t1", vec(p1=1),       vec(p2=1)),
    ("t2", vec(p2=1, p4=1), vec(p3=1)),
    ("t3", vec(p3=1),       vec(p1=1, p4=1)),
    ("t4", vec(p4=1),       vec(p5=1)),
    ("t5", vec(p1=1, p5=1), vec(p6=1)),
    ("t6", vec(p6=1),       vec(p4=1, p1=1)),
]

M0 = vec(p1=1, p4=1)


def enabled(m):
    return [(n, pre, post) for (n, pre, post) in T
            if all(m[i] >= pre[i] for i in range(6))]


def fire(m, pre, post):
    return tuple(m[i] - pre[i] + post[i] for i in range(6))


def show(m):
    on = [f"{P[i]}={m[i]}" for i in range(6) if m[i]]
    return "{" + ", ".join(on) + "}" if on else "{empty}"


# ---- BFS over the reachability graph ---------------------------------------
parent = {M0: None}          # marking -> (prev marking, transition name)
order = [M0]
q = deque([M0])
edges = []
dead = []
COV_LIMIT = 50               # unboundedness guard: no place should exceed this

while q:
    m = q.popleft()
    en = enabled(m)
    if not en:
        dead.append(m)
        continue
    for name, pre, post in en:
        m2 = fire(m, pre, post)
        edges.append((m, name, m2))
        if max(m2) > COV_LIMIT:
            raise SystemExit("unbounded-looking marking: " + show(m2))
        if m2 not in parent:
            parent[m2] = (m, name)
            order.append(m2)
            q.append(m2)


def trace(m):
    path = []
    cur = m
    while parent[cur] is not None:
        prev, name = parent[cur]
        path.append((prev, name, cur))
        cur = prev
    return list(reversed(path))


print("reachable markings :", len(parent))
print("graph edges        :", len(edges))
print("bound (max tokens in any place over all reachable markings):",
      max(max(m) for m in parent))
print()
print("--- all reachable markings ---")
for m in order:
    en = [n for n, _, _ in enabled(m)]
    print(f"  {show(m):<28} enabled: {en if en else 'NONE  <-- DEADLOCK'}")
print()
if dead:
    print(f"--- {len(dead)} DEAD marking(s) ---")
    for m in dead:
        print("  " + show(m))
        tr = trace(m)
        print(f"    shortest firing sequence from M0 ({len(tr)} steps):")
        print("      M0 = " + show(M0))
        for prev, name, nxt in tr:
            print(f"      --{name}--> {show(nxt)}")
else:
    print("no dead markings: the net is deadlock-free")

# ---- P-invariant check ------------------------------------------------------
print()
print("--- P-invariant check (verifies safeness / explains the structure) ---")
inv = [("p1+p2+p3+p6", vec(p1=1, p2=1, p3=1, p6=1)),
       ("p3+p4+p5+p6", vec(p3=1, p4=1, p5=1, p6=1))]
for label, y in inv:
    vals = {sum(y[i] * m[i] for i in range(6)) for m in parent}
    print(f"  {label:<14} constant over all reachable markings: {vals}")

# ---- can the deadlock still be avoided once we are underway? ---------------
print()
print("--- from which reachable markings is a dead marking still reachable? ---")
def reaches_dead(start):
    seen, stack = {start}, [start]
    while stack:
        m = stack.pop()
        if m in dead:
            return True
        for n, pre, post in enabled(m):
            m2 = fire(m, pre, post)
            if m2 not in seen:
                seen.add(m2)
                stack.append(m2)
    return False

for m in order:
    print(f"  {show(m):<28} -> deadlock still reachable: {reaches_dead(m)}")

# ---- reachability graph as DOT ---------------------------------------------
with open("reachability-graph.dot", "w") as f:
    f.write("digraph reachability {\n  rankdir=LR;\n  node [shape=box, fontname=monospace];\n")
    for m in order:
        attrs = []
        if m == M0:
            attrs.append('style=filled, fillcolor="#dbe9ff"')
        if m in dead:
            attrs.append('style=filled, fillcolor="#ffd6d6", penwidth=2')
        f.write(f'  "{show(m)}" [{", ".join(attrs)}];\n')
    for a, n, b in edges:
        f.write(f'  "{show(a)}" -> "{show(b)}" [label="{n}"];\n')
    f.write("}\n")
print("\nreachability graph written to reachability-graph.dot")
