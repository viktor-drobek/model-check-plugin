/* D3, --progress form: P loops on the single statement timeout; Q fires on timeout, then loops at a progress label.
   The np cycle (P alone, Q enabled by timeout, never moving) is weakly unfair: no weakly fair non-progress cycle. */
bit y;
active proctype P() { do :: timeout od }
active proctype Q() { timeout -> do :: progress: y = 1 - y od }
