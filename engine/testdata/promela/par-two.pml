/* Parallel-search fixture (performance plan 5): two processes, each counting
   to N. Cheap steps, two transitions per state and N+1 states per layer at
   the widest: the moderate shape, where a layer-by-layer search can win little.
   mcd check --promela par-two.pml -D N=1000 --sweep --unlimited
   (about 4 million states, 4005 layers). */
#ifndef N
#define N 100
#endif
int a, b;
active proctype A() {
  do
  :: a < N -> a++
  :: else -> break
  od
}
active proctype B() {
  do
  :: b < N -> b++
  :: else -> break
  od
}
