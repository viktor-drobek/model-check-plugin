byte n; byte m;
active proctype A() {
  atomic { n = 1; goto L };
  n = 5;
  atomic { skip; L: goto M };
  atomic { M: do :: n == 1 -> n = 2 :: else -> break od }
}
active proctype B() { m = 1; assert(n != 1) }
