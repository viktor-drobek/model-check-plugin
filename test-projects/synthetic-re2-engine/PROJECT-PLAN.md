# Synthetic RE2-Semantics Engine — Project Plan

## Status and scope

This directory will become a standalone Go 1.26.9 test project for a deliberately
small regular-expression engine. This plan is a design artifact only: it does not
run an LLM to generate a candidate implementation, execute a candidate, or claim
conformance.

The project has four deliverables:

1. a regexp engine implemented without ready-made regexp libraries;
2. frozen, RE2-generated conformance logs plus an optional log generator;
3. differential fuzzing against a RE2 oracle;
4. a model-checking prompt and candidate-comparison scripts for future LLM-produced
   implementations.

## Non-negotiable constraints

- Production packages must not import `regexp`, `regexp/syntax`, PCRE bindings, or
  any third-party regular-expression implementation.
- The engine must use a bounded-memory Thompson-style NFA virtual machine, not
  recursive backtracking. A match attempt must not have exponential runtime in the
  input length for the declared feature set.
- `regexp` is permitted only in oracle-labelled commands and test packages that
  generate logs or perform differential comparison. CI must reject forbidden
  production imports.
- RE2-generated logs are immutable test inputs. Regenerating them is an explicit
  maintainer action that records the Go and RE2-oracle version plus the generator
  seed.
- Future LLM candidate execution is opt-in. No comparison script may silently call
  an LLM service or execute a candidate merely because the repository tests run.

## Target API and wire contract

The engine exposes a command-line JSONL protocol so the native implementation and
future candidates have exactly one comparison surface:

```json
{"op":"compile","pattern":"(ab)+","flags":{"longest":false}}
{"op":"find","pattern":"a(b)?","text":"xxab","flags":{"longest":false}}
```

Successful `find` responses contain `matched`, `index` (byte offsets), and one
pair of offsets per capturing group; unmatched and unmatched groups use `-1`.
Compilation failures are structured `error` responses with a stable error class.
The initial operations are `compile`, `match`, `find`, and `find_all`; replacement,
streaming, and APIs outside this protocol are explicitly out of scope.

## RE2-like semantic contract

The implementation must document and test the following contract before adding
syntax:

- default matching is **leftmost-first**, selecting the first successful ordered
  RE2 alternative; `longest: true` selects the leftmost-longest match where the
  operation supports it;
- text positions and capture indexes are UTF-8 byte offsets, matching Go RE2 API
  conventions; the VM consumes decoded runes and treats malformed UTF-8 exactly as
  the selected oracle does;
- greedy and lazy repetitions affect ordered-thread preference, not language
  recognition;
- the initial syntax set is literals, UTF-8 escapes, `.`, anchors, concatenation,
  alternation, grouping and named/numbered captures, character classes/ranges,
  `*`, `+`, `?`, bounded repetitions, and ASCII/Unicode class escapes that have
  explicit fixtures;
- unsupported syntax is rejected deliberately: backreferences, look-around,
  conditionals, recursion, embedded code, and unbounded Perl-only constructs;
- flags, empty matches, anchors, newline handling, invalid patterns, and capture
  disambiguation each have a documented RE2-oracle fixture.

The semantic version in every test log must state the Go version and the selected
oracle mode. `regexp.Compile` is the default RE2-compatible oracle; POSIX-longest
mode is exercised only for tests whose flags request it.

## Review-confirmed specification (supersedes earlier ambiguity)

### Oracle identity and modes

The authoritative oracle is a pinned **C++ RE2** helper built from a recorded RE2
revision in an oracle-only container. Frozen files are therefore correctly named
RE2-generated logs. Metadata records RE2 commit, compiler/container digest,
semantic mode, source-corpus hash, seed, and byte-schema version. Go `regexp` may
be used only as an additional, explicitly named `go-regexp@<go-version>` diagnostic
oracle; it never supplies an authoritative expected result.

`leftmost_first` is the default C++ RE2 mode. `leftmost_longest` is C++ RE2's
longest-match option with C++ RE2's capture result, not POSIX ERE syntax and not
Go `CompilePOSIX`. Every operation accepts this mode and frozen cases pin capture
ambiguities under both modes.

### Exact JSONL contract

Each input carries exactly one of `pattern_utf8`/`pattern_b64` and exactly one of
`text_utf8`/`text_b64`; base64 is mandatory for invalid UTF-8. Indexes are byte
offsets into the decoded original bytes. Responses have a versioned `schema` field.
`compile` is stateless validation and returns `ok`, `group_count`, and ordered group
metadata. `match` returns `matched` and, when true, the same match object as `find`.
`find` returns `matched`, `span:[start,end]`, and `groups`, where `groups` is an
array ordered by numeric index, each entry is `{index,name?,span:[start,end]}` and
a non-participating group has `[-1,-1]`. `find_all` returns `{matches:[match...]}`
and an explicit `limit`; it follows RE2's non-overlapping iteration rule and,
after a zero-width result, advances by one decoded rune (or one malformed byte).

Errors use `{error:{class,byte_offset?,message}}`. The closed classes are
`syntax`, `unsupported_syntax`, `invalid_escape`, `invalid_class`, `invalid_repeat`,
`repeat_limit`, `program_limit`, `input_limit`, and `budget_exhausted`. Error text
is diagnostic only; conformance compares class and applicable offset.

### Supported boundary and resource limits

A versioned syntax matrix lists every C++ RE2 construct relevant to this project
as `supported`, `rejected`, or `fuzz-excluded`, including inline flags, anchors,
word boundaries, quoting, Unicode properties, and every escape. Rejected entries
name their error class. Initial hard limits are: pattern bytes 64 KiB, input bytes
16 MiB, repeat upper bound 1,000, program instructions 1,000,000, captures 1,024,
and `find_all` limit 10,000. The compiler refuses a limit before allocating the
program. The VM counts thread-instruction steps and returns `budget_exhausted`
separately from a semantic mismatch; per-operation wall-clock/RSS limits are
also enforced by the caller.

### VM and no-regexp enforcement

`internal/vm` is a Pike-style Thompson NFA VM: each ordered thread carries capture
slots, epsilon closure visits each program counter at most once per input position,
and greedy/lazy quantifiers differ by split-transition ordering. Input remains as
original bytes; decoding happens one rune at a time, and each malformed byte is
U+FFFD of width one byte. Production dependency checks run `go list -deps` and
reject `regexp`, `regexp/syntax`, cgo, `os/exec`, `go:linkname`, and any
non-allowlisted module in production packages. Test/oracle packages are separately
allowlisted and visibly labelled.

### Candidate evaluation security

The repository-owned conformance runner and differential fuzzer drive only the
candidate JSONL binary. A candidate never supplies a fuzzer, test adapter, or
oracle. `compare.sh` runs build and execution in a container or `bwrap` sandbox
with network disabled, an unprivileged user, read-only project/log mounts, empty
home, `CGO_ENABLED=0`, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOFLAGS=-mod=vendor`,
rlimits, and per-case/per-run timeouts. No candidate command is part of ordinary
unit tests or automatically invokes an LLM.

### Model-checking prompt acceptance

The prompt must ask the bundled `model-check` plugin to check a finite abstraction
with explicit bounds. Required properties are: epsilon closure terminates; every
PC enters a thread set at most once per position; capture-priority ordering is
preserved across split transitions; all group slots stay in range; an accepted
thread produces a span within input bounds; and no rejected syntax reaches the VM.
Every answer must state model bounds, status/evidence, and counterexamples.

## Architecture and implementation order

```text
syntax lexer/parser -> AST validation -> Thompson compiler -> ordered NFA VM
                                                          -> JSONL CLI
oracle log generator -> frozen logs -> conformance runner -> differential fuzzer
candidate adapter -> isolated build -> logs + fuzz runner -> comparison report
```

1. `internal/syntax`: lexer, parser, source locations, AST, and explicit unsupported
   construct rejection. No engine execution or oracle imports.
2. `internal/program`: immutable bytecode/instructions and Thompson compiler.
3. `internal/vm`: ordered thread sets, epsilon closure, rune stepping, zero-width
   assertions, captures, cancellation/budgets, and no recursive backtracking.
4. `cmd/rex`: JSONL protocol executable.
5. `internal/logs` and `cmd/conformance`: schema validation and execution of frozen
   JSONL logs against `rex`.
6. `cmd/re2loggen` and `cmd/difffuzz`: explicitly oracle-labelled tooling. These
   may import Go `regexp` but production packages cannot.
7. `cmd/candidate-compare` and shell wrappers: isolated candidate builds and
   reproducible reports; they never invoke an LLM.

Every behavior change starts with a focused Go test or a new frozen JSONL case.
The first implementation milestone is a failing literal/alternation/capture case,
not broad syntax support.

## RE2-generated logs and fuzzing

### Frozen logs

`testdata/re2logs/*.jsonl` contains one request/expected-response record per line.
Each file starts with metadata containing generator version, Go version, semantic
mode, seed, and SHA-256 of its source corpus. Cases cover successful matching,
no-match, captures, empty matches, invalid syntax, Unicode, anchors, quantifiers,
character classes, and declared rejection boundaries.

`cmd/re2loggen` regenerates logs only when called explicitly with `--out`,
`--seed`, and `--go-version`. It writes to a temporary file, validates every output
record, sorts deterministically, and requires an explicit `--replace` to update a
tracked fixture.

### Differential fuzzing

`cmd/difffuzz` generates bounded patterns and UTF-8/invalid-UTF-8 texts within the
declared syntax. For each seed it compares compilation class and observable JSONL
response from the native engine with Go's RE2-backed `regexp` oracle. It records
seed, case, shrink trace, oracle response, engine response, Go version, and
semantic mode in a replayable mismatch file. CI runs a deterministic bounded corpus;
longer randomized campaigns are manual and resource-bounded.

The fuzzer must distinguish an engine defect from a generator case outside the
implemented syntax. Exclusions are versioned and tested; they must not silently
hide a mismatch.

## LLM candidate comparison, without LLM execution

`prompts/model-check-regex-engine.md` asks a model-checking agent to construct and
verify a finite abstraction of parser/compiler/VM control flow. It must use the
bundled model-check plugin and report model bounds, property evidence, and every
counterexample. The prompt does not ask an LLM to write production code.

`tools/candidates/compare.sh` accepts an already-present candidate directory and:

1. copies it into an isolated temporary workspace;
2. rejects forbidden production regexp imports;
3. builds it with the pinned Go toolchain and the JSONL CLI contract;
4. runs the frozen RE2-generated logs;
5. invokes the candidate differential fuzzer adapter with fixed seeds and budgets;
6. writes a per-candidate machine-readable report and a normalized comparison table.

`tools/candidates/compare-all.sh` receives a manifest of candidate directories and
runs the same isolated pipeline per implementation. It does not invoke a model,
read model credentials, or use a network connection. It reports build failures,
protocol mismatches, log failures, fuzz mismatches, timeouts, and unsupported
features separately.

## Agent rules to generate

The project will contain:

- root `AGENTS.md` with the engine boundary, no-regexp rule, BDD/TDD workflow,
  test commands, RE2 semantic obligations, fuzzing limits, and documentation gate;
- `CLAUDE.md` as a verified compatibility symlink;
- focused `.cursor/rules/*.mdc` and matching `.claude/rules/*.md` for core VM,
  oracle tooling, and candidate scripts;
- `.codex/hooks.json` plus the existing tested adapter pattern, only after its
  scalar/list glob and edit-operation probes pass.

Rule bodies are English. The project will not invent a new Coddy-specific rules
surface; supported behavior is root `AGENTS.md` plus documented host integration.

## Acceptance gates

The implementation is complete only when all of these are green:

```bash
go test ./...
go test -race ./...
go vet ./...
test -z "$(gofmt -l .)"
./scripts/check-no-regexp-imports.sh
./cmd/conformance --logs testdata/re2logs
./cmd/difffuzz --seed-file testdata/fuzz/seeds.txt --max-cases <bounded-count>
```

The candidate runner has separate opt-in commands and is not part of the ordinary
unit-test target. Every run checks CPU, memory, disk, time, and generated-artifact
size before beginning.

## Cross-review protocol

This plan will be reviewed blind by these one-off reviewers:

- local Claude CLI using `opus`;
- Coddy using `neuraldeep/gemma-4-31b`;
- Coddy using `neuraldeep/qwen3.8-27b`;
- Coddy using `neuraldeep/kimi-k2.6`.

Reviewers receive this document and the intent only. They must not inspect the
working tree, invoke any LLM implementation task, build a candidate, run logs, or
run fuzzing. The orchestrator validates findings against this plan and produces a
single roadmap. A reviewer failure is reported as a failed review, not silently
replaced.
