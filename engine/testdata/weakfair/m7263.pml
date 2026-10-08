/* wf mode=claim */
bit a, b;
chan q = [1] of {bit};
active proctype P0()
{
	do
	:: a != 0; a = 0; b = 1 - b; a != 0
	od
}
never {
accept_S0:
	if
	:: (b == 1) -> goto accept_S1
	fi;
accept_S1:
	if
	:: (b != 0 && a != 0) -> goto accept_S0
	:: else -> skip
	fi;
}
