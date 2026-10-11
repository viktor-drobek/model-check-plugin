# Release 0.3.2 build record

Layer: G6 packaging. This record identifies the source, generated artifacts, verification, and executed platform coverage for 0.3.2. The change summary and verification boundaries are in `../RELEASE-NOTES-0.3.2.md`.

## Source and build

- Release source commit: `974d5afdb173a8f9d5f17cd28a95ee960ae928b3`.
- Version: `0.3.2` in both plugin manifests, `report.EngineVersion`, the three report goldens, and generated build metadata.
- Toolchain: Go 1.26.8, Python 3, POSIX shell, and `GOMAXPROCS=2`. The capacity gate passed before the build and verified 500 MiB of writable temporary space outside the packaged plugin tree.
- Command: `./build.sh --version 0.3.2 --source-commit 974d5afdb173a8f9d5f17cd28a95ee960ae928b3 --verify-repro` from `model-check-plugin/`.
- Result: five binaries, POSIX and Windows wrappers, `SHA256SUMS`, and `BUILD-INFO.json`. A second independent build matched every generated file byte for byte. The build stamped the version through the report overlay.

## Artifact checksums

`engine/bin/SHA256SUMS` validates these eight generated files:

```text
2016593c7f711e50296510350dcb46c4b946d56219868599050051ddac6dc463  mcd
0bebafa6be6fb91a815fa974eac858c08d0b37a563549b9d8764c323eca267f2  mcd-darwin-amd64
70b64b49dc47dc2ed3a9d422b8a20aeaedc281ebcd0890f9cbe16ca02a9659ce  mcd-darwin-arm64
a5b298721fe1c5b5dc09cc508757e403865e87b1b55270b23955e896ed54187d  mcd-linux-amd64
cbd0797bda4644b6de7add810951ba40a7c081d5d32d5e4e6f92fc6f8d65c1e0  mcd-linux-arm64
fd2157f68702362216da6578822d3048691bf3074655399b3a106974bca4dfd3  mcd-windows-amd64.exe
8cdaacbb9655dc5d2cb2d0b911003befea7eaec75629ace03971b9ce77e6516d  mcd.cmd
fd2157f68702362216da6578822d3048691bf3074655399b3a106974bca4dfd3  mcd.exe
```

The build ran `sha256sum -c SHA256SUMS` successfully. `engine/bin/mcd version` reported `mcd 0.3.2 (ir mcd-ir/1, report mcd-report/1)`.

## Validation

- The complete `go test ./...` suite passed on the release source with a disk-backed temporary directory outside `model-check-plugin`; this includes `tools/pandiff` and 855 Gherkin scenarios.
- `go vet ./...` and `test -z "$(gofmt -l .)"` passed on the same source.
- The post-build `go test . -run TestFeatures` package harness passed against the generated 0.3.2 plugin tree. It covers the packaged Go-less MCP sandbox, manifest synchronization, and public-tree path checks.
- Two independent Linux hosts passed the resource gate before each final model-checking run. The POR pipeline verified `deadlock` and `assert` exhaustively. The rendezvous weak-fairness reproduction exhaustively violated `<>(x == 1)`, which matches the documented `pan -f` partner convention.

## Platform coverage

| Platform | Built | Hashed | Executed locally |
| --- | --- | --- | --- |
| linux/amd64 | yes | yes | yes, `mcd version` and the POSIX wrapper |
| linux/arm64 | yes | yes | no |
| darwin/amd64 | yes | yes | no |
| darwin/arm64 | yes | yes | no |
| windows/amd64 | yes | yes | no |

No public repository push, tag, container-image publication, or registry operation was performed by this build record.
