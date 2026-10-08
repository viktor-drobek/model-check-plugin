/* wf mode=prog */
bit a, b;
active proctype P0()
{
	do
	:: atomic { b == 0; a == 1; b = 0; b = 0; b == 1; a = 1; b = 1 }
	od
}
