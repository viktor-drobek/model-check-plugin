# Trace ledger contract

`engine/trace` provides the data contract used by the first stage of issue #18. Its schema identifier is `mcd-trace-ledger/1`.

## What it records

A document has four ordered collections:

- **nodes** — typed artefacts such as a requirement, source symbol, protocol message, model transition, property, test, or review decision;
- **edges** — exact, typed relations between node IDs;
- **evidence** — append-only evidence records for a node or edge; and
- **gaps** — explicit obligations that remain unresolved.

Each record has provenance with a source revision or lowercase SHA-256 content digest, a stable anchor, and a producer name/version. Nodes and edges list one or more evidence record IDs; every evidence record must in turn be listed by its exact subject. The validator rejects unknown or case-variant fields, duplicate JSON keys at any nesting depth, excessive JSON nesting, trailing JSON, invalid hashes, unknown vocabulary values, duplicate IDs within a record collection, dangling relations, detached evidence, and evidence attached to the wrong subject.

## Go API

```go
ledger, err := trace.UnmarshalJSON(input)
if err != nil {
    // Input is rejected; do not continue with a partial ledger.
}
canonicalJSON, err := trace.JSON(ledger)
markdown, err := trace.Markdown(ledger)
sarif, err := trace.SARIF(ledger)
```

`trace.JSON` makes a validation-preserving copy, sorts every list by stable ID, and emits two-space-indented JSON with a trailing newline. `trace.Markdown` is a review summary. `trace.SARIF` reports only explicit `gaps` as SARIF warnings; recorded facts are not findings merely because they exist.

## Evidence is not a verdict transfer

The labels `syntactic`, `decoded`, `grammar`, `inferred`, `contract`, `normative_candidate`, `licensed_clause_reference`, `runtime`, `modelled`, `verified`, and `reviewed` describe the origin of a record. They are not rank-ordered and one label cannot silently replace another.

In particular:

- a source adapter can record syntactic observations, not an alias analysis or a reachability proof;
- an inferred protocol vocabulary stays a hypothesis until reviewed;
- a `verified` model result remains a result about the finite model and its assumptions; and
- an implementation-level claim requires the separate conformance argument described by issue #11.

The `modelled` and `verified` labels may attach only to `ModelTransition`, `Property`, or `Counterexample` nodes, or to edges wholly between those formal-model artefacts. This rejects a source-side verdict transfer at the ledger boundary rather than merely warning about it in a report.

## Adapter boundary

The package has no filesystem, network, external-tool, MCP, or model-checker dependency. A future adapter must normalize its own input, calculate the source hash, choose a stable anchor, and either produce a valid evidence-backed node/edge or an explicit gap. It must never guess an unresolved reference from a similar name.

For the complete staged implementation order, see [Issue #18 implementation plan](issue-18-trace-ledger.md).
