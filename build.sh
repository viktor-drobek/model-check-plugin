#!/usr/bin/env bash
# Reproducible build of the `mcd` engine for the model-check plugin (plan 14 §9, row G6).
#
# Produces, under engine/bin/:
#   mcd-<goos>-<goarch>[.exe]  one static binary per platform of PLATFORMS
#   mcd                        POSIX wrapper choosing the binary by uname
#                              (this is the command mcp/servers.json names)
#   mcd.exe                    Windows binary alias for portable MCP declarations
#   mcd.cmd                    Windows wrapper (shipped, NOT exercised by this repo's tests)
#   SHA256SUMS                 sha256 of every binary and wrapper, sorted by name
#   BUILD-INFO.json            version, toolchain, flags and per-platform sizes
#
# Reproducibility: -trimpath -buildvcs=false, CGO_ENABLED=0, a fixed -ldflags string
# and no wall-clock value in any artefact. Two runs from the same source tree produce
# byte-identical binaries and an identical SHA256SUMS (checked by `--verify-repro`).
#
# Version embedding: the binary prints report.EngineVersion (`mcd version`). That
# identifier is a *const* in engine/report/report.go, and `-ldflags -X` can only write
# to a string *var*. Rather than edit engine/ — owned by another agent during G6 — this
# script builds with `go build -overlay`, which substitutes a generated copy of
# report.go in which the single line `EngineVersion = "..."` has been moved out of the
# const block into a package-level var. No tracked file is modified. Once that const
# becomes a var upstream, the overlay is skipped automatically (see stamp_overlay).
#
# Usage:
#   build.sh [--version V] [--source-commit HASH]
#            [--platforms "goos/goarch ..."] [--host-only]
#            [--out DIR] [--verify-repro] [--no-verify]
# Environment: MCD_VERSION acts as the default for --version.
# Exit codes: 0 ok, 1 build or verification failure, 2 bad usage.

set -euo pipefail

PLUGIN_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ENGINE_DIR="$PLUGIN_DIR/engine"
OUT_DIR="$ENGINE_DIR/bin"

PLATFORMS_DEFAULT="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"
PLATFORMS="$PLATFORMS_DEFAULT"
VERSION="${MCD_VERSION:-}"
SOURCE_COMMIT="${MCD_SOURCE_COMMIT:-}"
HOST_ONLY=0
VERIFY=1
VERIFY_REPRO=0

die() { printf 'build.sh: %s\n' "$*" >&2; exit 1; }
usage() { sed -n '2,25p' "${BASH_SOURCE[0]}" >&2; exit 2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --version)   [ $# -ge 2 ] || usage; VERSION="$2"; shift 2 ;;
    --source-commit) [ $# -ge 2 ] || usage; SOURCE_COMMIT="$2"; shift 2 ;;
    --platforms) [ $# -ge 2 ] || usage; PLATFORMS="$2"; shift 2 ;;
    --out)       [ $# -ge 2 ] || usage; OUT_DIR="$2"; shift 2 ;;
    --host-only) HOST_ONLY=1; shift ;;
    --no-verify) VERIFY=0; shift ;;
    --verify-repro) VERIFY_REPRO=1; shift ;;
    -h|--help)   usage ;;
    *) printf 'build.sh: unknown argument %s\n' "$1" >&2; usage ;;
  esac
done

command -v go >/dev/null 2>&1 || die "no Go toolchain on PATH (build.sh is a build-time tool; the shipped plugin does not need one)"
command -v python3 >/dev/null 2>&1 || die "python3 is required (BUILD-INFO.json, plugin.json version)"

# ---------------------------------------------------------------- version
PLUGIN_VERSION="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' \
                   "$PLUGIN_DIR/plugin.json")"
CLAUDE_VERSION="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' \
                   "$PLUGIN_DIR/.claude-plugin/plugin.json")"
[ "$PLUGIN_VERSION" = "$CLAUDE_VERSION" ] || die "plugin versions differ: plugin.json=$PLUGIN_VERSION .claude-plugin/plugin.json=$CLAUDE_VERSION"
[ -n "$VERSION" ] || VERSION="$PLUGIN_VERSION"
case "$VERSION" in
  ''|*[![:print:]]*|*' '*) die "bad version string: '$VERSION'" ;;
esac

GO_VERSION="$(cd "$ENGINE_DIR" && go env GOVERSION)"
REQUIRED_GO_VERSION="${MCD_GO_VERSION:-go1.26.8}"
[ "$GO_VERSION" = "$REQUIRED_GO_VERSION" ] || die "Go $REQUIRED_GO_VERSION is required for reproducible release builds (found $GO_VERSION)"
HOST_GOOS="$(go env GOHOSTOS)"
HOST_GOARCH="$(go env GOHOSTARCH)"
[ -n "$SOURCE_COMMIT" ] || SOURCE_COMMIT="$(git -C "$PLUGIN_DIR" rev-parse HEAD 2>/dev/null || echo unknown)"

case "$SOURCE_COMMIT" in
  ''|*[![:alnum:]._-]*) die "bad source commit: '$SOURCE_COMMIT'" ;;
esac

[ "$HOST_ONLY" -eq 1 ] && PLATFORMS="$HOST_GOOS/$HOST_GOARCH"

# ------------------------------------------------- version-stamping overlay
# Echoes the path of an overlay JSON file, or nothing when the overlay is
# unnecessary (EngineVersion already a var) — in which case -X applies directly.
REPORT_GO="$ENGINE_DIR/report/report.go"
# Created here, not inside stamp_overlay: that function runs in a command
# substitution, so anything it assigns is lost to the subshell and the trap
# below would never see the directory.
STAMP_TMP="$(mktemp -d)"
cleanup() { rc=$?; rm -rf "$STAMP_TMP"; exit "$rc"; }
trap cleanup EXIT

stamp_overlay() {
  [ -f "$REPORT_GO" ] || die "missing $REPORT_GO"
  if grep -qE '^var[[:space:]]+EngineVersion[[:space:]]*=' "$REPORT_GO"; then
    return 0   # upstream made it a var: nothing to do
  fi
  grep -qE '^[[:space:]]*EngineVersion[[:space:]]*=[[:space:]]*"[^"]*"[[:space:]]*$' "$REPORT_GO" \
    || die "cannot find the 'EngineVersion = \"...\"' declaration in $REPORT_GO; version stamping would be silently inert — refusing to build"
  local n
  n="$(grep -cE '^[[:space:]]*EngineVersion[[:space:]]*=[[:space:]]*"[^"]*"[[:space:]]*$' "$REPORT_GO")"
  [ "$n" = "1" ] || die "expected exactly one EngineVersion declaration in $REPORT_GO, found $n"

  awk '
    /^[[:space:]]*EngineVersion[[:space:]]*=[[:space:]]*"[^"]*"[[:space:]]*$/ && !done {
      sub(/^[[:space:]]*EngineVersion[[:space:]]*=[[:space:]]*/, "", $0)
      val = $0; done = 1; next
    }
    { print }
    END { printf "\n// EngineVersion is written by build.sh through -ldflags -X.\nvar EngineVersion = %s\n", val }
  ' "$REPORT_GO" > "$STAMP_TMP/report.go"

  grep -qE '^var EngineVersion = "' "$STAMP_TMP/report.go" \
    || die "overlay generation failed: no 'var EngineVersion' in the generated copy"

  python3 - "$REPORT_GO" "$STAMP_TMP/report.go" > "$STAMP_TMP/overlay.json" <<'PY'
import json, sys
print(json.dumps({"Replace": {sys.argv[1]: sys.argv[2]}}))
PY
  printf '%s' "$STAMP_TMP/overlay.json"
}

OVERLAY="$(stamp_overlay)"

# --------------------------------------------------------------- building
mkdir -p "$OUT_DIR"
# Remove only files owned by this script. This makes a host-only build produce a
# self-contained output directory instead of retaining stale cross-built files.
rm -f "$OUT_DIR"/mcd-* "$OUT_DIR"/mcd "$OUT_DIR"/mcd.exe "$OUT_DIR"/mcd.cmd \
      "$OUT_DIR"/SHA256SUMS "$OUT_DIR"/BUILD-INFO.json
LDFLAGS="-s -w -X modelcheck/report.EngineVersion=$VERSION"

build_one() {
  local goos="$1" goarch="$2" out="$3"
  local -a flags=(-trimpath -buildvcs=false -ldflags "$LDFLAGS" -o "$out")
  [ -n "$OVERLAY" ] && flags+=(-overlay "$OVERLAY")
  ( cd "$ENGINE_DIR" \
    && env CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOFLAGS=-mod=readonly GOTOOLCHAIN=local \
       go build "${flags[@]}" ./cmd/mcd )
}

built=""
for p in $PLATFORMS; do
  goos="${p%%/*}"; goarch="${p##*/}"
  [ "$goos" = "$p" ] && die "bad platform '$p' (want goos/goarch)"
  name="mcd-$goos-$goarch"; [ "$goos" = "windows" ] && name="$name.exe"
  printf 'build.sh: %s/%s -> %s\n' "$goos" "$goarch" "$name" >&2
  build_one "$goos" "$goarch" "$OUT_DIR/$name"
  built="$built $name"
done

# -------------------------------------------------------------- wrappers
# The plugin's MCP config (mcp/servers.json, named by plugin.json) gives ONE
# command. Claude Code's manifest reference documents no per-platform command
# selection, so the choice is made here, by the wrapper, from uname. Tested on
# the host platform only (see steps/g6-confirmation.md §1).
cat > "$OUT_DIR/mcd" <<'SH'
#!/bin/sh
# Wrapper: exec the mcd binary matching this machine (runtime.GOOS/GOARCH names).
set -e
# ${0%/*} rather than dirname(1): the wrapper is the plugin's MCP command and
# must not depend on more of the host than a POSIX shell and uname.
case "$0" in
  */*) dir=$(CDPATH= cd -- "${0%/*}" && pwd) ;;
  *)   dir=$(pwd) ;;
esac
case "$(uname -s)" in
  Linux)   goos=linux ;;
  Darwin)  goos=darwin ;;
  MINGW*|MSYS*|CYGWIN*) goos=windows ;;
  *) echo "mcd: unsupported operating system $(uname -s)" >&2; exit 127 ;;
esac
case "$(uname -m)" in
  x86_64|amd64)   goarch=amd64 ;;
  arm64|aarch64)  goarch=arm64 ;;
  *) echo "mcd: unsupported architecture $(uname -m)" >&2; exit 127 ;;
esac
bin="$dir/mcd-$goos-$goarch"
[ "$goos" = windows ] && bin="$bin.exe"
if [ ! -x "$bin" ]; then
  echo "mcd: no binary for $goos/$goarch at $bin; run model-check-plugin/build.sh" >&2
  exit 127
fi
exec "$bin" "$@"
SH
chmod +x "$OUT_DIR/mcd"

cat > "$OUT_DIR/mcd.cmd" <<'CMD'
@echo off
rem Wrapper for Windows shells. Shipped for completeness; not exercised by this repo's tests.
"%~dp0mcd-windows-amd64.exe" %*
CMD

# Windows resolves a command without an extension through PATHEXT. Keep an
# executable alias next to the POSIX wrapper so the portable mcp.json command
# works on both families of hosts. mcd.cmd remains available for hosts that
# explicitly require a batch command. A host-only build may not include the
# Windows target, so only create the alias when that target was requested.
case " $built " in
  *" mcd-windows-amd64.exe "*) cp "$OUT_DIR/mcd-windows-amd64.exe" "$OUT_DIR/mcd.exe" ;;
esac

# ------------------------------------------------------------ SHA256SUMS
( cd "$OUT_DIR" && ls -1 | grep -v -e '^SHA256SUMS$' -e '^BUILD-INFO.json$' | LC_ALL=C sort \
  | xargs sha256sum > SHA256SUMS )

# --------------------------------------------------------- BUILD-INFO.json
python3 - "$OUT_DIR" "$VERSION" "$GO_VERSION" "$SOURCE_COMMIT" "$LDFLAGS" "$PLATFORMS" \
         "$([ -n "$OVERLAY" ] && echo overlay || echo direct)" > "$OUT_DIR/BUILD-INFO.json" <<'PY'
import json, os, sys
out, version, goversion, commit, ldflags, platforms, stamp = sys.argv[1:8]
entries = []
for p in platforms.split():
    goos, goarch = p.split("/")
    name = "mcd-%s-%s" % (goos, goarch) + (".exe" if goos == "windows" else "")
    path = os.path.join(out, name)
    entries.append({"goos": goos, "goarch": goarch, "file": name,
                    "size_bytes": os.path.getsize(path) if os.path.exists(path) else None})
doc = {
    "version": version,
    "engine_module": "modelcheck",
    "go_version": goversion,
    "cgo_enabled": "0",
    "build_flags": ["-trimpath", "-buildvcs=false", "-ldflags", ldflags],
    "version_stamp": stamp,
    "source_commit": commit,
    "binaries": sorted(entries, key=lambda e: e["file"]),
    "note": ("No wall-clock value is recorded, so BUILD-INFO.json is a function of the "
             "source tree and the arguments only. Cross-built binaries are produced and "
             "hashed here; only the host binary is executed by the build's own checks."),
}
print(json.dumps(doc, indent=2, sort_keys=True))
PY

# ------------------------------------------------------------ verification
host_name="mcd-$HOST_GOOS-$HOST_GOARCH"; [ "$HOST_GOOS" = "windows" ] && host_name="$host_name.exe"
if [ "$VERIFY" -eq 1 ] && [ -x "$OUT_DIR/$host_name" ]; then
  reported="$("$OUT_DIR/$host_name" version)"
  case "$reported" in
    *"$VERSION"*) : ;;
    *) die "version stamping did not take: 'mcd version' said '$reported', expected to contain '$VERSION'" ;;
  esac
  wrapped="$("$OUT_DIR/mcd" version)"
  [ "$wrapped" = "$reported" ] || die "wrapper picked a different binary: '$wrapped' vs '$reported'"
  ( cd "$OUT_DIR" && sha256sum -c --quiet SHA256SUMS ) || die "SHA256SUMS does not match the files just written"
  printf 'build.sh: %s\n' "$reported" >&2
fi

if [ "$VERIFY_REPRO" -eq 1 ]; then
  tmp="$(mktemp -d)"
  "$0" --version "$VERSION" --source-commit "$SOURCE_COMMIT" --platforms "$PLATFORMS" --out "$tmp" --no-verify >/dev/null
  python3 - "$OUT_DIR" "$tmp" <<'PY'
import os, sys

left, right = sys.argv[1:]
left_files = sorted(name for name in os.listdir(left) if os.path.isfile(os.path.join(left, name)))
right_files = sorted(name for name in os.listdir(right) if os.path.isfile(os.path.join(right, name)))
if left_files != right_files:
    raise SystemExit(f"not reproducible: generated file sets differ: {left_files!r} != {right_files!r}")
for name in left_files:
    with open(os.path.join(left, name), "rb") as a, open(os.path.join(right, name), "rb") as b:
        if a.read() != b.read():
            raise SystemExit(f"not reproducible: {name} differs between two builds")
PY
  rm -rf "$tmp"
  printf 'build.sh: reproducible (two builds agree on every generated artifact)\n' >&2
fi

printf 'build.sh: wrote %s\n' "$OUT_DIR" >&2
