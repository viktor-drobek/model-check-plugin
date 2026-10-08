/* D3 with atomic: P's step is the single atomic { timeout; a = 1 - a }. */
bit a; bit b;
active proctype P() { do :: atomic { timeout -> a = 1 - a } od }
active proctype Q() { timeout -> b = 1 }
never { accept_c: do :: (b == 0) od }
