# Logika review: Issue 18 Stages 4–7 trace claims

## Scope and method

This is a logic review of the claims in the Issue 18 Stage 4–7 trace and protocol
material, not a release validation and not a model-checking run. I read the
current files, rather than relying on summaries:

- `docs/trace-adapter-matrix.md`
- `docs/conformance-dossier.md`
- `docs/trace-impact.md`
- `docs/runtime-spans.md`, `docs/openapi-profile.md`, `docs/asyncapi-profile.md`,
  `docs/protobuf-profile.md`, and `docs/pact-profile.md`
- the Issue 18 and protocol-artifact documents that define the evidence vocabulary
- `features/g9-*.feature`
- `engine/trace/protocol/{runtime.go,contract_common.go,openapi.go,asyncapi.go,pact.go}`
- `engine/trace/dossier/dossier.go`
- `engine/trace/impact/impact.go`
- the relevant G9 step implementations

The review distinguishes formal validity (whether a conclusion follows from its
stated premises) from material truth (whether the premises and implementation
actually hold). No full test suite, release gate, security scan, remote model
confirmation, or publication check was run.

## Verdict

**Verdict:** logic is correct as written; no confirmed logical errors found (0
critical, 0 disputed).

The documents consistently preserve the key distinctions:

1. caller-supplied declaration, source, decoded, runtime, grammar, and contract
   records are evidence at their stated levels, not automatic correctness claims;
2. dossier links are caller-declared model hand-off structure, not proof that the
   source implements the requirement;
3. a model status is about a declared finite model under explicit assumptions;
4. impact analysis produces review obligations and only promotes a verdict when
   exact identities, dependencies, checker/registry provenance, mapping approval,
   and review coverage are present; and
5. the Stage 4 matrix expressly labels the new APIs “implemented,
   release-unverified candidates.”

These are not merely disclaimers. The source enforces the main boundaries: the
protocol path is offline and all-or-nothing; `dossier.Build` forces
`finite_model_only=true` and `implementation_conclusion="not_assessed"`; and
`impact.verdictState` rejects stale, incomplete, or unmapped verdicts.

This conclusion is a logical assessment of claims and code structure. It does
**not** establish that the implementation is bug-free or that any feature,
release, or formal-model result has passed its external gates.

## Structure of the argumentation

The central argument has four layers:

1. **Observation/contract premise.** The adapters receive caller-owned bytes or
   Go values, validate bounded schemas and identities, and emit a canonical
   ledger/dossier/report. Paths are provenance anchors; they are not opened.
2. **Evidence-level premise.** Each output labels what was directly represented
   and records unresolved material as a gap. The documents explicitly deny
   inference of reachability, execution, transport behavior, source linkage,
   conformance, or implementation correctness where the profile cannot establish
   it.
3. **Formal-model premise.** A dossier may contain a supplied model status, but
   its intake must declare bounds, atomicity, environment, scheduler/fairness,
   property scope, and references. The result is permanently marked finite-model
   only.
4. **Promotion premise.** Impact analysis treats exact immutable IDs and digests
   as necessary dependencies. A verdict is promotable only after exact current
   provenance and applicable review coverage. Therefore a clean impact report or
   a verified finite-model status cannot, by itself, become an implementation
   conformance claim.

The inference is valid because the conclusion is deliberately weaker than the
premises: it is a statement about a supplied bounded record and its obligations,
not about the whole implementation. The material truth of the premises remains a
separate empirical matter.

## Error table

| No. | Location / claim | Error (Russian + Latin) | Why it is or is not an error | Correct handling |
|---:|---|---|---|---|
| — | `docs/trace-adapter-matrix.md`, status table and boundary section | No confirmed error | The potentially dangerous word “implemented” is qualified by “release-unverified candidate,” while the same document expressly excludes full-suite, review, security, packaging, publication, and implementation-conformance conclusions. This avoids **подмену тезиса (ignoratio elenchi)**. | Preserve the qualification whenever the status is repeated. Do not shorten it to “verified,” “released,” or “conformant.” |
| — | `docs/conformance-dossier.md`, finite-model result and `dossier.Build` / `Validate` | No confirmed error | A supplied `verified` status is explicitly restricted to the declared finite model and its assumptions. The source enforces `FiniteModelOnly` and `ImplementationConclusion == "not_assessed"`; it does not use the model result as a source-implementation conclusion. | Keep model verdict and implementation conclusion as separate predicates. |
| — | `docs/trace-impact.md`, verdict promotion gates and `impact.go` | No confirmed error | Exact identity, digest, checker, registry, mapping, and review conditions are conjunctive gates. The code marks stale or incomplete results as obligations and emits `verdict-unpromoted`; it does not infer an association from names or paths. This avoids **ложную аналогию (falsa analogia)** and **предвосхищение основания (petitio principii)**. | Treat a clean supplied snapshot as “no recognized obligations,” not as conformance. |
| — | G9 dossier, impact, and trace-corpus scenarios | No confirmed error | The scenarios state the same boundary that the production types enforce: no fabricated links, exact review applicability, finite-model-only results, stale verdicts, and no implementation pass. The scenarios are tests of the specified behavior, not an argument that the production implementation is thereby universally correct. | Report scenario results only as test observations and keep their scope bounded. |
| — | `docs/runtime-spans.md` and protocol profile documents | No confirmed error | Caller assertions are not silently upgraded: runtime spans remain runtime evidence, contract profiles remain contract evidence, references remain unexpanded or gap-only, and accepted declarations carry explicit boundary gaps. The documents avoid **от сказанного с ограничением к безусловному (a dicto secundum quid)**. | Preserve the evidence level and every relevant gap in downstream reports. |

## Hidden premises

The argument is valid only with the following premises made explicit:

1. **The reviewed files are the actual current tree.** The report does not infer
   their contents from a prior review or from generated summaries.
2. **The input identity is truthful enough for the stated contract.** A matching
   caller-supplied digest proves byte equality with the supplied bytes; it does
   not prove that the bytes came from the claimed capture, repository, collector,
   checker, or run registry. The documents generally state this limitation.
3. **The declared model represents the intended system and requirements.** The
   dossier validates that required declarations and links exist and match exact
   IDs; it does not validate semantic adequacy of the model-to-source mapping.
4. **The caller’s review decision is genuine and competent.** Exact revision and
   scope matching prevents accidental carry-forward, but identity matching alone
   cannot authenticate the reviewer or establish the quality of the rationale.
5. **The finite bounds and scheduler/fairness assumptions are adequate for the
   question.** A verified result under inadequate bounds is still only a result
   under those bounds. This is a limitation of material adequacy, not a formal
   inference error in the documents.
6. **A clean impact snapshot is complete.** The analyzer can report only the
   artifacts, evidence, dossiers, changes, and verdicts supplied to it. Omitted
   records cannot be discovered by exact-ID analysis.
7. **The source code matches the documented contract.** The code inspection gives
   concrete support for the boundary, but source reading alone does not replace
   execution, tests, security checks, or release validation.

The first, second, third, fourth, and sixth premises are especially important:
weakening any of them changes the claim from a valid conditional statement into
an unsupported implementation or provenance assertion.

## Strength of non-deductive conclusions

There are no material causal or statistical generalizations in the reviewed
claims. The important conclusions are conditional and procedural rather than
inductive:

- “A supplied digest is checked” is an implementation/contract claim, not a
  conclusion that the artifact is authentic.
- “A matching exact review decision applies” is a rule of the dossier/impact
  contract, not a conclusion that the reviewer’s judgment is correct.
- “No findings” is a conclusion about the supplied snapshot and the analyzer’s
  recognized obligation rules. It is not evidence that no real-world obligation
  exists.
- “`verified`” is a finite-model status. It is not a universal theorem about an
  unbounded implementation and cannot be promoted by lexical similarity or a
  clean report.
- G9 scenarios provide examples and regression checks. Even if they pass, that
  is an observation about those scenarios, not an inductive proof over all inputs.

No post hoc, hasty universalization, circular proof, or false dilemma is needed
to obtain the documents’ stated conclusions. The main foreseeable error is an
external reader committing the prohibited non sequitur:

> the adapter accepted a declaration / the finite model was verified / the
> impact report is clean; therefore the implementation conforms.

That inference is invalid because its conclusion introduces a new predicate
(implementation conformance) absent from the premises. The documents and source
avoid it by retaining declarations-only gaps, exact review obligations,
`finite_model_only`, and `implementation_conclusion: not_assessed`.

## Evidence classification and review limits

| Evidence kind | What the reviewed material supports | What it does not support |
|---|---|---|
| Observation | The text and current source contain the stated boundary checks and labels. | That all paths have been executed or that external systems supplied the data. |
| Contract | The documented API/schema rules and source validation behavior define accepted/rejected inputs and output semantics. | General protocol conformance or correctness of an implementation using the contract. |
| Source | The inspected Go functions implement the visible guards and status boundaries. | Absence of defects without tests and broader review. |
| Formal-model result | A supplied result can be represented with explicit finite bounds and assumptions. | Correctness of the source implementation or an unbounded system. |
| Review decision | An exact target/scope decision can be carried into the dossier/impact workflow. | Authentication or substantive correctness of the human decision. |

## Final disposition

No correction to the reviewed source, tests, or existing documentation is
recommended on logical grounds. The safe disposition is to retain the current
qualifiers and to treat this report itself as a logic review only. Release gates,
full tests, security checks, cross-review, and bounded remote model confirmation
remain outside this review and are not claimed as passed here.
