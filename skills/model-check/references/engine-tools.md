# Engine tools: the `mcd` CLI and the seven MCP tools (as built in G0–G2)

Sources: `model-check-skill-notes/14-skill-building-plan.md` §3 (three layers),
§6 (tool table, status vocabulary, aggregation priority, `bounded`/`unknown` rule,
CLI), §9 (which build step delivers which capability), §11 (LTL evidence `unknown`
while experimental), §12 A5, A6; `model-check-skill-notes/11-skill-requirements.md`
§10 (staged execution, manifest), §13 (safety of artefacts), NFR-004, NFR-006,
NFR-007. Engine as built: `model-check-plugin/engine/cli/cli.go` (commands, flags,
exit codes), `engine/report/report.go` (report schema `mcd-report/1`, rules of
`report.Build`), `engine/mcp/*.go` (the seven tools, field names from the Go struct
tags), `engine/cmd/mcd/serve.go` (`mcd serve`), `steps/g0-confirmation.md` §4,
`steps/g1-confirmation.md` §1, `steps/g2-confirmation.md` §1, §4.

Contents: 1 the CLI · 2 exit codes and rejections · 3 the MCP server (`mcd serve`) ·
4 the seven tools · 5 the report (`mcd-report/1`) · 6 rules the engine enforces ·
7 reading a response · 8 capability by build step · 9 session directory.

The engine is one Go binary, `mcd`. Two layers reach it: the CLI (G0, Promela with
G1) and the MCP server `mcd serve` (G2). Both return the same statuses, evidence
levels and per-property records; the MCP layer wraps the report of §5 in the
response shapes of §4 and keeps files in a session directory (§9). Which layer you
use: the MCP tools when the plugin's server is registered in the session; otherwise
the CLI. **In this build `mcd serve` does not link the Promela frontend**: `mc_parse`
with `promela` answers `outcome: not-executed` ("promela frontend not available in
this build … not linked into this server"), so Promela models go through
`mcd parse --promela` / `mcd check --promela` until the server is wired (G4/G6);
until then the CLI is the only path for Promela.

## 1. The CLI

```
mcd parse   (--petri net.json | --ir model.json | --promela model.pml [-D NAME[=val]]…)
                                                                  → IR JSON on stdout
mcd check   (--petri net.json | --ir model.json | --promela model.pml [-D …])
            [--budget-states N] [--budget-depth N] [--budget-ms N] [--budget-mem-mb N]
            [--bfs] [--sweep] [--no-timing]                       → report JSON on stdout
mcd version                                      → "mcd <version> (ir <schema>, report <schema>)"
mcd serve   [flags of §3]                        → MCP server on stdio
```

| Flag | Meaning | Default |
|---|---|---|
| `--petri FILE` | Petri net JSON (`assets/petri-net.schema.json`, byte-for-byte the engine's `frontend/petri/schema.json`) | exactly one of `--petri` / `--ir` / `--promela` |
| `--ir FILE` | IR JSON (`mcd-ir/1`), validated and re-emitted canonically | — |
| `--promela FILE` | Promela text, the subset of `promela-subset.md` §1 (the flag `--promela` arrives with G1 — built) | — |
| `-D NAME` / `-D NAME=value` | preprocessor symbol, repeatable (`CH4/prop.pml` needs `-D PHI`) | none |
| `--budget-states N` | stop after N stored states (0 = unlimited) | 1 000 000 |
| `--budget-depth N` | states deeper than N transitions are stored and counted, not expanded (0 = unlimited) | 1 000 000 |
| `--budget-ms N` | wall-clock limit in milliseconds (0 = unlimited) | 60 000 |
| `--budget-mem-mb N` | limit on the engine's memory *estimate* in MiB (0 = unlimited) | 1024 |
| `--bfs` | breadth-first search: shortest counterexample for safety and deadlock | DFS |
| `--sweep` | keep searching after every property is decided — the state count of the whole graph, as `pan -c0`; needed when comparing counters with SPIN | stop when all properties are decided |
| `--no-timing` | omit `time_ms` so two runs of the same input are byte-for-byte equal | timing included |

The defaults are the "medium model" bounds of plan §12 A4 (`evidence-and-status.md`
§5). Flag names are part of the skill's contract and stay stable. In the CLI a budget
of 0 means *unlimited*; in MCP an absent or zero budget field means the *server
default* (§3) — the CLI budget unification (0 = default, `--unlimited` explicit)
arrives with G4, so until then read "0" by the layer you are on.

Rules that do not change with the layer: pass files by path, never interpolate model
text into a shell line (11 §13); show the exact command line in the report's
Execution section; parse the JSON on stdout, do not scrape prose from stderr
(frontend warnings — a `printf` that is ignored, a never claim that is parsed but not
executed — appear there *and* in the report's `warnings`); if the binary is missing (plan A6), every
property is `not-executed` with reason "engine binary unavailable" and the report says
how to build it (`go build -o /tmp/mcd ./cmd/mcd` in `model-check-plugin/engine`) —
do not simulate a result.

## 2. Exit codes and rejections

The division is "is there a result document?" (`steps/g0-confirmation.md` §4, decision 2):

| Exit code | Meaning | What is on stdout |
|---|---|---|
| exit code 0 | a result was produced — a report or an IR — whatever the verdicts, **including `invalid-model`** | the JSON document |
| exit code 1 | no result: tool error — unreadable file, unknown flag or command, internal failure | nothing; the message is on stderr |
| exit code 2 | no result: the input was rejected by a frontend — schema violation, Promela syntax/semantic error, construct outside the subset, invalid IR | `{"error": {"kind", "status", "path", "message"}}` |

Rejection fields: `kind` is `schema`, `unsupported-input` or `ir` for Petri/IR input
and `syntax`, `semantic` or `outside-subset` for Promela; `status` is always
`not-executed` (nothing rejected has been executed, so no verdict and no
`invalid-model` can be claimed); `path` points into the input (`transitions[0].inputs[0].weight`,
or `file:line:col` for Promela); `message` names the construct, file and line. Real
example — `mcd check --promela "Promela - examples/CH17/simple1.pr"` exits 2 with
`kind: outside-subset`, `path: …/CH17/simple1.pr:1:1`, `message: construct outside
subset: c_code (embedded C is outside the subset) (simple1.pr, line 1)`. Exit code 2 is
the "parser rejected a construct" case of `SKILL.md` step 3: the engine produced no
property record, so you assign `not-executed` yourself, quote `message` as the
reason, and propose a rewrite (`promela-subset.md` §3, `petri-nets.md` §7). Exit
code 1 means fix the command, not the model.

## 3. The MCP server: `mcd serve`

```
mcd serve [--session-dir DIR] [--allow-read DIR]… [--max-states N] [--max-depth N]
          [--max-ms N] [--max-memory-mb N] [--concurrency K] [--cleanup]
```

| Flag | Meaning | Default |
|---|---|---|
| `--session-dir DIR` | base directory under which every session gets `s<UTC time>-<n>/` | `$MCD_SESSION_DIR`, else a fresh temporary directory |
| `--allow-read DIR` | directory (repeatable) whose files `mc_parse` may read when the client names one in `file`; anything else is refused | none — only inline input |
| `--max-states N`, `--max-depth N`, `--max-ms N`, `--max-memory-mb N` | ceilings a client budget cannot exceed; a higher request is clamped and reported in `budget_notes` (0 = no ceiling) | 0 |
| `--concurrency K` | simultaneous `mc_check`/`mc_estimate` runs | 2 |
| `--cleanup` | remove session directories at shutdown (opt-in) | keep |

The plugin's `.mcp.json` starts it as `${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd serve
--max-states 5000000 --max-depth 5000000 --max-ms 300000 --max-memory-mb 2048
--concurrency 2` (installation and the binary build belong to G6).

**Budget rule (MCP).** `budget` has four optional fields `states`, `depth`, `ms`,
`memory_mb`; an absent or zero field means the server default (the CLI defaults of §1,
clamped into the ceilings); a value above the ceiling is clamped with a note. A client
cannot switch a limit off. Three kinds of answer (NFR-007): a **tool error** (`isError`,
text only — bad arguments, unknown kind, unknown session) is not a result; a
**rejected input** is a structured answer `outcome: rejected` with `rejection`
(`mc_parse`, `mc_check`) or an error prefixed `rejected input (<kind>):` (the other
tools); a **result** is a structured answer with `isError` false whatever the statuses.

## 4. The seven tools (field names as the server emits them)

| Tool | Input (JSON fields) | Output (JSON fields) | Call it when |
|---|---|---|---|
| `mc_parse` | exactly one of `promela` (text), `petri` (object), `ir` (object), `file` {`kind`, `path` under `--allow-read`}; `defines` {name: value}; `session_id` (omitted = new session) | `session_id`; `outcome` = `ir` \| `rejected` \| `not-executed`; `ir`, `ir_path` (session file `ir-N.json`), `origins` [{`element`, `file`, `line`, `name`}], `warnings`; `rejection` {`kind`, `construct`, `file`, `line`, `reason`}; `reason` for `not-executed` (frontend not linked — today: Promela) | always first; on every model change. `not-executed` here is a *parse outcome* (missing frontend), a homonym of the verification status |
| `mc_check` | `session_id` or `ir`; `properties` [{`id`, `kind` ∈ invariant \| deadlock \| reach \| ltl \| ctl \| progress, `expr` (IR expression or a bare variable name), `text`}] — they **replace** the model's own properties when given, and the implicit `assert` is always added; `fairness` = none \| weak; `budget`; `search` = dfs \| bfs; `no_timing`; `aggregate` | `session_id`; `outcome` = `report` \| `rejected`; `report_path` (`check-N.json`, the full `mcd-report/1` of §5); `search` {`mode`, `budget_requested`, `budget_applied`, `budget_notes`, `stop`, `complete`}; `properties` [{`id`, `kind`, `text`, `status`, `evidence`, `complete`, `reason`, `counters`, `counterexample` / `witness` {`id`, `path`, `summary`, `steps`, `user_names`}}]; `warnings`; `aggregate` {`status`, `basis`} only when asked; `rejection` | the check itself; rerun with a larger budget after `inconclusive`. Kinds `ltl`, `progress`, `ctl` return `not-executed`, evidence `unknown`, with `reason` naming the capability and the step (G4 for `ltl`/`progress`, G5 for `ctl`) — until then they are boundaries, not results |
| `mc_explain` | `session_id`, `counterexample_id` (from `counterexample.id` or `witness.id`) | `id`, `property_id`, `role` = counterexample \| witness, `path`, `prefix` [{`index`, `process`, `command`, `user_name`, `location`, `changes` [{var, before, after}], `origin`}], `loop` (empty until G4; `loop_note` says so), `final_state`, `summary`, `user_names` | every `violated`; before classifying the cause. A `../` or absolute id is refused (session guard) |
| `mc_simulate` | `session_id` or `ir`; `mode` = random \| guided; `seed` (same seed = same run), `steps` (default 100), `edges` for guided (`process/index`, the edge text, or the origin name) | `mode`, `seed`, `steps_taken`, `stopped` ∈ steps \| deadlock \| terminated \| edge not enabled \| edges exhausted \| assert failed \| invalid-model, `stop_reason`, `summary`, `trace_path` (`sim-N.json`), `trace` inline when ≤ 50 steps, `enabled_at_stop` | the sanity walk of step 5; never as evidence |
| `mc_lint_property` | `session_id` or `ir`; `expr`; `kind` = invariant \| reach | `expr` as the engine reads it, `atoms` (first occurrence order), `undefined` (atoms that are not globals), `type_ok`/`type_error`, `class` = safety \| reachability (with `class_basis`), `x_free`, `temporal`, `constant` (vacuity candidate), `notes` | every state property before a check. Temporal kinds (`ltl`, `ctl`, `progress`) are refused with the step named — the manual classification of `properties-ltl-ctl.md` stands in, and the report says so |
| `mc_estimate` | `session_id` or `ir`; `ms` (default 1000, capped by the server) | `time_limit_ms`, `elapsed_ms`, `states_visited`, `transitions`, `depth_reached`, `complete`, `states_per_second`, `growth` {`per_level` [{depth, states}], `rate`, `rate_basis`}, `projection` {`evidence` = approximate \| exhaustive, `states_at_next_level`, `note`}, `note` ("not a verification result") | before choosing budget and mode. A bounded BFS by levels (G2 interface; the growth model of plan §9 is G5) |
| `mc_manifest` | `session_id` | `path` (`manifest.json`), `manifest` {`session_id`, `created`, `engine` {name, version, ir_schema, report_schema, mcp_schema}, `server` {`default_budget`, `ceiling`, `concurrency`, `allow_read`}, `inputs` [{kind, source, sha256, path}], `calls` [{n, tool, started, duration_ms, outcome = ok \| error, error, params {search, fairness, budget_applied, seed, steps, mode, time_limit_ms}, artifacts}]} | every report (FR-012, NFR-002) |

**CLI stand-ins** when the server is not registered (the with-skill evals of
`evals-workspace/` ran this way): `mc_parse` → `mcd parse`; `mc_check` → `mcd check`
with the budget flags; `mc_explain` → read `counterexample.steps` and `final_state`
in the report (§5.3); `mc_simulate` → no equivalent, skip the walk and say so;
`mc_lint_property` → classify by `properties-ltl-ctl.md` and say it was manual;
`mc_estimate` → a `mcd check` with a small `--budget-states` is a crude stand-in (its
`counters.states` at the budget says how far the budget reached, not how the count
grows); `mc_manifest` → the report's `engine`, `inputs` (with `sha256`) and
`search.budget` cover versions, hashes and budgets; seed and start/end times have no
counterpart — record the command line and wall-clock time yourself and say the
manifest was assembled by hand.

## 5. The report (`mcd-report/1`)

### 5.1. Top level

| Field | Content |
|---|---|
| `engine` | `name` (`mcd`), `version`, `ir_schema`, `report_schema` |
| `inputs` | one entry per input file: `kind` (`petri` / `ir` / `promela`), `path`, `sha256` of the file's bytes |
| `model` | `name`, `state_bytes`, `processes`, `variables` |
| `search` | `mode` (`dfs` / `bfs`), `budget` {`states`, `depth`, `time_ms`, `mem_bytes`} as given, `stop` (why the search ended, as a sentence: "complete", "all properties decided", "invalid model" — the engine's literal text with a space, a stop reason, not a verdict — or a budget sentence), `complete` (true only when the whole reachable graph was expanded) |
| `warnings` | frontend warnings (G1): `printf` ignored, never claim not executed |
| `properties` | one record per property, in the model's property order (§5.2) |

### 5.2. One property record

| Field | Content |
|---|---|
| `id`, `kind`, `text` | the Petri frontend generates `deadlock` (kind `deadlock`) and `safe` (kind `invariant`); the Promela frontend generates `deadlock` and, when some statement is an `assert`, `assert` (kind `assert`); the IR may add `reach` and `invariant` |
| `status` | one of the six words (`evidence-and-status.md` §1) |
| `evidence` | `exhaustive`, `bounded`, `approximate`, `unknown` |
| `counters` | `states`, `transitions`, `depth` (greatest depth expanded), `time_ms` (absent under `--no-timing`), `memory_bytes_est`; the same for every property of one run |
| `complete` | copy of `search.complete` |
| `counterexample` | present for `violated` (the violating run) and for `invalid-model` (the run to the offending step); §5.3 |
| `witness` | present for a `reach` that came back `verified`: the run that reaches the condition |
| `reason` | for `inconclusive` the exhausted resource; for `not-executed` / `invalid-model` what is missing or overflowed; for `violated` the engine may add a one-line diagnosis (`deadlock`: which processes are blocked; `assert`: "assert(…) fails in the last step of the counterexample"; `reach`: no state satisfies the condition) |

### 5.3. A run (`counterexample` / `witness`)

`steps[]`: `index` (1-based), `process` (`user:0`, `init`, …), `command` (the
statement text; for a Petri net the transition name), `changes[]` {`var`, `before`,
`after`} — locals are qualified by instance (`user:1.me`), channel buffers appear as
values —, `location` (control location after the step, when named), `partner` for a
rendezvous step, `origin` {`file`, `line`, `name`} (the Promela line of the statement
is the mapping back to the source). `final_state[]` {`var`, `value`} lists **every**
variable of the last state. `summary` joins the step commands with ", " — for
`petrinet1` it is exactly `t1, t4`. Loops (prefix + cycle) arrive with G4; a run is
a finite path.

## 6. Rules the engine enforces on its own output (K1; `report.Build` refuses a document that breaks them)

- `violated` carries evidence `exhaustive`: its counterexample is an exact run of
  the model, whatever stopped the search afterwards. For `reach`, `violated` means
  "no state satisfies the condition" and requires a complete search.
- `verified` requires `complete` = true and evidence `exhaustive`, except for `reach`.
- `reach` is verified by a witness: the exact run attached as `witness`; the search
  may be incomplete and the verdict still stands.
- Budget exhaustion gives `inconclusive`, evidence `bounded`, `complete` false, with
  `reason` naming the exhausted resource: `state budget exhausted: N states stored`,
  `depth budget exhausted: …`, `time budget exhausted`, `memory budget exhausted: …`.
- A construct outside the subset gives `not-executed`; so does a capability the
  current build lacks — two different boundaries with the same status. A property
  kind the engine does not run yet (`ltl`, `ctl`, `progress`) gets `not-executed`
  from the engine with evidence `unknown` and the kind named; an unsupported *input*
  construct (an inhibitor arc, `c_code`, `unless`, …) is an exit code 2 rejection
  (§2) or `outcome: rejected` (§4), the engine produces no record, and you assign
  `not-executed` yourself (`evidence-and-status.md` §1).
- A domain overflow gives `invalid-model`: a `byte` wrap, a channel or a place pushed
  above its capacity, or a blocked `d_step` (G1), evidence `unknown`, with the run to
  the offending step attached as `counterexample` and the cause in `reason`; **every**
  property still undecided at that point gets `invalid-model`, while a property
  already decided keeps its verdict (`steps/g0-confirmation.md` §4, decision 3).
  SPIN wraps silently where the engine stops (`promela-subset.md` §2).
- `unknown` is in the vocabulary; the G0–G2 engine never emits it as a status.
- Aggregation priority: `invalid-model` > `not-executed` > `violated` > `inconclusive` > `unknown` > `verified` (plan §6). Statuses are per property; the engine aggregates only when `mc_check` is asked (`aggregate: true`) and then also returns `basis`. If the user insists on one word, take the first of this list that occurs and still list the per-property records.

Determinism (NFR-006): the same input and flags produce the same report except
`time_ms`; `--no-timing` / `no_timing` makes it byte-for-byte equal (the `petrinet2`
golden test and the G2 "two identical `mc_check` calls" scenario rely on that).
Anything else that differs between two runs is an engine defect to report.

## 7. Reading a response, in this order

1. **Exit code / `outcome`.** 1 or a tool error → fix the call. 2 or `rejected` →
   quote `kind`, `path`/`construct`, `message`/`reason`; assign `not-executed` to
   every property; propose a rewrite. 0 or `report` → read on.
2. **`search.stop` and `search.complete`** (MCP: also `budget_notes`) — whether the
   graph was exhausted or a budget cut the run, before any verdict.
3. **Per property: `status`, then `evidence`, then `complete`.** Take all three as
   they are; never upgrade an `inconclusive` or downgrade a `violated`.
4. **`counterexample` / `witness`** — decode through `origin` (file, line, name) and
   `final_state` (CLI) or `mc_explain`'s `prefix` with `user_name` and `changes`
   (MCP); classify the cause (`counterexamples.md`).
5. **`reason`** — quote it verbatim.
6. **`counters`** — into the Execution section; they are not evidence.

## 8. Capability by build step

| Capability | Available from |
|---|---|
| Petri JSON (`--petri`), IR JSON (`--ir`); `deadlock`, `invariant`, `reach`, `assert`; DFS/BFS; budgets; `mcd-report/1`; finite-path counterexamples with `origin`; exit codes 0/1/2 | **G0 (built)** |
| Promela subset via `--promela`, `-D`, `--sweep`; counterexamples mapped to Promela lines with instance-qualified locals and rendezvous partners; `pandiff` against `pan -d` | **G1 (built)** |
| MCP server `mcd serve` with the seven tools, session directory, manifest, budget ceilings, read allow-list (the MCP layer arrives with G2 — built; Promela not linked into the server yet) | **G2 (built)** |
| `ltl` (never claims and formulas), `progress`, weak fairness, loop counterexamples; CLI budget unification | G4 (LTL evidence `unknown`/experimental until the differential oracle passes, plan §11) |
| `ctl`, vacuity for temporal formulas, the growth model of `mc_estimate`, Promela v1 (`inline`, `typedef`, channels in messages, `_nr_pr`) | G5 |
| plugin installation (`.mcp.json`, binary build) validated with a real client | G6 |
| bitstate (`approximate`), POR, parallel BFS | G7 (vNext, if chosen) |

## 9. Session directory and artefacts (11 §13)

With the server, every session lives under `<session-dir>/<id>/`: `ir-N.json`,
`check-N.json`, `sim-N.json`, `cex/cex-N.json`, `manifest.json`; the server is the
only writer and refuses any path that leaves the directory (`..`, absolute, symlink).
Responses carry absolute paths of these files; cite them as the Artifacts of the
report. With the CLI, make the directory yourself (`mc-session-<date>/`), write the
input, the IR and the report there, and list the relative paths. Never overwrite the
user's sources; models, logs and traces may contain business data — ask before
storing them anywhere else (workflow question 14).
