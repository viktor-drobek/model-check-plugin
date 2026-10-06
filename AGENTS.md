# `model-check` plugin instructions

This directory is the complete public `model-check` plugin checkout. Preserve the
plugin root layout: `.claude-plugin/`, `engine/`, `mcp/`, `skills/`, `features/`,
`steps/`, `evals-workspace/`, and the build metadata are one release artifact.

## First use

1. Read `skills/model-check/SKILL.md` and the referenced workflow and engine-tool
   documents before interpreting a verification result.
2. Use `engine/bin/mcd` from this checkout, or the `model-check` MCP server. Do not
   look for SPIN, NuSMV, or another checker as a runtime replacement.
3. Run `engine/bin/mcd version` before a smoke test. If the host binary is absent,
   build it with `./build.sh --host-only --version 0.2.0` rather than substituting a
   binary from another checkout.
4. Keep `verified`, `violated`, `inconclusive`, `unknown`, `not-executed`, and
   `invalid-model` distinct, and report the evidence level and assumptions.
5. Treat every result as a statement about the finite model under its stated
   assumptions, not as a proof about an unmodelled implementation.

## Codex CLI setup

Codex does not treat the Claude Code `.claude-plugin/plugin.json` as a native Codex
plugin manifest. This `AGENTS.md` is the project instruction file Codex reads from
the checkout; the skill itself is available at `skills/model-check/SKILL.md`.

To expose the skill to a Codex installation, create a user-level skill link (run
from the plugin root):

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
ln -sfn "$PWD/skills/model-check" \
  "${CODEX_HOME:-$HOME/.codex}/skills/model-check"
```

To register the bundled MCP server in Codex, use the absolute checkout path because
Codex stores the command in the user configuration:

```bash
codex mcp add model-check -- \
  "$PWD/engine/bin/mcd" serve \
  --max-states 5000000 \
  --max-depth 5000000 \
  --max-ms 300000 \
  --max-memory-mb 2048 \
  --concurrency 2
```

The same command can be run without a global installation as
`npx --yes @openai/codex mcp add ...`. Inspect the result with:

```bash
codex mcp get model-check
codex mcp list
```

The Codex hook bridge from the monorepo is not copied into this standalone plugin:
it depends on the monorepo's `.cursor/rules/` tree. If you work in the full
`model-check` repository, use its existing `.codex/hooks.json` bridge and approve
it in Codex when requested.

## Coddy setup

The repository includes `.coddy/mcp.json`. Start Coddy with this checkout as the
workspace, inspect the declaration, and approve it before the first MCP session:

```bash
coddy mcp list --cwd "$PWD"
coddy mcp trust model-check --cwd "$PWD"
coddy --dry-run
```

The project declaration uses `${CWD}/engine/bin/mcd`, so it is portable and contains
no user-specific absolute path. Coddy project MCP files are untrusted by default;
the trust command is an explicit operator decision for this checkout. A global
`coddy --dry-run` may also report failures from unrelated configured servers; use
`coddy mcp list --cwd "$PWD"` to inspect this local declaration directly.

For a persistent user-level skill installation from the public repository, use:

```bash
coddy plugin install https://github.com/viktor-drobek/model-check-plugin.git
```

For a local checkout, Coddy can also be pointed at the skill directory using the
same `skills/model-check` path described above. Do not add a second project MCP
registration: keep one `model-check` server to avoid duplicate sessions.

## Testing boundary

Use `go test ./...`, `go vet ./...`, formatting checks, and SPIN-dependent checks
only when validating the source checkout. A shipped binary needs neither Go nor SPIN
for ordinary runtime use. Use `./build.sh --version 0.2.0 --source-commit
<SOURCE-COMMIT> --verify-repro` for a release build (`<SOURCE-COMMIT>` is the
commit just before the one that commits the built files under `engine/bin/`) and verify
`engine/bin/SHA256SUMS` and `BUILD-INFO.json` before publishing.
