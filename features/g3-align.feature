# G3 (alignment half) — plan 14 §9, §6, §8.2, §12 A4. Between the documentation
# half (steps/g3-docs-confirmation.md) and the evals-run half (waits for G1/G2).
# The skill package must describe the engine that exists after G0, not the
# plan's sketch, and the evals must be runnable the moment G1/G2 land.
#
# Everything here is checked against the engine itself: the schema is compared
# byte for byte, the fixture and the example net are parsed by the real Petri
# frontend, and the E3 fixture is run through the in-process CLI. Paths are
# relative to model-check-plugin/skills/model-check/ unless stated; engine paths
# are relative to model-check-plugin/engine/.
Feature: G3 skill package — alignment with the G0 engine

  Background:
    Given the skill directory "skills/model-check"

  # ----------------------------------------------------------------- schema
  Scenario: The skill's Petri-net schema is the engine's schema, byte for byte
    Then "assets/petri-net.schema.json" is byte-for-byte identical to the engine file "frontend/petri/schema.json"

  Scenario: The example net in the Petri reference is accepted by the engine's frontend
    Then every "json" code block in "references/petri-nets.md" that has a "places" key parses with the engine's Petri frontend

  # ------------------------------------------------------------ engine-tools
  Scenario: The engine-tools reference names the real CLI commands and flags
    Then "references/engine-tools.md" mentions each of:
      | mcd parse          |
      | mcd check          |
      | mcd version        |
      | --petri            |
      | --ir               |
      | --budget-states    |
      | --budget-depth     |
      | --budget-ms        |
      | --budget-mem-mb    |
      | --bfs              |
      | --no-timing        |
    And "references/engine-tools.md" describes exit codes 0, 1 and 2 each with a meaning
    And "references/engine-tools.md" says that the flag "--promela" arrives with "G1"
    And "references/engine-tools.md" says that the MCP layer arrives with "G2"
    And "references/engine-tools.md" says that until then the CLI is the only path

  Scenario: The engine-tools reference uses the report's real field names
    Then "references/engine-tools.md" mentions each of:
      | `status`           |
      | `evidence`         |
      | `complete`         |
      | `counters`         |
      | `counterexample`   |
      | `witness`          |
      | `reason`           |
      | `summary`          |
      | `final_state`      |
      | `memory_bytes_est` |
      | `time_ms`          |
      | `search`           |
    And "references/engine-tools.md" does not call the report fields illustrative

  # ------------------------------------------------------- rules and bounds
  Scenario Outline: Both status references encode the K1 status rules the engine enforces
    Then "<file>" states that violated carries evidence exhaustive
    And "<file>" states that verified requires complete true except for reach
    And "<file>" states that reach is verified by a witness
    And "<file>" states that budget exhaustion gives inconclusive naming the exhausted resource
    And "<file>" states that a construct outside the subset gives not-executed
    And "<file>" states that a domain overflow gives invalid-model
    And "<file>" lists the plan §6 aggregation priority in this order:
      | invalid-model |
      | not-executed  |
      | violated      |
      | inconclusive  |
      | unknown       |
      | verified      |

    Examples:
      | file                               |
      | references/engine-tools.md         |
      | references/evidence-and-status.md  |

  Scenario: The evidence reference carries the A4 size bounds and the bounded/unknown rule
    Then "references/evidence-and-status.md" has a size-bounds table with rows "small" and "medium"
    And the "small" row of that table contains "10⁵" for states and depth
    And the "medium" row of that table contains "10⁶", "128" and "60 s" and "1 GB"
    And "references/evidence-and-status.md" states that bounded is used when the engine can name the bound and unknown when it cannot

  # -------------------------------------------------------------- petri-nets
  Scenario: The Petri reference cites the G0 witness, the pan reconciliation and the fire(t) decision
    Then "references/petri-nets.md" mentions each of:
      | t1, t4                     |
      | p2=p5=1                    |
      | steps/g0-confirmation.md   |
      | +2                         |
      | pan                        |
      | G4                         |
    And "references/petri-nets.md" says that no fire atom is exposed by the G0 frontend
    And "references/petri-nets.md" uses the schema keys "name" and "weight" and not "id" or "multiplicity" in its JSON examples

  # ------------------------------------------------------------------- evals
  Scenario: evals.json uses assertions, one non-empty list per eval, each assertion machine-checkable
    Then "evals/evals.json" parses as JSON
    And every eval in "evals/evals.json" has a non-empty "assertions" list and no "expectations" key
    And every assertion in "evals/evals.json" has a "text" and a "check" whose "type" is one of:
      | regex            |
      | not_regex        |
      | regex_order      |
      | petri_json_valid |
      | json_field       |
    And every eval in "evals/evals.json" has a "runnable_from" that is one of:
      | G0 |
      | G1 |
      | G4 |
      | G5 |
    And the eval with id 3 has "runnable_from" equal to "G0"

  Scenario: The E3 fixture is the skill's own encoding of petrinet1 and the engine finds the hang in it
    Then "evals/fixtures/petrinet1.json" parses with the engine's Petri frontend
    And "evals/fixtures/petrinet1.json" is not a copy of any file under "Promela - examples"
    And running "mcd check --no-timing --petri" on "evals/fixtures/petrinet1.json" exits 0
    And the report has property "deadlock" with status "violated" and evidence "exhaustive" and complete true
    And that property's counterexample summary is "t1, t4" and its non-zero final state is "p2=1 p5=1"
    And the report has property "safe" with status "verified" and evidence "exhaustive" and complete true

  Scenario: The fixtures README lists current corpus hashes for the deferred evals
    Then every SHA-256 row in "evals/fixtures/README.md" matches the file it names, resolved from the repository root
    And "evals/fixtures/README.md" names each of:
      | Promela - examples/CH2/mutex_flaw.pml     |
      | Promela - examples/CH3/alternatingbit.pml |
      | Promela - examples/CH14/version1          |
      | Promela - examples/CH17/simple1.pr        |
      | evals/fixtures/petrinet1.json             |

  # --------------------------------------------------------- E3 graded run
  Scenario: E3 was graded for real: the run with the skill passes every assertion and the baseline does not
    Given the evals workspace "evals-workspace/iteration-1/eval-3-petri-hang"
    Then "with_skill/grading.json" has an "expectations" list where every entry has "text", "passed" and "evidence"
    And "without_skill/grading.json" has an "expectations" list where every entry has "text", "passed" and "evidence"
    And every entry of "with_skill/grading.json" has passed true
    And at least one entry of "without_skill/grading.json" has passed false
    And "timing.json" records tokens and duration for "with_skill" and "without_skill"
