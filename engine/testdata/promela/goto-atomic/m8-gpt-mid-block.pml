byte x;
active proctype P() {
  if
  :: goto L
  :: atomic {
       x = 1;
    L: x = 2;
       x = 3
     }
  fi
}
active proctype Q() {
  assert(x != 1)
}
