/* wf mode=prog */
bit a, b;
byte c;
active proctype P0() provided (b == 0)
{
	do
	:: skip; if :: b == 1 -> b == 1; skip :: else -> skip; b = 1 - b fi
	od
}
active proctype P1()
{
	do
	:: skip; if :: a == 1 -> c = (c + 1) % 3; b == 1; a = 1 - a; c = 0 :: else -> a != 1; c = 0; a == 0; a = 1 - a fi
	:: atomic { b = 0; b = 1 - b; a == 1 }
	:: atomic { b = 1; b != 0; c = 0; c = (c + 1) % 3 }
	od
}
active proctype P2()
{
	do
	:: skip; progress_2_0: b != 0; c = (c + 1) % 3; b = 1 - b
	:: skip; progress_2_1: a = 1 - a; b = 0; c = 2
	:: skip; progress_2_2: atomic { a = 1 - a; atomic { b = 1; a == 1; a = 0 }; a == 1; skip }
	od
}
