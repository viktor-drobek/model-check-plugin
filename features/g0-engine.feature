# G0 (plan 14 §9): IR, Petri-net frontend, explicit-state explorer (DFS/BFS),
# JSON report and the `mcd` CLI. Every scenario observes the engine through
# the CLI and its JSON output, never through Go structures.
#
# Exit criterion (14 §9, G0 row): petrinet1 → deadlock `t1, t4`; petrinet2
# gives a reproducible result; budget tests; the overhead of interpreting the
# IR relative to the Spike is measured and does not exceed 20×. The
# measurement itself lives in steps/g0-confirmation.md; the scenarios below
# fix the behaviour the measurement is taken on.
#
# Vocabulary (11 §14, one status per property, the six values partition the
# outcomes):
#   verified      — for deadlock/invariant/assert: the property holds on the
#                   whole reachable graph, which requires complete = true; for
#                   reach: a state satisfying the condition was found, and the
#                   witness (an exact run) is attached, so complete may be
#                   false. Evidence "exhaustive" in both cases.
#   violated      — for deadlock/invariant/assert: a counterexample was found;
#                   it is an exact run of the model, so the evidence is
#                   "exhaustive" even when the search was cut short afterwards
#                   (complete may be false). For reach: a complete search found
#                   no satisfying state (evidence "exhaustive", complete = true).
#   inconclusive  — a budget (states, depth, time, memory) stopped the search
#                   before the property was decided; evidence "bounded",
#                   `reason` names the exhausted resource, complete = false.
#   invalid-model — the model itself misbehaved: a variable left its domain
#                   (e.g. a place exceeded its capacity), an index left its
#                   array, a division by zero. Every property still undecided
#                   at that step gets this status, evidence "unknown", and the
#                   trace to the offending step as its counterexample. A
#                   verdict decided earlier stands: its own run reached no
#                   offending step, otherwise it would have been invalid there.
#   not-executed  — the property kind is outside what this engine version
#                   executes; evidence "unknown", reason says which kind.
#   unknown       — reserved (11 §12: a backend answer that is not even a
#                   partial coverage); G0 never emits it.
#
# "deadlock" means exactly this, in the feature, in the code and in the
# report: a reachable state with no enabled transition in which not every
# process is terminated; a process is terminated when its control location
# carries the `end` label or has no outgoing edge. For a Petri net encoded as
# one looping process this is Holzmann's *hang* (§8.10): no transition is
# enabled in the marking.
#
# Exit codes of `mcd`, one per outcome: 0 — a result document (report or IR)
# was produced, whatever the verdicts; 2 — no result: the input was rejected
# by a frontend, and stdout carries a JSON error document instead; 1 — no
# result: tool error (unreadable file, bad flags), message on stderr.
#
# Timing (`time_ms`) is the only report field that is not a function of the
# input; `--no-timing` omits it so that runs can be compared byte for byte.

Feature: G0 engine — Petri nets through IR to a JSON verdict via the mcd CLI

  # --- Petri frontend and explorer on the corpus --------------------------

  Scenario: petrinet1 hangs after t1, t4 (Holzmann §8.10, App_C/petrinet1)
    Given the Petri net file "testdata/petri/petrinet1.json"
    When I run "mcd check --petri <file> --budget-states 100000 --budget-depth 100000 --budget-ms 10000"
    Then the exit code is 0
    And the property "deadlock" has status "violated" with evidence "exhaustive"
    And the counterexample of "deadlock" fires the transitions "t1, t4"
    And the final marking of the counterexample of "deadlock" is "p2=1 p5=1"
    And the counterexample of "deadlock" maps its steps to the user names "t1, t4"

  # Oracle (spin 6.5.2, gcc -O2 -DNOREDUCE, pan -c0): 8 states stored, of
  # which 2 are the `p1 = 1` / `p4 = 1` initialisation steps of the Promela
  # init process that the Petri encoding does not have; 8 - 2 = 6 markings.
  Scenario: petrinet1 is safe and the full sweep stores the 6 reachable markings
    Given the Petri net file "testdata/petri/petrinet1.json"
    When I run "mcd check --petri <file> --budget-states 100000 --budget-depth 100000 --budget-ms 10000"
    Then the exit code is 0
    And the property "safe" has status "verified" with evidence "exhaustive"
    And the report is complete
    And the report counts 6 states

  # petrinet2 has no oracle-free expected verdict. The golden file is the
  # engine's own output, so this scenario is a regression pin and a
  # determinism check, not a correctness proof; the correctness ground is
  # the agreement with pan recorded in steps/g0-confirmation.md (verdict
  # class, first witness, pan stored = engine states + 2 init assignments),
  # obtained before the file was frozen.
  Scenario: petrinet2 gives a reproducible, golden result
    Given the Petri net file "testdata/petri/petrinet2.json"
    When I run "mcd check --petri <file> --budget-states 100000 --budget-depth 100000 --budget-ms 10000 --no-timing" twice
    Then both outputs are byte-identical
    And the output equals the golden file "testdata/golden/petrinet2.report.json"

  # --- Frontend rejections and model validity ------------------------------

  Scenario: an inhibitor arc is rejected with an explanation
    Given the Petri net file "testdata/petri/inhibitor.json"
    When I run "mcd parse --petri <file>"
    Then the exit code is 2
    And the error kind is "unsupported-input"
    And the error message mentions "inhibitor"
    And the error message mentions "Holzmann"
    And the error message mentions "t2"

  Scenario: a net that violates the JSON schema is rejected at the offending path
    Given the Petri net file "testdata/petri/bad-weight.json"
    When I run "mcd parse --petri <file>"
    Then the exit code is 2
    And the error kind is "schema"
    And the error message mentions "transitions[0].inputs[0].weight"

  # A place with capacity 1 that receives a second token: the model, not the
  # system, is at fault, so every property gets invalid-model (14 §11).
  Scenario: exceeding a place capacity is invalid-model, not a violation
    Given the Petri net file "testdata/petri/overflow.json"
    When I run "mcd check --petri <file> --budget-states 1000 --budget-depth 1000 --budget-ms 10000"
    Then the exit code is 0
    And the property "deadlock" has status "invalid-model" with evidence "unknown"
    And the property "safe" has status "invalid-model" with evidence "unknown"
    And the reason of "deadlock" mentions "capacity"
    And the reason of "deadlock" mentions "buf"
    And the counterexample of "deadlock" fires the transitions "produce, produce"

  # --- Budgets -------------------------------------------------------------

  # counters(K=10, N=5): 100000 states, no deadlock. IR-encoded version of the
  # Spike's synthetic model (see testdata/ir/README.md).
  Scenario Outline: exhausting a budget is inconclusive with the resource named
    Given the IR file "testdata/ir/counters-10-5.json"
    When I run "mcd check --ir <file> <flags>"
    Then the exit code is 0
    And the property "deadlock" has status "inconclusive" with evidence "bounded"
    And the reason of "deadlock" mentions "<resource>"
    And the report is not complete

    Examples:
      | flags                                                         | resource |
      | --budget-states 1000 --budget-depth 1000000 --budget-ms 60000 | states   |
      | --budget-states 1000000 --budget-depth 50 --budget-ms 60000   | depth    |

  # counters(K=10, N=6): 1 000 000 states, more than a second of work, so a
  # 20 ms budget always runs out before the sweep ends.
  Scenario: exhausting the time budget is inconclusive with the resource named
    Given the IR file "testdata/ir/counters-10-6.json"
    When I run "mcd check --ir <file> --budget-states 0 --budget-depth 0 --budget-ms 20"
    Then the exit code is 0
    # G4 (plan 14 §6 as amended): time and memory are limits that happened, not
    # declared search bounds, so their evidence is `unknown`; states and depth
    # stay `bounded`. Since G4, 0 in a budget flag means the default.
    And the property "deadlock" has status "inconclusive" with evidence "unknown"
    And the reason of "deadlock" mentions "time"
    And the report is not complete

  Scenario: the same model within budget is verified and complete
    Given the IR file "testdata/ir/counters-10-5.json"
    When I run "mcd check --ir <file> --budget-states 200000 --budget-depth 200000 --budget-ms 60000"
    Then the exit code is 0
    And the property "deadlock" has status "verified" with evidence "exhaustive"
    And the report is complete
    And the report counts 100000 states

  # Growing the exact visited set (performance plan, step 1). The store is an
  # open-addressing table over an arena of full state vectors. An arena that
  # grows by append copies every stored vector again and again (Go grows a
  # large slice by about a quarter at a time, so the copies add up to several
  # times the final arena); a chunked arena never moves a stored vector. The
  # observable consequence is pinned here: the verdict and the counts do not
  # change, and the bytes a complete run allocates stay within a small
  # multiple of the memory it reports. The bound is deliberately loose (the
  # table still doubles, and the report, the IR and the frames are allocated
  # too); it separates "grows in place" from "copies on every growth".
  Scenario: a complete run allocates little more than the memory it reports
    Given the IR file "testdata/ir/counters-10-5.json"
    When I run "mcd check --ir <file> --budget-states 200000 --budget-depth 200000 --budget-ms 60000 --no-timing" measuring the bytes allocated
    Then the exit code is 0
    And the property "deadlock" has status "verified" with evidence "exhaustive"
    And the report is complete
    And the report counts 100000 states
    And the bytes allocated are at most 3 times the reported memory estimate

  # --- Search modes --------------------------------------------------------

  # bfs-shortest.json: t1: p1→a, t2: a→b, t3: b→dead, t4: p1→dead. DFS in
  # transition order goes t1, t2, t3 and reports a three-step witness; BFS
  # finds the one-step witness t4.
  Scenario: DFS reports the first witness on its stack
    Given the Petri net file "testdata/petri/bfs-shortest.json"
    When I run "mcd check --petri <file> --budget-states 1000 --budget-depth 1000 --budget-ms 10000"
    Then the property "deadlock" has status "violated" with evidence "exhaustive"
    And the counterexample of "deadlock" fires the transitions "t1, t2, t3"

  Scenario: BFS returns a shortest witness
    Given the Petri net file "testdata/petri/bfs-shortest.json"
    When I run "mcd check --petri <file> --budget-states 1000 --budget-depth 1000 --budget-ms 10000 --bfs"
    Then the property "deadlock" has status "violated" with evidence "exhaustive"
    And the counterexample of "deadlock" fires the transitions "t4"

  # --- IR ------------------------------------------------------------------

  Scenario: the IR round-trips through JSON and checks identically
    Given the Petri net file "testdata/petri/petrinet1.json"
    When I run "mcd parse --petri <file>" and save the output as "ir1"
    And I run "mcd parse --ir ir1" and save the output as "ir2"
    Then the saved outputs "ir1" and "ir2" are byte-identical
    And the saved output "ir1" declares 6 global variables of type "byte" and 1 process with 6 edges
    When I run "mcd check --ir ir1 --budget-states 1000 --budget-depth 1000 --budget-ms 10000 --no-timing" and save the output as "r-ir"
    And I run "mcd check --petri <file> --budget-states 1000 --budget-depth 1000 --budget-ms 10000 --no-timing" and save the output as "r-petri"
    Then the saved outputs "r-ir" and "r-petri" are identical except for the inputs section

  Scenario: the IR carries the source mapping of every Petri element
    Given the Petri net file "testdata/petri/petrinet1.json"
    When I run "mcd parse --petri <file>"
    Then the exit code is 0
    And every global variable and every edge of the IR has an origin with a user name

  # --- CLI contract --------------------------------------------------------

  Scenario: mcd version prints the engine version
    When I run "mcd version"
    Then the exit code is 0
    And the output mentions "mcd"

  Scenario: a missing input file is a tool error
    When I run "mcd check --petri testdata/petri/does-not-exist.json"
    Then the exit code is 1

  Scenario: the report names the engine version and hashes its inputs
    Given the Petri net file "testdata/petri/petrinet1.json"
    When I run "mcd check --petri <file> --budget-states 1000 --budget-depth 1000 --budget-ms 10000"
    Then the report names the engine and its version
    And the report lists 1 input with a sha256 hash
