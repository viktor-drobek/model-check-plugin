/* A model without run that never reads _nr_pr: the process table is not
 * carried, the -end- guards are the conjunction over the younger processes
 * (G1), and the report must not depend on how _nr_pr is implemented.
 * pan (spin -a -o1 -o2 -o3, as tools/pandiff runs it): no error, 10 states stored.
 */
byte x;
active proctype A() { x = 1 }
active proctype B() { x = 2 }
