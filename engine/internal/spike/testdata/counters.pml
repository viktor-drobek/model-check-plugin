/* Promela version of the spike's synthetic "counters" model (plan 14 §9,
 * Spike). Reachable states: K^N. Used only as the pan oracle for throughput.
 * Build: spin -a counters.pml && gcc -O2 -DNOREDUCE -o pan pan.c && ./pan -m2500000 -w26
 * (-o1 -o2 -o3 gives the same counts: there is nothing to merge). */
#ifndef K
#define K 10
#endif
#ifndef N
#define N 5
#endif
active [N] proctype P() { byte c; do :: c = (c + 1) % K od }
