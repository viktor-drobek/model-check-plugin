# Fix — `_nr_pr` in a model without `run`

Layer: G1 (`frontend/promela`, lowering); G0 (`ir`: what decides that the vector carries the table; `explore`: the refusal of a property that reads it).
Follows the 0.2.0 release; the engine version is not changed by this fix.
Protocol: `BUILD-PROTOCOL.md`; BDD first (the scenarios were red before any code).

## What was wrong

In a Promela model with no `run`, `_nr_pr` never decreased when a process ended.
`active proctype A() { (_nr_pr == 1) }` next to `active proctype B() { skip }` was
reported by `mcd check` as `deadlock` / `violated` / `exhaustive` (3 states, a
two-step counterexample, `B`'s `skip` and `-end-`). SPIN 6.5.2 reports no error
for it (`spin -a -o1 -o2 -o3`, `gcc -O2 -DNOREDUCE`, `./pan -c0`: `errors: 0`, 5 states stored; plain `spin -a`
gives the same 5):
`B` ends, `_nr_pr` becomes 1, `A` proceeds. A guard that waits for the count to
fall was never true, so the engine reported a deadlock SPIN does not have: a wrong
`violated` with `exhaustive` evidence. The published 0.2.0 binary has the defect
(and every earlier one, since G5 added `_nr_pr`).

## How it was found, and the cause

Found by the orchestrating agent against the 0.2.0 binary and confirmed here with
`mcd parse`: the IR of the model contains the `nrpr` op, so `ir.NeedsTable` is true
and the state vector carries the live-process table, but `B`'s `-end-` edge has no
`leave` and `A`'s `-end-` guard is the static conjunction `pc(1) == 2`. The count
byte of the table therefore stays at the number of processes started.

Cause, verified in the code (it is the likely cause that was suspected, nothing
different): `frontend/promela/lower.go` gave the `-end-` edge the table encoding
(`Edge.Leave` plus the guard `youngest(k)`, pan's "only the youngest process may
leave the vector") only `if hasDynamic`, that is, when some process is created by
`run`. The layout, however, carries the table whenever `ir.NeedsTable` says so,
which is also the case when an expression reads `nrpr`. A model that reads
`_nr_pr` and has no `run` therefore had a table that nothing ever updated.
`explore` is correct (`apply` pops the table on a `Leave` edge); `run`-created
processes were never affected, because every model with `run` already used the
table encoding.

## The fix

`lower.go` asks `ir.NeedsTable` of the lowered processes themselves (a probe model
over `in.proc` of every instance) instead of looking only for `run`-created
instances, and uses the table encoding when it says yes. The frontend and the
layout now cannot disagree about whether there is a table, and the rule for a
process that ends is pan's own in both cases. In a model without `run` the two
encodings are equivalent for the order of termination (the old conjunction "every
younger process is dead" is `youngest(k)`), so the only thing that changes is that
the count falls.

Models that never read `_nr_pr` and have no `run` are lowered exactly as before:
no table, the conjunction guard. A read that the lowering drops (the arguments of
`printf`) does not create a table. The IR of such a model, and its report, are
byte-identical to 0.2.0 (see Measured).

A related defect with the same root, found while measuring, is closed by a refusal
rather than a lowering: a property is read after the model was lowered, so it cannot
ask the frontend for the table. Over a model whose processes keep no table `_nr_pr`
was the number of processes started for ever: `AG (_nr_pr == 2)` over two active
processes that end was `verified`, `EF (_nr_pr == 0)` `violated`, both `exhaustive`
and both wrong. The property is now `not-executed` (evidence `unknown`) with a reason
that says why and what to do (put a read of `_nr_pr` into the model, which makes the
frontend keep the table). This is the one part of the change that reduces what is
accepted; the alternative (re-lowering the model with a "keep the table" option when
the formula is known, which works for the CLI but not for an MCP session that parsed
the model before the formula existed) was not built.

The first version of the refusal covered CTL atoms only, and the diff cross-review
found it incomplete (Finding 1 below): `mc_check` also takes `invariant` and `reach`
properties whose `expr` is IR JSON, `{"op":"nrpr"}` is legal in them, and
`ir.NeedsTable` walked the properties as well as the processes. A property that read
`_nr_pr` therefore gave a model whose own processes never read it a table that
nothing updates, and was answered with the count that never falls; and a CTL formula
sent beside such a property was answered too, because the sibling switched the
refusal off. The rule is now:

- the vector carries the live-process table exactly when the model's **processes**
  need it (`ir.NeedsTable` no longer walks the properties; a process created by
  `run`, an edge that leaves the table, or a process expression that reads `_nr_pr`,
  a pid or the youngest test still makes it);
- every property that reads the table over a layout that has none is refused by
  itself, `not-executed` with evidence `unknown` and a reason that names the read and
  the way out: a CTL atom (`explore/ctlcheck.go`) and the expression of an `invariant`
  or `reach` (`explore/tableread.go`, decided when the model is compiled). A refused
  property is neither compiled nor evaluated, adds nothing to the state vector of the
  properties beside it (the table is not built for it), and when every property of a
  call is refused nothing is searched.

What reads the table, and what was checked about each reader: LTL and `progress`
properties (an `--ltl` or `mc_check` formula cannot read `_nr_pr`: the atom is an
undeclared variable, rejected through the CLI and through MCP; the np claim reads
program counters only); a `never` claim (a process of the model, lowered with it and
covered by the frontend fix); `mc_simulate` (no property is evaluated, so there is
nothing to refuse; the layout is the one the processes ask for); `--estimate` and
`mc_estimate` (they drop the properties before exploring). A hand-written IR whose
processes read `nrpr` but whose end edges carry no `leave` keeps the old behaviour:
the refusal covers properties, not the statements of a process, and the IR author
owns that encoding.

## Scenarios and tests

Red first (`features/g1-promela.feature`, `features/g5-ctl-v1.feature`, fixtures
`testdata/promela/nrpr-{active,order,youngest,mixed,unread}.pml`, golden files
`testdata/golden/nrpr-unread.{ir,report}.json` produced by the unfixed engine):

- red before the fix, green after: `nrpr-active` (deadlock `violated`, 3 states
  before; `verified`, 5 states after), `nrpr-order` (`violated`, 7; `verified`,
  10), four CTL formulas over `nrpr-order` (`EF (_nr_pr == 2)`, `== 1`, `== 0` were
  `violated`, `AG (_nr_pr == 3)` was `verified`), the CTL refusal over
  `nrpr-unread`, and the `pandiff` rows for `nrpr-active` and `nrpr-order`;
- green before and after, pinning what must not move: `nrpr-youngest` (the
  youngest process waits for `_nr_pr == 1`, which the older processes can never
  make true because they may not leave first: `violated`, 4 states, equal to pan),
  `nrpr-mixed` (`active` and `run` together, 15 states, equal to pan),
  `nrpr-unread` (IR and report byte-identical to the unfixed engine, apart from the
  engine version in the report).

Unit tests (`frontend/promela/promela_g5_test.go`): every non-claim `-end-` edge
leaves the table behind `youngest(k)` when `_nr_pr` is read in a guard, an assert,
a `provided` clause or a never claim; and a model that does not read it (also one
that mentions it only in a `printf`) keeps the pc conjunction and no table. The
first test fails on the unfixed lowering in all four cases.

Added for the cross-review findings (`features/g5-ctl-v1.feature`, steps in
`steps_g5_test.go`, unit tests in `explore/tableread_test.go` and `ir/table_test.go`):

- red before, green after: an invariant and a reach on `_nr_pr` over `nrpr-unread.pml`
  through MCP (were `verified` / `violated exhaustive`, now `not-executed`, with the
  reason naming `_nr_pr` and the process table, and a sibling invariant on `x`
  answered, 10 states); a CTL formula beside an invariant that reads `_nr_pr` (was
  `violated exhaustive`, 10 states; now `not-executed`); a refused property beside
  another does not change the state vector of the other (7 bytes before, 4 after);
  the same two properties in a hand-written IR on the command line
  (`testdata/ir/nrpr-property.json`);
- green before and after, pinning what must not move: the same reach and invariant
  over `nrpr-order.pml`, whose processes read `_nr_pr` (answered, `verified` /
  `violated`); an LTL formula that reads `_nr_pr` is rejected through MCP;
- unit level: the table is decided by the processes alone (a property reading
  `nrpr`, `pid` or `youngest` leaves `NeedsTable` false and the vector the same size),
  each process-level reader and an edge that leaves the table still make it,
  `Expr.TableRead`, refusal per property and by itself, `pid` and `youngest` named in
  the reason, no search when everything is refused, an edge that leaves the table keeps
  it for a property, the stepper ignores a property that reads the table.

Pinned divergence from SPIN (Finding 2, below): two `@spin` scenario outlines (five rows) of
`g5-ctl-v1.feature` over `testdata/spin-divergence/` assert that pan and the engine
differ, so they fail, and the documentation has to move with them, if the engine ever
counts the claim. They were green from the moment they were written (they pin
behaviour, they do not drive a change).

## Measured

Before = the 0.2.0 sources (738627b) built into a scratch binary; after = this
branch. Reports are `check --sweep --no-timing`.

Affected, the models that read `_nr_pr` and have no `run`:

| model | before | after | pan -c0 (`-o1 -o2 -o3`) |
|---|---|---|---|
| `nrpr-active.pml` | deadlock `violated`, 3 states | `verified`, 5 states | no error, 5 |
| `nrpr-order.pml` | `violated`, 7 | `verified`, 10 | no error, 10 |
| `nrpr-youngest.pml` | `violated`, 4 | `violated`, 4 (IR differs: the end edges leave) | invalid end state, 4 |
| `evals-workspace/subset-probes/out-nr-pr.pml` | `verified`, 3 | `verified`, 3 (IR differs, report identical) | not run |

The pan counts are `spin -a -o1 -o2 -o3`, `gcc -O2 -DNOREDUCE`, `./pan -c0`: the flags
`tools/pandiff` uses, and the ones that make pan store every state the engine stores.
Re-run by hand in the second review round with plain `spin -a` as well: `nrpr-active` 5,
`nrpr-youngest` 4 and `nrpr-mixed` 15 are the same either way. `nrpr-unread` is 10 with
`-o1 -o2 -o3` and 7 with plain `spin -a` (statement merging, which `-o2` switches off,
stores fewer states), and `nrpr-order.pml` does not compile with plain `spin -a`
(`pan.c: conflicting types for 'done'`: the global `done` of the model becomes a hidden
variable in `pan.h`, which collides with pan's own `done`) and gives 10 with
`-o1 -o2 -o3`; `-o2` alone is enough for both (10 states each). The number 10 for
`nrpr-unread` and `nrpr-order` is therefore a number of the unoptimised pan; the fixture
comments say so, and `nrpr-order.pml` keeps its global `done` (renaming it would change
fixtures and goldens for nothing).

Not affected (identical report): `nrpr-mixed` (15), `testdata/promela/nrpr.pml`
(31, with `run`), `CH15/client_server.pml` (191 200, with `run`; the only corpus
model that reads `_nr_pr`), `nrpr-unread` (10).

Whole-tree differential: `mcd parse` and `mcd check --sweep --no-timing
--budget-states 200000 --budget-ms 40000` over the 312 Promela files of the
repository (the SPIN textbook corpus under `Promela - examples/`, `testdata/`,
`evals-workspace/`, `skills/`), run with the 0.2.0-source binary and with the fixed
one: 617 of 624 runs byte-identical (stdout, stderr and exit code). The seven that
differ: the `parse` and `check` output of `nrpr-active` and `nrpr-order`, the
`parse` output of `nrpr-youngest` and of `out-nr-pr.pml` (the end edges leave: a
`leave` flag and a `youngest` guard), and the stderr of `CH15/uts_model` (a Go
panic whose goroutine addresses differ between the two runs; see "Found, not
fixed"). No corpus model other than the new fixtures changed its verdict or its
state count.

Goldens: `testdata/golden/petrinet2.report.json` is a Petri net and unaffected; no
existing golden changed. The two new goldens
(`nrpr-unread.{ir,report}.json`) were written by the unfixed engine; the report
golden holds the SHA-256 of the fixture, so editing `nrpr-unread.pml` means
regenerating it, and the scenario ignores only the engine version.

Partial-order reduction (`go test -run TestPORAgreesWithTheFullSearchOnTheCorpus`):
the models that read `_nr_pr` are refused for it, as before, and the reason still
begins "the model reads the process table". Before: 153 Promela files, 101
accepted, 86 compared, 45 with the reduction applied, 41 refused. After, with the
five new fixtures: 158, 106, 91, 46, 45; the added refusals are
`nrpr-active`/`-order`/`-youngest` ("the model reads the process table") and
`nrpr-mixed` ("process creation"), `nrpr-unread` is reduced like any model without
a table. One reason string moved: for `nrpr-youngest` it was
`the model reads the process table (nrpr)` and is now `(youngest)`, because the
end edge of the oldest process, which now carries the guard, is analysed before the
`_nr_pr` read of the youngest; `README.md`, `workflow.md`, `engine-tools.md` and
`PROVENANCE.md` list "a read of the process table (`_nr_pr`)" as a refusal and stay
true.

Generated models against SPIN: 60 small random models (1 to 4 `active` processes,
sometimes an `init` and one `run`, statements that read `_nr_pr` in guards, an
assert and an `if`), `tools/pandiff` (verdict, error class, `pan -c0` states):
all 60 agree after the fix (22 with no error, 38 with an error found).
The 0.2.0 sources disagree on 5 of them: one on the verdict (pan: no error;
engine: deadlock) and four on the state count with the same verdict (99 against 56,
38 against 30, 276 against 256, 145 against 132). The generator
(a seeded Python script) is scratch work and is not part of the release; the
property that matters, that the table encoding cannot change a verdict of a model
that does not read `_nr_pr`, is covered by the whole-tree differential above.

Second round (after the cross-review fixes below). Three scratch binaries: the 0.2.0
sources (738627b), the first fix (768ff2c) and the fixed code of this round, run one
at a time over the 310 Promela files of the tree (`Promela - examples/`, `testdata/`,
`evals-workspace/`, `skills/`, `steps/`) with `--no-timing --budget-states 50000
--budget-ms 20000`, plus the IR and Petri fixtures: `parse`, `check --sweep`,
`check --sweep --por`, `check --sweep --ltl '[] true' --ltl '<> true'`,
`check --sweep --progress` (310 each), `check --ir` (8 files), `check --petri`
(6), `check --ctl 'EF (_nr_pr == 0)' --ctl 'AG (_nr_pr >= 0)'` over the 32 fixtures of
`testdata/promela`, and `check --estimate` over the first 40 files; stdout, stderr and
exit code compared byte for byte.

- fixed code against 768ff2c: byte-identical in 1 579 of the 1 636 runs. The 57 that
  differ: the refusal reason of 23 CTL runs (the wording changed; they are
  `not-executed` in both, and the 25 pairs compared after normalising the sentence are
  identical) and the new IR fixture `testdata/ir/nrpr-property.json` (24 of the first
  batch); 27 `--estimate` runs, which report `states_per_second` and so cannot be
  byte-identical (`CH15/client_server` also stops on its time budget); the
  `--progress` run of `CH15/client_server`, which stops on the 20 s time budget at
  10 514 to 10 812 states (it also varies between two runs of the same binary); and
  five `--progress` runs that end in the pre-existing `cycle.go` lasso panic
  (`index out of range` in `(*cycleSearch).lasso`, the same panic in the 0.2.0 binary;
  the stack differs only in a line number of `explore.go`; a fix is on another branch
  and is not touched here). Everything else did not change: `parse`, `check --sweep`,
  `--por`, `--ltl` over all 310 files, the Petri and IR fixtures, and the CTL runs on
  models whose processes read `_nr_pr` or use `run`.
- fixed code against 0.2.0: byte-identical in 1 563 of the 1 636 runs. The 73 that
  differ are the 57 above, the runs of the models changed by the first fix
  (`nrpr-active`, `nrpr-order`: `parse`, `check`, `--por`, `--ltl`, `--progress` and the
  CTL runs; `nrpr-youngest`: `parse` and `--por`; `out-nr-pr.pml`: `parse`: 15 in all),
  and one more time-budget-bound `--progress` run (`CH5/sink_source_filter`). The 23
  CTL runs that 0.2.0 answered with the constant count are the ones that are refused
  now.
- through MCP (a session driven over stdio, `mcd serve`): `reach _nr_pr == 0` and
  `invariant _nr_pr == 2` on `A, B = skip` were `violated` / `verified`, both
  `exhaustive`, 7 states, in 0.2.0 and in 768ff2c; they are `not-executed`, `unknown`
  now. With `assert(_nr_pr >= 0)` in `B` the same two properties give the right
  answers before and after (`verified` / `violated`). `EF (_nr_pr == 0)` next to an
  invariant on `_nr_pr`: `violated exhaustive` (7 states) before, `not-executed` for
  both now. `ltl` `[] (_nr_pr >= 0)`: rejected before and after.

## Verified against SPIN, and what was not

Verified, without a claim: the five fixtures and the 60 generated models agree with
SPIN 6.5.2 on verdict, error class and `pan -c0` state count (the fixtures are in the
suite as `@spin` rows of `g1-promela.feature` and in `tools/pandiff`).

**Not verified, and false under a claim (cross-review Finding 2): agreement with SPIN
when a `never` claim or an `ltl` formula is checked.** An earlier version of this
record said that a model with a `never` claim that reads `_nr_pr` "was run (counts and
verdicts equal before and after)". That sentence was vacuous: the claim and the
unit test both used `_nr_pr == 7`, which this engine never reaches, so nothing could
differ. Measured now, pan 6.5.2 counts the claim in `_nr_pr`: the generated `pan.h`
defines `BASE 1` when there is a claim (`VERI`), `addproc` does `now._nr_pr += 1` for
it, and user processes get `_pid = h - BASE`. An `ltl` formula checked with `-a` is a
claim too. This engine counts only the model's own processes (the claim is a process
with `claim: true` and the layout does not count it), so the two `_nr_pr` differ by
one whenever a claim exists, and every verdict that reads `_nr_pr` under a claim can
differ.

Confirmed with the fixed binary and pan (`spin -a -o1 -o2 -o3`, `gcc -O2 -DNOREDUCE`,
`./pan -a -c0`; the LTL rows with `tools/pandiff -mode a`; the first three models are
in `testdata/spin-divergence/`, `c2` and `cr` are scratch models of the review):

| model | pan | 0.2.0 | now |
|---|---|---|---|
| `A`, `B` = `skip`, `never { do :: (_nr_pr == 0) -> break :: else od }` (`nrpr-claim-zero`) | no error, 7 states | `never` verified, 7 | `never` **violated**, 8 |
| same, claim on `_nr_pr == 3` (`nrpr-claim-three`) | "end state in claim reached", 3 | verified, 7 | verified, 7 |
| same, claim on `_nr_pr == 2` (`c2`) | "end state in claim reached", 8 | violated, 13 | violated, 12 |
| `proctype P(){skip}`, `init{run P()}`, claim on `== 0` (`cr`) | no error, 5 | violated, 6 | violated, 6 |
| `A { (_nr_pr == 1); assert(false) }`, `B { skip }`, `never { do :: skip od }` (`nrpr-claim-assert`) | no error, 3 | `assert` verified, 3 | `assert` **violated**, 6 |
| `A { (_nr_pr == 1); x = 1; do :: x = 1 od }`, `B { skip }`, `<> (x == 1)` (`nrpr-ltl`) | **violated** (acceptance cycle), 3 | violated, 3 | **verified**, 5 |
| same, `[] (x == 0)` | verified, 3 | verified, 3 | **violated**, 6 |
| `A { (_nr_pr == 1); done = true }`, `B` counts to 3 and ends, `<> done` (scratch) | violated, 9 | violated, 9 | verified, 11 |

The pan columns were re-run in the second review round with `spin -a -o1 -o2 -o3` and
with plain `spin -a` (the LTL rows as an `ltl` block in the model, `./pan -a -c0`): the
same verdicts and state counts in every row either way; the model of the last row is
not kept, it was re-created from its description and also gives 9.

The same models without a claim agree (`nrpr-ltl` under `pandiff` without `-mode`: no
error, 5 states on both sides; `nrpr-active`, `nrpr-order`, `nrpr-youngest`,
`nrpr-mixed`, `nrpr-unread`; the 60 generated models). Several rows agreed in 0.2.0 by
accident: before the first fix `_nr_pr` was a constant that happened to equal pan's
claim-inflated count in these shapes; after it `_nr_pr` is the true count of the
model's own processes, which is what pan gives without a claim and what the safety
fixtures agree on. So the change moves the claim cases from agreement-by-accident to a
divergence that is now stated.

The divergence is documented in the four places that spoke of agreement (the
`_nr_pr` row of `promela-subset.md`, `PROMELA_SUBSET.md` §1.12, item 6 of §6 and the
`_nr_pr` bullet of §4 in `properties-ltl-ctl.md`, and this record), and pinned by two
`@spin` scenario outlines of `g5-ctl-v1.feature` that assert pan's and the engine's
verdicts side by side (`testdata/spin-divergence/`). It is not changed in the engine:
see "Open".

Also not verified: the values `_nr_pr` takes are compared with pan only through
verdicts and state counts, never state by state; the refusal has no SPIN counterpart
(SPIN has no CTL, and an `invariant` over `_nr_pr` in an IR has no Promela form);
hand-written IR whose processes read `nrpr` but whose `-end-` edges carry no `leave`
still has the old behaviour (the IR author owns that encoding, and the per-property
refusal covers properties, not process statements); `mc_simulate` was not run on these
models (it shares `explore`'s `apply`, which is unchanged, and evaluates no property).

The whole suite: `go test -count=1 -p 2 ./...` from `engine/` passes, which includes
`tools/pandiff` against SPIN 6.5.2, `go vet ./...` is clean and `gofmt -l .` prints
nothing. First fix: 153.9 s for `pandiff`, 116.5 s for the root package with the godog
suite, load average about 6.4 at the start. This round (one run, at the end, with
other jobs on the machine): `pandiff` 187.2 s, the root package 154.2 s, 210 s wall,
load average about 7 at the start and after; every package `ok`.

Mutation check of the new rules, from a green baseline (the unit tests of `ir/` and
`explore/`): eight mutants, eight killed. The refusal never fires; the refusal fires
even when the layout has a table; a property gives the model a table again
(`NeedsTable` walks the properties); an edge that leaves the table no longer makes
it; the CTL atom check is dropped; `pid` and `youngest` stop counting as reads of the
table; a call whose properties are all refused still searches; a refused property
still counts as undecided.

## Cross-review of the diff (approve with changes)

The review verified the core fix (the frontend asks `ir.NeedsTable` of the lowered
processes; every active-only, `run` and mixed model tried agrees with pan; the five
fixtures match `pan -c0` on verdict, error class and state count) and raised two
findings that are applied here and a few items that are not.

1. **MEDIUM, fixed: the refusal was incomplete.** MCP `invariant` and `reach`
   properties and a sibling property bypassed it and got a wrong exhaustive verdict;
   see "The fix", the scenarios and the second differential above. The table is now
   decided by the processes alone and every property that reads it is refused by
   itself.
2. **MEDIUM, fixed (documentation, no engine change): the docs and this record claimed
   SPIN agreement that is false under any claim.** The four places now state the
   divergence, the vacuous sentence is replaced by the measurements above, and the
   divergence is pinned.

Not worth fixing (recorded, not acted on):

- hand-written IR that reads `nrpr` in its processes with no `leave` on the end edges
  keeps the old behaviour: the refusal of Finding 1 covers properties, not the
  statements of a process;
- the CTL refusal is conservative where the count really is constant:
  `active proctype A() { do :: x = 1 od }` with `AG (_nr_pr == 1)` was a correct
  `verified` in 0.2.0 and is `not-executed` now; the reason gives the workaround
  (read `_nr_pr` in the model, which makes the frontend keep the table);
- the reason text suggests `assert(_nr_pr >= 0)` (cosmetic);
- the partial-order reason for `nrpr-youngest` names `(youngest)` instead of `(nrpr)`:
  still a refusal;
- `TestNrPrNotReadLeavesTheStaticEncoding` inspects only process 0 (the IR golden
  covers the other process).

## Second cross-review (approve with changes)

Reviewed: the second layer of the fix at 8499114 (the processes-only `ir.NeedsTable`,
`Expr.TableRead`, `explore/tableread.go`, the refusal per property, the SPIN-divergence
documentation and its `@spin` scenarios). Verdict: approve with changes, six findings of low
severity, all applied below. No path was found on which a layout without a table evaluates
`nrpr`, `pid` or `youngest` as a constant and returns a verdict.

What the reviewers and the orchestrator verified: 923 of 942 runs over 314 `.pml` files are
byte-identical with 0.2.0 (the 19 that differ are exactly the models that read `_nr_pr`
without `run`); about 17 further shapes of the first layer agree with pan; the eight mutants
of the first review round were reproduced and all are killed; every pan number of the record
and of the docs reproduces (under the flags named in fix 6).

The six fixes, in the order of the findings; each code change was written test first:

1. **Docs, `evidence-and-status.md`** (d24cd1a). Row 1 of the status table and the phrasing row
   said that after G5 the only capability boundary left was `fairness: strong`. Both name the
   second one now, a property that reads `_nr_pr` over a model whose processes keep no table
   (pointing at `properties-ltl-ctl.md` §4), as `SKILL.md` and `engine-tools.md` already did.
   The other references that list `not-executed` causes were read and agree (`SKILL.md`,
   `engine-tools.md`, `promela-subset.md`, `properties-ltl-ctl.md`, `fairness.md`,
   `workflow.md`).
2. **`analyzePOR` walked refused properties** (708a1e5 red, 1073869 fix, 0d1d248 CLI
   scenarios). `mcd check --ir testdata/ir/nrpr-property.json --sweep --por` gave
   `reduction.applied=false` with the reason "the model reads the process table (nrpr)",
   although no process reads it: the property that reads `nrpr` was already refused
   (`not-executed`) and was still walked for "what a property reads is visible". A refused
   property is skipped there now. Red on the code of 8499114: the unit test
   `TestAPropertyRefusedForTheTableDoesNotRefuseTheReduction` ("a refused property that reads
   _nr_pr switched the reduction off: the model reads the process table (nrpr)"), the new row of
   `TestPORRefusals` (a refused property must not refuse) and the g7-por scenario "a property
   that was refused for the process table does not refuse the reduction". Unchanged and green
   before and after: a process that reads the table still refuses the reduction
   (`TestTheReductionIsStillRefusedWhenAProcessReadsTheTable`, the old "process table" row of
   `TestPORRefusals`, now built from a process that asserts on `_nr_pr`, and the scenario on
   `nrpr-active.pml`). Without `--por` nothing is touched. The one visible change is the
   `--por` report of that IR: `applied` is now true (`reduced_states` 0, `fully_expanded_states`
   5: the two processes conflict on `x`), the verdicts are the same.
3. **`mc_lint_property` called `_nr_pr` constant** (5b45666 red, 50c9ee8 fix; pre-existing).
   The atom walk counted only `var` and `index` as reads of the state, so
   `{"op":"eq","args":[{"op":"nrpr"},2]}` was reported `constant: true`, "a vacuity candidate",
   also over a model that has a table, and the lint did not predict the refusal `mc_check` then
   returns. The engine had no predicate for "reads the state" to reuse (`por.go` computes
   footprints and refuses on `timeout`; `Eval` is the only other place that knows every op), so
   `ir.Expr.ReadsState` is new: every op that is not a constant, arithmetic, a comparison or a
   connective is a read (a new op counts as a read until it is classified), with a test that
   classifies every op of the language. `constant` is now `no variable atom and !ReadsState`;
   `pc`, `len`, `clen`, `cfull`, `timeout`, `nrpr`, `pid` and `youngest` count. Over a model
   without a table the lint of an `invariant`, a `reach` and, by the same decision (a small
   extension of the finding), a `ctl` property that reads the table adds the note "mc_check will
   not execute this property: …" with the refusal's own reason; the decision is taken from
   `explore` (`TableReadRefusal`, `TableReadRefusalCTL`, which `runCTL` now calls too), so lint
   and check cannot drift apart. Red before: three scenarios of `g5-ctl-v1.feature` through MCP
   ("lint constant = true, want not constant", the note missing) and a unit test that did not
   compile; the three-row outline for `pc`, `timeout` and `clen` was added afterwards and is
   killed by the obvious mutation (dropping the `ReadsState` term fails 5 scenarios). The
   description of the `constant` field and `engine-tools.md` / `properties-ltl-ctl.md` say so
   (9f22441). The text of the constant note is unchanged.
4. **`TestAnEdgeThatLeavesTheTableKeepsItForAProperty`** (0a4ce04). The test pinned an IR that
   breaks the `Edge.Leave` contract (the older process had the unguarded `leave` while the
   younger was live; `Layout.Leave` pops the youngest entry, so the `verified` of
   `reach _nr_pr == 1` was an arithmetic artifact). The `leave` now sits on B, the youngest
   (last) process, and the test also pins `reach _nr_pr == 0` as violated (A never leaves).
   Deviation from the finding: B's edge has no `youngest(1)` guard. With the guard the model
   has a table because the guard reads the table, and the mutation "`NeedsTable` ignores a
   `Leave` edge" survived; without it the `leave` is the only reason for the table, B is the
   last process so it is always the youngest while live (a legal use), and the mutation is
   killed. Mutants, from a green baseline: `NeedsTable` ignores `Leave` (killed), the refusal
   ignores `HasTable` (killed), `Layout.Leave` is a no-op (killed). The optional hardening (an
   `invalid-model` error when a `leave` fires in a process that is not the youngest) was not
   added: see (b).
5. **`Layout` header comment** (af5eddc). It said the table is carried when some expression
   reads `nrpr` / `pid` / `youngest`. It now lists the conditions of `NeedsTable` exactly (a
   dynamic process, an edge that leaves, an expression of a process that reads them) and says
   a property never gives the model a table; the closing sentence "a model without dynamic
   processes has no table" was stale for the same reason and is corrected. The empty
   `for i := range m.Globals { _ = i }` in `walkProcessExprs` was dead and is removed.
6. **The flags behind the pan counts** (e119420). The record and two fixture comments quoted
   `spin -a`, `gcc -O2 -DNOREDUCE`, `./pan -c0`. Re-run by hand with plain `spin -a` and with
   `spin -a -o1 -o2 -o3` (SPIN 6.5.2): `nrpr-active` 5, `nrpr-youngest` 4, `nrpr-mixed` 15 either
   way; `nrpr-unread` 7 with plain flags and 10 with `-o1 -o2 -o3` (`-o2` alone gives 10);
   `nrpr-order` does not compile with plain `spin -a` (`conflicting types for 'done'`) and gives
   10 with `-o1 -o2 -o3` (`-o2` alone compiles and gives 10). Every pan number of the
   divergence table (7, 3, 8, 5, 3, 3, 3, 9) is the same with both flag sets. The record now
   names the flags (the first paragraph, the table of Measured, the divergence table) and the
   comments of `nrpr-unread.pml` and `nrpr-order.pml` say `spin -a -o1 -o2 -o3`; the lines keep
   their number, and the report golden of `nrpr-unread` carries the new SHA-256 of the
   fixture. `done` in `nrpr-order.pml` is not renamed (it would churn fixtures and goldens for
   nothing).

Measured in this round. Before = a binary built from 8499114, after = the head of the branch,
both run one at a time under `timeout 60`, `--no-timing --budget-states 20000 --budget-ms 8000`,
stdout, stderr and exit code compared byte for byte. Inputs: the 101 Promela models of
`testdata/` and 71 more (every third file of the rest of the tree, spread over the corpus,
`evals-workspace`, `skills`, `steps`), 8 IR fixtures and the 2 hand-written IR of the review, 6
Petri nets. Runs: `parse`, `check --sweep` and `check --sweep --por` over the 172 models; three
CTL formulas (two on `_nr_pr`) and `--ltl '[] true'` over the 101 testdata models; `check --ir
--sweep`, `--sweep --por` and `--por` over the 10 IR; `check --petri --sweep` with and without
`--por` over the Petri nets. 758 of 760 runs are byte-identical; the two that differ are
`nrpr-property.json` with `--por` (with and without `--sweep`), where the reduction is applied
now and the reason is gone, as fix 2 says; the verdicts are the same. `mc_lint_property`
through MCP (13 expressions and formulas over `nrpr-unread`, `nrpr-order` and `nrpr-mixed`, 39
calls): 25 identical; the 14 that differ are exactly the ones fix 3 changes (`constant` true
to false for `nrpr`, `pc` and `timeout`, and the refusal note on `nrpr-unread`, where the table
is missing). `TestPORAgreesWithTheFullSearchOnTheCorpus` still reports 158 files, 106 accepted,
91 compared, 46 reduced, 45 refused.

Left as they are:

- (a) A hand-written IR that reads `nrpr` in an edge assert while the peer's end edge carries
  no `leave` keeps a table that never falls and gives a wrong exhaustive verdict. Identical in
  0.2.0; it needs an IR that breaks the documented `Edge.Leave` contract, and it cannot be
  closed statically in general, because a process that never ends has no `leave` and may
  legitimately read `_nr_pr`. A possible follow-up: a report warning when a table is needed
  but no edge has `leave`.
- (b) The unguarded `leave` on an older process has the same status (the IR breaks the
  contract; `Layout.Leave` pops the youngest entry whoever leaves). The optional check in
  `apply` (an `invalid-model` error when a `leave` fires in a process that is not the youngest)
  would close it and is an open follow-up.
- (c) A call whose properties are all refused returns `search.stop: ""`, `complete: false` and
  counters 0, and a refused row carries the run's counters and `complete: true`. Cosmetic; it
  is the existing pattern for `not-executed` rows without `Stats`.
- (d) The CTL refusal is conservative (the case recorded in the first review round).
- (e) OPEN, a decision for the maintainer: counting the `never` claim and the claim of an `ltl`
  formula in `_nr_pr` as pan does; the cost is the one recorded under "Open".
- (f) OPEN: whether the contract "an end edge must carry `leave`, guarded by `youngest(k)`, for
  `_nr_pr` to fall" belongs in a user-facing reference. Today it appears only in
  `testdata/ir/README.md`, in this record and in Go comments; the skill's references document no
  hand-written IR schema.
- (g) Noticed, not touched: `PROMELA_SUBSET.md` §2.1 still lists CTL and weak fairness as
  `not-executed` (stale since G4 and G5, older than this fix and unrelated to it).

The whole suite after the last code change (head 9f22441): `go test -count=1 -p 2 ./...` from
`engine/` passes in 194 s wall (the root package with the godog suite 141.8 s, `tools/pandiff`
against SPIN 6.5.2 174.7 s; load average 8.1 at the start, other jobs on the machine), `go vet
./...` is clean and `gofmt -l .` prints nothing. It was run once, at the end; the text of this
section was written after it.

## Documentation corrected

`skills/model-check/references/promela-subset.md` (the `_nr_pr` row of the differences
table: the 0.2.0 behaviour, what the claim does to the count, the measured
divergences, the refusal), `references/properties-ltl-ctl.md` (§4, the `_nr_pr`
bullet: a property refused per property, MCP IR included; §6, the sixth detail of how a
claim runs, and the intro that said all five matched `pan`),
`engine/frontend/promela/PROMELA_SUBSET.md` (§1.12), `skills/model-check/SKILL.md` (the
`not-executed` row of the status table) and `references/engine-tools.md` (the second
`not-executed` boundary), `testdata/spin-divergence/README.md` and
`testdata/ir/README.md`, the header comment of `lower.go`, the comments of
`ir.NeedsTable` and `Layout.NrPr`, the status vocabulary of `explore.go`, the header of
`features/g2-mcp.feature`, the comment of `notExecutedReason` in `mcp/check.go`. Nothing in `README.md` or `PROVENANCE.md` said that `_nr_pr` did not
fall or that it agreed with SPIN under a claim.

## Found, not fixed

`mcd check --promela "Promela - examples/CH15/uts_model" --sweep` panics in the
0.2.0 binary (`index out of range [26] with length 26`,
`explore.(*cycleSearch).stateOf` called from `lasso`, `explore/cycle.go:998`) while
searching the `never` claim of the model; without `--sweep` it finishes. The model
does not read `_nr_pr`, the panic is the same before and after this change, and it
was not investigated further.

The same panic also ends five `--progress --sweep` runs over the corpus in the 0.2.0
binary and after this change (`CH2/prodcons2`, `CH5/pathfinder`,
`testdata/mutate/sample`, `testdata/promela/atomic-t5`,
`alternatingbit-ghost` of the iteration-3 evals): `(*cycleSearch).lasso`, found by
the second differential above. It is not touched here; a fix is being prepared on its
own branch.

## Open

Should the engine count the `never` claim, and the claim of an `ltl` formula, in
`_nr_pr`, as pan does? A design decision for the maintainer, not settled here. What it
would cost: the count the layout keeps would have to include a claim process, and the
`pid` that `pid = run P()` stores, which is `NrPr() - 1` (`lower.go`, the `run`
statement), would have to subtract the claim count; an `ltl` formula's claim is not a
process of the layout (the product search adds it), so LTL, `progress` and accept-label
runs would need the extra one separately. Until then the divergence above is the
documented behaviour. No warning is emitted at parse time either: that is part of the
same decision.

Where the contract "an end edge must carry `leave`, guarded by `youngest(k)`, for `_nr_pr` to
fall" should be documented (second cross-review, item (f)): today only `testdata/ir/README.md`,
this record and the Go comments state it, and the skill's references describe no hand-written
IR schema. If hand-written IR is to be a supported input, the contract belongs next to that
schema; a report warning for a model that needs a table but has no `leave` edge, and an
`invalid-model` error for a `leave` that fires in a process that is not the youngest, are the
two checks that would close the cases of items (a) and (b).

## After the integration (0.3.0)

Merged with `perf/step6-por-extension` (`integration-0.3.0-notes.md`). The partial-order paragraph above is
history: on the merged tree the models that read `_nr_pr` without `run` are reduced like any other (cell T),
`nrpr-active/-order/-youngest` are no longer refused, and `TestPORAgreesWithTheFullSearchOnTheCorpus` reports 190
Promela files, 137 accepted, 120 compared, 75 applied (35 smaller), 45 refused. The tests and the g7 scenario
that said "refused" now say "applied, and answers as the full search". `PROMELA_SUBSET.md` section 2.1 (item (g)
above) is brought up to date. Models of this shape were run against `pan` in full and reduced (fixtures, 270
fuzzed models, 100 000 through the engine in full and reduced); with the claim counted (constants one higher for
pan) the verdicts under a never claim, an `ltl` formula and non-progress agree on 765 rows.
