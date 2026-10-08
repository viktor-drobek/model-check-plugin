/* wf mode=prog */
bit a, b;
byte d;
chan q = [1] of {bit};
active proctype P0()
{
	do
	:: skip; progress_0_0: atomic { d != 2; skip; d = 0 }
	od
}
active proctype P1()
{
	do
	:: skip; progress_1_0: skip; if :: b != 1 -> b = 1 - b; b == 0; b == 0; d = (d + 1) % 3 :: else -> b == 1; d = 0 fi
	:: d = (d + 1) % 3
	od
}
active proctype P2()
{
	do
	:: atomic { q!1; d = 0; d = 2; d = (d + 1) % 3 }
	:: d = 2; skip; a = 1 - a; d = (d + 1) % 3
	:: skip; progress_2_2: atomic { a = 1 - a; b == 0; d = (d + 1) % 3; a = 1 - a }
	od
}
