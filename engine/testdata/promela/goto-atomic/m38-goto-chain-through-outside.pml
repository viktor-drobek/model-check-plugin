byte n;
active proctype A() {
  atomic { n = 1; goto L };
L: goto M;
  atomic { skip; M: n = 2; n = 3 }
}
active proctype B() { assert(n != 1) }
