bit b;
active proctype P() { do :: timeout -> skip od }
active proctype Q() { do :: timeout -> b = 1 od }
never { accept: if :: b == 0 -> goto accept fi }
