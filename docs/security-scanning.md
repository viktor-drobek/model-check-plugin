# Security scanning

How the open-source scanners (trivy, semgrep, govulncheck) run on this repository, locally and in CI, where the reports land and how the gate is set.

One script is the only place the scanners are invoked, so a finding that fails the pipeline reproduces locally with one command from the repository root:

```bash
scripts/security-scan.sh                                              # every scanner, the CI gate
SEC_SCANNERS=govulncheck scripts/security-scan.sh                     # one scanner
SEC_FAIL_TRIVY=off SEC_FAIL_GOVULNCHECK=off scripts/security-scan.sh  # report only
```

The workflow `.github/workflows/security.yml` runs the same script on pull requests to `main`, on pushes to `main` and every Monday (a clean
dependency gains a CVE without any commit), and uploads trivy's and semgrep's SARIF to GitHub code scanning (Security tab) and every report as the
artifact `security-reports`.

## What runs

- **trivy**: filesystem scan: dependency vulnerabilities (`engine/go.mod`, `go.sum`), secrets, misconfiguration. Misconfiguration is report-only.
- **semgrep**: SAST with `SEMGREP_CONFIGS` (default `p/golang p/python`). Report-only until the backlog is triaged.
- **govulncheck**: the Go vulnerability database against the call graph of the module in `GOVULNCHECK_DIR` (default `engine` here). It judges the
  standard library of the toolchain the build uses, which is Go 1.26.8 in the pinned image and in `engine/go.mod`. To judge a built file:
  `govulncheck -mode=binary engine/bin/mcd-linux-amd64`.

A scanner binary on `PATH` is used when present; otherwise a pinned docker image runs (`tag@sha256:` constants at the top of the script).
To bump a scanner: pull the new image, take its digest (`docker images --digests`), update the constant, run the script and compare the summary
with the previous one, in a change of its own.

## Gate settings

| Variable | Values | Default |
|---|---|---|
| `SEC_SCANNERS` | comma list of `trivy,semgrep,govulncheck` | all |
| `SEC_FAIL_TRIVY` | `UNKNOWN` to `CRITICAL`, `off` | `CRITICAL` (vulnerabilities and secrets) |
| `SEC_FAIL_SEMGREP` | `ERROR`, `WARNING`, `INFO`, `off` | `off` |
| `SEC_FAIL_GOVULNCHECK` | `symbol`, `package`, `module`, `off` | `symbol` |
| `GOVULNCHECK_DIR` | module directory | `engine` |
| `SEC_DOCKER` | `0`, `1` | `1` |
| `SEC_DOCKER_ARGS` | extra arguments of every `docker run` | none |

Exit code 0: every requested scan ran and the gate is clean; 1: a gate failure or an operational error (missing tool, dead docker, a crashed scan);
2: a bad setting. The proxy variables of the environment are passed into the containers, and a proxy on localhost gets `--network host`: without a
network govulncheck cannot fetch the Go modules and reports type errors in packages that build.

## Reports and triage

`dist/security/` (ignored by git): `trivy.json`, `trivy.sarif`, `semgrep.json`, `semgrep.sarif`, `govulncheck.json`, `summary.md`. The SARIF files
leave secret findings out. An accepted finding goes in `.trivyignore.yaml` with a statement and an expiry, or in an inline `nosemgrep` with a reason;
path exclusions are in `trivy.yaml` and `.semgrepignore` and must never cover product code.
