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
- Bounded assurance is enough → continue with a declared states or depth budget. Do not promise the outcome in advance: a run under a budget still returns `violated` / `exhaustive` when it finds a counterexample, and `verified` / `exhaustive` for a `reach` whose witness it found, because one run decides both. `inconclusive` / `bounded` is what you get only if nothing was decided before the budget stopped it.
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

The engine's MVP/v1 has no partial-order reduction, symmetry or abstraction; there is nothing to enable. Record in the report that the search is unreduced. When POR appears (vNext) it applies only to `X`-free properties and never to models with priorities (`provided`).

### Node 9 — size estimate

Run `mc_estimate` and record states, transitions, growth, and the projected fit into the budget. If the projection exceeds the budget, go back to node 6.

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
| Server or tool error | any | (no property status) | tool error separated from result (NFR-007); retry or CLI fallback |

## 5. Heuristics that recur in the sources (10 §12)

- Property first, then model: the state holds what the property observes and what changes enabledness.
- Small finite domains are a diagnostic tool; carrying the result to real sizes needs an argument.
- A nondeterministic environment beats one "typical" scenario.
- Start with safety and short errors; liveness and fairness after the model is debugged.
- Avoid `X` unless the exact step is part of the requirement.
- Replay every witness. Keep completeness as data, not as an adjective.
- After any model change, rerun all properties.
