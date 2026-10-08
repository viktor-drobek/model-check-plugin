bit a, b;
byte d;
active proctype P0()
{
	do
	:: a = 1 - a; d == 2; a != 1
	:: b = 1; d = (d + 1) % 3
	od
}
active proctype P1()
{
	skip; accept_1_0: atomic { d = (d + 1) % 3 }
}
active proctype P2()
{
	do
	:: d != 1; a = 1 - a
	od
}
active proctype P3()
{
	d = (d + 1) % 3; d != 0; a == 0
}
