/* D2 with a never claim instead of an accept label. */
bit y;
active proctype P0() provided (false) { skip }
active proctype P1() { do :: y = 1 - y od }
never { accept_c: do :: (true) od }
