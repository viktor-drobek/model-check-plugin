byte n;
active proctype A() {
  if
  :: atomic { skip; L: goto OUT }
  :: atomic { n = 1; goto L }
  fi;
OUT: n = 2
}
