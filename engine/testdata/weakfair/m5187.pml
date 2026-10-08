/* wf mode=prog */
bit a, b;
active proctype P0() provided (b == 0)
{
	do
	:: skip; if :: a != 1 -> a == 0; a == 1 :: else -> a != 0 fi
	od
}
active proctype P1()
{
	do
	:: atomic { a = 1 - a; b = 1 - b; a = 1 - a; a == 1 }
	od
}
