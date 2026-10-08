/* The youngest process waits for _nr_pr == 1, which cannot happen: the older
 * processes may leave the table only after it has left, and it is blocked.
 * pan: invalid end state, 4 states stored. The process table must not let an
 * older process leave early.
 */
byte n;
active proctype A() { skip }
active proctype B() { skip }
active proctype C() { (_nr_pr == 1); assert(n == 0); n = 1 }
