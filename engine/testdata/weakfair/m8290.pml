/* wf mode=claim */
bit a, b;
byte d;
chan q = [1] of {bit};
active proctype P0() provided (d == 1)
{
	do
	:: b = 1 - b; a == 1; d = 0
	od
}
active proctype P1()
{
	do
	:: atomic { a = 1 - a; a = 1 - a; a == 0; a == 1 }
	od
}
never {
S0:
	if
	:: (d > 0) -> goto accept_S2
	:: (true) -> goto accept_S2
	:: (true) -> goto S0
	fi;
S1:
	if
	:: (b == 1 && d == 1) -> goto S1
	:: (d > 1) -> goto S0
	:: (d != 2 && a != 0) -> goto S0
	fi;
accept_S2:
	if
	:: (d == 2) -> goto accept_S2
	:: (d == 0 && b == 0) -> goto S0
	:: (d == 0) -> goto S1
	:: else -> skip
	fi;
}
