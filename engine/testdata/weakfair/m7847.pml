/* wf mode=claim */
bit a, b;
byte d;
chan rv = [0] of {bit};
active proctype P0()
{
	do
	:: d = 0; a == 0; rv!1
	:: b == 1; a != 0; d = 0
	:: atomic { skip; atomic { d = (d + 1) % 3; b = 1 - b; rv!1; a = 1 - a }; skip; b = 1 - b }
	od
}
active proctype P1()
{
	do
	:: skip; rv!1; d == 1; d != 1
	od
}
never {
S0:
	if
	:: (d == 1) -> goto S0
	:: (d == 1) -> goto accept_S2
	:: (a == 0) -> goto S0
	fi;
S1:
	if
	:: (true) -> goto S1
	:: (b == 0 && a == 1) -> goto S0
	fi;
accept_S2:
	if
	:: (b == 1) -> goto S0
	:: (b == 0 && a == 0) -> goto S1
	fi;
}
