byte n; byte m;
active proctype A() {
  goto L;
  if
  :: L: n = 1
  :: n == 0 -> assert(0)
  fi
}
active proctype B() { m = 1; m = 2 }
