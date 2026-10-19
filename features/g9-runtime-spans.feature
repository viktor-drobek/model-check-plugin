@runtime_internal
Feature: Runtime span export records
  The runtime adapter projects a pinned offline span export into the trace
  ledger. It records caller assertions and their provenance without inferring
  communication edges or verification claims.

  Scenario: project a single runtime span with no producer metadata
    Given the synthetic runtime span case "runtime"
    When the runtime span adapter analyses the export
    Then the runtime span output is a deterministic ledger
    And the runtime span output has 1 runtime span node
    And the runtime span output has 2 gaps of kind "span.provenance_missing"
    And the runtime span output has a gap with reason "producer build revision was not supplied"
    And the runtime span output has a gap with reason "collector name and version were not supplied"
    And the runtime span output contains an anchor with "occurrences=1"
    And the runtime span output has no span.parent_unmapped gap

  Scenario: project a span carrying the immutable build revision
    Given the synthetic runtime span case "runtime-with-build"
    When the runtime span adapter analyses the export
    Then the runtime span output is a deterministic ledger
    And the runtime span output has 1 runtime span node
    And the runtime span output has 1 gap of kind "span.provenance_missing"
    And the runtime span output has a gap with reason "collector name and version were not supplied"
    And the runtime span output records the source revision

  Scenario: project a span carrying the collector identity
    Given the synthetic runtime span case "runtime-with-collector"
    When the runtime span adapter analyses the export
    Then the runtime span output is a deterministic ledger
    And the runtime span output has 1 runtime span node
    And the runtime span output has 1 gap of kind "span.provenance_missing"
    And the runtime span output has a gap with reason "producer build revision was not supplied"

  Scenario: project a span with complete producer metadata
    Given the synthetic runtime span case "runtime-full"
    When the runtime span adapter analyses the export
    Then the runtime span output is a deterministic ledger
    And the runtime span output has 1 runtime span node
    And the runtime span output has no gaps

  Scenario: record a parent identity without inferring a relation
    Given the synthetic runtime span case "runtime-parent"
    When the runtime span adapter analyses the export
    Then the runtime span output is a deterministic ledger
    And the runtime span output has 1 runtime span node
    And the runtime span output has 2 gaps of kind "span.provenance_missing"
    And the runtime span output has 1 gap of kind "span.parent_unmapped"
    And the runtime span output has a gap with reason "parent identity was recorded without inferring a communication relation"

  Scenario: fold exact duplicates into one node with an occurrence count
    Given the synthetic runtime span case "runtime-dedup"
    When the runtime span adapter analyses the export
    Then the runtime span output is a deterministic ledger
    And the runtime span output has 1 runtime span node
    And the runtime span output contains an anchor with "occurrences=2"
    And the runtime span output has 2 gaps of kind "span.provenance_missing"

  Scenario: preserve output across a nontrivial permutation with duplicate spans
    Given the synthetic runtime span case "runtime-distinct-occurrence-group"
    When the runtime span adapter analyses the export
    Then the runtime span output is unchanged after reversing the export entries
    And the runtime span output has 2 runtime span node
    And the runtime span output contains an anchor with "occurrences=2"

  Scenario: keep the same trace with different bytes as distinct nodes
    Given the synthetic runtime span case "runtime-same-trace-different-bytes"
    When the runtime span adapter analyses the export
    Then the runtime span output is a deterministic ledger
    And the runtime span output has 2 runtime span node

  Scenario: validate every envelope before decoding an earlier raw span
    Given the synthetic runtime span case "runtime-envelope-precedence"
    When the runtime span adapter analyses the export
    Then the runtime span output is absent with rule "stage4.metadata_invalid"

  Scenario Outline: reject runtime span exports atomically with a stable non-secret rule
    Given the synthetic runtime span case "<case>"
    When the runtime span adapter analyses the export
    Then the runtime span output is absent with rule "stage4.<rule>"
    Examples:
      | case                                | rule                |
      | runtime-bad-export-digest           | digest_mismatch     |
      | runtime-raw-digest-mismatch         | digest_mismatch     |
      | runtime-raw-digest-late-duplicate   | digest_mismatch     |
      | runtime-identity-ambiguous          | identity_ambiguous  |
      | runtime-dedup-conflict              | identity_ambiguous  |
      | runtime-span-limit                  | span_limit          |
      | runtime-oversized-export            | input_length        |
      | runtime-oversized-raw               | input_length        |
      | runtime-bad-path                    | metadata_invalid    |
      | runtime-scalar-bound                | schema_invalid      |
      | runtime-bad-build                   | metadata_invalid    |
      | runtime-partial-collector           | metadata_invalid    |
      | runtime-secret-name                 | secret_indicator    |
      | runtime-unknown-field               | schema_invalid      |
      | runtime-entry-extra-field           | schema_invalid      |
      | runtime-extra-export-field          | schema_invalid      |
      | runtime-bad-schema                  | schema_invalid      |
      | runtime-bad-export-schema           | schema_invalid      |
      | runtime-bad-kind                    | schema_invalid      |
      | runtime-bad-status                  | schema_invalid      |
      | runtime-number-bound                | schema_invalid      |
      | runtime-negative-number             | schema_invalid      |
      | runtime-end-before-start            | schema_invalid      |
      | runtime-malformed-utf8              | utf8_invalid        |
      | runtime-bom                         | utf8_invalid        |
      | runtime-surrogate                   | json_syntax         |
      | runtime-duplicate-key               | json_duplicate_name |
      | runtime-trailing                    | json_trailing_data  |
      | runtime-depth                       | json_depth          |

  Scenario: reject a runtime span export that exceeds the local record budget
    Given the synthetic runtime span case "runtime"
    When the runtime span adapter analyses the export under the reduced budget
    Then the runtime span output is absent with rule "stage4.preflight_limit"
    And a later runtime span call admits again

  Scenario: reject a runtime span export while the admission slot is occupied
    Given the synthetic runtime span case "runtime"
    When the runtime span adapter holds the admission slot
    Then the runtime span output is absent with rule "stage4.resource_admission_busy"

  Scenario: report contention in the admission test helper without blocking
    Given the synthetic runtime span case "runtime"
    Then a second runtime admission holder reports contention promptly
    And a later runtime span call admits again
