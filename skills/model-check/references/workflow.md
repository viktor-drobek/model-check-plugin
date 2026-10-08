# Workflow: decision tree with exit points

Sources: `model-check-skill-notes/11-skill-requirements.md` §4 (mandatory questions),
§6 (decision tree), §10 (staged execution), §12 (`inconclusive`/`unknown`);
`model-check-skill-notes/10-cross-book-synthesis.md` §3 (end-to-end workflow), §12
(heuristics); `model-check-skill-notes/14-skill-building-plan.md` §7.2 (eight steps).

Read this at the start of a session and whenever you are unsure whether to stop, ask,
or continue. The tree below is the 12-step tree of 11 §6 re-cut for an engine that
only does explicit-state checking of finite untimed models; steps that concerned
external symbolic or BMC backends have become exit points.

## 1. How to read the tree

Each node has exactly one of three outcomes: **continue** to the next node, **ask**
the user one grouped set of questions, or **exit** with a status. Exits are
`not-executed` (nothing ran) or, after a run, one of the statuses in
`evidence-and-status.md`. An exit is a legitimate end of the workflow; the report is
still written, with the exit reason in section 9 (Limitations) and the route in
section 10 (Next actions).

At every node take the **first** branch whose condition holds; the branches are
ordered so that exits come before continuations. If no branch fits, that is a defect
in this file — record it in the report's Limitations and pick the branch that claims
least.

## 2. Mandatory questions (11 §4)

Ask what is missing and cannot be safely inferred; group the questions; do not repeat
what the user already said. For a teaching example you may state labelled defaults
instead of asking. For a critical check, fairness, atomicity and the system boundary
need the user's explicit confirmation, defaults are not enough.

1. What is the system and what is the environment? Which components are excluded?
2. What statement is to be checked, and what counts as a violation?
3. Is the goal bug-finding, a bounded check, or an exhaustive result?
4. Is the system finite? If not, which data, queues, process counts or time are unbounded?
5. What is the atomic step: one statement, one automaton transition, one event, one synchronous tick?
6. What is the composition: interleaving, rendezvous, buffered channels, synchronous update?
7. How are the initial states given; are deadlocks or terminal states acceptable?
8. What is assumed about the scheduler, message delivery, failures, environment inputs?
9. Is fairness needed; why are the excluded unfair paths unrealistic? Continuously or infinitely-often enabled?
10. Which events and variables are observable by the property; is `X` needed?
11. Are there hard time bounds or probabilities? (If yes → exit at node 4.)
12. Which tools are allowed? (Here: the built-in engine; MCP or CLI.)
13. Limits on time, memory, depth, disk?
14. May models, logs and traces be saved; do they contain secrets or personal data?

## 3. The tree

### Node 1 — goal

- A guarantee is wanted (alone or together with bug-finding) → continue; nodes 3 and 6 must both pass for `exhaustive` to be possible.
- Bounded assurance is enough → continue with a declared states or depth budget, and say what such a run can and cannot establish (`evidence-and-status.md` §3: it names the bound that stopped it, and under a depth bound it may also have missed paths shorter than the bound). Do not promise the outcome in advance: a run under a budget still returns `violated` / `exhaustive` when it finds a counterexample, and `verified` / `exhaustive` for a `reach` whose witness it found, because one run decides both. `inconclusive` / `bounded` is what you get only if nothing was decided before the budget stopped it.
- Only a concrete failing scenario is wanted → continue with early stop enabled (`mc_check` stops at the first violation per property).

### Node 2 — is there a model?

- A Promela file or Petri JSON exists → validate it (`mc_parse`), continue.
- An implementation or a semi-formal description (diagram, transition table) exists → build the model and a mapping table (source name → model name); continue.
- Only prose → **ask** for the state/transition/property contract (questions 1, 2, 5, 6, 7); then build the model in the Promela subset. Direct IR authoring is experimental (plan A7) and is offered only after the Promela route was tried.

### Node 3 — finiteness

- All domains, process counts and capacities bounded → finite; continue.
- Unbounded integers, heap, recursion, queues, dynamic process creation, dense time → potentially infinite. **Ask** whether a bound may be introduced (question 4). With a bound: continue, mark every bound as a visible parameter, and the result is about the bounded model only. Without a bound: **exit** `not-executed`, route: symbolic/abstraction methods outside this engine.

### Node 4 — semantic class

- Untimed, nondeterministic → continue.
- Real-time clocks or deadlines in time units → **exit** `not-executed`, route: timed model checking (TCTL, zones). Before exiting, check with the user whether the requirement is really about time: "the protocol has timeouts" is often untimed — Promela `timeout` is "no other action is enabled", not a clock (10 §13).
- Probabilities or expected values → **exit** `not-executed`, route: probabilistic model checking (PCTL/CSL).

### Node 5 — property class

- Time bound inside the property → back to node 4 exit.
- Probability inside the property → back to node 4 exit.
- "From every state there exists a path to …", "it is possible that …" (an existential or nested path quantifier) → CTL; continue. CTL is checked without fairness (engine scope).
- "Every request is eventually answered", "infinitely often", "eventually forever" → liveness; continue **through node 7**.
- "Bad state never reached" → invariant / safety / deadlock; continue.

### Node 6 — complete or bounded

- `mc_estimate` shows the reachable space fits the budget → exhaustive search; continue.
- It does not fit → reduce the model first (smaller data, fewer processes, smaller queues — each change documented), then rerun the estimate. If it still does not fit → run with the budget anyway and expect `violated` (a counterexample is a counterexample) or `inconclusive`; `verified` appears only if the estimate was pessimistic and the search actually completed.

### Node 7 — fairness (liveness only)

- First run the liveness property **without** fairness and read what came back. Three
  outcomes, not one: `verified` (no lasso exists — say that the result does not depend on
  fairness and go on), `inconclusive`/`unknown` (the budget stopped the run — fairness is
  not the question yet, node 9 is), or `violated` with a lasso, which is the case below.
- If the lasso is realistic → it is a counterexample; continue to node 11.
- If the lasso is an unfair scheduling artefact and the user can justify weak fairness from the operating environment → rerun with weak fairness; report both results as different results.
- If only strong fairness would exclude the lasso → **exit** for that property with `not-executed`, reason "strong fairness unsupported"; offer the manual check described in `fairness.md` §5.
- Never add fairness whose only justification is that the counterexample goes away.

### Node 8 — reductions

The engine has no symmetry or abstraction, and its partial-order reduction (`mcd check --por`, or `por: true` in `mc_check`) is opt-in and covers the safety search only: `deadlock`, `assert`, `invariant`, `reach`, depth-first. It refuses, and says why in `search.reduction`, any model with a rendezvous channel, a channel named by a value, `timeout` or `provided`, and any run that checks an `ltl`, `progress` or `ctl` property — so it can never be applied to a formula with `X`, nor to priorities. It does cover atomic sequences and `run` with the process table (`_nr_pr`: a model that reads it is reduced, not refused; the step that creates a process and the end of every process stay unreduced, because they write the table): a ring of processes started by `init { atomic { run ... } }` is its best case, with a pipeline of producer, filter and consumer over buffered channels. Use it when a safety check ends `inconclusive` on a state budget and the processes mostly work on their own data or pass messages along buffered channels. Report the run as reduced, quote `reduced_states` and `fully_expanded_states`, and do not compare its state count with the unreduced one, with SPIN's `pan -c0`, or with a bound you gave: the count is that of the reduced graph. The verdicts are the ones to carry over. Without `--por`, record that the search is unreduced.

Parallelism is not a reduction, and it is the other opt-in for a safety check that is too slow or ends `inconclusive` on a state budget: `--workers N` (`workers` in `mc_check`) searches the same graph with N workers, breadth-first. For a run that completes the verdicts and the counts are the sequential ones exactly, so unlike `--por` the counts can be compared with `pan -c0`; the report says `search.mode: "bfs"`, its `depth` is the number of breadth-first layers (an atomic sequence that runs through is one unit of it and of `--budget-depth`, and one that blocks part-way counts one unit per uninterrupted run, where `--bfs` counts every step, so on a model with atomic sequences the two can differ; a refused run is the sequential one and counts transitions) and its counterexamples are shortest ones in layers. Try it when the state graph is wide (the run's `search.parallel.layers` and `max_layer_states` say how wide: thousands of states in an average layer is the target); do not try it on a single long chain or counter, on a model of under about 10^5 states, or to find a bug fast (a violation is found only after every layer above it is complete; hunt with the default depth-first search or `--por`). It is refused, and the run is the sequential one, with an `ltl`, `progress` or `ctl` property in the run, and where `--por` applies. Report the run as parallel and quote `search.parallel`.

### Node 9 — size estimate

Run `mc_estimate` and record states, transitions, growth, and the projected fit into the budget. If the projection exceeds the budget, go back to node 6.

**Before every run that searches (nodes 9 and 11), look at the machine.** Run `sh assets/wait-for-capacity.sh --need-cores N --need-mem-mb M --dir <session dir> --min-disk-mb 500` with N the `workers` you will pass (1 for the default search) and M the `--budget-mem-mb` (or `--max-memory-mb` of the server). It waits while CPU or memory use is above 90%, or the run would push it above 90%, and exits 3 if the machine is still busy when its timeout ends. On exit 3 do one of three things, in this order: wait and look again; lower `workers`, `--budget-mem-mb` and the other budgets so the run fits; or report to the user that the run was not started and why. Never launch into a saturated machine: starting there slows every run, can push the machine into swap, and makes a time budget stop the search early (`inconclusive`, evidence `unknown`). Look again before each rerun with a larger budget, since the load changes. Do not take speed or timing measurements while the machine is busy. Exit 4 means the script cannot measure this platform: read the load, free memory and free disk by hand.

### Node 10 — artefacts

Before running: model, property list with IDs, mapping table, intake card, and the budget.

`mc_manifest` takes **only** `session_id`. It renders what the session recorded by itself:
the tool calls in order with their applied parameters (search, fairness, budget, seed,
steps, time limit), the artefacts each call wrote, engine version and hashes. It does
**not** receive or store the intake card, the mapping table, or the expression of a state
property — the report keeps a property's `text`, its `kind` and, for temporal ones,
`temporal.formula`, but not the `expr` JSON the check was given. So, for FR-012/NFR-002:

- put the exact expression into the property's `text` field, so the report carries it;
- save the `mc_check` request itself (the property list as sent) beside the report, and
  attach it to the report with the intake card and the mapping table;
- quote the manifest for versions, parameters and hashes — the things it does hold.

### Node 11 — staged run (11 §10)

1. `mc_parse` — syntax and type check.
2. `mc_simulate` — short random simulation with a fixed seed; sanity: the interesting states are reached.
3. `mc_check` on the sanity/reachability properties.
4. `mc_check` on safety and deadlock.
5. `mc_check` on liveness without fairness, then with weak fairness if node 7 allowed it.
6. Small pilot budget → target budget → stepwise increase on `inconclusive` (FR-024). A budget stop is `inconclusive` with the exhausted resource named, never `verified`.
7. Sensitivity runs (queue sizes, process count, loss/duplication switches) when the user wants robustness, each as its own result.

### Node 12 — interpretation

- `verified` only with `exhaustive` evidence (`evidence-and-status.md` §4; the experimental-LTL exception ended with the G4 oracle and is no longer written).
- `violated` only after `mc_explain` decoded the trace and you classified the cause. Say *decoded*; the word *replayed* belongs to a run of `mc_simulate` in `guided` mode over those steps (`counterexamples.md` §5), and a verdict that carries no run at all — an unreachable `reach`, a CTL verdict with `witness_note` — has nothing to pass to `mc_explain`.
- Everything else: `inconclusive` / `unknown` with the reason, what was covered, what was not, and the next minimal step (bigger budget, smaller model, different fairness, different logic).

## 4. Exit summary

| Exit | Where | Status | What the report says |
|---|---|---|---|
| Missing contract | node 2 | (ask) | questions, no result |
| Unbounded, no bound accepted | node 3 | `not-executed` | route to symbolic / abstraction methods |
| Timed requirement | node 4 | `not-executed` | route to timed model checking |
| Probabilistic requirement | node 4 | `not-executed` | route to probabilistic model checking |
| Construct outside subset | node 2/11.1 | `not-executed` | construct and line named; rewrite offered |
| Strong fairness required | node 7 | `not-executed` (that property) | manual lasso check offered |
| Budget exhausted after steps | node 11.6 | `inconclusive` | resource, coverage, next step |
| Engine reports overflow | node 11 | `invalid-model` | model defect, fix before any claim |
| Server or tool error | any | (no property status) | tool error separated from result (NFR-007); retry or CLI fallback; a message that says `internal error` or begins `internal:` is a defect of mcd, so neither helps: no verdict exists, keep the model and the call for a bug report |

## 5. Heuristics that recur in the sources (10 §12)

- Property first, then model: the state holds what the property observes and what changes enabledness.
- Small finite domains are a diagnostic tool; carrying the result to real sizes needs an argument.
- A nondeterministic environment beats one "typical" scenario.
- Start with safety and short errors; liveness and fairness after the model is debugged.
- Avoid `X` unless the exact step is part of the requirement.
- Replay every witness. Keep completeness as data, not as an adjective.
- After any model change, rerun all properties.
