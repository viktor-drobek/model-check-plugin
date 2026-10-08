/* wf mode=claim */
bit a, b;
byte c;
active proctype P0() provided (a != 0)
{
	do
	:: skip; if :: b == 1 -> b = 1; c = 0 :: else -> a = 1 - a; skip; c = (c + 1) % 3 fi
	:: atomic { a = 1 - a; a != 1 }
	od
}
never {
S0:
	if
	:: (b != 0) -> goto accept_S2
	:: (b == 0) -> goto S1
	fi;
S1:
	if
	:: (true) -> goto accept_S2
	:: (b == 1) -> goto S1
	fi;
accept_S2:
	if
	:: (c != 1 && a != 1) -> goto accept_S2
	fi;
}
