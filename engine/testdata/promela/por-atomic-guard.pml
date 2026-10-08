/* POR fixture (performance plan, step 6, atomic sequences): P's atomic
   sequence goes on into one of two branches, each guarded by x, which Q
   writes; the branch for x == 1 blocks for ever (y is never 1). The deadlock
   is reached only if Q writes first, so P must not be expanded alone before Q
   has moved: the reduction has to count the edges the sequence goes on into,
   not only its first one. */
byte x = 0;
byte y = 0;
active proctype P() { atomic { skip; if :: x == 1 -> y == 1 :: x == 0 -> skip fi } }
active proctype Q() { x = 1 }
