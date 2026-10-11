byte n; byte x; byte m;
active proctype A() {
  goto L;
  skip;
  do
  :: atomic { x = 1; L: do
       :: n == 0 -> n = 1
       :: n == 1 -> n = 2
       :: else -> break
     od }
  :: n == 2 -> break
  od
}
active proctype B() { m = 1; m = 2 }
