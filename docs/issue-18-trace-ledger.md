# Issue #18 implementation plan

Issue [#18](https://github.com/viktor-drobek/model-check-plugin/issues/18) adds a reproducible, fail-closed workflow for tracing requirements, source, protocol artefacts, formal models, and human review. It does **not** make a source scan, packet dissection, LLM description, test pass, or model verdict into a proof about an implementation.

## Delivery order

| Stage | Deliverable | Boundary and acceptance gate |
| --- | --- | --- |
| 1 — trace ledger | `mcd-trace-ledger/1`, strict Go validator, canonical JSON, Markdown and SARIF gap reports | Closed node/relation/evidence vocabularies; exact IDs only; provenance on every record; deterministic output; malformed or dangling records rejected. |
| 2 — source adapters | Language-neutral source-adapter contract and Go, Python, Rust, Java fixtures | Emit `syntactic` facts only. Calls are emitted only when resolution is unambiguous. No adapter claims alias analysis, reachability, or correctness. |
| 3 — offline protocol and standards imports | TShark JSON, Kaitai/Spicy references, RFC and IEEE clause-manifest adapters | Captures remain user-named local input; no live capture. IEEE input stays a user-provided concise manifest; no licensed PDF download, storage, or redistribution. |
| 4 — contracts and runtime | OpenAPI, AsyncAPI, Protobuf/Buf, Pact, and explicit runtime-span contracts | Missing optional tools return explicit unavailable records. Contract and runtime evidence remain distinct from source and model evidence. |
| 5 — model hand-off | Deterministic algorithm/protocol brief, model-intake card, and conformance dossier skeleton | A model cannot be generated or checked until bounds, atomicity, scheduler/fairness, and property scope are explicit. Existing `mcd` statuses and evidence semantics remain unchanged. |
| 6 — review and change impact | Orphan/gap analysis, Git-diff invalidation, stable SARIF findings, reviewer-decision workflow | No implementation-level conclusion survives a source/model/property/environment revision without review. |
| 7 — packaging and end-to-end corpus | Agent-facing package, supported-adapter matrix, and intentional mismatch example | The example must block an implementation claim until its dossier is resolved. Run documented full suite, static checks, review, and bounded confirmation gates. |

### Stage 3a status

Stage 3 begins with offline imports for caller-provided RFC text and an IEEE clause manifest. The implementation records RFC identifiers, publication metadata, source and line anchors, and BCP 14 candidates only after an in-document declaration; it records IEEE clauses and concise summaries solely as `licensed_clause_reference` evidence. It does not fetch documents, download or process IEEE PDFs, analyse traffic, parse RFC XML, or handle Kaitai/Spicy artefacts. See [Offline standards imports](standards-imports.md) for the input contracts and deferred boundaries.

### Stage 3b status

The `engine/trace/protocol` Go APIs consume only caller-supplied offline bytes:
`tshark-json-projection/1` with the exact `4.4.10` / `tshark-4.4.10-core-1`
profile, raw-admitted Kaitai `0.10` or Spicy `1.12.0` grammar files, and
`mcd-grammar-reference/1` manifests. TShark presence facts use `decoded`
evidence; grammar registration uses an isolated schema-v2 `GrammarArtifact` with
`grammar` evidence and its mandatory unverified-anchor gap; absent grammar bytes
produce only an unresolved-reference gap. See [Offline protocol artifacts](protocol-artifacts.md)
for executable tests and exact admission, hashing, limits and disclosure rules.
These APIs do not invoke TShark or grammar compilers, open logical paths, parse
PCAPs, resolve external references, infer state machines, or establish implementation
conformance. Stages 4–7 remain separate workstreams, with candidate implementation
artifacts documented separately below; final validation and release gates remain open.

### Stage 4 candidate status

Stage 4 of the delivery order covers "contracts and runtime"; only the
**runtime** half has a scoped contract so far. `AnalyzeRuntimeSpans(RuntimeSpansInput)`
in `engine/trace/protocol` is a caller projection of an explicit offline
runtime-span export into `mcd-trace-ledger/1` `RuntimeSpan` nodes with `runtime`
evidence, occurrence deduplication, and explicit provenance gaps. It reuses the
shared Stage 3b admission, decoder, hashing, budget, and canonical encoder; it
is a Go API, not a new `mcd` command or MCP tool, and it emits no inferred
communication, call, or contract edges and no verification claim. See
[Explicit offline runtime-span records](runtime-spans.md) for the exact input,
the six envelope and nine raw fields, digest boundaries, deduplication key,
provenance and gaps, validation precedence and error normalization, and
non-claims.

The current tree also contains candidate implementations and package-test
candidates for OpenAPI, AsyncAPI, Protobuf/Buf, and Pact. These remain offline
Go library APIs with `contract` evidence and explicit declaration gaps; they are
not new `mcd` commands or MCP tools and make no implementation-conformance or
model-checking claim. See the individual profile documents for their scoped
boundaries.

This is an **implemented, release-unverified candidate** status, not final
validation or release approval. Stage 4 remains open until its acceptance and
release gates pass. Stages 5 and 6 also have source/test candidates in the
current tree, but their explicit dossier decisions are review inputs and do not
automatically prove implementation conformance. Stage 7 release gates remain
open; remote model confirmations are cancelled for this candidate and are not
passing evidence.

## Stage 1 contract

The Stage 1 schema is [`engine/trace/schema.json`](../engine/trace/schema.json). It defines:

- a closed node vocabulary: `Issue`, `Requirement`, `Assumption`, `StandardClause`, `ProtocolMessage`, `BinaryField`, `SourceSymbol`, `Endpoint`, `ModelTransition`, `Property`, `Scenario`, `Test`, `RuntimeSpan`, `Counterexample`, and `ReviewDecision`;
- closed relations, including `implements`, `maps_to`, `calls`, `sends`, `receives`, `decodes_as`, `tests`, `verifies`, `assumes`, `omits`, `contradicts`, and `derived_from`;
- a separate evidence ledger with non-ordered labels; and
- explicit `gaps` for unresolved obligations.

Every node, edge, evidence record, and gap has a stable ID and provenance: a content SHA-256 or source revision, a stable anchor, and a producer name/version. Edges resolve only exact node IDs. A dangling source or target is rejected; it is never repaired through label similarity. When a relationship is genuinely missing, an adapter emits a `gap` rather than a fabricated edge.

## Review and evidence boundary

Evidence labels such as `syntactic`, `decoded`, `inferred`, `contract`, `runtime`, `modelled`, `verified`, and `reviewed` are labels of how a record was obtained. They are deliberately not a total ordering and must not be silently upgraded. In particular, `verified` records a verdict about a finite formal model under its recorded assumptions; it does not establish source-code correctness. An implementation claim requires the conformance obligations tracked by issue #11 and a human review decision.

## Deferred work

Stage 1 does not read source trees, run CodeQL/Joern/Semgrep, parse packet captures, download RFCs, access licensed IEEE material, invoke MCP, or alter `mcd` verification reports. Those behaviours need their own bounded adapters and fixtures in later stages.
