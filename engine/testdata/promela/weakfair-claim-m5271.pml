/* Found by differential testing against pan (generated model m5271).
 * P0 takes `a == 0; a = 1 - a` and then blocks for good on the third
 * statement of its atomic sequence (a is 1). The never claim sits on the
 * accept location accept_S0, and from there no edge is enabled once a is 1:
 * the claim cannot move, so no infinite run of the product reaches past that
 * state. pan -a and pan -a -f: no error. */
bit a, b;
byte d;
active proctype P0()
{
	do
	:: atomic { a == 0; a = 1 - a; a == 0; d = 0 }
	od
}
never {
accept_S0:
	if
	:: (b == 1) -> goto S1
	:: (a == 0 && a == 0) -> goto accept_S0
	fi;
S1:
	if
	:: (d == 1) -> goto accept_S2
	:: (true) -> goto accept_S0
	fi;
accept_S2:
	if
	:: (b == 1) -> goto accept_S0
	fi;
}
