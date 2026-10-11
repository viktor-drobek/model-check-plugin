byte n;
active proctype P() {
  atomic {
    if
    :: goto L
    :: L: do
         :: n == 0 -> n = 1
         :: n == 1 -> n = 2
         :: else -> break
       od
    :: n == 1 -> assert(0)
    fi
  }
}
