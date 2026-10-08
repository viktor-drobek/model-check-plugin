/* The same loop as a d_step block: the whole loop is one step. pan: no error,
 * 7 states stored. The frontend used to lower the loop as a sequence of steps
 * of its own (the back edge was not part of the block), so the assertion
 * failed in the engine. */
byte n;

active proctype P()
{
	d_step {
		do
		:: n < 3 -> n++
		:: else -> break
		od
	}
}

active proctype Q()
{
	assert(n == 0 || n == 3)
}
