Feature: K3 — mutation tests and the second corpus (checkpoint measurements)
  Plan 14 §9, checkpoint K3, and §8.1 (mutation tests), §2.3 (second corpus). K3
  requires two measurements that no step has produced: the share of detected
  mutants and the share of second-corpus models in the differential set. This
  step builds the test tooling and the corpus and measures; it changes nothing in
  the engine (G5 owns it). A disagreement between the engine and pan is reported
  with both verdicts and the mutant file — never fixed here.

  Vocabulary:
  - mutant: the source of a corpus model with exactly ONE mutation applied
    (first-order). The mutator is a text-level rewriter over its OWN tokenizer
    (`engine/tools/mutate`): tokens carry a byte offset, a line and a column in
    the original file, comments and string literals are never mutated, and a
    `#` directive is one opaque token, so no macro body is mutated and every
    manifest position is a real position in the source file. The tokenizer is
    the mutator's own — not `frontend/promela`'s — so that this step is
    independent of the frontend changes G5 makes in parallel; the price is that
    the mutator does not know the subset, and whether a mutant is inside it is
    decided by running the engine on it. Operators (plan §8.1) and what
    each does:
      drop-atomic      remove the keyword `atomic` before `{` (the block stays)
      drop-dstep       remove the keyword `d_step` before `{`
      chan-cap-minus   `[N] of {…}` with N > 0 becomes `[N-1] of {…}`
      chan-cap-zero    `[N] of {…}` with N > 1 becomes `[0] of {…}` (rendezvous)
      invert-guard     the first statement of an `if`/`do` option that is a bare
                       expression becomes `!( … )` (not for `else`, send/receive,
                       assignment, `run`, `atomic`/`d_step`, `goto`, `break`)
      drop-end-label   a label `end…:` is removed
      weaken-assert    `assert( … )` becomes `assert(true)`
      swap-relop       `<` ↔ `<=` and `>` ↔ `>=` (one operator token)
      off-by-one       a decimal constant becomes its successor. A constant
                       directly inside `[ ]` is excluded: there it is an array
                       size, a channel capacity (which has its own operators),
                       an `active [N]` process count or an index, and mutating
                       those changes the model's shape rather than its
                       arithmetic. A constant inside a `#` directive is excluded
                       with every other directive token.
      drop-alternative one `:: …` option of an `if`/`do` with at least two options
                       is removed
  - manifest: `manifest.json` next to the mutants — a list, one entry per
    mutant, in generation order, with `id`, `operator`, `file` (mutant path),
    `source` (original path), `line`, `col`, `original` (the replaced text),
    `mutated` (its replacement). `original` is the replaced text with its
    surrounding whitespace trimmed; a deletion has `mutated` empty, which the
    tables below write as "(removed)". Generation order is deterministic:
    operators in the order above, then by source position; two runs give
    byte-identical manifests.
  - natural checks of a model: the properties the model itself carries —
    `assert` and `deadlock` (pan: assertion violated / invalid end state); the
    model's own `never` claim or `accept` labels (pan -a); `progress` labels
    (pan -l); and, where the G4 triples give an LTL formula for the model, that
    formula (`--ltl`, pan -a with `spin -f`'s claim). A model's checks are
    listed once in the run configuration and applied unchanged to every mutant.
  - verdict of a check: `violated` if any property of the check is violated,
    `verified` if all are verified with a complete search; for pan `violated`
    when errors > 0, `verified` when errors = 0 with a completed search.
    `invalid-model`, `inconclusive`, a frontend rejection, or an incomplete pan
    search are not verdicts.
  - classes of a mutant (per check, then the mutant takes the class of its
    checks: (iii) if any check is (iii), else (iv) if any check is (iv), else
    (i) if any check is (i), else (ii)):
      (i)   detected — the verdict changed against the original for BOTH the
            engine and pan, and the two mutant verdicts are the same
      (ii)  verdict-equivalent — the verdict is unchanged for both (this says
            nothing about behavioural equivalence: the state count may differ,
            and that sub-split is reported)
      (iii) DISAGREEMENT — the engine and pan give different verdicts on the
            mutant, or one changed its verdict and the other did not
      (iv)  not comparable — the engine rejects the mutant (outside subset,
            syntax), SPIN rejects it, the engine says `invalid-model` (domain
            overflow, which pan wraps silently — a documented policy difference,
            G1 §4 rule 10), or either search is incomplete
  - detection rate = (i) / ((i) + (iii)), on the mutants whose verdict changed
    for at least one side; the raw counts of all four classes are reported with
    it, and every (iii) is listed with the mutant path and both verdicts.
  - differential set (plan §2.3): the models on which the engine and pan were
    both run and compared (state count, verdict, class; or a triple). The share
    of second-corpus models is |corpus2 ∩ set| / |set|. A corpus2 listing the
    engine rejects is a test of rejection and is counted in the README, not in
    the set; a listing SPIN itself rejects can be in neither.
  - the pan numbers are produced by SPIN 6.5.2, `spin -a -o1 -o2 -o3`,
    `gcc -O2 -DNOREDUCE [-DNP]`, `./pan -c0`, `./pan`, `./pan -a|-l -c0`
    through `tools/pandiff`; scenarios that need SPIN are tagged @spin and skip
    cleanly when `spin` or `gcc` is absent.

  Background:
    Given the K3 mutator built from "engine/tools/mutate"

  # ---- 1. Operators produce syntactically valid mutants with a manifest --------

  Scenario Outline: an operator produces a valid mutant of CH2/mutex_flaw.pml
    Given the K3 corpus model "CH2/mutex_flaw.pml"
    When I generate the K3 mutants with the operator "<operator>"
    Then at least 1 mutant is generated
    And every K3 mutant parses with the engine's Promela frontend
    And every K3 manifest entry has the operator "<operator>", a line, a column, an original text and a mutated text
    And the K3 manifest entry 1 replaces "<original>" by "<mutated>"

    Examples:
      | operator         | original                | mutated                     |
      | invert-guard     | (y != 0 && y != me)     | !((y != 0 && y != me))      |
      | weaken-assert    | assert(cnt == 1)        | assert(true)                |
      | off-by-one       | 1                       | 2                           |
      | drop-alternative | :: (y != 0 && y != me) -> goto L1 | (removed)         |

  Scenario Outline: operators that do not apply to mutex_flaw are shown on the nearest corpus model
    # mutex_flaw has no atomic, d_step, channel, end label or < / <= operator;
    # each remaining operator is exercised on a corpus model that has the construct.
    Given the K3 corpus model "<model>"
    When I generate the K3 mutants with the operator "<operator>"
    Then at least 1 mutant is generated
    And every K3 mutant parses with the engine's Promela frontend
    And the K3 manifest entry 1 replaces "<original>" by "<mutated>"

    Examples:
      | model                       | operator       | original          | mutated          |
      | App_C/petrinet1             | drop-atomic    | atomic            | (removed)        |
      | CH8/trivial.pml             | drop-dstep     | d_step            | (removed)        |
      | CH3/alternatingbit2.pml     | chan-cap-minus | [2]               | [1]              |
      | CH3/alternatingbit2.pml     | chan-cap-zero  | [2]               | [0]              |
      | CH4/dijkstra.pml            | drop-end-label | end:              | (removed)        |
      | CH3/euclid.pml              | swap-relop     | >                 | >=               |

  Scenario: mutex_flaw yields no mutant for an operator whose construct it lacks
    Given the K3 corpus model "CH2/mutex_flaw.pml"
    When I generate the K3 mutants with the operator "drop-atomic"
    Then 0 mutants are generated

  Scenario: the manifest is deterministic and ordered by operator then position
    Given the K3 corpus model "CH2/mutex_flaw.pml"
    When I generate all K3 mutants twice
    Then the two K3 manifests are byte-identical
    And the K3 manifest entries are ordered by operator rank then by line and column

  @spin
  Scenario: SPIN accepts every mutant of mutex_flaw
    Given spin and gcc are available for K3
    And the K3 corpus model "CH2/mutex_flaw.pml"
    When I generate all K3 mutants
    Then spin -a accepts every K3 mutant

  # ---- 2. Known mutants change the verdict identically -------------------------

  @spin
  Scenario: the weakened assert of mutex_flaw turns violated into verified for both
    Given spin and gcc are available for K3
    And the K3 corpus model "CH2/mutex_flaw.pml"
    When I generate the K3 mutants with the operator "weaken-assert"
    And I compare the engine with pan on the original and on K3 mutant 1
    Then the engine verdict of the original is "violated" and of the mutant "verified"
    And the pan verdict of the original is "error found" and of the mutant "no error"
    And the K3 mutant is classified "(i) detected"

  @spin
  Scenario: dropping the first atomic of petrinet1 changes the state count identically
    Given spin and gcc are available for K3
    And the K3 corpus model "App_C/petrinet1"
    When I generate the K3 mutants with the operator "drop-atomic"
    And I compare the engine with pan on the original and on K3 mutant 1
    Then the state count of the original is 8 for both the engine and pan
    And the state count of the mutant is larger than 8 and equal for the engine and pan
    And the pan verdict of the original is "error found" and of the mutant "error found"
    And the K3 mutant is classified "(ii) verdict-equivalent"

  # ---- 3. Second corpus --------------------------------------------------------

  Scenario: a corpus2 listing with a cyrillic process name is rejected by the lexer
    # What the frontend actually does (measured): identifiers are ASCII-only, so the
    # first cyrillic byte is a lexical error of kind "syntax", not an outside-subset
    # rejection with a construct name. Recorded for G5 in steps/k3-confirmation.md.
    Given the corpus2 listing "karpov/01-mutex-p1.pml"
    When I run "mcd parse" on the corpus2 listing
    Then it is rejected with kind "syntax" and status "not-executed"
    And the rejection message mentions "unexpected character"

  @spin
  Scenario: SPIN 6.5.2 also rejects the cyrillic process name
    Given spin and gcc are available for K3
    And the corpus2 listing "karpov/01-mutex-p1.pml"
    Then spin -a rejects the corpus2 listing with a syntax error

  Scenario: the corpus2 README accounts for every listing
    Given the corpus2 directory "engine/testdata/corpus2"
    Then every ".pml" file under it has a row in its README table
    And every row names an engine outcome that is one of "parsed", "outside-subset", "syntax", "semantic", "invalid-model"
    And every row names a pan outcome that is one of "accepted", "rejected"
    And every listing starts with a header comment citing its source lines

  # ---- 4. The detection-rate report --------------------------------------------

  Scenario: the mutation report lists every disagreement with both verdicts
    Given the K3 mutation results "steps/k3-mutation-results.json"
    Then every mutant of class "(iii)" records a mutant path, the engine verdict and the pan verdict
    And every mutant of class "(i)" records a changed verdict for both the engine and pan
    And the detection rate equals (i) divided by (i) plus (iii) over the recorded counts
    And the report "steps/k3-mutation-report.md" names every mutant of class "(iii)"
