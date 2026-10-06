/* POR fixture: P counts to 20 on its own while Q writes x and then asserts it
   is still 0. With a small depth budget the full search finds the violation
   at depth 2, because it tries Q's step from the initial state; the reduced
   search expands P alone until it is done, so Q's step is first tried at
   depth 21, past the budget, and the answer is inconclusive. It is never
   `verified`: a truncated search is never complete. */
byte x = 0;

active proctype P() {
  byte i = 0;
  do
  :: i < 20 -> i++
  :: else -> break
  od
}

active proctype Q() {
  x = 1;
  assert(x == 0)
}
