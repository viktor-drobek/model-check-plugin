/* D3 control: Q guarded by false (truly disabled): the np cycle of P alone is weakly fair. */
bit y;
active proctype P() { do :: timeout od }
active proctype Q() { false -> do :: progress: y = 1 - y od }
