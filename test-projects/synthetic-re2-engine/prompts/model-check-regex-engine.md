# Model-check request: ordered Pike VM abstraction

You are reviewing the design of a synthetic regular-expression engine. **Do not
write, modify, build, fuzz, or execute the production regexp implementation.** Use
the bundled `model-check` plugin only to construct and verify a finite Promela,
Petri-net, or canonical-IR abstraction of the VM state machine described below.

## Goal

Model one bounded execution of an ordered Pike-style NFA VM. The objective is to
check control-flow and resource invariants, not to prove full Unicode or C++ RE2
semantic equivalence.

The abstraction must include:

- a finite instruction program with `Rune`, `Split`, `Save`, `Assert`, `Match`,
  and `Fail` operations;
- a finite input of at most 4 positions, including one malformed-UTF-8 byte that
  decodes as `U+FFFD` with width one byte;
- an ordered thread list, an epsilon-closure worklist, visited-PC flags, capture
  slots, and a current byte position;
- greedy and lazy `Split` transition order;
- a compile-rejection state for syntax or resource-limit failures;
- explicit step, program-size, group-count, and input-size bounds.

## Required properties

Check each property separately and preserve every counterexample/witness artifact.

1. **Epsilon closure terminates:** the worklist empties before the stated step
   budget when the program and input are within the model bounds.
2. **PC uniqueness:** at an input position, no program counter enters the ordered
   thread set more than once.
3. **Capture-slot safety:** every `Save` accesses a slot within the fixed capture
   array; no accepted match reports an out-of-range span.
4. **Input-span safety:** any accepted match has `0 <= start <= end <= input_bytes`.
5. **Priority preservation:** a greedy split prefers its consume branch; a lazy
   split prefers its exit branch. A lower-priority duplicate thread cannot replace
   a higher-priority thread at the same program counter and input position.
6. **Rejection isolation:** a pattern rejected for unsupported syntax, repeat
   limit, program limit, or input limit never reaches the VM transition loop.
7. **Malformed UTF-8 accounting:** the malformed byte advances exactly one byte
   while presenting `U+FFFD` to rune predicates.
8. **No implicit backtracking:** the model has no recursive call-stack state and
   the number of active VM threads is bounded by the program-size bound.

## Execution protocol

1. State the chosen modelling formalism and the finite bounds before execution.
2. Run `mc_parse`, then a short fixed-seed `mc_simulate`, then `mc_estimate`.
3. Check the required safety/reachability properties with explicit state, depth,
   time, and memory budgets. Use LTL only when it adds a distinct liveness claim.
4. For every `violated` result, call `mc_explain` and report the exact abstraction
   defect, not a claim about the unmodelled production engine.
5. Run `mc_manifest` and retain its artifact path.

## Reporting rules

For every property report its status (`verified`, `violated`, `inconclusive`,
`unknown`, `not-executed`, or `invalid-model`), evidence, budgets, abstraction
bounds, fairness assumptions, and links/identifiers for every witness or
counterexample. A bounded or time-limited run is not a proof. Clearly distinguish
claims about this finite model from claims about a full RE2-compatible engine.
