#!/usr/bin/env bash
# Build the build container of the plugin and, with --push, store it in a docker registry.
#
#   build-image.sh [--push] [--registry HOST[:PORT]] [--name REPO] [--no-check]
#
# The image is named REGISTRY/REPO:VERSION and REGISTRY/REPO:VERSION-goGOVERSION, with VERSION from
# plugin.json and GOVERSION from the Dockerfile's base image. The registry is the one in --registry or MCD_REGISTRY; with
# neither, the image gets no registry prefix and --push is refused. REPO defaults to model-check/build. A registry that
# speaks plain HTTP must be listed in the docker daemon's insecure-registries.
#
# The proxy of the environment (HTTP_PROXY, HTTPS_PROXY, NO_PROXY, either case) is given to the build as build
# arguments, and a proxy on localhost makes the build use the host network, because apt and the Go modules cannot
# be fetched otherwise on a host that reaches the net through one. The build arguments are not stored in the image.
#
# After the build the image is checked: the tools answer, the Go version is the one build.sh requires, and the
# modules of the engine build with no network. A failed check stops before --push.
#
# Exit codes: 0 ok, 1 build, check or push failed, 2 usage.
set -euo pipefail

HERE="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$(dirname "$HERE")"
REGISTRY="${MCD_REGISTRY:-}"
REPO="model-check/build"
PUSH=0
CHECK=1

die() { printf 'build-image.sh: %s\n' "$*" >&2; exit "${2:-1}"; }
usage() { sed -n '2,17p' "${BASH_SOURCE[0]}" >&2; exit 2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --push) PUSH=1; shift ;;
    --no-check) CHECK=0; shift ;;
    --registry) [ $# -ge 2 ] || usage; REGISTRY="$2"; shift 2 ;;
    --name) [ $# -ge 2 ] || usage; REPO="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) printf 'build-image.sh: unknown argument %s\n' "$1" >&2; usage ;;
  esac
done

command -v docker >/dev/null 2>&1 || die "docker is required"
VERSION="$(sed -n 's/^  "version": "\(.*\)",$/\1/p' "$PLUGIN_DIR/plugin.json" | head -1)"
[ -n "$VERSION" ] || die "no version in plugin.json"
GOVERSION="$(sed -n 's/^FROM golang:\([0-9][0-9.]*\)-.*/\1/p' "$HERE/Dockerfile" | head -1)"
[ -n "$GOVERSION" ] || die "no golang base image in the Dockerfile"
REQUIRED="$(sed -n 's/^REQUIRED_GO_VERSION="${MCD_GO_VERSION:-go\(.*\)}"$/\1/p' "$PLUGIN_DIR/build.sh")"
[ "$GOVERSION" = "$REQUIRED" ] || die "the base image has Go $GOVERSION, build.sh requires Go $REQUIRED"
IMAGE="${REGISTRY:+$REGISTRY/}$REPO"

args=(--build-arg "VERSION=$VERSION" -t "$IMAGE:$VERSION" -t "$IMAGE:$VERSION-go$GOVERSION")
local_proxy=0
for v in HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy; do
  if [ -n "${!v-}" ]; then
    args+=(--build-arg "$v=${!v}")
    case "$v:${!v}" in NO_PROXY:*|no_proxy:*) ;; *:*127.0.0.1*|*:*localhost*) local_proxy=1 ;; esac
  fi
done
[ "$local_proxy" = 1 ] && args+=(--network host)

printf 'build-image.sh: building %s:%s (Go %s)\n' "$IMAGE" "$VERSION" "$GOVERSION" >&2
docker build "${args[@]}" -f "$HERE/Dockerfile" "$PLUGIN_DIR" || die "docker build failed"

if [ "$CHECK" = 1 ]; then
  printf 'build-image.sh: checking the image\n' >&2
  docker run --rm --network none "$IMAGE:$VERSION" sh -c '
    set -e
    go version | grep -q "go'"$GOVERSION"' " || { go version >&2; exit 1; }
    spin -V; gcc --version | head -1; python3 --version; git --version; make --version | head -1
    test "$(go env GOTOOLCHAIN)" = local
    govulncheck -version | head -1' || die "the tool check of the image failed"
  # The modules of the engine are in the image: a build with no network, and a read-only checkout, must work.
  docker run --rm --network none --user "$(id -u):$(id -g)" -v "$PLUGIN_DIR/engine:/work:ro" "$IMAGE:$VERSION" \
    sh -c 'go build -o /tmp/mcd ./cmd/mcd && /tmp/mcd version && go vet ./ir ./report ./cex' \
    || die "the module check of the image failed (no network, modules from the image)"
fi

if [ "$PUSH" = 1 ]; then
  [ -n "$REGISTRY" ] || die "--push needs a registry: pass --registry HOST[:PORT] or set MCD_REGISTRY"
  for t in "$VERSION" "$VERSION-go$GOVERSION"; do
    docker push "$IMAGE:$t" || die "docker push $IMAGE:$t failed"
  done
  digest="$(docker image inspect --format '{{index .RepoDigests 0}}' "$IMAGE:$VERSION" 2>/dev/null || true)"
  printf 'build-image.sh: pushed %s\n' "${digest:-$IMAGE:$VERSION}" >&2
fi
