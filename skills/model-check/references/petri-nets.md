# Petri nets as an input formalism

Sources: `model-check-skill-notes/14-skill-building-plan.md` §2.2 (Holzmann §8.10:
seven claims; translation to IR; hang/live/safe; CTL vs LTL for transition
liveness; manual check of `petrinet1`), §5.3 (JSON schema; standard property set;
multiplicity; capacity 255; inhibitor arcs rejected); `model-check-skill-notes/02-design-and-validation-of-computer-protocols.md`
гл. 8 (FSMs, product machines, EFSM), гл. 10 (place synchronisation, tokens as
interpretation); `model-check-skill-notes/04-verification-web-services.md` гл. 3
(CPN/HCPN: colour sets, bindings, guards, pages, port/socket, compound places,
timed CPN, Murata liveness levels); `model-check-skill-notes/10-cross-book-synthesis.md`
§4, §11 (Petri/CPN routing); `model-check-skill-notes/11-skill-requirements.md`
FR-003, FR-013, FR-018.

## 1. Position: an input formalism, not a separate algorithm

Holzmann (§8.10, via plan §2.2) introduces Petri nets as a *restricted* kind of
finite-state machine and concludes that for protocols they give a picture, not
analytical power. The engine follows that conclusion: the Petri frontend
translates a place/transition net into the same intermediate representation the
Promela frontend produces, and the same explorer checks it. What you gain is a
natural notation for token games, resources and causal concurrency; what you do
not gain is any new kind of result.

## 2. The seven claims taken from Holzmann §8.10

1. **Definition.** Places, transitions, arcs place↔transition, a marking (tokens per
   place). A transition is enabled when every input place holds a token; firing
   removes one token from each input place and adds one to each output place.
2. **Conflict.** Transitions sharing an input place are in conflict; exactly one of
   them fires per step. This is interleaving semantics — the same as the engine's.
3. **Marking properties.** *live*: every firing sequence can be extended forever
   (a hang is unreachable); *hang*: no transition is enabled; *safe*: no place ever
   holds more than one token.
4. **Subclasses.** Marked graphs and transition diagrams are special cases. The
   frontend does not special-case them.
5. **Negation is not expressible**: a transition cannot test for the *absence* of a
   token. This is why inhibitor arcs are rejected (§7).
6. **No standard procedures beyond reachability**, because a token mixes three
   roles — a condition, a control point, and a resource. Consequence for you: before
   decoding a counterexample, ask the user what a token *in this place* stands for.
7. **FIFO nets** (queues attached to the net) are as powerful as communicating
   finite-state machines but give no better procedures. A net that needs message
   queues is better written in the Promela subset directly.

## 3. Translation to IR (plan §2.2, §5.3)

| Net element | IR element |
|---|---|
| place `p` with capacity `cap` (default 255) | variable `p` with domain `0..cap` |
| initial marking | initial state |
| transition `t` with inputs `(p_i, m_i)` and outputs `(q_j, n_j)` | guarded command: guard `∧_i p_i ≥ m_i`; effect `p_i -= m_i; q_j += n_j` executed atomically |
| firing of `t` | one step of the explorer; the atom `fire(t)` is true in the state reached by that step (how the frontend exposes the atom is fixed in G0; use it as the tool documents it) |
| multiplicity `m > 1` | decrement/increment by `m` (an extension of Holzmann's one-token arcs; plan §5.3) |
| capacity check | generated always: an effect that would push a place above its capacity is reported as `invalid-model`, never as `violated` (plan §11) |

The corpus encodes the same translation by hand in Promela: `App_C/petrinet1`
uses `#define inp1(x) (x>0) -> x--`, `#define out1(x) x++` and one `atomic`
option per transition inside `init`. Keep that file in mind when a user asks "what
does the engine actually check": the JSON is exactly that model without the
macros. The order of `places` and `transitions` in the JSON fixes the engine's
deterministic transition order (NFR-006), so keep the user's order.

### Example: `petrinet1` as JSON (validates against `assets/petri-net.schema.json`)

```json
{
  "name": "petrinet1",
  "places": [
    {"id": "p1", "initial": 1}, {"id": "p2", "initial": 0}, {"id": "p3", "initial": 0},
    {"id": "p4", "initial": 1}, {"id": "p5", "initial": 0}, {"id": "p6", "initial": 0}
  ],
  "transitions": [
    {"id": "t1", "inputs": [{"place": "p1"}], "outputs": [{"place": "p2"}]},
    {"id": "t2", "inputs": [{"place": "p2"}, {"place": "p4"}], "outputs": [{"place": "p3"}]},
    {"id": "t3", "inputs": [{"place": "p3"}], "outputs": [{"place": "p1"}, {"place": "p4"}]},
    {"id": "t4", "inputs": [{"place": "p4"}], "outputs": [{"place": "p5"}]},
    {"id": "t5", "inputs": [{"place": "p1"}, {"place": "p5"}], "outputs": [{"place": "p6"}]},
    {"id": "t6", "inputs": [{"place": "p6"}], "outputs": [{"place": "p4"}, {"place": "p1"}]}
  ]
}
```

Manual check (plan §2.2), which you can repeat for the user without any tool: at
`p1 = p4 = 1` the enabled transitions are `t1` and `t4`; after `t1` then `t4` the
marking is `p2 = p5 = 1` and no transition is enabled — a hang. Expected engine
result for the `deadlock` property: `violated`, evidence `exhaustive`,
counterexample `t1, t4`. This is also eval E3.

## 4. Mapping the three marking properties to engine properties

| Petri notion | Engine property | Logic | Fairness |
|---|---|---|---|
| hang reachable? | `deadlock` | — (finite witness) | none needed |
| live (Holzmann) = hang unreachable | `deadlock` read positively: `verified` on `deadlock` means live | — | none needed |
| safe | `invariant`: every place `≤ 1` | invariant | none needed |
| bounded by `k` | `invariant`: every place `≤ k`; capacity overflow is `invalid-model` | invariant | none needed |
| transition `t` dead (never fires) | `reach` on `fire(t)`: unreachable ⇒ dead | reachability | none needed |
| transition `t` can always fire again | `ctl`: `AG EF fire(t)` | CTL | **none** — by construction |
| transition `t` fires infinitely often on every run | `ltl`: `[]<> fire(t)` | LTL | **required** — ask before running |

The last two rows are the distinction the plan insists on (§2.2, §5.3) and the one
users most often blur. `AG EF fire(t)` says: from every reachable marking there is
*some* continuation in which `t` fires again — a statement about possibility,
independent of how transitions are scheduled.
`[]<> fire(t)` says: on *every* infinite run `t` fires infinitely often — a
statement about obligation, false whenever a conflicting transition can win the
conflict forever; it becomes meaningful only after a fairness assumption excludes
those runs (`fairness.md`). Present both to the user as two different questions,
run the one they mean, and never report a CTL answer to an LTL question or the
reverse (FR-007).

Note 04 гл. 3 mentions Murata's liveness ladder L0–L4. The engine maps only the two
ends that the plan defines: L0 (dead) ⇔ `fire(t)` unreachable, and "can fire again
from every reachable marking" ⇔ `AG EF fire(t)`. Do not label a result with an
intermediate Murata level; the engine does not compute it.

## 5. Standard property set the frontend generates (plan §5.3)

On `mc_parse` of a Petri JSON the frontend proposes: `deadlock` (hang), `invariant`
"every place ≤ 1" (safe), and the capacity check (always on, reported as
`invalid-model`). On request it adds per-transition liveness in the CTL form or the
LTL form — the LTL form only after the fairness question. Confirm the set with the
user; a net where a place legitimately holds two tokens will fail "safe" without
being wrong.

## 6. Coloured and hierarchical nets: translation with an explicit loss list

CPN/HCPN (04 гл. 3) are translated to a plain P/T net before the JSON is written.
Every translation step loses something; the report lists each loss under
Limitations (plan §2.2; FR-018 direction and preservation).

| CPN feature | Translation | Loss / condition |
|---|---|---|
| colour set on a place | one P/T place per colour value (`p_red`, `p_blue`, …) — the marking is the count per colour | exact if the colour set is finite (feasible only if it is small); an infinite set must be cut, and a cut is an **under-approximation**: `verified` on the cut net does not transfer to the original |
| binding of arc variables | one P/T transition per binding that satisfies the guard | the number of transitions is the product of the domains; the estimate (`mc_estimate`) decides feasibility |
| guard | folded into the binding enumeration (a binding that fails the guard yields no transition) | exact |
| arc expression yielding a multiset | one arc per colour with the multiset count as multiplicity | exact for constant multisets; data-dependent expressions need binding enumeration |
| hierarchy (substitution transitions, pages) | flattening: subpage contents inlined, port/socket pairs merged into compound places | exact behaviour, but module boundaries and instance identity disappear from the trace; the mapping table must record page and instance for `mc_explain` |
| timed CPN (timestamps, delays) | time dropped | this is a change of semantic class (`model-classification.md` §2): only acceptable if the user confirms the property does not depend on time; otherwise `not-executed`, route to timed model checking |
| CPN ML code segments | not translatable | `not-executed` unless the user rewrites the code as a finite function over the colour sets |

If the loss list contains an under-approximation or a dropped time semantics,
the status of a `verified` property is reported with the loss named in the same
sentence.

## 7. Inhibitor arcs are rejected

An inhibitor arc enables a transition only when a place is *empty*. The engine's
Petri frontend rejects a net that contains one, with this explanation: the accepted
formalism is Holzmann's basic place/transition net, in which negation — a test for
the absence of a token — is not expressible (claim 5). A net that needs the test is
not a P/T net in that sense, and the JSON schema has no way to write it
(`assets/petri-net.schema.json` closes every object with `additionalProperties:
false` and has no inhibitor field). The honest alternative is to write the system
in the Promela subset, where a guard `p == 0` is ordinary and the same explorer
checks it; offer that rewrite, and note in the report that the result is then
about a guarded-command model, not about a Petri net.

## 8. Second corpus model: `petrinet2`

`App_C/petrinet2` is two symmetric halves (places `P*`/`p*`, `RC/CC/RD/CD` and their
lower-case twins) that exchange tokens. The plan makes it a golden test after the
first agreed run of engine and SPIN (§2.2, §8.1). Until that golden exists, do not
quote a verdict for it from memory.
