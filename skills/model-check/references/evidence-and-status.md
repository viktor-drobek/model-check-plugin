# Status, evidence, and what you may say (FR-014, FR-016, NFR-001)

Sources: `model-check-skill-notes/11-skill-requirements.md` §1.1 (five kinds of
positive result), §11 (six statuses), §12 (`inconclusive` vs `unknown`), §14
(report order; the forbidden phrase), §9 (`not-executed` instead of a simulated
result), §10 (budget exhaustion → `inconclusive`), FR-013, FR-014, FR-016, NFR-001,
AC-01, AC-02, AC-11, AC-12, AC-15; `model-check-skill-notes/10-cross-book-synthesis.md`
§2 (the contract of every answer), §3.6 (classified outcomes), §14 (red lines);
`model-check-skill-notes/14-skill-building-plan.md` §4.1 (overflow →
`invalid-model`), §6 (one vocabulary for all tools; evidence levels; statistical
not produced), §11 (LTL evidence `unknown` while experimental), §12 A4 (size
bounds published here); `model-check-skill-notes/02-design-and-validation-of-computer-protocols.md`
гл. 11, 14 ("no errors" holds only for the chosen options and bounds).

## 1. The vocabulary

Status vocabulary: `verified`, `violated`, `inconclusive`, `unknown`, `not-executed`, `invalid-model`

Evidence levels: `exhaustive`, `bounded`, `approximate`, `unknown`

Every property gets exactly one status and exactly one evidence level, from the
engine, unchanged (FR-014). Anything you derive beyond that — a reading under an
assumption the engine did not check — is a sentence in the report, never a status. The same words are used by all tools and in the report
(plan §6). Statistical or numerical evidence is not produced by the engine — it
checks no probabilistic models — so the fifth kind of positive result in 11 §1.1
(a statistical or numerical estimate) never appears; if a user asks for a
probability, the route is `not-executed` (see `model-classification.md`).

## 2. Decision procedure for the status

The six statuses partition the outcomes. Answer the questions in order; the first
"yes" decides. The engine is expected to apply the same order (to be confirmed
against the result schema in G2); if its word and your reading disagree, report the
engine's word and the disagreement.

| # | Question | Status |
|---|---|---|
| 1 | Did nothing run for this property? (construct outside the subset, unsupported semantics or capability, no engine binary, user declined, missing input) | `not-executed` |
| 2 | Did the run stop on a defect of the model itself? (domain overflow, capacity overflow, blocking inside `d_step`) | `invalid-model` |
| 3 | Did the run find a concrete violation (a bad state, a deadlock, an accepting cycle, a non-progress cycle, a failing CTL state) and was it replayed? | `violated` |
| 4 | Did the run complete the search without finding a violation? | `verified` |
| 5 | Did the run stop before completing (budget exhausted: time, states, depth, memory) or was the search incomplete by construction (bounded, approximate)? | `inconclusive` |
| 6 | Otherwise: the outcome cannot be read even as partial coverage (semantics ambiguous, result not interpretable) | `unknown` |

Notes on exclusivity:

- 1 comes before everything: a property that never ran has no other status.
- 2 comes before 3: a state after an overflow is not a state of the intended model,
  so a violation found *after* it would be meaningless. The engine stops at the
  first finding in its deterministic order; after you fix the model, rerun, and a
  violation may then appear.
- 3 before 4 and 5: a concrete, replayed counterexample is a counterexample even
  in an incomplete search (02 гл. 11: an incomplete search does not create false
  errors).
- 4 requires **completion**; 5 covers every stop before completion. A search that
  hit the depth bound and found nothing is 5, not 4 (11 §10; AC-11).
- 6 is the remainder (11 §12): use it when none of 1–5 applies — a solver-style
  "don't know", an ambiguous semantics the engine flags, an experimental mode whose
  result the engine refuses to classify.

## 3. Evidence levels

| Level | Meaning | Produced when |
|---|---|---|
| `exhaustive` | the whole reachable state space (of the model × property automaton) was explored with an exact visited set | the search completed within budget, no reductions that lose states |
| `bounded` | the search covered everything up to an explicit bound (depth, steps, states) and nothing beyond | the run was configured with a bound, or a budget cut it off at a point the engine can name (depth d, N states) |
| `approximate` | the visited set may have lost states (hash collisions); coverage is a probability, not a fact | bitstate / hash-compact modes (vNext only) |
| `unknown` | the engine does not vouch for coverage | `not-executed`, `unknown`, experimental LTL (plan §11), a stop whose coverage the engine cannot name (server-side interruption) |

## 4. Status × evidence: what each combination allows you to say

| Status | Evidence | You may say | You may not say |
|---|---|---|---|
| `verified` | `exhaustive` | "Property P holds on model M under assumptions A; the search was exhaustive (N states)." (AC-01) | "the system is correct"; anything about the implementation |
| `verified` | `unknown` (experimental LTL) | "The search completed and found no counterexample; the LTL translation is experimental and the engine does not vouch for this result. Treat it as not established." | "holds", "proved" |
| `verified` | `bounded`, `approximate` | not produced: an incomplete search without a violation is `inconclusive` (§2, row 5) | — |
| `violated` | any | "P is violated on M; counterexample replayed: … (prefix …, loop …)." | "the system has a bug" before the cause classification (`counterexamples.md` §3) |
| `inconclusive` | `bounded` | "No counterexample within bound k / within N states / depth d; beyond that nothing is known." (AC-02) | "holds up to k" as if it said something about beyond k; "no errors" |
| `inconclusive` | `approximate` | "No counterexample found in an approximate search with estimated coverage c." (AC-12) | "exhaustive", "proved" |
| `inconclusive` | `unknown` | "The run stopped (resource R exhausted) before coverage could be measured." | any coverage claim |
| `unknown` | `unknown` | "The engine could not classify the result: reason. Next step: …" (AC-15) | any of the other five statuses |
| `not-executed` | `unknown` | "Not checked: reason (construct X at line n / unsupported semantics / no engine). Model and properties are attached; route: …" | any result, including "likely fine" |
| `invalid-model` | any | "The model overflowed domain D at step s (trace attached); fix the model before any property claim." | "violated"; "the system overflows" |

## 5. Size bounds (plan A4 as fixed at control point K1)

The engine targets "small and medium" models. Plan 14 §12 A4, fixed after the Spike
(`model-check-plugin/steps/spike-confirmation.md`):

| Class | States | DFS depth | State vector | Time | Memory |
|---|---|---|---|---|---|
| small | ≤ 10⁵ | ≤ 10⁵ | — | 60 s | 1 GB |
| medium | ≤ 10⁶ | ≤ 10⁶ | ≤ 128 bytes | 60 s | 1 GB |

Depth is a budget of its own, separate from the state count. Spike measurements:
0.5–1.2× the speed of `pan`, 32 bytes per stored state; the overhead of
interpreting the IR was not measured and is checked in G0 (the plan allows up to
20×). Above the medium class `mc_estimate` warns before the run and a full run ends
`inconclusive`, not in silent waiting. Quote the numbers of the actual run from the
manifest, not this table.

## 6. Forbidden phrasings

These appear in reports and make the status unfounded. Do not use them, and rewrite
a user's phrasing when you quote it back.

| Do not write | Because | Write instead |
|---|---|---|
| "no errors", "the model has no errors", "ошибок нет" | 11 §14 forbids it outright; it claims completeness that only `exhaustive` supports and hides which properties were checked | "no counterexample was found for properties P1–P3 in an exhaustive search" / "…in a bounded search up to k" |
| "proved", "proven", "доказано", "verified" as a plain adjective | only `verified` + `exhaustive` supports it, and only about the model | "P holds on model M under assumptions A (exhaustive search)" |
| "the system is correct", "the protocol is safe", "the implementation is verified" | the result is about the model; transfer needs a conformance argument (11 §1.3; 10 §12) | "on the model M, …; carrying this to the implementation requires …" |
| "holds up to k" implying beyond k | AC-02, AC-07 | "no counterexample of length ≤ k; nothing is claimed beyond k" |
| "the search timed out but found nothing, so it is probably fine" | 11 §10: budget exhaustion is `inconclusive`, never a hint of `verified` | "inconclusive: time budget exhausted after N states; coverage unknown" |
| "with fairness the property holds" without the result without fairness | AC-05; 03 гл. 6: fairness can prove by forbidding | both results, each with its assumption |
| "the trace shows a bug in the system" before the cause classification | 11 §11: classify system / specification / model / translation first | "the trace violates P on the model; cause class: …" |
| "the property is true" when the antecedent is unreachable | vacuity (AC-13) | "P holds vacuously: `req` is unreachable; the response guarantee is empty" |
| "simulation shows it works" | 02 гл. 12: one trace proves nothing | "simulation reached the expected states (sanity check); no property result" |
| "bitstate/approximate search found no errors, so exhaustive" | AC-12 | "approximate search, coverage estimate c, status `inconclusive`" |
| "the model was verified by SPIN/NuSMV" | the engine is the backend; SPIN is only the test oracle | "checked by the built-in engine version v (manifest attached)" |
| "statistically", "with probability" about a result | the engine produces no statistical evidence | remove, or route the question to a probabilistic tool as `not-executed` |

## 7. Allowed phrasings (11 §14)

- "Property P1 holds on the finite model M under the stated assumptions; the search
  was exhaustive (N states, depth d)."
- "No counterexample was found in a bounded search up to k = …; the result is
  inconclusive beyond that bound."
- "P2 is violated; the counterexample (prefix of n steps, loop of m steps) was
  replayed; cause class: model defect (channel capacity 2 admits a reordering the
  real link cannot produce)."
- "P3 was not executed: the model uses `unless` (line 12), which is outside the
  engine's subset; a rewrite is proposed below."
