byte n; byte m;
active proctype A(){
  do
  :: atomic { n = 1; if :: goto L :: n = 5 fi }
  :: atomic { n = 2; L: n = 3; n = 4 }; break
  od
}
active proctype B(){ m = 1; m = 2 }
