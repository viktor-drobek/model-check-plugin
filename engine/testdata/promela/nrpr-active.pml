/* _nr_pr in a model without run (G1). The older process waits until it is
 * the only one left; the younger one just ends. In SPIN a process that has
 * reached its end leaves the process table, so _nr_pr falls from 2 to 1 and
 * the older process proceeds: pan reports no error. The model has no run, so
 * nothing but the `_nr_pr` read asks for the process table.
 */
active proctype A() { (_nr_pr == 1) }
active proctype B() { skip }
