byte a;

active proctype p1() { do :: a < 255 -> a++ :: a > 0 -> a-- od }
