/* wf mode=claim */
bit a, b;
byte c;
chan rv = [0] of {bit};
active proctype P0()
{
	do
	:: skip; if :: a == 1 -> b == 1; a = 0; rv?b; rv?a :: else -> a = 1 - a; a = 1 - a fi
	:: atomic { b = 1; d_step { a = 1 - a; b = 1 - b }; c = 2; rv!1 }
	:: atomic { c = 0; rv?a }
	od
}
active proctype P1()
{
	do
	:: atomic { b == 1; d_step { c = 0; a = 1 - a }; c == 1 }
	od
}
never {
accept_S0:
	if
	:: (true) -> goto accept_S1
	fi;
accept_S1:
	if
	:: (b == 1) -> goto accept_S1
	fi;
}
