# Runner image of the self-hosted CI job of this repository (the label model-check-ci): the official GitHub Actions
# runner image (pinned by digest, the version of the other runners on the host) plus what ci.yml needs, so that
# the job downloads no toolchain: Go 1.26.8 in the runner's tool cache (actions/setup-go finds it there and does not
# fetch one), SPIN 6.5.2 and gcc for the differential tests, Python 3, numactl and bc, and the Go modules of the
# engine already in the module cache of the runner user.
#
# The Go toolchain and the modules are taken from the build container (Dockerfile in this directory) as it is stored in a
# registry, by digest, not downloaded or built again: one source for both images. BUILD_IMAGE names it (name@sha256:digest);
# build-runner-image.sh looks the digest up and passes it.
#
# Build it on the host that runs the runner, with model-check-plugin/ as the context:
#   build-container/build-runner-image.sh --registry HOST[:PORT] [--host SSH_HOST] [--push]
# The tool versions are those of Dockerfile in this directory; scenarios keep both on the Go version of go.mod.
ARG BUILD_IMAGE
FROM ${BUILD_IMAGE} AS go

FROM ghcr.io/actions/actions-runner:2.337.0@sha256:e5496277be5d09bc968b3d64911b74e219ac4a3f2edce956a3ecf9271bea1ef4

ARG VERSION=dev
ARG GOVERSION=1.26.8
LABEL org.opencontainers.image.title="model-check CI runner" \
      org.opencontainers.image.description="GitHub Actions runner with Go ${GOVERSION}, SPIN 6.5.2, gcc, Python 3 and the Go modules of the model-check engine" \
      org.opencontainers.image.version="${VERSION}"

USER root
RUN set -eu \
    && apt-get update \
    && apt-get install -y --no-install-recommends spin gcc libc6-dev make python3 bc numactl git \
    && rm -rf /var/lib/apt/lists/* \
    && spin -V \
    && printf '#include <pthread.h>\n#include <stdint.h>\nint main(void){return 0;}\n' | gcc -x c - -o /dev/null

# actions/setup-go looks for <tool cache>/go/<version>/<arch> and the marker <arch>.complete before it downloads anything.
COPY --from=go /usr/local/go /opt/hostedtoolcache/go/${GOVERSION}/x64
RUN set -eu \
    && touch /opt/hostedtoolcache/go/${GOVERSION}/x64.complete \
    && chown -R runner:docker /opt/hostedtoolcache/go \
    && /opt/hostedtoolcache/go/${GOVERSION}/x64/bin/go version | grep -q "go${GOVERSION} "

# The runner looks for tools in $RUNNER_TOOL_CACHE, and when it is not set in <work>/_tool, where the Go above is not: setup-go
# then downloads a Go that is already in the image. Both names, because the runner reads AGENT_TOOLSDIRECTORY and exports the other.
USER runner
ENV RUNNER_TOOL_CACHE=/opt/hostedtoolcache \
    AGENT_TOOLSDIRECTORY=/opt/hostedtoolcache \
    GOTOOLCHAIN=local \
    GOFLAGS=-modcacherw
# The module cache of the build container (the modules of engine/go.mod and go.sum, already downloaded), for the runner user.
COPY --from=go --chown=runner:docker /opt/go/pkg/mod /home/runner/go/pkg/mod
RUN set -eu \
    && test -n "$(ls /home/runner/go/pkg/mod/github.com | head -1)"
