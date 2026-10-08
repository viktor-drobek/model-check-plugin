/* wf mode=prog */
bit a, b;
byte c;
active proctype P0() provided (a == 1)
{
	do
	:: atomic { b = 1 - b; d_step { b = 1 - b }; c > 1; c = 2 }
	:: skip; progress_0_1: atomic { c = (c + 1) % 3; atomic { a = 1 - a; a = 0 }; a != 0; c = (c + 1) % 3; a == 0 }
	od
}
active proctype P1()
{
	do
	:: d_step { b = 0 }
	:: atomic { c != 2; a = 1 - a; b = 1 - b; skip; b = 1; c = (c + 1) % 3; a = 0; b = 1 }
	od
}
