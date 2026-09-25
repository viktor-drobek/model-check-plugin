/* alternatingbit.pml with ONE single message loss permitted.
   Sender/Receiver code is unchanged.  A daemon may steal at most one
   datagram from either channel, then stops.  This is the weakest
   possible lossy-channel assumption: the very assumption the real
   alternating bit protocol is designed to survive. */

mtype = { msg0, msg1, ack0, ack1 };

chan	to_sndr = [2] of { mtype };
chan	to_rcvr = [2] of { mtype };

byte budget = 1;

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

active proctype Loss()
{	mtype m;
	do
	:: atomic { (budget > 0 && len(to_rcvr) > 0) -> to_rcvr?m; budget-- }
	:: atomic { (budget > 0 && len(to_sndr) > 0) -> to_sndr?m; budget-- }
	:: budget == 0 -> break
	od
}
