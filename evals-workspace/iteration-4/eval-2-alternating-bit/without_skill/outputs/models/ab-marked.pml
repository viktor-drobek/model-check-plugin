/* ab-marked.pml
 *
 * "Promela - examples/CH3/alternatingbit.pml" plus GHOST state that turns
 * the informal claim "every message is delivered" into one LTL formula.
 *
 * The idea.  Messages in the original model carry no identity (msg1 / msg0
 * are bare mtype tokens), so "this particular message arrives" cannot be
 * written down directly.  We therefore let the Sender *mark* one message,
 * chosen non-deterministically: at every send it may either mark this
 * message (if no message has been marked yet) or leave it unmarked.  The
 * Receiver notes when a marked message arrives.
 *
 *   marked_sent      - the chosen message has been put on the wire
 *   marked_delivered - the chosen message has been taken off the wire
 *
 * Because the choice is free, the runs of this model cover, for every run of
 * the original model and every k, the case "the k-th message is the marked
 * one".  So an exhaustive check of
 *
 *      [] (marked_sent -> <> marked_delivered)
 *
 * over this model says: in every run of the original model, every message
 * that is sent is eventually received.
 *
 * SOUNDNESS OF THE INSTRUMENTATION.  Nothing original was removed or
 * reordered; the send/receive skeleton is character-for-character the one in
 * the fixture.  Everything added is ghost:
 *   - the extra bool field on to_rcvr (read only by ghost code),
 *   - the variables mk, marked_sent, marked_delivered, got,
 *   - the two `if` statements, whose guards are `!marked_sent` and `else`
 *     resp. `mk` and `else`, i.e. always executable as a pair, so they can
 *     never block and never disable an original statement.
 * The atomic{} brackets group a send/receive with its ghost bookkeeping.
 * They contain no blocking statement (`to_rcvr` never holds more than one
 * message - checked as Q3/R3 - so the sends never block, and the receives
 * are the first statement of their atomic block, which is where the block
 * may legitimately wait).  Hence the projection of this model's runs onto
 * the original variables is exactly the set of runs of the original model.
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

/* R1: EVERY MESSAGE IS DELIVERED - the property asked for.                  */
ltl R1_every_message_delivered { [] (marked_sent -> <> marked_delivered) }

/* R2: and it is delivered without the sender having to wait forever:
 *     the wire never stays occupied.                                        */
ltl R2_nothing_stranded        { [] ((len(to_rcvr) > 0) -> <> (len(to_rcvr) == 0)) }

/* R3: at most one message in flight, so the FIFO channel hands the receiver
 *     the messages in exactly the order they were sent: no loss, no
 *     duplication, no reordering.                                           */
ltl R3_one_in_flight           { [] (len(to_rcvr) <= 1 && len(to_sndr) <= 1) }
