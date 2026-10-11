byte n; byte m;
active proctype A() {
  if
  :: goto L
  :: skip
  fi;
  atomic { n = 1; L: n = 2; n = 3 }
}
active proctype B() { m = 1; m = 2 }
