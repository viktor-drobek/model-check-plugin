# Status, evidence, and what you may say (FR-014, FR-016, NFR-001)

Sources: `model-check-skill-notes/11-skill-requirements.md` §1.1 (five kinds of
positive result), §11 (six statuses), §12 (`inconclusive` vs `unknown`), §14
(report order; the forbidden phrase), §9 (`not-executed` instead of a simulated
result), §10 (budget exhaustion → `inconclusive`), FR-013, FR-014, FR-016, NFR-001,
AC-01, AC-02, AC-11, AC-12, AC-15; `model-check-skill-notes/10-cross-book-synthesis.md`
§2 (the contract of every answer), §3.6 (classified outcomes), §14 (red lines);
`model-check-skill-notes/14-skill-building-plan.md` §4.1 (overflow →
`invalid-model`), §6 (one vocabulary for all tools; evidence levels; aggregation
priority; `bounded` vs `unknown`), §11 (LTL evidence `unknown` while experimental),
§12 A4 (size bounds published here); `model-check-skill-notes/02-design-and-validation-of-computer-protocols.md`
гл. 11, 14 ("no errors" holds only for the chosen options and bounds). Engine as
built: `model-check-plugin/engine/report/report.go` (rules enforced by `Build`),
`model-check-plugin/steps/g0-confirmation.md` §3.2 (measurements), §4 (decisions 3–4).

## 1. The vocabulary

Status vocabulary: `verified`, `violated`, `inconclusive`, `unknown`, `not-executed`, `invalid-model`

Evidence levels: `exhaustive`, `bounded`, `approximate`, `unknown`

Every property gets exactly one status and exactly one evidence level, from the
engine, unchanged (FR-014). The one case in which you write the status yourself is
when the engine produced no record for the property — nothing ran (§2, row 1: input
rejected with exit code 2, no binary, user declined): then the status is
`not-executed` with evidence `unknown`, and the report says the engine did not assign
it. Anything you derive beyond that — a reading under an
assumption the engine did not check — is a sentence in the report, never a status.
The same words are used by the CLI, by the MCP tools (G2, `mc_check`'s `status`
field) and in the report (plan §6). Statistical or numerical evidence is not produced by the
engine — it checks no probabilistic models — so the fifth kind of positive result
in 11 §1.1 (a statistical or numerical estimate) never appears; if a user asks for
a probability, the route is `not-executed` (see `model-classification.md`).

## 2. How a property gets its status

The six statuses partition the outcomes of one property. Answered for one property
in the order below, the questions reproduce the engine's verdicts (`engine/explore`,
`engine/report`; the per-property rules are the K1 rules and `report.Build` refuses a
document that breaks them). The engine itself decides in time order — the first
finding for a property wins and is never revised — which the questions encode by
asking about the property's own run, not about the run as a whole. When you
assign a status yourself — because nothing ran, or the input was rejected with
exit code 2 — use the same order; if the engine's word and your reading disagree,
report the engine's word and the disagreement.

| # | Question | Status | Evidence | Rule |
|---|---|---|---|---|
| 1 | Did nothing run for this property? A construct outside the subset gives `not-executed` (input rejected by the frontend with exit code 2); so does a capability the current build lacks — after G5 the only one left among property kinds is `fairness: strong` (FR-008), since `ltl`, `progress` and `ctl` all run — and so do: no engine binary, user declined, missing input | `not-executed` | `unknown` | `reason` names the construct or the capability; which of the two boundaries was hit is said in the report |
| 2 | Was the property still undecided when the run hit a defect of the model itself? A domain overflow gives `invalid-model`: a `byte` wrap, a place above its capacity, a division by zero or an out-of-range index in a guard or effect | `invalid-model` | `unknown` | the run to the offending step is attached as `counterexample`; every property still undecided gets this status, a property already decided keeps its verdict (g0-confirmation §4, decision 3) |
| 3 | Did the run find a concrete violation — a bad state, a hang, a failing `assert` — or, for `reach`, complete the search without any state satisfying the condition? | `violated` | `exhaustive` | `violated` carries evidence `exhaustive` always: the counterexample is an exact run of the model whatever cut the search afterwards; for `reach` because only a complete search establishes unreachability |
| 4 | Did the search complete (whole reachable graph expanded) without a violation? For `reach`: was a state satisfying the condition found? | `verified` | `exhaustive` | `verified` requires `complete` = true, except for `reach`, which is verified by a witness — the exact run attached as `witness`, so `complete` may be false |
| 5 | Did a budget stop the run first (states, depth, time, memory)? Budget exhaustion gives `inconclusive` with `reason` naming the exhausted resource | `inconclusive` | `bounded` for a **states** or **depth** stop, `unknown` for a **time** or **memory** stop (§3) | `complete` is false; with `bounded` the engine can name the bound that stopped it, and §3 says what that bound does and does not cover, with `unknown` not even the bound can be named |
| 6 | Otherwise: the outcome cannot be read even as partial coverage (semantics ambiguous, result not interpretable, an interruption without a bound) | `unknown` | `unknown` | in the vocabulary, but the G0 engine never emits it; it is reserved for tool errors without a bound (plan §6) and for experimental modes |

Notes on exclusivity:

- 1 comes before everything: a property that never ran has no other status.
- 2 comes before 3 and 4 for the properties still open: a state after an overflow
  is not a state of the intended model, so a violation found *after* it would be
  meaningless. Fix the model, rerun; a violation may then appear.
- 3 before 4 and 5: a concrete counterexample is a counterexample even in an
  incomplete search (02 гл. 11: an incomplete search does not create false errors).
- 4 requires **completion** (or, for `reach`, a witness); 5 covers every stop
  before completion. A search that hit the depth bound and found nothing is 5,
  not 4 (11 §10; AC-11). The depth budget is a budget of its own: states deeper
  than D are stored and counted but not expanded, and `reason` says how many.
- For `reach` write the status with its gloss — `verified` (reachable) / `violated` (unreachable): the bare word reads as "no violation found" / "a bad state was found", the opposite of what a reachability result can mean when the reached state is the defect scenario itself (E1 of the evals: "both users at L7" is `verified`, and that is the bad news).
- 6 is the remainder (11 §12); with the G0 engine it can only come from you, and
  only when neither a bound nor a construct can be named.

**Aggregation priority** (plan §6). Statuses are per property and the engine does
not aggregate them. If the caller needs one word for the whole run, the order is
fixed: Aggregation priority: `invalid-model` > `not-executed` > `violated` > `inconclusive` > `unknown` > `verified` — the first of these that occurs among the properties is the overall word, and the per-property records are still listed. Note that this differs from the per-property order above (rows 1–2): a run with one `not-executed` property and one `invalid-model` property is summarised as `invalid-model`. Plan §6 fixes the order without giving a reason; our reading of it: the model must be fixed before anything else is worth executing.

## 3. Evidence levels

| Level | Meaning | Produced when |
|---|---|---|
| `exhaustive` | the whole reachable state space (of the model × property automaton) was explored with an exact visited set, or an exact run decides the property | a complete search (`verified`); any `violated`; a `reach` witness |
| `bounded` | the search stopped at a bound the engine can name; the result is reported by naming that bound, which is not the same as being covered up to it — see the caveat below | a declared search bound cut the run: N stored states, depth D, or the process-instance pool (`inconclusive`) |
| `approximate` | the visited set may have lost states (hash collisions); coverage is a probability, not a fact | bitstate / hash-compact modes (vNext only) |
| `unknown` | the engine does not vouch for coverage | `not-executed`, `invalid-model`, `unknown`, and an `inconclusive` stopped by time or memory |

The rule that separates the last two (plan §6): `bounded` is used when the engine can
name the bound at which it stopped and the result is phrased relative to it;
`unknown` is used when it cannot — a tool error, an interruption without a bound, a
result that cannot be classified.

**Which budget stop gets which (as built in G4).** A **states** or **depth** stop is
`bounded`: the engine names the bound it stopped at, and the same run repeated stops
at the same place. A
**time** or **memory** stop is `unknown`: the engine knows how many states it had
stored when the clock or the estimate ran out, but that number is not a *bound* on
anything — it is an artefact of the machine and the load, it is not reproducible,
and no set of behaviours is described by it. G0 had extended `bounded` to those two
stops on the grounds that `counters.states` could be quoted; G4 reversed that, for
both the temporal and the safety search, and the rule above is the one the engine
now follows. Quoting `counters.states` next to an `unknown` is still useful — as a
measurement, not as coverage.

**What `bounded` does not mean.** It names the bound that stopped the search; it does
not say that everything below the bound was searched. Under a **depth** budget the DFS
stores a state it reaches deeper than D without expanding it (`reason`: "N state(s) at
depth > D were stored but not expanded"), and a state it has already stored is never
expanded a second time — so when a later, shorter path reaches that same state at a
depth below D, its successors still go unexplored. Paths of length ≤ D through such a
state are therefore missing from the search. It is not a bound in the other direction either: the
search evaluates a newly stored state's properties *before* it tests the depth limit, so a
property can be decided — `violated` with a real counterexample — at depth D+1. Under a
**states** budget the run simply stops at the N-th state, which is a point in the
traversal, not a horizon in the model.
Write "no counterexample was found before the run stopped at <bound>", never "everything
up to <bound> was checked". The one bound that does cover what it names is
`complete: true`: nothing was cut.

**LTL evidence.** Plan §11 asked for evidence `unknown` on LTL results "while the
translation is experimental". The differential oracle that ends that condition ran
in G4: 43 triples (engine, engine with SPIN's own claim, `pan`) agree on every
verdict, and the product state counts agree with `pan` where they are comparable —
not under `pan -f`, whose copies do not enter "stored", and not under the engine's own
automaton, where the count follows the automaton (`steps/g4-confirmation.md` §3.1–3.2). LTL results therefore
carry the ordinary evidence levels of this file — `exhaustive` for a completed
product search or any counterexample — and the sentence "the LTL translation is
experimental, treat this as not established" is no longer written. What still has to
be written for every temporal result is the formula that was actually checked and
the fairness setting it was checked under (`engine-tools.md` §7).

## 4. Status × evidence: what each combination allows you to say

| Status | Evidence | You may say | You may not say |
|---|---|---|---|
| `verified` | `exhaustive` | "Property P holds on model M under assumptions A; the search was exhaustive (N states)." (AC-01) | "the system is correct"; anything about the implementation |
| `verified` | `exhaustive`, for an `ltl` or `progress` property | "Formula φ holds on model M under fairness F; the product with the automaton for !φ was searched exhaustively (N states)." Name the formula and the fairness; a temporal verdict without them is unreadable | "holds" without the formula or the fairness; anything about the implementation |
| `verified` | `bounded`, `approximate` | not produced: an incomplete search without a violation is `inconclusive` (§2, row 5) | — |
| `violated` | `exhaustive` (the only combination the engine emits) | "P is violated on M; counterexample replayed: … (final state …)." | "the system has a bug" before the cause classification (`counterexamples.md` §3) |
| `inconclusive` | `bounded` | "No counterexample was found before the run stopped at N states / depth D; what lies beyond — and, for a depth stop, what lies below it through an unexpanded state — is not known." (AC-02) | "holds up to k" as if it said something about beyond k; "everything up to the bound was checked"; "no errors" |
| `inconclusive` | `approximate` | "No counterexample found in an approximate search with estimated coverage c." (AC-12) | "exhaustive", "proved" |
| `inconclusive` | `unknown` | produced since G4 by a **time** or **memory** budget stop (§3): "The run stopped after T ms with N states stored; how much of the state space that is, is not known." | "up to N states" as if N were a bound; any coverage claim |
| `unknown` | `unknown` | "The engine could not classify the result: reason. Next step: …" (AC-15) | any of the other five statuses |
| `not-executed` | `unknown` | "Not checked: reason (construct X at line n / strong fairness not implemented, FR-008 / no engine binary / user declined). Model and properties are attached; route: …" | any result, including "likely fine"; for strong fairness, the weak-fairness verdict presented as if it answered the question |
| `invalid-model` | `unknown` | "The model overflowed domain D at step s (trace attached); fix the model before any property claim." | "violated"; "the system overflows" |

## 5. Size bounds (plan §12 A4 as fixed at control point K1, measured in G0)

The engine targets "small and medium" models. The CLI defaults are the medium
bounds; smaller budgets are set with the `--budget-*` flags (`engine-tools.md` §2).

| Class | States | DFS depth | State vector | Time | Memory |
|---|---|---|---|---|---|
| small | ≤ 10⁵ | ≤ 10⁵ | ≤ 128 bytes | 60 s | 1 GB |
| medium | ≤ 10⁶ | ≤ 10⁶ | ≤ 128 bytes | 60 s | 1 GB |

Depth is a budget of its own, separate from the state count. G0 measurements
(`steps/g0-confirmation.md` §3.2, `counters` model, linux/arm64): 10⁶ states in
1.1–1.6 s of DFS with a peak RSS of 0.17 GB, 0.65–0.90 M states/s; the interpretation
overhead the plan allowed (≤ 20× the Spike) came out below 1× on that model, with
the caveat that its guards are trivial. Above the medium class a full run ends
`inconclusive` with the resource named, not in silent waiting; `mc_estimate` (G5)
extrapolates the state count and the vector width from a short run and warns before the
run — it does not estimate memory per state (`steps/g5-confirmation.md` §7), so its
warning is a signal about size, not a prediction of the memory budget. The table is a target, not a guarantee:
the G0 numbers come from a model with trivial guards, and a temporal product or a CTL
labelling over the same state count costs more. Quote the numbers of the actual run from the report's
`counters`, not this table.

## 6. Forbidden phrasings

These appear in reports and make the status unfounded. Do not use them, and rewrite
a user's phrasing when you quote it back.

| Do not write | Because | Write instead |
|---|---|---|
| "no errors", "the model has no errors", "ошибок нет" | 11 §14 forbids it outright; it claims completeness that only `exhaustive` supports and hides which properties were checked | "no counterexample was found for properties P1–P3 in an exhaustive search" / "…before the bounded run stopped at N stored states" |
| "proved", "proven", "доказано", "verified" as a plain adjective | only `verified` + `exhaustive` supports it, and only about the model | "P holds on model M under assumptions A (exhaustive search)" |
| "the system is correct", "the protocol is safe", "the implementation is verified" | the result is about the model; transfer needs a conformance argument (11 §1.3; 10 §12) | "on the model M, …; carrying this to the implementation requires …" |
| "holds up to k" implying beyond k | AC-02, AC-07 | "no counterexample was found before the run stopped at k; nothing is claimed beyond k, and §3 says what a bound does not cover below it" |
| "everything up to depth D was checked", "all states within the budget were explored" | a depth-bounded DFS leaves states stored but not expanded, and never re-expands a state a shorter path reaches later (§3) | "the run stopped at depth D with N state(s) stored but not expanded; paths through them were not followed" |
| "the search timed out but found nothing, so it is probably fine" | 11 §10: budget exhaustion is `inconclusive`, never a hint of `verified` | "inconclusive: time budget exhausted after N states; nothing is known beyond them" |
| "with fairness the property holds" without the result without fairness | AC-05; 03 гл. 6: fairness can prove by forbidding | both results, each with its assumption |
| "the property holds under weak fairness" when the run without fairness already returned `verified` | it presents an assumption as a premise of a result that does not need it (`fairness.md` §6a) | "the result does not depend on the fairness setting; it holds with `none` and with `weak`" |
| a temporal verdict without the formula and the fairness it was checked under | the status alone does not say which of several readings was checked | "φ = `…` under fairness `none`: `verified`, evidence `exhaustive` (N states)" |
| "the model has a non-progress cycle" for a model with no `progress` label | every cycle is then non-progress; the run found no defect (`properties-ltl-ctl.md` §7) | "the model carries no progress labels, so this search cannot say anything yet; here is where progress would be" |
| "the trace shows a bug in the system" before the cause classification | 11 §11: classify system / specification / model / translation first | "the trace violates P on the model; cause class: …" |
| "the property is true" when the antecedent is unreachable | vacuity (AC-13) | "P holds vacuously: `req` is unreachable; the response guarantee is empty" |
| "simulation shows it works" | 02 гл. 12: one trace proves nothing | "simulation reached the expected states (sanity check); no property result" |
| "bitstate/approximate search found no errors, so exhaustive" | AC-12 | "approximate search, coverage estimate c, status `inconclusive`" |
| "the model was verified by SPIN/NuSMV" | the engine is the backend; SPIN is only the test oracle | "checked by the built-in engine `mcd` version v (report's `engine` and `inputs` sections attached)" |
| "statistically", "with probability" about a result | the engine produces no statistical evidence | remove, or route the question to a probabilistic tool as `not-executed` |

## 7. Allowed phrasings (11 §14)

- "Property P1 holds on the finite model M under the stated assumptions; the search
  was exhaustive (N states, depth d)."
- "No counterexample was found in a bounded search up to N states; the result is
  inconclusive beyond that bound."
- "P2 is violated; the counterexample (n steps, final state …) was replayed; cause
  class: model defect (channel capacity 2 admits a reordering the real link cannot
  produce)."
- "P3 was not executed: the model uses `unless` (line 12), which is outside the
  engine's subset; a rewrite is proposed below."
