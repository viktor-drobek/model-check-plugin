# Manifest (assembled by hand — the MCP server was not registered in this session)

`mc_manifest` has no CLI counterpart (`references/engine-tools.md` §4, "CLI stand-ins").
Versions, input hashes and budgets below are copied from the reports' `engine`,
`inputs` and `search` sections; command lines, wall-clock time and the machine are
recorded by hand. There is no seed, because `mc_simulate` has no CLI equivalent and
no simulation was run.

## Engine

| Field | Value |
|---|---|
| engine | `mcd` 0.1.0 (`mcd version` → `mcd 0.1.0 (ir mcd-ir/1, report mcd-report/1)`) |
| binary | `model-check-plugin/engine/bin/mcd` (pre-built; not rebuilt in this session) |
| ir schema | `mcd-ir/1` |
| report schema | `mcd-report/1` |
| layer | CLI (the plugin's MCP server `mcd serve` was not registered in this session) |
| host | Linux aarch64 |
| date | 2026-09-27, 13:39–13:42 (+02:00); every run finished in ≤ 1 ms of engine time |

## Inputs

| Kind | Path | sha256 |
|---|---|---|
| promela (corpus, unchanged) | `Promela - examples/CH14/version1` | `acdfcacad083c29ff47cba18de2fb84ec784e9e064e0b64c435497e07121ec3b` |
| promela (derived variant, this session) | `…/with_skill/outputs/models/version1-progress.pml` | `9ec15da3ba0289fb04e28b8a0d942c130f5b0ce47129b4a3921e942f0c8e2094` |

Paths are relative to the repository root
`/home/user/Desktop/VirtualBuddyShared/Yandex.Disk.localized/drobek/model-check`, which
is also the working directory of every command below.

## Calls

Budget in every `check` run: the defaults — 1 000 000 states, depth 1 000 000,
60 000 ms, 1 024 MiB; search `dfs`; no budget was lifted (`--unlimited` not used).

| # | Command | Artefact | Outcome |
|---|---|---|---|
| 1 | `mcd parse --promela "Promela - examples/CH14/version1"` | `mc-session-2026-09-27/ir-1.json`, `parse-1.stderr` | exit 0; 1 warning (printf) |
| 2 | `mcd check --promela "Promela - examples/CH14/version1" --estimate` | `estimate-1.json` | exit 0; 9 states, complete, class `small` |
| 3 | `mcd check --promela "Promela - examples/CH14/version1" --ctl 'EF (subscriber@Busy)' --ctl 'EF (switch@Busy)' --ctl 'AG EF (subscriber@Idle)' --ctl 'AG (subscriber@Busy -> AF subscriber@Idle)' --ctl 'AG EF (switch@Idle)' --ctl 'AG (switch@Busy -> AF switch@Idle)'` | `check-1.json`, `check-1.stderr` | exit 0; report, 7 property records |
| 4 | `mcd check --promela "Promela - examples/CH14/version1" --progress` | `check-2-noLabels-progress.json` | exit 0; `progress` `violated` — the model carries no progress label (see report §8) |
| 5 | `mcd parse --promela outputs/models/version1-progress.pml` | `ir-2-progress.json`, `parse-2.stderr` | exit 0; 1 warning (printf) |
| 6 | `mcd check --promela outputs/models/version1-progress.pml` | `check-3-progress-none.json` | exit 0; `deadlock` + `progress` (fairness `none`) |
| 7 | `mcd check --promela outputs/models/version1-progress.pml --fairness weak` | `check-4-progress-weak.json` | exit 0; the same, fairness `weak` |
| 8 | `mcd check --promela outputs/models/version1-progress.pml --ctl 'EF (subscriber@Busy)' --ctl 'EF (switch@Busy)' --ctl 'AG EF (subscriber@progress_Idle)' --ctl 'AG (subscriber@Busy -> AF subscriber@progress_Idle)' --ctl 'AG EF (switch@Idle)' --ctl 'AG (switch@Busy -> AF switch@Idle)'` | `check-5-progress-ctl.json` | exit 0; all properties re-run after the model change |
| 9 | `mcd check --promela "Promela - examples/CH14/version1" --ltl '[] (subscriber@Busy -> <> subscriber@Idle)'` | `check-6-ltl-rejected.json`, `check-6.stderr` | **exit 2**, input rejected, `kind: "ltl"` — recorded deliberately as the documented boundary (report §9) |

## Not run, and why

| Tool | Why |
|---|---|
| `mc_simulate` (random walk, seed) | no CLI equivalent; the sanity walk of SKILL.md step 5 was not performed. Its place is taken by the two `reach`-style properties P1/P2 and by the `witness` traces the engine attached to them |
| `mc_lint_property` | no CLI equivalent; the properties were classified by hand against `references/properties-ltl-ctl.md` §1, §3 and the vacuity check was done with P1/P2 |
| `mc_estimate` | replaced by the CLI's own `--estimate` (call 2), which is the same growth estimate |
| `mc_explain` | needed only for `violated` properties. The single `violated` record (call 4) was decoded by hand from the report's `counterexample.steps` and `counterexample.loop` |
