# Explicit offline runtime-span records

`engine/trace/protocol` adds one more caller-byte Go API for Stage 4a:
`AnalyzeRuntimeSpans(RuntimeSpansInput)` returns canonical `mcd-trace-ledger/1`
JSON built from a caller-supplied span export. Like the Stage 3b APIs it never
opens the logical path, fetches a reference, invokes a collector, or infers
communication edges, and it is a Go API, not a new `mcd` command or MCP tool.
This document describes only the runtime profile. The independent contract
profiles are documented in [OpenAPI](openapi-profile.md),
[AsyncAPI](asyncapi-profile.md), [Protobuf/Buf](protobuf-profile.md), and
[Pact](pact-profile.md); none of these profiles is a release approval.

The record is a **named caller projection**: the caller asserts the span facts
and their build/collector metadata; the adapter recomputes and verifies the
declared digests and records what remains unverified as explicit gaps. It is not
an OTLP/OTel decoder or a collector, and it performs no contract matching, no
inferred communication edges, and no verification claims.

## Go API

```go
canonical, err := protocol.AnalyzeRuntimeSpans(protocol.RuntimeSpansInput{
    Artifact: protocol.Input{Path: "spans/export.json", Content: exportBytes},
    ExportSHA256: exportSHA256, // lowercase SHA-256 of the exact export bytes
})
if err != nil {
    // Input is rejected; canonical is nil. Do not continue with a partial ledger.
}
ledger, err := trace.UnmarshalJSON(canonical)
```

Do not mutate the slice in `Artifact.Content` during the call, as for the Stage
3b APIs. The returned bytes already include canonical serialization, so do not
call `trace.JSON` again merely to re-serialize them.

## Exact input

`RuntimeSpansInput` carries the shared `Artifact` input (logical `Path` plus
`Content`) and a mandatory `ExportSHA256`, the lowercase SHA-256 of the exact
export bytes. `Artifact.Path` is logical only, obeys the shared `validPath`
rule, and is at most **64** bytes — the runtime path's additional limit on top
of the shared metadata checks.

The export is one JSON object with exactly two members, `schema` and `spans`:

```json
{"schema":"mcd-runtime-span-export/1","spans":[{"trace_id":"trace.demo","content_sha256":"<computed lowercase SHA-256 of raw UTF-8 bytes>","raw":"<one JSON span encoded as a JSON string>","build_revision":"","collector_name":"","collector_version":""}]}
```

Each `spans` entry has exactly the **six** string members below:

| Member | Constraint |
| --- | --- |
| `trace_id` | ASCII `[A-Za-z][A-Za-z0-9._:-]{0,63}`; must match the enclosing raw span's `trace_id`. |
| `content_sha256` | Lowercase SHA-256 asserted for the exact decoded `raw` bytes. |
| `raw` | One JSON span encoded as a JSON string; decodes to UTF-8 bytes. |
| `build_revision` | Empty, or a lowercase 40- or 64-hex immutable revision. |
| `collector_name` | Empty, or `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`. |
| `collector_version` | Empty, or `[A-Za-z0-9][A-Za-z0-9._-]{0,63}`. |

`collector_name` and `collector_version` are both empty or both present; partial
collector metadata is invalid. The profile accepts no base64, compressed bytes,
attributes bag, references, external resource URLs, side-channel labels, or
alternate top-level casing.

## Raw span record

The decoded `raw` must be one JSON object with exactly the **nine** fields:

```json
{"schema":"mcd-runtime-span/1","trace_id":"trace.demo","span_id":"span.one","parent_id":"","name":"accept","kind":"server","status":"ok","start_ns":0,"end_ns":1}
```

| Field | Constraint |
| --- | --- |
| `schema` | Exactly `mcd-runtime-span/1`. |
| `trace_id` | ASCII `[A-Za-z][A-Za-z0-9._:-]{0,63}`; must equal the enclosing entry's `trace_id`. |
| `span_id` | ASCII `[A-Za-z][A-Za-z0-9._:-]{0,63}`. |
| `parent_id` | Empty, or the same ASCII pattern. Recorded, not lowered. |
| `name` | ASCII `[A-Za-z][A-Za-z0-9._:-]{0,63}`. |
| `kind` | Exactly one of `internal`, `server`, `client`, `producer`, `consumer`. |
| `status` | Exactly one of `unset`, `ok`, `error`. |
| `start_ns` | Canonical unsigned decimal, `0` through `9223372036854775807` inclusive. |
| `end_ns` | Same; must satisfy `end_ns >= start_ns`. |

`start_ns` and `end_ns` are JSON number tokens with no strings, fractions,
exponents, signs, or negative zero. The kind and status values are caller
assertions, not inferred classifications. All metadata and raw strings are
checked against the shared secret indicators; the restricted scalar alphabets
exclude host paths, escapes, whitespace, control characters, and reference
URLs. Schema literals are the only admitted slash-containing scalar strings.
Unknown fields are rejected, not dropped — in particular this profile does not
accept service attributes or arbitrary OTel attributes, so no unsupported fact
enters the ledger silently.

## Digests

Two independent digests cover the export:

- The mandatory per-entry `content_sha256` hashes the exact **decoded** `raw`
  bytes, including whitespace — not a re-serialized object and not the
  surrounding JSON string token.
- `ExportSHA256` hashes the exact enclosing export bytes and is verified
  independently of the per-entry digests.

Every declared raw digest is checked, even for entries later deduplicated. A
well-formed declared digest that differs from the recomputed raw digest is a
`stage4.digest_mismatch`; a digest in a malformed form is metadata-invalid in the
envelope-validation phase.

## Occurrence deduplication

The deduplication key is exactly `(entry.trace_id, computed raw SHA-256)`:

- Identical keys with **identical** build/collector metadata yield one node and
  an occurrence count `N`.
- Identical keys with **different** build/collector metadata fail the whole
  export as `stage4.identity_ambiguous`; no provenance is picked by order.
- The same `trace_id` with different computed hashes yields distinct nodes, even
  when `span_id` is identical.

No label similarity and no span-ID-only deduplication is permitted. The
occurrence count is the only output difference between one record and repeated
byte-identical records with identical metadata, so a byte-identical export
yields identical output, and byte-identical records reordered within the export
also yield identical output.

## Provenance and gaps

For each unique deduplication group:

- The hash tuple is NUL-separated through `tupleHash`:
  `("runtime-span/1", logicalPath, callerTraceID, computedRawSHA256)`.
- Node ID `span.<hash>` and evidence ID `evidence.span.<hash>`. These IDs do not
  include the export digest, ordinal, occurrence count, or collector metadata,
  so reordering records cannot alter the output and an assertion conflict cannot
  be hidden by splitting a deduplication group within one export.
- Node kind `RuntimeSpan`, level `runtime`, no edges, and label exactly
  `Observed span <name> (<kind>, <status>), <start_ns>..<end_ns> ns`.
- Provenance content hash is the computed raw digest. The source revision is the
  supplied immutable build revision, omitted when absent. The producer name is
  `mcd-runtime-span-adapter`, version
  `1;collector=<name-or-unavailable>@<version-or-unavailable>`. Neither the
  adapter name nor a collector assertion is verified provenance.
- Anchor exactly
  `span:<logicalPath>#trace=<trace_id>&span=<span_id>&parent=<parent_id>&sha256=<computed>&occurrences=N`,
  all restricted ASCII, with anchor length at most 512 bytes.

Missing provenance yields explicit gaps rather than invented facts:

| Condition | Gap ID | Kind | Reason |
| --- | --- | --- | --- |
| Empty `build_revision` | `gap.span.build_missing.<hash>` | `span.provenance_missing` | `producer build revision was not supplied` |
| Empty collector name/version | `gap.span.collector_missing.<hash>` | `span.provenance_missing` | `collector name and version were not supplied` |
| Nonempty `parent_id` | `gap.span.parent_unmapped.<hash>` | `span.parent_unmapped` | `parent identity was recorded without inferring a communication relation` |

Parent IDs are recorded in anchors. Parentage is deliberately not lowered to
`sends`/`receives`: it does not establish message flow. Every nonempty parent
ID yields `span.parent_unmapped`, including when the parent span happens to be
present in the same export; a later reviewed increment may add a relation. Every
gap references `node:<nodeID>` and carries the same provenance.

## Admission, limits, and failure order

The runtime path reuses the shared Stage 3b admission, strict JSON decoder,
hashing, budget, and canonical encoder; it adds no new dependency, subprocess,
file access, environment or tool probing, schema vocabulary, or global
admission primitive.

Shared admission, in its actual order, checks the export length
**1 through 1 MiB** inclusive, reserves the single process-wide construction
slot, snapshots the bytes, checks the logical path and digest metadata (the
runtime path's additional 64-byte limit is part of this metadata validation),
checks the metadata for secrets, and verifies the exact export digest. Shared
decoding then validates export UTF-8/BOM, complete JSON syntax, surrogate
pairs, trailing bytes, decoded-name duplicates, and depth at most **16** — the
unchanged Stage 3b precedence. The runtime additions, before any graph
record is emitted, are:

1. The exact export field set and types, then the export schema literal
   (`stage4.schema_invalid`), then the entry count **1 through 1024**
   (`stage4.span_limit`). Every entry is validated in input array order before
   any raw span is parsed: the exact six-member field set and string types
   (`stage4.schema_invalid`), trace-ID format, declared digest format, build
   revision format, collector pairing, collector-name format, collector-version
   format (all `stage4.metadata_invalid`), then the decoded raw byte length
   **1 through 4096** (`stage4.input_length`), in that order. A malformed
   declared digest is metadata-invalid; a well-formed digest unequal to the
   computed raw digest is `stage4.digest_mismatch` in the next phase. Thus
   invalid envelope metadata in a later entry precedes malformed raw JSON in an
   earlier entry. Validation is entry-by-entry: an earlier entry's raw-length
   failure can precede a later entry's metadata failure.
2. In input array order, decode each raw JSON with the same decoder, validate its
   exact field set and scalar constraints, verify its declared digest
   (`stage4.digest_mismatch`), check trace-ID agreement
   (`stage4.identity_ambiguous`), and check secrets in the admitted metadata and
   raw values (`stage4.secret_indicator`).
3. Reject duplicate-key groups with conflicting build/collector metadata
   (`stage4.identity_ambiguous`).
4. Check the conservative output/working/record budgets before graph allocation
   (`stage4.preflight_limit`).
5. Construct and validate the whole ledger and return canonical bytes, or
   return an error with nil output. A successful result is never partial.

The closed profile bounds are: at most 1024 input records, 4096 decoded bytes
per raw record, 1 MiB per export, 16 JSON scopes, a 64-byte logical path,
64-byte IDs/name/collector scalars, a 512-byte anchor, and a 200-byte label —
and at most five graph records per unique span (the node, the evidence record,
and the three possible gaps). The preflight bound is
`checkBudget(5*inputCount, 4096+16*exportBytes+16384*inputCount, limits)`
with unsigned, bounded arithmetic, taken before graph allocation. It accounts
for already-bounded parsing/snapshot overhead and repeated strings; it gates
graph allocation, not the earlier parsing allocations. An unexported
`analyzeRuntimeSpansWithBudget(input RuntimeSpansInput, limits budget)` helper
supplies the local budget for tests; the public entry point passes
`defaultBudget`.

### Error normalization

Errors are `*RuleError` carrying a `stage4.` code and a safe constant locator
only. A single runtime-local normalizer sits at the public API boundary without
changing the shared functions. For a `*RuleError` it either preserves an
explicitly supported `stage4.*` code or remaps an established shared
`stage3b.*` code to the same suffix under `stage4.`. The supported shared
suffixes are `input_length`, `resource_admission_busy`, `metadata_invalid`,
`secret_indicator`, `digest_mismatch`, `utf8_invalid`, `json_syntax`,
`json_trailing_data`, `json_duplicate_name`, `json_depth`, `schema_invalid`, and
`preflight_limit`; the extra runtime suffixes are `span_limit`,
`identity_ambiguous`, and `ledger_invalid`. Every locator is replaced by a
constant: `json` for `utf8_invalid` and the `json_*` codes, otherwise `input`.
Messages are replaced by a constant safe message derived only from the approved
code; the original error text is never copied. All non-rule errors and
unrecognized rule codes — including failures from ledger construction,
validation, or encoding — are normalized to `stage4.ledger_invalid` with locator
`input` and a constant message. Every failure returns nil bytes. `input_length`
covers both export and raw length failures, and `span_limit` covers the span
count. No error message or locator includes a raw value, byte offset, caller
path, or identity.

## Non-claims

- A `RuntimeSpan` record is a projection of caller-supplied span facts, not an
  execution trace the adapter observed. Neither the adapter name nor a collector
  assertion is verified provenance.
- Parentage is recorded in anchors and disclosed through `span.parent_unmapped`;
  it is not lowered to `sends`/`receives`, so a span record never asserts that a
  parent and a child exchanged a message.
- `RuntimeSpan` nodes carry no edges. The adapter does not infer communication,
  call, or contract edges, does not match spans to a contract, and makes no
  implementation-conformance conclusion.
- `verified`, `modelled`, or any verdict label does not attach to a runtime
  span; the record's level is `runtime` and its evidence is the caller's
  projection.
- Missing build or collector metadata produces an explicit gap, never an
  invented value; no fact silently enters the ledger.

## Go usage and verification

Build and test with the toolchain pinned in `engine/go.mod`; no additional
dependency is required:

```sh
cd model-check-plugin/engine
go test ./trace/protocol -count=1
go test ./trace/protocol -run '^TestRuntimeSpanFeatures$' -count=1
```

A caller computes the export digest before calling the adapter; the adapter
recomputes and verifies it:

```go
sum := sha256.Sum256(exportBytes)
canonical, err := protocol.AnalyzeRuntimeSpans(protocol.RuntimeSpansInput{
    Artifact: protocol.Input{Path: "spans/export.json", Content: exportBytes},
    ExportSHA256: fmt.Sprintf("%x", sum),
})
if err != nil {
    // Reject atomically; canonical is nil. Handle RuleError without logging input.
    return err
}
ledger, err := trace.UnmarshalJSON(canonical)
```

Imports are `crypto/sha256`, `fmt`, `modelcheck/trace`, and
`modelcheck/trace/protocol` inside this Go module. Fixtures in the unit and G9
tests are synthetic, generated inline, and redistributable with this repository;
they contain no third-party trace or collector material. See [Trace
ledger](trace-ledger.md), [Offline protocol artifacts](protocol-artifacts.md),
[the staged plan](issue-18-trace-ledger.md), and [provenance](../PROVENANCE.md).
