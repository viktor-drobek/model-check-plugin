# G8 (performance plan, step 5): parallel exploration of the safety search,
# opt-in with `mcd check --workers N`. Every scenario observes the engine
# through the CLI and its JSON report (the report has no atomic-step count, so
# that one is checked by the package tests of explore).
#
# The parallel search is a level-synchronous breadth-first search over a
# partitioned visited set. What it must keep, for a run that completes:
#   - the verdict and the evidence of every property;
#   - `states` and `transitions` of the sequential run (and its atomic-step
#     count, in the package tests);
# and for every run:
#   - the report is the same for any number of workers (only the three
#     worker-count fields of search.parallel may differ), for a repeated run,
#     and under a budget;
#   - a counterexample is a shortest one (in layers) and replays as a run.
# What it changes, and says so in the report: `search.mode` is "bfs" and `depth`
# is the number of breadth-first layers (`--bfs`'s depth on a model without
# atomic sequences, and it can differ on one with them): a layer counts hops
# between stored states, so an atomic sequence that runs through is one unit of
# `depth` and of `--budget-depth`, and one that blocks part-way counts one unit
# per uninterrupted run (the state where its holder blocks is stored), where
# `--bfs` counts every step of it (a d_step block is one move in every search, so
# there it makes no difference); a counterexample is generally not the
# depth-first one; counts of a run that stops early are those of a deterministic
# group boundary. That is what the parallel search does WHEN IT IS APPLIED: a
# run that is refused is the sequential one, and counts transitions.
#
# It refuses, with the reason in `search.parallel`, whatever it cannot yet
# prove: any ltl, progress or ctl property in the run, and `--por` where the
# reduction applies. A refused run is the sequential run, unchanged.
#
# Without --workers the report has no `parallel` object and nothing changes.
#
# The scenarios run in the default suite (the @pending tags of the days before
# the CLI flag existed are gone); the package tests of explore are the gate of
# what the report cannot show.

Feature: G8 parallel exploration - the same answers, found by several workers

  Scenario: a complete run has the sequential run's verdicts and counts
    Given the model under parallel test "testdata/promela/bench-indep.pml"
    When I check it sequentially with "-D N=4 -D K=4 --sweep --no-timing"
    And I check it in parallel with 4 workers and "-D N=4 -D K=4 --sweep --no-timing"
    And I check it sequentially with "-D N=4 -D K=4 --sweep --bfs --no-timing" as the breadth-first run
    Then the parallel search was applied with 4 workers
    And the parallel run stores 41371 states
    And every property has the same status and evidence in the parallel and the sequential run
    And both runs have the same states and transitions
    And the parallel run reports the search mode "bfs"
    And the parallel depth equals the breadth-first depth

  Scenario: a narrow model, one state per layer, completes with equal counts
    Given the model under parallel test "testdata/promela/par-chain.pml"
    When I check it sequentially with "-D N=300 --sweep --no-timing"
    And I check it in parallel with 4 workers and "-D N=300 --sweep --no-timing"
    Then the parallel search was applied with 4 workers
    And every property has the same status and evidence in the parallel and the sequential run
    And both runs have the same states and transitions
    And the parallel run has as many layers as states

  Scenario: the report is the same for 1, 2 and 7 workers and for a repeated run
    Given the model under parallel test "testdata/promela/bench-indep.pml"
    When I check it in parallel with 1 workers and "-D N=3 -D K=3 --sweep --no-timing"
    And I check it in parallel with 2 workers and "-D N=3 -D K=3 --sweep --no-timing" as the second parallel run
    And I check it in parallel with 7 workers and "-D N=3 -D K=3 --sweep --no-timing" as the third parallel run
    And I check it in parallel with 7 workers and "-D N=3 -D K=3 --sweep --no-timing" as the repeated run
    Then the reports of all parallel runs are equal except for the worker-count fields
    And the reports of the third parallel run and the repeated run are byte-identical

  # Counterexamples are shortest (in layers) and deterministic. por-shared.pml:
  # two processes race on a counter; the assert is violated.
  Scenario: a violated assert has a shortest counterexample that replays
    Given the model under parallel test "testdata/promela/por-shared.pml"
    When I check it in parallel with 1 workers and "--sweep --no-timing"
    And I check it in parallel with 8 workers and "--sweep --no-timing" as the second parallel run
    And I check it sequentially with "--sweep --bfs --no-timing" as the breadth-first run
    Then property "assert" is "violated" in the parallel run
    And the counterexample of "assert" in the parallel run is as long as the breadth-first one
    And the counterexample of "assert" in the parallel run replays as a run of the model
    And the reports of all parallel runs are equal except for the worker-count fields

  Scenario: a deadlock is found and its counterexample does not depend on the workers
    Given the model under parallel test "testdata/promela/por-deadlock.pml"
    When I check it in parallel with 1 workers and "--sweep --no-timing"
    And I check it in parallel with 5 workers and "--sweep --no-timing" as the second parallel run
    And I check it sequentially with "--sweep --no-timing"
    Then property "deadlock" is "violated" in the parallel run
    And every property has the same status and evidence in the parallel and the sequential run
    And the reports of all parallel runs are equal except for the worker-count fields

  Scenario: an invariant violated and a reach witness
    Given the model under parallel test "testdata/ir/por-visible.json"
    When I check it in parallel with 3 workers and "--sweep --no-timing"
    And I check it sequentially with "--sweep --no-timing"
    Then property "inv" is "violated" in the parallel run
    And property "can1" is "verified" in the parallel run
    And every property has the same status and evidence in the parallel and the sequential run

  # The checks of the initial state belong to no expansion; par-initial.json
  # has an invariant that is false and a reach condition that is true in it.
  Scenario: the initial state itself violates the invariant
    Given the model under parallel test "testdata/ir/par-initial.json"
    When I check it in parallel with 1 workers and "--no-timing"
    And I check it in parallel with 8 workers and "--no-timing" as the second parallel run
    Then property "inv" is "violated" in the parallel run
    And the counterexample of "inv" in the parallel run has 0 steps
    And property "can" is "verified" in the parallel run
    And the witness of "can" in the parallel run has 0 steps
    And the reports of all parallel runs are equal except for the worker-count fields

  Scenario: an evaluation error on the initial state is an invalid model at once
    Given the model under parallel test "testdata/ir/par-initial-error.json"
    When I check it in parallel with 4 workers and "--no-timing"
    Then property "inv" is "invalid-model" in the parallel run
    And the counterexample of "inv" in the parallel run has 0 steps
    And the parallel run stores 1 states

  # dstep-block.pml: a d_step blocks (a model error). The trace ends at the
  # offending step and properties decided earlier keep their verdict.
  Scenario: an invalid model reports the run to the offending step
    Given the model under parallel test "testdata/promela/dstep-block.pml"
    When I check it in parallel with 1 workers and "--sweep --no-timing"
    And I check it in parallel with 8 workers and "--sweep --no-timing" as the second parallel run
    And I check it sequentially with "--sweep --bfs --no-timing" as the breadth-first run
    Then property "deadlock" is "invalid-model" in the parallel run
    And the counterexample of "deadlock" in the parallel run is as long as the breadth-first one
    And the reports of all parallel runs are equal except for the worker-count fields

  # par-order.pml (plan 3.7): an assert violated on one branch, a domain
  # overflow on the other. The depth-first and the breadth-first search already
  # answer differently; the parallel search promises the legal outcomes and the
  # same report for any number of workers, not a particular winner.
  Scenario: an ending event and a violation in different branches
    Given the model under parallel test "testdata/promela/par-order.pml"
    When I check it in parallel with 1 workers and "--sweep --no-timing"
    And I check it in parallel with 8 workers and "--sweep --no-timing" as the second parallel run
    And I check it in parallel with 8 workers and "--sweep --no-timing" as the repeated run
    Then no property of the parallel run is "verified"
    And property "assert" in the parallel run is "violated" or "invalid-model" with a counterexample that replays
    And the reports of all parallel runs are equal except for the worker-count fields
    And the reports of the second parallel run and the repeated run are byte-identical

  # A decided property is never evaluated again: the invariant a[i] == 0 is
  # false in layer 1; in layer 2 the index is out of range, which would be an
  # error if the expression were evaluated.
  Scenario: an error in the expression of a decided property is never raised
    Given the model under parallel test "testdata/ir/par-decided.json"
    When I check it in parallel with 4 workers and "--no-timing"
    And I check it sequentially with "--no-timing"
    Then property "inv" is "violated" in the parallel run
    And property "deadlock" is "verified" in the parallel run
    And the parallel run is complete
    And every property has the same status and evidence in the parallel and the sequential run

  # The checks of one state go on past the failure of a decided property. In
  # par-skip-reach.json and par-skip-inv.json the invariant a[i] == 0 is violated
  # by the first successor of the initial state, and the second successor has i
  # equal to 5, where evaluating the invariant fails. The invariant is decided,
  # so the sequential search skips it on that state and goes on to the next
  # property (reach i == 5, or the invariant i != 5), which that state alone
  # decides. A parallel search that evaluated the invariant anyway, and gave up
  # checking the state when that failed, would report the reach condition as
  # never satisfied and the second invariant as verified, both exhaustive.
  Scenario Outline: a failing check of a decided property does not hide the other checks of the state
    Given the model under parallel test "<model>"
    When I check it sequentially with "--bfs <sweep> --no-timing"
    And I check it in parallel with 1 workers and "<sweep> --no-timing"
    And I check it in parallel with 2 workers and "<sweep> --no-timing" as the second parallel run
    And I check it in parallel with 4 workers and "<sweep> --no-timing" as the third parallel run
    Then property "inv" is "violated" in the parallel run
    And property "<other>" is "<status>" in the parallel run
    And the <evidence> of "<other>" in the parallel run has 1 steps
    And every property has the same status and evidence in the parallel and the sequential run
    And the parallel run stops with the reason of the sequential run
    And the reports of all parallel runs are equal except for the worker-count fields

    Examples:
      | model                           | sweep   | other  | status   | evidence       |
      | testdata/ir/par-skip-reach.json |         | reach5 | verified | witness        |
      | testdata/ir/par-skip-reach.json | --sweep | reach5 | verified | witness        |
      | testdata/ir/par-skip-inv.json   |         | inv5   | violated | counterexample |
      | testdata/ir/par-skip-inv.json   | --sweep | inv5   | violated | counterexample |

  # One edge with a false assert and an effect that overflows its variable: the
  # error of the step comes first, the assert is not reported.
  Scenario: an error of a step suppresses the failed assert of the same step
    Given the model under parallel test "testdata/ir/par-assert-overflow.json"
    When I check it in parallel with 4 workers and "--sweep --no-timing"
    Then property "assert" is "invalid-model" in the parallel run
    And the counterexample of "assert" in the parallel run has 1 steps

  # Atomic sequences: the intermediate states of an atomic sequence are
  # expanded and not stored; stored states, transitions and atomic steps are
  # those of the sequential run.
  Scenario: atomic sequences give the sequential counts
    Given the model under parallel test "testdata/promela/bench-sym.pml"
    When I check it sequentially with "-D N=3 --sweep --no-timing"
    And I check it in parallel with 4 workers and "-D N=3 --sweep --no-timing"
    Then the parallel search was applied with 4 workers
    And every property has the same status and evidence in the parallel and the sequential run
    And both runs have the same states and transitions

  Scenario: a model that is mostly atomic
    Given the model under parallel test "testdata/promela/atomic-t3.pml"
    When I check it sequentially with "--sweep --no-timing"
    And I check it in parallel with 2 workers and "--sweep --no-timing"
    Then every property has the same status and evidence in the parallel and the sequential run
    And both runs have the same states and transitions

  # Rendezvous channels and `run`: CH15/client_server.pml starts processes and
  # hands messages over synchronously.
  Scenario: rendezvous and run give the sequential counts
    Given the model under parallel test "../../Promela - examples/CH15/client_server.pml"
    When I check it sequentially with "--sweep --no-timing"
    And I check it in parallel with 4 workers and "--sweep --no-timing"
    Then every property has the same status and evidence in the parallel and the sequential run
    And both runs have the same states and transitions

  # A `run` that finds no dormant instance left in its pool is a bound of the
  # engine: the properties are inconclusive, never a verdict.
  Scenario: an exhausted process pool is the sequential bound
    Given the model under parallel test "../../Promela - examples/CH15/client_server.pml"
    When I check it sequentially with "--max-procs 1 --sweep --bfs --no-timing" as the breadth-first run
    And I check it in parallel with 3 workers and "--max-procs 1 --sweep --no-timing"
    Then the parallel run stops with a reason starting "process budget exhausted"
    And the parallel run is not complete
    And every property of the parallel run is "inconclusive"

  # An atomic loop that never blocks ends in the declared bound, with a
  # verdict-free answer, and does not hang.
  Scenario: an atomic loop that never blocks ends at the bound
    Given the model under parallel test "testdata/ir/par-atomic-loop.json"
    When I check it in parallel with 1 workers and "--no-timing"
    And I check it in parallel with 8 workers and "--no-timing" as the second parallel run
    Then the parallel run stops with a reason starting "depth budget exhausted: an atomic sequence exceeds 100000 steps"
    And every property of the parallel run is "inconclusive"
    And the reports of all parallel runs are equal except for the worker-count fields

  # A never claim that is not asked as a property is stored in the state
  # vector and never executed by the safety search.
  Scenario: a never claim that is not a property of the run is only stored
    Given the model under parallel test "testdata/ir/par-claim.json"
    When I check it sequentially with "--sweep --no-timing"
    And I check it in parallel with 4 workers and "--sweep --no-timing"
    Then every property has the same status and evidence in the parallel and the sequential run
    And both runs have the same states and transitions

  Scenario: the vacuity watch records the same coverage
    Given the model under parallel test "testdata/promela/leader3.pml"
    When I check it sequentially with "--sweep --no-timing"
    And I check it in parallel with 4 workers and "--sweep --no-timing"
    Then every property has the same status and evidence in the parallel and the sequential run
    And both runs have the same states and transitions
    And the warnings of both runs are equal

  # --sweep: after a violation the whole graph is still searched, and the
  # counters are those of the sequential sweep.
  Scenario: the sweep after a violation counts the whole graph
    Given the model under parallel test "testdata/promela/por-shared.pml"
    When I check it sequentially with "--sweep --no-timing"
    And I check it in parallel with 3 workers and "--sweep --no-timing"
    Then property "assert" is "violated" in the parallel run
    And both runs have the same states and transitions

  # Budgets in parallel. The state budget is exact; the report is the same for
  # any number of workers, because the groups, and so every stop point, are a
  # function of counts and not of the workers.
  Scenario: a state budget stores exactly the budget and says so
    Given the model under parallel test "testdata/promela/bench-indep.pml"
    When I check it in parallel with 1 workers and "-D N=4 -D K=4 --budget-states 5000 --no-timing"
    And I check it in parallel with 8 workers and "-D N=4 -D K=4 --budget-states 5000 --no-timing" as the second parallel run
    Then the parallel run stores 5000 states
    And the parallel run stops with a reason starting "state budget exhausted: 5000 states stored"
    And every property of the parallel run is "inconclusive"
    And every property of the parallel run has evidence "bounded"
    And the reports of all parallel runs are equal except for the worker-count fields

  # A state budget that falls between a violation and a later event of the same
  # group: only the events before the stop point count.
  Scenario: a violation found before the state budget stands and the budget is exact
    Given the model under parallel test "testdata/promela/por-shared.pml"
    When I check it in parallel with 4 workers and "--budget-states 30 --no-timing"
    And I check it in parallel with 1 workers and "--budget-states 30 --no-timing" as the second parallel run
    Then the parallel run stores at most 30 states
    And the reports of all parallel runs are equal except for the worker-count fields
    And no property of the parallel run is "verified"

  # (bench-indep.pml has no atomic sequence, so the parallel depth is --bfs's.)
  Scenario: a depth budget stops at the layer and says how many states were stored but not expanded
    Given the model under parallel test "testdata/promela/bench-indep.pml"
    When I check it sequentially with "-D N=3 -D K=3 --budget-depth 7 --bfs --no-timing" as the breadth-first run
    And I check it in parallel with 1 workers and "-D N=3 -D K=3 --budget-depth 7 --no-timing"
    And I check it in parallel with 8 workers and "-D N=3 -D K=3 --budget-depth 7 --no-timing" as the second parallel run
    Then the parallel run stops with the reason of the breadth-first run
    And the parallel depth equals the breadth-first depth
    And every property of the parallel run is "inconclusive"
    And the reports of all parallel runs are equal except for the worker-count fields

  # Depth counts hops between stored states, so an atomic sequence that runs
  # through is ONE unit of `depth` and of --budget-depth in the parallel search,
  # where --bfs counts each of its steps. par-atomic-depth.pml: an atomic
  # sequence of two steps, then a failing assert. With --budget-depth 1 --bfs
  # stores the state after the sequence at depth 2 and does not expand it (the
  # assert stays undecided); the parallel search has it in layer 1, expands it,
  # and finds the failing assert. Neither is unsound: both run the model, and the
  # assert does fail.
  Scenario Outline: an atomic sequence that runs through is one unit of depth and of the depth budget
    Given the model under parallel test "testdata/promela/par-atomic-depth.pml"
    When I check it sequentially with "--bfs --budget-depth <budget> --no-timing" as the breadth-first run
    And I check it in parallel with 2 workers and "--budget-depth <budget> --no-timing"
    And I check it in parallel with 7 workers and "--budget-depth <budget> --no-timing" as the second parallel run
    Then property "assert" is "<bfs status>" in the breadth-first run
    And property "assert" is "violated" in the parallel run
    And the breadth-first run has depth <bfs depth>
    And the parallel run has depth <par depth>
    And the reports of all parallel runs are equal except for the worker-count fields

    Examples:
      | budget | bfs status   | bfs depth | par depth |
      | 1      | inconclusive | 0         | 1         |
      | 2      | violated     | 2         | 2         |

  # An atomic sequence that BLOCKS part-way is cut where it blocks: the state in
  # which its holder blocks is stored (the other processes may move there), and
  # the continuation starts with the step of whichever process unblocks it, so the
  # sequence counts one unit per uninterrupted run, not one in all. atomic-at.pml:
  # A runs `atomic { x = 1; y == 1; x = 2 }; x = 3` and B sets y. A takes x = 1 and
  # blocks on y == 1, which is one unit; B's y = 1 unblocks it, and y == 1 and
  # x = 2 follow in the same hop, which is the second. --bfs counts all four
  # steps. The rows pin, for the budgets 1 to 5, where each search cuts: depth,
  # states and transitions at the stop, and whether the run completes (the
  # parallel search does at budget 5, where its depth is 5; --bfs needs 7). This
  # outline passed on its first run, because it pins behaviour that already held;
  # it is here so that a text that says an atomic sequence is one unit, with no
  # word about the one that blocks, cannot come back unnoticed.
  Scenario Outline: an atomic sequence that blocks part-way counts one unit per uninterrupted run
    Given the model under parallel test "testdata/promela/atomic-at.pml"
    When I check it sequentially with "--bfs --budget-depth <budget> --no-timing" as the breadth-first run
    And I check it in parallel with 2 workers and "--budget-depth <budget> --no-timing"
    And I check it in parallel with 4 workers and "--budget-depth <budget> --no-timing" as the second parallel run
    Then the breadth-first run has depth <bfs depth>
    And the breadth-first run has <bfs states> states and <bfs transitions> transitions
    And the breadth-first run stops with a reason starting "depth budget exhausted"
    And property "deadlock" is "inconclusive" in the breadth-first run
    And the parallel run has depth <par depth>
    And the parallel run has <par states> states and <par transitions> transitions
    And the parallel run stops with a reason starting "<par stop>"
    And property "deadlock" is "<par deadlock>" in the parallel run
    And the reports of all parallel runs are equal except for the worker-count fields

    Examples:
      | budget | bfs depth | bfs states | bfs transitions | par depth | par states | par transitions | par stop               | par deadlock |
      | 1      | 1         | 6          | 7               | 1         | 6          | 7               | depth budget exhausted | inconclusive |
      | 2      | 2         | 8          | 13              | 2         | 9          | 15              | depth budget exhausted | inconclusive |
      | 3      | 3         | 8          | 15              | 3         | 10         | 19              | depth budget exhausted | inconclusive |
      | 4      | 4         | 9          | 17              | 4         | 11         | 20              | depth budget exhausted | inconclusive |
      | 5      | 5         | 10         | 19              | 5         | 11         | 20              | complete               | verified     |

  # A guard as the FIRST statement of an atomic block does not split it: the block
  # is entered only when the guard holds, and then runs through, so it is one unit
  # (atomic-guard-first.pml is atomic-at.pml with `y == 1` moved to the front).
  # The unbudgeted depth is the same as for atomic-at.pml (7 for --bfs, 5 for the
  # parallel search) for different reasons: there the block takes two units, here
  # it takes one and B's step in front of it takes the other. The rows are not the
  # same, and show it. At budget 3 --bfs has no state at depth 3 (its atomic
  # chain goes from depth 1 to depth 4), so its depth stays 2. Passed on its first
  # run, as the outline above.
  Scenario Outline: an atomic sequence that starts with a guard does not split and is one unit
    Given the model under parallel test "testdata/promela/atomic-guard-first.pml"
    When I check it sequentially with "--bfs --budget-depth <budget> --no-timing" as the breadth-first run
    And I check it in parallel with 2 workers and "--budget-depth <budget> --no-timing"
    And I check it in parallel with 4 workers and "--budget-depth <budget> --no-timing" as the second parallel run
    Then the breadth-first run has depth <bfs depth>
    And the breadth-first run has <bfs states> states and <bfs transitions> transitions
    And the breadth-first run stops with a reason starting "depth budget exhausted"
    And property "deadlock" is "inconclusive" in the breadth-first run
    And the parallel run has depth <par depth>
    And the parallel run has <par states> states and <par transitions> transitions
    And the parallel run stops with a reason starting "<par stop>"
    And property "deadlock" is "<par deadlock>" in the parallel run
    And the reports of all parallel runs are equal except for the worker-count fields

    Examples:
      | budget | bfs depth | bfs states | bfs transitions | par depth | par states | par transitions | par stop               | par deadlock |
      | 1      | 1         | 4          | 5               | 1         | 4          | 5               | depth budget exhausted | inconclusive |
      | 2      | 2         | 5          | 8               | 2         | 6          | 10              | depth budget exhausted | inconclusive |
      | 3      | 2         | 5          | 8               | 3         | 7          | 12              | depth budget exhausted | inconclusive |
      | 4      | 4         | 6          | 10              | 4         | 8          | 13              | depth budget exhausted | inconclusive |
      | 5      | 5         | 7          | 12              | 5         | 8          | 13              | complete               | verified     |

  # Without a budget both models are complete in every search, with the same
  # states and transitions, and the parallel depth is 5 against 7 for --bfs.
  Scenario Outline: without a budget the blocking and the non-blocking atomic block have the same states and transitions in both searches
    Given the model under parallel test "<model>"
    When I check it sequentially with "--bfs --no-timing" as the breadth-first run
    And I check it in parallel with 3 workers and "--no-timing"
    Then the breadth-first run has depth 7
    And the parallel run has depth 5
    And the breadth-first run has <states> states and <transitions> transitions
    And the parallel run has <states> states and <transitions> transitions
    And the parallel run is complete

    Examples:
      | model                                   | states | transitions |
      | testdata/promela/atomic-at.pml          | 11     | 20          |
      | testdata/promela/atomic-guard-first.pml | 8      | 13          |

  # A d_step block is NOT a unit of depth of its own, unlike an atomic sequence:
  # `fire` runs the whole chain as ONE move, so the default search, --bfs and the
  # parallel search count it as one step and cut at the same place under every
  # --budget-depth. par-dstep-depth.pml: a d_step block of two statements, then a
  # failing assert. This scenario pins behaviour that already held when it was
  # written (it passed on its first run); it is here so that a text that says the
  # d_step case differs from the sequential searches cannot come back unnoticed.
  # All three searches are pinned: the stop reason (the parallel run against each
  # of the other two), the depth (one figure for all three), `states` and
  # `transitions` (the parallel run against each of the other two, and the
  # number of states spelled out) and the status of every property (the parallel
  # run against the default run, with the evidence, and both properties spelled
  # out for the breadth-first and the parallel run).
  Scenario Outline: a d_step block is one move and one unit of depth in every search
    Given the model under parallel test "testdata/promela/par-dstep-depth.pml"
    When I check it sequentially with "--bfs --budget-depth <budget> --no-timing" as the breadth-first run
    And I check it sequentially with "--budget-depth <budget> --no-timing"
    And I check it in parallel with 2 workers and "--budget-depth <budget> --no-timing"
    Then the parallel run stops with the reason of the breadth-first run
    And the parallel run stops with the reason of the sequential run
    And the sequential run has depth <depth>
    And the breadth-first run has depth <depth>
    And the parallel run has depth <depth>
    And both runs have the same states and transitions
    And the breadth-first run and the parallel run have the same states and transitions
    And the parallel run stores <states> states
    And every property has the same status and evidence in the parallel and the sequential run
    And property "assert" is "violated" in the breadth-first run
    And property "assert" is "violated" in the parallel run
    And property "deadlock" is "<deadlock>" in the breadth-first run
    And property "deadlock" is "<deadlock>" in the parallel run

    Examples:
      | budget | depth | states | deadlock     |
      | 1      | 1     | 3      | inconclusive |
      | 2      | 2     | 4      | inconclusive |
      | 3      | 3     | 4      | verified     |

  Scenario: without a budget the parallel depth is shorter than --bfs's by the steps inside atomic sequences
    Given the model under parallel test "testdata/promela/par-atomic-depth.pml"
    When I check it sequentially with "--bfs --no-timing"
    And I check it in parallel with 3 workers and "--no-timing"
    Then the sequential run has depth 4
    And the parallel run has depth 3
    And both runs have the same states and transitions

  # The layer beyond the depth budget is stored and not expanded, but its
  # states are still checked: x != 2 is violated in layer 2.
  Scenario: a violation in the layer beyond the depth budget is still found
    Given the model under parallel test "testdata/ir/por-visible.json"
    When I check it in parallel with 2 workers and "--budget-depth 1 --no-timing"
    Then property "inv" is "violated" in the parallel run
    And the parallel run is not complete

  Scenario: a memory budget stops the run on the estimate and never verifies
    Given the model under parallel test "testdata/ir/counters-10-5.json"
    When I check it in parallel with 1 workers and "--budget-mem-mb 1 --no-timing"
    And I check it in parallel with 6 workers and "--budget-mem-mb 1 --no-timing" as the second parallel run
    Then the parallel run stops with a reason starting "memory budget exhausted"
    And no property of the parallel run is "verified"
    And the reports of all parallel runs are equal except for the worker-count fields

  # A model whose states have an abrupt fan-out under a small memory budget: a
  # group whose records would not fit is cut, and a report comes back.
  Scenario: an abrupt fan-out under a small memory budget still returns a report
    Given the model under parallel test "testdata/ir/par-fanout.json"
    When I check it in parallel with 1 workers and "--budget-mem-mb 6 --no-timing"
    And I check it in parallel with 8 workers and "--budget-mem-mb 6 --no-timing" as the second parallel run
    Then the check exits with 0 in the parallel run
    And no property of the parallel run is "verified" unless the parallel run is complete
    And the reports of all parallel runs are equal except for the worker-count fields

  # Every transition fails an assert and every state has a high fan-out, under
  # a small memory budget: the events retained are bounded.
  Scenario: a model that fails an assert on every transition under a small memory budget
    Given the model under parallel test "testdata/ir/par-fanout-assert.json"
    When I check it in parallel with 1 workers and "--budget-mem-mb 4 --no-timing"
    And I check it in parallel with 8 workers and "--budget-mem-mb 4 --no-timing" as the second parallel run
    Then the check exits with 0 in the parallel run
    And property "assert" is "violated" in the parallel run
    And the reports of all parallel runs are equal except for the worker-count fields

  # Default behaviour: without --workers the report has no parallel object, the
  # mode is dfs, and the bytes are those of the engine before the option existed
  # (testdata/golden/par-default-bench-indep-3.report.json was recorded by the
  # unchanged engine).
  Scenario: without --workers nothing changes
    Given the model under parallel test "testdata/promela/bench-indep.pml"
    When I check it sequentially with "-D N=3 -D K=3 --sweep --no-timing"
    Then the report of the sequential run has no parallel object
    And the sequential run reports the search mode "dfs"
    And the report of the sequential run equals the recorded report "testdata/golden/par-default-bench-indep-3.report.json"

  # A parallel run is a breadth-first run: the mode says so, with --bfs or not.
  Scenario: a parallel run reports the breadth-first mode
    Given the model under parallel test "testdata/promela/bench-indep.pml"
    When I check it in parallel with 2 workers and "-D N=3 -D K=3 --no-timing"
    Then the parallel run reports the search mode "bfs"
    And the parallel search was applied with 2 workers

  Scenario: --bfs with --workers is the parallel search
    Given the model under parallel test "testdata/promela/bench-indep.pml"
    When I check it in parallel with 2 workers and "-D N=3 -D K=3 --bfs --no-timing"
    Then the parallel run reports the search mode "bfs"
    And the parallel search was applied with 2 workers

  # A temporal property is decided by a nested depth-first search that this
  # version does not parallelise: the run is the sequential one, with the reason.
  Scenario: an ltl property in the run refuses the parallel search
    Given the model under parallel test "testdata/promela/por-shared.pml"
    When I check it sequentially with "--sweep --no-timing --ltl '[] (x <= 2)'"
    And I check it in parallel with 4 workers and "--sweep --no-timing --ltl '[] (x <= 2)'"
    Then the parallel search was refused with a reason mentioning "ltl, progress or ctl"
    And the report of the parallel run equals the sequential report except for the parallel object
    And the parallel run reports the search mode "dfs"

  # Weak fairness belongs to the cycle search (the n + 2 copies, cycle.go), which
  # this version does not parallelise: --fairness weak never makes a run parallel,
  # and the run is the sequential weak-fairness run, report and verdicts included
  # (integration of the weak-fairness and parallel branches).
  Scenario Outline: --fairness weak with --workers is the sequential weak-fairness run, with the refusal
    Given the model under parallel test "testdata/promela/<file>"
    When I check it sequentially with "--sweep --no-timing --fairness weak <extra>"
    And I check it in parallel with 4 workers and "--sweep --no-timing --fairness weak <extra>"
    Then the parallel search was refused with a reason mentioning "ltl, progress or ctl"
    And the report of the parallel run equals the sequential report except for the parallel object
    And the parallel run reports the search mode "dfs"

    Examples:
      | file                         | extra                |
      | claim-atomic-timeout.pml     |                      |
      | claim-atomic-starve.pml      |                      |
      | weakfair-timeout-claim.pml   |                      |
      | por-shared.pml               | --ltl '[] (x <= 2)'  |
      | por-shared.pml               | --progress           |

  # The semantics the other branches fixed, under the parallel search: a handshake
  # needs both processes' `provided` clauses to hold, and `timeout` is true only
  # when nothing else can move. The parallel run and the breadth-first run, which
  # share the stepper, must agree on every status, state and transition.
  Scenario Outline: a model with provided and a rendezvous, or with timeout, has the breadth-first run's answers in parallel
    Given the model under parallel test "testdata/promela/<file>"
    When I check it sequentially with "--sweep --bfs --no-timing"
    And I check it in parallel with 4 workers and "--sweep --no-timing"
    Then the parallel search was applied with 4 workers
    And every property has the same status and evidence in the parallel and the sequential run
    And both runs have the same states and transitions

    Examples:
      | file                  |
      | provided-rv-send.pml  |
      | provided-rv-recv.pml  |
      | timeout-gate.pml      |

  # A property that reads `_nr_pr` over a model whose processes keep no process
  # table is refused by itself (`not-executed`, g5-ctl-v1.feature): it is neither
  # compiled nor evaluated, so no worker sees it, and the properties beside it
  # are answered as the breadth-first search answers them.
  Scenario: a property refused for the process table does not disturb the parallel search of the others
    Given the model under parallel test "testdata/ir/nrpr-property.json"
    When I check it sequentially with "--sweep --bfs --no-timing"
    And I check it in parallel with 3 workers and "--sweep --no-timing"
    Then the parallel search was applied with 3 workers
    And property "none" is "not-executed" in the parallel run
    And property "two" is "not-executed" in the parallel run
    And property "sane" is "verified" in the parallel run
    And every property has the same status and evidence in the parallel and the sequential run
    And both runs have the same states and transitions

  # --por wins where the reduction applies (it is depth-first): the reduced
  # sequential run, with the parallel search refused and the reason.
  Scenario: --por with --workers is the reduced run where the reduction applies
    Given the model under parallel test "testdata/promela/bench-indep.pml"
    When I check it in parallel with 4 workers and "-D N=3 -D K=3 --sweep --por --no-timing"
    Then the parallel search was refused with a reason mentioning "partial-order reduction is depth-first only"
    And the reduction was applied in the parallel run
    And the parallel run reports the search mode "dfs"

  # Where the reduction refuses (a rendezvous channel; an atomic sequence was
  # the example before step 6 of the performance plan reduced them), the
  # parallel search runs and the reduction record keeps its own reason.
  Scenario: --por with --workers where the reduction refuses runs the parallel search
    Given the model under parallel test "testdata/promela/por-rendezvous.pml"
    When I check it in parallel with 4 workers and "--sweep --por --no-timing"
    Then the parallel search was applied with 4 workers
    And the reduction was not applied in the parallel run

  # An atomic sequence is reduced since step 6, so --por wins there as it does
  # for any model the reduction applies to: the reduced depth-first run, with
  # the parallel search refused and the reason.
  Scenario: --por with --workers on an atomic sequence is the reduced run
    Given the model under parallel test "testdata/promela/atomic-t3.pml"
    When I check it in parallel with 4 workers and "--sweep --por --no-timing"
    Then the parallel search was refused with a reason mentioning "partial-order reduction is depth-first only"
    And the reduction was applied in the parallel run
    And the parallel run reports the search mode "dfs"

  # What the depth rules above say holds when the parallel search is APPLIED. A
  # run that is refused is the sequential one, so its search mode is the one
  # asked for and --budget-depth counts transitions, not layers of stored
  # states. par-atomic-depth.pml with --budget-depth 1: the applied run (second
  # parallel run) expands the layer-1 state that holds the whole atomic sequence
  # and decides the failing assert; the refused run, which is the default search,
  # leaves it inconclusive at depth 1 in transitions, and is otherwise the same
  # report as a run without --workers. Passed on its first run.
  Scenario Outline: a refused parallel run counts the depth budget in transitions, like the sequential run
    Given the model under parallel test "testdata/promela/par-atomic-depth.pml"
    When I check it sequentially with "--budget-depth 1 --no-timing <temporal>"
    And I check it in parallel with 2 workers and "--budget-depth 1 --no-timing <temporal>"
    And I check it in parallel with 2 workers and "--budget-depth 1 --no-timing" as the second parallel run
    Then the parallel search was refused with a reason mentioning "ltl, progress or ctl"
    And the parallel run reports the search mode "dfs"
    And the parallel run has depth 1
    And property "assert" is "inconclusive" in the parallel run
    And the report of the parallel run equals the sequential report except for the parallel object
    And the second parallel run reports the search mode "bfs"
    And property "assert" is "violated" in the second parallel run

    Examples:
      | temporal          |
      | --ltl '[] true'   |
      | --progress        |
      | --ctl 'AG(true)'  |

  # The same for --por where the reduction applies: the reduced depth-first run,
  # with --budget-depth counting transitions.
  Scenario: a --por run that refuses the parallel search counts the depth budget in transitions
    Given the model under parallel test "testdata/promela/bench-indep.pml"
    When I check it sequentially with "-D N=3 -D K=3 --por --budget-depth 4 --no-timing"
    And I check it in parallel with 2 workers and "-D N=3 -D K=3 --por --budget-depth 4 --no-timing"
    Then the parallel search was refused with a reason mentioning "partial-order reduction is depth-first only"
    And the parallel run reports the search mode "dfs"
    And the parallel run has depth 4
    And the parallel run has 6 states and 5 transitions
    And the report of the parallel run equals the sequential report except for the parallel object

  Scenario: --workers is a usage error with --estimate and out of range
    When I run mcd with "check --promela testdata/promela/bench-indep.pml --estimate --workers 4"
    Then the last run exits with 1
    And the standard error of the last run mentions "--workers"
    When I run mcd with "check --promela testdata/promela/bench-indep.pml --workers -1"
    Then the last run exits with 1
    When I run mcd with "check --promela testdata/promela/bench-indep.pml --workers 1000"
    Then the last run exits with 1
