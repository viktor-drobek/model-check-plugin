/* D3 CONTROL: P1 = timeout, then a separate statement; the state between them is not a timeout state, so P2 is
   disabled there: a weakly fair accepting run exists and every tool agrees. */
bit a; bit b;
active proctype P1() { do :: timeout -> a = 1 - a od }
active proctype P2() { timeout -> b = 1 }
never { accept_c: do :: (b == 0) od }
