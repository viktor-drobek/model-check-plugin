/* One d_step block of two statements, then a failing assert. Unlike an atomic
   sequence, a d_step chain is ONE move (fire runs the whole chain), so the
   default search, the sequential breadth-first search and the parallel search
   all count it as one step: depth 3 and 4 states in each, and --budget-depth
   cuts at the same place in each. features/g8-parallel.feature pins that, next
   to par-atomic-depth.pml, where the searches do differ. */
byte x;

active proctype P() {
	d_step { x = 1; x = 2 };
	assert(false)
}
