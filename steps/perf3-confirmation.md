# Performance plan, step 3 — one word per slot in the visited table

Layer: G0 (`explore`). Follows `perf1-confirmation.md` (chunked arena) and
`perf2-confirmation.md` (partial-order reduction). Protocol:
`BUILD-PROTOCOL.md` step 6.

## What changed

A slot of the exact visited set was a 64-bit hash in one array and a 32-bit
index in another: 12 bytes, and two cache lines touched to find a state that is
already stored (the hash to compare, then the index to reach the vector), which
is what most transitions of a search do. The slot is now one 64-bit word: the
high 32 bits of the hash (a fingerprint) over the index of the vector plus one,
so that zero is an empty slot. A probe reads one word and compares the whole
vector only when the fingerprint matches, so there are still no false positives:
two vectors sharing a fingerprint (one probe in 2^32) are told apart by the
vector. The table costs 8 bytes a slot instead of 12.

The word keeps no low bits of the hash, so growing the table finds the positions
by hashing the vectors again, in index order, which reads the arena
sequentially: one hash per state per doubling, nanoseconds against the cache
miss of a random write into the new table.

A word can number at most 2^32-2 states. Past that the index would spill into
the fingerprint and the search would answer wrongly without a sign of it, so the
set stops with a message instead (the old `uint32` index wrapped silently in the
same way). It takes about 70 GB of 16-byte vectors to get there.

## Behaviour pinned (tests first)

- `TestCompactTableCostsEightBytesASlot`: a fresh table of 1 024 slots is 8 192
  bytes, a table sized for 100 000 states 2^18 × 8. Red before: 12 288.
- `TestCompactEqualFingerprintsAreToldApartByTheVector`: through a seam
  (`addHashed`/`hasHashed`) that stores vectors under hashes the test chose,
  the same hash, the same fingerprint at another position and another
  fingerprint at the same position are kept apart, found at their own index and
  never mistaken for an absent vector. Random data cannot reach this case, so it
  is built.
- `TestCompactRefusesMoreStatesThanAWordCanIndex`.
- The existing cross-check against the map reference on random sequences
  (vector lengths 0 to above a chunk, across table growths) passes unchanged.

States, transitions, verdicts, counterexamples and report text are unchanged;
`memory_bytes_est` falls (golden `petrinet2.report.json` 38 424 → 30 232: the
2 048-slot table of a default set is 8 192 bytes lighter).

## Results

Same machine as before (Xeon Gold 6154, Go 1.26.1). Medians of 7 interleaved
runs of the previous commit's test binary and the new one; wall times vary about
±10%, so only the large run is more than a hint.

| Benchmark | Time | Allocated |
|---|---:|---:|
| `Counters10x5` | 142.7 → 133.7 ms (−6.3%) | 26.4 → 24.3 MB |
| `VisitedAdd`, 1 M states | 384.4 → 360.5 ms (−6.2%) | 67.1 → 50.4 MB |
| `EngineIndepN5` | 666.6 → 679.5 ms (+1.9%, noise) | 59.8 → 43.1 MB |
| `EngineSymN6` | 809.2 → 728.5 ms (−10.0%) | 60.3 → 43.6 MB |
| `bench-indep` N=6, 8 108 731 states, three runs each | **14.9 → 12.4 s (−17%)** | resident set **461 → 362 MB (−21%)**; estimate 356 → 289 MB |

The report of the large run is identical to the previous commit's except for
`memory_bytes_est`. The gain grows with the size of the model, as expected from
a change that removes a cache miss: below a million states the table fits in
cache and there is little to remove.

## Not done

- A smaller `uint32` index and fingerprint split for tables above 2^32 slots
  (positions then overlap the fingerprint bits); out of reach in memory.
- The tracked binaries in `engine/bin/` are not rebuilt.
