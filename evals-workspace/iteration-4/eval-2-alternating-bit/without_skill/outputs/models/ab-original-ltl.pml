/* ab-original-ltl.pml
 *
 * "Promela - examples/CH3/alternatingbit.pml", byte-for-byte unchanged,
 * with three LTL claims appended.  Nothing in the model itself is touched:
 * the claims only observe the two channels.
 */

#define carrying (len(to_rcvr) > 0)   /* a message is on the wire   */
#define idle      (len(to_rcvr) == 0)  /* the wire is empty          */

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

/* Q1: no message is ever stranded in the data channel: whenever the channel
 *     holds something, it is eventually empty again.                        */
ltl Q1_nothing_stranded { [] (carrying -> <> idle) }

/* Q2: the protocol never stalls: messages are put on the wire forever.      */
ltl Q2_sends_forever    { [] <> carrying }

/* Q3: at most one message and one ack are in flight at any time, so the
 *     FIFO channel delivers in send order and "the k-th message received is
 *     the k-th message sent".                                               */
ltl Q3_one_in_flight    { [] (len(to_rcvr) <= 1 && len(to_sndr) <= 1) }
