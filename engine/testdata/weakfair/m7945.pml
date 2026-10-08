/* wf mode=prog */
bit a, b;
active proctype P0() provided (b != 0)
{
	do
	:: skip; progress_0_0: atomic { a = 1 - a; b = 1; a = 1 - a }
	:: skip; progress_0_1: b = 0; a = 1
	:: a == 1; b = 1; a = 1
	od
}
active proctype P1()
{
	do
	:: skip; progress_1_0: skip; if :: a == 0 -> b == 1 :: else -> a = 1 - a; a != 0; a = 0 fi
	:: b == 0; a = 1 - a; a = 1 - a; a = 1
	:: d_step { a = 0 }
	od
}
