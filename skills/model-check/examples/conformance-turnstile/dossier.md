# Conformance dossier: deliberate turnstile mismatch

## Identity and scope

- Implementation revision: `sha256:29bcbb7a75d9ff2d0620a1e55fd366ab46c8405b832eefc21f9983240a14aa48`
- Model: `model.pml`, `sha256:af150058d77471e7ae4d6ebc986e2f45ac0c5e2178ae93fc07996acefb171d84`
- Property: accepted entries increase `passages`
- Model verdict: `assert` and `deadlock` are `verified` with `exhaustive` evidence
  on the complete three-state graph, using `mcd 0.3.4` and `mcd check -promela model.pml -no-timing`
- Implementation claim: **blocked**

## Model and implementation

The model transition increments `passages` for every accepted entry. The
implementation's `AcceptEntry` accepts an entry but leaves `passages` unchanged.
The source-to-model mapping therefore fails for the only operation in scope.

## Mapping and abstraction

| Source element | Model element | Difference | Direction | Evidence |
|---|---|---|---|---|
| `Counter.AcceptEntry` | `passages++` | source omits increment | under-approximation | source review |

## Preservation obligations

The safety preservation obligation is blocked: an implementation execution exists
that has no corresponding model transition with the promised post-state. No model
verdict may be reported as an implementation verification claim.

## Evidence and review

This is a **deliberate mismatch**. The required review outcome is **blocked**, not a
workaround or a changed property. Fix the source or revise the model and mapping,
then rerun the model check and the conformance review.

## Revalidation

Revalidation requires a pinned implementation revision, a model hash, the exact
property result, updated mapping evidence, and an independent reviewer decision.
