/* wf mode=acc */
bit a, b;
active proctype P0() provided (a == 1)
{
	do
	:: b = 1 - b; b = 1 - b; a = 0; b = 1
	od
}
active proctype P1()
{
	do
	:: skip; accept_1_0: atomic { b = 1 - b; a = 1; a == 1; a = 1 - a }
	:: b = 1
	od
}
