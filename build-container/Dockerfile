# Build container of the model-check plugin: everything the engine needs to build, test and verify, installed
# once. Build context: model-check-plugin/ (.dockerignore lets in go.mod and go.sum only).
#
#   build-container/build-image.sh [--push]      builds it and, with --push, stores it in a registry (--registry or MCD_REGISTRY)
#
# The Go image is pinned by digest and is the Go version that engine/go.mod and build.sh require: the release
# binaries carry the standard library of the Go that builds them, and build.sh refuses any other. Run the container
# as the user of the checkout it works on (`--user "$(id -u):$(id -g)"`): HOME, the build cache and the module
# cache are set so that any user id can use them; the modules the engine needs are already in the image.
FROM golang:1.26.8-bookworm@sha256:dc9ad6c05acc7a88e5b71bde60a5fe3bd4b9f0db209011711b464107438a8107

ARG VERSION=dev
ARG SOURCE=https://github.com/viktor-drobek/model-check-plugin
LABEL org.opencontainers.image.title="model-check build container" \
      org.opencontainers.image.description="Go toolchain, SPIN, gcc, Python 3 and the Go modules of the model-check engine" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="${SOURCE}"

# SPIN 6.5.2 is the oracle of the engine's differential tests (spin -a, then gcc builds pan); Python 3 is used by the
# build script and the release metadata; numactl pins workers to a NUMA node in the speed-up measurements; bc is used by
# the measurement scripts. gcc, make, git and curl are in the base image: they are checked below, not assumed.
RUN set -eu \
    && apt-get update \
    && apt-get install -y --no-install-recommends spin python3 bc numactl \
    && rm -rf /var/lib/apt/lists/* \
    && for t in gcc make git curl python3 spin numactl bc; do command -v "$t" >/dev/null || { echo "missing: $t" >&2; exit 1; }; done \
    && spin -V

# The toolchain must be exactly the one the base image has: the build refuses another Go and must never download one.
ENV GOTOOLCHAIN=local \
    GOFLAGS=-modcacherw \
    GOPATH=/opt/go \
    GOMODCACHE=/opt/go/pkg/mod \
    GOBIN=/usr/local/bin \
    GOCACHE=/tmp/go-build \
    HOME=/tmp

# The Go modules of the engine (the MCP SDK, godog and what they need), downloaded once from go.mod and go.sum; the
# vulnerability checker the security gate runs (the version of scripts/security-scan.sh).
COPY engine/go.mod engine/go.sum /opt/modules/
RUN set -eu \
    && cd /opt/modules \
    && go mod download \
    && go install golang.org/x/vuln/cmd/govulncheck@v1.8.0 \
    && chmod -R a+rX /opt/go /usr/local/bin/govulncheck \
    && rm -rf /root/.cache /tmp/go-build

WORKDIR /work
CMD ["bash"]
