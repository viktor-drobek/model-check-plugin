/* wf mode=prog */
bit a, b;
byte c;
chan q = [1] of {bit};
active proctype P0()
{
	do
	:: atomic { c = (c + 1) % 3; atomic { a == 1; b = 1; a = 1; b = 0 }; b = 1 - b; a = 1 - a; b = 0 }
	od
}
