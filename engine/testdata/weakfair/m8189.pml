/* wf mode=claim */
bit a, b;
byte c;
active proctype P0() provided (c == 2)
{
	do
	:: atomic { a = 1 - a; atomic { c > 0; b = 1; c = 2 }; c = 1; b == 0 }
	od
}
active proctype P1()
{
	do
	:: skip; if :: c == 2 -> c = 0 :: else -> b = 1 fi
	:: atomic { a = 0; a = 1 - a }
	:: atomic { a = 1 - a; a != 0; b = 0 }
	od
}
never {
S0:
	if
	:: (b != 1) -> goto accept_S1
	fi;
accept_S1:
	if
	:: (b == 0) -> goto S0
	:: (a == 0 && a != 0) -> goto S0
	:: (b != 0) -> goto S0
	fi;
}
