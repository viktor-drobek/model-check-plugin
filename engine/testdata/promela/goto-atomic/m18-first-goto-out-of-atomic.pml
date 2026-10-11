byte n; byte m;
active proctype A() { atomic { n = 1; if :: goto L :: n = 5 fi }; n = 2; L: n = 3 }
active proctype B() { m = 1; assert(n != 1) }
