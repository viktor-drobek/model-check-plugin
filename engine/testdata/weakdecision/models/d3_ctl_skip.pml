/* D3 CONTROL: `timeout -> skip` is two statements (skip is a pc of its own), so the same holds. */
bit b;
active proctype P() { do :: timeout -> skip od }
active proctype Q() { timeout -> b = 1 }
never { accept_c: do :: (b == 0) od }
