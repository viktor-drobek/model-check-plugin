# G7 (performance plan, step 2): partial-order reduction of the safety search,
# opt-in with `mcd check --por`. Every scenario observes the engine through
# the CLI and its JSON report.
#
# What a reduction is allowed to change: the number of stored states and of
# transitions (they are those of the reduced graph, and the report says so),
# and which counterexample is found first. What it must not change: the status
# and the evidence of any property. A reduction is applied only where the
# search can prove the omitted interleavings are redundant:
#   - a process at a location is expanded alone only if none of its edges
#     there conflicts with an edge of any other process (a shared variable
#     cell, a channel, a program-counter read);
#   - an edge that writes something a property reads, or that carries an
#     assert, is never expanded alone (it is "visible");
#   - a state whose reduced successors close a cycle on the search stack is
#     expanded fully (the cycle proviso).
# The reduction refuses, with the reason in the report, whatever it cannot
# prove yet: rendezvous, dynamic channels, `timeout`, `provided`,
# breadth-first search, ltl/progress/ctl properties, and two shapes of process
# creation that the Promela frontend never emits (a dynamic process that
# re-enters its dormant location without leaving the process table, a `run`
# whose entry is the dormant location). Atomic sequences, `run` and the process
# table (`_nr_pr`, `pid`) are reduced since step 6: the unit that is commuted is
# the macro-step, an atomic sequence being one, and the table is one cell that
# every `run` and every end of a process writes; see steps/perf6-plan.md.
#
# Without --por the report has no `reduction` object and nothing changes.

Feature: G7 partial-order reduction — fewer states, the same verdicts

  Scenario: independent processes are explored one after the other
    Given the model under reduction "testdata/promela/bench-indep.pml"
    When I check it with "-D N=4 -D K=4 --sweep --por --no-timing"
    And I check it without reduction with "-D N=4 -D K=4 --sweep --no-timing"
    Then the check exits with 0
    And the reduction was applied
    And the reduction reduced at least one state
    And the reduced run stores at most 60 states
    And the baseline run stores 41371 states
    And every property has the same status and evidence in both runs
    And property "deadlock" is "verified" in the reduced run

  Scenario: processes that share a variable keep the interleavings that matter
    Given the model under reduction "testdata/promela/por-shared.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And property "assert" is "violated" in the reduced run
    And the reduced run has a counterexample for "assert"
    And every property has the same status and evidence in both runs
    And the reduced run stores fewer states than the baseline run

  Scenario: a deadlock survives the reduction of the processes around it
    Given the model under reduction "testdata/promela/por-deadlock.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And property "deadlock" is "violated" in the reduced run
    And every property has the same status and evidence in both runs
    And the reduced run stores fewer states than the baseline run

  # por-visible.json: A writes x = 1 then x = 2 while B and C count privately;
  # the invariant x != 2 is violated by A's second write, and the reach
  # property x == 1 holds. Both read x, so A's writes are visible and are never
  # postponed behind the others' steps.
  Scenario: a write that a property reads is never reduced away
    Given the model under reduction "testdata/ir/por-visible.json"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And property "inv" is "violated" in the reduced run
    And property "can1" is "verified" in the reduced run
    And every property has the same status and evidence in both runs
    And the reduced run stores fewer states than the baseline run

  # Found by cross-review of the first version. P at location 0 has a free
  # edge and an edge guarded by pc(Q) == 1; Q's one step enters 1 and enables
  # the guarded edge, behind which P waits for a variable nobody writes.
  # The two edges are alternatives of one process, so expanding the free one
  # alone loses the deadlock: the first version answered `verified`.
  Scenario: a step another process enables keeps its alternative in the reduced search
    Given the model under reduction "testdata/ir/por-enabling.json"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And property "deadlock" is "violated" in the reduced run
    And property "deadlock" is "violated" in the baseline run
    And every property has the same status and evidence in both runs

  # The same trap one step removed: a d_step goes on into the first enabled
  # of two edges, one guarded by pc(Q) == 1, so what the step does depends on
  # whether Q has entered 1.
  Scenario: a d_step whose continuation reads a program counter is not expanded alone
    Given the model under reduction "testdata/ir/por-dstep.json"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then property "deadlock" is "violated" in the reduced run
    And every property has the same status and evidence in both runs

  # Under a depth budget the two searches reach different states within it, so
  # either can decide what the other leaves inconclusive. P counts to 20 alone
  # and Q writes x and asserts it is still 0. The full search tries Q's step
  # from the initial state (depth 2); the reduced search runs P to its end
  # first, so Q's step is at depth 21, past a budget of 5, and the property is
  # left inconclusive. The next scenario is the other direction. Neither ever
  # contradicts a search that completes: a truncated search is never complete,
  # so it never says `verified`.
  Scenario: with a small depth budget the reduced run can answer inconclusive where the full run finds the violation
    Given the model under reduction "testdata/promela/por-depth.pml"
    When I check it with "--sweep --por --budget-depth 5 --no-timing"
    And I check it without reduction with "--sweep --budget-depth 5 --no-timing"
    Then property "assert" is "violated" in the baseline run
    And property "assert" is "inconclusive" in the reduced run

  # The other direction (a model found by the second cross-review): within a
  # budget of 7 the full search stores states it cannot expand and leaves every
  # property inconclusive, while the reduced search, whose paths are shorter,
  # finishes and decides. Its verdicts are those of the unbounded full search.
  Scenario: with a small depth budget the reduced run can finish where the full run cannot
    Given the model under reduction "testdata/ir/por-depth-reverse.json"
    When I check it with "--sweep --por --budget-depth 7 --no-timing"
    And I check it without reduction with "--sweep --budget-depth 7 --no-timing"
    Then property "deadlock" is "inconclusive" in the baseline run
    And property "deadlock" is "verified" in the reduced run

  # --- channel ends (performance plan, step 4) ------------------------------------
  # On a buffered channel a send and a receive are independent: when both are
  # enabled they commute and neither disables the other. So the send end and
  # the receive end are separate: two sends (or two receives) still depend on
  # each other, and so does anything that uses the whole channel (its length, a
  # clear), but the stages of a pipeline need not be explored in every
  # interleaving. What the other end can still do to the process that is
  # expanded alone is enable one of its alternatives: the receiver's pop frees a
  # full channel, the sender's push fills an empty one. A process is expanded
  # alone only in a state where no send of its location is blocked by a full
  # channel and no receive by an empty one.

  Scenario: a pipeline over buffered channels is explored stage after stage
    Given the model under reduction "testdata/promela/por-pipeline.pml"
    When I check it with "-D K=3 --sweep --por --no-timing"
    And I check it without reduction with "-D K=3 --sweep --no-timing"
    Then the reduction was applied
    And the reduced run stores at most 45 states
    And the baseline run stores 579 states
    And every property has the same status and evidence in both runs
    And property "assert" is "verified" in the reduced run

  Scenario: a send that only the receiver's pop enables keeps its alternative
    Given the model under reduction "testdata/promela/por-fullchan.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And property "deadlock" is "violated" in the reduced run
    And every property has the same status and evidence in both runs

  Scenario: a receive that only the sender's push enables keeps its alternative
    Given the model under reduction "testdata/promela/por-emptychan.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And property "deadlock" is "violated" in the reduced run
    And every property has the same status and evidence in both runs

  Scenario: two senders on one channel keep the order that matters
    Given the model under reduction "testdata/promela/por-order.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then property "assert" is "violated" in the reduced run
    And every property has the same status and evidence in both runs

  # --- atomic sequences (performance plan, step 6) --------------------------------
  # What a process does inside an atomic sequence is one step of the search (the
  # states inside it are not stored), so the reduction commutes the whole
  # sequence: it counts every edge the sequence can go on into, reads a program
  # counter there whole, treats a channel operation there as a use of the whole
  # channel, and follows each sequence to the states it ends in when it looks for
  # a cycle on the search stack.

  Scenario: a lock taken in an atomic step is explored far less often
    Given the model under reduction "testdata/promela/bench-sym.pml"
    When I check it with "-D N=3 --sweep --por --no-timing"
    And I check it without reduction with "-D N=3 --sweep --no-timing"
    Then the reduction was applied
    And the reduction reduced at least one state
    And the baseline run stores 1348 states
    And the reduced run stores at most 400 states
    And every property has the same status and evidence in both runs
    And property "deadlock" is "verified" in the reduced run

  # por-atomic-guard.pml: P's atomic sequence goes on into a branch guarded by x,
  # which Q writes; the branch for x == 1 blocks for ever. The deadlock exists
  # only if Q writes first, so P is not expanded alone at the start.
  Scenario: an atomic sequence whose continuation reads a variable keeps the order that matters
    Given the model under reduction "testdata/promela/por-atomic-guard.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And property "deadlock" is "violated" in the baseline run
    And property "deadlock" is "violated" in the reduced run
    And the reduced run has a counterexample for "deadlock"
    And every property has the same status and evidence in both runs

  # por-atomic-blocked.pml: two processes block inside their atomic sequences.
  # The state stored after the first process has blocked has the exclusive byte
  # set, and the two stuck states differ only in that byte; the reduction keeps
  # one of them and the deadlock.
  Scenario: processes blocked inside atomic sequences are one state up to the exclusive byte
    Given the model under reduction "testdata/promela/por-atomic-blocked.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And the baseline run stores 5 states
    And the reduced run stores at most 3 states
    And property "deadlock" is "violated" in the reduced run
    And every property has the same status and evidence in both runs

  # --- process creation and the process table (performance plan, step 6) -----------
  # leader3.pml is CH12/leader for three nodes, started by
  # `init { atomic { run ...; run ...; run ... } }` on named buffered channels:
  # the shape the channel ends of step 4 reduce, which `run` and the atomic
  # block blocked. The table (`_nr_pr`, `pid`, who is the youngest process) is
  # one cell that every `run` and every end of a process writes, so the creating
  # step itself is expanded in full and the gain comes from the nodes.

  Scenario: a ring of processes started by init is explored far less often
    Given the model under reduction "testdata/promela/leader3.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And the reduction reduced at least one state
    And the baseline run stores 679 states
    And the reduced run stores at most 100 states
    And every property has the same status and evidence in both runs
    And property "deadlock" is "verified" in the reduced run

  # leader5.pml is the same ring unrolled to five nodes (41692 states, the count
  # of CH9/leader.pml, which cannot be reduced because its channels are `chan`
  # parameters). The reduced graph grows linearly with the number of nodes.
  Scenario: a ring of five nodes shrinks from tens of thousands of states to a few hundred
    Given the model under reduction "testdata/promela/leader5.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And the baseline run stores 41692 states
    And the reduced run stores at most 300 states
    And every property has the same status and evidence in both runs

  Scenario: a model that reads _nr_pr is reduced and keeps its verdicts
    Given the model under reduction "testdata/promela/nrpr.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And the baseline run stores 31 states
    And the reduced run stores at most 25 states
    And every property has the same status and evidence in both runs
    And property "assert" is "verified" in the reduced run

  # por-run-pool.pml: `run` in a loop. The engine pre-instantiates a bounded
  # pool of the process; the run that finds the pool empty stops the search,
  # which answers inconclusive, with or without the reduction.
  Scenario: a run that exhausts its pool stops both searches the same way
    Given the model under reduction "testdata/promela/por-run-pool.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And property "deadlock" is "inconclusive" in the baseline run
    And property "deadlock" is "inconclusive" in the reduced run
    And every property has the same status and evidence in both runs

  # por-dormant.json: a dynamic process whose end goes back to its dormant
  # location without leaving the table. The frontend never emits it; with it a
  # pool slot becoming free would not be ordered against the run.
  Scenario: a dynamic process that returns to its dormant location without leaving the table is not reduced
    Given the model under reduction "testdata/ir/por-dormant.json"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was not applied and its reason mentions "dormant"
    And the reduced run stores as many states as the baseline run
    And every property has the same status and evidence in both runs

  # A handshake is one step of two processes: not reduced in this version
  # (steps/perf6-plan.md section 6.1).
  Scenario: a rendezvous channel is not reduced, and says why
    Given the model under reduction "testdata/promela/por-rendezvous.pml"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was not applied and its reason mentions "rendezvous"
    And the reduced run stores as many states as the baseline run
    And every property has the same status and evidence in both runs

  # The reasons stay accurate where the other branches changed the semantics
  # the analysis reads: `provided` now gates both sides of a rendezvous, and
  # `timeout` is a read of "can anything else move". The refusal says which, and
  # the run is the full search, with the same verdicts.
  Scenario Outline: a process with a provided clause or a timeout is not reduced, and says why
    Given the model under reduction "testdata/promela/<file>"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was not applied and its reason mentions "<reason>"
    And the reduced run stores as many states as the baseline run
    And every property has the same status and evidence in both runs

    Examples:
      | file                  | reason   |
      | provided-rv-send.pml  | provided |
      | provided-rv-recv.pml  | provided |
      | timeout-gate.pml      | timeout  |

  # The loop at the start of an atomic or d_step block keeps the exclusive control
  # across its back edge (the frontend, weak-fairness branch): the iterations are
  # one macro-step for the reduction, whose closure follows an atomic edge back to
  # the head of the loop.
  Scenario Outline: a loop at the start of an atomic or d_step block is reduced and answers as the full search
    Given the model under reduction "testdata/promela/<file>"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the reduction was applied
    And the baseline run stores 7 states
    And every property has the same status and evidence in both runs
    And property "assert" is "verified" in the reduced run

    Examples:
      | file                   |
      | atomic-loop-entry.pml  |
      | dstep-loop-entry.pml   |

  Scenario: breadth-first search is not reduced, and says why
    Given the model under reduction "testdata/promela/bench-indep.pml"
    When I check it with "-D N=3 -D K=2 --bfs --por --sweep --no-timing"
    And I check it without reduction with "-D N=3 -D K=2 --bfs --sweep --no-timing"
    Then the reduction was not applied and its reason mentions "breadth-first"
    And the reduced run stores as many states as the baseline run

  Scenario: a temporal property is not reduced, and says why
    Given the model under reduction "testdata/promela/bench-indep.pml"
    When I check it with "-D N=3 -D K=2 --ltl []<>true --por --sweep --no-timing"
    Then the reduction was not applied and its reason mentions "temporal"

  # A process that reads `_nr_pr` makes the model keep the process table. Since
  # step 6 the reduction models the table (cell T), so such a model is reduced
  # where the rules allow it and answers as the full search does; the models
  # that the `_nr_pr` fix lowers with end edges that leave the table (`active`
  # processes, no `run`) are the shape this scenario pins. A property that reads
  # `_nr_pr` over a model whose processes keep no table is refused by itself
  # (`not-executed`, see g5-ctl-v1.feature): it is not evaluated and reads
  # nothing, so it is no reason to refuse the reduction.
  Scenario Outline: a model whose process reads _nr_pr without run is reduced and answers as the full search
    Given the model under reduction "testdata/promela/<file>"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the check exits with 0
    And the reduction was applied
    And every property has the same status and evidence in both runs

    Examples:
      | file               |
      | nrpr-active.pml    |
      | nrpr-order.pml     |
      | nrpr-youngest.pml  |

  Scenario: a property that was refused for the process table does not refuse the reduction
    Given the model under reduction "testdata/ir/nrpr-property.json"
    When I check it with "--sweep --por --no-timing"
    And I check it without reduction with "--sweep --no-timing"
    Then the check exits with 0
    And the reduction was applied
    And property "none" is "not-executed" in the reduced run
    And property "sane" is "verified" in the reduced run
    And every property has the same status and evidence in both runs

  Scenario: without --por the report carries no reduction object
    Given the model under reduction "testdata/promela/bench-indep.pml"
    When I check it with "-D N=3 -D K=2 --sweep --no-timing"
    Then the check exits with 0
    And the reduction object is absent
