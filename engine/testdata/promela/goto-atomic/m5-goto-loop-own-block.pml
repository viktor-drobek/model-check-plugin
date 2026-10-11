byte n; byte m;
active proctype A() {
  goto L;
  atomic { L: do
       :: n == 0 -> n = 1
       :: n == 1 -> n = 2
       :: else -> break
     od }
}
active proctype B() { m = 1; m = 2 }
