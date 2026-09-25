/* SENSITIVITY EXPERIMENT — NOT the user's model.
 * The ghost model alternatingbit-ghost.pml with ONE change: the medium may lose
 * a frame on the way to the receiver (nondeterministic choice at the send).
 * Everything else — including the absence of a timeout and of retransmission —
 * is as in "Promela - examples/CH3/alternatingbit.pml".
 * Purpose: show what the verdict on the corpus file does and does not cover.
 */

mtype = { msg0, msg1, ack0, ack1 };

chan	to_sndr = [2] of { mtype };
chan	to_rcvr = [2] of { mtype };

bit	pend1, pend0;	/* ghost: submitted and not yet delivered */
bit	got1, got0;	/* ghost: toggled on delivery */

active proctype Sender()
{
again:	pend1 = 1;
	if
	:: to_rcvr!msg1		/* the frame enters the medium */
	:: skip			/* the frame is lost */
	fi;
	to_sndr?ack1;
	pend0 = 1;
	if
	:: to_rcvr!msg0
	:: skip
	fi;
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
