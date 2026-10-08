/* wf mode=claim */
bit a, b;
chan q = [1] of {bit};
active proctype P0() provided (b == 1)
{
	do
	:: atomic { skip; a = 0; a = 0 }
	:: atomic { a = 1 - a }
	od
}
never {
accept_S0:
	if
	:: (a == 0 && b == 1) -> goto accept_S1
	fi;
accept_S1:
	if
	:: (a == 0) -> goto accept_S0
	:: (true) -> goto accept_S0
	fi;
}
