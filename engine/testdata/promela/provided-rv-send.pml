/* `provided` gates every transition of a process, the initiating side of a
 * rendezvous send included. b stays 0, so P never runs and Q waits for ever
 * at its receive: pan finds the invalid end state and no assertion violation
 * (1 state). The engine used to let P send: it reported `deadlock` verified
 * and `assert` violated, both wrong. */
bit b;
byte x;
chan ch = [0] of { byte };

active proctype P() provided (b == 1)
{
	ch ! 1
}

active proctype Q()
{
	ch ? x;
	assert(x == 0)
}
