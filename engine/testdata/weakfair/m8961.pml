/* wf mode=prog */
bit a, b;
byte c;
byte d;
active proctype P0() provided (d == 2)
{
	do
	:: atomic { d = 0; b = 1 - b; a == 0 }
	:: skip; progress_0_1: skip; if :: a == 1 -> b = 0; b == 1; d = 0 :: else -> c = 0; c = (c + 1) % 3; a = 1 - a; a = 1 - a fi
	:: skip; if :: a == 0 -> d = (d + 1) % 3; d = 0; skip :: else -> skip fi
	od
}
active proctype P1()
{
	do
	:: atomic { b = 1; b = 1; a = 1 - a }
	od
}
