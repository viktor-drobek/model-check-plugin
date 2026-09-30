# Coddy integration

This project-local MCP declaration starts the bundled `model-check` server when
Coddy is launched with this directory as its workspace. It uses `${CWD}` so the
checkout can live anywhere; no private absolute path is stored.

## First run

From the plugin root:

```bash
coddy mcp list --cwd "$PWD"
coddy mcp trust model-check --cwd "$PWD"
coddy --dry-run
```

Coddy treats `.coddy/mcp.json` as untrusted project content by default. `trust`
approves the exact command, arguments, and environment declaration for this
workspace. If the declaration changes, Coddy asks for approval again. The global
`coddy --dry-run` also probes unrelated configured servers; failures there do not
mean this local declaration is broken, so inspect the `model-check` row separately.

Install the skill from the public repository with:

```bash
coddy plugin install https://github.com/viktor-drobek/model-check-plugin.git
```

The MCP declaration and the skill installation are separate: the former connects
`mcd serve`, while the latter makes `skills/model-check/SKILL.md` available to the
agent. Keep one `model-check` server registration and do not add a duplicate
`.mcp.json` or global entry for the same checkout.
