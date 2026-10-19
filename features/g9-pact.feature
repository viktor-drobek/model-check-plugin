Feature: Pact v4 declaration profile
  The offline Pact adapter records only directly declared contract shape.

  Scenario: an HTTP interaction is contract evidence, not verification
    Given a valid Pact v4 HTTP declaration
    When the Pact declaration is adapted twice
    Then the Pact ledgers are byte reproducible
    And the Pact ledger contains one endpoint and two direct messages
    And all Pact evidence is contract only

  Scenario: a recognized message interaction is gap-only
    Given a valid Pact v4 message declaration
    When the Pact declaration is adapted
    Then the Pact ledger contains no inferred message facts
    And the Pact ledger discloses the unmapped message declaration

  Scenario: unsafe Pact declarations fail without an output ledger
    Given a Pact declaration with an unsafe route
    When the Pact declaration is adapted
    Then the Pact declaration is rejected with "schema_invalid"
    And the Pact adapter returns no partial ledger
