Feature: Trace ledger contract
  The multi-language analysis roadmap starts with a reproducible ledger rather
  than an implicit claim that a model verdict proves an implementation.

  Scenario: preserve evidence classes and unresolved obligations
    Given the trace ledger fixture "trace/valid-ledger.json"
    Then the trace ledger is accepted
    And the trace ledger keeps evidence level "syntactic" distinct from "verified"
    And the trace report names the gap "gap.timeout"
    And the trace report states that a model verdict is not transferred to an implementation claim

  Scenario: reject a dangling relation instead of guessing its target
    Given the trace ledger fixture "trace/dangling-edge.json"
    Then the trace ledger is rejected with "does not name a node"

  Scenario: reject evidence detached from its subject
    Given the trace ledger fixture "trace/orphan-evidence.json"
    Then the trace ledger is rejected with "is not listed by its subject"

  Scenario: reject case-variant JSON field names
    Given the trace ledger fixture "trace/case-variant-field.json"
    Then the trace ledger is rejected with "unknown field"

  Scenario: retain the finite-model boundary for verified evidence
    Given the trace ledger fixture "trace/source-verified.json"
    Then the trace ledger is rejected with "must apply only to a formal-model artifact"

  Scenario: reject a schema-invalid provenance field even when another pin is present
    Given the trace ledger fixture "trace/empty-content-sha.json"
    Then the trace ledger is rejected with "content_sha256 must be a lowercase SHA-256"

  Scenario: preserve a URL gap reference without inventing a line number
    Given the trace ledger fixture "trace/url-gap.json"
    Then the trace ledger is accepted
    And the trace SARIF report preserves the URL reference "https://example.org:443"
