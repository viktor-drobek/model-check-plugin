# Build container

A container in which the engine builds, is tested and is verified with nothing installed on the host but docker. It
is the preparation of `README.md` ("Requirements", "Prepare and build") done once.

## What is in the image

| Tool | Version | Used for |
|---|---|---|
| Go | 1.26.8 (`golang:1.26.8-bookworm`, pinned by digest in the Dockerfile) | `go build`, `go test`, `go vet`, `build.sh` (which refuses any other Go; `GOTOOLCHAIN=local`, so none is downloaded) |
| SPIN | 6.5.2 (Debian package `spin`) | `spin -a` for the differential tests against `pan` (`tools/pandiff`, the `@spin` scenarios) |
| gcc, make, git, curl | Debian 12 (bookworm) | build `pan.c`, run the scripts, source-commit identification |
| Python 3 | 3.11 | `build.sh`, release metadata |
| numactl, bc | Debian 12 | pinning workers to a NUMA node and timing, in the speed-up measurements |
| govulncheck | v1.8.0 | the vulnerability check of `scripts/security-scan.sh` and of the release binaries |
| Go modules of the engine | those of `engine/go.mod` and `go.sum` | in `/opt/go/pkg/mod`, so that a build and the tests need no network |

Not in the image: the plugin itself (mount the checkout), `claude`, `codex` or any agent host, trivy and semgrep (the
security script runs them from their own pinned images).

## The image in a registry

The image is named `<registry>/model-check/build`, with the tags `<plugin version>` (`0.3.0`) and `<plugin version>-go<Go version>`
(`0.3.0-go1.26.8`). Build it, or pull it from the registry where your team keeps it; the examples below write the registry as
`$REGISTRY` (a host and port, for example `registry.example.org:5000`). A registry that speaks plain HTTP must be listed in the docker
daemon's `insecure-registries`.

```bash
export REGISTRY=registry.example.org:5000
docker pull $REGISTRY/model-check/build:0.3.0
```

## Use

Run it as the user of the checkout, with the checkout mounted:

```bash
docker run --rm -it --user "$(id -u):$(id -g)" -v "$PWD:/work" -w /work/model-check-plugin/engine \
    $REGISTRY/model-check/build:0.3.0 \
    sh -c 'go vet ./... && go test ./...'
```

Release build, reproducible, from the plugin root (the Go of the image is the one `build.sh` requires):

```bash
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/work" -w /work/model-check-plugin \
    $REGISTRY/model-check/build:0.3.0 \
    ./build.sh --version 0.3.0 --source-commit "$(git rev-parse HEAD)" --verify-repro
```

Vulnerability check of the built files: `govulncheck -mode=binary engine/bin/mcd-linux-amd64`.

`HOME` is `/tmp`, the build cache is `/tmp/go-build`, and the module cache is read-only to a user other than root:
the image works for any user id, and a build leaves its cache in the container, not in the checkout. A test that
builds docker images (none does) would need the docker socket, which is not mounted.

## Build and store the image

```bash
build-container/build-image.sh                            # build and check, nothing is stored
build-container/build-image.sh --push --registry $REGISTRY # and store it in the registry
```

The script names the image from `plugin.json` (version) and the Dockerfile (Go version), refuses a base image
whose Go differs from the one `build.sh` requires, gives the build the proxy of the environment (a proxy on localhost
makes it use the host network), and checks the result: the tools answer, the Go version is the required one, and the
engine builds and passes `go vet` on a read-only checkout with `--network none`, from the modules in the image. A
failed check stops before `--push`. Options: `--registry HOST[:PORT]` (or `MCD_REGISTRY`; required for `--push`), `--name REPO`
(default `model-check/build`), `--no-check`.

Rebuild when `engine/go.mod` or `go.sum` change (the modules are in a layer of their own), when `build.sh` asks for another
Go, or when the plugin version changes; the scenarios of `features/s3-build-container.feature` fail when the Dockerfile
and `go.mod` or `build.sh` disagree about the Go version.

## The CI runner

The job `Build & Test` of a CI workflow that selects a self-hosted runner needs the toolchain on it. `runner.Dockerfile` makes an image of
the official GitHub Actions runner (version and digest pinned) with Go in the runner's tool cache (so `actions/setup-go` finds it and
downloads nothing), SPIN 6.5.2, gcc and libc headers, Python 3, numactl, bc, and the Go modules. The Go toolchain and the modules are
copied from the build container as it is stored in your registry, by digest, not built again.

```bash
build-container/build-runner-image.sh --registry $REGISTRY --host BUILD_HOST --push   # build on the host that runs the runner, check, store
```

The image sets `RUNNER_TOOL_CACHE` and `AGENT_TOOLSDIRECTORY` to `/opt/hostedtoolcache`, where the Go is: without them the runner looks in
`<work>/_tool`, finds nothing, and `actions/setup-go` downloads a Go that is already in the image. A workflow can check the tool cache (and
`go` on `PATH`) for the Go of `go.mod` first and run `setup-go` only when it is not there. After a change of `engine/go.mod`, `go.sum` or
the Go version, rebuild the build image, then the runner image, and restart the runner.
