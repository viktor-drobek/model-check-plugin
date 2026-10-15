# Issue #18 implementation plan

Issue [#18](https://github.com/viktor-drobek/model-check-plugin/issues/18) adds a reproducible, fail-closed workflow for tracing requirements, source, protocol artefacts, formal models, and human review. It does **not** make a source scan, packet dissection, LLM description, test pass, or model verdict into a proof about an implementation.

## Delivery order

| Stage | Deliverable | Boundary and acceptance gate |
| --- | --- | --- |
| 1 — trace ledger | `mcd-trace-ledger/1`, strict Go validator, canonical JSON, Markdown and SARIF gap reports | Closed node/relation/evidence vocabularies; exact IDs only; provenance on every record; deterministic output; malformed or dangling records rejected. This PR delivers this stage. |
| 2 — source adapters | Language-neutral source-adapter contract and Go, Python, Rust, Java fixtures | Emit `syntactic` facts only. Calls are emitted only when resolution is unambiguous. No adapter claims alias analysis, reachability, or correctness. |
| 3 — offline protocol and standards imports | TShark JSON, Kaitai/Spicy references, RFC and IEEE clause-manifest adapters | Captures remain user-named local input; no live capture. IEEE input stays a user-provided concise manifest; no licensed PDF download, storage, or redistribution. |
| 4 — contracts and runtime | OpenAPI, AsyncAPI, Protobuf/Buf, Pact, and explicit runtime-span contracts | Missing optional tools return explicit unavailable records. Contract and runtime evidence remain distinct from source and model evidence. |
| 5 — model hand-off | Deterministic algorithm/protocol brief, model-intake card, and conformance dossier skeleton | A model cannot be generated or checked until bounds, atomicity, scheduler/fairness, and property scope are explicit. Existing `mcd` statuses and evidence semantics remain unchanged. |
| 6 — review and change impact | Orphan/gap analysis, Git-diff invalidation, stable SARIF findings, reviewer-decision workflow | No implementation-level conclusion survives a source/model/property/environment revision without review. |
| 7 — packaging and end-to-end corpus | Agent-facing package, supported-adapter matrix, and intentional mismatch example | The example must block an implementation claim until its dossier is resolved. Run documented full suite, static checks, review, and bounded confirmation gates. |

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
