# Properties: classification, LTL, CTL, and where they diverge (FR-004, FR-007)

Sources: `model-check-skill-notes/05-principles-of-model-checking.md` гл. 3
(linear-time properties, safety/liveness, fairness), гл. 5 (LTL, patterns, vacuity),
гл. 6 (CTL, CTL*, fixed points, witnesses); `model-check-skill-notes/03-karpov-model-checking.md`
гл. 2, 3, 4, 6 (LTL/CTL semantics, patterns, polarity); `model-check-skill-notes/07-lectures-01-09.md`
лекции 7–9 (never claims, LTL in SPIN syntax, expressiveness); `model-check-skill-notes/11-skill-requirements.md`
§5 (CTL/LTL not interchangeable), §8 (property requirements), FR-004, FR-007, FR-011;
`model-check-skill-notes/14-skill-building-plan.md` §4.2, §6 (`mc_lint_property`).

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
release, `!`, `&&`, `||`, `->`, `<->`. Atoms are `#define`d expressions over
variables, channel predicates (`len`, `empty`, `full`) and control labels
(`proc@label`, `proc[i]@label`). A ready-made `never { }` claim is accepted as an
alternative to a formula (plan §4.2); the corpus uses never claims, not `ltl { }`
blocks, and the engine follows that priority.

CTL uses `A`/`E` path quantifiers directly followed by `X`, `F`, `G`, `U`:
`AG p`, `EF p`, `AG(p -> AF q)`, `E(p U q)`, `AG EF p`. The engine marks the
reachable graph with `EX`, `EU`, `EG` as the basis and derives the rest by duality;
CTL fairness is out of scope, so every CTL result is "over all paths including
unfair ones".

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
| "no non-progress cycle" | `progress` labels | — | — | engine mode `progress` (v1) |

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
  (`fair -> φ`) or the engine applies weak fairness in the search; CTL cannot
  express fairness in a formula and the engine's CTL has no fairness switch. So a
  CTL liveness result is fairness-free by construction; say so in the report.
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
| vacuity candidates | antecedent of an implication never true, an atom that is constant on all reachable states | add a reachability property for the antecedent; if unreachable, report the main property as vacuous and do not call it a guarantee (AC-13) |
| polarity note | for a hand-written never claim: whether it looks like the property or its negation | confirm with the user which one it is |

## 6. Formalisation checklist (11 §8, 07 лекция 8)

1. Paraphrase the formula back in the user's language before running.
2. Check the scope of negations and the precedence — parenthesise.
3. Decide `U` vs the weak form: must the right side occur?
4. Decide whether the trigger must occur at all (`<> req` as a separate sanity property).
5. Test the formula mentally on one good trace, one bad trace and one vacuous trace.
6. Ask whether `X` is really part of the requirement; if not, remove it.
7. For liveness: which fairness, and why is it realistic (see `fairness.md`).
8. For CTL: is one trace enough as a witness, or is a tree needed?
