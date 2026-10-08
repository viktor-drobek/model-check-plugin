# Speed-up of the parallel search, 0.3.0

Layer: G8 (`explore/parallel*.go`, `mcd check --workers N`). Two measurements of the same binary
source on two machines, each taken while the machine was otherwise idle. This record says what
was measured, how, and what the numbers do and do not show; the earlier figures (taken under a
load of 3 to 5) are in `perf5-confirmation.md`.

## What was run

Models (all complete, every run of a model stored the same number of states): `bench-indep` N=5 and
N=6 K=4 (579 195 and 8 108 731 states), `par-two` N=1000 (4 010 007), `par-chain` N=1 000 000
(2 000 003), `CH15/client_server` (191 200), `counters-10-6`
(1 000 000). `--sweep --no-timing`, state, depth and memory budgets lifted (200 000 000 states).
Comparison baselines: the default depth-first search (`dfs`) and the breadth-first search (`bfs`,
`--bfs`); `par w` is `--workers w`, which is a breadth-first search by `w` workers.

| | Local (this machine) | `ml` (a 32-core host) |
|---|---|---|
| CPU | Intel Xeon Gold 6154, 16 cores, 2 NUMA nodes | Intel Xeon Gold 6154, 32 cores, 4 NUMA nodes |
| Memory | 30 GB | 60 GB |
| Binary | `go test ./explore -run TestTuneParallel` of the tree at `bfefda4` (the parallel code is unchanged since) | `engine/bin/mcd-linux-amd64` of the release build, version 0.3.0, run as `mcd check` |
| Harness | the tuning harness: 3 repetitions, best and mean wall time | `steps/release-0.3.0-speedup-ml.tsv` (raw rows), 3 repetitions per cell, median |
| Load | the job started when CPU use was 3% (the load average, 2.4, was the decay of earlier work); nothing else of the author's ran | before every run a script waited until CPU use was at most 10%; the host had no other heavy job (load average 0.05 before the series); the `load1` column of the table includes the load of the series itself |
| Pinning | none | none, and one series pinned to NUMA node 0 (`numactl --cpunodebind=0 --membind=0`, 8 cores) |

## Local, best of 3 (wall seconds; speed-up against the default depth-first search)

| model | dfs | w=1 | w=2 | w=4 | w=8 | w=16 |
|---|---:|---:|---:|---:|---:|---:|
| indep5 | 0.534 | 0.651 (x0.8) | 0.398 (x1.3) | 0.256 (x2.1) | 0.137 (x3.9) | 0.134 (x4.0) |
| indep6 | 10.608 | 11.773 (x0.9) | 6.939 (x1.5) | 3.730 (x2.8) | 2.012 (x5.3) | 1.567 (x6.8) |
| two | 1.943 | 2.487 (x0.8) | 2.349 (x0.8) | 2.143 (x0.9) | 2.221 (x0.9) | 2.147 (x0.9) |
| chain | 1.125 | 1.215 (x0.9) | 1.254 (x0.9) | 1.328 (x0.8) | 1.181 (x1.0) | 1.230 (x0.9) |
| client | 0.336 | 0.416 (x0.8) | 0.232 (x1.4) | 0.128 (x2.6) | 0.088 (x3.8) | 0.101 (x3.3) |
| counters | 1.487 | 1.292 (x1.2) | 0.938 (x1.6) | 0.505 (x2.9) | 0.427 (x3.5) | 0.221 (x6.7) |

## `ml`, median of 3 (wall seconds; speed-up against the default depth-first search, and against `--bfs`)

| model | dfs | bfs | w=1 | w=2 | w=4 | w=8 | w=16 | w=32 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| indep5 | 0.686 | 1.436 | 0.802 (x0.9) | 0.532 (x1.3) | 0.323 (x2.1) | 0.238 (x2.9) | 0.218 (x3.2) | 0.229 (x3.0) |
| indep6 | 15.801 | 28.715 | 14.438 (x1.1) | 13.110 (x1.2) | 5.426 (x2.9) | 3.205 (x4.9) | 2.259 (x7.0) | 2.226 (x7.1) |
| two | 2.870 | 6.287 | 2.746 (x1.0) | 3.511 (x0.8) | 3.086 (x0.9) | 3.333 (x0.9) | 2.899 (x1.0) | 3.016 (x1.0) |
| chain | 1.826 | 2.278 | 1.627 (x1.1) | 1.581 (x1.2) | 1.601 (x1.1) | 1.617 (x1.1) | 1.631 (x1.1) | 1.688 (x1.1) |
| client | 0.445 | 0.655 | 0.468 (x1.0) | 0.338 (x1.3) | 0.245 (x1.8) | 0.142 (x3.1) | 0.163 (x2.7) | 0.142 (x3.1) |
| counters | 2.362 | 3.204 | 1.620 (x1.5) | 0.983 (x2.4) | 0.624 (x3.8) | 0.436 (x5.4) | 0.349 (x6.8) | 0.365 (x6.5) |

Against the breadth-first search with one worker's worth of the same algorithm (`--bfs`), which is
the like-for-like baseline of the parallel search (it is breadth-first too), `w=8` gives x9.0 on
`indep6`, x7.4 on `counters`, x4.6 on `client`, x6.0 on `indep5`.

Pinned to NUMA node 0 on `ml` (8 cores, median of 3), speed-up against the default depth-first search:
`indep6` w=1 x1.0, w=2 x1.5, w=4 x3.2, w=8 x5.0 (3.175 s); `counters` w=1 x1.5, w=2 x2.6, w=4 x4.8, w=8 x6.7
(0.352 s); `two` w=4 x1.2, w=8 x1.4 (2.123 s). On this host placing the 8 workers on one node did
not hurt and, on `counters` and `two`, was faster than 8 unpinned workers (x6.7 against x5.4, x1.4
against x0.9). The 4 workers of a pinned run beat the 4 of an unpinned one on `indep6` (x3.2 against
x2.9) too.

## Reading

- **The stop rule of `perf5-confirmation.md`** (abandon the parallel search if it gives less than x3
  at 8 workers on `bench-indep` N=6 or `counters-10-6` at a load of 2 or below) is **not triggered** on
  either machine, against the default search: `indep6` x5.3 (local) and x4.9 (`ml`), `counters` x3.5
  (local, best of 3) and x5.4 (`ml`). The criterion was taken at a load of 2 or below: the local run
  started at 3% CPU use (load average 2.4 and falling), the `ml` series ran with the host otherwise
  idle; neither is a controlled experiment, and no run was repeated at a second load.
- **It pays on wide graphs and not on narrow ones**, as the README says. `two` (two counters, one
  state per layer at the frontier of the product) and `chain` do not speed up at any worker count (x0.8
  to x1.2): the layers are narrow.
- **One worker is not free.** `w=1` is slower than the depth-first search on four of the six models
  (x0.8 to x0.9 locally): the layered search costs a constant factor; it wins only with more workers.
- **Scaling stops near 16 workers** on these models (`w=32` gains nothing over `w=16` on `ml`).
- **NUMA placement mattered less than the earlier record feared** on `ml`; that record's cores 0 and 8
  case (two workers on different nodes) was not repeated here.

## What was not measured

Memory: no resident-size measurement was taken (the report's estimate is unchanged). No run was
repeated at a load other than the one stated; no 8-worker run was taken on a loaded machine, so
nothing here says how the search behaves when the machine is busy. Three repetitions per cell is few:
the spread between the minimum and the median was up to a third on `indep6` at 2 workers (7.1 s and 10.8 s
pinned) and nothing is smoothed away in the tsv. Platforms other than linux/amd64: none.
`mc_check` over MCP was not timed (it runs the same search behind a semaphore). The earlier record's
`two1000` missed its criterion at eight workers by 7%; `two` here does not speed up at all (x0.9 at
8 workers); the difference between the two was not investigated.
