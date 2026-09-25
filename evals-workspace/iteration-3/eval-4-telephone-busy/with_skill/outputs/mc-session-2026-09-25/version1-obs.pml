/* Rewrite of "Promela - examples/CH14/version1"
   (sha256 acdfcacad083c29ff47cba18de2fb84ec784e9e064e0b64c435497e07121ec3b).
   CHANGES vs. the corpus file (all of them, and each is an assumption of the report):
   1. two observation variables were added, because the engine does not accept
      control-label atoms (proc@label) in LTL:
        byte sw       — shadow of the switch's control label   (IDLE/DIAL/WAIT/CONN/BUSY)
        bit  sub_busy — shadow of the subscriber's Busy label  (1 = off-hook)
   2. in proctype switch every printf(...) statement was REPLACED by the
      assignment that records the label the switch moves to.  printf is a no-op
      step for the engine, so the step structure of the switch is unchanged:
      8 printf steps became 8 assignment steps, one for one.
   3. in proctype subscriber two assignment statements were ADDED (sub_busy = 1
      after the off-hook, sub_busy = 0 on the hang-up).  These are the only new
      steps in the model; they are always executable and touch no channel, so
      they add stutter steps but no new channel behaviour.
   Lag: sw / sub_busy are updated in the step AFTER the rendezvous that leaves a
   label, so "sw == BUSY" covers the Busy label plus the single transient state
   between the onhook rendezvous and the assignment.  The shadow therefore
   over-approximates Busy, which is the safe direction for "cannot stay in Busy". */

#define IDLE 0
#define DIAL 1
#define WAIT 2
#define CONN 3
#define BUSY 4

mtype = { offhook, digits, onhook };

chan tpc = [0] of { mtype };

byte sw = IDLE;
bit  sub_busy = 0;

active proctype subscriber()
{
Idle:	tpc!offhook;
	sub_busy = 1;

Busy:	if
	:: tpc!digits -> goto Busy
	:: tpc!onhook -> sub_busy = 0; goto Idle
	fi
}

active proctype switch()	/* outgoing calls only */
{
Idle:
	if
	:: tpc?offhook -> sw = DIAL; goto Dial		/* was: dialtone */
	fi;
Dial:
	if
	:: tpc?digits -> sw = WAIT; goto Wait		/* was: notone */
	:: tpc?onhook -> sw = IDLE; goto Idle		/* was: notone */
	fi;
Wait:
	if
	:: sw = CONN -> goto Connect;			/* was: ringtone */
	:: sw = BUSY -> goto Busy			/* was: busytone */
	fi;
Connect:
	if
	:: sw = BUSY -> goto Busy			/* was: busytone */
	:: sw = BUSY -> goto Busy			/* was: notone, no remote hang up */
	fi;
Busy:
	tpc?onhook -> sw = IDLE; goto Idle		/* was: notone */
}
