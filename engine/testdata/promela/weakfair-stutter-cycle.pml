/* Like weakfair-stutter-accept.pml, but the claim alternates between an
 * accepting and a non-accepting location while the blocked state repeats.
 * The stuttering run is an acceptance cycle and, with every process blocked,
 * weakly fair. pan -a: error. pan 6.5.2 -a -f: no error, because since
 * SPIN 6.5.2 (pan.c: "9/2025 added !fairness") the claim's own stutter step
 * is off under -f and the default move that replaces it exists only while a
 * fairness round is open. The engine keeps the stutter extension under weak
 * fairness, as without it. */
bit a;
active proctype P()
{
	a == 1
}
never {
accept_S0:
	if
	:: (true) -> goto S1
	fi;
S1:
	if
	:: (true) -> goto accept_S0
	fi;
}
