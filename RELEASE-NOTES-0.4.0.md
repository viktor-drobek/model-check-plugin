# model-check plugin 0.4.0 release notes

## Release boundary

This document describes the `0.4.0` release candidate metadata and the new
trace-ledger Go-library APIs in the current tree. Local source checks, blind
review, security scanning, reproducible cross-builds and Linux amd64 artifact
smoke have passed; see the [local verification record](steps/issue-18-stage4-7-local-verification.md).
Other platform binaries were not executed, and required remote confirmations
were cancelled rather than passed. Full issue confirmation and release
publication remain pending. Version `0.3.4` remains the last verified release.

The existing `mcd` CLI, the seven MCP tools, their request/response surfaces, and
the `mcd-trace-ledger/1` contract are unchanged. No new CLI command or MCP tool is
introduced by these APIs.

## Trace-ledger Go-library APIs

The `modelcheck/trace/protocol` package provides bounded, offline, caller-byte
adapters that return canonical `mcd-trace-ledger/1` JSON or a fail-closed error:

- `AnalyzeRuntimeSpans(RuntimeSpansInput)` records explicit runtime-span
  projections with `runtime` evidence, content-addressed duplicate occurrences,
  and gaps for missing provenance or unmapped parents. It is not an OTLP/OTel
  decoder or collector and does not infer communication edges.
- `AnalyzeOpenAPI(ContractInput)` accepts the documented JSON-only OpenAPI 3.0.3
  or 3.1.0 subset and emits direct endpoint/schema/property declarations with
  `contract` evidence and explicit unmapped/reference gaps.
- `AnalyzeAsyncAPI(ContractInput)` accepts the documented JSON-only AsyncAPI 2.6.0
  or 3.0.0 subset and records direct channels, messages, and fields as contract
  declarations. It does not contact a broker or infer transport direction.
- `AnalyzeProtobuf(ContractInput)` accepts the bounded proto3 declaration subset
  for messages, fields, and unary RPC endpoints. It does not invoke `protoc`, read
  imports, generate code, or execute RPCs.
- `AnalyzeBuf(ContractInput)` accepts the JSON form of the bounded Buf v2 local
  module configuration. It records unresolved module gaps and does not invoke Buf,
  read modules, or resolve dependencies.
- `AnalyzePact(ContractInput)` accepts the documented Pact v4 JSON HTTP-declaration
  subset. It records contract declarations and gaps without evaluating matchers,
  contacting providers, or running Pact verification.

All profiles use caller-supplied bytes and declared SHA-256 digests, bounded
admission, safe logical anchors, secret/path checks, and atomic output. They do not
read the logical path, fetch remote references, start subprocesses, or establish
runtime behavior, source linkage, model results, or implementation conformance.
Exact accepted versions, limits, evidence labels, and rejection rules are in the
[trace-adapter matrix](docs/trace-adapter-matrix.md) and the linked profile
documents.

## Dossier and impact APIs

The `modelcheck/trace/dossier` package provides deterministic `Build` and `JSON`
APIs for `mcd-conformance-dossier/1`. A dossier makes finite-model bounds,
atomicity, environment, scheduler/fairness, property scope, evidence links, and
reviewer decisions explicit. It does not generate or execute a model and keeps
missing or mismatched decisions as gaps.

The `modelcheck/trace/impact` package provides deterministic `Analyze` and `SARIF`
APIs for caller-supplied artifact/evidence/dossier/change snapshots. It reports
missing, stale, orphaned, renewal, and unpromoted obligations using exact IDs and
dependency identities. Optional Git change acquisition remains bounded and
shell-free. A clean impact report is not an implementation-conformance proof.

See [conformance dossier](docs/conformance-dossier.md) and [trace-impact workflow](docs/trace-impact.md)
for the complete contracts and non-claims. The synthetic end-to-end corpus keeps
an unresolved contract/runtime disagreement visible and therefore blocks an
implementation claim until an exact reviewed decision resolves it.

## Compatibility and limitations

The existing v1 ledger and model-checking result semantics remain unchanged. The
new APIs are Go-library surfaces only; they are not available through `mcd` or MCP.
Inputs are offline and caller-owned, and accepted declarations remain contract or
runtime evidence rather than verified, modelled, or implementation evidence.
