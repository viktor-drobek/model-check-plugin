/* wf mode=ltl formula=<>[]p */
bit a, b;
byte c;
#define p (b == 0)
#define q2 (a == 1)
active proctype P0()
{
	do
	:: skip; if :: b == 1 -> a != 0 :: else -> c = (c + 1) % 3; b = 1 - b; c = (c + 1) % 3; c = (c + 1) % 3 fi
	:: a == 1; b = 1 - b; a = 1 - a
	od
}
