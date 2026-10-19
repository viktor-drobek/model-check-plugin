Feature: Deliberately incomplete end-to-end trace corpus
  The same offline bytes retain their evidence authority and unresolved obligations.

  Scenario: Contract source and runtime observations do not imply conformance
    Given the synthetic incomplete trace corpus
    When the corpus is imported twice through its adapters
    Then the corpus imports are byte reproducible
    And contract syntactic and runtime evidence remain separate
    And the corpus disagreement remains an explicit SARIF obligation
    And the corpus dossier retains the disagreement without an implementation claim

  Scenario: Only an exact review decision applies to the corpus
    Given the synthetic incomplete trace corpus
    When the corpus is imported twice through its adapters
    Then unrelated revision and scope decisions cannot resolve the corpus review
    And an exact corpus review is recorded without erasing evidence or claiming conformance
