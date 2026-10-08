/* wf mode=claim */
bit a, b;
byte c;
active proctype P0() provided (a == 1)
{
	do
	:: skip; if :: a != 1 -> b = 0; c = 0; b = 1 - b; c = 2 :: else -> a = 1 - a; b = 1 - b; c > 2 fi
	od
}
never {
accept_S0:
	if
	:: (c == 0) -> goto accept_S2
	fi;
accept_S1:
	if
	:: (a == 0) -> goto accept_S0
	fi;
accept_S2:
	if
	:: (c > 2) -> goto accept_S0
	:: else -> skip
	fi;
}
