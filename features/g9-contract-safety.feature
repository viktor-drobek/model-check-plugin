Feature: Stage 4 contract structural safety
  Offline safety rules distinguish consumed routes from opaque metadata.

  Scenario: local pointers traverse arrays without aliases
    Then contract pointers support canonical array indices and reject cycles

  Scenario: routes and URLs are admitted only at declared scopes
    Then contract route exceptions and gap-only URLs respect structural scope

  Scenario: identical invalid input has a stable diagnostic
    Then contract safety errors are deterministic for mixed violations

  Scenario: documented schema keywords retain exact gaps
    Then both API profiles admit all documented schema keywords with exact gaps

  Scenario: nested properties remain schema-checked gaps, never direct facts
    Then both API profiles retain nested properties as schema-checked explicit gaps only

  Scenario: admitted names retain their full input boundary
    Then API routes and channel names accept the 256 byte boundary

  Scenario: API safety rejections preserve normalized failure semantics
    Then API version type errors and embedded file references fail atomically

  Scenario: encoded records remain within the preflight allowance
    Then maximally escaped contract records fit their output preflight allowance

  Scenario: URL exceptions do not spread to neighboring declarations
    Then API URL exceptions admit exact gaps and reject consumed neighbors

  Scenario: mixed operation gaps admit only their gap-only siblings
    Then AsyncAPI v2 fact-producing descendants reject URL-shaped declarations

  Scenario: reference-only operations do not exempt consumed identifiers
    Then AsyncAPI operation identifiers reject URLs independently of message facts

  Scenario: channel labels cannot embed HTTP schemes or surrounding whitespace
    Then AsyncAPI channel labels reject embedded URLs and whitespace variants

  Scenario: v3 operation metadata retains exact gap anchors
    Then AsyncAPI v3 operations emit exact metadata gaps and check message references

  Scenario: URL declaration keys require an enclosing metadata gap
    Then URL declaration keys cannot inherit their own gap exception

  Scenario: nested schema property names cannot use metadata URL exceptions
    Then AsyncAPI rejects URL-shaped property names at every schema depth

  Scenario: document descriptions retain their exact metadata gap
    Then AsyncAPI info descriptions emit one exact unmapped gap
