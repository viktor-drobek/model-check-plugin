/* Rewrite of "Promela - examples/CH14/version1"
   (sha256 acdfcacad083c29ff47cba18de2fb84ec784e9e064e0b64c435497e07121ec3b).
   CHANGE vs. the corpus file, and the only one:
     the switch's control label  Idle:  is renamed to  progress_Idle:
     (and the two `goto Idle` inside proctype switch follow the rename).
   Rationale: the engine does not accept control-label atoms (proc@label), so
   "the switch is out of Busy / the call has ended" is expressed by a progress
   label.  A label whose name starts with "progress" marks the state in which the
   switch is idle and able to take a new call; the non-progress search then looks
   for an infinite run in which the switch never returns to that state.
   No variable, no statement and no channel operation was added or removed, so the
   set of runs is identical to the corpus model. */

mtype = { offhook, digits, onhook };

chan tpc = [0] of { mtype };

active proctype subscriber()
{
Idle:	tpc!offhook;

Busy:	if
	:: tpc!digits -> goto Busy
	:: tpc!onhook -> goto Idle
	fi
}

active proctype switch()	/* outgoing calls only */
{
progress_Idle:
	if
	:: tpc?offhook -> printf("MSC: dialtone\n"); goto Dial
	fi;
Dial:
	if
	:: tpc?digits -> printf("MSC: notone\n"); goto Wait
	:: tpc?onhook -> printf("MSC: notone\n"); goto progress_Idle
	fi;
Wait:
	if
	:: printf("MSC: ringtone\n") -> goto Connect;
	:: printf("MSC: busytone\n") -> goto Busy
	fi;
Connect:
	if
	:: printf("MSC: busytone\n") -> goto Busy
	:: printf("MSC: notone\n")   -> goto Busy	/* no remote hang up */
	fi;
Busy:
	tpc?onhook -> printf("MSC: notone\n"); goto progress_Idle
}
