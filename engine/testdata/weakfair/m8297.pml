/* wf mode=ltl formula=[]<>p -> []<>q2 */
bit a, b;
#define p (b != 1)
#define q2 (b == 1)
active proctype P0()
{
	do
	:: skip; if :: a == 0 -> a == 1; b == 1 :: else -> a = 1 - a; skip; b = 1 - b; a = 1 - a fi
	od
}
