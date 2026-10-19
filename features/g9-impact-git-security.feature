Feature: Git impact input and subprocess security
  The Git impact adapter must not inherit repository-routing controls or hang
  when bounded subprocess output exceeds its limit.

  Scenario: reject inherited Git repository routing variables
    Given an environment containing inherited Git repository routing variables
    When the Git environment is sanitized
    Then repository routing variables are absent

  Scenario: normalize locale for deterministic Git diagnostics
    Given an environment containing caller locale variables
    When the Git environment is sanitized
    Then the Git locale is fixed to C

  Scenario: isolate real Git test helpers from caller Git state
    Given a hostile caller Git environment and global hook configuration
    When an isolated Git test repository is initialized and committed
    Then the caller repository is untouched and no global hook ran

  Scenario: prove the global pre-commit hook is reachable without isolation
    Given a hostile caller Git environment and global hook configuration
    When an unisolated Git test repository is initialized and committed
    Then the global pre-commit hook ran

  Scenario: restore hostile Git routing variables after scenario cleanup
    Given a hostile caller Git environment and global hook configuration
    When the hostile Git environment is restored
    Then the hostile Git environment variables are restored

  Scenario: identify an oversized tree preflight listing
    Given a Git preflight listing child that exceeds the output cap
    When the Git tree preflight is checked
    Then a command-specific Git preflight output-limit error is returned

  Scenario: terminate a Git child that exceeds the stdout cap
    Given a Git child that writes beyond the output cap
    When the bounded Git command is run
    Then the bounded Git command returns an output-limit error promptly

  Scenario: terminate descendants after a Git child closes stdout and is cancelled
    Given a Git child that closes stdout with a surviving descendant
    When the bounded Git command is cancelled
    Then the Git descendant terminates promptly after cancellation

  Scenario: reject included executable repository configuration
    Given a repository with an included executable Git filter configuration
    When the repository configuration is inspected
    Then executable repository configuration is rejected

  Scenario Outline: reject unsafe or unbounded static repository configuration
    Given a repository with "<kind>" Git configuration
    When the repository configuration is inspected
    Then executable repository configuration is rejected

    Examples:
      | kind                      |
      | legacy filter section     |
      | same-line filter payload  |
      | continued filter section  |
      | quoted continuation       |
      | CRLF continuation         |
      | worktree redirection      |
      | common Git directory      |
      | Git directory redirect    |
      | alternate object store    |
      | oversized configuration   |
      | non-regular configuration |

  Scenario: reject configuration replaced after metadata inspection
    When a Git configuration is replaced between inspection and opening
    Then the replacement Git configuration is rejected

  Scenario: explicitly clean up Git helper resources
    Given a Git helper scenario acquires temporary resources
    When explicit helper scenario cleanup runs
    Then the helper environment and temporary resources are restored

  Scenario: reject executable mode changes despite core.fileMode being false
    Given a tracked executable with core.fileMode disabled
    When repository cleanliness is checked
    Then the tracked mode change is rejected

  Scenario: reject malformed or abbreviated raw Git metadata
    Given malformed raw Git metadata
    When the raw Git diff is parsed
    Then parsing fails closed

  Scenario: ignore caller global Git attributes
    Given a repository and caller XDG global Git attributes
    When the bounded Git attributes query is run
    Then caller global Git attributes are ignored

  Scenario Outline: reject tracked files hidden by index flags
    Given a tracked repository file marked "<flag>"
    When repository cleanliness is checked
    Then the hidden tracked file is rejected

    Examples:
      | flag             |
      | assume-unchanged |
      | skip-worktree    |
