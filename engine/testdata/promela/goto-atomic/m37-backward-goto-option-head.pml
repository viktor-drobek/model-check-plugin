byte n; byte m;
active proctype A() {
  do
  :: L: n = n + 1; if :: n < 2 -> goto L :: else -> break fi
  :: n == 1 -> assert(0)
  od
}
active proctype B() { m = 1; m = 2 }
