# Performance plan, step 5 — parallel exploration of the safety search (PLAN)

> **Status (phase 2).** Implemented on branch `perf/step5-parallel`; what was built, what
> was measured, the deviations from this plan (the memory record guard cuts a group after the
> longest prefix whose records fit instead of halving it, so the inline path can follow it;
> the surfaces' option arrived with the events; the shared counter of the guard is batched)
> and what was not verified are in `perf5-confirmation.md`. This document is left as it was
> reviewed in phase 1. What the depth is for a parallel run (a layer counts hops between stored
> states: an atomic sequence that runs through is one unit and one that blocks part-way one unit per
> uninterrupted run, a `d_step` block is one move in every search; none of it for a refused run, which
> is sequential) and the report's `note` are in the confirmation, review log; this document still says
> "BFS depth" and shows the first `note` text where it describes them.

Layer: G0 (`explore`), with thin additions in `cli`, `report` and `mcp` (G2). Follows
`perf1..perf4-confirmation.md`. Protocol: `BUILD-PROTOCOL.md` step 6. This document is the
plan of phase 1 of the stream (measure, decide, cross-review the plan). Nothing in it is
implemented in the repository yet; every number below was taken with throwaway prototypes
kept outside the repository, and the conditions of each are stated next to it.

## 0. Summary of the decisions

| # | Decision | Why (details in the section) |
|---|---|---|
| D1 | **Go**, opt-in: `mcd check --workers N`, `mc_check` field `workers`. Default (no flag) is byte-identical. | A deterministic prototype reaches x4–9 at 8 workers over the sequential DFS on the four wide models measured (§1.4); the stream is worth doing. It is **not** worth doing for narrow graphs or for hunting bugs (§1.5, §8). |
| D2 | **Level-synchronous breadth-first search, deterministic partitioned set** (fixed number of hash partitions; per level: workers expand frontier segments and bucket the successors by partition; then each partition is filled by exactly one worker in a fixed record order). No locks, no CAS, no shared mutable table: the only synchronisation is two barriers per group of segments. | At least as fast as the shared-lock variant (clearly ahead at 4 workers and on large models after tuning, §2.1), and determinism is by construction instead of being bolted on. |
| D3 | Not chosen: a lock-free open-addressing table with CAS on the packed slot (argued, **not prototyped**), sharded locks (prototyped, slower), work-stealing DFS (not prototyped; evidence is non-deterministic and non-shortest). | §2.1. |
| D4 | One compiled copy of the model per worker (only `ir.Layout.Timeout` is mutable; `ir` is not touched). A compile costs 0.01–0.7 ms and 7–282 KB on the corpus. | §2.4. |
| D5 | Complete runs: `states`, `transitions`, `atomic_steps` are **exactly** the sequential ones; `depth` is the BFS depth (= `--bfs` without atomic sequences; never above the DFS depth, which is an artefact of the DFS order). The report says so. | §3. |
| D6 | Counterexamples are **shortest** (in layers; in moves when there is no atomic sequence) and **deterministic** (a function of model and options, not of the worker count or of timing): the decisive event is the first one in the frontier order of the first level that has one; the path comes from one parent id per state (4 bytes) and is re-derived move by move. | §2.7. |
| D7 | Refused with a reason in `search.parallel`: any `ltl`/`progress`/`ctl` property in the run; `--por` when the reduction applies (it is depth-first); `--estimate` (usage error). Everything else of the safety search (atomic, d_step, rendezvous, `run`, `timeout`, `provided`, `ClearChans`, never-claim processes that the safety search stores but does not execute, `--sweep`, `--watch`, `--bfs`) is supported. | §4. |
| D8 | State, depth and memory budgets stop at deterministic points (the state budget exactly), so a budget-truncated report is also independent of the worker count; only the time budget is not reproducible. | §3.5. |
| D9 | Acceptance gate with a stop rule: if the real implementation does not reach x3 at 8 workers on `bench-indep` N=6 and `counters-10-6` at load <= 2, the stream is abandoned and reported, not tuned until it looks good. | §1.6, §7. |

## 1. Baseline and target

### 1.1 Conditions of every measurement

* Machine: 16 vCPU (`nproc` 16; `lscpu` shows 4 sockets x 4 cores, 1 thread per core, which is how
  the virtual machine presents itself; the physical topology is unknown), Xeon Gold 6154,
  30 GB, Go 1.26.1. The machine is shared with the other stream and with reviewers, so
  `uptime` is noted for every table. A figure taken at load above 2 is **not a benchmark**; it is
  a hint with its spread. The acceptance numbers of §1.6 must be re-taken at load <= 2.
* Sequential figures: `mcd check --promela ... --sweep --unlimited --no-timing` (the tracked
  engine of this branch, built to the scratch directory), or `explore.Run` with `Sweep` in a
  test binary (identical code path). Medians of 3 unless stated; "DFS" is the default search,
  "BFS" is `--bfs`.
* Prototype figures: throwaway Go test files added to a **copy** of the engine in the scratch
  directory (never the repository). They count states and transitions of a complete sweep and
  check them against the sequential run; they have no properties, no counterexamples, no atomic
  sequences, no budgets. They therefore measure the structure of the search, not the finished
  feature, and every figure below that comes from them is labelled "prototype".

### 1.2 What is measured now (sequential engine)

Sequential rate: about 1.0 M stored states/s and 4.7 M transitions/s on `bench-indep`
(about 210 ns per transition). The cost is a cache-miss-bound visited set plus the interpreter:

pprof of `Run` on `bench-indep` N=5 (3 repetitions, 1.68 s, load 8.7):

| Where | flat | cumulative |
|---|---:|---:|
| `compact.find` (table probe and vector compare, cache misses) | 30% | 34% |
| `compact.grow` (rehash at each doubling) | 14% | 16% |
| `ir.Compiled.Eval` (guards, effects) | 11% | 11% |
| `search.nextEnabled` + `enabled` | 17% | 22% |
| `search.apply` + `fire` | 5% | 12% |
| `maphash` (aeshash) | 4% | 4% |
| `compact.Add` in total | | 54–58% |

So 55–60% of the time is the visited set, 35–40% is stepping the model. Neither is a serial
bottleneck in the Amdahl sense: both parallelise, provided the table is not a contention point
and the work per level is large enough to amortise the barriers.

Two sequential facts that shape the plan:

* The sequential **BFS** (`--bfs`) is not a fair baseline for a parallel BFS: it keeps a parent
  link, a chain of moves (a slice header plus its backing array) and a depth per state. On
  `bench-indep` N=6 it needs 1.78 GB resident and 18 s, against 0.36 GB and 10.5–12 s for the
  default DFS. The parallel search is compared with the default DFS (what users run today),
  and also with the sequential BFS where noted.
* The default DFS depth is an artefact of the search order: `counters-10-6` (55 BFS layers) has
  1 000 000 states, each with 6 successors, so the DFS walks one path through every state and
  reports `depth` 999 999 (the figures here are taken without a depth budget; the default budget,
  1 000 000, would cut a model only slightly larger). The parallel search reports the BFS depth
  (§3.3).

### 1.3 Baseline table (sequential, `--sweep --unlimited`)

Load 2.7–8 while taken; spread is min–max of 3 runs.

| Model | States | Transitions | BFS layers | widest / mean layer | DFS time | BFS time | resident set (DFS) |
|---|---:|---:|---:|---:|---:|---:|---:|
| `bench-indep` N=5 K=4 | 579 195 | 2 689 120 | 71 | 24 367 / 8 158 | 0.55–0.68 s | 1.06–1.13 s | 48 MB |
| `counters-10-6` (IR) | 1 000 000 | 6 000 000 | 55 | 55 252 / 18 181 | 1.58–1.72 s | 2.1–2.2 s | not measured (estimate 105 MB) |
| `client_server` (CH15) | 191 200 | 620 252 | 69 | 9 903 / 2 771 | 0.30–0.35 s | 0.44–0.46 s | not measured |
| `bench-indep` N=6 K=4 | 8 108 731 | 45 177 216 | 85 | 313 873 / 95 396 | 10.5–12.1 s | 17.6–18.4 s | **362 MB** (BFS: 1 778 MB) |
| `two1000` (2 x counter to 1000, `int`) | 4 010 007 | 8 016 008 | 4 005 | 2 002 / 1 001 | 2.1–2.2 s | 4.1–4.5 s | not measured |
| `chain` (one counter to 10^6, two edges per step) | 2 000 003 | 2 000 002 | 2 000 003 | 1 / 1 | 1.06–1.19 s | 1.2–1.4 s | not measured |

`two1000` and `chain` are not corpus models: they are the adversarial shapes for a
level-synchronous search (many layers, few states per layer) and they are here to bound the
damage, §1.5.

### 1.4 What the earlier spike and the better prototypes measured

The spike (level-synchronous BFS over 1024 mutex-protected `compact` shards, one `[]byte` per
new state, static slicing of the frontier) re-run on this branch's `compact` at load 4–8:

| Model | w=1 | 2 | 4 | 8 | 16 (speedup over the sequential DFS) |
|---|---:|---:|---:|---:|---:|
| `bench-indep` N=5 | 0.91 | 1.45 | 1.80 | 2.12 | 2.76 |
| `counters-10-6` | 1.13 | 1.78 | 2.55 | 3.16 | 4.02 |
| `bench-indep` N=6 | 0.76 | 1.25 | 1.92 | 1.89 | 3.01 |

Two further prototypes (both check states and transitions against the sequential run, both
without allocation per state):

* **P1**: the same sharded locks, but the frontier is flat per-worker byte buffers handed to the
  next level in segments of 256 states taken dynamically. At 16 workers: x4.8 (`indep` N=5),
  x6.4–7.3 (`counters-10-6`), x6.1 (`indep` N=6), x4.0 (`client_server`).
* **P2** (the design of §2): fixed 256 partitions, fixed-seed hash, per group of segments a phase
  of expansion into per-segment record buffers counting-sorted by partition and a phase in which
  each partition is filled in record order; segment 256 states, group 256 segments. The first
  three rows were re-taken on a quieter machine (load 2.7–3.2; median of 3, min–max within 7%
  of the median except one cell); the others at the load stated (median of 3):

| Model | w=1 | 2 | 4 | 8 | 16 |
|---|---:|---:|---:|---:|---:|
| `bench-indep` N=5 | 1.03 | 1.70 | 3.26 | 5.07 | 4.80 |
| `counters-10-6` | 1.70 | 2.92 | 5.32 | 8.99 | 7.98 |
| `client_server` (0.3 s run) | 0.87 | 1.56 | 2.77 | 4.06 | 3.64 |
| `bench-indep` N=6 (load 3.5–4) | 1.18 | | 3.93 | 7.05 | 9.22 |
| `two1000` (load 9–16: mine plus others) | 0.66 | 1.00 | | 1.48 | 1.24 |
| `chain` (load 9–16) | **0.15** | 0.15 | | 0.15 | 0.15 |

Reading of P2:

* On wide models (layers of 10^4–10^6 states) it reaches x5–9 at 8 workers and x4.8–9.2 at 16
  over the DFS, and x8–15 over the sequential BFS, and is not slower than the DFS at 1 worker
  (x1.0–1.7; `client_server`, a 0.3 s run, x0.87), because
  filling a small partition table in a batch has far fewer cache misses than random probes of
  one 128 MB table. At 8 -> 16 workers it flattens: this VM does not give 16 independent cores
  under load. The x5.3 of `counters-10-6` at 4 workers is above 4 because the baseline DFS is slow
  on that model (a 1 M-frame stack) and P2 is already x1.7 at one worker.
* On `two1000` (cheap steps, 2 transitions per state, 1 000 states per layer) it is x0.66 at one
  worker and x1.5 at eight: the record traffic (writing, counting-sorting and re-reading a
  record per transition) is about 35% of the single-worker time (pprof: `ppart.add` 34% of the
  time, the two record loops 36% flat, model stepping 13%), and a layer of 1 000 states gives
  4 segments of work.
* On `chain` the prototype is **x0.15 at every worker count**: 3.2 µs of fixed work per layer
  (loops over all 256 partitions, per-segment histograms) times 2 million layers. This is a
  defect of the prototype that the real design must remove (§2.9) and a hard acceptance
  criterion (§1.6). **The fix was prototyped (P3, load 4.1–4.2):** the frontier is a list of the
  ranges the previous level touched (no loop over partitions), and a group of fewer than T = 128
  states is expanded and inserted directly, in record order, on the calling goroutine with no
  record buffers. Results over the sequential DFS: `chain` **x1.8 faster** (0.70 s against 1.27 s)
  at 1 and at 8 workers; `two1000` at 1 worker x0.66 with the batched path but **x1.00 with the
  inline path** (and x1.35 against x0.94 at 8 workers: the batched path pays where several
  workers can use it); wide models at 1 worker, inline against batched: `bench-indep` N=5 x1.05 /
  x1.05, `counters-10-6` x1.11 / x1.84 (batching by partition is a cache win there),
  `client_server` x0.85 / x0.89. The checksum of §1.4's determinism bullet is the same for
  T = 0 (always batched), 128 and infinity (always inline) on `counters-10-5` and `client_server`,
  and equals P2's: **the inline path is bit-identical to the batched path**.
* Memory (`bench-indep` N=6, resident set): sequential DFS 362 MB, P2 at 1 worker 420 MB
  (+16%), at 16 workers 436 MB (+20%); the prototype's own estimate 323 MB against the
  sequential estimate 289 MB (+12%). The sequential BFS needs 1 778 MB.
* Partition count (P2, `bench-indep` N=6 and `counters-10-6`, 8 and 16 workers, load 7–10,
  2 repetitions): 256 partitions was the best or equal; 64 was 10–25% slower at 16 workers
  (too coarse to balance), 1 024 was 15–60% slower (the per-segment histogram has as many
  entries as the segment has records). 256 stays the starting value; it is a constant of the
  implementation and part of what fixes the (deterministic) choice of counterexample.
* `go test -race` of the P2 prototype on `counters-10-5` (1, 4, 16 workers): no report.
* Partition balance of the prototype's fixed hash (256 partitions, states of the complete runs):
  `counters-10-6` 3 727–4 063 states per partition (mean 3 906), `bench-indep` N=5 2 108–2 404
  (mean 2 262), `client_server` 652–843 (mean 747), `two1000` 15 157–16 000 (mean 15 664); chi-square
  256–291 against an expected 255 (sigma about 23): uniform within what a good hash gives, no
  partition more than 13% from the mean even at 650 states per partition.
* **The determinism claim of §2.7 holds in the prototype.** A checksum over every partition's
  arena (vectors in id order) and parent array is identical for 1, 2, 3, 5, 8 and 16 workers with
  segments of 1 to 100 000 states and groups of 1 to 10^6 segments, on `counters-10-5` (100 000
  states) and `client_server` (191 200 states, rendezvous and `run`): the ids, the frontier order
  and the parents do not depend on the worker count, the segment size or the group size. That is
  evidence for the construction, not a proof, and it covers states and parents but not events.

### 1.5 Which models matter, and which will not benefit

A rule of thumb from the five prototype points of §1.4 (mean layer width -> speedup at 8 workers
over the DFS): 1 000 -> x1.5; 2 800 -> x4.1; 8 000 -> x5.1; 18 000 -> x9.0; 95 000 -> x7.1 (memory
bound). It is a description of five measurements, not a law, and the plan does not rely on it;
the report's `layers` and `max_layer_states` let the user compute the mean width of their own run.

The corpus is small: of 99 Promela files the frontend accepts (fixtures, `corpus2`, the SPIN
textbook), 4 reach 100 000 states, 2 are bounded by the 5 M-state test budget:

| Model | States | Layers | Widest layer | Note |
|---|---:|---:|---:|---|
| `CH5/sink_source_filter` | > 5 000 000 (budget) | 21 | 796 068 | the best case: wide, 11.5 M transitions in the first 5 M states; ends on the state budget today |
| `bench-indep` | 579 195 | 71 | 24 367 | synthetic, independent processes |
| `CH15/client_server` | 191 200 | 69 | 9 903 | rendezvous, `run` |
| `bench-sym` (default N; N=6 is 543 076 states, perf1) | 76 516 | 81 | 2 895 | atomic sequences |
| `CH9/leader` | 41 692 | 124 | 1 541 | atomic, dynamic channels |
| `CH5/counter` | unbounded (5 M budget) | 5 000 000 | **1** | one counter: the worst case for any layered search |

Families, by what decides the speedup (the **average layer width**, states/layers, against the
number of workers):

1. **Wide and shallow** (independent or loosely coupled processes, symmetric workers, buffered
   channel pipelines with data in the messages): width >= 10^4. The target of this stream.
2. **Moderate** (width 10^2–10^3, thousands of layers: two long counters, token rings with many
   states per phase): at most x1.5 at 8 workers in the prototype; the criterion is "not slower".
3. **Narrow and deep** (one long chain, a single process with a huge counter): no parallelism to
   find; the criterion is a bounded slowdown from the inline path of §2.9.
4. **Models with a violation**: a depth-first search often finds a counterexample after a tiny
   fraction of the graph; a breadth-first search must complete every layer before the one that
   holds the violation. The parallel search is therefore **a tool for proofs (complete runs)**.
   A user hunting a bug uses the default DFS or `--por`; the report says what the parallel run
   does (§3.4) and the documentation says it plainly. No speedup is claimed for violated runs.
5. **Models with temporal properties**: untouched (refused, §4).

The default budgets (1 M states, 60 s, 1 GiB) cap a default run at the size where one core is
enough; the stream matters when the user raises them.

### 1.6 Acceptance criteria (measurable; each is a test or a recorded measurement)

Speedup is `median(sequential default DFS) / median(parallel)` of 5 runs on one binary, same
model and flags (`--sweep --unlimited --no-timing`), taken at **load <= 2** with `uptime` printed
before and after; a run at a higher load is reported as such and does not count.

| Criterion | Models | Bound |
|---|---|---|
| A1 speedup at 2 workers | `bench-indep` N=5, `counters-10-6`, `bench-indep` N=6 | >= x1.4 |
| A2 speedup at 4 workers | same | >= x2.5 |
| A3 speedup at 8 workers | same | >= x4.0, and **the stop rule**: below x3.0 on `bench-indep` N=6 or `counters-10-6` the stream is abandoned. The figure is read next to the one-worker-normalised one (the same algorithm at 1 worker as the base): the prototype gives x5.3 (`counters-10-6`, which has the slow DFS baseline), x4.9 (`bench-indep` N=5) and x6.0 (`bench-indep` N=6) at 8 workers, i.e. 66–75% parallel efficiency |
| A4 speedup at 16 workers | same | >= x4.0, and not below 0.85 of the 8-worker figure (the VM flattens beyond 8: the prototype gives 4.8, 8.0 and 9.2 at 16 against 5.1, 9.0 and 7.1 at 8) |
| A5 mid-size | `client_server` | >= x2.5 at 8 workers |
| A6 one worker | wide models above | >= x0.8 of the sequential DFS |
| A7 moderate shape | `two1000` | >= x0.6 at 1 worker, >= x1.0 at 8 workers |
| A8 narrow shape | `chain` | <= 1.5x the sequential DFS time at **every** worker count |
| A9 memory | `bench-indep` N=6 | resident set <= 1.3x the sequential DFS at 1 and 16 workers; `memory_bytes_est` <= 1.25x the sequential estimate |
| A10 counters | every model of the corpus and every random model, complete runs | `states`, `transitions`, `atomic_steps` equal to the sequential DFS **exactly**; `depth` equal to the sequential `--bfs` depth, and `Levels` (an internal check: it is a field of `explore.Result`, not of the report) equal to the sequential BFS's, for models without atomic sequences (with them: depth never above it) |
| A11 determinism | same | the report is byte-identical (except the worker-count fields of `search.parallel`: `requested_workers`, `workers`, `worker_bytes_est`) for 1, 2, 3, 8, 16 workers, and for 5 repetitions of the same count; budget-truncated reports (states, depth, memory) too |
| A12 verdicts | every model whose reachable graph has **no ending event** (§3.7: a model error, pool exhaustion, an over-long atomic sequence), and no budget stop | status and evidence of every property equal to the sequential DFS and `--bfs`; `reason` equal except for a violated `assert`, whose reason names the failing assert and so depends on which of several distinct failing asserts the order meets first (its counterexample is compared by replay and by length, not by equality) |
| A12b verdicts, ending event reachable | the other models | no property `verified`; every `violated` has a replayable exact counterexample; under `--sweep` the parallel run stops on an ending event iff the sequential DFS and BFS do; which properties were decided before it may differ from both orders (§3.7), and the report is the same for every worker count |
| A12c verdicts under a budget | same | as §3.4: either search may decide what the other leaves inconclusive; never `verified` on an incomplete run |
| A13 counterexamples | every violated or invalid property | the trace replays as an exact run of the model (`Stepper`); for models without atomic sequences its length equals the sequential `--bfs` counterexample length (with them it is shortest in layers, see §2.7); the same trace for every worker count |
| A14 default | all existing tests and goldens | unchanged; without `--workers` no `parallel` key and no changed byte |

A6, A7 and A8 are the "nothing slower than a stated bound" criteria. The batched prototype (P2)
passes A6 (x0.87–1.7 at one worker), passes A7 at one worker only narrowly (x0.66 against 0.6) and
fails A8 by a factor of 7 (x0.15). The P3 prototype (touched ranges and the inline path for narrow
groups, §1.4 and §2.9) passes all three on the same models: `chain` x1.8, `two1000` x1.00 at one
worker, wide models x0.85–1.84 at one worker. Those are prototype figures; the criteria are
re-measured on the finished feature (step 8).

## 2. Design

### 2.1 The options considered

| Option | Verdict | Evidence |
|---|---|---|
| (a) Work-stealing parallel DFS over one shared visited set (Holzmann–Bošnački / DiVinE style) | **rejected** | Counters of a complete run would match, but the depth is meaningless, the counterexample is whatever a thread found (long, different every run), the depth budget cannot be deterministic, and termination detection needs a protocol. Verdict soundness is fine; the evidence standard of this project (reproducible reports, shortest counterexamples) is not. Not prototyped. |
| (b) Level-synchronous BFS over a shared lock-free open-addressing table (CAS on the packed 64-bit slot, chunked arena) | **not chosen; not prototyped** | The arena never moves a vector (a real gift), but: a state's index must be known before the slot is published, so either holes (indices taken from per-worker blocks) or a two-step claim; the table cannot grow while workers insert (growth only at barriers needs a worst-case bound per level, or a stop-the-world rehash protocol); `Bytes()` is no longer a function of the number of states; and the first writer of a duplicate decides the parent, so a deterministic shortest counterexample needs a CAS-min on every same-level duplicate (in `bench-indep` every duplicate is a same-level duplicate: 2.1 M of 2.7 M transitions). Each of those is solvable; together they are the most delicate code of the stream, hunted with the race detector only. I did not measure it, so I do not claim it would be slower. |
| (c) Level-synchronous BFS over lock-sharded sets (the spike, P1) | **not chosen** | Prototyped: x3–4 at 16 workers with allocation per state (spike), x4–7 with flat buffers (P1). Paired in the same run, both untuned (P1 has no groups): P2 is ahead of P1 at 4 workers in 4 of 4 models (x2.9–4.6 against x2.2–3.9), and the two trade places at 8–16 workers (P2 ahead in 4 of 8 cells: `indep` N=6 at both counts, `counters-10-6` at 8, `indep` N=5 at 8; P1 ahead on the small `client_server` and at 16 on `indep` N=5 and `counters-10-6`). After tuning segment and group size, P2 gained 40–60% on the two large wide models (x8–9 at 16 workers; same-run P1 x6.1). So **speed does not decide between P1 and P2; determinism does**: in P1 the first inserter of a duplicate decides the parent, so a deterministic shortest counterexample needs extra machinery per duplicate. |
| (d) Level-synchronous BFS over a deterministic partitioned set (P2) | **chosen** | x5–9 at 8 workers on wide models (prototype); deterministic by construction; no shared mutable structure inside a phase, so the race detector checks the argument instead of being the argument. Costs: records are written and read once more than a shared table would need; a barrier pair per group; a fixed-seed hash of our own. |

Why a partitioned set is faster than the shared table, in one paragraph: the two profile hot spots
of the sequential engine are `find` and `grow`, both cache-miss-bound on a table of up to 128 MB.
In the partitioned design a worker fills one partition (8k–32k states, 0.3–1.3 MB) with a batch
of records, so the probes hit L2 and each doubling rehashes a small table inside the worker that
owns it, in parallel with the others; the price is a copy of every successor, written and read
sequentially.

### 2.2 The algorithm

**Constants (part of the engine, tested, never derived from the worker count):** `P` = 256
partitions; the hash is a fixed-seed 64-bit function of the vector (§2.3); partition =
bits 56–63 of the hash.

**State id:** `uint32` = `partition << 24 | index within the partition's arena`. A partition holds
at most 2^24 - 1 states, so the id `0xFFFFFFFF` is never a state's and is the parent of the
initial state; reaching the capacity of a partition stops the run as a declared bound
("partition capacity", inconclusive, evidence bounded), never a wrong verdict; it takes about
70 GB of 16-byte vectors to get there, and a skewed hash is excluded by the balance test.

**Levels, segments, groups.** The frontier of a level is the list of non-empty ranges
`(partition, lo, hi)` of states appended to the partitions during the previous level, in
ascending partition order: **the frontier order is ascending state id**, and it needs no memory
of its own (the states are in the arenas). A *segment* is a run of at most `S` consecutive
frontier states (a scheduling unit; `S` may depend on the worker count and the level width).
**Invariant: `S` enters no quantity that reaches a result or a stop point.** A *group* is the next
`G` frontier states taken in frontier order, counted in **states** (`G` is chosen from counts only,
§2.8, the first group included: 1 024 states; never from the worker count, the segment size or
timing); the group is the unit of the memory check, of the budget stops and of the discard of a
cancelled run, so everything tied to a group boundary is the same for every worker count. Groups
bound the memory of the records.

**Step 0, single-threaded, before any worker runs:** build the initial state, insert it into its
partition, evaluate its checks in the exact order of `checkState` (watch, invariants, reach), apply
the resulting events (they have the smallest keys) through the same merge, and only then begin the
first group, unless an event already stopped the run. This is `bfs()`'s `checkState(init, ...)`
(`explore.go:1632-1645`); without it an initial state that violates an invariant would be missed,
because phase 3 only checks states a record has inserted.

For each group, in order:

1. **Expand** (parallel over the segments of the group, dynamic scheduling by an atomic cursor).
   A worker copies a frontier state out of its arena (read-only: nothing writes any arena during
   this phase), expands it exactly as the sequential breadth-first search does — `nextEnabled`,
   `fire`, the intermediate-state test and the depth-first walk over the intermediate states of
   an atomic sequence — and for every move that lands on a **stored** state appends a record
   `(hash, parent id, q, vector)` to the segment's buffer; transitions and atomic steps
   are counted per worker. Asserts, evaluation errors, pool exhaustion and the deadlock rule
   produce *events* (§2.7) instead of side effects. **A move whose `fire` returns an error
   produces no record** (the vector in `next` is half written), and the worker abandons the rest
   of that state's expansion: the error is a terminating event, so the group is the last one that
   matters. The walk over intermediate atomic states stops at the sequential bound (`dstepLimit`
   steps, a budget event with the sequential sentence) and checks the stop flag as it goes, so an
   atomic loop that never blocks ends in a verdict-free `inconclusive`, not in a hang. At the end of the segment the records are
   counting-sorted by partition (a permutation of 32-bit record numbers rather than a copy of
   the records, to be decided by measurement, §2.2.1).
2. **Barrier** (every worker has finished expanding).
3. **Insert** (parallel over partitions, dynamic scheduling). The worker that takes partition `p`
   walks the segments of the group in frontier order and, in each, the records of `p` in
   expansion order; for each record it probes the partition's table: a vector already stored is
   dropped; a new one is appended to the arena, its parent id is stored, and **the invariant,
   reach and watch expressions are evaluated on it** through the code of `checkState` (including
   its reset of `Layout.Timeout`; events again), skipping the properties already decided when
   the group began. Only the owner of `p` touches `p`'s table, arena and parent array in this
   phase, and **phase 3 always runs to completion** (§2.6).
4. **Barrier**, then on the calling goroutine alone: merge the events of the group (§2.7),
   apply the decisive ones, check the budgets (§3.5), decide whether to stop, and size the next
   group.

At the end of a level the new ranges become the frontier. The search is complete when the
frontier is empty. There is no work to steal and no termination protocol: a barrier-synchronised
loop ends when a level adds no state.

#### 2.2.1 What the prototype says about the cost of the records

At one worker on `two1000` the two record loops are 36% flat of the time, the table 34%, the
model 13%. The first optimisations to measure in step 8 (each is invisible to every result):
a permutation of record numbers instead of a sorted copy; no stored hash (recompute, 4 ns) or no
stored parent (the segment knows it); one histogram per group instead of per segment (with
256-state segments the 257-entry histogram has as many entries as the segment has records);
adaptive segment size `S = clamp(groupStates / (8 * workers), 32, 512)`.

### 2.3 The visited structure

A new type `partSet` in a new file `explore/parvisited.go`; `compact` and the `Visited`
interface are **not modified** (the default path keeps its exact code and speed).

* **Hash:** a fixed-seed 64-bit function of the vector, a few lines of word-at-a-time multiply
  and fold arithmetic (wyhash-like), little-endian reads, so it is the same on every platform.
  `hash/maphash` cannot be used for routing: its seed is random per process, which would make
  the partition of a state — hence the frontier order, hence the counterexample and the memory
  estimate — change from run to run. A test pins the value of the hash on a few vectors and a
  statistical test checks the balance of the partitions on structured vectors (counters,
  one-hot program counters, channel buffers).
* **Table per partition:** an open-addressing table of 64-bit words as in `compact`
  (fingerprint = bits 24–55 of the hash over the index plus one, position = the low bits;
  the partition is bits 56–63, so the fingerprint keeps its 32 bits of information within a
  partition), grown
  by doubling when it would exceed half full, rehashing the arena in index order; a partition
  is created at its first insert with a 256-slot table (so a 20-state model costs kilobytes,
  not the 28 MB that 256 eagerly allocated partitions would).
* **Arena per partition:** the chunked arena of `compact` (fixed 16 KiB chunks, vectors never
  move). Because no phase reads an arena while another writes it, the chunk directory is an
  ordinary slice and needs no atomic access; this is the whole synchronisation story of the
  arena.
* **Parent array per partition:** `[]uint32`, the id of the state whose expansion first produced
  this one (4 bytes per state; +12% of the 32 bytes a 16-byte state costs, less for larger
  vectors). The initial state has none.
* `Bytes()` of the set is a pure function of the partition sizes (chunks, table length, parent
  array counted by formula, not by `cap`), and the partition sizes are a function of the set of
  states and the fixed hash: **it does not depend on the worker count, the scheduling or the
  run**. It does depend on the hash constant, like every figure that depends on a data layout.

### 2.4 Workers, compiled copies, scratch

* `compile(m)` is called once per worker. The only mutable state of a compiled model is
  `ir.Layout.Timeout` (written at `explore.go:960` in `hasEnabled`, `:1000` and `:1042` in
  `nextEnabled`, `:1087` in `fire` and `:1320` in `checkState`; read by `Compiled.Eval`, `ir/expr.go:322`);
  everything else `fire` writes is its own `next` buffer. A copy costs 0.01–0.7 ms and 7–282 KB on the corpus (largest:
  `leader`, 9 processes, 270 edges), so a 16-worker run pays about 11 ms and 4.5 MB: negligible,
  (the compiles run one after the other on the coordinator before any worker starts;
  `NewLayout` only builds new `Slot`s that point at the model's `Var`s and writes nothing into the
  model, which is therefore read-only for all workers), and `ir` stays untouched (a change to the signature of `Compiled.Eval` would touch every
  frontend and verifier). The compiled copies share the immutable `*ir.Model`; the race
  detector must show that nothing in `ir` or `cex` writes lazily (a grep found no `sync`,
  cache or memo in either package).
* Each worker owns: a `search` shell (its compiled copy, `cur`/`next` buffers, the stack of
  intermediate atomic states), its event list, its counters (transitions, atomic steps), its
  watch coverage (OR-ed at the end, which is commutative), its free list of record buffers.
* The watch expressions (`Options.Watch`) are compiled once per worker against its own layout.
* Worker 0 is the calling goroutine; `W-1` further goroutines are started once per run and woken
  by a channel per phase. **With one worker no goroutine is started at all**: the parallel
  algorithm runs single-threaded, which is both the cheapest configuration and the one in which a
  data race is impossible; the tests compare it with `W > 1` (§5).

### 2.5 Frontier representation and hand-off

The frontier is a slice of `(partition, lo, hi)` ranges built from the partitions that received
a state, **not** a loop over 256 partitions: that is what keeps narrow levels cheap (§2.9). A
segment is `(partition, lo, hi)` with `hi - lo <= S`. States are copied out of the arena into the
worker's `cur`; nothing is handed over by pointer, so the arena may be written (phase 3) after
the segment is done without any aliasing question.

### 2.6 Termination, cancellation

Termination: the frontier is empty after a group sequence (or a budget or an event stopped the
search; the run is then `finish()`ed like a sequential one, which is **reused unchanged** to turn
the stop reason into `Complete` and into the verdicts of undecided properties).

A panic in a worker (an engine defect, never a model error: model errors are `error` returns) is
recovered in the worker; the deferred recovery **counts as the worker's arrival at the barrier**
(`wg.Done` runs after the recover), so the others are not left waiting, and the run ends with an
`explore.InternalError`. That is a **distinct error type**: `cli` maps it to exit 1 with the message
on stderr and the MCP server to a tool error — never to the rejection of exit 2 that
`runCheck` produces for any other error of `Run` (the input was not refused). The run produces no
verdict, as a failure of the sequential search would. A test injects a panic into a worker in each
phase and checks the exit code, the tool error and that the call returns.

Cancellation (the time budget, `ctx.Err()`): the workers check it every 256 states **in phase 1 and
in the walk over atomic intermediates, and between phases, never inside phase 3**: the arena,
table and parent array of a partition are append-only and cannot be withdrawn, so phase 3 of a
group that has started runs to completion (bounded: at most `Rcap` bytes of records). A stop flag
seen in phase 1 **discards the whole group** (records, events and counts), and so does a flag seen
before phase 3 begins; the run stops with "time budget exhausted" and is inconclusive, with
`states` the sum of the partitions after the last group that completed. No group is ever half
inserted, and a discarded group never contributes to a verdict.

### 2.7 Events and the deterministic, shortest counterexample

**The order, and what it is not.** The parallel search expands the frontier in ascending state id
(partition, then arrival index). That is a third breadth-first order: neither the DFS's nor that
of `--bfs`, whose queue (`explore.bfs()`) is in order of first discovery (the parent's queue
position, then the move's rank). I do **not** preserve `--bfs`'s order: reproducing it means
sorting every level by (parent rank, move rank) and reading the frontier through a permutation
(random arena reads and a sort per level), which spends the locality that makes the search fast
and buys only the identity of the decisive event among several. What this costs in semantics is
stated in §3.7, with a model in which the existing DFS and `--bfs` already disagree.

Everything the sequential search does as a side effect (`decide`, `fail`, `budget`) becomes an
*event* with a **total key** that places it in the sequential program order of the expansion of
its state `u`:

`key = (layer, id of u, q, slot, sub)`

* `q` is a step counter of the expansion of `u`, incremented at every observable step in the order
  the sequential code performs them: each call of `nextEnabled` (an error it returns — in `else`,
  `provided`, rendezvous matching, `timeout` evaluation — is an event of that step, with no `fire`
  before it), each `fire`, each `intermediate` test. Atomic intermediates continue the same
  counter, in the depth-first order of their walk.
* `slot` orders what happens in one step: 0 the **single outcome of the step itself** (a `fire`
  returns `(failed, err)` and the sequential code looks at `err` first and breaks, so a `fire`
  that returns an error is one error event and its `failed` assert is **not an event at all**; only
  when `err` is nil can `failed` produce the assert event), 1 the decision to store the successor
  (the state budget; the partition capacity), 2 the checks of the stored successor.
* `sub` orders the checks of slot 2 as `checkState` evaluates them: the watch expressions, then the
  invariants by property index, then the reach conditions by property index. A deadlock of a state
  has `q` = infinity (after the last step); the checks of the initial state have the smallest key.

The events and their sequential counterparts:

| Event | Produced | Slot | Sequential counterpart | Applied as |
|---|---|---|---|---|
| checks of the initial state | driver | smallest key | `checkState(init)` | `decide` / `fail` |
| error of `nextEnabled`, `fire` or `intermediate` (domain, index, d_step block, guard errors...); **suppresses the failed assert the same `fire` returned** | phase 1 | 0 | `handleErr` | `fail` with the trace to the offending step |
| pool exhaustion (`poolExhausted`) | phase 1 | 0 | `handleErr` -> `budget` | stop "process budget exhausted" |
| failed assert | phase 1 | 0 | `assertFailed` | `decide` violated for every undecided assert property |
| atomic sequence longer than `dstepLimit` | phase 1 | 0 | `budget` | stop "depth budget exhausted: an atomic sequence..." |
| the state budget cannot store a new state | the exact-tail path | 1 | `store` -> `budget` | stop "state budget exhausted" |
| partition capacity reached | phase 3 | 1 | (none: new bound) | stop "partition capacity" |
| evaluation error of a check on a new state (it names the property, or the watch) | phase 3 | 2 | `fail` | `fail` **unless that property is already decided at its key position** |
| invariant violated on a new state | phase 3 | 2 | `decide` | `decide` violated |
| reach condition holds on a new state | phase 3 | 2 | `decide` | `decide` verified with a witness |
| deadlock of a stored state | phase 1 | after the last step | `deadlockAt` | `decide` violated |

* **A decided property is never evaluated again.** The sequential `checkState` skips an invariant or
  reach condition whose property is decided (`explore.go:1333`, `:1345`), so an evaluation error
  in its expression never surfaces after the decision (an invariant `a[i] == 0` violated in layer
  1 stays `violated`, the run goes on and `deadlock` is still `verified` when a later state has
  `i` out of range). Phase 3 skips the properties decided when the group began, and the merge
  drops an error event whose property was decided by an earlier event of the group, in key order.
  Errors in watch expressions stay as they are today.
* **Bounded retention.** Events wait for the group barrier, so a model that fails an assert on every
  transition would otherwise queue an event per transition. Each worker therefore keeps **at most
  one event per kind and property**: the smallest key of the decisions for each property (assert,
  deadlock, invariant, reach), the smallest key of the check errors of each property and of the
  watch, and the smallest key of each terminating kind (error of `fire`/`nextEnabled`/
  `intermediate`, pool exhaustion, atomic bound, partition capacity). That loses nothing: after the
  first decision of a property every later one is a no-op ("first verdict stands"); of the errors
  of a property, either the smallest comes after its decision (then all are dropped) or it
  applies and stops the run (then the rest are dropped); of a terminating kind only the first
  can apply. The merge sorts the union of the workers' retained events (at most
  `(properties + 6) * workers`, each with the state it needs for a trace: one vector) and applies
  it in key order. The retained events are accounted in `worker_bytes_est`, not in `est`, because they
  depend on the worker count; scenario 31 has a high fan-out model with an always-failing assert
  and a small memory budget.
* Phase 1 produces the events marked so; phase 3 and the exact-tail path produce the others, and the
  key of such an event is that of the record that first stored the state (or that could not),
  which the record carries (`id of u`, `q`).
* At the end of a group the events of all workers are sorted by key and **applied in that order
  through the same `decide` / `fail` / `budget` code the sequential search uses**: "the first
  verdict stands", "an error ends the search and turns every undecided property invalid-model",
  "a verdict decided before an error keeps its verdict", "a budget stops the search". An event
  whose terminating effect has been applied (`fail`, a budget, or `decide` making every property
  decided without `--sweep`) **drops every later event of the group**, including those produced
  speculatively by a later expansion of the same group: a truncated run never claims a violation
  that lies after its stop point in this order. The budget stops are events of the stream, so the
  "found before the stop" rule is the key order and nothing else.
* Within a layer the groups are consumed in frontier order and every key of a group lies above
  every key of the previous group, so applying events group by group equals applying them layer by
  layer (the layer is the leading component of the key).
  A property's candidates within one layer lie in the same layer of the graph (a deadlock of a state
  of layer d needs d hops; an assert, an invariant or a reach found while expanding layer d needs
  d + 1), and the layers are processed in increasing order, so **the decisive event of a property is
  on a counterexample that is shortest in layers**, and among those it is the first in key order.
  Without atomic sequences a hop is one move, so it is shortest in moves; with them, candidates of
  one layer can differ in the number of moves of their atomic chains, and the sequential
  breadth-first search has the same property (it counts the chain in `depth` but queues by hop),
  so the length of a counterexample is compared with `--bfs` only for models without atomic
  sequences. Which of several shortest counterexamples is reported is **not** that of `--bfs`.
* After the stop the counterexample is rebuilt on the calling goroutine: follow parent ids to the
  initial state; for each step re-expand the parent (single-threaded, a single state) and take
  the **first** move or atomic chain whose stored leaf equals the child — the first in expansion
  order, which is the record that won the insertion, so the path is the one the search took;
  replay it with `fire` to obtain the intermediate states; `cex.Build`. The last step is the
  event's own move. An internal consistency check (the replayed state equals the stored vector,
  the first state equals `Initial()`) turns any mismatch into an internal error, never into a
  wrong trace.
* **Why the choice does not depend on the worker count**: the frontier order is ascending id; the
  id of a state is its partition (a fixed hash) and its arrival index in that partition; the
  arrival order in a partition is the order of the records in the groups' segments, which is the
  frontier order of the expanded states, then their expansion order. None of these involves a
  worker, a segment boundary, a group boundary or a clock.

### 2.8 Memory accounting and the record guard

`memory_bytes_est` and `--budget-mem-mb` stay a pure function of the run:

`est = partSet.Bytes() + groupBuffers + frontier`, where `groupBuffers` is the maximum
over the groups of `records * recordBytes + segments * segmentBytes` (`recordBytes` covers the
hash, the parent id, the step counter, the vector and the record's share of the sorting arrays;
`segmentBytes` the per-segment histogram) and `frontier` is the range list at its peak, all counts
times constants, not `cap`. The retained events are bounded (§2.7) and counted in `worker_bytes_est`.
**Not included** (stated in the report and the docs, `search.parallel.worker_bytes_est` gives
it): the per-worker copies of the compiled model and the per-worker scratch, which depend on the
worker count. The budget is checked at the end of each group (and by the guard below), and the
message is the sequential one ("memory budget exhausted: estimate N bytes exceeds M").

**Group size** is `G` frontier states, `G = clamp(Rsoft / (avgRecords * recordBytes), 256, 262144)`
with `Rsoft` = 64 MiB, `avgRecords` the records per frontier state of the previous group that
completed (carried across levels; the first group of the run takes 1 024 states): a function of
counts and of the model alone, never of the worker count (segments, which do depend on it, are
cut inside a group and change no result).

**The record guard (a hard bound, because the size of a group comes from the previous group and a
fan-out can jump).** The phase-1 workers add the bytes of their records to one shared counter
before they write them. The group has a hard capacity `Rcap = min(256 MiB, memory budget - est)`.
When the counter passes `Rcap` the group is **overflowed**: every worker leaves, the group's
records, events and counts are discarded, and the group is redone with half as many frontier
states (the halves in frontier order), recursively. Whether a group overflows is a function of
its total record bytes alone (the counter is a sum, abandoning early only skips counting what is
already known to exceed), so the sequence of groups — hence every result — is the same for every
worker count. A group of one state that still overflows ends the run as memory-budget exhausted
("memory budget exhausted: the expansion of one state needs N bytes of records"), `inconclusive`,
evidence `unknown`. A run therefore returns a report instead of relying on the runtime to survive
an abrupt fan-out; the cost is the work of an overflowed attempt, bounded by `Rcap` per halving.
A test with a model whose fan-out jumps by two orders of magnitude and a small memory budget
checks that a report comes back, the same for 1 and 8 workers.

Measured (prototype, `bench-indep` N=6): resident set +16–20% over the sequential DFS. The real
implementation adds the event lists (tiny) and the larger vector arena of wide models is the
same.

### 2.9 Narrow levels (the criterion A8)

A group of fewer than `T` states is run **inline** on the calling goroutine: no wake-up, no
barrier, no record buffers: each successor is inserted at once, in *global record order* (not
partition order), with the exact same events. The result is bit-identical to the batched path
because insertion order **within each partition** is the same (the record order restricted to
that partition); the prototype shows it (§1.4). The per-level fixed work is O(touched ranges),
not O(P). This path is also what runs the last group of a run with a state budget (§3.5). `T`
may depend on the worker count and is tuned by measurement in step 8 (the prototype used 128;
`two1000` suggests that at one worker a much larger `T` is right, and at several workers a
smaller one); it can never change a result, and a test forces it to 0 and to infinity. With one
worker the same two paths run, without goroutines. Measured cost on `chain`: below the
sequential DFS (§1.4); the criterion is <= 1.5x.

## 3. Semantics that must not change silently

### 3.1 What stays exactly the same

* **Verdicts and evidence** of deadlock, assert, invariant, reach and the invalid-model rule,
  **for complete runs and for every model without an ending event, are the sequential ones**
  (A12; with an ending event, §3.7): the stored set is the reachable set (every
  reachable state is the successor of a state of the previous layer, and every successor of
  every expanded state is either inserted or found stored), invariants and reach conditions are
  evaluated on every stored state exactly once, the deadlock rule on every stored state's
  expansion, asserts on every `fire`. `complete` is set by the same `finish()`.
* **`states`, `transitions`, `atomic_steps`** of a complete run: each is a sum of a function of
  the stored state (transitions out of it; atomic steps in its atomic tree) over the set of
  stored states, hence independent of the order. Equality with the sequential DFS is exact and is
  tested on the corpus and on random models, not argued (A10).
* `fire`, `apply`, `nextEnabled` are **called, not copied**: atomic sequences and d_step,
  rendezvous partner enumeration, the two-phase `timeout`, `provided`, `else`, `run` into the
  process pool and its exhaustion, `ClearChans`, domain overflow, index errors. The new code owns
  only the loop around them.
* The Promela/IR frontends, the report schema (additive field only), the `Stepper`, `estimate`:
  untouched.

### 3.2 `invalid model`, `--sweep`, `--watch`

* `invalid model`: an evaluation error is an event; at the end of its group it is applied in key
  order, so `fail` assigns the reason and the trace *to the offending step* exactly as the
  sequential breadth-first search does (the worker captures the partially fired state with the
  event). Properties decided by earlier events keep their verdicts. A run that ends this way is
  not complete, as today.
* `--sweep`: `decide` does not stop the search when `Sweep` is set, so the run goes on to the end
  of the graph; counters then are the full ones (A10 also holds for violated models under
  `--sweep`).
* `--watch` (vacuity, FR-011): the watch expressions are evaluated on every stored state in phase
  3 by the owner; per-worker "ever true / ever false" flags are OR-ed at the end; the result is
  meaningful only when the run is complete, as today. Because refusals exclude `ltl`/`ctl`
  properties, the watch normally holds only what the caller passed; it is supported so the
  differential oracle can compare it.

### 3.3 `depth`

The sequential DFS reports the greatest stack height; that is a property of the search order
(`counters-10-6`: 999 999; the shortest-path depth is 54). The parallel search reports the
**greatest layer expanded** (`counters.depth`), which equals the sequential `--bfs` depth for a
model without atomic sequences. With them it is never above it (the sequential BFS adds the
length of an atomic chain to a state's depth; a layer counts a whole atomic sequence as one
hop) and never above the DFS depth (a hop is at least one move). The report sets `search.mode` to `"bfs"` when the
parallel search ran, and `search.parallel.layers` / `max_layer_states` expose the shape.

`--budget-depth D` in parallel: layers `<= D` are expanded; states of layer `D + 1` are stored and
not expanded; the run ends with the sequential sentence ("depth budget exhausted: N state(s) at
depth > D were stored but not expanded") with `N` = the size of layer `D + 1`, and is
inconclusive. For a model with atomic sequences the layer count differs from the sequential BFS's chain-based
`depth`, so the depth budget cuts at different states there (documented; the rule "stored but
not expanded" is the same). Consequence, stated in the report note and the docs: a depth budget is satisfied
more easily by the parallel search than by the DFS (the BFS depth is the smaller one), so a run
the DFS leaves inconclusive on depth can complete here, and never the reverse for completeness
(a DFS run that completes has BFS depth <= D too).

### 3.4 Counterexamples, truncation and what a truncated run may claim

* Counterexamples are shortest and deterministic (§2.7). They are generally **not** the DFS's, and
  among several of the shortest length not `--bfs`'s either.
  `mc_explain` and the replay tools work unchanged (a trace is a trace).
* A run stopped by a budget is `inconclusive` with evidence `bounded` (states, depth) or
  `unknown` (time, memory), exactly as now; it never says `verified`, and `complete` is false.
  A violation found before the stop stands (its trace is an exact run). A `reach` witness found
  before the stop makes `reach` verified, as today.
* **Either search can decide what the other leaves inconclusive under a budget** (the same
  sentence as for `--por`): the parallel search stores different states within a state budget
  than the DFS does, and a BFS finds a shallow violation a DFS may not reach, while a DFS may
  find a deep one long before the BFS completes the layers above it. Neither contradicts a search
  that completes.
* The parallel search is not asked to find bugs faster; §1.5 item 4.

### 3.5 Budgets in parallel

| Budget | Behaviour | Deterministic across worker counts? |
|---|---|---|
| states | `states` never exceeds the budget. While `stored + records of the group <= budget` the group runs in parallel (no insertion can cross the budget). Otherwise the group is inserted by the inline path in global record order, and the first new record that cannot be stored is a **budget event with its own key** (§2.7, slot 1): every later event of the group is dropped, so no violation after the stop point is claimed, and `states` equals the budget exactly. `transitions` and `atomic_steps` of that group are counted in full (the whole group was expanded); documented. | yes |
| depth | §3.3 | yes |
| memory | checked at group ends on the §2.8 estimate | yes |
| time | the context deadline, §2.6; the stop is at a group boundary (the group in flight is discarded), so the counters are those of the completed groups | no (never was) |

### 3.6 What changes, stated for the report and the docs

`search.mode` becomes `bfs`; `depth` is the BFS depth; counterexamples are shortest; counts of
runs that stop early (violation, error, budget) are those of a deterministic group boundary, not of the
DFS's accident; `memory_bytes_est` excludes per-worker copies; the order of `--sweep` events
differs. None of these can turn a `verified` into a `violated` or the reverse for a complete run.

### 3.7 Outcomes that depend on the search order (a property of the engine, not of this design)

The sequential search has *ending events*: an evaluation error or domain overflow in the model
("invalid model", `fail`), a `run` that exhausts the process pool, an atomic sequence longer than
`dstepLimit`. `fail` turns every property that is **still undecided** into `invalid-model`, and
`decide` keeps the first verdict. Which properties were decided before the ending event depends on
the order in which the search meets the events, so the statuses of a model that has a reachable
ending event depend on the search order — already today, between the DFS and `--bfs`. Measured on
the unchanged engine, on a model with two branches (`x = 1; x = 2; assert(x != 2)` and
`b = b + 1` with `byte b = 255`):

| Search | `assert` | `deadlock` | stop |
|---|---|---|---|
| default (DFS) | **violated** (exhaustive, 4-step counterexample) | invalid-model (domain overflow) | invalid model |
| `--bfs` | **invalid-model** (domain overflow) | invalid-model (domain overflow) | invalid model |

So "equal to the sequential search" cannot be a requirement for such models: there are two
sequential answers. The parallel search is deterministic in **its own** order (§2.7) and the plan's
contract is:

1. A model **without** a reachable ending event: every property has the status, evidence and reason
   of the sequential DFS and `--bfs` (A12), for complete runs and for violated ones.
2. A model **with** one: nothing is ever `verified` (the run is not complete); every `violated` or
   `invalid-model` carries an exact replayable run; under `--sweep` the search ends on an ending
   event iff the sequential ones do (they all meet it); the properties decided before it are those
   decided before it in the parallel order, which may match neither the DFS nor `--bfs`; the report
   does not depend on the worker count.
3. Under a budget: either search may decide what the other leaves inconclusive (§3.4).

Tests: scenario 20 (the model above, `testdata/promela/par-order.pml`) asserts the contract, not a
particular winner; the oracle classifies each random model by a sequential `--sweep` run
("ending event reachable?") and applies A12 or A12b accordingly; a directed pair of models, one
per order, makes the parallel result differ from the DFS's and from `--bfs`'s and checks that it is
still one of the legal outcomes and the same at 1 and 8 workers.

## 4. What is refused, and the surface

### 4.1 Refusals (reason in the report, run is the sequential default)

A refused run is **the default sequential run**, byte-identical to a run without `--workers`
except for the `search.parallel` object that says why. Reasons (exact text is a test):

1. **Temporal property** in the run (`ltl`, `progress`, `ctl`, including `--progress`): "an ltl,
   progress or ctl property is decided by a nested depth-first search or a graph labelling that
   this version does not parallelise; the whole run was executed sequentially". (A mixed run
   could parallelise its safety part; that is an increment, not v1: the safety search of a run
   with a never claim is usually small next to the product search, and `applyVacuity` consumes
   its coverage, which is one more coupling to test.)
2. **`--por` that applies**: partial-order reduction is depth-first (its cycle proviso reads the
   DFS stack). "partial-order reduction is depth-first only; with `--por` the reduced sequential
   search ran. Drop `--por` to search in parallel". When `--por` was asked but its own analysis
   refuses (atomic sequences, rendezvous, ...), **the parallel search runs** and
   `search.reduction` keeps its `applied: false` and its reason; neither report field refers to
   the other.
3. `NewVisited` supplied (a test hook): refused, internal.

`--estimate --workers N` is a usage error (exit 1, message on stderr): the estimate is a
sequential time-bounded measurement and an ignored flag would be a lie. (`--estimate --por`
silently ignores `--por` today; that is recorded in perf2 and is not changed here.)

`--workers` below 0 or above 256 is a usage error. A value above the number of CPUs is accepted
(oversubscription is the caller's choice; the report echoes the request).

### 4.1.1 Where the dispatch sits

**`Options.Mode` is never changed by `Workers`.** It stays the *requested sequential mode* (DFS
unless `--bfs`), which is what the POR block reads: `Run` refuses the reduction when `Mode` is BFS
(`explore.go:316-319`) and otherwise analyses it before any traversal is chosen. The decision
sequence is therefore: (1) POR analysis with the requested mode, unchanged; (2) the choice between
sequential and parallel, made after it; (3) `Result.Parallel` records what ran, and the surfaces
(CLI, MCP, `report.Build`) compute the reported mode from the request and from `Result.Parallel`
(§4.2). With `--por --workers N`: the reduction applies (DFS requested, plan accepts) and the run
is the reduced sequential one; with `--por --bfs --workers N`, or when the analysis refuses, the
parallel search runs.

`explore.go` gets two small hunks: the field `Options.Workers` (and `Result.Parallel`), and one
branch where `Run` chooses between `s.bfs()` and `s.dfs()`: when `opt.Workers > 0`, a function in
`parallel.go` decides (`s.por != nil` means the reduction applied, the properties of `c.props` say
whether a temporal one is present, `opt.NewVisited != nil` is the test hook) and either runs
`s.parallelBFS(opt.Workers)` or records the refusal and falls through to the unchanged code. The
POR block and everything before `hasSafety` are not edited.

### 4.2 Names and JSON (decided)

* CLI: `--workers N` (int, default 0 = sequential, unchanged). `N >= 1` selects the parallel
  search; `N = 1` runs it single-threaded (§2.4), so results do not depend on `N`.
  `--bfs --workers N` runs the parallel search (it is a breadth-first search). **One rule for the
  reported mode, used by the CLI and by the MCP server alike: `search.mode` is `"bfs"` iff `--bfs`
  (`search: "bfs"`) was given or `Result.Parallel` says the parallel search was applied**; the
  surfaces derive it from the result after the run (the CLI builds `Meta.Mode` from `--bfs` before
  the run today, the MCP handler records `Params.Search` before it), and the session's `Params` also
  records `workers`.
* `explore.Options.Workers int`; `explore.Result.Parallel *Parallel`.
* Report `search.parallel`, present **only** when `workers > 0` was given:

```json
"parallel": {
  "requested_workers": 8,
  "applied": true,
  "workers": 8,
  "layers": 71,
  "max_layer_states": 24367,
  "note": "breadth-first layers; depth, counterexample and stop point are those of the layered search, counters of a complete run equal the sequential ones",
  "worker_bytes_est": 1048576
}
```

  and, when refused, `{"requested_workers": 8, "applied": false, "reason": "..."}`. `layers` and
  `max_layer_states` are functions of the model (for a complete run), so they do not break A11;
  `requested_workers`, `workers` and `worker_bytes_est` are the only fields that may differ between worker counts. `Result.MaxDepth` is the greatest layer whose states were expanded (a group counts once its expansion phase has run), and `states` at a stop is the sum of the partition sizes after the group: both are deterministic.
* MCP `mc_check` input `workers` (integer, optional, `omitempty`), clamped to the server ceiling
  (`GOMAXPROCS` by default) with an entry in `search.budget_notes` (the existing channel for a
  clamped field; `search.parallel.note` stays a constant sentence so that A11 remains a byte
  comparison); `mc_manifest`, the schema tests
  and the alignment tests follow from the generated schema. `search.parallel` in the `SearchOut`.
  The server's `concurrency` semaphore counts checks, not workers; the docs say that the
  CPU use is `concurrency * workers`.

### 4.3 Interaction with `estimate`

`estimate` stays sequential and unchanged (its `states_per_second` is the sequential rate). The
skill documentation says: an estimate that classifies a model as "large" is the case where
`--workers` is worth trying, with the wide/narrow caveat of §1.5. Using the parallel search for
estimates (deeper levels in the same time) is a non-goal here.

## 5. Test strategy

BDD first: `features/g8-parallel.feature` and `steps_g8_test.go` (`registerG8Steps`) are written
red before the engine code, as in perf2/perf4. Layers: the partitioned set (lowest) first, then
the search, then the surfaces.

### 5.1 Gherkin scenarios (`features/g8-parallel.feature`)

1. Without `--workers` the report has no `parallel` object and `search.mode` is `dfs` (bytes equal
   to a recorded run).
2. A complete run has the same verdicts, `states`, `transitions` as the sequential run
   (`bench-indep` N=4: 41 371 states) and `depth` equal to the `--bfs` run's.
3. The report is the same for 1, 2 and 7 workers (all fields but `parallel.workers`) and for a
   repeated run.
4. A violated assert (`por-shared.pml`): a shortest counterexample, as long as the `--bfs` one,
   replayable, the same for 1 and 8 workers.
5. A deadlock (`por-deadlock.pml`) found; counterexample deterministic across workers.
6. An invariant violated and a reach witness (`por-visible.json`).
7. Atomic sequences (`bench-sym.pml` N=3, `atomic-t3.pml`): the same counts as the sequential run
   (stored states, transitions, atomic steps).
8. Rendezvous and `run` (`client_server.pml`): same counts; the pool-exhaustion bound
   (`--max-procs` small) gives the sequential "process budget exhausted" inconclusive.
9. A state budget: `states` equals the budget exactly, inconclusive, evidence bounded; the same
   report for 1 and 8 workers.
10. A depth budget: the sequential sentence with the size of the first unexpanded layer;
    inconclusive, bounded; the same for 1 and 8 workers.
11. A memory budget: stops with the estimate sentence, never `verified`.
12. An invalid model (`dstep-block.pml`, a domain overflow): `invalid-model` with the trace to
    the offending step; same for 1 and 8 workers; properties decided earlier keep their verdict.
13. `--sweep` after a violation: the whole graph is counted (equal to the sequential `--sweep`).
14. Vacuity watch: the same coverage as the sequential run on a complete run.
15. A temporal property in the run: `parallel.applied` false with the reason, and the report equals
    the sequential one apart from that object.
16. `--por --workers 4` on a model where the reduction applies: the reduced run, `parallel`
    refused with the reason; where it is refused (atomic): `parallel` applied and
    `reduction.applied` false.
17. `--workers 4 --estimate`: exit 1; `--workers -1` and `--workers 1000`: exit 1.
18. (`g2-mcp.feature`) `mc_check` with `workers`: `search.parallel` present and applied; without:
    absent; a request above the ceiling is clamped with a note; a refusal is not a tool error.
19. A narrow model (`chain.pml` scaled down): complete and equal counts (the inline path).
20. An ending event and a violation in different branches (`par-order.pml`, §3.7): the report is
    the same for 1 and 8 workers and for a repeated run, no property is `verified`, the `assert`
    status is `violated` or `invalid-model` and carries a replayable run either way.
21. An abrupt fan-out under a small memory budget (a model whose states have 1 and then 300
    successors): a report comes back, `inconclusive` with the memory sentence, the same for 1 and
    8 workers (the record guard of §2.8).
22. An error of a guard during enabledness probing (`else` / `provided` / a division by zero in a
    guard) and an assert on neighbouring steps: the status of every property follows the key
    order of §2.7 and is the same for 1 and 8 workers.
23. A state budget that falls between an assert event and an error event of the same group: `states`
    equals the budget, and only events before the stop point count (a truncated run claims no
    violation that lies after it).
24. An invariant violated in layer 1 and an evaluation error in its expression on a later state
    (an array indexed by a variable), with a `deadlock` property and no `--sweep`: `invariant`
    violated, `deadlock` verified, the run complete, as sequentially (a decided property is never
    evaluated again).
25. An atomic loop that never blocks (`atomic { do :: x = (x + 1) % 2 od }`): inconclusive with the
    sequential sentence ("depth budget exhausted: an atomic sequence exceeds 100000 steps"), not
    a hang, for 1 and 8 workers.
26. The initial state itself violates the invariant: `violated` with a one-state counterexample,
    the same for 1 and 8 workers; likewise an initial reach witness, an initial watch truth
    (coverage) and an evaluation error in an expression on the initial state (the step 0 of §2.2).
27. A depth budget with the violation in layer `D + 1`: the stored-but-unexpanded layer is still
    checked, so the invariant is `violated` (as sequentially), and the run is not complete.
28. A Promela model with a `never` claim that is not asked as a property: the safety search stores
    the claim's location and does not execute it; counts equal the sequential run's.
29. An injected engine panic in a worker (a test knob): exit 1 with the message on stderr, not exit
    2 and not a verdict; for the MCP tool, a tool error.
30. One edge with `assert(0)` and an effect that overflows its domain (`byte x; x = 300`): the
    properties are `invalid-model` with the trace to that step and the assert is not `violated`,
    as sequentially (an error of `fire` suppresses its failed assert).
31. A model whose every transition fails an assert and whose states have a high fan-out, under a
    small memory budget: a report comes back (the retention rule of §2.7 and the record guard of
    §2.8), the same for 1 and 8 workers.
32. The same `--por` / `--bfs` / `--workers` combinations through the MCP tool (`por`, `search`,
    `workers`): applicable `por` with `workers` is the reduced run with `parallel` refused; `search:
    "bfs"` with `workers` is the parallel run; both report the mode the run had.

### 5.2 Unit tests (package `explore`)

* `parvisited_test.go`: `partSet` against the map reference on random sequences of lengths 1, 3,
  16, 100, 5000 and above a chunk, across table growths; a duplicate is never inserted; ids map to
  the stored vector; `Bytes()` is the same for any insertion order and any partition-visiting
  order; the fixed hash is pinned on literals; partition balance (chi-square bound) on structured
  vectors; the per-partition capacity stop; equal fingerprints of different vectors are told
  apart (the `addHashed` seam technique of `visited_test.go`).
* `parallel_events_test.go`: the event order of §2.7 by direct construction (events built out of
  order and sorted): an error of `nextEnabled` before and after a failed assert of the same state;
  an error of `fire` together with a failed assert (the error wins); a watch error, an invariant
  that is false and a later invariant that errors on one stored state (the sequential
  `checkState` order); two properties decided on one state; events on both sides of an exact
  state-budget boundary and of an `all properties decided` stop; a deadlock after the last step
  of its state; the drop of every event after a terminating one.
* `parallel_test.go`: the search against the sequential one on the fixtures; one worker equals
  many; events sorted and applied in key order (a unit test on the merge with events given out of
  order); budget points; the inline path equals the parallel path (forced by a test knob that
  sets `T` to 0 or to infinity); segment and group sizes forced to 1, 3, 1 000 000 **while the worker count varies too** give the same
  report, budget-truncated ones included; counterexample reconstruction on atomic chains and on duplicate-producing moves (two
  moves from one state reach the same successor: the first is reported).

### 5.3 The differential oracle (sequential against parallel)

`explore/parallel_random_test.go`, in the style of `por_random_test.go`, over **random models**
from the existing generators extended to what POR refuses and this search supports (atomic,
rendezvous, `run`, `timeout`, `provided`, d_step, `else`, `ClearChans`, assert, invariants and
reach on one and two variables; process counts 1–6), and `MCD_PAR_MODELS` for deeper runs:

* classification first: a sequential `--sweep` run (DFS) says whether an ending event is reachable;
  A12 applies to the models without one, A12b to the others (§3.7);
* complete runs: per-property status, evidence, reason; `states`, `transitions`, `atomic_steps`;
  the **set of stored states** (through a test hook on the partitions against the recorder of
  `por_random_test.go` on the sequential side) and the **set of states without an enabled move**;
  `Levels` equal to the sequential BFS's (models without atomic sequences); `depth` per §3.3;
* every counterexample and witness replayed with `Stepper` as an exact run (`replay` of the POR
  oracle), its length equal to the sequential `--bfs` one (models without atomic sequences);
* error reachability (`Stop == "invalid model"`) compared before the completeness skip, as the
  second POR review taught (the comparison sat after a skip it could never pass);
* the same report for workers 1, 2, 3, 8, 16 and for a repeated run, **including** budget-truncated
  runs (random state, depth and memory budgets);
* a generator-calibration test: with a known defect put back (§5.6), the oracle must find it
  within a stated number of models; a clean run is evidence, not proof.

`parallel_corpus_test.go` (root package, like `por_corpus_test.go`): every Promela model of the
fixtures and the SPIN corpus that the frontend accepts, sequential DFS and BFS against the
parallel search at 1 and 8 workers: the A10–A13 checks above. Refusals (temporal) are checked to
equal the sequential report. SPIN: `tools/pandiff` is extended with a `--workers`-style run of
the engine side only (the engine comparison already takes a state count and a verdict from
`explore.Run`); the SPIN-dependent tests run it on the models pandiff already covers and require
`states` equal to `pan -c0` for the parallel run as for the sequential one. Where `spin` or `gcc`
is missing the test skips, as pandiff's do.

### 5.4 Races (the arena and the tables)

The phase discipline makes the arena and the tables race-free by construction (no phase reads a
structure another writes, one writer per partition, barriers between phases), so the hunt is for
violations of that discipline, not for subtle atomics:

* `go test -race ./explore ./...` on the whole of §5.1–5.3 with workers 2, 3, 8 and **64 on
  GOMAXPROCS 2** (more goroutines than cores shakes out interleavings), `-count` 5 on the
  concurrency tests;
* a **stress test**: tiny groups and segments (1 state), tiny and huge models (1 state; 10^6
  states), many levels (`chain` shape), many partitions touched per level (random-walk models),
  all under `-race`;
* the audit above for lazily initialised shared data in `ir`/`cex`/`frontend` types reachable from
  a worker (none found by grep; the race tests are the check);
* a **tripwire test hook** (compiled in tests only): the partition methods assert that they run in
  the phase that owns them (a plain phase variable written only by the single coordinating
  goroutine between barriers, read by workers); a deliberately wrong test harness that inserts
  during the expansion phase must make it fire under `-race`.

### 5.5 Determinism tests

Reports compared as bytes: 1, 2, 3, 8, 16 workers, repeated 5 times, on the corpus, on 2 000+
random models, with random segment and group knobs, with `GOMAXPROCS` 1 and 16, and with the
Go scheduler perturbed (`runtime.Gosched` injected at the barriers by a test knob). The hash seed
is fixed by construction; the test also runs the search twice in one process, because a random
seed per set (the `compact` way) would give different partitions in the two runs.

### 5.6 Mutation tests (concurrency- and order-critical rules)

A scratch harness, as for POR: a list of textual mutants, the **baseline run first and required
green** (the perf4 lesson: a red baseline counted every mutant as killed), each mutant run against
the focused tests, every survivor investigated (killed by a new test, shown equivalent, or
removed as dead code). Mutants planned (killers in brackets):

1. insertion walks the segments out of frontier order, e.g. by completion order [determinism];
2. a partition is inserted by two workers (no ownership) [race test];
3. the first writer of a duplicate is overwritten (last parent wins) [counterexample determinism];
4. dedup only within a group, not across groups [state-count oracle];
5. a group's last segment is not expanded [counters oracle];
6. transitions counted per record instead of per fire [counters oracle on atomic models];
7. atomic steps lost or double counted on the inline path [counters oracle];
8. events applied unsorted, or sorted by the step counter only [event-order unit test, two violations in one level];
9. an event after a terminating event is applied [error-then-assert test];
10. "first verdict stands" removed [two violations];
11. state budget: the exact-tail path disabled [budget scenario, `states` equals the budget];
12. depth truncation off by one layer [depth scenario];
13. memory estimate depends on the worker count [A11];
14. `complete` set when a group was discarded by cancellation [time-budget test with an injected deadline];
15. the compiled copy shared between workers (one `Layout.Timeout`) [`timeout` models under race];
16. partition chosen by a random seed [two runs in one process];
17. hash balance degraded (low byte only) [balance test];
18. counterexample path takes the *last* move instead of the first [duplicate-successor test];
19. the inline path skips the invariant check [inline-forced oracle];
20. the one-worker path starts goroutines [no-goroutine test via `runtime.NumGoroutine`];
21. depth reported as the stack-like count of expanded levels plus one [depth oracle];
22. `reach` witness not carried through the group merge [`por-visible.json` scenario];
23. an invalid-model trace ends at the parent instead of at the offending step [scenario 12];
24. the refusal of a temporal property removed [scenario 15];
25. the key's `q` not incremented for `nextEnabled` errors [scenario 22, the event-order unit tests];
26. the budget stop not an event of the stream (later events of the group survive it) [scenario 23];
27. an overflowed group not discarded, or redone with the same size [scenario 21];
28. the group size taken from the number of segments (so from the worker count) [determinism across workers in the oracle with budgets];
29. `slot` order of an error and a failed assert swapped [event-order unit tests];
30. the checks of one state evaluated in another order than `checkState`'s [event-order unit tests];
31. the error of an already decided property applied [scenario 24; the generator gets array-indexed and division-bearing property expressions];
32. the initial state's own checks not run [scenario 26];
33. a partition skipped by the insert scheduler [the state-set oracle];
34. the frontier range taken after insertion began, so the states of the layer being built are expanded in it [the depth/`Levels` oracle on a model without atomic sequences];
35. the invariant checks skipped on the stored-but-unexpanded layer under a depth budget [scenario 27];
36. an errored `fire` still leaving a record (a half-written vector stored) [scenario 12 with the state-set oracle on a run that ends in an error];
37. cancellation observed inside phase 3 (a half-inserted group) [the injected-deadline test];
38. a recovered panic that does not arrive at the barrier [scenario 29, which must return];
39. the atomic walk without its bound or its stop-flag check [scenario 25];
40. the group size derived from the segment size or the worker count [the budget runs of the oracle, forced segment sizes and worker counts varied together];
41. the clamp of the worker count missing from `budget_notes` [scenario 18];
42. the failed assert of a `fire` that also returned an error applied as an event [scenario 30];
43. a worker keeping the last event per kind instead of the first, or one per transition [event-order unit tests with two candidates; scenario 31];
44. `Options.Mode` set to BFS by `Workers`, so that POR never applies [scenarios 16 and 32].

### 5.7 Standing checks

`go test ./...`, `go vet ./...`, `test -z "$(gofmt -l .)"`, `go test -race ./...`, the SPIN
tests, then the §1.6 measurements, from `model-check-plugin/engine`; then the deployment gate if
`model-check-plugin/` changes (it does: docs, features, manifest schema), without rebuilding
`engine/bin` (a release step).

## 6. Risks

| # | Risk | Mitigation |
|---|---|---|
| R1 | **A lost record or a group-boundary bug gives a false `verified`** (the worst failure of this engine). | Counts and the **set** of stored states are compared with the sequential run on every model; mutants 4, 5, 6; the corpus differential at 1 and 8 workers; refusal rather than approximation for whatever is not proved. |
| R2 | A hidden shared mutable structure (in `ir`, `cex`, `frontend`) makes workers race. | Race tests on every model with W > 1; audit by grep recorded here (none found). |
| R3 | Non-determinism leaks (map iteration, completion order, `append` growth order, time). | §5.5 on the corpus, on random models and under scheduler perturbation; mutants 1, 13, 16. |
| R4 | The group's records explode (a fan-out that jumps) and the run exhausts memory before it can report. | The record guard of §2.8: a hard capacity, an overflowed group is discarded and redone with half the states, deterministically; the peak is in the estimate; a test with an abrupt fan-out and a small memory budget. |
| R5 | A narrow or deep model is much slower than the sequential run. | Inline path; criteria A7, A8; `layers`/`max_layer_states` in the report so the user sees why; the documentation says "not for narrow models". |
| R6 | The prototype overstates the speedup (no properties, no atomic, no events, loaded machine, VM). | The numbers of §1.6 are targets re-measured at load <= 2 on the finished feature; the stop rule A3. |
| R7 | The user reads a parallel counterexample as "the" counterexample or a parallel `depth` as the DFS's. | `search.mode` "bfs", the note, the docs; shortest by construction. |
| R8 | A merge conflict with step 6 (POR extension) in `explore.go`/`por.go`. | New code lives in `parallel.go`, `parvisited.go`; `explore.go` gets two hunks: the `Options.Workers` field and one dispatch line next to `s.bfs()`/`s.dfs()`; the POR block is not edited (the dispatch asks `s.por != nil`). |
| R9 | A 32-bit id limit (2^24 per partition) is reached by a skewed hash. | Counted and guarded: a declared bound, never a wrong verdict; balance test. |
| R10 | GC pressure from the per-transition `make([]int64)` in `apply` (not touched here) grows with workers. | Measured in step 8; allocation is per-P cached in Go; if it limits scaling, a separate change to `fire`'s internals, not part of this stream. |
| R11 | Literature claims in the docs outrun what is in the monorepo. | §9 and the provenance work item. |

## 7. Work breakdown (each step: a red test first, then green; commit per step)

The red test of steps 1–6 is a package test (the oracle and the unit tests), because the flag that
the Gherkin scenarios drive arrives in step 7. As in perf2, the feature file is written first and
seen red (`flag provided but not defined: -workers`) before step 7's code, and is committed with
it; steps 3–6 are gated by the package tests alone, and the table says so.

| Step | What | Red test first | Gate |
|---|---|---|---|
| 0 | this plan, measured; cross-review of the plan | n/a | review log below |
| 1 | `explore/parvisited.go`: fixed hash, `partSet`, `Bytes()`, ids, capacity stop | `parvisited_test.go` (map reference, hash pin, balance, order-independence of `Bytes`) | tests, race, vet, gofmt |
| 2 | `explore/parallel.go`, one worker only (no goroutines): the search without properties, no atomic, no budgets; the batched and the inline path; counts, set of states, levels, checksum of the arenas | `parallel_test.go`: counts and state sets against the sequential run on fixtures and random models; inline equals batched | oracle green at W = 1 |
| 3 | events with the total key of §2.7, deadlock/assert/invariant/reach/invalid-model, the deterministic merge, counterexample reconstruction | `parallel_events_test.go`; the oracle with replay and shortest length (package level) | oracle, mutants 3, 8–10, 18, 22, 23, 25, 29, 30–32, 36, 42, 43 |
| 4 | the rest of the semantics through `fire`: atomic and its bound, d_step, rendezvous, `run` and its exhaustion, `timeout`, `provided`, `ClearChans`, never-claim processes, `--sweep`, `--watch`, the decided-property rule | generators extended (atomic, rendezvous, run, timeout, erroring property expressions); the oracle | oracle on the extended generator; mutants 6, 7, 31, 39 |
| 5 | goroutine pool, many workers, cancellation (phase 1 only), the panic path, time budget | race and stress tests, determinism across W, scheduler perturbation, the injected panic and deadline | `-race` at W = 2, 3, 8, 64 on GOMAXPROCS 2; mutants 1, 2, 15, 16, 20, 33, 37, 38 |
| 6 | budgets: exact state tail with its key, depth layers, memory estimate and the record guard, group sizing, the inline threshold | truncated-run determinism in the oracle; the abrupt-fan-out test | mutants 11–14, 19, 21, 26, 27, 34, 35, 40 |
| 7 | surfaces: `Options.Workers`, `Result.Parallel`, `InternalError`, CLI flag and usage errors, refusals, the mode rule, `report.Search.Parallel`, MCP `workers` with `budget_notes`, `Params` | the whole of `features/g8-parallel.feature` and the `g2-mcp.feature` scenario, written first and seen red | whole suite incl. manifest/schema/alignment tests; mutants 24, 41, 44 |
| 8 | performance: segment size, group size, record layout (§2.2.1), the inline threshold per worker count, partition count | the §1.6 measurements, recorded in `perf5-confirmation.md` | **A1–A9; stop rule A3** |
| 9 | hardening: mutation campaign, 100 000+ random models, corpus and SPIN differential, `-race` full suite | the campaign itself | no survivor unexplained |
| 10 | docs: `engine-tools.md`, `workflow.md`, README option list, `steps/perf5-confirmation.md`, provenance (§9), CLI/package comments | the skill alignment tests | deployment checklist items that apply (no binary rebuild) |
| 11 | cross-review of the implementation (a separate round, with the diff) | | review log |

## 8. Non-goals, and what I will not claim

Non-goals of this stream: parallel nested DFS (ltl, progress), parallel CTL graph building, a
parallel safety part inside a run that has temporal properties, parallel `estimate`, work
stealing, lock-free tables, symmetry or compression, distributed memory, changing the
sequential default, changing `fire`/`apply`/`ir`, the engine version, the tracked binaries.

I will not claim: a speedup on violated runs, on narrow or deep models, or on models of under
about 10^5 states; that the prototype figures hold for the finished feature (they are
structural, counting only, and loaded); that 16 workers give more than 8 on this VM; that the
parallel counterexample equals the DFS one; that memory or counters of a time-truncated run are
reproducible; that the absence of a race report is a proof of race freedom (it is evidence on top
of an argument by construction); that a clean differential run proves soundness (the POR
reviews show what a generator does not cover, and §5.3 says how the generator is calibrated);
anything about SPIN's own multi-core search beyond a state-count comparison.

## 9. Open questions for the user, and provenance

1. **Mode and depth reporting.** I decided that a parallel run reports `search.mode: "bfs"` and the
   BFS depth, with `search.parallel` carrying the rest. The alternative (report `dfs` and the
   requested mode) would be misleading about `depth` and the counterexample. Say if you want the
   other.
2. **Refuse mixed runs** (safety plus ltl/ctl) in v1, rather than parallelise the safety part and
   run the temporal part sequentially. The second is an increment of perhaps a day once the core
   exists; I preferred the smaller first step.
3. **`--por` with `--workers`**: POR wins where it applies. If you would rather have a usage error
   for the combination, say so.
4. **Worker ceiling on the MCP server**: `GOMAXPROCS`, overridable by configuration. Whether a
   public server should default lower is your call.
5. **The stop rule A3** (abandon below x3 at 8 workers): confirm it is the bar you want.

Literature (the monorepo holds only a pointer): *Handbook of Model Checking*, ch. 5 (Holzmann),
§5.9 (`books-md/1clarke_edmund_handbook_of_model_checking/…md`) lists the multi-core algorithms
this stream is related to — Barnat, Brim, Ročkai (DiVinE multi-core, ATVA 2009); Holzmann,
Bošnački (a multi-core extension of SPIN, TSE 2007); Holzmann (parallelizing SPIN, SPIN 2012);
Holzmann, Joshi, Groce (swarm verification, TSE 2011) — as references [3, 16–18, 20]; the papers
themselves are **not** in the monorepo and the design above does not rest on them. The
partitioning by hash owner is the classic distributed-memory idea (Stern and Dill, 1997) and the
shared lock-free table of option (b) is the multi-core one (Laarman, van de Pol, Weber, 2010);
both are cited from memory, are not in the monorepo, and must be checked against the papers
before they go into `PROVENANCE.md`, where the new entries belong with the monorepo-only paths
labelled as such.

## 10. Reproducing the measurements

The prototypes (`P1`, `P2`, the spike driver, the profiling tests, the corpus-shape test and the
models `two1000`, `chain`) are throwaway test files added to a copy of the engine in the
scratch directory of the session; they are not part of this repository, and the plan does not
depend on them being preserved. The figures that matter are the ones re-taken in step 8 on the
real implementation.

## 11. Review log (cross-review of this plan, crossreview 2.0.0)

Three blind reviewers on the plan at commit `992f5ce` (read-only checkout of the repository):
`codex-terra-high` (Coddy, `ndlcdx/gpt-5.6-terra`, reasoning high), `coddy-gemma` (Coddy,
`ndsub/gemma-4-31b`), `claude-fable` (`claude-fable-5-1`). Verdicts as given: gemma *approve with
changes*, fable *approve with changes*, terra *needs rework* (second attempt; the first was cut off). Every finding below was checked against the
code or by running the unchanged engine; the decisions are mine. Items marked **fix** changed this
document; **rejected** and **not worth fixing** say why. The first answer of terra (its stream was truncated
after three complete findings and the start of a fourth, findings 1–4 below) was kept and used; the
retry gave a second, different set (findings 26–30). **My verdict on the plan: approve with
changes, now made.** No reviewer found a flaw in the central design (the deterministic partitioned
level-synchronous search) or in the measurements; every finding is a gap in the specification of
events, groups, budgets, errors and surfaces, each now specified, and the revised specification has
**not itself been reviewed again**: the implementation review of step 11 (and, if wanted, a short
second round on §2.7, §2.8, §3.7 and §4) is where it will be attacked.

| # | Finding (model) | Decision | Reason / evidence |
|---|---|---|---|
| 1 | The partition-major frontier is not `--bfs`'s discovery order, so "same verdicts / first verdict stands as the sequential BFS" and A12 are unattainable when an ending event and a violation sit in different branches (terra, high; fable 6, medium; gemma 1, medium) | **fix**: contract rewritten, **rejected**: the proposed "global discovery rank" | Verified: `bfs()` queues in discovery order (`explore.go:1632-1728`); the existing DFS and `--bfs` already disagree on a two-branch model (assert `violated` under DFS, `invalid-model` under `--bfs`, run on the unchanged engine). §2.7 states the order, §3.7 and A12/A12b/A12c state the contract, scenario 20 and the oracle's classification test it. Preserving the discovery rank needs a sort per level and a frontier read through a permutation, which spends the locality the design is built on and buys only which of several same-layer events wins. |
| 2 | The event key is not a total order (enabledness-probing errors, several checks on one state, the state-budget stop) (terra, high; fable 14, low) | **fix** | Key `(layer, u, q, slot, sub)` with a step counter that includes `nextEnabled` errors; the budget stop is an event in the stream; later events of the group are dropped. §2.7, scenarios 22–23, event-order unit tests, mutants 25, 26, 29, 30. |
| 3 | No hard bound on the record buffers; the group size comes from the previous group's fan-out (terra, high) | **fix** | Real hole. Record guard with a hard capacity, an overflowed group is discarded and redone with half the states, deterministically; the estimate counts the buffers, segment metadata and events. §2.8, scenario 21, mutant 27. |
| 4 | Budget-truncated reports are not worker-count independent because the first group is `64 segments` and `S` may depend on `W` (fable 2, high; terra 4, high) | **fix** | Verified in the text under review. Groups are counted in states, the first included (1 024); invariant "`S` enters no quantity that reaches a result or a stop point"; the forced-size tests vary `W` too; mutants 28, 40. |
| 5 | A decided property's expression error must not surface: sequential `checkState` skips decided properties (fable 1, high) | **fix** | Verified at `explore.go:1333` and `:1345`. Phase 3 skips properties decided at the start of the group and the merge drops an error event of a property decided earlier in key order. Scenario 24, mutant 31; the generator gets erroring property expressions. |
| 6 | Atomic walk has no bound and no cancellation check: a non-blocking atomic loop would hang (fable 3, medium) | **fix** | The sequential BFS rule is kept verbatim as a budget event (`explore.go:1697-1699`) and the stop flag is checked in the walk. Scenario 25, mutant 39. |
| 7 | A recovered worker panic would be reported by `cli` as a rejection (exit 2) and could hang the barrier (fable 4, medium) | **fix** | Verified: `runCheck` rejects any error of `Run` (`cli.go:350-359`). A distinct `explore.InternalError` (exit 1, MCP tool error); the deferred recovery arrives at the barrier. §2.6, scenario 29, mutant 38. |
| 8 | Cancellation inside phase 3 would leave a half-applied group (fable 5, medium) | **fix** | Cancellation is observed in phase 1, in the atomic walk and between phases, never in phase 3, which always completes (bounded by the record cap). §2.6, mutant 37. |
| 9 | Mutants missing for the rules whose loss gives a false `verified`: the initial state's checks, a skipped partition, a frontier taken after insertion, the depth-budget layer `D + 1` (fable 7, medium) | **fix** | Added as mutants 32–35 with scenarios 26 and 27. |
| 10 | An errored `fire` must produce no record (half-written vector) (fable 8, medium) | **fix** | Stated in §2.2; mutant 36. |
| 11 | `search.mode` is derived by the surfaces before the run; `--bfs --workers` undefined (fable 9, low) | **fix** | Verified (`cli.go:338-342`, `mcp/check.go:146-152`). One rule: `bfs` iff `--bfs` or the parallel search applied; `Params` records it after the run. §4.2. |
| 12 | The clamp note belongs in `budget_notes` (fable 10, low) | **fix** | `note` stays a constant so A11 is a byte comparison. §4.2, mutant 41. |
| 13 | "Counts of runs that stop early" omits errors (fable 11, low) | **fix** | §3.6 and scenario 12. |
| 14 | The 999 999 depth is a consequence of the model, not of a budget (fable 12, low) | **fix** | §1.2 reworded. |
| 15 | The fingerprint shares its top bits with the partition bits, so it carries 24 bits (fable 13, low) | **fix** | Partition = bits 56–63, fingerprint = bits 24–55. §2.3. |
| 16 | Never-claim processes are not mentioned (fable, question) | **fix** | D7 and scenario 28; the called code skips them. |
| 17 | Phase 3 must reset `Layout.Timeout` (fable, question) | **fix** | Phase 3 runs the code of `checkState`. §2.2. |
| 18 | A3 is measured against a slow DFS baseline on `counters-10-6` (fable, question) | **fix** | The one-worker-normalised figures are stated next to it (66–75% efficiency). §1.6. |
| 19 | Steps 3–6 cite CLI scenarios whose flag arrives in step 7 (fable, question) | **fix** | §7 says so and gates those steps by package tests. |
| 20 | `Levels` is not in the report (fable, question) | **fix** | A10 says it is an internal check. |
| 21 | The parent sentinel needs a value; `NewLayout` might write into the model (fable, questions) | **fix** / **rejected** | Sentinel `0xFFFFFFFF`, and the partition capacity is 2^24 - 1 so it is never an id. `NewLayout`/`place` write nothing into the model (read), and the compiles run on the coordinator before the workers start. |
| 22 | The choice among several shortest counterexamples is not `--bfs`'s (gemma 1) | **fix** | One sentence in §2.7 and §3.4. |
| 23 | `depth` changes meaning for a user comparing runs (gemma 2, low) | **not worth fixing** | Already stated (§3.3, §3.6, `search.mode`, the note); nothing more can be said than that it is the BFS depth. |
| 24 | A time-budget stop is at a group boundary (gemma 3, low) | **fix** | §3.5 and §2.6 say so; the report's reason stays the sequential sentence. |
| 25 | State-id capacity versus a skewed hash; memory estimate excluding per-worker scratch (gemma, questions) | answered | Balance measured on four models (§1.4: within 13% of the mean); `worker_bytes_est` carries the excluded part. |
| 26 | A `fire` can return both a failed assert and an error; the sequential code handles the error first, so the plan's equal-key events are wrong (terra attempt 2, critical) | **fix** | Verified: `fire` returns `(failed, err)` (`explore.go:1098-1100`) and both searches test `err` first (`:1526-1531`, `:1677-1688`). My own wording in an intermediate revision ("an error, then its failed assert") was also wrong. §2.7: one outcome event per fire, an error suppresses the assert; scenario 30, mutant 42, unit test. |
| 27 | Group boundaries depend on the worker count (terra attempt 2, high) | **fix** | Same as 4. |
| 28 | `Options.Mode` serves POR eligibility and the reported mode, so POR precedence and `search.mode: "bfs"` conflict (terra attempt 2, high) | **fix** | Verified (`explore.go:316-319`, `cli.go:338-342`, `mcp/check.go:146-152`). `Workers` never changes `Options.Mode`; the sequence POR analysis, then the choice, then `Result.Parallel`, then the surfaces compute the reported mode. §4.1.1, scenarios 16 and 32 (CLI and MCP, all four combinations), mutant 44. |
| 29 | Events are retained until the barrier: a model that fails an assert on every transition makes an event per transition, outside the estimate (terra attempt 2, medium) | **fix** | Bounded retention: one event per kind and property per worker, with the argument that nothing is lost; counted in `worker_bytes_est`. §2.7, scenario 31, mutant 43. |
| 30 | The initial state's checks have no step in the algorithm (terra attempt 2, medium; fable 7a) | **fix** | The table listed them but phase 3 only checks inserted records. Explicit single-threaded step 0 (`explore.go:1632-1645`); scenario 26 covers an initial violation, reach witness, watch truth and error. |
