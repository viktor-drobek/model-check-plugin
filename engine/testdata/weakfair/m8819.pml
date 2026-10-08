/* wf mode=prog */
bit a, b;
byte c;
chan q = [1] of {bit};
active proctype P0() provided (a == 1)
{
	do
	:: skip; progress_0_0: atomic { a = 0; skip; a = 1 - a; b = 0 }
	od
}
active proctype P1()
{
	do
	:: atomic { b == 0; q!1; b = 1 }
	:: atomic { b = 0 }
	:: skip; progress_1_2: b == 0; c = 0
	od
}
