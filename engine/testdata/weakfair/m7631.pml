/* wf mode=ltl formula=[](p -> <>q2) */
bit a, b;
byte c;
chan rv = [0] of {bit};
#define p (b == 0)
#define q2 (b == 1)
active proctype P0() provided (a != 0)
{
	do
	:: atomic { a = 1 - a; atomic { b = 1 - b }; skip }
	od
}
active proctype P1()
{
	do
	:: skip; if :: a == 1 -> b = 1 :: else -> b = 1 - b fi
	:: atomic { a == 0 }
	:: atomic { c == 0 }
	od
}
active proctype P2()
{
	do
	:: atomic { c = 0; c = 0 }
	od
}
