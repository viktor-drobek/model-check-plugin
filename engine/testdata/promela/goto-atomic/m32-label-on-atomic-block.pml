byte n; byte m;
active proctype A() { atomic { n = 1; goto L }; n = 5; L: atomic { n = 2; n = 3 } }
active proctype B() { m = 1; assert(n != 1) }
