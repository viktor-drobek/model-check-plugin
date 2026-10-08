/* The smallest shape of weakfair-progress-r1.pml. P blocks for good at its
 * progress label. np_ is false there, so the np_ automaton, which has just
 * entered its accepting location, has no enabled edge: the product has no
 * infinite run, with or without weak fairness. pan -l and pan -l -f: no error. */
bit a;
active proctype P()
{
	skip;
progress_p:
	a == 1
}
