# M3.0a confirmation record

## Reviewed revision

- Base commit: `1d439dfa2b062a6467180eddb18275379b7f2241`
- Original reviewed implementation patch SHA-256: `70d447bdd1b1ebf4ce17e85ac5b916b5e252244a4be38f3f38dcaf2186980cb4`
- Final-gate patch SHA-256: `6a39004b491666b87a5350aa1551a3e6eafafb941d0b90c3d2a1aea06a661990`
- Scope: the fail-closed in-process G0 backend-contract kernel, its BDD and unit coverage, and the final coverage gates. Production source remains unchanged by this follow-up.

## Local verification

- Focused backend tests and the complete Godog feature harness passed after the new coverage was added.
- The focused `go test -race -count=1 -timeout=10m ./backend` passed after the concurrency test was strengthened to use distinct request identities.
- The final `go test -race -count=1 -timeout=30m ./...` passed with persistent scratch storage and capacity admission. The complete run finished in 26 minutes 45 seconds.
- `go vet ./...`, `test -z "$(gofmt -l .)"`, the CLI smoke build, and `mcd version` passed on the final diff.
- `scripts/security-scan.sh` passed for the reviewed kernel. This follow-up changes tests and BDD only, with no production or dependency change.

## Fresh blind review

- Claude Opus, Coddy Gemma, and Coddy Qwen 3.8 with high reasoning enabled reviewed the final uncommitted diff without seeing another review's findings.
- All three returned `approve`. The orchestrator checked questions about capability precedence, request-to-host binding, nonce-derived identities, and verified-result reasons against the contract order, existing unit coverage, and the race run. No defects or required fixes remained.
- An earlier independent three-reviewer round also found one low test-coverage gap. The final concurrency test now uses distinct nonces, asserts unique identities, and validates request-bound host facts for each request. The subsequent review found no remaining issues.

## Bounded host confirmations

Two independent, isolated confirmation environments used the full source context because the standalone public plugin checkout lacks sibling corpus fixtures and security scripts required by the Godog harness. Each environment checked that its `model-check-plugin/` tree matched the reviewed pre-patch source, verified the final patch SHA-256 before applying it, passed capacity admission, stayed below the 90% CPU and memory threshold, and removed its temporary checkout afterward.

Each environment passed:

```text
go test -count=1 -timeout=10m ./backend
go test -count=1 -timeout=10m . -run '^TestFeatures$'
go test -count=1 -timeout=10m ./explore -run '^TestPetrinet1DeadlockAndSweep$'
```

An earlier standalone public-plugin attempt was not counted as passing evidence because an incomplete checkout left the Godog fixture and security scenarios unavailable. The successful confirmations above ran against the monorepo source and the exact final patch.

## Deployment impact

This increment adds an internal G0 contract kernel only. It does not link `backend` into `mcd`, add an external backend, alter CLI or MCP schemas, or require release-binary regeneration. No downstream components are affected. The standalone plugin layout limitation noted above remains a separate packaging concern and is not changed by this follow-up.

## Acceptance decision

M3.0a final gates are complete for this reviewed diff. The full race suite, static checks, fresh blind review, and both bounded host confirmations passed. Later M3.0 increments still require protocol decoding, an isolated out-of-process adapter, cancellation and isolation enforcement, trace replay, certificate validation, independent oracle binding, model and property translation equivalence, CLI and MCP representation, contract-version propagation, and the complete M3.0 conformance-harness matrix.
