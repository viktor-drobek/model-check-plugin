/* An atomic block whose body is a loop, as one option of an outer loop: the
 * inner loop and the other option share the control location in front of them.
 * Inside the block the process must not take the other option; pan keeps a
 * separate state for the inside of the block (no error, 12 states stored), the
 * engine's one location per point between statements cannot tell the two
 * apart, so the frontend refuses the shape by name instead of answering for a
 * model in which the other option is available inside the block. */
byte n;

active proctype P()
{
	do
	:: atomic {
		do
		:: n < 3 -> n++
		:: else -> break
		od
	}
	:: skip -> n = 0
	od
}

active proctype Q()
{
	assert(n == 0 || n == 3)
}
