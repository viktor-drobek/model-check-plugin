# S4 — the plugin tree carries its own security checks.
#
# The plugin tree is what a release deploys to the public plugin repository, and a deploy replaces that repository's content with the tree:
# a check that lived only beside the tree would be erased by the next release. The scanner script, its configuration, the workflow and the guide
# are therefore part of the tree, and the script is the one of the monorepo (the same bytes: it finds the module directory itself).

Feature: S4 The plugin tree carries its security checks

  Scenario: The scan script, its configuration and its guide are in the plugin tree
    Given the repository root
    Then the repository file "model-check-plugin/scripts/security-scan.sh" exists and is executable
    And the repository file "model-check-plugin/trivy.yaml" exists
    And the repository file "model-check-plugin/.trivyignore.yaml" exists
    And the repository file "model-check-plugin/.semgrepignore" exists
    And the repository file "model-check-plugin/docs/security-scanning.md" documents every SEC_ setting of the plugin's scan script
    And "model-check-plugin/scripts/security-scan.sh" is the same file as "scripts/security-scan.sh"

  Scenario: The workflow of the plugin tree runs the script, on pull requests and weekly, and uploads to code scanning
    Given the repository root
    Then the workflow "model-check-plugin/.github/workflows/security.yml" runs "scripts/security-scan.sh"
    And the workflow "model-check-plugin/.github/workflows/security.yml" is triggered by "pull_request"
    And the workflow "model-check-plugin/.github/workflows/security.yml" is triggered by "schedule"
    And the workflow "model-check-plugin/.github/workflows/security.yml" has a job timeout
    And the workflow "model-check-plugin/.github/workflows/security.yml" skips the SARIF upload for fork pull requests
    And the repository file "model-check-plugin/.github/workflows/security.yml" mentions "security-events: write"
    And the repository file "model-check-plugin/.github/workflows/security.yml" mentions "actions: read"
