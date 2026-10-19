Feature: Git impact subprocess lifecycle
  The bounded Git adapter must observe both output streams and keep helper
  subprocess coverage bounded even when output pipes are inherited.

  Scenario: reject stderr overflow after stdout EOF
    Given a lifecycle Git helper that closes stdout and floods stderr
    When the bounded lifecycle Git command is run
    Then it returns a stderr output-limit error promptly

  Scenario: close inherited output pipes after a parent exits
    Given a lifecycle Git helper whose child inherits output pipes
    When the bounded lifecycle Git command is run
    Then the self-expiring child closes the inherited pipes before the context expires

  Scenario: terminate a descendant after the Git leader exits
    Given a lifecycle Git helper whose leader exits after closing stdout
    When the bounded lifecycle Git command is cancelled
    Then the surviving descendant terminates promptly
