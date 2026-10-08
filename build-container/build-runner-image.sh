#!/usr/bin/env bash
# Build the image of the self-hosted CI runner (build-container/runner.Dockerfile) on the docker host that runs the runner,
# check it, and with --push store it in a docker registry.
#
#   build-runner-image.sh --registry HOST[:PORT] [--host SSH_HOST] [--push] [--no-check]
#
# With --host the build runs on that machine over ssh (the context is sent as a tar stream: the Dockerfile, nothing else; the Go
# toolchain and the modules come from the build container in the registry); without --host, on the local docker. The image is named
# REGISTRY/model-check/runner:VERSION and REGISTRY/model-check/runner:VERSION-goGOVERSION (VERSION from plugin.json, GOVERSION
# from the Dockerfile). The registry is the one in --registry or MCD_REGISTRY, as the docker host builds and pushes to it (a
# registry on the build host itself may be named localhost:5000 there). The build container image (build-image.sh) is the source
# of the Go toolchain and the modules: its digest is looked up in that registry and given to the build, so the runner image
# is always built from the stored build container, never from a name that could move.
#
# The check runs the image: the runner user, SPIN, the Go toolchain in the runner's tool cache with its completion marker (so
# actions/setup-go downloads nothing), and the module cache. A failed check stops before --push.
#
# Exit codes: 0 ok, 1 build, check or push failed, 2 usage.
set -euo pipefail

HERE="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$(dirname "$HERE")"
HOST=""
REGISTRY="${MCD_REGISTRY:-}"
PUSH=0
CHECK=1

die() { printf 'build-runner-image.sh: %s\n' "$*" >&2; exit 1; }
usage() { sed -n '2,17p' "${BASH_SOURCE[0]}" >&2; exit 2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --host) [ $# -ge 2 ] || usage; HOST="$2"; shift 2 ;;
    --push) PUSH=1; shift ;;
    --registry) [ $# -ge 2 ] || usage; REGISTRY="$2"; shift 2 ;;
    --no-check) CHECK=0; shift ;;
    -h|--help) usage ;;
    *) printf 'build-runner-image.sh: unknown argument %s\n' "$1" >&2; usage ;;
  esac
done
[ -n "$REGISTRY" ] || die "a registry is required: pass --registry HOST[:PORT] or set MCD_REGISTRY"

# dk runs docker where the image is built.
dk() { if [ -n "$HOST" ]; then ssh -o BatchMode=yes "$HOST" docker "$@"; else docker "$@"; fi; }

VERSION="$(sed -n 's/^  "version": "\(.*\)",$/\1/p' "$PLUGIN_DIR/plugin.json" | head -1)"
[ -n "$VERSION" ] || die "no version in plugin.json"
GOVERSION="$(sed -n 's/^ARG GOVERSION=\(.*\)$/\1/p' "$HERE/runner.Dockerfile" | head -1)"
[ -n "$GOVERSION" ] || die "no ARG GOVERSION in runner.Dockerfile"
REQUIRED="$(sed -n 's/^REQUIRED_GO_VERSION="${MCD_GO_VERSION:-go\(.*\)}"$/\1/p' "$PLUGIN_DIR/build.sh")"
[ "$GOVERSION" = "$REQUIRED" ] || die "the Dockerfile has Go $GOVERSION, build.sh requires Go $REQUIRED"
IMAGE="$REGISTRY/model-check/runner"
BUILD_REF="$REGISTRY/model-check/build:$VERSION"
dk pull "$BUILD_REF" >/dev/null || die "the build container $BUILD_REF is not in the registry (build-image.sh --push first)"
# The inspect output is parsed here, not with a --format template: over ssh a template would be parsed twice.
BUILD_DIGEST="$(dk image inspect "$BUILD_REF" | python3 -c '
import json, sys
want = sys.argv[1] + "@"
for img in json.load(sys.stdin):
    for d in img.get("RepoDigests") or []:
        if d.startswith(want):
            print(d); raise SystemExit
' "$REGISTRY/model-check/build")"
[ -n "$BUILD_DIGEST" ] || die "no digest for $BUILD_REF"

printf 'build-runner-image.sh: building %s:%s (Go %s) on %s\n' "$IMAGE" "$VERSION" "$GOVERSION" "${HOST:-the local docker}" >&2
tar -C "$PLUGIN_DIR" -cf - build-container/runner.Dockerfile \
  | dk build --build-arg "VERSION=$VERSION" --build-arg "GOVERSION=$GOVERSION" --build-arg "BUILD_IMAGE=$BUILD_DIGEST" \
      -f build-container/runner.Dockerfile -t "$IMAGE:$VERSION" -t "$IMAGE:$VERSION-go$GOVERSION" - \
  || die "docker build failed"

if [ "$CHECK" = 1 ]; then
  printf 'build-runner-image.sh: checking the image\n' >&2
  check_script="set -e
test \"\$(id -un)\" = runner
spin -V; gcc --version | head -1; python3 --version; git --version
G=/opt/hostedtoolcache/go/$GOVERSION/x64
test -f \$G.complete
\$G/bin/go version | grep -q 'go$GOVERSION '
test -n \"\$(ls /home/runner/go/pkg/mod/github.com | head -1)\"
test -x /home/runner/run.sh"
  printf '%s\n' "$check_script" | dk run --rm -i --network none --entrypoint sh "$IMAGE:$VERSION" -s || die "the check of the image failed"
fi

if [ "$PUSH" = 1 ]; then
  for t in "$VERSION" "$VERSION-go$GOVERSION"; do
    dk push "$IMAGE:$t" || die "docker push $IMAGE:$t failed"
  done
  printf 'build-runner-image.sh: pushed %s:%s\n' "$IMAGE" "$VERSION" >&2
fi
