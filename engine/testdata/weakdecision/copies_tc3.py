# independent re-derivation of the n+2 copies product for tc3.pml (accept mode, no claim, weak fairness)
# (testdata/promela/weakfair-timeout-null.pml; the engine's product must have exactly this many states, features/g4-ltl.feature)
# B: 0 == 1 (never enabled). P: do :: accept_p: timeout -> a = 1 - a od.  n = 2 non-claim processes (B, P).
# system state (a, pcP); B never moves; timeout is true iff no ordinary statement is executable
def moves(s):
    a, pc = s
    if pc == 0: return [("P", (a, 1))]          # timeout: B blocked, P's only edge is the timeout one
    return [("P", (1 - a, 0))]
def blocked(p, s): return p == "B"
def accepting(s): return s[1] == 0
n, procs = 2, ["B", "P"]
init = ((0, 0), 0)
seen, todo, edges = {init}, [init], 0
while todo:
    s, k = todo.pop()
    succ = []
    if k == 0:
        if accepting(s): succ.append((s, 1))
        else: succ += [(t, 0) for _, t in moves(s)]
    else:
        if k <= n and blocked(procs[k-1], s): succ.append((s, k + 1))
        for p, t in moves(s):
            if k == n + 1: succ.append((t, 0))
            else: succ.append((t, k + 1 if procs[k-1] == p else k))
    for x in succ:
        edges += 1
        if x not in seen: seen.add(x); todo.append(x)
print("states", len(seen), "edges", edges)
