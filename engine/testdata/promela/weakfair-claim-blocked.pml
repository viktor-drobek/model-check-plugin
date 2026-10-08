/* The smallest shape of weakfair-claim-m5271.pml. P sets a to 1 and then
 * blocks for good on `a == 0`. The never claim sits on accept_S0 and its only
 * edge needs a == 0, so once a is 1 the claim cannot move: the product has no
 * infinite run, with or without weak fairness. pan -a and pan -a -f: no error. */
bit a;
active proctype P()
{
	a = 1;
	a == 0
}
never {
accept_S0:
	if
	:: (a == 0) -> goto accept_S0
	fi;
}
