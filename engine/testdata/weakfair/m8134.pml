/* wf mode=ltl formula=<>[]p */
bit a, b;
byte c;
chan q = [1] of {bit};
#define p (c > 0)
#define q2 (b == 1)
active proctype P0()
{
	do
	:: a = 1; b = 1 - b; b == 1; a = 1 - a
	od
}
active proctype P1()
{
	do
	:: c = 0; skip; a = 1 - a; b == 1
	od
}
active proctype P2()
{
	do
	:: atomic { b == 1 }
	:: atomic { b = 1 - b; q?a; q!1; c = 1 }
	od
}
