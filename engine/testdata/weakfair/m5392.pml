/* wf mode=prog */
bit a, b;
chan q = [1] of {bit};
active proctype P0() provided (a == 1)
{
	do
	:: skip; progress_0_0: atomic { q?a }
	od
}
active proctype P1()
{
	do
	:: atomic { q!1; b != 0; b != 1; b = 1 }
	od
}
