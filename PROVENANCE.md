# Provenance and literature

This file records the sources that informed the `model-check` skill, the finite
explicit-state engine, its property vocabulary, and the evaluation workflow. A
citation describes background and design provenance; it is not a promise that the
current engine implements every technique in the cited work.

## Public references

- **E. M. Clarke, T. A. Henzinger, H. Veith, and R. Bloem (eds.), _Handbook of
  Model Checking_ (Springer, 2018).** This is the modern handbook cited for the
  broad model-checking taxonomy, abstraction, temporal logic, and verification
  algorithms. [Springer](https://link.springer.com/book/10.1007/978-3-319-10575-8)
- **E. M. Clarke, O. Grumberg, and D. A. Peled, _Model Checking_ (Springer,
  1999).** This earlier monograph is cited separately for the classic
  Clarke–Grumberg–Peled presentation of transition systems and temporal model
  checking. [Springer](https://link.springer.com/book/10.1007/978-3-662-22679-3)
- **G. J. Holzmann, _Design and Validation of Computer Protocols_.** This is the
  SPIN/Promela-oriented reference for executable protocol models, partial-order
  reasoning, and counterexample-driven validation.
  [Author's page](http://spinroot.com/gerard/popd.html)
- **G. J. Holzmann, “The Model Checker SPIN,” _IEEE Transactions on Software
  Engineering_ 23(5), 1997.** [DOI](https://doi.org/10.1109/32.588521)
- **C. Baier and J.-P. Katoen, _Principles of Model Checking_ (MIT Press,
  2008).** This is the primary reference for LTL/CTL terminology, transition
  systems, and the distinction between safety and liveness properties.
  [MIT Press](https://mitpress.mit.edu/9780262026499/principles-of-model-checking/)
- **SPIN project.** The project homepage is the authoritative public entry point
  for SPIN releases, documentation, and examples.
  [spinroot.com](https://spinroot.com/)
- **U. Karpov, _Model Checking: Verification of Concurrent Systems_** (Russian).
  The Google Books record is used as the public bibliographic reference for the
  Russian-language terminology and examples.
  [Google Books](https://books.google.com/books/about/MODEL_%D0%A1HECKING_%D0%92%D0%B5%D1%80%D0%B8%D1%84%D0%B8%D0%BA%D0%B0%D1%86%D0%B8%D1%8F.html?id=xpui56eRsHgC)

## Monorepo-only source notes

The following notes and extracted reading materials are maintained in the full
`model-check` monorepo, not copied into this standalone plugin release. Their
paths are intentionally shown as source provenance, not as files users should
expect to find beside this file:

- `books-md/1clarke_edmund_handbook_of_model_checking/` and
  `model-check-skill-notes/01-handbook-of-model-checking.md`
- `books-md/Design_and_Validation_of_Computer_Protocols_-_Gerard_Holzmann/` and
  `model-check-skill-notes/02-design-and-validation-of-computer-protocols.md`
- `books-md/Karpov_U._Model_checking/` and
  `model-check-skill-notes/03-karpov-model-checking.md`
- `books-md/_principles_of_model_checking/` and
  `model-check-skill-notes/05-principles-of-model-checking.md`
- `books-md/Verification Protocols Web - tok/` and
  `model-check-skill-notes/04-verification-web-services.md`
- `books-md/velder_verification_posobie_nauka/` and
  `model-check-skill-notes/09-verification-of-automata-programs.md`
- `books-md/graph-encoded-tuple-set-for-spin/`,
  `books-md/graph encoded tuple set for SPIN/`, and
  `model-check-skill-notes/06-state-space-compression-gets.md`
- `books-md/lect01-lect09.md` and
  `model-check-skill-notes/07-lectures-01-09.md`
- `books-md/modelchk/` and `model-check-skill-notes/08-modelchk.md`
- `model-check-skill-notes/10-cross-book-synthesis.md`,
  `11-skill-requirements.md`, `12-skill-development-plan.md`,
  `13-coverage-matrix.md`, and `14-skill-building-plan.md`

The executable references shipped with this plugin are under
`skills/model-check/references/`. They are the authoritative description of what
this release actually accepts and reports. When a local note and the current
engine disagree, the engine tests and shipped references win.

## What the partial-order reduction (0.2.0) rests on

`mcd check --por` and the `por` parameter of `mc_check` reduce the safety search
by ample sets. The conditions they implement (a non-empty set, a dependency
condition, invisibility with respect to the properties, and a cycle proviso on
the depth-first stack) are those of the presentations of partial-order reduction
in the Handbook of Model Checking (chapter 6;
`model-check-skill-notes/01-handbook-of-model-checking.md`) and in Baier–Katoen
(chapter 8; `model-check-skill-notes/05-principles-of-model-checking.md`).
`engine/explore/por.go` also cites Clarke–Grumberg–Peled (chapter 10); that
book has no note in the monorepo. The
independence of a send and a receive on a buffered channel, the treatment of the
program counters that Promela's termination order reads, and the state-dependent
condition for channel operations are this project's own arguments, not taken from
those books; each is stated in `engine/explore/por.go`.

The sources do not guarantee this implementation. What the release asserts is what
its tests confirm. On random models (`engine/explore/por_random_test.go`) the
reduced search is compared with the full one for the reachability of a model error
and, for the models on which both searches finish (most of them; a model whose
error is reachable is compared only on that error), for the status and evidence of
every property, the set of states without an enabled move, and the replay of every
counterexample as a run of the model. On the Promela models of
`engine/testdata/promela`, `engine/testdata/corpus2` and the SPIN corpus
(`Promela - examples/`) that the frontend accepts and that both searches finish
(`engine/por_corpus_test.go`) it is compared for the status and evidence of every
property and for the reachability of a model error, and it must never store more
states than the full search, and exactly as many when the reduction is refused;
that test does not compare the states without a move or replay counterexamples.
The analysis was also mutation-tested: the mutants (chosen by hand) and the
survivors that exposed gaps in the tests are recorded in
`steps/perf2-confirmation.md` and `steps/perf4-confirmation.md`; the scratch
harness that ran them is not part of the release. The reduction preserves the
safety properties only; it is refused, with the reason in the report, for atomic
sequences, rendezvous and dynamic channels, process creation, a model that reads
the process table (`_nr_pr`; the frontend folds `_pid` to a constant, so it does
not count), `timeout`, `provided`, breadth-first search and temporal properties.

## Updating the index

When a source changes the supported model formats, property semantics, result
statuses, or release workflow, update this file and the corresponding public
README/references in the same change. Do not use a citation to fill a capability
gap: unsupported timed, probabilistic, symbolic, unbounded, or game semantics
must remain explicitly outside this plugin's scope.
