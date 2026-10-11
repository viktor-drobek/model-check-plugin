# model-check plugin 0.3.3 release notes

0.3.3 is a security-maintenance patch release. It updates the release toolchain from Go 1.26.8 to Go 1.26.9 and preserves the existing engine, IR, report, MCP, and plugin-manifest schemas.

## Security remediation

The 0.3.2 GitHub Actions security gate found nine reachable Go standard-library vulnerabilities in binaries built with Go 1.26.8. The Go security database identifies Go 1.26.9 as the fixed toolchain version for those findings.

This release updates every release-relevant Go pin to 1.26.9:

- `engine/go.mod` and `build.sh` require Go 1.26.9.
- The build container and runner tool cache use Go 1.26.9.
- The local and packaged `security-scan.sh` scripts use the immutable `golang:1.26.9-bookworm` image digest for `govulncheck`.
- Public build and security documentation, platform binaries, `BUILD-INFO.json`, and `SHA256SUMS` identify the new release and toolchain.

## Verification boundary

The source and all shipped platform artifacts are rebuilt with Go 1.26.9. The GitHub Actions security scan is rerun against the published 0.3.3 commit. No engine behavior, model semantics, property language, resource budget policy, or supported Promela subset changes in this release.
