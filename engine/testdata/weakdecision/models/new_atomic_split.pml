/* NEW probe 2: P loops forever in an atomic block of two statements; Q (y = 1) is starved while P is inside. */
bit x, y;
active proctype P() { do :: atomic { x = 1 - x; x = 1 - x } od }
active proctype Q() { y = 1 }
never { accept_c: do :: (y == 0) od }
