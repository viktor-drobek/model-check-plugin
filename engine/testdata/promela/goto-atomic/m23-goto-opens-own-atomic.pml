byte n; byte m;
active proctype A() { n = 7; atomic { goto L }; n = 9; atomic { n = 1; L: n = 2; n = 3 } }
active proctype B() { m = 1; assert(n != 7) }
