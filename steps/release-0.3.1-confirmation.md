# Release 0.3.1 — build record

Layer: G6 (packaging). A patch release of 0.3.0: the engine is unchanged; the record says what was built from which commit, how it was checked,
and what was and was not executed. What changes is in `../RELEASE-NOTES-0.3.1.md`; the 0.3.0 record is `release-0.3.0-confirmation.md`.

## The build

- Source commit (`BUILD-INFO.json`, `source_commit`): `03ff666c9fb1c2893793211029cf78e068cef9c9`, the commit just before the one that commits `engine/bin/`.
- Command: `./build.sh --version 0.3.1 --source-commit 03ff666c9fb1c2893793211029cf78e068cef9c9 --verify-repro` from `model-check-plugin/`, Go 1.26.8 first
  on `PATH` (the script sets `GOTOOLCHAIN=local` and requires `go1.26.8`; the machine's own Go is 1.25.0, so the toolchain
  `golang.org/toolchain@v0.0.1-go1.26.8.linux-amd64` of the module cache was used), `nice` and `GOMAXPROCS=2`, after `assets/wait-for-capacity.sh` answered 0.
- Result: five platform binaries, the POSIX wrapper `mcd`, `mcd.cmd`, `mcd.exe`, `SHA256SUMS`, `BUILD-INFO.json` (`go_version` go1.26.8, `version` 0.3.1,
  `version_stamp` overlay); "reproducible (two builds agree on every generated artifact)". `sha256sum -c SHA256SUMS`: eight files OK.
  `engine/bin/mcd version`: `mcd 0.3.1 (ir mcd-ir/1, report mcd-report/1)`.
- The version is in `report.EngineVersion`, `plugin.json`, `.claude-plugin/plugin.json`, the engine record of three goldens, and the build examples and image tags of the documentation.
- `govulncheck` (v1.8.0) in binary mode on `mcd-linux-amd64` and `mcd-windows-amd64.exe`: no vulnerabilities (the other three binaries were scanned for 0.3.0 and the sources are the same).

## The engine is the 0.3.0 one

`git diff d6ef7e1..` (the merged 0.3.0) over `engine/` (the Go sources, `go.mod`, `go.sum`) shows no change except `report.EngineVersion` and the three goldens that embed it, and the
regression fixture `testdata/promela/weakfair-end-blocked.pml` with its scenario (the reproduction of issue 1 of the public repository, which passes on 0.2.0, 0.3.0 and 0.3.1).

## Images

The build container and the runner image were rebuilt with SPIN 6.5.2 built from the upstream source (`-DNXT`) and stored in the registry under the tags `0.3.1` and
`0.3.1-go1.26.8`; the 0.3.0 tags hold the same content. The Dockerfiles passed the scan of the public repository's pull request (trivy, semgrep, govulncheck).

## What was executed

| Platform | Built | Hashed | Executed |
|---|---|---|---|
| linux/amd64 | yes | yes | yes: `mcd version`, the test suite of the source repository's CI on its runner (Go from the image's tool cache, SPIN built with `-DNXT`) |
| linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 | yes | yes | no |

## Tests

The CI of the source repository (self-hosted runner `model-check-ci`, image built from `build-container/runner.Dockerfile`) ran `go vet`, `gofmt`, the build and
`go test -race ./...` on the 0.3.0 sources with SPIN 6.5.2: every test passes (33 min). The test run of the 0.3.1 pull request is that run's successor; its result is the
status of the pull request.

## Not done

Nothing was pushed to or tagged in the public `model-check-plugin` repository by the build.
