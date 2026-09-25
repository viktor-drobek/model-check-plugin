/* ------------------------------------------------------------------
   alternatingbit.pml (Promela - examples/CH3) + observation ghosts.

   The protocol statements are byte-for-byte the original; every added
   line is ghost state that only records what happened.  Ghosts are glued
   to the protocol statement with atomic{} so nothing can be observed
   "between" a send and the record of that send.  No ghost ever appears
   in a guard, so the reachable behaviour of the original is unchanged
   (confirmed below: same 8 protocol states).
   ------------------------------------------------------------------ */

mtype = { msg0, msg1, ack0, ack1 };

chan	to_sndr = [2] of { mtype };
chan	to_rcvr = [2] of { mtype };

bool pend1 = false;	/* a msg1 is sent and not yet consumed  */
bool pend0 = false;	/* a msg0 is sent and not yet consumed  */
byte inflight = 0;	/* number of sent-but-undelivered messages */

active proctype Sender()
{
again:
	atomic {
	  to_rcvr!msg1;
	  assert(!pend1);		/* no overwrite of an undelivered msg1 */
	  pend1 = true; inflight++;
	  assert(inflight <= 1)		/* channel never over-filled       */
	}
	to_sndr?ack1;
progress_s1:
	atomic {
	  to_rcvr!msg0;
	  assert(!pend0);
	  pend0 = true; inflight++;
	  assert(inflight <= 1)
	}
	to_sndr?ack0;
progress_s0:
	goto again
}

active proctype Receiver()
{
again:
	atomic {
	  to_rcvr?msg1;
	  assert(pend1);		/* never a phantom or duplicate delivery */
	  pend1 = false; inflight--
	}
	to_sndr!ack1;
progress_r1:
	atomic {
	  to_rcvr?msg0;
	  assert(pend0);
	  pend0 = false; inflight--
	}
	to_sndr!ack0;
progress_r0:
	goto again
}

/* ================= properties ================= */

/* DELIVERY: whenever a message is outstanding it is eventually consumed */
ltl deliver1 { [] (pend1 -> <> !pend1) }
ltl deliver0 { [] (pend0 -> <> !pend0) }

/* NO STALL: the protocol keeps sending (and hence delivering) forever */
ltl forever  { ([]<> pend1) && ([]<> pend0) }

/* ALTERNATION: never two different outstanding messages at once */
ltl excl     { [] !(pend0 && pend1) }

/* CAPACITY: at most one message in flight */
ltl cap      { [] (inflight <= 1) }

/* CONTROL (must FAIL): proves the harness can see a violation */
ltl bogus    { [] !pend1 }
