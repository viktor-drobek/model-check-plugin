/* wf mode=prog */
bit a, b;
active proctype P0()
{
	do
	:: skip; progress_0_0: atomic { a == 1; skip; b != 1 }
	od
}
