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
only under `#if 0`) and remote references are covered by the engine's own test models
(`engine/testdata/promela/`) and unit tests, not by the corpus.

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
| Properties | `never` claim (`never { … }`; parsed and stored as a claim process, **not executed** until G4 — the engine emits a warning and **no property record** for the claim; `#ifdef` pairs select claims, as `CH4/prop.pml` with `-D PHI`), `assert`, `end`/`progress`/`accept` labels |
| Expressions | integer arithmetic and comparison, `&&`, `||`, `!`, `%`; remote label references `P@label` inside never claims |

## 2. Semantic decisions users will notice against SPIN

All of these were settled by probes against `pan` (SPIN 6.5.2, `-DNOREDUCE`); for the
24 files of chapters 2–3 that the parser accepts, the state counts match `pan -c0`
exactly except where this table says otherwise (the other 12 files are rejected before
any search, §3).

| Topic | Engine | SPIN (`pan`) | What to tell the user |
|---|---|---|---|
| **Byte/short/int overflow** (`byte` leaving 0–255, channel capacity exceeded) | `invalid-model`, evidence `unknown`, the run to the offending step attached as `counterexample`, the overflow named in `reason`; every property still undecided at that point gets `invalid-model` (plan §4.1) | **wraps silently** (`255 + 1 = 0`) and keeps searching | Say it explicitly: `CH3/counter.pml`, `counter2.pml`, `xr.pml` give `invalid-model` here and "no errors" in SPIN. The model's domain, not the engine, is what differs; fix or justify the domain before any property claim |
| **Blocking inside `d_step`** | `invalid-model` with reason "block in d_step seq" (03 гл. 5: a modelling error) | run-time error, search aborts | Same verdict class; the engine attaches the run |
| **Nondeterminism inside `d_step`** | the first executable alternative is taken (as SPIN) | same | — |
| **`atomic` storage rule** (explanatory) | intermediate states inside an `atomic` sequence are **not stored** while the holder can move; the state where an `atomic` sequence is interrupted (its statement blocks) **is stored** | same rule | This is why `App_C/petrinet1` has 8 states, not 28: the marking updates inside `atomic` are one stored step. Invariants and `reach` are checked on **stored** states; `assert` on every step. Counters compare with `pan -c0`, not with the number of statements executed |
| **`run`** | every proctype has a **pool** of instances (`--max-procs`, default 8, bounds it); the static processes (`active`, `init`) come first in textual order, then each proctype's pool; **every `run` draws from its own proctype's pool and takes the first dormant slot at the moment it fires** | same, including that a slot — and so a pid — is **reused** once its process has died | The choice of slot is made during the search, not by the frontend: two `run`s of the same proctype with **equal arguments** are the case that shows it. `engine/testdata/mutate/state-count/you_run2-equal-run-arguments.pml` gives 12 states, exactly `pan -c0`; a per-statement slot gave 14 and was a real divergence, found by the K3 mutation campaign and fixed in the G5 addendum (`steps/g5-addendum-confirmation.md` §4). Never tell a user that pids are not recycled, and do not read two instances of one proctype as two distinct identities unless the model stores the pid itself |
| **Rendezvous** | one step for sender and receiver; each matching receiver is a separate transition; the trace step carries `partner`; rendezvous inside `d_step` is outside the subset | same | — |
| **`else`** | dynamic: executable iff no other option of the same `if`/`do` is | same | — |
| **`timeout`** | two-phase: true iff nothing at all is executable with `timeout` false (02 гл. 13) | same | an abstraction of "stuck", not a clock |
| **Process termination** | the `-end-` transition of a process is executable only when no younger process is alive (SPIN removes only the last process of the vector); locals are zeroed | same | end states are valid only when every process is terminated or at an `end` label |
| **Never claim** | parsed as a process with `claim: true`, **not executed**; the report carries the warning "never claim (line N) parsed and stored as a claim process; not executed in this engine version — safety properties only; the claim's product with the system is G4" and **no record** for the claim; the search is the safety search of G0/G1 | executed in lockstep | until G4 you assign `not-executed` yourself to the property the claim expresses, quoting the warning as the reason; the `deadlock`/`assert` properties of the same model are still checked and reported |
| **`printf`** | a step without effect; warning "printf ignored" | prints | properties cannot observe it |
| **Undeclared variable** (`CH3/scope.pml`) | `kind: semantic` rejection | SPIN also refuses | fix the model |

Unchanged from the notes: state = globals + per process (location, locals) + channel
contents; step = one executable statement of one process; executability is the only
synchronisation (an expression statement executes iff non-zero; `c!` iff the channel is
not full, rendezvous iff a matching `c?` is executable now; `c?` iff the head message
matches the constants of the pattern); a state with nothing executable is a deadlock
unless every process is terminated or at an `end` label (02 гл. 6); channels are FIFO
and lossless (no SPIN `-m` mode); `progress`/`accept` labels are parsed and become
meaningful with G4.

## 3. Outside the subset — the `not-executed` rule

The parser refuses these with `kind: outside-subset` and names the construct and the
line; the honest report is `not-executed` for every property, with the construct, the
line, and (where one exists) the rewrite. `kind: syntax` and `kind: semantic` refusals
are model errors to fix, not boundaries to explain.

> **This table predates the G5 subset extension and is wider than the engine's
> refusals now are.** Rows marked "v1 (G5)" were written when G5 was unbuilt; probes
> of the current binary accept `provided` (`CH3/toggle.pml`), `inline`
> (`CH3/inline.pml`), `typedef` (`CH3/typedef.pml`) and channels carried in messages
> (`CH3/rendezvous2.pml`, `CH15/client_server.pml`), all of which this table still
> calls deferred. `unless` and `c_code` are still refused, as the plan intends.
> Until the row-by-row pass against `steps/g5-confirmation.md` is done, **do not
> report a construct as outside the subset on the strength of this table alone** —
> run `mcd parse` and quote what the engine actually says. A construct wrongly
> called `not-executed` is the same kind of error as a verdict wrongly claimed.

| Construct | Tier | Corpus file | Rewrite to offer |
|---|---|---|---|
| `inline name(args) { … }` | v1 (G5) | `CH3/inline.pml`, `CH2/prodcons2.pml` | paste the body by hand (say so in the report) |
| `typedef` | v1 (G5) | `CH3/typedef.pml` | flatten into scalars/arrays |
| `provided (e)` | v1 (G5) | `CH3/toggle.pml`, `CH5/pathfinder.pml` | an explicit turn variable, if the priority matters to the property |
| channels as message fields, arrays of channels, channel variables, uninitialised channels | v1 (G5) | `CH3/rendezvous2.pml`, `CH3/pots.pml`, `CH3/wc.pml`, `CH15/client_server.pml` | static channels per pair |
| `_nr_pr` | v1 (G5) | `CH15/client_server.pml` | a counter the model maintains itself |
| `unless` | excluded | `CH7/example1.pml`–`example3.pml`, `CH3/pots.pml` | explicit `do` with a guard on the escape condition |
| `c_code`, `c_expr`, `c_decl`, `c_state`, `c_track` | excluded (NFR-004: no host code) | `CH17/simple1.pr` (line 1), `CH17/simple2.pr`, `CH10/fahr.pml` | model the C effect as Promela assignments and guards |
| `eval(e)` in receive, `priority`, bit operators, `?:`, `run` inside an expression, remote variable references, `c?[…]`/`c??`/`c?<…>` (poll, sorted, random, copy), `unsigned`, `hidden`/`show`/`local` qualifiers | excluded / not in the plan | `CH3/notpossible.pml` (`run` in an expression — SPIN refuses too) | — |
| `ltl name { … }` blocks | excluded (plan gives `never { }` priority) | — | a never claim (G4) |
| `#include` | outside | `CH15/*` | paste the included text |
| block-scoped redeclaration of a local (SPIN 6 allows) | outside, with a message | none in the corpus | rename |

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
