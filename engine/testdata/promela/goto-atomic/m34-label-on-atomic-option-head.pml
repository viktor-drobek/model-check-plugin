byte n; byte m;
active proctype A() {
  do
  :: L: atomic { if :: n == 0 -> n = 1 :: else -> n = 2 fi; if :: n == 1 -> goto L :: else fi }
  :: n == 1 -> assert(0)
  od
}
active proctype B() { m = 1; m = 2 }
