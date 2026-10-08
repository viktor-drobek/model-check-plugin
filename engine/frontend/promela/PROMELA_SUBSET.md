# Promela Subset Supported by the Engine

This document describes the subset of SPIN's Promela language that the
model-check engine parses, validates, and lowers to its intermediate
representation (IR). The subset is the **MVP subset** defined in plan 14
§5.2 (as amended after K1). Every construct in the language falls into
exactly one of three categories:

1. **Accepted** — parsed into the AST and lowered to the IR.
2. **Rejected with a named error** — the parser recognises the construct
   as outside the subset and returns a structured rejection naming the
   construct, its file, line, and the reason.
3. **Syntax error** — a malformed construct within the accepted subset
   (e.g. missing closing brace, unexpected token).

The division is **exhaustive**: nothing is silently accepted or silently
rejected. An unknown keyword in statement position is a syntax error,
never a silent no-op.

---

## 1. Accepted constructs

### 1.1 Variable types

| Type    | Range / Notes                                    |
|---------|--------------------------------------------------|
| `bit`   | 0 or 1                                           |
| `bool`  | false (0) or true (non-zero)                     |
| `byte`  | 0..255 (signed in SPIN, but engine uses byte)    |
| `short` | 16-bit integer                                   |
| `int`   | 32-bit integer                                   |
| `mtype` | Enumerated type declared with `mtype { ... }`    |
| `pid`   | Process ID (internal use)                        |
| `chan`  | Channel type (declared with capacity and msg fields) |

**Note on `byte` overflow**: SPIN's `pan` silently wraps byte values
(256 → 0, -1 → 255). The engine detects overflow and reports
`invalid-model` with reason "domain overflow" rather than silently
wrapping. This is a **documented systematic difference** (G1 test
`CH3/counter.pml`).

### 1.2 Type declarations

- **mtype**: `mtype { A, B, C };` — defines an enumerated type.
- **typedef**: `typedef Record { bit x; byte y; };` — defines a struct
  type. Fields are **flattened** during parsing: an instance `Record r`
  becomes variables `r.x` and `r.y` in the IR, matching SPIN's own
  flattening. Nested structs are supported (fields get dotted paths).

### 1.3 Variable declarations

Variables may be declared at the module level (global) or inside
proctype/init/never bodies (local).

```promela
global bit flag;
global chan q = [2] of { bit, mtype };
byte count = 0;
```

- Arrays: `byte buf[10];` — length must be a positive constant.
- Channel initialisation: `chan q = [2] of { bit, mtype };` — capacity
  0..255, message fields are basic types.
- Initialisers: `byte x = 42;` — right-hand side must be a constant
  expression (no variables).

**Channel restrictions**:
- Message field types are limited to: `bit`, `bool`, `byte`, `short`,
  `int`, `mtype`, `pid`, `chan`.
- User-defined typedef instances **cannot** be channel message fields;
  send their fields separately.

### 1.4 Process declarations

- **proctype**: `proctype P() { ... }` — named process template.
- **init**: `init { ... }` — initial process (exactly one allowed).
- **never**: `never { ... }` — never claim for LTL property checking
  (exactly one allowed).
- **active**: `active proctype P() { ... }` or `active [N] proctype P()`
  — spawns N instances (1..254).

Process parameters are supported:
```promela
proctype P(bit arg1, mtype arg2) { ... }
```

The `provided` guard is supported:
```promela
proctype P() provided (cond) { ... }
```

**Restrictions**:
- No array parameters.
- No initialisers on parameters.
- No `D_proctype` (SPIN internal directive).

### 1.5 Control flow

- **Blocks**: `{ ... }` — compound statement.
- **if/fi**: Non-deterministic selection.
  ```promela
  :: x == 0 -> x = 1
  :: x == 1 -> x = 2
  :: else -> x = 0
  fi
  ```
- **do/od**: Non-deterministic loop.
  ```promela
  do
  :: x < 10 -> x++
  :: x >= 10 -> break
  od
  ```
- **Labels and goto**: `L1:` and `goto L1;` — forward and backward
  jumps.
- **break**: Exits the innermost `do` loop.
- **else**: Inside `if` — taken when no other option is enabled.
- **atomic / d_step**: `atomic { ... }` and `d_step { ... }` — atomic
  blocks. `d_step` that blocks (no transition enabled inside) produces
  `invalid-model` with reason "block in d_step". A `do` loop inside the
  block stays inside it: the process keeps the exclusive control from one
  iteration to the next and gives it up when it leaves the loop. A loop that
  is the first statement of the block, and that shares its entry with another
  alternative (the block is one option of an outer `if`/`do`), is refused
  (`outside-subset`, "shares its entry with another alternative"): pan keeps a
  separate state for the inside of the block, and this engine's one location
  per point between statements cannot tell the two apart. Earlier versions
  gave the control up at the back edge of such a loop and could report an
  assertion violation pan does not find.

### 1.6 Assertions

- **assert**: `assert(cond);` — checked at runtime. Violation produces
  `violated` status with a counterexample.
- **assume**: `assume(cond);` — prunes states where cond is false.

### 1.7 Channel operations

- **Send**: `q!msg;` — standard send.
- **Receive**: `q?msg;` — standard receive.
  - Wildcard: `q?_;` — discard value.
  - Partial receive: `q?x;` — receive into variable.
  - Array element receive: `q?arr[i];`
  - Match receive: `q?1;` — match constant value.
- **Channel array indexing**: `q[i]!msg;` — send to indexed channel.
- **Struct field paths**: `r.c!msg;` — channel name can include struct
  field path.

### 1.8 Process creation

- **run**: `run P(args);` — spawn a new process instance.
- **run with target**: `x = run P(args);` — assign process ID.
- **Struct passing**: When passing a typedef instance to `run`, it is
  expanded field-by-field (matching SPIN's behaviour).

### 1.9 I/O

- **printf**: `printf("message %d\n", x);` — format string required.
- **printm**: `printm(mtype_val);` — print mtype value by name.

### 1.10 Channel hints

- **xr**: `xr q1, q2;` — hint that channels are for "request" (send
  before receive).
- **xs**: `xs q1, q2;` — hint that channels are for "response" (receive
  before send).

These are stored in the IR as hints on channel declarations.

### 1.11 Preprocessing

- **#define**: `#define MAX 10` — object-like macros.
- **#ifdef / #endif**: Conditional compilation.
  ```promela
  #ifdef PHI
  never { ... }
  #endif
  ```
- **-D defines**: Passed via `mcd check -D PHI ...` on the command line.
- **Inline expansion**: Macros are expanded before parsing.

### 1.12 Expressions

Arithmetic and boolean expressions are supported:

- **Arithmetic**: `+`, `-`, `*`, `/`, `%`
- **Comparison**: `==`, `!=`, `<`, `<=`, `>`, `>=`
- **Boolean**: `!`, `&&`, `||`
- **Unary minus**: `-x`
- **Variables**: `x`, `x[i]`, `x.f` (struct fields)
- **Constants**: integer literals
- **Function calls**: `f(args)` — but only in non-restricted contexts
- **`_nr_pr`**: the number of live processes of the model itself; a never
  claim, and the claim an `ltl` formula is checked with, are not counted. It
  falls when a process ends, in a model with `run` and in one without: the
  ending process leaves the process table, and only the youngest live process
  may. The table is carried only when a process is created by `run` or some
  expression of a process reads `_nr_pr`; a model that does neither keeps the
  vector G1 fixed against pan. **Divergence from SPIN:** pan counts the claim
  as a process, so under a `never` claim or an `ltl` formula its `_nr_pr` is
  one larger, and a verdict that reads `_nr_pr` and is checked under a claim
  can differ from pan's. Without a claim the two agree
  (`testdata/spin-divergence/`, `steps/fix-nrpr-confirmation.md`). A property
  that reads `_nr_pr` (a CTL atom, or an `invariant` / `reach` expression sent
  through MCP as IR) over a model whose own processes do not read it is
  `not-executed`: the table is decided by the model's processes alone.

**Byte arithmetic**: Operations on `byte` variables wrap silently in
SPIN. The engine detects overflow and reports `invalid-model`.

---

## 2. Explicitly excluded constructs

The following constructs are **rejected** with a structured error
(`kind: "outside-subset"`) naming the construct, file, line, and reason.

| Construct | Reason |
|-----------|--------|
| `c_code`, `c_decl`, `c_state`, `c_track`, `c_expr` | Embedded C is outside the subset |
| `unless` | Plan 14 §5.2: outside the subset |
| `priority` | Process priorities are outside the subset |
| `ltl { ... }` | Inline LTL blocks; use `never { }` instead |
| `trace`, `notrace` | Event traces are outside the subset |
| `select` | Outside the subset |
| `for` | Outside the subset |
| `hidden`, `show`, `local` | Variable qualifiers are outside the subset |
| `unsigned` | Outside the subset |
| `eval()` | Plan 14 §5.2: not in the corpus, outside the subset |
| `enabled()` | Outside the subset |
| `_last`, `np_*` | Outside the subset |
| `get_priority()`, `set_priority()` | Outside the subset |
| `D_proctype` | SPIN internal directive |
| `mtype:name` | Typed mtype (tagged unions) |
| `!!` (sorted send) | Outside the subset |
| `??` (random receive) | Outside the subset |
| `?<` (copy receive) | Outside the subset |
| `?[` (channel poll) | Outside the subset |
| Remote references (e.g. `P@label`) | Outside the subset |
| Second `never` claim | One never claim per model |
| Second `init` | One init process per model |
| Array of structs | Declare fields as arrays instead |
| User-defined typedef in channel msg | Send fields separately |
| Expression in receive arg (non-constant) | Needs `eval()`, which is outside the subset |
| Local variable in `never` claim | Outside the subset |
| Parameter with initialiser | Value comes from call |
| Array parameter | Outside the subset |

### 2.1 Unsupported property kinds

The engine supports:
- **Safety** (default): checks for assertion violations and invalid end
  states.
- **LTL**: via `never` claims (converted to properties of kind "ltl"), `ltl`
  blocks and `--ltl` formulas, with the nested depth-first search of G4.
- **Progress**: non-progress cycles (`--progress`, `progress` labels).
- **Weak fairness** (`--fairness weak`, `pan -f`) for LTL and progress
  properties (G4; `references/fairness.md`).
- **CTL** (`--ctl`, G5), by graph labelling.

The following are **not-executed** (status `not-executed` with a reason):
- **Strong fairness**: FR-008 not yet implemented.
- A property that reads `_nr_pr` over a model whose processes keep no process
  table (§1.12).

---

## 3. Semantics differences from SPIN

### 3.1 Byte overflow

| Aspect | SPIN | Engine |
|--------|------|--------|
| `byte` overflow | Silent wrap (256 → 0) | `invalid-model` with "domain overflow" |

This is a **documented systematic difference**. The engine's stricter
behaviour catches bugs that SPIN would silently miss.

### 3.2 d_step blocking

| Aspect | SPIN | Engine |
|--------|------|--------|
| `d_step` with no enabled transition | Treated as deadlock | `invalid-model` with "block in d_step" |

### 3.3 State counts

The engine's state counts match SPIN's `pan -c0` (no optimisation) for
models inside the subset, except where this document or `references/promela-subset.md`
says otherwise: a process that blocks inside an `atomic` sequence (the engine
stores the state with its exclusive-control byte set, pan does not), `_nr_pr`
under a never claim or an `ltl` formula (§1.12), a run with `--por` (the
count is that of the reduced graph) and a run with `--workers` that stops early. The pandiff tool (`tools/pandiff`) verifies
this at statement granularity by comparing the engine's statement table
against `pan -d` output.

### 3.4 Process naming

Process instances are named `<proctype>:<pid>` with SPIN's pid order:
`active` and `init` in textual order, then `run`-created processes.
Locals are qualified as `<proctype>:<pid>.<name>`.

---

## 4. Limitations and future work

### 4.1 Not yet implemented

- **Strong fairness**: only weak fairness is executed (§2.1).
- **`eval()`**: Cannot evaluate expressions at runtime in receive
  arguments.
- **`enabled()`**: Cannot query enabledness of guards.
- **Process priorities**: No priority scheduling.
- **Event traces**: No `trace`/`notrace`.
- **`for` loops**: No C-style for loops.
- **User-defined typedef in channel messages**: Must send fields
  separately.

### 4.2 Known gaps in the corpus

The test corpus (`Promela - examples/`) contains models that exercise
the subset. Models outside the subset (e.g. `CH17/simple1.pr` with
`c_code`, `CH3/pots.pml` with `unless`) are used as rejection tests.

---

## 5. Test models reference

| Model | Path | Exercises |
|-------|------|-----------|
| `mutex_flaw.pml` | CH2/ | Globals, proctype, assert, deadlock |
| `peterson.pml` | CH2/ | Mutual exclusion, proctype, channels |
| `prodcons.pml` | CH2/ | Producer-consumer, channels, printf |
| `alternatingbit.pml` | CH3/ | Buffered channels, mtype |
| `peterson2.pml` | CH2/ | Deadlock, invalid end state |
| `counter.pml` | CH3/ | Byte overflow → invalid-model |
| `macro.pml` | CH3/ | Function-like macros, #define |
| `rendezvous.pml` | CH3/ | Channel handshake, process creation |
| `prop.pml` | CH4/ | #ifdef, never claims, LTL |
| `typedef.pml` | CH3/ | Struct types, field flattening |

---

## 6. Design rationale

### 6.1 Why this subset?

The MVP subset was chosen to cover the **core verification scenarios**
found in Holzmann's "Design and Validation of Computer Protocols"
chapters 2-3, while keeping the implementation manageable. The excluded
constructs are either:

1. **SPIN-specific extensions** (c_code, priorities, traces) that add
   complexity without increasing the core verification capability.
2. **Advanced LTL/CTL features** (eval, enabled, fairness) that require
   more complex automata construction.
3. **Convenience features** (for loops, unless) that can be expressed
   with the accepted constructs.

### 6.2 Why strict rejection?

Every rejected construct is named explicitly with its location and
reason. This prevents silent misinterpretation and helps users
understand the boundaries of the engine. SPIN sometimes accepts
constructs that produce unexpected behaviour; the engine's strict
rejection is a feature, not a limitation.

### 6.3 Why byte overflow as invalid-model?

SPIN's silent byte wrap is a common source of subtle bugs. By detecting
overflow and reporting `invalid-model`, the engine helps users catch
these bugs during verification rather than missing them silently.
