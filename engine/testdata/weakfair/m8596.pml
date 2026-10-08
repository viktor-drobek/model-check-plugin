/* wf mode=ltl formula=<>p */
bit a, b;
byte d;
chan q = [1] of {bit};
#define p (a == 1)
#define q2 (b != 1)
active proctype P0() provided (a == 1)
{
	do
	:: d = (d + 1) % 3; b = 1; d = 2; b = 1 - b
	od
}
active proctype P1()
{
	do
	:: atomic { b = 1 - b; skip; if :: a != 0 -> a == 0; a = 0 :: else -> a = 0; a = 1 - a; a = 1 - a fi; skip }
	:: a = 1 - a; a = 1 - a
	od
}
