# G3 (evals, stage 3) — plan 14 §9, row G3, staged criterion after G4:
# "E2 and E4 pass assertions with the skill and fail without" (E1/E3/E5 passed
# after G0/G1, steps/g3-evals-confirmation.md; they are re-run here so that
# iteration-3 is complete for every eval the engine can execute before G5).
#
# What G4 delivered and this half must reflect (steps/g4-confirmation.md §7):
#   - `mcd check` flags --ltl (repeatable), --progress, --fairness none|weak|strong
#     (strong → not-executed with a reason), --unlimited (CLI only); an absent or 0
#     budget flag now means the default in the CLI as in MCP;
#   - report fields `temporal{source, formula, negated, atoms, stutter_invariant,
#     automaton_*, fairness, claim}` and `counterexample.loop{start, steps}`; steps
#     of the claim process `never…`/`np_` and null steps of process `-`;
#   - MCP: `mc_parse{promela}` returns IR (the frontend is linked), `mc_check`
#     properties carry `formula` for ltl, `mc_explain` splits `prefix`/`loop`;
#   - the frontend adds `never` / `accept` / `progress` properties from the model.
#
# What the evals must reflect (found in G4, steps/g4-confirmation.md §5):
#   CH3/alternatingbit.pml is lock-step — delivery holds with and without
#   fairness (pan agrees), so E2 asks the skill to state the assumption and NOT
#   to claim more than the lock-step model shows; the starvation case is
#   E2b (id 7) on engine/testdata/promela/starvation.pml, where `<>done` is
#   violated without fairness by a loop in which only A moves and verified under
#   weak fairness; its workspace directory is eval-2b-starvation-loop, named by
#   the eval's own `dir` field rather than by its id.
#   E4 (CH14/version1) has no variables and no progress labels, and label atoms
#   (`proc@label`) are rejected by the engine: `--progress` on it as written
#   reports a non-progress cycle because no cycle can visit a progress label —
#   the skill must read that as "the model has no progress labels" and place one
#   in a declared rewrite before answering, after which the frontend adds the
#   `progress` property by itself.
#
# The runs are made by subagents through the CLI (`mcd` built from engine/);
# an MCP server cannot be registered into a subagent's session; the MCP path
# is exercised once by hand (steps/g3-evals3-mcp-session.md). Paths are
# relative to model-check-plugin/skills/model-check/ unless a step says
# "workspace" (relative to model-check-plugin/), "repository file" or "engine
# test model" (relative to model-check-plugin/engine/testdata/promela/).
Feature: G3 evals — stage 3 after G4: E2, E2b and E4 with and without the skill

  Background:
    Given the skill directory "skills/model-check"

  # --------------------------------------------------------- engine-tools
  Scenario: The engine-tools reference names every flag of mcd check after G4 and the new report fields
    Then "references/engine-tools.md" mentions every flag that "mcd check" accepts according to its usage text
    And "references/engine-tools.md" mentions every flag that "mcd parse" accepts according to its usage text
    And "references/engine-tools.md" mentions each of:
      | --ltl                   |
      | --progress              |
      | --fairness              |
      | --unlimited             |
      | `temporal`              |
      | `formula`               |
      | `negated`               |
      | `atoms`                 |
      | `stutter_invariant`     |
      | `automaton_states`      |
      | `automaton_transitions` |
      | `automaton_accepting`   |
      | `fairness`              |
      | `claim`                 |
      | `loop`                  |
      | `start`                 |
      | `loop_note`             |
      | `prefix`                |
      | `never`                 |
      | `accept`                |
      | `progress`              |
      | `np_`                   |
    And "references/engine-tools.md" says that --unlimited exists in the CLI only
    And "references/engine-tools.md" says that the budget rule is the same in the CLI and in MCP
    And "references/engine-tools.md" says that mcd serve links the Promela frontend
    And "references/engine-tools.md" says that fairness strong gives not-executed
    # Amended by the G5 addendum pass: CTL is executed since G5, so the old step
    # ("ctl is not-executed until G5") was green and wrong (g5-confirmation.md §8).
    And "references/engine-tools.md" says that ctl is executed since G5
    And "references/engine-tools.md" has at most 300 lines or a table of contents

  Scenario: The engine-tools reference copies its MCP examples from the recorded session, not from memory
    Then "references/engine-tools.md" mentions each of:
      | steps/g3-evals3-mcp-session.md |
      | never:ltl1                     |
    And "references/engine-tools.md" says that the MCP examples were copied from the recorded session

  # ------------------------------------------------------------ fairness
  Scenario: The fairness reference states what the engine does after G4
    Then "references/fairness.md" states that strong fairness is not supported by the engine
    And "references/fairness.md" states that weak fairness is supported
    And "references/fairness.md" mentions each of:
      | `fairness: weak`   |
      | `--fairness weak`  |
      | `not-executed`     |
      | starvation.pml     |
      | `-`                |
      | n + 2              |
    And "references/fairness.md" says that fairness is reported as an assumption and not as a fact about the system
    And "references/fairness.md" says that alternatingbit is lock-step so its delivery does not depend on fairness

  # ------------------------------------------------------ counterexamples
  Scenario: The counterexamples reference documents the lasso as the engine emits it
    Then "references/counterexamples.md" mentions each of:
      | `loop.start`  |
      | `loop.steps`  |
      | `never`       |
      | `np_`         |
      | `-`           |
      | `loop_note`   |
      | `prefix`      |
    And "references/counterexamples.md" says that loop.start is the 1-based index of the first loop step
    And "references/counterexamples.md" says how to read a stuttering process in a lasso

  # ---------------------------------------------------- properties, claims
  Scenario: The properties reference states the claim semantics and the limits of this build
    Then "references/properties-ltl-ctl.md" mentions each of:
      | `stutter_invariant`  |
      | stutter extension    |
      | claim moves first    |
      | `progress`           |
    And "references/properties-ltl-ctl.md" says that a formula with X gives stutter_invariant false
    And "references/properties-ltl-ctl.md" says that the engine checks asserts over the whole state space while pan -a checks them in claim scope
    And "references/properties-ltl-ctl.md" says that a claim reaching its end is a violation on a finite prefix
    And "references/properties-ltl-ctl.md" says that label atoms are rejected in ltl and accepted in ctl
    And "references/properties-ltl-ctl.md" says that the progress property is added when the model has progress labels
    And "references/properties-ltl-ctl.md" says that ctl is executed since G5

  # ------------------------------------------------------------ evidence
  Scenario: The evidence reference carries the G4 budget rule
    Then "references/evidence-and-status.md" states that bounded is used when the engine can name the bound and unknown when it cannot
    And "references/evidence-and-status.md" says that a states or depth budget stop is bounded and a time or memory stop is unknown
    And "references/evidence-and-status.md" says that ltl results carry evidence exhaustive after the G4 oracle

  Scenario: SKILL.md tells the reader what G4 changed in its own steps
    Then "SKILL.md" mentions each of:
      | `fairness: weak` |
      | `loop`           |
      | `progress`       |
      | `formula`        |
    And "SKILL.md" says that strong fairness is unsupported
    And "SKILL.md" says that Promela input goes through the promela field of mc_parse or through "mcd parse --promela"
    And the body of "SKILL.md" has at most 500 lines

  # ---------------------------------------------------------- evals.json
  Scenario: evals.json has E2 rewritten for the lock-step model and E2b for the starvation model
    Then "evals/evals.json" parses as JSON
    And "evals/evals.json" has exactly 7 evals with ids 1 to 7
    And the eval with id 7 has "variant_of" equal to 2
    And the eval with id 7 has "dir" equal to "eval-2b-starvation-loop"
    And the eval with id 7 has "fixture" equal to "model-check-plugin/engine/testdata/promela/starvation.pml"
    And the eval with id 2 has "runnable_from" equal to "G4"
    And the eval with id 4 has "runnable_from" equal to "G4"
    And the eval with id 7 has "runnable_from" equal to "G4"
    And the evals in "evals/evals.json" with "runnable_from" at or before "G4" are exactly ids "1, 2, 3, 4, 5, 7"
    And every assertion in "evals/evals.json" has a "text" and a "check" whose "type" is one of:
      | regex              |
      | not_regex          |
      | regex_order        |
      | petri_json_valid   |
      | json_field         |
      | json_list_contains |

  Scenario: The fixtures README records the E2, E2b and E4 goldens the assertions rest on
    Then every SHA-256 row in "evals/fixtures/README.md" matches the file it names, resolved from the repository root
    And "evals/fixtures/README.md" names each of:
      | model-check-plugin/engine/testdata/promela/starvation.pml |
      | Promela - examples/CH3/alternatingbit.pml                 |
      | Promela - examples/CH14/version1                          |
    And "evals/fixtures/README.md" mentions each of:
      | --ltl                |
      | --fairness weak      |
      | --progress           |
      | lock-step            |
      | loop                 |

  # ---------------------------------------------------------- goldens
  Scenario: E2 golden — delivery on alternatingbit is verified with and without weak fairness
    Then running "mcd check --no-timing --promela" on the repository file "Promela - examples/CH3/alternatingbit.pml" with the ltl formula "[] ((len(to_rcvr) > 0) -> <> (len(to_rcvr) == 0))" and fairness "none" exits 0
    And the report has property "ltl1" with status "verified" and evidence "exhaustive" and complete true
    And that property's temporal record has fairness "none" and stutter_invariant true
    And that property's counters show 10 states
    Then running "mcd check --no-timing --promela" on the repository file "Promela - examples/CH3/alternatingbit.pml" with the ltl formula "[] ((len(to_rcvr) > 0) -> <> (len(to_rcvr) == 0))" and fairness "weak" exits 0
    And the report has property "ltl1" with status "verified" and evidence "exhaustive" and complete true
    And that property's temporal record has fairness "weak" and stutter_invariant true

  Scenario: E2b golden — <>done on starvation.pml is violated by a loop in which only A moves, and verified under weak fairness
    Then running "mcd check --no-timing --promela" on the engine test model "starvation.pml" with the ltl formula "<> done" and fairness "none" exits 0
    And the report has property "ltl1" with status "violated" and evidence "exhaustive" and complete false
    And that property's counterexample has a loop starting at step 3 of 4 steps
    And every loop step of that property is by process "A:0" or by the claim
    And that property's reason names "B:1" as enabled throughout the loop and never moving
    Then running "mcd check --no-timing --promela" on the engine test model "starvation.pml" with the ltl formula "<> done" and fairness "weak" exits 0
    And the report has property "ltl1" with status "verified" and evidence "exhaustive" and complete true
    Then running "mcd check --no-timing --promela" on the engine test model "starvation.pml" with the ltl formula "<> done" and fairness "strong" exits 0
    And the report has property "ltl1" with status "not-executed" and evidence "unknown" and complete true

  Scenario: E4 golden — version1 as written has no progress label, so --progress reports a non-progress cycle
    Then running "mcd check --no-timing --progress --promela" on the repository file "Promela - examples/CH14/version1" exits 0
    And the report has property "progress" with status "violated" and evidence "exhaustive" and complete false
    And that property's temporal record has source "np" and claim "np_"
    And that property's counterexample has a loop of 16 steps
    And the report has property "deadlock" with status "verified" and evidence "exhaustive" and complete true

  # ------------------------------------ the G5 addendum: stutter and the pool
  # After G4 the engine extended every finite run into an infinite one, in every
  # product. K3's mutation campaign showed that this reported a deadlock as a
  # non-progress cycle on eight mutants of CH4/dijkstra_progress.pml where pan
  # reported no error, and the G5 addendum suppressed the extension in the np_
  # product only (steps/g5-addendum-confirmation.md §1-§3). The two scenarios
  # below pin both halves of that distinction on one of those eight mutants, so
  # that a future change cannot quietly restore either behaviour, and the third
  # checks that the references teach the distinction rather than the old rule.

  Scenario: A blocked system is a deadlock, and the np_ search builds no cycle through it
    Then running "mcd check --no-timing --progress --sweep --promela" on the engine file "testdata/mutate/disagreements/CH4_dijkstra_progress/m002-invert-guard.pml" exits 0
    And the report has property "progress" with status "verified" and evidence "exhaustive" and complete true
    And that property has no counterexample
    And the report has property "deadlock" with status "violated" and evidence "exhaustive" and complete true
    And that property's counterexample has no loop

  Scenario: The stutter extension stays where G4 put it, for an ltl property on the same model
    Then running "mcd check --no-timing --promela" on the engine file "testdata/mutate/disagreements/CH4_dijkstra_progress/m002-invert-guard.pml" with the ltl formula "[]<> nempty(sema)" and fairness "none" exits 0
    And the report has property "ltl1" with status "violated" and evidence "exhaustive" and complete false
    And that property's counterexample has a loop starting at step 11 of 2 steps
    And the loop of that property contains a stutter step of process "-"

  Scenario: The references teach the difference between the two searches, not a rule with an exception
    Then "references/counterexamples.md" says that the np_ search does not extend a blocked state
    And "references/counterexamples.md" mentions each of:
      | `progress` (the `np_` search) |
      | self-loop                     |
    And "references/properties-ltl-ctl.md" says that a blocked system is a deadlock and not a non-progress cycle

  Scenario: Every run draws from its proctype's pool, so equal arguments do not split the state space
    Then running "mcd check --no-timing --sweep --promela" on the engine file "testdata/mutate/state-count/you_run2-equal-run-arguments.pml" exits 0
    And the report has property "deadlock" with status "verified" and evidence "exhaustive" and complete true
    And that property's counters show 12 states
    And "references/promela-subset.md" says that every run draws from its proctype's pool and a pid is reused

  # ------------------------------------ the subset table, re-derived from the binary
  # references/promela-subset.md §3 is a set of claims about what the parser
  # refuses. The probes under evals-workspace/subset-probes exercise one construct
  # each; these scenarios re-run a sample of them through the in-process CLI and
  # compare the outcome with what the table says, so the table cannot drift from
  # the engine without a red scenario. Evidence and the full 57-probe run:
  # steps/g3-evals3-subset-probe.md.

  Scenario Outline: The engine refuses what the subset table says it refuses
    Then running "mcd parse --promela" on the probe "<probe>" is rejected with kind "<kind>"
    And "references/promela-subset.md" lists "<construct>" as outside the subset

    Examples:
      | probe                    | kind           | construct |
      | out-unless               | outside-subset | `unless`  |
      | out-c-code               | outside-subset | `c_code`  |
      | out-eval-in-receive      | outside-subset | `eval(e)` |
      | out-priority             | outside-subset | `priority n` |
      | out-bit-operators        | outside-subset | bitwise   |
      | out-poll-receive         | outside-subset | poll      |
      | out-ltl-block            | outside-subset | `ltl name { … }` blocks |
      | out-include              | outside-subset | `#include` |
      | out-remote-variable      | syntax         | `P[i]:var` |

  Scenario Outline: The engine accepts what the subset table no longer calls a boundary
    Then running "mcd parse --promela" on the probe "<probe>" is accepted
    And "references/promela-subset.md" does not list "<construct>" as outside the subset

    Examples:
      | probe                  | construct   |
      | out-inline             | `inline`    |
      | out-typedef            | `typedef`   |
      | out-provided           | `provided`  |
      | out-chan-in-message    | channels carried in messages |
      | out-nr-pr              | `_nr_pr`    |
      | out-pc-value           | `pc_value`  |
      | in-never-claim         | `never`     |

  Scenario: The subset table no longer carries the warning that replaced the missing pass
    Then "references/promela-subset.md" does not warn that its table predates the G5 extension
    And "references/promela-subset.md" says that the rows were derived by probing the engine

  # ---------------------------------------------------------- graded runs
  Scenario: Every eval runnable from G4 or earlier has a graded run in iteration-3 in both configurations
    Then every eval in "evals/evals.json" with "runnable_from" at or before "G4" has a graded run under the workspace "evals-workspace/iteration-3" with "with_skill" and "without_skill"

  Scenario Outline: The staged criterion — the run with the skill passes every assertion, the baseline does not
    Given the evals workspace "evals-workspace/iteration-3/<dir>"
    Then "eval_metadata.json" names eval id <id> and the prompt of that eval in "evals/evals.json"
    And "with_skill/grading.json" has an "expectations" list where every entry has "text", "passed" and "evidence"
    And "without_skill/grading.json" has an "expectations" list where every entry has "text", "passed" and "evidence"
    And "with_skill/grading.json" grades every assertion of eval <id> in "evals/evals.json"
    And every entry of "with_skill/grading.json" has passed true
    And at least one entry of "without_skill/grading.json" has passed false
    And "timing.json" records tokens and duration for "with_skill" and "without_skill"
    And "with_skill/outputs/answer.md" exists in the workspace
    And "without_skill/outputs/answer.md" exists in the workspace

    Examples:
      | dir                          | id |
      | eval-1-mutex-flaw            | 1  |
      | eval-2-alternating-bit       | 2  |
      | eval-3-petri-hang            | 3  |
      | eval-4-telephone-busy        | 4  |
      | eval-5-c-code-boundary       | 5  |
      | eval-2b-starvation-loop      | 7  |

  Scenario: No graded run keeps a verbatim copy of a corpus file
    Given the evals workspace "evals-workspace/iteration-3"
    Then no file under the workspace is a copy of a file under "Promela - examples"

  # ------------------------------------------------------------ benchmark
  Scenario: The benchmark aggregates iteration-3 and lists the skill configuration before the baseline
    Given the evals workspace "evals-workspace/iteration-3"
    Then the workspace file "benchmark.json" parses as JSON
    And in the workspace file "benchmark.json" the "run_summary" keys begin with "with_skill" then "without_skill"
    And in the workspace file "benchmark.json" the runs of "with_skill" come before the runs of "without_skill"
    And every run in the workspace file "benchmark.json" has a "configuration" of "with_skill" or "without_skill" and a "result" with "pass_rate", "passed", "total", "time_seconds" and "tokens"
    And the workspace file "benchmark.json" lists "evals_run" equal to "1, 2, 3, 4, 5, 7"
    And "benchmark.md" exists in the workspace
    And "review.html" exists in the workspace

  # ---------------------------------------------------------- MCP session
  Scenario: The MCP path was exercised once by hand and the session file holds real tool output
    Then the plugin file "steps/g3-evals3-mcp-session.md" contains a json block with "outcome" equal to "ir"
    And the plugin file "steps/g3-evals3-mcp-session.md" contains a json block with "outcome" equal to "report"
    And the plugin file "steps/g3-evals3-mcp-session.md" contains a json block with "role" equal to "counterexample"
    And the plugin file "steps/g3-evals3-mcp-session.md" contains a json block that has the keys "prefix", "loop" and "loop_note"
    And the plugin file "steps/g3-evals3-mcp-session.md" mentions each of:
      | mc_parse   |
      | mc_check   |
      | mc_explain |
      | mutex_flaw |
      | tools/call |
