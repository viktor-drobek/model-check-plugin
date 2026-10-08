/* NEW probe: P holds the atomic lock for ever (an atomic loop that never blocks), Q (y = 1) can never be scheduled.
   Claim accepting while y == 0. Weak fairness: Q is not executable while P is inside the atomic. */
bit x, y;
active proctype P() { atomic { do :: x = 1 - x od } }
active proctype Q() { y = 1 }
never { accept_c: do :: (y == 0) od }
