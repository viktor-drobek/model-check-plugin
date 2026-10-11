# model-check plugin 0.3.4 release notes

0.3.4 is a security-maintenance patch release. It replaces the non-cryptographic random source in the engine scratch-space probe with `crypto/rand` and preserves the existing engine, IR, report, MCP, and plugin-manifest schemas.

## Security remediation

The scratch-space probe allocates and writes a buffer to verify that `TMPDIR` can hold the requested temporary space before a test or comparison run starts. The probe now fills that buffer with `crypto/rand.Read` instead of `math/rand/v2`.

This resolves the source condition reported by Semgrep code-scanning alert 23. The probe still writes the same amount of data, removes its probe file, and returns an error if its random source cannot fill the buffer.

## Verification boundary

The S1 security feature now requires `crypto/rand` and rejects `math/rand` in the production `internal/tmpspace` probe source. The focused tmpspace unit tests, the full Gherkin suite, the full Go validation, reproducible platform-artifact build, and the GitHub Actions security scan are rerun for the published release. No model semantics, property language, resource budget policy, or supported Promela subset changes in this release.
