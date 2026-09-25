# Fixtures: references, not copies (plus one encoding of our own)

The Promela models used by `evals/evals.json` belong to the corpus
`Promela - examples/` (Holzmann, *The SPIN Model Checker*, 2003). They are **not copied**
into the skill (plan 14 §2.1, licence note): a fixture is a repository
path plus the SHA-256 hash of the file's content. An eval runner resolves the path
relative to the repository root and refuses to run if the hash differs — the
assertions were written against this exact content. A Cucumber scenario
(`features/g3-align.feature`) recomputes every hash in the table below on each
build.

The one file that lives here, `petrinet1.json`, is **not** a corpus copy: it is the
skill's own JSON encoding of the net that `Promela - examples/App_C/petrinet1`
encodes in Promela, written to `assets/petri-net.schema.json` (itself a byte-for-byte
copy of the engine's `engine/frontend/petri/schema.json`). It is the expected shape
of the JSON an E3 run should produce, and the grader compares the produced net with
it structurally (places, initial marking, transitions, arcs). A second scenario
checks that no file under this directory has the hash of any corpus file.

Hashes computed with `sha256sum` on 2026-09-24.

| Eval | Fixture path (repository root) | SHA-256 | Bytes |
|---|---|---|---|
| E1 | `Promela - examples/CH2/mutex_flaw.pml` | `b9cb0230f9a306494af4a12592df3411988254b35526b5fbcdbf17ab328fdac4` | 391 |
| E2 | `Promela - examples/CH3/alternatingbit.pml` | `09006d65d6619931809544f074da440a9f3758e7715d7cc7493657164267698d` | 321 |
| E3 | `model-check-plugin/skills/model-check/evals/fixtures/petrinet1.json` (own encoding; the prompt gives the net in words) | `de136a9393a1956815aef9719e43408af2b2a9559bd8f6509c8d7a32e74f3ac9` | 1123 |
| E3 (reference) | `Promela - examples/App_C/petrinet1` (the corpus encoding the net comes from; not an input of the eval) | `b20970178bb7adfec73a131d56c7f9e6fc00e7ec07c995f9a1c7285f60d81d87` | 526 |
| E4, E6 | `Promela - examples/CH14/version1` | `acdfcacad083c29ff47cba18de2fb84ec784e9e064e0b64c435497e07121ec3b` | 752 |
| E5 | `Promela - examples/CH17/simple1.pr` | `d7a34d649c9cc83ecb852dd2ca584c1ce3b977fceb5a2dbb5d09db542b2d4b70` | 184 |

Which evals can run today: E3 (Petri JSON through `mcd check --petri`, build step G0),
E1 and E5 (Promela through `--promela`, build step G1). E2 and E4 need `ltl` /
`progress` and weak fairness (G4); E6 needs `ctl` (G5). The `runnable_from` field of
each eval records this.

Golden results the assertions rest on: `petrinet1` → `deadlock` `violated` after
`t1, t4` with final marking `p2=1 p5=1`, 6 states (`steps/g0-confirmation.md`;
`mcd` agrees with `pan` up to the +2 init states); `mutex_flaw` → invariant
violated through `L1`–`L4` (429 states, matches `pan`); `simple1.pr` → parser
rejection (`c_code`), status `not-executed`. `App_C/petrinet2`
(`19aff7aa4b92b7db14999b9339aeb95fe17c251f36e71a2c3be1b5ea2df2a1b5`) has its golden
report in `engine/testdata/golden/petrinet2.report.json` since G0.

## E1 — exact command line and what it returns (mcd 0.1.0-g0, 2026-09-25)

Run from the repository root; `mcd` is the binary built with
`go build -o /tmp/mcd ./cmd/mcd` in `model-check-plugin/engine`:

```
mcd check --no-timing --promela "Promela - examples/CH2/mutex_flaw.pml"
```

Exit code 0. Report: `inputs[0]` = `{kind: promela, sha256: b9cb0230…8fdac4}`;
`model` = `{name: mutex_flaw, state_bytes: 9, processes: 2, variables: 6}`;
`search.stop` = `complete`, `search.complete` = true; counters 429 states, 858
transitions, depth 65 (the same for both properties; `pan -c0 -DNOREDUCE` stores
429 too). Properties: `deadlock` → `verified` / `exhaustive`; `assert` → `violated`
/ `exhaustive`, reason "assert(cnt == 1) fails in the last step of the
counterexample", counterexample of 54 steps whose last step is `user:0` executing
`assert(cnt == 1)` at `origin` line 23 of the file (label `L7`, the critical
section), with `final_state` `cnt = 2`, `x = y = z = 1`, `user:0.me = 1`,
`user:1.me = 2` — both users inside the section at once. The with-skill answer of
E1 must decode this run through the labels `L1`–`L4` and `L7` of the source.

## E5 — expected rejection (mcd 0.1.0-g0, 2026-09-25)

```
mcd check --no-timing --promela "Promela - examples/CH17/simple1.pr"
```

The result is exit code 2 and, on stdout,

```json
{"error": {"kind": "outside-subset", "status": "not-executed",
           "path": "Promela - examples/CH17/simple1.pr:1:1",
           "message": "construct outside subset: c_code (embedded C is outside the subset) (simple1.pr, line 1)"}}
```

`mcd parse --promela` on the same file gives the same document. The construct is
`c_code`, the file `CH17/simple1.pr`, the line 1 (the global `c_code { int x; }`;
the `c_expr` inside the `assert` on lines 6–7 is never reached because the parser
stops at the first construct outside the subset). No property record exists, so the
with-skill answer assigns `not-executed` itself, names the construct and the line,
says no host code is executed, and offers a rewrite of the C effect as Promela.

How to add a corpus fixture: add a row with path, hash and size; never add the file
itself. Models from the second corpus (lectures 07, Karpov 03, Velder 09 — extracted
from `books-md/`) follow the same rule with their `books-md/` source path and the
line range of the listing. A file may be added here only when it is our own
encoding, as `petrinet1.json` is, and then its row says so.
