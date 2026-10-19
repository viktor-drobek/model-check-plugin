# Supported trace-adapter matrix

> **Current-tree status.** The Stage 4 APIs below are implemented in
> `engine/trace/protocol` and have package tests in the current checkout. They are
> **implemented, release-unverified candidates**: this document records the API
> and its boundary, not a completed release, a passed full suite, a security scan,
> a review, a published artifact, or an implementation-conformance result.
>
> Version **0.4.0** is the documented release candidate; its release gates remain
> pending, and **0.3.4** is the last verified release. The adapter library does not add an `mcd` command
> or an MCP tool, so no manifest change is required for these Go APIs.

All adapters are in-memory Go library APIs in the `modelcheck` module. Their inputs
are caller-owned byte slices; the logical `Path` fields are provenance anchors and
are never opened. There is no trace-adapter CLI subcommand and no trace-adapter MCP
tool. The existing `mcd` CLI and seven MCP tools retain their documented model
checking surfaces; see [engine tools](../skills/model-check/references/engine-tools.md).

## Status and evidence boundary

| Status | Meaning | It does not mean |
| --- | --- | --- |
| **Landed baseline** | Existing public adapter API in the current tree. | A release-wide verification or implementation claim. |
| **Implemented, release-unverified candidate** | Stage 4 source and package tests are present in the current tree. | That unrun full-suite, review, security, packaging, or publication gates passed. |

| Evidence level | Direct claim | Non-claim |
| --- | --- | --- |
| `syntactic` | Direct, unambiguous Go AST facts. | Dispatch, reachability, data flow, behavior, or correctness. |
| `normative_candidate` | RFC requirement line following an in-document BCP 14 declaration. | A reviewer-approved requirement or source/model claim. |
| `licensed_clause_reference` | IEEE clause metadata pinned to a licensed document hash. | PDF content, redistribution, or legal interpretation. |
| `decoded` | Directly represented fields from a pinned protocol artifact. | Capture identity, parsing correctness, runtime behavior, or conformance. |
| `grammar` | Registered grammar artifact or reference. | Syntax validity, compilation, or grammar provenance. |
| `runtime` | Caller-projected, digest-verified span record. | Communication inference, a complete execution trace, or a verdict. |
| `contract` | Direct declaration from a digest-pinned contract input. | Execution, source linkage, transport correctness, conformance, or a model verdict. |

Every adapter retains a gap when its evidence boundary leaves an assertion
unresolved. It never upgrades evidence to `verified` or `modelled`.

## Adapter matrix

| Status and API | Accepted input | Output and evidence | Explicit boundary |
| --- | --- | --- | --- |
| **Landed baseline** `source.AnalyzeGo` / `source.GoAdapter{}.Analyze` | Caller-supplied Go source bytes. | `*trace.Document`; `syntactic` evidence. | Only direct unambiguous calls become relations; unresolved calls stay gaps. |
| **Landed baseline** `standards.ImportRFC` | Caller-provided RFC text and a canonical RFC publisher URL. | `*trace.Document`; `normative_candidate` evidence after an in-document BCP 14 declaration. | No network fetch, reviewer approval, source linkage, or model result. |
| **Landed baseline** `standards.ImportIEEEManifest` | Strict caller-provided JSON clause manifest. | `*trace.Document`; `licensed_clause_reference` evidence. | No IEEE PDF input, licensed text redistribution, or legal interpretation. |
| **Landed baseline** `protocol.AnalyzeTShark` | Pinned `tshark-json-projection/1` JSON, exact TShark profile metadata, caller digest and capture metadata. | Canonical `mcd-trace-ledger/1` bytes; `decoded` evidence. | No capture collection, raw payload handling, or capture-identity proof. |
| **Landed baseline** `protocol.AnalyzeGrammar` / `protocol.AnalyzeGrammarReference` | Narrow caller-supplied Kaitai/Spicy artifact or grammar-reference manifest. | Canonical ledger bytes; `grammar` evidence or an unresolved reference gap. | No grammar parser, compiler, download, or provenance assertion. |
| **Implemented, release-unverified candidate** `protocol.AnalyzeRuntimeSpans` | `RuntimeSpansInput`: a caller-supplied `mcd-runtime-span-export/1` JSON byte slice and its lowercase SHA-256. Each `raw` member is a `mcd-runtime-span/1` JSON object. | Canonical `mcd-trace-ledger/1` bytes containing `RuntimeSpan` nodes with only `runtime` evidence. | No OTLP/OTel decoder, collector, network, subprocess, attribute bag, or inferred communication edge. Missing build/collector data and every nonempty parent ID remain gaps. See [runtime spans](runtime-spans.md). |
| **Implemented, release-unverified candidate** `protocol.AnalyzeOpenAPI` | Caller-supplied JSON only; OpenAPI `3.0.3` or `3.1.0`; required `info` and `paths`; safe local JSON pointers only. | Canonical ledger bytes with `contract` evidence for direct endpoints, schemas, and properties. | No YAML, URL fetch, reference expansion, request execution, code generation, runtime, or conformance claim. Unmapped declarations stay gaps. See [OpenAPI profile](openapi-profile.md). |
| **Implemented, release-unverified candidate** `protocol.AnalyzeAsyncAPI` | Caller-supplied JSON only; AsyncAPI `2.6.0` or `3.0.0`; required `info` and `channels`. | Canonical ledger bytes with `contract` evidence for direct channels, messages, and fields. | No broker access, runtime traffic, direction inferred from names, reference expansion, or conformance claim. |
| **Implemented, release-unverified candidate** `protocol.AnalyzeProtobuf` | One caller-supplied UTF-8 proto3 source, beginning `syntax = "proto3";`; bounded top-level messages, unary services, and supported fields. | Canonical ledger bytes with `contract` evidence for messages, fields, and RPC endpoints. | No `protoc`, imports, code generation, streaming, nested declarations, execution, or conformance claim. See [Protobuf and Buf profiles](protobuf-profile.md). |
| **Implemented, release-unverified candidate** `protocol.AnalyzeBuf` | Caller-supplied JSON-form Buf v2 configuration with 1–32 logical local modules and no dependencies. | Canonical ledger bytes with `buf.module_unresolved` and declaration-boundary gaps. | No general YAML, Buf invocation, module read, dependency resolution, lint, breaking-change check, or execution evidence. See [Protobuf and Buf profiles](protobuf-profile.md). |
| **Implemented, release-unverified candidate** `protocol.AnalyzePact` | Caller-supplied Pact JSON with `pactSpecification.version` `4.0`, bounded interactions, and the narrow documented envelope. | Canonical ledger bytes with `contract` evidence for supported HTTP declarations and declaration gaps. | No Pact executable, provider contact, matcher/generator evaluation, payload execution, provider verification, or conformance claim. See [Pact profile](pact-profile.md). |

The complete baseline protocol-adapter limits remain in
[protocol artifacts](protocol-artifacts.md). The Stage 4 profiles above share the
same offline, all-or-nothing shape, but they do not claim the baseline adapters'
release evidence.

## Shared Stage 4 admission and bounds

`RuntimeSpansInput` and `ContractInput` carry an `Artifact` (`Path`, `Content`) plus
the lowercase SHA-256 of the exact supplied bytes. The adapters snapshot the caller
slice while processing it; callers must not mutate `Artifact.Content` during a call.
They do not read the path, access a network, start a process, or probe an installed
tool.

All Stage 4 calls are bounded and fail closed: accepted input produces canonical
ledger bytes; rejected input produces `nil, error`, never a partial ledger. The
current implementation applies a 1 MiB artifact ceiling and strict JSON depth 16
where JSON is accepted. Runtime exports allow 1–1024 entries and at most 4096 decoded
bytes per raw span. Contract parsing limits decoded JSON values to 4096, local
references to 256, planned facts/gaps to 1024, Buf modules to 1–32, and Pact
interactions to 1–256. Individual profile constraints are described in their linked
profile documents.

### Error contract

Errors are `*protocol.RuleError` values with a `stage4.*` code and a constant safe
locator. The code is an input-rejection classification, not a verification verdict.

- **Runtime spans use only `input` or `json` locators.** Their supported suffixes
  are the normalized admission/JSON suffixes `input_length`,
  `resource_admission_busy`, `metadata_invalid`, `secret_indicator`,
  `digest_mismatch`, `utf8_invalid`, `json_syntax`, `json_trailing_data`,
  `json_duplicate_name`, `json_depth`, `schema_invalid`, and `preflight_limit`,
  plus runtime-specific `span_limit`, `identity_ambiguous`, and `ledger_invalid`.
  Runtime spans do **not** use `unsupported_version` or
  `unsupported_declaration`.
- **Contract profiles use only `input`, `metadata`, `digest`, `json`, `schema`,
  or `budget` locators.** In addition to shared admission and JSON/schema
  rejections, a profile may return `unsupported_version`,
  `unsupported_declaration`, `secret_indicator` (the proto3 raw-source check),
  `preflight_limit`, or `ledger_invalid` where its parser reaches that boundary.

No Stage 4 adapter requires an external executable. Therefore there is no
"tool unavailable" execution path to fabricate: removing TShark, Buf, Pact,
`protoc`, SPIN, or a grammar compiler cannot change a Stage 4 call because none is
invoked.

## Go API usage

Use the package APIs from code built with the `modelcheck` module. They are not CLI
or MCP requests.

```go
import (
    "crypto/sha256"
    "fmt"

    "modelcheck/trace"
    "modelcheck/trace/protocol"
)

runtimeBytes := []byte(`{"schema":"mcd-runtime-span-export/1","spans":[...]}`)
runtimeDigest := sha256.Sum256(runtimeBytes)
runtimeLedger, err := protocol.AnalyzeRuntimeSpans(protocol.RuntimeSpansInput{
    Artifact: protocol.Input{Path: "spans/export.json", Content: runtimeBytes},
    ExportSHA256: fmt.Sprintf("%x", runtimeDigest),
})
if err != nil {
    return err // runtimeLedger is nil; do not continue with partial output
}
_, err = trace.UnmarshalJSON(runtimeLedger)

openAPIBytes := []byte(`{"openapi":"3.1.0",...}`)
contractDigest := sha256.Sum256(openAPIBytes)
contractLedger, err := protocol.AnalyzeOpenAPI(protocol.ContractInput{
    Artifact: protocol.Input{Path: "contracts/api.json", Content: openAPIBytes},
    ContentSHA256: fmt.Sprintf("%x", contractDigest),
})
if err != nil {
    return err // contractLedger is nil
}
_ = contractLedger

// AnalyzeAsyncAPI, AnalyzeProtobuf, AnalyzeBuf, and AnalyzePact take the same
// ContractInput shape. The content must satisfy the selected profile.
```

## End-to-end mismatch and release boundary

The synthetic corpus under `engine/testdata/trace/end-to-end/` intentionally keeps
an unresolved disagreement: `api.json` declares `POST /accept`, while `span.json`
records `drop`. The mismatch is a review obligation, not an automatic proof. It
remains visible until an exact reviewed decision identifies the target revision,
scope, and disposition. A model result, a similar name, or an unrelated decision
cannot resolve it. The associated implementation-conformance workflow is documented
in [conformance](../skills/model-check/references/conformance.md),
[the conformance dossier](conformance-dossier.md), and
[the trace-impact workflow](trace-impact.md). These documents preserve review
obligations; they do not turn an unresolved mismatch into an implementation claim.

Before treating a Stage 4 candidate as released, re-run the normal project release
procedure for the final reviewed tree, including the appropriate tests, review,
security checks, manifest/build-artifact checks, and reproducible build. This candidate
update ran none of those commands and did not verify the `0.4.0` release, build
artifacts, or published package; historical `0.3.4` artifacts remain unchanged.

## Related public documents

- [Trace ledger](trace-ledger.md) — ledger schema, evidence, gaps, and reporting.
- [Protocol artifacts](protocol-artifacts.md) — landed Stage 3b profiles and limits.
- [Runtime spans](runtime-spans.md) — Stage 4 runtime profile.
- [OpenAPI profile](openapi-profile.md), [Protobuf and Buf profiles](protobuf-profile.md), and [Pact profile](pact-profile.md) — implemented Stage 4 declaration profiles.
- [Source adapters](source-adapters.md) and [standards imports](standards-imports.md) — baseline adapter boundaries.
- [Engine tools](../skills/model-check/references/engine-tools.md) and [conformance workflow](../skills/model-check/references/conformance.md) — CLI/MCP boundaries and the separate dossier process.
