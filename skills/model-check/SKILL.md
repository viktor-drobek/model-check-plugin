---
name: model-check
description: >
  Model checking of protocols, concurrent algorithms, state machines and Petri nets
  with a built-in explicit-state engine (Go, exposed as MCP tools mc_parse, mc_check,
  mc_explain and others, or as the `mcd` CLI) — no external SPIN, NuSMV or other model
  checker is needed or should be looked for. Use this skill whenever the user wants to
  verify, prove or refute a behavioural property of a system with several interacting
  parts: deadlock, mutual exclusion, invariants, reachability, liveness, response,
  starvation, fairness, LTL, CTL, Promela models, never claims, Petri nets, token
  nets, alternating-bit or handshake protocols, telephone switches, producer/consumer,
  leader election, "can this hang", "is a counterexample possible", "verify this
  protocol", "check this model", "model checking", "deadlock", "counterexample".
  Russian triggers, use the skill for any of them: «проверь модель», «может ли
  зависнуть», «докажи, что никогда», «сеть Петри», «тупик», «инвариант», «взаимное
  исключение», «живость», «liveness», «справедливость», «Promela», «never claim»,
  «LTL», «CTL», «model checking», «верификация протокола», «контрпример», «гонка»,
  «переведи сеть Петри во что-нибудь проверяемое». Also use it when the user only
  describes the system in words and asks whether something bad can ever happen or
  something good must eventually happen — that is a model-checking question even if
  they do not say so. Do not use it for unit tests of sequential code, data-race
  linters, single-function theorem proving, or for drawing diagrams without analysis.
---

# model-check

You verify finite-state models with a built-in explicit-state engine. The engine
does the searching; you do the intake, the classification, the formalisation, the
choice of budget and mode, and the honest interpretation of what came back. Nothing
in this skill asks you to reproduce checking logic in scripts — if a computation is
deterministic (state counting, LTL translation, trace decoding) it belongs to the
engine, and you call a tool for it.

Two facts shape every answer you give:

1. A result is a statement about a **model** under **assumptions**, never about the
   real system. Carrying it over to the implementation needs a separate argument
   (conformance, testing, a documented refinement), which you name but do not invent.
2. The worst outcome is a false `verified`. When the search was not exhaustive, when
   fairness was added to silence a counterexample, or when the property is vacuously
   true, say so with the exact status and evidence level; never round up to "proved".

## Where the reasoning comes from

The engine and this skill were built from a corpus of notes on model checking
(`model-check-skill-notes/01`–`11`) and a corpus of Promela models
(`Promela - examples/`). Reference files cite their sources at the top; when a
reference and your memory disagree, trust the reference — it was checked against
the sources, your memory was not.

## Workflow in eight steps

Follow the steps in order. Steps 1–4, 6 and 7 have exit points where the honest answer
is a question, a `not-executed`, an `invalid-model` or a rerun; taking an exit is a
success, not a failure. Read
`references/workflow.md` before the first run in a session: it holds the full
decision tree with the exit conditions and the staged-budget rule.

### 1. Intake

Fill `assets/intake-card.yaml` from what the user gave you. The card has the fields
FR-001 asks for: system boundary, state, transition semantics, properties,
assumptions, fairness, budget, expected evidence. Where a field is empty and cannot be
safely inferred, ask (the mandatory questions are in `references/workflow.md` §2); for a
teaching example you may fill explicitly labelled defaults instead, but never a
business-critical assumption. Why this matters: the engine can only check what the
card says, and an unasked question about the atomic step or the channel capacity
silently becomes a claim in your report.

### 2. Classify the model

Two independent axes (FR-003): finiteness (finite / potentially infinite) and
semantic class (untimed / timed / probabilistic). Then decide the input formalism:

- asynchronous processes, channels, shared variables → Promela subset
  (`references/promela-subset.md`);
- places, transitions, tokens, "marking", a token game → Petri net JSON conforming
  to `assets/petri-net.schema.json` (`references/petri-nets.md`; keys `name`,
  `initial`, `inputs`/`outputs` with `place` and `weight`), which the engine turns
  into the same intermediate representation;
- a system described only in words → write it in the Promela subset first; direct IR
  authoring is experimental (plan assumption A7).

Exit points: timed or probabilistic semantics, unbounded data, or a construct outside
the subset → status `not-executed` with the reason and a route (which specialised
tool class would fit), no imitation of a result. Details and the branch table:
`references/model-classification.md`.

### 3. Model → IR with `mc_parse`

Two layers reach the same engine (`references/engine-tools.md`). When the plugin's MCP
server is registered in the session, call the `mc_*` tools; otherwise every `mc_*` name
in this file means its `mcd` CLI equivalent: `mcd parse --petri|--ir|--promela [-D …]`
and `mcd check --petri|--ir|--promela [-D …] [--ltl 'φ']… [--progress]
[--fairness none|weak] [--budget-*] [--unlimited] [--bfs] [--sweep] [--no-timing]`.
Promela input goes through the `promela` field of `mc_parse` or through
`mcd parse --promela` / `mcd check --promela` — both work; a Petri net goes through
`petri` / `--petri`. Exit code 2 (CLI) or `outcome: rejected` (MCP) is the parser
rejection meant below; exit code 0 / `outcome: report` is a result even when it says
`invalid-model`. An `--ltl` formula the engine cannot parse is also a rejection, not
a verdict.

Call `mc_parse` on the Promela text or the Petri JSON. Read the warnings, not just the
success flag: `printf` ignored, never claim not executed, capacity defaults applied.
If the parser rejects a construct, the choices are to rewrite the model inside the
subset or to explain the boundary; working around the parser (hand-editing its
output, faking the construct) is not one of them, because the differential tests that
back the `exhaustive` evidence level only cover the grammar the parser accepts.

### 4. Properties

Classify each property on three axes (FR-004): class (safety / reachability /
liveness), logic (invariant / LTL / CTL), extension (untimed / timed / probabilistic).
Run `mc_lint_property` on every property before checking — a state property by
`expr`, an LTL property by `formula` — it returns the atoms, whether they are
defined, vacuity candidates, the safety/liveness class and whether the formula is
`X`-free. Always add at least one sanity reachability property (is the trigger
reachable? is the interesting state reachable?) next to the main requirement; a
response property whose request never happens holds for nothing.

An LTL property is passed as `formula` (SPIN syntax), never as `expr`; the engine
checks it by searching the product with the automaton for its **negation**, and the
record it returns carries `temporal` — the formula, the atoms, the fairness, and
`stutter_invariant`, which is false exactly when the formula uses `X`. Control-label
atoms (`proc@label`) are rejected by the **LTL** parser (`kind: "ltl"`, "unexpected
character '@'") but **accepted in CTL**, where they normalise to `pc(…)`. So a
requirement about a control location is asked either in CTL directly, or in LTL
after adding a `progress` label or a variable to the model — and that addition is a
change to the model, which you declare. `ctl` is executed since G5.

LTL and CTL are not interchangeable (FR-007). Keep the logic the user chose or the
requirement implies; the table of equivalent and diverging patterns is in
`references/properties-ltl-ctl.md`. For every liveness property ask about fairness
**before** running (FR-008): which infinite paths does the user consider
unrealistic, and why. The engine supports weak fairness only (`fairness: weak`,
`--fairness weak`); **strong fairness is unsupported** — the engine accepts
`fairness: strong` and answers `not-executed`, and the report must say that the
question was not answered by a search, not quietly present the weak-fairness run in
its place (`references/fairness.md`). Report fairness as an assumption you made,
never as a fact about the system; and when the run without fairness already returns
`verified`, say the result does not depend on fairness rather than adding the
assumption to it.

### 5. Pilot

Run `mc_simulate` (random walk with a fixed seed, a few hundred steps) to see that
the model moves the way the user expects, and `mc_estimate` to get the growth of the
state count under a short time limit. Choose from these the budget (time, states,
depth) and the search mode: DFS by default, BFS when a shortest safety counterexample
is worth the memory. Simulation is a sanity check; it is never evidence for a
property.

### 6. Check with `mc_check`

Pass the IR, the property list, the fairness setting and an explicit budget. An
absent or zero budget field means the default in both layers; only the CLI can lift
a budget, with `--unlimited`. Liveness is run **without** fairness first
(`references/fairness.md` §3). Note which properties you did not ask for: the
Promela frontend adds `deadlock`, the model's own `assert`, and — from the model —
`never`, `accept` and `progress` when the corresponding labels or claim are there.
The `progress` property in particular appears automatically as soon as any process
carries a `progress` label, and a model with **no** progress label makes every cycle
a non-progress cycle, so a `violated` `progress` on such a model means "no labels
were placed", not "the system hangs". When a
property returns `inconclusive`, increase the budget in steps (FR-024) and rerun; do
not reformulate the result. When the engine reports an overflow (`byte` wrap, channel
capacity, place capacity) the status is `invalid-model`: the engine cannot tell
whether the real system has the same bound, so fix or justify the domain in the
model first and make no property claim until then. After any change to the model, rerun every property, not only the one
that failed — fixing a deadlock can open a non-progress cycle.

### 7. Analyse with `mc_explain`

For every `violated` property call `mc_explain` to get the counterexample as prefix
and loop with the user's names and per-step variable diffs. A temporal violation is
a **lasso**: its `loop` record (`counterexample.loop`) gives `start` (the 1-based
index of the first loop step) and `steps`, and everything from there repeats forever — say that plainly, or
the reader counts the steps and asks what happens next. Three process names in a
lasso are not the user's processes and must be dropped or explained: the claim
(`never:…`, or `np_` for a non-progress search), and `-`, a null step that is either
the weak-fairness bookkeeping or the stutter extension of a system that has stopped.
Then classify the cause:
system defect, model defect (including artefacts of the model's over-approximation
of the real system), property defect (wrong polarity, wrong atom, wrong logic), or a
fairness/environment artefact that a justified assumption would exclude. The
classification procedure and the "first causal fork" rule are in
`references/counterexamples.md`. Propose the fix as a hypothesis to verify, never
as an edit to the requirement.

### 8. Report

Use `assets/report-template.md` — its section order is fixed by the requirements
(11 §14). Attach `mc_manifest` output (versions, hashes, parameters, seed, times).
Check the wording against the forbidden-phrasing list in
`references/evidence-and-status.md` before sending. Write in the user's language;
keep the status and evidence tokens in English exactly as the engine returns them.

## Status and evidence — the short version

Every property gets one status from the vocabulary of 11 §14 and one evidence level:

| Status | Meaning |
|---|---|
| `verified` | search completed, no violation found |
| `violated` | the engine found a concrete counterexample; you replay and classify it before reporting |
| `inconclusive` | a correct but incomplete search: budget exhausted, bounded, approximate |
| `unknown` | the result cannot be read even as partial coverage |
| `not-executed` | nothing was run: out of subset, unsupported semantics, no binary, user declined |
| `invalid-model` | the model itself is defective (overflow, capacity, undefined atom) |

Evidence: `exhaustive`, `bounded`, `approximate`, `unknown`. Only `verified` with
`exhaustive` may be phrased as "the property holds on the model". Everything else is
phrased as what was searched and what was not. A budget stop on **states or depth**
is `bounded` (the bound can be named); a stop on **time or memory** is `unknown` —
the state count reached is a measurement, not a coverage claim. The engine does not
produce statistical evidence because it checks no probabilistic models.

Read `references/evidence-and-status.md` for the decision procedure that assigns
the status, the status × evidence table, and the list of phrasings you must not use.

## Reference files — what to read when

| Read this… | …when |
|---|---|
| `references/workflow.md` | at the start of a session; when deciding whether to stop, ask, or continue |
| `references/model-classification.md` | the system's class is unclear, or it might be timed, probabilistic or infinite |
| `references/properties-ltl-ctl.md` | translating a requirement into a formula; the user mixes LTL and CTL; choosing a logic |
| `references/fairness.md` | any liveness property; the user proposes or rejects fairness |
| `references/promela-subset.md` | writing or reviewing a Promela model; the parser rejected something |
| `references/petri-nets.md` | the input is a Petri net, token net, CPN, or workflow with places |
| `references/engine-tools.md` | before calling a tool for the first time; reading a tool response; no MCP available |
| `references/counterexamples.md` | a property came back `violated`; the user asks what the trace means |
| `references/evidence-and-status.md` | writing the report; any doubt about which status applies |
| `references/pitfalls.md` | reviewing a model or a property someone else wrote; before declaring `verified` |

## Four short examples from the corpus

- **`CH2/mutex_flaw.pml`** — invariant `assert(cnt == 1)` in the critical section.
  Expected: `violated`, evidence `exhaustive`, counterexample through labels `L1`–`L4`
  showing both users at `L7`. Cause class: system defect (the algorithm is wrong).
- **`CH3/alternatingbit.pml`** — sender and receiver alternate in lock-step, so
  delivery comes back `verified` / `exhaustive` **with and without** weak fairness:
  the honest sentence is that the result does not depend on the assumption, not that
  the assumption justifies it. The file has no loss, no retransmission and no
  timeout, so the verdict is about a handshake, not about the alternating-bit
  protocol — say which. For the opposite case (a liveness property that is
  `violated` by an unfair loop and `verified` under `fairness: weak`) use
  `engine/testdata/promela/starvation.pml`.
- **`CH5/pathfinder.pml`** — deadlock through priority inversion. The model uses
  `provided`, which is v1 of the subset; until then the honest status is
  `not-executed` with the construct named. When accepted, partial-order reduction is
  inapplicable because of the priorities — a fact to record, not a knob to try.
- **`CH14/version1`** — a telephone switch with labels and no variables. "Can it
  get stuck in `Busy`?" is liveness, and it cannot be written as a formula here:
  label atoms are not accepted and the model has no variable to point at. The route
  is a `progress` label placed where the requirement says progress is, declared as a
  change to the model, and then the `progress` property the frontend adds by itself.
- **`App_C/petrinet1`** — a Petri net encoded in Promela. As Petri JSON: initial
  marking `p1 = p4 = 1`; firing `t1` then `t4` leaves `p2 = p5 = 1` with no enabled
  transition. Expected: hang (deadlock) `violated` with the two-step counterexample.

## What this skill does not do

- It does not run or recommend external model checkers; the built-in engine is the
  backend, and SPIN exists in the project only as an oracle for the engine's tests.
- It does not check CTL yet (`ctl` → `not-executed`, G5) and it does not check
  strong fairness (`not-executed`, FR-008); it says so rather than substituting a
  neighbouring result.
- It does not check timed, probabilistic, BDD-symbolic or SAT-bounded problems.
  For those it produces the model, the property classification and a plan with status
  `not-executed` (FR-020, FR-021 stay at routing level).
- It does not accept `c_code`, `c_expr`, `c_decl` or `unless`; it does not execute
  model code as host code.
- It does not carry a model's result over to the implementation, and it does not
  edit a requirement to make a counterexample disappear.
