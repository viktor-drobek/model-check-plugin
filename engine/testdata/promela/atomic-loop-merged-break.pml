/* A do loop inside an atomic block whose iteration ends in an inner loop left
 * by `else -> break`: the node that closes the iteration is merged with the
 * head of the outer loop only after the loop body was lowered, so the edge into
 * it must still keep the exclusive control. B can then never see n = 1 or
 * n = 2: pan finds no error. An earlier frontend compared raw node ids when it
 * marked the back edges, missed this one, gave the control up mid-loop and
 * reported a false assertion violation. */
byte n;

active proctype A()
{
	atomic {
		do
		:: n < 3 -> n++;
			do
			:: false -> skip
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
