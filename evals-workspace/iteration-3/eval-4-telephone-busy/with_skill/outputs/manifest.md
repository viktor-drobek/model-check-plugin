# Manifest (assembled by hand)

`mc_manifest` is an MCP tool and the plugin's MCP server is **not registered in this
session**, so this file was assembled by hand from the reports' `engine`, `inputs` and
`search.budget` fields plus the command lines, as `references/engine-tools.md` §4
("CLI stand-ins") prescribes. What a real `mc_manifest` would add and this file cannot:
per-call start/end timestamps and durations (every run was made with `--no-timing` so
that two runs of the same input are byte-for-byte equal), and a simulation seed — there
is no CLI equivalent of `mc_simulate`, so no random walk was made.

## Engine

| Field | Value |
|---|---|
| name | `mcd` |
| version | `0.1.0-g0` |
| ir_schema | `mcd-ir/1` |
| report_schema | `mcd-report/1` |
| binary | `/tmp/mcd` (pre-built; not rebuilt in this session) |
| layer | CLI (`mcd parse`, `mcd check`) — MCP server not registered |
| reductions | none (unreduced search) |
| session date (UTC) | 2026-09-25 |

## Inputs

| kind | path | sha256 |
|---|---|---|
| promela | `Promela - examples/CH14/version1` (corpus, unchanged, read-only) | `acdfcacad083c29ff47cba18de2fb84ec784e9e064e0b64c435497e07121ec3b` |
| promela | `mc-session-2026-09-25/version1-progress-switch.pml` (rewrite A) | `7968fd779dc364f6fb9c14500c2b8d3a7f323c7cbafe6bf6b117ff557c7ffdf8` |
| promela | `mc-session-2026-09-25/version1-progress-subscriber.pml` (rewrite B) | `ba471c2e882e4b3a37b2cfd4f43bf19f0ecf0d6c3838278082cd801f774be686` |
| promela | `mc-session-2026-09-25/version1-obs.pml` (rewrite C) | `cd37e760ee9a3dd3532b1bc115b823c89ed859c73152ed39f4ca4f9ebd032ae6` |

The corpus file was never copied into this directory; it is referenced by path and hash.

## Budget

Every `check` run used the CLI defaults, which the report echoes as
`states 1000000, depth 1000000, time_ms 60000, mem_bytes 1073741824`.
The single exception is call 7, a deliberate pilot with `--budget-states 5`.
`--unlimited` was not used.

## Calls

All commands were run from the repository root
`/home/user/Desktop/VirtualBuddyShared/Yandex.Disk.localized/drobek/model-check`.
`$S` below is `model-check-plugin/evals-workspace/iteration-3/eval-4-telephone-busy/with_skill/outputs/mc-session-2026-09-25`.

| # | tool | command | outcome | artefact |
|---|---|---|---|---|
| 1 | `mc_parse` | `/tmp/mcd parse --promela "Promela - examples/CH14/version1"` | ok (exit 0), 1 warning | `$S/ir-1-original.json`, `$S/parse-1-original.stderr` |
| 2 | `mc_check` | `/tmp/mcd check --no-timing --promela "Promela - examples/CH14/version1"` | ok (exit 0) | `$S/check-1-original-baseline.json` |
| 3 | `mc_parse` | `/tmp/mcd parse --promela "$S/version1-progress-switch.pml"` | ok (exit 0) | `$S/ir-2-progress-switch.json` |
| 4 | `mc_check` | `/tmp/mcd check --no-timing --promela "$S/version1-progress-switch.pml"` | ok (exit 0), fairness `none` | `$S/check-2-progress-switch-none.json` |
| 5 | `mc_check` | `… --fairness weak` (same model) | ok (exit 0), fairness `weak` | `$S/check-3-progress-switch-weak.json` |
| 6 | `mc_check` | `/tmp/mcd check --no-timing --promela "Promela - examples/CH14/version1" --progress` | ok (exit 0) — the deliberate "no progress label" demonstration | `$S/check-4-original-progress-nolabels.json` |
| 7 | `mc_parse` | `/tmp/mcd parse --promela "$S/version1-progress-subscriber.pml"` | ok (exit 0), 1 warning | `$S/ir-version1-progress-subscriber.json` |
| 8 | `mc_check` | `/tmp/mcd check --no-timing --promela "$S/version1-progress-subscriber.pml"` | ok (exit 0), fairness `none` | `$S/check-5-progress-subscriber-none.json` |
| 9 | `mc_check` | `… --fairness weak` (same model) | ok (exit 0), fairness `weak` | `$S/check-6-progress-subscriber-weak.json` |
| 10 | `mc_parse` | `/tmp/mcd parse --promela "$S/version1-obs.pml"` | ok (exit 0), no warnings | `$S/ir-version1-obs.json` |
| 11 | `mc_estimate` stand-in | `/tmp/mcd check --no-timing --promela "$S/version1-obs.pml" --budget-states 5` | ok (exit 0) — pilot; `deadlock` came back `inconclusive` / `bounded`, reason `state budget exhausted: 5 states stored` | `$S/check-7-obs-pilot.json` |
| 12 | `mc_check` | `/tmp/mcd check --no-timing --promela "$S/version1-obs.pml" --ltl '[] (sw != BUSY)' --ltl '[] ((sw == BUSY) -> <> (sw != BUSY))' --ltl '[] <> (sw == IDLE)' --ltl '[] (sub_busy == 0)' --ltl '[] ((sub_busy == 1) -> <> (sub_busy == 0))'` | ok (exit 0), fairness `none` | `$S/check-8-obs-ltl-none.json` |
| 13 | `mc_check` | the same five `--ltl` with `--fairness weak` | ok (exit 0), fairness `weak` | `$S/check-9-obs-ltl-weak.json` |
| 14 | `mc_check` | `/tmp/mcd check --no-timing --bfs --promela "$S/version1-obs.pml" --ltl '[] (sw != BUSY)'` | ok (exit 0) — BFS cross-check | `$S/check-10-obs-bfs.json` |
| 15 | `mc_check` | `/tmp/mcd check --no-timing --promela "Promela - examples/CH14/version1" --ltl '[] (switch@Busy -> <> !switch@Busy)'` | **rejected input**, exit 2, `kind: "ltl"`, `status: "not-executed"`, message `property ltl1: formula: unexpected character '@' (at offset 10)` — no report produced | `$S/check-11-label-atom-rejected.json` |

`mc_simulate` (step 5 of the skill's workflow) has no CLI equivalent; the sanity walk
was **not** made, and nothing in this report rests on one. `mc_lint_property` also has
no CLI equivalent; the property classification in section 4 of the report was done by
hand against `references/properties-ltl-ctl.md` and is labelled as manual there.
