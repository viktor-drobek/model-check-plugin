# Cross-review — Synthetic RE2-Semantics Engine Plan

## Verdict

**Needs rework; revised before implementation.** The reviewer quorum was complete,
but the original plan left key semantic, wire-format, boundedness, and candidate
isolation details unspecified. No implementation, candidate build, test, fuzzing,
or LLM project-generation run occurred during this review.

## Reviewers and quorum

| Reviewer | Model | Result | Duration |
|---|---|---:|---:|
| Local Claude CLI | `opus` | needs rework | 1m 08s |
| Coddy | `neuraldeep/gemma-4-31b` | approve with changes | 2m 55s |
| Coddy | `neuraldeep/qwen3.8-27b` | approve with changes | 2m 50s |
| Coddy | `neuraldeep/kimi-k2.6` | needs rework | 1m 57s |

**Quorum:** 4 of 4 required reviewers answered. Reviewers were blind to one
another and received the plan brief only.

## Accepted fixes

The revised plan must:

1. call C++ RE2, not Go `regexp`, the authoritative source of frozen logs and
   differential-fuzz answers; any Go `regexp` comparison is separately labelled;
2. define `longest` as the exact C++ RE2 longest-match mode, including capture
   behavior, rather than conflate it with Go POSIX syntax;
3. represent pattern/text bytes as base64 when UTF-8 is invalid, with offsets in
   original byte indexes;
4. use a Pike-style ordered-thread NFA VM with per-thread capture slots, explicit
   greedy/lazy split ordering, and one visited program counter per epsilon closure;
5. publish complete JSONL schemas for every operation, explicit `find_all`
   zero-width advancement, capture order/name data, and a closed error class set;
6. pin supported/rejected syntax and flags, repetition/program/input limits, and
   a separately reported budget-exceeded outcome;
7. make the repository-owned fuzzer drive candidate binaries through the JSONL
   protocol; candidates never provide their own evaluator;
8. isolate future candidate builds/runs with no network, an unprivileged user,
   read-only inputs, disabled cgo, local toolchain/module mode, rlimits, and
   per-case timeouts;
9. enforce the no-regexp boundary through dependency inspection plus bans on cgo,
   external process execution, `go:linkname`, and non-allowlisted dependencies;
10. list concrete model-checking properties and model bounds in the prompt.

## Rejected or deferred findings

- **Stateful compile IDs:** rejected. The JSONL contract is deliberately stateless
  per request for deterministic cross-process candidate comparison. `compile` is
  a validation/metadata operation; execution operations carry a pattern.
- **Raw-byte VM only:** rejected. The selected C++ RE2 contract is rune-aware;
  the VM retains original bytes and decodes one rune at a time, treating each
  malformed byte as U+FFFD with width one byte. This preserves byte offsets.
- **Named captures as a map only:** rejected. Maps lose numeric ordering and are
  unsuitable for duplicate/unnamed groups. The schema uses an ordered group array
  with numeric index and optional name.
- **Codex adapter omission:** deferred until the standalone project is created.
  The plan will either vendor a tested adapter with probes or document Codex as
  unsupported; it will not refer vaguely to a parent-repository implementation.

## Evidence boundary

All four review calls were plan-only, tool-less review requests. No LLM was asked
to implement or test the synthetic regexp project, and no candidate, conformance,
or fuzzing command was run.
