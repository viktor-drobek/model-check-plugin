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
# The first version refuses, with the reason in the report, whatever it cannot
# prove yet: atomic sequences, rendezvous, dynamic channels, `run`, a read of the
# process table (`_nr_pr`), `timeout`, `provided`, breadth-first search,
# ltl/progress/ctl properties.
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

  Scenario: a model with atomic sequences is not reduced, and says why
    Given the model under reduction "testdata/promela/bench-sym.pml"
    When I check it with "-D N=3 --sweep --por --no-timing"
    And I check it without reduction with "-D N=3 --sweep --no-timing"
    Then the reduction was not applied and its reason mentions "atomic"
    And the reduced run stores as many states as the baseline run
    And every property has the same status and evidence in both runs

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

  Scenario: without --por the report carries no reduction object
    Given the model under reduction "testdata/promela/bench-indep.pml"
    When I check it with "-D N=3 -D K=2 --sweep --no-timing"
    Then the check exits with 0
    And the reduction object is absent
