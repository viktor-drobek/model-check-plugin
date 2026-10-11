byte n; byte m;
active proctype A() {
  do
  :: atomic { L: do
       :: n == 0 -> n = 1
       :: n == 1 -> n = 2
       :: else -> break
     od; n = 0; goto L }
  :: n == 1 -> assert(0)
  od
}
active proctype B() { m = 1; m = 2 }
