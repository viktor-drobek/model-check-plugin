/* D3 control: P1 can always move without timeout, so timeout is never true: P2 is never enabled. */
bit a; bit b;
active proctype P1() { do :: a = 1 - a od }
active proctype P2() { timeout -> b = 1 }
never { accept_c: do :: (b == 0) od }
