/* wf mode=ltl formula=<>[]p */
bit a, b;
chan q = [1] of {bit};
#define p (b == 1)
#define q2 (b != 1)
active proctype P0()
{
	do
	:: atomic { skip; a != 0; skip }
	od
}
