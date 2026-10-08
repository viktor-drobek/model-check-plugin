/* The receiving side: P is not executable (provided is false), so it cannot
 * take part in the handshake either, and Q never gets past its send. pan: the
 * invalid end state, no assertion violation, 1 state. The engine used to pair
 * the send with the receive of a process whose provided clause is false and
 * reported `assert` violated. */
bit b;
byte x;
chan ch = [0] of { byte };

active proctype P() provided (b == 1)
{
	ch ? x
}

active proctype Q()
{
	ch ! 1;
	assert(false)
}
