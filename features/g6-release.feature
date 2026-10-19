@release_ci
Feature: G6 release binary publication
  These scenarios inspect the workflow contract, not a live GitHub Release upload.
  Cross-compilation does not imply execution on every target operating system.

  Scenario: release publication uses the reproducible build and a separate upload job
    Given the plugin file ".github/workflows/release.yml" contains each of:
      | types: [published]                                                  |
      | workflow_dispatch:                                                  |
      | contents: read                                                      |
      | contents: write                                                     |
      | persist-credentials: false                                           |
      | go-version-file: engine/go.mod                                       |
      | cache: false                                                        |
      | GOTOOLCHAIN: local                                                   |
      | Preflight published mutable release                                  |
      | test -s "$asset"                                                    |
      | --verify-repro                                                      |
      | --source-commit "$SOURCE_COMMIT"                                     |
      | linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64         |
      | sha256sum --check SHA256SUMS                                          |
      | gh release upload                                                   |
      | mcd-windows-amd64.exe                                                |
      | BUILD-INFO.json                                                      |
    And the plugin file "scripts/validate-release.py" contains each of:
      | refs/tags/                                                          |
      | .claude-plugin/plugin.json                                          |
      | release tag does not match plugin version                            |
      | git                                                                 |
      | rev-parse                                                           |
