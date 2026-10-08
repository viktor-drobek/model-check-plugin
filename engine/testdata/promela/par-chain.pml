/* Parallel-search fixture (performance plan 5): one process counting to N.
   The reachable graph is a single path, one state per breadth-first layer:
   the narrowest shape there is, the worst case for any layer-by-layer search.
   mcd check --promela par-chain.pml -D N=1000000 --sweep --unlimited
   (1000002 states, 1000001 layers). */
#ifndef N
#define N 1000
#endif
int c;
active proctype P() {
  do
  :: c < N -> c++
  :: else -> break
  od
}
