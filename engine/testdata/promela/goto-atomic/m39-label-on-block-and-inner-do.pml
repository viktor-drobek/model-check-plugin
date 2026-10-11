byte n; byte m;
active proctype A() {
  atomic { n = 1; goto K };
  n = 5;
K: atomic { L: do :: n == 1 -> n = 2 :: else -> break od }
}
active proctype B() { m = 1; assert(n != 1) }
