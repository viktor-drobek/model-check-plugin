/* Rewrite of "Promela - examples/CH14/version1"
   (sha256 acdfcacad083c29ff47cba18de2fb84ec784e9e064e0b64c435497e07121ec3b).
   CHANGE vs. the corpus file, and the only one:
     the SUBSCRIBER's control label  Idle:  is renamed to  progress_Idle:
     (and the `goto Idle` of the subscriber follows the rename).
   This is the second reading of the question: "the telephone set does not stay
   off-hook (label Busy) forever" — progress is the subscriber going back on-hook
   to the idle state.  No variable, statement or channel operation was added or
   removed, so the set of runs is identical to the corpus model. */

mtype = { offhook, digits, onhook };

chan tpc = [0] of { mtype };

active proctype subscriber()
{
progress_Idle:	tpc!offhook;

Busy:	if
	:: tpc!digits -> goto Busy
	:: tpc!onhook -> goto progress_Idle
	fi
}

active proctype switch()	/* outgoing calls only */
{
Idle:
	if
	:: tpc?offhook -> printf("MSC: dialtone\n"); goto Dial
	fi;
Dial:
	if
	:: tpc?digits -> printf("MSC: notone\n"); goto Wait
	:: tpc?onhook -> printf("MSC: notone\n"); goto Idle
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
	tpc?onhook -> printf("MSC: notone\n"); goto Idle
}
