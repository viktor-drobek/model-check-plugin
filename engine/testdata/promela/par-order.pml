/* Parallel-search fixture (performance plan 5, §3.7): two branches. A writes x
   twice and asserts x != 2 (violated); B overflows a byte. The model has an
   ending event (the overflow), so which of the properties were decided before
   the run ends depends on the order the search meets the branches in: the
   depth-first search says the assert is violated, the sequential breadth-first
   search says invalid-model. The parallel search has its own order; what it
   promises is that no property is `verified`, that every verdict carries a run
   that replays, and that the answer is the same for every worker count. */
byte x;
byte b = 255;

active proctype A() { x = 1; x = 2; assert(x != 2) }
active proctype B() { b = b + 1 }
