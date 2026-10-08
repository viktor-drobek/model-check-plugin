#!/bin/sh
# Build SPIN 6.5.2 from the official source and install it as /usr/local/bin/spin.
#
# The distribution packages are not the same SPIN: Ubuntu 24.04's package (6.5.2+dfsg-1) is built without -DNXT, so
# `spin -f` refuses the temporal operator X ("tl_spin: expected predicate, saw 'X'"), and the differential tests against
# `pan` use it. The upstream makefile builds with -DNXT. The tarball is pinned by its sha256.
#
# Run as root in an image build, with gcc, make, libc headers, curl and bison (or byacc) installed. The build tools that only
# this step needs (bison) are removed again by the Dockerfile.
set -eu

VERSION=6.5.2
URL="https://github.com/nimble-code/Spin/archive/refs/tags/version-$VERSION.tar.gz"
SHA256=e46a3bd308c4cd213cc466a8aaecfd5cedc02241190f3cb9a1d1b87e5f37080a
DIR=$(mktemp -d)
trap 'rm -rf "$DIR"' EXIT

curl -fsSL -o "$DIR/spin.tar.gz" "$URL"
echo "$SHA256  $DIR/spin.tar.gz" | sha256sum -c -
tar -xzf "$DIR/spin.tar.gz" -C "$DIR"
cd "$DIR/Spin-version-$VERSION/Src"
make CFLAGS="-O2 -DNXT" YACC="bison -y" spin
install -m 0755 spin /usr/local/bin/spin

# The build is checked by what it is for: the version, and the operator the packages lack.
test "$(spin -V)" = "Spin Version $VERSION -- 6 December 2019"
spin -f '!(X p)' | grep -q 'never'
