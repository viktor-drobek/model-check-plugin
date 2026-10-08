/* wf mode=ltl formula=[]<>p -> []<>q2 */
bit a, b;
byte c;
byte d;
chan q = [1] of {bit};
#define p (a != 1)
#define q2 (d == 2)
active proctype P0()
{
	do
	:: skip; if :: b == 1 -> a = 0; b = 1 :: else -> q?a; d = 1; a = 1 fi
	od
}
