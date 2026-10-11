# Release 0.3.3 build record

Layer: G6 packaging. This record identifies the source, generated artifacts, verification, and executed platform coverage for 0.3.3. The change summary and security rationale are in `../RELEASE-NOTES-0.3.3.md`.

## Source and build

- Release source commit: `ebbb4ddcfa43987dccd5f19eb0bb79a1585ac048`.
- Version: `0.3.3` in both plugin manifests, `report.EngineVersion`, the three report goldens, and generated build metadata.
- Toolchain: Go 1.26.9, Python 3, POSIX shell, and `GOMAXPROCS=2`. The capacity gate passed before the build and verified 1 GiB of writable temporary space outside the packaged plugin tree.
- Command: `./build.sh --version 0.3.3 --source-commit ebbb4ddcfa43987dccd5f19eb0bb79a1585ac048 --verify-repro` from `model-check-plugin/`.
- Result: five binaries, POSIX and Windows wrappers, `SHA256SUMS`, and `BUILD-INFO.json`. A second independent build matched every generated file byte for byte. The build stamped the version through the report overlay.

## Artifact checksums

`engine/bin/SHA256SUMS` validates these eight generated files:

```text
2016593c7f711e50296510350dcb46c4b946d56219868599050051ddac6dc463  mcd
c00e336f8536ad81f1618acdc3368d9b063af2b679323d4ee35d24ab02023f60  mcd-darwin-amd64
db43070ff76b8a17f2e1c16fa0a42ecc9985790bee150a604a86a5e5c957dea7  mcd-darwin-arm64
efb79f50fc21b5d4a7785666ff4f41920801e11740340804bbd083c96b4839e3  mcd-linux-amd64
f0a9920b15733173e2b5ac9cacf562a0bd2c018fc644382f1122c6aaa4d8c24d  mcd-linux-arm64
324e0c6012471aed7c50ae811af4b90e0310da27cc385332d42e28d4658325fd  mcd-windows-amd64.exe
8cdaacbb9655dc5d2cb2d0b911003befea7eaec75629ace03971b9ce77e6516d  mcd.cmd
324e0c6012471aed7c50ae811af4b90e0310da27cc385332d42e28d4658325fd  mcd.exe
```

The build ran `sha256sum -c SHA256SUMS` successfully. `engine/bin/mcd version` reported `mcd 0.3.3 (ir mcd-ir/1, report mcd-report/1)`.

## Validation

- The focused Gherkin feature harness passed under Go 1.26.9.
- The complete `go test ./...` suite passed with a disk-backed temporary directory outside `model-check-plugin`; this includes `tools/pandiff` and SPIN-dependent coverage.
- `go vet ./...` and `test -z "$(gofmt -l .)"` passed under the same toolchain.
- The capacity gate passed before the focused harness, full suite, and release build.
- The reproducible build, checksum validation, and Linux host-binary version check passed.

## Platform coverage

| Platform | Built | Hashed | Executed locally |
| --- | --- | --- | --- |
| linux/amd64 | yes | yes | yes, `mcd version` and the POSIX wrapper |
| linux/arm64 | yes | yes | no |
| darwin/amd64 | yes | yes | no |
| darwin/arm64 | yes | yes | no |
| windows/amd64 | yes | yes | no |

No public repository push, tag, container-image publication, or registry operation was performed by this build record.
