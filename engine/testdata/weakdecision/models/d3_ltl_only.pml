bit b;
#define q (b == 1)
active proctype P() { do :: timeout od }
active proctype Q() { timeout -> b = 1 }
