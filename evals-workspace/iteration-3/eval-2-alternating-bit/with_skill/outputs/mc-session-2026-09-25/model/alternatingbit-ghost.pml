/* Derived model: the control structure of
 *   "Promela - examples/CH3/alternatingbit.pml"
 *   (sha256 09006d65d6619931809544f074da440a9f3758e7715d7cc7493657164267698d)
 * with four GHOST variables added so that "this message was delivered" can be
 * written as an atom.  The ghosts never guard a statement, so they do not change
 * which transitions are enabled; they only make the receive events observable.
 *
 *   pend1/pend0 : a copy of msg1/msg0 is in to_rcvr and not yet taken out
 *   got1/got0   : toggled on every delivery of msg1/msg0 (a delivery counter mod 2)
 *
 * Each ghost update is fused with the send/receive it records by `atomic`, so it
 * is not a step of its own: the only thing `atomic` hides here is the ghost
 * assignment itself, which no property of the original system can observe.
 */

mtype = { msg0, msg1, ack0, ack1 };

chan	to_sndr = [2] of { mtype };
chan	to_rcvr = [2] of { mtype };

bit	pend1, pend0;	/* ghost */
bit	got1, got0;	/* ghost */

active proctype Sender()
{
again:	atomic { to_rcvr!msg1; pend1 = 1 };
	to_sndr?ack1;
	atomic { to_rcvr!msg0; pend0 = 1 };
	to_sndr?ack0;
	goto again
}

active proctype Receiver()
{
again:	atomic { to_rcvr?msg1; pend1 = 0; got1 = 1 - got1 };
	to_sndr!ack1;
	atomic { to_rcvr?msg0; pend0 = 0; got0 = 1 - got0 };
	to_sndr!ack0;
	goto again
}
