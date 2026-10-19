Feature: Trace-ledger source adapters
  Source adapters produce syntactic observations without performing semantic analysis.

  Scenario: record an unambiguous direct Go call
    Given the Go source adapter fixture "source/go/direct-call.go"
    When the Go source adapter analyses the fixture
    Then the source trace ledger is accepted
    And the source trace ledger contains the SourceSymbol "caller"
    And the source trace ledger contains the syntactic call "caller" to "helper"
    And the source trace ledger has no gaps

  Scenario: preserve an unresolved selector call as a gap
    Given the Go source adapter fixture "source/go/selector-call.go"
    When the Go source adapter analyses the fixture
    Then the source trace ledger is accepted
    And the source trace ledger contains the SourceSymbol "caller"
    And the source trace ledger contains a gap with "not unambiguously resolved"
    And the source trace ledger does not contain a syntactic call "caller" to "Println"

  Scenario: preserve a dot-imported identifier call as a gap
    Given the Go source adapter fixture "source/go/dot-import.go"
    When the Go source adapter analyses the fixture
    Then the source trace ledger is accepted
    And the source trace ledger contains a gap with "not unambiguously resolved"
    And the source trace ledger does not contain a syntactic call "caller" to "Println"

  Scenario: preserve a generic type conversion as a gap
    Given the Go source adapter fixture "source/go/type-parameter.go"
    When the Go source adapter analyses the fixture
    Then the source trace ledger is accepted
    And the source trace ledger contains a gap with "not unambiguously resolved"
    And the source trace ledger does not contain a syntactic call "caller" to "T"

  Scenario: preserve a call inside a function literal as a gap
    Given the Go source adapter fixture "source/go/function-literal.go"
    When the Go source adapter analyses the fixture
    Then the source trace ledger is accepted
    And the source trace ledger contains a gap with "not unambiguously resolved"
    And the source trace ledger contains a gap for the call "unknown"

  Scenario: reject syntactically invalid Go source
    Given the Go source adapter fixture "source/go/invalid.go"
    When the Go source adapter analyses the fixture
    Then the source trace ledger is rejected with "parse Go source"

  Scenario Outline: register offline protocol artifacts without upgrading their authority
    Given the synthetic offline protocol case "<case>"
    When the offline protocol adapter consumes the supplied bytes
    Then the offline protocol output is deterministic schema "<schema>"
    And the offline protocol output retains the disclosure "<gap>"
    Examples:
      | case              | schema             | gap                                  |
      | tshark            | mcd-trace-ledger/1  | tshark.producer_unverified            |
      | tshark-unsupported| mcd-trace-ledger/1  | tshark.unsupported_dissector          |
      | tshark-omitted    | mcd-trace-ledger/1  | tshark.permitted_fields_omitted       |
      | tshark-truncated  | mcd-trace-ledger/1  | tshark.export_truncated               |
      | tshark-unknown    | mcd-trace-ledger/1  | tshark.export_status_unknown          |
      | kaitai            | mcd-trace-ledger/2  | grammar.anchor_unverified            |
      | spicy             | mcd-trace-ledger/2  | grammar.anchor_unverified            |
      | grammar-reference | mcd-trace-ledger/1  | grammar.reference_unresolved         |

  Scenario Outline: reject offline artifacts atomically with a stable non-secret rule
    Given the synthetic offline protocol case "<case>"
    When the offline protocol adapter consumes the supplied bytes
    Then the offline protocol output is absent with rule "stage3b.<rule>"
    Examples:
      | case                          | rule                   |
      | tshark-bad-digest              | digest_mismatch        |
      | tshark-bad-version             | metadata_invalid       |
      | tshark-bad-path                | metadata_invalid       |
      | tshark-secret-capture          | secret_indicator       |
      | tshark-unknown-field           | schema_invalid         |
      | tshark-wrong-layer             | schema_invalid         |
      | tshark-chain-mismatch          | schema_invalid         |
      | tshark-dns-overflow            | schema_invalid         |
      | tshark-unsupported-fields      | schema_invalid         |
      | tshark-number                 | schema_invalid         |
      | tshark-late-invalid            | schema_invalid         |
      | tshark-escaped-duplicate       | json_duplicate_name    |
      | tshark-malformed-duplicate     | json_syntax            |
      | tshark-trailing-duplicate      | json_trailing_data     |
      | tshark-trailing-depth          | json_trailing_data     |
      | tshark-duplicate-depth         | json_duplicate_name    |
      | tshark-depth-duplicate         | json_depth             |
      | tshark-empty                   | schema_invalid         |
      | tshark-only-frame              | schema_invalid         |
      | tshark-missing-layers          | schema_invalid         |
      | tshark-surrogate              | json_syntax            |
      | tshark-bom                    | utf8_invalid           |
      | tshark-oversized               | input_length           |
      | kaitai-bad-digest              | digest_mismatch        |
      | kaitai-imports                | grammar_source_invalid |
      | kaitai-marker                 | grammar_source_invalid |
      | kaitai-reverse-solidus        | grammar_source_invalid |
      | kaitai-crlf                   | grammar_source_invalid |
      | kaitai-bad-anchor             | metadata_invalid       |
      | kaitai-bad-path                | metadata_invalid       |
      | spicy-include                 | grammar_source_invalid |
      | spicy-comment-marker          | grammar_source_invalid |
      | grammar-reference-bad-digest   | digest_mismatch        |
      | grammar-reference-duplicate    | json_duplicate_name    |
      | grammar-reference-secret      | secret_indicator       |
      | grammar-reference-unknown      | schema_invalid         |
      | grammar-reference-bad-path     | metadata_invalid       |

  Scenario Outline: select the earliest byte across compound raw grammar violations
    Given the synthetic offline protocol case "<case>"
    When the offline protocol adapter consumes the supplied bytes
    Then the offline protocol output is absent with rule "stage3b.grammar_source_invalid"
    And the offline protocol rejection locator is "byte:<offset>"
    Examples:
      | case                     | offset |
      | kaitai-first-import      | 2      |
      | kaitai-first-marker      | 0      |
      | kaitai-first-backslash   | 1      |
      | kaitai-first-utf8        | 2      |
      | spicy-first-include      | 3      |
      | spicy-first-control      | 2      |
