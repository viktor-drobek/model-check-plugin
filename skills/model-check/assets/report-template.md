# Report template (section order fixed by 11 §14)

Source: `model-check-skill-notes/11-skill-requirements.md` §14 (order of sections; the
phrase "no errors" is forbidden), §10 (manifest), §11 (`violated` obligations).
Write the report in the user's language; keep the section order; keep status and
evidence tokens exactly as the engine returns them. Delete the italic hints.

---

# Model check: <system name>

## 1. Summary

*One sentence per property: status + evidence level.* Example: "P1 (mutual
exclusion): `violated`, evidence `exhaustive`, counterexample of 11 steps. P2 (no
deadlock): `verified`, evidence `exhaustive`, 128 states."

## 2. Scope and assumptions

- System boundary: *what is inside the model; what is environment; what is excluded*
- Transition semantics: *interleaving / rendezvous / buffered channels; atomic step*
- Fairness: *`none` / `weak`; which infinite paths are excluded and why that is admissible*
- Bounds and capacities: *every domain, queue size, process count that limits the model*
- Relation to the implementation: *hand-written model / extracted / built from prose; what is not claimed*

## 3. Model

- State variables and domains
- Processes / places and transitions (one line each)
- Initial state(s)
- Mapping table: *source name → model name* (attach or link)
- Parser warnings that matter

## 4. Properties

| ID | User's wording | Class | Logic | Formula | Atoms | Vacuity check |
|---|---|---|---|---|---|---|
| P1 | | safety / reachability / liveness | invariant / LTL / CTL / deadlock / progress | | | *reachable antecedent? constant atoms?* |

## 5. Method and backend

- Engine: `mcd` version *v* (built-in explicit-state engine; MCP or CLI)
- Search: DFS / BFS; fairness mode; reductions: *none (unreduced search)*
- Why this method fits the model class (one or two sentences)

## 6. Execution

- Manifest: *path from `mc_manifest`*
- Budget: time / states / depth / memory
- Metrics per run: states, transitions, depth, elapsed, and `memory_bytes_est` — the
  engine's estimate of the state table, not a peak RSS; call it an estimate
- Staging: parse → simulate (seed) → estimate → check (pilot budget → target budget)

## 7. Result

| ID | Status | Evidence | Counters | Note |
|---|---|---|---|---|
| P1 | | | | |

*Use only the six statuses and four evidence levels. For `inconclusive`: the exhausted
resource and what was covered. For `not-executed`: the construct or capability.
For `invalid-model`: the overflowed domain and step.*

## 8. Counterexample

*Per `violated` property **that carries a run** (an unreachable `reach` and some CTL
verdicts do not — quote their `reason` or `temporal.witness_note` instead):*
one-sentence summary; chronology in source names
(step, process, statement, changed variables, sends/receives); the first causal
fork; for lassos the loop as a separate block with the unmet obligation; cause
class (system / model / property / fairness-environment artefact) with the
reasoning; whether one trace was enough (CTL).

## 9. Limitations

*What is not established: what a `bounded` run did **not** cover (`evidence-and-status.md`
§3), unreduced search, fairness assumptions, the `enabled(t)` proxy when a Petri liveness
question was asked in LTL, abstraction losses (Petri/CPN loss list), constructs not
executed, platforms the binary was not run on, and the missing conformance argument for
the implementation.*

## 10. Next actions

*Prioritised: fix the model / refine the property / raise the budget / add a
justified fairness assumption and rerun / route to a specialised tool / rerun all
properties after any change.*

## 11. Artifacts

*Relative paths in the session directory: IR, properties, mapping, traces,
statistics, manifest, this report. Nothing outside the session directory unless the
user agreed.*
