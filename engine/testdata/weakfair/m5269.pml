/* wf mode=prog */
bit a, b;
byte c;
active proctype P0()
{
	do
	:: skip; progress_0_0: atomic { b != 0; c = 2; c = 0; c = 0 }
	:: skip; progress_0_1: atomic { c = (c + 1) % 3; a = 1 }
	:: skip; progress_0_2: atomic { b = 1; atomic { c = 1 }; skip }
	od
}
active proctype P1()
{
	do
	:: skip; if :: a == 1 -> c = (c + 1) % 3; c != 0; c = 0 :: else -> c = 0; a != 1 fi
	:: a = 1 - a; a = 0; skip
	:: skip; progress_1_2: atomic { skip; a = 1 - a; c > 1; c != 1 }
	od
}
