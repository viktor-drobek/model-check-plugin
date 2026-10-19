Feature: Trace-ledger standards imports
  Offline standards imports preserve their evidence boundaries.

  Scenario: import BCP 14 candidates from a pinned RFC text
    Given the RFC import fixture "standards/rfc-8174.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc8174.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the StandardClause "RFC 8174, category Best Current Practice, updates 2119"
    And the standards trace ledger contains the normative candidate "An implementation MUST"
    And the standards trace ledger does not contain the normative candidate "The key words"

  Scenario: do not infer BCP 14 semantics without its declaration
    Given the RFC import fixture "standards/rfc-without-bcp-declaration.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9001.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not infer BCP 14 semantics from an RFC header
    Given the RFC import fixture "standards/rfc-bcp-declaration-in-header.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9015.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: reject an RFC source URL for another RFC number
    Given the RFC import fixture "standards/rfc-8174.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9000.txt"
    Then the standards trace ledger is rejected with "does not identify RFC 8174"

  Scenario: reject a noncanonical RFC publisher path
    Given the RFC import fixture "standards/rfc-8174.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/errata/rfc8174"
    Then the standards trace ledger is rejected with "canonical RFC URL"

  Scenario: do not infer a BCP 14 declaration when its predicate names another subject
    Given the RFC import fixture "standards/rfc-bcp-declaration-wrong-subject.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9016.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not infer a BCP 14 declaration through unrelated prose
    Given the RFC import fixture "standards/rfc-unrelated-rfc-reference.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9008.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: emit candidates only after a BCP 14 declaration
    Given the RFC import fixture "standards/rfc-bcp-declaration-order.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9003.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "after the declaration"
    And the standards trace ledger does not contain the normative candidate "before the BCP 14 declaration"

  Scenario: parse RFC headers without consuming their right-hand column
    Given the RFC import fixture "standards/rfc-multicolumn-headers.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9004.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the StandardClause "RFC 9004, category Standards Track, updates 1111, 2222"

  Scenario: parse a wrapped numeric Updates header
    Given the RFC import fixture "standards/rfc-wrapped-updates.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9006.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the StandardClause "RFC 9006, category Informational, updates 1111, 2222, 3333"

  Scenario: reject an ambiguous numeric right-hand Updates column
    Given the RFC import fixture "standards/rfc-updates-numeric-right-column.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9017.txt"
    Then the standards trace ledger is rejected with "malformed Updates"

  Scenario: reject a numeric right-hand Updates column with generic text
    Given the RFC import fixture "standards/rfc-updates-numeric-column.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9024.txt"
    Then the standards trace ledger is rejected with "malformed Updates"

  Scenario: reject a wrapped numeric right-hand Updates column with generic text
    Given the RFC import fixture "standards/rfc-updates-wrapped-numeric-column.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9025.txt"
    Then the standards trace ledger is rejected with "malformed Updates"

  Scenario: reject RFC numbers in a right-hand Updates column
    Given the RFC import fixture "standards/rfc-updates-numbers-right-column.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9018.txt"
    Then the standards trace ledger is rejected with "malformed Updates"

  Scenario: reject an unseparated numeric Updates column
    Given the RFC import fixture "standards/rfc-updates-unseparated-number.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9019.txt"
    Then the standards trace ledger is rejected with "malformed Updates"

  Scenario: reject an incomplete RFC Updates header
    Given the RFC import fixture "standards/rfc-incomplete-updates.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9007.txt"
    Then the standards trace ledger is rejected with "malformed Updates"

  Scenario: do not read RFC-like headers from body text
    Given the RFC import fixture "standards/rfc-body-header.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9005.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the StandardClause "RFC 9005, category Informational"
    And the standards trace ledger contains the normative candidate "An implementation MUST"

  Scenario: reject unsafe RFC header metadata before normalization
    Given the RFC import fixture "standards/rfc-unsafe-category.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9020.txt"
    Then the standards trace ledger is rejected with "publication category"

  Scenario: reject a category that is truncated at a column separator
    Given the RFC import fixture "standards/rfc-category-double-space.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9021.txt"
    Then the standards trace ledger is rejected with "publication category"

  Scenario: reject noncanonical whitespace in a mandatory RFC header
    Given the RFC import fixture "standards/rfc-noncanonical-header-whitespace.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9038.txt"
    Then the standards trace ledger is rejected with "publication category"

  Scenario: reject Unicode whitespace in an RFC Updates header
    Given the RFC import fixture "standards/rfc-updates-unicode-whitespace.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9046.txt"
    Then the standards trace ledger is rejected with "malformed Updates"

  Scenario: reject Unicode whitespace at an RFC header-column boundary
    Given the RFC import fixture "standards/rfc-header-unicode-column-whitespace.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9053.txt"
    Then the standards trace ledger is rejected with "Request for Comments number"

  Scenario: reject Unicode whitespace in an RFC Updates continuation
    Given the RFC import fixture "standards/rfc-updates-continuation-unicode-whitespace.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9054.txt"
    Then the standards trace ledger is rejected with "malformed Updates"

  Scenario: retain a wrapped Updates list after a date right-hand column
    Given the RFC import fixture "standards/rfc-updates-date-column-wrapped.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9022.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the StandardClause "RFC 9022, category Informational, updates 1111, 2222"

  Scenario: do not infer a BCP 14 declaration from negated prose
    Given the RFC import fixture "standards/rfc-negated-bcp-declaration.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9023.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not infer a BCP 14 declaration from a template phrase
    Given the RFC import fixture "standards/rfc-bcp-template-declaration.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9026.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not infer a BCP 14 declaration from a relative-clause template phrase
    Given the RFC import fixture "standards/rfc-bcp-template-relative-clause.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9045.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not infer a BCP 14 declaration from a sentence-boundary template phrase
    Given the RFC import fixture "standards/rfc-bcp-template-sentence-boundary.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9049.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not infer a BCP 14 declaration from an RFC quotation
    Given the RFC import fixture "standards/rfc-bcp-quoted-declaration.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9042.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: preserve quotation context when it shares a paragraph with a declaration
    Given the RFC import fixture "standards/rfc-bcp-declaration-quotation-overlap.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9055.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "A genuine requirement MUST become a candidate"

  Scenario: do not infer a BCP 14 declaration from a bracketed RFC quotation
    Given the RFC import fixture "standards/rfc-bcp-bracketed-quotation.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9047.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: retain RFC body prose that begins with an RFC number
    Given the RFC import fixture "standards/rfc-bcp-body-prose.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9043.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "RFC 9000 endpoints MUST discard such packets"

  Scenario: retain body prose that resembles page furniture
    Given the RFC import fixture "standards/rfc-bcp-page-furniture-body-prose.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9048.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "Servers MUST return page 1"
    And the standards trace ledger contains the normative candidate "RFC 9000  endpoints MUST discard such packets"

  Scenario: retain body prose with multiple header-like columns
    Given the RFC import fixture "standards/rfc-bcp-running-header-like-body.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9050.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "RFC 9000  endpoints MUST discard such packets"

  Scenario: reject an RFC candidate with a leading vertical tab
    Given the RFC import fixture "standards/rfc-unsafe-candidate-vertical-tab.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9051.txt"
    Then the standards trace ledger is rejected with "candidate line contains unsafe characters"

  Scenario: retain operational uses of this and following text
    Given the RFC import fixture "standards/rfc-bcp-operational-text.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9044.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "Clients MUST include this text in the Reason header"
    And the standards trace ledger contains the normative candidate "Senders MUST use the following text encoding"

  Scenario: retain uppercase template-like operational requirements
    Given the RFC import fixture "standards/rfc-bcp-uppercase-template-like-operational.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9052.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "Editors SHOULD include this text in the Reason header"
    And the standards trace ledger contains the normative candidate "Clients MUST validate the source"

  Scenario: do not infer a BCP 14 declaration after two template separator lines
    Given the RFC import fixture "standards/rfc-bcp-template-double-blank.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9029.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not infer a BCP 14 declaration from a same-paragraph template phrase
    Given the RFC import fixture "standards/rfc-bcp-template-same-paragraph.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9031.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not infer a BCP 14 declaration after template page furniture
    Given the RFC import fixture "standards/rfc-bcp-template-page-footer.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9033.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not infer a BCP 14 declaration from a replacement quotation
    Given the RFC import fixture "standards/rfc-bcp-replacement-quotation.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9034.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger has no normative candidates

  Scenario: do not emit a quoted replacement requirement after a BCP 14 declaration
    Given the RFC import fixture "standards/rfc-bcp-replacement-after-declaration.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9037.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "A genuine requirement MUST"
    And the standards trace ledger does not contain the normative candidate "RFC 2119 senders MUST"
    And the standards trace ledger does not contain the normative candidate "Another replacement requirement MUST"

  Scenario: do not emit an extended replacement section after a BCP 14 declaration
    Given the RFC import fixture "standards/rfc-bcp-new-text-after-declaration.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9039.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "A genuine requirement MUST"
    And the standards trace ledger does not contain the normative candidate "Senders MUST reject the value"

  Scenario: do not emit an RFC quotation after a BCP 14 declaration
    Given the RFC import fixture "standards/rfc-bcp-quotation-after-declaration.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9040.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "A genuine requirement MUST"
    And the standards trace ledger does not contain the normative candidate "Senders MUST reject the value"

  Scenario: do not emit a keyword-bearing RFC running header after a BCP 14 declaration
    Given the RFC import fixture "standards/rfc-bcp-running-header-after-declaration.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9041.txt"
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the normative candidate "A genuine requirement MUST"
    And the standards trace ledger does not contain the normative candidate "TLS MUST-Staple"

  Scenario: reject an ambiguous RFC number right-hand column
    Given the RFC import fixture "standards/rfc-ambiguous-request-column.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9027.txt"
    Then the standards trace ledger is rejected with "Request for Comments number"

  Scenario: reject an ambiguous Category right-hand column
    Given the RFC import fixture "standards/rfc-ambiguous-category-column.txt"
    When the standards adapter imports the RFC from "https://www.rfc-editor.org/rfc/rfc9028.txt"
    Then the standards trace ledger is rejected with "publication category"

  Scenario: preserve an RFC fixture's logical input path in provenance
    Given the RFC import fixture "standards/rfc-8174.txt"
    When the standards adapter imports the RFC fixture with logical paths "one/rfc-8174.txt" and "two/rfc-8174.txt" from "https://www.rfc-editor.org/rfc/rfc8174.txt"
    Then the RFC trace ledger anchors distinguish the logical input paths

  Scenario: reject an RFC source outside RFC Editor and IETF
    Given the RFC import fixture "standards/rfc-8174.txt"
    When the standards adapter imports the RFC from "https://example.invalid/rfc8174.txt"
    Then the standards trace ledger is rejected with "RFC Editor or IETF"

  Scenario: import a user-provided IEEE clause manifest without a PDF
    Given the IEEE clause manifest fixture "standards/ieee-clause-manifest.json"
    When the standards adapter imports the IEEE clause manifest
    Then the standards trace ledger is accepted
    And the standards trace ledger contains the StandardClause "IEEE 802.1Q-2022, clause 8.6.8"
    And the standards trace ledger contains licensed clause reference evidence

  Scenario: reject a malformed IEEE clause manifest
    Given the IEEE clause manifest fixture "standards/ieee-invalid-manifest.json"
    When the standards adapter imports the IEEE clause manifest
    Then the standards trace ledger is rejected with "document_sha256"

  Scenario: reject an IEEE clause manifest that includes a document URL
    Given the IEEE clause manifest fixture "standards/ieee-manifest-with-document-url.json"
    When the standards adapter imports the IEEE clause manifest
    Then the standards trace ledger is rejected with "document_url"
