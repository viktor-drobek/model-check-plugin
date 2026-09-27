#!/bin/sh
# Reproduce the finding. Run from a scratch directory.
set -e
SRC="${1:?usage: run.sh <path-to-.pr>}"
cp "$SRC" ./model.pr
spin -a model.pr
gcc -O2 -o pan pan.c
./pan -m10000 || true
[ -f model.pr.trail ] && ./pan -r || true
