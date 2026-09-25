# Second corpus (plan 14 §2.3)


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

Measured with `mcd 0.1.0-g0 (ir mcd-ir/1, report mcd-report/1)` and `Spin Version 6.5.2 -- 6 December 2019` on 2026-09-25T19:43:59Z.

| file | source lines | constructs | engine | pan | agree? |
|---|---|---|---|---|---|
| `karpov/01-mutual-exclusion-p1.pml` | 3677-3683 | atomic, non-ASCII identifier | syntax — unexpected character "Ð" (01-mutual-exclusion-p1.pml, line 8) | rejected — spin: m.pml:8, Error: syntax error	saw ''\' = 92' | spin: m.pml:10, Error: syntax error	saw 'an identifier' near 'm1' | spin: m.pml:9, Error: no runable process… | — |
| `karpov/02-inc-dec-reset-check.pml` | 3737-3743 | atomic, run, assert | syntax — unexpected x at the top level (expected a declaration, proctype, init or never) (02-inc-dec-reset-check.pml, line 8) | rejected — spin: m.pml:8, Error: syntax error	saw 'an identifier' | spin: m.pml:11, Error: undeclared variable: x	saw 'operator: <' | | — |
| `karpov/03-state-race.pml` | 3905-3905 | run | parsed | accepted | yes (engine violated/56, pan violated/56) |
| `karpov/04-state-race-atomic.pml` | 3911-3911 | atomic, run | parsed | accepted | yes (engine violated/9, pan violated/9) |
| `karpov/05-weak-fairness-p-q.pml` | 4359-4359 | active | parsed | accepted | yes (engine verified/18, pan verified/18) |
| `karpov/06-transfer-protocol.pml` | 4665-4671 | chan, chan param, mtype, atomic, run | outside-subset — construct outside subset: channel-typed variable (local channels and channel variables are outside the subset; declare channels globally) (06-transfer-protocol.… | accepted | — |
| `karpov/07-par-protocol.pml` | 4691-4733 | chan, rendezvous, mtype | outside-subset — construct outside subset: uninitialised channel (channel c1 has no [capacity] of { … }) (07-par-protocol.pml, line 11) | rejected — spin: m.pml:28, Error: undeclared variable: qoto	saw 'an identifier' near 'q3' | | — |
| `karpov/08-two-phase-commit.pml` | 4763-4769 | chan, rendezvous, chan param, mtype, run, assert, directives | syntax — unexpected > in expression (08-two-phase-commit.pml, line 44) | rejected — spin: m.pml:10, Error: syntax error	saw '',' = 44' | spin: m.pml:26, Error: undeclared variable: Pchan	saw 'operator: ?' | | — |
| `karpov/09-mutual-exclusion-exercise.pml` | 4825-4825 | active, assert | syntax — expected a name, got [ (09-mutual-exclusion-exercise.pml, line 9) | rejected — spin: m.pml:9, Error: syntax error	saw ''[' = 91' | spin: m.pml:9, Error: no runable process | | — |
| `karpov/10-needham-schroeder-alice.pml` | 5259-5297 | chan, rendezvous, chan array, mtype, typedef, active, non-ASCII identifier | syntax — unexpected character "â" (10-needham-schroeder-alice.pml, line 31) | rejected — spin: m.pml:24, Error: undeclared variable: partnerA	saw 'operator: =' | | — |
| `karpov/11-needham-schroeder-intruder.pml` | 5317-5323 | active, non-ASCII identifier | syntax — unexpected character "Ð" (11-needham-schroeder-intruder.pml, line 21) | rejected — spin: m.pml:12, Error: undeclared variable: network	saw 'operator: ?' | | — |
| `lectures/01-run-irun-init.pml` | 2287-2287 | run, printf | syntax — unterminated character constant (01-run-irun-init.pml, line 8) | rejected — spin: m.pml:8, Error: character quote missing: ' | | — |
| `lectures/02-provided-toggle.pml` | 2333-2333 | provided, active, printf | outside-subset — construct outside subset: provided (plan 14 §5.2: v1 (G5)) (02-provided-toggle.pml, line 9) | accepted | — |
| `lectures/03-assert-invariant.pml` | 2457-2457 | active, assert | parsed | accepted | yes (engine verified/3, pan verified/3) |
| `lectures/04-executability-run-b.pml` | 2467-2467 | run, printf | syntax — unterminated character constant (04-executability-run-b.pml, line 15) | rejected — spin: m.pml:15, Error: character quote missing: ' | | — |
| `lectures/05-producer-consumer-turn.pml` | 2495-2495 | mtype, active, printf | syntax — unterminated character constant (05-producer-consumer-turn.pml, line 12) | rejected — spin: m.pml:12, Error: character quote missing: ' | | — |
| `lectures/06-consumer-do-variant.pml` | 2509-2509 | active, printf | syntax — unterminated character constant (06-consumer-do-variant.pml, line 9) | rejected — spin: m.pml:8, Error: undeclared variable: turn	saw 'operator: ==' | | — |
| `lectures/07-consumer-if-goto-variant.pml` | 2515-2515 | active, printf | syntax — unterminated character constant (07-consumer-if-goto-variant.pml, line 10) | rejected — spin: m.pml:9, Error: undeclared variable: turn	saw 'operator: ==' | | — |
| `lectures/08-mutex-busy-flag.pml` | 2543-2543 | atomic, run, active, assert, printf | syntax — unexpected character "\\" (08-mutex-busy-flag.pml, line 13) | rejected — m.pml:13:37: warning: missing terminating ' character |    13 |     printf(P%d in critical section\n',i); |       |                                     ^ | spin… | — |
| `lectures/09-mutex-two-flags.pml` | 2569-2569 | active, assert, printf | parsed | accepted | yes (engine violated/88, pan violated/88) |
| `lectures/10-peterson.pml` | 2619-2619 | mtype, active, assert | parsed | accepted | yes (engine verified/110, pan verified/110) |
| `lectures/11-lamport-variant.pml` | 2635-2635 | active, array, assert | parsed | accepted | not comparable (engine invalid-model/11, pan violated/117) |
| `lectures/12-semaphore-rendezvous.pml` | 2776-2776 | chan, rendezvous, chan param, mtype, active | parsed | accepted | yes (engine verified/6, pan verified/6) |
| `lectures/13-type-ranges-tryme.pml` | 2901-2901 | chan, chan param, mtype, active | outside-subset — construct outside subset: channel-typed variable (local channels and channel variables are outside the subset; declare channels globally) (13-type-ranges-tryme.… | rejected — spin: m.pml:14, Error: invalid use of chan name statement separator | | — |
| `lectures/14-variable-scope.pml` | 2925-2925 | active, printf | syntax — unterminated character constant (14-variable-scope.pml, line 15) | rejected — spin: m.pml:10, Error: redeclaration of 'y' statement separator | | — |
| `lectures/15-cpp-macros.pml` | 2956-2956 | chan, mtype, atomic, active, directives | semantic — a constant is required here (15-cpp-macros.pml, line 8) | rejected — spin: m.pml:8, Error: syntax error	saw 'an identifier' near 'MAXQ' | spin: m.pml:15, Error: syntax error | spin: m.pml:15, Error: no runable process | | — |
| `lectures/16-abp-two-slot.pml` | 3177-3189 | chan, chan param, mtype, active, eval | outside-subset — construct outside subset: eval (plan 14 §5.2: not in the corpus, outside the subset) (16-abp-two-slot.pml, line 19) | rejected — spin: m.pml:35, Error: syntax error | | — |
| `lectures/17-eq-abstract-domain.pml` | 3339-3339 | mtype | syntax — unexpected m at the top level (expected a declaration, proctype, init or never) (17-eq-abstract-domain.pml, line 8) | rejected — spin: m.pml:8, Error: syntax error	saw 'an identifier' | spin: m.pml:10, Error: syntax error	saw 'data typename' near 'byte' | spin: m.pml:8, Error: no runable … | — |
| `lectures/18-abstraction-do-else-init.pml` | 4016-4016 | run, printf | syntax — unterminated character constant (18-abstraction-do-else-init.pml, line 9) | rejected — spin: m.pml:9, Error: character quote missing: ' | | — |
| `lectures/19-abstraction-mtype-inline.pml` | 4050-4050 | mtype, inline, run, printf | syntax — unterminated character constant (19-abstraction-mtype-inline.pml, line 11) | rejected — spin: m.pml:11, Error: character quote missing: ' | | — |
| `lectures/20-busywait-do-loop.pml` | 4260-4260 | active | syntax — expected a name, got { (20-busywait-do-loop.pml, line 8) | rejected — spin: m.pml:8, Error: syntax error	saw ''{' = 123' | spin: m.pml:8, Error: no runable process | | — |
| `lectures/21-blocking-expression.pml` | 4268-4268 | active | syntax — expected a name, got { (21-blocking-expression.pml, line 8) | rejected — spin: m.pml:8, Error: syntax error	saw ''{' = 123' | spin: m.pml:8, Error: no runable process | | — |
| `lectures/22-rendezvous-call-return.pml` | 4306-4306 | chan, rendezvous, chan param, active | syntax — expected "{", got bit (22-rendezvous-call-return.pml, line 6) | rejected — spin: m.pml:6, Error: syntax error	saw 'data typename' near 'bit' | spin: m.pml:7, Error: syntax error	saw 'data typename' near 'bit' | spin: m.pml:12, Error: u… | — |
| `lectures/23-assert-race.pml` | 4472-4472 | active, assert | parsed | accepted | yes (engine violated/7, pan violated/7) |
| `lectures/24-assert-race-atomic.pml` | 4494-4494 | atomic, active, assert | syntax — expected "{", got ( (24-assert-race-atomic.pml, line 10) | rejected — spin: m.pml:10, Error: syntax error	saw ''(' = 40' | spin: m.pml:10, Error: syntax error	saw '')' = 41' | spin: m.pml:15, Error: syntax error	saw ''(' = 40' | s… | — |
| `lectures/25-semaphore-invariant-assert.pml` | 4510-4516 | chan, rendezvous, chan param, mtype, active, assert | parsed | accepted | yes (engine verified/12, pan verified/12) |
| `lectures/26-semaphore-invariant-loop.pml` | 4534-4540 | chan, rendezvous, chan param, mtype, active, assert | parsed | accepted | yes (engine verified/4, pan verified/4) |
| `lectures/27-semaphore-invariant-dstep.pml` | 4554-4560 | chan, rendezvous, chan param, mtype, d_step, active, assert | parsed | accepted | yes (engine verified/4, pan verified/4) |
| `lectures/28-semaphore-end-labels.pml` | 4576-4576 | chan, rendezvous, chan param, mtype, end label, active | parsed | accepted | yes (engine verified/4, pan verified/4) |
| `lectures/29-semaphore-progress-label.pml` | 4590-4590 | chan, rendezvous, chan param, mtype, progress label, active | parsed | accepted | yes (engine verified/4, pan verified/4) |
| `lectures/30-nonprogress-x.pml` | 4602-4602 | active | parsed | accepted | yes (engine verified/2, pan verified/2) |
| `lectures/31-nonprogress-xy.pml` | 4660-4660 | active | parsed | accepted | yes (engine verified/4, pan verified/4) |
| `lectures/32-progress-label-xy.pml` | 4692-4692 | progress label, active | parsed | accepted | yes (engine verified/16, pan verified/16) |
| `lectures/33-leader-election.pml` | 4719-4719 | chan, chan array, chan param, mtype, atomic, run, directives | outside-subset — construct outside subset: array of channels (outside the subset) (33-leader-election.pml, line 15) | rejected — spin: m.pml:21, Error: syntax error	saw '293' | spin: error, m.pml:21, bad node type 0 (.m) | | — |
| `lectures/34-abp-lossy.pml` | 4793-4805 | chan, chan param, mtype, progress label, active, timeout, eval | outside-subset — construct outside subset: eval (plan 14 §5.2: not in the corpus, outside the subset) (34-abp-lossy.pml, line 18) | rejected — spin: m.pml:33, Error: syntax error	saw ''{' = 123' | spin: m.pml:36, Error: syntax error	saw 'keyword: do' near 'do' | | — |
| `lectures/35-abp-lossy-progress.pml` | 4837-4837 | mtype, progress label, active, eval | outside-subset — construct outside subset: eval (plan 14 §5.2: not in the corpus, outside the subset) (35-abp-lossy-progress.pml, line 22) | rejected — spin: m.pml:10, Error: undeclared variable: from_s	saw 'operator: ?' | | — |
| `lectures/36-mutex-trace-proc-a.pml` | 4978-4978 | active, printf | syntax — unterminated character constant (36-mutex-trace-proc-a.pml, line 13) | rejected — spin: m.pml:13, Error: character quote missing: ' | | — |
| `lectures/37-invariant-not-p-or-not-q.pml` | 5004-5004 | active, assert | semantic — undeclared variable p (37-invariant-not-p-or-not-q.pml, line 7) | rejected — spin: m.pml:7, Error: undeclared variable: p	saw 'operator: ||' | | — |
| `lectures/38-invariant-p-then-q.pml` | 5010-5010 | active, assert | semantic — undeclared variable p (38-invariant-p-then-q.pml, line 7) | rejected — spin: m.pml:8, Error: undeclared variable: p statement separator near 'do' | | — |
| `lectures/39-never-control-labels.pml` | 5124-5124 | chan, rendezvous, chan param, mtype, never claim, active, assert, remote label ref | syntax — expected ";" or "->" after a statement, got @ (39-never-control-labels.pml, line 10) | rejected — spin: m.pml:10, Error: undeclared variable: user	saw '@' | | — |
| `lectures/40-runner-else-break.pml` | 5130-5130 | active | syntax — unexpected . in expression (40-runner-else-break.pml, line 8) | rejected — spin: m.pml:8, Error: syntax error	saw '293' | spin: m.pml:10, Error: syntax error	saw 'keyword: od' near 'od' | spin: error, m.pml:8, bad node type 0 (.m) | | — |
| `lectures/41-runner-end-label.pml` | 5136-5136 | active | syntax — unexpected . in expression (41-runner-end-label.pml, line 8) | rejected — spin: m.pml:8, Error: syntax error	saw '293' | spin: m.pml:10, Error: syntax error	saw 'keyword: od' near 'od' | spin: error, m.pml:8, bad node type 0 (.m) | | — |
| `lectures/42-collatz-base.pml` | 6481-6481 | active | parsed | accepted | yes (engine violated/1, pan violated/1) |
| `lectures/43-collatz-never-always-p.pml` | 6542-6542 | never claim, accept label, active, directives | parsed | accepted | yes (engine verified/1, pan verified/1) |
| `lectures/44-collatz-never-gf-p.pml` | 6586-6586 | never claim, accept label, active, directives | parsed | accepted | yes (engine violated/2, pan violated/2) |

55 listings: 19 in the differential set, 2 refused by the frontend as outside the subset while SPIN accepts them, 33 refused by SPIN itself, 1 otherwise not comparable.
