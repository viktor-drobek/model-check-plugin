/* One atomic sequence of two steps, then a failing assert. The sequential
   breadth-first search counts both steps of the sequence in the depth of the state
   after it (depth 2); the parallel search counts the hop from the initial
   state to that stored state once (layer 1). features/g8-parallel.feature pins
   what --budget-depth 1 does with it. */
byte x;

active proctype P() {
	atomic { x = 1; x = 2 };
	assert(false)
}
