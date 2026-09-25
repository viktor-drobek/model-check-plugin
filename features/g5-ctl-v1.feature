Feature: G5 CTL labelling, vacuity hints, mc_estimate growth model, Promela v1
  Plan 14 §9, row G5: CTL model checking by graph labelling over the fully stored
  reachable graph (basis EX / EU / EG, time linear in |graph| × |formula|); vacuity
  hints (FR-011); the growth model of `mc_estimate` (§6, FR-022); and the v1 Promela
  subset of §5.2 — `inline`, `typedef`, `provided`, `_nr_pr`, dynamic `run` (from any
  process body and inside loops), channel arrays, channel-typed parameters and message
  fields, `pc_value`. Exit criterion, verbatim: E6 passes; `client_server.pml` parses;
  the differential set is the whole corpus inside the subset.

  Vocabulary (the same words as the report and the MCP answers):

  - CTL syntax: `AG f`, `AF f`, `AX f`, `EG f`, `EF f`, `EX f`, `A[f U g]`, `E[f U g]`,
    `!`, `&&`, `||`, `->`, `<->`, `true`, `false`, parentheses. An atom is a boolean
    state expression over the globals — an identifier (read as "non-zero", Promela
    convention), a comparison or arithmetic over variables and array elements,
    `len(ch)` / `empty(ch)` / `nempty(ch)` / `full(ch)` / `nfull(ch)`, `pc_value(p)`
    (the control location of process p), or `P@label` (process P is at the location
    named `label`) — or a symbol `#define`d in the Promela input, expanded before
    parsing. CTL and LTL are separate logics with separate parsers: the engine never
    rewrites a CTL formula into LTL or an LTL formula into CTL, and says which logic
    it used in `temporal.logic` (`ctl` or `ltl`).
  - normalisation: the labeller works in the basis `EX`, `E[f U g]`, `EG f`; the other
    operators are the standard equivalences `EF f = E[true U f]`, `AX f = !EX !f`,
    `AF f = !EG !f`, `AG f = !EF !f = !E[true U !f]`, `A[f U g] = !(E[!g U (!f && !g)]) && !EG !g`.
    The normalised formula is printed in `temporal.normalised`.
  - the graph: CTL is decided on the *fully stored* reachable graph of the model with
    its successor lists (the same stored states, and therefore the same state count,
    as `mcd check --sweep`; intermediate states of an `atomic` sequence are not stored,
    G1 rule). A state with no enabled transition (a deadlock or a terminated system)
    gets a self-loop, so the transition relation is total, as CTL semantics requires;
    the report says so in `temporal.note`. If a budget stops the graph short, the
    property is `inconclusive` — CTL is never decided on a partial graph.
  - witnesses and counterexamples per operator (the division is exhaustive; anything
    not listed gets no run and the answer says `not available` with the reason):
      * formula HOLDS at the initial state and its top operator is `EF`, `EX` or
        `E[..U..]` → a finite witness path from the initial state to the state that
        fulfils it;
      * formula HOLDS at the initial state and its top operator is `EG` → a lasso
        (prefix + loop, the same `loop.start` / `loop.steps` shape as an LTL cycle
        counterexample), every state of which satisfies the operand;
      * formula FAILS at the initial state and it is `AG f` → a finite counterexample
        path to a reachable state where `f` is false (shortest such path);
      * formula FAILS at the initial state and it is `AX f` → a path of one step to a
        successor where `f` is false;
      * formula FAILS at the initial state and it is `AF f` or `A[f U g]` → a lasso on
        which the required state never comes (the witness of the equivalent `EG`);
      * every other case — `EF`/`EX`/`EU`/`EG` that fails, `AG`/`AF`/`AX`/`AU` that
        holds, or a formula whose top operator is a boolean connective or an atom —
        carries no run: the answer says `not available` and why (a universal property
        that holds is justified by the complete graph, not by one run; a failing
        existential property has no run to show).
      A nested failure is not opened up: the counterexample of `AG EF p` ends at the
      state from which `p` is no longer reachable, and the answer says that the reason
      the inner `EF p` fails is not a finite run.
  - CTL fairness is out of scope (plan 14 §4.2). A `ctl` property requested with
    `fairness` `weak` or `strong` is `not-executed` with evidence `unknown` and a
    reason saying so; it is never silently checked without fairness.
  - vacuity (FR-011) is a hint, never a verdict: `warnings` may say that an atom is
    never true, or never false, in the reachable graph; for `[](a -> b)` and
    `AG(a -> b)` an antecedent that is never true sets `vacuous: true` and names the
    atom. The status and evidence of the property are exactly what the search found;
    a vacuous `verified` stays `verified`.
  - `mc_estimate` (plan 14 §6, FR-022): partial breadth-first levels within a time
    budget, states per level, a fitted growth factor, a projection to a requested
    target depth, and a size class from the A4 bounds of plan 14 §12 — `small`
    (≤ 1e5 states and depth), `medium` (≤ 1e6 states, depth 1e6, state vector
    ≤ 128 bytes, 60 s, 1 GiB) or `large` (beyond them) — with a recommendation.
    An estimate is never a verification result and carries no property status.
  - the pan numbers quoted were produced by SPIN 6.5.2, `spin -a -o1 -o2 -o3`,
    `gcc -O2 -DNOREDUCE`, `./pan -c0 -m1000000` on 2026-09-25. `CH15/client_server.pml`
    is compiled for pan with `return` renamed (`return` is a reserved word in SPIN
    6.5.2 and the file predates it); the engine reads the file as written.

  # ------------------------------------------------------- exit criterion: E6, CTL

  Scenario: E6 — AG EF idle on CH14/version1 through the CLI, with the witness rules stated
    Given the corpus model "CH14/version1"
    When I run mcd check on it with the CTL formula "AG EF subscriber@Idle"
    Then the exit code is 0
    And the property "ctl1" has status "verified" with evidence "exhaustive"
    And the property "ctl1" reports temporal logic "ctl"
    And the property "ctl1" reports the normalised formula "!E[true U !E[true U (pc(0) == 0)]]"
    And the property "ctl1" has no counterexample and says the witness is "not available"
    And the reason of "ctl1" says that a universal property that holds is justified by the complete graph
    And the property "ctl1" counts 9 states

  Scenario: E6 — the same question through MCP, as property kind ctl
    Given a session in which the Promela model "CH14/version1" was parsed
    When I call "mc_check" in that session with properties:
      | id   | kind | formula               |
      | idle | ctl  | AG EF subscriber@Idle |
    Then the call is not an error
    And the answer property "idle" has status "verified" with evidence "exhaustive"
    And the answer property "idle" has temporal logic "ctl"
    And the answer property "idle" has no field "witness"

  Scenario: a ctl request is never answered with an LTL automaton
    Given a session in which the Promela model "CH14/version1" was parsed
    When I call "mc_check" in that session with properties:
      | id   | kind | formula               |
      | idle | ctl  | AG EF subscriber@Idle |
    Then the answer property "idle" has temporal logic "ctl"
    And the answer property "idle" has no temporal field "automaton_states"
    And the answer property "idle" has no temporal field "negated"

  Scenario: an AG that fails names the state it fails in and the path to it
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "AG (x < 2)"
    Then the exit code is 0
    And the property "ctl1" has status "violated" with evidence "exhaustive"
    And the counterexample of "ctl1" is finite and ends in a state where "x" is 2

  Scenario: EF is verified with a finite witness path
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "EF (x == 2)"
    Then the property "ctl1" has status "verified" with evidence "exhaustive"
    And the witness of "ctl1" is finite and ends in a state where "x" is 2

  Scenario: E[p U q] is verified with a finite witness whose states satisfy p until q
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "E[(x < 2) U (x == 2)]"
    Then the property "ctl1" has status "verified" with evidence "exhaustive"
    And the witness of "ctl1" is finite and ends in a state where "x" is 2

  Scenario: an EG witness is a lasso, not a finite path
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "EG (x < 3)"
    Then the property "ctl1" has status "verified" with evidence "exhaustive"
    And the witness of "ctl1" has a loop
    And the loop of the witness of "ctl1" is closed
    And every state of the witness of "ctl1" satisfies "x < 3"

  Scenario: an existential property that fails carries no witness and says so honestly
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "EF (x == 9)"
    Then the property "ctl1" has status "violated" with evidence "exhaustive"
    And the property "ctl1" has no counterexample and says the witness is "not available"
    And the reason of "ctl1" says that a failing existential property has no run to show

  Scenario: AF that fails is refuted by a lasso that never reaches the goal
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "AF (x == 2)"
    Then the property "ctl1" has status "violated" with evidence "exhaustive"
    And the counterexample of "ctl1" has a loop
    And the loop of the counterexample of "ctl1" is closed

  Scenario: a nested failure is not opened up — AG EF says where, not why
    Given the test model "ctl-trap.pml"
    When I run mcd check on it with the CTL formula "AG EF (x == 0)"
    Then the property "ctl1" has status "violated" with evidence "exhaustive"
    And the counterexample of "ctl1" is finite
    And the reason of "ctl1" says that the nested subformula's failure is not a finite run

  Scenario: CTL fairness is out of scope and is refused, never silently dropped
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "AG EF (x == 0)" and fairness "weak"
    Then the exit code is 0
    And the property "ctl1" has status "not-executed" with evidence "unknown"
    And the reason of "ctl1" mentions "fairness"
    And the reason of "ctl1" mentions "plan 14 §4.2"

  Scenario: a CTL formula that does not parse rejects the input instead of guessing
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "AG U x"
    Then the exit code is 2
    And the rejection has kind "ctl" and status "not-executed"

  Scenario: a budget that stops the graph short makes the CTL property inconclusive
    Given the IR model "testdata/ir/counters-10-5.json"
    When I run mcd check on the IR with the CTL formula "AG EF (P0.c == 0)" and budget-states 100
    Then the property "ctl1" has status "inconclusive" with evidence "bounded"
    And the reason of "ctl1" mentions "state budget"

  # ------------------------------------------------------------------ vacuity (FR-011)

  Scenario: an implication whose antecedent is never true is flagged vacuous, verdict unchanged
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "AG ((x == 7) -> (x == 8))"
    Then the property "ctl1" has status "verified" with evidence "exhaustive"
    And the property "ctl1" is marked vacuous naming the atom "(x == 7)"
    And the warnings mention "never true"

  Scenario: the same vacuity hint for an LTL implication, still only a hint
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the LTL formula "[]((x == 7) -> (x == 8))"
    Then the property "ltl1" has status "verified" with evidence "exhaustive"
    And the property "ltl1" is marked vacuous naming the atom "(x == 7)"

  Scenario: an atom that is never false is reported as a hint and does not change the verdict
    Given the test model "ctl-switch.pml"
    When I run mcd check on it with the CTL formula "AG (x >= 0)"
    Then the property "ctl1" has status "verified" with evidence "exhaustive"
    And the warnings mention "never false"
    And the property "ctl1" is not marked vacuous

  Scenario: mc_lint_property lints a CTL formula and names the vacuity candidate
    Given a session in which the Promela model "CH3/counter3.pml" was parsed
    When I call "mc_lint_property" in that session with kind "ctl" and formula "AG ((count == 250) -> EF (count == 0))"
    Then the call is not an error
    And the lint says the logic is "ctl"
    And the lint atoms are "(count == 250)", "(count == 0)"
    And the lint notes mention "antecedent"

  Scenario: mc_lint_property refuses a CTL formula that is not CTL, without guessing an LTL reading
    Given a session in which the Promela model "CH3/counter3.pml" was parsed
    When I call "mc_lint_property" in that session with kind "ctl" and formula "[]<>(count == 0)"
    Then the call is not an error
    And the lint reports a type error mentioning "CTL"

  # ------------------------------------------------------------- mc_estimate growth model

  Scenario: mc_estimate on the counters IR reports levels, a growth factor and a size class
    Given a session in which the IR "testdata/ir/counters-10-5.json" was parsed
    When I call "mc_estimate" in that session with 1500 ms and target depth 12
    Then the call is not an error
    And the estimate has at least 3 growth levels
    And the estimate growth rate is greater than 1
    And the estimate projects a state count for depth 12
    And the estimate size class is "small"
    And the estimate recommendation mentions "plan 14 §12"
    And the estimate says it is not a verification result

  Scenario: a model that fits in the budget is reported complete and exact, not projected
    Given a session in which the Petri net "testdata/petri/petrinet1.json" was parsed
    When I call "mc_estimate" in that session with 1000 ms and target depth 20
    Then the estimate is complete
    And the estimate projection evidence is "exhaustive"
    And the estimate size class is "small"

  Scenario: the CLI exposes the same estimate under --estimate
    Given the IR model "testdata/ir/counters-10-5.json"
    When I run mcd estimate on the IR with 1500 ms and target depth 12
    Then the exit code is 0
    And the estimate output names the size class "small"
    And the estimate output has a growth rate and per-level counts

  # ------------------------------------------------- Promela v1: inline, typedef, provided

  Scenario: inline with arguments is substituted and CH3/inline.pml agrees with pan
    Given the corpus model "CH3/inline.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 5
    And the property "assert" has status "violated" with evidence "exhaustive"
    And the counterexample step 1 is at line 2 of the inline body

  Scenario: an inline that declares a local variable is accepted (CH3/inline2.pml)
    Given the corpus model "CH3/inline2.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 6
    And there is no violated property

  Scenario: an inline used inside an atomic option (CH2/prodcons2.pml) agrees with pan
    Given the corpus model "CH2/prodcons2.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 14
    And there is no violated property

  Scenario: typedef structs are flattened to fields and CH3/typedef.pml agrees with pan
    Given the corpus model "CH3/typedef.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 5
    And the IR declares the variables "goo.a[3]", "goo.fld1", "goo.fld2.f", "goo.fld2.g", "goo.p[3]", "goo.b"
    And the IR variable "foo.f" has initial value 3

  Scenario: provided (expr) guards every edge of the process — CH3/toggle.pml as pan
    Given the corpus model "CH3/toggle.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 6

  Scenario: CH5/pathfinder.pml — the priority inversion is a deadlock, with pan's verdict and count
    Given the corpus model "CH5/pathfinder.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 12
    And the property "deadlock" has status "violated" with evidence "exhaustive"
    And the counterexample of "deadlock" ends in a state where "mutex" is 2

  # ----------------------------------------------- Promela v1: dynamic run, _nr_pr, channels

  Scenario: the static runs of CH3/you_run2.pml keep their G1 counts after the dynamic-run rework
    Given the corpus model "CH3/you_run2.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 14

  Scenario: a model without run keeps its count too (CH3/you_run.pml)
    Given the corpus model "CH3/you_run.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 7

  Scenario: run inside a loop instantiates processes in pan's pid order (test model nrpr.pml)
    Given the test model "nrpr.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 7
    And the property "assert" has status "verified" with evidence "exhaustive"

  Scenario: _nr_pr counts the live processes and falls back when one terminates
    Given the test model "nrpr.pml"
    When I run mcd check on it with the CTL formula "EF (_nr_pr == 3)"
    Then the property "ctl1" has status "verified" with evidence "exhaustive"

  Scenario: run beyond the engine's process pool is a declared bound, not a wrong answer
    Given the corpus model "CH3/splurge.pml"
    When I run mcd check on it in sweep mode with max-procs 4
    Then the exit code is 0
    And the property "deadlock" has status "inconclusive" with evidence "bounded"
    And the reason of "deadlock" mentions "max-procs"

  Scenario: a channel array and channel-typed parameters — CH9/leader.pml parses and agrees with pan
    Given the corpus model "CH9/leader.pml"
    When I run mcd check on it in sweep mode with the unlimited budget
    Then the exit code is 0
    And the state count is 41692
    And there is no violated property

  Scenario: channels carried in messages — CH3/rendezvous2.pml parses and agrees with pan
    Given the corpus model "CH3/rendezvous2.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 5

  Scenario: CH15/client_server.pml parses and its state count equals pan's
    Given the corpus model "CH15/client_server.pml"
    When I run mcd check on it in sweep mode with the unlimited budget
    Then the exit code is 0
    And the state count is 191200
    And there is no violated property

  Scenario: pc_value is a state expression — CH4/pcval.pml parses and agrees with pan
    Given the corpus model "CH4/pcval.pml"
    When I run mcd check on it in sweep mode
    Then the exit code is 0
    And the state count is 11

  Scenario: the constructs still outside the subset are refused by name, not half-executed
    When I parse the corpus model "<model>" with mcd parse
    Then the exit code is 2
    And the rejection kind is "outside-subset" and it names "<construct>" at line <line>

    Examples:
      | model              | construct              | line |
      | CH17/simple1.pr    | c_code                 | 1    |
      | CH3/pots.pml       | unless                 | 20   |
      | CH14/v14_16.pml    | remote reference (P@label) | 22 |

  # --------------------------------------------------------------- differential corpus

  @spin
  Scenario Outline: the engine and pan agree on every corpus file now inside the subset
    Given spin and gcc are installed
    And the model "<model>" for the differential check
    When I compare the engine with pan
    Then pan and the engine agree on the verdict, the error class and the state count

    Examples: chapter 2-3 files unlocked by v1
      | model               |
      | CH2/prodcons2.pml   |
      | CH3/inline.pml      |
      | CH3/inline2.pml     |
      | CH3/typedef.pml     |
      | CH3/toggle.pml      |
      | CH3/rendezvous2.pml |

    Examples: chapters 4, 5, 9, 14, 15
      | model                    |
      | CH4/pcval.pml            |
      | CH5/pathfinder.pml       |
      | CH5/diskhead.pml         |
      | CH9/leader.pml           |
      | CH14/version1            |
      | CH14/version2            |
      | CH14/version4            |
      | CH15/client_server.pml   |

  @spin
  Scenario: the G4 leader triple now runs on the original CH12/leader, not on the rewrite
    Given spin and gcc are installed
    And the model "CH12/leader" for the differential triple with "-D N=3"
    When I compare the engine, the SPIN claim and pan for "<>[]oneLeader" with fairness "none"
    Then all three agree on the verdict
    When I compare the engine, the SPIN claim and pan for "<>[]oneLeader" with fairness "weak"
    Then all three agree on the verdict
