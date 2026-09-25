Feature: G5 CTL labelling, vacuity hints, mc_estimate growth model, Promela v1
  Plan 14 §9, row G5: CTL model checking by graph labelling over the fully stored
  reachable graph (basis EX / EU / EG, time linear in |graph| × |formula|); vacuity
  hints (FR-011); the growth model of `mc_estimate` (§6, FR-022); and the v1 Promela
  subset of §5.2 — `inline`, `typedef`, `provided`, `_nr_pr`, dynamic `run` (from any
  process body and inside loops), channel arrays, channel-typed parameters and message
  fields, `pc_value`. Exit criterion, verbatim: E6 passes; `client_server.pml` parses;
  the differential set is the whole corpus inside the subset.

  The step wording of this feature is its own (every step reads "G5 …" or a phrase no
  other feature uses): godog runs strict, so two features that shared a step text
  would make it ambiguous rather than shared.

  Vocabulary (the same words as the report and the MCP answers):

  - CTL syntax: `AG f`, `AF f`, `AX f`, `EG f`, `EF f`, `EX f`, `A[f U g]`, `E[f U g]`,
    `!`, `&&`, `||`, `->`, `<->`, `true`, `false`, parentheses. An atom is a boolean
    state expression over the globals — an identifier (read as "non-zero", Promela
    convention), a comparison or arithmetic over variables and array elements,
    `len(ch)` / `empty(ch)` / `nempty(ch)` / `full(ch)` / `nfull(ch)`, `_nr_pr`,
    `pc_value(p)` (the control location of process p), or `P@label` (process P is at
    the location named `label`) — or a symbol `#define`d in the Promela input,
    expanded before parsing. CTL and LTL are separate logics with separate parsers:
    the engine never rewrites a CTL formula into LTL or an LTL formula into CTL, and
    says which logic it used in `temporal.logic` (`ctl` or `ltl`).
  - normalisation: the labeller works in the basis `EX`, `E[f U g]`, `EG f`; the other
    operators are the standard equivalences `EF f = E[true U f]`, `AX f = !EX !f`,
    `AF f = !EG !f`, `AG f = !E[true U !f]`,
    `A[f U g] = !E[!g U (!f && !g)] && !EG !g`. The normalisation is printed in
    `temporal.normalised`, so a reader can check which question was answered.
  - the graph: CTL is decided on the *fully stored* reachable graph of the model with
    its successor lists. It is built by the same breadth-first search, with the same
    rule for storing states, that `mcd check --sweep` uses — intermediate states of an
    `atomic` sequence are not stored (the G1 rule) — so its state count is that one;
    the scenarios below check the two against each other and against pan model by
    model, and that is the ground for the claim, not a proof for all models. A state
    with no enabled transition (a deadlock or a terminated system) gets a self-loop,
    so the transition relation is total, as CTL semantics requires; the report says so
    in `temporal.note`. A state whose expansion a budget cut short gets no self-loop:
    it has no move *yet*, and the graph is incomplete anyway. If a budget stops the
    graph short, the property is `inconclusive` — CTL is never decided on a partial
    graph.
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
      * formula FAILS at the initial state and it is `AF f` → a lasso on which the
        required state never comes (the witness of the equivalent `EG !f`);
      * formula FAILS at the initial state and it is `A[f U g]` → the one case with two
        shapes, because its negation is a disjunction: a finite path to a state where
        `f` failed before `g` ever held (`E[!g U (!f && !g)]`), and otherwise a lasso on
        which `g` never comes (`EG !g`). The finite path is preferred: it is shorter;
      * every other case — `EF`/`EX`/`EU`/`EG` that fails, `AG`/`AF`/`AX`/`AU` that
        holds, or a formula whose top operator is a boolean connective or an atom —
        carries no run: the answer says `not available` and why (a universal property
        that holds is justified by the complete graph, not by one run; a failing
        existential property has no run to show).
      A nested failure is not opened up: the counterexample of `AG EF p` ends at the
      state from which `p` is no longer reachable, and the answer says that the reason
      the inner `EF p` fails is not a finite run.
      One more answer exists and is not part of the division above: if the labelling
      says a run exists and the search for it comes back empty, the answer says that
      this is an engine defect and asks for the case to be reported. It is written
      down so that a defect can never borrow the words of a principled absence.
  - CTL fairness is out of scope (plan 14 §4.2). A `ctl` property requested with
    `fairness` `weak` or `strong` is `not-executed` with evidence `unknown` and a
    reason saying so; it is never silently checked without fairness.
  - vacuity (FR-011) is a hint, never a verdict: `warnings` may say that an atom is
    never true, or never false, in the reachable graph; for `[](a -> b)` and
    `AG(a -> b)` an antecedent that is never true sets `vacuous: true` and names the
    atom. The status and evidence of the property are exactly what the search found;
    a vacuous `verified` stays `verified`.
  - `mc_estimate` (plan 14 §6, FR-022): one breadth-first search under a time budget
    reports the states within each depth it finished expanding; from the last of
    those levels a growth factor is fitted and projected to a requested target depth;
    and the result is placed in a size class from the A4 bounds of plan 14 §12 —
    `small` (≤ 1e5 states), `medium` (≤ 1e6 states with a state vector ≤ 128 bytes)
    or `large` (beyond them) — with a recommendation. An estimate is never a
    verification result and carries no property status.
  - "inside the subset" and "in the differential set" are two different things, and
    G5 is the step where they come apart. Inside the subset = the frontend accepts the
    file. In the differential set = the engine's verdict, error class and state count
    are compared with pan's. `CH4/pcval.pml` is inside the subset and outside the
    differential set: `pc_value(n)` yields *this engine's* control-location index,
    pan yields its own internal state number, the two numberings are built differently,
    and a model whose behaviour depends on the number therefore has a different state
    space here — by construction, not by accident. The same separation holds for the
    three byte-overflow files of G1, for `wc.pml`, and for `splurge`/`splurge2`; every
    exclusion is named with its reason in steps/g5-confirmation.md.
  - the pan numbers quoted were produced by SPIN 6.5.2, `spin -a -o1 -o2 -o3`,
    `gcc -O2 -DNOREDUCE`, `./pan -c0 -m1000000` on 2026-09-25. `CH15/client_server.pml`
    is compiled for pan with `return` renamed (`return` is a reserved word in SPIN
    6.5.2 and the file predates it); the engine reads the file as written.

  # ------------------------------------------------------- exit criterion: E6, CTL

  Scenario: E6 — AG EF idle on CH14/version1 through the CLI, with the witness rule stated
    Given the model "CH14/version1" of the corpus
    When I check it with the CTL formula "AG EF subscriber@Idle"
    Then the run exits with 0
    And the G5 property "ctl1" is "verified" with evidence "exhaustive"
    And the G5 property "ctl1" reports logic "ctl"
    And the G5 property "ctl1" reports the normalised formula "!E[true U !E[true U (pc(0) == 0)]]"
    And the G5 property "ctl1" carries no run and says so
    And the note of the G5 property "ctl1" says that a universal property that holds is justified by the complete graph
    And the G5 property "ctl1" counts 9 states

  Scenario: E6 — the same question through MCP, as property kind ctl
    Given a G5 session with the corpus model "CH14/version1" parsed
    When I call mc_check in the G5 session with properties:
      | id   | kind | formula               |
      | idle | ctl  | AG EF subscriber@Idle |
    Then the G5 call is not an error
    And the G5 answer property "idle" is "verified" with evidence "exhaustive"
    And the G5 answer property "idle" reports logic "ctl"
    And the G5 answer property "idle" has no witness

  Scenario: a ctl request is never answered with an LTL automaton
    Given a G5 session with the corpus model "CH14/version1" parsed
    When I call mc_check in the G5 session with properties:
      | id   | kind | formula               |
      | idle | ctl  | AG EF subscriber@Idle |
    Then the G5 answer property "idle" reports logic "ctl"
    And the G5 answer property "idle" has no temporal field "automaton_states"
    And the G5 answer property "idle" has no temporal field "negated"

  Scenario: an AG that fails names the state it fails in and the path to it
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "AG (x < 2)"
    Then the run exits with 0
    And the G5 property "ctl1" is "violated" with evidence "exhaustive"
    And the run of the G5 property "ctl1" is finite and ends with "x" equal to 2

  Scenario: EF is verified with a finite witness path
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "EF (x == 2)"
    Then the G5 property "ctl1" is "verified" with evidence "exhaustive"
    And the run of the G5 property "ctl1" is finite and ends with "x" equal to 2

  Scenario: E[p U q] is verified with a finite witness whose states satisfy p until q
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "E[(x < 2) U (x == 2)]"
    Then the G5 property "ctl1" is "verified" with evidence "exhaustive"
    And the run of the G5 property "ctl1" is finite and ends with "x" equal to 2

  Scenario: an EG witness is a lasso, not a finite path
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "EG (x < 3)"
    Then the G5 property "ctl1" is "verified" with evidence "exhaustive"
    And the run of the G5 property "ctl1" is a lasso
    And the lasso of the G5 property "ctl1" is closed
    And every value "x" takes on the run of the G5 property "ctl1" is below 3

  Scenario: an existential property that fails carries no witness and says so honestly
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "EF (x == 9)"
    Then the G5 property "ctl1" is "violated" with evidence "exhaustive"
    And the G5 property "ctl1" carries no run and says so
    And the note of the G5 property "ctl1" says that a failing existential property has no run to show

  Scenario: AF that fails is refuted by a lasso that never reaches the goal
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "AF (x == 2)"
    Then the G5 property "ctl1" is "violated" with evidence "exhaustive"
    And the run of the G5 property "ctl1" is a lasso
    And the lasso of the G5 property "ctl1" is closed

  Scenario: a nested failure is not opened up — AG EF says where, not why
    Given the model "ctl-trap.pml" of the engine testdata
    When I check it with the CTL formula "AG EF (x == 0)"
    Then the G5 property "ctl1" is "violated" with evidence "exhaustive"
    And the run of the G5 property "ctl1" is finite and ends with "x" equal to 1
    And the note of the G5 property "ctl1" says that the nested subformula's failure is not a finite run

  Scenario: CTL fairness is out of scope and is refused, never silently dropped
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "AG EF (x == 0)" under fairness "weak"
    Then the run exits with 0
    And the G5 property "ctl1" is "not-executed" with evidence "unknown"
    And the reason of the G5 property "ctl1" mentions "fairness"
    And the reason of the G5 property "ctl1" mentions "plan 14 §4.2"

  Scenario: a CTL formula that does not parse rejects the input instead of guessing
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "AG U x"
    Then the run exits with 2
    And the G5 rejection has kind "ctl" and status "not-executed"

  Scenario: a budget that stops the graph short makes the CTL property inconclusive
    Given the IR "testdata/ir/counters-10-5.json" of the engine testdata
    When I check the IR with the CTL formula "EF (pc_value(0) == 0)" and a state budget of 100
    Then the G5 property "ctl1" is "inconclusive" with evidence "bounded"
    And the reason of the G5 property "ctl1" mentions "state budget"
    And the reason of the G5 property "ctl1" mentions "complete reachable graph"

  # ------------------------------------------------------------------ vacuity (FR-011)

  Scenario: an implication whose antecedent is never true is flagged vacuous, verdict unchanged
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "AG ((x == 7) -> (x == 8))"
    Then the G5 property "ctl1" is "verified" with evidence "exhaustive"
    And the G5 property "ctl1" is marked vacuous naming the atom "x == 7"
    And the G5 warnings mention "never true"

  Scenario: the same vacuity hint for an LTL implication, still only a hint
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the LTL formula "[]((x == 7) -> (x == 8))"
    Then the G5 property "ltl1" is "verified" with evidence "exhaustive"
    And the G5 property "ltl1" is marked vacuous naming the atom "x == 7"

  Scenario: an atom that is never false is reported as a hint and does not change the verdict
    Given the model "ctl-switch.pml" of the engine testdata
    When I check it with the CTL formula "AG (x >= 0)"
    Then the G5 property "ctl1" is "verified" with evidence "exhaustive"
    And the G5 warnings mention "never false"
    And the G5 property "ctl1" is not marked vacuous

  Scenario: mc_lint_property lints a CTL formula and names the vacuity candidate
    Given a G5 session with the corpus model "CH3/counter3.pml" parsed
    When I call mc_lint_property in the G5 session with kind "ctl" and formula "AG ((count == 250) -> EF (count == 0))"
    Then the G5 call is not an error
    And the G5 lint reports logic "ctl"
    And the G5 lint atoms are "count == 250, count == 0"
    And the G5 lint notes mention "antecedent"

  Scenario: mc_lint_property refuses a CTL formula that is not CTL, without guessing an LTL reading
    Given a G5 session with the corpus model "CH3/counter3.pml" parsed
    When I call mc_lint_property in the G5 session with kind "ctl" and formula "[]<>(count == 0)"
    Then the G5 call is not an error
    And the G5 lint reports a type error mentioning "CTL"

  # ------------------------------------------------------------- mc_estimate growth model

  Scenario: mc_estimate on the counters IR reports levels, a growth factor and a size class
    Given a G5 session with the IR "testdata/ir/counters-10-5.json" parsed
    When I call mc_estimate in the G5 session with 5000 ms and target depth 12
    Then the G5 call is not an error
    And the G5 estimate has at least 3 growth levels
    And the G5 estimate levels grow from one depth to the next
    And the G5 estimate growth rate names the levels it was fitted over
    And the G5 estimate projects a state count for depth 12
    And the G5 estimate size class is "small"
    And the G5 estimate recommendation mentions "plan 14 §12"
    And the G5 estimate says it is not a verification result

  Scenario: a model that fits in the budget is reported complete and exact, not projected
    Given a G5 session with the Petri net "testdata/petri/petrinet1.json" parsed
    When I call mc_estimate in the G5 session with 1000 ms and target depth 20
    Then the G5 estimate is complete
    And the G5 estimate projection evidence is "exhaustive"
    And the G5 estimate size class is "small"

  Scenario: the CLI exposes the same estimate under --estimate
    Given the IR "testdata/ir/counters-10-5.json" of the engine testdata
    When I estimate the IR with 5000 ms and target depth 12
    Then the run exits with 0
    And the G5 estimate output names the size class "small"
    And the G5 estimate output has a growth rate and per-level counts

  # --------------------------------------------- Promela v1: inline, typedef, provided

  Scenario: inline with arguments is substituted and CH3/inline.pml agrees with pan
    Given the model "CH3/inline.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 5
    And the G5 property "assert" is "violated" with evidence "exhaustive"
    And step 1 of the run of the G5 property "assert" is at line 2, inside the inline body

  Scenario: an inline that declares a local variable is accepted (CH3/inline2.pml)
    Given the model "CH3/inline2.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 6
    And no G5 property is violated

  Scenario: an inline used inside an atomic option (CH2/prodcons2.pml) agrees with pan
    Given the model "CH2/prodcons2.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 14
    And no G5 property is violated

  Scenario: typedef structs are flattened to fields and CH3/typedef.pml agrees with pan
    Given the model "CH3/typedef.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 5
    And the parsed IR declares the variables "goo.a, goo.fld1, goo.fld2.f, goo.fld2.g, goo.p, goo.b"
    And the parsed IR variable "foo.f" starts at 3

  Scenario: provided (expr) guards every edge of the process — CH3/toggle.pml as pan
    Given the model "CH3/toggle.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 6

  Scenario: CH5/pathfinder.pml — the priority inversion is a deadlock, with pan's verdict and count
    Given the model "CH5/pathfinder.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 12
    And the G5 property "deadlock" is "violated" with evidence "exhaustive"

  # ------------------------------------------- Promela v1: dynamic run, _nr_pr, channels

  Scenario: the static runs of CH3/you_run2.pml keep their G1 counts after the dynamic-run rework
    Given the model "CH3/you_run2.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 14

  Scenario: a model without run keeps its count too (CH3/you_run.pml)
    Given the model "CH3/you_run.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 7

  Scenario: run inside a loop instantiates processes in pan's pid order
    Given the model "nrpr.pml" of the engine testdata
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 31
    And the G5 property "assert" is "verified" with evidence "exhaustive"

  Scenario: _nr_pr counts the live processes, so a state with three of them is reachable
    Given the model "nrpr.pml" of the engine testdata
    When I check it with the CTL formula "EF (_nr_pr == 3)"
    Then the G5 property "ctl1" is "verified" with evidence "exhaustive"

  Scenario: run beyond the engine's process pool is a declared bound, not a wrong answer
    Given the model "CH3/splurge.pml" of the corpus
    When I check it sweeping the whole graph with max-procs 4
    Then the run exits with 0
    And the G5 property "deadlock" is "inconclusive" with evidence "bounded"
    And the reason of the G5 property "deadlock" mentions "max-procs"

  Scenario: a channel array and channel-typed parameters — CH9/leader.pml parses and agrees with pan
    Given the model "CH9/leader.pml" of the corpus
    When I check it sweeping the whole graph with no budget
    Then the run exits with 0
    And the G5 state count is 41692
    And no G5 property is violated

  Scenario: channels carried in messages — CH3/rendezvous2.pml parses and agrees with pan
    Given the model "CH3/rendezvous2.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 state count is 5

  Scenario: CH15/client_server.pml parses and its state count equals pan's
    Given the model "CH15/client_server.pml" of the corpus
    When I check it sweeping the whole graph with no budget
    Then the run exits with 0
    And the G5 state count is 191200
    And no G5 property is violated

  # Amended during G5: `pc_value(n)` is a state expression of the subset and
  # CH4/pcval.pml now parses, but the number it yields is *this engine's*
  # control-location index, not pan's internal state number, and the two
  # numberings are built differently. A model whose behaviour depends on the
  # value — as this one does, through `pc_value(0) > 2` — therefore has a
  # different state space here than under pan, by construction, not by
  # accident. The scenario records the deviation instead of claiming
  # agreement, and the file stays out of the differential set.
  Scenario: pc_value is a state expression, over the engine's own location numbering
    Given the model "CH4/pcval.pml" of the corpus
    When I check it sweeping the whole graph
    Then the run exits with 0
    And the G5 warnings mention "pc_value"
    And the G5 warnings mention "numbering"

  Scenario: a goto to a label no statement carries is rejected, not silently dropped
    Given the model "goto-undefined.pml" of the engine testdata
    When I parse it
    Then the run exits with 2
    And the G5 rejection has kind "semantic" and status "not-executed"
    And the G5 rejection mentions "undefined label Nowhere"
    And the G5 rejection points at line 10

  Scenario Outline: the constructs still outside the subset are refused by name, not half-executed
    Given the model "<model>" of the corpus
    When I parse it
    Then the run exits with 2
    And the G5 rejection has kind "outside-subset" and status "not-executed"
    And the G5 rejection mentions "<construct>"
    And the G5 rejection points at line <line>

    Examples:
      | model           | construct                  | line |
      | CH17/simple1.pr | c_code                     | 1    |
      | CH3/pots.pml    | unless                     | 20   |
      | CH14/version5   | random receive (??)        | 211  |
      | CH14/version6   | remote reference (P@label) | 231  |

  # --------------------------------------------------------------- differential corpus

  @spin
  Scenario Outline: the engine and pan agree on every corpus file now inside the subset
    Given spin and gcc are on PATH
    And the model "<model>" of the corpus for the G5 differential check
    When I compare it with pan
    Then pan and the engine agree on the verdict, the error class and the state count

    Examples: chapter 2-3 files unlocked by v1
      | model               |
      | CH2/prodcons2.pml   |
      | CH3/inline.pml      |
      | CH3/inline2.pml     |
      | CH3/typedef.pml     |
      | CH3/toggle.pml      |
      | CH3/rendezvous2.pml |

    Examples: chapters 5, 9, 14, 15
      | model                  |
      | CH5/pathfinder.pml     |
      | CH5/diskhead.pml       |
      | CH9/leader.pml         |
      | CH14/version1          |
      | CH14/version2          |
      | CH14/version3          |
      | CH14/version4          |
      | CH15/client_server.pml |

  # G4 ran the leader oracle on `engine/testdata/promela/leader3.pml`, a hand
  # rewrite, because the original uses a channel array, channel parameters
  # and `run` inside a loop — all outside the G1 subset. Since G5 they are
  # inside it, so the oracle runs on the corpus file's own text. Only the
  # size constant is changed: the file writes `#define N 7` outright (not
  # `#ifndef`), so a `-D` on the command line is overridden by the file
  # itself, and the ring is cut to three nodes by rewriting that one line —
  # 2801652 states at N = 7 is not a size for a test suite. Everything the
  # step is about — the channel array, the channel parameters, the `run` in
  # the loop — is the corpus file's, unchanged.
  @spin
  Scenario Outline: the G4 leader triple now runs on the corpus CH12/leader, not on the rewrite
    Given spin and gcc are on PATH
    And the model "CH12/leader" of the corpus with N cut to 3 for the G5 differential check
    When I compare the engine, the SPIN claim and pan for "<formula>" under fairness "<fairness>"
    Then the three G5 verdicts agree

    # The formulas are CH12/leader.ltl's with its `#define`s substituted:
    # oneLeader is (nr_leaders == 1), noLeader is (nr_leaders == 0). They are
    # written out because those defines live in the .ltl file, which the
    # model itself does not include.
    Examples:
      | formula               | fairness |
      | <>[](nr_leaders == 1) | none     |
      | <>[](nr_leaders == 1) | weak     |
      | [](nr_leaders == 0)   | none     |
