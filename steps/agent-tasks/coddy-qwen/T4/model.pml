byte a; byte b; byte c; byte d;

active proctype p1() { do :: a < 255 -> a++ :: a > 0 -> a-- od }
active proctype p2() { do :: b < 255 -> b++ :: b > 0 -> b-- od }
active proctype p3() { do :: c < 255 -> c++ :: c > 0 -> c-- od }
active proctype p4() { do :: d < 255 -> d++ :: d > 0 -> d-- od }
