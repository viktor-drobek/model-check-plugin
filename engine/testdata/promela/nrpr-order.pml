/* Three active processes: only the youngest live process may leave the
 * table, so C ends first, then B, and only then is the oldest process alone
 * and _nr_pr == 1 true (pan, spin -a -o1 -o2 -o3: no error, 10 states stored).
 */
byte done;
active proctype A() { (_nr_pr == 1); done = 1 }
active proctype B() { skip }
active proctype C() { skip }
