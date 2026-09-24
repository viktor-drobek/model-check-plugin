# Engine tools: the seven MCP tools and the `mcd` CLI fallback

Sources: `model-check-skill-notes/14-skill-building-plan.md` §3 (three layers),
§6 (tool table, status vocabulary, CLI), §9 (which build step delivers which
capability), §11 (LTL evidence `unknown` while experimental), §12 A5, A6;
`model-check-skill-notes/11-skill-requirements.md` §10 (staged execution,
manifest), §13 (safety of artefacts), NFR-004, NFR-006, NFR-007.

The engine is one Go binary, `mcd`. As an MCP server (stdio) it exposes seven
tools; the same binary with `--cli` gives the command line for environments without
MCP. Both return the same JSON. The tools never read a path you did not pass and
never write outside the session directory (NFR-004). Long results (counterexamples,
statistics) are written to the session directory; the response carries the path
and a summary (plan §6).

## 1. The seven tools

| Tool | Input | Output | Call it when |
|---|---|---|---|
| `mc_parse` | Promela text **or** Petri JSON **or** IR JSON | IR, mapping table (IR element → file/line/user name), warnings; or an error with position and construct | always first; on every model change |
| `mc_simulate` | IR, `seed`, `steps`, `mode` (`random` / `guided` with a step list) | a trace of steps (process, statement, changed variables, send/receive) | sanity check after parsing; to replay a counterexample by hand; never as evidence |
| `mc_check` | IR, list of properties (`invariant` / `deadlock` / `reach` / `ltl` / `ctl` / `progress`), `fairness` (`none` / `weak`), budget (time, states, depth, memory), search mode (`dfs` / `bfs`) | per property: `status`, `evidence`, counters (states, transitions, depth, elapsed, peak memory), a counterexample id or the reason for `inconclusive`; server errors separately | the check itself; rerun with a larger budget on `inconclusive` |
| `mc_explain` | counterexample id, mapping | prefix and loop, per-step variable diff, user names, the violated subformula / the deadlocked processes | every `violated`; before classifying the cause |
| `mc_lint_property` | formula (LTL or CTL or never claim), IR | atoms and their definedness, syntactic class (safety / liveness), vacuity candidates, `X`-free flag, polarity note for never claims | every property before `mc_check` |
| `mc_estimate` | IR, time limit | growth of the state count over a partial traversal; projected fit into a budget | before choosing budget and mode; before a large run |
| `mc_manifest` | session id | engine and schema versions, input hashes, parameters, seed, fairness, budgets, start/end times | for every report (FR-012, NFR-002) |

The order of a normal session: `mc_parse` → `mc_lint_property` (each formula) →
`mc_simulate` → `mc_estimate` → `mc_check` (staged budgets) → `mc_explain` (each
violation) → `mc_manifest`. `workflow.md` node 11 gives the staging.

## 2. Reading a `mc_check` response

For each property you get one record. Read the fields in this order:

1. **Server error present?** A server error (bad input, timeout of the *server*,
   crash) is not a property result; it is reported separately (NFR-007). Retry or
   fall back to the CLI; the property gets no status from a server error, and the
   report says the run did not complete.
2. **`status`** — one of the six words of `evidence-and-status.md`. Take it as is.
3. **`evidence`** — `exhaustive`, `bounded`, `approximate`, `unknown`. Take it as
   is. Two engine rules you must expect: while the LTL translation has not passed
   the differential oracle (build step G4), LTL results carry evidence `unknown` with
   an `experimental` flag even when the search completed (plan §11); an
   `approximate` level appears only when a bitstate mode exists (vNext).
4. **Counters** — states, transitions, depth, elapsed, peak memory. They go into
   the report's Execution section; they are not evidence by themselves.
5. **Counterexample id** (for `violated`) or **reason** (for `inconclusive`:
   which budget was exhausted; for `unknown`: why the result is not interpretable;
   for `not-executed`: which construct or capability is missing; for
   `invalid-model`: which domain or capacity overflowed and where).

Determinism (NFR-006): the same IR, properties, fairness, budget and seed produce
the same statuses, formulas and traces. If two runs differ in anything but
elapsed time and memory, that is an engine defect to report, not a nuance to smooth
over.

## 3. Capability by build step

The engine grows in steps (plan §9). A tool that is asked for something its build
does not have returns `not-executed` with the capability named; never fill the
gap with your own reasoning.

| Capability | Available from |
|---|---|
| Petri JSON, `deadlock`, `invariant`, `reach`, DFS/BFS, JSON report, CLI | G0 |
| Promela subset (chapters 2–3), counterexamples with reverse mapping | G1 |
| MCP server with all seven tools, manifest, session directory | G2 |
| `ltl` (never claims and formulas), `progress`, weak fairness | G4 (LTL evidence `unknown`/experimental until the differential oracle passes) |
| `ctl`, vacuity in `mc_lint_property`, `mc_estimate`, Promela v1 (`inline`, `typedef`, channels in messages) | G5 |
| bitstate (`approximate`), POR, parallel BFS | G7 (vNext, if chosen) |

## 4. CLI fallback

Without MCP, run the same binary as a command: `mcd --cli <subcommand>`. The plan's
example shape is `mcd check --ir model.ir.json --ltl '[]p' …` (plan §6); the exact
flag names are fixed when G2 lands — read `mcd --cli --help` and do not guess. Rules
that do not change:

- Pass files by path; do not interpolate model text into a shell line (11 §13).
- Show the command in the report (Execution section) and in the manifest.
- The JSON on stdout is the same schema as the MCP result; parse it, do not scrape
  prose.
- If neither MCP nor the binary is available (plan A6), every property is
  `not-executed` with the reason "engine binary unavailable" and the report tells
  the user how to obtain it. Do not simulate a result.

## 5. Session directory and artefacts (11 §13)

The server keeps a per-session directory for IR, traces, statistics and the
manifest. Report its relative paths in the Artifacts section. It never overwrites
the user's sources; generated files are separate from user files; cleanup removes
only what the manifest lists. Models, logs and traces may contain business data —
ask before storing them outside the session directory (workflow question 14).
