# Codex integration

Codex consumes the plugin checkout through the root `AGENTS.md` and the standard
skill file at `skills/model-check/SKILL.md`. The Claude Code manifest remains useful
for Claude Code, but it is not a Codex-native plugin manifest.

## Skill

From the plugin root:

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
ln -sfn "$PWD/skills/model-check" \
  "${CODEX_HOME:-$HOME/.codex}/skills/model-check"
```

## MCP

Register the bundled server with an absolute path:

```bash
codex mcp add model-check -- \
  "$PWD/engine/bin/mcd" serve \
  --max-states 5000000 \
  --max-depth 5000000 \
  --max-ms 300000 \
  --max-memory-mb 2048 \
  --concurrency 2
```

Check the resulting user configuration with `codex mcp get model-check` and
`codex mcp list`. Do not commit that user configuration: it contains a
machine-specific absolute checkout path.

The monorepo's `.codex/hooks.json` is intentionally not duplicated here. Its hook
reads the monorepo `.cursor/rules/` source tree; this standalone artifact has no
such dependency. `AGENTS.md` is the portable Codex rule surface for this plugin.
