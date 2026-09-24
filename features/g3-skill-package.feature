# G3 (documentation half) — plan 14 §9, §7. Exit criterion of G3 in the plan is
# "E1/E3/E5 pass assertions; baseline without skill does not". That half needs a
# running engine and MCP server (G0–G2) and is deferred; the scenarios below fix
# the machine-checkable shape of the skill package that does not depend on the
# engine: SKILL.md, references/, assets/, evals/evals.json. The alignment half
# (features/g3-align.feature) later replaced the sketch schema by a copy of the
# engine's and filled the assertions; the three scenarios marked "(aligned)" were
# rewritten there and then, the rest is unchanged.
#
# Paths are relative to model-check-plugin/skills/model-check/ unless stated.
# Notes are in ../model-check-skill-notes/ relative to model-check-plugin/.
Feature: G3 skill package — documentation half

  Background:
    Given the skill directory "skills/model-check"

  # ---------------------------------------------------------------- SKILL.md
  Scenario: SKILL.md has the required frontmatter
    Then the file "SKILL.md" exists
    And the frontmatter field "name" equals "model-check"
    And the frontmatter field "description" is non-empty

  Scenario: The description carries Russian and English trigger phrases
    Then the frontmatter field "description" contains each of:
      | проверь модель        |
      | может ли зависнуть    |
      | докажи, что никогда   |
      | сеть Петри            |
      | тупик                 |
      | инвариант             |
      | liveness              |
      | Promela               |
      | LTL                   |
      | CTL                   |
      | model checking        |
      | verify this protocol  |
      | deadlock              |
      | counterexample        |
    And the frontmatter field "description" mentions the built-in engine

  Scenario: SKILL.md body stays within the progressive-disclosure budget
    Then the body of "SKILL.md" has at most 500 lines

  Scenario: Every file referenced from SKILL.md exists
    Then every relative path referenced from "SKILL.md" exists in the skill directory
    And "SKILL.md" references each of:
      | references/workflow.md             |
      | references/model-classification.md |
      | references/properties-ltl-ctl.md   |
      | references/fairness.md             |
      | references/promela-subset.md       |
      | references/petri-nets.md           |
      | references/engine-tools.md         |
      | references/counterexamples.md      |
      | references/evidence-and-status.md  |
      | references/pitfalls.md             |
      | assets/report-template.md          |
      | assets/intake-card.yaml            |
      | assets/petri-net.schema.json       |

  # ------------------------------------------------------------ traceability
  Scenario: Every FR/NFR id mentioned in the skill exists in the requirements note
    Then every "FR-" or "NFR-" identifier used in the skill package exists in "model-check-skill-notes/11-skill-requirements.md"

  Scenario: Every reference file cites its source notes at the top
    Then every file under "references" names its source notes within its first 12 lines

  # ---------------------------------------------------------------- statuses
  Scenario: Status vocabulary is the six words of 11 §14 — no status-labelled token or misspelling outside it
    Then "references/evidence-and-status.md" lists exactly these statuses:
      | verified      |
      | violated      |
      | inconclusive  |
      | unknown       |
      | not-executed  |
      | invalid-model |
    And no file in the skill package uses a status token outside that list
    And no file in the skill package contains a misspelt status such as "not executed", "not_executed", "invalid_model" or "proved" as a status

  Scenario: Evidence levels are the four the engine produces
    Then "references/evidence-and-status.md" lists exactly these evidence levels:
      | exhaustive  |
      | bounded     |
      | approximate |
      | unknown     |
    And "references/evidence-and-status.md" states that statistical evidence is not produced
    And "references/evidence-and-status.md" has a section of forbidden phrasings that includes "no errors" and "proved"

  Scenario: Fairness reference declares strong fairness unsupported
    Then "references/fairness.md" states that strong fairness is not supported by the engine
    And "references/fairness.md" states that weak fairness is supported

  # -------------------------------------------------------------- structure
  Scenario: Long reference files have a table of contents
    Then every file under "references" with more than 300 lines has a table of contents in its first 40 lines

  Scenario: Engine tools reference names all seven MCP tools and the CLI fallback
    Then "references/engine-tools.md" mentions each of:
      | mc_parse         |
      | mc_simulate      |
      | mc_check         |
      | mc_explain       |
      | mc_lint_property |
      | mc_estimate      |
      | mc_manifest      |
      | mcd              |

  Scenario: Properties reference contains the LTL/CTL comparison table
    Then "references/properties-ltl-ctl.md" contains a markdown table whose header includes "LTL" and "CTL"
    And "references/properties-ltl-ctl.md" mentions each of:
      | AG EF  |
      | <>[]   |
      | []<>   |

  Scenario: Petri-net reference carries the plan §2.2 content
    Then "references/petri-nets.md" mentions each of:
      | hang          |
      | live          |
      | safe          |
      | AG EF fire    |
      | []<> fire     |
      | inhibitor     |
      | petrinet1     |
      | t1            |
      | t4            |

  Scenario: Pitfalls reference illustrates every anti-pattern with a corpus path
    Then "references/pitfalls.md" has at least 17 anti-pattern entries
    And every anti-pattern entry in "references/pitfalls.md" names a corpus path or says "no corpus model"

  # ----------------------------------------------------------------- assets
  Scenario: Report template follows the 11 §14 section order
    Then "assets/report-template.md" has these headings in this order:
      | Summary          |
      | Scope            |
      | Model            |
      | Properties       |
      | Method           |
      | Execution        |
      | Result           |
      | Counterexample   |
      | Limitations      |
      | Next actions     |
      | Artifacts        |

  Scenario: Intake card has the FR-001 fields
    Then "assets/intake-card.yaml" has top-level keys:
      | system      |
      | state       |
      | transitions |
      | properties  |
      | assumptions |
      | fairness    |
      | budget      |
      | evidence    |

  Scenario: Petri-net JSON schema is valid and admits inhibitor arcs only to reject them (aligned)
    Then "assets/petri-net.schema.json" parses as JSON
    And the schema declares "$schema" as JSON Schema draft 2020-12
    And the schema defines "places" items with an integer "initial" and an optional integer "capacity" defaulting to 255
    And the schema defines "transitions" items with "inputs" and "outputs" arcs whose "weight" has minimum 1
    And every object in the schema sets "additionalProperties" to false
    And the schema's only key mentioning "inhibitor" is the arc flag whose description says the engine rejects it

  # ------------------------------------------------------------------ evals
  Scenario: evals.json holds the six plan §8.2 prompts with assertions (aligned)
    Then "evals/evals.json" parses as JSON
    And "evals/evals.json" has "skill_name" equal to "model-check"
    And "evals/evals.json" has exactly 6 evals with ids 1 to 6
    And every eval in "evals/evals.json" has a non-empty "prompt" and a non-empty "assertions" list
    And "evals/fixtures/README.md" exists
    And "evals/fixtures/README.md" explains that fixtures reference corpus paths and hashes instead of copying files
    And no file under "evals/fixtures" is a copy of a file under "Promela - examples"
