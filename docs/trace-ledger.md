# Trace ledger contract

`engine/trace` provides the data contract used by the first stage of issue #18.
The original immutable schema identifier is `mcd-trace-ledger/1`; the explicit
`mcd-trace-ledger/2` opt-in adds the isolated `GrammarArtifact` record described
below. Existing producers remain v1 unless they intentionally select v2.

## What it records

A document has four ordered collections:

- **nodes** — typed artefacts such as a requirement, source symbol, protocol message, model transition, property, test, or review decision;
- **edges** — exact, typed relations between node IDs;
- **evidence** — evidence records for a node or edge; and
- **gaps** — explicit obligations that remain unresolved.

Each record has provenance with a source revision or lowercase SHA-256 content digest, a stable anchor, and a producer name/version. Nodes and edges list one or more evidence record IDs; every evidence record must in turn be listed by its exact subject. The validator rejects unknown or case-variant fields, duplicate JSON keys at any nesting depth, excessive JSON nesting, trailing JSON, invalid hashes, unknown vocabulary values, duplicate IDs within a record collection, dangling relations, detached evidence, and evidence attached to the wrong subject.

Stage 1 validates one ledger document; it does not enforce append-only history
or make a ledger tamper-evident. Preserving evidence history and reviewing
changes are Stage 6 responsibilities.

## Go API

```go
ledger, err := trace.UnmarshalJSON(input)
if err != nil {
    // Input is rejected; do not continue with a partial ledger.
}
canonicalJSON, err := trace.JSON(ledger)
markdown, err := trace.Markdown(ledger)
sarif, err := trace.SARIF(ledger)

v2 := &trace.Document{Schema: trace.SchemaV2}
v2Schema := trace.JSONSchemaV2()
```

`trace.JSON` makes a validation-preserving copy, sorts every list by stable ID, and emits two-space-indented JSON with a trailing newline. `trace.Markdown` is a review summary. `trace.SARIF` reports only explicit `gaps` as SARIF warnings; recorded facts are not findings merely because they exist.

SARIF creates a source region only for an unambiguous relative `path:line`
gap reference. A bare `name:line` that could be an opaque URI must be written
as `./name:line`; a path containing a slash is already unambiguous. Relative
references retain their query or fragment, while syntactically absolute URIs
(including opaque forms such as `urn:`), network-path references, and any
reference with a query or fragment remain URI-only. Windows absolute paths
(`C:/...`) are rendered as paths, but their drive colon does not create a source
region; POSIX-absolute paths are URI-only. Source-path URIs escape literal
percent signs so a ledger filename is never decoded as URI escaping. The exact
ledger reference is retained in each result's `properties.reference`.

## Evidence is not a verdict transfer

The labels `syntactic`, `decoded`, `grammar`, `inferred`, `contract`, `normative_candidate`, `licensed_clause_reference`, `runtime`, `modelled`, `verified`, and `reviewed` describe the origin of a record. They are not rank-ordered and one label cannot silently replace another.

In particular:

- a source adapter can record syntactic observations, not an alias analysis or a reachability proof;
- an inferred protocol vocabulary stays a hypothesis until reviewed;
- a `verified` model result remains a result about the finite model and its assumptions; and
- an implementation-level claim requires the separate conformance argument described by issue #11.

The `modelled` and `verified` labels may attach only to `ModelTransition`, `Property`, or `Counterexample` nodes, or to edges wholly between those formal-model artefacts. This rejects a source-side verdict transfer at the ledger boundary rather than merely warning about it in a report.

## Schema v2 grammar-artifact migration

Schema v2 adds `GrammarArtifact` only for the registration of exact supplied
Kaitai or Spicy grammar bytes. It is an isolated node with exactly one `grammar`
evidence record and one `grammar.anchor_unverified` gap. Its deterministic anchor
states a declared grammar anchor; the mandatory gap makes clear that the declared
anchor was not syntax-verified.

In v2, `grammar` evidence means grammar-file registration provenance only. It
does not establish that a grammar compiles, names a real declaration, recognises
traffic, decodes a message, or conforms to an implementation. In contrast,
`decoded` records the projection adapter's interpretation of caller-supplied,
TShark-labelled bytes, not a verified execution of TShark. Neither evidence label establishes
implementation correctness; that still needs a separate conformance argument.

The v2 decoder is selected only when the first JSON root member is the exact raw
selector `"schema": "mcd-trace-ledger/2"`. All other byte sequences remain on
the immutable v1 decoder path. The v2 path rejects malformed UTF-8, malformed
escapes or surrogate pairs, duplicate decoded member names, unknown fields, and
trailing data before applying its schema rules.

The declared grammar anchor has the form
`grammar:<kaitai|spicy>@<major.minor[.patch]>:<logical-path>#declared=<name>`.
The node, evidence, and disclosure-gap identifiers are deterministic SHA-256
identities over the language, version, logical path, declaration name, and exact
grammar-content digest. `trace.JSONSchemaV2()` exposes the embedded structural
schema for this wire format. JSON Schema validation is necessary but not sufficient:
`trace.UnmarshalJSON` and `trace.Validate` also enforce cross-record identity,
matching provenance and evidence, GrammarArtifact isolation, and the matching
mandatory disclosure gap.

## Adapter boundary

The package has no filesystem, network, external-tool, MCP, or model-checker dependency. A future adapter must normalize its own input, calculate the source hash, choose a stable anchor, and either produce a valid evidence-backed node/edge or an explicit gap. It must never guess an unresolved reference from a similar name.

The Stage 3a offline RFC-text and IEEE clause-manifest adapters are documented in [Offline standards imports](standards-imports.md). They do not add network, filesystem, or licensed-document access to this package.

Stage 3b adds the caller-byte Go APIs in `engine/trace/protocol`: a pinned
TShark JSON projection, raw-admitted Kaitai/Spicy grammar registration, and an
unresolved grammar-reference manifest. See [Offline protocol artifacts](protocol-artifacts.md)
for the exact formats, version pairs, resource limits, secret/path policy and
non-claims. These APIs return canonical ledger bytes under process-wide single-flight
admission; they do not read captures or files, invoke producers, or expose new CLI/MCP commands.

Stage 4a adds the explicit offline runtime-span record in the same
`engine/trace/protocol` package: `AnalyzeRuntimeSpans(RuntimeSpansInput)`
projects a caller-supplied span export into `mcd-trace-ledger/1` `RuntimeSpan`
nodes with `runtime` evidence, occurrence deduplication, and explicit
provenance gaps. See [Explicit offline runtime-span records](runtime-spans.md)
for the exact input, the six envelope and nine raw fields, digest boundaries,
deduplication key, provenance and gaps, validation precedence and error
normalization, and non-claims. It is a caller projection, not an OTLP/OTel
decoder or collector, and it emits no inferred communication or contract
edges.

For the complete staged implementation order, see [Issue #18 implementation plan](issue-18-trace-ledger.md).
