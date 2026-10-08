/* atomic-at.pml with the guard moved to the FIRST statement of the atomic block:
   A enters the block only when y == 1 holds, so once entered the block runs
   through (x = 1; x = 2) and never blocks part-way. In the parallel search it is
   one unit of depth, where the block of atomic-at.pml, which blocks on y == 1
   after x = 1, is cut there and counts two. features/g8-parallel.feature pins the
   depth, states and transitions of both under --budget-depth 1 to 5, against
   --bfs. */
byte x, y;
active proctype A() { atomic { y == 1; x = 1; x = 2 }; x = 3 }
active proctype B() { y = 1 }
