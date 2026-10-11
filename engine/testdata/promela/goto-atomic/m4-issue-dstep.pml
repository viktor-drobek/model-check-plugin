byte n;
active proctype A() {
  goto L;
  do
  :: d_step { L: do
       :: n == 0 -> n = 1
       :: n == 1 -> n = 2
       :: else -> break
     od }
  :: n == 1 -> assert(0)
  od
}
