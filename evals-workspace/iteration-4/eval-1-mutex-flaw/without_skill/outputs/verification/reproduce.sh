#!/bin/sh
# Reproduces every result quoted in ../answer.md.
# Run from this directory. Requires: spin 6.5.2, a C compiler.
# NB: pan exits non-zero when it finds errors - that is the expected
# outcome for steps 2, 5 and 6, so this script deliberately does not use `set -e`.
cd "$(dirname "$0")"
REPO=$(cd ../../../../../../.. && pwd)   # repository root, as an absolute path
SRC="$REPO/Promela - examples/CH2/mutex_flaw.pml"

cp "$SRC" mutex_flaw.pml

echo "### 1. parse ###"
spin -a mutex_flaw.pml

echo "### 2. exhaustive safety search, stop at first error -> counterexample ###"
cc -O2 -DSAFETY -o pan pan.c
./pan -m100000 -w26            # assertion violated, writes mutex_flaw.pml.trail

echo "### 3. same search, do not stop -> full state space ###"
./pan -m100000 -w26 -c0        # 429 states, 8 errors, search completed

echo "### 4. deadlock only, assertions ignored ###"
./pan -A -m100000 -w26 -c0     # errors: 0

echo "### 5. shortest counterexample (breadth-first) and its decoding ###"
rm -f mutex_flaw.pml.trail
cc -O2 -DSAFETY -DBFS -o pan_bfs pan.c
./pan_bfs -m100000 -w26        # assertion violated at depth 14
spin -t -p -g -l -w mutex_flaw.pml

echo "### 6. corroboration: mutual exclusion as an LTL invariant over labels ###"
# mutex_flaw_ltl.pml is the same model with the counter and the assert removed,
# so that the only property checked is [] !(user[0]@L7 && user[1]@L7).
spin -a mutex_flaw_ltl.pml
rm -f mutex_flaw_ltl.pml.trail
cc -O2 -DSAFETY -DBFS -o pan_ltl_bfs pan.c
./pan_ltl_bfs -m100000 -w26    # claim violated at depth 11
spin -t -p -g -w mutex_flaw_ltl.pml

echo "### 7. reference points: the correct algorithms from the same directory ###"
mkdir -p fix-check && cd fix-check
cp "$REPO/Promela - examples/CH2/peterson.pml" .
spin -a peterson.pml
cc -O2 -DSAFETY -o pan pan.c
./pan -m100000 -w26            # Peterson: errors: 0
cd ..

mkdir -p fix-check/dekker && cd fix-check/dekker
cp "$REPO/Promela - examples/CH2/mutex.pml" .
spin -a mutex.pml
cc -O2 -DSAFETY -o pan pan.c
./pan -m100000 -w26            # Dekker: errors: 0
cd ../..

rm -f pan pan_bfs pan_ltl_bfs pan.c pan.h pan.t pan.m pan.p pan.b _spin_nvr.tmp
for d in fix-check fix-check/dekker; do
  rm -f "$d"/pan "$d"/pan.c "$d"/pan.h "$d"/pan.t "$d"/pan.m "$d"/pan.p "$d"/pan.b "$d"/_spin_nvr.tmp
done
echo "### done - see ../answer.md ###"
