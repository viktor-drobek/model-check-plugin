/* wf mode=acc */
bit a, b;
byte c;
chan q = [1] of {bit};
active proctype P0() provided (a == 0)
{
	do
	:: c = 1; skip; a = 1 - a; a == 1
	od
}
active proctype P1()
{
	do
	:: skip; accept_1_0: d_step { c = 0; c = (c + 1) % 3; c = 0 }
	:: atomic { c = 2; b = 0; q?b }
	od
}
active proctype P2()
{
	do
	:: d_step { c = 0 }
	:: d_step { a = 1; b = 1 - b }
	od
}
