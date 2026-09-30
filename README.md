# model-check plugin

`model-check` is an agent plugin for finite explicit-state model checking. It provides an agent skill, a deterministic Go engine, a CLI, and a seven-tool MCP server for Promela models, Petri-net JSON, IR JSON, safety properties, reachability, LTL, CTL, simulation, estimation, counterexamples, and manifests.

This directory is the complete public plugin artifact. Do not distribute only `skills/model-check/`, only `engine/bin/mcd`, or only `mcp/servers.json`; the manifest, engine, features, steps, evaluations, references, build protocol, and release metadata are part of the plugin.

## Contents

- `.claude-plugin/plugin.json` — plugin metadata and references to `./skills` and `./mcp/servers.json`.
- `engine/` — Go module `modelcheck`, source packages, tests, fixtures, CLI, MCP server, and release binaries.
- `features/` — Gherkin scenarios for the engine layers and integration tracks.
- `steps/` — confirmation records, logical reviews, MCP sessions, and evaluation evidence.
- `mcp/servers.json` — the stdio MCP server declaration.
- `skills/model-check/` — `SKILL.md`, workflows, property guidance, format references, assets, and report templates.
- `evals-workspace/` — evaluation fixtures, graders, benchmark runs, and review data when included in the release.
- `BUILD-PROTOCOL.md` — the six-part BDD/build protocol.
- `AGENT-COMMON.md` — shared historical build-agent guidance.
- `build.sh` — reproducible multi-platform build and release verification script.

## Requirements

### Runtime

A ready-made release needs an agent/plugin host that supports the plugin manifest and MCP stdio servers. It does not need Go or SPIN at runtime. The selected platform needs the corresponding executable under `engine/bin/`:

- POSIX systems use the `engine/bin/mcd` wrapper, which selects `mcd-<goos>-<goarch>` using `uname`.
- Windows uses `engine/bin/mcd.cmd` and `mcd-windows-amd64.exe`.

### Build and test

- Go 1.26, declared in `engine/go.mod`.
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
./build.sh --host-only --version 0.1.0
engine/bin/mcd version
```

For a release containing all configured platforms, use the full build described below. Do not silently replace a missing binary with an executable from another checkout.

### Register with the agent host

Point the plugin host at the plugin root using its documented installation mechanism. For local Claude Code development, use:

```bash
claude --plugin-dir /path/to/model-check-plugin
```

Confirm the following before starting a verification session:

1. `.claude-plugin/plugin.json` exists and names `./skills` and `./mcp/servers.json`.
2. `mcp/servers.json` resolves `${CLAUDE_PLUGIN_ROOT}/engine/bin/mcd`.
3. The selected binary is executable and matches the host platform.
4. The host shows one `model-check` MCP server, not a duplicate project-level registration.

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

### Test and lint

```bash
cd engine
go test ./...
go vet ./...
test -z "$(gofmt -l .)"
spin --version
cd ..
```

The suite includes Go unit tests and the Godog harness. CI additionally runs the race detector and verifies SPIN on the configured self-hosted runner.

### Reproducible release build

```bash
./build.sh --version 0.1.0 --verify-repro
```

The script:

- builds static `mcd` binaries for Linux amd64/arm64, Darwin amd64/arm64, and Windows amd64 by default;
- writes the POSIX `mcd` wrapper and Windows `mcd.cmd` wrapper;
- writes sorted `engine/bin/SHA256SUMS`;
- writes `engine/bin/BUILD-INFO.json` with version, Go version, flags, source commit, platforms, and binary sizes;
- checks the host binary, wrapper, checksums, and version stamp;
- performs a second build when `--verify-repro` is supplied and compares binary hashes.

For a faster local build:

```bash
./build.sh --host-only --version 0.1.0
```

Useful options are `--platforms "goos/goarch ..."`, `--out DIR`, `--no-verify`, and `--verify-repro`. Use `--no-verify` only for an intentional intermediate build. Record which platforms were built and which were actually smoke-tested; cross-building is not the same as executing a platform binary.

After a release build:

```bash
engine/bin/mcd version
( cd engine/bin && sha256sum -c SHA256SUMS )
cat engine/bin/BUILD-INFO.json
```

The complete build/release protocol is in [`BUILD-PROTOCOL.md`](BUILD-PROTOCOL.md).

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

# Parse Petri-net JSON or IR JSON.
engine/bin/mcd parse --petri /path/to/net.json
engine/bin/mcd parse --ir /path/to/model.json

# Check LTL or CTL formulas from the command line.
engine/bin/mcd check --promela /path/to/model.pml --ltl '[] (request -> <> response)'
engine/bin/mcd check --promela /path/to/model.pml --ctl 'AG (count <= 1)'

# Start the MCP server manually when the host is not launching it.
engine/bin/mcd serve --session-dir /tmp/mcd-sessions
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

### The result is inconclusive

Read the exhausted resource. Use `mc_estimate`, reduce the model only with that change documented, or increase a states/depth/time/memory budget deliberately. Do not relabel `inconclusive` as `verified`.

### A SPIN-dependent check fails

Confirm `spin --version`, verify that SPIN is on `PATH`, and inspect the relevant `features/` and `steps/` confirmation record. SPIN is required for comparison/test paths, not for ordinary use of a ready-made binary.

### A build checksum or reproducibility check fails

Start from a clean checkout, use the declared Go toolchain and `GOTOOLCHAIN=local`, remove only generated `engine/bin` outputs, rebuild with the same version and platform arguments, and inspect `BUILD-INFO.json`. Do not publish a release whose checksum or reproducibility check fails.

## Public release checklist

Before publishing this plugin:

- preserve the complete versioned `model-check-plugin/` tree;
- validate `.claude-plugin/plugin.json` and `mcp/servers.json`;
- run `go test ./...`, `go vet ./...`, formatting, and SPIN checks;
- run `./build.sh --version <VERSION> --verify-repro`;
- verify `engine/bin/SHA256SUMS`, `BUILD-INFO.json`, the host wrapper, and platform records;
- check all examples and logs for private paths, secrets, tokens, and host data;
- update installation, preparation, build, usage, tooling, limitations, troubleshooting, and provenance instructions in the same change;
- include the source commit and version in the release record.

## Literature and provenance

The plugin's engine and skill are grounded in the following source corpus and synthesis notes:

- Clarke, Grumberg, and Peled, *Handbook of Model Checking*.
- Gerard Holzmann, *Design and Validation of Computer Protocols*.
- U. Karpov, *Model checking*.
- Baier and Katoen, *Principles of Model Checking*.
- *Verification Protocols Web*.
- Automata-program verification notes.
- SPIN graph-encoded tuple-set material.
- Lecture notes 01–09.
- `modelchk` material.
- Cross-book synthesis, requirements, coverage, and the skill-building plan.

In the monorepo, source texts and notes are under `books-md/` and `model-check-skill-notes/`. The project README contains the path-level provenance index. New sources must be added to the public documentation and coverage records; bibliography is not a substitute for current tests and implementation behavior.
