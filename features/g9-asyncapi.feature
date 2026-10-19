Feature: G9 AsyncAPI declaration profile
  The offline adapter records direct AsyncAPI declarations as contract evidence.

  Scenario: AsyncAPI 2.6 direct channel message is declaration evidence
    Given a valid AsyncAPI 2.6 declaration document
    When I analyse the AsyncAPI declaration
    Then the AsyncAPI ledger contains only declaration containment facts
    And the AsyncAPI ledger has a declarations-only gap

  Scenario: AsyncAPI 3 operations do not infer transport behavior
    Given a valid AsyncAPI 3 declaration with an action
    When I analyse the AsyncAPI declaration
    Then the AsyncAPI ledger has an operation-unmapped gap
    And the AsyncAPI ledger contains no behavior edge

  Scenario Outline: AsyncAPI reference-only schema children do not duplicate gaps
    Given an AsyncAPI 3 declaration with a reference-only schema child "<keyword>"
    When I analyse the AsyncAPI declaration
    Then the AsyncAPI ledger has no duplicate gaps

    Examples:
      | keyword               |
      | items                 |
      | additionalProperties  |
      | not                   |

  Scenario: AsyncAPI rejects duplicate v2 operation IDs
    Given an AsyncAPI 2.6 declaration with duplicate operation IDs
    When I analyse the AsyncAPI declaration
    Then AsyncAPI analysis fails with rule "stage4.schema_invalid"
