byte x;
active proctype A() {
  atomic {
    do
    :: x == 0 -> x = 1
    :: goto L
    od;
L:  x = 0;
    skip
  }
}
active proctype B() {
  assert(x == 0)
}
