# Conformance dossier: <system>

## Identity and scope

- Implementation revision: `<immutable revision and source hash>`
- Model revision and hash: `<immutable revision and model hash>`
- Properties: `<IDs and exact text>`
- Engine/version and invocation: `<version, CLI command or MCP request>`
- Environment: `<OS, build flags, deployment configuration>`
- System boundary and excluded components: `<scope>`

## Model and implementation

- Model verdict: `<status and evidence, or no result>`
- Implementation claim: `<not established | blocked | justified claim>`
- Scheduler, fairness, memory model, and atomicity: `<assumptions>`
- Finite bounds and abstractions: `<domains, queues, process counts, timing>`

## Mapping and abstraction

| Source element | Model element | Difference | Direction | Evidence |
|---|---|---|---|---|
| | | | over / under / exact | |

List every omitted path, introduced nondeterminism, data abstraction, timing
abstraction, and unresolved assumption.

## Preservation obligations

| Obligation | Property class | Basis | Status | Reviewer |
|---|---|---|---|---|
| Initial states | safety / liveness | | open / satisfied / blocked | |
| Operation or transition class | safety / liveness | | open / satisfied / blocked | |
| Environment and scheduler | safety / liveness | | open / satisfied / blocked | |
| Fairness, divergence, and stuttering | liveness | | open / satisfied / blocked | |

## Evidence and review

- Mapping evidence: `<links to source, model, review, tests, or checked obligations>`
- Counterexample/mismatch classification: `<implementation defect | model defect | abstraction artefact | unresolved mismatch>`
- Remaining obligations and assumptions: `<list>`
- Reviewer decisions: `<name, date, decision, rationale>`

## Revalidation

- Invalidated by: `<source/model/property/environment changes>`
- Revalidation decision: `<date, reviewer, scope, outcome>`
- Supersedes: `<prior dossier or none>`
