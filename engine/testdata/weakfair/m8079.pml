/* wf mode=ltl formula=[]<>p */
bit a, b;
byte c;
byte d;
chan q = [1] of {bit};
#define p (d == 2)
#define q2 (c > 0)
active proctype P0() provided (c > 2)
{
	do
	:: atomic { b == 0; b = 1; skip }
	:: d_step { b = 1; a = 1 }
	:: atomic { a = 1 - a; b = 1 - b; a = 1 }
	od
}
active proctype P1()
{
	do
	:: atomic { a == 1; d = 2; a == 1; c = 0 }
	:: q!1
	od
}
active proctype P2()
{
	do
	:: atomic { a = 1 - a; c = (c + 1) % 3; skip }
	:: a = 1; b = 1 - b; b = 0
	:: atomic { a = 1 - a }
	od
}
