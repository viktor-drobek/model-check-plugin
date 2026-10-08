/* `else` in a never claim is enabled only when no other edge of the claim's
 * location is. Here a stays 0 for ever, so the claim keeps taking its first
 * edge and never falls off its closing brace: pan -a and pan -a -f find no
 * error. The engine treated an `else` edge as always enabled (the edges of a
 * claim are enumerated by their guards, and an `else` has none), so the claim
 * could leave through it at once and the engine reported "end state in claim
 * reached", a violation that is no run. */
bit a;

active proctype P()
{
	do
	:: a = 0
	od
}

never {
S0:
	if
	:: (a == 0) -> goto S0
	:: else -> skip
	fi;
}
