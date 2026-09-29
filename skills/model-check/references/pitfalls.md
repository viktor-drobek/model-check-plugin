# Pitfalls: the anti-patterns of 11 §15 on corpus models

Sources: `model-check-skill-notes/11-skill-requirements.md` §15 (seventeen
anti-patterns; each warning must name the conclusion that loses its basis), §5,
§10, §11, §14; `model-check-skill-notes/10-cross-book-synthesis.md` §12
(heuristics), §13 (conflicts), §14 (red lines); `model-check-skill-notes/14-skill-building-plan.md`
§2.1 (corpus rows), §4 (engine scope), §8.1 (mutants); corpus `Promela - examples/`.

Each entry: what the anti-pattern looks like, a corpus model where it can be shown,
and **which conclusion loses its basis** (11 §15 requires the warning to say so).
Where the engine's scope makes an anti-pattern impossible to commit here, the entry
says so and names the route instead. Entries without a natural corpus model say
"no corpus model".

## Property-side anti-patterns

### 1. Wrong polarity: checking the formula instead of its negation

Look: a hand-written `never` claim that accepts the *good* traces; an LTL formula
passed with an extra or missing `!`. Corpus: `CH4/prop.pml` contains both a claim
for `[]p` and one for `![]p` under `#ifdef PHI`; `CH12/leader.ltl` shows the SPIN
convention — the claim is the *negated* formula. Loses its basis: the verdict as a
whole; `verified` and `violated` swap meaning. Guard: paraphrase the claim by hand and
say which of the two it encodes — `mc_lint_property` does not read claims and has no
polarity note (`properties-ltl-ctl.md` §5); then paraphrase the accepted language in the
report (`properties-ltl-ctl.md` §2).

### 2. Properties over engine phases instead of source events

Look: an atom refers to an internal step (a `printf`, a helper variable's transient
value, an intermediate location inside `atomic`) rather than to the event the
requirement names. Corpus: `CH14/version1` prints MSC lines with `printf`; the
engine ignores `printf`, so a property "phrased on the printout" observes nothing —
put atoms on labels (`switch@Busy`) and variables. Loses its basis: the mapping
between requirement and formula; the result is about a different observation.
Guard: the atom dictionary in every property record (11 §8); AC-16.

### 3. Unreachable antecedent and vacuous liveness

Look: `[](req -> <> ack)` where `req` never becomes true — through a misspelt label,
a wrong process index, an environment that never produces the request, or fairness
that excludes the requesting paths. Corpus illustration: take `CH14/version1` and
restrict the subscriber so that it never sends `digits`; then `switch@Wait` is
unreachable and `[](switch@Wait -> <> switch@Busy)` holds vacuously. (A misspelt
label is a different failure: `mc_lint_property` reports an undefined atom and
nothing runs.) Loses its basis:
`verified` for the response property; nothing was guaranteed. Guard: the sanity
property `<> req` (or `reach`) next to every implication; `mc_lint_property`
vacuity candidates; AC-13.

### 4. LTL used for a branching requirement, or CTL for a linear one

Look: "from every state recovery is possible" written as `[]<> reset`; "eventually
stable" written as `AF AG stable`. Corpus: `App_A/example` and `CH12/leader.ltl`
check persistence `<>[]p`, which has no CTL equivalent; a CTL `AF AG p` would be a
stronger claim. Loses its basis: the correspondence between the user's requirement
and the checked formula (FR-007; AC-17). Guard: the divergence table in
`properties-ltl-ctl.md` §3; never rewrite across logics to fit a tool.

### 5. `X` where the atomic step is not part of the requirement

Look: `[](req -> X ack)` in an interleaving model, where "next" is one statement
of one process. Corpus: any two-process model — in `CH2/peterson.pml` the "next"
step after a process raises its flag may belong to the other process, so `X` tests
scheduling, not the algorithm. Loses its basis: the truth value changes with the granularity of
the model and with any stutter-preserving reduction; the formula is not
stutter-invariant. Guard: `mc_lint_property` `X`-free flag; 10 §5; AC-06.

## Model-side anti-patterns

### 6. `atomic`/`d_step` hiding interleavings the property observes

Look: a check-then-set sequence wrapped in `atomic` "because it is one line in the
source". Corpus: `CH2/mutex_flaw.pml` is violated because the reads and writes of
`x`, `y`, `z` interleave; wrapping `L1`–`L6` in `atomic` would make the flawed
algorithm `verified` — and the verdict would be about a different algorithm.
Contrast `App_C/petrinet1`, where `atomic` is *correct*: a Petri transition fires
indivisibly by definition. Loses its basis: `verified` for any property that can
observe the hidden intermediate states. Guard: for every `atomic`, one sentence on
why the real system is indivisible there (`promela-subset.md` §5).

### 7. An unbounded object quietly cut to a small bound

Look: a queue of "any length" modelled as `[2]`, a counter as `byte`, "N clients" as
`N = 2`, without the bound appearing in the report. Corpus: `CH15/client_server.pml`
(`N`, `M`), `App_C/ex1` (a `byte` counter that grows forever — here the engine
reports `invalid-model` on overflow rather than silently wrapping). Loses its basis:
transfer of `verified` from the bounded model to the unbounded system (AC-07).
Guard: every bound is a visible parameter of the result; sensitivity runs (FR-024).

### 8. Deadlock hidden by an artificial self-loop or a careless `end` label

Look: `do :: skip od` appended "so the process never terminates", or `end:` placed
on a location that is not a legitimate final state. Corpus: `CH4/dijkstra_progress.pml`
and `CH5/pathfinder.pml` use `end:` deliberately on their main loops;
on `CH2/prodcons.pml` (producer/consumer) remove or move an `end` label and compare
the deadlock verdicts. Loses its basis: `verified` for `deadlock` (a real stop is
disguised as activity or as a valid end). Guard: `end` labels justified one by one;
compare with and without.

### 9. Synchronous translation of an asynchronous system without a scheduler variable

Look: an interleaving system encoded as a synchronous round where every component
steps at once. No corpus model — the engine's IR is interleaving by construction,
so this can only happen when someone encodes a round structure by hand
(`model-classification.md` §3). Loses its basis: every result; the model admits
impossible simultaneities and forbids real orderings (09 гл. 3). Guard: an explicit
scheduler/turn variable and a documented atomic step.

### 10. Incomplete `next`: a variable that freezes or changes at random

Look: SMV-style `next(v)` with missing cases. No corpus model — a Promela variable
keeps its value unless assigned, so the engine cannot produce this defect; it
belongs to the vNext SMV export. Loses its basis: every property over `v`. Guard:
in the IR every transition's effect is explicit (11 §7.1).

### 11. Abstraction without a relation, a direction, and a spurious-trace check

Look: "we left out the data, it does not matter" with no statement of whether the
model over- or under-approximates and which properties survive. Corpus:
`CH5/diskhead.pml`, `CH5/sink_source_filter.pml`, `CH5/counter.pml` are deliberate
abstractions — the report for any of them must state the direction (FR-018).
Loses its basis: transfer of `verified` (needs over-approximation) or of `violated`
(needs the trace to be feasible) to the system (AC-08). Guard:
`counterexamples.md` §3 question 2; the loss list for Petri translations.

### 12. Ordinary LTL for a timed or probabilistic requirement

Look: "within 5 seconds" encoded as "within 5 steps"; "rarely" encoded as a
nondeterministic branch and then reported as a probability. Corpus:
`CH15/client_server.pml` redefines `timeout` as `(_nr_pr <= N+M)` — Promela's
`timeout` is a condition on enabledness, not a clock (10 §13). Loses its basis: the
result answers a different question than the requirement (11 §5; AC-09, AC-10).
Guard: `model-classification.md` §2 → `not-executed` with a route.

## Search-side anti-patterns

### 13. A budget stop, a bound, or an approximate search passed off as a proof

Look: "the search ran out of memory after 10⁷ states and found nothing — looks
fine"; "bitstate found no errors"; (for other tools) "UNSAT at k = 20". Corpus:
`App_C/ex1` with `int` instead of `byte` is the plan's budget bomb — the correct
outcome is `inconclusive` with the exhausted resource named (plan §8.1). Loses its
basis: `verified`; coverage was not achieved (AC-02, AC-11, AC-12). Guard: §2 of
`evidence-and-status.md` — completion is required for `verified`.

### 14. Partial-order reduction without stutter-invariance, or under priorities

Look: reduction switched on for a formula with `X`, or for a model with
`provided`. Corpus: `CH5/pathfinder.pml` — its header says POR cannot be used
because `provided` models priorities; `CH12/leader.ltl` carries SPIN's warning
that the claim must be stutter-closed. The current engine has no POR, so today
this cannot be committed; when vNext adds it, the engine refuses it for `X`
formulas and priorities (FR-017). Loses its basis: `verified` for liveness and
for `X`-properties (missed interleavings). Guard: the `X`-free flag; AC-06.

### 15. Comparing results obtained under different fairness or atomicity

Look: "tool A says `violated`, our run says `verified`" when one run had weak
fairness or a coarser `atomic`. Corpus: `CH2/peterson.pml` vs `CH2/peterson2.pml`,
and any `CH4/fair*.pml` run with and without `fairness: weak`. Loses its basis: the
comparison itself; the two results are about different sets of paths or different
models. Guard: the manifest (fairness, model hash, options) attached to every
result; 11 §15.

### 16. Fairness added only to make a counterexample disappear

Look: a liveness property fails, `fairness: weak` is switched on, it passes, the
report shows only the second run. Corpus: `CH8/fairness.pml` (a cycle that weak
fairness does *not* remove — a useful surprise), `CH3/alternatingbit.pml` (eval E2:
the fairness question must precede the run). Loses its basis: `verified` for the
liveness property; the excluded paths may be real (03 гл. 6; AC-05). Guard:
`fairness.md` §3 — justification from the environment, both results reported.

## Report-side anti-patterns

### 17. A counterexample without version, command and mapping

Look: a list of steps, or a raw trail, with no model hash, engine version, options,
or source names. Corpus: `CH14/version3.trail` and `CH14/version6.trail` are bare
`depth:process:transition` triples — meaningless without `version3`, the SPIN
version and the mapping. Loses its basis: reproducibility and therefore `violated`
itself (11 §11 requires replay). Guard: `mc_explain` + `mc_manifest` in every
report (FR-012, FR-015).

### 18. Transferring the model's result to the implementation

Look: "the protocol is verified" after checking a hand-written model. Corpus:
`CH10/fahr.c` → `CH10/fahr.pml` is an *extracted* model; even there the verdict is
about `fahr.pml` and the extraction, not about the C program. Loses its basis: any
claim about the system (11 §1.3; 10 §12 "verification is not validation"). Guard:
the Limitations section names the missing conformance argument; forbidden
phrasings in `evidence-and-status.md` §6.

### 19. BDD growth ignored (not applicable)

Look: peak BDD nodes exploding without trying variable reordering. No corpus model
and not applicable: the engine is explicit-state and has no BDDs. If a user
brings an SMV workflow, route it (`not-executed`) and mention the ordering issue
from 03 гл. 9. Loses its basis: nothing here; listed for completeness of 11 §15.

## Quick self-check before declaring `verified`

1. Evidence is `exhaustive`? (13)
2. Every implication's antecedent shown reachable? (3)
3. Every `atomic` justified? (6)
4. Every bound and capacity listed as a parameter? (7)
5. Fairness result reported together with the no-fairness result? (16)
6. Logic matches the requirement (LTL vs CTL)? (4)
7. Polarity of hand-written claims confirmed? (1)
8. Manifest attached? (17)
9. Wording says "on the model", not "the system"? (18)
