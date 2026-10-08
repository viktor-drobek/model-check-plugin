# Performance plan, step 5 — parallel exploration of the safety search

Layer: G0 (`explore`), with thin additions in `cli`, `report` and `mcp` (G2). Follows
`perf1..perf4-confirmation.md`. Protocol: `BUILD-PROTOCOL.md` step 6. The plan, as
reviewed in phase 1, is `perf5-plan.md`; this is the record of phase 2, the
implementation, and, in the review log at the end, of the cross-review of the diff
(phase 3) and of what was changed after it.

## What was asked, and what was built

`mcd check --workers N` (and `mc_check` `workers`) searches the safety properties
(`deadlock`, `assert`, `invariant`, `reach`) with N workers: a level-synchronous
breadth-first search over a visited set split into 256 partitions by a fixed hash,
in which every partition is written by exactly one worker at a time and nothing is
locked. Every result is a function of the model, never of the worker count, a
segment size or a clock, except under `--budget-ms`, which is a clock and is not
reproducible. It is opt-in and refuses, with the reason in the report,
what it cannot yet do. Without the flag the engine is unchanged: the report of 162
default runs (every fixture under `testdata/promela`, `ir` and `petri`, with the
flags `--sweep`, `--bfs`, `--por`, `--progress`, budgets) is byte-identical to the
one of the unmodified binary, apart from the line numbers of one existing panic's
stack trace (see "Found along the way"), and so are the golden files.

| Plan step | What | Commit |
|---|---|---|
| 1 | `explore/parvisited.go`: fixed hash, `partSet`, ids, capacity stop, byte count | `b08de10` |
| 2 | `explore/parallel.go`: the level-synchronous search, batched and inline paths, one worker | `6e698db` |
| 3 | events with a total key, the deterministic merge, counterexample re-derivation; `Options.Workers`, `Result.Parallel`, the dispatch in `Run` and its refusals | `48c2a09` |
| 4 | the rest of the semantics through `fire`: generators, directed models, the corpus differential | `4877bf3` |
| 5 + 6 | the goroutine pool, cancellation, the panic path, the tripwire; the budgets (exact state budget, depth layers, memory estimate, the record guard) | `b74125b` |
| 7 | the surfaces: CLI flag, `search.parallel`, the mode rule, MCP `workers`, the feature file | `5f5cc4f` |
| 8 | tuning: batched record accounting, sorted segments, O(1) work per level, the inline threshold; group records 16 MiB | `788953e`, `91bf41f` |
| 9 | hardening: the mutation campaign, the SPIN differential, coordinator panics, deeper campaigns | `735af75` |
| 10 | docs: README, `engine-tools.md`, `workflow.md`, this file | the commit that adds this file |

Steps 5 and 6 are one commit because both rewrote the driver; their tests are
separate files (`parallel_pool_test.go`, `parallel_budget_test.go`).

## The design as built, and where it differs from the plan

The design of §2 of the plan is what was built. Differences, each with its reason:

- **The option and the dispatch came at step 3, not 7.** The oracle runs through `Run`,
  and a commit that gives wrong verdicts for a refused case is not a state to leave
  behind, so `Options.Workers`, `Result.Parallel` and the refusals (a temporal property,
  an applicable `--por`, a caller-supplied visited set) came with the events. Step 7 added
  the CLI, the report, the MCP field and the scenarios.
- **The record guard cuts a group after the longest prefix whose records fit, instead of
  halving it recursively.** The plan's halving needs a group to be discarded and redone,
  which an inline group cannot do (it has inserted already), so the inline threshold would
  have changed results under a small memory budget. The prefix rule is a function of the
  counts of records alone and is applied by both paths: an inline group expands one state at
  a time into a small buffer before it inserts it, so a state that does not fit is left out
  whole. A group that overflows in the batched path is thrown away, the prefix is counted by
  a sequential pass, and the prefix is run again. A mutant that reran the whole group inline
  instead survived because the inline path cuts at the same prefix: equivalent, and the
  counting pass is kept for the exact figure of the one-state message.
- **The guard's shared counter is batched.** The plan has every worker add each record's
  bytes to one shared counter before it writes the record. That was the first thing the
  measurement found (see "Measured"): one cache line fought over by every core, 39% of all
  CPU of an 8-worker run. A worker now adds in batches of 16 KiB and at the end of a segment.
  The batches sum to the same total, so whether a group overflows is still a function of its
  total record bytes alone.
- **A segment is copied partition by partition** (a counting sort) into one piece per
  partition instead of being read through a permutation of record numbers: the owner of a
  partition then reads its records in one piece, and the tail group of an exact state budget is
  run again as an inline group instead of through a second insertion path (the plan's
  "global record order" is what an inline group already does).
- **The byte count of the set is kept from the partitions each group touched**, and
  the frontier and group buffers are reused. A model of two million layers (`par-chain.pml`)
  otherwise pays a loop over 256 partitions per layer: x2.4 the sequential time before, x1.0
  after.
- **The inline threshold is 4096/workers states, between 512 and 4096** (the plan's 128 was
  the prototype's). One worker or a narrow layer does not make up for batching (`two1000`:
  x0.55 with the batched path at one worker, x0.80 with this threshold).
- **Forcing the group size in a test is semantics for a run that stops early.** The counters
  of a run stopped by an event or a budget are those of a group boundary, so §5.2 of the plan
  ("segment and group sizes forced to 1, 3, 1 000 000 give the same report, budget-truncated
  ones included") cannot hold for the group size. What is tested, and holds: the segment size,
  the inline threshold and the worker count never change a result, with or without a stop, and
  the group size changes only the counters of a run that stops early (verdicts, traces, stop
  reasons and the counters of a run that completes are the same). The target size of a group's
  records is 16 MiB (64 MiB in the plan): at 64 MiB the memory bound A9 was missed.
- **The report has no `atomic_steps` field**, so the feature scenarios compare `states` and
  `transitions`; the atomic-step counter is compared in the Go oracle.
- **Scenario 29 (an injected panic) is not a Gherkin scenario**: no input can make a worker
  panic. It is a set of Go tests in `explore`, `cli` and `mcp` (an injected panic in each
  phase and in the coordinator returns an `InternalError`; the CLI maps it to exit 1 with the
  message on stderr and the MCP tool to a tool error; the call returns and no goroutine is left).
- **A group that consumes no state without stopping the run is an `InternalError`**, and a
  panic of the coordinating goroutine is one too, instead of a loop or a crash.

## Behaviour pinned

- `explore/parvisited_test.go`: the set against a map (lengths 0 to 5000, across table growths
  and chunks), ids that name the partition and the arrival index, `Bytes` independent of the
  order of insertion, the hash pinned on literals and against an independent statement (big
  integers), balance on structured vectors, equal fingerprints told apart, the capacity stop.
- `explore/parallel_test.go`, `parallel_oracle_test.go`, `parallel_gen_test.go`,
  `parallel_semantics_test.go`: the differential oracle against the sequential depth-first and
  breadth-first searches. Complete runs: status, evidence and reason of every property
  (the reason of an assert is compared as "assert violated"), `states`, `transitions`, atomic
  steps, vacuity coverage, `Levels` and depth (equal to the breadth-first ones without atomic
  sequences), every counterexample replayed as a run of the model through the `Stepper` (also
  when the last step is the one that failed, and through states whose enabledness cannot be
  listed to the end), its length equal to the breadth-first one. Runs with an ending event (an
  error of the model, a pool exhausted, an atomic bound): nothing `verified`, every trace
  replays, and the searches may disagree on whether the run ends only about an error in a
  property expression (a decided property is not evaluated again, so such an error exists only
  in a search that meets the state first). The models are those of the plain generator
  (`randomPORModel`, the reduction's) and of a rich one built on it (atomic steps where they
  cannot loop, rendezvous, `run` pools that can be exhausted, `timeout`, `provided`, never claims,
  guards and property expressions that fail to evaluate, a vacuity watch). **Until the review of
  the diff the oracle ran every search with `Sweep: true`, with one worker, and a model got at
  most one property that can fail to evaluate**; both limits hid the defect of finding 1 of the
  review log. It now also runs every model without the sweep (where a search stops once every
  property is decided and a decided property is not evaluated again), with 1, 2 or 4 workers by
  seed, checks the exhaustive verdicts of a complete run (an invariant left undecided is
  verified, a reach condition left undecided is violated) against the reachable graph that
  `BuildGraph` stores, which needs no order to agree on, and treats a run that stops early
  without the sweep as no witness that the graph has no error. Its generators: the rich one
  gives half of the models two to four more invariant and reach properties, some of which fail to
  evaluate where another is decided, and a third generator
  (`randomFailingPropertyModel`: small acyclic models, three to six properties over `a[i]`,
  `6 / j`, `i`, `j`) builds that pair on purpose.
- `explore/parallel_oracle_test.go`, after the review: the two models of the defect
  (`parSkippedReach`, `parSkippedInvariant`) over every knob set, 1, 3 and 4 workers, with and
  without the sweep, against both sequential searches; a failing watch expression (never skipped,
  ends the run, as in the sequential search).
- `explore/parallel_oracle_test.go`, after the second review: a trace is also checked against what
  its verdict says about the state it ends in (`replayOutcome`): the invariant is false there, the
  reach condition true, no move is enabled in a deadlock state that is not terminated, and the last
  step of an assert trace is a step in which an assert fails. The check is part of the replay walk,
  so a step that matches several edges is settled by the one that passes it. In the check against
  the reachable graph the final state must also be a stored state of it, with the expression
  evaluated there. Before this, a trace to another state of the same layer (a valid run, the right
  length, its own final valuation) passed everything (round 2, finding 3). After the third review a
  trace of no step is also refused for a violated assert (the walk returned at its first call, before
  the assert check, and the final-state switch has no assert case, so an empty assert counterexample
  passed); a hand-built outcome with an empty trace is a negative test for the four kinds, which also
  shows the real traces accepted (round 3, finding 3).
- `explore/parallel_events_test.go`: the event order by direct construction (the smallest key
  of a property wins, an error ends the run and drops what follows it, an error of a decided
  property is dropped, all properties decided drops the rest, a deadlock follows the steps of
  its state), the retention of the first key of a class, the step counter (a stored successor
  is checked before the next guard fails), the key of the state budget event.
- `explore/parallel_pool_test.go`: 1, 2, 3, 8 and 64 workers give the same result, under a
  scheduler perturbed at every barrier and with GOMAXPROCS 1 and 2; one worker starts no
  goroutine; none is leaked; a panic in a worker or in the coordinator returns an
  `InternalError`; a write to the set during the expansion panics (tripwire); a deadline that
  expires during the expansion discards the group, one that expires during the insertion lets
  it finish, and the run stops at a boundary that an uncancelled run also has; a stress test
  (one-state model, a chain, 64 workers, segments of one state).
- `explore/parallel_budget_test.go`: the state budget is exact (every size from 1 to total + 1,
  five knob sets, 1 and 3 workers); an assert after the stop point is not claimed; the depth
  budget cuts at a layer and says what the breadth-first search says (sentence, states, depth,
  `Levels`; on a model without atomic sequences: with them a layer counts an atomic sequence as
  one hop and the two cut at different states, pinned by two scenarios of the feature file) and
  still checks the stored layer; the memory budget; the record guard cuts a group (the run has more groups than
  levels) and a state that does not fit ends the run with the sentence; the groups are the same
  for any worker count and segment size; random models under random state, depth and memory
  budgets give the same report for 1, 2, 5 and 16 workers.
- `parallel_corpus_test.go` (root): every Promela file of the fixtures, `corpus2` and the SPIN
  corpus that the frontend accepts (107; 92 compared, 81 of them complete, 11 with an ending
  event, 15 with atomic steps; the others finish above the test budget or are refused for a
  temporal property; the first version of this record said 104, 89, 78, 11 and 13, counted
  before the fixtures `par-atomic-depth.pml`, `par-dstep-depth.pml` and `atomic-guard-first.pml` of
  the three review rounds were added), at 1 and 8 workers; the temporal ones must be refused and equal the
  sequential run.
- `tools/pandiff`: `TestDifferentialCorpusParallel`, 22 models (chapters 2 and 3, the atomic
  fixtures, `leader`, `client_server`, `pathfinder`, `merging`) at 1 and 4 workers: the number of
  stored states is `pan -c0`'s, with the sequential engine's error class and completeness.
- Features: `features/g8-parallel.feature` (34 scenarios, written first, seen red against the
  engine without the flag, `@pending` removed with step 7; after the review of the diff 41: a
  scenario outline of four rows for the defect of finding 1, written first and seen red on the
  unfixed code, and an outline of two rows and a scenario that pin the depth semantics of
  finding 2 on `testdata/promela/par-atomic-depth.pml`; after the second review 44: an outline
  of three rows on `testdata/promela/par-dstep-depth.pml` pins that a `d_step` block is one move,
  and one unit of `depth` and of `--budget-depth`, in the default search, `--bfs` and `--workers`;
  after the third review 60: an outline of five rows on `atomic-at.pml` and one of five on the new
  `atomic-guard-first.pml` pin where `--bfs` and `--workers` cut under `--budget-depth` 1 to 5 for an
  atomic block that blocks part-way and for one that starts with a guard (depth, states,
  transitions, stop reason, status), an outline of two rows pins the unbudgeted run of both, an
  outline of three rows (`--ltl`, `--progress`, `--ctl`) and a scenario (`--por`) pin that a refused
  run counts `--budget-depth` in transitions, and the `d_step` outline now pins all three searches
  on stop reason, depth, states, transitions and statuses)
  and six in `features/g2-mcp.feature`
  (workers applied, absent without it, clamped to the ceiling with a note, a refusal is not a
  tool error, `por` with `workers`, `search: "bfs"` with `workers`). Unit tests in `cli` and `mcp`.

## Evidence

- **Differential campaigns** (`MCD_PAR_MODELS`, `MCD_PAR_SEED`) before the review of the diff:
  200 000 models of the two generators (100 000 each, from seed 3 000 000) and 60 000 before the
  final changes (30 000 each, from seeds 1 000 000 and 2 000 000), no disagreement. **That was
  not evidence of absence**: the review found a wrong exhaustive verdict (finding 1 of the review
  log) with a seed range of its own, because the generator gave a model at most one property
  that can fail to evaluate and the oracle ran only with the sweep (see "Behaviour pinned").
  On the unfixed code the rich generator, with the extra properties, meets the defect in about
  1 model of 30 000 to 25 000 (seed 61 000 000: 1 failing model in 30 000; seed 71 000 000: 2 in
  50 000); the generator built for the pair, in about 1 of 1 300 (seed 70 000 000: 45 failing models
  in 60 000, of which 34 report a reach condition violated or an invariant verified that the
  reachable graph contradicts or that was never checked, and 11 a trace longer than the
  shortest). What the earlier campaigns covered: of the rich generator's 100 000 models, 50 882
  complete (33 119 with a violation), 44 767 ending in an error or a bound, 4 474 with atomic
  steps, 20 732 with a `run`; of the plain one's, 94 819 complete (55 335 with a violation) and
  5 180 ending in an error. Further 20 000-model campaigns (seed 4 000 000): the
  report is the same for 1, 2, 3, 8, 16 and 8 workers with random segment sizes and inline
  thresholds; random budgets give the same report for 1, 2, 5 and 16 workers; the states of a
  run that ends in an error are reachable states.
- **Campaigns after the fix** (the test binary built from `c5c78ac`, which has the fix and the
  oracle changes; the later commits change comments, texts, one threshold (the generator's sanity
  bound, `0c1bd7f`), one test (the failing watch expression, `0c1bd7f`) and the text of the `note`
  that the report carries (`206a99e`), which no campaign reads; every oracle
  model is run with and without the sweep, with 1, 2 or 4 workers (the plain generator's test:
  1) and one of five knob sets by seed; machine load 3 to 9 while they ran; seed ranges that no
  earlier campaign used, except the reviewers' own; none failed):

  | Generator | Seeds | Models | Complete | Ending in an error or a bound | Notes |
  |---|---|---:|---:|---:|---|
  | failing-property (new) | from 70 000 000 | 60 000 | 46 796 | 13 204 | 9 417 with a decided property that fails to evaluate on another state |
  | failing-property (new) | from 74 000 000 | 60 000 | 46 706 | 13 294 | 9 399 of those |
  | rich, with extra properties | from 71 000 000 | 50 000 | 17 563 | 30 932 (+1 505 over the budget) | 24 514 with extra properties, 1 504 with atomic steps, 10 923 with `run` |
  | rich, with extra properties | from 75 000 000 | 50 000 | 17 532 | 30 978 (+1 490) | 24 607 with extra properties |
  | rich, the reviewers' range | from 31 000 000 | 60 000 | 21 030 | 37 174 (+1 796) | the range on which the review found the defect |
  | plain (`randomPORModel`) | from 72 000 000 | 50 000 | 47 439 | 2 561 | 27 635 with a violation |
  | determinism (1, 2, 3, 8, 16, 8 workers, random segment and inline sizes, random budgets) | from 73 000 000 | 20 000 | n/a | n/a | the report is the same for every worker count |

  On the same two ranges the unfixed code fails 45 of the 60 000 failing-property models (from
  70 000 000) and 2 of the 50 000 rich ones (from 71 000 000; seeds 71006021 and 71015456), and the
  reviewers' model, seed 31 034 067, on its own. The oracle is therefore sensitive to this defect.
  How sensitive it is to others is what the mutation campaign above measured, with the oracle
  of that day; it was not run again with this one.
- **The race detector**: `go test -race -short -run 'TestPar...'` on `explore`, `cli`, `mcp`,
  `tools/pandiff` and the root corpus test, green, after the first run found a real race (see
  below). 64 workers on GOMAXPROCS 1 and 2 are in the tests.
- **Mutation campaign** (a scratch harness, not part of the release, with the baseline run
  first and required green): 49 textual mutants of the concurrency- and order-critical rules of
  §5.6 — insertion out of frontier order, a partition inserted by two workers, last parent
  wins, dedup against the current level only, a group's last segment unexpanded, transitions
  per record, atomic steps lost on the inline path, events unsorted or sorted by the step
  counter, an event after a terminating one, first verdict stands removed, the exact tail and
  the depth layer, a memory estimate that depends on the worker count, a discarded group that
  does not stop the run, a shared compiled copy, a salted or degraded hash, the last duplicate
  move in a trace, the checks skipped on the inline path or on the stored layer, a goroutine for
  one worker, the depth plus one, a witness not carried, a trace ending at the parent, a refusal
  removed, the step counter, the budget event's key, the group size from the worker count or
  the segment size, the order of the checks, an error of a decided property applied, the
  initial state's checks, a skipped partition, the frontier range, an errored fire leaving a
  record, cancellation inside the insertion, a panic that does not arrive at the barrier, an
  assert applied with its error, the last event kept, `Workers` changing the mode, an unflushed
  segment, the byte count, the atomic bound, the stop flag, the MCP note, the mode rule.
  **47 killed, 2 survive and are equivalent**: the rerun of a group inline after an overflow
  (the inline path cuts at the same prefix), and the stop flag read at the end of `stopped()`
  (the loop that takes segments reads it too, so the mutant only keeps expanding the segment in
  hand: latency, not result). Three were killed only by a hang (a discarded group that does not
  stop the run, an extra goroutine that makes every test sleep, a barrier that is never left);
  the tests bound a hang by the test timeout. **Eight mutants first survived, each a gap in
  the tests**, now closed: the first of two moves to the same state (a trace must name the
  first), a stored successor checked before the next call of `nextEnabled` fails, the key of the
  budget event, the group size from the worker count (needs layers wider than 4096 states at
  64 workers: `counters(6,7)`) or from the segment size, the flush at the end of a segment, the
  exact bound of an atomic loop, and a half-written vector left by an errored fire (which no
  oracle compared, because a run that ends in an error is not compared with the sequential one:
  the states of every run are now required to be reachable states).

## Measured

Machine: 16 vCPU (Xeon Gold 6154, two NUMA nodes of 8 cores as the VM shows them), 30 GB, Go
1.26.1, shared with the other stream and with reviewers. `mcd check --sweep --unlimited
--no-timing`, the binary built from this branch, `bench5.py`: the sequential depth-first search
and every worker count run in turn in each round, five rounds (two for `bench-indep` N=6), median
and spread over the rounds. **The background load was never at or below 2**: before each round
3 to 4 other tasks were runnable (`/proc/loadavg`, minus the sampler), 1-minute load 3 to 6. By
the rule of the plan these figures **are not benchmarks**; they are what the machine gave, with
its load.

Unpinned (the scheduler places the threads; the spread of a cell is 5-27%):

| Model | states | seq. DFS | 1 worker | 2 | 4 | 8 | 16 |
|---|---:|---:|---:|---:|---:|---:|---:|
| `bench-indep` N=5 | 579 195 | 0.586 s | x0.81 | x1.36 | x2.00 | x2.71 | x2.60 |
| `counters-10-6` | 1 000 000 | 1.944 s | x1.48 | x2.48 | x3.60 | **x5.95** | x6.33 |
| `bench-indep` N=6 | 8 108 731 | 12.47 s | x0.95 | x1.04 | x2.77 | **x4.83** | x5.89 |
| `client_server` | 191 200 | 0.353 s | x0.87 | x1.29 | x2.09 | x2.90 | x3.01 |
| `two1000` (narrow, 4 005 layers) | 4 010 007 | 2.14 s | x0.80 | x0.79 | x0.86 | x0.93 | x0.88 |
| `chain` (one state per layer) | 2 000 003 | 1.531 s | x1.10 | x1.09 | x1.12 | x1.10 | x1.17 |

Pinned to the eight cores of one NUMA node (`taskset -c 0-7`, five rounds; the background tasks
were not pinned away, so the load figures are the same; spreads 1-7%):

| Model | seq. DFS | 1 worker | 2 | 4 | 8 |
|---|---:|---:|---:|---:|---:|
| `bench-indep` N=5 | 0.498 s | x0.85 | x1.47 | x2.51 | x3.50 |
| `counters-10-6` | 1.472 s | x1.38 | x2.41 | x4.02 | **x6.75** |
| `bench-indep` N=6 | 9.556 s | x0.94 | x1.33 | x3.07 | **x5.20** |

Against the criteria of §1.6 (a figure under the load above, so a hint, not a verdict):

| Criterion | Result |
|---|---|
| A1, x1.4 at 2 workers | `indep` N=5 x1.36 unpinned / x1.47 pinned; `counters` x2.48 / x2.41; `indep` N=6 **x1.04 unpinned / x1.33 pinned: missed** (see the NUMA note) |
| A2, x2.5 at 4 | `indep` N=5 **x2.00 / x2.51**; `counters` x3.60 / x4.02; `indep` N=6 x2.77 / x3.07 |
| A3, x4.0 at 8, and the stop rule x3.0 on `indep` N=6 and `counters` | `counters` x5.95 / x6.75 and `indep` N=6 x4.83 / x5.20: **the stop rule held**; `indep` N=5 x2.71 / x3.50 is below x4.0 |
| A4, x4.0 at 16 and 0.85 of the 8-worker figure | `counters` x6.33, `indep` N=6 x5.89; `indep` N=5 x2.60 (below x4.0; 0.96 of its 8-worker figure) |
| A5, `client_server` x2.5 at 8 | x2.90 |
| A6, wide models at one worker >= x0.8 | x0.81, x1.48, x0.95 |
| A7, `two1000` >= x0.6 at one worker and >= x1.0 at eight | x0.80 at one worker; **x0.93 at eight: missed by 7%** |
| A8, `chain` <= 1.5x the sequential time at every worker count | 0.91x or less at every count |
| A9, `indep` N=6 resident set <= 1.3x at 1 and 16 workers, estimate <= 1.25x | 422 and 445 MB against 354 MB (x1.19, x1.26); estimate 339 MB against 289 MB (x1.17) |

What the figures say. On the two models that decide the stop rule the search is x5 to x7 at 8
workers, which is 60 to 85% of the one-worker-normalised ideal. It is not x5 on the smallest
wide model: `bench-indep` N=5 is a 0.5 s run, the in-process time of `Run` at 8 workers is
0.12-0.14 s and the CLI's 0.14 s (pinned) to 0.22 s (unpinned) includes process start, parsing,
the report and, probably, the first touch of 60 MB of fresh memory by eight threads; that split
was not measured separately.
On a machine of two NUMA nodes the placement of two workers decides the result: the same
`bench-indep` N=6 run with two workers takes 6.0 s on cores 0-3, 6.2 s on cores 0 and 4 and
15.6 s (user time 30 s) on cores 0 and 8, against 12.4 s sequentially; the scheduler's own
placement gave 10 s. Each partition is touched by whichever worker owns it in a group, so a
second node makes most accesses remote. A static owner per partition (a partition's memory
local to one worker) would remove that, at the price of the balancing; it is not done here.
At 16 workers nothing is gained over 8 on this VM, as in the prototype.

**What the measurement found.** The first measurement of the finished feature was x1.8 at 8
workers on `bench-indep` N=5, where the prototype had x5. The profile put 39% of all CPU in
`appendRecord`: the plan's guard adds every record's bytes to one shared atomic counter, and
eight cores fight over its cache line. Batching the additions (16 KiB per worker, and at the end
of a segment), with the sorted segments, took the in-process time from 0.31 s to 0.12 s. Then a loop over 256 partitions per
layer cost a two-million-layer model 1 s; the inline threshold and the buffers were tuned
(x0.55 to x0.80 for the moderate shape). The memory bound was missed at first (estimate x1.30,
resident set x1.30 to x1.38) and met after the target size of a group's records came down
from 64 to 16 MiB, with no change in speed.

Not measured: a quiet machine; `bench-indep` N=6 at one worker with the exact figure of
`worker_bytes_est` against the real compiled copies (the estimate is a formula, 64 MB for 16
workers, mostly the buffers); GC settings other than the default.

## What is refused

With the reason in `search.parallel.reason` and the run executed sequentially and unchanged
(byte-identical to a run without the flag apart from that object, checked on the whole corpus):

- any `ltl`, `progress` or `ctl` property in the run, including `--progress` (a mixed run is not
  split: decision 2 below);
- `--por` where the reduction applies (it is depth-first); where the reduction refuses itself
  (atomic sequences, rendezvous, ...) the parallel search runs and `search.reduction` keeps its
  own reason (decision 3);
- a caller-supplied visited set (a test hook).

`--workers` with `--estimate`, below 0 or above 256 is a usage error (exit 1). A `workers`
request above the MCP server's ceiling is clamped with a note in `budget_notes`.

## What was NOT verified

- Any speedup at load <= 2. See above: the figures are hints under 3 to 5 other runnable tasks.
- Any platform but linux/amd64: the hash reads little-endian explicitly and nothing else depends
  on the platform, but no run on darwin, arm64, big-endian or Windows was made, and `engine/bin`
  is not rebuilt (a release step).
- Race freedom: the race detector found nothing after the first race was fixed; that is evidence
  on top of the argument by construction (one writer per partition, barriers between phases), not
  a proof.
- The time budget's reproducibility (it never was reproducible); the group in flight when it
  expires is discarded, and the figure of `states` is that of a completed group.
- The capacity of a partition (2^24 - 1 states) is exercised only through a lowered limit; a
  skewed hash is excluded by a balance test on structured vectors, not by a run of 16 million
  states in a partition.
- `mc_estimate` is untouched and sequential; the server's `concurrency` counts calls, not
  workers (the CPU a server may use is `concurrency` times the worker ceiling).
- The Promela models that the sequential engine runs out of memory on (an atomic loop with a
  branch: the breadth-first search keeps a chain per intermediate state, quadratic in the length,
  and the depth-first search has no bound on an atomic sequence) cannot be compared with it; the
  parallel search stops at the 100 000-step bound, which the directed test checks to the step.
- Not settled by the review of the diff, and left open: the capacity of a partition (2^24 - 1
  states, a 4-byte id) is verified by reading the code and by the test with a lowered limit,
  never by a run that fills a partition; the speedups were not measured again under a quiet
  machine (the box was at load 5 to 190 while the reviewers ran); no platform but linux/amd64;
  the panic and goroutine-leak path was verified by reading and by the existing tests only.
- The "overshoot by up to about one group" sentence rests on one measurement (an estimate of 949 MB
  for a budget of 800 MB, see round 1, finding 2); it was not measured again.
- The changes of the third review round (wording of the depth statements in every copy, the
  oracle's empty-trace check, three scenario groups) are texts and tests; no fourth round reviewed
  them.

## Found along the way

- **The plan's guard was a scalability defect** (above), found by the first profile and fixed by
  batching. The plan's mutant 28 (the group size taken from the worker count) could be killed only
  by a model with layers wider than the group size that 64 workers would give.
- **My first `partSet` kept a shared state counter** that every owner incremented: a data race the
  first `-race` run reported. `Len` now sums the partitions.
- **A race in my own test hook** (it read the set's size from a worker during the insertion
  phase): worker-stage hooks are given no count now.
- **What the oracle taught about the sequential engine**: (a) its breadth-first search keeps a
  copy of the chain of moves for every intermediate state of an atomic sequence, so a loop of
  atomic steps costs memory quadratic in the length before the 100 000-step bound stops it (about
  100 GB), and its depth-first search has no bound on an atomic sequence at all and explores the
  tree of its branches; (b) an atomic loop arises from a `d_step` that continues into an atomic
  edge, or from rendezvous that hand the exclusive control back and forth; (c) an evaluation error
  in the expression of a decided property exists only in a search that meets the state before it
  decides the property, so even the two sequential searches disagree on whether a run ends in it.
- **A pre-existing bug, not in this stream**: `mcd check --promela testdata/promela/atomic-t5.pml
  --progress` panics in `explore/cycle.go` (`stateOf`, index out of range, from `lasso`) on the
  unmodified engine; a task was filed for it.

## Decisions awaiting the user

The user has not answered the open questions of §9; these defaults are implemented and each can
be reversed:

1. `search.mode` is `"bfs"` and `depth` the number of breadth-first layers for a run in which the
   parallel search is applied (the same as `--bfs`'s on a model without atomic sequences, and it
   can differ on one with them: an atomic sequence that runs through is one unit of `depth` and of
   `--budget-depth` here, one that blocks part-way counts one unit per uninterrupted run, and every
   step of it counts in `--bfs`; a `d_step` block is one move, so one unit, in every search); the
   requested mode stays visible in `search.parallel`. A refused run is the sequential one, with its
   own mode and its depth in transitions.
2. A run that mixes a safety property with `ltl`/`progress`/`ctl` is refused (the whole run is
   sequential); parallelising its safety part is an increment.
3. `--por --workers`: the reduction wins where it applies and the report says the parallel search
   was not applied and why; a usage error for the combination is the alternative.
4. The MCP worker ceiling is `GOMAXPROCS`, set by `mcd serve --max-workers N`.
5. The stop rule A3 (abandon below x3 at 8 workers on `bench-indep` N=6 or `counters-10-6` at load
   <= 2) is kept; it held under load (x4.8 to x6.8), but it was never tested at load <= 2.

## Review log

### Round 1: cross-review of the diff (head `1569428`), and what was done

Three blind reviewers on different models, one brief; the orchestrator verified every finding
against the code before accepting it. Verdict: **approve with changes**, one finding blocking.

What the reviewers could not demonstrate: a hang, a crash, a nondeterministic report (the reports
were byte-identical for 1, 2, 3, 5, 8 and 16 workers), a difference between a default run and the
run of the unmodified binary (1 114 comparisons, all identical), a verdict difference between the
parallel and the sequential search on 131 inputs, or a disagreement with SPIN (22 of 22 models).
What they found:

1. **Blocking, confirmed: a wrong exhaustive verdict** (`checkNew`, `explore/parallel_events.go`).
   After an evaluation error in an invariant or a reach condition, `checkNew` returned, which
   abandoned the checks of the other properties on the same new state. The sequential `checkState`
   skips a property that is decided, so it does not evaluate it and goes on; the parallel search knows
   the statuses only at the start of a group, evaluates the property anyway, and the error event is
   dropped when the events are applied, because an earlier state of the group decided the property.
   The checks the return skipped were never recovered. Model: one process, edges `a[0] = 1` and
   `i = 5`, the invariant `a[i] == 0` (violated by the first successor, an index error on the
   second) and a reach condition `i == 5` or an invariant `i != 5` that only the second decides:
   `--bfs` says `verified` / `violated`; `--workers 1`, `2`, `3`, `4` said `violated` ("no reachable
   state satisfies i == 5 (complete search)") / `verified`, both exhaustive, with and without
   `--sweep`; reachable through `mcd check --ir` and MCP `mc_check` with `properties`, not through
   Promela, which has no invariant or reach properties. **Fixed** (`c9e9ffc`): the two `return`s
   after the error event are `continue`s; a failing watch expression keeps its `return` (it is never
   skipped and always ends the run; a test pins that the outcome is the sequential one). Tests,
   written first and seen red on the unfixed code: the scenario outline of four rows (the two models,
   with and without `--sweep`, 1, 2 and 4 workers against `--bfs`), a unit test over every knob set,
   the oracle changes above, and the generators. The unfixed code fails them with `reach5 is
   violated, want verified` and `inv5 is verified, want violated`, and 45 of the 60 000 models of
   the new generator. The 200 000 models of the earlier campaigns did not meet it, and the reasons
   are in "Behaviour pinned": at most one property that can fail to evaluate per model, and an
   oracle that ran with the sweep only, with one worker. **An independent seed range found it**: the
   orchestrator ran the author's oracle from seed 31 000 000 (the author had used 1 000 000 to
   4 000 000) and it failed at seed 31 034 067.

   Siblings searched for (the same shape: an error or an early exit that abandons checks the
   sequential search would still make on the same state or group): every `return` of `walk`
   (an error of `nextEnabled`, of `fire`, of the atomic test, the atomic bound, the cancellation
   and the record cap), `checkNew`'s watch `return`, `store` at a full partition, the inline path's
   `skip` after the state budget, `dropEvents`, the discarded and the overflowing group, and the
   group-start snapshot of `decided`, `openAssert` and `openDeadlock`. Each either ends the run
   when it applies (an error, a bound or a budget event ends the search, so nothing after it in the
   key order can matter, and the retained event of a class is the smallest key, which is the one that
   applies) or throws the group away and runs it again. The two `return`s fixed above are the only
   place where an event that can be dropped took other checks with it. The watch `return` is
   equivalent to `continue` (a test pins the outcome, so the mutant of removing it survives).
2. **Medium, docs and semantics: depth and the depth budget with atomic sequences.** A parallel
   layer counts hops between stored states, so an atomic sequence is one unit of `depth`
   and of `--budget-depth`; `--bfs` and the default (depth-first) search count every step of it. (The first wording of
   this round said "an atomic or `d_step` sequence", which is false for `d_step`: round 2,
   finding 1.) The text said the depth is
   the breadth-first depth in eight places (the package and the option comments, the `--workers`
   usage text, the README paragraph, `engine-tools.md`, `workflow.md`, the feature header, this
   file) and the report's own `note`. For `P(){ atomic{x=1;x=2}; assert(0) }` with `--budget-depth 1`, `--bfs`
   leaves both properties `inconclusive` and `--workers 2` finds the assert `violated` (a real run, so
   not unsound; with `--budget-depth 2` both decide). **Fixed** (the text only, the alternative of a
   move-depth per stored state was not built): every place says that a layer counts stored-state
   hops, and a scenario outline and a scenario pin it on `par-atomic-depth.pml` (depth 4 for `--bfs`
   against 3, `--budget-depth 1` and `2`). The review counted 14 corpus models whose `depth`
   differs from `--bfs`'s with equal verdicts, states and transitions; that count was not made
   again here. The memory budget is an estimate and a parallel run can overshoot it by about one
   group (the review measured an estimate of 949 MB for a budget of 800 MB and a resident size of
   1.1 to 1.2 GB against 864 MB sequentially, on a model with 3 KB states): said in the same places.
3. **Low, stale records. Fixed**: the feature header said the scenarios were `@pending` and promised
   an atomic-step count that the report does not have; the "no disagreement" sentence and the README's
   "200 000 random models" now say what the review found and what was run after the fix; the oracle
   description says it ran only with the sweep.

Decided not to fix (recorded so that a later reader does not find them again): the inline path
never looks at the clock (the sequential search is no better; measured); the memory estimate counts
a group's records once; `countPrefix` costs a sequential expansion of the group after an overflow;
`safely` does not wrap `finalize`; `InternalError.Msg` carries a stack. Two comments were improved
along the way: `parHash`'s claim that the three bit ranges never overlap (a fuller partition's table
reaches into the fingerprint bits, which only weakens a filter) and the missing comment of
`InternalError.Error`.

Left open, not verified: see "What was NOT verified".

### Round 2: cross-review of the correction round (head `67482da`, base `1569428`), and what was done

Three blind reviewers on different models, one brief, over the correction round (the `continue`
fix, the generators and the oracle, the depth text). Verdict: **approve with changes**. No wrong
verdict, crash or hang was found. What the orchestrator re-ran and found as recorded: a 134-file
corpus differential (with and without the sweep, 1, 2, 4 and 8 workers against `--bfs` and the
depth-first search: 0 disagreements); campaigns on seed ranges that no earlier campaign used, with
no failure (failing-property 3 000 and 150 000 models, rich 120 000, plain 60 000, determinism
15 000); the mutants of the unfixed code fail exactly as round 1 says; every number of this log and
every campaign size and outcome of the record reproduced.

1. **Medium, docs and wire-visible text, introduced by round 1: the `d_step` claim was false, in 14
   places** (the finding counted 13 and listed 14). They said that an atomic or `d_step` sequence
   is one unit of depth, unlike `--bfs`. That holds for atomic sequences only: `fire` runs a `d_step`
   chain as ONE move, so the default search, `--bfs` and `--workers` all count it as one step.
   Verified by the reviewers: `d_step{x=1;x=2}; assert(false)` gives depth 3 and 4 states in `--bfs`
   and in `--workers 2`, `--budget-depth` 0 to 3 give the same stop, statuses, depth and states, a
   model with two `d_step` blocks matches, and in the 134-file differential all 15 models whose depth
   differs contain `atomic` (the `d_step`-only models, `dstep-block.pml`, `27-semaphore-invariant-
   dstep.pml` and `por-dstep.json`, show no difference). **Fixed** (the text, and a scenario): "or
   `d_step`" is gone from `cli.go` (the comment and the `--workers` help), `explore.go` (the package
   and the `Options.Workers` comments), `parallel.go` (two comments and `parNote`, which the report
   carries as `search.parallel.note`: "an atomic sequence is one unit of depth and of
   `--budget-depth`, unlike --bfs"), `mcp/check.go` (the `workers` schema text), the README,
   `engine-tools.md`, `workflow.md`, the feature header and this file (decision 1 and round 1,
   finding 2). A grep of the whole plugin tree for `d_step` next to depth and unit, and for the
   wrapped forms, found no copy beyond these, the plan included (`perf5-plan.md` has none; it keeps
   the old `note` text where it shows the report, and says so in its status box). Three sentences
   read "the depth equals `--bfs`'s only on a model without atomic sequences" as a necessary
   condition; they now say it can differ on a model with them. A new scenario outline (three rows) on
   `par-dstep-depth.pml` pins the **behaviour**: the same stop, depth (1, 2, 3), states and statuses
   for `--bfs`, the default search and `--workers 2` under `--budget-depth` 1, 2 and 3. It passed on
   its first run, because it pins what already held; it was checked to fail when an expected figure
   is wrong. The fixture is one more model of the corpus tests, so their counts moved (see above).
2. **Low: the definitions of depth said "in transitions", false for a parallel run.** In a parallel
   run `--budget-depth N` counts layers of stored states: on `par-atomic-depth.pml` `--workers 2
   --budget-depth 1` expands past layer 1 and decides the assert, `--bfs` leaves it inconclusive, and
   without a budget `--bfs` reports depth 4, the parallel run 3. **Fixed**: "(with --workers: layers
   of stored states; an atomic sequence is one unit)" is appended to the `--budget-depth` help, the
   `budget.depth` schema of `mc_check`, the `Counters.Depth` comment and the `--budget-depth` row of
   `engine-tools.md`. This changes help and schema text, not a field name or a type; the tracked
   binaries in `engine/bin` carry the old text until a release rebuilds them.
3. **Low, test hardening: nothing evaluated a property on the final state of a counterexample or a
   witness.** The oracle replayed a trace as a run ending in its own final valuation, so a trace to
   ANOTHER state of the same layer passed. The orchestrator's mutant (the trace of every invariant
   and reach event taken to the state with id minus one) failed 1 model of 6 000, by a trace-length
   mismatch alone. **Fixed**, and re-measured here with two mutants of the unchanged code, built in a
   scratch copy:
   (A) the same id-minus-one mutant: the old oracle failed 2 models (failing-property seed 4165 and
   plain seed 157), both by a length mismatch; the hardened one fails 7 of the 6 000 failing-property
   models (seeds 1115, 1787, 3671, 4165, 4764, 4881, 5712) and the plain test, including models where
   the length matches: on seed 3671 the mutated trace of the reach condition `p5` is `a[0] = 1`, the
   breadth-first one `i = 1`, one step each, and only the new check says "ends in a state where its
   expression is false";
   (B) the trace taken to another stored state of the same layer, so of the right length: the old
   oracle failed **no** model (the mutant changed 4 478 of 32 742 traces in the two campaigns that
   count them); the hardened one fails 1 436 of 6 000 failing-property models, 90 of 2 400 rich ones,
   and the fixture, shortest-counterexample, budgeted and plain tests;
   (C) a deadlock trace taken to another state of the same layer: the old oracle failed no model, the
   hardened one fails 93 rich models and six tests (the final state "has 1 enabled moves"); and an
   assert trace that drops its last step (the failing one): the old oracle failed 3 rich models and
   five tests, by a length mismatch on models without atomic sequences, the hardened one 4 and six, and
   sees it where the lengths are not compared (the state-budget test, rich seed 258).
   The hardened oracle evaluates the invariant (false), the reach condition (true), the deadlock (no
   enabled move and not terminated) and the assert (a step that fails an assert is the last one) on
   the replayed final state, in both branches of `checkParallelCore` (the run with an ending event
   and the complete one), in the budgeted-run test and the state-budget test, and in the check
   against the reachable graph, where the final state must also be a stored state of it. The deadlock
   and assert checks are new and were not asked for in the finding; they needed no loosening on the
   campaigns below. **The unmutated code passes it**: the default sizes with the old and with a new
   seed (7 300 001), and larger runs on seed ranges that no campaign used (failing-property 60 000
   models from 7 400 001, rich 24 000 from 7 500 001, plain 36 000 from 7 600 001, budgeted 3 000
   from 7 700 001), no failure; machine load 4 to 12 while they ran.
4. **Low, records. Fixed in this file**: "never of a clock" now says "except under `--budget-ms`"; the
   later commits after `c5c78ac` are described as comments, texts, one threshold, one test and the
   text of the `note`; the corpus counts were already one short of the fixtures (104, 89, 78, 11, 13
   against 105, 90, 79, 11, 14 before this round's fixture) and are now 106, 91, 80, 11, 14, which
   changes the README's "89" to 91; the open item about the memory sentence is listed.
5. **One sentence added to the user docs** (README and `engine-tools.md`): a property expression that
   fails to evaluate can end one search `invalid-model` where another completes, because the parallel
   frontier is in partition order and not FIFO order (the depth-first search differs from `--bfs` the
   same way); complete runs never disagree. The orchestrator measured it on 20 000 failing-property
   models: the parallel search ended `invalid-model` while `--bfs` completed in 12 runs, the reverse in
   6, and in no run were both complete with different statuses (no seed range was recorded for those
   figures, and they were not reproduced; round 3 measured it again on four named ranges).

Checked after the changes (they touch texts, one fixture, one scenario and the oracle only): the
whole suite, `go test -count=1 -p 2 ./...` from `model-check-plugin/engine`, green (including
`tools/pandiff` against SPIN, 204 s; the root package 123 s, `explore` 76 s; machine load 4 to 9);
`go vet ./...` and `gofmt -l .` clean; `go test -race -short ./explore` green (343 s). Default reports of
the unmodified base binary (`67482da`) and of this one over 65 fixtures (52 give a report, 13 are
rejected by the frontend or exit 2 the same way in both), each with seven invocations (`--no-timing`
with small budgets: default, `--sweep`, `--sweep --bfs`, `--por`, `--workers 2`, `--workers 4
--sweep`, `--workers 1 --budget-depth 3`): 455 runs, the four without `--workers` byte-identical
(stdout, stderr and exit code), the three with it identical except for the `note` text. (That set was not recorded as a list or a
glob and the figure 65 cannot be reproduced from one; round 3 names its set.)

Decided not to change, recorded so that a later reader does not find them again: `perf5-plan.md` is
a frozen phase-1 document (it still says "BFS depth" in three places and shows the old `note`; its
status box now points here for the depth semantics); the comparison of the "lenient" branch of the
oracle (a model whose searches disagree about ending) does not compare deadlock and assert verdicts,
which the hardened trace checks narrow but do not remove; the campaign size of the rich test (the
regression is gated by the two IR fixtures, the unit test and the four scenario rows); on
`par-atomic-depth.pml` with `--budget-depth 1` the default search reports depth 1 where `--bfs`
reports 0 (existing behaviour, outside this diff). A finding that was rejected: that an assert
evaluation failure produces `evCheckErr` (it goes through `fire` to `evError`).

Left open, not verified: the sentence that a run can overshoot the memory budget by about one group of
records (one measurement); the capacity of a partition (2^24 - 1 states, a 4-byte id) never run to
the limit; speedups under a quiet machine; platforms other than linux/amd64; the panic and
goroutine-leak path (reading and the existing tests only).

### Round 3: short cross-review of the depth statements, the wire strings and the oracle (head `7de760e`, base `67482da`), and what was done

Three blind reviewers on different models, one short brief over the depth statements, the wire-visible
strings and the oracle hardening of round 2. Verdict: **approve with changes**; four findings, all low
(wording and tests); it found no wrong verdict. What the orchestrator verified and recorded: no `d_step` statement is false any more (`d_step` alone, atomic in `d_step`, `d_step` in
atomic and `par-dstep-depth.pml` give the same depth, stop reason, states and statuses in the default
search, `--bfs` and `--workers` 1, 2 and 4 under budgets 0 to 4); the wire contract changed only in
seven `description` strings of the schema (field names, types and required lists untouched; 756 runs on
108 fixtures: the 432 default-search invocations were byte-identical, of the 324 with `--workers` 123
were identical and 201 differed only in `search.parallel.note`); no complete run answered differently
(85 models on which `--bfs` and `--workers 2` both completed have the same verdicts, states and
transitions, and the 12 with a different depth all contain `atomic`, the parallel depth being the
lower); the three oracle mutants of round 2 are killed by the hardened oracle; unmutated oracle
campaigns on three new seed ranges found no failure; the counts of this record reproduced.

1. **Low (two reviewers rated it medium): "an atomic sequence is one unit of depth" is inexact when
   the block blocks part-way.** The rule is "hops between stored states". When the holder of an
   atomic sequence blocks, the state is stored, so the sequence is cut there, and the continuation
   starts with the step of whichever process unblocks it. `testdata/promela/atomic-at.pml`
   (`atomic { x=1; y==1; x=2 }; x=3` and `B: y=1`): `--bfs` reports depth 7 and `--workers` depth 5
   (11 states and 20 transitions in both): the block takes two units (`x=1`; then B's `y=1` with `y==1`
   and `x=2`) where one that runs through takes one. **Fixed** (text and tests): every copy now says
   "an atomic sequence that runs through is one unit; one that blocks part-way counts one unit per
   uninterrupted run", with the exact rule ("layers of stored states") beside it. Places, wire-visible
   first: `parNote` (`explore/parallel.go`, the report's `search.parallel.note`), the `budget.depth`
   and `workers` schema descriptions of `mc_check` (`mcp/budget.go`, `mcp/check.go`), the
   `--budget-depth` and `--workers` usage text (`cli/cli.go`); then comments and docs: `cli/cli.go`
   (the flag comment), `explore/explore.go` (package and `Options.Workers`), `explore/parallel.go`
   (the budget paragraph and `layerExpanded`), `report/report.go` (`Counters.Depth`), `README.md`,
   `skills/model-check/references/engine-tools.md` (the `--budget-depth` and `--workers` rows),
   `references/workflow.md`, the header and one outline of `features/g8-parallel.feature`, and the
   status box of `perf5-plan.md` (the body of the plan is frozen and still says "BFS depth").
   Copies beyond those the finding listed: the `search` row of the report table in
   `engine-tools.md` and the comment of `ReportedMode` (`cli/cli.go`). Grep over the whole tracked tree,
   with the wrapped lines of comments and Markdown joined, finds no other copy; the earlier rounds of
   this record keep their wording, as history.
   *Re-verified by running* (the binary built from the branch into a scratch directory; the default
   search, `--bfs` and `--workers` 1, 2 and 4, budgets 0 to 9, comparing depth, stop reason, states,
   transitions and statuses): `--workers 1`, `2` and `4` give the same values in every field for every
   budget on `atomic-at.pml`, `par-atomic-depth.pml`, `par-dstep-depth.pml`, the new
   `atomic-guard-first.pml`, `par-chain.pml -D N=6` and `bench-indep.pml -D N=2 -D K=2` (`par-chain.pml`
   and `bench-indep.pml` have neither atomic nor `d_step`: the default search, `--bfs` and `--workers`
   never differ in depth, stop reason, states, transitions or statuses, for budgets 0 to 12 and 0 to
   10; `par-dstep-depth.pml` likewise for 0 to 9). Where `--bfs` and `--workers` differ it is in depth,
   stop reason, states, transitions and statuses, as the text says. Unbudgeted depth, default search /
   `--bfs` / `--workers`: `atomic-at` 7/7/5, `par-atomic-depth` 4/4/3, `par-dstep-depth` 3/3/3,
   `atomic-guard-first` 7/7/5, `par-chain` 14/14/14, `bench-indep` 16/16/16. The rule gave the
   parallel depth of three more models, worked out by hand before they were run (and the `--bfs` depth
   as the sum of the steps): two consecutive blocks 4 (`--bfs` 6), a nested block 3 (5), a block that
   blocks twice, each time with another process to unblock it, 6 (10); the reviewer's block that blocks
   after two statements agrees (5 and 9). The guard-first case
   (`atomic { y==1; x=1; x=2 }`) does not split but gives the same two depths as `atomic-at.pml`, 7 and
   5, for a different reason (the block is one unit, B's step before it another; in `atomic-at.pml`
   the block is two units): the rows of the two outlines differ (6 states and 10 transitions against 9
   and 15 at budget 2), and that is what pins the difference.
2. **Low: "with --workers: layers of stored states" is wrong when the parallel search is refused.**
   `--workers 2` with an `ltl`, `progress` or `ctl` property, or with `--por` where the reduction
   applies, runs the sequential search, and `--budget-depth` counts transitions. On
   `par-atomic-depth.pml --budget-depth 1`: `--workers 2` gives mode `bfs`, `applied: true`, the assert
   `violated`; with `--ltl '[] true'`, `--progress` or `--ctl 'AG(true)'` it gives mode `dfs`,
   `applied: false`, the assert `inconclusive`, depth 1 in transitions. **Fixed**: the strings now say
   "when the parallel search is applied" (the `--budget-depth` and `--workers` help, the `budget.depth`
   and `workers` schema, `Counters.Depth`) and that a refused run is the sequential one with its own
   mode (the `workers` schema, the `--workers` comment of `cli.go` and `Options.Workers`, the refusal
   sentence of the README and of `engine-tools.md`, `workflow.md`, the `ReportedMode` comment). The
   `note` is present in applied runs only (checked: a refused report carries `reason` and no `note`).
   An outline of three rows (the three temporal kinds) and a scenario (`--por`, `bench-indep.pml -D N=3
   -D K=3 --por --budget-depth 4`: depth 4, 6 states, 5 transitions) compare the refused run with the
   sequential report (equal except for `search.parallel`) and with the applied one.
3. **Low (test): `replayTrace` accepted an empty assert counterexample.** `walk(0, ...)` returns at
   `i == n` before the `failedLast` check and `replayOutcome`'s switch has no assert case. **Fixed**
   in `replayTrace` (`failedLast && !mayFail && n == 0` is an error), with a negative test, written
   first: `TestParallelOracleRejectsATraceWithoutTheStepsOfItsVerdict` takes the outcome of a real
   parallel run for an assert, an invariant, a reach witness and a deadlock (the real trace must be
   accepted), replaces its trace with one of no step, once with the initial valuation and once with
   none, and requires both to be refused. **Before the fix** it failed on the assert row only
   ("the oracle accepts a trace of no step with the initial valuation as the violated of assert", and
   likewise with no valuation); **after** it passes. The same hole for the other kinds does not
   exist: for an invariant, a reach witness and a deadlock the final-state check evaluates the
   expression (or the enabled moves) on the initial state, which is not the answer in those models, and
   refuses; an empty trace is legitimate where the initial state is the answer
   (`TestParallelChecksTheInitialState` pins that). The case of an assert trace with its failing step
   cut off, caught before, is a row of the same test. The short oracle (`-short -run TestParallel`) is
   green with the check.
4. **Low (test): the `d_step` outline pinned less than its comment said.** **Fixed**: the outline of
   `par-dstep-depth.pml` now also checks that every property has the same status and evidence in the
   parallel and the default run (existing step) and that `--bfs` and the parallel run have the same
   states and transitions (a new step binding, which shares a helper with the existing one), so all
   three searches are pinned on stop reason, depth, states, transitions and statuses. The scenario
   passed on its first run (it pins what already held); each new check was shown to fail where the
   searches do differ (the status check on `par-atomic-depth.pml` at `--budget-depth 1`: "property
   assert: parallel violated/exhaustive, sequential inconclusive/bounded"; the counters check on
   `atomic-at.pml` at `--budget-depth 2`: "states: 8 in the breadth-first run, 9 in the parallel run").

The new scenarios (items 1, 2 and 4) passed on their first run, since they pin behaviour that already
held. Each expected value was perturbed, one at a time (every number, stop reason and status of the
two outlines, the unbudgeted rows, the fixed lines of the refused scenarios): 96 variants, all fail.
The fixture `atomic-guard-first.pml` is one more model of the corpus tests (counts above).

Decided in this round, from the "may" list of the review: the flag `--dfs` of round 1 is now "the
default (depth-first) search"; the order-dependence figures of round 2 were measured again, with a
named range, in both directions: 20 000 failing-property models from each seed 70 000 000,
80 000 000, 81 000 000 and 82 000 000, `--bfs` against `--workers 2`, no sweep, 30 000 states and a
depth of 20 000 as budgets: the parallel search ended `invalid-model` while `--bfs` did not in 6, 2,
3 and 2 models, the reverse in 4, 2, 4 and 5, both ended so in about 4 300 per range, and in no range
was there a model where neither ended so and a status differed (the 12 and 6 of round 2 came from a
range that was not recorded and were not reproduced; the claim, that the order decides it in both
directions and that complete runs never disagree, holds); the "65 fixtures" of round 2 are dropped
(the set cannot be reproduced) and replaced by a named set below. Not changed, as the review said:
the corpus test, the g8 "replays" step and the graph check do not apply the final-state check of an
assert trace (the graph check skips assert traces rightly: an assert can end inside an atomic
sequence), and the random generators reach atomic sequences rarely (no action).

Checked after the changes (they touch texts, one fixture, scenarios, a step file and a helper of the
oracle's tests; the search code of the engine is untouched):

- the whole suite, `go test -count=1 -p 2 -timeout 45m ./...` from `model-check-plugin/engine`: green,
  4 min 54 s (the root package 123 s, `explore` 80 s, `tools/pandiff` against SPIN 207 s); machine load
  6.6 at the start and 8.5 at the end;
- `go vet ./...` and `gofmt -l .` clean;
- `go test -race -short -count=1 ./explore`: green, 315 s, load 8.1 to 9.7;
- an oracle campaign with the new check, on a seed range that no campaign used
  (`MCD_PAR_MODELS=8000 MCD_PAR_SEED=90000001`: 8 000 rich models, 8 000 failing-property models, 8 000
  plain ones, the fixtures and the directed models): no failure; load 9.6. The larger campaigns of
  round 2 were not repeated;
- default reports of the binary built from `7de760e` and of the one built from this round's code, over
  the 55 models of `testdata/promela/*.pml`, `testdata/ir/*.json` and `testdata/petri/*.json` (the new
  fixture included), each with seven invocations (`--no-timing`, 3 000 states, depth 30, 10 s, 256 MiB:
  default, `--sweep`, `--sweep --bfs`, `--por`, `--workers 2`, `--workers 4 --sweep`, `--workers 1
  --budget-depth 3`): 384 invocations, all with a JSON report; the 219 without `--workers` are
  byte-identical (stdout, stderr and exit code), the 165 with it identical except for the text of
  `search.parallel.note` (one text in each binary). `--sweep --bfs` on `testdata/ir/par-atomic-loop.json`
  was left out of the second run: the sequential breadth-first search keeps a chain of moves per
  intermediate state of an atomic sequence, so its memory is quadratic in the length of the sequence
  and a depth budget does not bound it (the first attempt, with a 40 s timeout, was killed by it; no
  other process was affected).

Left open, not verified: the tracked binaries in `engine/bin` were not rebuilt (a release step), so the
help and the schema that ship still have round 2's wording until a release rebuilds them; the
capacity of a partition (2^24 - 1 states), the one-measurement memory-overshoot figure, speedups under
a quiet machine and platforms other than linux/amd64 (as before); the larger campaigns, the
mutation campaign and the 756-run wire comparison of the review were not repeated at their size; the
claim that an atomic sequence "counts one unit per uninterrupted run" was checked on the models named
above and on the review's list, not proved for every model.

## After the integration (0.3.0)

Merged with the other four branches (`integration-0.3.0-notes.md`). The sequential breadth-first search no longer
copies the chain of moves at every step of an atomic sequence (a divergent atomic block is `inconclusive` at the bound,
in memory linear in it): the "about 100 GB" of "Found along the way" is gone for the breadth-first search and the CTL
graph; the depth-first search in a library call without a depth budget still has no bound of its own. `--por`
with `--workers` is the reduced run on atomic sequences too (step 6 reduces them). A parallel run whose properties
are all refused for the process table reports that the parallel search was not applied. A parallel worker's panic is
answered with one line, the stack on the server's standard error. The corpus test of the merged tree compares 106
models (137 accepted, 25 refused for a temporal property). New campaigns on the merged tree: 50 000 plain, 20 000 rich,
10 000 failing-property and 30 000 table-shape models, no disagreement.
