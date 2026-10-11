byte n; byte k; byte m;
active proctype A() {
  atomic { L: do
       :: n == 0 -> n = 1
       :: n == 1 -> n = 2
       :: else -> break
     od;
     skip;
     d_step { k++; k++ };
     if :: k < 4 -> n = 0; goto L :: else fi }
}
active proctype B() { m = 1; assert(n != 0 || k == 0) }
