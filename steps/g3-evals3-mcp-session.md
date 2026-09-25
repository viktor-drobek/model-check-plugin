# G3 (evals, stage 3) — the MCP path, driven by hand once

Protocol point 4 of the step: the examples in `skills/model-check/references/engine-tools.md`
must be copied from real server output, not written from memory. This file is that
output. It was produced by one JSON-RPC session over **stdio** against the binary
built from `model-check-plugin/engine` (`go build -o /tmp/mcd ./cmd/mcd`), driven by
a small Python client (`initialize` → `notifications/initialized` → `tools/call`),
the same way `steps/g2-confirmation.md` §3 drove the G2 server.

```
/tmp/mcd serve --session-dir <session-dir> --max-states 1000000 --max-ms 60000
mcd serve 0.1.0-g0: sessions under <session-dir>
```

Two things are being checked here that the eval runs cannot check (an MCP server
cannot be registered into a subagent's session):

1. `mc_parse{promela}` **works now**. In the G2/G3-stage-2 build it answered
   `outcome: not-executed` ("promela frontend … not linked into this server");
   G4 wired `mcp.PromelaViaCLI` into `cmd/mcd/serve.go`, and the call below returns
   `outcome: "ir"`. The paragraph in `engine-tools.md` and the scenario in
   `features/g3-evals.feature` that recorded the old state were retargeted in this
   step.
2. The G4 fields reach the MCP layer: `formula` on an `ltl` property, `temporal`
   in the report, `counterexample.loop`, and `prefix`/`loop`/`loop_note` from
   `mc_explain`.

Paths and session ids below are replaced by `<session-dir>` and `s<UTC>-000n`;
nothing else is edited. Long documents (the IR, the full report) are truncated
where marked and the truncation is stated.

Contents: 1 session A — `mutex_flaw.pml` (parse, assertion check, explain) ·
2 session B — `starvation.pml` (an `ltl` formula, a lasso, the three fairness
settings) · 3 the manifest · 4 what this session establishes.

## 1. Session A — `Promela - examples/CH2/mutex_flaw.pml`

### 1.1. `mc_parse` with the `promela` field

Request (the model text is passed inline, not as a path — the server reads files only under `--allow-read`):

```json
{
 "name": "mc_parse",
 "arguments": {
  "promela": "«the 30 lines of CH2/mutex_flaw.pml»"
 }
}
```

Response (`structuredContent`; the `ir` object is the full `mcd-ir/1` document and is left out here — it is written to `ir_path`):

```json
{
 "ir": "«the mcd-ir/1 document, also written to ir_path»",
 "ir_path": "<session-dir>/s<UTC>-0001/ir-1.json",
 "origins": [
  {
   "element": "model",
   "file": "inline",
   "name": "inline"
  },
  {
   "element": "globals[0]",
   "file": "inline",
   "line": 1,
   "name": "cnt"
  },
  {
   "element": "globals[1]",
   "file": "inline",
   "line": 2,
   "name": "x"
  },
  {
   "element": "globals[2]",
   "file": "inline",
   "line": 2,
   "name": "y"
  },
  "… 43 more origin entries"
 ],
 "outcome": "ir",
 "session_id": "s<UTC>-0001",
 "warnings": []
}
```

`outcome` is `ir`: **the Promela frontend is linked into `mcd serve`**. `origins` gives `file: "inline"` because the text came inline; with `file` input it carries the real path.

### 1.2. `mc_check` with an assertion property

The property list replaces the model's own properties, and the implicit `assert` is always added — so the answer carries both `mutex` (the invariant asked for, as an IR expression) and `assert` (the model's own `assert(cnt == 1)`):

```json
{
 "name": "mc_check",
 "arguments": {
  "session_id": "s<UTC>-0001",
  "properties": [
   {
    "id": "mutex",
    "kind": "invariant",
    "expr": {
     "op": "le",
     "args": [
      {
       "op": "var",
       "var": "cnt"
      },
      {
       "op": "const",
       "value": 1
      }
     ]
    },
    "text": "at most one process in the critical section"
   }
  ],
  "search": "bfs",
  "no_timing": true
 }
}
```

Response:

```json
{
 "outcome": "report",
 "properties": [
  {
   "complete": false,
   "counterexample": {
    "id": "cex-1",
    "path": "<session-dir>/s<UTC>-0001/cex/cex-1.json",
    "steps": 14,
    "summary": "x = me, (y == 0 || y == me), z = me, (x == me), x = me, (y == 0 || y == me), y = me, (z == me), cnt++, z = me, (x == me), y = me, (z == me), cnt++",
    "user_names": [
     "x = me",
     "(y == 0 || y == me)",
     "z = me",
     "(x == me)",
     "x = me",
     "(y == 0 || y == me)",
     "y = me",
     "(z == me)",
     "cnt++",
     "z = me",
     "(x == me)",
     "y = me",
     "(z == me)",
     "cnt++"
    ]
   },
   "counters": {
    "depth": 14,
    "memory_bytes_est": 49152,
    "states": 363,
    "transitions": 657
   },
   "evidence": "exhaustive",
   "id": "mutex",
   "kind": "invariant",
   "reason": "invariant cnt <= 1 is false in the final state of the counterexample",
   "status": "violated",
   "text": "at most one process in the critical section"
  },
  {
   "complete": false,
   "counterexample": {
    "id": "cex-2",
    "path": "<session-dir>/s<UTC>-0001/cex/cex-2.json",
    "steps": 15,
    "summary": "x = me, (y == 0 || y == me), z = me, (x == me), x = me, (y == 0 || y == me), y = me, (z == me), cnt++, z = me, (x == me), y = me, (z == me), cnt++, assert(cnt == 1)",
    "user_names": [
     "x = me",
     "(y == 0 || y == me)",
     "z = me",
     "(x == me)",
     "x = me",
     "(y == 0 || y == me)",
     "y = me",
     "(z == me)",
     "cnt++",
     "z = me",
     "(x == me)",
     "y = me",
     "(z == me)",
     "cnt++",
     "assert(cnt == 1)"
    ]
   },
   "counters": {
    "depth": 14,
    "memory_bytes_est": 49152,
    "states": 363,
    "transitions": 657
   },
   "evidence": "exhaustive",
   "id": "assert",
   "kind": "assert",
   "reason": "assert(cnt == 1) fails in the last step of the counterexample",
   "status": "violated",
   "text": "no assert statement fails"
  }
 ],
 "report_path": "<session-dir>/s<UTC>-0001/check-1.json",
 "search": {
  "budget_applied": {
   "depth": 1000000,
   "memory_mb": 1024,
   "ms": 60000,
   "states": 1000000
  },
  "budget_notes": [],
  "budget_requested": {},
  "complete": false,
  "mode": "bfs",
  "stop": "all properties decided"
 },
 "session_id": "s<UTC>-0001",
 "warnings": []
}
```

### 1.3. `mc_explain` on the counterexample

```json
{
 "name": "mc_explain",
 "arguments": {
  "session_id": "s<UTC>-0001",
  "counterexample_id": "cex-1"
 }
}
```

Response (the `prefix` is the whole run — a safety violation has no loop, and `loop_note` says so):

```json
{
 "final_state": [
  {
   "value": 1,
   "var": "user:0.me"
  },
  {
   "value": 2,
   "var": "user:1.me"
  },
  {
   "value": 2,
   "var": "cnt"
  },
  {
   "value": 2,
   "var": "x"
  },
  {
   "value": 2,
   "var": "y"
  },
  {
   "value": 2,
   "var": "z"
  }
 ],
 "id": "cex-1",
 "loop": [],
 "loop_note": "finite run: the loop is empty (a cycle counterexample of an ltl or progress property has a non-empty loop, the steps that repeat forever)",
 "path": "<session-dir>/s<UTC>-0001/cex/cex-1.json",
 "prefix": [
  {
   "changes": [
    {
     "after": 1,
     "before": 0,
     "var": "x"
    }
   ],
   "command": "x = me",
   "index": 1,
   "location": "L2",
   "origin": {
    "file": "inline",
    "line": 5,
    "name": "x = me"
   },
   "process": "user:0",
   "user_name": "x = me"
  },
  {
   "changes": [],
   "command": "(y == 0 || y == me)",
   "index": 2,
   "location": "L3",
   "origin": {
    "file": "inline",
    "line": 8,
    "name": "(y == 0 || y == me)"
   },
   "process": "user:0",
   "user_name": "(y == 0 || y == me)"
  },
  {
   "changes": [
    {
     "after": 1,
     "before": 0,
     "var": "z"
    }
   ],
   "command": "z = me",
   "index": 3,
   "location": "L4",
   "origin": {
    "file": "inline",
    "line": 10,
    "name": "z = me"
   },
   "process": "user:0",
   "user_name": "z = me"
  },
  "… steps 4–14",
  {
   "changes": [],
   "command": "(z == me)",
   "index": 13,
   "location": "L7",
   "origin": {
    "file": "inline",
    "line": 18,
    "name": "(z == me)"
   },
   "process": "user:1",
   "user_name": "(z == me)"
  },
  {
   "changes": [
    {
     "after": 2,
     "before": 1,
     "var": "cnt"
    }
   ],
   "command": "cnt++",
   "index": 14,
   "origin": {
    "file": "inline",
    "line": 22,
    "name": "cnt++"
   },
   "process": "user:1",
   "user_name": "cnt++"
  }
 ],
 "property_id": "mutex",
 "role": "counterexample",
 "session_id": "s<UTC>-0001",
 "summary": "x = me, (y == 0 || y == me), z = me, (x == me), x = me, (y == 0 || y == me), y = me, (z == me), cnt++, z = me, (x == me), y = me, (z == me), cnt++",
 "user_names": [
  "x = me",
  "(y == 0 || y == me)",
  "z = me",
  "(x == me)",
  "x = me",
  "(y == 0 || y == me)",
  "y = me",
  "(z == me)",
  "cnt++",
  "z = me",
  "(x == me)",
  "y = me",
  "(z == me)",
  "cnt++"
 ]
}
```

## 2. Session B — `engine/testdata/promela/starvation.pml`, an `ltl` formula

### 2.1. `mc_parse`, then `mc_check` with `formula` and `fairness: none`

```json
{
 "name": "mc_check",
 "arguments": {
  "session_id": "s<UTC>-0002",
  "properties": [
   {
    "id": "delivery",
    "kind": "ltl",
    "formula": "<> done",
    "text": "B eventually sets done"
   }
  ],
  "fairness": "none",
  "no_timing": true
 }
}
```

Response — `violated`, with `temporal` and `counterexample.loop`:

```json
{
 "outcome": "report",
 "properties": [
  {
   "complete": false,
   "counterexample": {
    "id": "cex-1",
    "loop": {
     "start": 3,
     "steps": 4
    },
    "path": "<session-dir>/s<UTC>-0002/cex/cex-1.json",
    "steps": 6,
    "summary": "(!(done)) -> goto accept_S1, t = 1 - t; loop: (!(done)) -> goto accept_S1, t = 1 - t, (!(done)) -> goto accept_S1, t = 1 - t",
    "user_names": [
     "(!(done))",
     "t = 1 - t",
     "(!(done))",
     "t = 1 - t",
     "(!(done))",
     "t = 1 - t"
    ]
   },
   "counters": {
    "depth": 3,
    "memory_bytes_est": 24872,
    "states": 4,
    "transitions": 5
   },
   "evidence": "exhaustive",
   "id": "delivery",
   "kind": "ltl",
   "reason": "acceptance cycle: the loop of the counterexample passes through an accept state of the automaton for !(<> done) infinitely often, so the run satisfies the negated property (SPIN pan -a: acceptance cycle); in the loop only A:0, never:delivery move(s); B:1 is enabled throughout the loop and never moves (the loop is not weakly fair; rerun with fairness weak to exclude such runs)",
   "status": "violated",
   "temporal": {
    "atoms": [
     "done"
    ],
    "automaton_accepting": 1,
    "automaton_states": 2,
    "automaton_transitions": 2,
    "claim": "never:delivery",
    "fairness": "none",
    "formula": "<> done",
    "negated": "!(<> done)",
    "source": "formula",
    "stutter_invariant": true
   },
   "text": "B eventually sets done"
  }
 ],
 "report_path": "<session-dir>/s<UTC>-0002/check-1.json",
 "search": {
  "budget_applied": {
   "depth": 1000000,
   "memory_mb": 1024,
   "ms": 60000,
   "states": 1000000
  },
  "budget_notes": [],
  "budget_requested": {},
  "complete": false,
  "mode": "dfs",
  "stop": "all properties decided"
 },
 "session_id": "s<UTC>-0002",
 "warnings": []
}
```

### 2.2. `mc_explain` — `prefix`, `loop`, `loop_note`

```json
{
 "name": "mc_explain",
 "arguments": {
  "session_id": "s<UTC>-0002",
  "counterexample_id": "cex-1"
 }
}
```

Response:

```json
{
 "final_state": [
  {
   "value": 1,
   "var": "A:0.t"
  },
  {
   "value": 0,
   "var": "done"
  }
 ],
 "id": "cex-1",
 "loop": [
  {
   "changes": [],
   "command": "(!(done)) -> goto accept_S1",
   "index": 3,
   "location": "accept_S1",
   "origin": {
    "name": "(!(done))"
   },
   "process": "never:delivery",
   "user_name": "(!(done))"
  },
  {
   "changes": [
    {
     "after": 0,
     "before": 1,
     "var": "A:0.t"
    }
   ],
   "command": "t = 1 - t",
   "index": 4,
   "origin": {
    "file": "inline",
    "line": 11,
    "name": "t = 1 - t"
   },
   "process": "A:0",
   "user_name": "t = 1 - t"
  },
  {
   "changes": [],
   "command": "(!(done)) -> goto accept_S1",
   "index": 5,
   "location": "accept_S1",
   "origin": {
    "name": "(!(done))"
   },
   "process": "never:delivery",
   "user_name": "(!(done))"
  },
  {
   "changes": [
    {
     "after": 1,
     "before": 0,
     "var": "A:0.t"
    }
   ],
   "command": "t = 1 - t",
   "index": 6,
   "origin": {
    "file": "inline",
    "line": 11,
    "name": "t = 1 - t"
   },
   "process": "A:0",
   "user_name": "t = 1 - t"
  }
 ],
 "loop_note": "lasso: steps 1..2 are the prefix, steps 3..6 the loop, which repeats forever — after step 6 the state equals the state before step 3 (claim moves are steps of the claim process; a step of process \"-\" is a weak-fairness null step)",
 "path": "<session-dir>/s<UTC>-0002/cex/cex-1.json",
 "prefix": [
  {
   "changes": [],
   "command": "(!(done)) -> goto accept_S1",
   "index": 1,
   "location": "accept_S1",
   "origin": {
    "name": "(!(done))"
   },
   "process": "never:delivery",
   "user_name": "(!(done))"
  },
  {
   "changes": [
    {
     "after": 1,
     "before": 0,
     "var": "A:0.t"
    }
   ],
   "command": "t = 1 - t",
   "index": 2,
   "origin": {
    "file": "inline",
    "line": 11,
    "name": "t = 1 - t"
   },
   "process": "A:0",
   "user_name": "t = 1 - t"
  }
 ],
 "property_id": "delivery",
 "role": "counterexample",
 "session_id": "s<UTC>-0002",
 "summary": "(!(done)) -> goto accept_S1, t = 1 - t; loop: (!(done)) -> goto accept_S1, t = 1 - t, (!(done)) -> goto accept_S1, t = 1 - t",
 "user_names": [
  "(!(done))",
  "t = 1 - t",
  "(!(done))",
  "t = 1 - t",
  "(!(done))",
  "t = 1 - t"
 ]
}
```

### 2.3. The same formula under `fairness: weak` and `fairness: strong`

`weak` (response abbreviated to the property record):

```json
{
 "properties": [
  {
   "complete": true,
   "counters": {
    "depth": 8,
    "memory_bytes_est": 25238,
    "states": 11,
    "transitions": 24
   },
   "evidence": "exhaustive",
   "id": "delivery",
   "kind": "ltl",
   "reason": "no acceptance cycle in the complete product with the automaton for !(<> done): no run satisfies the negated property; under weak fairness",
   "status": "verified",
   "temporal": {
    "atoms": [
     "done"
    ],
    "automaton_accepting": 1,
    "automaton_states": 2,
    "automaton_transitions": 2,
    "claim": "never:delivery",
    "fairness": "weak",
    "formula": "<> done",
    "negated": "!(<> done)",
    "source": "formula",
    "stutter_invariant": true
   },
   "text": "<> done"
  }
 ],
 "outcome": "report"
}
```

`strong` — accepted as an argument and answered with `not-executed`, evidence `unknown`, the reason naming FR-008. This is a *result*, not a tool error: `isError` is false and `outcome` is `report`.

```json
{
 "properties": [
  {
   "complete": true,
   "counters": {
    "depth": 0,
    "memory_bytes_est": 0,
    "states": 0,
    "transitions": 0
   },
   "evidence": "unknown",
   "id": "delivery",
   "kind": "ltl",
   "reason": "strong fairness is not executed by this engine version (plan 14 §4.2): only weak fairness (every continuously enabled process eventually moves; pan -f) is implemented by the n+2 copies construction; rerun with fairness weak or none",
   "status": "not-executed",
   "temporal": {
    "fairness": "strong",
    "source": ""
   },
   "text": "<> done"
  }
 ],
 "outcome": "report"
}
```

## 3. `mc_manifest` for session B

```json
{
 "manifest": {
  "calls": [
   {
    "artifacts": [
     "ir-1.json"
    ],
    "duration_ms": 0,
    "n": 1,
    "outcome": "ok",
    "started": "2026-09-25T19:10:08Z",
    "tool": "mc_parse"
   },
   {
    "artifacts": [
     "check-1.json",
     "cex/cex-1.json"
    ],
    "duration_ms": 0,
    "n": 2,
    "outcome": "ok",
    "params": {
     "budget_applied": {
      "depth": 1000000,
      "memory_mb": 1024,
      "ms": 60000,
      "states": 1000000
     },
     "fairness": "none",
     "search": "dfs"
    },
    "started": "2026-09-25T19:10:08Z",
    "tool": "mc_check"
   },
   {
    "artifacts": [],
    "duration_ms": 0,
    "n": 3,
    "outcome": "ok",
    "started": "2026-09-25T19:10:08Z",
    "tool": "mc_explain"
   },
   {
    "artifacts": [
     "check-2.json"
    ],
    "duration_ms": 0,
    "n": 4,
    "outcome": "ok",
    "params": {
     "budget_applied": {
      "depth": 1000000,
      "memory_mb": 1024,
      "ms": 60000,
      "states": 1000000
     },
     "fairness": "weak",
     "search": "dfs"
    },
    "started": "2026-09-25T19:10:08Z",
    "tool": "mc_check"
   },
   {
    "artifacts": [
     "check-3.json"
    ],
    "duration_ms": 0,
    "n": 5,
    "outcome": "ok",
    "params": {
     "budget_applied": {
      "depth": 1000000,
      "memory_mb": 1024,
      "ms": 60000,
      "states": 1000000
     },
     "fairness": "strong",
     "search": "dfs"
    },
    "started": "2026-09-25T19:10:08Z",
    "tool": "mc_check"
   },
   {
    "artifacts": [],
    "duration_ms": 0,
    "n": 6,
    "outcome": "ok",
    "started": "2026-09-25T19:10:08Z",
    "tool": "mc_manifest"
   }
  ],
  "created": "2026-09-25T19:10:08Z",
  "engine": {
   "ir_schema": "mcd-ir/1",
   "mcp_schema": "mcd-mcp/1",
   "name": "mcd",
   "report_schema": "mcd-report/1",
   "version": "0.1.0-g0"
  },
  "inputs": [
   {
    "kind": "promela",
    "path": "ir-1.json",
    "sha256": "43092539c371572b0314278ddd7ba4bddd7c8af66a6f5362fb37ca62f4397c0b",
    "source": "inline"
   }
  ],
  "server": {
   "allow_read": [],
   "ceiling": {
    "ms": 60000,
    "states": 1000000
   },
   "concurrency": 2,
   "default_budget": {
    "depth": 1000000,
    "memory_mb": 1024,
    "ms": 60000,
    "states": 1000000
   }
  },
  "session_id": "s<UTC>-0002"
 },
 "path": "<session-dir>/s<UTC>-0002/manifest.json",
 "session_id": "s<UTC>-0002"
}
```

## 4. What this session establishes, and what it does not

Established, by the output above and nothing else:

- `mc_parse{promela: <text>}` returns `outcome: "ir"` — the frontend is linked.
- `mc_check` accepts an IR-expression `expr` for an `invariant` and a `formula` for
  an `ltl` property, and returns `temporal` and `counterexample.loop` for the latter.
- `mc_explain` splits a run into `prefix` and `loop` and explains an empty `loop`
  through `loop_note`.
- `fairness: strong` is a structured `not-executed` result, not a tool error.
- `mc_manifest` records the engine version, the input hashes and every call.

Not established here: that a *skill running in Claude Code* calls these tools. The
server was driven by a Python client, as in G2; the with-skill eval runs of
`evals-workspace/iteration-3` went through the CLI, because an MCP server cannot be
registered into a subagent's session. Exercising the installed plugin end to end
belongs to G6.

