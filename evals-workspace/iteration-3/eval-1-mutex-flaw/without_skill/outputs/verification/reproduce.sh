#!/bin/sh
# Reproduces every check reported in ../answer.md.
# Requires: spin 6.5.2, gcc. Run from this directory.
set -e
SRC=mutex_flaw.pml

echo "### 1. Mutual exclusion via assert(cnt == 1) -- exhaustive safety search"
spin -a $SRC
gcc -O2 -DSAFETY -DNOREDUCE -DREACH -o pan_reach pan.c
./pan_reach -i -m100000 || true          # -i = shortest counterexample
spin -t -p -g $SRC                        # replay the 15-step counterexample

echo "### 2. Same property as an explicit LTL safety claim  [] (cnt <= 1)"
( cd ltl && spin -a mutex_flaw_ltl.pml && gcc -O2 -o pan_ltl pan.c && ./pan_ltl -a -m100000 ) || true

echo "### 3. Deadlock / invalid end states (assertions ignored) -- expected: 0 errors"
gcc -O2 -DSAFETY -o pan pan.c
./pan -A -m100000

echo "### 4. Starvation: non-progress cycles under weak fairness"
( cd progress && spin -a mutex_flaw_prog.pml && gcc -O2 -DNP -o pan_np pan.c \
  && ./pan_np -l -f -m200000 && spin -t -p -g mutex_flaw_prog.pml ) || true

echo "### 5. Reference: peterson.pml under the same property -- expected: 0 errors"
( cd peterson-reference && spin -a peterson.pml && gcc -O2 -DSAFETY -o pan_p pan.c && ./pan_p -m100000 )
