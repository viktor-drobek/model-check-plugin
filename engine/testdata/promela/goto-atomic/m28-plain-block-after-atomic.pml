byte n; byte m;
active proctype A() {
  atomic { L: do :: n < 3 -> n++ :: else -> break od;
     if :: n == 3 -> n = 4; goto L :: else fi };
  { m = 1 }
}
