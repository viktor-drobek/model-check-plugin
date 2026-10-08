/* wf mode=prog */
bit a, b;
byte c;
chan rv = [0] of {bit};
active proctype P0()
{
	do
	:: skip; progress_0_0: atomic { rv?a; rv?b; a == 0; rv?a }
	:: skip; progress_0_1: skip; if :: b == 1 -> b = 1; b = 1 - b; a = 1 - a; skip :: else -> a = 0; skip; c = (c + 1) % 3; c == 1 fi
	:: skip; if :: a != 0 -> c == 2; a = 1 - a :: else -> a = 1 - a; b = 1 - b; rv!1 fi
	od
}
