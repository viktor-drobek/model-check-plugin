#!/bin/bash
# Reproduces every verification run in this directory.  Requires spin + gcc.
# Usage: bash reproduce.sh
set -e
HERE="$(cd "$(dirname "$0")" && pwd)"
run() { # run <dir> <model> <gcc-flags> <pan-args...>
  d="$HERE/run/$1"; m="$2"; flags="$3"; shift 3
  mkdir -p "$d"; cp "$HERE/models/$m" "$d/"; cd "$d"
  spin -a "$m" >/dev/null; gcc -O2 $flags -o pan pan.c
  ./pan "$@" -m100000
}
echo "== 01 safety, original model (POR on) ==";        run 01-safety        abp-original.pml     "-DSAFETY"
echo "== 03 safety, original model (POR off) ==";       run 03-orig-noreduce abp-original.pml     "-DSAFETY -DNOREDUCE"
echo "== 02 safety, instrumented ==";                   run 02-safety-instr  abp-instrumented.pml "-DSAFETY -DNOREDUCE"
echo "== 04 non-progress cycles (+ -f) ==";             run 04-np            abp-instrumented.pml "-DNP -DNOREDUCE" -l
                                                        (cd "$HERE/run/04-np" && ./pan -l -f -m100000)
echo "== 05 LTL claims =="
mkdir -p "$HERE/run/05-ltl"; cp "$HERE/models/abp-ltl.pml" "$HERE/run/05-ltl/"; cd "$HERE/run/05-ltl"
spin -a abp-ltl.pml >/dev/null; gcc -O2 -DNOREDUCE -o pan pan.c
for c in deliver1 deliver0 forever excl cap bogus; do
  for f in "" "-f"; do echo "--- $c $f ---"; ./pan -a $f -N $c -m100000 | grep -E "errors:"; done
done
echo "== 06 alternatingbit.pml under ONE message loss (expected: FAILS) ==" ; run 06-lossy abp-lossy1.pml  "-DSAFETY -DNOREDUCE" || true
echo "== 07 alternatingbit2.pml under ONE message loss (expected: holds) ==" ; run 07-abp2  abp2-lossy1.pml "-DSAFETY -DNOREDUCE"
