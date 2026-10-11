byte n; byte m;
active proctype A() {
  goto L;
  d_step { n = 1; L: n = 2; n = 3 }
}
active proctype B() { m = 1; m = 2 }
