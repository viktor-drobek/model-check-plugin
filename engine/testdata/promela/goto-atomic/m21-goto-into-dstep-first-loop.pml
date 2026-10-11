byte n; byte m;
active proctype A() { goto L; d_step { L: do :: n == 0 -> n = 1 :: else -> break od } }
active proctype B() { m = 1; m = 2 }
