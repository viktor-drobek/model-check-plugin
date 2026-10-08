/* wf mode=claim */
bit a, b;
byte c;
active proctype P0()
{
	do
	:: atomic { a = 1 - a; b == 1; a = 1 - a }
	:: atomic { c = 0 }
	od
}
never {
accept_S0:
	if
	:: (c != 0 && c == 0) -> goto accept_S0
	:: (true) -> goto S1
	:: (b == 1) -> goto accept_S2
	fi;
S1:
	if
	:: (a == 1) -> goto accept_S0
	fi;
accept_S2:
	if
	:: (true) -> goto S1
	:: (a != 0) -> goto accept_S0
	:: else -> skip
	fi;
}
