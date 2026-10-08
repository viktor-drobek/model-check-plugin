# model-check plugin 0.3.0 — release notes

0.3.0 is the integration of five branches on top of the published 0.2.0 plus the fixes that a review of
the merged tree found. The full records are `steps/integration-0.3.0-notes.md` (what was merged, how every
difference from 0.2.0 was attributed, the review and the campaigns), `steps/release-0.3.0-confirmation.md`
(the build) and `steps/release-0.3.0-speedup.md` (the parallel-search measurements).

## New

- **Parallel safety search** (`mcd check --workers N`, `mc_check` `workers`, `mcd serve --max-workers`): a
  breadth-first search by N workers over a partitioned visited set. Opt-in; without the flag nothing
  changes byte for byte. It pays on wide graphs (x4.9 to x5.4 at 8 workers on `bench-indep` N=6 and
  `counters-10-6` against the default search on a 32-core host; x3.5 to x5.3 on a 16-core one) and not on
  narrow or deep ones (`par-two`, `par-chain`: no gain). `steps/release-0.3.0-speedup.md`.
- **Partial-order reduction** (`--por`) also reduces atomic sequences, `run` with the process table, and models
  that read `_nr_pr`; it never changes a verdict, only what `search.reduction` reports.
- **Weak fairness** (`--fairness weak`) reworked: the null-step cycle at a state where every process is blocked
  is no longer reported; `provided` on both sides of a rendezvous, a pending `timeout`, the claim not being the
  last process, `else` in a never claim and the timeout moves of every claim edge are handled.
- **Internal failures** answer with exit code 1 and a message (0.2.0: a Go panic with exit code 2, or a dead
  `mcd serve`); a panic in a parallel worker keeps its stack out of the tool answer and the manifest;
  `manifest.json` is written atomically.
- **Build container** (`build-container/`): Go 1.26.8, SPIN 6.5.2, gcc, make, git, Python 3, govulncheck and the Go modules of the
  engine in one image, stored in a docker registry as `<registry>/model-check/build:0.3.0`; build and test with nothing
  installed on the host but docker.
- **Resources**: `skills/model-check/assets/wait-for-capacity.sh` and the instruction to look at the machine
  before a run and wait while it is above 90% (SKILL.md step 5, the README section "Running on a shared machine").

## Verdicts that differ from 0.2.0

Each is a defect of 0.2.0 on the models named in `steps/integration-0.3.0-notes.md` §6; none is a change in
a default-search verdict of a model that existed in 0.2.0 (checked on 5 712 comparisons over 476 inputs).

| What 0.2.0 answered wrongly | Direction |
|---|---|
| a guard that waits for `_nr_pr` in a model without `run` (`A: (_nr_pr == 1)`, `B: skip`) was a `deadlock`: the count never fell | false violation |
| a property over `_nr_pr` in a model whose processes keep no table was answered with a count that never changes | both; now `not-executed` with the reason |
| `atomic { do ... od }` and `d_step { do ... od }` with the loop as the first statement let go of the control at the back edge | false violation of an `assert` |
| a process whose `provided` clause was false took part in a rendezvous | missed `deadlock`, false `assert` |
| `else` in a never claim was always enabled | false violation |
| the timeout moves were enumerated for the first enabled claim edge only | missed violation |
| weak fairness: a null-step cycle was reported at any accepting state in which every process is blocked | false violation (88 of the comparisons: `progress` `violated` to `verified`) |
| weak fairness: a process at a bare `timeout` counted as blocked in every state | false violation |
| weak fairness: copy k stood for process k-1 although the claim need not be last | missed violation |
| a Go panic in the lasso of a cycle through an atomic sequence | crash, no verdict |

## Toolchain

Built with **Go 1.26.8**; `go.mod` says `go 1.26.8` and `golang.org/x/sys` is v0.44.0. A build on 1.26.1 contains 13
standard-library vulnerabilities that `govulncheck` finds reachable from the engine (`net/http`, `crypto/tls`,
`crypto/x509`, ...; fixed in 1.26.2 to 1.26.6), so the release requires 1.26.8: `build.sh` refuses any other Go.
`govulncheck` reports no vulnerability in the source or in the five binaries.

## Changes you may notice

- `atomic { do ... od }` as one option of an outer `if` or `do` (the loop head shared with another option) is
  refused with `outside-subset` (39 of 1 000 generated models; 0.2.0 accepted them with a wrong lowering).
- `mc_estimate` and `--bfs` answer `inconclusive` on an atomic loop that never ends, within a bound of 100 000
  steps (0.2.0 ran out of memory).
- A model that has a `never` claim or an `ltl` formula counts the claim differently from `pan` in `_nr_pr`
  (`testdata/spin-divergence`): unchanged, now documented and pinned.

## Known limitations

- Quadratic memory of `--bfs`, `--ctl` and `mc_estimate` on an atomic loop in which every step can also leave;
  the time budget is not looked at inside an atomic descent that stores no state (both as in 0.2.0).
- A forward `goto` into an atomic loop that shares its entry with another option bypasses the refusal above
  (as in 0.2.0, a false `assert` violation).
- Weak fairness: a starved receiver of a rendezvous counts as blocked; the SCC oracle shares the Stepper with
  the engine; a claim's `provided` clause (hand-written IR only) is ignored.
- The nesting depth of the Promela and LTL parsers is not bounded (a 1.5-million-level input overflows the stack).
- Only linux/amd64 was executed for this release; darwin/amd64, darwin/arm64, linux/arm64 and windows/amd64
  binaries were built, hashed and reproduced, not run. The SPIN-dependent checks (the `@spin` scenarios and
  `tools/pandiff` against SPIN 6.5.2) were run on the release tree and passed; the weak-fairness comparison with `pan`
  of the integration record was not repeated.

The complete list of open items is §7 and §12.4 of `steps/integration-0.3.0-notes.md`.
