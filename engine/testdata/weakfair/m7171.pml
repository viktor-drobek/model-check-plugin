/* wf mode=prog */
bit a, b;
byte c;
chan q = [1] of {bit};
active proctype P0()
{
	do
	:: atomic { b = 1 - b; b = 1; b == 0; b = 1 - b; c = 1 }
	od
}
active proctype P1()
{
	do
	:: skip; progress_1_0: atomic { a = 1; skip; if :: c > 2 -> a == 1; c = (c + 1) % 3 :: else -> a == 1 fi; b = 1 - b; c = (c + 1) % 3; a = 1 - a }
	:: atomic { skip; b = 0; q!1; a == 1 }
	:: skip; progress_1_2: b = 1 - b; b = 1 - b; c = (c + 1) % 3
	od
}
