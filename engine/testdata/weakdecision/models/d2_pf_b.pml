/* D2, other process order: the looping accept process is P0, the blocked-by-provided one is P1. */
bit y;
active proctype P0() { do :: accept_p0: y = 1 - y od }
active proctype P1() provided (false) { skip }
