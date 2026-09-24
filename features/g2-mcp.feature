# G2 (plan 14 §9): the MCP server — seven tools of plan 14 §6, one session
# directory per session, budgets enforced inside the server, manifest.
#
# Exit criterion (14 §9, G2 row): the skill can call all seven tools; limits
# fire; writing outside the session directory is impossible. The scenarios
# below drive the server exactly as the skill does — through an MCP client
# connected to the server (in-process transport in the tests, stdio in the
# plugin) — and observe only tool lists, tool results and files in the
# session directory.
#
# Three kinds of answers, kept apart (NFR-007: syntax / model / tool /
# resource errors are distinguishable):
#   1. A tool failure — bad arguments, a policy refusal (path not allowed),
#      an internal error. The MCP result carries isError = true and a text
#      message; there is no structured result and no status. Nothing about
#      the model is being claimed.
#   2. An input rejection — a frontend refuses the input (schema violation,
#      unsupported construct, invalid IR). mc_parse / mc_check answer with
#      isError = false and `outcome: "rejected"` plus
#      `rejection: {construct, file, line, reason}`. No property gets a status.
#   3. A verification result — every property carries exactly one status of
#      the 11 §14 vocabulary (verified, violated, inconclusive, unknown,
#      not-executed, invalid-model) and one evidence level (exhaustive,
#      bounded, approximate, unknown), with the meanings fixed by
#      features/g0-engine.feature. Budget exhaustion is `inconclusive` with the
#      resource named (resource error); a model that misbehaves is
#      `invalid-model` (model error). Property kinds ltl, ctl and progress are
#      `not-executed` in G2 and the reason names the missing capability and
#      the step that brings it (G4 for LTL and progress, G5 for CTL); the
#      server never fabricates a verdict for them.
#
# Aggregation: statuses are not aggregated unless the caller asks
# (`aggregate: true`); then the priority is
# invalid-model > not-executed > violated > inconclusive > unknown > verified.
#
# Session directory: every tool answer names its session id; large results
# (IR, reports, traces) are files under <base>/<session id>/ and the answer
# gives their paths. Every write goes through one guard that resolves
# symlinks and refuses any path that leaves the session directory; the same
# guard resolves ids the client supplies (counterexample ids) into paths.
# mc_parse reads a file only when the client names it explicitly and the
# server was started with --allow-read for a prefix of it; otherwise inputs
# are inline.
#
# Budgets: a client budget field left at 0 takes the server default; a field
# above the server ceiling is clamped to the ceiling and the answer says so in
# `search.budget_notes`. Concurrent mc_check calls beyond --concurrency wait.
Feature: G2 — MCP server with the seven tools, session directory, budgets

  Background:
    Given an MCP server with a fresh session base directory and ceiling states 1000, depth 1000, ms 5000, memory_mb 256

  # --- tool list -------------------------------------------------------------

  Scenario: the server lists exactly the seven tools of plan 14 §6
    When I list the tools
    Then the tool names are exactly "mc_parse, mc_simulate, mc_check, mc_explain, mc_lint_property, mc_estimate, mc_manifest"
    And every tool has an input schema of type object and an output schema of type object
    And every tool has a non-empty description

  # --- mc_parse --------------------------------------------------------------

  Scenario: mc_parse of petrinet1 returns the IR and a session id
    When I call "mc_parse" with the Petri net "testdata/petri/petrinet1.json" inline
    Then the call is not an error
    And the answer has a session id
    And the answer field "outcome" is "ir"
    And the answer IR declares 6 global variables and 1 process with 6 edges
    And the answer field "ir_path" names a file inside the session directory
    And that IR file equals the output of "mcd parse --petri testdata/petri/petrinet1.json"
    And the answer lists 6 origins with user names "t1, t2, t3, t4, t5, t6" among them

  Scenario: mc_parse rejects an unsupported construct with a structured rejection, not a tool error
    When I call "mc_parse" with the Petri net "testdata/petri/inhibitor.json" inline
    Then the call is not an error
    And the answer field "outcome" is "rejected"
    And the rejection names the construct "inhibitor" and its reason mentions "8.10"
    And the answer has no field "ir"

  Scenario: mc_parse with Promela source reports the frontend that is missing in this build
    When I call "mc_parse" with the Promela source "active proctype P() { skip }"
    Then the call is not an error
    And the answer field "outcome" is "not-executed"
    And the answer field "reason" mentions "promela frontend"

  Scenario: mc_parse reads a file only if the server allows its prefix
    When I call "mc_parse" with the file path "testdata/petri/petrinet1.json" of kind "petri"
    Then the call is an error whose message mentions "--allow-read"
    And the tool error message also mentions "inline"

  Scenario: mc_parse reads a file under an allowed prefix
    Given the server was started with --allow-read "testdata"
    When I call "mc_parse" with the file path "testdata/petri/petrinet1.json" of kind "petri"
    Then the call is not an error
    And the answer field "outcome" is "ir"

  Scenario: mc_parse with no input at all is a tool error, not a rejection
    When I call "mc_parse" with no input
    Then the call is an error whose message mentions "exactly one of"

  # --- mc_check --------------------------------------------------------------

  Scenario: mc_check finds the deadlock of petrinet1 and stores the counterexample in the session directory
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_check" in that session with the model's own properties
    Then the call is not an error
    And the answer property "deadlock" has status "violated" with evidence "exhaustive"
    And the answer property "safe" has status "verified" with evidence "exhaustive"
    And the answer says the search is complete
    And the counterexample of "deadlock" has summary "t1, t4" and 2 steps
    And the counterexample of "deadlock" is a file inside the session directory
    And the answer field "report_path" names a file inside the session directory
    And that report file is a report whose property "deadlock" has status "violated"

  Scenario: a tiny state budget makes the check inconclusive and names the resource
    Given a session in which the IR "testdata/ir/counters-10-5.json" was parsed
    When I call "mc_check" in that session with the model's own properties and budget states 100, depth 0, ms 0, memory_mb 0
    Then the call is not an error
    And the answer property "deadlock" has status "inconclusive" with evidence "bounded"
    And the answer says the search is not complete
    And the reason of "deadlock" names the exhausted resource "states"
    And the applied budget has states 100

  Scenario: a tiny depth budget names depth as the exhausted resource
    Given a session in which the IR "testdata/ir/counters-10-5.json" was parsed
    When I call "mc_check" in that session with the model's own properties and budget states 0, depth 5, ms 0, memory_mb 0
    Then the answer property "deadlock" has status "inconclusive" with evidence "bounded"
    And the reason of "deadlock" names the exhausted resource "depth"

  Scenario: a client budget above the server ceiling is clamped and the answer says so
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_check" in that session with the model's own properties and budget states 1000000, depth 5000, ms 60000, memory_mb 4096
    Then the call is not an error
    And the applied budget has states 1000
    And the applied budget has depth 1000
    And the applied budget has ms 5000
    And the applied budget has memory_mb 256
    And the budget notes mention "states" clamped to 1000
    And the budget notes mention "memory_mb" clamped to 256

  Scenario: a budget left at zero takes the server default, which lies within the ceiling
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_check" in that session with the model's own properties
    Then the applied budget has states 1000
    And the budget notes are empty

  Scenario Outline: ltl, ctl and progress are not executed in G2 and the reason names the missing step
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_check" in that session with properties:
      | id   | kind   | expr |
      | live | <kind> | p6   |
    Then the call is not an error
    And the answer property "live" has status "not-executed" with evidence "unknown"
    And the answer reason of "live" mentions "<step>"
    And the answer reason of "live" mentions "<capability>"

    Examples:
      | kind     | step | capability   |
      | ltl      | G4   | LTL          |
      | progress | G4   | non-progress |
      | ctl      | G5   | CTL          |

  Scenario: an unknown property kind is a tool error, not a status
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_check" in that session with properties:
      | id | kind | expr |
      | q  | foo  | p6   |
    Then the call is an error whose message mentions "kind"

  Scenario: an invariant that fails is violated with a counterexample readable by mc_explain
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_check" in that session with properties:
      | id     | kind      | expr            |
      | p2zero | invariant | {"op":"eq","args":[{"op":"var","var":"p2"},{"op":"const","value":0}]} |
    Then the answer property "p2zero" has status "violated" with evidence "exhaustive"
    And the counterexample of "p2zero" has summary "t1" and 1 steps
    When I call "mc_explain" for the counterexample of "p2zero"
    Then the call is not an error
    And the explanation has 1 prefix steps and an empty loop
    And the explanation step 1 changes "p1" from 1 to 0 and "p2" from 0 to 1
    And the explanation maps its steps to the user names "t1"
    And the explanation says that loop counterexamples arrive with G4

  Scenario: the aggregate status is computed only on request and follows the fixed priority
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_check" in that session with aggregate requested and properties:
      | id   | kind     | expr |
      | dl   | deadlock |      |
      | live | ltl      | p6   |
    Then the answer property "dl" has status "violated" with evidence "exhaustive"
    And the answer property "live" has status "not-executed" with evidence "unknown"
    And the aggregate status is "not-executed"
    When I call "mc_check" in that session with the model's own properties
    Then the answer has no field "aggregate"

  Scenario: mc_check without a session or an IR is a tool error
    When I call "mc_check" with no input
    Then the call is an error whose message mentions "ir"

  Scenario: mc_check of an IR whose expression is ill-typed is a rejection
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_check" in that session with properties:
      | id  | kind      | expr                          |
      | bad | invariant | {"op":"var","var":"nowhere"}  |
    Then the call is not an error
    And the answer field "outcome" is "rejected"
    And the rejection reason mentions "nowhere"

  # --- determinism (NFR-006) ---------------------------------------------------

  Scenario: two identical mc_check calls produce byte-identical report files when timings are excluded
    Given a session in which the Petri net "testdata/petri/petrinet2.json" was parsed
    When I call "mc_check" in that session with the model's own properties without timings, twice
    Then the two report files are byte-identical
    And the two report files differ from each other only in name

  # --- session directory guard (NFR-004) -------------------------------------

  Scenario: a counterexample id that escapes the session directory is refused
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_explain" in that session for the counterexample id "../../escape"
    Then the call is an error whose message mentions "session directory"
    And no file named "escape" exists anywhere under the session base directory

  Scenario: the session writer refuses paths that leave the session directory
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When the session is asked to write "../outside.json"
    Then the write is refused with a message that mentions "session directory"
    And no file named "outside.json" exists anywhere under the session base directory
    When the session is asked to write "sub/../../outside2.json"
    Then the write is refused with a message that mentions "session directory"
    When the session is asked to write "/tmp/outside3.json"
    Then the write is refused with a message that mentions "session directory"

  Scenario: the session writer refuses a symlink that points outside the session directory
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    And a symlink "link" inside the session directory pointing outside the session base directory
    When the session is asked to write "link/escape.json"
    Then the write is refused with a message that mentions "session directory"
    And no file named "escape.json" exists anywhere under the session base directory

  # --- mc_simulate -------------------------------------------------------------

  Scenario: a random simulation is reproducible from its seed
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_simulate" in that session with seed 7, 20 steps and mode "random", twice
    Then the two simulation answers are identical except for the trace path
    # A random walk on petrinet1 either runs into the deadlock or uses up its
    # steps in the t1, t2, t3 cycle; which one depends on the seed.
    And the simulation stopped because of one of "deadlock, steps"
    And the simulation trace file is inside the session directory

  # After t1 the marking is p2=1 p4=1; t2 fires; then t4 needs p4, which t2
  # consumed, so the run stops there with t3 (init/2) the only enabled edge.
  Scenario: a guided simulation follows the edge ids and stops when an edge is not enabled
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_simulate" in that session guided by the edges "t1, t2, t4"
    Then the simulation took 2 steps with summary "t1, t2"
    And the simulation stopped because of one of "edge not enabled"
    And the simulation names the enabled edges at the stop as "init/2"

  # A deadlock reached under guidance is reported as a deadlock, not as "edge
  # not enabled": the stronger fact wins.
  Scenario: a guided simulation that reaches the deadlock says so
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_simulate" in that session guided by the edges "t1, t4, t2"
    Then the simulation took 2 steps with summary "t1, t4"
    And the simulation stopped because of one of "deadlock"
    And the simulation names the enabled edges at the stop as ""

  # --- mc_lint_property ---------------------------------------------------------

  Scenario: mc_lint_property lists atoms, undefined atoms and the property class
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_lint_property" in that session with kind "invariant" and expr {"op":"and","args":[{"op":"gt","args":[{"op":"var","var":"p1"},{"op":"const","value":0}]},{"op":"var","var":"p9"}]}
    Then the call is not an error
    And the lint lists the atoms "p1, p9"
    And the lint lists the undefined atoms "p9"
    And the lint class is "safety"
    And the lint says the expression is X-free and not temporal

  Scenario: mc_lint_property flags a constant expression as a vacuity candidate
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_lint_property" in that session with kind "reach" and expr {"op":"const","value":1}
    Then the lint class is "reachability"
    And the lint notes mention "constant"

  # --- mc_estimate --------------------------------------------------------------

  Scenario: mc_estimate reports the states visited within a time limit and a growth projection
    Given a session in which the IR "testdata/ir/counters-10-5.json" was parsed
    When I call "mc_estimate" in that session with a time limit of 2000 ms
    Then the call is not an error
    And the estimate reports at least 100 states visited and a positive states-per-second rate
    And the estimate has a growth table with at least 2 levels and a growth rate
    And the estimate projection carries evidence "approximate"
    And the estimate is not a verification result and says so

  # --- mc_manifest --------------------------------------------------------------

  Scenario: mc_manifest lists the engine version, input hashes and every prior call in order
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_check" in that session with the model's own properties
    And I call "mc_manifest" in that session
    Then the call is not an error
    And the manifest names the engine "mcd" with a version and the schemas "mcd-ir/1" and "mcd-report/1"
    And the manifest lists 1 input with a sha256 hash
    And the manifest lists the calls "mc_parse, mc_check, mc_manifest" in this order
    And every manifest call has a duration and an outcome of "ok" or "error"
    And the manifest records the applied budget of the check
    And the manifest is a file inside the session directory

  Scenario: mc_manifest of an unknown session is a tool error
    When I call "mc_manifest" for the session id "no-such-session"
    Then the call is an error whose message mentions "unknown session"
