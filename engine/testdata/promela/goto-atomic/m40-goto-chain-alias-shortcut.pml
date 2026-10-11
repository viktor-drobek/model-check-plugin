byte n;
active proctype A() {
  atomic { n = 1; goto L };
K: goto M;
  atomic { skip; L: goto K };
  atomic { skip; M: n = 2; n = 3 }
}
active proctype B() { assert(n != 1) }
