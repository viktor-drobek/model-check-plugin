Feature: G9 bounded Protobuf and Buf declaration profiles
  The Stage 4 declaration adapters are offline, bounded, and do not invoke tools.

  # Parser boundary regressions (identifier grammar, decimal-only tags,
  # unsupported map/options punctuation, and budget locators) are exercised by
  # the focused protobuf package tests; this feature remains limited to the
  # existing step vocabulary.

  Scenario: Protobuf declarations yield only contract facts
    Given the protobuf contract case "valid"
    When the protobuf contract adapter analyses the artifact
    Then the protobuf contract output is a deterministic ledger
    And the protobuf contract output has 2 protocol messages
    And the protobuf contract output has 3 binary fields
    And the protobuf contract output has 1 endpoint
    And the protobuf contract output has a gap of kind "protobuf.declarations_only"

  Scenario: Unsupported Protobuf imports fail closed
    Given the protobuf contract case "import"
    When the protobuf contract adapter analyses the artifact
  Then the protobuf contract output is absent with rule "stage4.unsupported_declaration"

  Scenario: Protobuf rejects colliding default JSON field names
    Given the protobuf contract case "json-name-collision"
    When the protobuf contract adapter analyses the artifact
    Then the protobuf contract output is absent with rule "stage4.schema_invalid"

  Scenario Outline: Protobuf rejects every effective JSON-name collision
    Given the protobuf contract case "<case>"
    When the protobuf contract adapter analyses the artifact
    Then the protobuf contract output is absent with rule "stage4.schema_invalid"

    Examples:
      | case                         |
      | json-name-trailing-underscore |
      | json-name-consecutive-underscore |

  Scenario Outline: Protobuf rejects scalar vocabulary as declarations
    Given the protobuf contract case "<case>"
    When the protobuf contract adapter analyses the artifact
    Then the protobuf contract output is absent with rule "stage4.schema_invalid"

    Examples:
      | case              |
      | scalar-message    |
      | scalar-service    |

  Scenario: Protobuf accepts spaced qualified package names
    Given the protobuf contract case "spaced-package"
    When the protobuf contract adapter analyses the artifact
    Then the protobuf contract output is a deterministic ledger
    And the protobuf contract output has 1 protocol messages
    And the protobuf contract output has 1 binary fields

  Scenario: Buf modules are unresolved declaration references
    Given the protobuf contract case "buf-valid"
    When the protobuf contract adapter analyses the artifact
    Then the protobuf contract output is a deterministic ledger
    And the protobuf contract output has 2 gaps of kind "buf.module_unresolved"
    And the protobuf contract output has a gap of kind "buf.declarations_only"

  Scenario: Buf dependencies are not fetched
    Given the protobuf contract case "buf-dependency"
    When the protobuf contract adapter analyses the artifact
    Then the protobuf contract output is absent with rule "stage4.unsupported_declaration"
