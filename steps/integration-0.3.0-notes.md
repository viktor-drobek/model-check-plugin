# Integration 0.3.0 — five branches merged into one, and what the merge was checked against

Layer: all of them (G0 `explore`, G1 `frontend/promela`, G2 `mcp`, G4 `cycle`). Branch
`integration/0.3.0`, created from `release/v0.2.0` (`738627b`, the published 0.2.0). The
version, `engine/bin`, `SHA256SUMS` and `BUILD-INFO.json` are untouched: the tracked
binaries are still the 0.2.0 ones and the tests that use them (G6) pass on that state. The
engine reports `0.2.0` until the maintainer cuts a release.

This record is the integration's own. It says what was merged, how each conflict was
resolved, what was run to see that the pieces work together (numbers, seeds, tools), what
the integration found and fixed, how the merged engine differs from 0.2.0 (by cause), the
list of verdict-changing fixes for the release notes, every open or unverified item of the
five source records (none dropped), what has not been reviewed, and the decisions that
belong to the maintainer. The records of the five branches stay as they are, as history;
where a statement in one of them stopped being true on the merged tree this record says so.

## 1. What was merged

Five real merge commits (`git merge --no-ff`), in this order, from the same base:

| # | Merge commit | Branch (head) | Commits | What it brings |
|---|---|---|---:|---|
| 1 | `1e971a1` | `fix/cycle-lasso-panic` (`ceec03a`) | 22 | the `s.tmp` lifetime fix in the nested-DFS lasso (`explore/cycle.go`), `balanced()` and `explore.ErrInternal`, the panic nets `cli.Run` and `recoverTool` (MCP) with the manifest rewrite and atomic `manifest.json` writes, four regression models |
| 2 | `ab39054` | `fix/weak-fairness-null-steps` (`4738e1c`) | 18 | the n+2-copies weak-fairness fix, `provided` on both sides of a rendezvous, loops at the start of atomic/d_step blocks keep the control (and one shape is refused), Stepper without timeout moves next to ordinary ones, `blocked` by the definition for timeout processes, timeout moves of every claim edge, `else` in a never claim, claim-not-last, the SCC oracle, `testdata/weakfair`, `testdata/weakdecision`, 32 mutants, fairness.md §6b |
| 3 | `63043f4` | `fix/nr-pr-process-table` (`fdf6949`) | 21 | `_nr_pr` without `run` (table encoding chosen by `ir.NeedsTable` of the processes), `Expr.TableRead`/`ReadsState`, the per-property refusal of a property that reads the table over a model without one, `mc_lint_property` fix, SPIN-divergence docs and fixtures |
| 4 | `56fd651` | `perf/step6-por-extension` (`842836d`) | 33 | partial-order reduction of atomic sequences and of `run` with the process table (cell T), oracles O1/O2/O3, generators, the `pormut` mutation harness (54 mutants), the SPIN fuzzer for the reduction |
| 5 | `1caa4ea` | `perf/step5-parallel` (`222cc3e`) | 39 | opt-in parallel exploration (`--workers`, `explore/parallel*.go`), `mc_check` `workers`, g8 feature, the parallel oracle |

Then, on the merged tree, in the order they were found (§4): the models the lasso defect had
kept out (`624fea2`), the oracles and scenarios for the interactions (`791a2ee`), the
corpus floors and a clock for the weak-fairness oracle (`cc61093`), the answer of a panic in
a parallel worker (`1ae4340`), the memory of a breadth-first search over an atomic sequence
(`1bf3ff5`), a fixture moved out of the corpus-walked directory (`775fc8c`), the
`PROMELA_SUBSET.md` property kinds (`f42cbb4`), the parallel oracle on the table shape and the
document fixes (`aeb8dc1`), the weak-fairness oracle on table models (`0a18738`), the floors of the
weak-fairness oracle scaled with the size of the run (`b31e093`), and this record's commits.

## 2. Conflicts and how they were resolved

The rule was: keep both sides, drop no test, scenario, fixture or paragraph; where two code
paths meet, reason about the interaction and put the reasoning in the merge commit message
(the messages of `ab39054`, `63043f4`, `56fd651` and `1caa4ea` carry it).

| Merge | File | Resolution |
|---|---|---|
| 1 | none | `cycle` is the base plus one branch |
| 2 | `features/g4-ltl.feature` | both sides appended rows to the differential outline: the four `claim-atomic-*` rows of the cycle branch stay in the first table, the weak branch's Examples tables follow it |
| 2 | `explore/cycle.go` (no textual conflict) | disjoint hunks, checked by hand: the lasso fix is the lifetime of `s.tmp` entries (`innerDFS`, `run`, `balanced`); the weak-fairness fix is `nextProduct`/`apply`/`renderMove` and never touches `s.tmp`. A frame of an intermediate atomic state stores the whole product state, copy byte included; the first micro-step advances the copy and the continuation reads the advanced byte back, where the mover is no longer `sys[k-1]`, so the copy cannot advance twice in one sequence |
| 3 | `features/g1-promela.feature` | the weak branch's `provided`/rendezvous and atomic-loop sections and the nrpr branch's pandiff outline share the trailing steps; combined in that order, each with its own Examples |
| 3 | `testdata/ir/README.md` | both paragraphs |
| 3 | `frontend/promela/lower.go` (no conflict) | the weak branch marks the edges that close an iteration of a loop inside an atomic/d_step block (`keepAtomic`/`keepDStep`, applied in `finalise`); the nrpr branch picks the `-end-` encoding after `finalise` by probing the lowered processes with `ir.NeedsTable`. They act on different edges, in sequence, so a loop at the start of an atomic block in a model that reads `_nr_pr` gets both |
| 4 | `explore/por_test.go` `TestPORRefusals` | step 6 turned the "dynamic process" and "process table" refusals into applied reductions; the nrpr branch's row "a refused property must not refuse" is kept |
| 4 | `testdata/ir/README.md` | all paragraphs |
| 4 | consequences, not conflicts | the nrpr branch said "a model whose process reads `_nr_pr` is not reduced"; after step 6 it is. `TestTheReductionIsStillRefusedWhenAProcessReadsTheTable` became `TestTheReductionIsAppliedWhenAProcessReadsTheTable` (applied, and every status equal to the full search) and the g7 scenario on `nrpr-active.pml` became an outline over `nrpr-active/-order/-youngest` asserting the same |
| 5 | `explore/explore.go` Options and `Run` | both fields; the search runs when a safety property is open or, with no temporal property, when no property was refused (nrpr); `case parallel` sits in the same switch before BFS/DFS |
| 5 | `cli/cli.go`, `mcp/check.go`, `mcp/server.go` | `Workers` passed with `POR`/`Fairness`/`Defines`; the `ErrInternal` exit-1 branch (cycle) stays in front of `runFailure`, which maps `*explore.InternalError` (parallel) to the same exit code; `por` schema text is step 6's followed by `workers`; imports `runtime` and `runtime/debug` |
| 5 | `tools/pandiff/pandiff.go` | one `runEngine(…, por, workers)` behind `RunEngine`, `RunEngineReduced`, `RunEngineWorkers` |
| 5 | `references/engine-tools.md`, `testdata/ir/README.md` | `--por` row (step 6) and `--workers` row, one capability table with G7 and G8 rows; all IR paragraphs |
| 5 | consequence | the g8 scenario "`--por` with `--workers` where the reduction refuses runs the parallel search" used `atomic-t3.pml`, which step 6 now reduces; it uses `por-rendezvous.pml`, and a new scenario pins that `--por --workers` on `atomic-t3.pml` is the reduced run with the parallel search refused |

## 3. Interaction checks

Tools: the repository's own (`explore` oracles with `MCD_POR_*`/`MCD_PAR_*`/`MCD_WF_*`, the
fuzzers of `tools/pandiff`, SPIN 6.5.2 with `gcc -O2 -DNOREDUCE`) and scratch harnesses that
are not committed (the generators and runners of the weak-fairness author, two small MCP
clients, a fault-injecting scratch build). Every number below was taken on the merged tree.
The machine was shared (load 4 to 60 from other sessions): no time is a benchmark.

### A. What the branches left for the merge

- The six generated models the lasso defect kept out of `testdata/weakfair` (`m7441`, `m7516`,
  `m7631`, `m8016`, `m8596`, `m8961`) are in it now (50 models), with `d3_at` and
  `new_atomic_split` of `testdata/weakdecision` (their PENDING notes removed) and rows in
  `g4-ltl.feature`. The SCC oracle compares all six with the engine under none and weak
  fairness: 6 of 6 compared, no disagreement. All six are `provided` models: without fairness
  the engine and pan agree (6 of 6, `violated`); under weak fairness the engine says `violated`,
  `pan -a -f` / `-l -f` say no error: the documented D2 gap of pan. None panics any more.
  `d3_at` and `new_atomic_split`: none `violated`, weak `verified`, as the definition and `pan`.
- `claim-atomic-starve` and `claim-atomic-timeout` under `--fairness weak`, the disagreement the
  cycle branch recorded (timeout: engine violated, pan no error): on the merged tree both are
  `verified` under weak fairness and `pan -a -f` reports 0 errors; without fairness both are
  `violated` (pan: 2 errors, 2 and 6 states stored). Not a bug and not one of the D1/D2 gaps: the
  old answer was the weak-fairness defect D3 (a process whose next statement is a bare `timeout`
  counted as blocked in every state), fixed on the other branch. Rows in the differential outline.

### B (a). Partial-order reduction x the `_nr_pr` fix

The risk: the frontend now gives models that read `_nr_pr` without `run` the table encoding
(every `-end-` edge `leave`s behind `youngest(k)`), and the oracles of step 6 made table models
only through `run`; in `genRun` the one model in eight that had a table and no `run` never left
it. New generator `nrpr` (`genNrPr`, two to four static processes, every end edge leaves, guards,
asserts, effects and properties on the table; the models of the old generators are unchanged: 3 000
seeds of `run`, `run-atomic`, `base` and `atomic` hash identically before and after).

| What | Size and seed | Result |
|---|---|---|
| O1, verdict differential (`TestPORDifferentialOnTheGenerators`) | 300 000 models, seed 930 000 001, 3 workers, `ulimit -v` 8 GB, load 8 to 20 | 296 375 applied (296 355 smaller), 3 625 skipped on a budget, 0 failures |
| O3, acyclicity of the reduced graph | 300 000 models, seed 940 000 001 | 297 059 checked, 0 failures |
| O2, semantic audit (K = 2) | 30 000 models, seed 950 000 001 (17.5 min), load about 20 | 9 749 552 (state, process) pairs, 0 failures |
| default sizes (O1 3 000, O3 3 000, O2 150) | the generator's own seeds | 0 failures; 98.7% of the models reduced (floors added) |
| Promela fuzzer, engine full against reduced, through the frontend (new: 2 to 4 `active` processes, `_nr_pr` guards that wait for the count, asserts and `if` on it, atomic and d_step blocks, a bounded loop, a buffered channel, sometimes an `init` that runs one) | 100 000 models, seed 7 000 001 | 97 009 read `_nr_pr`, 0 rejected, 4 skipped, 83 544 with a violation, 99 996 reduced, 66 321 smaller; no disagreement |
| the same models against `pan -DNOREDUCE` (verdict, error class; state count of the full search) | 120 models, seed 3 000 001, and 150 models, seed 3 000 121 | 115 and 147 agree in full and reduced (95 and 123 with an error), 8 skipped, 77 + 94 reduced; the state count of the full search equals pan's on all 119 models of the second run that have no atomic block that can block inside, and differs on 13 of the 28 that have one (and on the 13 of the first run, all of that kind): the known open item, §7 |
| the fixtures `nrpr-active/-order/-youngest/-mixed/-unread` and `nrpr.pml` against pan | six models | full: verdict, class and state count equal to pan (5, 10, 4, 15, 10, 31); reduced: verdict and class equal (5, 8, 3, 12, 9, 21) |
| the repository's fuzzer (`TestPORFuzzedPromelaAgreesWithSPIN`), whose SPIN comparison used to skip every model that reads `_nr_pr` with `active` | 400 models, seed 1 000 001 | 369 agree with the engine in full and reduced (32 of them read `_nr_pr` with `active`: they were 36 not compared before), 31 skipped, 305 reduced; before the fix and the change of the test: 337 agree, 27 skipped, 36 not compared |
| `TestPORAgreesWithTheFullSearchOnTheCorpus` | the integrated tree | 190 Promela files, 137 accepted, 120 compared, 75 with the reduction applied (35 smaller), 45 refused; with `MCD_CORPUS_ALL_FILES=1` 248 files, 150 accepted, 131 compared, 78 applied, 35 smaller, 53 refused; no disagreement |

Result: no interaction found. The inference of the merge commit (every end edge writes T and
every guard on `_nr_pr`/`youngest` reads it, so end edges and readers are dependent) holds on
the shape the frontend now emits.

### B (b). Partial-order reduction x the frontend's atomic and d_step loops

The risk: the macro-step closure and the cycle proviso were validated on IR shapes the old frontend
produced; `atomic { do ... od }` now keeps the control across the back edge, so an atomic edge returns
to the head of a loop that is also a stored location. New fuzzer: bounded loops inside atomic and d_step
blocks (first statement of the block, after a statement, before one; bodies that assign, assert,
test and, in atomic blocks, can block), plus a second loop in one block in one model of eight.

| What | Size and seed | Result |
|---|---|---|
| Promela fuzzer, engine full against reduced | 100 000 models, seed 8 000 001 | 421 736 loops in blocks (125 041 the first statement of their block), 0 rejected, 64 401 with a violation, 100 000 reduced, 80 128 smaller; no disagreement |
| the same models against `pan -DNOREDUCE` | 300 models, seed 5 000 001, and 300 models, seed 5 000 401 | 600 of 600 agree on verdict and class in full and reduced (191 and 201 with an error; 369 and 384 loops at the start of a block; 245 and 235 reduced). The state count of the full search equals pan's on all 138 models of the second run that have no atomic block that can block inside, and on 57 of the 162 that have one (it differs on the other 105 and on 104 of the first run, all of that kind: the known open item, §7) |
| O1 `atomic` | 300 000 models, seed 1 110 000 001, 4 workers, load 3 to 20 | 3 286 refused, 278 357 applied (47 321 smaller), 15 904 error in both, 2 453 skipped, 0 failures; tight limits 44 090 smaller |
| O3 `atomic` | same | 276 953 checked (tight 276 939), 0 failures |
| O2 `atomic` | same | 8 964 711 pairs, 0 failures |
| O1/O3/O2 `loop` | seed 1 120 000 001 | 300 000 applied, 165 401 smaller (tight 90 019); 300 000 checked (tight 300 000); 2 308 876 pairs; 0 failures |
| O1/O3/O2 `run` | seed 1 130 000 001 | 30 144 refused, 165 659 applied (83 058 smaller), 102 317 error in both, 1 880 skipped; 165 674 checked; 9 933 676 pairs (63 041 at a `run`); 0 failures |
| O1/O3/O2 `run-atomic` | seed 1 140 000 001 | 29 914 refused, 164 475 applied (76 623 smaller; tight 74 713), 104 261 error in both, 1 350 skipped; 164 403 checked (tight 164 399); 7 171 149 pairs (45 846 at a `run`); 0 failures |
| O1 (and tight, for `atomic-reads`) / O3 (and tight) / O2 `base`, `reads`, `atomic-reads` | 60 000 models for O1 and O3, 20 000 for O2 (smaller than the 300 000 of the others: see below), seeds 1 260 000 001, 1 270 000 001, 1 280 000 001, 3 workers, load 10 to 22 | `base`: 701 refused, 56 206 applied (14 449 smaller), 3 067 error in both, 26 skipped; 56 191 checked; 369 798 pairs. `reads`: 250 refused, 57 536 applied (25 099 smaller), 2 087 error in both, 127 skipped; 57 446 checked; 1 219 277 pairs. `atomic-reads`: 682 refused, 54 034 applied (8 163 smaller; tight 7 892), 3 264 error in both, 2 020 skipped; 53 344 checked (tight 53 340); 755 177 pairs. 0 failures |
| corpus differential | as in (a) | no disagreement |

Result: no interaction found. The first run of the scale script was stopped by the stream watchdog on a machine at a load
of 100 to 170 (another project's runs plus mine, which ran several jobs at once); the three last
generators were then run at the smaller sizes above, one job at a time, at a load of 10 to 22.
What ran at the plan's size of 300 000 models: `atomic`, `loop`, `run`, `run-atomic` and `nrpr` (O2 of `nrpr` at 30 000);
the audit K = 3 and `go test -race` at scale were not repeated.

### B (c). Partial-order reduction x `provided` + rendezvous x the Stepper's timeout

The analysis reads enabledness and footprints and refuses `timeout`, `provided` and rendezvous
channels. Checked that the reasons are still accurate and that the refused run is the full search:
a scenario outline over `provided-rv-send`, `provided-rv-recv` (reason names `provided`) and
`timeout-gate` (reason names `timeout`), asserting the same statuses and as many states as the
baseline; the oracles above (the `provided` generator checks that a refusal is the full search, in
the scale runs too). No interaction found.

### B (d). Parallel search x everything

| What | Size and seed | Result |
|---|---|---|
| the parallel oracle on plain random models (`TestParallelAgreesWithTheSequentialSearchesOnRandomModels`, with and without the sweep, every knob set) | 50 000 models, seed 12 000 001 | 47 376 complete (27 656 with a violation), 2 622 ending in an error of the model, 2 skipped; no disagreement |
| rich models (rendezvous, `run` pools, `timeout`, `provided`, never claims, failing property expressions) | 20 000 models, seed 13 000 001 | pass |
| determinism across workers and budgets | 20 000, seed 14 000 001 | pass |
| the `nrpr` shape (table model without `run`) | 30 000 models, seed 15 000 001 | 29 996 complete (17 489 with a violation), 4 skipped, no disagreement |
| failing-property models (`...WhenAPropertyFailsToEvaluate`: a property that fails on a state after another decided it) | 10 000 models, seed 16 000 001 | 7 755 complete, 1 502 of them with a property that fails after it was decided, 2 245 ending in an error; 0 failed |
| corpus differential at 1 and 8 workers (`TestParallelAgreesWithTheSequentialSearchesOnTheCorpus`) | the integrated tree | 137 Promela files accepted, 106 compared (95 complete, 11 with an ending event, 19 with atomic steps), 25 refused for a temporal property, 32 violated properties replayed |
| `g8-parallel.feature` and the root suite | | pass; new: an outline of five rows (`--fairness weak`), one of three (`provided`/`timeout`), one scenario on `nrpr-property.json`, and the `atomic-t3` scenario rewritten |

- `--workers` with `--fairness weak`: the run is refused (temporal property), says why, and equals the
  sequential weak-fairness run except for the `parallel` object (five rows: two never-claim models, a
  timeout claim, `--ltl`, `--progress`). `--workers` with `--por`: the reduction wins where it applies
  (and now on atomic sequences, which step 6 reduces); where it refuses (rendezvous) the parallel search
  runs and `search.reduction` keeps its reason.
- The Stepper's timeout change and the `provided` fix against the parallel search: `provided-rv-send/recv`
  and `timeout-gate` give the breadth-first run's statuses, states and transitions in parallel (scenario),
  and the rich generator covers them at scale.
- `ErrInternal` from a worker: not possible by construction (it comes from the cycle search, which a
  parallel run never executes; a run with a temporal property is the sequential one).
- Two nets for an engine defect: §4, item 2. The CLI path of a worker's panic (scratch build, exit 1,
  empty stdout, one message on stderr) and the MCP path (one isError line, `error` in the manifest, one
  stack on the server's stderr, the next call served).
- `manifest.json` with parallel sessions: 12 concurrent parallel `mc_check` calls beside 60
  `mc_lint_property` calls on one session, six rounds with the injected fault and six without: valid
  every time, 74 calls recorded, no temporary file left.
- The wire contract of the seven tools, `tools/list` of 0.2.0 against the merged server: only additions
  (`workers` in the input of `mc_check`, `search.parallel` in its output, `workers` and `max_workers` in
  the manifest), and the `description` strings of the budget `depth`, `por`, `constant` fields; no field
  renamed, retyped or removed, no `required` list changed.
- A property refused for the process table beside a parallel search (unit tests: the others are answered
  as `--bfs` answers them, with and without the sweep, 1, 2 and 4 workers; scenario on `nrpr-property.json`)
  and a call whose properties are all refused (found a defect: §4, item 1).

### B (e). Weak fairness x the lasso fix

- The SCC oracle on new seeds, 100 000 models each (`MCD_WF_MODELS=100000`), none and weak fairness:
  seeds 9 100 001, 9 300 001, 9 500 001: 95 449, 95 595 and 95 508 models compared of 100 000 (the rest has no
  such property, is not decided, or is stopped by the clock: 1, 2 and 0 models in an atomic loop that never
  gives up the control), 0 disagreements; fairness changes the verdict in 1 377, 1 302 and 1 296 models.
  The lasso panic skipped 604 of 90 000 rows in the weak-fairness record's run; the helper that skipped
  them is gone and the oracle runs the engine plainly.
- Differential against `pan -a|-l [-f] -E -c0 -m200000`, both directions, on 2 000 new generated models
  (the author's generators: seeds 30 000 to 30 999 and 40 000 to 40 999; `spin -a`, `gcc -O2 -DNOREDUCE`,
  the engine `--budget-states 200000`): rows compared without fairness 862 + 827 = 1 689, **0
  disagreements**; with weak fairness 764 + 730 = 1 494, 23 disagreements: 22 "engine violated, pan no
  error" (15 `provided` models, D2; 7 stopped systems whose claim keeps moving, D1; in each of the 7 the
  loop of the engine's counterexample contains a stutter step, and with `pan.c` edited to restore the
  stutter step under `-f` the rows still give no error except one that gives 2, the class the weak
  record describes) and 1 "engine verified, pan error" (`m40633`, class C5: `pan -a` finds no error at
  all, `pan -a -f` finds one with `-o1 -o2 -o3` and with plain `spin -a`, none with `-DNOSTUTTER`; the
  SCC oracle agrees with the engine). The Promela-level SCC oracle on the same directories: 951 of
  1 000 and 906 of 1 000 compared, 0 disagreements.
- Lasso exposure (the claim-`else` models whose old `violated` was the spurious "end state in claim
  reached"): 154 + 155 generated models with `else` in the claim; 315 violated runs, 215 with a rendered
  lasso, 100 "end state in claim reached"; no run crashed. 39 of the second 1 000 models are refused
  by the frontend (`outside-subset`, the atomic loop that shares its entry): all 39 checked to be exactly
  that refusal.

### B (f). `_nr_pr` x weak fairness / LTL, and the refusal x parallel

- Weak fairness on table models: `TestWeakFairnessMatchesTheSCCOracleOnTableModels` (new) gives the oracle's
  random cases an end location and an `-end-` edge that leaves the table behind `youngest` on every
  non-claim process and `_nr_pr` guards, with `provided`, timeout guards, atomic edges and the claim
  anywhere: 1 500 models by default (1 429 compared, 0 disagreements), and 2 x 100 000 on new seeds
  (71 000 001 and 72 000 001): 95 921 + 95 743 compared, 0 disagreements, 20 448 with the claim not
  last, 30 487 with a `provided` clause, fairness changes the verdict in 1 321 + 1 287 models.
  The layout lists processes (never the claim) in its table, so a claim in the middle does not shift
  `youngest`.
- Promela models that read `_nr_pr` with `active` processes, under none and weak fairness, in the four
  modes (accept labels, progress labels, a never claim, an `ltl` formula; 1 000 models, seeds 50 000 to
  50 999): the SCC oracle compares 858, 0 disagreements. Against pan: in the mode without a claim
  (accept labels) 225 rows without fairness and 200 with weak fairness, **0 disagreements**; with a claim
  (never claim, `ltl`, non-progress) the engine and pan differ, as documented: pan counts the claim
  in `_nr_pr` (10 + 14 + 13 rows without fairness, 9 + 17 + 15 with weak fairness). Giving pan the same models with every `_nr_pr` constant one higher (in the three modes that have a
  claim; the same 1 000 shapes), the difference goes away: without fairness 765 rows (225 accept
  labels, 163 never claims, 178 `ltl`, 199 non-progress), **0 disagreements**; with weak fairness 722
  rows, 7 disagreements, all "engine violated, pan no error" and all of the documented class D1 (the
  engine's loop contains a stutter step; no `provided`; `pan.c` with the stutter step restored still says no
  error). That confirms that the divergence under a claim is the claim counted in `_nr_pr` and nothing else.
- The refusal x parallel: §B (d).

### What was not verified

- Any platform but linux/amd64, and the release itself: no binary was rebuilt, no `build.sh` was run, the
  reproducibility check and the platform smoke tests are the maintainer's; the tracked binaries are 0.2.0.
- The mutation harness of step 6 was not run in full on the merged tree (54 mutants, hours): `go test
  ./tools/pormut` passes (every anchor applies, every layer matches a test), which says the harness is intact,
  not what the new `nrpr` generator adds to it. The audit with K = 3 and `go test -race` at the scale
  of the oracles were not repeated; `go test -race -short` ran on `explore`, `mcp` and `cli` (§E).
- The POR oracles ran at 300 000 models for `atomic`, `loop`, `run`, `run-atomic` and `nrpr` only; `base`, `reads`
  and `atomic-reads` at 60 000 (O2: 20 000), because of the load of the machine.
- The panic nets were checked end to end only on a scratch build with an injected fault (not committed);
  the fixed engine has no input that makes a worker panic, so the CLI and MCP paths of
  `*explore.InternalError` and of `explore.ErrInternal` have no automated end-to-end test.
- CTL x the other branches was exercised by the suites (the graph builder carries the BFS fix; a CTL
  run is refused by the reduction and the parallel search) but no campaign targets it.
- `mc_simulate` on models with the live-process table and on `timeout` models after the Stepper change:
  covered by the branch's own tests and scenarios, not by a campaign.
- Speedups of the parallel search, and every number that depends on the load, were not re-measured; the
  load while the campaigns ran is written next to them (4 to 60, most at 10 to 25).
- The runners of the pan comparisons are scratch scripts (the weak-fairness author's); the generator of the
  `_nr_pr` models is `steps/integration-0.3.0-nrpr-wf-gen.py`, the weak-fairness author's generators are in the
  scratch of that branch and are not committed; the numbers are.

## 4. What the integration found, and what was done

(Each item was written test first where a test could be written; the red state is in the
commit message.)

1. **The parallel search reported itself applied when nothing was searched** (`1caa4ea`).
   A call whose properties are all refused for the process table (nrpr) searches nothing,
   and the parallel dispatch ran before that was known, so `search.parallel` said
   `applied: true` with zero workers and zero layers. `chooseParallel` now knows the run is
   idle and answers not applied with a reason. Red: `TestParallelWhenEveryPropertyIsRefusedForTheTable`.
2. **The answer to a panic in a parallel worker carried the Go stack** (`1ae4340`).
   `recoverTool` (cycle branch) keeps the stack out of the answer and the manifest and writes it
   to the server's standard error; a panic in a worker comes back from `explore.Run` as an
   `*explore.InternalError` whose message carries the stack, and `mc_check` passed it on unchanged:
   the stack, with absolute paths, reached the client, the manifest entry and `manifest.json`.
   `Server.internalToolError` answers with the first line plus the sentence `recoverTool`
   uses and writes the full message to standard error once. Checked end to end on a scratch
   build that panics in the parallel walk (not committed), through `mcd serve`: one isError
   line, `error` in the manifest in memory and on disk, one stack on stderr, the next call
   served, exit 0 at shutdown; 12 concurrent parallel `mc_check` calls beside 60
   `mc_lint_property` calls on one session, six rounds with the fault and six without:
   `manifest.json` valid every time, 74 calls recorded, 12 errors with the fault and none
   without, no temporary file left. The CLI path (exit 1, empty stdout, one message) was
   checked on the same build. The two nets do not stack: a worker's panic is recovered by the
   pool and returned as an error, not re-panicked, so neither `cli.Run`'s recover nor
   `recoverTool` sees it.
3. **A divergent atomic loop killed the breadth-first searches** (`1bf3ff5`, `775fc8c`).
   The sequential BFS and the CTL graph copied the whole chain of moves at every step of an
   atomic sequence (n steps cost n²/2 references). Latent in 0.2.0 (the parallel record notes
   it), but unreachable from `atomic { do ... od }`, which the old frontend lowered outside the
   block. With the weak-fairness frontend change the block is real and a loop that never ends is
   one line: `mcd check --bfs` and `mcd check --estimate` (that is `mc_estimate`, which the skill's workflow
   runs before the exhaustive check, `workflow.md` node 9) ran out of memory in two seconds at 2.2 GB under a 6 GB cap,
   with no end in sight, where 0.2.0 answered in ten milliseconds (for the wrong reason). In
   `mcd serve` that is the end of the server. The chain is now a persistent list turned into a
   slice only for a stored state or a trace; the divergent block answers
   `inconclusive`/`bounded` at the existing bound of 100 000 steps in 1 s and 283 MB (estimate
   0.1 s, 41 MB). Red: 1 584 MB allocated for a sequence of 5 000 steps, in the BFS and in the
   CTL graph. The scenarios were written after the fix because the unfixed engine takes the test
   process down, and the fixture went to `testdata/unbounded/` after the first version, in
   `testdata/promela/`, killed the corpus-wide parallel test (those tests run the depth-first
   search with no depth budget, which does not bound an atomic sequence either: the same model
   under `mcd check` is bounded by the default depth budget).
4. **Stale tests and a stale document, found by the merge**: the nrpr test and scenario that
   said the reduction is refused (§2), the g8 scenario on `atomic-t3.pml` (§2), the 44-model
   citations (now 50), `PROMELA_SUBSET.md` §2.1/§4.1 (CTL and weak fairness listed as not
   implemented since G4 and G5), and the POR corpus floors (§5).
5. **The weak-fairness oracle could hang**: a model with an atomic edge that loops for ever
   with no guard keeps the cycle search busy with no state to store, in 0.2.0 too; the oracle
   had no clock, so a campaign met it about once in 100 000 models and stood still. Each run has a
   10 s deadline now and a run that meets it is a counted skip.
6. **The oracle helper that hid the lasso panic**: `runUntilLassoPanic` turned exactly the panic
   the cycle fix removes into a skipped row, so a regression of that fix would have been
   invisible to the weak-fairness oracle. Removed: the engine is run plainly.

7. **`go test -short ./explore` failed** (`b31e093`), with and without `-race`, on the merged tree and on the
   weak-fairness branch alone: the oracle's "the generator is too tame" check wanted three models per
   mode where fairness changes the verdict, and 600 models give 1, 2 and 0 (the same floor sat exactly on the
   claim mode's count at the default size: 3 000 models give 9, 21 and 3). The floors are counted over all
   the modes and proportional to the run (found by the `go test -race -short` of the final checklist).

No oracle failed on production code, and no verdict of a model differs between the merged engine and the
engine of the branch that fixed it. The defects above are in the interactions (1 to 3: code, 4 to 7: tests
and documents).

## 5. The merged engine against 0.2.0, by cause

The published `engine/bin/mcd-linux-amd64` (0.2.0) and the merged binary, `check` with
`--no-timing --budget-states 20000 --budget-depth 20000 --budget-ms 120000`, under an
address-space cap and `timeout 120`, over 476 inputs (the SPIN corpus including the files
without a suffix, the fixtures of `testdata/{promela,corpus2,mutate,weakfair,weakdecision,spin-divergence}`,
the IR and Petri fixtures, `evals-workspace`, `skills`) in 12 command lines each: plain, `--sweep`,
`--bfs`, `--por`, `--por --sweep`, `--ltl '<>false'` (with and without `--sweep`), `--progress`, `--fairness
weak` (alone, with `--ltl '<>false'`, with `--progress`, with `--ltl '[]<>false' --sweep`).
`--workers` is new and has no 0.2.0 counterpart (§3, parallel). Exit code and standard output
compared byte for byte.

476 inputs: 339 existed at 0.2.0 (the SPIN corpus, the earlier fixtures) and 137 were added by the
branches (each a repro of a fix). 5 712 comparisons: 4 068 on the existing inputs, 1 644 on the
new ones. 4 279 are byte-identical; 1 433 differ. Each difference was attributed to the first merge
that changes it, by running the binaries built at the five merge commits on every differing pair.

| First changed by | Differences | On models that existed at 0.2.0 | On the branches' new fixtures |
|---|---:|---:|---:|
| the cycle-lasso merge | 185 | 43 | 142 |
| the weak-fairness merge | 1 071 | 508 | 563 |
| the `_nr_pr` merge | 56 | 0 | 56 |
| the partial-order merge | 118 | 82 | 36 |
| the parallel merge | 0 | 0 | 0 |
| the integration's own commits | 1 | 1 | 0 |
| the wall clock of a loaded machine | 2 | 2 | 0 |

By cause, on the models that existed at 0.2.0 (633 differing comparisons):

- **cycle (43)**: 0.2.0 stopped with a Go panic (exit 2, no report) and the merged engine answers: 9 models in 11 files
  (`CH15/uts_model` under `--sweep` and the temporal variants, `CH2/prodcons2.pml`, `CH5/pathfinder.pml`,
  `App_C/petrinet1`, `App_C/petrinet2`, `testdata/mutate/sample.pml`, `testdata/promela/atomic-t5.pml`,
  `abp-instrumented.pml` and `alternatingbit-ghost.pml` of the iteration-3 evals, in four copies). No other difference.
- **weak (508)**, all in the `--fairness weak` variants and `--progress`: 279 differ in counters and trace
  only (the null step that closed a round was a stored state of its own: fewer product states, a lasso
  without the note "every process has moved or been blocked once"), 122 in the trace only, 14 in the counters only
  (9 files; `CH2/protocol` and `CH2/protocol2` under `--progress` store 63 product states, as `pan -l`, where 0.2.0 stored 36),
  88 on 85 files change `progress` from `violated` to `verified` (the false non-progress cycle at the final
  state of a model that ends), 4 files change it from `violated` to `inconclusive` (the spurious cycle used to
  stop the search early: `bench-indep`, `bench-sym`, `CH12/leader`, `CH9/leader`) and 1 to `invalid-model`
  (`CH3/counter`: the search now reaches the `count--` below zero that the search without fairness reports too).
  Not one of them is a change in the other direction.
- **partial-order (82)**, all under `--por`: the report of the reduction (`applied`, `reason`, the note, the
  reduced and fully expanded counts) for the models with atomic sequences or `run` that 0.2.0 refused (59, 29 files),
  with the states and transitions of the reduced graph (21, 11 files), and `bench-sym` where the reduced search now
  finishes inside the 20 000-state budget (`deadlock` `inconclusive` to `verified`, 2).
- **integration (1)**: `testdata/ir/par-atomic-loop.json --bfs`: 0.2.0 runs out of memory under the cap (exit 2), the
  merged engine answers `inconclusive` (§4, item 3).
- **clock (2)**: `CH14/version4` and `CH15/client_server.pml` with `--ltl '<>false' --sweep` reached the 120 s
  limit of the harness at a load of 60 to 160; under a state budget of 3 000 the two binaries give byte-identical reports.

Without `--fairness`, `--por` or a temporal property (`plain`, `--sweep`, `--bfs`): 1 017 comparisons on existing
models, 1 016 byte-identical, 1 different (`CH15/uts_model --sweep`, the panic). With a temporal property but no
fairness (`--ltl '<>false'`, `--progress`): the panics above and the `CH2/protocol` counters only.

On the new fixtures the differences are the repros: the verdict changes of §6 (`provided-rv-*`, `atomic-loop-entry`,
`dstep-loop-entry`, `claim-else-idle`, `weakfair-timeout-claim`, `nrpr-active`, `nrpr-order`, `nrpr-ltl`, the weak-fairness
models), the refusal of `nrpr-property.json`'s properties and of `atomic-loop-option.pml`, the documented divergence of
the `spin-divergence` fixtures (the claim counted in `_nr_pr`), `new_atomic_hold.pml` (`violated` or `verified` became
`inconclusive`: the atomic loop is real now) and the panics of the `claim-atomic-*` models that 0.2.0 does not survive.

No default-search verdict changed on a model that existed in 0.2.0, and none changed in a way
that a branch record does not list. Every difference was attributed to the first merge that
changes it (binaries built at each merge commit, run on every differing pair).

Goldens: no existing golden changed. Three were added by the branches
(`nrpr-unread.{ir,report}.json`, `par-default-bench-indep-3.report.json`).

## 6. Verdict-changing fixes, for the release notes

Direction: **false violation** = 0.2.0 said `violated`, the right answer is `verified`;
**missed violation** = 0.2.0 said `verified`, the right answer is `violated`. "Right" is
SPIN's `pan` where the construct is in the subset, the textbook definition otherwise (weak
fairness). Each item says which models, and in which search it shows.

| # | What was wrong in 0.2.0 | Direction | Models (fixtures; corpus where one is affected) | Branch |
|---|---|---|---|---|
| 1 | `_nr_pr` never fell in a model without `run`: a guard that waits for the count (`A: (_nr_pr == 1)`, `B: skip`) was a `deadlock` | false violation (default search, exhaustive evidence) | `nrpr-active`, `nrpr-order`, `nrpr-ltl`; no corpus model (`CH15/client_server` has `run`) | nrpr |
| 2 | a property over `_nr_pr` (CTL atom, `invariant`, `reach` through the IR) in a model whose processes keep no table was answered with the count that never changes | both (`AG (_nr_pr == 2)` verified, `EF (_nr_pr == 0)` violated), exhaustive | `nrpr-unread`, `testdata/ir/nrpr-property.json`; now `not-executed` with the reason | nrpr |
| 3 | `atomic { do ... od }` and `d_step { do ... od }` with the loop as the first statement of the block lowered the loop outside the block: the process let go of the control at the back edge | false violation of an `assert` in the default search (and false answers with a claim); the frontend now refuses the one shape it cannot lower (the loop head shared with another alternative: `outside-subset`, 39 of 1 000 generated models) | `atomic-loop-entry`, `dstep-loop-entry`; `atomic-loop-option` refused | weak |
| 4 | a process whose `provided` clause was false took part in a rendezvous, on either side | missed violation (`deadlock`) and false violation (`assert`) in the default search | `provided-rv-send`, `provided-rv-recv` | weak |
| 5 | `else` in a never claim was always enabled, so a claim that had to keep looping fell off its closing brace ("end state in claim reached") | false violation | `claim-else-idle` (0.2.0, with and without fairness); 19 of 3 000 generated claim-`else` models | weak |
| 6 | the timeout moves of the system were enumerated for the first enabled edge of a claim only | missed violation, with a nondeterministic claim (an LTL automaton has one) in a timeout state, with and without fairness; and a product that lost half its states | `weakfair-timeout-claim`, `weakfair-timeout-null`; `CH2/protocol --progress` stored 36 product states where `pan -l` stores 63 (verdict unchanged), now 63 | weak |
| 7 | `--fairness weak`: a null-step cycle was reported on any accepting state in which every process is blocked | false violation | every model that ends or blocks under `--progress`: 88 of the comparisons above on 85 existing models, 126 corpus runs in the branch's own comparison (and `violated -> inconclusive` on `CH12/leader`, `CH9/leader`, `bench-indep`, `bench-sym`, `violated -> invalid-model` on `CH3/counter`); `weakfair-claim-blocked`, `-claim-m5271`, `-np-claim`, `-np-deadlock`, `-progress-r1` | weak |
| 8 | `--fairness weak`: a process whose next statement is a bare `timeout` counted as blocked in every state | false violation | `d3_one`, `d3_ltl`, `d3_np`, `claim-atomic-timeout` | weak |
| 9 | `--fairness weak`: copy k of the n+2 copies stood for process k-1 although the claim need not be the last process (an IR from `--ir` or an inline `ir` argument) | missed violation | `testdata/ir/weakfair-claim-{first,middle}.json` | weak |
| 10 | `mc_simulate` listed timeout moves next to ordinary ones and `Apply` accepted them | wrong simulation (no verdict) | any model with `timeout` and a second process | weak |
| 11 | a Go panic in the lasso of an acceptance or non-progress cycle that crosses an atomic sequence: exit code 2 with no report, or a dead `mcd serve` | crash, no verdict | `CH15/uts_model --sweep`, `CH2/prodcons2`, `CH5/pathfinder`, `App_C/petrinet1`, `App_C/petrinet2`, `testdata/mutate/sample`, `testdata/promela/atomic-t5` under `--ltl`/`--progress`, and the four `claim-atomic-*` fixtures; 43 comparisons on 9 existing models above | cycle |

Not verdict changes, listed so that nothing is claimed twice: the partial-order reduction
changes what `--por` reports (counts, `search.reduction`) and never a verdict; `--workers` is
new and opt-in; the exit code of an internal failure is 1 with a message (was 2 or a dead
server); `manifest.json` is written atomically; a call whose properties are all refused
searches nothing; the BFS and the CTL graph are bounded in memory on an atomic sequence.

## 7. Known open issues (none of the five records' items is dropped)

Tagged with the record that carried them. "Resolved" says what the integration did.

**From `fix-cycle-panic-confirmation.md`**
- On a blocking atomic the state count is 5 against pan's 4 (`A: do :: atomic { x = 1; (y == 1); x = 0 } od`, `B: do :: y = 1 - y od`); the verdict agrees; pre-existing. The engine stores the state with the exclusive-control byte set while the holder is blocked, pan does not. Seen again on 13 of 115 and 104 of 300 generated models (all with an atomic block that can block inside, none without) in §3.
- The corpus-wide tests walk `*.pml` only (13 corpus files have no suffix, `CH15/uts_model` among them). Partly resolved: `MCD_CORPUS_ALL_FILES=1` runs the POR corpus differential on them too (248 files, 150 accepted, 131 compared, 78 applied, 35 smaller, 53 refused, no disagreement); the default walk and the parallel corpus test are unchanged.
- A fatal runtime error cannot be recovered: a stack overflow in `mc_parse` of a 3 MB input with about 1.5 million nested parentheses (between 100 000 and 400 000 levels) still ends `mcd serve` and the CLI with exit code 2. Ticket: a nesting-depth limit in the Promela and LTL parsers.
- `pan -a -c0` stores 45 313 states on `CH15/uts_model` against the engine's 45 311; which two is untraced; no effect on the verdict.
- `mcd serve` itself (`cmd/mcd/serve.go`) is outside `cli.Run`'s recover: a panic in the transport or in `mcp.New` would end with exit code 2 and a trace.
- Not verified there: the claim's guards in the replay of a lasso (the stepper has no claim API; the guards are covered by the verdict and the state count against pan); the guards only through an injected panic; the rendering of every cycle after the verdict under `--sweep` is still done and dropped; the `s.tmp` balance check does not tell that an entry in use is the right one; M3 (the `if !found` release in `innerDFS`) is an equivalent mutant; a failed `rename` or `write` of the manifest was not injected; the handlers read and write `Session.seq`, `cexs` and `model` without `s.mu` (a burst of 200 + 200 calls under `-race` reported nothing; not demonstrated, not investigated); a Go stack of a developer build carries absolute paths; `--fairness weak` on the timeout model of the third round (resolved: §3, item A).

**From `fix-weakfairness-confirmation.md`**
- The 22 to 28 generated rows where the engine says `violated` and `pan -f` says no error are two documented classes (D1: a stopped system whose claim keeps moving, `pan.c` switches the claim's stutter step off under `-f`; D2: a process kept from moving by `provided`, which `pan -f` counts as not blocked, and misses depending on the order of the processes) and one in the other direction (C5: pan's "accept stutter" artifact, which depends on SPIN's optimisation flags). Confirmed again on 2 000 new models in §3.
- `pan -f` has no answer on 184 of the author's weak rows (its search ran out of depth), and on every model it refuses.
- The oracle's gaps: a never claim with an `assert` or an `atomic` edge, `run`, an atomic sequence that does not end within 200 steps, a product above 20 000 states (4 000 in the random runs). What it shares with the engine: `Stepper.Enabled`/`Apply` (so the semantics of a move: rendezvous with `provided`, `timeout`, atomic), the stutter extension, "enabled means has a move of its own", the claim guard evaluation. Its agreement proves the decision procedure, not conformity with pan on those four; the independent checks are the hand graphs of `testdata/weakdecision` (`wfcheck.py`) and the pan rows.
- The 299-row split and the "48 of 48 before, 0 of 46 after" against `pan -l -f -A` of the first round are the author's counts, not independently reproduced.
- Strong fairness is not executed; CTL does not use the construction; no timing or memory benchmark of the weak-fairness product (fewer states from the null-step fix, more from the timeout moves of every claim edge); `evals-workspace` still holds records of what the tool answered then (the old note "every process has moved or been blocked once" and one Petri-net run with a lasso of null steps alone).
- An atomic loop that never gives up the control is `inconclusive` (the engine stores no state inside an atomic sequence, so no cycle closes there; pan cannot answer either). Seen on the generated models: about one in 100 000 keeps the cycle search busy until a clock stops it (the CLI's default time budget does; a library call without a context deadline does not).
- The frontend refuses `atomic { do ... od }` as one option of an outer `if`/`do` (the loop head shared with another alternative); node splitting (duplicating the options for the first iteration) would lower it exactly but changes the location structure the state counts rest on, and was not attempted. 39 of 1 000 generated models of the weak-fairness author's second generator hit it.
- Decisions D1 (stutter extension kept under weak fairness), D2 (`provided` false is blocked), D3 (timeout, changed), C1 (`hasEnabled` and the exclusive holder: not the cause, the frontend was), C5 (pan's accept stutter kept as an artifact) stand as the branch recorded them (`fairness.md` §6b).

**From `fix-nrpr-confirmation.md`**
- OPEN, a decision for the maintainer: should the engine count the `never` claim, and the claim of an `ltl` formula, in `_nr_pr`, as pan does? Today it does not, and every verdict that reads `_nr_pr` under a claim can differ from pan's by one (`testdata/spin-divergence`, two `@spin` outlines pin the difference). What it would cost: the layout's count includes a claim process, `pid = run P()` (`NrPr() - 1` in `lower.go`) subtracts the claim, and the claim of an `ltl` formula (not a process of the layout) needs the extra one separately in LTL, `progress` and accept-label runs. No warning at parse time either.
- OPEN: where the contract "an end edge must carry `leave`, guarded by `youngest(k)`, for `_nr_pr` to fall" is documented. Today only `testdata/ir/README.md`, the record and Go comments; the skill's references describe no hand-written IR schema.
- Hand-written IR whose processes read `nrpr` while the peer's end edge has no `leave` keeps a table that never falls and answers exhaustively wrong (identical in 0.2.0; it cannot be closed statically in general). Likewise an unguarded `leave` on an older process (`Layout.Leave` pops the youngest entry whoever leaves). Follow-ups: a report warning when a table is needed and no edge has `leave`; an `invalid-model` error when a `leave` fires in a process that is not the youngest.
- A call whose properties are all refused returns `search.stop: ""`, `complete: false` and counters 0, and a refused row carries the run's counters and `complete: true`; cosmetic. (With `--workers` such a call now says the parallel search was not applied and why: §4, item 1.)
- The CTL refusal is conservative: `active proctype A() { do :: x = 1 od }` with `AG (_nr_pr == 1)` was a correct `verified` in 0.2.0 and is `not-executed`; the reason gives the workaround.
- The reason text suggests `assert(_nr_pr >= 0)` (cosmetic); the partial-order reason of `nrpr-youngest` named `(nrpr)` before step 6 and the refusal itself is gone since.
- Not verified there: the values `_nr_pr` takes are compared with pan only through verdicts and state counts, never state by state; the refusal has no SPIN counterpart; `mc_simulate` was not run on those models; the pan numbers of the divergence table are the same with `spin -a` and `spin -a -o1 -o2 -o3`, but `nrpr-order.pml` does not compile with plain `spin -a` (a global `done` collides with pan's own).
- Noticed there, not touched: `PROMELA_SUBSET.md` §2.1 listed CTL and weak fairness as not-executed. Resolved by the integration (`f42cbb4`).

**From `perf6-confirmation.md`**
- The soundness argument (macro-step induction with the exclusive-byte equivalence, the table cell, the two checks) is the plan's, reviewed on paper; not proved mechanically. The oracles check it on random models up to K = 2 macro-steps of the others (K = 3 on small models in the scale runs), not in general; the audit shares the engine's enabledness and firing code, so a misreading of the semantics that the full and reduced searches share is invisible to it (SPIN is the outside witness, on the shapes the fuzzers make).
- Models above 5 000 states under the oracles (their budget) are seen only through the corpus, the benchmarks and the scale series. The oracles see a hole in the footprints only in the shapes the generators make; hand-built shapes that no generator makes (a `run` initialiser that assigns a global, a heterogeneous pool, an atomic edge into a sink, the length of a channel named by a value, a failing assert inside a chain) are pinned by directed tests only, and the harness at its default size is not sensitive to rare shapes (mutant x21 first survived 3 000 models of every generator).
- Mutants a4 and r13 survive and are argued equivalent, not proved; a12 and x10 cover for each other (pinned now); r12 (check c2) changes nothing the oracles see.
- Not repeated in the branch's later rounds: the audit with K = 3, the 100 000-model fuzz through the frontend, `go test -race` at scale (it does not work under `ulimit -v`), the full run of the 54 mutants (8 were run on the final tree). Platforms other than linux/amd64; any performance claim under a load other than the one stated.
- Refused, as decided there: `provided` (decision 2), rendezvous channels (rejected), channels named by a value, `timeout` (postponed), breadth-first search and temporal properties; two shapes of process creation the frontend never emits (a dynamic process that goes back to its dormant location without leaving the table, a `run` that enters its target at the dormant location).
- The interplay with the parallel stream, which the record lists as not merged: done here (§3): the parallel search is refused where the reduction applies, the reduction wins, the oracles ran on the merged tree.

**From `perf5-confirmation.md`**
- No speedup at a load of 2 or below was ever measured; the figures are hints taken under 3 to 5 other runnable tasks (x4.8 to x6.8 at 8 workers on the two models that decide the stop rule); two-node NUMA placement of two workers can decide the result (6.0 s on cores 0-3, 15.6 s on cores 0 and 8 against 12.4 s sequentially). `indep` N=6 missed A1 (x1.04 unpinned, x1.33 pinned) and `two1000` missed A7 at eight workers by 7%.
- Any platform but linux/amd64; race freedom is evidence (the race detector after the first race was fixed) on top of an argument by construction, not a proof; `--budget-ms` is not reproducible; the capacity of a partition (2^24 - 1 states, a 4-byte id) is exercised only through a lowered limit; the "overshoot by about one group" sentence of the memory budget rests on one measurement (949 MB estimate for an 800 MB budget, 1.1 to 1.2 GB resident against 864 MB sequentially, on a model with 3 KB states); `mc_estimate` is untouched and sequential, and the server's `concurrency` counts calls, not workers (the CPU a server may use is `concurrency` times the worker ceiling).
- The sequential breadth-first search kept a chain of moves per intermediate state of an atomic sequence (memory quadratic in its length) and the depth-first search has no bound on an atomic sequence of its own in the library (the CLI's depth budget bounds it). The first is resolved (§4, item 3); the second stays: a library call with no depth budget on a divergent atomic block grows the stack until killed (`testdata/unbounded/`).
- The changes of the third review round (the wording of the depth statements, the oracle's empty-trace check, three scenario groups) are texts and tests that no fourth round reviewed; the larger campaigns, the 49-mutant campaign and the 756-run wire comparison were not repeated at their size; a property expression that fails to evaluate can end one search `invalid-model` where another completes (the parallel frontier is in partition order); on `par-atomic-depth.pml --budget-depth 1` the default search reports depth 1 where `--bfs` reports 0 (existing behaviour); `perf5-plan.md` is frozen and still says "BFS depth".
- Decisions awaiting the user (defaults taken, each reversible): (1) `search.mode` is `bfs` and `depth` the number of layers when the parallel search is applied; (2) a run that mixes a safety property with `ltl`/`progress`/`ctl` is refused as a whole; (3) `--por --workers`: the reduction wins and the report says the parallel search was not applied (a usage error is the alternative); (4) the MCP worker ceiling is `GOMAXPROCS` (`mcd serve --max-workers`); (5) the stop rule A3 (abandon below x3 at 8 workers on `bench-indep` N=6 or `counters-10-6` at a load of 2 or below) was never tested at that load. The decisions of the step-6 record: `provided` not implemented, rendezvous rejected, dynamic channels and `timeout` postponed, the `_nr_pr` difference out of scope (now fixed).

**Found by the integration**
- A library call of `explore.Run` with only a state budget (no depth budget, no clock) on a divergent atomic block grows the stack of the depth-first search until the process is killed (`testdata/unbounded/README.md`); `mcd check` and `mcd serve` always carry a depth budget (1 000 000 by default) and a time budget, and answer `inconclusive`. Found when the first version of the BFS test fixture, put in `testdata/promela/`, killed the corpus-wide parallel test.
- The coverage regression of the frontend change, in one sentence for the release notes: 39 of 1 000 models of the weak-fairness author's second generator (atomic loops as one option of an outer `if`/`do`) were accepted by 0.2.0 with a wrong lowering and are refused now with `outside-subset`.

## 8. Unreviewed since

- **The second round of `fix/weak-fairness-null-steps`** (its section 11: `90d88fd` to `4738e1c`, twelve commits: `provided` on both sides of a rendezvous, the frontend change for atomic loops, the Stepper timeout change, `blocked` for timeout processes, claim-not-last, timeout moves of every claim edge, `else` in a never claim, the extended oracle and the 32 mutants). The branch's own record says so ("the second round has not been cross-reviewed"), it exceeded its brief with three extra wrong-verdict fixes (the loop lowering, `else`, the timeout moves), and the merge reaches the default search, not only `--fairness weak`. What the integration did about it: the 0.2.0 comparison (§5: no default-search verdict changed on a model that existed in 0.2.0), 2 000 new generated models against pan in both directions with every residual row classified (§3), the SCC oracle on 300 000 new random models and 200 000 new table models, on three directories of 1 000 generated Promela models and on the six held-back ones, and the atomic-loop and `_nr_pr` fuzzers. None of that is a review.
- **The integration itself**: the five merge resolutions, the code and test changes of §4 (`chooseParallel`'s idle case, `Server.internalToolError`, the persistent chain of the BFS and the CTL graph, the strict weak-fairness oracle with its clock, the `nrpr` generator and the two Promela fuzzers, the changed POR tests and scenarios of §2), and the documents of §10.
- The bounded BFS (`1bf3ff5`) changes code that every breadth-first run goes through; the reports are byte-identical to the previous merged binary on the 5 712 comparisons and the full suite passes, but it is new code with no review.

## 9. Test results of the final tree

`cd model-check-plugin/engine`, SPIN 6.5.2 and gcc present, so the `@spin` scenarios and `tools/pandiff` ran,
`go` 1.26.1, linux/amd64, a shared machine (load 4 to 8 during the runs):

- `go build ./...`, `go vet ./...` clean; `gofmt -l .` prints nothing.
- `go test -count=1 -p 2 -timeout 60m ./...` at `b6fa826` (the production code of the final tree: no non-test Go
  file changed since `1bf3ff5`): every package `ok` (`modelcheck` with the godog scenarios 307 s,
  `explore` 189 s, `tools/pandiff` against SPIN 278 s, `tools/pormut` 4.7 s, the rest under 10 s); 8 minutes
  wall. After the last commit (`b31e093`, test floors only) `go test ./explore ./tools/pormut` again `ok`
  (201 s and 4.7 s).
- `go test -race -short -count=1 ./explore ./mcp ./cli`: `ok` (explore 462 s, mcp 19 s, cli 1 s), after the fix of
  item 7 of §4 (the first run failed on that floor only).
- `go test ./tools/pormut` `ok`.
- The worktree was clean after the suite (the platform scenario of `g6-package.feature` builds into a scratch
  directory); `git diff 738627b..HEAD` touches no file under `engine/bin`, no `SHA256SUMS`, no `BUILD-INFO.json`, no
  manifest, no version.

## 10. Documents brought into line

README (the internal-error entry, the corpus counts of the parallel test, the oracle and fuzzer descriptions), PROVENANCE (the stale `_nr_pr` sentence, nine generator shapes), `engine-tools.md` (exit code 1 and the isError message for a parallel worker's panic), `fairness.md` (50 models), `PROMELA_SUBSET.md` (property kinds, state counts), `por_corpus_test.go` (floors and the figures of the integrated tree: 190 files, 137 accepted, 120 compared, 75 applied, 35 smaller, 45 refused). The version string, the tracked binaries, `SHA256SUMS` and `BUILD-INFO.json` are as published for 0.2.0: the release rebuild, the platform smoke tests and the reproducibility check are the maintainer's decision, and the shipped binaries still contain every defect this record lists as fixed.

## 11. Decisions for the maintainer

Judgement calls taken during the integration (each can be reversed):

- **The reduction applies to the models the `_nr_pr` fix now lowers with a table.** The `_nr_pr` branch's tests said
  such a model is refused by `--por`; step 6 removed the refusal (the table is a cell of the analysis). The more
  conservative choice is to refuse a model whose static processes read `_nr_pr` and leave the table (a few lines in
  `analyzePOR`). Not taken, because the evidence is for the cell: the new `nrpr` generator (300 000 models through
  O1 and O3, 30 000 through O2), the 100 000 Promela models and the 120 and 150 against pan, the fixtures, the corpus.
  The tests that said "refused" were changed to say "applied and answers as the full search".
- **`--por --workers` stays "the reduction wins"** (perf5 decision 3), now also on atomic sequences, which step 6
  reduces; the g8 scenario that used `atomic-t3.pml` as the example of "the reduction refuses" uses a rendezvous model.
- **A panic of a parallel worker is answered like a panic of a tool handler**: one line in the answer and the
  manifest, the stack on the server's standard error (`1ae4340`). The alternative, keeping the stack in the answer,
  contradicts the cycle branch's rule that the manifest carries no stack.
- **The BFS chain fix** (`1bf3ff5`) changes code every breadth-first run goes through; the alternative was to
  document that `--bfs`/`mc_estimate` can run out of memory on a divergent atomic block. Not taken: that is the
  end of `mcd serve` on a one-line model, in a call the skill's workflow makes before every exhaustive check.
- **No change to `explore.Run`'s own depth-first search** on a divergent atomic block (the library API carries no
  default budget); `testdata/unbounded/` holds the model, out of the corpus walks.

For the maintainer:

1. **Whether to release this tree as 0.3.0** (or a patch release): §6 lists changes of default-search verdicts, so the release notes need it. The records say the weak-fairness branch "must not be released without `fix/cycle-lasso-panic`": it is merged here.
2. **Count the claim in `_nr_pr`?** (§7, nrpr, OPEN.)
3. **The new refusal** (`atomic { do ... od }` as one option of an outer `if`/`do` is `outside-subset`): keep, or build node splitting (changes the location structure and the state counts of the corpus).
4. **A bound for the sequential depth-first search on an atomic sequence in the library** (the CLI and the server have the depth budget; `explore.Run` with a state budget only does not). Not changed: it is a default of the library API.
5. **Widen the corpus-wide tests to the files without a `.pml` suffix** by default (cost: the POR corpus test goes from 2 s to 24 s; the parallel corpus test was not run with them).
6. **Hand-written IR**: whether to add the two checks of §7 (a warning when a table is needed and no edge has `leave`; `invalid-model` when a `leave` fires in a process that is not the youngest) and document the `leave` contract next to an IR schema in the skill.
7. **Parallel defaults** (§7, perf5, decisions 1 to 5), unchanged and still awaiting an answer.

## 12. After the integration: a quorum review, the extended campaigns, the release build

Added after §1 to §11 were written. The head of the record above is `b31e093`; this section covers
`bfefda4` (CI and the security gate) to `b02bda0` (the 0.3.0 artifacts). Where it contradicts §8
("Unreviewed since") or the "not repeated" items of §3, this section is the later word.

### 12.1 The review

Six reviewers read the parts that no one had reviewed (the second round of `fix/weak-fairness-null-steps`,
the five merge resolutions, the follow-up commits of §4, the breadth-first change `1bf3ff5`), each
blind to the others: four Claude reviewers (one per area, read-only, with a built CLI to try models),
Codex (gpt-5.6-sol, reasoning high, the same four areas, read-only sandbox), Qwen3.8-27b and
Gemma-4-31b through Coddy (the diffs in blocks of at most 120 lines, because the larger prompts timed
out; a block that timed out was halved and sent again). Every finding was checked against the code, and
where a model could be written, run, by the orchestrator; a vote makes a finding urgent, it does not make it true.
SPIN was not installed on the machine of the review: the claims about what `pan` answers were reasoning from
Promela semantics at the time. SPIN 6.5.2 was installed afterwards and the fixtures of items 1, 2 and 6 were run through
`pan -c0`: it agrees with the fixed engine on items 1 and 2 (no error on the four merged-head fixtures, one error on
`atomic-loop-first-break-leaves`) and finds no error on the forward-label shape of item 6, where the engine reports a violation.

| # | Finding | Reviewers | Checked | Outcome |
|---|---|---|---|---|
| 1 | The back edges of a loop inside an atomic or d_step block were found by comparing raw node ids; an iteration that ends in a node merged with the head later (an inner loop left by `else -> break`, a `goto` to the loop's label) gave up the control mid-loop: a **false `assert` violation** (`atomic-loop-merged-break`, `-goto-head`, `dstep-loop-merged-break`). Introduced by `d20b6cb` on the weak-fairness branch, not in 0.2.0 | Claude A | run, three models, red on the old build and green on the fix | fixed `5b143c4` (marking waits for `finalise`) |
| 2 | The fix of 1 made the edge before a non-first `break` of a loop that is the only statement of an outer loop's option a "back edge" (its target is the outer head): `d_step { do :: x = 1; break od }` ended `invalid-model` ("did not finish"), the atomic one kept the control after leaving the block and starved the other process (`inconclusive` where `pan` and 0.2.0 find the violation) | Codex A, Codex C | run, two models, 0.2.0 correct, `5b143c4` wrong | fixed `fe0d131` (`breakFrom`) |
| 3 | The copy index of a weak-fairness null step was kept in an `int8`: from 126 processes the lasso showed the null steps as stutter steps (the verdict was right) | Claude B, Codex B | run, 130 processes | fixed `5b143c4` (`int16`) |
| 4 | The CTL and simulate campaigns of this section could pass having compared nothing (every model skipped) | Codex C | read | fixed `91f9e85` (floors) |
| 5 | Two scenarios of `g2-mcp.feature` asked for three or four workers and failed on a machine with fewer CPUs (the server clamps to `GOMAXPROCS`) | the release run, not a reviewer | run under `GOMAXPROCS` 1, 2, 3 | fixed `a39fcf3` (two workers; the file says two CPUs are needed) |
| 6 | A forward label into an atomic loop (`goto L; do :: atomic { L: do ... od } :: other od`) overwrites the label node's block metadata, so the refusal of a loop that shares its entry with another option does not fire and the other option runs inside the block: a false `assert` violation. The same in 0.2.0 | Codex A | run (0.2.0 and the fix give `violated`; without the `goto` the shape is refused) | **open**, not a regression |
| 7 | `insideAll` compares the lengths of the block stacks, not their members | Gemma, Qwen (Codex A's finding 6 is the same root) | read; no model that fails by it alone was found | **open**, latent |
| 8 | The SCC oracle of weak fairness builds its graph with `Stepper.Enabled`/`Apply`, so a fault of the rendezvous, `provided` or timeout code is invisible to it; only the hand graphs of `testdata/weakdecision` and the `pan` rows are independent | Claude B, Codex B | read (as `fairness.md` §6b already says) | **open**, a limit of the oracle |
| 9 | A starved receiver of a rendezvous counts as "blocked" for weak fairness, so a run in which the sender always pairs with another receiver is judged weakly fair; the textbook definition says it is not | Claude B | run | **open**, a decision (the oracle shares the premise; `pan` unchecked) |
| 10 | A claim's `provided` clause is ignored by the product (a hand-written IR only; the frontend never emits one) | Codex B | read | **open** |
| 11 | A claim guard that reads `timeout` may be evaluated with a stale flag after a phase-1 enumeration | Claude B | read, low | **open** |
| 12 | The breadth-first search still keeps a chain per stored state: an atomic loop in which every step can also leave (`do :: break :: x++ od`) stores states with chains of length 1..n, quadratic memory; `--bfs` and `--ctl` die at 2.4 GB where 0.2.0 died too | Claude D (run), Codex D | run | **open**, not a regression; `1bf3ff5` fixes the shape with one exit at the end |
| 13 | The time budget and the context are not looked at inside an atomic descent that stores no state (`i<40`, two options: 2^40 paths): `--budget-ms 500` ran until killed; memory of intermediate copies is not in `MemBytes`. The same in 0.2.0 | Claude D (run), Codex D | run | **open**, not a regression |
| 14 | The reason "depth budget exhausted: an atomic sequence exceeds 100000 steps" appears whatever budget the user gave | Claude D | run | **open**, cosmetic |
| 15 | Weak floors: the short-mode floors of two POR generators and of the weak-fairness oracle cannot fail; rows stopped by the clock are skipped without a floor; the test of the parallel panic answer calls the helper, not the route; the BFS memory test has no memory limit | Claude C, Codex C, Codex D | read; one run of the short oracle | **open**, tests |
| 16 | Refuted: `cany` never reset (Gemma, Qwen: the frame is new for every expansion), `k = 0` indexing `sys[k-1]` (Gemma: guarded), a data race on `layout.Timeout` (Gemma: the cycle search is single-threaded, the parallel workers have copies), a missing `f.enabled++` (Gemma), "the fix lost the edge filters" (Gemma: it still requires the head and excludes the break nodes), an unused `els` (Qwen: would not compile), `openLoops` not reset after an error (Qwen: an error ends the whole lowering) | Gemma, Qwen | read / run | no action |

The merge resolutions (Claude C, Codex C): no verdict-changing defect, no line of either side lost
except the ones the record lists as superseded, the wire contract (schema text, flags, exit codes)
consistent. Codex also looked at `1ae4340` (the parallel panic answer) and found it sound.

### 12.2 The extended campaigns

All on the tree at `bfefda4` plus the two campaign tests, from a separate worktree, through a queue that
admitted a job only when it fitted (CPU and memory use below 90% including the job's own share; a running
job was paused when the machine went above 90% and resumed below 75%). The machine was shared with other
sessions; the queue paused a job five times and resumed it five times.

| Campaign | Size and seed | Result |
|---|---|---|
| Mutation harness of step 6 (`cmd/pormut`) | 54 mutants, 3000 models per generator, 2 workers, 3610 s | 45 killed, 7 pinned by a directed test, 2 survived (`a4-dirok-for-atomic-edge`, `r13-dormancy-read-dropped`: the equivalents the step-6 record argues), 0 against the contract; the baseline was green |
| Audit O2 with K = 3 | 20 000 models for each of nine generators, seed 970 000 001, 3790 s | about 10 million (state, process) pairs, 4 358 and 3 582 of them at a `run` edge (`run`, `run-atomic`), 0 failures |
| `go test -race` at the scale of the oracles | O1 and O3 at 30 000 models per generator, seed 980 000 001; the parallel oracle on random and `_nr_pr` models at 30 000 (seed 980 000 001); the SCC oracles at their default size; 5 590 s | 0 failures, no data race; the other packages under `-race` at their default sizes passed (the root package in 244 s). The first `-race` run of `./explore` failed at Go's default test timeout of ten minutes (the package takes 27 minutes under `-race` here); CI now has `-timeout 60m` |
| CTL against an independent evaluator (`explore/ctl_campaign_test.go`) | the graph is rebuilt from the Stepper and every formula is evaluated by naive fixed points; 100 000 models for each of five generators without atomic sequences, 8 random formulas of depth up to 3 each, seed 990 000 001, 1 638 s | 500 000 models, about 2.7 million compared verdicts of formulas, 0 disagreements. The oracle was shown to fail when `EF`/`AF` is deliberately broken |
| `mc_simulate` on models that read the process table (`mcp/simulate_table_campaign_test.go`) | 3 000 generated Promela models (active and `run` processes that snapshot `_nr_pr`), 60 seeds each, each seed asked twice through the MCP server, against the engine's own exhaustive search of the same model; 2 759 s | 180 000 runs (60 728 stopped by `deadlock`, 119 272 `terminated`), 0 failures: no snapshot value outside the reachable set, no `deadlock` or `assert failed` that the search does not find, no two answers for one seed; 11 294 of 12 532 reachable non-zero snapshot values were seen |
| Speed-up of the parallel search | two machines, see `release-0.3.0-speedup.md` | `indep6` x5.3 and `counters` x3.5 at 8 workers (local), x4.9 and x5.4 (`ml`), against the default search; no speed-up on `two` and `chain` |

What these do not say: the CTL oracle shares the Stepper with the engine; the simulate campaign compares
with the engine's own search, not with SPIN; the weak-fairness and `pan` comparisons of §3 were not repeated
(SPIN was not installed); the speed-up numbers are not a controlled experiment at a stated load.

### 12.3 The release build

`./build.sh --version 0.3.0 --source-commit 4a6fbb1… --verify-repro`, Go 1.26.8 (the script runs with
`GOTOOLCHAIN=local`, so a Go 1.26.8 must be on `PATH`; the machine's Go is 1.25.0 and the pinned toolchain
of the module cache was used; a first build on Go 1.26.1 was replaced because it contained 13 reachable
standard-library vulnerabilities, see `release-0.3.0-confirmation.md`), five platforms, two builds that agree on every generated file; `SHA256SUMS`
verified; `mcd version` prints `mcd 0.3.0`; the version is in `report.EngineVersion`, both manifests and three
goldens (the report's engine record). Record: `release-0.3.0-confirmation.md`. Public notes:
`../RELEASE-NOTES-0.3.0.md`. Nothing was pushed to the public repository, tagged or published by this
section: publishing is the maintainer's step.

### 12.4 What is still open

Items 6 to 15 of the table in 12.1; the items of §7 and §11 (decisions for the maintainer) are unchanged,
except that the release bar of 300 000 models per generator was met for the audit with K = 3 (20 000) and
for `-race` (30 000) only at the sizes in 12.2, not at 300 000. SPIN-dependent checks (`tools/pandiff`
against `pan`, the `@spin` scenarios) were run on the release tree after SPIN 6.5.2 was installed and passed
(`release-0.3.0-confirmation.md`); the weak-fairness comparison with `pan` of §3 was not repeated.
