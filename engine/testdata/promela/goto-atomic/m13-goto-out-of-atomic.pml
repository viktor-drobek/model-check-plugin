byte n; byte m;
active proctype A(){
  atomic { n = 1; goto L; n = 9 };
  n = 2;
L: n = 3
}
active proctype B(){ m = 1; m = 2 }
