byte n; byte m;
active proctype A() { goto L; if :: L: n = 1 fi; n = 2 }
active proctype B() { m = 1; m = 2 }
