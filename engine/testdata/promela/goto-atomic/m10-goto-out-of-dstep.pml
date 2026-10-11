byte n;
active proctype P() {
  d_step { n = 1; goto L };
  n = 2;
L: n = 3
}
