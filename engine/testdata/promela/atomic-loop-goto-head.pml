/* A do loop inside an atomic block whose option jumps back to the loop's label
 * with `goto`: the goto merges the node it leaves with the head, so the edge
 * before it goes back to the head although its raw target is another node. It
 * keeps the exclusive control like every back edge of the loop: B can never
 * see n = 1 or n = 2 (pan finds no error). An earlier frontend compared raw
 * node ids when it marked the back edges and gave the control up here. */
byte n;

active proctype A()
{
	atomic {
	L:	do
		:: n < 3 -> n++; goto L
		:: else -> break
		od
	}
}

active proctype B()
{
	assert(n == 0 || n == 3)
}
