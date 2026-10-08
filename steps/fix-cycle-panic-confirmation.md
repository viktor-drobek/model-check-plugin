# Fix — a panic in the lasso of a nested-DFS acceptance cycle

Layer: G0/G4 (`explore/cycle.go`), plus a guard at each process boundary (`cli`,
`mcp`). Starts from the published 0.2.0 (`738627b`); the version is not changed,
no binary under `engine/bin` is rebuilt, and no release is cut by this change.
Protocol: `BUILD-PROTOCOL.md` step 6 (BDD first, red before green).

## What was wrong

`mcd check --promela ".../CH15/uts_model" --sweep --no-timing` ended with
`panic: runtime error: index out of range [26] with length 26` at
`explore/cycle.go:998` (`cycleSearch.stateOf`, called from `lasso`, `foundCycle`,
`run`) and exit code 2. Exit code 2 is the code of a *rejected input* with an
error document on stdout; a crash has neither. Without `--sweep` the same model
finished (`never` violated, exhaustive). The panic was reachable from `mcd
serve` as well: the Go MCP SDK (v1.8.0) does not recover a panic of a tool
handler, so the whole stdio server died without answering (shown with the 0.2.0
binary on the tiny model below: `mc_parse` answered, `mc_check` never did,
the server's exit code was 2).

## Cause

The nested search keeps two stacks. A frame of an *intermediate atomic state*
(the state in the middle of an `atomic` block, expanded but never stored, G1
rule) does not hold a visited-set index: its `idx` is negative and addresses the
state in `s.tmp` (`-idx-1`). `innerDFS` released the inner stack's entries of
`s.tmp` in a `defer` (`s.tmp = s.tmp[:tmpBase]`), and the lasso of a found cycle
is drawn *after* `innerDFS` returned, from that same inner stack (`foundCycle`
-> `lasso` -> `stateOf`). So whenever the path of the inner search from the seed
back to the outer stack crossed an atomic intermediate state, `lasso` indexed
`s.tmp` at `tmpBase` or beyond: always out of range (`tmpBase` is the number of
intermediate states of the *outer* stack, which is why the message said 26 of
26). It is a lifetime defect, not a wrong invariant of the lasso construction:
the frames, the closing position and the states they name were right, the state
store behind them had already been shortened. It could produce a crash, never a
wrong verdict or a wrong trace: an index past `len(s.tmp)` always panics.

## Why only with `--sweep` (it is not `--sweep` that causes it)

`--sweep` does not touch the cycle search. It only stops `decide` from ending the
search when the property is decided (`Options.Sweep`), so the outer DFS goes on
after the first acceptance cycle and finds more of them, each of which is
rendered (the verdict is final, the later traces are dropped, but they are
still built). Instrumented 0.2.0 build, `uts_model`:

| run | cycles found | inner path with an atomic frame |
|---|---|---|
| default | 1 (stops: all properties decided) | no |
| `--sweep` | 6 | the sixth: `istack` of 2 frames, one atomic; `len(s.tmp)` 26 |

A model whose *first* cycle crosses an atomic sequence panics without `--sweep`
too: the tiny `claim-atomic-loop.pml` below does, and so did four corpus and
fixture models under `--ltl '<>false'` (`atomic-t5.pml`, `App_C/petrinet1`,
`CH2/prodcons2.pml`, `CH5/pathfinder.pml`; siblings, below).

## Minimal reproducers (committed; the corpus model is not needed)

- `engine/testdata/promela/claim-atomic-loop.pml`: one process looping on
  `atomic { y = 1; y = 0 }`, a never claim `accept: do :: true od`. Panics with
  `index out of range [0] with length 0` with and without `--sweep`.
- `engine/testdata/promela/claim-atomic-second-cycle.pml`: the same atomic pair
  next to a plain `x = 1 - x` alternative listed first. The first cycle (on `x`)
  decides the property; the second (through the atomic pair) is reached only by
  `--sweep`. Default run: fine; `--sweep`: panic. This is the shape of
  `uts_model`.
- SPIN 6.5.2 (`spin -a -o1 -o2 -o3`, `gcc -O2 -DNOREDUCE`, `./pan -a -c0`):
  acceptance cycle on both, 1 and 2 states stored; the engine reports the same
  verdict and the same counts (`never`, `--sweep`).
- Added in the third review round (see there): `claim-atomic-starve.pml` (P
  loops on an atomic pair while Q is starved) and `claim-atomic-timeout.pml` (an
  atomic sequence that begins with `timeout`), two models from the weak-fairness
  decision study on which the published 0.2.0 panics with and without `--sweep`.

## The fix

`innerDFS` leaves the inner stack's intermediate states in `s.tmp` when it
returns a found cycle (the deferred truncation now runs only when it found
nothing or failed), and `run` truncates `s.tmp` back to its length before the
call right after `foundCycle` has drawn the lasso. Nothing else in the search
changed: the stack discipline of `s.tmp` is the same, the entries live a little
longer on one path. (Dropping the truncation altogether would have been benign
for the verdicts, since `s.tmp` is a stack and a leaked top entry is popped in
place of its owner's, but it would grow the stack and the memory estimate, so it
is released.) The change is a few lines and a comment; a `recover` or a
bounds guard at `stateOf` would have hidden the defect and drawn a lasso from
the wrong states, and was not used.

## What was verified, and against what

- **BDD, red then green.** Two scenarios in `features/g4-ltl.feature` ("an
  acceptance cycle through an atomic sequence is reported as an exact run, as pan
  -a"; "--sweep goes on after the verdict, reaches a second cycle through an
  atomic sequence, and keeps the first counterexample") and `TestLassoThroughAnAtomicSequence`
  in `explore/cycle_test.go` (default and sweep, both fixtures). Red on `5c4b16b`
  (`index out of range [0] with length 0` in every one), green on `9284567`.
  The scenarios check an exact run, not only a verdict: a replay step walks the
  report's lasso against the model (every system step is an enabled move of the
  stepper, in order from the initial state; every claim step is an edge of the
  claim out of the location the previous one reached; the loop reaches a
  location with the `accept` label; the system state after the last step is the
  state before the loop). It does not evaluate the claim's guards (the stepper
  has no claim API); those are covered by pan below. Two rows in the
  differential outline of the same feature put both fixtures against `pan -a`.
- **Verdicts with and without `--sweep` agree, and so does the trace.**
  `uts_model`: default `never` violated/exhaustive (216 states), `--sweep`
  violated/exhaustive (45 311 states); the counterexample (455 steps, loop of 15
  from step 441) is byte-identical in the two reports, and a scenario asserts the
  same on the fixture. State count against SPIN 6.5.2 (`spin -a -o1 -o2 -o3`,
  `gcc -O2 -DNOREDUCE`; the numbers below were reproduced in the second review
  round): `pan -a -c0` says 45 313 stored (48 377 visited); `pan -c0` without
  `-a` (the product without the nested search; a `-DSAFETY` build gives the
  same) says 45 311 stored, as the engine does. A `-DCHECK` build of `pan -a
  -c0` prints 48 377 `New state` lines, which is its visited count: 45 311
  plain `New state N` lines with 45 311 distinct numbers (the outer search
  numbers each product state once) and 3 066 `New state N+` lines (printed when
  a state is entered with the nested search's own mark set), and every one of
  those 3 066 names a state that the outer search had already numbered. So the
  engine's 45 311 is exactly the number of distinct product states pan reaches
  and no state is missing. **Which two of the 3 066 re-visits pan also counts
  in "stored" (45 313 against 45 311) was not traced**, so the surplus of two
  is not explained here. The G4 feature's note on pan's counter covers one
  re-insertion when the *initial* state is already accepting (CH8/fairness.pml,
  CH4/fair.pml); it was wrongly cited for this model, where the initial state
  is not accepting.
- **Replay of every counterexample the fixed engine reports** (scratch harness,
  not committed; system steps only): 612 counterexamples of `ltl` and `progress`
  properties, 544 of them lassos, over every Promela model of the corpus and the
  fixtures that the frontend accepts, in seven variants (own claim or default
  properties with and without `--sweep`; `--ltl '<>false'` with and without
  `--sweep`; the same with `--fairness weak`; `--progress` with and without weak
  fairness), state budget 300 000. All replay as runs of the model, and every
  lasso closes on the system state. (Counterexamples of an `invalid-model` end in
  the step that raises the error; the harness accepts that last step.)
- **Siblings, same root cause, all fixed.** Scan of 218 files (the SPIN corpus
  including the files without a `.pml` suffix, `testdata/promela`, `corpus2`,
  `mutate`, the IR and Petri fixtures) in those seven variants, each with the
  0.2.0 binary and the fixed one: 126 Promela models, 6 IR and 4 Petri nets are
  accepted, 82 files are rejected by a frontend (not Promela, or outside the
  subset; every one an error document with exit code 2, none a Go trace). The
  0.2.0 binary panicked in 43 runs on 9 files: `uts_model` (own claim with
  `--sweep`; `--ltl`, `--progress`), `CH2/prodcons2.pml`, `CH5/pathfinder.pml`,
  `App_C/petrinet1`, `App_C/petrinet2`, `testdata/promela/atomic-t5.pml`,
  `testdata/mutate/sample.pml` and the two new fixtures. The fixed binary: no
  panic, no exit code other than 0 and 2, in all 1 526 runs. A CTL scan (`EG true`,
  `AF false`, two formulas together; 654 runs) found panics only on the three
  models that carry a never claim (the same cause, in the claim's own cycle
  search that runs next to the CTL properties); `ctlcheck.go` and `graph.go` have
  no panic of their own.
- **SPIN on the siblings.** For the seven corpus/fixture siblings, `<>false`
  (none and weak fairness) and `--progress` (none and weak) against `pan -a` /
  `pan -l`: 27 rows agree. Four more rows ("model as written", no claim) differ
  only because `pan -a` also reports an *invalid end state* on `sample.pml`,
  `petrinet1`, `petrinet2` and `pathfinder.pml` (a deadlock verdict, not an
  acceptance cycle; with `pan -a -E` pan reports no error and the same state count
  as the engine). `uts_model` completed one row (`<>false`, no fairness: agree;
  its own claim: agree, above); the weak-fairness row did not complete: the
  harness has no budget and hit `go test`'s ten-minute limit (when it stopped
  it was building a 19 688-step trace); not investigated.
- **Default behaviour is byte-identical.** Of the 1 526 runs of the first scan,
  900 reports (exit 0) and 574 error documents (exit 2) are byte-identical to the
  0.2.0 binary's; the 43 panics are the only runs that changed, and nine more
  differed only in the counters of three heavy models (`CH14/version4`,
  `CH15/client_server.pml`, `CH5/sink_source_filter.pml`) that the 20 s
  wall-clock budget stopped at different points under load; rerun with a state
  budget instead of time the twelve variants are identical. The binary built from
  the final commit was compared again with the 0.2.0 one (`--sweep` or not, own
  claim, `--ltl '<>false'`, `--progress`; 872 runs, state budget 100 000): 523
  reports and 328 error documents identical, 18 panics fixed, and the three
  `--progress` runs that hit the 120 s clock identical under a state budget.
- **Full suite**, from `model-check-plugin/engine`, on the final commit:
  `go test -count=1 -p 2 ./...` passes (root package with the godog features 111 s,
  `tools/pandiff` against SPIN 151 s, `explore` 9 s; load average 5.2 at start),
  `go vet ./...` and `test -z "$(gofmt -l .)"` pass.

## The guards (a panic must not end as exit 2, or end the server)

Before this change nothing in the code recovered a panic (`recover()` appeared
in one test only), and the contracts say what an internal failure is: the CLI
header and `references/engine-tools.md` §2 say exit code 1 ("tool error …
internal failure"), `mcp/server.go` and `features/g2-mcp.feature` say an isError
result. A Go panic did neither: exit code 2 in the CLI and a dead server in MCP.

- (red `698c7f1`, green `0aa0dcb`.) `cli.Run` recovers, prints `mcd: internal error: <value>` and the Go stack on
  stderr and returns exit code 1. Scenario: "an internal failure is a tool error
  with its message on stderr…" (`g0-engine.feature`), driven through the real
  entry point with a standard output whose `Write` panics (the only way to make
  `cli.Run` panic without a defect of the engine).
- Each MCP tool handler is wrapped by `recoverTool` (`mcp/server.go`; it was
  `guard` in the first version, renamed because `guard.go` is the session-path
  guard): a panic becomes an isError result, `internal error in <tool>:
  <value>`, with the stack on the server's standard error, and the next call is
  served. Scenario: "a crash inside a tool is a tool error, and the server keeps
  answering" (`g2-mcp.feature`), through `Config.Promela`, the existing frontend
  seam, with a frontend that panics. Red: the test binary itself died, as the
  real server would.
- The panicked call is also an error in the session manifest (found by the
  second review; red `f2447d0`, green `1bc1809`). A handler's deferred
  `timer.end` runs while the panic unwinds, before `recoverTool` recovers, so
  it saw no error and left `outcome: "ok"`; for a panic before the handler had
  registered its defer the entry was not closed and `manifest.json` did not
  contain the call at all. `begin` now hands the call's timer to a per-call slot
  that `recoverTool` put into the context, and the recover marks that entry as
  an error (`internal error in <tool>: <value>`, no stack, the stack stays on
  stderr) and rewrites `manifest.json`; parameters and artifacts the handler
  recorded are kept. One place does it, for every tool, whether or not the
  handler's own defer ran. Tests: the crash scenario now asks `mc_manifest`
  (outcome, message, no `goroutine`, `manifest.json`), and `mcp/panic_test.go`
  injects a panic into each of the seven tools through `Server.fault` (a nil
  field in a real server, called by `begin`) and checks the client answer, the
  stack on `Server.errw` and only there, and the manifest in memory, on disk and
  as `mc_manifest` returns it; it also passes under `-race`.
- The end of the cycle search checks its `s.tmp` (found by the second review;
  `03c0a91`). Nothing asserted the balance: with `s.tmp = s.tmp[:tmpBase]`
  dropped in `run` (a real leak, one inner stack's intermediate states per cycle
  found under `--sweep`) every test of `explore`, `cli`, `ltl`, `mcp` and
  `frontend/promela` and the atomic/sweep scenarios stayed green. `balanced()` is
  an O(1) check after the loop in `run`: a search that emptied its outer stack
  must hold nothing in `s.tmp`; a stale entry is an error wrapping the new
  `explore.ErrInternal`, returned through `runCycle` and `Run`. A stopped search
  (decision or budget) still has a stack and is not checked, so a default run is
  unchanged. `ErrInternal` is a tool failure, not a rejected input as every
  other error of `Run` is: the CLI ends with exit code 1 and `mcd check:
  internal: …` on stderr, `mc_check` answers isError `internal: …` (and the
  manifest records `error`).
- Both guards change how a defect *ends* and nothing else: the value and the
  stack are in the message, `explore` is untouched (core packages do not
  convert), and the red scenarios of the cause (above) are independent of them.
  `engine-tools.md` now says so in §2, §3 and §7 (an `internal error` message
  is a defect of mcd, not a mistake in the command or the call).

## What was not verified

- The claim's guards in the replay (the stepper has no claim API): the
  scenarios replay claim *edges*; the guards are covered by the verdict and
  state count against pan, not by the replay.
- The guards only through an injected panic (a writer, a frontend, the
  `Server.fault` seam). No model panics the engine any more, so the end-to-end
  path through the built binary was shown only for the *absence* of the crash
  (the fixed binary answers the `mc_check` that killed the 0.2.0 server). The
  manifest path was additionally shown through `mcd serve` on a scratch build
  that puts the original lifetime defect back (see the second round below).
- Rendering of every cycle after the verdict under `--sweep` is still done
  (and dropped); on `uts_model` it costs nothing measurable (0.3 s), on a model
  with very many accepting cycles it is work for nothing. Not changed: an early
  skip would also have hidden this defect from every `--sweep` run.
- The balance of `s.tmp` is asserted since the second review (see "The
  guards"), but only in the sense that nothing is left over when the outer stack
  is empty; the check cannot tell that an entry in use is the right one. A dropped release
  in `innerDFS` for a search that finds nothing (the `if !found` truncation)
  survives every test, and is an equivalent mutant: that search pops its own
  intermediate frames and the truncation only matters after a budget stop,
  where the search ends anyway.
- Platforms other than linux/amd64 were not run; `engine/bin`, `SHA256SUMS`,
  `BUILD-INFO.json` and the version are untouched, so the 0.2.0 binaries still
  contain the defect until the next release is built.

## A gap found on the way (not changed here)

`engine/por_corpus_test.go` walks `*.pml` only. Thirteen of the 126 corpus and
fixture models the frontend accepts have no suffix (`App_A/example`, `App_C/ex1`,
`ex2`, `petrinet1`, `petrinet2`, `CH12/leader`, `CH14/version1..4`,
`CH15/uts_model`, `CH2/protocol`, `protocol2`), so no corpus-wide test ever ran
`uts_model`: the model that exposed this defect was outside the tests that sweep
the corpus. Widening that walk changes what a heavy test costs and what it
compares, so it is left for a decision of its own.

## Cross-review, second round (HEAD `8a1b69e`): approve with changes

The record had no cross-review section before this one, and describes no
earlier round. Verdict of the round: **approve with changes**. The lifetime fix
itself was found correct: no model was found on which the fixed engine panics,
draws a lasso that does not replay, leaks or grows `s.tmp`, differs between
`--sweep` and the default run, or on which a tool call still kills `mcd serve`.
What the orchestrator ran for that: about 2 000 runs of generated models with an
invariant harness (6.7 million invariant checks), 562 corpus lassos replayed
strictly, the same harness on the old lifetime (it panics on 33 runs), eight of
the nine files that panic on 0.2.0 reproduced independently, and a 790-run CLI
diff of the 0.2.0 binary against the fixed one in which every difference was
explained.

The five findings, and what was done (each verified against the code first):

1. **MEDIUM, a panicked call is `outcome: "ok"` in the manifest** (confirmed on
   a scratch build with the original lifetime defect put back and the guard
   kept: `mc_check` answers isError, `mc_manifest` shows the call as `ok`). Red
   `f2447d0`, green `1bc1809`; see "The guards". One deviation from the
   suggested remedy: the recovery marks the manifest entry in `recoverTool`
   (through a per-call slot in the context) instead of making the deferred
   closure of each of the seven handlers panic-aware. It is one shared helper
   too, but a handler cannot forget it, and it also covers a panic before the
   handler registered its own defer, which the per-handler closure cannot (the
   entry is then not closed at all, and `manifest.json` lacks the call).
2. **LOW, no troubleshooting text** for the new failure mode: an entry in the
   README's Troubleshooting (`903ae00`), and `engine-tools.md` now also names
   the `internal:` form of the message.
3. **LOW, the exit-code header of `g0-engine.feature`** did not say that exit
   code 1 is also an internal failure (`903ae00`).
4. **LOW, nothing asserts the `s.tmp` balance** (`03c0a91`, see "The guards").
   Mutation record, same build and tests before and after the check:
   - M1, the truncation dropped in `run` (`s.tmp = s.tmp[:tmpBase]`): before the
     check, `go test` of `explore`, `frontend/promela`, `ltl` and `cli` and the
     g4 atomic/sweep scenarios all pass (the leak is invisible); after the
     check, `TestLassoThroughAnAtomicSequence` fails in both `--sweep` subtests
     with `internal: the cycle search ended with an empty stack but still holds
     1 intermediate atomic state(s)`, and the g4 scenario "--sweep goes on
     after the verdict, reaches a second cycle through an atomic sequence …"
     fails. The two non-sweep subtests stay green by construction (a search
     that stopped on a decision still has a stack).
   - M3, the `if !found` release in `innerDFS` dropped: survives everything,
     before and after; equivalent (see "What was not verified").
   - The mapping of `ErrInternal` was shown on scratch builds only, through
     the real binary: a build with M1 gives `mcd check` exit code 1, empty
     stdout, `mcd check: internal: the cycle search ended with an empty stack
     …` for `claim-atomic-loop.pml --sweep`, where the fixed binary gives a
     report (exit 0); a build whose check always fires gives isError
     `internal: …` from `mc_check` through `mcd serve`, the call recorded as
     `error` in the manifest, and the next call (`mc_manifest`) served. There is
     no automated test of the CLI/MCP mapping: no input makes the fixed engine
     leak, and the seam to fake it would be a production hook for one error.
5. **LOW (record), the claim about pan's surplus of two**: reproduced and
   rewritten in the state-count bullet above (numbers reproduced: 45 313 stored
   and 48 377 visited with `-a`, 45 311 without, 45 311 distinct plain `New
   state` lines and 3 066 `N+` lines that name states already numbered); which
   two of the 3 066 pan also counts as stored is untraced.

Checked after the changes, that nothing which worked has changed (scratch
builds of `738627b` = 0.2.0, of `8a1b69e`, and of this branch's HEAD; `-no-timing`;
a state budget of 20 000 and a depth budget of 20 000 and `timeout` on every run):

- CLI: 235 files (the corpus, the fixtures, `corpus2`, `mutate`, the Petri and IR
  fixtures; `--promela`, `--petri` or `--ir`) in six variants (own claim or
  default properties, each with and without `--sweep`; `--ltl '<>false'` with
  and without `--sweep`; `--progress`; `--ltl '<>false' --fairness weak`), 1 410
  cases of three binaries each. 786 reports and 594 error documents are
  identical in all three. The 0.2.0 binary panics in 27 cases, on nine files
  (`atomic-t5.pml`, `claim-atomic-loop.pml`, `claim-atomic-second-cycle.pml`,
  `CH5/pathfinder.pml`, `App_C/petrinet1`, `petrinet2`, `CH2/prodcons2.pml`,
  `mutate/sample.pml`, `CH15/uts_model`); there the binaries of `8a1b69e` and of
  this branch agree with each other. The binaries of `8a1b69e` and of this
  branch never differ, except in three runs of the heavy models `CH14/version4`,
  `CH15/client_server.pml` and `CH5/sink_source_filter.pml` with `--ltl
  '<>false' --sweep`, where the harness's 40 s wall-clock budget was reached at
  different points under a load average of 40 to 80 (the earlier rounds saw the
  same three models). Rerun with a state budget of 3 000 and no clock, all three
  binaries give byte-identical reports for all three models; `CH14/version4`
  at 20 000 states is identical for `8a1b69e` and this branch too. No
  difference between the 0.2.0 binary and this branch's occurred that is not a
  panic of the 0.2.0 binary.
- `mcd serve`: one session per binary and model for 14 models (five Promela
  fixtures, six corpus models, two Petri nets, one IR; `mc_parse`,
  `mc_lint_property`, `mc_simulate`, four `mc_check` variants, `mc_explain`,
  `mc_manifest`; answers normalised for session ids, paths and timings, the
  manifest compared call by call: tool, outcome, error, parameters, number of
  artifacts; budget 3 000 states). The answers and manifests of `8a1b69e` and
  this branch are identical on all 14. The 0.2.0 server dies on a call on five
  of them (`claim-atomic-loop.pml`, `atomic-t5.pml`, `prodcons2.pml`,
  `pathfinder.pml`, `App_C/petrinet1`); on the other nine it is identical too.
  So a session in which nothing panics is unchanged, manifest included. The one
  intended change, the outcome of a call that panicked, cannot occur in these
  sessions (no model panics the fixed engine) and was shown above, on the
  scratch builds that put the defect back.
- Full suite on the branch (`go test -count=1 -p 2 ./...` from
  `model-check-plugin/engine`, then `go vet ./...` and `test -z "$(gofmt -l
  .)"`): every package passes (`tools/pandiff` against SPIN 467 s,
  `explore` 14 s). The root package, which carries the godog features, hit
  `go test`'s default ten-minute limit inside the scenario "build.sh produces a
  binary for every listed platform" (it was 8 minutes into that cross-build; the
  load average of the shared machine was 60 to 200 at the time), which is not a
  failure of an assertion; rerun alone with `-timeout 45m` at a load of about 30
  it passes in 182 s. `go vet` and `gofmt` are clean, and the worktree is clean
  after the suite (the platform scenario builds into a scratch directory).
- The load of the machine was 16 to 200 during these runs (other agents), so no
  timing is claimed.

Not changed, by the findings' own judgement: `mcd serve` itself
(`cmd/mcd/serve.go`) is not under `cli.Run`'s recover, so a panic in the
transport or in `mcp.New` would still end with exit code 2 and a Go trace (the
tool handlers are covered; accepted); the Go stack of a developer build
(no `-trimpath`) carries absolute paths, to the user's own stderr, and a release
build has none; a panic prints no partial stdout (`json.Encoder.Encode` writes
once, after marshalling); the red test on 0.2.0 panics the whole test binary
instead of failing one subtest (cosmetic); the shipped 0.2.0 binaries still
contain the defect while the documentation describes the guard (intended, as
recorded above). Rejected: one reviewer's claim that a panic comes from the
`innerDFS` defer truncating `s.tmp` — a panic runs no truncation, and the engine
recovers nothing internally; the defect was the lifetime described above.

## Cross-review, third round (HEAD `91861db`): approve with changes

Verdict of the round: **approve with changes**. This is the second fix cycle of
the branch (the base of the round was `8a1b69e`). No false internal error from
`balanced()` was found: code reading of every return and break of `run`, and an
invariant-instrumented build over 5 750 CLI runs (250 newly generated models in
23 variants, 158 corpus files in 8 variants, 2 970 budget-sweeping runs, 6 hand
models) with 0 violations; with the mutation M1 the same stress fires
`ErrInternal` in 1 273 of the 5 750 runs, so the harness can see it. No panic,
no wrong or non-replaying counterexample, no tool call that kills `mcd serve` by
panic; the reports are byte-identical to `8a1b69e` in 2 500 of 2 500 generated
runs, and the pan numbers of `uts_model` reproduce (45 313 stored and 48 377
visited with `-a`, 45 311 without; the engine 45 311).

The findings, each verified against the code first, and what was done:

1. **MEDIUM, `manifest.json` was written after `s.mu` was released** (`endCall`,
   `failCall`). The SDK serves tool calls concurrently (`jsonrpc2.Async`), so a
   shorter snapshot could land over the tail of a longer one (a file that is not
   JSON) or an older snapshot could win (a call or an error outcome missing on
   disk); `failCall` added a second writer to the same file. Pre-existing in
   `endCall` (0.2.0). Red `2b51c18`, green `4374dfa`.
   - Red: `TestConcurrentCallsKeepManifestFileWhole` (`mcp/manifest_concurrent_test.go`)
     races 150 ordinary calls (`mc_lint_property`) against 150 calls that panic
     through `Server.fault` (six rounds, one session each) and compares
     `manifest.json` with the manifest in memory, call by call. On the unchanged
     code each run failed in 2 to 3 of 6 rounds ("manifest.json is not valid
     JSON: invalid character ... after top-level value"); the scenario "the
     manifest file stays whole when calls end at the same time, some of them in
     a crash" (`g2-mcp.feature`; a panicking Promela frontend through the
     public API, 300 + 300 calls) failed in 4 of 5 runs. The scenario has to read
     the file before it asks for the manifest: the next call rewrites the whole
     file and heals a torn one (a first version of the scenario, which asked
     first, passed on the unchanged code).
   - Fix: both functions write inside the critical section (`WriteFile` does not
     take `s.mu`, so there is no deadlock), through a temporary file in the
     session directory that is renamed over `manifest.json`
     (`Session.writeFileAtomic`, same guard as `WriteFile`; a failed write
     removes the temporary file and is still ignored, as documented). A reader
     sees the previous manifest or the new one, never part of either. The cost is
     that calls of one session now queue behind each other's manifest write
     (the write the call already did); the concurrent scenario takes 4 s
     instead of 1 to 2.5 s.
   - Green: the test passes in 5 runs of 6 rounds, also under `go test -race`;
     the scenario passes in 6 of 6 runs. `-race` does not see the defect (the
     race is between two file writes, not between two memory accesses: on the
     unchanged code the test also passed in the one `-race` run made), so
     `-race` here only shows that the fix adds no data race.
   - Through the real binary (`mcd serve`, stdio client, one session): 300
     concurrent `mc_lint_property` calls, the 91861db binary left an invalid
     `manifest.json` in 5 of 10 rounds, this one in 0 of 10 (301 calls recorded
     every time); 150 `mc_simulate` calls made to panic (a scratch build that
     sets the `Server.fault` seam) mixed with 150 `mc_lint_property`, 3 of 6
     rounds invalid before, 0 of 6 after, with 301 calls and 150 errors on disk
     every time. The 0.2.0 binary tears the file too (the review measured 4 of 6
     rounds).
2. **LOW, the sibling consistency check reported an engine defect as a
   verdict** (`innerDFS`: `internal: closing state %d not on the outer stack`
   still went through `s.fail`, so the CLI printed an `invalid-model` report and
   exited 0, against the text "no verdict exists for that call"). Red `c2a67f9`,
   green `7abce41`. The error now wraps `ErrInternal` and `run` returns it
   instead of failing the property; every other `innerDFS` error (a step the
   model cannot take) keeps the `fail` path. The test
   (`explore/cycle_closing_test.go`) builds the cycle search of
   `claim-atomic-loop.pml` by hand, as `runCycle` does, with a visited set that
   answers one lookup with a state the search's own bookkeeping puts on the outer
   stack but no frame holds (nothing a model can do reaches that, which is why
   a production seam was not added for it); a control run with an honest set on
   the same model finds the acceptance cycle. Red: `run` returned no error and
   the outcome was `invalid-model` "internal: closing state 1 not on the outer
   stack". Through the real binary on a scratch build whose lookup never finds
   the state (the 91861db source and this one, same mutation): the old code gives
   exit 0 and `never: invalid-model`; this one gives exit 1, nothing on stdout,
   `mcd check: internal: closing state 0 not on the outer stack (a defect of
   mcd, not a verdict on the model)`, and through `mcd serve` `mc_check` answers
   isError with the same text, the manifest records the call as `error`, and the
   next call is served. As before, there is no automated test of that CLI/MCP
   mapping (see below).
3. **LOW, the skill recognised an internal failure only by the text `internal
   error`**, while the new messages begin `internal:`. Red `802eb50` (a g3
   scenario that requires both forms in each place), green `815c939`. A search
   of the whole tree (README, features, skill, references, engine sources) for
   `internal error`, `internal:`, `internal failure` and for guidance that treats
   exit code 1 or a tool error as a bad command found these places that key on
   the text: `references/engine-tools.md` section 2 (the paragraph after the
   exit-code table) and section 7 (reading order, item 1), changed to name both
   forms; the `Server or tool error` row of the exit table in
   `references/workflow.md` (advised "retry or CLI fallback" for any tool error;
   it now says that for an `internal error` / `internal:` message neither helps,
   no verdict exists, keep the model and the call for a bug report). Already
   right: `engine-tools.md` section 2's table row and section 3 (both forms), the
   README troubleshooting entry (both forms), and `g0-engine.feature` /
   `g2-mcp.feature` (they assert `internal error`, which a panic does print).
   `SKILL.md` has no such text.
4. **Two more regression models for the lasso fix**, from the weak-fairness
   decision study: `claim-atomic-starve.pml` and `claim-atomic-timeout.pml`
   (`885b5f7`, `a7c791e`). Without fairness both the published 0.2.0 and the
   weak-fairness branch panic on them (an acceptance cycle crossing an atomic
   sequence: the defect of this branch).
   - 0.2.0 binary (`738627b`), `check --promela`, with and without `--sweep`:
     `panic: runtime error: index out of range [0] with length 0` (the starve
     model) and `[1] with length 1` (the timeout model) in
     `cycleSearch.stateOf`, exit code 2, no report.
   - This branch, with and without `--sweep`: `never` violated, exhaustive; a
     lasso that replays as a run of the model with the claim's accept location in
     the loop (starve: a loop of 3 steps from step 1, the claim's step and the two
     statements of P; timeout: a loop of 6 steps from step 1, the claim's step, P's
     `timeout` and its assignment, twice);
     the counterexample is the same with and without `--sweep` (the state count
     is not: 2 and 4 stored without `--sweep` for the starve and timeout models
     as the search stops at the first cycle, and 2 and 6 with it, which is pan's
     `-c0` count).
   - SPIN 6.5.2 (`spin -a -o1 -o2 -o3`, `gcc -O2 -DNOREDUCE`): `pan -a -c0`:
     acceptance cycle, 2 states stored (starve) and 6 (timeout); `pan -c0` without
     `-a`: 2 and 6. Two rows of the differential outline of `g4-ltl.feature`
     (none fairness, mode `a`) put both models against `pan -a`; a scenario
     outline checks the verdict, the replayed lasso and the state counts above.
     The same two models are rows of `TestLassoThroughAnAtomicSequence`.
   - Red on the base search: the scenarios run against the base `cycle.go`
     (`738627b`, through a scratch overlay, since the fix is in this branch) fail
     with `mcd: internal error: runtime error: index out of range [0] with
     length 0` (this branch's CLI guard turns the panic into exit 1); the 0.2.0
     binary itself dies as above.
   - Not pinned: with `--fairness weak` this branch agrees with `pan -a -f` on
     the starve model (`never` verified; pan: no error) and does not on the
     timeout model (`never` violated; pan: no error, 6 states). That is the
     weak-fairness defect that has its own branch; the new scenarios and rows
     therefore use no fairness. When the two branches are merged, the timeout
     model under `--fairness weak` is a case to check.

What changed and what did not, checked with scratch builds of `91861db` (the
previous HEAD of this branch) and of the final commit, `-no-timing`, a state
budget of 20 000, a depth budget of 20 000, `timeout` on every run:

- CLI: every fixture of `engine/testdata` (Promela, `mutate`, `corpus2`, Petri, IR,
  with the two new models) and a deterministic third of the SPIN corpus, 148
  files, in eight variants (own claim or default properties; `--ltl '<>false'`;
  `--progress`; `--ltl '<>false' --fairness weak`; each with and without
  `--sweep`): 1 184 cases, 2 368 runs. 1 182 cases are byte-identical (726
  reports, exit 0, and 456 error documents, exit 2); no run panics. Two differ,
  `CH14/version4` with `--ltl '<>false' --sweep` and with `--progress --sweep`, in
  the counters only, because the 40 s wall-clock budget stopped the run at
  different points (two runs of the *same* binary differ the same way: 16 832
  against 16 515 states); rerun with a state budget of 3 000 the two binaries
  give identical reports. This model was the same exception in the two earlier
  rounds.
- `mcd serve`: one session per binary and model for 16 models (seven Promela
  fixtures including the two new ones, six corpus models, two Petri nets, one IR;
  `mc_parse`, `mc_lint_property`, `mc_simulate`, `mc_check` in four variants,
  `mc_explain`, `mc_manifest`; answers normalised for session ids, paths and
  timings; budget 3 000 states). The answers, the calls of the manifest (tool,
  outcome, error, parameters, number of artifacts), the calls recorded in
  `manifest.json` on disk (equal to the manifest in memory) and the list of files
  in the session directory (no temporary file is left) are identical in all 16.
  Only the moment and the atomicity of the manifest write changed.
- Full suite, from `model-check-plugin/engine`, on the code of this round (the
  only change after it is this record): `go test -count=1 -p 2 -timeout 45m
  ./...` passes (root package with the godog features 155 s, `tools/pandiff`
  against SPIN 197 s, `explore` 10 s, `mcp` 10 s; the load average was 5 to 7 at
  the start, so no timing is claimed), `go vet ./...` and `test -z "$(gofmt -l
  .)"` pass, `go test -race ./mcp` passes (16 s), and the worktree is clean
  afterwards.

Deviations from the findings' wording, and why:

- Fix 2 has no production seam; the red test builds the search by hand in a
  new test file (`cycle_closing_test.go`), which does not touch the existing test
  file that another branch also changes. The one line of the fix and its test
  live in `cycle.go` / that file only.
- Fix 3 also changed the `Server or tool error` row of `workflow.md`, which does
  not key on the text but advised the wrong recovery for this case.
- The red of item 4 is shown against the base search (a scratch overlay of the
  base `cycle.go`) and the base binary, because the fix is already in this
  branch; the previous rounds did the same.

Not changed, by the findings' own judgement:

- No automated test of the mapping of `ErrInternal` to the CLI's exit code 1 and
  to `mc_check`'s isError: no input makes the fixed engine produce one, and the
  mapping was shown by hand on two scratch mutants (the balance check that
  always fires, and the closing-state lookup that never finds). An `ErrInternal`
  from one temporal property makes `explore.Run` return `nil, err`, so the other
  properties' verdicts of the same call are dropped; acceptable for a defect of
  the engine.
- The `Server.fault` seam sits in `begin`, before the handlers' `defer timer.end`,
  so it does not run the order "`endCall` wrote `ok`, then `failCall`
  overwrote it" that a real panic in a handler produces; that order was shown
  once on a scratch build that puts the original lifetime defect back (see the
  second round).
- A failed manifest write is ignored (as before). A panic before `begin` leaves
  no manifest entry (documented in the code; ordinary pre-`begin` errors are
  unrecorded too).
- M3 (the `if !found` release in `innerDFS`) is an equivalent mutant.
- Rejected: a nil `timer.sess` in `failCall`; a duplicated prefix in the panic
  message; a panic inside a `sess.mu` critical section hanging the wrappers.

Not verified here: a failed `rename` or `write` of the manifest (the temporary
file is removed; not injected); the unprotected access of the handlers to
`Session.seq`, `cexs` and `model` (they are read and written without `s.mu`
while calls run concurrently): a burst of 200 `mc_simulate` and 200 `mc_check`
calls on one session under `go test -race` reported nothing, so it is not a
demonstrated race, and it was not investigated further.

## Open (separate tickets, not fixed here)

- On a blocking atomic the state count is 5 against pan's 4 (the verdict agrees;
  pre-existing and identical on 0.2.0): `A: do :: atomic { x = 1; (y == 1); x =
  0 } od` with `B: do :: y = 1 - y od`. It looks like the exclusive-control byte
  that is stored while the holder is blocked. The new fixtures do not block
  inside an atomic.
- `--fairness weak` reports violations that SPIN's `pan -f` does not. Being
  fixed on its own branch (`fix/weak-fairness-null-steps`); the fairness logic
  is not touched here.
- The corpus-wide tests walk `*.pml` only (see "A gap found on the way").
- A fatal runtime error cannot be recovered: a stack overflow in `mc_parse` of a
  3 MB input with about 1.5 million nested parentheses (the threshold lies between
  100 000 and 400 000 levels) still kills `mcd serve` and the CLI with exit
  code 2. Pre-existing, not caused by this change, and the README claims only the
  panic case. Ticket: a nesting-depth limit in the Promela and LTL parsers that
  rejects such an input as a frontend rejection.
- `pan` counts two states more than the engine on `uts_model` (45 313 stored
  against 45 311, see the state-count bullet); which two is untraced; no effect
  on the verdict.
- `mcd serve` is outside `cli.Run`'s recover: a panic in the transport or in
  `mcp.New` would still end with exit code 2 and a trace.
- On `--fairness weak` the timeout model of the third round disagrees with
  `pan -a -f` (see there): to be checked when this branch and the weak-fairness
  branch meet.

## After the integration (0.3.0)

Merged with the other four branches (`integration-0.3.0-notes.md`). What stopped being true: the
timeout model under `--fairness weak` agrees with `pan -a -f` on the merged tree (`verified`, 0 errors;
`claim-atomic-starve` and `claim-atomic-timeout` are rows of the none/weak differential outline), the old
answer being the weak-fairness defect D3. The `s.tmp` balance and the panic nets were checked against
the parallel search's own panic path: a worker's panic comes back as an `*explore.InternalError` and is
answered like a handler's panic (one line, the stack on standard error); `go test -race -short ./mcp` passes.
