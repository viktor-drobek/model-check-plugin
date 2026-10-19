# Conformance dossier

`engine/trace/dossier` builds a deterministic, in-memory hand-off record for a
**finite model**. It makes the inputs to model construction reviewable; it does
not generate a model, execute a check, inspect source code, read files, use a
network, or start processes.

The package is separate from `mcd-trace-ledger/1`. It neither rewrites a trace
ledger nor changes the meaning of `mcd` statuses or trace evidence levels.

## API

```go
import "modelcheck/trace/dossier"

document, err := dossier.Build(dossier.Input{
    Brief: brief, Intake: intake,
    Decisions: decisions,
    Links: links, // caller-declared structural relations
    Gaps: gaps,   // caller-known unresolved obligations
    Result: result,
})
jsonBytes, err := dossier.JSON(document)
```

`Build` returns `mcd-conformance-dossier/1`. `JSON` validates, sorts the
ID-bearing content lists, uses fixed indentation, and appends a newline.
The serialized provenance lists must already be in their builder-produced
ID order; validation rejects reordered provenance rather than repairing it.
Repeating a build with the same logical input produces identical bytes,
including when the caller presented its input slices in a different order. Neither function mutates
caller-owned slices. Determinism helpers must propagate a failed build or JSON
encoding operation; an empty byte slice is not a successful canonical result.

All data is supplied as Go values. The package has no path, environment,
network, subprocess, or file-system API.

`Input.Links` and `Input.Gaps` are optional arrays, but they are never
invented or discarded. `Build` copies valid caller links and gaps into the
returned document. Generated gap IDs reserve caller-supplied gap IDs and use a
stable collision-avoiding derivation, so caller ordering cannot change the
result. Generated gap kinds (`requirement_trace_missing`, `relation_missing`,
`review_missing`, and `review_target_mismatch`) are builder-owned and are
rejected when supplied by callers. Validation also rejects a generated gap
whose obligation is stale after the corresponding relation or decision is
present. Generated gaps record missing required relation kinds, a missing
requirement trace for a scoped requirement, or the absence/mismatch of a
reviewer decision.

Completed-document validation checks generated-gap references as follows:

| Kind | Required reference domain |
| --- | --- |
| `requirement_trace_missing` | A requirement ID in `Intake.PropertyScope` |
| `relation_missing` | A relation kind required for the current dossier |
| `review_missing` | Exactly `Brief.ID` |
| `review_target_mismatch` | A supplied reviewer decision ID not present among the applicable `ReviewerDecisions` |

Generated references must be stable identifiers; surrounding whitespace is not
accepted. Validation also rejects duplicate generated obligations with the same
`(Kind, Reference)` pair, even when their gap IDs differ.

The first three kinds are also rejected when the corresponding relation or
decision already satisfies the obligation. A `review_target_mismatch` gap is
stale when its own referenced decision is applicable; an applicable decision
with another ID does not erase a mismatch. `Build` serializes a
`review_provenance` ledger containing deterministic fingerprints for every
applicable decision and the IDs of every non-applicable decision. Validation
requires the current applicable decisions to match those fingerprints exactly,
and requires one mismatch gap for every retained non-applicable ID. When the
provenance is unchanged, removing an applicable decision or substituting an
arbitrary entity ID is rejected, including after a JSON round trip.
This is an internal consistency check, not authentication or historical audit
proof: the decisions, unkeyed fingerprints, links, and gaps all reside in the
same caller-editable document. A coordinated rewrite of those records can pass
validation. Consumers that need tamper evidence must independently retain and
verify a trusted external commitment to the original document; this package
does not create or verify such a commitment. A decoded document without
`review_provenance` is unverifiable and is rejected rather than silently
accepted. `Build` creates mismatch gaps from the supplied decisions; callers
cannot inject these reserved kinds through `Input.Gaps`. Consistent generated
gaps remain valid in a completed `Document`.

Validation also recomputes every builder-owned generated gap and compares its
ID, kind, reference, and reason exactly. It recomputes the deterministic
coverage ID for each property-scope requirement as well; renaming a coverage
entry or changing a generated gap's reason is rejected.

## Required hand-off declarations

A `dossier.Brief` pins the hand-off to a stable brief ID and an immutable
target digest (`sha256:` followed by exactly 64 lowercase hexadecimal
characters), reviewed scope IDs, target IDs, requirement references,
trace-node references, and concise algorithm/protocol descriptions.

A `dossier.ModelIntake` must explicitly provide all of the following:

- one or more named finite `StateBounds` with limits from 1 through 1,000,000;
- atomicity declarations;
- modelled-environment declarations;
- scheduler/fairness declarations;
- a property scope made only of the brief's requirement IDs;
- anchored model-transition, property, and assumption references.

A reference has a stable ID plus a safe, non-private anchor. Missing bounds,
assumptions, identifiers, anchors, or any required declaration rejects the
hand-off. The API caps each input collection at 128 entries. Stable IDs are
limited to 128 bytes; free-text metadata is limited to 512 bytes and must be
well-formed UTF-8. Links use the same effective 128-entry limit; the implementation
does not support a larger quadratic link limit. Caller gaps and generated gaps
share a combined 128-entry limit, so 128 caller gaps are rejected if the build
would add another generated gap. It rejects control characters, selected obvious secret-like
metadata, absolute/home-relative/path-traversal anchors, and file-URI anchors.
These checks are heuristics, not a general privacy or secret-scrubbing
guarantee; callers must scrub and admit only public metadata.

Free-text fields also reject a leading `/` or `~` after trimming whitespace,
any backslash, a leading lowercase `file:`, and a drive-like prefix such as
`C:/`. This conservative policy can reject explanatory prose as well as paths.
The free-text `file:` heuristic is case-sensitive; anchor checks reject file
URIs case-insensitively. Do not rely on these checks to remove private paths.
The control-character policy rejects C0 characters and DEL. Other well-formed
Unicode, including C1 controls, bidirectional formatting characters and line
separators, is not filtered. Consumers displaying caller-controlled text must
apply their own safe-rendering policy; UTF-8 validity is not a display-safety
guarantee.

The serialized `review_provenance.applicable` and
`review_provenance.non_applicable_ids` lists are each capped at 128 entries.
Their combined population, and the current applicable reviewer decisions plus
the non-applicable IDs, are also capped at 128 entries. Provenance IDs are
checked for identifier syntax and uniqueness before fingerprints or generated
review gaps are derived. Outer collection bounds and reviewer-decision field
bounds are admitted before provenance fingerprints are computed, so nested
decision fields cannot bypass the bounded offline-data contract through the
provenance path.

Secret-marker checks intentionally use different policies for identifiers and
free text. Identifier validation rejects the `bearer` marker as an unsafe
substring, while text validation rejects the token-like phrase `bearer `;
descriptive text such as `bearer-based` is therefore allowed. This is a
deliberate text-vs-ID policy, not an accidental inconsistency. Free-text checks
also reject conservative substrings such as `authorization` and `secret`, so
ordinary descriptions such as `OAuth authorization code flow` and `Shamir secret
sharing` are rejected too. Rephrase public descriptions before submission; these
checks do not distinguish explanatory prose from credentials.

## Reviewer decisions and gaps

A `ReviewerDecision` contains only an ID, target revision, scope IDs, target
IDs, and disposition. It is copied into `Document.ReviewerDecisions` only if
all three target facts exactly match the brief:

1. the target revision;
2. the set of scope IDs; and
3. the set of target IDs.

Order does not matter, but no subset, superset, approximate label match, or
revision similarity is accepted. If no decision applies, the dossier records a
`review_missing` gap. If supplied decisions do not match, it records one
`review_target_mismatch` gap per decision, for example
`gap.review.review.r1.target_mismatch`. No reviewer-to-target link is invented
for a mismatched decision.

## Explicit relations and coverage

The caller, not the dossier builder, declares the hand-off graph through
`Input.Links`. The closed relation vocabulary is:

| Kind | Source category | Target category |
| --- | --- | --- |
| `requirement_trace` | declared requirement | declared trace node |
| `trace_transition` | declared trace node | declared model transition |
| `transition_property` | declared model transition | declared property |
| `assumption_transition` | declared assumption | declared model transition |
| `review_target` | applicable reviewer decision | the brief ID |

Every link ID and endpoint is validated. A link cannot target an undeclared
artifact, substitute a similarly named category, or link a reviewer decision
whose immutable revision, scope, or target IDs do not match.

`trace_transition`, `transition_property`, and `assumption_transition` are
required relation kinds for a complete hand-off. When one is absent, `Build`
adds a stable `relation_missing` gap such as
`gap.relation.trace_transition.missing`; it does not choose the first trace
node, transition, property, or assumption and does not make any relation up.
For every requirement in the property scope, at least one explicit
`requirement_trace` relation with that requirement as its source is required.
When one is absent, `Build` adds a deterministic
`requirement_trace_missing` gap such as
`gap.requirement_trace.req.delivery.missing`. This obligation concerns only
the requirement-to-trace relation; it does not claim that the trace reaches a
transition or property.
When an applicable reviewer decision exists, a missing `review_target` link is
reported the same way. Ordinary caller-provided gaps are retained alongside
these generated gaps. Requirement coverage is still reported from the explicitly
declared property scope. Each property-scope requirement's coverage entry must
contain exactly the deduplicated set of explicitly declared
`requirement_trace` targets; omitting an explicit target is invalid, as is
adding a non-explicit target.

Canonical output normalizes every collection to a JSON array, including empty
`links`, `gaps`, and `Coverage[].Related`; it never uses JSON `null` for an
empty collection.

Links catalogue caller-declared model hand-off structure only. They are not
evidence that source code implements a requirement.

## Status and evidence boundary

`ModelResult.Status` accepts the existing `mcd` values: `verified`,
`violated`, `inconclusive`, `unknown`, `not-executed`, and `invalid-model`.
`not-executed` must have no verdict. Other statuses need an explicit supplied
finite-model verdict.

Every returned dossier states:

```json
{
  "finite_model_only": true,
  "implementation_conclusion": "not_assessed"
}
```

Accordingly, even a supplied `verified` result says only that the declared
finite model and its stated bounds, scheduler/fairness, environment, and other
assumptions produced that result. It is not a conclusion about a whole program,
an unbounded system, or a source implementation.
