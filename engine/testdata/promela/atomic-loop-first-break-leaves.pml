/* The atomic version of dstep-loop-first-break.pml, with an assertion that can
 * fail: A leaves the atomic block after `x = 1; break`, so B can run and see
 * x == 1 (pan: the assertion is violated). A frontend that marked the edge
 * `x = 1` as a back edge of the loop kept the exclusive control after leaving
 * the block, B never ran, and the engine answered inconclusive. */
byte x;

active proctype A()
{
	do
	:: atomic { do :: x = 1; break od }
	od
}

active proctype B()
{
	assert(x == 0)
}
