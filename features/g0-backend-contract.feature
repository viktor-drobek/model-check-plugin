Feature: G0 backend contract kernel

  Scenario: an exact verified backend result validates
    Given a valid M3.0a backend request
    When I validate an exact verified backend result
    Then the backend contract status is "verified"

  Scenario: arbitrary backend failure text is rejected
    Given a valid M3.0a backend request
    When I create backend failure host facts with arbitrary text
    Then the backend host facts are rejected

  Scenario Outline: permitted backend rows validate
    Given a valid M3.0a backend request
    When I validate the permitted backend row "<row>"
    Then the backend contract status is "<status>"

    Examples:
      | row          | status        |
      | verified     | verified      |
      | inconclusive | inconclusive  |
      | resource     | resource-stop |

  Scenario Outline: host terminal rows synthesize their status
    Given a valid M3.0a backend request
    When I validate the host terminal "<terminal>"
    Then the backend contract status is "<status>"

    Examples:
      | terminal          | status             |
      | invalid-input     | invalid-input      |
      | cancelled         | cancelled          |
      | isolation-refused | isolation-refused  |
      | backend-failure   | backend-failure    |
      | resource-stop     | resource-stop      |

  Scenario Outline: deferred evidence and trace claims are rejected
    Given a valid M3.0a backend request
    When I validate a backend result with <field> "<value>"
    Then the backend contract status is "contract-invalid"

    Examples:
      | field    | value                      |
      | evidence | certified-sound            |
      | evidence | oracle-exception-reduced   |
      | trace    | replayed                   |
      | trace    | trace-absent-approved      |
      | trace    | replay-unavailable-approved |

  Scenario: a mismatched backend identity is rejected
    Given a valid M3.0a backend request
    When I validate a backend result with a mismatched identity
    Then the backend contract status is "contract-invalid"

  Scenario Outline: a mismatched backend identity field is rejected
    Given a valid M3.0a backend request
    When I validate a backend result with a mismatched <field>
    Then the backend contract status is "contract-invalid"

    Examples:
      | field           |
      | backend ID      |
      | backend version |

  Scenario: missing backend provenance is rejected
    Given a valid M3.0a backend request
    When I validate a backend result without provenance
    Then the backend contract status is "contract-invalid"

  Scenario: an unavailable requested capability is synthesized as unsupported
    Given a valid M3.0a backend request
    When I validate without the requested backend capability
    Then the backend contract status is "unsupported"

  Scenario: an unavailable requested capability rejects a nonzero payload
    Given a valid M3.0a backend request
    When I validate without the requested backend capability and a nonzero payload
    Then the backend contract status is "contract-invalid"

  Scenario: a backend cannot self-authorise a missing acknowledgement
    Given a valid M3.0a backend request
    When I validate a backend result with an invalid capability acknowledgement
    Then the backend contract status is "contract-invalid"

  Scenario: an approximate inconclusive result validates without a verdict
    Given a valid M3.0a backend request
    When I validate an approximate inconclusive backend result
    Then the backend contract status is "inconclusive"

  Scenario: a backend violation is deferred by this contract version
    Given a valid M3.0a backend request
    When I validate a violated backend result
    Then the backend contract status is "contract-invalid"

  Scenario Outline: invalid request nonce or bound is rejected
    Given a valid M3.0a backend request
    When I create an M3.0a request with invalid <field>
    Then the backend request is rejected

    Examples:
      | field        |
      | nonce        |
      | states       |
      | depth        |
      | milliseconds |
      | memory       |

  Scenario Outline: invalid resource-stop data is rejected
    Given a valid M3.0a backend request
    When I validate a backend result with invalid resource-stop <field>
    Then the backend contract status is "contract-invalid"

    Examples:
      | field                 |
      | resource              |
      | limit                 |
      | phase                 |
      | admission consumption |
      | execution consumption |

  Scenario: a host terminal rejects a nonzero payload
    Given a valid M3.0a backend request
    When I validate a cancelled host terminal with a payload
    Then the backend contract status is "contract-invalid"

  Scenario: a malformed model is rejected before backend validation
    When I create an M3.0a request with a malformed model
    Then the backend request is rejected
