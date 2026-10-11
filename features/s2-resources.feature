# S2 — the repository keeps its rule about resources.
#
# The rule says: look at CPU, memory and disk before a run and wait while the
# machine is busy. These scenarios check that the rule, its mirror, its index
# entries and the plugin's script and instructions are present and consistent,
# and that the engine's own test harness enforces the disk part of the rule:
# it refuses to start into a scratch directory (TMPDIR) that cannot hold the
# temporary files of SPIN, gcc and go test. They do not measure the machine
# (the capacity script's own contract is in g3-skill-package.feature); the
# harness scenarios observe the test binary through its exit code and message.
# The rule files live at the monorepo root, outside model-check-plugin/, so
# these scenarios belong to the monorepo suite.

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

  # A tmpfs with a user quota reports gigabytes free to df and statfs and still
  # refuses every write: the full suite failed half-way with "Disk quota
  # exceeded" from gcc. The harness therefore probes TMPDIR by writing
  # MCD_TMP_MIN_MB megabytes before the first test, and says what to do. A
  # quota cannot be produced here; the request below is refused on the free-
  # space check before a byte is written, which exercises the refusal, the
  # exit code and the message, not the write itself (the unit tests of
  # internal/tmpspace cover the write and the words for a quota).
  Scenario: The test harness refuses to start into a scratch directory without room
    When the engine test binary starts with MCD_TMP_MIN_MB above any free space
    Then the test binary exits with 2 before any test runs
    And its message names the scratch directory, the amount asked and "TMPDIR"

  Scenario: The test harness starts when the probe is switched off
    When the engine test binary starts with MCD_TMP_MIN_MB "0"
    Then the test binary exits with 0

  Scenario: The rule, the instructions and CI name the scratch directory
    Given the repository root
    Then the repository file ".cursor/rules/resources.mdc" mentions "TMPDIR"
    And the repository file ".cursor/rules/resources.mdc" mentions "GOTMPDIR"
    And the repository file ".cursor/rules/resources.mdc" mentions "MCD_TMP_MIN_MB"
    And the repository file "model-check-plugin/README.md" mentions "MCD_TMP_MIN_MB"
    And the repository file "model-check-plugin/README.md" mentions "GOTMPDIR"
    And the repository file "model-check-plugin/AGENTS.md" mentions "TMPDIR"
    And the repository file ".cursor/rules/architecture.mdc" mentions "internal/tmpspace"
    And the repository file ".claude/rules/architecture.md" mentions "internal/tmpspace"
    And the repository file ".github/workflows/ci.yml" mentions "TMPDIR"
