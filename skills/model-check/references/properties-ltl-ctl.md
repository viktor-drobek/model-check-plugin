# Properties: classification, LTL, CTL, and where they diverge (FR-004, FR-007)

Sources: `model-check-skill-notes/05-principles-of-model-checking.md` гл. 3
(linear-time properties, safety/liveness, fairness), гл. 5 (LTL, patterns, vacuity),
гл. 6 (CTL, CTL*, fixed points, witnesses); `model-check-skill-notes/03-karpov-model-checking.md`
гл. 2, 3, 4, 6 (LTL/CTL semantics, patterns, polarity); `model-check-skill-notes/07-lectures-01-09.md`
лекции 7–9 (never claims, LTL in SPIN syntax, expressiveness); `model-check-skill-notes/11-skill-requirements.md`
§5 (CTL/LTL not interchangeable), §8 (property requirements), FR-004, FR-007, FR-011;
`model-check-skill-notes/14-skill-building-plan.md` §4.2, §6 (`mc_lint_property`).
Engine as built: `engine/ltl/` (parser, NNF, GPVW tableau, Büchi automaton),
`engine/explore/cycle.go` (the product and the nested DFS),
`engine/frontend/promela/lower.go` (which properties the frontend adds),
`steps/g4-confirmation.md` §1, §5.

Contents: 1 classification axes · 2 engine syntax and its limits · 3 LTL/CTL
patterns · 4 where they diverge · 5 `mc_lint_property` · 6 never claims and claim
semantics · 7 `progress` labels and non-progress · 8 formalisation checklist.

## 1. Three classification axes (FR-004)

| Axis | Values | How to decide |
|---|---|---|
| Class | safety, reachability, liveness | safety: every violation has a finite bad prefix; reachability: "some state with p is reachable" (a sanity or planning question); liveness: every finite prefix can still be extended to a good behaviour, violation is an infinite path (a lasso in a finite model) |
| Logic | invariant, LTL, CTL | invariant: a state formula that must hold in every reachable state; LTL: a statement about every path; CTL: a statement about the tree of continuations with explicit path quantifiers |
| Extension | untimed, timed, probabilistic | timed/probabilistic → `not-executed` (see `model-classification.md`) |

Deadlock is handled by the engine as its own property kind (`deadlock`): "a reachable
state has no enabled transition and not every process is at an `end` label". It is a
safety-type finding (finite witness) but is not written as a formula, because the
usual Kripke semantics assumes a total transition relation.

Every property record carries (11 §8): an ID, the user's wording, class, logic,
the formula, the atom dictionary (atom → model expression → source-system meaning),
assumptions (fairness, bounds), and the expected form of evidence (finite trace,
lasso, tree, none).

## 2. Engine syntax

LTL uses SPIN syntax: `[]` always, `<>` eventually, `X` next, `U` strong until, `V`
release, `!`, `&&`, `||`, `->`, `<->` (and `/\`, `\/`). Atoms are comparisons and
arithmetic over the model's variables, array elements `a[i]`, and the channel
predicates `len`, `empty`, `nempty`, `full`, `nfull`; object-like `#define`s of the
model are expanded into the formula before parsing, so a corpus model's own macro
names work as atoms. A ready-made `never { }` claim is accepted as an alternative to
a formula (§6); the corpus uses never claims, not `ltl { }` blocks, and the engine
follows that priority.

**Control-label atoms (`proc@label`, `proc:pid@label`): rejected in LTL, accepted in
CTL.** The bracket form `proc[i]@label` is *not* parsed — the instance is selected with a
colon and the engine's own 1-based pid (`switch:1@Idle`), and a bare `switch@Idle` means
the first instance of that proctype. In an `--ltl` formula `mcd` rejects the whole run with `kind: "ltl"`
("unexpected character '@'") and produces no report. In a `--ctl` formula the same
atom is accepted and normalised to a program-counter test: `AG EF (subscriber@Idle)`
on `CH14/version1` is evaluated as `!E[true U !E[true U (pc(0) == 0)]]` and comes
back `verified` / `exhaustive`. So when the requirement is about a control location —
"the phone is in `Busy`" — there are three routes, and only the first leaves the
model alone:

1. ask it in **CTL**, over the label itself;
2. add a `progress` label and use the non-progress search (§7);
3. add a variable the process sets at that location and write an LTL atom over it.

Routes 2 and 3 change the model and must be declared as such in the report; route 1
does not, and is the one to reach for first when the question is branching anyway
("from every state, can it get back to `Idle`?").

**`stutter_invariant`.** Carried by the records the engine compiled a formula for — `ltl`
and `progress`; it is `omitempty`, so a `ctl` record and a record without a compiled
formula simply do not have the field, and its absence is not `false` (`temporal`,
`engine-tools.md` §5.2). It is `false` exactly when the formula uses `X`, and true
otherwise. An `X`-formula distinguishes runs that differ only in how one step is
split, so its verdict depends on the model's atomic-step decisions — the `atomic`
and `d_step` boundaries of `promela-subset.md` §2 — and on nothing the user asked
about. When you see `stutter_invariant: false`, either justify the atomic step in
the report or rewrite the requirement without `X` (checklist step 6).

CTL — `A`/`E` path quantifiers directly followed by `X`, `F`, `G`, `U`: `AG p`,
`EF p`, `AG(p -> AF q)`, `E[p U q]`, `A[p U q]`, `AG EF p` — **is executed since G5**,
(until is written with brackets: `E(p U q)` is rejected with "expected U inside `[ … U … ]`"), by
labelling the reachable graph (`--ctl 'φ'`, properties `ctl1`, `ctl2`, …; over MCP a
property of kind `ctl` with `formula`, not `expr`). The record carries
`temporal.logic` = `ctl`, the `normalised` form the engine actually evaluated
(`AG (cnt <= 1)` → `!E[true U !(cnt <= 1)]`), and a `note`. The classification of §3
and §4 is therefore about **choosing** the logic, not about routing round a missing
one: keep the logic the requirement implies instead of offering the LTL reading as a
substitute. For the full CTL vocabulary — witness and counterexample shapes, the
vacuity fields, the division of evidence — the authority is
`steps/g5-confirmation.md` §8 and `features/g5-ctl-v1.feature`; this file has not
yet had its row-by-row pass against them.

Polarity: for an LTL property the engine builds the automaton for the **negation**
and searches for an accepting cycle in the product; a `never { }` claim you pass is
already the negation (07 лекция 7, 03 гл. 4). When you hand-write a never claim,
state in the report which language it accepts — the corpus file `CH4/prop.pml` shows
both `[]p` and `![]p` as never claims under `#ifdef PHI`, and confusing them inverts
the verdict.

## 3. Typical properties in LTL and CTL

"Same?" says whether the two formulas have the same truth value on every finite
model without fairness and with a total transition relation (the engine reports
deadlocks separately, so the models it evaluates formulas on are total). Where they differ, the last column says which one to use.

| Intent (user's words) | LTL (SPIN syntax) | CTL | Same? | Note |
|---|---|---|---|---|
| "Bad never happens", mutual exclusion | `[] !bad`, `[] !(cs1 && cs2)` | `AG !bad` | yes | prefer the engine's `invariant` kind — cheaper, finite witness |
| "p is always followed by q" (response) | `[](req -> <> ack)` | `AG(req -> AF ack)` | yes | liveness → ask about fairness; check that `req` is reachable (vacuity) |
| "p happens infinitely often" (recurrence, progress) | `[]<> p` | `AG AF p` | yes | for Petri transitions `[]<> fire(t)` needs a fairness assumption to be meaningful |
| "eventually forever" (persistence, stabilisation) | `<>[] p` | no CTL equivalent; `AF AG p` is strictly stronger | **no** | LTL only (05 гл. 6); `App_A/example` and `CH12/leader.ltl` check `<>[]p` |
| "p eventually" (inevitability) | `<> p` | `AF p` | yes | remember: terminal states with no successor change the reading; the engine reports deadlock separately |
| "from every reachable state recovery is possible" | not expressible | `AG EF reset` | **no** | CTL only; a linear trace cannot refute it — the witness is a tree (05 гл. 6, 07 лекция 9) |
| "it is possible that p" (sanity, planning) | not expressible as a universal LTL check; `!<> p` violated ⇔ p reachable | `EF p` | **no** | use the engine's `reach` kind or `EF p`; a trail to `p` is the witness |
| "p until q" | `p U q` | `A(p U q)` | yes | strong until: `q` must occur |
| "in the next step p" | `X p` | `AX p` | yes | both depend on the atomic step; flag as not stutter-invariant |
| "on some path p holds forever" | not expressible | `EG p` | **no** | CTL only; the witness is a lasso inside `p`-states |
| "a transition t can always fire again" (Petri liveness) | `[]<> fire(t)` — needs fairness | `AG EF fire(t)` — no fairness | **no** | see `petri-nets.md` §4; the two are different questions |
| "no deadlock" | not a formula | not a formula | — | engine property kind `deadlock` with `end` labels |
| "no non-progress cycle" | `progress` labels | — | — | engine property kind `progress`, built in G4 (§7) |

Equivalences marked "yes" hold for the common fragment of LTL and CTL (the universal
formulas whose CTL version puts `A` in front of every temporal operator and whose
LTL version reads all paths); the "no" rows are exactly the places where the two
logics are incomparable (05 гл. 6; 03 гл. 2; 09 гл. 2).

## 4. Where they diverge in practice

- **Branching.** `AG EF p` says every reachable state can still reach `p`;
  `[]<> p` says every path actually visits `p` infinitely often. A model where a
  path can turn away from `p` forever satisfies the first and violates the second.
  The requirement "from each state there exists a path to recovery" is the first
  (AC-17 in 11 §18).
- **Persistence.** `<>[] p` (LTL) holds when every path eventually stays in `p`;
  `AF AG p` (CTL) demands a moment after which *all* continuations stay in `p`, a
  stronger claim (05 гл. 6).
- **Existence.** Anything starting with `E` has no LTL form because LTL is
  implicitly universal. A "reachable" question in LTL is asked backwards: check
  `[] !p`, and a `violated` result *is* the witness.
- **Fairness.** LTL can put the fairness assumption into the formula
  (`fair -> φ`) or the engine applies weak fairness in the search (`fairness.md`).
  CTL cannot express fairness in a formula, and this build does not check CTL **under**
  fairness either: a `ctl` property asked with `fairness: weak` or `strong` comes back
  `not-executed` with that reason, never as a quietly unfair answer. So a CTL result is
  always a fairness-free result — say so, and put a liveness question that needs a
  fairness assumption in LTL.
- **Witness shape.** LTL violation: one lasso (prefix + loop). CTL: a path for
  `EF`/`EG` witnesses and for `AG p` violations; a tree for violations of nested
  universal-existential formulas such as `AG EF p` (11 §8, 05 гл. 6). Say in the
  report whether one trace was enough.
- **Cost.** Explicit CTL is linear in model × formula; LTL is linear in the model
  and exponential in the formula (the automaton). Pick the logic by meaning, not by
  cost (10 §5).

Never rewrite a CTL requirement into LTL or the reverse to fit a tool; the engine
accepts both (eval E6 in plan §8.2 checks exactly this).

## 5. What `mc_lint_property` returns and what to do with it

| Field | Meaning | Action |
|---|---|---|
| atoms | the atomic expressions with their definedness in the IR | an undefined atom → fix the `#define` or the label; do not run |
| class | safety / liveness (syntactic classification) | liveness → fairness question (FR-008) |
| x_free | whether the formula contains `X` | `X` present → the result depends on the atomic step; POR (vNext) will be disabled |
| vacuity candidates | **syntactic only**: the antecedent of an implication, and an expression the engine can fold to a constant. `mc_lint_property` explores nothing, so it cannot know which atoms are reachable — its note says "check it with a reach property" | add that reachability property and run it; the *reachable*-state vacuity hint comes later, from `mc_check`'s per-property `warnings`. If the antecedent is unreachable, report the main property as vacuous and do not call it a guarantee (AC-13) |
| — (no polarity note) | `mc_lint_property` takes `invariant`, `reach`, `ltl` and `ctl`, and a claim is not one of them: nothing in the engine reads a hand-written `never { }` and tells you whether it encodes the property or its negation | do it by hand — paraphrase the claim, say which of the two it is, and confirm with the user before quoting a verdict from it |

## 6. Never claims and claim semantics (G4)

A `never { }` claim in the model becomes a property `never` (kind `ltl`) that the
frontend adds by itself; a `--ltl 'φ'` formula is compiled into exactly the same
kind of claim process, named `never:ltl1`, for the automaton of **`!φ`**. Five
details of how the engine runs a claim decide what a counterexample means; all five
match `pan`, and the differential oracle of `steps/g4-confirmation.md` §3.1 is what
says so.

1. **The claim moves first.** At every step the claim takes its transition and then
   the system takes one. A claim step is therefore interleaved into the trace before
   the system step it constrains; that is why step 1 of a lasso is almost always a
   claim step (`counterexamples.md` §2a).
2. **A blocked claim cuts the path.** If no claim transition is enabled, the run is
   not continued — that path simply does not satisfy the negation, and its absence
   is not a verdict about the system.
3. **A claim reaching its end is a violation on a finite prefix.** When the claim
   process runs off its last statement, the negated property is satisfied by the
   prefix alone: the counterexample is finite and has **no `loop`**. Report it as a
   bad prefix, not as "the loop was too short to find".
4. **Stutter extension — for a claim, and not for the `np_` search.** SPIN calls it
   the stutter extension, and so does the engine's step text. For an `ltl` property
   or a model's own `never` claim, a run that cannot be continued (all processes
   terminated, or a deadlock) is extended by repeating the final state forever, so
   that the formula can be evaluated on an infinite run; in the trace that is a step
   of process `-` whose command text says "stutter", exactly as under `pan -a`.
   The non-progress search does **not** do this: there a state where no process can
   move has no successors at all, as under `pan -l` (§7). A third mechanism serves
   the same end for `ctl`: a blocked state carries a self-loop, so that the
   transition relation is total and `EG`/`AF` are defined on it, and the property
   record's `temporal.note` says so. The three cases are tabulated in
   `counterexamples.md` §2a, which is also where you read a `-` step.
5. **`assert` scope — the one place the engine deliberately differs from `pan -a`
   in what it reports.** SPIN's `pan -a` evaluates `assert` statements only while a
   claim is in scope, so a run cut off by the claim never reports an assertion that
   would fail later. The engine checks the model's `assert` statements over the
   **whole** state space, as its own `assert` property, next to the claim property.
   Consequence for the user: a `--ltl` run of `mcd` can report a `violated` `assert`
   where `pan -a` on the same model with the same never claim reports none. That is
   not a disagreement about the model; it is a different question being answered. If
   a user is comparing your output with SPIN's, say this before they find it — and
   note the symmetric fact that the *claim* verdicts and the product state counts do
   agree (43 differential triples, G4 §3.1).

When you hand-write a claim, state in the report which language it accepts: the
corpus file `CH4/prop.pml` carries both `[]p` and `![]p` as never claims under
`#ifdef PHI`, and reading one for the other inverts the verdict.

## 7. `progress` labels and the non-progress search (G4, amended by the G5 addendum)

"The system cannot run forever without making progress" is not written as a formula.
You mark the statements that count as progress with labels whose name begins with
`progress`, and the engine searches for a cycle that visits none of them — SPIN's
`pan -l`.

- **The `progress` property is added automatically when the model has progress
  labels.** The Promela frontend puts a property `progress` (kind `progress`) into
  the IR as soon as some process carries such a label; it appears alongside `deadlock` in
  every report of a run **that used the model's own property list**. Pass your own
  `properties` and it is gone with the rest of them — only the implicit `assert` survives
  (`SKILL.md` step 6), so put it back into the list you send. `--progress`
  (MCP: a property of kind `progress`) asks for the same search on a model that has
  no label of its own.
- **A model with no progress label makes every cycle a non-progress cycle.** Running
  `--progress` on such a model returns `violated` with a lasso, and the lasso is
  simply the model's ordinary behaviour. Read it as "no progress labels have been
  placed", say so, and only then decide where progress actually is. Reporting that
  `violated` as a finding about the system is the classic misreading; the corpus
  pair `CH4/dijkstra.pml` (no label, non-progress cycle) and
  `CH4/dijkstra_progress.pml` (one label, `verified`) is the same semaphore twice
  and exists to make the point.
- Placing a label **changes the model**, so it is an assumption in the report: "the
  statement at line n is what we agreed counts as progress". The verdict depends on
  that choice at least as much as on the system.
- The claim process in a non-progress counterexample is `np_`, the automaton the
  engine synthesises; fairness applies to this search exactly as it does to `ltl`.
- **A blocked system is not a non-progress cycle.** In the `np_` product a state
  where no process can move has **no successors**, so no cycle runs through it and
  no counterexample can be built from it — the same rule as `pan -l`. A model that
  can deadlock therefore answers `--progress` with `progress` `verified` beside
  `deadlock` `violated`, and that pair is the correct reading: the system stops, it
  does not spin. This is the one place where the `np_` search deliberately differs
  from the `ltl` product, which *does* extend such a run (§6 item 4); the difference
  exists because "stuck" and "running without progressing" are different defects
  with different fixes, and reporting the first as the second hides it
  (`steps/g5-addendum-confirmation.md` §1).

## 8. Formalisation checklist (11 §8, 07 лекция 8)

1. Paraphrase the formula back in the user's language before running.
2. Check the scope of negations and the precedence — parenthesise.
3. Decide `U` vs the weak form: must the right side occur?
4. Decide whether the trigger must occur at all (`<> req` as a separate sanity property).
5. Test the formula mentally on one good trace, one bad trace and one vacuous trace.
6. Ask whether `X` is really part of the requirement; if not, remove it.
7. For liveness: which fairness, and why is it realistic (see `fairness.md`).
8. For CTL: is one trace enough as a witness, or is a tree needed? Since G5 this is
   a question about a real run — check what the record's `witness` / `counterexample`
   and `temporal.note` actually contain rather than assuming the shape.
