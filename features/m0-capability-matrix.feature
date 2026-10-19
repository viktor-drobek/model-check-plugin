# M0 is the P0 baseline of the issue #11 roadmap. These scenarios constrain
# documentation and its conformance checks only; they do not add runtime statuses
# before the M3.0 backend-contract milestone.
Feature: M0 capability and semantics baseline

  Background:
    Given the issue 11 capability matrix is loaded

  Scenario: The public matrix is a complete versioned baseline
    Then the matrix schema is "model-check-capability-matrix/1"
    And every matrix entry has an identifier, category, status, semantics version, and limits
    And every matrix entry cites an exercised feature fixture
    And the matrix includes frontend, property, backend, reduction, evidence, and model-class categories

  Scenario: Matrix surface identifiers resolve to the shipped product
    Then every matrix CLI surface names a documented mcd command or check flag
    And every matrix MCP surface is one of the seven registered MCP tools
    And every matrix manifest surface is declared by the shipped plugin manifests
    And every supported matrix entry cites an exercised feature fixture

  Scenario: Non-default and delegated capabilities carry executable evidence
    Then every supported, experimental, or delegated-integrated entry cites exercised feature evidence
    And every experimental entry declares default-off CLI, MCP, and report markers
    And every delegated-integrated entry identifies its compliant backend and fixture evidence

  Scenario: Unsupported capabilities preserve non-verdict behavior
    Then every unsupported matrix entry with an exposed surface cites an exercised negative feature fixture
    And the matrix does not classify a runtime result status as a capability status

  Scenario: Boundary-only unsupported capabilities have no executable product surface
    Then every boundary-only unsupported matrix entry has no product surface

  Scenario: Public documentation links to the canonical matrix
    Then the README, skill workflow, and CLI and MCP reference link to the capability matrix

  Scenario: The reconciliation table keeps owner decisions explicit
    Then the issue 11 reconciliation table maps every roadmap obligation exactly once
    And the arithmetic alternative selection is recorded as pending owner decision
    And no reconciliation row is deferred or out of scope without a tracker-owner decision link
