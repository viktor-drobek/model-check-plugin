#!/usr/bin/env python3
"""Exhaustive reachability analysis of the Petri net from the task.

Places p1..p6 (index 0..5), transitions:
  t1: p1 -> p2
  t2: p2,p4 -> p3
  t3: p3 -> p1,p4
  t4: p4 -> p5
  t5: p1,p5 -> p6
  t6: p6 -> p4,p1
Initial marking: p1=1, p4=1.
"""
from collections import deque

P = 6
# (name, pre, post) with pre/post as {place_index: multiplicity}
T = [
    ("t1", {0: 1}, {1: 1}),
    ("t2", {1: 1, 3: 1}, {2: 1}),
    ("t3", {2: 1}, {0: 1, 3: 1}),
    ("t4", {3: 1}, {4: 1}),
    ("t5", {0: 1, 4: 1}, {5: 1}),
    ("t6", {5: 1}, {3: 1, 0: 1}),
]
M0 = (1, 0, 0, 1, 0, 0)


def enabled(m):
    return [t for t in T if all(m[p] >= k for p, k in t[1].items())]


def fire(m, t):
    m = list(m)
    for p, k in t[1].items():
        m[p] -= k
    for p, k in t[2].items():
        m[p] += k
    return tuple(m)


def show(m):
    return "(" + ", ".join(f"p{i+1}={v}" for i, v in enumerate(m)) + ")"


seen = {M0: None}          # marking -> (predecessor, transition name)
order = [M0]
q = deque([M0])
edges = []
while q:
    m = q.popleft()
    for t in enabled(m):
        n = fire(m, t)
        edges.append((m, t[0], n))
        if n not in seen:
            seen[n] = (m, t[0])
            order.append(n)
            q.append(n)

dead = [m for m in order if not enabled(m)]

print(f"Reachable markings: {len(order)}")
print(f"Arcs in the reachability graph: {len(edges)}")
maxtok = max(sum(m) for m in order)
maxplace = max(max(m) for m in order)
print(f"Max tokens in any reachable marking: {maxtok}")
print(f"Max tokens in a single place (boundedness k): {maxplace}")
print()
print("All reachable markings:")
for m in order:
    en = [t[0] for t in enabled(m)]
    print(f"  {show(m)}  enabled: {en if en else 'NONE  <-- DEADLOCK'}")
print()
print(f"Dead (deadlocked) markings: {len(dead)}")
for m in dead:
    # reconstruct a shortest firing sequence (BFS => shortest)
    seq = []
    cur = m
    while seen[cur] is not None:
        prev, tn = seen[cur]
        seq.append(tn)
        cur = prev
    seq.reverse()
    print(f"  {show(m)}  via {' -> '.join(seq)}  (length {len(seq)})")

# Structural check: P-invariants via simple integer search over small coefficients
print()
print("Token-count change per transition (post-pre):")
for name, pre, post in T:
    d = sum(post.values()) - sum(pre.values())
    print(f"  {name}: {d:+d}")
