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
§4 q. 9, §6 step 7, §7.3, FR-008, AC-05. Engine as built: `engine/explore/cycle.go`
(the copies construction, the null step), `steps/g4-confirmation.md` §1, §3.

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

## 2. What the engine supports (as built in G4)

**Weak fairness is supported**, in the sense of SPIN's `pan -f`: **process-level
weak fairness** — every process that is continuously executable from some point on
executes infinitely often. It is implemented by the method of copies (n + 2 copies
of the product, one per process plus two bookkeeping copies, plan §4.2), applies to
`ltl` properties and to the non-progress (`progress`-label) search, and is asked for
per run: `fairness: weak` in `mc_check`, `--fairness weak` on the CLI. Default:
`none`.

A weakly fair counterexample may contain steps of a process written `-`. That is a
**null step** of the copies construction: it advances the fairness bookkeeping
without any process moving. Drop it when you present the trace, and never describe
it to the user as an action of the system (`counterexamples.md` §2a).

**Strong fairness is not supported by the engine.** There is no search mode for it.
`fairness: strong` / `--fairness strong` is nevertheless accepted, and the engine
answers with a *result*, not an error: status `not-executed`, evidence `unknown`,
and a `reason` that names FR-008 and says only weak fairness is implemented. What
the skill must do with that:

- report the status the engine gave — `not-executed`, never a silent downgrade to
  the weak-fairness run;
- say in the report that the question the user asked (strong fairness) was **not
  answered by a search**, and say which question was;
- run `none` and `weak` as well, and use §4 to say what the weak result does and
  does not imply about strong fairness;
- if the argument really needs strong fairness, model it explicitly (§5, step 4).

**Fairness is reported as an assumption, never as a fact about the system.** The
engine has no way to know whether the scheduler of the real system is weakly fair;
`fairness: weak` only removes paths from the model's set of runs. So a fair result
is written "under the assumption of weak process fairness, P holds on model M", and
the assumption goes into the report's assumptions section and the manifest — not
into a footnote, and not into the verdict word. A liveness result whose report does
not name its fairness setting is unreadable (`engine-tools.md` §7, step 5).

Two more limits, both consequences of the definition above:

- The engine's fairness is about **which process** is scheduled, not about which
  branch a process takes inside an `if`/`do`. A process that is fairly scheduled may
  still forever pick the same alternative (07 лекция 6). If the requirement needs
  fair choice among branches, model it explicitly (a counter, a turn variable) and
  say so.
- CTL is executed since G5, but **the copies construction is not wired into it**:
  fairness in CTL changes the domain of the path quantifiers (05 гл. 6) and is a
  different mechanism. Do not assume `fairness: weak` reaches a `ctl` property —
  check the record's `temporal.fairness` and say in the report which paths the
  verdict quantifies over. A branching requirement that genuinely needs fair paths
  must be moved to LTL with `fairness: weak`, with the change of meaning stated
  (`properties-ltl-ctl.md` §3), or reported as not answered.

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
   eventually scheduled), rerun with `fairness: weak` (`--fairness weak`). Report
   **both** results, as two different statements about two different sets of paths
   (AC-05). The engine helps with step 2: when a lasso is found under `fairness:
   none`, `reason` already names the processes that move in the loop and the ones
   that are enabled throughout it and never move.
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
   loop is strongly fair and this one counterexample survives the assumption — provided
   step 1 listed *every* transition enabled in *every* state of the loop. That listing is
   yours, not the engine's: say in the report how you obtained it, and if the loop is long
   or the enabling conditions are data-dependent, say that the check is a reading of the
   trace rather than a search. **The
   status does not change**: the engine answered `not-executed` / `unknown` for
   `fairness: strong` and that is what the status field and the report's property table
   keep saying (`evidence-and-status.md` §1). What you add is a sentence of your own, in
   the analysis and not in the status column: "this lasso is strongly fair, checked by
   hand over its N steps, so the requirement fails on it as well; no search for other
   strongly fair counterexamples was performed." Attribute it to yourself, because a
   reader who sees `violated` will otherwise believe a search produced it.
4. If some transition is enabled in the loop and never taken, the loop is not
   strongly fair. The counterexample does not stand under strong fairness, and the
   engine cannot search for another one → status `not-executed` for the
   strong-fairness reading, reason "strong fairness unsupported; the weak-fairness
   counterexample is excluded by the assumption", next step: model the fairness
   explicitly and rerun. The usual device — a counter that forces the skipped
   transition after k consecutive skips — is **not** an encoding of strong fairness:
   strong fairness puts no bound on how long the wait may be (§1), so a bounded counter
   assumes strictly more than the user asked for and removes real behaviours from the
   model. Name that loss in the report, or ask the question in a tool that has strong
   fairness.

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

## 6a. Two models that show both outcomes (run them before you explain fairness)

`engine/testdata/promela/starvation.pml` — process `A` flips its own bit forever,
process `B` has one statement, `done = 1`. `<> done` under the three settings:

| run | result |
|---|---|
| `mcd check --promela starvation.pml --ltl '<> done' --fairness none` | `violated`, evidence `exhaustive`; lasso with `loop.start` 3 and 4 loop steps in which only `A:0` and the claim move; `reason`: "`B:1` is enabled throughout the loop and never moves (the loop is not weakly fair; rerun with fairness weak to exclude such runs)" |
| the same with `--fairness weak` | `verified`, evidence `exhaustive`, complete — the loop above is no longer a run of the fair model |
| the same with `--fairness strong` | `not-executed`, evidence `unknown`, reason FR-008 (§2) |

That is the whole shape of a fairness argument in three runs, and it is the one to
show a user: the property did not become true, the set of paths became smaller.

`Promela - examples/CH3/alternatingbit.pml` is the opposite case and the more
instructive one: it is lock-step, so its delivery does not depend on the fairness
setting. Its sender and receiver alternate in **lock-step**: after every
send exactly one statement of the other process is enabled, so no process can be
starved by any scheduling at all. Delivery there is `verified` with evidence
`exhaustive` **without any fairness assumption** (and again, with a larger product,
under `fairness: weak`) — `pan` agrees. Do not add a fairness assumption to that
model: it changes nothing, and reporting it as a premise of the result claims the
result needs it. It also does not: the honest sentence is "the result does not
depend on the fairness setting, because the model is lock-step". Note what that
model *does not* contain — message loss, retransmission, timeouts — which is where
the real alternating-bit protocol's liveness question lives; see §7.

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
  claim, so the fairness question is asked before the run; but the answer for *this*
  file is that the assumption is not needed (§6a). The trap to avoid is carrying the
  verdict from the file to the protocol: the corpus model has lossless channels and
  no retransmission, and the alternating-bit protocol exists precisely to survive
  loss. A `verified` here is a statement about a lock-step handshake, not about the
  protocol; say which, in the report.
