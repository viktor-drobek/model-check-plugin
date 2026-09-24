Feature: G1 Promela subset — chapter 2–3 models through the Promela frontend to IR and verdicts
  Plan 14 §9, row G1: the Promela subset of §5.2 (as amended after K1) is parsed,
  lowered to the IR of G0 and checked by the G0 explorer. Exit criterion, verbatim:
  `mutex_flaw`, `peterson`, `prodcons`, `alternatingbit` agree with SPIN on verdict and
  error class; state counts agree with SPIN's optimisations disabled
  (`spin -a -o1 -o2 -o3`, `gcc -O2 -DNOREDUCE`, `./pan -c0`); a comparison utility
  against `pan -d` at statement granularity exists.

  Vocabulary used below (the same words as the report):
  - a *rejection* is a JSON `{"error": {kind, status, path, message}}` on stdout with
    exit code 2; its status is always `not-executed`; kinds are `syntax`, `semantic`
    and `outside-subset`. Nothing that is rejected has been executed.
  - `invalid-model` is produced only by execution: domain overflow, index out of
    range, division by zero, blocking inside `d_step`. The two sets — rejected before
    execution, misbehaving during execution — are disjoint and together cover every
    input that yields no ordinary verdict.
  - the *state count* is pan's "states, stored" with `-c0`, which does not stop at the
    first error; `mcd check --sweep` is the matching mode (the search does not stop
    when every property is decided). The pan numbers quoted here were produced by
    SPIN 6.5.2 with the flags above on 2026-09-24.
  - `else` is enabled in a state iff no other edge out of the same control location is
    enabled; `timeout` is enabled iff no process has an enabled edge whose guard does
    not mention `timeout`. Both are evaluated by the explorer, not precomputed.
  - Process instances are named `<proctype>:<pid>` with SPIN's pid order (`active`
    and `init` in textual order, then `run`-created); locals are qualified as
    `<proctype>:<pid>.<name>`.

  # ---------------------------------------------------------------- exit criterion

  Scenario: mutex_flaw — the assertion is violated and the counterexample reads as Promela
    Given the Promela model "CH2/mutex_flaw.pml" from the corpus
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "assert" is "violated" with evidence "exhaustive"
    And the reason for "assert" mentions "assert(cnt == 1)"
    And the counterexample of "assert" ends with "cnt" equal to 2
    And the counterexample of "assert" passes through the labels "L1, L2, L3, L4"
    And every step of the counterexample of "assert" names a process, a source line and the statement text
    And property "deadlock" is "verified" with evidence "exhaustive"
    And the search is complete
    And the state count is 429

  Scenario: peterson — mutual exclusion holds and the search is complete
    Given the Promela model "CH2/peterson.pml" from the corpus
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "assert" is "verified" with evidence "exhaustive"
    And property "deadlock" is "verified" with evidence "exhaustive"
    And the search is complete
    And the state count is 74

  Scenario: prodcons — no deadlock, as pan reports no invalid end state; printf is kept as a step
    Given the Promela model "CH2/prodcons.pml" from the corpus
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "deadlock" is "verified" with evidence "exhaustive"
    And the search is complete
    And the state count is 6
    And the warnings mention "printf"

  Scenario: alternatingbit — safety check: no deadlock, buffered channels of mtype
    Given the Promela model "CH3/alternatingbit.pml" from the corpus
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "deadlock" is "verified" with evidence "exhaustive"
    And the search is complete
    And the state count is 8

  Scenario Outline: state counts equal pan -c0 and the error class agrees (chapters 2–3, inside the subset)
    Given the Promela model "<model>" from the corpus
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And the search is complete
    And the state count is <states>
    And the pan error class is "<class>"

    Examples:
      | model                    | states | class               |
      | CH2/mutex_flaw.pml       | 429    | assertion violated  |
      | CH2/peterson.pml         | 74     | no error            |
      | CH2/prodcons.pml         | 6      | no error            |
      | CH3/alternatingbit.pml   | 8      | no error            |
      | CH2/peterson2.pml        | 42     | invalid end state   |
      | CH2/mutex.pml            | 190    | no error            |
      | CH2/protocol             | 32     | invalid end state   |
      | CH2/protocol2            | 25     | invalid end state   |
      | CH2/false.pml            | 3      | assertion violated  |
      | CH2/hello.pml            | 3      | no error            |
      | CH2/hello2.pml           | 3      | no error            |
      | CH3/alternatingbit2.pml  | 16     | no error            |
      | CH3/counter3.pml         | 3      | no error            |
      | CH3/counter4.pml         | 3      | no error            |
      | CH3/euclid.pml           | 10     | no error            |
      | CH3/macro.pml            | 5      | assertion violated  |
      | CH3/mtype.pml            | 3      | no error            |
      | CH3/rendezvous.pml       | 3      | invalid end state   |
      | CH3/send_recv.pml        | 10     | invalid end state   |
      | CH3/you_run.pml          | 7      | no error            |
      | CH3/you_run2.pml         | 14     | no error            |

  # ---------------------------------------------------------------- semantics made visible

  Scenario: peterson2 — the deadlock is found with a counterexample of Promela statements
    Given the Promela model "CH2/peterson2.pml" from the corpus
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "deadlock" is "violated" with evidence "exhaustive"
    And the reason for "deadlock" mentions "no transition is enabled"
    And the counterexample of "deadlock" ends with "flag[1]" equal to 1
    And the counterexample of "deadlock" ends with "flag[2]" equal to 1

  Scenario: a function-like macro is expanded and the assertion keeps the line of the macro call
    Given the Promela model "CH3/macro.pml" from the corpus
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "assert" is "violated" with evidence "exhaustive"
    And the last step of the counterexample of "assert" is "assert(a)" at line 9

  Scenario: rendezvous — a handshake is one step and the unmatched second send is a deadlock
    Given the Promela model "CH3/rendezvous.pml" from the corpus
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "deadlock" is "violated" with evidence "exhaustive"
    And the counterexample of "deadlock" contains the step "name!msgtype(124)"
    And the counterexample of "deadlock" ends with "B:1.state" equal to 124

  Scenario: local variables are qualified by the process instance in the counterexample
    Given the Promela file "testdata/promela/local-assert.pml"
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "assert" is "violated" with evidence "exhaustive"
    And the counterexample of "assert" changes "P:1.k" from 0 to 2

  Scenario: byte overflow is invalid-model, where pan silently wraps (documented systematic difference)
    Given the Promela model "CH3/counter.pml" from the corpus
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "deadlock" is "invalid-model" with evidence "unknown"
    And the reason for "deadlock" mentions "domain overflow"
    And the reason for "deadlock" mentions "count--"

  Scenario: blocking inside d_step is invalid-model, never a rejection
    Given the Promela file "testdata/promela/dstep-block.pml"
    When I execute "mcd check --promela <model> --no-timing"
    Then the command exits with 0
    And property "deadlock" is "invalid-model" with evidence "unknown"
    And the reason for "deadlock" mentions "block in d_step"
    And the reason for "deadlock" mentions "x == 2"

  # ---------------------------------------------------------------- rejections: not-executed, never invalid-model

  Scenario: a model with c_code is rejected naming the construct, file and line
    Given the Promela model "CH17/simple1.pr" from the corpus
    When I execute "mcd check --promela <model>"
    Then the command exits with 2
    And the rejection has kind "outside-subset" and status "not-executed"
    And the rejection mentions "c_code"
    And the rejection mentions "simple1.pr"
    And the rejection mentions "line 1"

  Scenario Outline: every construct outside the MVP subset is named with its line
    Given the Promela model "<model>" from the corpus
    When I execute "mcd parse --promela <model>"
    Then the command exits with 2
    And the rejection has kind "outside-subset" and status "not-executed"
    And the rejection mentions "<construct>"
    And the rejection mentions "line <line>"

    Examples:
      | model               | construct                    | line |
      | CH3/inline.pml      | inline                       | 1    |
      | CH3/typedef.pml     | typedef                      | 1    |
      | CH3/toggle.pml      | provided                     | 4    |
      | CH3/pots.pml        | channel-typed message field  | 4    |
      | CH3/rendezvous2.pml | channel-typed message field  | 3    |
      | CH2/prodcons2.pml   | inline                       | 6    |
      | CH3/splurge.pml     | run outside init             | 4    |
      | CH3/wc.pml          | uninitialised channel        | 1    |
      | CH3/notpossible.pml | run inside an expression     | 3    |

  Scenario: a syntax error is a rejection with the position, not an engine result
    Given the Promela file "testdata/promela/syntax-error.pml"
    When I execute "mcd parse --promela <model>"
    Then the command exits with 2
    And the rejection has kind "syntax" and status "not-executed"
    And the rejection mentions "line 3"

  Scenario: an undeclared variable is a semantic rejection, as SPIN itself rejects CH3/scope.pml
    Given the Promela model "CH3/scope.pml" from the corpus
    When I execute "mcd parse --promela <model>"
    Then the command exits with 2
    And the rejection has kind "semantic" and status "not-executed"
    And the rejection mentions "y"
    And the rejection mentions "line 11"

  # ---------------------------------------------------------------- preprocessor and never claims

  Scenario: -D selects the never claim through #ifdef in CH4/prop.pml; the claim is parsed and ignored with a warning
    Given the Promela model "CH4/prop.pml" from the corpus
    When I execute "mcd parse --promela <model> -D PHI"
    Then the command exits with 0
    And the IR has a claim process whose locations carry the label "accept"
    And the warnings mention "never claim"
    When I execute "mcd parse --promela <model>"
    Then the command exits with 0
    And the IR has a claim process whose locations carry no label
    And the IR has a claim process with an edge whose text is "!(x != 0)"

  Scenario: a never claim does not take part in a safety check
    Given the Promela model "CH4/prop.pml" from the corpus
    When I execute "mcd check --promela <model> -D PHI --no-timing"
    Then the command exits with 0
    And property "deadlock" is "verified" with evidence "exhaustive"
    And the warnings mention "never claim"
    And the state count is 3

  Scenario: xr and xs are accepted and stored as hints
    Given the Promela file "testdata/promela/xrxs.pml"
    When I execute "mcd parse --promela <model>"
    Then the command exits with 0
    And the IR channel "q" has the hints xs "S" and xr "R"

  # ---------------------------------------------------------------- IR round trip and origins

  Scenario: the parsed Promela model round-trips through the IR and gives the same report
    Given the Promela model "CH2/mutex_flaw.pml" from the corpus
    When I execute "mcd parse --promela <model>" and keep the output as "a.json"
    And I execute "mcd parse --ir a.json" and keep the output as "b.json"
    Then the kept outputs "a.json" and "b.json" are byte-identical
    When I execute "mcd check --ir a.json --no-timing" and keep the output as "ir.report"
    And I execute "mcd check --promela <model> --no-timing" and keep the output as "pml.report"
    Then the kept outputs "ir.report" and "pml.report" are identical except for the inputs section

  Scenario: every edge carries its origin and instances are named by proctype and pid
    Given the Promela model "CH2/mutex_flaw.pml" from the corpus
    When I execute "mcd parse --promela <model>"
    Then the command exits with 0
    And the IR names the processes "user:0, user:1"
    And every edge of the IR has an origin with the file, a positive line and the statement text
    And the local "me" of process "user:0" is initialised to 1 and of "user:1" to 2

  # ---------------------------------------------------------------- differential utility

  @spin
  Scenario Outline: pandiff agrees with pan on verdict, error class and state count
    Given spin is installed
    And the Promela model "<model>" from the corpus
    When I run pandiff on the model
    Then pandiff reports agreement on the verdict, the error class and the state count
    And pandiff reports the statement table of every proctype as matching pan -d

    Examples:
      | model                   |
      | CH2/mutex_flaw.pml      |
      | CH2/peterson.pml        |
      | CH2/prodcons.pml        |
      | CH3/alternatingbit.pml  |
