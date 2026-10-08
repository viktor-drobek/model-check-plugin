/* wf mode=prog */
bit a, b;
byte c;
chan q = [1] of {bit};
active proctype P0()
{
	do
	:: skip; if :: a != 1 -> b = 1; b = 1; a = 0; a = 1 :: else -> a = 0; b == 1; c > 2; b = 0 fi
	:: skip; progress_0_1: atomic { b = 0; q?a }
	:: atomic { skip; d_step { c = 2; a = 1 - a }; skip }
	od
}
active proctype P1()
{
	do
	:: skip; progress_1_0: q?b; c = (c + 1) % 3; b != 0
	:: skip; progress_1_1: b = 1; a = 1
	od
}
active proctype P2()
{
	do
	:: skip; progress_2_0: d_step { a = 1; c = 2 }
	:: skip; if :: a == 0 -> skip; a = 0 :: else -> c = 2 fi
	:: atomic { b == 1; c = 0; b == 0; c = 0 }
	od
}
