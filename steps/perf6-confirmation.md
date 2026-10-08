# Performance plan, step 6 — a wider partial-order reduction

Layer: G0 (`explore`; strings in `mcp`; `tools/pandiff` and `cmd/pormut` for the
oracles). Follows `perf2-confirmation.md` (the reduction), `perf4-confirmation.md`
(channel ends) and `perf6-plan.md` (the plan this step carried out, with the
review it had). Protocol: `BUILD-PROTOCOL.md` step 6.

Status of this document: **the implementation and its oracles, after the first
and the second cross-review and the fixes they led to.** What the reviews found and what
was done about each finding is in "Cross-review" at the end; the sections above
were corrected in place where they had said something wrong (the mutant r16, the
corpus numbers, the SPIN fuzz counts, the redundancy of a12 and x10, the strength claims
of the oracles) and extended where a fix added to them (the two generators of reads,
the mutants of the reviews' probes, the floors of the oracle runs, the scale runs on new
seeds).

## What was done

1. **The oracles first** (commits `624581f`, `1f67816`), on the unchanged 0.2.0
   analysis: generators of the shapes the new rules are aimed at; the verdict
   differential (O1) with the comparison of the states without a move up to the
   exclusive byte; a semantic audit of every eligible process at every stored
   state of the full graph (O2) that reads nothing of the footprints; an
   acyclicity audit of the cycle proviso (O3) through a nil-by-default trace
   hook; and a mutation harness that runs the unmutated baseline first. The
   audit passed 50 000 base models (980 075 (state, process) pairs) on the 0.2.0
   code and saw each hand-forced plan for the traps of steps 2 and 4.
2. **Atomic sequences** (`3155c37`): the macro-step as the unit; the closure
   follows an `Atomic` edge as it follows a `DStep` one; a location an atomic
   edge enters is read whole, and the channel operations in it are
   whole-channel cells; the cycle proviso follows each ample move through the
   sequence, along every branch, to the stored states it can end in, with a
   chain limit and a budget per pick that both answer "not leaving"; the
   refusal is gone.
3. **Process creation and the process table** (`907f49b`): the table is one
   cell, T (`nrpr`, `pid`, `youngest` read it; every `run` and every edge that
   leaves the table write it); a `run` also reads and writes the program
   counter of each target at its dormant location and reads its arguments and
   initialisers; two checks refuse what the Promela frontend never emits. The
   refusals for `run` and the table are gone.
4. The documents (`b334b3b`), the corpus pins, the measurements, and this file.
5. The fixes of the first cross-review of the diff (`0236921` to the commit that
   closes this file; "Cross-review" below).

## Decisions awaiting the user's veto

The user had not answered the plan's open questions; these defaults were taken.

| # | Question | Default taken | Effect |
|---|---|---|---|
| 1 | atomic sequences and `run` with the process table | **done** | the two rules above |
| 2 | `provided` | **not implemented**, stays refused with its reason | the plan measured 3 models that would gain "applied" and none that would shrink; one more rule is one more hole to look for. The argument of the plan (§5, a read added to every edge's footprint) is unchanged and short if the user wants it. |
| 3 | rendezvous; dynamic channels; `timeout` | rendezvous **rejected** (refused; the plan §6.1 measured no model with two independent handshakes), dynamic channels and `timeout` **postponed** (refused; designs in plan §6.2, §6.3) | `leader.pml` as written (channels are `chan` parameters) stays unreduced: 41 692 states. Its static twin with named channels (`testdata/promela/leader5.pml`) is reduced to 108. |
| 4 | the `_nr_pr` difference from SPIN found by the plan | **out of scope, documented below** | the default search is unchanged |

## The oracle: what it is, and what it saw

Every layer shares generators (`explore/por_gen_test.go`): `base` (the models of
steps 2 and 4), `atomic` (every `Atomic` and `DStep` edge goes forward, so every
macro-step is finite and a d_step can end in an atomic edge; continuations that
are guarded, read a program counter, are an `else`, a channel operation, an
assert, a d_step; one process in four gets a d_step-into-atomic chain of its
own), `loop` (a process that cycles through a chain of one to three atomic edges
with two or three branches that return, stay or leave, beside a writer and an
asserting process, in both orders), `run` (three modes: frontend-shaped ends, no
end edge at all, a dynamic end that does not leave the table; pools of one to
three; runs in static and dynamic processes; arguments, initialisers, `pid p =
run`; a waiter on the counter of any member of a pool with a creator that makes
that many runs; one model in twenty enters at the dormant location), `run-atomic`
(atomic around the runs), `provided` (refused: it checks that a refusal is
the full search) and, added after the first cross-review, `reads` and
`atomic-reads` (below). Each generator has a floor on the share of models reduced,
the run generators a floor on the audited states at a `run` edge (the figure
that told the plan's author the first generator did not exercise the `run`
footprint).

Switches: `MCD_POR_MODELS`, `MCD_POR_SEED`, `MCD_POR_GEN`, `MCD_POR_WORKERS`,
`MCD_POR_ALL`, `MCD_POR_AUDIT_K`. Default sizes are small (the package runs in
about a minute and a half with the two generators of reads; it was about 45 s
before them); the release bar is 300 000 models per generator.

### The floors of the oracle runs

A generator that stops exercising the reduction must fail its test and not pass
silently, so each run has a floor on a count of its models: how many came out smaller
(O1, and O1 under the tight limits), how many were checked for cycles (O3), how many
pairs the audit looked at and how many stood at a `run` edge (O2). The floors were "one
model in n" with n set a little below what the default seeds gave, and on a fresh seed
(`MCD_POR_SEED`, the harness's `-seed`) that is flaky with production code unchanged.
Measured for the second cross-review's finding on 8 fresh windows of the default size and 16
of `go test -short` (seeds 710 000 001 to 880 000 001): the old floors failed one default
window in eight (`atomic-reads` under the tight limits: 363 smaller models in 3 000
against a floor of 375) and eight of the sixteen short windows (the tight floors of
`atomic`, `atomic-reads` and `loop`, and the at-run floor of the audit: 1 pair against 3).
A count of the models that came out smaller is a binomial count, and its noise relative to
its mean grows as the run shrinks (375 models: 55 plus or minus 7; 3 000: 440 plus or
minus 16), so no single "one in n" serves both sizes.

The floors of the counts of models are now `porFloorAt` (`explore/por_floor_test.go`): the
share of the models a generator is known to reach, less a number of binomial standard
deviations (6; 5 for the tight-limit floors, which have to fail when the tight budget is
lowered, below), and `go test -short` counts half the share (a floor of zero is no
check). The shares are not guesses: for the generators `atomic`, `atomic-reads`, `loop`
and `run-atomic`, 24 000 fresh seeds each (900 000 001 on) were run through O1 with the default
limits, the tight limits and the tight limits with the budget lowered to one, and
every window of 375 and of 3 000 consecutive seeds was counted (21 000 to 23 600 windows
each). Rates found: smaller with the default limits 0.157 / 0.142 / 0.555 / 0.250
(atomic / atomic-reads / loop / run-atomic), tight 0.147 / 0.137 / 0.306 / 0.244, tight with
the budget lowered to one 0.100 / 0.103 / 0.240 / 0.230. The variance of the count of a
window against the binomial's: 0.3 to 0.9 at 3 000 models, 0.9 to 1.2 at 375 and 1.7 to
3.0 for `loop` at 375 (the models of consecutive seeds are not independent draws), so the
binomial deviation is an upper bound at the default size and not at the short one, which is
why a short run halves the share. Floors with the final shares: the minimum over all
windows of 3 000 seeds is 399 / 380 / 874 / 678 for the tight count against floors of 344 /
316 / 788 / 611 (never below the floor in 21 001 windows), and the exact binomial probability
of a count below a floor is at most 3e-7 (5 deviations) or 1e-9 (6) for every share of the
tables (`TestPORFloorsAreNotReachedByChance`). The 24 windows that failed the old floors (8
default, 16 short) run clean with the new floors, 0 failures anywhere.

The release-bar run found one more thing, and it is the reason for the last rule. At 300 000
models the binomial margin is small in relative terms (five deviations are 1.4% of the mean) and
a share is an estimate from 24 000 seeds, 1 to 2% off: 300 000 models of `loop` under the tight
limits (seeds 940 000 001 on) gave 90 218 smaller models (0.3007) against a floor of 90 239 from the
share 0.305 that the 24 000 seeds had given (0.3058), and the test failed with every oracle
green (`TestPORFloorAtTheReleaseBarDoesNotFailAMeasuredRun`, red first). A floor is now never
closer than 5% to its mean (`porFloorSlack`); it does not change a floor at 3 000 models or
fewer (the margin there is 22% or more), and it binds from about 30 000 models on.

Strong enough to fail when it must: lowering the tight budget of the proviso from three
micro-steps to one (the red check of the first review, done again on a scratch copy of the
tests) gives at the default seeds 289 / 311 / 708 / 696 smaller models for `atomic` /
`atomic-reads` / `loop` / `run-atomic` against 435 / 417 / 901 / 748 with the budget of
three, and the floors 344 / 316 / 788 / 611 fail the first three (`atomic-reads` by five
models: 311 against 316) and cannot fail the fourth (the budget moves that count by 6%);
on a fresh window of 3 000 seeds the lowered budget fails them in 100%, 64% and 100% of the
windows. At `go test -short` the lowered budget is not told from chance (the floors there
are 24 or less): the short run checks only that a generator is not dead. The at-run floor of
the audit (pairs, not models: they come in bunches) applies from 1 000 models: only about
one model in forty has a pair at a run edge (24 and 21 models in 1 000, `run` and
`run-atomic`, 1 to 6 in 125) and 1 000 models give 94 to 589 pairs.

### The generators of reads (added after the first cross-review)

The review found that the base generators fill the places where a footprint must
contain a READ with constants or locals: a send's arguments and a receive's match
are constants, a receive binds a local or a scalar, an array element is chosen by
a constant or by a local, and an assert reads a scalar that some other process
also reads. So a hole in the clause "the arguments of a send are read", "the
match of a receive is read", "the index of a receive that binds an array element
is read", "the index of an effect's target is read" or "the expression of an
assert is read" could not change what any random model does: of 600 models of
`base` and of `atomic`, **none** has a send argument, a receive match, a receive
index or an effect index that reads a scalar that another process writes
(`TestPORReadsGeneratorsMakeTheirShapes` counts them).

`reads` (two models in three from the pipeline generator, the rest from the base
one) and `atomic-reads` (from the atomic one) apply one post-pass, `readShapes`:
a writer process of a scalar beside the others (a pipeline model has a scalar no
process writes), send arguments and receive matches that read a scalar, a
receive that binds an element of an array chosen by a scalar, an effect on such
an element, and, in about one model in five, an assert that reads a scalar `t` that
only the writer touches (set to 1 and back to 0; the draw is one in three, but only
a model of at most three processes can take it: 667 of 3 000 models of `reads` and
663 of `atomic-reads` at the default seeds, 22%). The arrays that a scalar indexes
are, but for one case, the process's own (`b<p>`, touched by nobody else), so the
step stays eligible and a footprint that has forgotten the read would expand it
alone; a shared array would make every such step dependent on the others by its
write alone. The one case is the effect on the shared array `a` of the base
generator, which `readShapes` also indexes by a scalar: there the whole-array write
conflicts anyway and the read cannot show. Of 600 models, `reads` has the four
shapes in 199 / 137 / 105 / 285 and `atomic-reads` in 154 / 113 / 87 / 438 (send
argument / receive match / receive index / effect index, with a scalar another
process writes); of the effect-index models, 244 and 344 have it on a private array
and 41 and 94 only on `a` (the calibration test counts both, and the assert on `t`:
123 and 130 of 600).

### Calibration of the audit on the three bugs of 0.2.0 (before the first rule)

The harness (`go run ./cmd/pormut`) puts each known bug back into a scratch copy
of the 0.2.0 analysis. First run, 3 000 base models, each oracle stopping at its
first failure: the pc refinement applied to the edges expanded alone is seen by
the audit and by the directed test and **not by the verdict oracle**; visibility
(C2) switched off, the same; the channel requirement of step 4 removed, by the
verdict oracle and the audit; the cycle proviso removed, by the verdict oracle
and the acyclicity audit. The hand-forced plans (no switch in production code)
make the audit report each of: the enabling trap, the d_step trap, a send that
only the pop enables, a receive that only the push enables (both orders), a
clear that overlaps the receive end (both orders), a visible write, a shared
write.

Counts, from the final run of the harness (`MCD_POR_ALL=1`, 3 000 base models, the
audit checking every eligible process): the pc refinement on the expanded edges:
audit **3**, verdict oracle **0**; visibility off: audit **18**, verdict oracle
**0**; the channel requirement off: audit **65**, verdict oracle **8**; the
cycle proviso off: verdict oracle **22**, acyclicity audit **906**. These
reproduce the throwaway calibration of the plan (3, 16, 42 and 8, 906); the audit
counts are a little higher than the plan's for the same reason the plan gave: it
audits every eligible process, not only the one `pick` returns.

## Atomic sequences

The rule, in one paragraph. The reduction commutes the macro-step: the sequence
of micro-moves of a process from a stored state to the next stored state. Its
footprint is the union over every edge it can be made of, enabled or not. A
location entered by an `Atomic` edge is read whole, and the channel operations
there are whole-channel cells (a continuation `c!x` is enabled by the receiver's
pop; the directed ends of step 4 do not apply to an edge a sequence goes on
into). The cycle proviso follows each move through the sequence, all branches,
and compares the stored states it ends in **exactly** with the stack. The
exclusive byte is the one new lemma: two commuting macro-steps can leave
different bytes (the last step writes it), so the two orders reach two stored
states that differ only in that byte; the byte is read in two places only
(`startIter` and `intermediate`, checked by grep), a stored state has no holder
that can move, so such states have the same moves, effects, property values and
successors up to the byte. The proof runs on exact states and uses the
equivalence only to carry a path past a commutation (`perf6-plan.md` §3.3); the
oracles compare up to the byte. Limits: a chain longer than the d_step limit, or
more than 10 000 micro-steps fired in one pick, is not followed: full expansion.

Directed tests (`explore/por_atomic_test.go`, every trap in every order of its
processes): a continuation that reads a variable; the same with an `else`; three
hops; an edge that is both atomic and d_step; a d_step that ends in an atomic
edge; a continuation guarded by a program counter; a continuation that sends on
a channel whose receiver pops; a write in the middle of a sequence that a
property reads; two sequences that block inside (5 states in full, 3 reduced,
one state without a move up to the byte and two exactly); a stored state whose
holder is blocked is expanded through an ample set; a cycle through a chain, with
one branch and with two; the proviso looks at every move; a chain that never ends
(both limits answer "full expansion", and the depth budget stops the search). The
audit is shown each trap as a hand-forced plan. Scenarios in
`features/g7-por.feature`: `bench-sym` N=3 (1 348 states, 369 reduced), a
continuation that reads a variable (deadlock kept), two blocked sequences, the
rendezvous refusal.

**What the audit found while the rule was being written.** The first version had
a silent no-op in my own edit script: the location entered by an atomic edge was
not marked whole. Every directed test and 3 000 verdict-oracle models passed it;
the audit failed on the 348th model of the atomic generator (seed 20 000 348), a
process that was eligible at a location whose atomic edge went on into a send on
a channel that another process receives from, and named the state, the process
and the macro-steps that had changed. That is the case the plan built the audit
for, and it was not a case I had thought of.

## Process creation and the process table

The rule is in the header of `explore/por.go`. Directed tests
(`explore/por_run_test.go`, every order of the processes): a creator whose `run`
enables a guard on the counter of a process with no edge of its own (with no end
edge anywhere, so the table is never left), and the same with a guard on the
number of live processes; a property that reads `_nr_pr` (the `run` is visible);
arguments and initialisers that read a global another process writes; an effect
that reads the table after the `run`'s own enter; the arguments of a `run` that
read what the same edge changes (a channel it clears, the creator's own counter);
nested creation; the pid of a process that is not live (an error both searches
reach); a pool of one run twice, with and without a wait for the end; a `run` in
a loop that exhausts its pool; (after the first cross-review) an initialiser that
assigns a global, and heterogeneous pools (an initialiser that is a local of one
instance and a global for the other, instances with bodies of their own), both in
hand-written IR only; the eligibility tables (a `run` is expanded alone
only where nothing else touches the table, which in frontend output with end
edges is never); the two refusals. The audit is shown each trap as a hand-forced
plan.

Scenarios: `leader3.pml` (679 states, 76 reduced), `leader5.pml` (41 692, 108),
`nrpr.pml` (31, 21), a `run` that exhausts its pool (both searches stop on the
process budget and answer inconclusive), the refusal of a dormant re-entry
(`testdata/ir/por-dormant.json`).

## Mutants

`tools/pormut/mutants.json`: exact textual replacements, an anchor that must
occur exactly once (`go test ./tools/pormut` keeps the data honest). Before anything is
copied or run, the harness applies every mutant of the list, not only the selected ones,
and checks that every layer's `-run` regex matches a test, and lists all that fail at once
(the anchors used to be applied lazily, after the baseline and for the selected mutants
only: a stale anchor went unseen for the others, as b4 and a7b did for three commits, and a
layer that matched no test counted as green: "ok [no tests to run]"); an `-only` id that
names no mutant is refused. A combination (`with`: two or more mutants applied to the same
copy) is one entry of the list. The harness runs the unmutated copy first and refuses to go
on when any layer is red; a mutant that does not build is never counted as killed; one that
only a directed test kills means the generator has a blind spot and is reported as a failure.

52 mutants (11 of the older rules (the three 0.2.0 bugs and the proviso, four of closure,
whole and else, three of the channel requirement), 13 of rule A, 17 of rule R, and 11
of the footprint clauses that the first cross-review probed, rule F, ids `x*`), 3 000
models per generator, `MCD_POR_ALL=1`, three workers, every generator of the table
below; each cell is the failures among the 3 000 models of that generator ("tight": the
same oracle with the tight chain limits); a "-" is a layer that stayed green. The run
(33 minutes, load average 4 to 6 at the start): **45 mutants killed, every one by an
oracle on the generators alone**; 4 pinned by a directed test (r12, r16, r17, x10); 3
survived and say why in the data (a4 and r13 equivalent, a12 only loses power; a12 is
pinned since the second cross-review, below); **0 against the contract**. The baseline was green on every layer, on all eight generators,
before the first mutant. The harness was run in full again after the fixes of the first
cross-review: every count of the first version reproduces, except the "tight" columns
of a1, a6, a7, a7b, a11 and r1 (and r8's, which turned green), which moved because the
tight budget of three micro-steps is now spent by all the candidates of a pick together,
and r7's directed cell, which the new test of a heterogeneous pool also kills. The classes
before the review were 35 killed, 1 pinned, 4 surviving as "equivalent" or "power"; r16,
labelled equivalent, is not (below).

| Mutant | What it removes | O1 | O2 | O3 | directed | Outcome |
|---|---|---|---|---|---|---|
| `b1-pc-refinement-own` | 0.2.0 bug: the pc refinement on the edges expanded alone | - | base 3 | - | kill | killed |
| `b2-channel-requirement-off` | 0.2.0 requirement of step 4 removed | base 8 | base 65 | - | kill | killed |
| `b3-visibility-off` | 0.2.0 C2 (visibility) removed | - | base 18 | - | kill | killed |
| `b4-proviso-off` | cycle proviso removed | base 22 | - | base 906 | kill | killed |
| `a1-closure-no-atomic` | closure does not follow `Atomic` | atomic 4, run-atomic 27 / tight: atomic 3, run-atomic 9 | atomic 250, run-atomic 171 | - | kill | killed |
| `a2-closure-atomic-one-hop` | closure follows `Atomic` after `Atomic` one hop only | run-atomic 1 | atomic 7, run-atomic 3 | - | kill | killed |
| `a3-whole-not-set-for-atomic` | no whole reading of the location an atomic edge enters | - | atomic 10 | - | pass | killed |
| `a4-dirok-for-atomic-edge` | directed ends kept for the atomic edge's own channel operation | - | - | - | pass | survived (equivalent) |
| `a5-continuation-writes-dropped` | continuation writes not in the footprint | atomic 3, run-atomic 1, base 1 / tight: atomic 3, run-atomic 1 | base 4, atomic 102, run-atomic 29 | - | kill | killed |
| `a6-proviso-intermediate-is-off-the-stack` | proviso treats the intermediate state as the stored successor | atomic 4, loop 333, run-atomic 1 / tight: atomic 4, loop 333, run-atomic 1 | - | atomic 74, loop 2381, run-atomic 16 / tight: atomic 68, loop 2381, run-atomic 14 | kill | killed |
| `a7-proviso-first-branch-only` | proviso follows the first branch of a chain only | - | - | atomic 23, loop 1188, run-atomic 4 / tight: atomic 16, loop 391, run-atomic 2 | kill | killed |
| `a7b-proviso-first-move-only` | proviso looks at the first ample move only | - | - | base 279, atomic 178, loop 560, run-atomic 251 / tight: atomic 177, loop 549, run-atomic 249 | kill | killed |
| `a8-closure-atomic-not-after-dstep` | closure does not cross from a d_step edge to an atomic one | atomic 1 / tight: atomic 1 | atomic 15 | - | kill | killed |
| `a9-visibility-first-edges-only` | visibility judged on the first edges' writes | - | atomic 2, run-atomic 9 | - | kill | killed |
| `a10-chain-limit-answers-leaves` | chain limit answers "leaves" | tight: loop 112 | - | tight: loop 807 | kill | killed |
| `a11-pick-budget-answers-leaves` | budget of a pick answers "leaves" | tight: run-atomic 3 | - | tight: atomic 46, loop 92, run-atomic 63 | kill | killed |
| `a12-error-in-chain-leaves` | error or failed assert in a chain counts as leaving | - | - | - | kill | pinned by a directed test (was: survived (power)) |
| `r1-run-writes-no-table` | `run` writes no T | run 21, run-atomic 14 / tight: run-atomic 14 | run 214, run-atomic 177 | - | kill | killed |
| `r2-leave-writes-no-table` | `Leave` writes no T | run 3 | run 47, run-atomic 42 | - | pass | killed |
| `r3-table-reads-dropped` | no read of T (nrpr, pid, youngest) | run 7, run-atomic 6 / tight: run-atomic 6 | run 84, run-atomic 71 | - | kill | killed |
| `r3n-nrpr-read-dropped` | `nrpr` reads nothing | run 3, run-atomic 4 / tight: run-atomic 4 | run 53, run-atomic 43 | - | kill | killed |
| `r3y-youngest-read-dropped` | `youngest` reads nothing | run-atomic 1 / tight: run-atomic 1 | run 7, run-atomic 8 | - | kill | killed |
| `r3p-pid-read-dropped` | `pid` reads nothing | - | run 14, run-atomic 7 | - | kill | killed |
| `r7-run-first-target-only` | `run` covers the first slot of its pool only | - | run 1 | - | kill | killed |
| `r8-dormant-counter-not-written` | `run` does not write pc(q) at the dormant location | run-atomic 1 | run 1, run-atomic 1 | - | kill | killed |
| `r13-dormancy-read-dropped` | `run` does not read the dormancy of its slots | - | - | - | pass | survived (equivalent) |
| `r9-args-reads-dropped` | arguments of a `run` not read | - | run 2, run-atomic 2 | - | kill | killed |
| `r10-init-reads-dropped` | initialisers of a `run` not read | run-atomic 1 / tight: run-atomic 1 | run-atomic 1 | - | kill | killed |
| `r11-check-c1-removed` | check c1 removed | run 16, run-atomic 10 / tight: run-atomic 10 | run 37, run-atomic 33 | - | kill | killed |
| `r12-check-c2-removed` | check c2 removed | - | - | - | kill | pinned by a directed test |
| `r14-run-effect-reads-dropped` | effect of a `run` edge reads nothing | - | run 1 | - | pass | killed |
| `r15-table-property-not-visible` | what a property reads of T is not visible | - | run 35, run-atomic 26 | - | kill | killed |
| `r16-init-global-write-dropped` | initialiser that assigns a global is not a write | - | - | - | kill | pinned by a directed test |
| `r17-init-scope-of-first-target-only` | an initialiser is resolved in the scope of the first target of a pool only | - | - | - | kill | pinned by a directed test |
| `c1-closure-no-dstep` | closure does not follow `DStep` | atomic 4, base 1 / tight: atomic 4 | base 7, atomic 56 | - | kill | killed |
| `c2-whole-not-set-for-dstep` | no whole reading of the location a d_step enters | base 2 | base 4, atomic 6 | - | kill | killed |
| `c3-dirok-ignores-whole` | directed ends in else and d_step/atomic locations | atomic 3, base 2 / tight: atomic 2 | base 7, atomic 20 | - | kill | killed |
| `c4-else-siblings-not-whole` | else siblings read with the pc refinement | atomic 1, base 1 | base 5, atomic 1 | - | kill | killed |
| `p1-send-room-not-required` | requirement: send without room allowed | base 2 | base 22 | - | kill | killed |
| `p2-recv-message-not-required` | requirement: receive on an empty channel allowed | base 8 | base 56 | - | kill | killed |
| `p3-requirement-reads-channel-0` | requirement always reads channel 0 | base 3 | base 13 | - | kill | killed |
| `x1-assert-reads-dropped` | the reads of an assert are not in the footprint | reads 336, atomic-reads 327 / tight: atomic-reads 326 | reads 337, atomic-reads 349 | - | kill | killed |
| `x2-clearchans-write-dropped` | the clear of a channel is not a write of it | atomic 1, reads 3, atomic-reads 1, base 3 / tight: atomic 1, atomic-reads 1 | base 19, atomic 13, reads 29, atomic-reads 13 | - | kill | killed |
| `x3-recv-match-reads-dropped` | the match of a receive argument is not read | reads 6 | reads 24, atomic-reads 2 | - | kill | killed |
| `x4-send-args-reads-dropped` | the arguments of a send are not read | reads 88, atomic-reads 17 / tight: atomic-reads 16 | reads 204, atomic-reads 39 | - | kill | killed |
| `x5-effect-index-reads-dropped` | the index of an effect target is not read | reads 46, atomic-reads 11 / tight: atomic-reads 9 | reads 122, atomic-reads 75 | - | kill | killed |
| `x7-dynamic-index-is-element-0` | an array element chosen by a non-constant is element 0, not the whole array | - | base 1, atomic 4, atomic-reads 2 | - | kill | killed |
| `x10-assert-does-not-block-eligibility` | `!u.assert` removed: a location with an assert may be expanded alone | - | - | - | kill | pinned by a directed test |
| `x11-reads-vs-others-writes-dropped` | what the process reads is not checked against what the others write | atomic 6, run 22, run-atomic 16, reads 170, atomic-reads 38, base 18 / tight: atomic 6, run-atomic 14, atomic-reads 32 | base 135, atomic 169, run 95, run-atomic 71, reads 521, atomic-reads 302 | - | kill | killed |
| `x15-len-read-is-send-end` | the length of a channel is a read of its send end only | reads 2, base 1 | base 5, reads 6, atomic-reads 2 | - | kill | killed |
| `x17-reach-not-visible` | what a reach property reads is not visible | - | base 6, atomic 6, run 23, run-atomic 13, reads 2, atomic-reads 4 | - | kill | killed |
| `x20-recv-index-reads-dropped` | the index of a receive that binds an array element is not read | reads 3 | reads 10, atomic-reads 3 | - | kill | killed |
| `x10a12-assert-guards-both-removed` | a combination of x10 and a12 (second cross-review): a failing assert may be expanded alone and counts as leaving the stack | - | - | - | kill | pinned by a directed test: a false `verified` on hand-written IR |
| `x21-recv-bind-write-dropped` | the write of the variable a receive binds is not in the footprint (the second review's probe q5) | - | - | - | kill | pinned by a directed test |

After the second cross-review the harness has 54 mutants: the 52 of the table above, the
combination of x10 and a12, and x21; a12 is pinned. The classes are 45 killed by an oracle, 7
pinned by a directed test (r12, r16, r17, x10, a12, x10a12, x21) and 2 that survive by argument
(a4, r13); `go run ./cmd/pormut -list` shows them. Not all of them were run again. The
analysis (`por.go`) is the same as in the full run above, and the full run was not repeated;
the mutants the round changed or that its changes could affect were run on the final tree
(8 mutants, 3 000 models per generator, `MCD_POR_ALL=1`, three workers, load average 1 to 2,
baseline green on all four layers first, the layers checked by the new preflight): a4 and r13
survive (equivalent), a10 and a11 are killed with the counts of the table above (a10: O1 tight
`loop` 112, O3 tight `loop` 807; a11: O1 tight `run-atomic` 3, O3 tight `atomic` 46, `loop` 92,
`run-atomic` 63), a12, x10, x10a12 and x21 are pinned by a directed test with the oracles green:
**0 against the contract**.

The two that survive by argument: **a4** (the atomic edge's own channel operation keeps the
directed ends): the plan's argument is that only the first edge's own operation is
concerned and its continuations are whole by the location rule; it survived 3 000
models of each of four generators and, in a second run of the harness with the four
survivors of that time alone (a4, a12, r13 and r16), 20 000 models of each generator the mutant names (O1, O2, O3 and
the tight variants all green; the baseline at that size green too); in the run of the
table above it also survived 3 000 models of `atomic-reads`, whose continuations send
and receive with arguments that read a scalar. **r13** (the `run` does not
read the dormancy of its slots): every writer of the counter at the dormant
location is a `run`, which writes it too, or an edge that leaves the table
(check c1), which writes T, so the write already orders it; the same finding as the
pool-slot cell of the plan's first prototype. (a12, which was the third, is not one
of them since the second cross-review: below.)

**r16 and r17 are not equivalent, and the first version of this file was wrong to
say so** (the first cross-review of the diff found it). r16 drops the write of an
initialiser that assigns a global; r17 resolves the initialisers in the scope of the
first target of a pool only. The frontend and the generators never make either shape
(an initialiser assigns a local of the new process, which is not a cell, and a pool
is the copies of one proctype), so the oracles stay green; but `mcd check --ir` and
`mc_check` with an inline `ir` accept both, and r16 is a **false `verified`** by hand:
W waits for `g == 1` beside a free edge and the only writer of `g` is the initialiser of
S's `run`; with W before S in the process order the reduced search answers `assert`
verified (3 states) where the full one finds the violation (6 states). They are
pinned by directed tests in every order of the processes
(`TestPORAnInitialiserThatAssignsAGlobalIsAWriteOfIt`,
`TestPORAHeterogeneousPoolIsTheUnionOverItsTargets`: an initialiser that is a
local of one instance and a global for the other, and instances with bodies of their
own); the harness classifies them "pinned", which means the oracles are green and a
directed test is red. The red check was done: the r16 trap fails in the three orders
in which W precedes S and passes in the other three on the r16 mutant, and passes
on production in all six.

**x10 and a12 cover for each other** (the second cross-review found it). x10
(`!u.assert` removed from the eligibility of a location, so that a location with an
assert edge may be expanded alone) and a12 (an error or a failing assert inside a chain
counts as leaving the stack) were each argued redundant from the other. x10: "an assert
that would fail in the state is a failing micro-step in the walk of the cycle proviso
(`leavesStack`), which counts as not leaving the stack, so the state is expanded in
full and the ordinary search finds it". a12: "the ample set then contains the erroring
move, which the ordinary search takes and reports". In production a failing assert never
reaches the walk, because `!u.assert` keeps every location whose closure has an assert
out of the ample sets, so the clause of the walk is dead code there; and nothing pinned
it: a12 passed the directed layer, and O3 skips the steps that fail
(`por_acyclic_test.go`), so a cycle through a failing step is invisible to every oracle.
Each is harmless only while the other stands. Removed together they are a **false
`verified`**, on hand-written IR: P has `0 -> 1` atomic and `1 -> 0 assert(false)`, Q has
`0 -> 1 x = 1`, the properties are assert, the invariant `x != 1` and deadlock. The full
search, production, x10 alone and a12 alone find the invariant violated; with both removed
`mcd check --ir --sweep --por` answers the invariant `verified` with one state stored (P
is expanded alone at the initial state, its step ends in that same state, Q never moves).
Reproduced on scratch builds of the four trees (the answers are the same with and
without `--por` for the first three). Now each clause is pinned on its own: x10 by
`TestPORAssertEdgeIsVisible` (as before), a12 by
`TestPORAFailingAssertInAChainIsNotALeavingStepWhateverTheAnalysisSays`, which makes P
eligible by hand (`forcedPlan`) and wants the full expansion at the initial state of the
model, in both orders of the processes, with a control (an assert that holds, whose chain
leaves for a fresh state, is picked) so that the test cannot pass because the plan is never
used; and the verdicts of the model are pinned, in every order, by
`TestPORAFailingAssertInAChainDoesNotHideTheInvariant`. Red check, on scratch builds of the four trees: the forced-plan test fails on a12 and
on the double mutant (both orders of the processes, both failing-assert shapes, and
not the control), the verdict test fails on the double mutant alone (the invariant is
`verified` with the reduction and violated without, in both orders),
`TestPORAssertEdgeIsVisible` fails on x10 and on the double mutant; production passes all
three. a12 is therefore "pinned", and no longer "power": the argument that it only
loses power was circular. The harness has the double mutant as a combination
(`x10a12-assert-guards-both-removed`: `with` names two mutants that are applied to the same
copy). What no oracle can see: O3 skips the failing steps, and no generator makes a failing
assert on a cycle of a chain; the directed tests are the only pin. The clause of x10 is kept
as the textbook rule (an assert is visible, C2); it costs reduction only in the processes
that assert.

What the first runs forced: five mutants (a8, a10, a11, r8, r12) were first killed
only by a directed test and one (r7) by nothing, so the generators were extended
until the oracles alone killed all of them but r12. The closure that does not cross from a d_step to an atomic
edge (a8): the d_step-into-atomic chain was added to `genAtomic`. The chain limit
and the budget answering "leaves" (a10, a11): no random model reaches 100 000
micro-steps, so O1 and O3 also run with tight limits (a chain of more than two
micro-steps, more than three fired in a pick), through two unexported test-only
options of `explore.Options`; and `genLoop` got chains of one to three atomic
edges. The dormant counter not written by the `run` (r8): my first waiter
generator put the assert on the guarded edge itself, which makes the waiter's
location ineligible and the trap vacuous (found because the mutant survived);
the waiter now leads to a location of its own. The footprint of a `run` that
covers only the first slot of a pool (r7): needs a waiter on the second member
and a creator that makes two runs (it survived 3 000 models until that shape was
common). One I had wrong: I expected "`youngest` reads dropped" (r3y) to be
equivalent, because the end edges that carry the guard also write the table; the
generator's guards on `youngest` in ordinary edges kill it. One is not a verdict
at all: check c2 (a `run` that enters its target at the dormant location). The
target would keep looking dormant and a second `run` would take its slot and
overwrite its locals, but with the dormant location as its entry the target has
no edge to observe them from, and an edge out of the dormant location writes the
counter cell the run reads and writes. Removing the check changes nothing the oracles see: the harness
ran the mutant at 50 000 models per generator (`run`, `run-atomic`, `base`; one
model in twenty of the two run generators enters its targets at the dormant
location) through O1, O2, O3 and the tight variants, and all stayed green. It is
kept because the plan asked for it, a directed test pins its reason, and the
harness classifies the mutant as "pinned". The argument is for the engine's
present semantics; a change to how `run` or a dormant process works must look at
it again.

## What the oracles can and cannot see

A claim about the strength of the random layers is a claim about the shapes the
generators make. The plan said that O2 "sees a hole as soon as one state exhibits
it"; that holds for a hole in a clause whose shape some generator builds, and for
nothing else. The first cross-review put eleven mutants of the footprint clauses
(rule F in the data) through the harness: at 2 000 models per generator six of them
survived O1, O2 and O3 and were killed only by directed tests; at 12 000 models one
of the six (the reads of an assert) was killed by the audit. Five of the six are real
clauses, not equivalent mutants: the directed tests of `por_reads_test.go` (written
after the second review of step 2, which had found the same blind spot at 15 000
random models) kill them, and a model that exposes each is a few lines long. The
sixth, the `!u.assert` test of the eligibility, is argued redundant and pinned (x10,
above).

The cause, measured: the base, atomic, loop, run and provided generators send and
match constants, bind a local or a scalar, choose an array element by a constant or
a local, and make an assert over a scalar that other processes also read. Of 600
models of `base` and of `atomic`, none has a send argument, a receive match, a
receive index or an effect index that reads a scalar another process writes. The
generators `reads` and `atomic-reads` were added to make those shapes (see above),
and with them, at 3 000 models, on production the three oracles are green and the
mutants are killed by an oracle; with the old generators alone, in the same run:

| Mutant | killed by an oracle on the old generators (base, atomic, loop, run, run-atomic, provided)? | on `reads` / `atomic-reads` |
|---|---|---|
| x3 receive match not read | no (0 failures in 3 000 models of each) | O1 6 / 0, O2 24 / 2 |
| x4 send arguments not read | no | O1 88 / 17, O2 204 / 39 |
| x5 effect index not read | no | O1 46 / 11, O2 122 / 75 |
| x20 receive index not read | no | O1 3 / 0, O2 10 / 3 |
| x1 assert reads not in the footprint | O2 on `atomic` only, 1 failure in 3 000 | O1 336 / 327, O2 337 / 349 |

A rare shape is a different failure of the same kind. The second cross-review's probe q5
(the write of the variable a receive binds is not in the footprint; the harness has it as
`x21-recv-bind-write-dropped`) leaves O1, O2 and O3 green on all eight generators at 3 000
models and is killed by directed tests only (`TestPORRecvBindIsAWrite`,
`TestPORDirectedChannelsRespectTheVisibleCells`); the audit sees it at 20 000 models (`base` 1
failure, `atomic` 3). The harness at its default size is not sensitive to rare shapes, so
a mutant of such a clause is "pinned", not killed by an oracle.

What this does and does not establish. It establishes that these five clauses are now
seen by an oracle, in the shapes of `readShapes` (a scalar that another process
writes, read in a send's argument, a receive's match, a receive's index, an effect's
index, an assert; the arrays are the process's own, but for the effects on the shared
array `a`, where the read cannot show: x5 is killed through the private shape). It does
not establish that the oracles see a hole in any clause whose shape no generator builds. The clauses and
shapes that no generator makes, and that only a directed test pins: a `run`
initialiser that assigns a global (r16, a false `verified` by hand, found by the
review), initialisers resolved per target of a heterogeneous pool (r17), an atomic
edge into a sink location (`atomicIntoASink`), the length of a channel named by a
value (`clen`, `cfull`; `por_reads_test.go`, no mutant), the `!u.assert` test (x10,
argued redundant above) and check c2 (r12). Everything the plan lists as "what the
oracles do not cover" in "What was not verified" below stands.

## Scale runs

300 000 models per generator, seeds that were not used while writing the rules
(`MCD_POR_SEED` 200 000 001 for `atomic` and `loop`, 300 000 001 for `run` and
`run-atomic`), three workers, `ulimit -v 8000000`, one job at a time, roughly 25 minutes for
each pair of generators, no failure anywhere. `K` = 2.

| Generator | O1 (smaller / applied / error in both / skipped) | O1 with tight chain limits | O2 pairs audited | O3 models checked |
|---|---|---|---:|---:|
| `atomic` | 46 664 / 278 561 / 15 815 / 2 320 | 45 129 smaller | 8 910 867 | 277 164 (tight 277 159) |
| `loop` | 165 322 / 300 000 / 0 / 0 | 148 216 smaller | 2 306 301 | 300 000 (tight 300 000) |
| `run` | 77 977 / 172 378 / 95 380 / 1 079 | | 6 368 606 (34 444 at a `run`) | 172 290 |
| `run-atomic` | 72 276 / 171 190 / 97 037 / 834 | 71 639 smaller | 4 732 004 (26 859 at a `run`) | 171 073 (tight 171 069) |

("Error in both": the two searches stop on an error of the model, an
evaluation error or an exhausted process pool, in both or in neither; the
properties of those models are not compared, only the reachability of the
error.) Also: 100 000 Promela models of the fuzzer through the frontend, engine
in full and reduced: no disagreement, the reduction applied to all of them and
smaller on 85 396 (`MCD_POR_FUZZ=100000`). `go test -race` on the oracles with
three workers and 300 models per generator: clean.

The two series above were taken before the last change to a generator (the waiter
of `genRun` became the common shape, with a bias to a member after the first of
its pool, which is what kills the mutant that covers the first slot only). The
whole set was therefore run again on the final generators and the final test
tree, 100 000 models per generator (`MCD_POR_SEED` 400 000 001), no failure:

| Generator | O1 (smaller / applied / error in both / skipped) | O1 tight: smaller | O2 pairs audited | O3 models checked (tight) |
|---|---|---:|---:|---:|
| `base` | 23 855 / 93 631 / 5 173 / 37 | | 1 952 187 | 93 596 |
| `atomic` | 15 613 / 92 871 / 5 213 / 827 | 15 065 | 2 991 339 | 92 393 (92 392) |
| `loop` | 55 297 / 100 000 / 0 / 0 | 49 559 | 770 678 | 100 000 (100 000) |
| `run` | 27 717 / 55 310 / 34 053 / 630 | | 3 374 967 (21 089 at a `run`) | 55 311 |
| `run-atomic` | 25 447 / 54 744 / 34 836 / 461 | 25 243 | 2 428 320 (16 557 at a `run`) | 54 732 (54 732) |

The audit with K = 3 (three macro-steps of the others in a row, on the models of
at most 500 stored states; K = 2 otherwise), 100 000 models of `atomic`,
`run-atomic` and `run` (seed 500 000 001): 3 058 668, 2 349 374 and 3 231 070
(state, process) pairs audited (16 414 and 21 191 at a `run`), no failure.

### Scale runs after the first cross-review (new seeds, with and without the reads)

The first cross-review's fixes changed the analysis in one place (the budget of the
proviso is one per pick) and the oracles in three (the two generators of reads, the
floors, the tight-limit floor), so the oracles were run again at the plan's size on
seeds that nothing had used: 300 000 models per generator, on all seven generators the
reduction applies to (`provided` is refused and has no scale run), three workers,
`ulimit -v 8000000`, `MCD_POR_ALL=1`, `K` = 2, one job at a time, 2 h 16 min in all
(load average 4 to 7), on the analysis of commit `7490194` and the oracles of `ec7caed`
(`MCD_POR_SEED`: `base` 600 000 001, `atomic` 610 000 001, `loop` 620 000 001, `run`
630 000 001, `run-atomic` 640 000 001, `reads` 650 000 001, `atomic-reads` 660 000 001).
**No failure anywhere**, in O1, O2, O3 and the tight variants. The first two rows are the
generators with the new shapes; the other five are the old generators, the control.

| Generator | O1 (smaller / applied / error in both / skipped; refused) | O1 tight: smaller | O2 pairs audited | O3 models checked (tight) |
|---|---|---:|---:|---:|
| `reads` | 125 434 / 287 253 / 10 842 / 730; 1 175 | | 18 237 766 | 286 831 |
| `atomic-reads` | 41 915 / 269 722 / 16 524 / 10 426; 3 328 | 40 573 | 11 369 572 | 266 031 (266 013) |
| `base` | 71 701 / 280 807 / 15 540 / 138; 3 515 | | 5 846 489 | 280 719 |
| `atomic` | 47 080 / 278 572 / 15 696 / 2 381; 3 351 | 43 947 | 9 023 883 | 277 195 (277 188) |
| `loop` | 165 270 / 300 000 / 0 / 0; 0 | 89 897 | 2 310 851 | 300 000 (300 000) |
| `run` | 82 890 / 165 877 / 102 172 / 1 833; 30 118 | | 9 973 616 (60 752 at a `run`) | 165 881 |
| `run-atomic` | 76 374 / 164 187 / 104 257 / 1 426; 30 130 | 74 444 | 7 205 772 (46 512 at a `run`) | 164 125 (164 123) |

The "smaller" count of the tight runs of `loop` is lower than in the earlier series
(89 897 of 300 000 against 148 216): the tight budget of three micro-steps is now shared by
the candidates of a pick. The refused models of `run` and `run-atomic` are the ones the
generator makes to be refused (a dynamic end that does not leave the table, a `run` that
enters at the dormant location). `go test -race` on the oracles, three workers, 300
models per generator on all eight generators: clean. Not repeated in this round: the
audit with K = 3, the fuzz of 100 000 Promela models through the frontend, and
`go test -race` at scale (see "Cross-review", "Not verified").

### Scale runs after the second cross-review (new seeds, the final tests)

The correction round changed no analysis (`por.go` is the file of `7490194`) and none of the
generators' draws, only the tests, the harness, the floors and the records, so the oracles were run
again at the plan's size on seeds that nothing had used: 300 000 models per generator, on all seven
generators the reduction applies to, three workers, `ulimit -v 8000000`, `MCD_POR_ALL=1`, `K` = 2,
one job at a time, on a shared machine (load average 1 to 11 while they ran), resident memory
under 20 MB. `MCD_POR_SEED`: `base` 920 000 001, `atomic` 930 000 001, `loop` 940 000 001, `run`
950 000 001, `run-atomic` 960 000 001, `reads` 970 000 001, `atomic-reads` 980 000 001. `base` and
`atomic` ran on the floors before the last rule of the previous section (they pass the final floors
too: a floor can only have got lower), `loop` was run twice, the first time on those floors and
failing only the tight floor with the counts below, the other five on the final tests, `5d71289`.
**No failure anywhere**, in O1, O2, O3 and the tight variants.

| Generator | O1 (smaller / applied / error in both / skipped; refused) | O1 tight: smaller | O2 pairs audited | O3 models checked (tight) |
|---|---|---:|---:|---:|
| `base` | 71 957 / 280 981 / 15 462 / 115; 3 442 | | 5 865 494 | 280 883 |
| `atomic` | 46 798 / 278 428 / 15 798 / 2 502; 3 272 | 43 588 | 8 996 043 | 277 076 (277 066) |
| `loop` | 165 680 / 300 000 / 0 / 0; 0 | 90 218 | 2 311 198 | 300 000 (300 000) |
| `run` | 83 017 / 165 917 / 102 302 / 1 762; 30 019 | | 9 856 723 (64 850 at a `run`) | 165 886 |
| `run-atomic` | 76 218 / 163 938 / 104 726 / 1 353; 29 983 | 74 284 | 7 186 340 (47 234 at a `run`) | 163 894 (163 889) |
| `reads` | 125 828 / 287 408 / 10 687 / 747; 1 158 | | 18 431 917 | 286 960 |
| `atomic-reads` | 41 819 / 270 094 / 16 311 / 10 270; 3 325 | 40 437 | 11 335 789 | 266 439 (266 428) |

The counts are of the size of the earlier series (other seeds; the "smaller" count of the tight run
of `loop` is 90 218 here against 89 897 in the first review's series: the same budget shared by the
candidates of a pick). Not repeated in this round: the audit with K = 3, the fuzz of 100 000 Promela
models through the frontend, `go test -race` at scale, the SPIN fuzz.

## The outside witnesses

- **SPIN's pan, `-DNOREDUCE`, on fuzzed Promela** (`tools/pandiff/porfuzz_test.go`,
  O5): 400 models (seeds 1 000 001 to 1 000 400) in the shapes of `init { atomic {
  run ... } }`, `init { run ... }` and `active` with `atomic`, `d_step`,
  `if`/`else`, bounded loops, `_nr_pr` guards and a buffered channel, run with
  `MCD_POR_FUZZ_SPIN=400` (704 s): **337 agree with pan for the engine in full
  and reduced**, 27 skipped (the frontend rejects the model, a search ends on a
  budget or an error of the model, pan does not compile or does not finish), 36
  not compared (below), 277 of the 337 reduced. No disagreement. The counts are
  those of the generator as committed, rerun after the first cross-review (the
  orchestrator's rerun and the second reviewer's gave the same figures). An
  earlier version of this file said "344 agree / 19 skipped / 37 not compared /
  293 reduced": that was measured with an earlier version of the generator and
  does not reproduce. **What is compared, precisely:** whether pan reports *some*
  error (`pan.Errors > 0`) against whether the engine reports *some* property
  `violated`, for the full and the reduced search each. It is neither per
  property nor per class of error; the per-class comparison is the one of the
  corpus below. The engine-only comparison of the same generator (full against
  reduced, every status and evidence, through the frontend; `MCD_POR_FUZZ`,
  default 400 models) is a separate test.
- **The corpus against pan with the reduction applied**: the 51 files of the
  differential test of G1/G5; with `MCD_POR_CORPUS_SPIN=100`, 34 are reduced and
  agree with pan on the verdict and on the error class, 17 are refused (the full
  search, compared by the older test). The default run of the test compares the
  first 12 that are reduced: each costs a `spin -a` and a `gcc` run.
- **The corpus against the full search** (`por_corpus_test.go`): of the 86 models
  that both searches finish, 0.2.0 applied the reduction to 45 and shrank 16, and
  refused 41; on the same 86 files it now applies to 60, shrinks 26 and refuses
  26 (10 rendezvous, 2 channels named by a value, 1 `timeout`, 3 `provided`, 10
  temporal properties). This step added five fixtures (`leader5`,
  `por-atomic-guard`, `por-atomic-blocked`, `por-rendezvous`, `por-run-pool`; the
  last one exhausts its pool and is not compared, the others are), and the
  figures the test measures at the head of the branch, rerun after the first
  cross-review (`go test -run TestPORAgreesWithTheFullSearchOnTheCorpus -v .`), are
  **158 Promela files, 106 accepted, 90 compared, 63 with the reduction applied, 29 of
  them smaller, 27 refused** (11 rendezvous, 2 channels named by a value, 1
  `timeout`, 3 `provided`, 10 temporal: the sum is 27). An earlier version of this
  file said "62 / 28 / 27 of 89 with the three fixtures": that count was taken
  before `leader5.pml` was added and was stale. The test pins a floor of 63 / 29
  and requires `bench-sym`, `leader3` and `nrpr` to shrink.

## Measurements

One machine, 16 cores shared with other agents; `uptime` load average at the
start of each series is in the last column; times are wall-clock of the whole
`mcd check --sweep --no-timing` process (min / median / max over the repeats),
states and memory are the engine's own (`memory_bytes_est` is the estimate of
the visited set and the stack, not the resident size). The series were taken in
a quiet window (load 1.1 to 1.3); they are indications, not a benchmark.

| Model | Full: states, time | `--por`: states, time | Load |
|---|---|---|---:|
| `bench-sym` N=4 | 10 420, 12 ms (11 to 16) | 1 471, 6 ms | 1.3 |
| `bench-sym` N=5 | 76 516, 70 ms (65 to 76) | 5 522, 8 ms | 1.3 |
| `bench-sym` N=6 | 543 076, 631 ms (584 to 657), 25 MB est | 19 914, 17 ms, 0.9 MB est | 1.3 |
| `bench-sym` N=7 | 3 762 340, 5 280 ms (5 189 to 6 312), 183 MB resident | 69 811, 57 ms | 1.1 |
| ring `leader3` | 679 | 76 | |
| ring unrolled to 5 (`leader5.pml`) | 41 692, 55 ms | 108, 6 ms | 1.3 |
| ring unrolled to 6 | 341 316, 588 ms, 86 MB est | 136, 7 ms | 1.2 |
| ring unrolled to 7 | 2 801 652, 7 657 ms, 809 MB est | 164, 9 ms | 1.1 |
| `nrpr.pml` | 31 | 21 | |
| `you_run2` / `karpov/03-state-race` | 14 / 56 | 11 / 52 | |
| `atomic-at`, `-t1`, `-t3`, `-t4`, `-t6` | 11, 10, 17, 9, 10 | 9, 9, 15, 8, 9 | |
| `por-atomic-guard`, `por-atomic-blocked` (fixtures) | 10, 5 | 8, 3 | |

The N=7 row settles what the plan could not: the full search of `bench-sym` N=7
finishes (3 762 340 states, `deadlock` verified, 183 MB), so the verdict of the
69 811-state reduced run is confirmed against it. The growth with the number of
nodes of the ring is linear on the reduced side (76, 88, 108, 136, 164 states for
3 to 7 nodes: 28 more per node from 5 on), against a factor of about 8 per node in
full; `bench-sym` grows by a factor of about 7 per process in full and 3.5 reduced. The
18 models of the corpus whose count changed from 0.2.0 are the ones in the plan's
table, with the same counts.

The cost of the analysis and of the longer proviso where there is nothing to
gain was looked at on the model where `--por` applies and shrinks nothing,
`CH5/sink_source_filter` (2 353 659 states, eight-slot channels over three values):
the 0.2.0 binary and the current one, alternating, five runs each, `--por
--sweep`: 3.51 s and 3.51 s (minimum), 3.72 s and 3.65 s (median), the same
states (load average 11.6, so a comparison of two runs taken together and not a
benchmark). The smaller models of the oracles are too fast to show a difference.

What these numbers do not say: "applied" is not "smaller" (34 of the 63 models
the reduction applies to do not shrink, almost all toys of 1 to 20 states);
`bench-sym` and the ring are the two models that matter and both are synthetic
or hand-unrolled; the reduced counts are those of the reduced graph and are not
comparable with `pan -c0`.

## What is refused, and why

| Refused | Reason in the report | Why it stays |
|---|---|---|
| rendezvous channel | "the model has a rendezvous channel (...): a handshake moves two processes in one step" | rejected: a handshake is one transition of two processes, so a real gain needs a group ample set; the plan measured 28 files and found no model with two independent handshakes |
| channel named by a value | "the model names a channel by a value (a dynamic channel)" | postponed: the dependence relation depends on the state; `CH9/leader` is the one beneficiary (41 692 states, about 100 once its channels are static) |
| `timeout` | "the model uses timeout, which reads whether any other process can move" | postponed: sound in principle, no measurable gain on the 3 files that use it |
| `provided` | "the process P has a provided clause, which guards every step of the process and which this version of the reduction does not model" (the 0.2.0 text called it "a priority", which it is not in this engine) | not implemented by decision 2 |
| temporal properties, `--bfs`, a vacuity watch | as in 0.2.0 | step 4 measured that they unlock nothing that needs them |
| a dynamic process that goes back to its dormant location without leaving the table; a `run` that enters its target at the dormant location | "...re-enters its dormant location without leaving the process table", "a run of D enters it at its dormant location" | the frontend never emits either; the first is a real condition of the soundness argument, the second is pinned |

## Found along the way, not changed: `_nr_pr` in a model without `run`

The Promela frontend gives the `-end-` edges of a process the `Leave` of the
live-process table only when some process of the model is created by `run`
(`lower.go`). A model that merely reads `_nr_pr` has the table (so `_nr_pr` is a
state variable) but nothing ever leaves it: `_nr_pr` stays at the number of
processes that started with the system. SPIN's goes down when a process ends.

```
active proctype A() { (_nr_pr == 1) }
active proctype B() { skip }
```

The engine answers `deadlock` **violated** (3 states, in full and with `--por`);
`pan -DNOREDUCE` finds no error (5 states stored). It is a difference of the
default search that predates this step, it changes the verdicts of every model
that reads `_nr_pr` without creating a process, and fixing it changes default
results, so it is not fixed here. It does not touch the reduction: the table
then has no writer, T has no writer, the reads never conflict, and the reduced
and the full search share the difference, which is what the oracles compare. The
SPIN fuzzer does not compare such models with pan (37 of 400). **Any
disagreement with SPIN on a model that reads `_nr_pr` and starts its processes
with `active` is this bug.** If a later step gives the static processes `Leave`
edges, the rule above already covers them.

## What was not verified

- The soundness argument (the macro-step induction with the exclusive-byte
  equivalence, the table cell, the two checks) is the plan's, reviewed once on
  paper; it has not been proved mechanically. The oracles check it on random
  models up to K = 2 macro-steps of the others at every stored state they visit
  (K = 3 on small models in the scale runs), not in general; the audit shares the
  engine's enabledness and firing code, so a misreading of the semantics shared
  by the full and the reduced search is invisible to it (SPIN is the outside
  witness for that, on the shapes the fuzzer makes).
- Models of more than 5 000 states under the oracles (their budget): behaviour
  on large models is seen only through the corpus, the benchmarks and the two
  scale series above.
- Anything the generators and the fuzzer do not generate: `else` after a `run`,
  atomic blocks with rendezvous (refused), heterogeneous pools and an `Init` that
  assigns a global (hand-built IR only: two directed tests pin them in every order
  of the processes, no random model makes either shape), `timeout` and `provided`
  under the reduction (refused).
- Other platforms and the release gate: `engine/bin`, `SHA256SUMS`,
  `BUILD-INFO.json` and the version are untouched; the plugin was not rebuilt or
  smoke-tested on any platform; the manifest and MCP declaration are unchanged
  but for one description string in `mc_check`.
- That the two surviving mutants are equivalent (a4, r13): each has an argument in
  the data and survived the runs described above, none has a proof. (r16 was in this
  list and is not equivalent, and a12, which only "loses power", is not harmless by
  itself: see the paragraphs above.)
- The interplay with the parallel-exploration stream (step 5): not merged.
- A performance claim under any load other than the one stated.

## Deviations from the plan, and why

- The mutation runner is `engine/cmd/pormut` with its data in
  `engine/tools/pormut/`, not "a short runner in `steps/`": a Go program has to
  live in the module.
- The Promela fuzzer is Go (`tools/pandiff/porfuzz_test.go`), not Python: no new
  language in the repository, and it runs under `go test` (the SPIN part skips
  without `spin` and `gcc`).
- O1 and O3 also run with tight chain limits, through two test-only options, to
  kill the limit mutants with an oracle (the plan asked for a directed test).
- `explore.go` is not touched in comments only: `Options` gained three
  unexported test-only fields (`porTrace`, `porChainLimit`, `porPickBudget`, next
  to the existing `porNoProviso`) and the line that builds the `porRun` passes
  them. The other stream edits the same struct; the merge is a few lines.
- The default size of an oracle run is 1 000 to 8 000 models, not 8 000 each, to
  keep the package at about 45 s (about a minute and a half since the two
  generators of reads were added).
- `genProvided` exists although `provided` is not implemented (decision 2): it
  checks that a refusal is the full search.
- The `mc_check` `por` parameter text (a Go string) no longer lists atomic and
  `run` as refused; the plan expected it unchanged.
- A fixture was added that the plan did not list, `testdata/promela/leader5.pml`
  (2.4 KB, the ring unrolled to 5), because its full count is that of
  `CH9/leader.pml`.

## Cross-review

### Round 1: the diff at `b704f02`

Three blind reviewers on different models read the diff of this branch; the
orchestrating agent verified every finding against the code and ran its own fuzzers
(its mutants of the footprint clauses, a SPIN fuzz rerun). Verdict: **approve with
changes. No model was found on which the reduced search answers differently from the
full one**; the defects were in tests, in the strength of an oracle and in records,
and one of them (r16) is a false `verified` on hand-written IR that the records had
called equivalent. The reviewers' answers are not kept in the repository; the
decisions are below, with the commits that carry them.

| # | Finding | Severity | Decision | What changed |
|---|---|---|---|---|
| 1 | r16 (an initialiser that assigns a global is not a write) labelled "equivalent" is a wrong `verified` on IR given to `mcd check --ir` / `mc_check`; no directed trap; a regression would survive the harness | medium | **fix** | `0236921`: a directed trap in every order of the processes (red on the r16 mutant in the three orders in which W precedes S, green on production), a test for a heterogeneous pool (the initialiser is a local of one instance and a global for the other; instances with bodies of their own), one for an atomic edge into a sink; r16 "pinned", the new mutant r17 pinned; records corrected |
| 2 | `BenchmarkEngineSymN6POR` expects 543 076 states and fails (19 914 since the atomic reduction); the default suite does not run benchmarks | low | **fix** | `1982b7f`: the count and the comment; all benchmarks of the three packages that have any run once with `-benchtime 1x` |
| 3 | stale corpus numbers (62 / 28 / 27 of 89) | low | **fix** | `9e90ebd`: 158 files, 106 accepted, 90 compared, 63 applied, 29 smaller, 27 refused; floors 63 / 29 |
| 4 | the 10 000 micro-step budget is not "per pick": `leavesStack` took a fresh one per candidate | low | **fix, by making the code match the documents** | `7490194`, `134c35c`: `choose` allocates the budget once and the candidates spend from it. Chosen over rewording to "per candidate" because the plan, the header comment and these records all say per pick, because a pick would otherwise cost up to the number of eligible processes times 10 000, and because the change is three lines whose only effect is on a pick that fires more than the budget in all (no corpus model does; exhaustion answers "not leaving", so soundness is unchanged). Test first (red, then green): a candidate that spends one micro-step and is rejected, behind which a chain of exactly 85 is picked under a budget of 85 only if the budget is per candidate. The harness anchors of b4 and a7b had to follow: they were broken at the three commits from `7490194` to `7dde2a1` (I ran the harness on a10 and a11 only, and not `go test ./tools/pormut`, before committing) and are repaired by `134c35c`; every commit from there on has the anchors test green |
| 5 | the SPIN fuzz counts do not reproduce; the comparison is not stated precisely | low | **fix** | `eeaaad3`: rerun once, 337 / 27 / 36 / 277 (same as the orchestrator's); the record says what is compared |
| 6 | the `CopyEngine` comment says "without version control files" but only `bin` is skipped | low | **fix, the clause dropped** | `9a814d9`: the engine directory contains no `.git*` entry (the directory or the worktree's file is at the repository root, outside the tree that is copied), so there is nothing to skip |
| 7 | oracle blind spots: six footprint clauses survive O1, O2, O3 at 2 000 models and are killed only by directed tests | low | **fix** | `ec7caed`, `f29727c`: the generators `reads` and `atomic-reads`; the eleven probes as mutants (rule F); the whole harness re-run (52 mutants, baseline green first); the oracles at 300 000 models on new seeds, with and without the new shapes; the claims about the strength of the oracles limited to the shapes the generators make ("What the oracles can and cannot see") |
| 8 | `TestPORDifferentialWithTightChainLimits` has no floor on "smaller" | low | **fix** | `7dde2a1`: floors 1 in 8 (`atomic`), 1 in 4 (`loop`), 1 in 5 (`run-atomic`), measured just below what the run gives; red check: the tight budget lowered to one fails the floor for `atomic`, `loop` and `atomic-reads` |
| + | records of models that read `_nr_pr` (README, engine-tools, workflow, PROVENANCE, this file) | note | **checked, one sentence each** | `9e2fbd3`: none said "refused", none said what happens; they now say a model that reads `_nr_pr` is reduced where the table rules allow it (`nrpr.pml`: 31 states, 21 with `--por`), and that the creating step and the ends of processes stay unreduced |

What the fixes found while being made. (a) The shared budget lowers what the tight
oracle reduces (`loop`: one model in three, was one in two), which is why item 8's
floors are what they are. (b) The mutant `a11` (the budget answers "leaves") is now
also killed by the verdict oracle at the tight limits, and `r7` (the run covers the
first slot of a pool only) is now also killed by a directed test. (c) The assert shape
(`t`, a scalar only the writer and an assert touch) was not among the four shapes the
review listed; it was added because the mutant of the assert's reads (x1) needed it
(without it only the audit on `atomic` saw it, once in 3 000 models).

Not fixed, recorded as the review's notes for the record. O1 on the `run`
generators compares only the reachability of an error for about a third of the models
(5 204 of 15 000 for `run`, 5 324 of 15 000 for `run-atomic` in the review; 102 172 and
104 257 of 300 000 in the scale run above): it is the "error in both" column, a
property of the generators (a pool that is run once too often), not a defect; larger
pools in a generator mode would add coverage and were not done. `genAtomic` never
makes an atomic edge into a sink (the frontend cannot emit one); a directed test covers
the shape (`TestPORAnAtomicEdgeIntoASinkKeepsTheVerdict`). An infinite atomic loop
overshoots `--budget-ms` by about 2x in both the full search and the base binary
(pre-existing, not touched). The mutants a4, r13 and a12: the arguments were checked by
the reviewers and accepted. Rejected: the finding that the README says four oracles (O4
is the corpus test, `perf6-plan.md` section 7.1), and one reviewer's "no defects".

Deviations from the findings. Item 7 asked to extend the generators; the shapes are
two new named generators (`reads`, `atomic-reads`) and not edits of `base` and `atomic`,
so that the old generators stay as the control of the comparison "with and without the
new shapes", and so that every seed of every earlier run still means the same model.
The assert shape is an addition (above). `x10` (`!u.assert`) is pinned with an argument
for its redundancy, as the review allowed ("argue it or pin it"): it is both. Items 1 and
4 are done as the findings say; item 6 drops the clause instead of skipping `.git*`,
for the reason in the table.

Not verified, as the review said to record them. The soundness argument remains a paper
argument plus the oracles; it has not been proved mechanically, and the oracles check
what the generators make. The 0.2.0 binaries in `engine/bin`, `SHA256SUMS` and
`BUILD-INFO.json` do not contain this change, while the documents (README, PROVENANCE,
the skill references, the `mc_check` parameter text) describe the wider reduction: a
rebuild and a smoke test on every platform are needed before a release. Also not
verified in this round: the audit with K = 3 and the 100 000-model fuzz through the
frontend were not repeated; the SPIN fuzz was run once at 400 models; the interplay
with the parallel-exploration stream (step 5) is not merged.

### Round 2: the correction round at `5765450` (base of the round: `b704f02`)

Three blind reviewers read the correction round of the first review (the per-pick
micro-step budget in `choose`, the generators `reads` and `atomic-reads`, 52 mutants, the
records). The orchestrating agent verified every finding. Verdict: **approve with changes;
all five findings are low and none changes a verdict of the engine.** The orchestrator found
no model on which `--por` answers differently from the full search, no crash, no hang
(as it reported: O1 on 30 000 models of each of seven generators on new seeds; its own stress
generator, with an atomic edge into a sink, random process order, `run` initialisers that
assign a global, heterogeneous pools, constants replaced by reads of scalars and an `else`
after an atomic run: O1 300 000 models, O2 about 1.2 million audited pairs, O3 60 000; 3 959 and
15 000 Promela models through the frontend; SPIN: the documented 400 models reproduce exactly
(337 / 27 / 36 / 277) and 200 fresh seeds (170 agree, 13 skipped, 17 not compared, 150
reduced) show no disagreement). The per-pick budget change was judged sound and
deterministic: none of about 880 000 candidates was accepted by the shared budget and rejected
by a fresh one, and it can only lose reduction, order-dependently, under a tight budget of
three. The numbers of the records reproduced. The reviewers' answers are not kept in the
repository; the decisions are below.

| # | Finding | Severity | Decision | What changed |
|---|---|---|---|---|
| 1 | the tight floors of O1 (`atomic-reads` 8, `atomic` 8) are too close to the rate: the test fails on fresh seeds with production code unchanged (`atomic-reads` smaller 394, 370, 435, 415, 434, 398 against 375: about 5% of the runs; `-short`: floor 46, default seed 48, about 24%) | low | **fix, and wider than asked** | `4d0685b`: every floor of the counts of models is a share less binomial deviations, sized to the run (6 deviations, 5 for the tight floors, half the share under `-short`), the shares measured on 24 000 fresh seeds per generator; measured at both sizes on 24 fresh windows: the old floors failed 1 of 8 default windows and 8 of 16 short ones (the tight floors, and the at-run floor of the audit, which the review had not named), the new ones fail none; the red check (tight budget lowered to one) still fails the floors of `atomic`, `atomic-reads` (by five models) and `loop` at the default seeds; "The floors of the oracle runs" above; and `5d71289`, found by the 300 000-model run: a floor is never closer than 5% to its mean |
| 2 | a12 and x10 depend on each other: a failing assert never reaches the chain walk in production (`!u.assert` keeps it out), so the clause of the walk was dead code that nothing pinned (O3 skips bad steps); removed together they are a false `verified` on hand-written IR | low (the engine's verdicts are unchanged; the false `verified` needs both clauses removed) | **fix** | `01127c7`: the double mutant reproduced on scratch builds (full, production, x10 alone, a12 alone: the invariant is violated; x10 and a12 together with `--por`: `verified`, 1 state); a directed test with `forcedPlan` that fails on a12 and on the double mutant and passes on production (with a control), a verdict test of the model in every order; a12 "pinned", both notes and both paragraphs reworded to "harmless only while the other clause stands"; the harness got combinations (`with`) and the double mutant as an entry |
| 3 | the harness applies anchors lazily, after the whole baseline, only for the `-only` set; a layer whose `-run` regex matches no test is green | low | **fix** | `38bf813`: before anything is copied or run, every anchor of the list is applied (in memory) and every layer is checked with one `go test -list`, all failures listed at once; unit tests first (red on the old `Run`: a stale anchor, a doubled anchor, a missing file, an empty regex, an all-skipped layer, an unparsable regex, an unknown `-only` id were all accepted and the baseline started); a static test that every oracle named by the layers still exists |
| 4 | the generator comment and the records overstate: the arrays a scalar indexes are not all the process's own (`readShapes` also indexes `a`), and the `t` assert is not made in one model in three | low | **fix** | `7cac199`: counters for the effect index on a private array against only on `a`, and for the assert on `t`; recounted: of 600 models 41 of the 285 `reads` and 94 of the 438 `atomic-reads` models with the effect-index shape have it only on `a` (244 and 344 on a private array), the assert on `t` is in 123 and 130 (667 and 663 of 3 000 at the default seeds: 22%); comments and records reworded |
| 5 | strength claims without the qualifier (PROVENANCE, the audit's comment); q5 not recorded | low | **fix** | `4d0685b` (the audit's comment), `310462f` (PROVENANCE, README, this file): "on the shapes the generators make; hand-built shapes are pinned by directed tests only", with a pointer to "What the oracles can and cannot see"; q5 is in the harness data as `x21-recv-bind-write-dropped`, "pinned" |

What the fixes found while being made. (a) The review named two floors; there were more: the
at-run floor of the audit (1 pair against 3 in half the short windows: the pairs come in
bunches, and only one model in forty has one) and the tight floors of `loop` and `run-atomic`.
It also showed that no "one in n" serves both sizes: the same share of 375 models and of 3 000
has a relative noise that differs by a factor of 3, and for the `loop` generator the models of
consecutive seeds are not independent draws (variance 1.7 to 3.0 times the binomial's at 375).
(b) With the harness's own preflight the real engine passes: all 54 anchors apply and all four
layers match tests. (c) The red check of the tight floors passes by five models for
`atomic-reads` (311 against 316) at the default seeds and fails the lowered budget in 64% of
fresh windows only: the budget moves that count by a quarter, and its noise is 3 to 5%. A paired
comparison (the same models with the default limits) would be sharper and was not built. (d) The
oracles at the plan's size caught a flaw of the new floors: five binomial deviations are 1.4% of the mean at
300 000 models and the share of `loop` under the tight limits (0.305) was 1.7% above the rate of that
window (0.3007); the run failed its floor with every oracle green. The floors now keep 5% below
their mean at any size (`5d71289`).

Not fixed, as the review's notes for the record. The chain limit (100 000) is unreachable under
the default pick budget of 10 000 (dead but harmless, documented); the `t`-assert skip rate in
`atomic-reads` (3.3% against 0.7%); r13's stated reason is true but indirect; a4 and r13 survive
the larger runs (the arguments hold); there is no mutant for "per-candidate budget" (the
directed test is red on the revert). Rejected: a reviewer's "the iteration order of `s.c.procs`
(a Go map) makes the choice non-deterministic" (it is a slice, `choose` loops by index) and "the
textbook rule is implemented by the proviso" (not a safe substitute: item 2).

Deviations from the findings. Item 1 asked for floors of one in ten for `atomic` and
`atomic-reads` and a measurement of every floor; the floors are shares with a binomial margin,
because one in ten is 37 of 375 under `-short`, where the count is 50 plus or minus 7 (a
failure on about one run in a hundred, not "vanishingly rare"). The O3 and audit-pair floors are
unchanged: they are 11 or more deviations below their counts at both sizes (measured on the same
24 windows). Item 2 asked for the double mutant as "a harness entry or a documented probe if the
harness supports it": the harness did not, and now does (`with`). Item 3 also refuses an `-only`
id that names no mutant and checks that the oracles named by the layers exist (a layer that is
half empty: O1 names three tests); neither was asked for.

Not verified in this round, as the review said to record them. `go test -race` at scale (it
does not work under `ulimit -v`); the audit with K = 3; the full harness run of 54 mutants (the
subset above was run: 8 mutants); the release gate: README, PROVENANCE and the skill references
describe the wider reduction while `engine/bin` and `SHA256SUMS` are the 0.2.0 binaries (a
rebuild and a platform smoke test are needed before a release); the interplay with the
parallel-exploration stream (step 5), not merged.

## After the integration (0.3.0)

Merged with the other four branches (`integration-0.3.0-notes.md`). The difference "Found along the way" (the
`_nr_pr` that never fell) is fixed by `fix/nr-pr-process-table`; its models are now reduced and the new `nrpr`
generator makes them for the oracles (300 000 models through O1 and O3, 30 000 through O2, no failure). The
interplay with the parallel stream is done: the parallel search is refused where the reduction applies. The
oracles were run on the merged tree at 300 000 models for `atomic`, `loop`, `run`, `run-atomic` and `nrpr`
and at 60 000 for `base`, `reads` and `atomic-reads`, no failure. The corpus figures on the merged tree: 190 files,
137 accepted, 120 compared, 75 applied, 35 smaller, 45 refused.
