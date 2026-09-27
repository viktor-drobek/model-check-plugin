#!/usr/bin/env python3
"""Assemble the property list for the check.

This script does no checking and no reachability computation: it only builds
IR expression objects (engine-tools.md §4, "Adding your own properties without
the server") and appends them to the `properties` array of the IR that
`mcd parse --petri net.json` produced.  The engine decides every verdict.

in:  ir.json       (output of `mcd parse --petri net.json`)
out: ir-props.json (same IR + the reach properties below)
"""
import json, pathlib

here = pathlib.Path(__file__).parent
ir = json.loads((here / "ir.json").read_text())

var = lambda p: {"op": "var", "var": p}
const = lambda n: {"op": "const", "value": n} if n else {"op": "const"}
cmp_ = lambda op, p, n: {"op": op, "args": [var(p), const(n)]}


def conj(terms):
    e = terms[0]
    for t in terms[1:]:
        e = {"op": "and", "args": [e, t]}
    return e


places = ["p1", "p2", "p3", "p4", "p5", "p6"]

props = []

# P3 — the suspected hang marking, as an exact marking.
props.append({
    "id": "reach_hang_marking",
    "kind": "reach",
    "text": "the marking p2=1, p5=1 and every other place empty is reachable",
    "expr": conj([cmp_("eq", p, 1 if p in ("p2", "p5") else 0) for p in places]),
})

# P4..P9 — enabledness of each transition is reachable.
# petri-nets.md §4: transition t is dead iff "all input places of t hold their
# weights" is unreachable.  All weights here are 1.
inputs = {
    "t1": ["p1"],
    "t2": ["p2", "p4"],
    "t3": ["p3"],
    "t4": ["p4"],
    "t5": ["p1", "p5"],
    "t6": ["p6"],
}
for t, ins in inputs.items():
    props.append({
        "id": "reach_enabled_" + t,
        "kind": "reach",
        "text": "a reachable marking enables %s (inputs %s hold a token)" % (t, ", ".join(ins)),
        "expr": conj([cmp_("ge", p, 1) for p in ins]),
    })

ir["properties"].extend(props)
(here / "ir-props.json").write_text(json.dumps(ir, indent=2) + "\n")
print("added:", ", ".join(p["id"] for p in props))
