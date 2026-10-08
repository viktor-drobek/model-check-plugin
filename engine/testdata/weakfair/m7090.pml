/* wf mode=prog */
bit a, b;
active proctype P0()
{
	do
	:: atomic { a = 0; d_step { a = 1 - a; a = 1 - a }; a = 1; b == 1; b = 0 }
	od
}
active proctype P1() provided (b == 1)
{
	do
	:: skip; progress_1_0: b = 1; a = 1 - a; b = 1 - b
	:: atomic { b = 1; a = 1; skip }
	od
}
