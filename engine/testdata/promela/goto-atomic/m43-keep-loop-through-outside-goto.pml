byte n;
active proctype A() {
  goto START;
OUT: goto L;
START: atomic {
  L: do
     :: n == 0 -> n = 1; goto OUT
     :: n == 1 -> n = 0; break
     od
  }
}
active proctype B() { assert(n != 1) }
