/* G1: local variables are qualified by the process instance in counterexamples */
active [2] proctype P()
{	byte k;
	k = _pid + 1;
	assert(k == 1)
}
