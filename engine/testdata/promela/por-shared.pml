/* POR fixture: two processes race on a shared counter (the lost update), so
   `assert(x == 2)` is violated. The step t = t + 1 touches nothing shared; the
   reads and writes of x are what the race needs, and a sound reduction must
   keep every interleaving of those. */
byte x = 0;
byte done = 0;

active [2] proctype P() {
  byte t;
  t = x;
  t = t + 1;
  x = t;
  done++
}

active proctype Check() {
  done == 2 -> assert(x == 2)
}
