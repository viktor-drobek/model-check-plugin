/* wf mode=prog */
bit a, b;
byte d;
active proctype P0()
{
	do
	:: skip; progress_0_0: atomic { b != 1; skip; a = 0; b == 0 }
	:: skip; progress_0_1: b == 1; a != 1; d == 0; d = 0
	od
}
active proctype P1() provided (d > 0)
{
	do
	:: d = (d + 1) % 3; b = 1 - b
	:: atomic { b = 1; b = 1; b = 1 }
	:: skip; progress_1_2: b = 1 - b; d > 2; b = 1 - b; b = 1
	od
}
