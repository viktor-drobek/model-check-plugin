# Deliberately incomplete trace corpus

All files in this directory were authored for this repository as synthetic test
fixtures. They contain no captured traffic, third-party standards text, licensed
material, personal data, or deployment configuration. Redistribution is permitted
under the repository license. No external artifact acquisition is necessary.

The intended obligation is: an accepted request reaches the `Accept` operation.
`api.json` declares `POST /accept`; `handler.go` directly calls `Accept`.
`span.json` instead records an observed operation named `drop`, with no producer
build or collector identity. These names are not a machine-proved correspondence:
the mismatch is an explicit, unresolved review obligation, not a similarity match.

Hash each file's exact bytes when supplying it to an adapter. The runtime export
must embed the exact `span.json` bytes as its `raw` string and their SHA-256 as
`content_sha256`, then hash the exact export independently. No installed OpenAPI
or telemetry tooling is required.

A finite model verdict, if attached, applies only to that model and its recorded
assumptions. The generated dossier must retain the missing-provenance and
contract/runtime-disagreement gaps and must not claim implementation correctness.
An unrelated decision, a decision for another revision/scope, or a model verdict
cannot resolve the mismatch. A reviewer must explicitly address the exact target
revision, scope and obligation; the ledger evidence itself remains unchanged.
