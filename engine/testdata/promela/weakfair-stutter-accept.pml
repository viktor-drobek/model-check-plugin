/* The other side of weakfair-claim-blocked.pml: P blocks for good, and the
 * never claim can move: it loops on accept_S0 whatever the state is. SPIN's
 * stutter extension repeats the blocked state forever and the claim goes on
 * moving alone, so the run is an acceptance cycle; every process is blocked
 * in it, so it is weakly fair. pan -a: error. pan -a -f: error. */
bit a;
active proctype P()
{
	a == 1
}
never {
accept_S0:
	if
	:: (true) -> goto accept_S0
	fi;
}
