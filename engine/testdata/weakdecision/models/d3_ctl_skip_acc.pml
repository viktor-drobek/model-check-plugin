/* D3 with an accept label: P loops on timeout at an accept label; Q waits for timeout, never moves on the cycle. */
bit b;
active proctype P() { do :: accept_p: timeout -> skip od }
active proctype Q() { timeout -> b = 1 }
