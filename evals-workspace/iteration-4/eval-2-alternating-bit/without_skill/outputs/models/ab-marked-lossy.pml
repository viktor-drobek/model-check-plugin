/* ab-marked-lossy.pml   --  NEGATIVE CONTROL, EXPECTED TO FAIL.
 *
 * ab-marked.pml with one process added that may silently remove a message
 * from the data channel, i.e. the channel may lose messages.  The real
 * alternating-bit protocol is designed exactly for this case and survives it
 * by retransmitting after a timeout.  The fixture
 * "Promela - examples/CH3/alternatingbit.pml" has no timeout and no
 * retransmission, so under loss it does not recover: R1 fails and the model
 * deadlocks.  This run is what limits the verdict to the fixture as written.
 */

mtype = { msg0, msg1, ack0, ack1 };

chan	to_sndr = [2] of { mtype };
chan	to_rcvr = [2] of { mtype, bool };   /* + ghost field: the mark */

bool marked_sent      = false;   /* ghost */
bool marked_delivered = false;   /* ghost */
bool got;                        /* ghost: mark of the message just received */

active proctype Sender()
{	bool mk;                     /* ghost */

again:
	atomic {
	  if
	  :: !marked_sent -> mk = true; marked_sent = true   /* mark THIS one */
	  :: mk = false                                      /* or a later one */
	  fi;
	  to_rcvr!msg1(mk)
	};
	to_sndr?ack1;
	atomic {
	  if
	  :: !marked_sent -> mk = true; marked_sent = true
	  :: mk = false
	  fi;
	  to_rcvr!msg0(mk)
	};
	to_sndr?ack0;
	goto again
}

active proctype Receiver()
{
again:
	atomic {
	  to_rcvr?msg1(got);
	  if
	  :: got  -> marked_delivered = true
	  :: else -> skip
	  fi
	};
	to_sndr!ack1;
	atomic {
	  to_rcvr?msg0(got);
	  if
	  :: got  -> marked_delivered = true
	  :: else -> skip
	  fi
	};
	to_sndr!ack0;
	goto again
}

active proctype Channel()
{	bool junk;
	do
	:: atomic { to_rcvr?msg1(junk) }   /* message lost */
	:: atomic { to_rcvr?msg0(junk) }   /* message lost */
	od
}

/* R1: EVERY MESSAGE IS DELIVERED - the property asked for.                  */
ltl R1_every_message_delivered_LOSSY { [] (marked_sent -> <> marked_delivered) }

/* R2: and it is delivered without the sender having to wait forever:
 *     the wire never stays occupied.                                        */
ltl R2_nothing_stranded        { [] ((len(to_rcvr) > 0) -> <> (len(to_rcvr) == 0)) }

/* R3: at most one message in flight, so the FIFO channel hands the receiver
 *     the messages in exactly the order they were sent: no loss, no
 *     duplication, no reordering.                                           */
ltl R3_one_in_flight           { [] (len(to_rcvr) <= 1 && len(to_sndr) <= 1) }
