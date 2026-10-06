# Manifest (assembled by hand — `mc_manifest` has no CLI equivalent)

The plugin's MCP server is not registered in this session, so there is no session
directory and no `manifest.json`. Per `references/engine-tools.md` §4 ("CLI
stand-ins") the engine version, the input hashes and the applied budgets are taken
from the report's `engine`, `inputs` and `search.budget`; the command lines and the
wall-clock time are recorded here by hand. There is no seed: `mc_simulate` has no
CLI equivalent, so no simulation was run.

- Engine: `mcd 0.1.0-g0 (ir mcd-ir/1, report mcd-report/1)`, binary `${TMP_DIR}/mcd`
  (pre-built for this session; not rebuilt)
- Host: linux/aarch64
- Date (UTC): 2026-09-25
- External model checkers used: none (SPIN was not installed, not run, not consulted)

## Inputs

| Kind | Path | SHA-256 |
|---|---|---|
| corpus source (read only, never copied) | `Promela - examples/CH17/simple1.pr` | `d7a34d649c9cc83ecb852dd2ca584c1ce3b977fceb5a2dbb5d09db542b2d4b70` |
| rewrite inside the subset | `model/simple1-rewrite.pml` | `8dbf0034f98f0678a49b528e9ddd8bb450a3ce50043f9cd7c8e5ebae145f33ab` |
| negative control (mutant) | `model/simple1-mutant-assert5.pml` | `9e955f0f4e6363abe5bab63e279b99ec9b1dcc5aa041a85cac5aaeaf01cbbc61` |

## Calls, in order

| # | Command (cwd = repo root, except runs 3–7 whose cwd is this `outputs/` directory) | Exit | Artefact |
|---|---|---|---|
| 0 | `${TMP_DIR}/mcd version` | 0 | `session/00-version.txt` |
| 1 | `${TMP_DIR}/mcd parse --promela "Promela - examples/CH17/simple1.pr"` | **2** | `session/01-parse-original.stdout.json` |
| 2 | `${TMP_DIR}/mcd check --promela "Promela - examples/CH17/simple1.pr"` | **2** | `session/02-check-original.stdout.json` |
| 3 | `${TMP_DIR}/mcd parse --promela model/simple1-rewrite.pml` | 0 | `session/03-parse-rewrite.ir.json` |
| 4 | `${TMP_DIR}/mcd check --promela model/simple1-rewrite.pml --budget-states 3` | 0 | `session/04-check-pilot-3states.json` |
| 5 | `${TMP_DIR}/mcd check --promela model/simple1-rewrite.pml --sweep` | 0 | `session/05-check-main.json` |
| 6 | `${TMP_DIR}/mcd check --promela model/simple1-rewrite.pml --ltl '[] !(x == 4)' --ltl '[] !(x == 6)' --sweep` | 0 | `session/06-check-reachability-ltl.json` |
| 7 | `${TMP_DIR}/mcd check --promela model/simple1-mutant-assert5.pml --bfs` | 0 | `session/07-check-mutant.json` |

Budgets applied in runs 5–7: the CLI defaults — `states 1000000`, `depth 1000000`,
`time_ms 60000`, `mem_bytes 1073741824`. Run 4 lowered `states` to 3 on purpose
(`mc_estimate` stand-in). `--unlimited` was never used. Every run took `time_ms: 0`
(below the engine's millisecond resolution); total wall-clock for all seven calls was
a few seconds.

Determinism: all runs were made with timing on, so two identical invocations differ
only in `time_ms` (NFR-006). `session/*.stderr.txt` are all empty — the frontend
emitted no warnings for the rewrite.
