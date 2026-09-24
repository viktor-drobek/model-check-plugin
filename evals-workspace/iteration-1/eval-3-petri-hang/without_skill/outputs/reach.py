from collections import deque
P=['p1','p2','p3','p4','p5','p6']
T={'t1':(['p1'],['p2']),'t2':(['p2','p4'],['p3']),'t3':(['p3'],['p1','p4']),
   't4':(['p4'],['p5']),'t5':(['p1','p5'],['p6']),'t6':(['p6'],['p4','p1'])}
def fire(m,t):
    pre,post=T[t]
    if any(m[p]<1 for p in pre): return None
    n=dict(m)
    for p in pre: n[p]-=1
    for p in post: n[p]+=1
    return n
def key(m): return tuple(m[p] for p in P)
def show(m): return '{'+', '.join(f'{p}={m[p]}' for p in P if m[p])+'}'
m0={p:0 for p in P}; m0['p1']=1; m0['p4']=1
seen={key(m0):(m0,[])}; q=deque([m0]); edges=[]
while q:
    m=q.popleft()
    for t in T:
        n=fire(m,t)
        if n is None: continue
        edges.append((show(m),t,show(n)))
        if key(n) not in seen:
            seen[key(n)]=(n,seen[key(m)][1]+[t]); q.append(n)
print('states:',len(seen))
for k,(m,path) in seen.items():
    en=[t for t in T if fire(m,t)]
    print(show(m),'enabled:',en,'path:',path, '<-- DEADLOCK' if not en else '')
print('bound:',max(max(k) for k in seen))
for e in edges: print(*e)
