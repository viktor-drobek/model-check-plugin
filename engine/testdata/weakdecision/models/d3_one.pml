/* D3 minimal: P's step is the single statement `timeout`. Every state of the cycle is a timeout state,
   Q is enabled there and never moves on the cycle. Claim: accepting while b == 0. */
bit b;
active proctype P() { do :: timeout od }
active proctype Q() { timeout -> b = 1 }
never { accept_c: do :: (b == 0) od }
