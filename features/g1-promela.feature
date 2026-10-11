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

  # ---------------------------------------------------------------- resource limits (issue #9)
  # Model text is untrusted input. Shapes that amplify in the frontend — deep
  # nesting, long operator chains, macro, inline and typedef fan-out, huge
  # arrays, many heavy expressions — used to exhaust the stack or the memory
  # with a fatal error that no recover sees: exit 2, no report, and a running
  # mcd serve dead with the request. Every such shape now meets a stated limit
  # and is refused by name, as a rejection. The inputs are generated by the
  # step, since the smallest of them is a few kilobytes.
  Scenario Outline: a shape that amplifies beyond a limit is refused by name, not with a crash
    Given a generated Promela model "<shape>" of size <n>
    When I execute "mcd parse --promela <model>"
    Then the command exits with 2
    And the rejection has kind "outside-subset" and status "not-executed"
    And the rejection mentions "<mentions>"
    And the rejection mentions "limit"

    Examples:
      | shape           | n    | mentions               |
      | nested-parens   | 1001 | 1000                   |
      | plus-chain      | 1001 | 1000                   |
      | paren-chain     | 40   | 1000                   |
      | nested-if       | 1001 | statements nested      |
      | if-parens       | 1001 | #if                    |
      | define-doubling | 23   | macro expansion        |
      | define-chain    | 5000 | macro expansion nested |
      | define-fanout   | 3000 | macro expansion        |
      | inline-doubling | 22   | inline expansion       |
      | inline-fanout   | 3000 | inline expansion       |
      | typedef-fanout  | 40   | fields                 |
      | index-nest      | 1001 | 1000                   |
      | huge-array-init | 0    | 65535                  |
      | many-arrays     | 2000 | state vector           |
      | chan-arrays     | 1    | 255                    |
      | wide-chans      | 250  | state vector           |
      | chains          | 100  | IR                     |

  Scenario Outline: the same shapes within the limits are parsed
    Given a generated Promela model "<shape>" of size <n>
    When I execute "mcd parse --promela <model>"
    Then the command exits with 0

    Examples:
      | shape           | n    |
      | nested-parens   | 1000 |
      | plus-chain      | 1000 |
      | wide-mixed      | 400  |
      | nested-if       | 200  |
      | if-parens       | 1000 |
      | define-doubling | 10   |
      | define-chain    | 500  |
      | inline-doubling | 6    |
      | typedef-fanout  | 4    |
      | chains          | 3    |

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

  # ---------------------------------------------------------------- the time budget holds on every path (issue #8)
  # The time budget is a deadline. It used to be looked at only when a new
  # state was stored, so an atomic sequence that branches without storing (2^40
  # paths here) ran past every budget, in every search mode, and mcd serve never
  # answered. A budget check now fires by work done: the run stops in about the
  # budget, inconclusive with evidence unknown, whatever the mode.
  Scenario Outline: an atomic sequence that never stores a state still stops at the time budget
    Given the Promela file "testdata/budget/time-budget.pml"
    When I execute "mcd check --promela <model> <flags> --budget-ms 300 --no-timing"
    Then the command exits with 0
    And property "deadlock" is "inconclusive" with evidence "unknown"
    And the reason for "deadlock" mentions "time budget"
    And the search is not complete
    And the command finished within 5 seconds

    Examples:
      | flags       |
      |             |
      | --bfs       |
      | --por       |
      | --workers 4 |

  Scenario: the estimate stops at its time limit inside an atomic sequence
    Given the Promela file "testdata/budget/time-budget.pml"
    When I execute "mcd check --promela <model> --estimate --estimate-ms 300 --no-timing"
    Then the command exits with 0
    And the command finished within 5 seconds

  # ---------------------------------------------------------------- memory of a breadth-first descent (issue #7)
  # A breadth-first expansion descends an atomic sequence inside one step and
  # kept, for every state it stored, a copy of the chain of moves that led
  # there. An atomic loop that can leave at every step stores one state per
  # level with a chain of its own length: n²/2 references, gigabytes and a
  # fatal out-of-memory before any budget spoke. The chain is now kept as a
  # shared node and counted in the memory estimate, so the run allocates
  # little more than it reports and a memory budget stops it.
  Scenario Outline: a breadth-first run over an atomic loop that exits at every step allocates what it reports
    Given the Promela file "testdata/budget/bfs-memory.pml"
    When I execute "mcd check --promela <model> <flags> --budget-states 3000 --no-timing" measuring the bytes allocated
    Then the command exits with 0
    And property "<property>" is "inconclusive" with evidence "bounded"
    And the reason for "<property>" mentions "state budget"
    # Measured on an idle server: 3.1x for --bfs, 3.5-3.8x for the graph of
    # --ctl (TotalAlloc against the estimate of that property; the copies
    # were 1 400x). The estimate compared is the one of the search that made
    # the chains: the DFS counters of deadlock say nothing about the graph.
    And the bytes the command allocated are at most 10 times the memory estimate of "<property>"

    Examples:
      | flags            | property |
      | --bfs            | deadlock |
      | --ctl AG(x>=0)   | ctl1     |

  Scenario: a memory budget stops the breadth-first run where it used to die
    Given the Promela file "testdata/budget/bfs-memory.pml"
    When I execute "mcd check --promela <model> --bfs --budget-mem-mb 8 --no-timing"
    Then the command exits with 0
    And property "deadlock" is "inconclusive" with evidence "unknown"
    And the reason for "deadlock" mentions "memory budget"
    And the search is not complete

  # ---------------------------------------------------------------- goto and atomic blocks (issue #6)
  # A forward goto pre-allocates the location of its label; when the label
  # was then defined inside an atomic block, the location took the blocks of
  # that point instead of the blocks of the position it stands for, so the
  # head of an outer do counted as inside the block: the refusal of a loop at
  # the start of an atomic block that shares its entry did not fire, and the
  # outer alternative was available while the process held the control — a
  # false assert violation where pan finds none. The location now takes the
  # blocks of its position. Whether the edge that ends in a goto keeps the
  # control is told by where the goto and its label are written: both inside
  # a block keeps it (a jump between two atomic blocks included), either
  # outside gives it up, and a jump into a block gets the control at the
  # block's first statement, as pan. A goto across a d_step boundary is
  # refused as SPIN refuses it; a goto that keeps the control into the
  # location in front of a block is refused when that location has another
  # alternative, like the loop without the goto. The same family: a label on
  # the first statement of an if/do option is lowered to the point of
  # choice, so a goto to it would make the other options available after
  # the jump (pan jumps to the statement itself); such a goto is refused.
  Scenario Outline: a goto to a location where another alternative would be available after the jump is refused
    Given the Promela file "testdata/promela/goto-atomic/<file>"
    When I execute "mcd parse --promela <model>"
    Then the command exits with 2
    And the rejection has kind "outside-subset" and status "not-executed"
    And the rejection mentions "<mentions>"
    And the rejection mentions "line <line>"

    Examples:
      | file                              | line | mentions                            |
      | m1-issue.pml                      | 5    | loop at the start of an atomic      |
      | m6-goto-sibling-block.pml         | 5    | loop at the start of an atomic      |
      | m7-backward-goto-atomic-loop.pml  | 4    | loop at the start of an atomic      |
      | m9-gpt-inside-shared-head.pml     | 6    | loop at the start of an atomic      |
      | m22-goto-to-block-start-shared.pml | 4    | goto to the start of an atomic      |
      | m33-goto-option-head-shared.pml   | 3    | label on the first statement of an option |
      | m34-label-on-atomic-option-head.pml | 4  | label on the first statement of an option |
      | m36-goto-if-option-head-shared.pml | 3   | label on the first statement of an option |
      | m37-backward-goto-option-head.pml | 4    | label on the first statement of an option |

  Scenario Outline: a goto across a d_step boundary is refused as SPIN refuses it
    Given the Promela file "testdata/promela/goto-atomic/<file>"
    When I execute "mcd parse --promela <model>"
    Then the command exits with 2
    And the rejection has kind "semantic" and status "not-executed"
    And the rejection mentions "d_step boundary"

    Examples:
      | file                             |
      | m4-issue-dstep.pml               |
      | m10-goto-out-of-dstep.pml        |
      | m11-goto-into-dstep-own.pml      |
      | m20-goto-into-dstep-first.pml    |
      | m21-goto-into-dstep-first-loop.pml |

  @spin
  Scenario Outline: a goto into, out of, between or inside atomic blocks agrees with pan
    Given spin is installed
    And the Promela file "testdata/promela/goto-atomic/<file>"
    When I run pandiff on the model
    Then pandiff reports agreement on the verdict, the error class and the state count

    Examples:
      | file                                       |
      | m2-plan-fixture.pml                        |
      | m3-goto-option-into-block.pml              |
      | m3b-goto-straight-into-block.pml           |
      | m5-goto-loop-own-block.pml                 |
      | m8-gpt-mid-block.pml                       |
      | m12-goto-sibling-no-shared.pml             |
      | m13-goto-out-of-atomic.pml                 |
      | m14-backward-goto-first-option-inside.pml  |
      | m15-goto-option-from-loop-head-inside.pml  |
      | m16-nonfirst-goto-into-atomic.pml          |
      | m17-backward-goto-atomic-loop-own.pml      |
      | m18-first-goto-out-of-atomic.pml           |
      | m19-nonfirst-goto-atomic-to-atomic.pml     |
      | m23-goto-opens-own-atomic.pml              |
      | m24-goto-chain-label-on-goto.pml           |
      | m25-goto-chain-inside.pml                  |
      | m26-nested-atomic-backward-goto.pml        |
      | m27-nested-dstep-backward-goto.pml         |
      | m28-plain-block-after-atomic.pml           |
      | m29-goto-chain-leaves-block.pml            |
      | m30-label-on-break-exit-outside.pml        |
      | m31-plain-block-chain-out.pml              |
      | m32-label-on-atomic-block.pml              |
      | m35-goto-chain-into-block-entry.pml        |
      | m38-goto-chain-through-outside.pml         |
      | m39-label-on-block-and-inner-do.pml        |
      | m40-goto-chain-alias-shortcut.pml          |
      | m41-goto-inside-same-dstep.pml             |
      | m42-goto-single-option-head.pml            |
      | m43-keep-loop-through-outside-goto.pml     |
