/* D3 sanity: both processes loop on timeout, each moves on the cycle: weakly fair, accepting. */
bit b;
active proctype P() { do :: timeout od }
active proctype Q() { do :: timeout od }
never { accept_c: do :: (b == 0) od }
