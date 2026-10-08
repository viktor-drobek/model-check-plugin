/* wf mode=prog */
bit a, b;
byte c;
chan rv = [0] of {bit};
active proctype P0()
{
	do
	:: b = 1 - b; c == 1; b = 1 - b
	:: skip; progress_0_1: c == 2; a = 1 - a
	od
}
