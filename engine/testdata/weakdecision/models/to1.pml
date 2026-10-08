bit a; bit b;
active proctype P1() { do :: timeout -> a = 1 - a od }
active proctype P2() { timeout -> b = 1 }
never { accept: do :: (b == 0) od }
