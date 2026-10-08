# model-check plugin

`model-check` is an agent plugin for finite explicit-state model checking. It provides an agent skill, a deterministic Go engine, a CLI, and a seven-tool MCP server for Promela models, Petri-net JSON, IR JSON, safety properties, reachability, LTL, CTL, simulation, estimation, counterexamples, and manifests.

This directory is the complete public plugin artifact. Do not distribute only `skills/model-check/`, only `engine/bin/mcd`, or only `mcp/servers.json`; the manifest, engine, features, steps, evaluations, references, build protocol, and release metadata are part of the plugin.

## The model flow in one minute

The plugin checks a **finite model**, not the implementation directly. First choose a
small, explicit abstraction of the system and state what one atomic step means. The
frontend then lowers the input to the common intermediate representation (IR), and
the explicit-state engine explores the reachable state graph. A `verified` result is
therefore a claim about the model, its bounds, its transition semantics, and its
fairness assumptions; it becomes a claim about the implementation only after a
separate conformance argument.

```mermaid
flowchart TD
    A[System, protocol, or design] --> B{Choose a finite abstraction}
    B -->|Processes, channels, shared state| P[Promela subset]
    B -->|Places, tokens, transitions| N[P/T Petri-net JSON]
    B -->|Existing canonical graph| I[IR JSON]
    P --> Q[mc_parse / mcd parse]
    N --> Q
    I --> Q
    Q --> R[Common IR]
    R --> S[Reachable finite state graph]
    S --> E[mc_estimate]
    E --> C[mc_check / mcd check]
    C --> O{Result and evidence}
    O -->|verified| V[Property holds for the explored model]
    O -->|violated| X[Counterexample or witness]
    O -->|inconclusive| U[Increase budget or reduce the model]
    O -->|not-executed| D[Use a supported formalism or specialised tool]
```

The recommended order is: **model → parse → simulate → estimate → sanity/reachability
checks → safety/deadlock → liveness and fairness → explain every counterexample →
retain the manifest and artifacts**. See [`skills/model-check/SKILL.md`](skills/model-check/SKILL.md)
for the full decision tree and exit conditions.

## Which model should I use?

These are input formalisms, not competing verification algorithms. Promela and Petri
JSON are two ways to describe a system; IR is the normal form shared by both. Choose
the representation that makes the states, transitions, and assumptions easiest to
review.

| Model or notation | What it represents | Use it when | Particularly useful checks | Important boundary |
|---|---|---|---|---|
| **Promela subset** (`.pml`) | Concurrent processes, program locations, finite variables, shared state, FIFO channels, rendezvous, and nondeterministic interleaving | The system is a protocol or concurrent program with message exchange and process control flow | Mutual exclusion, races in the abstraction, deadlocks, reachability, response properties, progress, LTL, CTL, and weak-fairness questions | Only the documented subset is accepted; time, probability, embedded C, and unbounded data are outside this engine |
| **P/T Petri net** (JSON) | Places hold tokens; transitions fire when their input markings are available; a marking is the state | The problem is naturally about resources, workflow, causal concurrency, or token flow | Reachable hangs/deadlocks, place safety, boundedness, resource conflicts, and reachability of a marking | The frontend accepts basic place/transition nets; inhibitor arcs are rejected, and coloured/timed features require an explicit lossy translation |
| **Canonical IR** (JSON) | The engine's finite state variables, control locations, transitions, and properties after parsing | A tool needs a stable interchange format, custom properties, or reproducible debugging | Re-running a parsed model, inspecting the lowering, and attaching explicit invariant/reachability properties | Prefer generating IR with `mc_parse`/`mcd parse`; direct authoring is an advanced interface and must respect `mcd-ir/1` |
| **Automata/program abstraction** | A finite control graph plus bounded data and labelled actions | The source design is already an FSM, EFSM, statechart, or automata program | State coverage, illegal transitions, deadlock, recovery, and temporal response | Map each source event and atomic action explicitly to the model; the result is about that mapping |

### Model choice at a glance

```mermaid
flowchart TD
    Q[What is the dominant structure?]
    Q -->|Processes, messages, guards, shared variables| P[Promela]
    Q -->|Tokens, resources, workflow, firing rules| N[Petri-net JSON]
    Q -->|States and transitions already enumerated| I[IR JSON or Promela init/goto]
    Q -->|Clocks or deadlines are part of the property| T[Not executed by this engine: use timed model checking]
    Q -->|Probabilities or expected values are required| R[Not executed by this engine: use probabilistic model checking]
    Q -->|Unbounded queues, integers, processes, or heap| U[Add a visible bound, or use an unbounded/symbolic method]
    P --> F[Finite, untimed, explicit-state search]
    N --> F
    I --> F
```

A protocol can contain a timeout and still be an untimed model when `timeout` means
“no ordinary action is currently executable”. It becomes a timed-model question only
when the requirement says *within five seconds*, uses clocks, or otherwise depends on
elapsed time. The same distinction applies to “rarely”: it is a safety question if it
means “never”, but a probabilistic question if it asks for a probability.

## Plugin deployment flow

The two plugin declaration families select the same server contract. The portable
manifest uses `${PLUGIN_ROOT}`; the Claude manifest uses `${CLAUDE_PLUGIN_ROOT}`.
The shipped `mcd.exe` alias lets Windows resolve the same command name, while the
POSIX wrapper selects the matching native binary on Unix.

```mermaid
flowchart LR
    M[plugin.json or .claude-plugin/plugin.json] --> C[mcp.json or mcp/servers.json]
    C --> L[platform command resolution]
    L --> B[engine/bin/mcd or mcd.exe]
    B --> S[mcd serve]
    S --> A[session manifest and reports]
```

## What should I check?

The input model and the property logic answer different questions. Start with a small
safety or reachability check to validate the abstraction, then move to liveness only
after the trigger, atomic step, and fairness assumptions are explicit.

| User question | Property to prefer | Evidence to expect | Typical example |
|---|---|---|---|
| “Can the system ever reach a bad state?” | `reach` or an invariant/safety property | A finite witness for reachability, or a finite bad prefix for a violation | `cnt > 1`, both processes in a critical section |
| “Can the system get stuck?” | `deadlock` | A finite counterexample ending in a state with no executable transition | A producer and consumer both waiting |
| “Is this always safe?” | `invariant` | Exhaustive reachable-state search for `verified`; a finite counterexample for `violated` | `!(cs1 && cs2)`, every place has at most one token |
| “Will every request eventually receive a response?” | LTL, for example `[](request -> <> response)` | Usually a finite prefix plus a lasso when the liveness property is violated | A request that can be postponed forever |
| “From every state, is recovery possible along some continuation?” | CTL, for example `AG EF recovered` | A branching-state witness/counterexample, not necessarily one linear trace | Every reachable failure state has a route back to idle |
| “Does useful work continue forever?” | `progress` or LTL recurrence, with an explicit fairness decision | A non-progress lasso, or exhaustive absence of one under the stated assumptions | A scheduler can spin without visiting a progress label |
| “How large will the search be?” | `mc_estimate` before verification | Approximate growth data, never a verification verdict | Choose state, depth, time, and memory budgets |

LTL and CTL are deliberately not interchangeable. LTL describes every complete path
as a sequence; CTL can quantify over the branching set of continuations. For example,
`AG EF reset` means that every reachable state has *some* path to `reset`, whereas
`[]<> reset` requires every path to visit `reset` infinitely often. Liveness may need a
justified weak-fairness assumption; the report must name it rather than silently
discarding an unfair counterexample.

## Contents

- `plugin.json` — portable Agent Plugins 1.0 metadata.
- `.claude-plugin/plugin.json` — Claude Code compatibility metadata and reference to `./mcp/servers.json`.
- `engine/` — Go module `modelcheck`, source packages, tests, fixtures, CLI, MCP server, and release binaries.
- `features/` — Gherkin scenarios for the engine layers and integration tracks.
- `steps/` — confirmation records, logical reviews, MCP sessions, and evaluation evidence.
- `mcp.json` — portable stdio MCP server declaration using `${PLUGIN_ROOT}`.
- `mcp/servers.json` — Claude Code stdio MCP server declaration using `${CLAUDE_PLUGIN_ROOT}`.
- `skills/model-check/` — `SKILL.md`, workflows, property guidance, format references, assets, and report templates.
- `evals-workspace/` — evaluation fixtures, graders, benchmark runs, and review data when included in the release.
- `BUILD-PROTOCOL.md` — the six-part step protocol (scenarios first, tests, logic review, confirmation record); the build itself is documented in the build section below and in `build.sh`.
- `RELEASE-NOTES-0.3.0.md` — what changed in 0.3.0, the verdicts that differ from 0.2.0, the known limitations.
- `scripts/security-scan.sh`, `trivy.yaml`, `.trivyignore.yaml`, `.semgrepignore`, `docs/security-scanning.md`, `.github/workflows/security.yml` — the security scan (trivy, semgrep, govulncheck) and its guide.
- `AGENT-COMMON.md` — shared historical build-agent guidance.
- `build.sh` — reproducible multi-platform build and release verification script.
- `build-container/` — the build container (Go 1.26.8, SPIN, gcc, Python 3, the Go modules of the engine): `Dockerfile`, `build-image.sh`, the runner image, its README.

## Requirements

### Runtime

A ready-made release needs an agent/plugin host that supports the plugin manifest and MCP stdio servers. It does not need Go or SPIN at runtime. The selected platform needs the corresponding executable under `engine/bin/`:

- POSIX systems use the `engine/bin/mcd` wrapper, which selects `mcd-<goos>-<goarch>` using `uname`.
- Windows uses `engine/bin/mcd.exe` (the file the portable `engine/bin/mcd` command resolves to; a byte copy of `mcd-windows-amd64.exe`), the `engine/bin/mcd.cmd` wrapper where a host needs a batch file, and `mcd-windows-amd64.exe`.

### Build and test

- Go 1.26.8, declared in `engine/go.mod`; `build.sh` refuses any other Go, because the release binaries carry the standard library of the Go that built them.
- Python 3, required by `build.sh` and release metadata generation.
- A POSIX shell for `build.sh`.
- SPIN 6.5.2 or a compatible installation for SPIN-dependent tests and comparisons.
- Git for source-commit identification and release review.

SPIN is a test/comparison dependency, not a runtime dependency of an already-built `mcd` binary.

## Installation

### Obtain the public plugin

Use the public `model-check-plugin` project/repository or the complete `model-check-plugin/` directory from a reviewed release. Keep the directory layout intact; in particular, do not move `engine/bin`, `skills`, or `mcp` out of the plugin root.

### Build if binaries are not included

From the plugin root:

```bash
./build.sh --host-only --version 0.3.0
engine/bin/mcd version
```

For a release containing all configured platforms, use the full build described below. Do not silently replace a missing binary with an executable from another checkout.

### Register with the agent host

Point the plugin host at the plugin root using its documented installation mechanism. For local Claude Code development, use:

```bash
claude --plugin-dir /path/to/model-check-plugin
```

Confirm the following before starting a verification session:

1. `plugin.json` and `mcp.json` are present for portable Agent Plugins hosts.
2. `.claude-plugin/plugin.json` exists for Claude Code and names `./skills` and `./mcp/servers.json`.
3. `mcp.json` resolves `${PLUGIN_ROOT}/engine/bin/mcd`; Claude Code's `mcp/servers.json` resolves `${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd`.
4. The selected binary is executable and matches the host platform.
5. The host shows one `model-check` MCP server, not a duplicate project-level registration.

The server belongs in `mcp/servers.json`. Do not add a second root `.mcp.json` for this plugin: that can register the same server twice and leave `${CLAUDE_PLUGIN_ROOT}` unset in the duplicate registration.

### Codex CLI and Coddy

The plugin also ships agent-facing integration files for Codex and Coddy:

- `AGENTS.md` is the portable Codex project-instruction file.
- `.codex/README.md` documents Codex skill and MCP registration.
- `.coddy/mcp.json` declares the project-local Coddy MCP server using `${CWD}`.
- `.coddy/README.md` documents Coddy trust and skill installation.

Codex does not consume the Claude Code plugin manifest as a native plugin. From the
plugin root, expose the skill and register the bundled MCP server as follows:

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
ln -sfn "$PWD/skills/model-check" \
  "${CODEX_HOME:-$HOME/.codex}/skills/model-check"
codex mcp add model-check -- \
  "$PWD/engine/bin/mcd" serve \
  --max-states 5000000 --max-depth 5000000 --max-ms 300000 \
  --max-memory-mb 2048 --concurrency 2
codex mcp get model-check
```

Coddy can install the skill from the public source and use the project-local MCP
declaration when the plugin checkout is the workspace:

```bash
coddy plugin install https://github.com/viktor-drobek/model-check-plugin.git
cd /path/to/model-check-plugin
coddy mcp list --cwd "$PWD"
coddy mcp trust model-check --cwd "$PWD"
coddy --dry-run
```

Project-local Coddy MCP declarations are untrusted by default. Approve only the
exact command you intend to run. Keep one `model-check` registration; do not add a
second `.mcp.json` or duplicate global server entry. `coddy --dry-run` also probes
other global MCP servers, so an unrelated global timeout is not a failure of this
local declaration; inspect the local row with `coddy mcp list --cwd`.

### Smoke test

```bash
engine/bin/mcd version
engine/bin/mcd parse --promela "../Promela - examples/CH2/mutex.pml"
```

The second command assumes the plugin is inside the monorepo. In a standalone plugin checkout, use an absolute or otherwise valid path to a model file, or provide the model inline through MCP.

## Prepare and build

Run the checks from a clean, reviewed checkout. Do not include credentials, private absolute paths, Python bytecode, temporary SPIN output, or accidental host data in a release.

### Build container

Instead of installing the toolchain on the host, build and test in the container `<registry>/model-check/build:0.3.0` (Go 1.26.8, SPIN 6.5.2, gcc, make, git, Python 3, govulncheck, and the Go modules of the engine already downloaded; stored in the docker registry your team uses). Its definition, use and how it is built and stored are in [`build-container/README.md`](build-container/README.md).

### Security scanning

`scripts/security-scan.sh` runs trivy (dependencies, secrets, misconfiguration), semgrep and govulncheck over the checkout, with pinned docker images when the tools are not installed; `.github/workflows/security.yml` runs it on pull requests, on pushes to `main` and weekly, and uploads the SARIF to GitHub code scanning. Settings, exit codes and triage: [`docs/security-scanning.md`](docs/security-scanning.md). It needs docker (or the three tools) and, for the container scans, network access to the Go module proxy.

### Test and lint

```bash
cd engine
go test ./...
go vet ./...
test -z "$(gofmt -l .)"
spin --version
cd ..
```

The suite includes Go unit tests and the Godog harness. CI additionally runs the race detector and verifies SPIN on the configured self-hosted runner. The CI `Security scan` workflow (trivy, semgrep, govulncheck) is a monorepo-root pipeline and is not shipped inside this plugin tree; its guide is `docs/security-scanning.md` in the monorepo. CI test steps use a 60-minute `go test` timeout because the `explore` package takes about 27 minutes under `-race`.

Performance work is measured, not asserted. The whole-engine benchmarks run the Promela fixtures `engine/testdata/promela/bench-*.pml` through `cli.Run` and check their state counts, so a faster engine that explores something else fails:

```bash
cd engine
go test -run XXX -bench . -benchmem ./explore .
```

Compare two revisions by running both test binaries alternately on one machine; a single run varies by about 10%. The parallel search has its own benchmarks, one per shape of state graph and per worker count (`go test -run XXX -bench Parallel -benchtime 3x .` from `engine/`; `MCD_BENCH_BIG=1` adds the 8-million-state model): a speedup is only meaningful on a quiet machine, so read `uptime` first.

### Partial-order reduction (`--por`)

`mcd check --por` explores, in a state, the moves of one process alone when the analysis can prove that nothing the other processes do could matter first. It applies to the safety search only (`deadlock`, `assert`, `invariant`, `reach`), depth-first, and it is opt-in. The verdicts are the same as without it; the number of states and transitions in the report are those of the *reduced* graph (so they no longer match `pan -c0`), and the first counterexample can differ. The report's `search.reduction` says whether the reduction was applied and, when it was not, why. Refused in this version, each with its reason in the report: rendezvous channels (a handshake is one step of two processes; no model measured had two independent handshakes, so a rule for it would cost a proof and gain nothing), channels named by a value, `timeout` and `provided` (postponed and not needed by anything measured: `steps/perf6-plan.md` §5 and §6), `--bfs`, any `ltl`, `progress` or `ctl` property, and two shapes of process creation that the Promela frontend never emits (a dynamic process that returns to its dormant location without leaving the process table, a `run` that enters its target at the dormant location).

What is reduced. A producer and its consumer do not make a conflict: the send end and the receive end of a buffered channel are separate, and a process at a channel operation is expanded alone only while the channel can act (room for a send, a message for a receive), so a pipeline of stages is explored stage after stage. **Atomic sequences** are reduced: what is commuted is the whole sequence up to the next state the search stores (a *macro-step*), its footprint is every edge it can be made of, and the cycle proviso follows each sequence to the states it ends in; a process that blocks inside its sequence loses exclusive control and is stored with the exclusive byte set, and two such states that differ only in that byte are the same state for the purposes of the comparison with the full search. **`run` and the process table** are reduced: the live-process table (`_nr_pr`, `_pid` of a process created at run time, which process is the youngest and may end) is one cell that every `run` and every end of a process writes, so in a model that creates processes the creating step is itself expanded in full and the gain comes from the other processes (the ring of `testdata/promela/leader3.pml`, started by `init { atomic { run ... } }`, shrinks from 679 states to 76, and the same ring with five nodes from 41 692 to 108). Everything that needs the table or a `run` is covered by the same rule: a `run` whose arguments read a shared variable, a guard on `_nr_pr`, a property that mentions `_nr_pr`. A model that reads `_nr_pr` is therefore reduced like any other where these rules allow it, not refused: `testdata/promela/nrpr.pml` (two workers started by a `run` in a loop, each asserting `_nr_pr > 1`) stores 31 states in full and 21 with `--por`; what stays unreduced is the step that creates a process and the end of every process, which write the table the guard reads. Where the search cannot finish (a budget, an error of the model such as an exhausted process pool) the two searches stop in different places, and their counts are not comparable.

The MCP tool `mc_check` takes the same request as `por: true` and answers with the same `search.reduction`; a request it cannot honour is a result with `applied: false`, not a tool error.

The reduction is checked against the unreduced search by four oracles that share random generators of atomic sequences, loops through atomic chains, process creation with the table, the table model the frontend gives a model that reads `_nr_pr` and has no `run` (every process leaves the table when it ends), `provided`, and reads of globals that another process writes (send arguments, receive matches and indexes, array indexes of effects, asserts: `explore/por_gen_test.go`): the verdict differential (`por_oracle_test.go`, `por_random_test.go`: the status and evidence of every property, the set of states without an enabled move compared up to the exclusive byte, the reduced states a subset of the full ones, every counterexample replayed as a run of the model, whether a model error is reachable, and the counts equal when the reduction is refused), a semantic audit of every eligible process at every stored state of the full graph that does not read the footprints (`por_audit_test.go`), an acyclicity audit of the cycle proviso (`por_acyclic_test.go`), and the models of the `Promela - examples` corpus and the fixtures (`por_corpus_test.go` at the root of the `engine` module: the status and evidence of each property, the reachability of a model error, a state count that is never larger and equal when the reduction is refused, and a floor under the number of models it applies to). `tools/pandiff` runs fuzzers of Promela models in the shapes the frontend emits (`atomic`, `d_step`, `run` from `init`, `_nr_pr` guards with `active` processes, loops inside atomic and d_step blocks, a buffered channel) through the engine in full and reduced and against `pan -DNOREDUCE`, and sets the corpus verdicts against `pan`. A mutation harness (`go run ./cmd/pormut -scratch DIR`, mutants in `tools/pormut/mutants.json`) applies each mutant of the analysis to a scratch copy and runs the oracles on it, after a baseline run that must be green. `go test ./explore -run TestPOR` runs the analysis tests and the oracles at their default sizes (about a minute and a half; the sizes are set by `MCD_POR_MODELS`, `MCD_POR_SEED`, `MCD_POR_GEN`, `MCD_POR_WORKERS`, `MCD_POR_ALL`, `MCD_POR_AUDIT_K`; the release bar is 300 000 models per generator). The oracles see a hole in the footprints only in the shapes the generators make: hand-built shapes that no generator makes (a `run` initialiser that assigns a global, a heterogeneous pool, an atomic edge into a sink, the length of a channel named by a value, a failing assert inside a chain) are pinned by directed tests only, and the harness at its default size is not sensitive to rare shapes (see "What the oracles can and cannot see" in `steps/perf6-confirmation.md`). What was measured and what was not is in `steps/perf6-confirmation.md`.

### Parallel search (`--workers`)

`mcd check --workers N` searches the safety properties (`deadlock`, `assert`, `invariant`, `reach`) with N workers: a level-synchronous breadth-first search over a visited set split into 256 partitions by a fixed hash, in which each partition is written by exactly one worker at a time. It is opt-in; without the flag nothing changes, byte for byte (the report has no `parallel` object and `search.mode` is what it was). The MCP tool `mc_check` takes the same request as `workers` (clamped to the server's `mcd serve --max-workers`, which is `GOMAXPROCS` unless set; the clamp is a note in `budget_notes`).

What a parallel run promises, and what it changes. For a run that completes, every property has the verdict, evidence and reason of the sequential search, and `states` and `transitions` are the sequential ones exactly (checked against the sequential searches, with and without `--sweep`, on random models from three generators and on the 106 fixture and corpus models that finish within the test budget (137 accepted by the frontend, 25 of them refused for a temporal property) at 1 and 8 workers, and against SPIN's `pan -c0` on 22 models at 1 and 4 workers where the SPIN tests run; the first 200 000 random models, all from the author's own seed ranges and with the sweep on, missed a wrong exhaustive verdict that the review of the diff found with a range of its own and that is fixed, see `steps/perf5-confirmation.md`). `depth` is the number of breadth-first layers, and the report says `search.mode: "bfs"`; a layer counts hops between stored states, so an atomic sequence that runs through is **one unit of `depth` and of `--budget-depth`**, and one that blocks part-way counts one unit per uninterrupted run (the state where its holder blocks is stored, and the continuation starts with the step of whichever process unblocks it), where `--bfs` and the default search count every step of it (a `d_step` block is one move in every search): the depth equals `--bfs`'s on a model without atomic sequences and can differ on one with them (the verdicts, states and transitions of a complete run still agree, but a `--budget-depth` run can stop at a different layer than `--bfs`, or complete where `--bfs` is cut). A counterexample is a shortest one in layers and the same for every worker count, but generally not the depth-first one. A property expression that fails to evaluate (an index out of range) can end one search `invalid-model` where another completes, because the parallel frontier is in partition order, not FIFO order (the depth-first search differs from `--bfs` the same way); complete runs never disagree. The report is the same for every N, byte for byte, apart from the three worker-count fields of `search.parallel`, and so are the reports of runs cut by a budget: `--budget-states` is exact (the run stores that many states and no more, and claims no verdict that lies after its stop point), `--budget-depth` cuts at a layer (counted as above), `--budget-mem-mb` is checked on an estimate that does not depend on N; only `--budget-ms` is not reproducible. The memory estimate is an estimate, not the resident memory: it counts the stored set, the records of the largest group of states and the largest frontier, and a parallel run can overshoot the budget by up to about one group of records (measured once: an estimate of 949 MB for a budget of 800 MB, a resident size of about 1.1 to 1.2 GB against 864 MB for the sequential run, on a model with 3 KB states). `search.parallel` carries the number of breadth-first layers and the width of the widest, which says whether the graph was wide enough to pay.

When it pays and when it does not. It pays on **wide** graphs, with thousands of states in an average layer: independent or loosely coupled processes, symmetric workers, buffered pipelines with data in the messages. It does not pay on narrow or deep ones (a single counter, a chain of states: one state per layer, and a run costs the same as the sequential one at best), nor on models of under about 10^5 states, and it is a tool for **proofs**, not for finding bugs fast: a breadth-first search completes every layer above a violation before it finds it, so a user hunting a bug keeps the default depth-first search or `--por`. See `steps/perf5-confirmation.md` for the measured speedups, with the load of the machine they were taken under, and for what was not measured.

What it refuses, with the reason in `search.parallel.reason` and the run executed sequentially and unchanged (so its `search.mode` is the requested one, and its `depth` and `--budget-depth` count transitions, not layers of stored states): any `ltl`, `progress` or `ctl` property in the run (a mixed run is not split), and `--por` where the reduction applies (it is depth-first; where it refuses, the parallel search runs and `search.reduction` keeps its own reason). `--workers` with `--estimate`, below 0 or above 256 is a usage error. The memory estimate counts the stored set, the records of the largest group of states and the largest frontier; the compiled copies of the model that each worker keeps are in `search.parallel.worker_bytes_est`.

### Reproducible release build

```bash
./build.sh --version 0.3.0 --source-commit <SOURCE-COMMIT> --verify-repro
```

`<SOURCE-COMMIT>` is the commit the engine sources are built from. For a release it is the commit just before the one that commits the built files under `engine/bin/` (the artifacts commit), because a commit cannot name itself; `BUILD-INFO.json` records it as `source_commit`. Without `--source-commit` the script records `git rev-parse HEAD` of the plugin checkout (or `unknown` outside a Git checkout), which after the artifacts commit is the artifacts commit, so the committed `BUILD-INFO.json` is not reproduced from that checkout.

The script:

- builds static `mcd` binaries for Linux amd64/arm64, Darwin amd64/arm64, and Windows amd64 by default;
- writes the POSIX `mcd` wrapper, the Windows `mcd.cmd` wrapper, and `mcd.exe`, a copy of the Windows binary;
- writes sorted `engine/bin/SHA256SUMS`;
- writes `engine/bin/BUILD-INFO.json` with version, Go version, flags, source commit, platforms, and binary sizes;
- checks the host binary, wrapper, checksums, and version stamp;
- performs a second build when `--verify-repro` is supplied and compares every generated file (binaries, wrappers, `SHA256SUMS`, `BUILD-INFO.json`) byte for byte.

For a faster local build:

```bash
./build.sh --host-only --version 0.3.0
```

Useful options are `--source-commit HASH`, `--platforms "goos/goarch ..."`, `--out DIR`, `--no-verify`, and `--verify-repro`; the options and exit codes are listed at the top of `build.sh`. Use `--no-verify` only for an intentional intermediate build. Record which platforms were built and which were actually smoke-tested; cross-building is not the same as executing a platform binary.

After a release build:

```bash
engine/bin/mcd version
( cd engine/bin && sha256sum -c SHA256SUMS )
cat engine/bin/BUILD-INFO.json
```

The build and its options are described in this section and at the top of `build.sh`. [`BUILD-PROTOCOL.md`](BUILD-PROTOCOL.md) is the six-part protocol every development step follows (scenarios first, tests, logic review, confirmation record), not a build manual; the record of a release is a `steps/*-confirmation.md` file (for 0.2.0, the addendum at the top of `steps/g6-confirmation.md`; for 0.3.0, `steps/release-0.3.0-confirmation.md`).

## Use the CLI

The CLI is `engine/bin/mcd`. It can also be built or run with `go run ./cmd/mcd` from `engine/`.

```bash
# Parse Promela into canonical IR JSON.
engine/bin/mcd parse --promela /path/to/model.pml

# Check a Promela model with a bounded breadth-first search.
engine/bin/mcd check \
  --promela /path/to/model.pml \
  --bfs \
  --budget-states 100000 \
  --no-timing

# Search the safety properties with several workers (a wide state graph that
# ends `inconclusive` on a budget or takes long; see "Parallel search" below).
engine/bin/mcd check --promela /path/to/model.pml --workers 8

# Parse Petri-net JSON or IR JSON.
engine/bin/mcd parse --petri /path/to/net.json
engine/bin/mcd parse --ir /path/to/model.json

# Check LTL or CTL formulas from the command line.
engine/bin/mcd check --promela /path/to/model.pml --ltl '[] (request -> <> response)'
engine/bin/mcd check --promela /path/to/model.pml --ctl 'AG (count <= 1)'

# Start the MCP server manually when the host is not launching it.
engine/bin/mcd serve --session-dir ${TMP_DIR}/mcd-sessions
```

The accepted Promela subset is documented in [`skills/model-check/references/promela-subset.md`](skills/model-check/references/promela-subset.md). Read it before interpreting an `outside-subset` rejection. The engine is finite and explicit-state: it does not provide timed, probabilistic, symbolic, or unbounded verification.

### Result vocabulary

Always report the result and its evidence separately:

| Status | Meaning |
|---|---|
| `verified` | The property was established under the stated model and search assumptions. For ordinary verification this requires complete exhaustive evidence. |
| `violated` | A counterexample or other exact refutation was found. |
| `inconclusive` | A budget stopped the search before the property was decided. |
| `unknown` | The result cannot be classified more strongly under the engine's evidence contract. |
| `not-executed` | The property or input was not executed, for example because the construct is outside the supported subset. |
| `invalid-model` | Execution found a domain/model defect such as an overflow or invalid state. |

Evidence is one of `exhaustive`, `bounded`, `approximate`, or `unknown`. A time or memory stop is not an exhaustive result. Read the search stop reason, budgets, warnings, assumptions, counterexample/witness, and generated artifact paths before making a claim.

## Use the MCP server

The manifest starts this stdio command:

```text
${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd serve --max-states 5000000 --max-depth 5000000 --max-ms 300000 --max-memory-mb 2048 --concurrency 2
```

The seven MCP tools are:

- `mc_parse` — parse Promela, Petri JSON, IR JSON, or an allowed file into IR;
- `mc_simulate` — run a seeded random or guided sanity walk;
- `mc_check` — check invariant, deadlock, reachability, LTL, CTL, and progress properties;
- `mc_explain` — decode a stored counterexample or witness;
- `mc_lint_property` — inspect property atoms, types, classes, and temporal normalisation;
- `mc_estimate` — estimate state-space growth and select a budget;
- `mc_manifest` — emit the session manifest with versions, hashes, calls, and artifacts.

The recommended workflow is:

1. Parse the model with `mc_parse`.
2. Run a short fixed-seed `mc_simulate` sanity walk.
3. Use `mc_estimate` to understand growth and budget needs.
4. Check reachability, safety, and deadlock properties.
5. Check liveness and fairness assumptions separately.
6. Use `mc_explain` for every reported counterexample.
7. Read `mc_manifest` and retain the session artifacts with the report.

The agent skill in [`skills/model-check/SKILL.md`](skills/model-check/SKILL.md) defines the complete intake, staged execution, interpretation, and reporting procedure. It is the source of operational guidance; this README is an installation and orientation summary.

## Running on a shared machine

A run uses CPU and memory in proportion to the state space, and the machine is often shared with other sessions, builds, and test campaigns. Look at the machine before you launch a run, and wait while it is busy. The script `skills/model-check/assets/wait-for-capacity.sh` does the looking:

```bash
sh skills/model-check/assets/wait-for-capacity.sh --need-cores 4 --need-mem-mb 2048 \
   --dir "$SESSION_DIR" --min-disk-mb 500 --timeout 900
```

It returns when CPU use and memory use, including what the run would add (`--need-cores`, `--need-mem-mb`), are within 90% (`--max-cpu`, `--max-mem`) and the session directory has the free space asked for. Exit codes: `0` there is room, `3` the machine was still busy when the timeout ended, `2` usage error, `4` this platform cannot be measured (Linux and macOS are; on Windows read the load, free memory and free disk by hand). On exit `3` do not start the run: wait and look again, lower `--workers` and the budgets, or tell the user why it was not started. Do not take timing or speed-up measurements on a busy machine, and do not run several heavy runs at once on a machine that cannot hold them: queue them. The agent skill carries the same instruction (`SKILL.md` step 5).

## Files, sessions, and safety

MCP sessions write IR, check, simulation, counterexample, and manifest artifacts under the configured session directory. The server applies path guards and an optional read allow-list. Do not pass arbitrary host paths, interpolate model text into shell commands, or copy models and traces containing sensitive data into public examples.

For CLI use, create a separate session directory and preserve the input, command line, report, and relevant traces. Never overwrite the user's source model. Treat reports and traces as potentially sensitive.

## Troubleshooting

### `mcd` is missing or not executable

Build the host binary with `./build.sh --host-only --version <VERSION>`, check the executable bit on `engine/bin/mcd`, and run `engine/bin/mcd version`. Confirm that the wrapper's selected `mcd-<goos>-<goarch>` file exists.

### The MCP server is listed twice or is pending approval

Remove the extra project-level MCP registration. Keep the plugin declaration in `mcp/servers.json`, ensure the plugin host sets `${CLAUDE_PLUGIN_ROOT}`, and restart the host so it reloads the manifest.

### The model is rejected

Read the `kind`, source location, and reason in the rejection. Check the accepted Promela subset and Petri-net schema. A rejected input is not a verification verdict; rewrite the model or property and run `mc_parse` again.

### `mcd` reports an internal error

The CLI ends with exit code 1, nothing on stdout, and `mcd: internal error: ...` (a failure of the engine that it caught, followed by the Go stack) or `mcd check: internal: ...` (a consistency check of the search failed, or a worker of the parallel search `--workers` panicked: `internal error in the parallel search`) on stderr. An MCP tool answers with `isError` and a message that starts with `internal error in <tool>:` (or, from `mc_check`, `internal:`; one line either way), the stack of a caught panic goes to the server's standard error and not into the answer or the manifest, the server stays up and serves the next call, and `mc_manifest` records the call with outcome `error`. This is a defect of `mcd`, not a verdict on the model: no verdict exists for that call, and rerunning with other budgets does not produce one. Keep the model, the exact command line or tool call, the output of `mcd version`, and the stderr text, and report them. Do not confuse it with exit code 2, which means the input was rejected by a frontend and is fixed in the model, and with exit code 1 for a bad flag or an unreadable file, whose message names the flag or the file. The stack of a release binary carries no source paths (it is built with `-trimpath`); check the stack of a developer build for private paths before posting it.

### The result is inconclusive

Read the exhausted resource. Use `mc_estimate`, reduce the model only with that change documented, or increase a states/depth/time/memory budget deliberately. Do not relabel `inconclusive` as `verified`.

### A SPIN-dependent check fails

Confirm `spin --version`, verify that SPIN is on `PATH`, and inspect the relevant `features/` and `steps/` confirmation record. SPIN is required for comparison/test paths, not for ordinary use of a ready-made binary.

### A build checksum or reproducibility check fails

Start from a clean checkout, use the declared Go toolchain and `GOTOOLCHAIN=local`, remove only generated `engine/bin` outputs, rebuild with the same version and platform arguments, and inspect `BUILD-INFO.json`. Do not publish a release whose checksum or reproducibility check fails.

## Public release checklist

Before publishing this plugin:

- preserve the complete versioned `model-check-plugin/` tree;
- validate `plugin.json`, `mcp.json`, `.claude-plugin/plugin.json`, and `mcp/servers.json`;
- run `go test ./...`, `go vet ./...`, formatting, and SPIN checks;
- run `./build.sh --version <VERSION> --source-commit <SOURCE-COMMIT> --verify-repro`;
- verify `engine/bin/SHA256SUMS`, `BUILD-INFO.json`, the host wrapper, and platform records;
- check all examples and logs for private paths, secrets, tokens, and host data;
- update installation, preparation, build, usage, tooling, limitations, troubleshooting, and provenance instructions in the same change;
- include the source commit and version in the release record.

## Literature and provenance

The public bibliography and the complete source map live in
[`PROVENANCE.md`](PROVENANCE.md). It distinguishes external publications from
reading notes that exist only in the full monorepo, so a standalone plugin checkout
does not contain broken local links.

Key public references are:

- Clarke, Henzinger, Veith, and Bloem (eds.), [*Handbook of Model
  Checking*](https://link.springer.com/book/10.1007/978-3-319-10575-8), for the
  modern model-checking taxonomy and algorithms.
- Clarke, Grumberg, and Peled, [*Model Checking*](https://link.springer.com/book/10.1007/978-3-662-22679-3), for the classic transition-system and temporal
  model-checking treatment.
- Baier and Katoen, [*Principles of Model Checking*](https://mitpress.mit.edu/9780262026499/principles-of-model-checking/), for LTL/CTL terminology,
  safety/liveness, fairness, and state-space reasoning.
- Holzmann, [*Design and Validation of Computer Protocols*](http://spinroot.com/gerard/popd.html), and [the SPIN project](https://spinroot.com/), for Promela and
  protocol verification.
- Karpov, [*Model Checking*](https://books.google.com/books/about/MODEL_%D0%A1HECKING_%D0%92%D0%B5%D1%80%D0%B8%D1%84%D0%B8%D0%BA%D0%B0%D1%86%D0%B8%D1%8F.html?id=xpui56eRsHgC), for Russian-language terminology and examples.

These sources provide background and provenance, not an implementation guarantee.
The current engine behavior is defined by the Go code, feature scenarios, and the
executable references under `skills/model-check/references/`. Unsupported timed,
probabilistic, symbolic, unbounded, or game semantics must remain explicitly
outside this plugin's scope.
