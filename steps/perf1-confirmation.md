# Performance plan, step 1 — the exact visited set and the search stack

Layer: G0 (`explore`). Builds on the research of 2026-10-05 (profile of the
unreduced engine, ranking of techniques). Protocol: `BUILD-PROTOCOL.md` step 6.

## What changed

| # | Change | Where |
|---|---|---|
| a | The arena of the exact visited set is a directory of fixed 16 KiB chunks instead of one slice grown by `append`. A stored vector is never copied or moved; `Get` views stay valid for the life of the set and are capped at their own end (an `append` by a caller copies instead of overwriting the neighbour). | `explore/visited.go` |
| b | Size hint: **not derived from the budget** (see Deviations). The three identical closures `NewCompact(n, 1024)` became one `defaultVisited`. | `explore/{explore,graph,cycle}.go` |
| c | `Bytes()` counts whole chunks, the chunk directory at its capacity and both table arrays. Still a pure function of the sequence of `Add`s. | `explore/visited.go` |
| d | **Added by the measurement, not in the original plan:** the search stack (`s.stack`, `istack`) grows by doubling (`pushFrame`) instead of `append`'s quarter-steps, and `frameBytes` is 72, the real size of a frame (it was 64, so the estimate undercounted the stack by 11%). | `explore/explore.go`, `explore/cycle.go` |

## Why (d) is here

The profile before the change attributed 20% of CPU to `memmove` under
`growslice`. The first reading was "the arena". It was the stack: 94% of the
`growslice` time was in `dfs` and 6% in `compact.Add`. A depth-first stack runs
as deep as the longest path (up to the state count), and `append` copies a
large slice about five times over. Fixing only the arena would have left the
larger share.

## Behaviour pinned (BDD, red first)

- `features/g0-engine.feature`, new scenario *a complete run allocates little
  more than the memory it reports*: verdict, completeness and the 100 000-state
  count unchanged, and bytes allocated ≤ 3× `memory_bytes_est`. Red before:
  52 059 792 bytes allocated = 4.4× the 11 723 648-byte estimate.
- `explore/visited_test.go`: views stay put while the set grows; a view cannot
  reach its neighbour; compact agrees with the map reference on random
  sequences for vector lengths 1, 3, 16, 100, 5000 and one above a chunk;
  `Bytes()` is deterministic, never below the vectors, monotone, unchanged by
  duplicates; `frameBytes` equals `unsafe.Sizeof(frame{})`; the stack
  reallocates ≤ 12 times for 100 000 pushes (28 before).
  Red before: the four behavioural tests failed, the oracle/invariant tests
  passed on the old code as intended.

## Results

Same states, transitions, verdicts, counterexamples and report text; the only
report field that moves is `memory_bytes_est` (golden `petrinet2.report.json`
26 048 → 38 424 for a 20-state net: one whole chunk and the first 64-frame
stack are now counted). Diffed line by line against the old binary on the
8 108 731-state run: that field only.

Machine: Xeon Gold 6154, 16 threads, Go 1.26.1. Medians; wall-clock numbers
vary about ±10% run to run, so the engine benchmarks are interleaved A/B pairs
against a pristine `git archive HEAD` build.

| Benchmark | Time before → after | Allocated before → after |
|---|---:|---:|
| `Counters10x5` (100 000 states, deep DFS) | 147.6 → 125.0 ms (−15%) | 52.0 → 26.4 MB (−49%) |
| `VisitedAdd` 1 M states, hint 1024 | 416.7 → 375.9 ms (−10%) | 152.9 → 67.1 MB (−56%) |
| `EngineIndepN5` (579 195 states, Promela) | 765 → 692 ms (−9.6%, 8 pairs) | 102.8 → 59.8 MB (−42%) |
| `EngineSymN6` (543 076 states, Promela) | 839 → 797 ms (−5.0%, 8 pairs) | 103.9 → 60.3 MB (−42%) |

The run that motivated the step, `bench-indep.pml -D N=6 -D K=4 --sweep
--unlimited` (8 108 731 states, 45 M transitions): **resident set 1 016 MB →
461 MB (−55%)**, wall time 17.6 s → 17.2 s (within noise), estimate 394 MB →
356 MB. At this size the run is bound by cache misses in the 12-byte-slot
table, not by copying, so the time gain of step 1 fades; the memory gain does
not.

Checks: `go test ./...` green (including the SPIN differential `pandiff`
tests: state counts still agree with pan), `go vet ./...`, `gofmt -l .` empty,
`go test -race ./explore`.

## Deviations from the plan, stated

1. **No size hint from the state budget.** Measured on 1 M 16-byte states, a
   table sized for all of them up front saves 3–8% of the time and 16% of the
   bytes allocated. A budget is a ceiling, not a forecast: sizing from the
   default 1 M-state budget would put 24 MB into the estimate of a 20-state
   model. A caller that knows the size (an estimate) can call `NewCompact`.
2. **Item (d) was added** (above).
3. `memory_bytes_est` of every model changes; it is documented as an estimate
   (`engine-tools.md`). It is still not the resident size: the Go runtime's
   headroom is not counted. Resident set over estimate, measured after the
   change: 1.30× at 8.1 M states (was 2.6×), 1.57× at 1 M, 1.87× at 579 k —
   about 10 MB of runtime baseline plus a third to a half of the estimate.

## Cross-review (crossreview 2.0.0)

Reviewers, blind to each other, brief = the working-tree diff (43 KB):
`codex-terra-high` (Coddy → `ndlcdx/gpt-5.6-terra`, reasoning level `high`,
checked in the session record) and `coddy-gemma` (Coddy → `ndsub/gemma-4-31b`).
An orchestrator subagent verified every finding against the code. Quorum 2 of 2
(gemma's first attempt hit its output limit and was retried). Raw verdicts:
terra "needs rework", gemma "approve"; the orchestrator's: approve with changes.

Fixed after the review:

- `NewCompact(0, …)` looped forever for a zero-length vector (the shift loop
  never ended); the old code handled length 0. Not reachable from the engine
  (the layout always has the exclusive-control byte), but an exported function
  regressed. Reproduced as a timeout first, then bounded (`maxChunkShift`).
- `Bytes()` counted `cap(chunks)`, which depends on how a Go release rounds an
  `append`; it counts `len(chunks)` now, so a report does not change with the
  toolchain.
- The `--budget-mem-mb` documentation promised the process stays within about
  a third of the estimate; that holds only for large runs (see item 3 above).
- The randomised cross-check against the reference set covers length 0 and now
  crosses several chunk boundaries at every length.

Known and left as it was (not made worse in kind, recorded so nobody
rediscovers it):

- The budget check charges the stack at `len × 72`, the report at `cap × 72`;
  with doubling the capacity can be twice the length, so a run can finish
  complete with a reported figure above its budget. Moving the check to `cap`
  would move where budgets trip, which scenarios pin; not changed here.
- The inner DFS stack of the cycle search is a local and its capacity is not in
  `MemBytes` (older than this change).
- `sliceHeaderBytes` and `frameBytes` assume a 64-bit target, the only kind
  the build ships.

## Not done, deferred

- Packing the table slot (hash 8 B + index 4 B → one 8-byte word with a
  fingerprint) to cut `find`'s two cache misses to one and 12 → 8 B per slot;
  the large run shows the table is now the limit. Rehash would then walk the
  arena in index order.
- The tracked binaries in `engine/bin/` are **not rebuilt**; `BUILD-INFO.json`
  and `SHA256SUMS` still describe the previous source. A release build
  (`build.sh`) is the G6 step and must follow before publishing.
- Next in the plan: partial-order reduction, then parallel exploration.
