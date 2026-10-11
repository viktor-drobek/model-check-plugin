byte n;
active proctype A() {
  n = 7;
  goto L;
  atomic {
    skip;
L:  goto M;
    skip;
M:  n = 2;
    n = 3
  }
}
active proctype B() { assert(n != 7) }
