/* wf mode=prog */
bit a, b;
byte c;
active proctype P0()
{
	do
	:: skip; if :: b == 0 -> c == 2 :: else -> b = 0 fi
	:: skip; b == 1; b = 1
	od
}
