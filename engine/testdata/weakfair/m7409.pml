/* wf mode=claim */
bit a, b;
byte c;
active proctype P0()
{
	do
	:: atomic { c = (c + 1) % 3; atomic { b = 1 - b; b == 0; b = 1 - b }; b == 1; c = 0 }
	od
}
never {
accept_S0:
	if
	:: (a == 1) -> goto accept_S1
	:: (a == 0) -> goto accept_S1
	:: (b != 0) -> goto accept_S1
	fi;
accept_S1:
	if
	:: (a == 1 && a == 1) -> goto accept_S1
	:: (a == 1) -> goto accept_S0
	:: (b == 0) -> goto accept_S0
	fi;
}
