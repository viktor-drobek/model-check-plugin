#!/usr/bin/env python3
"""Probe the engine for every construct `references/promela-subset.md` classifies.

The reference's §1 (inside the subset) and §3 (outside it) are claims about the
engine. This script turns each claim into a minimal model that exercises that one
construct and nothing else, runs `mcd parse` on it, and records what actually
happened. It is the evidence behind the table: re-run it instead of trusting the
table.

    python3 probe.py --mcd /tmp/mcd            # write the .pml files, run, report
    python3 probe.py --mcd /tmp/mcd --json results.json

Each probe declares `claim`: "inside" (the reference says the engine accepts it),
"outside" (the reference says it is refused) or "defect" (the engine accepts it, the
reference says so, and says it is a defect rather than a boundary — SPIN refuses the
file). The claims were brought into line with the engine by the row-by-row pass of
`steps/g3-evals3-subset-probe.md`, so **a clean run reports zero disagreements**:
any disagreement now means the engine and the reference have drifted apart again,
which is the whole point of keeping this script.

`mcd parse` exit codes (engine-tools.md §2): 0 = an IR document, 2 = the frontend
rejected the input, 1 = a tool error. A construct that parses at exit 0 is inside the
subset whatever the reference says about it.
"""

import argparse
import json
import os
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))

# (id, claim, construct as the reference names it, source) -> model text.
# Every model is the smallest thing that exercises the construct; where a construct
# needs a carrier (a proctype, a channel) the carrier is plain MVP Promela so that a
# rejection can only come from the construct under test.
PROBES = [
    # ---------------------------------------------------------------- §1: inside
    ("in-proctype-active-init", "inside", "proctype, active proctype, active [N] proctype, init", "§1 Processes",
     "proctype P() { skip }\nactive proctype A() { skip }\nactive [2] proctype B() { skip }\ninit { run P() }\n"),
    ("in-params-pid", "inside", "proctype parameters, _pid", "§1 Processes",
     "proctype P(byte x) { byte me = _pid; me = x }\ninit { run P(3) }\n"),
    ("in-run-in-loop", "inside", "run inside a loop (G5)", "§1 Processes",
     "byte n;\nproctype W() { skip }\ninit { do :: n < 2 -> n++; run W() :: else -> break od }\n"),
    ("in-run-recursive", "inside", "recursive run (G5)", "§1 Processes",
     "proctype R(byte d) { if :: d > 0 -> run R(d - 1) :: else -> skip fi }\ninit { run R(2) }\n"),
    ("in-types-scalar", "inside", "bit, bool, byte, short, int, pid", "§1 Types",
     "bit a; bool b; byte c; short d; int e; pid f;\nactive proctype P() { a = 1; b = true; c = 2; d = 3; e = 4; f = 0 }\n"),
    ("in-types-array", "inside", "one-dimensional array of fixed length", "§1 Types",
     "byte a[3];\nactive proctype P() { a[1] = 2 }\n"),
    ("in-mtype-eq-form", "inside", "mtype = { a, b }", "§1 mtype",
     "mtype = { red, green };\nactive proctype P() { mtype m; m = red; m = green }\n"),
    ("in-mtype-brace-form", "inside", "mtype { a, b }", "§1 mtype",
     "mtype { red, green };\nactive proctype P() { mtype m; m = red }\n"),
    ("in-chan-buffered", "inside", "chan c = [N] of { t }; send and receive", "§1 Channels",
     "chan c = [2] of { byte };\nactive proctype S() { c!1 }\nactive proctype R() { byte x; c?x }\n"),
    ("in-chan-rendezvous", "inside", "chan c = [0] of { t } (rendezvous)", "§1 Channels",
     "chan c = [0] of { byte };\nactive proctype S() { c!1 }\nactive proctype R() { byte x; c?x }\n"),
    ("in-chan-multifield", "inside", "c!e1,e2 and c!e1(e2) forms", "§1 Channels",
     "mtype = { m };\nchan c = [2] of { mtype, byte };\nactive proctype S() { c!m,1; c!m(2) }\n"),
    ("in-chan-recv-constant", "inside", "c?x with a constant (pattern match)", "§1 Channels",
     "mtype = { m };\nchan c = [2] of { mtype, byte };\nactive proctype R() { byte x; c?m,x }\n"),
    ("in-chan-predicates", "inside", "len, empty, full, nempty, nfull", "§1 Channels",
     "chan c = [2] of { byte };\nactive proctype P() { byte n; n = len(c);\n"
     "  if :: empty(c) -> skip :: full(c) -> skip :: nempty(c) -> skip :: nfull(c) -> skip fi }\n"),
    ("in-chan-xr-xs", "inside", "xr c / xs c as hints", "§1 Channels",
     "chan c = [2] of { byte };\nactive proctype P() { xr c; xs c; c!1 }\n"),
    ("in-control", "inside", "if/fi, do/od, ::, ->, else, break, goto, labels", "§1 Control",
     "active proctype P() { byte n;\nagain: if :: n == 0 -> n = 1 :: else -> goto again fi;\n"
     "  do :: n > 0 -> n--; break :: else -> break od }\n"),
    ("in-label-prefixes", "inside", "end, progress, accept label prefixes", "§1 Control",
     "bit x;\nactive proctype P() { do :: x = 1 - x od }\n"
     "active proctype Q() {\nprogress_p: x = 1;\naccept_a: x = 0;\nend_e: skip }\n"),
    ("in-atomic-dstep", "inside", "atomic { … }, d_step { … }", "§1 Atomicity",
     "byte n;\nactive proctype P() { atomic { n = 1; n = 2 }; d_step { n = 3; n = 4 } }\n"),
    ("in-statements", "inside", "assignment, guard, assert, skip, true, false, printf", "§1 Statements",
     "byte n;\nactive proctype P() { n = 1; n > 0; assert(n == 1); skip; true; printf(\"n=%d\\n\", n) }\n"),
    ("in-timeout", "inside", "timeout", "§1 Statements",
     "chan c = [0] of { byte };\nactive proctype P() { byte x; do :: c?x :: timeout -> break od }\n"),
    ("in-define-object", "inside", "#define NAME body", "§1 Preprocessor",
     "#define LIMIT 3\nbyte n;\nactive proctype P() { n = LIMIT }\n"),
    ("in-define-function", "inside", "#define NAME(args) body", "§1 Preprocessor",
     "#define INC(v) v = v + 1\nbyte n;\nactive proctype P() { INC(n) }\n"),
    ("in-define-continuation", "inside", "#define with a \\ continuation", "§1 Preprocessor",
     "#define BOTH(a, b) a = 1; \\\n                   b = 2\nbyte x; byte y;\nactive proctype P() { BOTH(x, y) }\n"),
    ("in-ifdef-family", "inside", "#ifdef/#ifndef/#if/#elif/#else/#endif, defined(), #undef", "§1 Preprocessor",
     "#define A 1\n#ifdef A\nbyte n;\n#endif\n#ifndef B\nbyte m;\n#endif\n"
     "#if defined(A)\nbyte k;\n#elif 1\nbyte j;\n#else\nbyte i;\n#endif\n#undef A\n"
     "active proctype P() { n = 1; m = 2; k = 3 }\n"),
    ("in-never-claim", "inside", "never { … }", "§1 Properties",
     "bit x;\nactive proctype P() { do :: x = 1 - x od }\n"
     "never { do :: !x -> skip od }\n"),
    ("in-expressions", "inside", "arithmetic, comparison, &&, ||, !, %", "§1 Expressions",
     "byte n;\nactive proctype P() { n = (1 + 2) * 3 - 4;\n"
     "  if :: n > 1 && n < 100 || !(n == 0) -> n = n % 5 :: else -> skip fi }\n"),
    ("in-remote-label-in-never", "outside", "remote label reference P@label inside a never claim", "§1 Expressions",
     "bit x;\nactive proctype P() {\nL0: x = 1;\nL1: x = 0 }\n"
     "never { do :: P@L0 -> skip od }\n"),

    # -------------------------------------------------------------- §3: outside
    ("out-inline", "inside", "inline name(args) { … }", "§3",
     "inline bump(v) { v = v + 1 }\nbyte n;\nactive proctype P() { bump(n) }\n"),
    ("out-typedef", "inside", "typedef", "§3",
     "typedef Pair { byte a; byte b };\nPair p;\nactive proctype P() { p.a = 1 }\n"),
    ("out-provided", "inside", "provided (e)", "§3",
     "byte turn;\nactive proctype P() provided (turn == 0) { turn = 1 }\n"),
    ("out-chan-in-message", "inside", "channels as message fields", "§3",
     "chan reply = [1] of { byte };\nchan req = [1] of { chan };\n"
     "active proctype S() { req!reply }\n"),
    ("out-chan-array", "inside", "arrays of channels", "§3",
     "chan c[2] = [1] of { byte };\nactive proctype P() { c[0]!1 }\n"),
    ("out-chan-uninitialised", "inside", "uninitialised channel variables", "§3",
     "chan c;\nactive proctype P() { skip }\n"),
    ("out-nr-pr", "inside", "_nr_pr", "§3",
     "byte n;\nactive proctype P() { n = _nr_pr }\n"),
    ("out-unless", "outside", "unless", "§3",
     "byte n;\nactive proctype P() { { n = 1; n = 2 } unless { n > 0 -> n = 3 } }\n"),
    ("out-c-code", "outside", "c_code", "§3",
     "c_code { int q; }\nactive proctype P() { skip }\n"),
    ("out-c-expr", "outside", "c_expr", "§3",
     "active proctype P() { if :: c_expr { 1 } -> skip fi }\n"),
    ("out-c-decl", "outside", "c_decl", "§3",
     "c_decl { extern int q; }\nactive proctype P() { skip }\n"),
    ("out-c-state", "outside", "c_state", "§3",
     "c_state \"int q\" \"Global\"\nactive proctype P() { skip }\n"),
    ("out-c-track", "outside", "c_track", "§3",
     "c_code { int q; }\nc_track \"&q\" \"sizeof(int)\"\nactive proctype P() { skip }\n"),
    ("out-eval-in-receive", "outside", "eval(e) in a receive", "§3",
     "chan c = [1] of { byte };\nactive proctype P() { byte x; c?eval(x) }\n"),
    ("out-priority", "outside", "priority", "§3",
     "active proctype P() priority 2 { skip }\n"),
    ("out-bit-operators", "outside", "bit operators", "§3",
     "byte n;\nactive proctype P() { n = (1 & 3) | (4 ^ 5) }\n"),
    ("out-shift-operators", "outside", "shift operators", "§3",
     "byte n;\nactive proctype P() { n = (1 << 2) >> 1 }\n"),
    ("out-conditional-expr", "outside", "?: conditional expression", "§3",
     "byte n;\nactive proctype P() { n = (1 > 0 -> 2 : 3) }\n"),
    ("out-run-in-expression", "inside", "run as the whole right-hand side (pid = run P())", "§3",
     "proctype Q() { skip }\nbyte n;\ninit { n = run Q() }\n"),
    ("out-run-nested-in-expression", "outside", "run nested inside a larger expression", "§3",
     "proctype Q() { skip }\nbyte n;\ninit { n = 1 + run Q() }\n"),
    ("out-remote-variable", "outside", "remote variable reference P[i]:var", "§3",
     "active proctype P() { byte v; v = 1 }\nactive proctype Q() { byte w; w = P[0]:v }\n"),
    ("out-poll-receive", "outside", "c?[…] poll", "§3",
     "chan c = [1] of { byte };\nactive proctype P() { byte x; if :: c?[x] -> skip fi }\n"),
    ("out-sorted-receive", "outside", "c?? random/sorted receive", "§3",
     "chan c = [1] of { byte };\nactive proctype P() { byte x; c??x }\n"),
    ("out-copy-receive", "outside", "c?<…> copy receive", "§3",
     "chan c = [1] of { byte };\nactive proctype P() { byte x; c?<x> }\n"),
    ("out-unsigned", "outside", "unsigned", "§3",
     "unsigned n : 3;\nactive proctype P() { n = 1 }\n"),
    ("out-hidden-qualifier", "outside", "hidden qualifier", "§3",
     "hidden byte n;\nactive proctype P() { n = 1 }\n"),
    ("out-local-qualifier", "outside", "local qualifier", "§3",
     "local byte n;\nactive proctype P() { n = 1 }\n"),
    ("out-show-qualifier", "outside", "show qualifier", "§3",
     "show byte n;\nactive proctype P() { n = 1 }\n"),
    ("out-ltl-block", "outside", "ltl name { … } block", "§3",
     "bit x;\nactive proctype P() { do :: x = 1 - x od }\nltl p { []<> x }\n"),
    ("out-pc-value", "inside", "pc_value(pid)", "§3 (plan §5.2 lists it as v1)",
     "byte n;\nactive proctype P() { skip }\nactive proctype Q() { n = pc_value(0) }\n"),
    ("out-include", "outside", "#include", "§3",
     "#include \"other.pml\"\nactive proctype P() { skip }\n"),
    # ---- name collisions: `semantic` refusals that SPIN makes too (G5 addendum 2)
    ("out-redeclare-enclosing", "outside", "redeclaration while the enclosing scope is open", "§3 names",
     "active proctype P() { byte n; n = 1; { byte n; n = 2 } }\n"),
    ("out-redeclare-same-scope", "outside", "redeclaration in the same scope", "§3 names",
     "active proctype P() { byte n; byte n; n = 1 }\n"),
    ("out-redeclare-parameter", "outside", "local shadowing a proctype parameter", "§3 names",
     "proctype P(byte n) { byte n; n = 1 }\ninit { run P(1) }\n"),
    ("out-redeclare-global", "outside", "local shadowing a global", "§3 names",
     "byte n;\nactive proctype P() { byte n; n = 1 }\n"),
    ("out-redeclare-if-options", "outside", "two if options declaring one name", "§3 names",
     "active proctype P() { if :: byte n; n = 1 :: byte n; n = 2 fi }\n"),
    ("out-duplicate-label", "outside", "duplicate label in one proctype", "§3 names",
     "active proctype P() {\nL: skip;\nL: skip }\n"),
    ("out-label-and-variable", "outside", "one identifier as both label and variable", "§3 names",
     "active proctype P() { byte L;\nL: L = 1 }\n"),
    ("out-label-and-global", "outside", "label colliding with a global", "§3 names",
     "byte g;\nactive proctype P() {\ng: skip }\n"),
    ("out-label-and-mtype", "outside", "label colliding with an mtype constant", "§3 names",
     "mtype = { m };\nactive proctype P() {\nm: skip }\n"),
    ("out-proctype-and-global", "outside", "proctype name colliding with a global", "§3 names",
     "byte P;\nactive proctype P() { skip }\n"),
    ("out-proctype-and-mtype", "outside", "proctype name colliding with an mtype constant", "§3 names",
     "mtype = { P };\nactive proctype P() { skip }\n"),
    ("out-two-globals", "outside", "two globals of one name", "§3 names",
     "byte n;\nbyte n;\nactive proctype P() { n = 1 }\n"),
    ("in-redeclare-siblings", "inside", "sibling blocks declaring one name (the scope has closed)", "§3 names",
     "active proctype P() { { byte n; n = 1 }; { byte n; n = 2 } }\n"),

    # ---- deliberate divergences: the engine is stricter than SPIN (G5 addendum 2 §4)
    ("out-run-arity-few", "outside", "run with fewer arguments than parameters", "§3 divergences",
     "proctype P(byte x) { x = 1 }\ninit { run P() }\n"),
    ("out-run-arity-many", "outside", "run with more arguments than parameters", "§3 divergences",
     "proctype P(byte x) { x = 1 }\ninit { run P(1, 2) }\n"),
    ("out-sibling-different-types", "outside", "sibling blocks declaring one name with different types", "§3 divergences",
     "active proctype P() { { int y; y = 1 }; { byte y; y = 2 } }\n"),
]


def run(mcd, path):
    p = subprocess.run([mcd, "parse", "--promela", path],
                       capture_output=True, text=True)
    out = {"exit": p.returncode}
    if p.returncode == 0:
        out["outcome"] = "parsed"
        try:
            ir = json.loads(p.stdout)
            out["processes"] = [q["name"] for q in ir.get("processes", [])]
            out["properties"] = [q["id"] for q in ir.get("properties", [])]
        except json.JSONDecodeError:
            out["outcome"] = "parsed-unreadable-ir"
        if p.stderr.strip():
            out["warnings"] = p.stderr.strip().splitlines()
        return out
    try:
        err = json.loads(p.stdout)["error"]
        out["outcome"] = "rejected"
        out["kind"] = err.get("kind")
        out["message"] = err.get("message")
        out["path"] = err.get("path")
    except (json.JSONDecodeError, KeyError):
        out["outcome"] = "tool-error"
        out["message"] = p.stderr.strip()[:200]
    return out


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--mcd", default="/tmp/mcd")
    ap.add_argument("--json", default=os.path.join(HERE, "results.json"))
    ap.add_argument("--keep", action="store_true",
                    help="only (re)write the .pml files, do not run")
    args = ap.parse_args(argv)

    rows, disagreements = [], []
    for pid, claim, construct, source, body in PROBES:
        path = os.path.join(HERE, pid + ".pml")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write("/* probe: %s\n * claim: the reference's %s says this is %s the subset\n */\n"
                     % (construct, source, "INSIDE" if claim == "inside" else "OUTSIDE"))
            fh.write(body)
        if args.keep:
            continue
        r = run(args.mcd, path)
        actual_inside = r["outcome"].startswith("parsed")
        # "defect" behaves like "inside" for the comparison: the engine does parse
        # it. The reference records it as a defect, and §3 says so in words.
        agrees = actual_inside == (claim in ("inside", "defect"))
        row = {"id": pid, "claim": claim, "construct": construct, "source": source,
               "agrees": agrees, **r}
        rows.append(row)
        if not agrees:
            disagreements.append(row)

    if args.keep:
        print("wrote %d probe files" % len(PROBES))
        return 0

    with open(args.json, "w", encoding="utf-8") as fh:
        json.dump({"mcd": subprocess.run([args.mcd, "version"], capture_output=True,
                                         text=True).stdout.strip(),
                   "probes": rows}, fh, ensure_ascii=False, indent=1)
        fh.write("\n")

    for r in rows:
        mark = "ok      " if r["agrees"] else "DISAGREE"
        detail = r["outcome"]
        if r["outcome"] == "rejected":
            detail += " (%s) %s" % (r.get("kind"), (r.get("message") or "")[:70])
        print("%s %-26s %-8s %s" % (mark, r["id"], r["claim"], detail))
    defects = [r for r in rows if r["claim"] == "defect"]
    print("\n%d probes, %d disagree with the reference" % (len(rows), len(disagreements)))
    if defects:
        print("%d probe(s) record a known engine defect rather than a boundary: %s"
              % (len(defects), ", ".join(r["id"] for r in defects)))
    if disagreements:
        print("A disagreement means references/promela-subset.md and the engine have "
              "drifted; re-derive the row rather than editing this script to agree.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
