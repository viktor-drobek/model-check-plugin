@g9-openapi
Feature: OpenAPI declaration contract profile
  The offline OpenAPI adapter records declarations without inferring behavior.

  Scenario: A supported OpenAPI document produces only contract evidence
    Given a supported OpenAPI declaration document
    When I analyze the OpenAPI declaration document
    Then the OpenAPI ledger contains declaration facts only

  Scenario: An unsupported OpenAPI version is rejected
    Given an OpenAPI declaration document with version "3.0.2"
    When I analyze the OpenAPI declaration document
    Then OpenAPI analysis fails with rule "stage4.unsupported_version"

  Scenario: OpenAPI rejects duplicate path template shapes
    Given an OpenAPI declaration with aliased path templates
    When I analyze the OpenAPI declaration document
    Then OpenAPI analysis fails with rule "stage4.schema_invalid"
