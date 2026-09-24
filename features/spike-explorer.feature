# Spike (plan 14 §9): a minimal explicit-state explorer on hard-coded models,
# used only to calibrate speed and memory against SPIN's pan. Nothing here is
# the product engine; the scenarios fix the behaviour the spike must show so
# that its measurements can be trusted.
#
# Exit criterion (14 §9, Spike row): measurements obtained; G0–G4 estimates
# re-estimated from them; go/continue decision recorded. Scenarios 1–3 make the
# engine trustworthy enough to measure; scenario 4 is the throughput floor the
# measurement is taken on. The estimates and the decision live in
# steps/spike-confirmation.md, not in code.
#
# Vocabulary: status is from 11 §14 (verified / violated / inconclusive);
# evidence is exhaustive when the whole reachable graph was visited.
# "deadlock" in this spike means: a reachable state with no enabled transition.
# Every hard-coded model has only non-terminating processes, so this coincides
# with SPIN's "invalid end state" for exactly these models and no others.

Feature: Spike explorer — calibration engine on hard-coded models

  Background:
    Given the spike explorer with a budget of 2000000 states and 60 seconds

  Scenario: petrinet1 hangs after t1, t4 (Holzmann §8.10, App_C/petrinet1)
    Given the hard-coded model "petrinet1" with initial marking p1=1 p4=1
    When I run the explorer
    Then the status is "violated" with violation kind "deadlock"
    And the evidence is "exhaustive"
    And the witness is the transition sequence "t1, t4"
    And the final state is described as "p2=1 p5=1"
    And the number of stored states is 4
    And replaying the witness from the initial state reproduces the final state

  # Oracle comparison (spin 6.5.2, gcc -DNOREDUCE, pan -c0): 8 states stored,
  # of which 2 are the `p1 = 1` / `p4 = 1` initialisation steps of the Promela
  # init process that the Petri-net encoding does not have; 8 - 2 = 6 markings.
  # The same figure results with and without spin -o1 -o2 -o3.
  Scenario: petrinet1 full sweep stores exactly the 6 reachable markings
    Given the hard-coded model "petrinet1" with initial marking p1=1 p4=1
    When I run the explorer continuing after violations
    Then the status is "violated" with violation kind "deadlock"
    And the number of stored states is 6
    And the number of violations seen is 1

  Scenario: mutex_flaw violates assert(cnt == 1) (CH2/mutex_flaw.pml)
    Given the hard-coded model "mutex_flaw" with two user processes
    When I run the explorer
    Then the status is "violated" with violation kind "assertion"
    And the evidence is "exhaustive"
    And the violated statement is "assert(cnt == 1)"
    And the final state has variable "cnt" equal to 2
    And the witness is not empty
    And replaying the witness from the initial state reproduces the final state

  # Oracle comparison (spin 6.5.2, gcc -DNOREDUCE, pan -c0, same figure with
  # -o1 -o2 -o3): 429 states stored. pan stores 202 states before its first
  # error, but that number depends on pan's own DFS order and is not compared.
  Scenario: mutex_flaw full sweep stores the same 429 states as pan -c0
    Given the hard-coded model "mutex_flaw" with two user processes
    When I run the explorer continuing after violations
    Then the status is "violated" with violation kind "assertion"
    And the number of stored states is 429

  Scenario Outline: repeated runs give byte-identical reports
    Given the hard-coded model "<model>"
    When I run the explorer twice
    Then both JSON reports are byte-identical

    Examples:
      | model               |
      | petrinet1           |
      | mutex_flaw          |
      | counters(K=10, N=4) |

  Scenario: synthetic counters model reaches 100000 states within budget
    Given the hard-coded model "counters(K=10, N=5)"
    When I run the explorer
    Then the status is "verified"
    And the evidence is "exhaustive"
    And the number of stored states is 100000
    And the run finished within the budget

  Scenario: exhausting the state budget is reported as inconclusive, not as verified
    Given the spike explorer with a budget of 1000 states and 60 seconds
    And the hard-coded model "counters(K=10, N=5)"
    When I run the explorer
    Then the status is "inconclusive" with reason "state budget exhausted"
    And the evidence is "bounded"
    And the number of stored states is 1000
