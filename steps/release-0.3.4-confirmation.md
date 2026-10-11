# Release 0.3.4 build record

Layer: G6 packaging. This record identifies the source, generated artifacts, verification, and executed platform coverage for 0.3.4. The change summary and security rationale are in `../RELEASE-NOTES-0.3.4.md`.

## Source and build

- Release source commit: `3d7f6e314c5dd7ebabe4f2ff9126cd03cd68fc71`.
- Version: `0.3.4` in both plugin manifests, `report.EngineVersion`, the three report goldens, and generated build metadata.
- Toolchain: Go 1.26.9, Python 3, POSIX shell, and `GOMAXPROCS=2`. The capacity gate passed before the build and verified 1 GiB of writable temporary space outside the packaged plugin tree.
- Command: `./build.sh --version 0.3.4 --source-commit 3d7f6e314c5dd7ebabe4f2ff9126cd03cd68fc71 --verify-repro` from `model-check-plugin/`.
- Result: five binaries, POSIX and Windows wrappers, `SHA256SUMS`, and `BUILD-INFO.json`. A second independent build matched every generated file byte for byte.

## Artifact checksums

`engine/bin/SHA256SUMS` validates these eight generated files:

```text
2016593c7f711e50296510350dcb46c4b946d56219868599050051ddac6dc463  mcd
d4b303445f92670e604ca7e4879794eda6367ac878513a937a56e1abf352dc38  mcd-darwin-amd64
161559869a2666f3f9353f72ab2d1a95604871cb4a824ec2e056a040ec5b1b06  mcd-darwin-arm64
ba19212c5a54e193af255c6631a2a4b881bf86bc047f574fa3c4ba08702aaa69  mcd-linux-amd64
f19adca9e0dc8677202254935275ddf35ea813f9679b585feec1f1bfe6028fd6  mcd-linux-arm64
10bd59c1bb1c12f11eeec13912933aa2ee0de7162eb4e6d76d90955fd7512c28  mcd-windows-amd64.exe
8cdaacbb9655dc5d2cb2d0b911003befea7eaec75629ace03971b9ce77e6516d  mcd.cmd
10bd59c1bb1c12f11eeec13912933aa2ee0de7162eb4e6d76d90955fd7512c28  mcd.exe
```

The build ran `sha256sum -c SHA256SUMS` successfully. `engine/bin/mcd version` reported `mcd 0.3.4 (ir mcd-ir/1, report mcd-report/1)`.

## Validation

- The new S1 scenario failed before the change because the production probe did not mention `crypto/rand`; it passed after the change with the complete Gherkin suite.
- The focused `go test ./internal/tmpspace` package passed under Go 1.26.9.
- The complete `go test ./...` suite, `go vet ./...`, and `test -z "$(gofmt -l .)"` passed under the same toolchain.
- The reproducible build, checksum validation, and Linux host-binary version check passed.
- The local security gate was started after a capacity check but stopped when its pull of the pinned Go image made no progress for almost 16 minutes. The published GitHub Actions Security scan is the authoritative Trivy, Semgrep, and govulncheck gate for this release.

## Platform coverage

| Platform | Built | Hashed | Executed locally |
| --- | --- | --- | --- |
| linux/amd64 | yes | yes | yes, `mcd version` and the POSIX wrapper |
| linux/arm64 | yes | yes | no |
| darwin/amd64 | yes | yes | no |
| darwin/arm64 | yes | yes | no |
| windows/amd64 | yes | yes | no |

No public repository push, tag, container-image publication, or registry operation was performed by this build record.
