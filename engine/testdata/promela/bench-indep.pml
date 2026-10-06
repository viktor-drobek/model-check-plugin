/* Benchmark fixture (performance plan, step 1): N processes, each with a
   private counter 0..K. The interleavings are independent, so the reachable
   graph is the product of the processes' local graphs - the worst case for a
   search that does not reduce them, and a stress test of the visited set.
   mcd check -promela bench-indep.pml -D N=5 -D K=4 -sweep   (579195 states) */
#ifndef N
#define N 5
#endif
#ifndef K
#define K 4
#endif
byte c[N];
active [N] proctype P() {
  byte i = 0;
  do
  :: i < K -> i++; c[_pid] = i
  :: else -> break
  od
}
