# Issue #11 reconciliation table

**Schema:** `issue-11-reconciliation/1`  
**Table revision:** `1.0.0`  
**Parent:** [issue #11](https://github.com/viktor-drobek/model-check-plugin/issues/11)  
**Source reviewed:** issue body and owner comments retrieved on 2026-10-14; the
versioned roadmap is [issue-11-plan.md](issue-11-plan.md).

This is the M0a baseline, not an issue-closure record. A row is `mandatory` unless a
tracker-owner decision linked from the row explicitly changes the parent issue's
completion scope. No implementer decision converts an obligation into `deferred` or
`out-of-scope`.

| Obligation ID | Explicit issue #11 obligation | Disposition | Delivering milestone or linked issue | Tracker-owner decision |
|---|---|---|---|---|
| I11-1 | Finite explicit-state scalability, reductions, exact storage, and GPU or hybrid evaluation | mandatory | [M5 #29](https://github.com/viktor-drobek/model-check-plugin/issues/29) | — |
| I11-2 | Promela compatibility and semantic differences from SPIN | mandatory | [M1a #17](https://github.com/viktor-drobek/model-check-plugin/issues/17), [M1b #20](https://github.com/viktor-drobek/model-check-plugin/issues/20), [M1c #21](https://github.com/viktor-drobek/model-check-plugin/issues/21) | — |
| I11-3 | Petri frontend restrictions | mandatory | [M2 #22](https://github.com/viktor-drobek/model-check-plugin/issues/22), [M2T #26](https://github.com/viktor-drobek/model-check-plugin/issues/26) | — |
| I11-4 | Timed, probabilistic, symbolic, and unbounded verification | mandatory | [M3.0 #23](https://github.com/viktor-drobek/model-check-plugin/issues/23), [M3.1 #24](https://github.com/viktor-drobek/model-check-plugin/issues/24), [M3.2 #25](https://github.com/viktor-drobek/model-check-plugin/issues/25), [M3.3 #26](https://github.com/viktor-drobek/model-check-plugin/issues/26), [M3.4 #27](https://github.com/viktor-drobek/model-check-plugin/issues/27) | — |
| I11-5 | Complete temporal parallelism | mandatory | [M4 #28](https://github.com/viktor-drobek/model-check-plugin/issues/28) | — |
| I11-6 | Resource exhaustion must not create proof | mandatory | [M6 #30](https://github.com/viktor-drobek/model-check-plugin/issues/30), repeated for every new frontend and backend | — |
| I11-7 | Model-to-implementation conformance process | mandatory | [M6 #30](https://github.com/viktor-drobek/model-check-plugin/issues/30) conformance and publication maintenance | — |

## Arithmetic alternative selection

| Field | Recorded value |
|---|---|
| Milestone | M1b — arithmetic and semantic compatibility policy |
| Selection | `pending-owner-decision` |
| Current default | Strict domain-overflow handling remains the shipped default. |
| Required before M1b closure | The tracker owner must select the opt-in arithmetic alternative or approve a scope change against issue #11. |
| Scope decision link | — |

## Preserved completed slices

The following owner comments are evidence for completed slices that M0 must preserve,
not a declaration that their broader workstreams are closed: deterministic parallel
CTL graph construction, resource-control follow-ups, the conformance workflow and
mismatch example, and the guarded `unless` slice. M1a remains linked to open issue
#17; the other mandatory rows remain open until their own acceptance evidence and
closure gates pass.

## M0a limitation

The matrix and its conformance checks are delivered locally by this baseline. The
tracker now contains the narrow child issues linked above. Every future deferral or
out-of-scope decision remains owner-governed work. Until the arithmetic selection and
all mandatory milestone evidence are recorded, M0 and issue #11 remain open.
