# Model classification (FR-003)

Sources: `model-check-skill-notes/10-cross-book-synthesis.md` §4 (taxonomy of
models, four axes); `model-check-skill-notes/11-skill-requirements.md` §6 steps 3–4
and §7 (normal form, finite/infinite), FR-003; `model-check-skill-notes/14-skill-building-plan.md`
§4 (engine scope), §5 (input formats), §12 A7.

FR-003 asks for two classification bases: **finiteness** (finite / potentially
infinite) and **semantic class** (untimed / timed / probabilistic). The synthesis note
adds descriptive axes that decide the input formalism. Classify on all of them; the
axes are independent features, not a single partition, so one model can be
"finite, untimed, asynchronous process network, hand-written abstraction".

## 1. Finiteness

| Finding | Class | Consequence |
|---|---|---|
| Every variable has a bounded domain (`bit`, `bool`, `byte`, `short`, `int` with the engine's range checks), process count fixed or bounded, every channel and place has a capacity | finite | exhaustive search is possible in principle |
| Unbounded integer, unbounded queue, unbounded recursion, dynamic process creation without a bound, a growing heap, dense time | potentially infinite | either introduce a visible bound (result then concerns the bounded model) or exit `not-executed` |

Two things are often mistaken for infinity and are not: a `byte` counter that "could
grow" is finite — it wraps or, in this engine, overflows into `invalid-model`; a
process that runs forever has finitely many states if its variables are bounded.
Two things are often mistaken for finiteness and are not: `run` inside a loop without
a bound on `_nr_pr`; a channel declared with a large capacity that the estimate shows
is never filled — that is finite but may be too large for the budget (node 6 of the
workflow), which is a different problem.

## 2. Semantic class

| Finding | Class | Engine |
|---|---|---|
| Nondeterministic choice, interleaving or synchronous steps, no clocks, no probabilities | untimed | supported |
| Clocks, deadlines in time units, "within 5 seconds", urgency | timed | not supported → `not-executed`, route to timed model checking |
| Probabilities of transitions, expected values, "with probability ≥ 0.999" | probabilistic | not supported → `not-executed`, route to probabilistic model checking |

Boundary cases decided by the requirement, not by the model text (10 §4, §13):

- A protocol with timeouts is untimed if the requirement is about the fact of a
  global block or a retransmission, and Promela's `timeout` (true only when nothing
  else is enabled) is the right abstraction. It is timed only if the requirement
  quantifies time.
- "Rarely" or "usually" in the requirement is a probability only if the user wants a
  number; if they want "never", it is untimed safety.

## 3. Form of state and transition (decides the input formalism)

| Form | Recognise by | Route |
|---|---|---|
| Asynchronous process network | processes, messages, channels, shared variables, "one process moves at a time" | Promela subset (`promela-subset.md`) |
| Program graph / EFSM | control locations plus finite data, guards and effects | Promela subset; one `proctype` per automaton, labels for locations |
| Petri net / token net | places, transitions, tokens, marking, "fires when all inputs have a token" | Petri JSON (`petri-nets.md`) |
| Coloured / hierarchical Petri net (CPN, HCPN) | typed tokens, arc expressions, guards, pages/modules | Petri JSON after a translation with an explicit loss list (`petri-nets.md` §6) |
| Synchronous system | all components update in one round, `init/next` style | encodable in the Promela subset only with an explicit scheduler variable and a round structure; otherwise `not-executed` (SMV export is vNext) |
| Automata program (UniMod-like) | event → transition → actions, nested automata | Promela subset after deciding the atomic step and the phase mapping (09 гл. 4); the report names the mapping |
| Kripke structure / LTS given explicitly | a state list and a transition list | IR written directly (experimental, plan A7) or a Promela `init` with `goto` |

## 4. Nature of transitions

| Finding | Class | Engine |
|---|---|---|
| Nondeterministic (environment, scheduler) | supported: both universal (LTL, `A…`) and existential (`E…`) properties |
| Probabilistic (Markov chain, MDP) | see §2 → `not-executed` |
| Timed automaton, hybrid | see §2 → `not-executed` |
| Game (two players) | not supported → `not-executed`; route to synthesis tools |

## 5. Relation to the implementation

Record it; it decides how the report's Limitations section is worded.

| Relation | Wording constraint |
|---|---|
| Hand-written abstract model | result holds for the model; the mapping to the system is the user's argument |
| Automatically extracted model (e.g. from C, corpus `CH10/fahr.c` → `fahr.pml`) | trust moves to the extractor; name it |
| Model built by the skill from prose | the mapping table is part of the deliverable and must be confirmed by the user |

## 6. Decision table (exclusive branches)

Evaluate in this order; take the first row that matches.

| # | Condition | Action |
|---|---|---|
| 1 | timed or probabilistic (§2) | `not-executed`, route |
| 2 | potentially infinite (§1) and user refuses a bound | `not-executed`, route |
| 3 | potentially infinite and a bound is accepted | continue with the bound as a visible parameter |
| 4 | Petri/token net (§3) | Petri JSON route |
| 5 | CPN/HCPN | translate with loss list, then Petri JSON route |
| 6 | synchronous system without scheduler encoding | `not-executed` (SMV export vNext) or ask to encode the round explicitly |
| 7 | everything else finite and untimed | Promela subset route |

Rows 1–2 exit, row 3 continues, rows 4–7 pick the formalism. A model that hits row 3
and row 4 (an unbounded place with an accepted bound) goes through row 3 first: the
bound becomes the place capacity in the JSON.

## 7. What to write down

For every model: `M = (S, S0, R, AP, L)` in words — state variables with domains and
encoding, initial states, one sentence per transition class, the atomic step, the
composition, channel capacities and FIFO order, deadlock/terminal semantics (`end`
labels), the source-name mapping. This is the "Model" section of the report and the
input to `mc_parse` warnings review.
