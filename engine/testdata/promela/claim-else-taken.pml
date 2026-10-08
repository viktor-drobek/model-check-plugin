/* The other direction: when the guards of the location are all false, the
 * `else` edge is enabled and the claim runs off its closing brace. a is 1 after
 * the first step, so the claim's `else` fires then: pan -a: 1 error ("end state
 * in claim reached"). */
bit a;

active proctype P()
{
	do
	:: a = 1
	od
}

never {
S0:
	if
	:: (a == 0) -> goto S0
	:: else -> skip
	fi;
}
