# S2 — the repository keeps its rule about resources.
#
# The rule says: look at CPU, memory and disk before a run and wait while the
# machine is busy. These scenarios check that the rule, its mirror, its index
# entries and the plugin's script and instructions are present and consistent.
# They do not measure the machine (the capacity script's own contract is in
# g3-skill-package.feature). The files live at the monorepo root, outside
# model-check-plugin/, so these scenarios belong to the monorepo suite.

Feature: S2 The resources rule stays in the project

  Scenario: The rule is mirrored and indexed
    Given the repository root
    Then the repository file ".cursor/rules/resources.mdc" exists
    And the repository file ".claude/rules/resources.md" exists
    And the bodies of ".cursor/rules/resources.mdc" and ".claude/rules/resources.md" are identical
    And the repository file ".codex/rules.md" mentions "resources.mdc"
    And the repository file "AGENTS.md" mentions "wait-for-capacity.sh"

  Scenario: The rule names the limit and the script the plugin ships
    Given the repository root
    Then the repository file ".cursor/rules/resources.mdc" mentions "90%"
    And the repository file ".cursor/rules/resources.mdc" mentions "wait-for-capacity.sh"
    And the repository file "model-check-plugin/AGENTS.md" mentions "wait-for-capacity.sh"
    And the repository file "model-check-plugin/README.md" mentions "Running on a shared machine"
    And the repository file "model-check-plugin/skills/model-check/assets/wait-for-capacity.sh" exists and is executable
