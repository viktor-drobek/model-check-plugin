/* D2 minimal pair, provided form (record section 6, class 2): P0 is disabled once P1 sets b. */
byte x; bit y; bit b;
active proctype P0() provided (b == 0) { do :: x = 0 od }
active proctype P1() { b = 1; do :: accept_p1: y = 1 - y od }
