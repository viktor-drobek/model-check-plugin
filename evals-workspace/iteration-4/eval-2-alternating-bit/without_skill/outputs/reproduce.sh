#!/bin/sh
# Reproduces every run reported in answer.md.
# Requires: spin (tested with 6.5.2), gcc.  Run from any directory.
#
#   sh reproduce.sh           # -> logs/ and trails/ are rebuilt
set -e
HERE=$(cd "$(dirname "$0")" && pwd)
M="$HERE/models"
L="$HERE/logs";   mkdir -p "$L"
T="$HERE/trails"; mkdir -p "$T"
WORK=$(mktemp -d); trap 'rm -rf "$WORK"' EXIT

run() {           # run <dir-tag> <model> <outfile> <pan-args...>
  d="$WORK/$1"; shift; m="$1"; shift; out="$1"; shift
  mkdir -p "$d"; cp "$M/$m" "$d/"; cd "$d"
  spin -a "$m" >/dev/null 2>&1
  case " $* " in *" -DSAFETY "*) CF="-DSAFETY -DNOREDUCE"; set -- $(echo "$@" | sed 's/-DSAFETY//');;
                 *) CF="-DNOREDUCE";; esac
  gcc -O2 $CF -o pan pan.c
  { echo "### ./pan $* -m100000 -w24   (compiled $CF)"; ./pan "$@" -m100000 -w24 2>&1; } > "$L/$out"
}

# 1-2  the untouched fixture: deadlock / assertion check
run orig ab-original.pml 01-original-safety.log            -DSAFETY
run orig ab-original.pml 02-original-safety-noreduce.log   -DSAFETY

# 3-8  LTL on the untouched fixture, without and with weak fairness
i=3
for N in Q1_nothing_stranded Q2_sends_forever Q3_one_in_flight; do
  run oltl ab-original-ltl.pml "$(printf %02d $i)-original-$N-nofair.log"   -a    -N "$N"; i=$((i+1))
  run oltl ab-original-ltl.pml "$(printf %02d $i)-original-$N-weakfair.log" -a -f -N "$N"; i=$((i+1))
done

# 9-15 the ghost-marked model: "every message is delivered"
run mark ab-marked.pml 09-marked-safety.log -DSAFETY
i=10
for N in R1_every_message_delivered R2_nothing_stranded R3_one_in_flight; do
  run mark ab-marked.pml "$(printf %02d $i)-marked-$N-nofair.log"   -a    -N "$N"; i=$((i+1))
  run mark ab-marked.pml "$(printf %02d $i)-marked-$N-weakfair.log" -a -f -N "$N"; i=$((i+1))
done

# 16-18 negative controls -- these are EXPECTED to report errors: 1
run nv    ab-marked-nonvacuity.pml 16-control-nonvacuity.log    -a -N N1_never_delivered
cd "$WORK/nv";    spin -t -p -g ab-marked-nonvacuity.pml > "$T/16-control-nonvacuity.trail.txt" 2>&1 || true

mkdir -p "$WORK/lsy"; grep -v '^ltl ' "$M/ab-marked-lossy.pml" > "$M/.ab-lossy-noltl.pml"
cp "$M/.ab-lossy-noltl.pml" "$WORK/lsy/ab-lossy-noltl.pml"; rm -f "$M/.ab-lossy-noltl.pml"
cd "$WORK/lsy"; spin -a ab-lossy-noltl.pml >/dev/null 2>&1
gcc -O2 -DSAFETY -DNOREDUCE -o pan pan.c
{ echo "### lossy control: deadlock check, no never claim"; ./pan -m100000 -w24 2>&1; } > "$L/17-control-lossy-safety.log"
spin -t -p ab-lossy-noltl.pml 2>&1 | tail -25 > "$T/17-control-lossy-deadlock.trail.txt" || true

run lossy ab-marked-lossy.pml 18-control-lossy-R1.log -a -f -N R1_every_message_delivered_LOSSY
cd "$WORK/lossy"; spin -t -p -g ab-marked-lossy.pml > "$T/18-control-lossy-R1.trail.txt" 2>&1 || true

echo; echo "=== summary (expected: errors: 0 for 01-15, errors: 1 for 16-18) ==="
for f in "$L"/*.log; do printf '%-56s %s\n' "$(basename "$f")" "$(grep -m1 'errors:' "$f")"; done
