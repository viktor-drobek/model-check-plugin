/* Benchmark fixture (performance plan, step 1): N identical processes
   contending for one lock; every permutation of the processes is a distinct
   state, so the graph has N!-fold redundancy. A second shape for the visited
   set: shared globals, atomic, wider vectors.
   mcd check -promela bench-sym.pml -D N=6 -sweep   (543076 states) */
#ifndef N
#define N 5
#endif
byte inCS = 0; bool lock = false; byte done = 0;
active [N] proctype Q() {
  byte s = 0;
  do
  :: s == 0 -> atomic { !lock -> lock = true }; s = 1
  :: s == 1 -> inCS++; s = 2
  :: s == 2 -> inCS--; s = 3
  :: s == 3 -> lock = false; s = 4
  :: s == 4 -> done++; break
  od
}
