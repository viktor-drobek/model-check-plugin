Feature: G4 LTL — Büchi translation, nested DFS, non-progress cycles, weak fairness, prefix+loop counterexamples
  Plan 14 §9, row G4: LTL in SPIN syntax is parsed, negated, translated to a Büchi
  automaton (tableau + degeneralisation), run in synchronous product with the model,
  and checked for acceptance cycles by nested DFS; a `never { }` claim of the model is
  run with the same product; non-progress cycles use SPIN's np_ automaton; weak
  fairness uses the n+2 copies construction of `pan -f`; strong fairness is refused
  with a reason (FR-008). Exit criterion, verbatim: verdicts agree with SPIN on CH4,
  CH8, CH12, App_A; counterexamples for cycles are prefix + loop.

  Vocabulary (the same words as the report):
  - `ltl` property: `formula` in SPIN syntax (`[]`, `<>`, `U`, `V`, `X`, `!`, `&&`,
    `||`, `->`, `<->`, `true`, `false`; atoms are identifiers or parenthesised
    boolean expressions over the global state; `#define`d symbols of a Promela input
    are expanded). The engine builds the automaton for the NEGATED formula `!(φ)`;
    an acceptance cycle of the product is a run satisfying `!(φ)`, hence φ is
    `violated`. An `ltl` property WITHOUT a formula means "SPIN's `pan -a` on the
    model as written": the model's own `never` claim if it has one (property id
    `never`, added by the Promela frontend), else acceptance cycles through
    `accept` labels of the processes (property id `accept`).
  - `progress` property: no non-progress cycle — a cycle in which no process visits a
    `progress` label (SPIN `pan -l` with `-DNP`); property id `progress`, added by
    the frontend when the model has progress labels, or requested explicitly.
  - claim semantics (SPIN's, stated once here and used everywhere): the claim moves
    first, observing the current state, then the system moves; a state after a
    claim move is not a stored state (as in pan) — stored states are the pairs
    (system state, claim location) after a system step, and the state counts below
    are pan's `states, stored`. A claim without an enabled edge prunes the path
    (nothing is claimed about it). A claim reaching its end (`}`) is a violation on a
    finite prefix (SPIN: "end state in claim reached"), and a failing `assert` in the
    claim (as `spin -f` writes them) likewise (SPIN: "assertion violated"). A
    cycle through a state whose claim location carries `accept` is a violation
    (SPIN: "acceptance cycle"). The claim does not move inside an atomic sequence
    (intermediate atomic states stay unstored, G1 rule preserved in the product).
  - counterexample of a cycle: `steps` is the whole run; `loop.start` is the 1-based
    index of the first step of the loop and `loop.steps` its length; after the last
    step the state equals the state before step `loop.start`. Claim moves are steps
    of the process named `never…`; a weak-fairness null step is a step of process
    `-` (no process moves; the fairness counter advances). A finite-prefix
    violation has no `loop`. The CLI JSON and `mc_explain` show the same split.
  - statuses: acceptance cycle found → `violated`, evidence `exhaustive`; no cycle
    and the search complete → `verified`, `exhaustive`; budget hit → `inconclusive`
    with evidence `bounded` when the declared states or depth limit was reached and
    `unknown` when time or memory ran out (plan 14 §6); strong fairness →
    `not-executed` with a reason; malformed formula or undeclared atom → the input
    is rejected (exit 2, status `not-executed`), never a verdict.
  - budgets (plan 14 §6 as amended): in the CLI as in MCP, an absent or `0` budget
    flag means the default (states 1000000, depth 1000000, 60000 ms, 1024 MiB);
    `--unlimited` (CLI only) lifts every limit and the report echoes 0 for them.
  - the pan numbers quoted were produced by SPIN 6.5.2, `spin -a -o1 -o2 -o3`,
    `gcc -O2 -DNOREDUCE [-DNP]`, `./pan -a [-f] -c0` / `./pan -l [-f] -c0`, on
    2026-09-25. One known counter artefact: when the initial state is already
    accepting (CH8/fairness.pml, CH4/fair.pml under -DNP) pan's "states, stored"
    counts one re-insertion of its nested search (`pan -DCHECK` prints "New state
    3+"); the product itself has as many states as pan's plain run, and that is the
    number the engine reports. Test models: `engine/testdata/promela/starvation.pml` (two-process
    starvation) and `leader3.pml` (CH12/leader rewritten for N = 3 because the
    original uses a channel array, channel parameters and `run` in a loop, which
    are outside the G1 subset).

  # ---------------------------------------------------------------- exit criterion: CH4

  Scenario: CH4/prop.pml with -D PHI — the claim for []p accepts a run that keeps p, as pan -a
    Given the corpus model "CH4/prop.pml"
    When I invoke "mcd check --promela <model> -D PHI --sweep --no-timing"
    Then it exits with 0
    And the property "never" is "violated" with evidence "exhaustive"
    And the reason of property "never" mentions "acceptance cycle"
    And the counterexample of "never" has a loop
    And every step of the loop of "never" is by process "init:0" or by the claim
    And the search for "never" is complete
    And the state count of "never" is 3

  Scenario: CH4/prop.pml without PHI — the claim for ![]p runs to its end when x becomes 0, as pan -a
    Given the corpus model "CH4/prop.pml"
    When I invoke "mcd check --promela <model> --sweep --no-timing"
    Then it exits with 0
    And the property "never" is "violated" with evidence "exhaustive"
    And the reason of property "never" mentions "end state in claim reached"
    And the counterexample of "never" has no loop
    And the final state of "never" has "x" equal to 0
    And the state count of "never" is 5

  Scenario: []p on CH4/prop.pml through the engine's own automaton — violated with prefix + loop
    Given the corpus model "CH4/prop.pml"
    When I invoke "mcd check --promela <model> --ltl '[]p' --no-timing"
    Then it exits with 0
    And the property "ltl1" is "violated" with evidence "exhaustive"
    And the property "ltl1" has formula "[]p" and negation "!([]p)"
    And the counterexample of "ltl1" has a loop
    And the prefix of "ltl1" contains a step "x = 0"
    And the loop of "ltl1" is closed: the state after the last step equals the state before the loop
    And the property "ltl1" is stutter-invariant

  Scenario: CH4/dijkstra_progress.pml — no non-progress cycle, as pan -l
    Given the corpus model "CH4/dijkstra_progress.pml"
    When I invoke "mcd check --promela <model> --sweep --no-timing"
    Then it exits with 0
    And the property "progress" is "verified" with evidence "exhaustive"
    And the search for "progress" is complete
    And the state count of "progress" is 39

  Scenario: CH4/fair.pml — every cycle is a non-progress cycle, as pan -l
    Given the corpus model "CH4/fair.pml"
    When I invoke "mcd check --promela <model> --progress --sweep --no-timing"
    Then it exits with 0
    And the property "progress" is "violated" with evidence "exhaustive"
    And the reason of property "progress" mentions "non-progress cycle"
    And the counterexample of "progress" has a loop
    And the state count of "progress" is 4

  Scenario: CH4/fair_accept.pml — acceptance cycle through B's accept label, with and without weak fairness, as pan -a and pan -a -f
    Given the corpus model "CH4/fair_accept.pml"
    When I invoke "mcd check --promela <model> --no-timing"
    Then the property "accept" is "violated" with evidence "exhaustive"
    And the counterexample of "accept" has a loop
    When I invoke "mcd check --promela <model> --fairness weak --no-timing"
    Then the property "accept" is "violated" with evidence "exhaustive"
    And the counterexample of "accept" has a loop
    And the loop of "accept" contains a step by process "A:0"
    And the loop of "accept" contains a step by process "B:1"

  Scenario: CH4/true.pml and false.pml — a finite model has no cycle; the assert decides
    Given the corpus model "CH4/false.pml"
    When I invoke "mcd check --promela <model> --ltl '[]true' --no-timing"
    Then it exits with 0
    And the property "assert" is "violated" with evidence "exhaustive"
    And the property "ltl1" is "verified" with evidence "exhaustive"

  # ---------------------------------------------------------------- exit criterion: CH8

  Scenario: CH8/fairness.pml — the accept label in A lies on a cycle even under weak fairness, as pan -a -f
    Given the corpus model "CH8/fairness.pml"
    When I invoke "mcd check --promela <model> --sweep --no-timing"
    Then the property "accept" is "violated" with evidence "exhaustive"
    And the state count of "accept" is 4
    When I invoke "mcd check --promela <model> --fairness weak --no-timing"
    Then the property "accept" is "violated" with evidence "exhaustive"
    And the loop of "accept" contains a step by process "B:1"

  Scenario: CH8/trivial.pml — the claim's accept state is on the x = 0, 1, 0, 1 cycle, with and without weak fairness, as pan
    Given the corpus model "CH8/trivial.pml"
    When I invoke "mcd check --promela <model> --sweep --no-timing"
    Then the property "never" is "violated" with evidence "exhaustive"
    And the state count of "never" is 2
    When I invoke "mcd check --promela <model> --fairness weak --no-timing"
    Then the property "never" is "violated" with evidence "exhaustive"
    And the counterexample of "never" has a loop

  Scenario: CH8/example.pml — a finite state space: the assert is violated and no acceptance property exists
    Given the corpus model "CH8/example.pml"
    When I invoke "mcd check --promela <model> --no-timing"
    Then it exits with 0
    And the property "assert" is "violated" with evidence "exhaustive"
    And there is no property "accept"

  # ---------------------------------------------------------------- exit criterion: App_A and CH12

  Scenario: App_A/example — the model's claim (the automaton FOR <>[]p) accepts no run, as pan -a, and the state count is pan's
    Given the corpus model "App_A/example"
    When I invoke "mcd check --promela <model> --sweep --no-timing"
    Then it exits with 0
    And the property "never" is "verified" with evidence "exhaustive"
    And the search for "never" is complete
    And the state count of "never" is 10

  Scenario: App_A/example — the engine's automata agree with the model's claim on the negation direction
    # The claim of App_A is written for <>[]p itself: it accepts the runs that
    # satisfy <>[]p, and pan's "no errors" means no run does (x runs 4, 2, 1, 4 …
    # and p = (x < 4) fails at every x = 4). So the property []<>!p holds and the
    # property <>[]p is violated by a lasso; both are what the engine's own
    # automata report.
    Given the corpus model "App_A/example"
    When I invoke "mcd check --promela <model> --ltl '[]<>!p' --ltl '<>[]p' --no-timing"
    Then it exits with 0
    And the property "ltl1" is "verified" with evidence "exhaustive"
    And the property "ltl1" has formula "[]<>!p" and negation "!([]<>!p)"
    And the property "ltl2" is "violated" with evidence "exhaustive"
    And the counterexample of "ltl2" has a loop
    And the property "never" is "verified" with evidence "exhaustive"

  Scenario: leader election (CH12 claim on the N = 3 rewrite) — engine automaton and SPIN never claim give the same verdict
    Given the test model "leader3.pml" with the corpus claim "CH12/leader.ltl" appended
    When I invoke "mcd check --promela <model> --ltl '<>[]oneLeader' --sweep --no-timing"
    Then it exits with 0
    And the property "never" is "verified" with evidence "exhaustive"
    And the property "ltl1" is "verified" with evidence "exhaustive"
    And the property "assert" is "verified" with evidence "exhaustive"
    And the state count of "never" is 1340
    And the atoms of "ltl1" are "nr_leaders == 1"

  # ---------------------------------------------------------------- fairness

  Scenario: starvation — <>done is violated without fairness by a loop in which only A moves
    Given the test model "starvation.pml"
    When I invoke "mcd check --promela <model> --ltl '<>done' --no-timing"
    Then it exits with 0
    And the property "ltl1" is "violated" with evidence "exhaustive"
    And the counterexample of "ltl1" has a loop
    And every step of the loop of "ltl1" is by process "A:0" or by the claim
    And the reason for "ltl1" names "B:1" as enabled throughout the loop and never moving

  Scenario: starvation — under weak fairness B must move, so <>done holds, as pan -a -f
    Given the test model "starvation.pml"
    When I invoke "mcd check --promela <model> --ltl '<>done' --fairness weak --no-timing"
    Then it exits with 0
    And the property "ltl1" is "verified" with evidence "exhaustive"
    And the property "ltl1" records fairness "weak"

  Scenario: CH3/alternatingbit.pml — every message handed to the receiver is taken, with and without fairness, as pan
    Given the corpus model "CH3/alternatingbit.pml"
    When I invoke "mcd check --promela <model> --ltl '[] (len(to_rcvr) > 0 -> <> (len(to_rcvr) == 0))' --no-timing"
    Then the property "ltl1" is "verified" with evidence "exhaustive"
    When I invoke "mcd check --promela <model> --ltl '[] (len(to_rcvr) > 0 -> <> (len(to_rcvr) == 0))' --fairness weak --no-timing"
    Then the property "ltl1" is "verified" with evidence "exhaustive"
    And the property "ltl1" records fairness "weak"

  Scenario: strong fairness is not executed, with a reason; safety properties are unaffected
    Given the test model "starvation.pml"
    When I invoke "mcd check --promela <model> --ltl '<>done' --fairness strong --no-timing"
    Then it exits with 0
    And the property "ltl1" is "not-executed" with evidence "unknown"
    And the reason of property "ltl1" mentions "strong fairness"
    And the reason of property "ltl1" mentions "weak"
    And the property "deadlock" is "verified" with evidence "exhaustive"

  # ---------------------------------------------------------------- formulas

  Scenario: a formula with X is flagged as not stutter-invariant
    Given the corpus model "App_A/example"
    When I invoke "mcd check --promela <model> --ltl 'X p' --no-timing"
    Then it exits with 0
    And the property "ltl1" is not stutter-invariant
    And the property "ltl1" is "violated" with evidence "exhaustive"

  Scenario: a malformed formula is a rejected input, not a verdict
    Given the corpus model "App_A/example"
    When I invoke "mcd check --promela <model> --ltl '[] (p ->' --no-timing"
    Then it exits with 2
    And the input is rejected with kind "ltl" and status "not-executed"
    And the rejection message mentions "formula"

  Scenario: an undeclared atom is a rejected input
    Given the corpus model "App_A/example"
    When I invoke "mcd check --promela <model> --ltl '<> nosuchvar' --no-timing"
    Then it exits with 2
    And the input is rejected with kind "ltl" and status "not-executed"
    And the rejection message mentions "nosuchvar"

  # ---------------------------------------------------------------- budgets (plan 14 §6 as amended)

  Scenario: a budget flag at 0 means the default in the CLI, as in MCP
    Given the corpus model "App_A/example"
    When I invoke "mcd check --promela <model> --budget-states 0 --budget-ms 0 --no-timing"
    Then it exits with 0
    And the report budget has states 1000000 and time_ms 60000

  Scenario: --unlimited lifts every limit
    Given the corpus model "App_A/example"
    When I invoke "mcd check --promela <model> --unlimited --no-timing"
    Then it exits with 0
    And the report budget has states 0 and time_ms 0
    And the property "never" is "verified" with evidence "exhaustive"

  Scenario: a state budget hit during a cycle search is inconclusive and bounded
    Given the test model "leader3.pml" with the corpus claim "CH12/leader.ltl" appended
    When I invoke "mcd check --promela <model> --budget-states 100 --no-timing"
    Then it exits with 0
    And the property "never" is "inconclusive" with evidence "bounded"
    And the reason of property "never" mentions "state budget"
    And the search for "never" is not complete

  Scenario: a time budget hit is inconclusive and unknown
    # 64 million states (G1 §3.2); len(q) <= 8 always holds, so the search
    # cannot stop early on a violation.
    Given the corpus model "CH5/sink_source_filter.pml"
    When I invoke "mcd check --promela <model> --ltl '[](len(q) <= 8)' --budget-ms 1 --no-timing"
    Then it exits with 0
    And the property "ltl1" is "inconclusive" with evidence "unknown"
    And the reason of property "ltl1" mentions "time budget"
    And the search for "ltl1" is not complete

  Scenario: two identical cycle checks are byte-identical
    Given the test model "starvation.pml"
    When I invoke "mcd check --promela <model> --ltl '<>done' --no-timing" twice
    Then the two reports are byte-identical

  # ---------------------------------------------------------------- MCP

  Scenario: mc_parse with promela over MCP returns IR (the frontend is linked into the server)
    # G2 left Config.Promela as a hook; G4 wires the G1 frontend into `mcd serve`.
    Given an MCP server with the Promela frontend linked as mcd serve links it
    When I call mc_parse with the corpus Promela source "CH2/mutex_flaw.pml"
    Then the mc_parse outcome is "ir" with 2 processes
    And the parsed model has the properties "deadlock, assert"

  Scenario: mc_check runs an ltl formula and mc_explain shows the loop
    Given an MCP session with the test model "starvation.pml" parsed
    When I call mc_check with properties:
      """
      [{"id": "live", "kind": "ltl", "formula": "<>done"}]
      """
    Then the MCP property "live" is "violated" with evidence "exhaustive"
    And the MCP property "live" has a counterexample with a loop
    When I call mc_explain for the counterexample of "live"
    Then the explanation has a non-empty loop and a prefix
    And every loop step of the explanation is by process "A:0" or by the claim

  Scenario: mc_check honours fairness weak and refuses strong with a reason
    Given an MCP session with the test model "starvation.pml" parsed
    When I call mc_check with fairness "weak" and properties:
      """
      [{"id": "live", "kind": "ltl", "formula": "<>done"}]
      """
    Then the MCP property "live" is "verified" with evidence "exhaustive"
    When I call mc_check with fairness "strong" and properties:
      """
      [{"id": "live", "kind": "ltl", "formula": "<>done"}]
      """
    Then the MCP property "live" is "not-executed" with evidence "unknown"
    And the MCP reason for "live" mentions "strong fairness"

  Scenario: mc_check runs a progress property
    Given an MCP session with the corpus model "CH4/fair.pml" parsed
    When I call mc_check with properties:
      """
      [{"id": "np", "kind": "progress"}]
      """
    Then the MCP property "np" is "violated" with evidence "exhaustive"
    And the MCP property "np" has a counterexample with a loop

  Scenario: a zero budget field in mc_check means the server default
    Given an MCP session with the test model "starvation.pml" parsed
    When I call mc_check with budget states 0 and properties:
      """
      [{"id": "live", "kind": "ltl", "formula": "<>done"}]
      """
    Then the MCP applied budget has states 1000000

  # ---------------------------------------------------------------- differential oracle (needs spin and gcc)

  @spin
  Scenario Outline: the engine, the engine with SPIN's claim, and pan give the same verdict
    Given spin and gcc are installed
    And the model "<model>" for the differential check
    When I compare the engine with pan for formula "<formula>" with defines "<defines>", fairness "<fairness>" and mode "<mode>"
    Then the three verdicts agree

    Examples:
      | model                      | formula                        | defines                                                   | fairness | mode |
      | CH4/prop.pml               | []p                            |                                                           | none     | a    |
      | CH4/prop.pml               | <>[]p                          |                                                           | none     | a    |
      | CH4/prop.pml               | []<>p                          |                                                           | none     | a    |
      | App_A/example              | <>[]p                          |                                                           | none     | a    |
      | App_A/example              | []<>p                          |                                                           | none     | a    |
      | App_A/example              | X p                            |                                                           | none     | a    |
      | CH8/trivial.pml            | []<>x                          |                                                           | none     | a    |
      | CH8/trivial.pml            | []<>x                          |                                                           | weak     | a    |
      | CH8/fairness.pml           |                                |                                                           | none     | a    |
      | CH8/fairness.pml           |                                |                                                           | weak     | a    |
      | CH4/fair_accept.pml        |                                |                                                           | weak     | a    |
      | CH4/fair.pml               | []<>(x == 1)                   |                                                           | none     | a    |
      | CH4/fair.pml               | []<>(x == 1)                   |                                                           | weak     | a    |
      | CH4/fair.pml               |                                |                                                           | none     | l    |
      | CH4/dijkstra_progress.pml  |                                |                                                           | none     | l    |
      | CH4/dijkstra_progress.pml  |                                |                                                           | weak     | l    |
      | CH3/alternatingbit.pml     | [] (full1 -> <> empty1)        | full1=(len(to_rcvr) > 0); empty1=(len(to_rcvr) == 0)      | none     | a    |
      | CH3/alternatingbit.pml     | [] (full1 -> <> empty1)        | full1=(len(to_rcvr) > 0); empty1=(len(to_rcvr) == 0)      | weak     | a    |
      | testdata:starvation.pml    | <>done                         |                                                           | none     | a    |
      | testdata:starvation.pml    | <>done                         |                                                           | weak     | a    |
      | testdata:starvation.pml    | done U (done)                  |                                                           | none     | a    |
      | testdata:leader3.pml       | <>[]oneLeader                  | oneLeader=(nr_leaders == 1)                               | none     | a    |
