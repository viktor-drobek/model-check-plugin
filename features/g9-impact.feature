Feature: Trace-ledger change impact
  The impact workflow uses exact immutable identities only. It records review
  obligations; it does not establish implementation correctness.

  Scenario: invalidate evidence after an exact source revision change
    Given an impact dossier with complete exact dependencies
    When the source artifact revision changes
    Then the impact report marks evidence "evidence.mapping" stale
    And the impact report marks dossier "dossier.protocol" for renewal
    And the impact SARIF finding has rule "trace-impact/stale-evidence" at "src/protocol.go:12"

  Scenario: reject repeated evidence changes even when the first hash is unchanged
    Given an impact dossier with complete exact dependencies
    When one evidence ID has an unchanged hash followed by a changed hash
    Then impact analysis rejects the ambiguous evidence changes

  Scenario: preserve evidence for unchanged exact dependencies
    Given an impact dossier with complete exact dependencies
    When no artifact revision changes
    Then the impact report has no findings

  Scenario: keep an executed model verdict stable when its provenance is unchanged
    Given an impact dossier with an executed model verdict and exact provenance
    When no artifact revision changes
    Then the impact report has no findings

  Scenario: invalidate a model verdict when the engine build revision changes
    Given an impact dossier with an executed model verdict and exact provenance
    When the engine build revision changes
    Then the impact report marks verdict "verdict.protocol" stale
    And the impact report marks dossier "dossier.protocol" for renewal

  Scenario: invalidate a model verdict when the toolchain changes
    Given an impact dossier with an executed model verdict and exact provenance
    When the engine toolchain changes
    Then the impact report marks verdict "verdict.protocol" stale
    And the impact report marks dossier "dossier.protocol" for renewal

  Scenario: invalidate a model verdict when a dependent evidence hash changes
    Given an impact dossier with an executed model verdict and exact provenance
    When the evidence content hash changes
    Then the impact report marks evidence "evidence.mapping" stale
    And the impact report marks verdict "verdict.protocol" stale
    And the impact report marks dossier "dossier.protocol" for renewal

  Scenario: do not promote a model verdict whose review mapping is no longer approved
    Given an impact dossier with an executed model verdict and exact provenance
    When the review revision changes
    Then the impact report does not promote verdict "verdict.protocol"
    And the impact report marks dossier "dossier.protocol" for renewal

  Scenario: do not promote a verdict with a scope-mismatched review
    Given an impact dossier with an executed model verdict and exact provenance
    When the verdict review scope does not match
    Then the impact report marks verdict "verdict.protocol" stale
    And the impact report does not promote verdict "verdict.protocol"

  Scenario: do not promote a verdict with an incomplete dependency set
    Given an impact dossier with an executed model verdict and exact provenance
    When a verdict dependency is missing
    Then the impact report marks verdict "verdict.protocol" stale
    And the impact report does not promote verdict "verdict.protocol"

  Scenario: refuse an executed model verdict without engine provenance
    Given an impact dossier with an executed model verdict missing its build revision
    When the impact dossier is analyzed
    Then the impact analysis is rejected with "engine"

  Scenario: a stale model verdict never becomes an implementation pass
    Given an impact dossier with an executed model verdict and exact provenance
    When the engine build revision changes
    Then the impact report marks verdict "verdict.protocol" stale
    And the impact report carries no implementation pass

  Scenario: a whitespace-only reviewer decision is not applicable coverage
    Given an impact dossier with an executed model verdict and exact provenance
    When the reviewer decision ID is whitespace only
    Then the impact report marks verdict "verdict.protocol" stale
    And the impact report does not promote verdict "verdict.protocol"

  Scenario: a root source file keeps its SARIF line region
    Given an impact dossier with complete exact dependencies at "main.go:12"
    When the impact report is rendered as SARIF
    Then the impact SARIF finding has rule "trace-impact/orphan-evidence" at "main.go:12"
