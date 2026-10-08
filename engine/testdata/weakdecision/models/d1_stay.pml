/* D1 control: blocked system, claim stays on an accepting location. */
bit a;
active proctype P() { a == 1 }
never { accept_S0: do :: (true) od }
