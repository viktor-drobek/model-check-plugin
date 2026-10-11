byte n; byte m;
active proctype A() {
  if :: goto L :: n = 9 fi;
  atomic { n = 1; L: goto M; M: n = 2; n = 3 }
}
active proctype B() { m = 1; assert(n != 9) }
