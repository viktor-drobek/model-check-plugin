/* wf mode=claim */
bit a, b;
byte c;
chan q = [1] of {bit};
active proctype P0()
{
	do
	:: b = 1 - b; c = (c + 1) % 3; c = 0; q?b
	:: atomic { c = (c + 1) % 3; skip; a == 0 }
	od
}
never {
accept_S0:
	if
	:: (b == 0 && b == 0) -> goto S1
	fi;
S1:
	if
	:: (true) -> goto S1
	:: (a == 0) -> goto S1
	:: (b != 0) -> goto accept_S2
	fi;
accept_S2:
	if
	:: (true) -> goto accept_S0
	:: (b != 0 && c != 0) -> goto accept_S0
	fi;
}
