/* wf mode=claim */
bit a, b;
chan q = [1] of {bit};
active proctype P0() provided (b == 1)
{
	do
	:: atomic { b = 1 }
	od
}
active proctype P1()
{
	do
	:: atomic { skip; a = 0; b = 1 - b }
	:: b = 1 - b; skip
	:: atomic { b == 0; a = 0; skip }
	od
}
never {
accept_S0:
	if
	:: (b == 0) -> goto accept_S0
	fi;
accept_S1:
	if
	:: (a == 1) -> goto accept_S1
	:: (b != 1 && a == 1) -> goto accept_S0
	:: (true) -> goto accept_S1
	fi;
}
