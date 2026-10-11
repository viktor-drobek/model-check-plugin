byte n;
active proctype A() {
  if
  :: atomic { skip; L: { goto OUT } }
  :: atomic { n = 1; goto L }
  fi;
OUT: n = 2
}
active proctype B() { assert(n != 1) }
