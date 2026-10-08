/* wf mode=acc */
bit a, b;
chan q = [1] of {bit};
active proctype P0() provided (b == 0)
{
	do
	:: skip; accept_0_0: skip; if :: a == 0 -> a = 1 - a :: else -> skip; b = 1 - b fi
	:: atomic { b = 1 }
	od
}
active proctype P1()
{
	do
	:: skip; accept_1_0: b == 1
	od
}
