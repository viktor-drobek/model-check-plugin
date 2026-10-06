# Manifest (assembled by hand — CLI fallback, no `mc_manifest`)

The MCP server was not registered in this session, so there is no server-generated
`manifest.json`. Versions, hashes and budgets below are copied from the engine's
report documents (`engine`, `inputs`, `search.budget`); the command lines and
wall-clock times were recorded by hand. Seeds: none (no `mc_simulate` on the CLI).

## Engine

- name: `mcd`, version `0.1.0-g0`, ir_schema `mcd-ir/1`, report_schema `mcd-report/1`
- binary: `${TMP_DIR}/mcd` (pre-built; `mcd version` → `mcd 0.1.0-g0 (ir mcd-ir/1, report mcd-report/1)`)
- layer: CLI (`mcd parse` / `mcd check`); MCP tools not available in this session

## Inputs

| kind | source | sha256 |
|---|---|---|
| promela (original, user's file, read-only) | `Promela - examples/CH17/simple1.pr` (copy: `simple1.pr.input`) | `d7a34d649c9cc83ecb852dd2ca584c1ce3b977fceb5a2dbb5d09db542b2d4b70` |
| promela (rewrite inside the subset, written by the skill) | `simple1-rewrite.pml` | `4831423a929538ceb331eaa56f8dd15c960ab71b943d5184b880caf3d66f1509` |
| ir (parser output of the rewrite + two `reach` sanity properties added by the skill) | `ir-rewrite-with-reach.json` | derived from `ir-rewrite.json` with `jq` |

## Calls (all on 2026-09-25, 09:46:58 UTC; each run < 1 s wall clock)

| n | command | exit | outcome | artefact |
|---|---|---|---|---|
| 1 | `${TMP_DIR}/mcd parse --promela "Promela - examples/CH17/simple1.pr"` | 2 | rejected: `outside-subset`, path `…/CH17/simple1.pr:1:1`, message `construct outside subset: c_code (embedded C is outside the subset) (simple1.pr, line 1)` | `parse-original.json` |
| 2 | `${TMP_DIR}/mcd check --promela "Promela - examples/CH17/simple1.pr" --no-timing` | 2 | same rejection (no report produced) | `check-original.json` |
| 3 | `${TMP_DIR}/mcd parse --promela simple1-rewrite.pml` | 0 | IR, no warnings | `ir-rewrite.json` |
| 4 | `${TMP_DIR}/mcd check --promela simple1-rewrite.pml --budget-states 1000 --budget-depth 1000 --budget-ms 10000 --no-timing` (pilot) | 0 | report; stop `complete`; 8 states, 7 transitions, depth 4 | `check-rewrite-pilot.json` |
| 5 | `${TMP_DIR}/mcd check --promela simple1-rewrite.pml --sweep` (target budget: CLI defaults 10^6 states / 10^6 depth / 60 000 ms / 1024 MiB; DFS) | 0 | report; stop `complete`; 8 states, 7 transitions, depth 4, time_ms 0, memory_bytes_est 25 024 | `check-rewrite.json` |
| 6 | `${TMP_DIR}/mcd check --promela simple1-rewrite.pml --bfs --sweep --no-timing` | 0 | report; stop `complete`; same counters (memory_bytes_est 24 960) | `check-rewrite-bfs.json` |
| 7 | `jq '.properties += [reach_x4, reach_x6]' ir-rewrite.json > ir-rewrite-with-reach.json` | 0 | IR with two sanity `reach` properties | `ir-rewrite-with-reach.json` |
| 8 | `${TMP_DIR}/mcd check --ir ir-rewrite-with-reach.json --sweep --no-timing` | 0 | report; stop `complete`; 4 properties, all `verified`/`exhaustive`; witnesses `x = 2, x = x + 2` (x=4) and `x = 2, x = x * 3` (x=6) | `check-rewrite-reach.json` |

Not run (no CLI equivalent): `mc_simulate`, `mc_lint_property` (classification done
by hand from `properties-ltl-ctl.md`), `mc_estimate` (the pilot run of call 4 stands in).

Determinism check: calls 5 and 6 (DFS vs BFS) and calls 4/8 agree on states,
transitions and depth (8 / 7 / 4).
