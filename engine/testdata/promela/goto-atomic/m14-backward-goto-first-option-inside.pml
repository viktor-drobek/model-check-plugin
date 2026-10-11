byte n;
active proctype P() {
  atomic { L: do
    :: n < 4 -> n++; if :: goto L fi
    :: else -> break
    od }
}
active proctype Q() { assert(n == 0 || n == 4) }
