/* P blocks for good after its progress label; np_ is true in the blocked
 * state. pan -l has no stutter extension, and the default move that pan -f
 * adds for a state in which no process can move exists only outside -DNP, so
 * a blocked state has no successor in the non-progress product and no cycle
 * runs through it, with or without weak fairness. pan -l and pan -l -f: no
 * error. (The engine reports the deadlock itself, by its safety search.) */
bit a;
active proctype P()
{
progress_p:
	skip;
	skip;
	a == 1
}
