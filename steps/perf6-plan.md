# Performance plan, step 6 — a wider partial-order reduction: the plan

Layer: G0 (`explore`; `cli`/`report` only for strings). Follows `perf2-confirmation.md` (the
reduction) and `perf4-confirmation.md` (channel ends). Protocol: `BUILD-PROTOCOL.md` step 6.
This is **phase 1 of the stream: a plan and its cross-review. No engine code is changed by this
document.** (Carried out in phase 2: what was built, measured and decided is in
`perf6-confirmation.md`, with the defaults taken for the open questions of section 11.)
The prototypes mentioned below were throwaway copies of the engine kept outside
the repository; they exist to measure and to find traps, not as an implementation.

Status of the numbers: measured on the 0.2.0 sources (commit `738627b`, docs-only on top of
`8703982`), one machine, load noted where timings appear. Figures taken from the throwaway
prototypes are *indications of what a correct reduction can reach*, not results: until the
rules below are implemented and pass the oracle of §7 they prove nothing about soundness.

## 0. Decisions in one table

| Candidate | Decision | Why, in one line |
|---|---|---|
| **A. atomic sequences** | **do** (first) | 8 corpus models refused for it alone, among them `bench-sym` (76 516 states -> about 5 500 in the prototype, 27x at N=6); the argument is the `d_step` closure plus one new lemma (the exclusive byte). |
| **R. process creation (`run`) and the process table (`_nr_pr`, `youngest`, `pid`)** | **do** (second) | `init { atomic { run ... } }` is the idiom of Promela; with A it unlocks the ring of `leader3` (679 -> 76 states; the same ring unrolled to N=6: 341 316 -> 136) and every model started by `init`. One new cell (the table) and one static check; no new search code. |
| **P. `provided`** | **do** (third, small) | in this engine it is a pure guard on every edge of the process (`enabled()`), so it is a read added to every edge's footprint; 3 models gain "applied", none shrinks; kept because the refusal text ("a priority") is not what the engine does. |
| D. dynamic channels (`chan` parameters, `Sel`) | **postpone**, design recorded (§6.2) | eight small or atypical models contain the construct and one of them matters (`leader`, 41 692 states; about 100 once its channels are static); it needs a *state-dependent* dependence relation: a new kind of argument and a new oracle, larger than A+R together. |
| V. rendezvous (capacity 0) | **reject for now** (§6.1) | measured on the 28 files with a rendezvous channel: every channel is either a semaphore shared by all processes or a private pair of the only two processes; no model has two independent pairs. Largest proof (a joint transition, a group ample set), no measured gain. |
| T. `timeout` | **postpone**, argument recorded (§6.3) | sound in principle (timeout moves are only enabled where no phase-0 move is, and the ample set stays enabled), but `movesOf` has a trap (it would enter the timeout phase for one process) and the 3 files that use it (two copies of one 55-state model, one of 16 states) gain nothing measurable. |
| temporal properties, `--bfs`, vacuity watch | unchanged refusals | step 4 measured that they unlock nothing that needs them; out of scope. |

What stays refused after this stream: rendezvous channels, channels named by a value, `timeout`,
every temporal property, `--bfs`, a vacuity watch. Everything else the corpus uses is covered.

## 1. Baseline: what is refused, and what it would be worth

### 1.1 The corpus test, with the reasons

`go test -run TestPORAgreesWithTheFullSearchOnTheCorpus -v .` from `model-check-plugin/engine`
(0.2.0; run again at `738627b`): 153 Promela files, 101 accepted by the frontend, 86 compared (both
searches complete), **45 with the reduction applied, 16 of them smaller, 41 refused**. The test
logs only the *first* reason of a refusal; the table adds what the models contain (measured with a
throwaway classifier over the IR of the same 101 models: it counts every blocking construct, not
the first).

| Refusal (first reason, 86 models) | Models | Models containing the construct (all 101) |
|---|---:|---:|
| process creation (`run`) | 10 | 12 (3 of them also read `_nr_pr`) |
| rendezvous channel | 9 | 12 |
| atomic sequences | 8 | 13 |
| temporal property (`never`, `accept`, `progress`) | 10 | 10 |
| `provided` | 3 | 3 |
| `timeout` | 1 | 1 |
| channel named by a value (only as a second reason) | 0 | 8 |

Combinations matter more than counts. Blocking sets of the 101 models: 54 none, 8 rendezvous alone,
8 atomic alone, 8 temporal alone, 5 `run` alone, 4 dynamic channels alone, 2 `run`+`_nr_pr`, 2 `provided`
alone, 2 rendezvous+temporal, 2 atomic+`run`, 2 atomic+dynamic channels+`run`, 1 atomic+`provided`,
1 `timeout`, 1 dynamic channels+rendezvous, 1 dynamic channels+`_nr_pr`+rendezvous+`run`.

The blocking-set counts above are over the 101 accepted models. The corpus test's own counts (45 applied,
16 smaller, 41 refused; the "first reason" column) are over the 86 of them that both searches finish: 15
models reach the 200 000-state budget or the 30 s limit and are not compared (nine of those need nothing
and are already reduced or fine). The `run`-alone count of 5 and the 2 `run`+`_nr_pr` models are in the 101,
not all in the 86; the corpus-test deltas in §1.2 (`run` alone: 45 -> 50) are over the 86.

**Size is what the corpus lacks.** Of the 86 compared models only 11 have 100 or more states in the full
search, and 5 of those are refused; the other refused ones are toys (the textbook models of Chapters 2-8
have 1 to 20 states, a reduction cannot save anything visible there). The refused models that are not toys:

| Model | Full states | Refused for |
|---|---:|---|
| `CH15/client_server` | 191 200 | `run`, `_nr_pr`, rendezvous, dynamic channels (4 constructs) |
| `bench-sym` (fixture, N=5) | 76 516 | atomic |
| `CH9/leader` | 41 692 | atomic, `run`, dynamic channels |
| `leader3` (fixture, static copy of `leader`, N=3) | 679 | atomic, `run` |
| `CH5/diskhead` | 217 | temporal property |
| the rest (rendezvous toys, `provided`, `timeout`, `run` toys) | 1 to 56 | one construct each |

A second corpus, the 54 models that the frontend accepts among those the agents of the earlier evaluation
rounds wrote (`evals-workspace`, `steps/agent-tasks`, the skill fixtures, the mutation samples: the nearest
thing to what users send), shows the same picture: 26 need nothing, 5 are refused for atomic alone, 5 for
rendezvous alone, 2 for a temporal property alone and 10 for one with a rendezvous, 3 for atomic with a
temporal property, 2 for atomic with `timeout`, 1 for `run`. 44 of the 54 finish in both searches, 6 of
those have 100 or more states, and only 2 of the 6 are refused (`mutate/sample.pml`, 145 states, atomic;
`mutex_flaw_prog`, 429 states, a progress property). Dynamic channels do not occur in it.

### 1.2 What a correct reduction could save (prototype, outside the repository)

A throwaway copy of the engine with the refusals lifted and the rules of §3-§5 written in the
simplest form (closure over atomic continuations, a chain-following cycle proviso, the table
cell and a static check, `provided` as a read of every edge) was run on the corpus. Verdicts agreed on all 86
models. State counts of the reduced graph (`--sweep --por`; a model that was refused has the full count
today):

| Model | Full | Today `--por` | Prototype |
|---|---:|---:|---:|
| `bench-sym` N=3 / 4 / 5 / 6 (atomic) | 1 348 / 10 420 / 76 516 / 543 076 | same | 369 / 1 471 / 5 522 / 19 914 |
| `bench-sym` N=7 | over 3 000 000 (budget) | same | 69 811, deadlock verified (unconfirmed: no full search finishes to check it against) |
| `leader3` (ring of 3 nodes, atomic + `run`) | 679 | 679 | 76 |
| the same ring unrolled to N=4 / 5 / 6 / 7 | 5 162 / 41 692 / 341 316 / not run | same | 88 / 108 / 136 / 164 |
| `nrpr.pml` (`run`, `_nr_pr`) | 31 | 31 | 21 |
| `you_run2` / `karpov/03-state-race` (`run`) | 14 / 56 | same | 11 / 52 |
| `atomic-at`, `atomic-t1`, `-t3`, `-t4`, `-t6` | 11, 10, 17, 9, 10 | same | 9, 9, 15, 8, 9 |
| `toggle`, `02-provided-toggle`, `pathfinder` (`provided`) | 6, 6, 12 | same | 6, 6, 12 (applied, nothing shrinks) |
| `dijkstra` with rendezvous refused only at the channel operations | 21 | 21 | 15 (the only rendezvous model that shrinks) |

The prototype's first-draft rules also passed the differential oracle and the semantic audit of §7 on
60 000 random models per generator (atomic, run and table, `provided`; 10 500 to 11 500 of each reduced),
and the corpus test on all 86 models. That is calibration of the rules and of the tooling, not
a proof; §7 says what else is required.

Corpus test with the prototype, applied / smaller / refused out of 86: today 45 / 16 / 41; atomic alone
53 / 22 / 33; `run` alone 50 / 19 / 36; `provided` alone 47 / 16 / 39; atomic + `run` 60 / 26 / 26;
atomic + `run` + `provided` 63 / 26 / 23; plus a rendezvous that only protects the processes that use it 71 / 27 / 15
(8 more models "applied", one more shrinks: no value).

Reading: **A and R together turn 15 of the 41 refusals into "applied", and 10 of those shrink**; the
two models that matter are `bench-sym` (14x at N=5, 27x at N=6) and the leader ring (9x at N=3, 386x at
N=5), whose growth with the number of processes becomes linear (about 28 states per node). Nothing else in
the corpus changes by more than a few states. `provided` and rendezvous change nothing that can be seen.

A ring of nodes on named buffered channels is exactly the shape the channel-ends rule of step 4 handles;
what blocked it was the `init { atomic { run ... } }` that starts it. The idiom is therefore the target,
not the toy models that happen to contain it.

### 1.3 What the numbers do not say

- Toy models cannot show a gain, so "models gained" overstates: "applied" is not a benefit, a smaller
  state count is. The ranking uses the second.
- The prototype is unproven. A large gain next to an unreviewed analysis is also what an unsound
  analysis looks like; §7 is the plan for telling them apart before any number is quoted.
- The corpus has no model with an independent pair on a rendezvous channel (§6.1), and one with dynamic
  channels (§6.2); both are rejections by measurement, and a user model of that shape would be
  evidence to reopen them.

## 2. The frame the new rules must fit

The existing reduction (`explore/por.go`) explores in a stored state the moves of one process alone, when
the static analysis proves **isolation**: every edge of the process at its location, with the d_step
continuation of each, (a) reads only cells that no other process ever writes, anywhere, (b) writes only
cells no other process ever reads or writes, (c) writes nothing a property reads and carries no assert;
plus the dynamic requirement on buffered-channel operations (`channelsCanAct`) and the cycle proviso on the
DFS stack. A cell is a global (an element for a constant index), a channel (send end, receive end, or whole),
or the program counter of a process, read "whole" or, for the edges *other* processes see, "at location c".

Why isolation is enough is the lemma the new rules are stated against (this is the textbook C0-C3
argument, restated in the form that the oracle of §7 audits):

> **Lemma I (isolation).** Let *s* be a stored state, *p* a process, *T* all macro-steps of *p* at *s*
> (a macro-step is the sequence of micro-moves a process makes from a stored state to the next stored
> state: one edge, or the edge and its d_step continuation, or — new — an atomic sequence). If *T*'s
> footprint (the union over every edge that can be part of a step of *p* at its location) is isolated as
> above, then for every path *s* -> *s1* -> ... -> *sk* of macro-steps of processes other than *p*:
> (1) the steps of *p* at *sk* are exactly those at *s* (the same edge sequences); (2) each of them
> commutes with the path: executing it at *sk* gives the state that executing the path after it gives (up
> to the exclusive byte, §3.3); (3) no step of the path is an error, a failed assert or a change of a
> property's value that is not also one after *p* moved first.
> Proof by induction on *k*: a step of another process writes no cell *p*'s footprint reads (so (1)), reads
> no cell it writes, and writes none of the same (so (2)); the footprint contract below makes "the cells an
> edge touches" mean *every* cell on which its enabledness, its effect, its chain shape or its error
> depend.

**Footprint contract (F).** For each construct the plan adds, the footprint must contain every cell on
which (F1) whether the edge is enabled, (F2) what the step writes, (F3) whether it errs, and (F4) where
a chain (d_step or atomic) continues, depend; and every cell the step writes. Each rule below is one
instance of F, and each mutant of §7.4 removes one clause of one instance.

Lemma I gives C1 and C2 of the textbook (the dependent transitions of *T* are *p*'s own steps and steps
that conflict; neither can occur first). C0 is non-emptiness, C3 the proviso. The reduced graph then
preserves what the safety properties ask: every reachable stored state without an enabled move, every
reachable error and failed assert, the truth of every invariant and reach expression on reachable states.

## 3. Feature A: atomic sequences

### 3.1 Semantics in this engine (read from `ir`, `explore.fire`, `startIter`, `dfs`)

- `Edge.Atomic` on an edge means "the process keeps exclusive control after this edge" (it is not the
  last statement of its `atomic {}`); `apply` writes `Excl = p+1` after such an edge and `Excl = 0` after
  any other (rendezvous partners excepted: refused). The byte is part of the state vector.
- `startIter`: if `Excl != 0` and the holder has an enabled edge, only the holder's moves are enabled;
  if the holder is blocked, exclusivity is lost and every process may move (the byte stays set).
- `dfs`: a successor that is **intermediate** (`Excl != 0` and the holder has an enabled edge) is not
  stored: it is pushed on a temporary frame and expanded with the holder's moves only; the first
  successor that is not intermediate is stored. So a stored state has `Excl = 0` or a blocked holder, and
  invariants and reach conditions are checked on stored states only. Asserts are checked on every edge.
- Nothing in `pick`, `movesOf` or `leavesStack` knows about this: `pick` runs on stored frames only
  (`top.idx >= 0`), and `leavesStack` today answers "not leaving the stack" (expand in full) whenever the
  successor has `Excl != 0`.

### 3.2 The unit of reduction: the macro-step

From a stored state a process's **macro-step** is a maximal chain `e1; e2; ...; ek`: `e1` enabled at the
stored state, `e_i` (i < k) an edge that is `Atomic` (or `DStep`) with the holder having an enabled
`e_(i+1)` out of its target, `e_k` not atomic or with the holder blocked after it. The search of the
engine already is a search over macro-steps (intermediate states are not stored). Branching inside the
chain gives several macro-steps with the same first edge; each is a transition of the macro system.

### 3.3 The exclusive byte (the one new lemma)

Two macro-steps that commute can leave different values in the `Excl` byte: the byte is written by every
step (0, or `q+1` if the step ends blocked inside an atomic sequence), so the last step decides it. Then
the two orders reach two *different* stored states which differ only in that byte.

> **Lemma E.** Let *s*, *s'* be stored states that differ only in `Excl`. Then they have the same enabled
> moves, the same effects, the same truth of every property, the same `allTerminated`, and the same
> successors up to `Excl`. Hence "equal up to `Excl`" is a bisimulation on stored states.
> Proof: `Excl` is read in two places only (`grep Excl explore/`): `startIter`, to restrict the moves to a
> holder that has an enabled edge, and `intermediate`, which is applied to the successor a step has just
> produced, whose byte that step wrote. A stored state has no holder with an enabled edge (else it would
> be intermediate), and whether the holder has one is a function of the other bytes. `apply` overwrites
> the byte.

Consequences.

(i) **The standard proof is run on exact states, and Lemma E transports the path.** The textbook induction
takes a path of the full graph from the reduced state *s*, moves a step of the ample set to the front by
commutation, and continues from the successor in the reduced graph. Here a commutation ends in a state that
may differ from the one the original order reaches only in the `Excl` byte; by Lemma E the rest of the path
can be executed from that state with the same steps and the same effects, so the induction goes on and the
reduced path ends in a state equal to the full path's up to `Excl`. All the states of the reduced graph are
exact states (a reduced run only ever fires real moves).

(ii) **C3 is a property of the exact reduced graph, and the proviso compares exact states.** Every cycle of
the exact reduced graph contains a fully expanded state: the stack check of `leavesStack` and the
back-edge argument of a depth-first search give it, and an infinite run of ample steps in a finite graph
repeats an exact state, which is such a cycle. (An earlier draft of this plan argued C3 in the quotient by
`Excl`; that argument is wrong, because the process `pick` chooses at a variant of a state can differ from
the one chosen at the state itself, so the quotient of the reduced graph is not a reduced graph with ample
sets. The exact route needs no such claim.) The oracle O3 of §7.1 checks the exact graph.

(iii) **The oracle must compare states up to `Excl`** (the set of states without an enabled move, in
particular); it keeps comparing "reduced states are a subset of full states" exactly, which is stronger and
still true. This is not a theoretical worry: with the prototype's reduction, whose verdicts all agree, an
exact comparison of the sets of states without an enabled move fails in 23 of 3 000 random atomic models
and a comparison up to `Excl` in none.

(iv) Lemma E is **load-bearing**: with the byte written by every step, no static restriction short of
"no macro-step of any process can end blocked inside an atomic sequence" gives exact commutation (an
ample step that ends with `Excl = 0` and another process's step that ends blocked inside its sequence leave
different bytes in the two orders, whatever the ample process does). See the fallback in §10.

### 3.4 The rule

1. **Closure.** `closure(p, e)` follows the edges out of the target while the edge taken carries `DStep`
   *or `Atomic`*; the footprint of a macro-step is the union over all edges that can occur in it, enabled
   or not, as for d_step. (`por.go` `closure`.)
2. **"Whole" reading.** A location entered by an `Atomic` edge is added to `whole` like one entered by a
   `DStep`: which continuation is taken is decided when the macro-step is made, so a guard on a program
   counter there reads the counter whole (the d_step trap of the first cross-review: under the refinement
   "j at c" a step of *j* into *c* changes the *shape* of a chain although the guard's edge is never
   co-enabled with it).
3. **Channels.** `dirOK` (the ends of a buffered channel as separate cells) is false for an `Atomic` edge
   and for the edges of a location it enters: a continuation `c!x` is enabled by the receiver's pop (or
   blocked by its absence), which changes where the chain ends, so the send is not independent of the pop
   there. Reads of lengths and clears stay whole-channel cells. (Conservative: the first edge of a chain
   could keep the directed rule; not worth a second argument.)
4. **Asserts and visibility.** An assert or a visible write on *any* edge of the closure makes the location
   ineligible (`own` closure merge already does it for d_step). Invariants are not checked inside a
   sequence, but the macro-step as a whole must not change a property's truth, so a visible write in the
   middle that is restored at the end is still treated as visible: conservative, and it keeps the audit of
   §7.2 simple.
5. **Cycle proviso over chains.** `leavesStack` must follow each move through the intermediate states to
   the stored states it can end in (all branches, depth-first, on a private buffer; `s.cur` saved and
   restored) and check each against `visited` and the stack. The stored states are compared
   **exactly** (§3.3 (ii)). A chain longer than a fixed limit (the d_step limit, 100 000 micro-steps), a
   total number of micro-steps explored in one `pick` above a budget (a wide `if` inside an atomic block must
   not make `pick` dominate the search), an error, or a failed assert counts as "not leaving": full
   expansion. A test must show that exhausting either limit gives full expansion, not a reduced state, a
   hang or unbounded allocation. This replaces the `s.next[Excl] != 0` shortcut, which is the only place that
   mentions atomic.
6. **Stored states with `Excl != 0`.** `pick` runs there unchanged (Lemma E).
7. The dynamic requirement `channelsCanAct` is unchanged: it concerns the edges of the location only.

### 3.5 The soundness argument, for a reviewer to attack

(a) Every macro-step of *p* at *s* is a chain of edges of *p*, each an out-edge of the previous target
following `Atomic`/`DStep` edges, so its footprint is contained in the closure (rule 1). (b) Whether the
chain continues after `e_i` depends on the guards, channel conditions and `else` siblings of every
out-edge of the target (F4): all are in the closure, read whole for program counters (rule 2) and with
whole-channel cells (rule 3), so by isolation no other process can change them; the chain shape at *sk* is
the one at *s*. (c) The effects commute by Lemma I (2) (the cells written are disjoint from everything
another process reads or writes); `Excl` differs at most, Lemma E. (d) Visibility, asserts: rule 4.
(e) C3: rule 5 and §3.3 (ii) (the exact reduced graph). (f) Errors: the evaluation errors of the chain depend on cells in the
closure (F3); a macro-step that errs when speculatively fired makes `leavesStack` answer "not leaving"
so the state is expanded in full and the ordinary search meets it, as today.

### 3.6 The traps this rule has (each gets a directed test and a mutant, §7)

1. A continuation edge that reads a variable another process writes (the first edge is isolated, the
   second is not): closure missing -> chain shape changes.
2. A continuation guarded by a program counter (`pc(j) == c`) that *j* enters: `whole` missing.
3. A continuation that is a send on a channel whose receiver pops: `dirOK` not cleared.
4. A visible write or an assert in the middle of a sequence.
5. A sequence that blocks in the middle (the holder is blocked and stored with `Excl != 0`), reached by
   two processes in two orders: the quotient (oracle compares up to `Excl`).
6. A macro-step whose end state is on the stack although its first micro-state is not: a cycle through an
   atomic sequence (proviso over chains).
7. An infinite atomic loop: the full search itself does not end; the chain limit must give "full
   expansion", never a hang inside `pick`. (Existing behaviour of the full search is not changed.)
8. An `else` edge at a continuation (enabledness depends on its siblings).
9. An edge that is both `Atomic` and `DStep`.
10. `pick` at a stored state whose holder is blocked: any process may move; the picked one must not be
    treated as exclusive.
11. A d_step that ends in an atomic edge: the exclusive byte is the one the *last* edge of the step
    leaves, so a non-atomic d_step edge can start an atomic chain. This is the one place where the
    closure of rule 1 crosses from the d_step mechanism (`fire` loops over `DStep` edges) to the atomic one
    (an intermediate state): a mutant that follows `Atomic` only after an `Atomic` edge, not after a
    `DStep` one, must be killed. The prototype's generator first built this shape *cyclically*, which makes
    a macro-step that never ends and exhausts memory unless a depth budget is set (the CLI's default depth
    budget stops it; the oracle must set one). The generator keeps the shape and makes it finite: every
    `Atomic` and `DStep` edge goes forward (to a later location). A directed test pairs a finite
    `d_step -> atomic` chain with a cyclic variant under a small chain limit.
12. A cycle of the reduced graph through an atomic chain with several branches, only some of which close it
    (the proviso must look at every branch): a mutant that looks at the first branch only gives the right
    verdicts on every random model of the prototype's generators and is seen only by the acyclicity audit
    O3 (§7.4).

### 3.7 What remains refused

An atomic sequence that contains a rendezvous send or receive (rendezvous stays refused as a whole).

## 4. Feature R: process creation and the process table

### 4.1 Semantics in this engine

- A `run` is an edge with `Run`: the target is `Targets()` (`Proc`, or the ordered `Pool`). `apply` does,
  in this order: writes the creator's counter to the edge's target, sets `Excl`, `Leave`s, clears the listed
  channels, then takes the first pool member that is dormant (`pc == Initial && Dynamic`), evaluates `Args`
  *in the state so far* (so `pc` of the creator is already the target location and a channel just cleared is
  empty; the comment on `RunOp` says "source state") into the target's first locals, applies `Init` in the
  target's scope, sets `pc(q) = Entry`, `Enter`s the table, and only then does the send, receive and effect
  of the edge (an effect that reads `nrpr` sees the table after the `Enter`). If no pool member is dormant the step is a **pool exhaustion**: a stop
  with `process budget exhausted`, inconclusive/bounded, never a verdict (`handleErr`).
- The table (`NrOff`, `TabOff`) exists when a process is `Dynamic` or an expression reads `nrpr`, `pid` or
  `youngest`. `Enter` appends, `Leave` pops the youngest and zeroes the slot, so the table is a stack and
  its *order* is part of the state. The Promela frontend, when any process is created by `run`, gives
  **every** process's `-end-` edge the guard `youngest(k)` and `Leave`; the `-end-` edge of a dynamic
  process goes back to its dormant location and resets its locals.
- `_pid` is folded to a constant by the frontend, so the `pid` op appears only in IR given directly;
  `_nr_pr` is the `nrpr` op, and `pid p = run P()` is a `run` edge whose effect assigns `nrpr - 1`.
- A dormant instance has no out-edge at its `Initial` location (the frontend). Hand-written IR may differ;
  the rule below does not rely on it.

### 4.2 The rule: one new cell, two static checks

New cell: **T**, the whole table (count and order). Footprints:

| Edge or expression | reads | writes |
|---|---|---|
| `nrpr`, `pid(k)`, `youngest(k)` anywhere (guard, effect, property, `Run.Args`) | T | |
| an edge with `Leave` (every `-end-` edge when the table is on) | T (through its `youngest` guard) | T |
| an edge with `Run` | the cells of `Args` (creator's scope) and of `Init` (target's scope); the dormancy of every `q` in `Targets()` (the counter at `Initial(q)`) | T; the program counter of every `q` in `Targets()` **at** `Initial(q)` |

Static checks (refusals, with the reason in the report): (c1) every edge that enters the dormant
location of a dynamic process leaves the table (the frontend always does this; hand-written IR may not);
(c2) the entry location of a `run` is not the dormant location of its targets (the frontend separates
them; with `Entry == Initial` a live process would still look dormant and a second `run` would overwrite its
locals, which the plan declares not to be cells). A dynamic process with an edge out of its dormant
location needs no check: its edge conflicts with the `run` through the counter cell.

Why these and no others (this section changed after the prototype, see §7.4):

- **T is one cell.** `Enter`/`Leave` are order-sensitive (two runs in two orders give two tables), so any
  two table writers conflict, and a `youngest(k)` guard is enabled or disabled by any `Enter` or `Leave`.
  One cell is coarser than the stack really is. It costs this: a process at its `-end-` location is never
  expanded alone (its end edge writes T, and so does every other end edge), and **a `run` is expanded alone
  only in a model in which no edge anywhere leaves the table**, because the `run` writes T and every
  `-end-` edge reads and writes it. For frontend IR that means: the creating step is always expanded in
  full; the gain on an `init`-started model comes from the *other* processes, which are expanded alone
  as before. The first prototype had a second cell (a "pool slot is free" cell) that the explorer's
  dormancy test seemed to need; it is not needed: a pool member becomes dormant again only through an
  edge that leaves the table (check c1), so T orders the `run` against it. Mutants of the slot cell were
  all equivalent, which is how this was seen.
- **The program counter of q at `Initial(q)`** is what the `run` changes of q's counter, and the dormancy
  it reads is the same cell. It must be a write, because a process that reads `pc(q)` whole (a guard, an
  alternative of the ample set) can be *enabled* by the `run`, and for a pool member with no edge of its
  own no other write of that counter exists (the prototype built the model: a false `verified` when this
  write is missing, §7.4). It does not conflict with q's other edges, which are enabled while q is somewhere
  else. (Writing the counter whole would make every created process conflict with its creator for ever.)
  **There is no write "at `Entry`", and none is needed**: a guard of another process that reads `pc(q)` at
  `Entry` is enabled by the `run`, never co-enabled with it, and cannot occur before it on any path;
  expanding the creator alone is therefore fine for it. The converse case, the guard's process being the
  ample one, is covered because the ample process's own edges read counters whole, and a whole read
  overlaps the write at `Initial`. Only a guard on the dormant location itself (`pc(q) == Initial`,
  disabled by the `run`) needs the fine-grained overlap, and has it.
- q's locals are not cells (nobody else names them), and `Run` writes them; the `Args` and `Init`
  expressions read globals, which are cells. A `run` whose `Args` read a global that another process
  writes is an order-dependent step (the new process gets a different parameter): in the prototype
  only the audit sees the mutant that forgets it.
- A property that reads `nrpr`/`pid`/`youngest` reads T, so every `run` and `Leave` becomes visible and is
  never expanded alone (C2, no new code).

### 4.3 The soundness argument

(a) A `run` is one edge (or part of an atomic chain, A). It is always enabled; whether it errs (pool
exhaustion) and which member it takes are a function of which pool members are dormant, i.e. of their
counters at `Initial` (F1/F3/F2). **Lemma D**: a counter changes to or from `Initial` only through a `run`
(which writes it and T) or through an edge that enters `Initial`, which by check c1 leaves the table and so
writes T; and a live process cannot look dormant by c2. So every writer of the dormancy that the `run` reads
conflicts with the `run`, through the counter cell or through T. A `run` on a pool member that is live
exhausts the pool, an error that the same conflicts order against the member's end. Its effects are the table
push, the counter of q, the locals of q: T, pc(q)@Initial, private (F2). (b) An `-end-` edge's
enabledness is `youngest(k)`, a function of T, and the counter of its process, as for every edge; it
writes T. (c) Lemma I then applies verbatim to every process, including one created later: it has edges
and cells like any other. (d) The only dependence between a created process and its creator that C1 needs
is the *enabling* of q's first edge by the `run`: q is dormant while the `run` is pending, so q is not a
candidate then; when q is a candidate its counter has left `Initial`, and a second `run` on q would
exhaust the pool, ordered by T against everything that makes a slot free. Guards of other processes on
q's counter are enabled by the `run`: that is the write of pc(q)@Initial against their whole reading.
(e) Termination order: the guard `youngest(k)` is a T read, so the refinement for `pc(j) == dead` of the
static models is not needed and not applied. (f) Pool exhaustion is an error in the sense of C1 (F3);
the oracle compares its reachability (§7.1).

### 4.4 The traps this rule has

1. `run` against the end of a member of its pool, and `run` against `run` on one pool: T orders them
   (the free slot or the exhausted pool, and the table order).
2. A model with *no* end edge anywhere: the `run` is then expanded alone, and its `Args`/`Init` reads
   and its writes are what is left to protect it (the generator has a mode without end edges).
3. **A run that enables a guard on the counter of a process with no edge of its own, with the guarded
   process of lower index than the creator** (the creator is picked first by index and masks the hole
   otherwise: both orders of processes are tested, as in step 4).
4. `nrpr` read in a guard, an effect (`pid p = run P()` reads it *after* the run's own `Enter`: the edge
   reads and writes T), a property, or `Run.Args`.
5. A property that reads `nrpr`/`pid`/`youngest`: visibility of `run` and `Leave`.
6. `pid(k)` of a process that is not live is -1: a domain overflow when stored in a byte (an error that
   must be reachable in both searches).
7. A created process that itself runs another (nested creation); a pool of one that is run twice.
8. A `run` inside a loop that exhausts the pool: the budget stop, in both searches or in neither.
9. The initial state of a model where the dynamic processes are dormant and the table lists only the
   static ones.
10. `Run.Args` and `Run.Init` that read globals, the table, the creator's own counter, or the length of a
    channel that the same edge clears (direct IR: `apply` evaluates them after the counter write, `Leave`
    and `ClearChans`, §4.1), and `Init` in the target's scope.
11. A dynamic process whose end edge returns to its dormant location without leaving the table (check
    c1: the model is refused; with the check removed the oracle must fail), and a `run` whose entry is the
    dormant location (check c2).

### 4.5 What remains refused

Nothing about `run` itself. A model that also has a rendezvous or a dynamic channel stays refused for
that (client_server, leader as written).

## 5. Feature P: `provided`

`enabled()` begins with the process's `provided` expression for **every** edge, `else` and `-end-`
included, and `hasEnabled`, the d_step continuation and the sibling test of `else` all go through
`enabled()`. So `provided (e)` is a conjunct of every guard of the process: the footprint of every edge
of the process gets the reads of `e` (`reads`, program counters whole). Nothing else changes. The
refusal text "a priority which the reduction does not model" is about SPIN, whose documentation says that
`provided` takes away the guarantee of its own reduction; in this engine `provided` is a guard and has no
priority semantics of its own.

Soundness: F1 with the extra conjunct; Lemma I verbatim. Remains refused: nothing. Two engine quirks are
noted, not changed: `nextEnabled` evaluates a rendezvous send's guard without `provided`, and `rvMatch` does
not test the receiver's `provided`. The rule of this section is stated for the semantics of the buffered
and local edges, and rendezvous stays refused, so the quirks do not enter; a later step that lifts the
rendezvous refusal must settle them first. Oracle: random `provided` expressions over globals, an array
element, another process's counter, and a local. Mutants: the reads dropped; the reads added only to the
first edge of a location (equivalent: the analysis unions the edges of a location); the reads not added to
`else` edges.

Value: none measured (3 models, 0 shrink). It is in the plan because it costs a dozen lines and because
the existing refusal is misleading; it goes **last**, and is dropped without ceremony if the
review finds a hole in the argument.

## 6. What is not done, and why

### 6.1 Rendezvous (capacity 0): rejected for this stream

What a handshake is here: `nextEnabled` enumerates, for a send edge `e` with `rv`, every receive edge on
that channel of the other processes (`rvMatch`: the receiver is at the location of its edge, its guard
holds, the channels and the `Match` values agree); the move is the pair, `apply` moves both counters,
binds the receiver's variables, and `Excl` may be taken by the partner. It is one transition of two
processes, so "expand one process alone" has no meaning for it.

What the existing machinery gives for free: if the channel operations are whole-channel cells (the ends
are not separate) and a location with a rendezvous operation is never expanded alone, a process that
*never* uses a rendezvous channel is independent of every handshake (the footprints of the sender's and
the receiver's edges are each in someone's `rw`, so the usual conflicts already hold). That
"bystander" version was measured: 8 more models are "applied", one more shrinks (`dijkstra`, 21 -> 15). A
refusal replaced by "applied, nothing reduced" is not a gain.

What a real gain needs: an ample set of a *group* (a sender, the receivers it can pair with, those
receivers' other alternatives), with the group's current locations and a state-dependent requirement
like step 4's (every rendezvous send of the sender enabled now with a partner that waits for nothing
else; no other process uses the channel). That is a new kind of ample set, a joint transition in the
conflict relation, and a proof of its own.

What the measurement says about the payoff (the 28 files with a rendezvous channel in the two corpora,
IR inspected; 8 of them are mutation variants of `dijkstra` and 5 are copies of one two-process handshake
in evaluation outputs): the channel is a semaphore used by every process of the model (`sema`: 4 senders, 4 receivers; `sem`: 2 and 2;
`to_server`, `request`, `release`: shared), or a private pair of the *only two* processes (`name`, `tpc`,
`glob`: nothing to commute with), except one three-process model with two private grant channels beside
shared request and release channels. No model has two independent pairs; the reduction of independent
pairs is the only thing the group rule would add. (I believe SPIN's own static reduction does not treat
rendezvous operations as safe, for the same reason; I have not checked that against its sources.)
**Rejected**; to be reopened only by a user model with independent
handshakes, with the design above as the starting point.

### 6.2 Dynamic channels (a channel named by a value): postponed

`leader.pml` as written passes its ring channels as `chan` parameters of `node`; the IR has `Sel = var in`.
Its static twin (the same ring with named channels, the unrolled `leader3` family) goes from 41 692 to 108
states under A+R. So the payoff is real on the one model that has it and the whole of it is blocked by a
value-dependent channel. A sound treatment is state-dependent: in a state, evaluate the channel locals
of every live process (provided the analysis has checked that nothing but `run` parameters and `Init` ever
writes them: that is a fact about each model and not about Promela, where channel values can be assigned and
received, so the frontend's acceptance of that is to be read, not assumed; they are then stable for the
life of the process), take the channels as the cells of its operations, and require that no process can still execute a
`run` (a static reachability bit per location) so that no process is created or re-created with other
values. The dependence relation then depends on the state, which the lemma above does not cover
(stability along all paths must be proved), the static plan becomes a plan plus a per-state computation,
errors (null channel, shape mismatch) need their own conflict, and the oracle needs channel-valued
parameters, pools and nested creation. That is larger than A and R together and has one measured
beneficiary. **Postponed** to a step of its own; first open question for the user (§11).

### 6.3 `timeout`: postponed

`timeout` is true only in the second phase of `nextEnabled`, when no process has an enabled phase-0 edge.
An ample set made of phase-0 moves is never accompanied by an enabled timeout edge along a path of the
others' moves, because it stays enabled (Lemma I (1)); by induction no timeout edge can occur before it, so
a timeout read is a constant false for the purposes of conflicts and the reduced search expands in full
the states that have no phase-0 move. The argument looks sound. Two reasons not to do it now: the trap in
`movesOf` (it enumerates with `ample` set and a per-process `f.enabled`, so a process without phase-0
moves would be "expanded alone" through its timeout edges although other processes have phase-0 moves:
a transition the full graph does not have) and the fact that the files that use it are one textbook model
of 16 states and two copies of one 55-state evaluation model. Recorded here with the trap so that a later step can take it.

### 6.4 Not touched

Breadth-first search (it needs another proviso), temporal properties and the vacuity watch (step 4's
measurement stands; `search.reduction.reason` unchanged for them), smarter choice among eligible
processes (SPIN prefers the fewest enabled moves), `--estimate --por`, bitstate.

### 6.5 Found along the way, not done here

The SPIN fuzzing of the prototype found one model on which the engine and `pan -DNOREDUCE` disagree and the
reduction has nothing to do with it. The frontend gives the `-end-` edges of a process the `Leave` of the
live-process table only when some process of the model is created by `run` (`lower.go`); a model that
merely reads `_nr_pr` has the table, so that `_nr_pr` is a state variable, but nothing ever leaves it, so
`_nr_pr` stays at the number of processes that were started with the system. SPIN's `_nr_pr` goes down when
a process ends. Minimal case: `active proctype A() { (_nr_pr == 1) }  active proctype B() { skip }` is
`deadlock violated` in the engine and `errors: 0` in `pan`. It is a pre-existing difference in the default
search, so it is recorded here and in the open questions and not fixed by this stream. It does not
affect the plan's rules: such a model has a table nobody writes, so T has no writer and the reads of
`nrpr` never conflict; if the difference is fixed by giving the static processes `Leave` edges, the rules
of §4 already cover them.

## 7. The oracle first

The earlier rounds each found a false `verified` that the tests had not caught; every time the hole was
in what the tests could *express*. This stream builds the oracle before the rules, builds an oracle
that does not read the footprints, and calibrates it on the bugs that are already known.

### 7.1 Layers

| # | Oracle | What it checks | Needs the analysis? |
|---|---|---|---|
| O1 | differential on random IR models (`explore/por_random_test.go`, extended) | status and evidence of every property; reduced states a subset of the full ones (exact); the set of states without an enabled move, **up to `Excl`**; error reachability, with the stop reasons classified (`invalid model`, `process budget` = pool exhaustion, any other budget skips the comparison); every counterexample and witness replayed as a run of the model; the reduced graph never larger; the models on which the reduction is refused have equal counts | no |
| O2 | **semantic audit of the ample sets** (new, `explore/por_audit_test.go`) | for every stored state of the *full* graph and **every process that is eligible there** (`eligible[p][loc]`, the channel requirement met, a move enabled; not only the one `pick` returns, which would hide a hole in a process of higher index): run the model, not the footprints: the macro-steps of the process are the same sets of edge sequences after any sequence of up to K = 2 (K = 3 on small models in the scale runs) macro-steps of the others (Lemma I (1)); each commutes with the sequence, up to `Excl` (2); an error or failed assert of an other's step is the same with and without the ample step first (3), and the process's own steps still end without error; no ample macro-step changes the value of an invariant or reach expression (C2) | yes, through the eligibility tables only |
| O3 | acyclicity audit of the proviso (new) | in the exact reduced graph (the states stored, with the choice `pick` made at each: full or the process), the states that were not expanded in full contain no cycle (C3 as a graph property, §3.3 (ii)). Needs a nil-by-default hook on `porRun` that records the choices, set only by tests | no (instrumented run) |
| O4 | the corpus test (`por_corpus_test.go`) | the 86 models, plus: the number of reduced models must not fall below the pinned figure, and `bench-sym`, `leader3`, `nrpr` must shrink | no |
| O5 | SPIN: `tools/pandiff` (`spin -a -o1 -o2 -o3`, `gcc -DNOREDUCE`) | verdict triple SPIN / engine full / engine `--por` on the corpus and on 300 or more Promela models from a fuzzer of the new shapes (each costs a `spin -a` and a `gcc` run, about 1.3 s) | no |
| O6 | Promela fuzzer through the frontend and the CLI (`mcd check --por` vs without) | the shapes the frontend really emits: `init { atomic { run P(); run Q() } }`, `active` processes, `atomic` blocks with guards, `d_step`, `if ... else`, bounded loops, `_nr_pr` guards, a buffered channel | no |
| O7 | exhaustive small scopes | **not part of the bar.** Round three of step 4 ran one at the end; if the orchestrator of the code review does the same it must state the grammar, the domains and the properties it enumerated, and until it does, "exhaustive" is not claimed | no |

Sizes. The committed `go test` default stays small (`por_random_test.go` runs 8 000 models; each new
generator gets the same default and the `MCD_POR_MODELS`/`MCD_POR_SEED` switches); the **release bar**
is the 300 000 models per generator of §7.5, run once and recorded, not on every `go test`.

O2 and O3 are the layers that are new in kind. The verdict oracle O1 sees a hole only when the hole changes a
verdict, a deadlock state or an error; O2 sees it as soon as one state exhibits it, and it sees holes in
states the reduced search never visits, because it audits the full graph. ("As soon as one state exhibits it"
holds for the shapes the generators make: a clause of the footprint whose shape no generator builds is not
covered by any layer. The cross-review of the diff found such clauses, the reads of a send's arguments, of a
receive's match and index, of an effect's index and of an assert, which the generators of that time filled
with constants or locals; see `perf6-confirmation.md`, "What the oracles can and cannot see".) **Calibration, done on the
0.2.0 code before this plan was written** (throwaway audit and a throwaway copy of the verdict oracle,
the base generator of `por_random_test.go`, seeds 1 to 3 000, models that finish): with the unmutated code
both pass all 3 000. With the first-round bug of step 2 put back (the `pc(j) == c` refinement applied
to the edges expanded alone) the audit fails in **3** models and the verdict oracle in **0** (the second
review of step 2 measured about 1 model in 6 000-30 000 for a verdict oracle on its own generator). With
the step-4 requirement (`channelsCanAct`) disabled: audit **42**, verdict oracle **8**. With visibility (C2)
off: audit **16**, verdict oracle **0** (step 2 recorded that "visibility ignored" survived its random
oracle). The audit must be built and calibrated **before** the first rule is implemented, and it must pass
on the unmutated 0.2.0 analysis, which the new step S0 commits. O3, on the same generators: it passes
the unmutated code on 3 000 models of three generators (the base one, the atomic one and a loop shape) and
fails when the proviso is switched off (906 of 3 000 base models, 2 796 of 3 000 loop models) and for the
two proviso mutants of §7.4.

### 7.2 The audit, precisely

For a stored state *s* and each process *p* that is eligible there (the eligibility tables and the channel
requirement of `pick`, the proviso ignored: it depends on the DFS stack, while C0-C2 depend on the state only). Macro-steps of a process are enumerated by `Stepper.Enabled` /
`Apply` with `intermediate()` deciding where a chain ends, so the audit shares no code with the
analysis. For every macro-step of the others, the audit asserts: *p*'s signature set is unchanged (a step of *p*
that has become an error or a failed assert changes its signature, so *p*'s own errors are covered);
*p*'s steps are still enabled; the two orders reach the same state up to `Excl`; an erroring step of the
others still errs and a clean one stays clean. Once every eligible process is audited at every stored state
of the full graph, Lemma I's induction needs only K = 1 (eligibility and the channel requirement are stable
along the others' moves: a pop only adds room, a push only adds a message, and any other use of the channel
conflicts); K = 2 is kept as redundancy. In the prototype auditing all eligible processes instead of the one
`pick` returns raised the failures of the mutants that the audit sees (6 000 models: A3 17 -> 23, A2 10 ->
11, A6 148 -> 157, `pid` reads 2 -> 3) and removed none. Depth K = 2 was enough for every mutant of §7.4 that the audit kills; whether
it is enough for a trap nobody has thought of is not claimed. The cost is the number of states times the
branching, fine on models of a few hundred states. The audit shares the enabledness code of the engine
(it runs `Stepper`), so a misreading of the semantics shared by the full and the reduced search is
invisible to it; that is what the SPIN comparison O5 is for.

### 7.3 Generators (the shapes, each aimed at a trap)

All are extensions of `randomPORModel` / `randomPipelineModel` or sibling functions, validated with
`ir.Validate` in a unit test, with seeds that do not overlap those of earlier steps. The oracle sets a
**depth budget** (3 000) as well as a state budget and skips runs that end on it: a macro-step that does
not end (trap 11 of §3.6) otherwise exhausts memory before the clock is looked at (the prototype's first
run used 24 GB before it was killed; every later run of the prototype used `ulimit -v` and a timeout, and
so must the test, which also sets the budget).

- **Atomic** (`genAtomic`, a post-pass over the base generator): every `Atomic` **and every `DStep`** edge
  goes forward (a later location), which keeps every macro-step finite and keeps trap 11's shape (a
  d_step that ends in an atomic edge). The prototype's first version forbade that shape instead, which
  hid the one place where the closure crosses from the d_step to the atomic mechanism; chains of 1-3;
  continuations that are guarded (so the holder blocks and is stored with `Excl != 0`), that carry an
  assert, a write, a `pc(j)` guard, an `else`, a channel operation of the pipeline model, or a d_step;
  edges that are both atomic and d_step; atomic in the pipeline model.
- **Loop** (`genLoop`, built in the prototype): a process that cycles through an atomic chain with two or
  three branches that return to the start, to the branch location or leave, beside a process that writes a
  visible variable and one that asserts on it, in both orders of the processes. It exists for the proviso
  (§7.4: the mutant that looks at the first branch only is seen by nothing else).
- **Run and table** (`genRun`): three modes. Frontend-shaped (every `-end-` edge guarded by `youngest(k)`
  with `Leave`, dynamic ones back to the dormant location with the locals reset; half of the models);
  **no end edge at all** (a third: the table is never left, so the `run` is the only writer of T and is
  eligible, which is the only way to exercise the footprint of the `run` itself); a dynamic end edge that
  returns to the dormant location without `Leave` (check c1 must refuse), and a `run` whose entry is the
  dormant location (check c2). Pools of 1-3 interchangeable
  instances; one type in eight has no body edge at all (a counter nobody writes but the `run`); `Run`
  edges in static and dynamic processes (nested creation) and "creator" runs added to the static
  processes; `Args` and `Init` over locals and globals, a writer of those globals in another process;
  `pid p = run` (an effect reading `nrpr`); `nrpr`, `pid(k)` (also of a process that is not live),
  `youngest(j)` and `pc(j) == 1` of a created process in guards, effects and properties; "light" models
  that touch few globals so that processes are independent where the reduction should act. One model in
  eight has the table and no `run`.
- **Provided**: `provided` over a global, an array element, another counter, a local, on a third of the
  processes of the base generator.
- **Mixes**: atomic around a `run` (the `init` idiom: directed and in the Promela fuzzer O6), atomic
  with d_step, run with channels.
- **Process order**: the directed traps are built in every order of their processes (the creator picked
  first by index masks a missing conflict, §4.4 trap 3).
- Each generator reports how many models were actually reduced; the guard "fewer than 1 in 25 reduced
  fails the test" is kept per generator, and a second guard counts the states in which a process
  standing at a `run` edge is picked alone (103 in 3 000 models in the prototype after the third mode was
  added, 0 before: the figure is what told the author that the `run` footprints were not being tested).

### 7.4 Mutation plan, and what a prototype of the first-draft rules showed

The harness runs the unmutated tests first and refuses to go on if they are red (the failing baseline of
step 4 counted every mutant as killed). The mutants are exact textual replacements kept as data with a
short runner in `steps/`, so that a reviewer can run them. Each mutant must be killed by O1 or O2 on the
generators alone, or the generator is extended until it is, or the mutant is shown equivalent in writing.

Prototype results (first-draft rules in a throwaway copy of the engine, generators of §7.3 as they stood,
models of seeds 1 to 3 000 or 6 000; failures of the verdict oracle O1 / failures of the audit O2; the
unmutated baseline: 0 / 0 on every generator):

| Rule | Mutant | O1 | O2 | Reading |
|---|---|---:|---:|---|
| A | closure does not follow `Atomic` | 9 | 157 | killed |
| A | closure follows `Atomic` for one hop only | 0 | 4 | **only the audit** |
| A | `whole` not set for a location an atomic edge enters | 0 | 8 | **only the audit** |
| A | continuation writes not merged | 4 | 69 | killed |
| A | proviso treats an intermediate successor as off the stack | 5 (345 of 3 000 loop models) | 0 | killed by O1; O3: 2 379 of 3 000 loop models |
| A | proviso looks at the first branch of a chain only | 0 | 0 | **survives O1 and O2 on every generator, including the loop shape (0 of 3 000)**; killed by O3 (1 158 of 3 000 loop models, 32 of 2 683 atomic models). On a depth-first search a state whose first branch is not on the stack and whose later branch is gives no wrong verdict that the prototype could build: the search reaches a fully expanded state elsewhere. The cycle is nevertheless a cycle of reduced states, which is what C3 forbids. |
| A | `dirOK` kept for the atomic edge's own channel operation | 0 | 0 | equivalent: the first edge could keep the directed rule; the plan clears it only to have one argument fewer |
| A | assert flag not merged through the continuation | 0 | 0 | not distinguishable: a failing assert in an enabled branch is met by the chain walk of the proviso (full expansion), and isolation keeps a chain that does not fail from failing later; kept as the textbook rule, as "asserts invisible" in step 2 |
| R | `run` writes no T | 1 | 82 | only the audit in practice |
| R | `Leave` writes no T | 0-10 | 71-172 | the audit is stable, the verdict oracle is not |
| R | reads of T dropped (all / `nrpr` / `youngest` / `pid`) | 3 / 1 / 1 / 0 | 45 / 27 / 6 / 2 | **`pid` only the audit**, at 2 in 6 000 |
| R | `Args` reads dropped | 0 | 3 | only the audit |
| R | `Init` reads dropped | 0 | 1 | only the audit, 1 in 6 000 |
| R | check c1 removed (a dynamic end edge that does not leave) | 0 | 3 | only the audit |
| R | write of pc(q)@`Initial` dropped | 0 | 0 | **survives the generators**; a directed model (§4.4 trap 3, creator of higher index) turns it into a false `verified` (3 states against 5, `assert` verified instead of violated) with the mutant, and passes without it |
| R | (first prototype) the "slot" cell dropped | 0 | 0 | all four slot mutants were equivalent: the cell was removed from the plan (§4.2) |
| P | `provided` reads dropped | 4 | 45 | killed |
| P | reads not added to `else` edges | 0 | 2 | only the audit |
| P | reads added to the first edge of a location only | 0 | 0 | equivalent: the analysis unions the edges of a location |

What this table is for. (1) It is the evidence that the audit sees what the verdict oracle does not:
seven of the killed mutants are killed only by O2 (and one only by O3), and the verdict oracle's counts are unstable from one
generator revision to the next (`Leave` writes no T: 10 failures with the first revision of the run
generator, 0 with the third) while the audit's are not. (2) It is the evidence that a green oracle is not
enough: two rules (the proviso over branches, the write of pc(q)@`Initial`) survive 6 000 models of
generators written for them; one of them is a false `verified` by hand, and the other is killed only
by the acyclicity audit. The first gets a directed test in every process order and a generator shape
(the creator of lower and of higher index, a process with no edge of its own that nothing but the `run`
moves); the second gets O3 and the loop shape; both before the rules are implemented. The others are
expected to be killed by the harness the same way. (3) It shows the cost of redundant rules: four mutants of
a cell that turned out unnecessary could not be killed, which is how its redundancy was found.

Mutants per rule that are still to be written for the real implementation (the prototype did not
carry them): **the closure follows `Atomic` only after an `Atomic` edge and not after a `DStep` one** (trap
11); visibility not checked through the atomic closure; the chain limit, and the per-`pick` budget,
answering "leaves"; check c2 removed; the dormancy read of the `run` dropped;
`run`'s effect (the `pid p = run` read of T) not read; table reads of a property not visible; the `pc`
reads of `provided` taken as "at c"; the ten mutants of perf4 that touch `closure`, `whole` and
`leavesStack`, to keep those tests honest after the rewrite of `closure` and `leavesStack`.
Mutants that only lose reduction power are listed as such; one that survives is documented (as in perf4).

### 7.5 How this stream finds a false `verified` before the reviewers do

1. The audit (O2) is built first and calibrated on the three known bugs of 0.2.0 (§7.1) and on the
   mutants above, so a new rule is judged by an oracle that has already shown it can see this kind of
   hole. The generators are calibrated by their *own* statistics (how many states a `run` edge is
   picked at), not only by the number of reduced models.
2. **The traps are written down before the rules** (§3.6, §4.4) and each gets a hand-built model, as a
   directed test, in every order of its processes, before the implementation: the test must be red for the
   right reason with the refusal lifted and the closure or write missing, then green.
3. Every rule has mutants that O1 or O2 must kill without the directed tests; a mutant that only a
   directed test kills means the generator has a blind spot, and the generator is extended until the
   oracle alone kills it (step 4 did this); a mutant that nothing kills is either proved equivalent in
   writing or the rule is removed (the slot cell).
4. Scale before a commit is called done: at least 300 000 models per generator on seeds that were not used
   while writing the rules, with the audit over the same, the largest runs repeated after the
   cross-review fixes. (The first-draft rules passed 60 000 per generator, seeds from 100 000, in the
   prototype; that is calibration, not the bar.)
5. SPIN as an outside witness for verdicts (O5), over models the engine's own full search might
   misunderstand in the same way as its reduction does (atomic and `run` semantics shared by both).
6. The orchestrator of the code review gets the traps list, the mutant list, the audit and the generators
   as part of the brief, and is asked to attack the traps list, not only the code.
7. What the plan does **not** rely on: a green O1 alone. Step 2's fix and step 4's clear-overlap hole were
   both invisible to a verdict oracle for thousands of models, and so, in the prototype, were seven of the
   killed mutants above, a mutant of the proviso, and one false `verified` (found by hand).
8. **Prototype results for the outside witnesses** (first-draft rules, throwaway): the Promela fuzzer of
   O6 (a few hundred lines of Python writing `init`/`active` models with `atomic`, `d_step`, `if`/`else`,
   bounded loops, `_nr_pr` guards and a buffered channel) found no disagreement in 3 000 models, 86% of them
   reduced, and kills the prototype's mutants A1, A6, A7, R1 and R3 in 2 000 models (12, 6, 1, 4 and 1
   disagreements; R2, `Leave` writes no T, survived 2 000: the fuzzer is not a substitute for the generators
   of §7.3). O5 (SPIN `-DNOREDUCE` against the prototype's `--por` verdicts) agreed on 282 of 300 models, 17
   were skipped (budget, compile) and 1 disagreed; the disagreement is not about the reduction (full and
   reduced searches agree with each other) but about a pre-existing difference between the engine and SPIN,
   see §6.5.

## 8. Report, CLI, MCP, documents

- **Default behaviour is byte-identical**: nothing changes without `--por`/`por: true`. With it, the models
  of A, R and P that used to answer `applied: false` now answer `applied: true`; that is the intended change
  of an opt-in option. `search.reduction` keeps its fields (`kind`, `applied`, `reason`, `reduced_states`,
  `fully_expanded_states`, `note`); `porNote` is unchanged; the `mc_check` schema is unchanged
  (`mc_manifest` and the alignment tests do not move).
- **Reason strings** (all in `analyzePOR`/`edge`/`channelOp`/`reads`): the atomic, process-creation,
  process-table and `provided` reasons disappear; the remaining ones are the rendezvous channel, the
  channel named by a value, `timeout`, temporal properties, the breadth-first search and the vacuity
  watch, **and the two new ones**: "a dynamic process re-enters its dormant location without leaving the
  process table" (c1) and "a run enters its target at the dormant location" (c2). Both appear only for IR
  that is not frontend output, and both go into the README and skill lists of refusals. The rendezvous and dynamic-channel refusals gain the sentence "refused in this version; see
  perf6-plan.md section 6" only in documents, not in the string (strings are matched by tests and agents).
- **Tests that pin the old refusals** (to be changed in the same commits, red first):
  `features/g7-por.feature` ("a model with atomic sequences is not reduced, and says why" becomes a model
  that is reduced and a new refusal scenario for a rendezvous channel), `explore/por_test.go`
  (`TestPORRefusals`: the "dynamic process", "process table", atomic and provided rows), the comment
  in `features/g2-mcp.feature` that lists atomic among the refusals, the header comment of `por.go`.
  (The scenario on `bench-sym.pml -D N=3` becomes a reduction scenario: 1 348 states in full, a few hundred
  reduced.)
- **Documents** (same commits): `README.md` (the `--por` section and its list of refusals),
  `skills/model-check/references/engine-tools.md` (`--por` row, the G7 table row), `.../workflow.md` Node 8
  (what the reduction covers and the sentence about "never to a formula with X"), `PROVENANCE.md`
  (the entry for the reduction already cites the textbook conditions C0-C3; the arguments of §3-§5 are
  this stream's own instances of them and are recorded as such, with the audit as their check, not as
  a citation), the
  `features/g7-por.feature` header, and `steps/perf6-confirmation.md` (what was measured, what was not,
  what is refused, each cross-review's findings), in the style of `perf4-confirmation.md`.
- `mc_check` and the CLI refuse nothing new; a request the engine still cannot honour is `applied: false`
  with the reason, as today.

## 9. Work breakdown

Each step is its own commit on this branch (small, reviewable, green), BDD first, and each ends with
`go test ./...`, `go vet ./...`, `test -z "$(gofmt -l .)"`. `explore.go` is touched in comments only (the
other stream edits it); everything else is in `por.go` and the tests.

| Step | Content | Red test first | Green when |
|---|---|---|---|
| S0 | the oracle, on the unchanged analysis: the audit (O2) and the acyclicity check (O3; a nil-by-default hook field on `porRun`, set only by tests, like `porNoProviso`), the stop-reason classes and the comparison up to `Excl` in O1, `genAtomic`/`genRun`/`genProvided` (the generators run, the reduction refuses them, the oracle compares equal counts) | the audit is given **hand-forced wrong plans** (a `porPlan` that marks the process eligible at the location of each known trap: the enabling trap, the d_step trap, the channel trap, a visible write) and must report each; no switch in production code. The mutation harness (patches applied to a scratch copy) additionally runs the three resurrected 0.2.0 bugs | the audit passes on 0.2.0 over 50 000 models, fails on each forced plan, and the harness shows it failing on each resurrected bug |
| S1 | A: closure, `whole`, `dirOK`, chain proviso with its two limits, refusal removed; directed tests for **traps 1-12** of §3.6, including the d_step-into-atomic chain (finite and cyclic, with the limits exhausted) and the two-branch cycle in both process orders; scenario in `g7-por.feature` (an atomic model reduced, verdicts kept, the `Excl != 0` stored state); `genAtomic` with forward d_step edges and `genLoop` | the scenario and the twelve traps fail with the refusal in place | O1 + O2 + O3 on `genAtomic` and `genLoop` over 300 000 models each; **every** A-mutant of §7.4 killed (the first-branch proviso mutant by O3 and the directed test, the d_step-into-atomic closure mutant by trap 11) or proved equivalent in writing; corpus test green |
| S2 | R: the T cell, the footprints of `run`/`Leave`/table reads and the dormancy read, checks c1 and c2, refusal removed; directed tests for §4.4 in every process order; scenario on `leader3` (76 states, verdicts kept) and on the pool-exhaustion trap | idem | O1 + O2 + O3 on `genRun` and on atomic+run mixes; every R-mutant killed or proved equivalent; SPIN triples (O5, O6) |
| S3 | P: reads of `provided` on every edge | idem | O1 + O2 on `genProvided`; P-mutants killed |
| S4 | the measurement (states, time, memory, load noted, repeated): `bench-sym` N=3..7, the leader ring N=3..7 (fixtures kept small), `nrpr`, `you_run2`; the corpus test pinned; the guard on the number of reduced models | | numbers in the confirmation file |
| S5 | the documents of §8 and `perf6-confirmation.md` | | the release checklist items that concern this change (manifest and MCP declaration unchanged; binaries not rebuilt) |
| S6 | code cross-review of the stream, fixes, a second round if the first finds anything above "not worth fixing" | | as `perf4-confirmation.md` |

Order inside S1/S2 is test-first: the trap models and the oracle, the rule last.

## 10. Risks, non-goals, what will not be claimed

**Risks.** (1) A hole the oracle cannot express: mitigated by O2, O3 and the calibration, not removed. (2)
Atomic and `run` together create the biggest gains and the largest blind spots (init with `atomic { run }` is
a chain of runs): they are reviewed separately and together. (3) Lemma E is a new kind of
argument in this engine and is load-bearing (§3.3 (iv)): if a review does not accept it, there is no small
fallback. Exact commutation needs that no macro-step of any process can end with `Excl != 0`, so the
fallback is to refuse every model in which some atomic edge's target has an edge that can block (a
guard, an `else`, a channel operation), which loses most of the atomic models and most of the gain. (4) T as one cell may cost
reduction on models that churn processes (`splurge`-like): measured, not assumed. (5) Merge with the
parallel-exploration stream: both streams touch the DFS only through `top.ample`/`pick`; this one edits
`por.go`, its tests and documents, and the comment above `dfs`.

**Non-goals.** Rendezvous, dynamic channels, `timeout`, temporal properties, BFS, symmetry, abstraction, a
smarter choice of the ample process, performance work on `pick` (a cost measurement is taken, not a
target).

**What will not be claimed.** That any state count is a benchmark without the load and the spread; that a
reduced count is comparable with `pan -c0` or with the full search; that "applied" means "reduced"; that
the reduction is complete (it never was: it is sound and static); that O2 proves C1 (it checks it to depth
K on the states it visits); that `provided`, rendezvous or `timeout` models gain anything by this stream;
that the atomic models of the corpus other than `bench-sym` shrink by more than a state or two.

## 11. Open questions for the user

1. Dynamic channels (`chan` parameters): the one corpus model that has them (`CH9/leader`) is the
   textbook case for this reduction (41 692 -> about 100 states once the channels are named). Postponed
   here for the reasons of §6.2; do you want a separate step for it after this one?
2. `provided`: do you want it in this stream (a dozen lines, no measured gain) or left refused?
3. `timeout`: the retransmission timer of the alternating-bit protocols is its usual use; the corpus has
   a 16-state and a 55-state model that use it. Worth a later step?
4. **Found along the way, outside this stream (§6.5):** in a model without `run`, `_nr_pr` never decreases
   when a process ends (SPIN's does), so `active proctype A() { _nr_pr == 1 } active proctype B() { skip }`
   is a deadlock for the engine and fine for `pan`. Do you want it fixed as a separate step? (It would change
   `_nr_pr` semantics in the default search, and so byte-identical reports of models that read it.)

## 12. Review log

Three blind reviewers read this plan at commit `cff7adb` (a detached worktree of the commit, read access to
the engine sources): one answered *needs rework*, two *approve with changes*; quorum 3 of 3. One of
the three hit its output limit on the first run (its reasoning ended in the answer, no verdict) and answered
on the retry. The orchestrator verified every finding against the sources or with the throwaway prototype
(the prototype is outside the repository; the figures below are its). **No reviewer found an unsound rule;
one proof of the plan was wrong and has been replaced; two surviving mutants, one missing generator shape and
several numbers were fixed.** Findings are numbered for this log only.

| # | Finding (raised by) | Decision, severity | Reason and what changed |
|---|---|---|---|
| 1 | The cycle-proviso argument "in the quotient by `Excl`" does not establish C3 (one reviewer) | **fix**, medium | Verified: `pick` chooses the first eligible process whose moves leave the stack, so two `Excl`-variants of a state can choose different processes, and the quotient of the reduced graph is not a reduced graph with ample sets. The conclusion holds by another route, now §3.3 (i)-(ii): run the textbook induction on exact states, use Lemma E only to carry the path through `Excl`-variants, and use C3 on the exact reduced graph; O3 checks the exact graph. The proviso compares exact states. |
| 2 | The fallback of risk (3) does not remove the quotient (one) | **fix**, medium | Verified by construction (an ample step ending with `Excl = 0`, another process's step ending blocked inside its sequence: the two orders leave different bytes). §10 now says Lemma E is load-bearing and states the only static fallback, which loses most of the gain. |
| 3 | The audit checks only the process that `pick` returns, so a hole in a higher-index process is masked (one) | **fix**, medium | Verified: `pick` takes the first eligible process by index, the same masking as trap 3 of §4.4. O2 now audits every eligible process at every stored state (6 000 models, mutants A3 17 -> 23, A2 10 -> 11, A6 148 -> 157, `pid` reads 2 -> 3 failures, none lost); with that, Lemma I's induction needs K = 1 and K = 2 stays as redundancy (§7.2). |
| 4 | The atomic generator excludes trap 11 (a d_step that ends in an atomic edge) by construction; the proviso, the chain limit and the closure at that boundary would not be exercised (two) | **fix**, medium | Verified: it was a workaround for a macro-step that never ends, whose memory use I had to kill at 24 GB. Every `Atomic` and `DStep` edge now goes forward, which keeps the shape and keeps it finite; trap 11 got a directed test (finite and cyclic, limits exhausted), a trap 12 was added, and the mutant "follow `Atomic` only after `Atomic`, not after `DStep`" is on the list. The oracle sets a depth budget. |
| 5 | The mutant "proviso looks at the first branch of a chain only" survives O1 and O2 and S1 does not require it to be killed (one) | **fix**, medium (the reviewer said high; no wrong verdict was built) | Verified in the prototype and extended: with a loop shape (`genLoop`) the mutant survives O1 and O2 (0 of 3 000) and is killed by O3 (1 158 of 3 000; 32 of 2 683 atomic models). On a depth-first search the cycle it leaves does not produce a wrong verdict that I could build, because a fully expanded state is reached elsewhere, so it is a violation of C3 as a graph property and only the acyclicity audit sees it. S1 now requires O3, the loop shape, a directed test in both orders, and every A-mutant killed or proved equivalent in writing. |
| 6 | `Run.Args` are not evaluated "in the source state" (`apply` writes the counter, `Leave`s and clears channels first) (one) | **fix**, low | Verified in `apply`. §4.1 states the order; trap 10 now asks for direct-IR cases of `Args`/`Init` that read the creator's counter, the table and a channel the same edge clears. |
| 7 | The effect of a `run` depends on the dormancy of every pool member, and `Entry == Initial` breaks the "locals are not cells" premise (one) | **fix**, low | Verified. The footprint of the `run` now reads the dormancy (the counter at `Initial`) of every target; §4.3 states Lemma D (who writes it); check c2 (entry is not the dormant location) was added next to c1, with a generator mode and a mutant. |
| 8 | Numbers disagree across sections (36 against 28 files, timeout sizes, "eight" against "nine", one corpus model with dynamic channels against eight, `run` alone) (one) | **fix**, low | Verified each; reconciled, and the population (101 accepted, 86 compared, two corpora) is stated at each count (§0, §1.1, §6.2, §6.3, §7.4, §7.5). The count of mutants killed only by the audit is seven (and one only by O3). |
| 9 | Checks c1 (and now c2) introduce refusals that §8 does not list (one) | **fix**, low | Added to §8, with the README and skill lists. |
| 10 | The audit compares the signature of the ample process's steps but not whether they err (one) | **fix** (clarified), low | A step that became an error or a failed assert changes its signature in the prototype's audit, so it was covered; §7.2 now says so. |
| 11 | The design premise of §6.2 ("channel locals are written only by `run`") is a fact about the frontend's acceptance (one) | **fix**, low | Reworded as a condition the analysis must check per model. |
| 12 | "SPIN's reduction is unsound with `provided`" overstates; the `provided`/rendezvous quirks should be stated as conditions (two) | **fix**, low | Softened to what SPIN documents; the quirks are stated as conditions of the rule, and a later step that lifts the rendezvous refusal must settle them. |
| 13 | Missing: a total budget per `pick` for wide chains; CI default against release bar; the `bench-sym` N=7 figure is unconfirmed; "exhaustive" O7 is undefined (two) | **fix**, low | §3.4 rule 5 has the budget and a test; §7.1 separates the default test size from the release bar; N=7 is marked unconfirmed; O7 is not part of the bar and may not be called exhaustive without its grammar. |
| 14 | O2 with K = 2 cannot see a hole whose enabling path has three or more preparatory steps (one) | **partly rejected** | The audit runs at *every* stored state of the full graph, including the one in which the preparatory steps have been made, and there K = 1 sees the changed step set; the reviewer's own example is caught at that state. Accepted: audit every eligible process (finding 3), K = 3 on small models in the scale runs, and the sentence that said the audit "finds each trap" was replaced by what it was calibrated on. |
| 15 | `run` must also write the counter at `Entry` (one, first answer, cut off) | **rejected** | A guard of another process at `Entry` is enabled by the `run` and never co-enabled with it, so it cannot occur before it and the creator may be expanded alone; the converse (the guard's process ample) is covered because the ample process reads counters whole and a whole read overlaps the write at `Initial`. The prototype passed 60 000 models with `pc(q) == 1` guards and a hand-built trap passes; the mutant that drops the write at `Initial` is a false `verified` by hand. An explanatory paragraph was added to §4.2. |
| 16 | The chain lookahead must compare states up to `Excl` (one) | **rejected** | The proof now uses exact states (finding 1); comparing up to `Excl` would only make the proviso more conservative. |
| 17 | A `run` inside an atomic chain must read the dormant location "whole" (one) | **rejected** | The chain continues through the *creator's* edges at the `run` edge's target; the created process's dormant location is not in the chain, and the dormancy it reads is ordered by the counter cell and T (Lemma D). |
| 18 | Fix the `provided` quirk of `nextEnabled` (one) | **not worth fixing** | Irrelevant while rendezvous is refused; documented as a condition (finding 12). |

Found by the orchestrator while building the oracle, also in this revision: calibration of the new audit
O3 (§7.1), the Promela fuzzer and SPIN results (§7.5 item 8), the `_nr_pr` difference from SPIN (§6.5, open
question 4), the finding that the first prototype's pool-slot cell was redundant (§4.2), and a directed
false `verified` for a missing write (§7.4).
