/* DERIVED MODEL — not a corpus file.  Sensitivity run only.
   Source: Promela - examples/CH3/alternatingbit.pml, plus one process that may
   take a message out of to_rcvr and throw it away (a single lossy link in the
   sender -> receiver direction).  Nothing else is changed.
   Purpose: show what the corpus model does when the environment the real
   alternating-bit protocol is built for — a link that loses messages — is put
   back in. */

mtype = { msg0, msg1, ack0, ack1 };

chan	to_sndr = [2] of { mtype };
chan	to_rcvr = [2] of { mtype };

active proctype Sender()
{
again:	to_rcvr!msg1;
	to_sndr?ack1;
	to_rcvr!msg0;
	to_sndr?ack0;
	goto again
}

active proctype Receiver()
{
again:	to_rcvr?msg1;
	to_sndr!ack1;
	to_rcvr?msg0;
	to_sndr!ack0;
	goto again
}

active proctype Link()
{
	do
	:: to_rcvr?msg1	/* message lost in transit */
	:: to_rcvr?msg0	/* message lost in transit */
	od
}
