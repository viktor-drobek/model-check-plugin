/* A d_step whose loop leaves at once (`x = 1; break`), the only statement of an
 * outer loop's option: the node after the break is the loop's own head, so the
 * edge `x = 1` goes back to the head by node id, yet it leaves the loop and is
 * the whole of one finite step. pan finds no error. A frontend that marked
 * every edge into the head as a back edge repeated the edge for ever and
 * answered invalid-model ("d_step did not finish"). */
byte x;

active proctype A()
{
	do
	:: d_step { do :: x = 1; break od }
	od
}

active proctype B()
{
	assert(x == 0 || x == 1)
}
