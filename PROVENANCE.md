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
book has no note in the monorepo. The arguments that apply those conditions to
this engine are this project's own, not taken from those books, and each is
stated in `engine/explore/por.go` and in `steps/perf6-plan.md`: the independence
of a send and a receive on a buffered channel, the treatment of the program
counters that Promela's termination order reads, the state-dependent condition
for channel operations; and, from the step that widened the reduction, the
*macro-step* (an atomic sequence, an edge with its d_step continuation, or one
edge) as the unit that is commuted, with the footprint of every edge it can be
made of and a cycle proviso that follows each sequence to the stored states it
ends in; the equivalence of two stored states that differ only in the exclusive
byte of an atomic sequence (the byte is read in two places, and a stored state
has no holder that can move), which the proof carries a path through and the
oracles compare up to; and the live-process table as one cell that every `run`
and every end of a process writes, with the program counter of a pool member at
its dormant location as the cell a `run` writes and reads. These are checked
against the engine, not cited from a source: nothing here is taken from SPIN's
sources, and SPIN's own reduction is not what the engine reproduces (it is run
without reduction, `-DNOREDUCE`, as a witness).

The sources do not guarantee this implementation. What the release asserts is what
its tests confirm. Random models of nine shapes (a base generator, atomic
sequences, loops through atomic chains, process creation with the table, atomic
sequences around process creation, `provided`, two that read globals another
process writes in the places the others fill with constants or locals, and one that
makes the table model the frontend gives a model that reads `_nr_pr` and has no `run`;
`engine/explore/por_gen_test.go`)
are run through four oracles. The verdict differential (`por_oracle_test.go`,
`por_random_test.go`) compares the reduced search with the full one for the
reachability of a model error (an evaluation error, a domain overflow, an
exhausted process pool) and, for the models on which both searches finish, for
the status and evidence of every property, the set of states without an enabled
move (up to the exclusive byte), the stored states of the reduced search being
states of the full one, the equality of the counts when the reduction is
refused, and the replay of every counterexample as a run of the model. The audit
of the ample sets (`por_audit_test.go`) runs, at every stored state of the full
graph and for every process the analysis calls eligible there, the model itself:
the macro-steps of the process must be the same after any sequence of up to two
macro-steps of the others, must commute with it, and must not change a property;
it reads nothing of the analysis but the eligibility tables, so, on the shapes the
generators make, it sees a hole in the footprints in states the reduced search never
visits (hand-built shapes that no generator makes are pinned by directed tests only:
`steps/perf6-confirmation.md`, "What the oracles can and cannot see"). The acyclicity
audit (`por_acyclic_test.go`) records the choices of the reduced search and
checks that the states not expanded in full contain no cycle. On the Promela
models of `engine/testdata/promela`, `engine/testdata/corpus2` and the SPIN
corpus (`Promela - examples/`) that the frontend accepts and that both searches
finish (`engine/por_corpus_test.go`) the reduction is compared for the status and
evidence of every property and for the reachability of a model error, and it must
never store more states than the full search, and exactly as many when the
reduction is refused; that test does not compare the states without a move or
replay counterexamples. `engine/tools/pandiff` sets a fuzzer of Promela models
(`atomic`, `d_step`, `run` from `init`, `_nr_pr`, a buffered channel) and the
verdicts of the corpus against `pan -DNOREDUCE`. The analysis is mutation-tested
by a harness that is part of the repository (`engine/cmd/pormut`, mutants in
`engine/tools/pormut/mutants.json`) and whose results, with the survivors and
what each shows, are in `steps/perf6-confirmation.md`; the earlier mutants of
steps 2 and 4 are recorded in `steps/perf2-confirmation.md` and
`steps/perf4-confirmation.md`. The reduction preserves the safety properties
only; it is refused, with the reason in the report, for rendezvous channels,
channels named by a value, `timeout`, `provided`, breadth-first search and
temporal properties, and for two shapes of process creation that the frontend
never emits (see `steps/perf6-plan.md`); a model that reads `_nr_pr` is not
refused but reduced where the table rules allow (`nrpr.pml`: 31 states, 21 with
the reduction). The engine and SPIN agree on `_nr_pr` in a model that reads it and
creates no process by `run` (the `_nr_pr` fix, `steps/fix-nrpr-confirmation.md`: such a
model gets the live-process table too, and a process that ends leaves it); step 6
recorded a difference there that no longer exists. Under a never claim or an `ltl`
formula `pan` counts the claim in `_nr_pr` and the engine does not, a documented
divergence (`skills/model-check/references/promela-subset.md`,
`engine/testdata/spin-divergence/`). The integration of the branches added
generators for models that read `_nr_pr` with `active` processes and for loops
inside atomic and d_step blocks to the oracles and the fuzzers
(`steps/integration-0.3.0-notes.md`).

## What `--fairness weak` rests on

Weak fairness is the definition of Baier–Katoen (chapter 3, "process fairness":
an action or process that is continuously enabled from some point on occurs
infinitely often; `model-check-skill-notes/05-principles-of-model-checking.md`),
decided on the synchronous product of the model and the claim by the n + 2 copies
of Holzmann's fairness rules as `pan -f` implements them (SPIN book, "fairness";
`model-check-skill-notes/14-skill-building-plan.md` §4.2). A system that has no move
is extended by a state that repeats for ever (Baier–Katoen §3.1, the stop state with a
self-loop), which is what `fairness.md` §6b calls the stutter extension. Where
`pan -f` and the definition differ, the engine follows the definition (`fairness.md`
§6b); the cases are documented, and the scenarios of `features/g4-ltl.feature` pass
only while both tools answer exactly as documented.

The sources do not guarantee this implementation. What the release asserts is what
its tests confirm: an independent decision procedure by strongly connected components
(`engine/explore/weakfair_oracle_test.go`, on random models and on generated Promela
models), hand-encoded graphs checked by a separate Python program that uses no engine
code (`engine/testdata/weakdecision/`), the differential against `pan`, and a mutation
test of the rules (`steps/fix-weakfairness-mutants.py`). The records, with what was
and was not checked, are `steps/fix-weakfairness-confirmation.md`.

## What the parallel search (`--workers`) rests on

`mcd check --workers N` and the `workers` parameter of `mc_check` search the safety
properties breadth-first, level by level, over a visited set split into partitions
by a fixed hash, each partition written by one worker at a time. The design, the
rule that makes every result independent of the worker count, and the argument for
a deterministic partitioned set instead of a shared lock-free table are this
project's own and are written down in `steps/perf5-plan.md` and
`steps/perf5-confirmation.md`; they do not rest on a published algorithm.

The multi-core literature the work is related to is named in the reference list of
the Handbook of Model Checking, chapter 5 (Holzmann), which the monorepo holds as
`books-md/1clarke_edmund_handbook_of_model_checking/` (entries 3, 16-18 and 20:
the DiVinE multi-core LTL checker; the multi-core extension of SPIN; parallelising
SPIN; swarm verification). Only that reference list is in the monorepo: **the papers
themselves are not, and nothing in the engine or its documentation was taken from
them**. The partitioning of states by the owner of their hash is the classic
distributed-memory idea and a shared lock-free table is the usual multi-core one;
both are named here from memory, are not in the monorepo, and are not cited as a
source of this implementation.

The sources do not guarantee this implementation. What the release asserts is what
its tests confirm: a differential oracle against the sequential searches (random
models of two generators, every Promela model of the fixtures and the SPIN corpus
that the frontend accepts, and `pan -c0` through `tools/pandiff`), the same report
for every worker count, the race detector, and a mutation campaign of the rules
that make the result deterministic; the scratch harness that ran the mutants is not
part of the release. See `steps/perf5-confirmation.md`.

## Updating the index

When a source changes the supported model formats, property semantics, result
statuses, or release workflow, update this file and the corresponding public
README/references in the same change. Do not use a citation to fill a capability
gap: unsupported timed, probabilistic, symbolic, unbounded, or game semantics
must remain explicitly outside this plugin's scope.
