/* wf mode=claim */
bit a, b;
byte c;
chan q = [1] of {bit};
chan rv = [0] of {bit};
active proctype P0() provided (b == 1)
{
	do
	:: b = 1; a = 0; c = (c + 1) % 3
	od
}
active proctype P1()
{
	do
	:: atomic { q!1 }
	:: a != 1; rv?a; a = 1 - a; a = 0
	od
}
active proctype P2()
{
	do
	:: atomic { a = 1 }
	:: atomic { a = 1 - a; atomic { q?a; c = 0 }; c == 2 }
	:: c = 0; b = 1; a = 1 - a; b == 0
	od
}
never {
S0:
	if
	:: (b == 0) -> goto accept_S1
	:: (c == 2 && c == 1) -> goto accept_S1
	fi;
accept_S1:
	if
	:: (a == 1) -> goto S0
	fi;
}
