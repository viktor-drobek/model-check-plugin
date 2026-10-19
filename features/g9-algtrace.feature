@algtrace
Feature: G9 algorithm trace — language-neutral static graph

  Scenario: one source corpus yields a deterministic graph across four languages
    Given the algorithm trace fixtures under "tools/algtrace/testdata"
    When I run the algtrace CLI
    Then the algorithm graph schema is "algtrace/v1"
    And the algorithm graph reports these languages:
      | go     |
      | java   |
      | python |
      | rust   |
    And every algorithm graph relation has evidence "syntactic"
    And the algorithm graph contains each relation kind:
      | sends        |
      | receives     |
      | writes_state |
      | branches     |
      | loops        |
