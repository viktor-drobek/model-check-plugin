/* DERIVED MODEL — not a corpus file.
   Source: Promela - examples/CH3/alternatingbit.pml (byte-identical apart from
   the two labels below).  Change declared in the report: the two statements at
   which the Receiver takes a message out of to_rcvr are marked as progress.
   Everything else — channels, capacities, message alphabet, control flow — is
   unchanged. */

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
again:
progress_msg1:
	to_rcvr?msg1;
	to_sndr!ack1;
progress_msg0:
	to_rcvr?msg0;
	to_sndr!ack0;
	goto again
}
