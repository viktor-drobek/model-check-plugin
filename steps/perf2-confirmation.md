# Performance plan, step 2 — partial-order reduction of the safety search

Layer: G0 (`explore`, `cli`, `report`); documented as G7 in the plan. Follows
`perf1-confirmation.md`. Protocol: `BUILD-PROTOCOL.md` step 6.

## What shipped

`mcd check --por`: in a state, expand the moves of one process alone when the
static analysis can prove nothing the other processes do could matter first.
Opt-in; without the flag every report is byte-for-byte what it was.

Scope of this version: the depth-first safety search (`deadlock`, `assert`,
`invariant`, `reach`). Everything else is refused, and the report says why in
`search.reduction` (`applied: false`, `reason`): atomic sequences, rendezvous
and dynamic channels, `run` and dynamic processes, the process table
(`nrpr`, `pid`, `youngest`), `timeout`, `provided`, `--bfs`, and any `ltl`,
`progress` or `ctl` property in the run. A refused run is the unreduced run
(same counts).

The four conditions (full statement in the header of `explore/por.go`):

- **C0** the ample set is non-empty;
- **C1** a process at a location is expanded alone only if none of its edges
  there (all of them, with the d_step continuation of each) conflicts with
  any edge of any other process: a global cell (one array element for a
  constant index, else the array), a channel, or a program counter; locals
  never conflict;
- **C2** an edge that writes something a property reads, or carries an
  `assert`, is never expanded alone;
- **C3** the cycle proviso on the DFS stack: a state uses its ample set only
  if none of its successors is on the stack; otherwise it is expanded in full.
  A speculative firing that errors makes the state expand in full, so
  `invalid-model` surfaces unchanged.

## The refinement Promela needed

SPIN lets a process terminate only when every younger one is dead; the
frontend lowers that as a guard `pc(j) == dead` on the `-end-` edge of every
older process. Read as a plain read of `pc(j)` it makes every younger process
dependent on every older one, and the first measurement showed it: on the
independent-processes fixture only process 0 was ever expanded alone
(2 969 states instead of the linear count). The dead location has no edge,
so that guard and any edge of `j` are never enabled together, and they are
independent. For the edges of the *other* processes, a guard with
`pc(j) == c` as a top-level conjunct is therefore read as "j at c", which
conflicts only with the edges of `j` that leave `c`.

**The first version applied that reading too widely, and the cross-review
found a false `verified` in it** (next section). It is right for the edges
other processes see; it is wrong for the edges expanded alone. Those are the
alternatives of one process, hence dependent on each other, and a step of
another process *into* `c` can enable one of them; the same holds for the
edges a `d_step` goes on into, chosen when the step is made. They now read a
program counter whole, as do the edges of a location with an `else` or one
entered by a `d_step` when seen by the others. Everything else that reads a
program counter (a property, any other position in a guard) always read it
whole.

## Behaviour pinned (BDD first)

`features/g7-por.feature`, eleven scenarios (eight first, three added by the cross-review), red before (`flag provided but
not defined: -por`, seven of eight; the eighth pins that a run without `--por`
has no `reduction` object): independent processes → 57 states instead of
41 371 with the same verdicts; a shared-variable race still violates its
assert; a deadlock survives the reduction of the workers around it (34 states
instead of 1 464); a write a property reads is never reduced away; refusals
for atomic, `--bfs` and temporal properties name their reason.

## Evidence of soundness

There is no ground truth for a reduction except the unreduced search, so the
evidence is differential, and each ingredient was mutation-tested. The
cross-review below shows what that evidence did not cover at first.

| Check | What it covers |
|---|---|
| `explore/por_test.go` (the analysis: 25 tests, 10 of them refusal cases) | conflicts of cells, constant-index precision, locals, visibility of properties and asserts, channels, `pc` reads, d_step closure, else siblings, every refusal and its reason |
| `explore/por_search_test.go` | linear count on independent processes (17 states against 625), visible writes kept in every order, the cycle proviso (with a `porNoProviso` run that shows the missed invariant it prevents), evaluation-error fallback, refused runs equal full runs |
| `explore/por_random_test.go` | 8 000 random concurrent models, half "light" and half "rich" (globals, an array read and written by constant and dynamic index, a channel with `len` guards and `ClearChans`, asserts, else, d_step chains, dead-end locations, `pc` guards alone, in a conjunct, negated, in a disjunction, or waiting for a dead end; one- and two-variable invariant and reach): same status and evidence for every property, **the same set of states without an enabled move**, no state the full search does not reach, every counterexample and witness replayed as a run of the model. `MCD_POR_MODELS=60000` for a deeper run: no disagreement |
| `por_corpus_test.go` | the Promela models of `testdata/promela`, `testdata/corpus2` and the SPIN corpus that the frontend accepts and both searches finish: 82 compared at this step (86 at 0.2.0, after the later fixtures), identical verdicts on all; it checks verdicts, the reachability of a model error and that the reduced graph is never larger, not the states without a move or the replay of counterexamples |
| Mutation of `por.go` | 28 mutants, each killed by at least one test (15 after the first review, 13 more after the second): visibility off, reads-vs-writes off (both directions), proviso off, constant index always precise, d_step closure off, channels never conflict, else siblings ignored or read exactly, pc written at the target, exact `pc` reading inside a disjunction, asserts invisible, three aimed at the first fix (expanded edges read exactly, own footprint exact, d_step target read exactly), and thirteen for reads and writes no test pinned (a d_step chain of more than one hop, the match, argument, index and bind of a send or receive, an assert's operand, an else continuation's effect, `clen`/`cfull`, visibility of the second property, `lt`/`ge` on a program counter, dead-end locations) |

Two things the mutation run taught, both fixed before this record:

- The first random generator read one variable per property and never a
  `pc`; "visibility ignored" and "program counter not written" survived. The
  generator now builds two-variable properties and `pc` guards, and directed
  tests were added. A passing differential test is only worth what it can
  fail.
- A visibility cell for the location an edge *enters* was redundant (a
  property reads the whole counter) and no test could tell it from its
  absence, so it was removed rather than kept as untested machinery.

"Asserts invisible" is killed only by its unit test: the random oracle cannot
tell it from the real rule, and I believe it is equivalent (the failing assert
is a step of its own process, and its operands are protected by the conflict
analysis). The rule stays because it is the textbook one and costs only the
locations that carry an assert.

## Cross-review (crossreview 2.0.0): needs rework, and it was right

Three blind reviewers on the diff of the commit (`ae9e3d1`), an orchestrator
verifying each finding by building models and running the binary with and
without `--por`: `codex-terra-high` (Coddy, `ndlcdx/gpt-5.6-terra`, reasoning
`high`, 6 min) said *needs rework*; `coddy-gemma` (`ndsub/gemma-4-31b`, 4 min)
said *approve* and retracted each of its own findings; `claude-fable`
(`claude-fable-5-1`, 7 min) said *needs rework*. Quorum 3 of 3.

**Confirmed, high: a false `verified`.** Found by `claude-fable` alone. With a
guard `pc(j) == c` on an edge of a location that has other edges, or inside a
d_step continuation, `--por` answered `verified` for `deadlock` where the full
search answered `violated`. Four reproductions (three IR models, one Promela
with `pc_value`), now `testdata/ir/por-enabling.json`, `por-dstep.json` and the
unit tests. Reachable only through `--ir`/MCP `ir` input with a guard on a
program counter, or Promela with `pc_value()`: in 106 Promela models and 15
shapes of loops, gotos, labels, d_step and atomic the generated `-end-` edge is
always alone at its location, so ordinary Promela was unaffected. A silent
wrong verdict in soundness-critical code all the same, so the commit did not
stand as it was.

**Why my own checks missed it, which is the part to learn from.** The random
generator had `pc` guards, but never on a location with several out-edges
where the other process *enters* the location, and no d_step; the oracle
compared statuses only, so losing one deadlock state while another remained
was invisible. The mutation run I did before the review killed twelve
mutants and I took that for strength; none of them was "ignore enabling
through a program counter". The oracle now builds d_step chains, dead-end
locations and six shapes of `pc` guard, and compares the **set of states
without an enabled move**; it failed on seed 1750 of the committed analysis
(15 of 16 such states lost) before the fix and passes 60 000 models after it.

**Fixed:** the two readings described above. Cost on the Promela corpus and
the fixtures: none (identical state counts; 12 of 82 models shrink). On
IR-heavy random models the extra care halves the models that are reduced; the
alternative fix proposed in the review (the refinement for sole edges only)
would also have been sound and was not taken because it loses more.

**Also from the review, handled:**

- A small `--budget-depth` can make a reduced run answer `inconclusive` where
  the full run found a violation (the reduced search reaches the other
  process's step deeper). Never a false `verified`: a truncated search is
  never complete. Reproduced (`por-depth.pml`), pinned by a scenario, stated
  in the report's `note` and `engine-tools.md`. **The second review showed my
  sentence "never the reverse" was false**: with the same budget the reduced
  run, whose paths are shorter, can finish and decide what the full run leaves
  `inconclusive` (`por-depth-reverse.json`, 159 of 22 697 depth-bounded pairs;
  the other direction 183). Neither contradicts the unbounded full search, in
  about 517 000 depth-bounded reduced runs. The note, the docs and a second
  scenario now say "either can decide what the other leaves inconclusive".
- Rejected after checking: a `uint8` wrap of the process index at 256
  processes (validation rejects more than 254, run with 300 processes);
  `movesOf` not counting `f.enabled` (timeout is refused, so phase 1 never
  runs on a non-claim edge); `fire` touching counters or `tmp` (it writes only
  `s.next`).
- Not worth fixing: the `onStack` bitset is not in `MemBytes` (one bit per
  state).
- Open: a d_step continuation with a *single* guarded edge. The step then
  depends on the program counter in theory, but the unreduced search blocks
  with the same error in the same state, so no verdict differs that anyone
  could build. The fix covers it anyway.

**Found by the review's own profiling, fixed separately:** `fire` rendered the
text of every candidate edge at every d_step step to prepare an error message
it almost never prints, and that was 62% of the time of d_step models. The
texts are now built only when the step blocks. The random oracle went from 90 s
to 4 s.

## Second cross-review, of the fixed code (HEAD `9a687db`)

The same three reviewers on the whole diff, an orchestrator asked to attack the
analysis itself as well. `codex-terra-high` said *needs rework* on one finding
(the `uint8` process index wraps at 255 processes), which is not reachable: a
model has at most 254 processes and validation rejects more (run with 254, 255
and 257). `claude-fable` said *approve with changes* (seven items).
`coddy-gemma` hit the model's output limit twice and gave no verdict and no
usable finding, so the quorum of 3 of 3 is nominal and the weight rests on two
reviewers and on the orchestrator.

**No unsound verdict found.** The orchestrator ran about 1.13 million random
models (the in-repo oracle on 300 000 unused seeds; its own generator, which
mixes `pc` with `len`, array elements, nested and reordered conjunctions,
several d_step levels, else beside a `pc` guard, Promela-style termination
edges, three-variable properties, 800 000 models; a Promela fuzzer through the
CLI, 28 000) and eight hand-built attacks, with no disagreement. Calibration:
the same generator, with the first bug put back, finds it in 1 model of 6 000
to 30 000, so a clean run is strong evidence but not proof, and I say so.

What the review did find, all handled:

- **A false sentence of mine in every `--por` report.** "...answer
  inconclusive where the full search found it, never the reverse" is wrong:
  under one depth budget the reduced run, whose paths are shorter, can finish
  and decide what the full run leaves `inconclusive` (159 of 22 697 pairs; the
  documented direction 183). Fixed in the note and the docs, with a second
  scenario (`por-depth-reverse.json`).
- **Nine lines of the analysis that no test pinned**: a d_step chain of more
  than one hop, the match, argument and index of a send or receive, the bind of
  a receive, an assert's operand, an else continuation's effect, a channel
  named by value. Deleting any of them gave a false `verified` on a small
  model and passed every test I had, and 15 000 random models of the
  orchestrator's. One model each is in `explore/por_reads_test.go`.
- **A dead branch in my own oracle.** The comparison of whether an error is
  reachable sat after the completeness skip, but an error leaves the search
  incomplete, so it never ran (1 058 of 8 000 runs skipped before it). It now
  runs first, on `Stop == "invalid model"` and not on the property statuses
  (a property decided before the error keeps its verdict), and ignores runs
  that stopped on a budget.
- **`replay` could take exponential time** on a trace of steps with equal
  texts; it now remembers the (step, state) pairs that led nowhere.
- Two predicates only the orchestrator's generator killed (only the first
  property visible; `lt`/`ge` read as an exact program counter), a plan whose
  `any` was true for every Promela model because dead-end locations counted as
  eligible, a test that pins the 254-process bound, and a property that names
  a channel by value.

Not changed: asserts stay visible (conservative); the dead `failed != nil`
test in `leavesStack` stays as a guard (an assert makes a process ineligible,
so it cannot fire today); `--estimate --por` ignores `--por` silently. Open:
a mutant that checks the cycle proviso for the first ample move only is not
distinguished by any test; I believe it equivalent and could not build a model
that separates it, and the implementation checks every move, as the textbook
does.

## Results

Machine: Xeon Gold 6154, Go 1.26.1; one run unless stated, wall times vary
about ±10%.

| Model | Full | `--por` |
|---|---:|---:|
| `bench-indep` N=4, K=4 | 41 371 states | 57 |
| `bench-indep` N=5, K=4 | 579 195 states, ~0.7 s | 71 states, 0.7 ms |
| `bench-indep` N=12, K=4 | 6^12 = 2.2·10^9 states, out of reach | 169 states, 2.7 ms |
| `por-deadlock` (deadlock kept) | 1 464 states | 34 |
| `por-shared` (lost update kept) | 55 states | 50 |
| `bench-sym` N=6 (atomic: refused) | 543 076 | 543 076; +2% median over 8 interleaved pairs, noise |

These are the best case: independent processes. On the corpus of 82 real
models only 12 shrink, and the others are mostly refused:

| Refused | Models |
|---|---:|
| process creation (`run`) | 10 |
| a temporal property (`never`, `accept`, `progress`) | 10 |
| rendezvous channel | 9 |
| atomic sequences | 8 |
| `provided`, `timeout` | 4 |

That list is the order of the next increments by what they would unlock.

## MCP increment

`mc_check` takes `por` (boolean, default false) and answers with
`search.reduction` only when it was asked, the same record as the CLI's; the
stored report file carries it too. A request the engine cannot honour
(breadth-first search, a temporal property, atomic sequences, ...) is the
unreduced answer plus `applied: false` and the reason, never a tool error.
Four scenarios in `features/g2-mcp.feature`: verdicts kept and fewer states, no
`reduction` without `por`, and the two refusals above. The schema is generated
from the input struct, so `mc_manifest` and the alignment tests followed.

## Not done, deferred

- **Temporal properties:** a model with a never claim or an `accept`/`progress`
  label is refused although its *safety* search could be reduced; only the
  shared counters and the vacuity watch block it. Separating them could
  unlock up to 10 of the 82 corpus models (the ones refused for that reason alone
  are not counted separately here).
- **Atomic, rendezvous, `run`:** each needs its own independence argument
  (exclusive control disables other processes; a handshake moves two; creation
  changes the process table). SPIN's `xr`/`xs` channel hints are stored in the
  IR already and unused.
- A smarter choice among eligible processes (SPIN prefers the one with the
  fewest enabled moves); the first eligible by index is taken.
- Breadth-first search is not reduced (it needs a different cycle proviso).
- The tracked binaries in `engine/bin/` are not rebuilt (a release step).
