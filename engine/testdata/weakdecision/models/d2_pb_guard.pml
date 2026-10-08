/* D2 minimal pair, guard form: the same blocking written as a guard in the body. */
byte x; bit y; bit b;
active proctype P0() { do :: b == 0 -> x = 0 od }
active proctype P1() { b = 1; do :: accept_p1: y = 1 - y od }
