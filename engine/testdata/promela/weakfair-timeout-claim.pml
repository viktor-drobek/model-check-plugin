/* A system whose only moves are `timeout` moves, and a never claim with two
 * enabled edges in its first location (a nondeterministic choice, as the claim
 * of an LTL formula has): the claim may stay where it is or go to its accepting
 * location, where it loops. The accepting run exists: P keeps firing its
 * timeout, the claim sits on accept_S1. pan -a: 2 errors, pan -a -f: 1 error.
 * The product search enumerated the system's moves once per claim edge but
 * decided whether `timeout` is true from a counter that the first claim edge
 * had already raised, so only the first claim edge ever got the timeout moves:
 * the engine said `verified`, with and without fairness. */
bit a;

active proctype P()
{
	do
	:: timeout
	od
}

never {
c0:
	if
	:: (1) -> goto c0
	:: (1) -> goto accept_S1
	fi;
accept_S1:
	do
	:: (1) -> goto accept_S1
	od
}
