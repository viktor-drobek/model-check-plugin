/* A do loop that is the first statement of an atomic block: the process keeps
 * the exclusive control through every iteration and gives it up only when the
 * loop is left. Q can therefore never see n = 1 or n = 2: pan finds no error
 * (7 states stored, -DNOREDUCE). The frontend used to give control up at the
 * back edge of the loop (its target is the entry of the block, which is
 * outside the block), so Q ran in between and the assertion failed. */
byte n;

active proctype P()
{
	atomic {
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
