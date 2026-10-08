/* wf mode=claim */
bit a, b;
byte c;
chan q = [1] of {bit};
active proctype P0()
{
	do
	:: atomic { b = 1 - b; a == 1; a = 1 - a }
	:: atomic { q?b; atomic { c = 2; b == 1 }; c = 0; skip; c = (c + 1) % 3 }
	:: b = 0; c = 0; a = 1 - a; q!1
	od
}
active proctype P1()
{
	do
	:: atomic { b = 1 - b; b != 0; a = 1; a = 1 }
	od
}
never {
S0:
	if
	:: (a == 0 && c == 0) -> goto accept_S2
	fi;
accept_S1:
	if
	:: (c > 1) -> goto S0
	:: (true) -> goto accept_S2
	:: (true) -> goto accept_S1
	fi;
accept_S2:
	if
	:: (b == 0) -> goto S0
	:: (a == 0) -> goto S0
	fi;
}
