/* wf mode=claim */
bit a, b;
active proctype P0() provided (b == 1)
{
	do
	:: atomic { a = 1 - a; b = 1; b == 0 }
	od
}
never {
S0:
	if
	:: (a == 0) -> goto accept_S1
	:: (b == 1) -> goto S0
	fi;
accept_S1:
	if
	:: (a == 0) -> goto S0
	:: (a == 1) -> goto S0
	:: else -> skip
	fi;
}
