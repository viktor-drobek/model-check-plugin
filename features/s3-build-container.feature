# S3 — the build container of the plugin.
#
# The plugin ships a container definition in which the engine builds and tests
# without anything installed on the host: the Go toolchain the release requires,
# SPIN and gcc for the comparison with pan, Python 3 and the shell tools of the
# build scripts, and the Go modules of the engine already downloaded. The image
# is stored in a docker registry (the registry is a parameter, never a built-in address).
#
# These scenarios check the definition (what it pins, installs and tags), not an
# image: building and pushing one needs docker and the network of the relay, and
# is described in build-container/README.md. The files live in the plugin tree, so
# they belong to the public release; the scenarios run from the monorepo root.

Feature: S3 The build container of the plugin

  Scenario: The container is based on a digest-pinned Go image of the version go.mod requires
    Given the repository root
    Then the repository file "model-check-plugin/build-container/Dockerfile" exists
    And the base image of "model-check-plugin/build-container/Dockerfile" is pinned by digest
    And the Go version of the base image of "model-check-plugin/build-container/Dockerfile" equals the go line of "model-check-plugin/engine/go.mod"
    And the Go version of the base image of "model-check-plugin/build-container/Dockerfile" equals the one "model-check-plugin/build.sh" requires

  Scenario: Every dependency of the build and of the tests is installed in the image
    Given the repository root
    Then the repository file "model-check-plugin/build-container/Dockerfile" mentions "spin"
    And the repository file "model-check-plugin/build-container/Dockerfile" mentions "python3"
    And the repository file "model-check-plugin/build-container/Dockerfile" mentions "gcc"
    And the repository file "model-check-plugin/build-container/Dockerfile" mentions "make"
    And the repository file "model-check-plugin/build-container/Dockerfile" mentions "git"
    And the repository file "model-check-plugin/build-container/Dockerfile" mentions "go mod download"
    And the repository file "model-check-plugin/build-container/Dockerfile" mentions "govulncheck"
    And the repository file "model-check-plugin/build-container/Dockerfile" mentions "GOTOOLCHAIN=local"
    And the repository file "model-check-plugin/.dockerignore" exists

  Scenario: The build script stores the image in the registry of the relay under the version of the plugin
    Given the repository root
    Then the repository file "model-check-plugin/build-container/build-image.sh" exists and is executable
    And the repository file "model-check-plugin/build-container/build-image.sh" mentions "MCD_REGISTRY"
    And the repository file "model-check-plugin/build-container/build-image.sh" mentions "--registry"
    And the repository file "model-check-plugin/build-container/build-image.sh" mentions "model-check/build"
    And the repository file "model-check-plugin/build-container/build-image.sh" mentions "plugin.json"
    And the repository file "model-check-plugin/build-container/build-image.sh" mentions "--push"
    And the repository file "model-check-plugin/build-container/build-image.sh" mentions "HTTPS_PROXY"

  Scenario: The container is documented where the build and the preparation are
    Given the repository root
    Then the repository file "model-check-plugin/build-container/README.md" mentions "$REGISTRY"
    And the repository file "model-check-plugin/README.md" mentions "build-container"
    And the repository file "model-check-plugin/AGENTS.md" mentions "build-container"

  Scenario: The CI runner image takes Go and the modules from the build container in the registry, by digest
    Given the repository root
    Then the repository file "model-check-plugin/build-container/runner.Dockerfile" exists
    And the repository file "model-check-plugin/build-container/runner.Dockerfile" mentions "ghcr.io/actions/actions-runner:2.337.0@sha256:"
    And the repository file "model-check-plugin/build-container/runner.Dockerfile" mentions "BUILD_IMAGE"
    And the repository file "model-check-plugin/build-container/runner.Dockerfile" mentions "COPY --from=go"
    And the repository file "model-check-plugin/build-container/runner.Dockerfile" mentions "/opt/hostedtoolcache/go/"
    And the repository file "model-check-plugin/build-container/runner.Dockerfile" mentions "x64.complete"
    And the repository file "model-check-plugin/build-container/runner.Dockerfile" mentions "spin"
    And the repository file "model-check-plugin/build-container/runner.Dockerfile" mentions "libc6-dev"
    And the repository file "model-check-plugin/build-container/build-runner-image.sh" exists and is executable
    And the repository file ".github/workflows/ci.yml" mentions "model-check-ci"

  Scenario: CI looks for the Go of go.mod on the runner first and sets up Go only when it is missing
    Given the repository root
    Then the repository file ".github/workflows/ci.yml" mentions "Check the Go toolchain on the runner"
    And the repository file ".github/workflows/ci.yml" mentions "steps.go.outputs.present"
    And the repository file ".github/workflows/ci.yml" mentions "/opt/hostedtoolcache/go/"
    And the repository file "model-check-plugin/build-container/runner.Dockerfile" mentions "RUNNER_TOOL_CACHE=/opt/hostedtoolcache"
    And the repository file "model-check-plugin/build-container/runner.Dockerfile" mentions "AGENT_TOOLSDIRECTORY=/opt/hostedtoolcache"

  Scenario: The plugin tree names no internal host, address or home directory
    Given the repository root
    Then no text file of "model-check-plugin" matches "192\.168\.[0-9]+\.[0-9]+"
    And no text file of "model-check-plugin" matches "relay-hur[o]n"
    And no text file of "model-check-plugin" matches "/home/(muron|huron)/"
