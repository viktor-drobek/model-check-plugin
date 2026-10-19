Feature: Conformance dossier hand-off
  A dossier records only a finite, declared model hand-off. It does not infer
  a source-program correctness conclusion.

  Scenario: accept a fully specified finite-model hand-off
    Given a complete conformance dossier intake
    When the conformance dossier is built
    Then the conformance dossier is accepted
    And the dossier contains the applicable reviewer decision "review.r1"
    And the dossier links requirements, trace nodes, transitions, properties, and assumptions
    And the dossier states that a model result is finite-model-only

  Scenario: reject invalid UTF-8 in free text before dossier derivation
    Given a complete conformance dossier intake with invalid UTF-8 free text
    When the conformance dossier is built
    Then the conformance dossier is rejected with "invalid UTF-8"

  Scenario: reject an underspecified model hand-off
    Given an underspecified conformance dossier intake without state bounds
    When the conformance dossier is built
    Then the conformance dossier is rejected with "state_bounds"

  Scenario: retain a mismatched reviewer decision as an explicit gap
    Given a complete conformance dossier intake with a reviewer decision for another revision
    When the conformance dossier is built
    Then the conformance dossier is accepted
    And the dossier names the gap "gap.review.review.r1.target_mismatch"
    And the dossier names a gap of kind "review_missing"
    And the dossier does not contain the reviewer decision "review.r1"

  Scenario: retain valid boundary-derived identifiers
    Given a complete conformance dossier intake with a maximum-length requirement ID
    When the conformance dossier is built
    Then the conformance dossier is accepted
    And all generated dossier identifiers are bounded

  Scenario: completed dossier retains every reviewer mismatch obligation
    Given a complete conformance dossier intake with a reviewer decision for another revision
    When the conformance dossier is built
    And the completed dossier loses its reviewer mismatch gaps
    Then validating the dossier is rejected with "missing derived review_target_mismatch gap"

  Scenario: removing an applicable decision cannot fabricate mismatch provenance
    Given a complete conformance dossier intake without declared links
    When the conformance dossier is built
    And the completed dossier replaces its applicable decision with a mismatch gap
    Then validating the dossier is rejected with "review provenance"

  Scenario: mutating an applicable decision cannot change review provenance
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed dossier mutates its applicable reviewer decision
    Then validating the dossier is rejected with "review provenance"

  Scenario: oversized non-applicable provenance is rejected before derivation
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed dossier has oversized non-applicable provenance
    Then validating the dossier is rejected with "review provenance exceeds"

  Scenario: duplicate non-applicable provenance is rejected before derivation
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed dossier has duplicate non-applicable provenance
    Then validating the dossier is rejected with "review provenance.non_applicable_ids contains duplicate"

  Scenario: malformed non-applicable provenance is rejected before derivation
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed dossier has malformed non-applicable provenance
    Then validating the dossier is rejected with "review provenance"

  Scenario: combined reviewer and provenance populations stay bounded
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed dossier has combined reviewer provenance populations over the cap
    Then validating the dossier is rejected with "review provenance population exceeds"

  Scenario: reviewer decision fields are admitted before provenance hashing
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed dossier has an oversized reviewer decision field
    Then validating the dossier is rejected with "reviewer_decisions[0].scope_ids exceeds"

  Scenario Outline: removing one generated obligation is rejected
    Given a complete conformance dossier intake without declared links
    When the conformance dossier is built
    And the completed dossier loses the generated gap of kind "<kind>"
    Then validating the dossier is rejected with "<message>"

    Examples:
      | kind                      | message                                     |
      | requirement_trace_missing | missing requirement_trace relation or gap   |
      | relation_missing          | missing derived relation_missing gap        |

  Scenario: deduplicate coverage endpoints but retain relation records
    Given a complete conformance dossier intake with duplicate requirement-trace endpoints
    When the conformance dossier is built
    Then the conformance dossier is accepted
    And requirement coverage has one related trace node

  Scenario: not-executed does not imply a result
    Given a complete conformance dossier intake with status "not-executed"
    When the conformance dossier is built
    Then the conformance dossier is accepted
    And the dossier has status "not-executed" without a model verdict

  Scenario: missing declared relations stay gaps rather than invented links
    Given a complete conformance dossier intake without declared links
    When the conformance dossier is built
    Then the conformance dossier is accepted
    And the dossier names the gap "gap.requirement_trace.req.delivery.missing"
    And the dossier has no fabricated relation links

  Scenario: canonical dossier output repeats exactly
    Given a complete conformance dossier intake
    When the conformance dossier is built twice
    Then the canonical conformance dossier JSON is identical

  Scenario: completed dossier retains the review-target obligation
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed dossier loses its review-target link
    Then validating the dossier is rejected with "review_target"

  Scenario: review-target mismatch references a reviewer decision
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed conformance dossier gets an invalid generated gap reference of kind "review_target_mismatch"
    Then validating the dossier is rejected with "unknown reviewer decision"

  Scenario: derived review-gap identifiers cannot collide with caller identifiers
    Given a conformance dossier intake with colliding derived review-gap identifiers
    When the conformance dossier is built
    Then the conformance dossier is accepted
    And the dossier contains two review-target mismatch gaps

  Scenario: review-gap identifiers are stable under reviewer input reorder
    Given a conformance dossier intake with colliding derived review-gap identifiers
    When the conformance dossier is built twice with reversed reviewer decision order
    Then the canonical conformance dossier JSON is identical

  Scenario: coverage identifiers remain unique across literal and hashed requirements
    Given a conformance dossier intake with colliding derived coverage identifiers
    When the conformance dossier is built
    Then the conformance dossier is accepted
    And all coverage identifiers are unique

  Scenario: completed dossier retains every explicit coverage target
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed dossier loses its coverage target
    Then validating the dossier is rejected with "coverage"

  Scenario: generated relation gaps avoid caller gap identifiers
    Given a conformance dossier intake with a caller gap colliding with a generated relation gap
    When the conformance dossier is built twice
    Then the conformance dossier is accepted
    And generated relation gaps avoid caller gap identifiers

  Scenario: every scoped requirement has an explicit trace obligation
    Given a conformance dossier intake with one scoped requirement missing a trace relation
    When the conformance dossier is built
    Then the conformance dossier is accepted
    And the dossier names a requirement-trace gap for "req.order"

  Scenario Outline: generated gap references stay within their obligation domains
    Given a complete conformance dossier intake
    When the conformance dossier is built
    And the completed conformance dossier gets an invalid generated gap reference of kind "<kind>"
    Then validating the dossier is rejected with "<message>"

    Examples:
      | kind                       | message           |
      | requirement_trace_missing  | generated requirement_trace_missing gap |
      | relation_missing           | generated relation_missing gap |
      | review_missing             | generated review_missing gap |
      | review_target_mismatch     | generated review_target_mismatch gap reference |

  Scenario Outline: caller-supplied generated gap kinds remain reserved
    Given a conformance dossier intake with a caller-supplied generated gap of kind "<kind>"
    When the conformance dossier is built
    Then the conformance dossier is rejected with "generated gap kind"

    Examples:
      | kind                       |
      | requirement_trace_missing  |
      | relation_missing           |
      | review_missing             |
      | review_target_mismatch     |

  Scenario Outline: stale generated gaps are rejected for every obligation domain
    Given a complete conformance dossier with a stale generated gap of kind "<kind>"
    Then validating the dossier is rejected with "stale generated <kind> gap"

    Examples:
      | kind                       |
      | requirement_trace_missing  |
      | relation_missing           |
      | review_missing             |
      | review_target_mismatch     |
