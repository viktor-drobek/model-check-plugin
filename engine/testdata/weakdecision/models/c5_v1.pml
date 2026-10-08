bit a; byte d;
active proctype P1() { accept_1: atomic { d = 1 } }
active proctype P3() { d == 1; a == 1 }
