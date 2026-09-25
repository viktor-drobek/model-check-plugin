# Полный перебор графа достижимости сети Петри (p1..p6, t1..t6).
from collections import deque
P = ["p1","p2","p3","p4","p5","p6"]
T = {  # name: (pre, post)
 "t1": ({"p1":1}, {"p2":1}),
 "t2": ({"p2":1,"p4":1}, {"p3":1}),
 "t3": ({"p3":1}, {"p1":1,"p4":1}),
 "t4": ({"p4":1}, {"p5":1}),
 "t5": ({"p1":1,"p5":1}, {"p6":1}),
 "t6": ({"p6":1}, {"p4":1,"p1":1}),
}
M0 = (1,0,0,1,0,0)
idx = {p:i for i,p in enumerate(P)}
def enabled(m,t):
    return all(m[idx[p]]>=k for p,k in T[t][0].items())
def fire(m,t):
    m=list(m)
    for p,k in T[t][0].items(): m[idx[p]]-=k
    for p,k in T[t][1].items(): m[idx[p]]+=k
    return tuple(m)
def show(m): return "{"+", ".join(f"{p}={v}" for p,v in zip(P,m) if v)+"}"
seen={M0:None}; q=deque([M0]); edges=[]; dead=[]
while q:
    m=q.popleft(); en=[t for t in T if enabled(m,t)]
    if not en: dead.append(m)
    for t in en:
        n=fire(m,t); edges.append((m,t,n))
        if n not in seen: seen[n]=(m,t); q.append(n)
print("Достижимых маркировок:",len(seen))
for m in seen: print(" ",show(m))
print("Дуги:")
for a,t,b in edges: print(f"  {show(a)} --{t}--> {show(b)}")
print("Максимум фишек в месте:",max(max(m) for m in seen))
print("Тупиковые маркировки:",[show(m) for m in dead])
for d in dead:
    path=[]; m=d
    while seen[m]: pm,t=seen[m]; path.append(t); m=pm
    print("  трасса из M0:", " -> ".join(reversed(path)), "=>", show(d))
