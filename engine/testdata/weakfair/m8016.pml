/* wf mode=prog */
bit a, b;
byte c;
chan q = [1] of {bit};
active proctype P0() provided (c == 1)
{
	do
	:: skip; progress_0_0: b = 1 - b; b = 1 - b; b = 1 - b; b = 1 - b
	od
}
active proctype P1()
{
	do
	:: atomic { q!1; a = 1 - a; b == 0; c = 0; b = 1 - b }
	od
}
active proctype P2()
{
	do
	:: atomic { a = 1; skip; c = 0; c = 0 }
	:: atomic { a = 1 - a; b = 0; c = 0 }
	:: atomic { c > 2; c = 0; skip; c = 1; skip }
	od
}
