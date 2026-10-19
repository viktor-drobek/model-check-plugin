# Model-to-implementation conformance

Sources: model-check-skill-notes/10-cross-book-synthesis.md §12; model-check-skill-notes/11-skill-requirements.md §1.3, §11 and §14.

A model-checking result is a result about a finite model under its recorded
assumptions. It is not, by itself, a result about an implementation. Use this
workflow when a report, release decision, or review intends to connect the two.

## 1. Create an identity record

Pin the implementation revision, model hash, property text, engine version, command
or MCP request, and environment. Record the scheduler, fairness, memory model,
atomicity boundary, build flags, deployment configuration, and every finite bound.
Do not replace a revision hash with a branch name.

## 2. Write the source-to-model mapping

For each implementation state field, operation, event, and control location, record
the corresponding model state or transition. List omitted behaviour, added
nondeterminism, timing and data abstractions, and whether each difference is an
over- or under-approximation. An omitted source path is an unresolved assumption,
not a blank cell.

## 3. State a property-specific preservation obligation

For safety, justify that each relevant implementation behaviour is represented by
the model abstraction. For liveness, additionally justify scheduler/fairness,
divergence, progress, and stuttering assumptions. One simulation relation does not
automatically transfer every temporal property. Name the initial-state, operation,
and environment obligations separately.

## 4. Gather reviewable evidence

Attach evidence for initial states, each transition class, input validation, and
environment assumptions. A refinement argument or checked obligation is stronger
than trace replay; instrumentation, tests, monitors, and differential runs can find
mismatches but do not establish universal correspondence. Keep reviewer decisions
and unresolved assumptions in the dossier.

## 5. Classify a counterexample or mismatch

Classify it as an implementation defect, model defect, abstraction artefact, or
unresolved mismatch. Validate a concrete execution when possible. Until the
classification is resolved, the model verdict must not be presented as an
implementation claim.

## 6. Revalidate on change

Invalidate or explicitly revalidate the dossier when the implementation revision,
model, properties, mappings, environment, scheduler, fairness, memory model, or
bounds change. Keep the old dossier as evidence; do not edit it in place to imply
that it covered a different revision.

## Reporting rule

Write the **model verdict** and the **implementation claim** in separate fields. A
model verdict may be `verified` with `exhaustive` evidence while the implementation
claim remains `not established` or `blocked`. Only make an implementation claim when
the dossier names its basis and remaining obligations.

## Trace-ledger library profiles

The plugin also documents offline, caller-byte Go APIs under `modelcheck/trace` for
runtime-span and contract declarations, model hand-off dossiers, and change-impact
analysis. These APIs produce ledger, dossier, or review-obligation records; they do
not add `mcd` CLI commands or MCP tools, and they do not change the `mcd-trace-ledger/1`
contract or promote evidence into an implementation claim. See the [supported
adapter matrix](../../../docs/trace-adapter-matrix.md), [conformance dossier](../../../docs/conformance-dossier.md),
and [trace-impact workflow](../../../docs/trace-impact.md) for their exact inputs,
bounds, and non-claims.

## Artifacts

Start from `assets/conformance-dossier-template.md`. The worked
`examples/conformance-turnstile/` example deliberately mismatches the source and
model; it shows that the process blocks an implementation claim instead of treating
a model verdict as program verification.
