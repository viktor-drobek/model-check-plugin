byte n; byte m;
active proctype A() {
  atomic { do
    :: n == 0 -> n = 3; L: break
    :: n == 2 -> n = 1; goto L
    od };
  n = 4
}
active proctype B() { n = 2; assert(n != 1) }
