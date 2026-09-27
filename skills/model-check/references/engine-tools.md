# Engine tools: the `mcd` CLI and the seven MCP tools (as built in G0–G4)

Sources: `model-check-skill-notes/14-skill-building-plan.md` §3 (three layers),
§6 (tool table, status vocabulary, aggregation priority, `bounded`/`unknown` rule,
CLI), §9 (which build step delivers which capability), §11 (LTL evidence `unknown`
while experimental), §12 A5, A6; `model-check-skill-notes/11-skill-requirements.md`
§10 (staged execution, manifest), §13 (safety of artefacts), NFR-004, NFR-006,
NFR-007. Engine as built: `model-check-plugin/engine/cli/cli.go` (commands, flags,
exit codes), `engine/report/report.go` (report schema `mcd-report/1`, rules of
`report.Build`), `engine/mcp/*.go` (the seven tools, field names from the Go struct
tags), `engine/cmd/mcd/serve.go` (`mcd serve`), `steps/g0-confirmation.md` §4,
`steps/g1-confirmation.md` §1, `steps/g2-confirmation.md` §1, §4,
`steps/g4-confirmation.md` §7 (what G4 added to these interfaces). **Every MCP
request and response quoted below was copied from one hand-driven stdio session,
recorded in `steps/g3-evals3-mcp-session.md`** — not written from memory; when you
need a shape this file abbreviates, read that file.

## Contents

1 the CLI · 2 exit codes and rejections · 3 the MCP server (`mcd serve`) ·
4 the seven tools · 5 the report (`mcd-report/1`) · 6 rules the engine enforces ·
7 reading a response · 8 capability by build step · 9 session directory.

The engine is one Go binary, `mcd`. Two layers reach it: the CLI (G0, Promela with
G1, temporal properties with G4) and the MCP server `mcd serve` (G2). Both return
the same statuses, evidence levels and per-property records; the MCP layer wraps the
report of §5 in the response shapes of §4 and keeps files in a session directory
(§9). Which layer you use: the MCP tools when the plugin's server is registered in
the session; otherwise the CLI. **`mcd serve` links the Promela frontend**: since G4
`mc_parse` with a `promela` field returns `outcome: "ir"` with the frontend's
`warnings` (session file §1.1), so a Promela model can be verified entirely over
MCP. The CLI remains the path when no server is registered, and it is the only layer
with `--unlimited` (§1).

## 1. The CLI

```
mcd parse   (--petri net.json | --ir model.json | --promela model.pml [-D NAME[=val]]…)
                                                                  → IR JSON on stdout
mcd check   (--petri net.json | --ir model.json | --promela model.pml [-D …])
            [--ltl 'formula']… [--progress] [--fairness none|weak|strong]
            [--budget-states N] [--budget-depth N] [--budget-ms N] [--budget-mem-mb N]
            [--unlimited] [--bfs] [--sweep] [--no-timing]         → report JSON on stdout
mcd version                                      → "mcd <version> (ir <schema>, report <schema>)"
mcd serve   [flags of §3]                        → MCP server on stdio
```

| Flag | Meaning | Default |
|---|---|---|
| `--petri FILE` | Petri net JSON (`assets/petri-net.schema.json`, byte-for-byte the engine's `frontend/petri/schema.json`) | exactly one of `--petri` / `--ir` / `--promela` |
| `--ir FILE` | IR JSON (`mcd-ir/1`), validated and re-emitted canonically | — |
| `--promela FILE` | Promela text, the subset of `promela-subset.md` §1 (the flag `--promela` is built, G1) | — |
| `-D NAME` / `-D NAME=value` | preprocessor symbol, repeatable (`CH4/prop.pml` needs `-D PHI`) | none |
| `--ltl 'φ'` | an LTL formula in SPIN syntax (`[] <> X U V ! && \|\| -> <->`), repeatable; the properties are named `ltl1`, `ltl2`, … in the order given. Atoms are expressions over the model's variables and channels; object-like `#define`s of the model are expanded. A formula the parser refuses is an input rejection (§2, `kind: "ltl"`), not a property result | none |
| `--progress` | search for non-progress cycles, SPIN's `pan -l`: adds a property `progress` checked against a synthesised `np_` automaton. The model's own `progress` labels decide which cycles count — a model *without* any progress label makes every cycle a non-progress cycle (`properties-ltl-ctl.md` §7) | off |
| `--fairness none\|weak\|strong` | the fairness assumption for `ltl` and `progress` properties. `weak` is process-level weak fairness (`pan -f`, the n + 2 copies construction); `strong` is accepted and answered with `not-executed` (`fairness.md` §2) | `none` |
| `--budget-states N` | stop after N stored states (absent or 0 = the default) | 1 000 000 |
| `--budget-depth N` | states deeper than N transitions are stored and counted, not expanded (absent or 0 = the default) | 1 000 000 |
| `--budget-ms N` | wall-clock limit in milliseconds (absent or 0 = the default) | 60 000 |
| `--budget-mem-mb N` | limit on the engine's memory *estimate* in MiB (absent or 0 = the default) | 1024 |
| `--ctl 'φ'` | a CTL formula, checked by graph labelling; repeatable, properties `ctl1`, `ctl2`, …. **Built in G5** | none |
| `--estimate`, `--estimate-ms N`, `--target-depth N` | print the state-space growth estimate of plan §6 *instead of* checking properties; `--estimate-ms` is its time limit, `--target-depth` the depth it projects to. **G5** | off, 1000 ms, next level |
| `--max-procs N` | instances pre-instantiated per proctype, i.e. the size of the pool every `run` of that proctype draws from (`promela-subset.md` §2). **G5** | 8 |
| `--unlimited` | lift every budget; the report echoes 0 for the lifted limits. **CLI only** — there is no MCP equivalent, because a client must not be able to switch a server ceiling off (§3). Meant for differential runs against `pan`, not for a user's model | off |
| `--bfs` | breadth-first search: shortest counterexample for safety and deadlock | DFS |
| `--sweep` | keep searching after every property is decided — the state count of the whole graph, as `pan -c0`; needed when comparing counters with SPIN | stop when all properties are decided |
| `--no-timing` | omit `time_ms` so two runs of the same input are byte-for-byte equal | timing included |

The defaults are the "medium model" bounds of plan §12 A4 (`evidence-and-status.md`
§5). Flag names are part of the skill's contract and stay stable. **The budget rule
is now identical in the CLI and in MCP** (G4 unified them): an absent or zero budget
means the default, in both layers, and nothing a caller writes switches a limit off.
Only the CLI can lift a budget, and only through the explicit `--unlimited`. Before
G4 a CLI `0` meant *unlimited*; if you find that reading in an older report or note,
it no longer holds.

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

Rejection fields: `kind` is `schema`, `unsupported-input` or `ir` for Petri/IR input,
`syntax`, `semantic` or `outside-subset` for Promela, and `ltl` for a formula the
LTL parser refuses or whose atoms are not in the model's scope (G4: a bad `--ltl`
is a rejected *input*, not a property verdict — the run produces no report at all);
`status` is always
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

The plugin declares the server in **`mcp/servers.json`** (`plugin.json` points at it
with `"mcpServers": "./mcp/servers.json"`), which starts it as
`${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd serve --max-states 5000000 --max-depth 5000000
--max-ms 300000 --max-memory-mb 2048 --concurrency 2`.

**Not `.mcp.json`, and not at the plugin root.** G6 found that a `.mcp.json` in the
plugin root is read twice when the plugin directory is itself the working directory:
once as the plugin's own declaration and once as a *project* config, which registered
the server a second time — `plugin:model-check:model-check ✔ Connected` beside a
second `model-check … ⏸ Pending approval` whose `${CLAUDE_PLUGIN_ROOT}` was unset.
Moving the file out of the root removes the second path; its contents did not change
(`steps/g6-confirmation.md` §3.2–3.3). If you are debugging an installation and see
the server listed twice, this is the shape to look for.

**Budget rule (the same in both layers since G4).** `budget` has four optional
fields `states`, `depth`, `ms`, `memory_mb`; an absent or zero field means the
server default (the CLI defaults of §1, clamped into the ceilings), and an absent
or zero CLI budget flag means the same default; a value above the ceiling is
clamped with a note. A client cannot switch a limit off — `--unlimited`
has no MCP counterpart by design. Three kinds of answer (NFR-007): a **tool error** (`isError`,
text only — bad arguments, unknown kind, unknown session) is not a result; a
**rejected input** is a structured answer `outcome: rejected` with `rejection`
(`mc_parse`, `mc_check`) or an error prefixed `rejected input (<kind>):` (the other
tools); a **result** is a structured answer with `isError` false whatever the statuses.

## 4. The seven tools (field names as the server emits them)

| Tool | Input (JSON fields) | Output (JSON fields) | Call it when |
|---|---|---|---|
| `mc_parse` | exactly one of `promela` (text), `petri` (object), `ir` (object), `file` {`kind`, `path` under `--allow-read`}; `defines` {name: value}; `max_procs` (instances pre-instantiated per proctype, default 8); `session_id` (omitted = new session) | `session_id`; `outcome` = `ir` \| `rejected` \| `not-executed`; `ir`, `ir_path` (session file `ir-N.json`), `origins` [{`element`, `file`, `line`, `name`}], `warnings`; `rejection` {`kind`, `construct`, `file`, `line`, `reason`}; `reason` for `not-executed` (a frontend this server does not link) | always first; on every model change. Promela works here since G4 (`outcome: "ir"`; inline text gives `file: "inline"` in `origins`). `not-executed` here is a *parse outcome*, a homonym of the verification status |
| `mc_check` | `session_id` or `ir`; `properties` [{`id`, `kind` ∈ invariant \| deadlock \| reach \| ltl \| ctl \| progress, `expr` (IR expression JSON or a bare variable name — state kinds only), **`formula`** (`ltl`: SPIN syntax; `ctl`: CTL syntax `AG AF AX EG EF EX`, `A[f U g]`, `E[f U g]` — **required**, and `expr` is never used for either), `text`}] — they **replace** the model's own properties when given, and the implicit `assert` is always added; `fairness` = none \| weak \| strong; `budget`; `search` = dfs \| bfs; `no_timing`; `aggregate` | `session_id`; `outcome` = `report` \| `rejected`; `report_path` (`check-N.json`, the full `mcd-report/1` of §5); `search` {`mode`, `budget_requested`, `budget_applied`, `budget_notes`, `stop`, `complete`}; `properties` [{`id`, `kind`, `text`, `status`, `evidence`, `complete`, `reason`, `counters`, `counterexample` / `witness` {`id`, `path`, `summary`, `steps`, `user_names`, and for a lasso `loop` {`start`, `steps`}}, **`temporal`** (§5.2)}]; `warnings`; `aggregate` {`status`, `basis`} only when asked; `rejection` (`kind: "ltl"` for a bad formula) | the check itself; rerun with a larger budget after `inconclusive`. `ltl` and `progress` are executed since G4, **and `ctl` since G5** (it takes `formula`, not `expr`). There is no property kind left that the engine refuses to run. `fairness: "strong"` is accepted and answered with `not-executed` (`fairness.md` §2), which is a result: `isError` false, `outcome: "report"` |
| `mc_explain` | `session_id`, `counterexample_id` (from `counterexample.id` or `witness.id`) | `id`, `property_id`, `role` = counterexample \| witness, `path`, `prefix` [{`index`, `process`, `command`, `user_name`, `location`, `changes` [{var, before, after}], `origin`}], `loop` (the same step shape; empty for a finite counterexample), `loop_note` (one sentence saying which step indices are the prefix and which are the loop, or that there is no loop), `final_state`, `summary`, `user_names` | every `violated`; before classifying the cause. A `../` or absolute id is refused (session guard). The split follows the report's `counterexample.loop.start`: steps before it are the prefix, the rest repeat forever (`counterexamples.md` §2a) |
| `mc_simulate` | `session_id` or `ir`; `mode` = random \| guided; `seed` (same seed = same run), `steps` (default 100), `edges` for guided (`process/index`, the edge text, or the origin name) | `mode`, `seed`, `steps_taken`, `stopped` ∈ steps \| deadlock \| terminated \| edge not enabled \| edges exhausted \| assert failed \| invalid-model, `stop_reason`, `summary`, `trace_path` (`sim-N.json`), `trace` inline when ≤ 50 steps, `enabled_at_stop` | the sanity walk of step 5; never as evidence |
| `mc_lint_property` | `session_id` or `ir`; `expr` with `kind` = invariant \| reach, or `formula` with `kind` = `ltl` \| `ctl` | `expr` as the engine reads it, `atoms` (first occurrence order), `undefined` (atoms that are not globals), `type_ok`/`type_error`, `class` = safety \| reachability (with `class_basis`), `x_free`, `temporal`, `constant` (vacuity candidate), `nnf` (ltl: negation normal form), `normalised` (ctl: the formula in the `EX`/`EU`/`EG` basis, as it will be labelled), `notes` | every property before a check. `ltl` since G4 and `ctl` since G5 are linted too: atoms, undefined atoms, `x_free`, the normalised form, a syntactic safety/liveness class and a note when the antecedent of an implication may be vacuous. `progress` is the one kind still refused — it is a label search, not a formula |
| `mc_estimate` | `session_id` or `ir`; `ms` (default 1000, capped by the server); `target_depth` (the depth to project the growth to; omitted = only the next level) | `time_limit_ms`, `elapsed_ms`, `states_visited`, `transitions`, `depth_reached`, `complete`, `states_per_second`, `growth` {`per_level` [{depth, states}], `rate`, `rate_basis`}, `projection` {`evidence` = approximate \| exhaustive, `states_at_next_level`, `note`}, `size` (the A4 size class of plan §12 — small \| medium \| large — and what to do about it), `note` ("not a verification result") | before choosing budget and mode. A bounded BFS by levels; the growth model of plan §9 and the `size` class arrived with G5 (CLI: `--estimate`, `--estimate-ms`, `--target-depth`) |
| `mc_manifest` | `session_id` | `path` (`manifest.json`), `manifest` {`session_id`, `created`, `engine` {name, version, ir_schema, report_schema, mcp_schema}, `server` {`default_budget`, `ceiling`, `concurrency`, `allow_read`}, `inputs` [{kind, source, sha256, path}], `calls` [{n, tool, started, duration_ms, outcome = ok \| error, error, params {search, fairness, budget_applied, seed, steps, mode, time_limit_ms}, artifacts}]} | every report (FR-012, NFR-002) |

**Adding your own properties without the server.** `mc_check`'s `properties` list
has no CLI flag, but it is not the only way in. The CLI reads a *property list from
the IR*: run `mcd parse` on the model, append records to the IR's `properties` array
— `{"id", "kind", "expr"}` for `invariant` and `reach` (an IR expression object:
`{"op": "ge", "args": [{"op": "var", "var": "p2"}, {"op": "const", "value": 1}]}`),
`{"id", "kind": "ltl", "formula"}` for a temporal one — and run `mcd check --ir` on
the edited file. This is how a `reach` property is asked for from the CLI; `--ltl`
and `--progress` are shortcuts for the temporal cases only. The model's own
properties stay unless you remove them.

**CLI stand-ins** when the server is not registered (the with-skill evals of
`evals-workspace/` ran this way): `mc_parse` → `mcd parse`; `mc_check` → `mcd check`
with the budget flags, and property lists through the edited IR as just described; `mc_explain` → read `counterexample.steps`, `counterexample.loop`
and `final_state` in the report (§5.3) — the CLI report carries the same steps, so
the only thing missing is the `loop_note` sentence, which you write yourself; `mc_simulate` → no equivalent, skip the walk and say so;
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
| `warnings` | frontend warnings (G1), e.g. that `printf` statements are kept as no-op steps and produce no output. Read them; they are also on stderr |
| `properties` | one record per property, in the model's property order (§5.2) |

### 5.2. One property record

| Field | Content |
|---|---|
| `id`, `kind`, `text` | the Petri frontend generates `deadlock` (kind `deadlock`) and `safe` (kind `invariant`); the Promela frontend generates `deadlock` and, when some statement is an `assert`, `assert` (kind `assert`), **and from the model itself**: `never` (kind `ltl`) when the file has a never claim, `accept` (kind `ltl`) when a process has an `accept` label and there is no claim, `progress` (kind `progress`) when some process has a `progress` label — you did not ask for these and they are still yours to read (`properties-ltl-ctl.md` §7). The CLI adds `ltl1`, `ltl2`, … for each `--ltl`, and `progress` for `--progress`; the IR may add `reach` and `invariant` |
| `status` | one of the six words (`evidence-and-status.md` §1) |
| `evidence` | `exhaustive`, `bounded`, `approximate`, `unknown` |
| `counters` | `states`, `transitions`, `depth` (greatest depth expanded), `time_ms` (absent under `--no-timing`), `memory_bytes_est`; the same for every property of one run |
| `complete` | for a state property, a copy of `search.complete`; a temporal property has its **own** `complete` — its search is a separate product with its own automaton, and `false` here means that product was not exhausted (a `violated` lasso stops the search, so `violated` normally comes with `complete: false` and evidence `exhaustive` — see §6) |
| `temporal` | present for `ltl`, `progress` and `ctl`. Always: `logic` (`ltl` \| `ctl`), `source` (`formula` for `--ltl`/`--ctl`, `claim` for the model's never claim, `np` for non-progress), `formula`, `atoms` in first-occurrence order, `fairness` (`none`/`weak`/`strong` as asked). **`ltl`/`progress` only**: `negated` (the engine builds the automaton for the negation), `stutter_invariant` (false exactly when the formula uses `X`), `automaton_states`, `automaton_transitions`, `automaton_accepting`, `claim` (the claim process's name in the trace: `never:<property id>` — `never:ltl1` for the CLI's first `--ltl` — or `np_`). **`ctl` only**: `normalised` (the formula rewritten into the `EX`/`EU`/`EG` basis, which is what was actually labelled — `AG (cnt <= 1)` becomes `!E[true U !(cnt <= 1)]`; quote this, not just `formula`, when the two differ), `note` (why a blocked state carries a self-loop), `witness_note` (why a verdict carries no run — "a universal property that holds is justified by the complete reachable graph, not by one run"), and the vacuity pair `vacuous` / `vacuous_atom` |
| `warnings` (per property) | a property may carry its own `warnings` beside the report's. This is where CTL vacuity reaches a reader in words: "vacuity hint for ctl1: the atom `cnt == 7` is never true in any of the 429 reachable states…" and, for an implication, "vacuous: the antecedent … is never true … the verdict below is unchanged — this is a hint, not a status". Read them: a `verified` with a vacuity warning is the case AC-13 forbids you to report as a guarantee |
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
`petrinet1` it is exactly `t1, t4`; for a lasso it inserts `; loop: ` before the
cycle.

**`loop` {`start`, `steps`} (G4)** turns the run into a lasso: `start` is the
1-based index of the first step of the cycle and `steps` its length, so steps
`1 … start-1` are the prefix and steps `start … start+steps-1` repeat forever. A
finite counterexample (invariant, `assert`, `deadlock`, a safety formula) has no
`loop` at all. Two process names appear only in lassos: the claim process
(`never:<property id>`, or `np_` for `--progress`), whose steps you drop when
presenting the trace to a user, and `-`, a **null step** of the weak-fairness
construction — it advances the fairness copy without any process moving
(`counterexamples.md` §2a).

## 6. Rules the engine enforces on its own output (K1; `report.Build` refuses a document that breaks them)

- `violated` carries evidence `exhaustive`: its counterexample is an exact run of
  the model, whatever stopped the search afterwards. For `reach`, `violated` means
  "no state satisfies the condition" and requires a complete search.
- `verified` requires `complete` = true and evidence `exhaustive`, except for `reach`.
- `reach` is verified by a witness: the exact run attached as `witness`; the search
  may be incomplete and the verdict still stands.
- `violated` for a temporal property is an **acceptance cycle** (or, for `--progress`,
  a non-progress cycle): the lasso of §5.3, with `reason` naming the promise that is
  never discharged and, when fairness was `none`, which process is enabled through
  the loop and never moves.
- Budget exhaustion gives `inconclusive` and `complete` false, with `reason` naming
  the exhausted resource. The evidence depends on **which** budget stopped it
  (G4, plan §6): a states or depth stop is `bounded` — the engine can name the bound
  and the result holds up to it; a time or memory stop is `unknown` — where the
  search stood when the clock ran out is not a bound anyone can state
  (`evidence-and-status.md` §3). Reasons: `state budget exhausted: N states stored`,
  `depth budget exhausted: …`, `time budget exhausted`, `memory budget exhausted: …`.
- A construct outside the subset gives `not-executed`; so does a capability the
  current build lacks — two different boundaries with the same status. The only
  capability boundary left in the property kinds is none: `invariant`, `deadlock`,
  `reach`, `assert`, `ltl`, `progress` and — since G5 — `ctl` all run. What remains
  a boundary is `fairness: strong`: that property is `not-executed`, evidence
  `unknown`, with FR-008 in the `reason`. An unsupported *input*
  construct (an inhibitor arc, `c_code`, `unless`, …) is an exit code 2 rejection
  (§2) or `outcome: rejected` (§4), the engine produces no record, and you assign
  `not-executed` yourself (`evidence-and-status.md` §1).
- A domain overflow gives `invalid-model`: a `byte` wrap, a channel or a place pushed
  above its capacity, or a blocked `d_step` (G1), evidence `unknown`, with the run to
  the offending step attached as `counterexample` and the cause in `reason`; **every**
  property still undecided at that point gets `invalid-model`, while a property
  already decided keeps its verdict (`steps/g0-confirmation.md` §4, decision 3).
  SPIN wraps silently where the engine stops (`promela-subset.md` §2).
- `unknown` is in the vocabulary; the engine still never emits it as a *status* —
  since G4 it does emit it as an *evidence* level for a time or memory budget stop.
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
   (MCP); when `loop` is present read it as a lasso and say which steps repeat;
   classify the cause (`counterexamples.md`).
5. **`temporal`**, for a temporal property — which formula was actually checked,
   whether it was negated (it always is), what the atoms are, and which fairness
   was in force. A report that shows a verdict without saying which formula
   produced it is unreadable; quote `temporal.formula` and `temporal.fairness`
   next to the status.
6. **`reason`** — quote it verbatim.
7. **`counters`** — into the Execution section; they are not evidence.

## 8. Capability by build step

| Capability | Available from |
|---|---|
| Petri JSON (`--petri`), IR JSON (`--ir`); `deadlock`, `invariant`, `reach`, `assert`; DFS/BFS; budgets; `mcd-report/1`; finite-path counterexamples with `origin`; exit codes 0/1/2 | **G0 (built)** |
| Promela subset via `--promela`, `-D`, `--sweep`; counterexamples mapped to Promela lines with instance-qualified locals and rendezvous partners; `pandiff` against `pan -d` | **G1 (built)** |
| MCP server `mcd serve` with the seven tools, session directory, manifest, budget ceilings, read allow-list (the MCP layer is built, G2). The CLI and the MCP layer reach the same engine and return the same statuses, evidence levels and property records | **G2 (built)** |
| `ltl` (never claims and `--ltl` formulas), `progress` (`--progress` and `progress` labels), weak fairness, lasso counterexamples (`loop`), `--unlimited`, the same budget rule in both layers, `mc_parse{promela}` over MCP | **G4 (built)**. Plan §11 kept LTL evidence at `unknown` only "while experimental"; the differential oracle of `steps/g4-confirmation.md` §3.1 (43 triples agreeing with `pan`) is what ends that, so LTL results now carry the ordinary evidence levels of §6 |
| `ctl` (`--ctl`, graph labelling), vacuity for temporal formulas, the growth model of `mc_estimate` (`--estimate`, `--target-depth`, and a `size` class), Promela v1 (`inline`, `typedef`, `provided`, channels in messages, arrays of channels, `_nr_pr`, `pc_value`, `run` in a loop with `--max-procs`) | **G5 (built)**. The fields are in §5.2 (`temporal`, per-property `warnings`) and §4 (`mc_check`, `mc_lint_property`, `mc_estimate`, `mc_parse`); the subset is in `promela-subset.md` §3, re-derived from the engine by the probes of `steps/g3-evals3-subset-probe.md`. The vocabulary of `features/g5-ctl-v1.feature` is the authority for witness and counterexample shapes |
| the G5 addendum: no stutter extension inside the `np_` product, and one instance pool per proctype (`counterexamples.md` §2a, `promela-subset.md` §2) | **G5 addendum (built)** |
| plugin installation (`mcp/servers.json`, the cross-platform binary build and `BUILD-INFO.json`) validated with a real client | **G6 (built)** |
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
