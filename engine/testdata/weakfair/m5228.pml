/* wf mode=prog */
bit a, b;
chan rv = [0] of {bit};
active proctype P0() provided (a == 0)
{
	do
	:: skip; progress_0_0: atomic { b != 0; atomic { rv?b; a = 1; a = 1; a = 1 }; b = 0 }
	:: skip; a = 1; a = 1 - a
	:: skip; progress_0_2: atomic { b = 1 - b }
	od
}
active proctype P1()
{
	do
	:: a = 0; b = 1; a = 1 - a; b != 1
	:: skip; progress_1_1: b != 0; a = 0
	:: atomic { rv!1; a = 1 - a }
	od
}
