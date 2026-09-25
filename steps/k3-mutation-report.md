# K3 mutation results

Engine: `mcd 0.1.0-g0 (ir mcd-ir/1, report mcd-report/1)`. SPIN: `Spin Version 6.5.2 -- 6 December 2019`. Run started 2026-09-25T22:14:32Z.
Operators: drop-atomic, drop-dstep, chan-cap-minus, chan-cap-zero, invert-guard, drop-end-label, weaken-assert, swap-relop, off-by-one, drop-alternative.

| class | mutants |
|---|---|
| (i) detected | 57 |
| (ii) verdict-equivalent | 145 |
| (iii) DISAGREEMENT | 8 |
| (iv) not comparable | 54 |
| total | 264 |

Agreement rate on verdict-moving mutants = (i)/((i)+(iii)) = 57/65 = **0.877**.
Share of detected mutants = (i)/all = 57/264 = **0.216** — a different number, reported so that
the two are not confused: it measures the choice of operators at least as much as the engine.

## Per model

| model | mutants | (i) | (ii) | (iii) | (iv) | baseline |
|---|---|---|---|---|---|---|
| CH2/mutex_flaw.pml | 17 | 5 | 12 | 0 | 0 | safety: engine violated (429) / pan violated (429) |
| CH2/peterson.pml | 3 | 1 | 1 | 0 | 1 | safety: engine verified (74) / pan verified (74) |
| CH2/peterson2.pml | 6 | 0 | 6 | 0 | 0 | safety: engine violated (42) / pan violated (42) |
| CH2/prodcons.pml | 2 | 2 | 0 | 0 | 0 | safety: engine verified (6) / pan verified (6) |
| CH2/mutex.pml | 15 | 4 | 9 | 0 | 2 | safety: engine verified (190) / pan verified (190) |
| CH2/protocol | 9 | 2 | 7 | 0 | 0 | safety: engine violated (32) / pan violated (32) |
| CH2/protocol2 | 9 | 2 | 7 | 0 | 0 | safety: engine violated (25) / pan violated (25) |
| CH2/false.pml | 1 | 1 | 0 | 0 | 0 | safety: engine violated (3) / pan violated (3) |
| CH2/hello.pml | 0 | 0 | 0 | 0 | 0 | safety: engine verified (3) / pan verified (3) |
| CH2/hello2.pml | 0 | 0 | 0 | 0 | 0 | safety: engine verified (3) / pan verified (3) |
| CH3/alternatingbit.pml | 4 | 0 | 4 | 0 | 0 | safety: engine verified (8) / pan verified (8) |
| CH3/alternatingbit2.pml | 10 | 1 | 8 | 0 | 1 | safety: engine verified (16) / pan verified (16) |
| CH3/counter3.pml | 8 | 5 | 3 | 0 | 0 | safety: engine verified (3) / pan verified (3) |
| CH3/counter4.pml | 7 | 1 | 4 | 0 | 2 | safety: engine verified (3) / pan verified (3) |
| CH3/euclid.pml | 10 | 4 | 5 | 0 | 1 | safety: engine verified (10) / pan verified (10) |
| CH3/macro.pml | 0 | 0 | 0 | 0 | 0 | safety: engine violated (5) / pan violated (5) |
| CH3/mtype.pml | 0 | 0 | 0 | 0 | 0 | safety: engine verified (3) / pan verified (3) |
| CH3/rendezvous.pml | 2 | 0 | 2 | 0 | 0 | safety: engine violated (3) / pan violated (3) |
| CH3/send_recv.pml | 3 | 0 | 3 | 0 | 0 | safety: engine violated (10) / pan violated (10) |
| CH3/you_run.pml | 0 | 0 | 0 | 0 | 0 | safety: engine verified (7) / pan verified (7) |
| CH3/you_run2.pml | 4 | 0 | 4 | 0 | 0 | safety: engine verified (14) / pan verified (14) |
| CH3/counter.pml | 2 | 0 | 0 | 0 | 2 | safety: engine invalid-model (3) / pan verified (5) |
| CH3/counter2.pml | 5 | 0 | 0 | 0 | 5 | safety: engine invalid-model (256) / pan verified (258) |
| CH3/xr.pml | 11 | 0 | 0 | 0 | 11 | safety: engine invalid-model (1785) / pan verified (1792) |
| CH4/dijkstra.pml | 10 | 9 | 1 | 0 | 0 | safety: engine verified (21) / pan verified (21) |
| CH4/dijkstra_progress.pml | 10 | 1 | 1 | 8 | 0 | safety: engine verified (21) / pan verified (21); non-progress (pan -l): engine verified (39) / pan verified (39); acceptance (pan -a) []<>(len(sema) == 0): engine verified (21) / pan verified (21) |
| CH4/fair.pml | 3 | 1 | 0 | 0 | 2 | safety: engine verified (2) / pan verified (2); non-progress (pan -l): engine violated (4) / pan violated (5); acceptance (pan -a) []<>(x == 1): engine verified (3) / pan verified (3) |
| CH4/fair_accept.pml | 3 | 0 | 1 | 0 | 2 | safety: engine verified (4) / pan verified (4); acceptance (pan -a): engine violated (4) / pan violated (4) |
| CH4/true.pml | 1 | 0 | 1 | 0 | 0 | safety: engine verified (3) / pan verified (3) |
| CH4/false.pml | 1 | 1 | 0 | 0 | 0 | safety: engine violated (3) / pan violated (3) |
| CH4/prop.pml | 10 | 2 | 8 | 0 | 0 | acceptance (pan -a) []p: engine violated (5) / pan violated (3) |
| CH8/example.pml | 5 | 1 | 4 | 0 | 0 | safety: engine violated (6) / pan violated (6) |
| CH8/fairness.pml | 2 | 0 | 0 | 0 | 2 | safety: engine verified (4) / pan verified (4); acceptance (pan -a): engine violated (4) / pan violated (5) |
| CH8/trivial.pml | 6 | 5 | 0 | 0 | 1 | acceptance (pan -a): engine violated (2) / pan violated (2); acceptance (pan -a) []<>x: engine verified (3) / pan verified (3) |
| App_A/example | 12 | 0 | 8 | 0 | 4 | acceptance (pan -a): engine verified (10) / pan verified (10); acceptance (pan -a) <>[]p: engine violated (8) / pan violated (8) |
| App_C/petrinet1 | 14 | 2 | 12 | 0 | 0 | safety: engine violated (8) / pan violated (8) |
| App_C/petrinet2 | 30 | 2 | 28 | 0 | 0 | safety: engine violated (22) / pan violated (22) |
| CH2/prodcons2.pml | 7 | 2 | 3 | 0 | 2 | safety: engine verified (14) / pan verified (14) |
| CH3/inline.pml | 1 | 1 | 0 | 0 | 0 | safety: engine violated (5) / pan violated (5) |
| CH3/inline2.pml | 1 | 0 | 1 | 0 | 0 | safety: engine verified (6) / pan verified (6) |
| CH3/typedef.pml | 2 | 0 | 2 | 0 | 0 | safety: engine verified (5) / pan verified (5) |
| CH3/toggle.pml | 0 | 0 | 0 | 0 | 0 | safety: engine verified (6) / pan verified (6) |
| CH3/rendezvous2.pml | 2 | 2 | 0 | 0 | 0 | safety: engine verified (5) / pan verified (5) |
| CH3/wc.pml | 14 | 0 | 0 | 0 | 14 | safety: engine invalid-model (1) / pan violated (1) |
| CH3/splurge.pml | 1 | 0 | 0 | 0 | 1 | safety: engine inconclusive (18) / pan violated (126) |
| CH3/splurge2.pml | 1 | 0 | 0 | 0 | 1 | safety: engine inconclusive (18) / pan violated (252) |

## Disagreements (class (iii))

| mutant | operator | line | original → mutated | check | engine | pan | original |
|---|---|---|---|---|---|---|---|
| `engine/testdata/mutate/disagreements/CH4_dijkstra_progress/m001-invert-guard.pml` | invert-guard | 9 | `(count == 1)` → `!((count == 1))` | non-progress (pan -l) | **violated** | **verified** | engine verified / pan verified |
| `engine/testdata/mutate/disagreements/CH4_dijkstra_progress/m002-invert-guard.pml` | invert-guard | 11 | `(count == 0)` → `!((count == 0))` | non-progress (pan -l) | **violated** | **verified** | engine verified / pan verified |
| `engine/testdata/mutate/disagreements/CH4_dijkstra_progress/m004-off-by-one.pml` | off-by-one | 6 | `1` → `2` | non-progress (pan -l) | **violated** | **verified** | engine verified / pan verified |
| `engine/testdata/mutate/disagreements/CH4_dijkstra_progress/m005-off-by-one.pml` | off-by-one | 9 | `1` → `2` | non-progress (pan -l) | **violated** | **verified** | engine verified / pan verified |
| `engine/testdata/mutate/disagreements/CH4_dijkstra_progress/m007-off-by-one.pml` | off-by-one | 11 | `0` → `1` | non-progress (pan -l) | **violated** | **verified** | engine verified / pan verified |
| `engine/testdata/mutate/disagreements/CH4_dijkstra_progress/m008-off-by-one.pml` | off-by-one | 12 | `1` → `2` | non-progress (pan -l) | **violated** | **verified** | engine verified / pan verified |
| `engine/testdata/mutate/disagreements/CH4_dijkstra_progress/m009-drop-alternative.pml` | drop-alternative | 9 | `:: (count == 1) ->
progress:	sema!p; count = 0` → `(removed)` | non-progress (pan -l) | **violated** | **verified** | engine verified / pan verified |
| `engine/testdata/mutate/disagreements/CH4_dijkstra_progress/m010-drop-alternative.pml` | drop-alternative | 11 | `:: (count == 0) ->
		sema?v; count = 1` → `(removed)` | non-progress (pan -l) | **violated** | **verified** | engine verified / pan verified |

## Class (iv) by reason

| reason | mutants |
|---|---|
| engine inconclusive | 2 |
| engine inconclusive, pan timeout | 1 |
| engine invalid-model | 37 |
| engine invalid-model: domain overflow: x = 2 leaves [0, 1] (bool domain) in step "x = 2" | 1 |
| engine invalid-model: domain overflow: x = 2324522933 leaves [-2147483648, 2147483647] (int domain) in step "x = 3 * x + 2", pan inconclusive: pan search not completed | 1 |
| engine invalid-model: domain overflow: x = 5726623061 leaves [-2147483648, 2147483647] (int domain) in step "x = 4 * x + 1" | 1 |
| engine rejected (syntax), pan spin rejects | 3 |
| engine tool error, pan n/a | 2 |
| the original did not agree | 6 |
