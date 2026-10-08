#!/usr/bin/env python3
"""Generator of Promela models that read `_nr_pr` with `active` processes, for the
differential of the engine against SPIN's pan under none and weak fairness
(steps/integration-0.3.0-notes.md, B (f)).

    python3 integration-0.3.0-nrpr-wf-gen.py OUTDIR COUNT FIRSTSEED [pan]

Writes OUTDIR/n<seed>.pml and n<seed>.meta ({"mode": "acc"|"prog"|"ltl"|"claim",
"formula": ...}), the layout the weak-fairness oracle reads with
MCD_WF_PROMELA_DIR. With the word `pan` the `_nr_pr` constants of the models that
have a claim (never claim, ltl, non-progress) are one higher: pan counts the claim in
`_nr_pr` and the engine does not (testdata/spin-divergence), so these are the same
models for pan. The runners (engine and pan, both fairness settings) are the
weak-fairness author's scratch scripts and are not part of the repository.
"""
import random, sys, os, json
out=sys.argv[1]; n=int(sys.argv[2]); seed0=int(sys.argv[3]); PAN=len(sys.argv)>4 and sys.argv[4]=="pan"
CUR={"mode":None}
os.makedirs(out,exist_ok=True)
def cmp_nr(r):
    op=r.choice(["==","<=",">=","<",">","!="]); k=r.randint(1,3)
    # pan counts the claim (never claim, ltl, np_) in _nr_pr: its constants are one above the engine's
    if PAN and CUR["mode"]!="acc": k+=1
    return "_nr_pr %s %d"%(op,k)
def simple(r, labels):
    k=r.random()
    if k<0.25: return "a = 1 - a"
    if k<0.4: return "b = 1 - b"
    if k<0.5: return "c = (c + 1) % 3"
    if k<0.8: return "(%s)"%cmp_nr(r)
    if k<0.9: return "skip"
    return "(a == %d)"%r.randint(0,1)
def body(r, mode, pidx, loops_ok):
    stmts=[simple(r,mode) for _ in range(r.randint(1,3))]
    if loops_ok and r.random()<0.75:
        alts=[]
        for _ in range(r.randint(1,3)):
            g=r.choice(["", "(%s) -> "%cmp_nr(r), "(a == %d) -> "%r.randint(0,1)])
            lab=""
            if mode=="acc" and r.random()<0.4: lab="accept_%d_%d: "%(pidx,len(alts))
            if mode=="prog" and r.random()<0.4: lab="progress_%d_%d: "%(pidx,len(alts))
            alts.append(":: %s%s%s"%(g, lab, simple(r,mode)))
        if r.random()<0.4: alts.append(":: else -> skip")
        loop="do\n\t"+"\n\t".join(alts)+"\n\tod"
        if r.random()<0.5: return "\n\t".join(s+";" for s in stmts)+"\n\t"+loop
        return loop
    return ";\n\t".join(stmts)
for i in range(n):
    r=random.Random(seed0+i)
    mode=r.choice(["acc","acc","prog","ltl","claim"]); CUR["mode"]=mode
    nproc=r.randint(2,3)
    src=["bit a, b;","byte c;"]
    formula=""
    if mode=="ltl":
        formula=r.choice(["[]<>p","<>[]p","[](p -> <>q)","<>p"])
        src.append("#define p (a == 1)"); src.append("#define q (b == 1)")
    for p in range(nproc):
        loops_ok = (p==nproc-1) or r.random()<0.5   # the last (youngest) usually loops: the older ones then wait at their end
        src.append("active proctype P%d()\n{\n\t%s\n}"%(p, body(r,mode,p,loops_ok)))
    if mode=="claim":
        c=r.choice(["accept_c: do :: (a == 1) od","do :: (a == 0) :: (b == 1) -> goto accept_c od; accept_c: do :: (b == 1) od","do :: skip :: (a == 1) -> goto T1 od; T1: do :: (b == 0) -> goto accept_d od; accept_d: do :: (b == 0) od"])
        src.append("never {\n%s\n}"%c)
    open("%s/n%d.pml"%(out,seed0+i),"w").write("\n".join(src)+"\n")
    json.dump({"mode":mode,"formula":formula},open("%s/n%d.meta"%(out,seed0+i),"w"))
