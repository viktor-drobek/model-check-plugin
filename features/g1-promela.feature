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
  - `invalid-model` is produced only by execution, when a step of the model cannot be
    carried out: domain overflow, index out of range, division by zero, blocking
    inside `d_step`, a `d_step` that does not finish, `run` of a process that is
    already running. So every input is in exactly one of two classes: rejected
    (exit 2, status `not-executed`, never a report) or accepted (exit 0, a report
    whose statuses come from the 11 §14 vocabulary; `invalid-model` can occur only
    there).
  - the *state count* is pan's "states, stored" with `-c0`, which does not stop at the
    first error; `mcd check --sweep` is the matching mode (the search does not stop
    when every property is decided). The pan numbers quoted here were produced by
    SPIN 6.5.2 with the flags above on 2026-09-24.
  - `else` is enabled in a state iff no other edge out of the same control location is
    enabled; `timeout` is true in a state iff no edge of any process is enabled when
    `timeout` is taken as false (SPIN's rule: the timeout alternatives are tried only
    after everything else has failed). Both are evaluated by the explorer, not
    precomputed.
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
    Given the Promela model "<file>" from the corpus
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And the search is complete
    And the state count is <states>
    And the pan error class is "<class>"

    Examples:
      | file                     | states | class               |
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
    And the step "name!msgtype(124)" of the counterexample of "deadlock" has the partner "name?msgtype(state)" in process "B:1"
    And the counterexample of "deadlock" contains the step "-end-"

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

  # Amended in G5: the v1 subset of plan 14 §5.2 is implemented, so `inline`,
  # `typedef`, `provided`, channel-typed message fields, channel-typed
  # variables and `run` outside init are no longer outside it — they are
  # checked by features/g5-ctl-v1.feature against pan's counts. What stays
  # outside keeps its place here, and the division remains exhaustive: an
  # input is parsed, or rejected by name and line.
  Scenario Outline: every construct outside the subset the engine accepts is named with its line
    Given the Promela model "<file>" from the corpus
    When I execute "mcd parse --promela <model>"
    Then the command exits with 2
    And the rejection has kind "outside-subset" and status "not-executed"
    And the rejection mentions "<construct>"
    And the rejection mentions "line <line>"

    Examples:
      | file                | construct                    | line |
      | CH3/pots.pml        | unless                       | 20   |
      | CH3/notpossible.pml | run inside an expression     | 3    |
      | CH17/simple1.pr     | c_code                       | 1    |
      | CH14/version6       | remote reference (P@label)   | 231  |

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

  # Amended in G4: the claim is no longer "parsed and ignored with a warning";
  # it becomes the model's `ltl` property `never` and is run in product with
  # the system (features/g4-ltl.feature). The claim's closing brace carries the
  # `end` label, which for a claim means "the claim terminated" (a violation).
  Scenario: -D selects the never claim through #ifdef in CH4/prop.pml; the claim becomes the property "never"
    Given the Promela model "CH4/prop.pml" from the corpus
    When I execute "mcd parse --promela <model> -D PHI"
    Then the command exits with 0
    And the IR has a claim process whose locations carry the label "accept"
    And the IR has a property "never" of kind "ltl"
    When I execute "mcd parse --promela <model>"
    Then the command exits with 0
    And the IR has a claim process whose locations carry no "accept" label
    And the IR has a claim process with an edge whose text is "!(x != 0)"

  Scenario: a never claim does not take part in a safety check
    Given the Promela model "CH4/prop.pml" from the corpus
    When I execute "mcd check --promela <model> -D PHI --no-timing"
    Then the command exits with 0
    And property "deadlock" is "verified" with evidence "exhaustive"
    And the state count is 3
    And property "never" is "violated" with evidence "exhaustive"

  Scenario: xr and xs are accepted and stored as hints
    Given the Promela file "testdata/promela/xrxs.pml"
    When I execute "mcd parse --promela <model>"
    Then the command exits with 0
    And the IR channel "q" has the hints xs "S" and xr "R"

  # ---------------------------------------------------------------- _nr_pr in a model without run

  # `_nr_pr` is the number of live processes: a process that has reached its
  # end leaves the process table, and only the youngest live process may leave
  # (SPIN's rule, the same one `run`-created processes follow). A model without
  # `run` that read `_nr_pr` used to carry a table that nothing ever updated,
  # so a guard waiting for the count to fall was never true and the engine
  # reported a deadlock that pan does not report
  # (steps/fix-nrpr-confirmation.md).
  Scenario: _nr_pr falls when the younger process ends, so the older one waiting for it proceeds (no run in the model)
    Given the Promela file "testdata/promela/nrpr-active.pml"
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And property "deadlock" is "verified" with evidence "exhaustive"
    And the search is complete
    And the state count is 5

  Scenario: _nr_pr falls one process at a time, because only the youngest live process leaves
    Given the Promela file "testdata/promela/nrpr-order.pml"
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And property "deadlock" is "verified" with evidence "exhaustive"
    And the search is complete
    And the state count is 10

  Scenario: an older process cannot leave before the youngest, so a youngest process waiting for _nr_pr == 1 is stuck
    Given the Promela file "testdata/promela/nrpr-youngest.pml"
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And property "deadlock" is "violated" with evidence "exhaustive"
    And the search is complete
    And the state count is 4

  Scenario: _nr_pr with active and run together keeps the table current, and the older process leaves after the younger
    Given the Promela file "testdata/promela/nrpr-mixed.pml"
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And property "deadlock" is "verified" with evidence "exhaustive"
    And the search is complete
    And the state count is 15

  Scenario: a model that never reads _nr_pr keeps its IR and its report byte for byte
    Given the Promela file "testdata/promela/nrpr-unread.pml"
    When I execute "mcd parse --promela <model>"
    Then the command exits with 0
    And the output is byte-identical to the file "testdata/golden/nrpr-unread.ir.json"
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And the report equals the file "testdata/golden/nrpr-unread.report.json" apart from the engine version
    And the state count is 10

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
    And the Promela model "<file>" from the corpus
    When I run pandiff on the model
    Then pandiff reports agreement on the verdict, the error class and the state count
    And pandiff reports the statement table of every proctype as matching pan -d

    Examples:
      | file                    |
      | CH2/mutex_flaw.pml      |
      | CH2/peterson.pml        |
      | CH2/prodcons.pml        |
      | CH3/alternatingbit.pml  |

  # ---------------------------------------------------------------- provided and rendezvous
  #
  # `provided (expr)` gates every transition of the process (explore.enabled
  # says so). A rendezvous is one step of two processes, so it needs both of
  # them executable: a process whose provided clause is false can neither start
  # a handshake nor answer one. The engine checked the clause on every other
  # kind of step and on neither side of a handshake.

  Scenario Outline: a process whose provided clause is false takes part in no rendezvous, on either side, as pan
    Given the Promela file "testdata/promela/<file>"
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And property "deadlock" is "violated" with evidence "exhaustive"
    And property "assert" is "verified" with evidence "exhaustive"
    And the state count is 1

    Examples:
      | file                  |
      | provided-rv-send.pml  |
      | provided-rv-recv.pml  |

  @spin
  Scenario Outline: pandiff agrees with pan on a rendezvous with a process kept from moving by provided
    Given spin is installed
    And the Promela file "testdata/promela/<file>"
    When I run pandiff on the model
    Then pandiff reports agreement on the verdict, the error class and the state count

    Examples:
      | file                  |
      | provided-rv-send.pml  |
      | provided-rv-recv.pml  |

  # ---------------------------------------------------------------- loops inside atomic and d_step
  #
  # `atomic { do ... od }`: the loop is inside the block, so the process keeps
  # the exclusive control (the engine's Edge.Atomic) after every iteration, and
  # `d_step { do ... od }` is one step. When the loop is the first statement of
  # the block its head is the control location in front of the block, which is
  # outside it, and the frontend, which called an edge atomic when its target
  # lies inside the block, let go of the control at the back edge. SPIN's
  # decision is per transition: the back edge of a loop inside the block keeps
  # the control, the break leaves it.

  Scenario Outline: a loop at the start of an atomic or d_step block keeps the exclusive control until it is left, as pan
    Given the Promela file "testdata/promela/<file>"
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And property "deadlock" is "verified" with evidence "exhaustive"
    And property "assert" is "verified" with evidence "exhaustive"

    Examples:
      | file                          |
      | atomic-loop-entry.pml         |
      | dstep-loop-entry.pml          |
      | atomic-loop-merged-break.pml  |
      | atomic-loop-goto-head.pml     |
      | dstep-loop-merged-break.pml   |
      | dstep-loop-first-break.pml    |

  # A break that is the last statement of an option leaves no edge of its own: its node is
  # merged with the exit of the loop. When the loop is the only statement of an outer
  # loop's option the exit is the loop's own head, and the edge before the break must not be
  # taken for a back edge: it leaves the loop, and the control is given up after it.
  Scenario: an atomic loop that leaves at once gives up the exclusive control, as pan
    Given the Promela file "testdata/promela/atomic-loop-first-break-leaves.pml"
    When I execute "mcd check --promela <model> --sweep --no-timing"
    Then the command exits with 0
    And property "assert" is "violated" with evidence "exhaustive"

  @spin
  Scenario Outline: pandiff agrees with pan on a loop at the start of an atomic or d_step block
    Given spin is installed
    And the Promela file "testdata/promela/<file>"
    When I run pandiff on the model
    Then pandiff reports agreement on the verdict, the error class and the state count

    Examples:
      | file                    |
      | atomic-loop-entry.pml   |
      | dstep-loop-entry.pml    |

  # The head of such a loop is the location in front of the block. When another
  # alternative starts there too (the block is one option of an outer if/do),
  # the lock bit alone cannot keep that alternative out of the block, and pan
  # needs a state of its own for the inside. The frontend refuses the shape by
  # name; it used to answer for a model in which the alternative was available
  # in the block (a false assertion violation here, where pan finds none).
  Scenario: a loop at the start of an atomic block that shares its entry with another option is refused by name
    Given the Promela file "testdata/promela/atomic-loop-option.pml"
    When I execute "mcd parse --promela <model>"
    Then the command exits with 2
    And the rejection has kind "outside-subset" and status "not-executed"
    And the rejection mentions "shares its entry with another alternative"

  # An atomic block whose loop never ends is a sequence that never ends. The
  # breadth-first searches (--bfs, the estimate that mc_estimate runs first) used
  # to copy the chain of moves at every step of such a sequence and died of
  # memory in seconds; they answer inconclusive at the bound of an atomic
  # sequence now, in memory linear in the bound. (Written after the fix: the
  # unfixed engine would take the test process down.)
  Scenario: a breadth-first search over an atomic sequence that never ends is bounded
    Given the Promela file "testdata/unbounded/atomic-never-ends.pml"
    When I execute "mcd check --promela <model> --bfs --no-timing"
    Then the command exits with 0
    And property "deadlock" is "inconclusive" with evidence "bounded"

  Scenario: the estimate of a model with an atomic sequence that never ends comes back
    Given the Promela file "testdata/unbounded/atomic-never-ends.pml"
    When I execute "mcd check --promela <model> --estimate --estimate-ms 2000 --no-timing"
    Then the command exits with 0

  @spin
  Scenario Outline: pandiff agrees with pan on the models that read _nr_pr without run
    Given spin is installed
    And the Promela file "testdata/promela/<file>"
    When I run pandiff on the model
    Then pandiff reports agreement on the verdict, the error class and the state count

    Examples:
      | file               |
      | nrpr-active.pml    |
      | nrpr-order.pml     |
      | nrpr-youngest.pml  |
      | nrpr-mixed.pml     |
      | nrpr-unread.pml    |
