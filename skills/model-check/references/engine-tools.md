# Engine tools: the `mcd` CLI today, the seven MCP tools with G2

Sources: `model-check-skill-notes/14-skill-building-plan.md` §3 (three layers),
§6 (tool table, status vocabulary, aggregation priority, `bounded`/`unknown` rule,
CLI), §9 (which build step delivers which capability), §11 (LTL evidence `unknown`
while experimental), §12 A5, A6; `model-check-skill-notes/11-skill-requirements.md`
§10 (staged execution, manifest), §13 (safety of artefacts), NFR-004, NFR-006,
NFR-007. Engine as built: `model-check-plugin/engine/cli/cli.go` (commands, flags,
exit codes), `model-check-plugin/engine/report/report.go` (report schema
`mcd-report/1` and the rules enforced by `report.Build`),
`model-check-plugin/steps/g0-confirmation.md` §1, §4 (decisions 2–6).

The engine is one Go binary, `mcd`, built in step G0. The MCP layer with the seven
tools arrives with G2; until then the CLI is the only path to the engine, and every
tool name in `SKILL.md` and `workflow.md` means its CLI equivalent from §2 of this
file. Both layers return the same JSON (`mcd-report/1`) by plan §6, so what §3 says about
reading a report is expected to hold when MCP lands; if G2 changes a field, this file
is corrected, not the rule. The G0 CLI never reads a path you did not pass and never
writes a file: output goes to stdout, and you redirect it into a directory you create
for the session (NFR-004).

## 1. The seven MCP tools (arrive with G2)

| Tool | Input | Output | Call it when | Until G2 |
|---|---|---|---|---|
| `mc_parse` | Promela text **or** Petri JSON **or** IR JSON | IR, mapping table (IR element → file/line/user name), warnings; or an error with position and construct | always first; on every model change | `mcd parse --petri` / `--ir`; `--promela` arrives with G1 |
| `mc_simulate` | IR, `seed`, `steps`, `mode` (`random` / `guided`) | a trace of steps | sanity check after parsing; never as evidence | no equivalent; skip the pilot walk, keep `mc_estimate`'s role for a small `--budget-states` run |
| `mc_check` | IR, properties (`invariant` / `deadlock` / `reach` / `ltl` / `ctl` / `progress`), `fairness` (`none` / `weak`), budget, search mode | per property: the record of §3.2 | the check itself; rerun with a larger budget after `inconclusive` | `mcd check` with the budget flags of §2 |
| `mc_explain` | counterexample id, mapping | prefix and loop, per-step variable diff, user names | every `violated`; before classifying the cause | read `counterexample.steps` in the report yourself (§3.3); loops arrive with G4 |
| `mc_lint_property` | formula, IR | atoms, definedness, safety/liveness class, vacuity candidates, `X`-free flag | every property before a check | no equivalent; do the classification by `properties-ltl-ctl.md` and say in the report that it was manual |
| `mc_estimate` | IR, time limit | growth of the state count over a partial run | before choosing budget and mode | no equivalent; a `mcd check` with a small `--budget-states` is a crude stand-in — its `counters.states` at the budget says how far the budget reached, not how the count grows |
| `mc_manifest` | session id | engine and schema versions, input hashes, parameters, times | for every report (FR-012, NFR-002) | the report's `engine`, `inputs` (with `sha256`) and `search.budget` sections cover the manifest's versions, hashes and budgets; its seed and start/end times have no counterpart in the report yet (no simulation, no timestamps) — record the command line and the wall-clock time yourself and say the manifest was assembled by hand |

## 2. The CLI as built (G0)

```
mcd parse   --petri net.json | --ir model.json                    → IR JSON on stdout
mcd check  (--petri net.json | --ir model.json)
           [--budget-states N] [--budget-depth N] [--budget-ms N]
           [--budget-mem-mb N] [--bfs] [--no-timing]              → report JSON on stdout
mcd version                                                       → "mcd <version> (ir <schema>, report <schema>)"
```

| Flag | Meaning | Default |
|---|---|---|
| `--petri FILE` | input is a Petri net JSON (`assets/petri-net.schema.json`, a byte-for-byte copy of the engine's `frontend/petri/schema.json`) | exactly one of `--petri` / `--ir` is required |
| `--ir FILE` | input is IR JSON (`mcd-ir/1`) | — |
| `--promela FILE` | input is Promela text — **arrives with G1**; today `mcd` exits 1 on it as an unknown flag | — |
| `--budget-states N` | stop after N stored states (0 = unlimited) | 1 000 000 |
| `--budget-depth N` | do not expand states deeper than N transitions; they are stored and counted (0 = unlimited) | 1 000 000 |
| `--budget-ms N` | wall-clock limit in milliseconds (0 = unlimited) | 60 000 |
| `--budget-mem-mb N` | limit on the engine's memory *estimate* in MiB (0 = unlimited) | 1024 |
| `--bfs` | breadth-first search: shortest counterexample for safety and deadlock | DFS |
| `--no-timing` | omit `time_ms` so two runs of the same input are byte-for-byte equal | timing included |

The defaults are the "medium model" bounds of plan §12 A4 (`evidence-and-status.md` §5).
Flag names are part of the skill's contract and stay stable across build steps.

**Exit codes** (`steps/g0-confirmation.md` §4, decision 2 — the division is "is there
a result document?"):

| Exit code | Meaning | What is on stdout |
|---|---|---|
| exit code 0 | a result was produced — a report or an IR — whatever the verdicts, **including `invalid-model`** | the JSON document |
| exit code 1 | no result: tool error — unreadable file, unknown flag or command, internal failure | nothing; the message is on stderr |
| exit code 2 | no result: the input was rejected by a frontend — schema violation, unsupported construct (inhibitor arc), invalid IR | `{"error": {"kind", "path", "message"}}`; `kind` is `schema`, `unsupported-input` or `ir`; `path` points into the input (e.g. `transitions[0].inputs[0].weight`) |

Exit code 2 is the "parser rejected a construct" case of `SKILL.md` step 3 (eval E5
expects the G1 Promela frontend to use the same code for `c_code`; G1 has to confirm
that); it is **not** a property result — the properties get `not-executed` in *your*
report with the rejection's `message` as the reason. Exit code 1 means fix the
command, not the model.

Rules that do not change with the layer:

- Pass files by path; do not interpolate model text into a shell line (11 §13).
- Show the exact command line in the report's Execution section.
- Parse the JSON on stdout; do not scrape prose from stderr.
- If the binary is missing (plan A6), every property is `not-executed` with reason
  "engine binary unavailable" and the report says how to obtain it (`go build
  ./cmd/mcd` in `model-check-plugin/engine`). Do not simulate a result.

## 3. The report (`mcd-report/1`)

### 3.1. Top level

| Field | Content |
|---|---|
| `engine` | `name` (`mcd`), `version`, `ir_schema`, `report_schema` |
| `inputs` | one entry per input file: `kind` (`petri` / `ir`), `path`, `sha256` of the file's bytes |
| `model` | `name`, `state_bytes` (state-vector length), `processes`, `variables` |
| `search` | `mode` (`dfs` / `bfs`), `budget` {`states`, `depth`, `time_ms`, `mem_bytes`} as given, `stop` (why the search ended: `complete`, `all properties decided`, `invalid model`, or a budget sentence), `complete` (true only when the whole reachable graph was expanded) |
| `properties` | one record per property, in the model's property order (§3.2) |

### 3.2. One property record

| Field | Content |
|---|---|
| `id`, `kind`, `text` | property identity; the Petri frontend generates `deadlock` (kind `deadlock`) and `safe` (kind `invariant`), the IR may add `reach` and `assert` |
| `status` | one of the six words (`evidence-and-status.md` §1) |
| `evidence` | `exhaustive`, `bounded`, `approximate`, `unknown` |
| `counters` | `states`, `transitions`, `depth` (greatest depth expanded), `time_ms` (absent under `--no-timing`), `memory_bytes_est`; the same for every property of one run |
| `complete` | copy of `search.complete` |
| `counterexample` | present for `violated` (the violating run) and for `invalid-model` (the run to the offending step); §3.3 |
| `witness` | present for a `reach` that came back `verified`: the run that reaches the condition |
| `reason` | for `inconclusive` the exhausted resource; for `not-executed` / `invalid-model` what is missing or overflowed; for `violated` the engine may add a one-line diagnosis (it does for `deadlock`: which processes are blocked; for `reach`: that no state satisfies the condition) |

### 3.3. A run (`counterexample` / `witness`)

`steps[]`: `index` (1-based), `process`, `command` (the transition or statement
text — for a Petri net the transition name), `changes[]` {`var`, `before`, `after`},
`location` (control location after the step, when named), `origin` {`file`,
`line`, `name`} — the mapping back to the user's source. `final_state[]` {`var`,
`value`} lists **every** variable of the last state, so a Petri marking can be read
off without replaying. `summary` joins the step commands with ", " — for
`petrinet1` it is exactly `t1, t4`. Loops (prefix + cycle) arrive with G4; a G0 run
is a finite path.

### 3.4. Rules the engine enforces on its own output (K1; `report.Build` refuses a document that breaks them)

- `violated` carries evidence `exhaustive`: its counterexample is an exact run of
  the model, whatever stopped the search afterwards. For `reach`, `violated` means
  "no state satisfies the condition" and requires a complete search.
- `verified` requires `complete` = true and evidence `exhaustive`, except for `reach`.
- `reach` is verified by a witness: the exact run attached as `witness`; the search
  may be incomplete (`complete` false) and the verdict still stands.
- Budget exhaustion gives `inconclusive`, evidence `bounded`, `complete` false, with
  `reason` naming the exhausted resource: `state budget exhausted: N states stored`,
  `depth budget exhausted: N state(s) at depth > D were stored but not expanded`,
  `time budget exhausted`, `memory budget exhausted: estimate B bytes exceeds L`.
- A construct outside the subset gives `not-executed`; so does a capability the
  current build lacks — two different boundaries with the same status. A property
  kind the engine version does not run (today `ltl`, `ctl`, `progress`; all inside the
  plan's subset, not yet built) gets `not-executed` from the engine with evidence
  `unknown` and the kind named; an unsupported *input* construct (outside the subset,
  e.g. an inhibitor arc, or `c_code` once G1 parses Promela) is an exit code 2
  rejection (§2), the engine produces no record, and you assign `not-executed`
  yourself (`evidence-and-status.md` §1).
- A domain overflow gives `invalid-model`: a `byte` wrap or a place pushed above its
  capacity, evidence `unknown`, with the run to the offending step attached as
  `counterexample` and the overflow named in `reason`; **every** property still
  undecided at that point gets `invalid-model`, while a property already decided
  (for example a `violated` found earlier on the same run) keeps its verdict
  (`steps/g0-confirmation.md` §4, decision 3).
- `unknown` is in the vocabulary but the G0 engine never emits it.
- Aggregation priority: `invalid-model` > `not-executed` > `violated` > `inconclusive` > `unknown` > `verified` (plan §6). Statuses are per property and are not aggregated by the engine; if the user insists on one word for the whole run, take the first of this list that occurs among the properties, and still list the per-property records.

Determinism (NFR-006): the same input and flags produce the same report except
`time_ms`; `--no-timing` makes it byte-for-byte equal (the `petrinet2` golden test
in `features/g0-engine.feature` relies on that). Anything else that differs
between two runs is an engine defect to report, not a nuance to smooth over.

## 4. Reading a response, in this order

1. **Exit code.** 1 → fix the command. 2 → a rejection: quote `kind`, `path`,
   `message`, assign `not-executed` to every property, propose a rewrite inside
   the subset (`promela-subset.md`, `petri-nets.md` §7). 0 → parse the report.
2. **`search.stop` and `search.complete`.** They tell you before any verdict
   whether the graph was exhausted or a budget cut the run.
3. **Per property: `status`, then `evidence`, then `complete`.** Take all three as
   they are. Do not upgrade an `inconclusive` because "it probably would have
   finished"; do not downgrade a `violated` because the run was cut afterwards.
4. **`counterexample` / `witness`** — decode through `origin.name` and
   `final_state`; classify the cause (`counterexamples.md`).
5. **`reason`** — the exhausted resource, the missing capability, or the overflow;
   quote it verbatim in the report.
6. **`counters`** — into the Execution section; they are not evidence by themselves.

## 5. Capability by build step

A capability the current build lacks yields `not-executed` with the capability
named (or an exit code 2 rejection); never fill the gap with your own reasoning.

| Capability | Available from |
|---|---|
| Petri JSON (`--petri`), IR JSON (`--ir`); properties `deadlock`, `invariant`, `reach`, `assert`; DFS/BFS with lazy successors; budgets for states, depth, time, memory; report `mcd-report/1` with `complete`; finite-path counterexamples with `origin`; exit codes 0/1/2 | **G0 (built)** |
| Promela subset (chapters 2–3) via `--promela`; counterexamples mapped to Promela lines | G1 |
| MCP server with the seven tools, session directory, manifest as a tool | G2 |
| `ltl` (never claims and formulas), `progress`, weak fairness, loop counterexamples (prefix + cycle) | G4 (LTL evidence `unknown`/experimental until the differential oracle passes, plan §11) |
| `ctl`, vacuity in `mc_lint_property`, `mc_estimate`, Promela v1 (`inline`, `typedef`, channels in messages) | G5 |
| bitstate (`approximate`), POR, parallel BFS | G7 (vNext, if chosen) |

## 6. Session directory and artefacts (11 §13)

With G2 the server keeps a per-session directory for IR, traces and the manifest.
Until then, make one yourself (`mc-session-<date>/`), write the input JSON, the
report and the `mcd parse` output there, and list its relative paths in the
Artifacts section. Never overwrite the user's sources; generated files stay
separate from user files. Models, logs and traces may contain business data — ask
before storing them anywhere else (workflow question 14).
