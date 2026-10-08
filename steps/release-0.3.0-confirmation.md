# Release 0.3.0 — build record

Layer: G6 (packaging). The record of what was built from which commit, how it was checked, and what was
and was not executed. The content of the release is in `../RELEASE-NOTES-0.3.0.md` and
`integration-0.3.0-notes.md`.

## The build

- Source commit (`BUILD-INFO.json`, `source_commit`): `4a6fbb11f9350a627b4566900a97b53976655e27`, the commit just
  before the one that commits `engine/bin/`.
- Command: `./build.sh --version 0.3.0 --source-commit 4a6fbb11f9350a627b4566900a97b53976655e27 --verify-repro`
  from `model-check-plugin/`, with Go 1.26.8 first on `PATH` (the script sets `GOTOOLCHAIN=local` and requires
  `go1.26.8` since `4a6fbb1`; the machine's own Go is 1.25.0, so the toolchain
  `golang.org/toolchain@v0.0.1-go1.26.8.linux-amd64` of the module cache was used), `nice` and `GOMAXPROCS=2` on a
  shared machine after `assets/wait-for-capacity.sh` answered 0.
- **History of the toolchain.** The first 0.3.0 build (source `a39fcf3`, Go 1.26.1) was replaced before anything was
  published: `govulncheck` found 13 standard-library vulnerabilities reachable from the engine in a 1.26.1 build
  (`net/http`, `crypto/tls`, `crypto/x509`, `net/url`, `encoding/asn1`, `net`, `net/textproto`; fixed in 1.26.2 to
  1.26.6) and one module vulnerability, `golang.org/x/sys` v0.41.0 (GO-2026-5024, windows only, not called). `go.mod`
  now says `go 1.26.8` and `golang.org/x/sys` is v0.44.0.
- Vulnerability scan of the result: `govulncheck` (v1.8.0) on the source tree ("No vulnerabilities found") and in
  binary mode on `mcd-linux-amd64`, `mcd-linux-arm64`, `mcd-darwin-amd64`, `mcd-darwin-arm64` and
  `mcd-windows-amd64.exe` (no vulnerabilities in any).
- Result: five platform binaries (`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
  `windows/amd64`), the POSIX wrapper `mcd`, `mcd.cmd`, `mcd.exe`; `SHA256SUMS` and `BUILD-INFO.json`
  (`go_version` go1.26.8, `version` 0.3.0, `version_stamp` overlay). "reproducible (two builds agree on every
  generated artifact)". `sha256sum -c SHA256SUMS`: eight files OK. `engine/bin/mcd version`:
  `mcd 0.3.0 (ir mcd-ir/1, report mcd-report/1)`.
- The version is also in `report.EngineVersion`, `plugin.json`, `.claude-plugin/plugin.json` and the engine record of
  three goldens (`nrpr-unread`, `petrinet2`, `par-default-bench-indep-3`); the build examples in `README.md` and
  `AGENTS.md` name 0.3.0.

## What was executed

| Platform | Built | Hashed | Executed |
|---|---|---|---|
| linux/amd64 | yes | yes | yes: `mcd version`; `mcd check` on the speed-up models on this machine and on a 32-core host (version 0.3.0 binary); the test suite below |
| linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 | yes | yes | no |

## Tests on the release tree

`go test -p 2 -count=1 ./...` from `engine/` at `4a6fbb1` plus the binaries of `9304b8b` (Go 1.26.8, `GOMAXPROCS=3`,
SPIN absent): every package `ok` (the root package with the godog scenarios 66 s, `explore` 167 s, `tools/pandiff`
and `tools/pormut` included). `go vet ./...` clean, `gofmt -l .` prints nothing, `sha256sum -c SHA256SUMS` clean.
The G6 scenarios (the platform build into a scratch directory, the manifest synchronisation, the sandbox without
Go) are part of that run. History: on the first 0.3.0 tree (Go 1.26.1, `0f0fa20`) one scenario failed because it asked
for four workers on a machine that gave the server three; it and a neighbour were changed to ask for two (`a39fcf3`;
the feature file says two CPUs are needed) and then passed under `GOMAXPROCS` 2 and 3.

SPIN 6.5.2 was installed after the build (the Debian package `6.5.2+dfsg-2build1`, also in the build container) and
the SPIN-dependent checks were run on the release tree: the root package with its `@spin` scenarios passed (310 s),
and `go test -count=1 -v ./tools/pandiff` passed in full (278 s, 14 tests): the differential corpus, sequential and
parallel (22 of 22 models compared with `pan` and the sequential engine), the partial-order fuzzers (400 models, 10
against `pan`, all agree in full and reduced), the `_nr_pr` fuzzer (600 models, 12 against `pan`: 11 agree, 1 skipped; the
six fixtures with `pan`'s state counts) and the atomic-loop fuzzer (600 models, 12 against `pan`, all agree). The new
fixtures of the review were run through `spin -a`, `gcc -O2 -DNOREDUCE` and `pan -c0`: `atomic-loop-merged-break`,
`atomic-loop-goto-head`, `dstep-loop-merged-break` and `dstep-loop-first-break` have no error, as the engine says;
`atomic-loop-first-break-leaves` has one error, as the engine says; the forward-label shape of the open finding
(`cx2`, §12.1 item 6 of the integration record) has no error in `pan` where the engine reports a violation, which confirms
that finding.

Not run for this release: the platform smoke tests of the four platforms above, the evaluation record's model-in-the-loop
evals (`evals-workspace`), a clean checkout of the public plugin repository alone (the scenarios that read
`model-check-skill-notes/` and `Promela - examples/` need the monorepo directories, as at 0.2.0).

## Not done

Nothing was pushed to or tagged in the public `model-check-plugin` repository by the build. The deployment
checklist in `README.md` (release checklist) still applies to the maintainer: review the complete tree, copy
it to the public project, check the manifests there, tag.
