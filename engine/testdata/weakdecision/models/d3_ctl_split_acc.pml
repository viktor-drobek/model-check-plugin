/* D3 with an accept label instead of a claim. */
bit a; bit b;
active proctype P1() { do :: accept_p1: timeout -> a = 1 - a od }
active proctype P2() { timeout -> b = 1 }
