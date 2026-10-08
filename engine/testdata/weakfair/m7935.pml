/* wf mode=claim */
bit a, b;
active proctype P0() provided (b == 1)
{
	do
	:: b != 1
	:: skip; if :: a == 0 -> skip :: else -> b = 0; b = 1 - b; skip fi
	:: skip; if :: a == 0 -> b = 1; a == 1 :: else -> b = 0 fi
	od
}
never {
S0:
	if
	:: (a == 0 && a == 0) -> goto accept_S2
	:: (b == 1 && a == 0) -> goto accept_S1
	:: (a == 0 && a == 0) -> goto accept_S2
	fi;
accept_S1:
	if
	:: (a == 0) -> goto S0
	fi;
accept_S2:
	if
	:: (true) -> goto S0
	:: (true) -> goto S0
	fi;
}
