/* wf mode=claim */
bit a, b;
active proctype P0() provided (a == 1)
{
	do
	:: atomic { skip; b = 1 - b; a = 1 - a }
	:: atomic { a == 1; b = 1; skip }
	:: atomic { a = 1 - a; atomic { skip }; skip }
	od
}
active proctype P1()
{
	do
	:: atomic { b = 1 - b }
	od
}
never {
accept_S0:
	if
	:: (a != 1) -> goto accept_S1
	:: (a == 1 && b != 1) -> goto accept_S0
	:: (a != 0 && a != 1) -> goto accept_S1
	fi;
accept_S1:
	if
	:: (a == 0) -> goto accept_S1
	:: (a != 1) -> goto accept_S0
	:: else -> skip
	fi;
}
