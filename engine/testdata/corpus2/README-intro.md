
Plan 14 §2.3 observes that almost every file in `Promela - examples/` was
written by one author, so an engine debugged only on them may have inherited
Holzmann's style — `end`/`progress` labels, short `atomic`, `mtype` messages —
as a hidden norm. This directory is the second corpus that makes the
assumption testable: 55 listings taken from two sources already in
`books-md/`, in two other styles.

| directory | source | style |
|---|---|---|
| `lectures/` | `books-md/lect01-lect09.md` | teaching slides, russian comments, `printf` inside models, deliberately broken examples |
| `karpov/` | `books-md/Karpov_U._Model_checking/…md` | numbered book listings, cyrillic identifiers (`Р1`, `nonceВ`), races without channels |

**How the listings were extracted.** In the markdown each listing has been
OCR-joined onto a single line. The extraction reconstructs the line breaks
from Promela syntax **and changes nothing else**: no bug is fixed, no missing
declaration is added, no unbalanced brace is closed, the `qoto` typo of the
PAR protocol stays, and Karpov's own misnumbered lines stay misnumbered. Three
mechanical repairs were applied and are recorded in the header of each file
that needed them: HTML entities decoded (`&amp;` → `&`), markdown escapes
removed (`\_pid` → `_pid`), and — for Karpov — the printed listing line
numbers deleted, since those numbers *are* the line-break markers. Slide
annotation prose that OCR glued onto the end of a listing is dropped and
quoted verbatim in the header. Every file starts with a comment citing the
exact source line range, which can be checked with `sed -n '<range>p'`.

Because nothing was repaired, many listings are *meant* to be rejected: they
are teaching examples of what goes wrong, or fragments with `...` in place of
a body. That is the point. Plan §2.3 says models the parser rejects count as
tests of rejection and are not dropped from the accounting silently, so the
table below carries every listing, including the ones SPIN itself refuses.

**How to read the table.**

- *engine* — what `mcd parse` did: `parsed`, or the rejection kind
  (`outside-subset` with the construct, `syntax`, `semantic`). A rejection is
  status `not-executed`: nothing rejected has been executed, so it carries no
  verdict.
- *pan* — whether SPIN 6.5.2 itself accepts the listing (`spin -a -o1 -o2
  -o3`, then `gcc -O2 -DNOREDUCE`). Where SPIN refuses, neither tool can be
  called right about the listing and it can be in nobody's differential set.
- *agree?* — for the listings both tools accept, the differential comparison:
  the engine's safety verdict and state count (`mcd check --sweep`) against
  `pan -c0`. `yes` means the verdict *and* the state count matched.
- *constructs* — detected mechanically from the code (comments stripped), so
  that a reader can see why a listing is in or out of the subset without
  opening it.

**Regenerating.** This file is generated; edit `README-intro.md` and rerun:

    go build -o /tmp/mcd ./cmd/mcd && go build -o /tmp/mutate ./cmd/mutate
    /tmp/mutate corpus2 -root testdata/corpus2 -mcd /tmp/mcd \
        -preamble testdata/corpus2/README-intro.md \
        -md testdata/corpus2/README.md -json /tmp/corpus2.json
