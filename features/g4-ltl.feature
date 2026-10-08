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
    (intermediate atomic states stay unstored, G1 rule preserved in the product);
    a claim edge inside `atomic { }` continues at once, on the same system state,
    with the next enabled claim edge (`spin -f` claims evaluate their `assert`
    this way). Stutter extension (pan's default): in a state where no process
    can move — all terminated, or blocked — the state repeats forever and the
    claim keeps moving alone; without a claim, an `accept` label in such a state
    is an acceptance cycle (SPIN: "accept stutter").
  - counterexample of a cycle: `steps` is the whole run; `loop.start` is the 1-based
    index of the first step of the loop and `loop.steps` its length; after the last
    step the state equals the state before step `loop.start`. Claim moves are steps
    of the process named `never…`; a weak-fairness null step is a step of process
    `-` (no process moves; the fairness counter advances). A finite-prefix
    violation has no `loop`. The CLI JSON and `mc_explain` show the same split.
  - weak fairness (`--fairness weak`): the product carries a copy counter, one
    copy per process plus two (the n+2 copies of `pan -f`); an accepting state
    of copy 0 starts a round, a process that moves or has no move of its own
    advances the copy, and a step of the product leaves the last copy. An
    advance without a move is a null step of process `-`: bookkeeping, not a
    step of the run, so it changes neither the system state nor the claim, and
    a cycle is accepted only if it contains a step of the product (a claim step
    with a system move, or a stutter step). A claim that cannot move, and under
    `progress` a blocked system, therefore end the path as they do in pan.
  - statuses: acceptance cycle found → `violated`, evidence `exhaustive`; no cycle
    and the property's own product search complete → `verified`, `exhaustive`
    (each ltl / progress property reports its own counters and `complete`; the
    report's top-level `search` describes the safety search); budget hit → `inconclusive`
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

  # ---------------------------------------------------------------- fairness: a null step is bookkeeping, not a step of the product
  #
  # pan counts a process that moves, or cannot move, inside the steps of the
  # product (claim step, then system step); a blocked claim ends the path, and
  # a blocked system has no successor under -l. The engine's null steps only
  # advance the copy, so a loop made of null steps alone is no run of the
  # product. Such a loop must never decide a verdict (found by differential
  # testing: the engine reported violations on a state where pan has none).

  Scenario Outline: weak fairness — a never claim that cannot move ends the path, so nothing is accepted there, as pan -a and pan -a -f
    Given the test model "<file>"
    When I invoke "mcd check --promela <model> --no-timing"
    Then the property "never" is "verified" with evidence "exhaustive"
    And the search for "never" is complete
    When I invoke "mcd check --promela <model> --fairness weak --no-timing"
    Then the property "never" is "verified" with evidence "exhaustive"
    And the search for "never" is complete
    And the property "never" records fairness "weak"

    Examples:
      | file                         |
      | weakfair-claim-blocked.pml   |
      | weakfair-claim-m5271.pml     |

  Scenario Outline: weak fairness — no non-progress cycle stands still on a state where the np_ automaton cannot move or the system is blocked, as pan -l and pan -l -f
    Given the test model "<file>"
    When I invoke "mcd check --promela <model> --progress --no-timing"
    Then the property "progress" is "verified" with evidence "exhaustive"
    And the search for "progress" is complete
    When I invoke "mcd check --promela <model> --progress --fairness weak --no-timing"
    Then the property "progress" is "verified" with evidence "exhaustive"
    And the search for "progress" is complete
    And the property "progress" records fairness "weak"

    Examples:
      | file                         |
      | weakfair-np-claim.pml        |
      | weakfair-progress-r1.pml     |
      | weakfair-np-deadlock.pml     |

  Scenario: weak fairness — a claim that keeps moving on a blocked system is still a weakly fair acceptance cycle, as pan -a and pan -a -f
    Given the test model "weakfair-stutter-accept.pml"
    When I invoke "mcd check --promela <model> --no-timing"
    Then the property "never" is "violated" with evidence "exhaustive"
    When I invoke "mcd check --promela <model> --fairness weak --no-timing"
    Then the property "never" is "violated" with evidence "exhaustive"
    And the counterexample of "never" has a loop
    And the loop of "never" contains a step that is not a weak-fairness null step

  # Engine semantics, not pan's: SPIN 6.5.2 switched the claim's stutter step off
  # under -f (pan.c: "9/2025 added !fairness") and keeps only a default move that
  # needs an open fairness count, so `pan -a -f` finds no error here although the
  # run (every process blocked, the claim alternating between an accepting and a
  # non-accepting location) is weakly fair and accepted. The engine keeps the
  # stutter extension under weak fairness, as without it; see
  # steps/fix-weakfairness-confirmation.md for the decision this leaves open.
  Scenario: weak fairness — the stutter extension stays on: a claim cycling through an accept state on a blocked system is an acceptance cycle
    Given the test model "weakfair-stutter-cycle.pml"
    When I invoke "mcd check --promela <model> --no-timing"
    Then the property "never" is "violated" with evidence "exhaustive"
    When I invoke "mcd check --promela <model> --fairness weak --no-timing"
    Then the property "never" is "violated" with evidence "exhaustive"
    And the counterexample of "never" has a loop
    And the loop of "never" contains a step that is not a weak-fairness null step

  # ---------------------------------------------------------------- fairness: an atomic loop that never gives up the control
  #
  # Decided by the model-check study of the weak-fairness semantics (hand-encoded
  # graphs and an independent checker under testdata/weakdecision, no engine and
  # no pan involved): P keeps the exclusive control for ever, Q cannot be
  # scheduled and so is not enabled, hence owed nothing, and the run that stays
  # in P's loop with y == 0 is weakly fair and accepted: the definition says
  # `violated`, with and without fairness. pan cannot answer (the search is
  # truncated: the atomic sequence never ends). The engine does not store the
  # states of an atomic sequence, so a loop made of them has no stored state to
  # close a cycle on; the honest answer is a budget verdict, never `verified`.
  # Until the frontend kept the control at the back edge of a loop that opens an
  # atomic block, the loop was lowered as plain steps, Q was enabled in every
  # state of it, the cycle was unfair and the engine said `verified` under weak
  # fairness: a missed violation (steps/fix-weakfairness-confirmation.md §6c).

  Scenario Outline: weak fairness — an atomic loop that never gives up the control is never reported as verified
    Given the decision model "new_atomic_hold.pml"
    When I invoke "mcd check --promela <model> --budget-depth 2000 --fairness <fairness> --no-timing"
    Then the property "never" is "inconclusive" with evidence "bounded"
    And the reason of property "never" mentions "depth budget"

    Examples:
      | fairness |
      | none     |
      | weak     |

  # ---------------------------------------------------------------- fairness: `timeout`
  #
  # `timeout` is true exactly when no statement of any process is executable. A
  # process whose next statement is a bare `timeout` is therefore enabled in
  # every timeout state, and weak fairness owes it a move there. The engine
  # judged a process enabled by its ordinary statements only (a `timeout` was
  # never counted), so a loop that was made of nothing but timeout states and in
  # which a timeout-only process never moved looked fair: a false violation. Both
  # pan (-f, -l -f) and the definition (testdata/weakdecision: hand graphs and an
  # independent checker) give `verified` there. The controls keep the other
  # direction: when the loop passes through a state where another statement is
  # executable (`timeout -> a = 1 - a` is two statements), the timeout-only
  # process is disabled there and the loop is weakly fair.

  Scenario Outline: weak fairness — a process waiting for a timeout is owed a move in every timeout state
    Given the decision model "<file>"
    When I invoke "mcd check --promela <model> <extra> --no-timing"
    Then the property "<id>" is "violated" with evidence "exhaustive"
    When I invoke "mcd check --promela <model> <extra> --fairness weak --no-timing"
    Then the property "<id>" is "<weak>" with evidence "exhaustive"

    Examples: the loop is made of timeout states only: no weakly fair cycle
      | file            | extra            | id       | weak     |
      | d3_one.pml      |                  | never    | verified |
      | d3_ltl.pml      |                  | never    | verified |
      | d3_ltl_only.pml | --ltl '<>q'      | ltl1     | verified |
      | d3_np.pml       | --progress       | progress | verified |

    Examples: the loop passes through a state where timeout is false, or no process waits for one
      | file            | extra            | id       | weak     |
      | d3_ctl_split.pml |                 | never    | violated |
      | d3_ctl_skip.pml |                  | never    | violated |
      | d3_two_loop.pml |                  | never    | violated |
      | d3_mixed.pml    |                  | never    | violated |
      | d3_np_flip.pml  | --progress       | progress | violated |
      | to1.pml         |                  | never    | violated |
      | to2.pml         |                  | never    | violated |

  Scenario: without fairness the reason names the process that waits for a timeout as enabled throughout the loop
    Given the decision model "d3_one.pml"
    When I invoke "mcd check --promela <model> --no-timing"
    Then the property "never" is "violated" with evidence "exhaustive"
    And the reason for "never" names "Q:1" as enabled throughout the loop and never moving

  # ---------------------------------------------------------------- fairness: where the never claim stands
  #
  # The Promela frontend, the LTL claim and np_ put the claim last among the
  # processes; an IR given to `mcd check --ir` (or `mc_check`) need not. The
  # copy of the weak-fairness product that stands for system process k belongs
  # to the k-th process that is not the claim, wherever the claim is. With the
  # claim first or in the middle the copy for the claim's position was the claim
  # itself (never blocked while it has an edge, its steps never moves), the round
  # never closed and the cycle below was lost: `verified` under weak fairness.
  # P0 flips a bit for ever, P1 is blocked for ever, the claim accepts everything.

  Scenario Outline: weak fairness — the verdict on an IR does not depend on where its never claim stands
    When I invoke "mcd check --ir testdata/ir/<file> --fairness <fairness> --no-timing"
    Then the property "never" is "violated" with evidence "exhaustive"

    Examples:
      | file                       | fairness |
      | weakfair-claim-last.json   | none     |
      | weakfair-claim-last.json   | weak     |
      | weakfair-claim-first.json  | none     |
      | weakfair-claim-first.json  | weak     |
      | weakfair-claim-middle.json | none     |
      | weakfair-claim-middle.json | weak     |

  # ---------------------------------------------------------------- timeout moves in the product
  #
  # The product search enumerates the system's moves once per enabled claim
  # edge. `timeout` is true only when the state has no other move, and the
  # engine decided that from a counter of the moves found so far in the frame,
  # which the first claim edge's timeout moves (and a weak-fairness null step)
  # had already raised: later claim edges never got the timeout phase, so the
  # accepting run below was lost and the engine said `verified`.

  Scenario Outline: a claim with a nondeterministic choice sees the timeout moves of the system on every edge, as pan
    Given the test model "weakfair-timeout-claim.pml"
    When I invoke "mcd check --promela <model> --fairness <fairness> --no-timing"
    Then the property "never" is "violated" with evidence "exhaustive"

    Examples:
      | fairness |
      | none     |
      | weak     |

  Scenario: weak fairness — a null step is not a move of the frame: the timeout moves of the system are still enumerated
    Given the test model "weakfair-timeout-null.pml"
    When I invoke "mcd check --promela <model> --fairness weak --sweep --no-timing"
    Then the property "accept" is "violated" with evidence "exhaustive"
    And the state count of "accept" is 16

  # ---------------------------------------------------------------- the decision study of weak fairness
  #
  # Ground truth is the textbook definition of weak fairness under the
  # stutter-extension convention (a state without a move repeats for ever):
  # a run is weakly fair iff every process that is enabled from some point on
  # moves infinitely often, and an accepting weakly fair run exists iff some
  # reachable SCC has a cycle, an accepting state and, for every process, a
  # step of it or a state where it is not enabled. The expected verdicts below
  # come from hand-encoded graphs checked by testdata/weakdecision/wfcheck.py,
  # which uses no engine code, no engine oracle and no pan (statement = edge,
  # `timeout` true iff nothing else is executable, `provided`, pan's -end- step,
  # the claim in lockstep, stutter extension on and off). Where pan -f differs
  # from the definition the engine follows the definition, and the difference is
  # a documented divergence (the @spin scenario after this one), not a pass.
  #
  # D1  the stutter extension stays under weak fairness: a system that stops with
  #     p false violates <>[]p, whatever the fairness assumption says.
  # D2  a process kept from moving by `provided` is blocked, as one that waits at
  #     a false guard: the accepting loop of the other process is weakly fair.
  # D3  (the scenarios of the `timeout` section above.)
  # C5  m31278, reduced to three processes: no stuttering run is accepting, and
  #     every accepting cycle is unfair (P1 is enabled at its label and never
  #     moves), so `verified`; pan -a -f reports an "accept stutter" there.
  # new_atomic_hold, see the atomic section above: not decidable by the engine.
  # new_atomic_split and d3_at (atomic blocks that end; a timeout inside an atomic
  # block) were held back until the cycle-lasso fix was merged: their rows are
  # the last Examples of the outline below and of the pan outline after it.

  Scenario Outline: weak fairness — the verdicts of the decision study, by the definition
    Given the decision model "<file>"
    When I invoke "mcd check --promela <model> <extra> --no-timing"
    Then the property "<id>" is "<none>" with evidence "exhaustive"
    When I invoke "mcd check --promela <model> <extra> --fairness weak --no-timing"
    Then the property "<id>" is "<weak>" with evidence "exhaustive"

    Examples: D1, the stutter extension stays
      | file             | extra                  | id    | none     | weak     |
      | d1_ev_always.pml |                        | never | violated | violated |
      | d1_alt.pml       |                        | never | violated | violated |
      | d1_stay.pml      |                        | never | violated | violated |
      | d1_ltl_only.pml  | --ltl '<>[]p'          | ltl1  | violated | violated |
      | d1_ltl_only.pml  | --ltl '<>[](X p)'      | ltl1  | violated | violated |
      | d1_ltl_only.pml  | --ltl '<>[](!p -> X p)' | ltl1 | violated | violated |
      | d1_ltl_only.pml  | --ltl '<>p'            | ltl1  | violated | violated |
      | d1_ltl_only.pml  | --ltl '[]<>p'          | ltl1  | violated | violated |

    Examples: D2, `provided` false is blocked
      | file             | extra | id     | none     | weak     |
      | d2_pf_a.pml      |       | accept | violated | violated |
      | d2_pf_b.pml      |       | accept | violated | violated |
      | d2_pb_prov.pml   |       | accept | violated | violated |
      | d2_pb_guard.pml  |       | accept | violated | violated |
      | d2_pf_claim.pml  |       | never  | violated | violated |

    Examples: C5, the accept stutter of pan is an artifact
      | file             | extra | id     | none     | weak     |
      | c5_min3.pml      |       | accept | violated | verified |
      | c5_m31278.pml    |       | accept | violated | verified |

    # The lasso of these two crosses an atomic sequence (the weakly fair cycle of
    # none, the copies of weak); they were held back until the cycle-lasso fix was
    # merged. By the definition: P loops for ever, Q is owed a move, so the cycle
    # is unfair and `verified` under weak fairness.
    Examples: an atomic sequence in the loop (new_atomic_split: two statements in one block; d3_at: a timeout inside it)
      | file                 | extra | id    | none     | weak     |
      | new_atomic_split.pml |       | never | violated | verified |
      | d3_at.pml            |       | never | violated | verified |

  # pan agrees where the definition and pan -f give the same answer, and the
  # whole fairness-free column agrees: the differences are all in the weak column.
  @spin
  Scenario Outline: the engine, the engine with SPIN's claim and pan agree on the models of the decision study
    Given spin and gcc are installed
    And the model "decision:<file>" for the differential check
    When I compare the engine with pan for formula "<formula>" with defines "", fairness "<fairness>" and mode "a"
    Then the three verdicts agree

    Examples:
      | file             | formula          | fairness |
      | d1_ev_always.pml |                  | none     |
      | d1_alt.pml       |                  | none     |
      | d1_stay.pml      |                  | none     |
      | d1_stay.pml      |                  | weak     |
      | d1_ltl_only.pml  | <>[]p            | none     |
      | d1_ltl_only.pml  | <>p              | weak     |
      | d1_ltl_only.pml  | []<>p            | weak     |
      | d2_pf_a.pml      |                  | none     |
      | d2_pf_b.pml      |                  | none     |
      | d2_pf_b.pml      |                  | weak     |
      | d2_pb_prov.pml   |                  | none     |
      | d2_pb_guard.pml  |                  | none     |
      | d2_pb_guard.pml  |                  | weak     |
      | d2_pf_claim.pml  |                  | none     |
      | c5_min3.pml      |                  | none     |
      | c5_m31278.pml    |                  | none     |
      | new_atomic_split.pml |              | none     |
      | new_atomic_split.pml |              | weak     |
      | d3_at.pml        |                  | none     |
      | d3_at.pml        |                  | weak     |

  # DOCUMENTED DIVERGENCES from pan 6.5.2 -f (fairness.md section 6b). Each row
  # says what the engine and pan answer; the row passes when they answer exactly
  # that, so a change on either side shows. They are not agreements.
  #  * engine violated, pan no error: the weakly fair counterexample vanishes under
  #    -f (D1: pan.c switches the claim's stutter step off under -f, "9/2025 added
  #    !fairness"), or a process kept from moving by `provided` is not counted as
  #    blocked (D2: pan skips it before the undo that the restart of the process
  #    loop relies on, so a miss that depends on the ORDER of the processes:
  #    d2_pf_a misses, d2_pf_b, the same model with the processes swapped, finds).
  #  * engine verified, pan error: pan's "accept stutter" frame copies the
  #    accepting bit of its parent while a round opened at P1's accept label is
  #    still open (C5; with -DNOSTUTTER pan says no error as well).
  @spin
  Scenario Outline: documented divergence from pan -f — the engine follows the definition of weak fairness
    Given spin and gcc are installed
    And the model "decision:<file>" for the differential check
    When I compare the engine with pan for formula "<formula>" with defines "", fairness "weak" and mode "a"
    Then the engine says "<engine>" and pan says "<pan>"

    Examples: D1, a stopped system and <>[]p
      | file             | formula          | engine   | pan      |
      | d1_ev_always.pml |                  | violated | verified |
      | d1_alt.pml       |                  | violated | verified |
      | d1_ltl_only.pml  | <>[]p            | violated | verified |
      | d1_ltl_only.pml  | <>[](X p)        | violated | verified |
      | d1_ltl_only.pml  | <>[](!p -> X p)  | violated | verified |

    Examples: D2, provided, the order-dependent miss of pan
      | file             | formula          | engine   | pan      |
      | d2_pf_a.pml      |                  | violated | verified |
      | d2_pb_prov.pml   |                  | violated | verified |
      | d2_pf_claim.pml  |                  | violated | verified |

    # m31278 (four processes, 774 states) is the model this was reduced from: with
    # a plain `spin -a` pan -a -f reports the accept stutter (1 error), with the
    # `-o1 -o2 -o3` that this harness passes it reports none (0 errors), so the
    # artifact depends on SPIN's optimisation flags and the model has no row here.
    Examples: C5, pan's accept stutter
      | file             | formula          | engine   | pan      |
      | c5_min3.pml      |                  | verified | violated |

  # CH2/protocol uses `timeout`. The np_ automaton has two enabled edges in its
  # first location, so before the fix the second edge never got the timeout moves
  # of the system: 36 product states, where pan -l (-DNP, -c0, optimisations
  # off) stores 63. The verdict was the same, only the state space was short.
  Scenario: the non-progress product of a model with timeout has pan's state count (CH2/protocol)
    Given the corpus model "CH2/protocol"
    When I invoke "mcd check --promela <model> --progress --sweep --no-timing"
    Then the property "progress" is "verified" with evidence "exhaustive"
    And the state count of "progress" is 63

  # ---------------------------------------------------------------- else in a never claim
  #
  # `else` is enabled only when no other edge of the same location is. The product
  # search enumerated a claim's edges by their guards, and an `else` edge has none,
  # so it was always enabled: a claim that should have kept looping could fall off
  # its closing brace at once, and the engine reported "end state in claim reached"
  # where pan finds no error (0.2.0 has the same defect).

  Scenario Outline: an else edge of a never claim is enabled only when no other edge of its location is, as pan
    Given the test model "<file>"
    When I invoke "mcd check --promela <model> --fairness <fairness> --no-timing"
    Then the property "never" is "<verdict>" with evidence "exhaustive"

    Examples:
      | file                  | fairness | verdict  |
      | claim-else-idle.pml   | none     | verified |
      | claim-else-idle.pml   | weak     | verified |
      | claim-else-taken.pml  | none     | violated |
      | claim-else-taken.pml  | weak     | violated |

  # ---------------------------------------------------------------- the six models the lasso defect kept out
  #
  # m7441 m7516 m7631 m8016 m8596 m8961 are generated models (testdata/weakfair)
  # on which the weak-fairness branch alone stopped with the lasso panic of the
  # cycle search, and which were therefore left out of it until the two fixes
  # met. Each has a `provided` process kept from moving by its clause and
  # accepting or non-progress cycles through atomic sequences. Without fairness
  # the engine and pan agree (violated). Under weak fairness the engine says
  # violated, as the independent SCC oracle (explore/weakfair_oracle_test.go)
  # does for every one of them, and pan -f finds no error: the documented
  # D2 gap of pan (a process kept out by `provided` is not counted as blocked
  # where the undo of its fairness rule is skipped; fairness.md section 6b).

  Scenario Outline: a weakly fair cycle through an atomic sequence of a generated model is reported with a lasso
    Given the generated weak-fairness model "<file>"
    When I invoke "mcd check --promela <model> <extra> --no-timing"
    Then it exits with 0
    And the property "<id>" is "violated" with evidence "exhaustive"
    And the counterexample of "<id>" has a loop
    When I invoke "mcd check --promela <model> <extra> --fairness weak --no-timing"
    Then it exits with 0
    And the property "<id>" is "violated" with evidence "exhaustive"
    And the counterexample of "<id>" has a loop

    Examples:
      | file       | extra                         | id       |
      | m7441.pml  |                               | never    |
      | m7516.pml  |                               | accept   |
      | m7631.pml  | --ltl '[](p -> <>q2)'         | ltl1     |
      | m8016.pml  | --progress                    | progress |
      | m8596.pml  | --ltl '<>p'                   | ltl1     |
      | m8961.pml  | --progress                    | progress |

  # The copy index of a null step of the weak-fairness lasso is kept in a small
  # signed number of the frames of the search. Past 125 processes it used to
  # overflow, and the null steps of the higher copies were shown as stutter
  # steps; the verdict was right and the trace was not.
  Scenario: the lasso of a weak-fairness run with more than 125 processes names every copy
    Given the test model "weakfair-many-processes.pml"
    When I invoke "mcd check --promela <model> --ltl '[] f' --fairness weak --no-timing"
    Then it exits with 0
    And the property "ltl1" is "violated" with evidence "exhaustive"
    And the counterexample of "ltl1" has a loop
    And a step of the counterexample of "ltl1" has the command containing "copy 129 -> 130"
    And a step of the counterexample of "ltl1" has the command containing "copy 130 -> 131"

  @spin
  Scenario Outline: the six generated models agree with pan without fairness
    Given spin and gcc are installed
    And the model "weakfair:<file>" for the differential check
    When I compare the engine with pan for formula "<formula>" with defines "", fairness "none" and mode "<mode>"
    Then the three verdicts agree

    Examples:
      | file       | formula         | mode |
      | m7441.pml  |                 | a    |
      | m7516.pml  |                 | a    |
      | m7631.pml  | [](p -> <>q2)   | a    |
      | m8016.pml  |                 | l    |
      | m8596.pml  | <>p             | a    |
      | m8961.pml  |                 | l    |

  @spin
  Scenario Outline: documented divergence from pan -f on the six generated models (D2, provided)
    Given spin and gcc are installed
    And the model "weakfair:<file>" for the differential check
    When I compare the engine with pan for formula "<formula>" with defines "", fairness "weak" and mode "<mode>"
    Then the engine says "violated" and pan says "verified"

    Examples:
      | file       | formula         | mode |
      | m7441.pml  |                 | a    |
      | m7516.pml  |                 | a    |
      | m7631.pml  | [](p -> <>q2)   | a    |
      | m8016.pml  |                 | l    |
      | m8596.pml  | <>p             | a    |
      | m8961.pml  |                 | l    |

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

  # ---------------------------------------------------------------- a cycle through an atomic sequence

  Scenario: an acceptance cycle through an atomic sequence is reported as an exact run, as pan -a
    # The loop of the counterexample runs through the unstored intermediate
    # state in the middle of `atomic { y = 1; y = 0 }`. The lasso used to be
    # rendered after the search had released those states, and the engine
    # stopped with a Go panic (exit 2, a trace on stderr, no report). pan -a:
    # acceptance cycle, 1 state stored.
    Given the test model "claim-atomic-loop.pml"
    When I invoke "mcd check --promela <model> --no-timing"
    Then it exits with 0
    And the property "never" is "violated" with evidence "exhaustive"
    And the counterexample of "never" has a loop
    And the loop of "never" contains a step by process "A:0"
    And the loop of "never" is closed: the state after the last step equals the state before the loop
    And the counterexample of "never" replays as a run of the model with the claim's accept location in the loop
    And the state count of "never" is 1

  Scenario: --sweep goes on after the verdict, reaches a second cycle through an atomic sequence, and keeps the first counterexample
    # The first cycle (on x) decides the property; the search then goes on and
    # finds the cycle through `atomic { y = 1; y = 0 }`, as pan -c0 does (pan:
    # 2 states stored). That cycle is rendered although the verdict is final,
    # so it has to be renderable. Without --sweep the run never reaches it.
    Given the test model "claim-atomic-second-cycle.pml"
    When I invoke "mcd check --promela <model> --no-timing"
    Then it exits with 0
    And the property "never" is "violated" with evidence "exhaustive"
    When I invoke "mcd check --promela <model> --no-timing --sweep"
    Then it exits with 0
    And the property "never" is "violated" with evidence "exhaustive"
    And the counterexample of "never" has a loop
    And the counterexample of "never" replays as a run of the model with the claim's accept location in the loop
    And the counterexample of "never" is the one that "mcd check --promela <model> --no-timing" reports
    And the state count of "never" is 2

  Scenario Outline: an acceptance cycle through an atomic sequence of a model with a second process is an exact run, with and without --sweep
    # Two more models on which the loop of an acceptance cycle crosses an
    # atomic sequence (claim-atomic-starve.pml: P loops on an atomic pair
    # while Q is starved; claim-atomic-timeout.pml: the atomic sequence begins
    # with a timeout). The published 0.2.0 stopped on both with a Go panic
    # (index out of range in the lasso), with and without --sweep. The verdict
    # and the counterexample do not depend on --sweep; only the state count
    # does, and with --sweep it is pan's -c0 count.
    Given the test model "<fixture>"
    When I invoke "mcd check --promela <model> --no-timing"
    Then it exits with 0
    And the property "never" is "violated" with evidence "exhaustive"
    And the counterexample of "never" has a loop
    And the loop of "never" contains a step by process "P:0"
    And the loop of "never" is closed: the state after the last step equals the state before the loop
    And the counterexample of "never" replays as a run of the model with the claim's accept location in the loop
    When I invoke "mcd check --promela <model> --no-timing --sweep"
    Then it exits with 0
    And the property "never" is "violated" with evidence "exhaustive"
    And the counterexample of "never" has a loop
    And the counterexample of "never" replays as a run of the model with the claim's accept location in the loop
    And the counterexample of "never" is the one that "mcd check --promela <model> --no-timing" reports
    And the state count of "never" is <states>

    Examples:
      | fixture                   | states |
      | claim-atomic-starve.pml   | 2      |
      | claim-atomic-timeout.pml  | 6      |

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
      | testdata:claim-atomic-loop.pml         |                                |                                                           | none     | a    |
      | testdata:claim-atomic-second-cycle.pml |                                |                                                           | none     | a    |
      | testdata:claim-atomic-starve.pml       |                                |                                                           | none     | a    |
      | testdata:claim-atomic-timeout.pml      |                                |                                                           | none     | a    |
      | testdata:claim-atomic-starve.pml       |                                |                                                           | weak     | a    |
      | testdata:claim-atomic-timeout.pml      |                                |                                                           | weak     | a    |

    # Weak fairness and the states where a null step is all there is (no step of
    # the product): pan has no error on them.
    Examples:
      | model                                | formula | defines | fairness | mode |
      | testdata:weakfair-claim-blocked.pml  |         |         | none     | a    |
      | testdata:weakfair-claim-blocked.pml  |         |         | weak     | a    |
      | testdata:weakfair-claim-m5271.pml    |         |         | weak     | a    |
      | testdata:weakfair-np-claim.pml       |         |         | weak     | l    |
      | testdata:weakfair-progress-r1.pml    |         |         | weak     | l    |
      | testdata:weakfair-np-deadlock.pml    |         |         | weak     | l    |
      | testdata:weakfair-stutter-accept.pml |         |         | none     | a    |
      | testdata:weakfair-stutter-accept.pml |         |         | weak     | a    |
      | testdata:weakfair-timeout-claim.pml  |         |         | none     | a    |
      | testdata:weakfair-timeout-claim.pml  |         |         | weak     | a    |
      | testdata:claim-else-idle.pml  |         |         | none     | a    |
      | testdata:claim-else-idle.pml  |         |         | weak     | a    |
      | testdata:claim-else-taken.pml  |         |         | none     | a    |
      | testdata:claim-else-taken.pml  |         |         | weak     | a    |

    # `timeout` under weak fairness: pan -f and -l -f follow the definition, so
    # the engine, the SPIN claim and pan agree on every one of them.
    Examples:
      | model                    | formula | defines     | fairness | mode |
      | decision:d3_one.pml      |         |             | none     | a    |
      | decision:d3_one.pml      |         |             | weak     | a    |
      | decision:d3_ltl.pml      |         |             | weak     | a    |
      | decision:d3_ltl_only.pml | <>q     | q=(b == 1)  | none     | a    |
      | decision:d3_ltl_only.pml | <>q     | q=(b == 1)  | weak     | a    |
      | decision:d3_np.pml       |         |             | none     | l    |
      | decision:d3_np.pml       |         |             | weak     | l    |
      | decision:d3_ctl_split.pml |        |             | weak     | a    |
      | decision:d3_ctl_skip.pml |         |             | weak     | a    |
      | decision:d3_two_loop.pml |         |             | weak     | a    |
      | decision:d3_mixed.pml    |         |             | weak     | a    |
      | decision:d3_np_flip.pml  |         |             | weak     | l    |
      | decision:to1.pml         |         |             | weak     | a    |
      | decision:to2.pml         |         |             | weak     | a    |

  # ---------------------------------------------------------------- weak fairness: a process blocked at an end label
  #
  # Issue 1 of the public repository: a second process blocked at an `end` label made --fairness weak report `violated` for a property that is
  # `verified` without fairness, with a lasso of null steps only. Weak fairness only removes runs, so the two answers must not contradict each other.
  Scenario Outline: a process blocked at an end label does not make weak fairness report a violation
    Given the test model "weakfair-end-blocked.pml"
    When I invoke "mcd check --promela <model> --ltl '[] (a -> <> b)' --fairness <fairness> --no-timing"
    Then it exits with 0
    And the property "ltl1" is "verified" with evidence "exhaustive"
    And the search for "ltl1" is complete

    Examples:
      | fairness |
      | none     |
      | weak     |
