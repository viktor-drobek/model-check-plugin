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
