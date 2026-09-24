# Promela subset accepted by the engine

Sources: `model-check-skill-notes/14-skill-building-plan.md` §5.2 (grammar by
version), §4.1 (deadlock, overflow), §2.1 (corpus rows); semantics from
`model-check-skill-notes/07-lectures-01-09.md` лекции 4–5 (executability,
blocking, `if`/`do`/`else`, channels, rendezvous, program-graph semantics) and
`model-check-skill-notes/02-design-and-validation-of-computer-protocols.md` гл. 5
(validation models), гл. 6 (`assert`, `end`, `progress`, `accept`), гл. 13
(`timeout` becomes available only after the ordinary transitions failed);
`model-check-skill-notes/03-karpov-model-checking.md` гл. 5 (`atomic`, `d_step`).

The engine parses a **fixed subset** of Promela. Everything outside it is rejected
with the line and the construct named, and the property gets `not-executed` — not
`invalid-model`: a model that uses `unless` is a fine model that this engine cannot
read (11 §19, plan §2.1). Do not work around the parser; rewrite inside the subset
or explain the boundary.

## 1. Grammar by version

Three tiers. MVP is what G0–G1 deliver (chapters 2–3 of the corpus without `inline`,
`typedef`, `unless`, `provided`); v1 is what G5 adds; "excluded" never enters.

### MVP

| Area | Accepted |
|---|---|
| Processes | `proctype`, `active proctype`, `active [N] proctype`, `init`, `run P(args)`; parameters of the scalar types below; `_pid` |
| Types | `bit`, `bool`, `byte`, `short`, `int`, `mtype`; one-dimensional arrays of fixed length; `mtype = { a, b }` and `mtype { a, b }` declarations |
| Channels | `chan c = [N] of { t1, t2, … }` with `N ≥ 0` (`0` = rendezvous); `c!e1,e2` / `c!e1(e2,…)`; `c?x,y` with constants (pattern match) and variables; `len(c)`, `empty(c)`, `full(c)`; `nempty(c)`, `nfull(c)` as their negations |
| Control | `if … fi`, `do … od`, `::` options, `->` and `;` as separators, `else`, `break`, `goto`, labels; label prefixes `end`, `progress`, `accept` |
| Atomicity | `atomic { … }`, `d_step { … }` |
| Statements | assignment, expression statements (guards), `assert(e)`, `skip`, `true`, `false`, `timeout` |
| Preprocessor | `#define NAME body` and `#define NAME(args) body` (needed by `App_C/petrinet1`), `#ifdef`/`#ifndef`/`#if 0`/`#else`/`#endif` as used by `CH4/prop.pml`; macro names are expanded before parsing and are not visible in traces (07 лекция 4) |
| Properties | `never { … }` claim, `assert`, `end`/`progress`/`accept` labels |
| Expressions | integer arithmetic and comparison, `&&`, `||`, `!`, `%`, remote label references `P@label`, `P[i]@label` in never claims |

### v1 (G5)

| Construct | Why later | Corpus file |
|---|---|---|
| `inline name(args) { … }` | macro-like expansion with its own scoping | `CH3/inline.pml`, `CH3/inline2.pml` |
| `typedef` | structured state vector | `CH3/typedef.pml` |
| `provided (e)` on a proctype | priorities; also forbids POR | `CH5/pathfinder.pml` |
| channels as message fields, arrays of channels | dynamic channel passing | `CH15/client_server.pml` |
| `_nr_pr` | process counting | `CH15/client_server.pml` |
| `run` inside loops (dynamic process creation) | bound on `_nr_pr` needed for finiteness | `CH15/client_server.pml` |

### Excluded (never)

| Construct | Reason | Corpus file |
|---|---|---|
| `c_code`, `c_expr`, `c_decl`, `c_state`, `c_track` | embedded C is host code; the engine executes no host code (NFR-004) | `CH17/simple1.pr`, `CH17/simple2.pr`, `CH10/fahr.pml`, `CH15/uts_model` |
| `unless` | escape semantics outside the guarded-command IR | `CH7/example1.pml`–`example3.pml`, `CH3/*` where used |
| `eval(e)` in receive | not in the corpus; not in the plan | — |
| `ltl name { … }` blocks | the corpus states properties as never claims, and the plan gives `never { }` priority | — |
| `xr c` / `xs c` channel assertions | not listed in plan §5.2; `CH3/xr.pml` keeps them under `#if 0` | `CH3/xr.pml` |

### Accepted and ignored

| Construct | Behaviour | Corpus file |
|---|---|---|
| `printf` | accepted syntactically and **ignored with a warning** (plan §5.2); it has no effect on the state, so properties cannot observe it | `CH14/version1` (MSC prints) |

### Items in the MVP table that plan §5.2 does not list literally

`_pid`, the `mtype { … }` declaration form without `=`, `nempty`/`nfull`, and the
preprocessor conditionals (`#ifdef`, `#if 0`, `#else`, `#endif`) are not named in
plan §5.2. They are listed as MVP here because the MVP corpus files need them
(`CH2/mutex_flaw.pml` uses `_pid`, `CH4/dijkstra_progress.pml` the `mtype { }`
form, `CH4/prop.pml` the conditionals). Until the plan confirms them, treat a parser
rejection of one of them as a plan question, not as a user error.

If a rejected construct is needed for the user's model, the honest report is
`not-executed` with the construct, the line, and (where one exists) the rewrite:
`unless` → an explicit `do` with a guard on the escape condition; `inline` → paste
the body (MVP) or wait for v1; `provided` → an explicit turn variable if the
priority is essential to the property.

## 2. Semantics the engine implements

The reference semantics is the program-graph / transition-system semantics of
07 лекция 5 and the operational reading of 02 гл. 5 and 13.

- **State** = the values of all global variables, for each running process its
  control location and local variables, and the contents of every channel
  (plan §4.1). Two states are equal when all of these are equal.
- **Step** = one executable statement of one process (interleaving). Which process
  moves is a nondeterministic choice; which option of an `if`/`do` is taken is a
  second, independent choice (07 лекция 4). The engine enumerates both in a fixed
  order (by process, then by option), which makes results deterministic.
- **Executability** is the only synchronisation primitive: an expression statement
  is executable iff its value is non-zero; an assignment is always executable;
  `c!` is executable iff the channel is not full (rendezvous: iff a matching `c?` is
  executable in another process right now); `c?` is executable iff the head message
  matches the constants in the pattern; `assert(e)` is always executable and reports
  a violation iff `e` is zero. A process whose current statement is not executable
  is blocked and simply does not move (07 лекция 4).
- **`if`/`do`**: any option whose first statement (the guard) is executable may be
  chosen; `else` is executable iff no other option is. `do` repeats until `break`
  or `goto` leaves it. Nondeterminism here models an unknown environment; do not
  turn it into a "typical" scenario (02 гл. 5).
- **`timeout`** is true iff **no statement of any process** is executable in the
  current state (02 гл. 13). It is an abstraction of "the system is stuck", not a
  clock (10 §13). A model that uses `timeout` for recovery is untimed.
- **`atomic { … }`**: the sequence executes without interleaving as long as its
  statements are executable; if a statement inside blocks, the atomic sequence is
  interrupted, other processes may run, and it resumes later (plan §5.2). Wrapping
  code in `atomic` removes interleavings — see `pitfalls.md` §5 before you do it.
- **`d_step { … }`**: like `atomic` but deterministic and non-blocking; a blocking
  statement inside `d_step` is a modelling error (03 гл. 5); the engine is expected to
  report it as `invalid-model` (to be confirmed in G1).
- **Channels** are FIFO. Capacity `0` is rendezvous: send and receive happen in one
  step of both processes. Capacity `N > 0` is asynchronous; a send on a full channel
  blocks; the plan defines no message-loss mode (SPIN's `-m`), so do not assume one.
- **Termination and deadlock**: a process that reaches its end is done. A state in
  which no statement is executable is a **deadlock** unless every remaining process
  is at an `end`-labelled location (or has terminated) — then it is a valid end state
  (02 гл. 6; plan §4.1).
- **`progress` labels** mark locations that must be visited infinitely often; a
  reachable cycle that visits none is a non-progress cycle (v1 search mode).
- **`accept` labels** mark Büchi acceptance in never claims: a reachable cycle
  through an `accept` location of the claim is an acceptance cycle — a violation
  of the property the claim negates (07 лекция 7).
- **Domains**: `bit`/`bool` 0–1, `byte` 0–255, `short` and `int` machine ranges as
  in SPIN. Leaving the domain (wrap-around) is reported as `invalid-model`, not as a
  property violation (plan §4.1). `App_C/ex1` (a `byte` incremented forever) is the
  corpus detector for this; `App_C/ex2` fills a channel of capacity `N` — read the
  engine's report to see whether it hit a domain overflow (`invalid-model`) or a
  blocked send with nothing else enabled (deadlock); do not guess.
- **`never` claim**: executes in lockstep with the system, one claim step per system
  step, observing but never changing the state. A claim that blocks stops the
  exploration of that path (07 лекция 7).

## 3. Reading `mc_parse` output

| Field | Meaning | Do |
|---|---|---|
| `ir` | the intermediate representation | pass it on unchanged |
| `mapping` | IR element → file, line, user name | keep it for `mc_explain`; it is what makes traces readable (NFR-011) |
| `warnings` | `printf` ignored, unused labels, capacity defaults, unreferenced `mtype` values | mention the ones that affect the property in the report |
| `error` | position and construct | classify: outside subset → `not-executed`; syntax error → fix the model |

## 4. Typical parser rejections and what to answer

| Message names | Tier | Answer |
|---|---|---|
| `c_code`/`c_expr` | excluded | `not-executed`; the engine runs no C; offer to model the C effect as Promela statements |
| `unless` | excluded | `not-executed`; offer the `do`/guard rewrite |
| `inline`, `typedef`, `provided`, channel in message | v1 | `not-executed` until v1; for `inline`, offer manual expansion |
| unknown identifier in a never claim | MVP | the atom is undefined: fix the `#define` or the label name; do not run |
| domain overflow at parse time (constant too large) | MVP | `invalid-model` |

## 5. Style rules that keep models checkable (02 гл. 5, 07 лекция 6)

- Only control data in the state; small domains; queues as small as the property
  allows — record every size as a parameter of the result.
- Nondeterministic environment for losses, duplicates, inputs.
- `atomic` only where the real system is indivisible.
- Labels (`end`, `progress`, `accept`) placed deliberately; a missing `end` label
  turns a legitimate final state into a deadlock report.
- One `proctype` per component; a monitor process for a global invariant only if you
  checked it does not add interleavings the property can observe (07 лекция 6).
