/* wf mode=ltl formula=[]<>p */
bit a, b;
byte c;
#define p (a == 1)
#define q2 (a == 1)
active proctype P0() provided (b == 1)
{
	do
	:: atomic { skip; atomic { a = 1 - a }; skip }
	:: atomic { c = 0 }
	:: c = 1; b = 1 - b; a = 0
	od
}
active proctype P1()
{
	do
	:: atomic { a = 0 }
	:: atomic { a == 1; atomic { c = (c + 1) % 3; a == 1 }; skip }
	od
}
