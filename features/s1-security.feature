# S1 — the repository keeps its security checks.
#
# Scope of the words used below, fixed here so the scenarios cannot be read as
# claiming more than they check:
#
#   "the security gate" — scripts/security-scan.sh at the monorepo root, the
#   only place trivy, semgrep and govulncheck are invoked. CI
#   (.github/workflows/security.yml) calls the same script.
#
#   These scenarios check that the gate, its pinned scanner images, its
#   triage files, its pipeline wiring, its rule and its guide are present and
#   consistent. They do NOT run the scanners and say nothing about whether the
#   code is free of findings: a scan result is a report of `scripts/security-scan.sh`
#   (dist/security/summary.md), not of this suite.
#
#   The files live at the monorepo root, outside model-check-plugin/. A
#   standalone checkout of the public plugin repository does not contain them,
#   so these scenarios belong to the monorepo suite.

Feature: S1 Security checks stay in the project

  Scenario: The security gate runs trivy, semgrep and govulncheck
    Given the repository root
    Then the repository file "scripts/security-scan.sh" exists and is executable
    And the security gate scans with "trivy,semgrep,govulncheck" by default

  Scenario: The scanner images are pinned by tag and digest
    Given the repository root
    Then the security gate pins the image "TRIVY_IMAGE" by digest
    And the security gate pins the image "SEMGREP_IMAGE" by digest
    And the security gate pins the image "GO_IMAGE" by digest

  Scenario: The Go image of the gate matches the Go line of the module
    Given the repository root
    Then the security gate Go image has the same minor version as "model-check-plugin/engine/go.mod"

  Scenario Outline: The security gate refuses a bad setting instead of passing
    Given the repository root
    When the security gate runs with <variable> set to "<value>"
    Then the security gate exits with status 2
    And the security gate output mentions "<variable>"

    Examples:
      | variable         | value  |
      | SEC_FAIL_TRIVY   | bogus  |
      | SEC_FAIL_SEMGREP | bogus  |
      | SEC_SCANNERS     | bogus  |
      | SEC_DOCKER       | maybe  |

  Scenario: The pipeline calls the same script on pull requests and weekly
    Given the repository root
    Then the workflow ".github/workflows/security.yml" runs "scripts/security-scan.sh"
    And the workflow ".github/workflows/security.yml" is triggered by "pull_request"
    And the workflow ".github/workflows/security.yml" is triggered by "schedule"
    And the workflow ".github/workflows/security.yml" has a job timeout

  Scenario: Code scanning upload is gated and never runs for a fork
    Given the repository root
    Then the workflow ".github/workflows/security.yml" uploads SARIF only when "SECURITY_SARIF_UPLOAD" is "true"
    And the workflow ".github/workflows/security.yml" skips the SARIF upload for fork pull requests

  Scenario: Triage files exist and do not hide product code
    Given the repository root
    Then the repository file "trivy.yaml" exists
    And the repository file ".trivyignore.yaml" exists
    And the repository file ".semgrepignore" exists
    And "trivy.yaml" excludes none of "model-check-plugin" "engine" "cmd" "frontend" "skills"
    And ".semgrepignore" excludes none of "model-check-plugin" "engine" "cmd" "frontend" "skills"

  Scenario: The security rule is mirrored and indexed
    Given the repository root
    Then the repository file ".cursor/rules/security.mdc" exists
    And the repository file ".claude/rules/security.md" exists
    And the bodies of ".cursor/rules/security.mdc" and ".claude/rules/security.md" are identical
    And the repository file ".codex/rules.md" mentions "security.mdc"
    And the repository file "AGENTS.md" mentions "scripts/security-scan.sh"

  Scenario: The guide documents every gate setting
    Given the repository root
    Then "docs/security-scanning.md" documents every SEC_ setting of the security gate

  Scenario: The security gate gives its containers the proxy of the environment
    Given the repository root
    Then the repository file "scripts/security-scan.sh" mentions "HTTPS_PROXY"
    And the repository file "scripts/security-scan.sh" mentions "--network host"
    And the repository file "scripts/security-scan.sh" mentions "SEC_DOCKER_ARGS"

  Scenario: A repository without code scanning does not fail the security job
    Given the repository root
    Then the workflow ".github/workflows/security.yml" lets every SARIF upload fail without failing the job
    And the workflow ".github/workflows/ci.yml" asks SPIN for its version with -V
