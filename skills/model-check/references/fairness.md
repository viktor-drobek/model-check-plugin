# Fairness (FR-008): what it is, what the engine does, what to say

Sources: `model-check-skill-notes/05-principles-of-model-checking.md` гл. 3
(unconditional/strong/weak fairness, realizability, fair satisfaction), гл. 5
(fairness as LTL premise), гл. 6 (fairness in CTL is a change of quantifier
domain); `model-check-skill-notes/07-lectures-01-09.md` лекция 6 (check without
fairness first; SPIN fairness covers process choice, not every choice);
`model-check-skill-notes/08-modelchk.md` гл. 2, 3 (fairness sets, fair SCC, fair
value zero when no fair path); `model-check-skill-notes/03-karpov-model-checking.md`
гл. 6 (fairness that "proves" by forbidding traces); `model-check-skill-notes/10-cross-book-synthesis.md`
§9; `model-check-skill-notes/14-skill-building-plan.md` §4.2 (weak fairness by the
copies method, strong fairness not supported); `model-check-skill-notes/11-skill-requirements.md`
§4 q. 9, §6 step 7, §7.3, FR-008, AC-05.

## 1. Definitions (05 гл. 3)

For an action or a process `a` on an infinite path:

| Kind | Requirement on the path | Also called |
|---|---|---|
| unconditional | `a` occurs infinitely often, regardless of being enabled | impartiality |
| strong | if `a` is enabled infinitely often, it occurs infinitely often | compassion |
| weak | if `a` is enabled continuously from some point on, it occurs infinitely often | justice |

Strong fairness is the stronger assumption: every strongly fair path is weakly fair,
not the reverse. The set of paths shrinks as the assumption grows:
all paths ⊇ weakly fair paths ⊇ strongly fair paths ⊇ unconditionally fair paths.

Fairness is an **assumption about the environment or scheduler**, not a property of
the model. It says which infinite paths are considered unrealistic and are excluded
from the quantifier "for all paths" in a liveness property. Safety is unaffected by
any realizable fairness (05 гл. 3): a bad prefix is finite and does not care about
what happens afterwards.

**Realizability.** A fairness assumption is realizable if from every reachable
state there is a fair continuation. Unrealizable fairness (easy to produce with
unconditional fairness, or with fairness over an action that is sometimes never
enabled again) leaves some states without any fair path, and every universal
statement about those states becomes vacuously true (05 гл. 3; 08 гл. 3: the fair
value of a formula is zero where no fair path exists). Always ask whether a fair
path exists before reading a `verified` liveness result.

## 2. What the engine supports

Weak fairness is supported, in the sense of SPIN's `pan -f`: **process-level weak
fairness** — every process that is continuously executable from some point on
executes infinitely often. It is implemented by the method of copies (n + 2 copies
of the property automaton in the product, plan §4.2), applies to LTL properties and
to non-progress (`progress`-label) search, and is switched on per `mc_check` call by
the `fairness` input (`none` or `weak`).

Strong fairness is not supported by the engine. There is no search mode for it, and
the skill must say so whenever the user's argument needs it (§5 below tells you what
can still be said).

Two more limits, both consequences of the definition above:

- The engine's fairness is about **which process** is scheduled, not about which
  branch a process takes inside an `if`/`do`. A process that is fairly scheduled may
  still forever pick the same alternative (07 лекция 6). If the requirement needs
  fair choice among branches, model it explicitly (a counter, a turn variable) and
  say so.
- CTL properties are checked **without fairness**; fairness in CTL changes the
  domain of the path quantifiers (05 гл. 6) and the engine's CTL has no such switch.
  A CTL liveness result therefore includes unfair paths; if the user wanted fair
  paths, the property must be moved to LTL with weak fairness, or reported as
  fairness-free.

## 3. Protocol for every liveness property (07 лекция 6; 10 §9; 11 §6 step 7)

1. Run **without** fairness first. If `verified`, no fairness assumption is needed
   and none should be added.
2. If `violated`, call `mc_explain` and look at the loop. Ask: which process or
   transition is enabled along the loop and never taken?
3. Decide with the user whether that starvation is realistic in the operating
   environment: a real scheduler that never runs a ready process is unrealistic; a
   channel that loses every message forever is often *realistic* for a safety
   argument and unrealistic for a liveness argument — the user has to say which.
4. If the justification is weak fairness (a continuously enabled process is
   eventually scheduled), rerun with `fairness: weak`. Report **both** results, as
   two different statements about two different sets of paths (AC-05).
5. If only strong fairness would exclude the loop (the starved process is enabled
   infinitely often but not continuously — typically a process waiting on a
   condition that flickers), go to §5.
6. Record in the report (11 §7.3): which infinite paths are excluded and why that is
   admissible. Fairness lives in the manifest, not in a hidden flag.

Never add fairness because the counterexample goes away (03 гл. 6; 11 §15). The
question to answer is "does the excluded path exist in the real system?", not "does
the result turn green?".

## 4. Reading weak-fairness results when the question is about strong fairness

Because strongly fair paths ⊆ weakly fair paths — and this holds whether the user's
strong fairness is about processes or about individual statements, since a
continuously enabled process has some statement enabled infinitely often:

| Weak-fairness result | What it says about strong fairness |
|---|---|
| `verified` (evidence `exhaustive`) | the property also holds on all strongly fair paths; write this inference out as a sentence in the report (the status field stays the engine's `verified` for the weak-fairness property) |
| `violated` | undecided: the counterexample loop may or may not be strongly fair — check it manually (§5) |
| `inconclusive` / `unknown` | undecided |

## 5. Manual check of a lasso against strong fairness

When strong fairness is the user's assumption and the weak-fairness run returned
`violated`, do this with the `mc_explain` output and record it in the report:

1. List the transitions (or processes) that are **enabled in at least one state of
   the loop** — enabled, not taken.
2. For each, check whether it is **taken somewhere in the loop**.
3. If every transition enabled somewhere in the loop is also taken in the loop, the
   loop is strongly fair and the counterexample stands under strong fairness →
   `violated` for the strong-fairness reading.
4. If some transition is enabled in the loop and never taken, the loop is not
   strongly fair. The counterexample does not stand under strong fairness, and the
   engine cannot search for another one → status `not-executed` for the
   strong-fairness reading, reason "strong fairness unsupported; the weak-fairness
   counterexample is excluded by the assumption", next step: model the fairness
   explicitly (e.g. a bounded counter of consecutive skips) and rerun.

This procedure decides one lasso, not the property; say so.

## 6. Fairness and vacuity

Fairness can make a liveness property true for the wrong reason:

- **No fair path at all** (unrealizable fairness): every universal formula holds
  vacuously (§1). Sanity property: an `EF`-style or `reach` check that the
  interesting states are reachable does not help here; ask `mc_check` for the same
  property under `none` and compare.
- **Fairness excludes exactly the paths where the antecedent occurs**: e.g. a
  request only arrives on paths the assumption rules out. The vacuity candidates
  from `mc_lint_property` catch the unreachable-antecedent case; the fairness case
  you catch by comparing runs with and without fairness.

## 7. Corpus models to keep in mind

- `CH4/fair.pml`, `CH4/fair_accept.pml`, `CH4/pcval.pml` — the SPIN book's micro-models
  for `pan -f`; they are the oracle for the engine's weak-fairness search (plan
  §2.1), and a good place to show a user what weak fairness changes.
- `CH8/fairness.pml` — process `A` carries an `accept:` label on its loop while `B`
  loops beside it. Interleaving `B`'s steps into the cycle keeps it accepting, so
  weak fairness does not remove this acceptance cycle: fairness excludes
  starvation of an enabled process, not every unwanted cycle.
- `CH4/dijkstra.pml` vs `CH4/dijkstra_progress.pml` — the same semaphore with and
  without a `progress` label: non-progress is a property of the labels you place,
  and fairness changes which cycles count.
- `CH3/alternatingbit.pml` (eval E2) — "every message is delivered" is a liveness
  claim; the fairness question is asked before the run, and the report shows the
  result with and without the assumption as different results.
