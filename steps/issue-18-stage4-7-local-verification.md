# Issue #18 — stages 4–7 local candidate verification

## Scope

The `0.4.0` candidate adds caller-byte runtime/contract adapters, conformance
dossiers, change-impact analysis, the adapter matrix and a synthetic mismatch
corpus. These are Go-library APIs; the existing CLI, seven MCP tools and v1 ledger
wire contract remain unchanged. Finite-model results do not prove implementation
conformance.

## Completed local checks

- Full engine `go test ./...`, `go vet ./...`, CLI build, package-owned Go
  formatting and whitespace checks passed after the UTF-8 and Git file-mode fixes.
- Race checks passed for the trace packages; this is not a full-module race claim.
- Blind quorum reviews of the contract, dossier and Git boundaries were completed.
  Confirmed findings were fixed or adjudicated against the documented contract.
  Strict provenance order and the documented free-text admission limitations
  remain intentional boundaries.
- The security scan exited successfully: Trivy and govulncheck had no findings;
  Semgrep reported 20 non-blocking, report-only warnings.
- Five targets were cross-built and checksummed: Linux amd64/arm64, Darwin
  amd64/arm64 and Windows amd64, including the Windows alias and wrappers.
  Repeated builds agreed on generated artifacts.
- Linux amd64 CLI/MCP smoke reported `0.4.0`, declared seven MCP tools and checked
  a synthetic model with `verified` / `exhaustive` results for assertions and
  deadlock freedom: 4 states, 3 transitions, depth 3. Budgets: 1,000 states,
  depth 100, 2,000 ms and 64 MiB. This is artifact smoke only.
- The packaging audit checked manifest synchronization, required layout, local
  documentation targets and absence of internal host names.

## Identity and outstanding gates

The checked-in [BUILD-INFO.json](../engine/bin/BUILD-INFO.json) identifies the exact
source commit used by the PR build; [SHA256SUMS](../engine/bin/SHA256SUMS) identifies
the generated binaries and wrappers. Detailed logs and review adjudications are
retained in the companion monorepository under
`docs/evidence/issue-18-stage4-7-local-verification.md`.

Only Linux amd64 was executed. Other platform binaries were **not executed**.
The required remote model confirmations were **cancelled, not passed**. Full
issue confirmation and release publication remain pending. Creating PRs does not
close the issue, tag a release or waive these gates.
