# Promela subset accepted by the engine (as built in G1)

Sources: `model-check-skill-notes/14-skill-building-plan.md` §5.2 (grammar by
version), §4.1 (deadlock, overflow), §2.1 (corpus rows); semantics from
`model-check-skill-notes/07-lectures-01-09.md` лекции 4–5 (executability,
blocking, `if`/`do`/`else`, channels, rendezvous, program-graph semantics) and
`model-check-skill-notes/02-design-and-validation-of-computer-protocols.md` гл. 5
(validation models), гл. 6 (`assert`, `end`, `progress`, `accept`), гл. 13
(`timeout` becomes available only after the ordinary transitions failed);
`model-check-skill-notes/03-karpov-model-checking.md` гл. 5 (`atomic`, `d_step`).
Engine as built: `model-check-plugin/engine/frontend/promela/` (parser, preprocessor,
lowering), `model-check-plugin/steps/g1-confirmation.md` §1, §3.1 (coverage of
chapters 2–3 against SPIN 6.5.2), §4 (semantic decisions), §5 (narrowings).

The engine parses a **fixed subset** of Promela. Everything outside it is rejected
with the construct and the line named (exit code 2, `kind: outside-subset`,
`status: not-executed`), and the property gets `not-executed` — not
`invalid-model`: a model that uses `unless` is a fine model that this engine cannot
read (11 §19, plan §2.1). Do not work around the parser; rewrite inside the subset
or explain the boundary. A construct outside the subset gives `not-executed` for
**every** property of the model, because nothing was executed.

## 1. What the parser accepts (MVP, built in G1)

Most rows are exercised by the corpus files of chapters 2–3 that agree with SPIN state
for state (`steps/g1-confirmation.md` §3.1: 21 files agree, 3 differ only by the
overflow policy of §2); `#elif`, `#undef`, `nempty`/`nfull`, `xr`/`xs` (in the corpus
only under `#if 0`) are covered by the engine's own test models
(`engine/testdata/promela/`) and unit tests, not by the corpus. Remote references are
**not** accepted (§3); the plan never included them, and this table promised them in
error until the row-by-row probe of `steps/g3-evals3-subset-probe.md` caught it.

| Area | Accepted |
|---|---|
| Processes | `proctype`, `active proctype`, `active [N] proctype`, `init`; parameters of the scalar types below; `_pid` (a constant per instance); `run P(args)`, including inside a loop and recursively — the G5 step lifted the G1 narrowing to straight-line `init` (§2, `run`) |
| Types | `bit`, `bool`, `byte`, `short`, `int`, `mtype`, `pid`; one-dimensional arrays of fixed length; local variables with a constant or `_pid` initialiser |
| mtype | both declaration forms, `mtype = { a, b }` and `mtype { a, b }`; several declarations (numbering as SPIN: last name of a declaration = 1, upward; the next declaration continues above) |
| Channels | `chan` declarations `chan c = [N] of { t1, t2, … }` with `N ≥ 0` (`0` = rendezvous); `c!e1,e2` / `c!e1(e2,…)`; `c?x,y` with constants (pattern match) and variables; the predicates `len`, `empty`, `full`, `nempty`, `nfull` (as `len(c)`, `empty(c)`, …); `xr c` / `xs c` — `xr` and `xs` are parsed and kept as **hints** (recorded in the IR, no effect on the search) |
| Control | `if … fi`, `do … od`, `::` options, `->` and `;` as separators, `else`, `break`, `goto`, labels; label prefixes `end`, `progress`, `accept` |
| Atomicity | `atomic { … }`, `d_step { … }` (semantics in §2) |
| Statements | assignment, expression statements (guards), `assert(e)`, `skip`, `true`, `false`, `timeout`, `printf` (a step with no effect; warning) |
| Preprocessor | `#define` (object-like `#define NAME body` and function-like `#define NAME(args) body`, with `\` continuation), `#undef`, `#ifdef`/`#ifndef`/`#if`/`#elif`/`#else`/`#endif` with constant expressions and `defined()`; symbols from the command line with `-D` (`-D NAME` / `-D NAME=value`, repeatable; MCP: `defines`); expanded tokens keep the line of the macro call for traces |
| Properties | `never` claim (`never { … }`; since G4 it is **executed** and the frontend adds a property `never` of kind `ltl` — probe `in-never-claim.pml` gives properties `deadlock, never`; `#ifdef` pairs select claims, as `CH4/prop.pml` with `-D PHI`), `assert`, `end`/`progress`/`accept` labels (an `accept` label adds a property `accept`, a `progress` label a property `progress` — probe `in-label-prefixes.pml` gives `deadlock, accept, progress`) |
| Expressions | integer arithmetic and comparison, `&&`, `\|\|`, `!`, `%` |

## 2. Semantic decisions users will notice against SPIN

All of these were settled by probes against `pan` (SPIN 6.5.2, `-DNOREDUCE`). The
subset widened in G5, so the counts of the G1 era no longer hold: of the 34 `.pml` /
`.pr` files in chapters 2–3, **31 now parse and 3 are refused** —
`CH3/notpossible.pml` (`run` inside a larger expression), `CH3/pots.pml` (`unless`),
and `CH3/scope.pml`, which is a `semantic` refusal that **SPIN makes too**, at the
same line and for the same reason ("undeclared variable: y"), because the file is a
teaching example of a scope error. Where the differential corpus test covers a model
the counts agree with `pan -c0` (`TestDifferentialCorpus`, 51 models, all agreeing —
`steps/g5-addendum-confirmation.md` §5); that test, not this paragraph, is the
authority for any particular file.

| Topic | Engine | SPIN (`pan`) | What to tell the user |
|---|---|---|---|
| **Domain overflow** (`byte` leaving 0–255, a Petri place above its declared capacity, an out-of-range index — *not* a send into a full channel, which is an ordinary blocked step and can only end in a deadlock, never in `invalid-model`) | `invalid-model`, evidence `unknown`, the run to the offending step attached as `counterexample`, the overflow named in `reason`; every property still undecided at that point gets `invalid-model` (plan §4.1) | **wraps silently** (`255 + 1 = 0`) and keeps searching | Say it explicitly: `CH3/counter.pml`, `counter2.pml`, `xr.pml` give `invalid-model` here and "no errors" in SPIN. The model's domain, not the engine, is what differs; fix or justify the domain before any property claim |
| **Blocking inside `d_step`** | `invalid-model` with reason "block in d_step seq" (03 гл. 5: a modelling error) | run-time error, search aborts | Same verdict class; the engine attaches the run |
| **Nondeterminism inside `d_step`** | the first executable alternative is taken (as SPIN) | same | — |
| **`atomic` storage rule** (explanatory) | intermediate states inside an `atomic` sequence are **not stored** while the holder can move; the state where an `atomic` sequence is interrupted (its statement blocks) **is stored** | same rule | This is why `App_C/petrinet1` has 8 states, not 28: the marking updates inside `atomic` are one stored step. Invariants and `reach` are checked on **stored** states; `assert` on every step. Counters compare with `pan -c0`, not with the number of statements executed |
| **`run`** | every proctype has a **pool** of instances (`--max-procs`, default 8, bounds it); the static processes (`active`, `init`) come first in textual order, then each proctype's pool; **every `run` draws from its own proctype's pool and takes the first dormant slot at the moment it fires** | same, including that a slot — and so a pid — is **reused** once its process has died | The choice of slot is made during the search, not by the frontend: two `run`s of the same proctype with **equal arguments** are the case that shows it. `engine/testdata/mutate/state-count/you_run2-equal-run-arguments.pml` gives 12 states, exactly `pan -c0`; a per-statement slot gave 14 and was a real divergence, found by the K3 mutation campaign and fixed in the G5 addendum (`steps/g5-addendum-confirmation.md` §4). Never tell a user that pids are not recycled, and do not read two instances of one proctype as two distinct identities unless the model stores the pid itself |
| **Rendezvous** | one step for sender and receiver; each matching receiver is a separate transition; the trace step carries `partner`; rendezvous inside `d_step` is outside the subset | same | — |
| **`else`** | dynamic: executable iff no other option of the same `if`/`do` is | same | — |
| **`timeout`** | two-phase: true iff nothing at all is executable with `timeout` false (02 гл. 13) | same | an abstraction of "stuck", not a clock |
| **Process termination** | the `-end-` transition of a process is executable only when no younger process is alive (SPIN removes only the last process of the vector); locals are zeroed | same | end states are valid only when every process is terminated or at an `end` label |
| **Never claim** | parsed as a process with `claim: true` and **executed** since G4: the frontend adds a property `never` (kind `ltl`), the claim moves first at every step, and its verdict comes back like any other (`properties-ltl-ctl.md` §6) | the same | the G1-era warning "not executed in this engine version" is gone; if you meet it in an old report, that report predates G4 |
| **`pc_value(n)`** | accepted, and **warned about**: "the value is this engine's control-location numbering, which is built differently from pan's internal state numbers; a model whose behaviour depends on the number behaves differently here than under SPIN" | pan has its own internal numbering, and warns that `pc_value` outside a never claim is unusual | this is the one construct the engine accepts while telling you the semantics differ. A verdict on a model that branches on a particular `pc_value` is about **this** engine's numbering and must not be carried to SPIN. `CH4/pcval.pml` parses (exit 0, three warnings) |
| **`printf`** | a step without effect; warning "printf ignored" | prints | properties cannot observe it |
| **Undeclared variable** (`CH3/scope.pml`) | `kind: semantic` rejection | SPIN also refuses | fix the model |
| **Redeclaration of a name** | `kind: semantic` rejection quoting SPIN's own wording ("label L redeclared", "redeclaration of n") | SPIN also refuses, on all the shapes that matter — the rule was re-established by running 52 collision shapes against SPIN 6.5.2, not inferred from two (`steps/g5-addendum2-confirmation.md` §2) | fix the model. The rule, and the one legal case, are in §3 "Name collisions". Two shapes are refused here where SPIN accepts, deliberately — §3 "Stricter than SPIN" |

Unchanged from the notes: state = globals + per process (location, locals) + channel
contents; step = one executable statement of one process; executability is the only
synchronisation (an expression statement executes iff non-zero; `c!` iff the channel is
not full, rendezvous iff a matching `c?` is executable now; `c?` iff the head message
matches the constants of the pattern); a state with nothing executable is a deadlock
unless every process is terminated or at an `end` label (02 гл. 6); channels are FIFO
and lossless (no SPIN `-m` mode); `progress`/`accept` labels are parsed and become
meaningful with G4.

## 3. Outside the subset — the `not-executed` rule

Every row of this table was derived by running `mcd parse` on a minimal model that
exercises that one construct: the probes are `evals-workspace/subset-probes/*.pml`,
the driver is `probe.py` beside them, and the run is recorded in
`steps/g3-evals3-subset-probe.md`. Re-run it rather than trusting the table —
`python3 probe.py --mcd /tmp/mcd` prints a disagreement for any row that has drifted.

Three outcomes, not two:

- **rejected** — exit code 2, the honest report is `not-executed` for every property,
  with the construct, the line, and (where one exists) the rewrite;
- **parsed** — inside the subset, whatever anything else says;
- **parsed with a warning** — accepted, but the semantics are not SPIN's. One
  construct is in this class, `pc_value` (§2); do not report it as a boundary and do
  not carry its verdict to SPIN.

Most rejections carry `kind: outside-subset`, and the row below says where they do
not. `kind: syntax` and `kind: semantic` normally mean a model error to fix rather
than a boundary to explain — with the one exception noted for `P[i]:var`.

| Construct | Rejection `kind` | Corpus file | Rewrite to offer |
|---|---|---|---|
| `unless` | `outside-subset` ("plan 14 §5.2: outside the subset") | `CH7/example1.pml`–`example3.pml`, `CH3/pots.pml` | explicit `do` with a guard on the escape condition |
| `c_code`, `c_expr`, `c_decl`, `c_state`, `c_track` | `outside-subset` ("embedded C is outside the subset"; a `c_track` file is refused at its `c_code`) | `CH17/simple1.pr` (line 1), `CH17/simple2.pr`, `CH10/fahr.pml` | model the C effect as Promela assignments and guards |
| `eval(e)` in a receive | `outside-subset` ("plan 14 §5.2: not in the corpus, outside the subset") | — | receive into a variable and guard on it |
| `priority n` | `outside-subset` ("process priorities are outside the subset") | `CH5/pathfinder.pml` | an explicit turn variable, if the priority matters to the property |
| bitwise `&`, `\|`, `^`, `~` and the shifts `<<`, `>>` | `outside-subset` ("bitwise operator …") | — | arithmetic, or a small array of bits |
| `?:` — the conditional expression `(c -> a : b)` | `outside-subset` ("conditional expression (c -> a : b)") | — | an `if … fi` with two options |
| `run` **inside a larger expression** | `outside-subset` — the message says it exactly: "run is accepted as a statement, on its own or as `pid = run P(...)`, not inside a larger expression" | `CH3/notpossible.pml` (`!run A()`; SPIN refuses it too) | assign first (`pid = run P()`), then test the pid |
| remote **variable** reference `P[i]:var` | **`syntax`**, not `outside-subset` ("expected \";\" or \"->\" after a statement, got :") — the one place where a `syntax` refusal is a boundary rather than a typo | `CH15/*` | a global the process writes |
| `c?[…]` (poll), `c??` (random/sorted receive), `c?<…>` (copy receive) | `outside-subset` ("channel poll (?[…])", "random receive (??)", "copy receive (?<…>)") | — | `nempty(c)` as a guard, then an ordinary receive |
| `unsigned` | `outside-subset` | — | `byte` / `short` / `int` with an explicit range check |
| `hidden`, `show`, `local` qualifiers | `outside-subset` ("variable qualifiers are outside the subset") | — | drop the qualifier; it does not change behaviour |
| `ltl name { … }` blocks | `outside-subset` ("inline LTL blocks are outside the subset") | — | a never claim, or `--ltl` on the command line |
| `#include` | `outside-subset` ("the model must be a single file") | `CH15/*` | paste the included text |
| remote **label** reference `P@label` | `outside-subset` ("remote reference (P@label)") — in a never claim and in an `--ltl` formula. **CTL accepts it** and normalises it to `pc(…)` (`properties-ltl-ctl.md` §2) | — | ask it in CTL, or add a `progress` label or a variable |

**Lifted by G5** — these were listed here while G5 was unbuilt and are now **inside**
the subset; probes accept all of them: `inline`, `typedef`, `provided (e)`, channels
carried in messages, arrays of channels, uninitialised channel variables, `_nr_pr`,
`pc_value` (with the warning of §2), and `run` as the whole right-hand side of an
assignment. Do not report any of them as a boundary.

### Name collisions — `semantic` refusals, and SPIN refuses them too

These are **not** boundaries of the subset: nothing here is a construct the engine
declines to support. They are model errors — the engine names the identifier, the file
and the line, and SPIN 6.5.2 rejects the same models. Report them as "fix the model",
quoting the message.

The **status** is still `not-executed`, and so is the status of every other rejected
input: the frontend refused the file, nothing ran, and `evidence-and-status.md` §2 row 1
gives that case `not-executed` / `unknown`. What the reason says is what distinguishes
the two: "outside the subset, construct X" sends the user to a different tool, "name
collision at line n" sends them back to the model. The same holds for a property the
engine will not compile — an undeclared variable in an `--ltl`/`--ctl` formula answers
`"status": "not-executed"` with the offending atom named, not `invalid-model`, because
`invalid-model` is reserved for a defect a **running** search walked into.

**The rule** (established by running 52 collision shapes against SPIN, not inferred
from a couple of examples — `steps/g5-addendum2-confirmation.md` §2): a declaration is
an error when the name is **visible where it stands** — in the same scope, or in a
scope still open around it (an enclosing block, the proctype body, a parameter, a
global). It is legal only when the sole earlier declaration was in a scope that has
since **closed**. Only `{ }` opens a scope; the options of an `if`/`do` do not, so two
options declaring one name are a redeclaration.

| Collision | Probe |
|---|---|
| a local redeclared while an enclosing scope is still open, or twice in one scope, or over a parameter, or over a global | `out-redeclare-enclosing`, `out-redeclare-same-scope`, `out-redeclare-parameter`, `out-redeclare-global` |
| two options of one `if`/`do` declaring the same name (options open no scope) | `out-redeclare-if-options` |
| two statements of one proctype carrying the same label | `out-duplicate-label` |
| one identifier used as both a label and a variable, in either order | `out-label-and-variable` |
| a label whose name is already a global, a channel, an `mtype` constant or a proctype | `out-label-and-global`, `out-label-and-mtype` |
| a proctype whose name is already an `mtype` constant, a channel or a global | `out-proctype-and-global`, `out-proctype-and-mtype` |
| two globals of one name | `out-two-globals` |

**The legal case**, and what it means: sibling blocks, where the first scope closed
before the second opened (`{ byte n; … }; { byte n; … }`, and the two expansions of an
`inline` that declares a variable). SPIN keeps **one** variable and re-initialises it
at each declaration — `pan -d` on `engine/testdata/promela/redeclared-siblings.pml`
gives `n = 0`, `n = 1`, `n = 0`, `n = 2` and 6 states, and the engine gives the same 6.
Do not tell a user those are two variables (probe `in-redeclare-siblings`).

### Stricter than SPIN — two deliberate refusals

Both are choices, recorded so that a differential run does not rediscover them as
defects, and so that a user porting a model from SPIN is told why it stopped.

| Refused here, accepted by SPIN | Why the engine refuses |
|---|---|
| **`run` whose arity does not match** — `run P()` for `proctype P(byte x)` (probe `out-run-arity-few`; SPIN fills the missing argument with zero and accepts, while rejecting a *surplus* argument) | SPIN's own behaviour is asymmetric and so is no model: a surplus argument is an error, a missing one is a silent zero. A missing argument is nearly always a typo or the trace of an edit that removed a parameter, and a silent zero produces a model the author did not write — then a `verified` about someone else's model, the exact failure the differential apparatus exists to prevent. The cost of refusing is bounded and visible: exit 2, `not-executed` (nothing ran, so no verdict is claimed), a message naming both counts. `run P(1, 2)` is refused too, naming both counts (`out-run-arity-many`) |
| **sibling blocks declaring one name with different types** — `{ int y; … }; { byte y; … }`, or a scalar and an array (probe `out-sibling-different-types`; SPIN makes two separate variables) | the engine keeps one flat set of locals per process, where the name *is* the slot. Accepting would mean renaming, which changes the state-vector layout and every counterexample in which the name appears — a bad trade for a shape no corpus model uses |


## 4. Reading a rejection

The CLI prints `{"error": {"kind", "status", "path", "message"}}` on stdout with exit
code 2; the MCP `mc_parse` answers `outcome: rejected` with `rejection {kind,
construct, file, line, reason}` (see `engine-tools.md` §2 and §4). Real example,
`CH17/simple1.pr`:

```
kind    outside-subset          status  not-executed
path    Promela - examples/CH17/simple1.pr:1:1
message construct outside subset: c_code (embedded C is outside the subset) (simple1.pr, line 1)
```

| `kind` | Meaning | Answer |
|---|---|---|
| `outside-subset` | the grammar is not accepted (§3) | `not-executed` with construct and line; rewrite from §3 |
| `syntax` | the text is not Promela the parser can read | fix the model; quote the position |
| `semantic` | undeclared variable, type mismatch, bad `run` arity | fix the model |

`invalid-model` never comes from the parser: it is a verdict of the **execution**
(overflow, blocked `d_step`), with the run attached.

## 5. Narrowings relative to plan §5.2 (recorded for the plan owner)

- **`run` only as a straight-line statement in `init`** — a G1 narrowing, **lifted
  in G5**: `run` is now accepted in a loop and recursively, each proctype has a pool
  of `--max-procs` instances (default 8), and the pool bound is what keeps the state
  vector finite. The entry is kept here because the plan's §5.2 still records the
  narrowing; it no longer describes the engine.
- **Overflow policy.** Plan §4.1 (`invalid-model`) is implemented; SPIN wraps. A
  "wrap like SPIN" mode would be one flag at the store; it does not exist today.
- **Constructs the plan defers to v1 and G5 built anyway.** `inline`, `typedef`,
  `provided`, channels in messages, arrays of channels, `_nr_pr`, `pc_value` and
  `run` in a loop are accepted now, ahead of the plan's own wording, which still
  files them under v1. The engine's behaviour is the fact; §3 lists what is actually
  refused.
- **`_pid`, the `mtype { }` form, `nempty`/`nfull`, `#ifdef` family, `xr`/`xs`** are
  accepted although §5.2 does not list them literally, because the MVP corpus files
  need them (`CH2/mutex_flaw.pml`, `CH2/mutex.pml`, `CH4/dijkstra_progress.pml`,
  `CH4/prop.pml`, `CH3/xr.pml`). Recorded in `steps/g3-docs-confirmation.md` and
  `steps/g1-confirmation.md` as plan questions; the engine's behaviour is the fact.

## 6. Style rules that keep models checkable (02 гл. 5, 07 лекция 6)

- Only control data in the state; small domains; queues as small as the property
  allows — record every size as a parameter of the result.
- Nondeterministic environment for losses, duplicates, inputs.
- `atomic` only where the real system is indivisible (`pitfalls.md` §5).
- Labels (`end`, `progress`, `accept`) placed deliberately; a missing `end` label
  turns a legitimate final state into a deadlock report.
- One `proctype` per component; a monitor process for a global invariant only if you
  checked it does not add interleavings the property can observe (07 лекция 6).
- Watch the domain: a `byte` counter that the real system lets wrap must be modelled
  with an explicit `% 256`, or the engine stops with `invalid-model` where SPIN
  would silently continue.
