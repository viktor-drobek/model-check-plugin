/* D2: P0 can never move (provided (false)); P1 loops forever at an accept label. */
bit y;
active proctype P0() provided (false) { skip }
active proctype P1() { do :: accept_p1: y = 1 - y od }
