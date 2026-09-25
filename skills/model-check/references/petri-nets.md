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
FR-003, FR-013, FR-018. Engine as built: `model-check-plugin/engine/frontend/petri/`
(schema and translation), `model-check-plugin/steps/g0-confirmation.md` §1, §3.1
(the `petrinet1`/`petrinet2` results and their reconciliation with `pan`).

## 1. Position: an input formalism, not a separate algorithm

Holzmann (§8.10, via plan §2.2) introduces Petri nets as a *restricted* kind of
finite-state machine and concludes that for protocols they give a picture, not
analytical power. The engine follows that conclusion: the Petri frontend
translates a place/transition net into the same intermediate representation the
Promela frontend produces (G1), and the same explorer checks it. What you gain
is a natural notation for token games, resources and causal concurrency; what you
do not gain is any new kind of result.

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

## 3. The JSON input and its translation to IR (plan §2.2, §5.3; engine G0)

`assets/petri-net.schema.json` is a **byte-for-byte copy** of the engine's
`engine/frontend/petri/schema.json` (a Cucumber scenario fails the build if they
drift). The keys that matter:

| Key | Meaning | Default |
|---|---|---|
| `places[].name` | identifier; doubles as the IR variable name | required |
| `places[].initial` | initial marking | 0 |
| `places[].capacity` | maximum tokens, 1…255 | 255 |
| `places[].line`, `transitions[].line` | optional source line for the mapping back to the user's document | — |
| `transitions[].name` | identifier; appears verbatim as `command` and `origin.name` in counterexamples | required |
| `transitions[].inputs[]`, `outputs[]` | arcs `{place, weight}`; a transition without inputs is always enabled | `[]` |
| arc `weight` | tokens moved along the arc, ≥ 1 | 1 |
| input arc `inhibitor` | recognised **only to be rejected** with the arc named (§7) | false |

Every object is closed (`additionalProperties: false`); an unknown key such as `id`
or `multiplicity` is a schema error (exit code 2, `kind` `schema`, `path` to the key).

| Net element | IR element |
|---|---|
| place `p` with capacity `cap` | global `byte` variable `p`; when `cap` < 255 the domain is narrowed to `0..cap` |
| initial marking | initial values of the variables, no steps (the corpus Promela encoding sets the marking in two statements — hence its +2 in §3.1; an encoding with initialisers would differ by 0) |
| transition `t` with inputs `(p_i, w_i)` and outputs `(q_j, v_j)` | one edge of the single process `init`, self-loop on location `do`: guard `∧_i p_i ≥ w_i` (written `p > 0` for weight 1), effect: the decrements, then the increments — one indivisible step |
| firing of `t` | one step of the explorer; in a report it is `steps[].command` = `t` and `origin.name` = `t` |
| the atom `fire(t)` for per-transition properties | **The G0 frontend exposes no `fire(t)` atom.** Properties in the IR are expressions over variables (places), and the frontend generates only `deadlock` and `safe` (§5). A transition-firing atom needs either a frontend-generated history variable or an event-based property kind; that decision belongs to G4 (LTL, `progress`) and is recorded there. Until then, "does `t` ever fire?" is answered by reading the reachable markings: `t` is dead iff the `reach` property "all input places of `t` hold their weights" is `violated` — and that `reach` property has to be authored in IR today, so tell the user it is a manual step |
| capacity check | generated always: an effect that would push a place above its capacity ends the run with `invalid-model` for every undecided property, never with `violated` (plan §4.1, §11) |

The corpus encodes the same translation by hand in Promela: `App_C/petrinet1`
uses `#define inp1(x) (x>0) -> x--`, `#define out1(x) x++` and one `atomic`
option per transition inside `init`. Keep that file in mind when a user asks "what
does the engine actually check": the JSON is exactly that model without the
macros. The order of `places` and `transitions` in the JSON fixes the state-vector
layout and the engine's deterministic firing order (NFR-006), so keep the user's
order.

### 3.1. `petrinet1` as JSON — the E3 fixture (`evals/fixtures/petrinet1.json`)

```json
{
  "name": "petrinet1",
  "places": [
    {"name": "p1", "initial": 1}, {"name": "p2"}, {"name": "p3"},
    {"name": "p4", "initial": 1}, {"name": "p5"}, {"name": "p6"}
  ],
  "transitions": [
    {"name": "t1", "inputs": [{"place": "p1", "weight": 1}], "outputs": [{"place": "p2", "weight": 1}]},
    {"name": "t2", "inputs": [{"place": "p2"}, {"place": "p4"}], "outputs": [{"place": "p3"}]},
    {"name": "t3", "inputs": [{"place": "p3"}], "outputs": [{"place": "p1"}, {"place": "p4"}]},
    {"name": "t4", "inputs": [{"place": "p4"}], "outputs": [{"place": "p5"}]},
    {"name": "t5", "inputs": [{"place": "p1"}, {"place": "p5"}], "outputs": [{"place": "p6"}]},
    {"name": "t6", "inputs": [{"place": "p6"}], "outputs": [{"place": "p4"}, {"place": "p1"}]}
  ]
}
```

Manual check (plan §2.2), which you can repeat for the user without any tool: at
`p1 = p4 = 1` the enabled transitions are `t1` and `t4`; after `t1` then `t4` the
marking is `p2=p5=1` and no transition is enabled — a hang.

Engine result (`mcd check --no-timing --petri petrinet1.json`, `steps/g0-confirmation.md`
§1, §3.1): property `deadlock` — `violated`, evidence `exhaustive`, `complete` true,
`counters.states` 6, `counters.transitions` 8, `counters.depth` 2, counterexample
`summary` = `t1, t4`, `final_state` non-zero entries `p2=1 p5=1`; property `safe` —
`verified`, `exhaustive`. This is eval E3.

Reconciliation with SPIN on the reference encoding `App_C/petrinet1` (same source,
§3.1): `pan -c0` stores **8** states, the engine **6**; the difference is exactly +2,
the two states `init` passes through while executing `p1 = 1; p4 = 1` as
statements, whereas the JSON gives the initial marking as initial values. The
verdict class (invalid end state = hang) and the first witness (`spin -t`: lines 15
and 18 = `t1`, `t4`) agree. `pan` reports depth 3 and `mcd` depth 2; how `pan`'s depth
counter accounts for the two init statements was not analysed in G0, so quote both
numbers without an explanation of their difference. When a user compares with a SPIN
run, explain the +2 in the state count before anything else.

## 4. Mapping the three marking properties to engine properties

| Petri notion | Engine property | Logic | Fairness | Available |
|---|---|---|---|---|
| hang reachable? | `deadlock` | — (finite witness) | none needed | G0 |
| live (Holzmann) = hang unreachable | `deadlock` read positively: `verified` on `deadlock` means live | — | none needed | G0 |
| safe | `invariant` `safe`: every place `≤ 1` | invariant | none needed | G0 (generated) |
| bounded by `k` | `invariant`: every place `≤ k` (authored in IR today); capacity overflow is `invalid-model` | invariant | none needed | G0 (IR), frontend option later |
| transition `t` dead (never fires) | `reach` on "inputs of `t` hold their weights" — unreachable ⇒ dead (§3: no `fire(t)` atom yet) | reachability | none needed | G0 (IR), atom in G4 |
| transition `t` can always fire again | `ctl`: `AG EF fire(t)` | CTL | **none** — by construction | G5 |
| transition `t` fires infinitely often on every run | `ltl`: `[]<> fire(t)` | LTL | **required** — ask before running | G4 |

The last two rows are the distinction the plan insists on (§2.2, §5.3) and the one
users most often blur. `AG EF fire(t)` says: from every reachable marking there is
*some* continuation in which `t` fires again — a statement about possibility,
independent of how transitions are scheduled.
`[]<> fire(t)` says: on *every* infinite run `t` fires infinitely often — a
statement about obligation, false whenever a conflicting transition can win the
conflict forever; it becomes meaningful only after a fairness assumption excludes
those runs (`fairness.md`). Present both to the user as two different questions,
run the one they mean — today neither can be run, and the answer is `not-executed`
with the kind named (`engine-tools.md` §5) — and never report a CTL answer to an
LTL question or the reverse (FR-007).

Note 04 гл. 3 mentions Murata's liveness ladder L0–L4. The engine maps only the two
ends that the plan defines: L0 (dead) ⇔ `fire(t)` unreachable, and "can fire again
from every reachable marking" ⇔ `AG EF fire(t)`. Do not label a result with an
intermediate Murata level; the engine does not compute it.

## 5. Standard property set the frontend generates (plan §5.3; G0)

On `mcd parse --petri` / `mcd check --petri` the frontend generates two properties
and puts them in every report, in this order: `deadlock` (kind `deadlock`, text
"hang (Holzmann §8.10): a reachable marking in which no transition is enabled") and
`safe` (kind `invariant`, "every place holds at most one token in every reachable
marking"). The capacity check is not a property: exceeding a capacity ends the run
with `invalid-model`. Per-transition liveness in the CTL or LTL form is not
generated yet (§4). Confirm the set with the user before reporting; a net where a
place legitimately holds two tokens will fail `safe` without being wrong, and then
the honest report says "`safe` is `violated` and that is by design of the net", not
"the net is wrong".

## 6. Coloured and hierarchical nets: translation with an explicit loss list

CPN/HCPN (04 гл. 3) are translated to a plain P/T net before the JSON is written.
Every translation step loses something; the report lists each loss under
Limitations (plan §2.2; FR-018 direction and preservation).

| CPN feature | Translation | Loss / condition |
|---|---|---|
| colour set on a place | one P/T place per colour value (`p_red`, `p_blue`, …) — the marking is the count per colour | exact if the colour set is finite (feasible only if it is small); an infinite set must be cut, and a cut is an **under-approximation**: `verified` on the cut net does not transfer to the original |
| binding of arc variables | one P/T transition per binding that satisfies the guard | the number of transitions is the product of the domains; a small `--budget-states` run (`engine-tools.md` §1, `mc_estimate` row) decides feasibility |
| guard | folded into the binding enumeration (a binding that fails the guard yields no transition) | exact |
| arc expression yielding a multiset | one arc per colour with the multiset count as `weight` | exact for constant multisets; data-dependent expressions need binding enumeration |
| hierarchy (substitution transitions, pages) | flattening: subpage contents inlined, port/socket pairs merged into compound places | exact behaviour, but module boundaries and instance identity disappear from the trace; record page and instance in the place and transition names so `origin.name` still says where a step came from |
| timed CPN (timestamps, delays) | time dropped | this is a change of semantic class (`model-classification.md` §2): only acceptable if the user confirms the property does not depend on time; otherwise `not-executed`, route to timed model checking |
| CPN ML code segments | not translatable | `not-executed` unless the user rewrites the code as a finite function over the colour sets |

If the loss list contains an under-approximation or a dropped time semantics,
the status of a `verified` property is reported with the loss named in the same
sentence.

## 7. Inhibitor arcs are rejected

An inhibitor arc enables a transition only when a place is *empty*. The schema has
the input-arc flag `inhibitor` for one reason: so that the frontend can name the
offending transition when it refuses the net. `mcd` exits with code 2 and
`{"error": {"kind": "unsupported-input", "path": "transitions", "message": …}}`;
the message explains that the accepted formalism is Holzmann's basic
place/transition net, in which negation — a test for the absence of a token — is
not expressible (claim 5). Setting `inhibitor` on an output arc is a schema error.
The honest alternative is to write the system in the Promela subset
(`promela-subset.md`, `--promela`), where a guard `p == 0` is ordinary and the same
explorer checks it; offer that rewrite, and note in the report that the result is
then about a guarded-command model, not about a Petri net. As a Petri net the
properties stay `not-executed` with the rejection message as reason.

## 8. Second corpus model: `petrinet2` — golden since G0

`App_C/petrinet2` is two symmetric halves (places `P*`/`p*`, `RC/CC/RD/CD` and their
lower-case twins) that exchange tokens. Its engine result is pinned as a golden
file (`engine/testdata/golden/petrinet2.report.json`) after the reconciliation with
`pan` in `steps/g0-confirmation.md` §3.1: 20 states (`pan` stores 22 = 20 + 2, the
same two init steps as for `petrinet1`), `deadlock` `violated` after `T1, t1` with
marking `P2=1 RC=1 p2=1 rc=1`, `safe` `verified`. Quote those numbers from the
golden file or a fresh run, not from memory, and expect the first witness to match
`spin -t` only as an observation (the agreement of the first witness was checked
on the two corpus nets and is not a guarantee of the engine).
