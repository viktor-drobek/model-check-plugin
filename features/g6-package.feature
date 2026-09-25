# G6 — plan 14 §9, row G6. Criterion, verbatim:
#   "Все evals; триггер-точность на held-out ≥ порога; плагин устанавливается на
#    чистой машине без Go; реальный клиент подтверждает, что `plugin.json` и
#    корневой `.mcp.json` не регистрируют сервер дважды."
#
# Scope of the words used below, fixed here so the scenarios cannot be read as
# claiming more than they check:
#
#   "produces a binary for <platform>" — `build.sh` cross-compiles with
#   CGO_ENABLED=0 and writes engine/bin/mcd-<goos>-<goarch>[.exe]. Cross-built
#   files are *produced and hashed* here; only the host platform's binary is
#   ever *executed* by these scenarios. A green run therefore says "five
#   binaries exist with recorded sums", never "five platforms were tested".
#   The host of record is named in steps/g6-confirmation.md §1.
#
#   "with Go absent from PATH" — the sandbox of A6 (plan §12): a copy of
#   model-check-plugin/ in a temporary directory, run with a PATH that holds
#   every ordinary utility of this machine and no Go tool (`go`, `gofmt`,
#   `gccgo`), and with GOROOT/GOTOOLCHAIN/GOPATH/GOFLAGS removed from the
#   environment. Whole PATH directories are NOT dropped: /usr/bin carries `go`
#   and `uname` alike, so dropping it would test a machine without coreutils
#   instead of a machine without Go. It simulates a machine that never had a Go
#   toolchain; it is not a fresh operating system, and the scenarios say only
#   what that sandbox shows.
#
#   "a clean build output directory" — a fresh temporary directory passed to
#   build.sh with --out. The committed engine/bin is left alone by the test run,
#   so its SHA256SUMS keeps describing the release build rather than a test one.
#
#   "exactly one server registration" — counted from what a client actually
#   reads: the plugin's declared MCP sources (plugin.json `mcpServers` and any
#   auto-discovered `.mcp.json` at the plugin root) resolved to server names.
#   Two sources naming the same server still count as two registrations of that
#   name unless the client is shown to deduplicate them; the confirmation
#   records which of the two this run established and by what evidence.
#
#   "held-out" — the 40 % test split of evals-workspace/trigger-eval.json. The
#   accuracy quoted by the exit criterion is the held-out one; the training
#   split is reported separately and is never the number compared to a
#   threshold.
#
# Paths are relative to model-check-plugin/ unless a step says otherwise.
Feature: G6 packaging, install validation and description triggering

  # ------------------------------------------------------------- build.sh
  Scenario: build.sh produces a binary for every listed platform, each with a SHA256SUMS entry
    Given a clean build output directory
    When the build script "build.sh" is run into it with version "0.1.0-g6-test"
    Then the script exits with code 0
    And the output directory contains a binary for each of these platforms:
      | goos    | goarch |
      | linux   | amd64  |
      | linux   | arm64  |
      | darwin  | amd64  |
      | darwin  | arm64  |
      | windows | amd64  |
    And every file in the output directory has an entry in "SHA256SUMS" whose digest matches it
    And "SHA256SUMS" has no entry for a file that is absent
    And the build record says the build used CGO_ENABLED=0

  Scenario: mcd version reports the version the build embedded
    Given a clean build output directory
    When the build script "build.sh" is run into it with version "0.1.0-g6-test"
    Then running the host binary with argument "version" prints "0.1.0-g6-test"
    And the version recorded in "BUILD-INFO.json" is "0.1.0-g6-test"

  Scenario: the host binary is selected without asking the caller which platform it is on
    Given the packaged plugin directory
    Then the command in ".mcp.json" resolves, on this host, to an executable file under the plugin directory
    And running that command with argument "version" prints the version recorded in "engine/bin/BUILD-INFO.json"

  # -------------------------------------------------- install validation (A6)
  Scenario: the packaged plugin serves its seven tools with Go absent from PATH
    Given a sandbox copy of the plugin in a temporary directory
    And no "go" executable on PATH inside the sandbox
    When the MCP server is started from the packaged binary over stdio inside the sandbox
    And the client sends "initialize" and then "tools/list"
    Then the server reports its implementation name "mcd"
    And "tools/list" returns exactly these tools:
      | name              |
      | mc_check          |
      | mc_estimate       |
      | mc_explain        |
      | mc_lint_property  |
      | mc_manifest       |
      | mc_parse          |
      | mc_simulate       |
    And no Go toolchain was reachable from the environment the server ran in

  Scenario: one mc_check round-trip succeeds inside the Go-less sandbox
    Given a sandbox copy of the plugin in a temporary directory
    And no "go" executable on PATH inside the sandbox
    When the MCP server is started from the packaged binary over stdio inside the sandbox
    And the client parses the petrinet1 net and checks "deadlock" on it
    Then the property "deadlock" has status "violated" and evidence "exhaustive"
    And the counterexample summary is "t1, t4"

  Scenario: plugin.json and the plugin-root .mcp.json together yield exactly one server registration
    Given the plugin manifest ".claude-plugin/plugin.json"
    Then the manifest is valid JSON with a kebab-case "name" and a "version"
    And the MCP sources the plugin declares resolve to exactly one server named "model-check"
    And the repository root holds no ".mcp.json" that declares a server named "model-check"

  # --------------------------------------------------- trigger eval set (§8.2)
  Scenario: the trigger eval set has at least sixteen queries and both classes
    Given the file "evals-workspace/trigger-eval.json"
    Then it is a JSON array of at least 16 entries
    And every entry has a non-empty "query" and a boolean "should_trigger"
    And at least 8 entries have "should_trigger" true
    And at least 8 entries have "should_trigger" false
    And at least one entry that must trigger is written in Russian
    And at least one entry that must trigger is written in English
    And no two entries have the same "query"

  Scenario: the measured trigger accuracy is reported on the held-out split
    Given the file "evals-workspace/trigger-results.json"
    Then it names the split sizes and the seed used to make the split
    And it carries a train score and a test score for every description variant measured
    And the variant recorded as chosen is the one with the best test score
    And the description of "skills/model-check/SKILL.md" equals the chosen variant

  Scenario: the chosen description keeps its Russian and English trigger phrases
    Given the file "skills/model-check/SKILL.md"
    Then its description mentions "model checking" and "deadlock" and "Petri"
    And its description mentions "сеть Петри" and "тупик" and "может ли зависнуть"
    And its description still says that no external model checker is needed

  # ------------------------------------------------------------ full evals
  Scenario: every eval the engine can run by G5 is graded in both configurations in iteration-4
    Given the eval file "skills/model-check/evals/evals.json"
    And the workspace iteration "evals-workspace/iteration-4"
    Then every eval whose "runnable_from" is at most "G5" has a graded run with the skill
    And every such eval has a graded run without the skill
    And "evals-workspace/iteration-4/benchmark.json" lists both configurations for each of them
    And the benchmark emits "with_skill" before "without_skill"
