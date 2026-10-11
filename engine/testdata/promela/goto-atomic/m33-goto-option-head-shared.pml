byte n; byte m;
active proctype A() {
  goto L;
  do
  :: L: n = 1; break
  :: n == 0 -> assert(0)
  od
}
active proctype B() { m = 1; m = 2 }
