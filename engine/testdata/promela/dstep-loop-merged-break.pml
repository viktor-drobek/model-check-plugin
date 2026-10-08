/* The d_step version of atomic-loop-merged-break.pml: the whole loop is one
 * step, so B can never see n = 1 or n = 2. */
byte n, m;

active proctype A()
{
	d_step {
		do
		:: n < 3 -> n++; m = 0;
			do
			:: m < 2 -> m++
			:: else -> break
			od
		:: else -> break
		od
	}
}

active proctype B()
{
	assert(n == 0 || n == 3)
}
