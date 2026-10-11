byte n;
active proctype A() {
  do
  :: atomic { n = 1; goto L }
  :: atomic { L: do
       :: n == 1 -> n = 2
       :: n == 2 -> n = 3
       :: else -> break
     od }
  :: n == 2 -> assert(0)
  od
}
