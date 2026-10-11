byte n; byte m;
active proctype A() { d_step { n = 1; if :: n == 1 -> goto L :: else fi; n = 5; L: n = 2 } }
active proctype B() { m = 1; assert(n != 1) }
