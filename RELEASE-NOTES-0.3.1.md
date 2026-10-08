# model-check plugin 0.3.1 — release notes

0.3.1 is a patch release of 0.3.0: **the engine is unchanged** (the same sources, the same verdicts; the binaries are rebuilt only so that
`mcd version` and the manifests say 0.3.1). It changes the build container, the CI runner image and the repository's checks, which 0.3.0 had
shipped in a state that did not work on the platform they were meant for.

## What changed

- **SPIN is built from the upstream source with `-DNXT`** (`build-container/install-spin.sh`, the tarball pinned by sha256) in the build
  container and in the runner image. The distribution package of Ubuntu 24.04 (`spin 6.5.2+dfsg-1`) is built without the temporal operator
  `X`, so `spin -f` answers `expected predicate, saw 'X'` and the differential tests against `pan` (the weak-fairness divergence scenarios,
  `TestDifferentialTriples`) fail in an image made from it. The build checks that `X` works.
- **The build container does not run as root by default** (a user `builder`, uid 1000; `--user` overrides it) and uses `WORKDIR` instead of
  `RUN cd`; the one finding that is accepted (no `HEALTHCHECK`: the images are tools, not services) is in `.trivyignore.yaml` with a reason and
  an expiry.
- **The security scan** (`scripts/security-scan.sh`, `.github/workflows/security.yml`, `docs/security-scanning.md`) is part of the tree, with
  its first runs on the public repository: trivy, semgrep and govulncheck.
- **The CI runner image** sets `RUNNER_TOOL_CACHE`, has the libc headers cgo needs, and the CI looks for the Go of `go.mod` on the runner before
  it downloads one; the CI builds the command to a scratch path instead of over the tracked `engine/bin/mcd`.
- The registry of the build container is a parameter (`--registry`, `MCD_REGISTRY`); no private address is in the tree, and a scenario keeps it so.

## What did not change

The engine, its verdicts, the partial-order reduction, the parallel search, the MCP tools and the skill are the 0.3.0 ones; the open findings and
the known limitations of `RELEASE-NOTES-0.3.0.md` stand. Only linux/amd64 was executed.
